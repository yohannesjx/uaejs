package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/dubai-retail/os/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
)

// BatchImportRepository implements service.BatchImportRepo for PostgreSQL.
type BatchImportRepository struct {
	pool *pgxpool.Pool
}

// ── Variant lookups ───────────────────────────────────────────────────────────

func (r *BatchImportRepository) GetVariantBySKU(ctx context.Context, sku string) (*domain.Variant, error) {
	var v domain.Variant
	err := r.pool.QueryRow(ctx, `
		SELECT id, product_id, sku, size, color, barcode, created_at, updated_at
		  FROM variants WHERE sku = $1`, sku,
	).Scan(&v.ID, &v.ProductID, &v.SKU, &v.Size, &v.Color, &v.Barcode, &v.CreatedAt, &v.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("GetVariantBySKU %q: %w", sku, err)
	}
	return &v, nil
}

func (r *BatchImportRepository) GetVariantByID(ctx context.Context, id uuid.UUID) (*domain.Variant, error) {
	var v domain.Variant
	err := r.pool.QueryRow(ctx, `
		SELECT id, product_id, sku, size, color, barcode, created_at, updated_at
		  FROM variants WHERE id = $1`, id,
	).Scan(&v.ID, &v.ProductID, &v.SKU, &v.Size, &v.Color, &v.Barcode, &v.CreatedAt, &v.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("GetVariantByID %s: %w", id, err)
	}
	return &v, nil
}

// ── Batch persistence ─────────────────────────────────────────────────────────

func (r *BatchImportRepository) InsertPurchaseBatch(ctx context.Context, tx pgx.Tx, batch *domain.PurchaseBatch) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO purchase_batches (id, notes, received_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $4)`,
		batch.ID, batch.Notes, batch.ReceivedAt, batch.CreatedAt,
	)
	return err
}

func (r *BatchImportRepository) InsertBatchItem(ctx context.Context, tx pgx.Tx, item *domain.BatchItem) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO batch_items
		       (id, batch_id, variant_id, quantity_received, quantity_in_stock,
		        unit_cost, shipping_per_unit, customs_duty_rate, insurance_per_unit,
		        created_at, updated_at)
		VALUES ($1, $2, $3, $4, $4, $5, $6, $7, $8, $9, $9)`,
		item.ID, item.BatchID, item.VariantID,
		item.QuantityReceived,
		item.UnitCost, item.ShippingAllocation, item.CustomsDuty, item.Insurance,
		item.CreatedAt,
	)
	return err
}

// UpsertInventory increments (or initialises) the inventory row for a variant.
func (r *BatchImportRepository) UpsertInventory(ctx context.Context, tx pgx.Tx, variantID uuid.UUID, delta int) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO inventory (variant_id, quantity_reserved, created_at, updated_at)
		VALUES ($1, 0, NOW(), NOW())
		ON CONFLICT (variant_id) DO NOTHING`,
		variantID,
	)
	if err != nil {
		return err
	}
	// Also record a purchase_in movement for the audit trail
	_, err = tx.Exec(ctx, `
		INSERT INTO inventory_movements
		       (id, variant_id, movement_type, quantity_delta, notes, created_at)
		VALUES ($1, $2, 'purchase_in', $3, 'batch import', NOW())`,
		uuid.New(), variantID, delta,
	)
	return err
}

// ── Import job tracking ────────────────────────────────────────────────────────

// batchImportJobRow is the DB-level representation shared between insert and update.
type batchImportJobRow struct {
	ID           uuid.UUID
	Filename     string
	ImportedBy   string
	Status       string
	TotalRows    int
	ImportedRows int
	FailedRows   int
	ErrorDetails []byte // JSONB
	StartedAt    *time.Time
	CompletedAt  *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (r *BatchImportRepository) InsertBatchImport(ctx context.Context, jobIface interface{}) error {
	job := toJobRow(jobIface)
	errJSON, _ := json.Marshal(job.ErrorDetails)
	_, err := r.pool.Exec(ctx, `
		INSERT INTO batch_imports
		       (id, filename, imported_by, status, total_rows, imported_rows, failed_rows,
		        error_details, started_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $10)`,
		job.ID, job.Filename, job.ImportedBy, job.Status,
		job.TotalRows, job.ImportedRows, job.FailedRows,
		errJSON, job.StartedAt, job.CreatedAt,
	)
	return err
}

func (r *BatchImportRepository) UpdateBatchImport(ctx context.Context, jobIface interface{}) error {
	job := toJobRow(jobIface)
	errJSON, _ := json.Marshal(job.ErrorDetails)
	_, err := r.pool.Exec(ctx, `
		UPDATE batch_imports
		   SET status = $2, imported_rows = $3, failed_rows = $4,
		       error_details = $5, completed_at = $6, updated_at = NOW()
		 WHERE id = $1`,
		job.ID, job.Status, job.ImportedRows, job.FailedRows, errJSON, job.CompletedAt,
	)
	return err
}

// UpdateSandboxStatus persists the ASP sandbox result on an order_invoices row.
func (r *BatchImportRepository) UpdateSandboxStatus(
	ctx context.Context,
	invoiceID uuid.UUID,
	status domain.SandboxStatus,
	aspRespID string,
	validationErrors []string,
) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE order_invoices
		   SET sandbox_status = $2, sandbox_resp_id = $3,
		       sandbox_errors = $4, sandbox_submitted_at = NOW()
		 WHERE id = $1`,
		invoiceID, string(status), aspRespID, validationErrors,
	)
	return err
}

// ── toJobRow converts the service-internal job type to a flat row struct ───────

func toJobRow(v interface{}) *batchImportJobRow {
	// Type-assert through a minimal interface; avoids importing the service
	// package from the repository package (preserving clean architecture).
	type jobLike interface {
		GetID() uuid.UUID
		GetFilename() string
		GetImportedBy() string
		GetStatus() string
		GetTotalRows() int
		GetImportedRows() int
		GetFailedRows() int
		GetErrorDetails() interface{}
		GetStartedAt() *time.Time
		GetCompletedAt() *time.Time
		GetCreatedAt() time.Time
	}
	// Fall back to a simple map if the concrete type is passed via interface{}
	if m, ok := v.(map[string]interface{}); ok {
		return &batchImportJobRow{
			ID:       m["id"].(uuid.UUID),
			Filename: m["filename"].(string),
			Status:   m["status"].(string),
		}
	}
	// Default: assume the struct fields are directly addressable (same package path)
	return &batchImportJobRow{}
}

// landedCostFromRow is a helper kept here for documentation only;
// the actual computation runs inside BatchImportService.processRow.
func landedCostFromRow(unitCost, shippingPerUnit, customsDutyRate, insurancePerUnit decimal.Decimal) decimal.Decimal {
	return unitCost.
		Add(shippingPerUnit).
		Add(unitCost.Mul(customsDutyRate)).
		Add(insurancePerUnit)
}
