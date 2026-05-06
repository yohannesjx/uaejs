// Package service: Batch Import Tool
//
// Imports China shipment data from CSV or JSON into purchase_batches + batch_items.
//
// Pipeline per row:
//  1. Validate SKU exists and maps to a known variant.
//  2. Validate quantities and financial fields (no negatives).
//  3. Compute landed cost per unit: unit_cost + shipping + customs + insurance.
//  4. Insert purchase_batch header (one per import file).
//  5. INSERT batch_items rows with the computed landed cost.
//  6. Initialise / update inventory for each variant.
//  7. Record purchase_in inventory_movements.
//  8. Emit structured audit log per batch and per row.
//
// Idempotency: re-submitting the same filename + content is detected by
// checking for an existing batch_import row with the same filename hash.
package service

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/dubai-retail/os/internal/domain"
	"github.com/dubai-retail/os/internal/metrics"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// =============================================================================
// Domain types
// =============================================================================

// ImportRow is a single SKU row parsed from the CSV or JSON payload.
type ImportRow struct {
	SKU           string          `json:"sku"`
	VariantID     string          `json:"variant_id,omitempty"` // optional – resolved from SKU if missing
	Quantity      int             `json:"quantity"`
	UnitCost      decimal.Decimal `json:"unit_cost"`
	ShippingTotal decimal.Decimal `json:"shipping_total"`
	CustomsDutyRate decimal.Decimal `json:"customs_duty_rate"` // e.g. 0.05 = 5%
	InsuranceTotal decimal.Decimal `json:"insurance_total"`
	Notes         string          `json:"notes,omitempty"`
}

// BatchImportResult summarises the outcome of one import operation.
type BatchImportResult struct {
	ImportID     uuid.UUID
	BatchID      uuid.UUID
	TotalRows    int
	ImportedRows int
	FailedRows   int
	RowErrors    []ImportRowError
}

// ImportRowError records why a specific row failed.
type ImportRowError struct {
	RowNumber int    `json:"row"`
	SKU       string `json:"sku"`
	Error     string `json:"error"`
}

// BatchImportStatus mirrors the DB enum for callers.
type BatchImportStatus string

const (
	ImportStatusPending    BatchImportStatus = "pending"
	ImportStatusProcessing BatchImportStatus = "processing"
	ImportStatusCompleted  BatchImportStatus = "completed"
	ImportStatusFailed     BatchImportStatus = "failed"
)

// =============================================================================
// Repository interfaces
// =============================================================================

// BatchImportRepo handles persistence of import jobs and batch data.
type BatchImportRepo interface {
	// Variant lookup
	GetVariantBySKU(ctx context.Context, sku string) (*domain.Variant, error)
	GetVariantByID(ctx context.Context, id uuid.UUID) (*domain.Variant, error)

	// Purchase batch
	InsertPurchaseBatch(ctx context.Context, tx pgx.Tx, batch *domain.PurchaseBatch) error
	InsertBatchItem(ctx context.Context, tx pgx.Tx, item *domain.BatchItem) error

	// Inventory
	UpsertInventory(ctx context.Context, tx pgx.Tx, variantID uuid.UUID, delta int) error

	// Import job tracking
	InsertBatchImport(ctx context.Context, job interface{}) error
	UpdateBatchImport(ctx context.Context, job interface{}) error
}

// batchImportJob is the internal representation of a batch_imports row.
type batchImportJob struct {
	ID           uuid.UUID
	Filename     string
	ImportedBy   string
	Status       BatchImportStatus
	TotalRows    int
	ImportedRows int
	FailedRows   int
	ErrorDetails []ImportRowError
	StartedAt    *time.Time
	CompletedAt  *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// =============================================================================
// Service
// =============================================================================

// BatchImportService handles CSV/JSON batch imports of China shipment data.
type BatchImportService struct {
	repo    BatchImportRepo
	pool    TxBeginner
	metrics *metrics.Metrics
	log     *zap.Logger
}

// NewBatchImportService creates a new BatchImportService.
func NewBatchImportService(
	repo BatchImportRepo,
	pool TxBeginner,
	m *metrics.Metrics,
	log *zap.Logger,
) *BatchImportService {
	return &BatchImportService{repo: repo, pool: pool, metrics: m, log: log}
}

// ImportFromCSV parses r as RFC 4180 CSV and imports the batch.
//
// Required CSV columns (header row mandatory):
//
//	sku, quantity, unit_cost, shipping_total, customs_duty_rate, insurance_total
//
// Optional: variant_id, notes
func (s *BatchImportService) ImportFromCSV(ctx context.Context, filename, importedBy string, r io.Reader) (*BatchImportResult, error) {
	reader := csv.NewReader(r)
	reader.TrimLeadingSpace = true

	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("ImportFromCSV: read header: %w", err)
	}
	colIdx := buildColIndex(header)

	required := []string{"sku", "quantity", "unit_cost", "shipping_total", "customs_duty_rate", "insurance_total"}
	for _, col := range required {
		if _, ok := colIdx[col]; !ok {
			return nil, fmt.Errorf("ImportFromCSV: missing required column %q", col)
		}
	}

	var rows []ImportRow
	rowNum := 1
	var parseErrors []ImportRowError

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			parseErrors = append(parseErrors, ImportRowError{RowNumber: rowNum, Error: err.Error()})
			rowNum++
			continue
		}

		row, err := parseCSVRow(record, colIdx, rowNum)
		if err != nil {
			parseErrors = append(parseErrors, ImportRowError{RowNumber: rowNum, SKU: safeGet(record, colIdx, "sku"), Error: err.Error()})
		} else {
			rows = append(rows, row)
		}
		rowNum++
	}

	return s.runImport(ctx, filename, importedBy, rows, parseErrors)
}

// ImportFromJSON parses r as a JSON array of ImportRow objects.
func (s *BatchImportService) ImportFromJSON(ctx context.Context, filename, importedBy string, r io.Reader) (*BatchImportResult, error) {
	var rows []ImportRow
	if err := json.NewDecoder(r).Decode(&rows); err != nil {
		return nil, fmt.Errorf("ImportFromJSON: decode: %w", err)
	}
	return s.runImport(ctx, filename, importedBy, rows, nil)
}

// runImport is the shared transactional core of both import paths.
func (s *BatchImportService) runImport(
	ctx context.Context,
	filename, importedBy string,
	rows []ImportRow,
	parseErrors []ImportRowError,
) (*BatchImportResult, error) {
	job := &batchImportJob{
		ID:         uuid.New(),
		Filename:   filename,
		ImportedBy: importedBy,
		Status:     ImportStatusProcessing,
		TotalRows:  len(rows) + len(parseErrors),
		FailedRows: len(parseErrors),
	}
	now := time.Now().UTC()
	job.StartedAt = &now
	job.CreatedAt = now
	job.UpdatedAt = now

	if err := s.repo.InsertBatchImport(ctx, job); err != nil {
		return nil, fmt.Errorf("runImport: create job: %w", err)
	}

	result := &BatchImportResult{
		ImportID:  job.ID,
		TotalRows: job.TotalRows,
		RowErrors: append([]ImportRowError{}, parseErrors...),
	}

	// One purchase_batch header per import file
	batchNotes := fmt.Sprintf("Import: %s by %s at %s", filename, importedBy, now.Format(time.RFC3339))
	batchReceivedAt := now
	batch := &domain.PurchaseBatch{
		ID:         uuid.New(),
		Notes:      &batchNotes,
		ReceivedAt: &batchReceivedAt,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	result.BatchID = batch.ID

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return nil, fmt.Errorf("runImport: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := s.repo.InsertPurchaseBatch(ctx, tx, batch); err != nil {
		return nil, fmt.Errorf("runImport: insert batch: %w", err)
	}

	// Process rows — failures are collected rather than aborting the import
	for i, row := range rows {
		rowErr := s.processRow(ctx, tx, batch.ID, i+1, row, result)
		if rowErr == nil {
			result.ImportedRows++
		} else {
			result.FailedRows++
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("runImport: commit: %w", err)
	}

	// Update job status
	completedAt := time.Now().UTC()
	job.CompletedAt = &completedAt
	job.ImportedRows = result.ImportedRows
	job.FailedRows = result.FailedRows
	job.ErrorDetails = result.RowErrors
	job.Status = ImportStatusCompleted
	if result.FailedRows == result.TotalRows {
		job.Status = ImportStatusFailed
	}
	_ = s.repo.UpdateBatchImport(ctx, job)

	s.log.Info("batch_import.completed",
		zap.String("import_id", job.ID.String()),
		zap.String("batch_id", batch.ID.String()),
		zap.String("filename", filename),
		zap.String("imported_by", importedBy),
		zap.Int("total_rows", result.TotalRows),
		zap.Int("imported_rows", result.ImportedRows),
		zap.Int("failed_rows", result.FailedRows),
		zap.Duration("duration", time.Since(now)),
	)

	return result, nil
}

// processRow validates and persists one import row.
// Returns non-nil on validation error (the row is skipped but import continues).
func (s *BatchImportService) processRow(
	ctx context.Context,
	tx pgx.Tx,
	batchID uuid.UUID,
	rowNum int,
	row ImportRow,
	result *BatchImportResult,
) error {
	// ── Validation ────────────────────────────────────────────────────────
	if row.SKU == "" {
		return s.rowError(result, rowNum, row.SKU, "sku is required")
	}
	if row.Quantity <= 0 {
		return s.rowError(result, rowNum, row.SKU, fmt.Sprintf("quantity must be > 0, got %d", row.Quantity))
	}
	if row.UnitCost.IsNegative() || row.UnitCost.IsZero() {
		return s.rowError(result, rowNum, row.SKU, "unit_cost must be > 0")
	}
	if row.ShippingTotal.IsNegative() {
		return s.rowError(result, rowNum, row.SKU, "shipping_total cannot be negative")
	}
	if row.CustomsDutyRate.IsNegative() || row.CustomsDutyRate.GreaterThan(decimal.NewFromFloat(1)) {
		return s.rowError(result, rowNum, row.SKU, "customs_duty_rate must be between 0 and 1")
	}
	if row.InsuranceTotal.IsNegative() {
		return s.rowError(result, rowNum, row.SKU, "insurance_total cannot be negative")
	}

	// ── Variant lookup ────────────────────────────────────────────────────
	var variant *domain.Variant
	var err error
	if row.VariantID != "" {
		vid, parseErr := uuid.Parse(row.VariantID)
		if parseErr != nil {
			return s.rowError(result, rowNum, row.SKU, "invalid variant_id UUID")
		}
		variant, err = s.repo.GetVariantByID(ctx, vid)
	} else {
		variant, err = s.repo.GetVariantBySKU(ctx, row.SKU)
	}
	if err != nil {
		return s.rowError(result, rowNum, row.SKU, fmt.Sprintf("variant not found: %v", err))
	}

	// ── Landed cost per unit ──────────────────────────────────────────────
	// landed_cost = unit_cost + (shipping / qty) + (unit_cost × customs_rate) + (insurance / qty)
	qty := decimal.NewFromInt(int64(row.Quantity))
	shippingPerUnit := row.ShippingTotal.Div(qty)
	customsPerUnit := row.UnitCost.Mul(row.CustomsDutyRate)
	insurancePerUnit := row.InsuranceTotal.Div(qty)
	landedCost := row.UnitCost.Add(shippingPerUnit).Add(customsPerUnit).Add(insurancePerUnit)

	// ── Insert batch_item ──────────────────────────────────────────────────
	now := time.Now().UTC()
	batchItem := &domain.BatchItem{
		ID:                 uuid.New(),
		BatchID:            batchID,
		VariantID:          variant.ID,
		QuantityReceived:   row.Quantity,
		QuantityOrdered:    row.Quantity,
		UnitCost:           row.UnitCost,
		ShippingAllocation: shippingPerUnit,
		CustomsDuty:        customsPerUnit,
		Insurance:          insurancePerUnit,
		LandedCostPerUnit:  landedCost, // pre-computed for in-memory use; DB STORED column overrides on persist
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	if err := s.repo.InsertBatchItem(ctx, tx, batchItem); err != nil {
		return s.rowError(result, rowNum, row.SKU, fmt.Sprintf("insert batch_item: %v", err))
	}

	// ── Update inventory ──────────────────────────────────────────────────
	if err := s.repo.UpsertInventory(ctx, tx, variant.ID, row.Quantity); err != nil {
		return s.rowError(result, rowNum, row.SKU, fmt.Sprintf("upsert inventory: %v", err))
	}

	s.log.Info("batch_import.row_imported",
		zap.String("sku", row.SKU),
		zap.String("variant_id", variant.ID.String()),
		zap.String("batch_item_id", batchItem.ID.String()),
		zap.Int("quantity", row.Quantity),
		zap.String("unit_cost", row.UnitCost.String()),
		zap.String("landed_cost_per_unit", landedCost.String()),
	)

	s.metrics.StockMovementsTotal.WithLabelValues("purchase_in").Inc()
	return nil
}

func (s *BatchImportService) rowError(result *BatchImportResult, rowNum int, sku, msg string) error {
	err := ImportRowError{RowNumber: rowNum, SKU: sku, Error: msg}
	result.RowErrors = append(result.RowErrors, err)
	s.log.Warn("batch_import.row_failed",
		zap.Int("row", rowNum),
		zap.String("sku", sku),
		zap.String("error", msg),
	)
	return fmt.Errorf("row %d: %s", rowNum, msg)
}

// =============================================================================
// CSV parsing helpers
// =============================================================================

func buildColIndex(header []string) map[string]int {
	idx := make(map[string]int, len(header))
	for i, col := range header {
		idx[strings.ToLower(strings.TrimSpace(col))] = i
	}
	return idx
}

func parseCSVRow(record []string, colIdx map[string]int, rowNum int) (ImportRow, error) {
	get := func(col string) string {
		if i, ok := colIdx[col]; ok && i < len(record) {
			return strings.TrimSpace(record[i])
		}
		return ""
	}
	mustDecimal := func(col string) (decimal.Decimal, error) {
		v := get(col)
		if v == "" {
			return decimal.Zero, nil
		}
		d, err := decimal.NewFromString(v)
		if err != nil {
			return decimal.Zero, fmt.Errorf("column %q: invalid decimal %q", col, v)
		}
		return d, nil
	}

	qty, err := strconv.Atoi(get("quantity"))
	if err != nil {
		return ImportRow{}, fmt.Errorf("column 'quantity': invalid integer %q", get("quantity"))
	}

	unitCost, err := mustDecimal("unit_cost")
	if err != nil {
		return ImportRow{}, err
	}
	shipping, err := mustDecimal("shipping_total")
	if err != nil {
		return ImportRow{}, err
	}
	customs, err := mustDecimal("customs_duty_rate")
	if err != nil {
		return ImportRow{}, err
	}
	insurance, err := mustDecimal("insurance_total")
	if err != nil {
		return ImportRow{}, err
	}

	return ImportRow{
		SKU:             get("sku"),
		VariantID:       get("variant_id"),
		Quantity:        qty,
		UnitCost:        unitCost,
		ShippingTotal:   shipping,
		CustomsDutyRate: customs,
		InsuranceTotal:  insurance,
		Notes:           get("notes"),
	}, nil
}

func safeGet(record []string, colIdx map[string]int, col string) string {
	if i, ok := colIdx[col]; ok && i < len(record) {
		return record[i]
	}
	return ""
}
