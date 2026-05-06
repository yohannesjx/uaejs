package service_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/dubai-retail/os/internal/domain"
	"github.com/dubai-retail/os/internal/metrics"
	"github.com/dubai-retail/os/internal/service"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// ── Fake repo ─────────────────────────────────────────────────────────────────

type fakeBatchImportRepo struct {
	variants     map[string]*domain.Variant // SKU → Variant
	batches      []*domain.PurchaseBatch
	batchItems   []*domain.BatchItem
	inventoryAdj map[uuid.UUID]int   // variantID → delta sum
	jobs         []*batchImportJobExported
}

// batchImportJobExported mirrors internal batchImportJob for inspection.
type batchImportJobExported struct {
	ID           uuid.UUID
	Status       service.BatchImportStatus
	ImportedRows int
	FailedRows   int
}

func newFakeBatchImportRepo(skus ...string) *fakeBatchImportRepo {
	r := &fakeBatchImportRepo{
		variants:     make(map[string]*domain.Variant),
		inventoryAdj: make(map[uuid.UUID]int),
	}
	for _, sku := range skus {
		vid := uuid.New()
		r.variants[sku] = &domain.Variant{ID: vid, SKU: sku}
	}
	return r
}

func (r *fakeBatchImportRepo) GetVariantBySKU(_ context.Context, sku string) (*domain.Variant, error) {
	v := r.variants[sku]
	if v == nil {
		return nil, fmt.Errorf("variant not found: %s", sku)
	}
	return v, nil
}

func (r *fakeBatchImportRepo) GetVariantByID(_ context.Context, id uuid.UUID) (*domain.Variant, error) {
	for _, v := range r.variants {
		if v.ID == id {
			return v, nil
		}
	}
	return nil, fmt.Errorf("variant not found: %s", id)
}

func (r *fakeBatchImportRepo) InsertPurchaseBatch(_ context.Context, _ pgx.Tx, b *domain.PurchaseBatch) error {
	r.batches = append(r.batches, b)
	return nil
}

func (r *fakeBatchImportRepo) InsertBatchItem(_ context.Context, _ pgx.Tx, item *domain.BatchItem) error {
	r.batchItems = append(r.batchItems, item)
	return nil
}

func (r *fakeBatchImportRepo) UpsertInventory(_ context.Context, _ pgx.Tx, variantID uuid.UUID, delta int) error {
	r.inventoryAdj[variantID] += delta
	return nil
}

func (r *fakeBatchImportRepo) InsertBatchImport(_ context.Context, job interface{}) error {
	return nil
}

func (r *fakeBatchImportRepo) UpdateBatchImport(_ context.Context, job interface{}) error {
	return nil
}

// ── Tests ─────────────────────────────────────────────────────────────────────

func TestBatchImport_CSV_HappyPath(t *testing.T) {
	repo := newFakeBatchImportRepo("SKU-DRESS-RED-S", "SKU-HANDBAG-BLK-M")
	svc := newBatchImportSvc(repo)

	csv := `sku,quantity,unit_cost,shipping_total,customs_duty_rate,insurance_total
SKU-DRESS-RED-S,100,45.00,500.00,0.05,200.00
SKU-HANDBAG-BLK-M,50,120.00,300.00,0.05,100.00`

	result, err := svc.ImportFromCSV(context.Background(), "march_shipment.csv", "admin", strings.NewReader(csv))
	if err != nil {
		t.Fatalf("ImportFromCSV failed: %v", err)
	}

	if result.ImportedRows != 2 {
		t.Errorf("expected 2 imported rows, got %d", result.ImportedRows)
	}
	if result.FailedRows != 0 {
		t.Errorf("expected 0 failed rows, got %d (errors: %v)", result.FailedRows, result.RowErrors)
	}

	// Verify landed cost calculation for row 1:
	// unit_cost=45, shipping=500/100=5, customs=45×0.05=2.25, insurance=200/100=2
	// landed = 45 + 5 + 2.25 + 2 = 54.25
	if len(repo.batchItems) == 0 {
		t.Fatal("no batch items inserted")
	}
	want := decimal.RequireFromString("54.25")
	got := repo.batchItems[0].LandedCostPerUnit
	if !got.Equal(want) {
		t.Errorf("landed cost row 1: want %s got %s", want, got)
	}

	// Inventory adjustments
	v := repo.variants["SKU-DRESS-RED-S"]
	if repo.inventoryAdj[v.ID] != 100 {
		t.Errorf("inventory delta for SKU-DRESS-RED-S: want 100 got %d", repo.inventoryAdj[v.ID])
	}
}

func TestBatchImport_CSV_UnknownSKU(t *testing.T) {
	repo := newFakeBatchImportRepo("SKU-KNOWN")
	svc := newBatchImportSvc(repo)

	csv := `sku,quantity,unit_cost,shipping_total,customs_duty_rate,insurance_total
SKU-KNOWN,10,50.00,100.00,0.05,50.00
SKU-UNKNOWN,5,30.00,50.00,0.05,20.00`

	result, err := svc.ImportFromCSV(context.Background(), "test.csv", "admin", strings.NewReader(csv))
	if err != nil {
		t.Fatalf("unexpected top-level error: %v", err)
	}

	if result.ImportedRows != 1 {
		t.Errorf("expected 1 imported row, got %d", result.ImportedRows)
	}
	if result.FailedRows != 1 {
		t.Errorf("expected 1 failed row, got %d", result.FailedRows)
	}
	if len(result.RowErrors) == 0 || !strings.Contains(result.RowErrors[0].Error, "variant not found") {
		t.Errorf("expected 'variant not found' error, got %v", result.RowErrors)
	}
}

func TestBatchImport_CSV_NegativeQuantity(t *testing.T) {
	repo := newFakeBatchImportRepo("SKU-A")
	svc := newBatchImportSvc(repo)

	csv := `sku,quantity,unit_cost,shipping_total,customs_duty_rate,insurance_total
SKU-A,-5,50.00,100.00,0.05,0.00`

	result, err := svc.ImportFromCSV(context.Background(), "bad.csv", "admin", strings.NewReader(csv))
	if err != nil {
		t.Fatalf("unexpected top-level error: %v", err)
	}
	if result.FailedRows != 1 {
		t.Errorf("expected 1 failed row for negative qty, got %d", result.FailedRows)
	}
	if !strings.Contains(result.RowErrors[0].Error, "quantity must be > 0") {
		t.Errorf("unexpected error: %s", result.RowErrors[0].Error)
	}
}

func TestBatchImport_CSV_MissingRequiredColumn(t *testing.T) {
	repo := newFakeBatchImportRepo("SKU-A")
	svc := newBatchImportSvc(repo)

	// Missing 'unit_cost' column
	csv := `sku,quantity,shipping_total,customs_duty_rate,insurance_total
SKU-A,10,100.00,0.05,0.00`

	_, err := svc.ImportFromCSV(context.Background(), "bad.csv", "admin", strings.NewReader(csv))
	if err == nil {
		t.Fatal("expected error for missing required column, got nil")
	}
	if !strings.Contains(err.Error(), "unit_cost") {
		t.Errorf("expected 'unit_cost' in error, got: %v", err)
	}
}

func TestBatchImport_JSON_HappyPath(t *testing.T) {
	repo := newFakeBatchImportRepo("SKU-JACKET-BLU-L")
	svc := newBatchImportSvc(repo)

	payload := `[{
		"sku": "SKU-JACKET-BLU-L",
		"quantity": 200,
		"unit_cost": "80.00",
		"shipping_total": "1600.00",
		"customs_duty_rate": "0.05",
		"insurance_total": "400.00"
	}]`

	result, err := svc.ImportFromJSON(context.Background(), "april_json.json", "warehouse_api", strings.NewReader(payload))
	if err != nil {
		t.Fatalf("ImportFromJSON failed: %v", err)
	}
	if result.ImportedRows != 1 {
		t.Errorf("expected 1 imported row, got %d", result.ImportedRows)
	}

	// landed = 80 + 8 + 4 + 2 = 94
	want := decimal.RequireFromString("94.00")
	got := repo.batchItems[0].LandedCostPerUnit
	if !got.Equal(want) {
		t.Errorf("landed cost: want %s got %s", want, got)
	}
}

func TestBatchImport_CustomsDutyRate_OutOfRange(t *testing.T) {
	repo := newFakeBatchImportRepo("SKU-A")
	svc := newBatchImportSvc(repo)

	csv := `sku,quantity,unit_cost,shipping_total,customs_duty_rate,insurance_total
SKU-A,10,50.00,0.00,1.50,0.00`

	result, err := svc.ImportFromCSV(context.Background(), "bad.csv", "admin", strings.NewReader(csv))
	if err != nil {
		t.Fatalf("unexpected top-level error: %v", err)
	}
	if !strings.Contains(result.RowErrors[0].Error, "customs_duty_rate") {
		t.Errorf("expected customs_duty_rate error, got: %v", result.RowErrors)
	}
}

// ── Helper ────────────────────────────────────────────────────────────────────

func newBatchImportSvc(repo service.BatchImportRepo) *service.BatchImportService {
	m := metrics.New(prometheus.NewRegistry())
	return service.NewBatchImportService(repo, newIntegrationFakeTxBeginner(), m, zap.NewNop())
}
