package service_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/dubai-retail/os/internal/domain"
	"github.com/dubai-retail/os/internal/service"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// =============================================================================
// Fakes
// =============================================================================

type fakeSupplierRepo struct {
	suppliers map[uuid.UUID]*domain.Supplier
	pos       map[uuid.UUID]*domain.PurchaseOrder
	items     map[uuid.UUID][]domain.POItem
}

func newFakeSupplierRepo() *fakeSupplierRepo {
	return &fakeSupplierRepo{
		suppliers: make(map[uuid.UUID]*domain.Supplier),
		pos:       make(map[uuid.UUID]*domain.PurchaseOrder),
		items:     make(map[uuid.UUID][]domain.POItem),
	}
}

func (r *fakeSupplierRepo) InsertSupplier(_ context.Context, s *domain.Supplier) error {
	s.ID = uuid.New()
	r.suppliers[s.ID] = s
	return nil
}
func (r *fakeSupplierRepo) UpdateSupplier(_ context.Context, s *domain.Supplier) error {
	r.suppliers[s.ID] = s
	return nil
}
func (r *fakeSupplierRepo) GetSupplierByID(_ context.Context, id uuid.UUID) (*domain.Supplier, error) {
	s := r.suppliers[id]
	if s == nil {
		return nil, fmt.Errorf("supplier not found")
	}
	return s, nil
}
func (r *fakeSupplierRepo) ListSuppliers(_ context.Context) ([]domain.Supplier, error) {
	var out []domain.Supplier
	for _, s := range r.suppliers {
		out = append(out, *s)
	}
	return out, nil
}
func (r *fakeSupplierRepo) InsertPurchaseOrder(_ context.Context, _ pgx.Tx, po *domain.PurchaseOrder) error {
	po.ID = uuid.New()
	r.pos[po.ID] = po
	return nil
}
func (r *fakeSupplierRepo) InsertPOItem(_ context.Context, _ pgx.Tx, item *domain.POItem) error {
	item.ID = uuid.New()
	r.items[item.PurchaseOrderID] = append(r.items[item.PurchaseOrderID], *item)
	return nil
}
func (r *fakeSupplierRepo) GetPurchaseOrderByID(_ context.Context, id uuid.UUID) (*domain.PurchaseOrder, error) {
	po := r.pos[id]
	if po == nil {
		return nil, fmt.Errorf("po not found")
	}
	po.Items = r.items[id]
	return po, nil
}
func (r *fakeSupplierRepo) GetPOItems(_ context.Context, poID uuid.UUID) ([]domain.POItem, error) {
	return r.items[poID], nil
}
func (r *fakeSupplierRepo) ListPurchaseOrders(_ context.Context) ([]domain.PurchaseOrder, error) {
	var out []domain.PurchaseOrder
	for _, po := range r.pos {
		out = append(out, *po)
	}
	return out, nil
}
func (r *fakeSupplierRepo) UpdatePOStatus(_ context.Context, _ pgx.Tx, id uuid.UUID, status domain.POStatus, _ *time.Time) error {
	if po := r.pos[id]; po != nil {
		po.Status = status
	}
	return nil
}
func (r *fakeSupplierRepo) UpdatePOItemReceivedQty(_ context.Context, _ pgx.Tx, id uuid.UUID, qty int) error {
	return nil
}

type fakeReceiveRepo struct {
	variants  map[uuid.UUID]*domain.Variant
	batches   []*domain.PurchaseBatch
	batchItems []*domain.BatchItem
}

func newFakeReceiveRepo() *fakeReceiveRepo {
	return &fakeReceiveRepo{variants: make(map[uuid.UUID]*domain.Variant)}
}

func (r *fakeReceiveRepo) GetVariantByID(_ context.Context, id uuid.UUID) (*domain.Variant, error) {
	v := r.variants[id]
	if v == nil {
		return nil, fmt.Errorf("variant not found: %s", id)
	}
	return v, nil
}
func (r *fakeReceiveRepo) InsertPurchaseBatch(_ context.Context, _ pgx.Tx, b *domain.PurchaseBatch) error {
	r.batches = append(r.batches, b)
	return nil
}
func (r *fakeReceiveRepo) InsertBatchItem(_ context.Context, _ pgx.Tx, bi *domain.BatchItem) error {
	r.batchItems = append(r.batchItems, bi)
	return nil
}
func (r *fakeReceiveRepo) UpsertInventory(_ context.Context, _ pgx.Tx, _ uuid.UUID, _ int) error {
	return nil
}

// =============================================================================
// Tests
// =============================================================================

func newTestSupplierService() (*service.SupplierService, *fakeSupplierRepo, *fakeReceiveRepo) {
	sr := newFakeSupplierRepo()
	rr := newFakeReceiveRepo()
	svc := service.NewSupplierService(sr, rr, newIntegrationFakeTxBeginner(), zap.NewNop())
	return svc, sr, rr
}

func TestSupplier_Create(t *testing.T) {
	svc, repo, _ := newTestSupplierService()

	s, err := svc.CreateSupplier(context.Background(), domain.Supplier{
		Name:    "Shanghai Textiles",
		Country: "CN",
		Rating:  4,
	})
	if err != nil {
		t.Fatalf("CreateSupplier: %v", err)
	}
	if s.ID == uuid.Nil {
		t.Error("expected non-nil ID")
	}
	if len(repo.suppliers) != 1 {
		t.Errorf("expected 1 supplier in repo, got %d", len(repo.suppliers))
	}
}

func TestSupplier_CreatePurchaseOrder(t *testing.T) {
	svc, repo, _ := newTestSupplierService()

	po, err := svc.CreatePurchaseOrder(context.Background(), service.CreatePOInput{
		ReferenceNumber: "PO-2025-001",
		Currency:        "USD",
	})
	if err != nil {
		t.Fatalf("CreatePurchaseOrder: %v", err)
	}
	if po.Status != domain.POStatusDraft {
		t.Errorf("expected draft, got %s", po.Status)
	}
	if len(repo.pos) != 1 {
		t.Errorf("expected 1 PO in repo")
	}
}

func TestSupplier_AddPOItem(t *testing.T) {
	svc, _, rr := newTestSupplierService()

	variantID := uuid.New()
	rr.variants[variantID] = &domain.Variant{ID: variantID, SKU: "DRESS-S-RED"}

	po, _ := svc.CreatePurchaseOrder(context.Background(), service.CreatePOInput{})
	item, err := svc.AddPOItem(context.Background(), service.AddPOItemInput{
		PurchaseOrderID: po.ID,
		VariantID:       variantID,
		Quantity:        50,
		UnitCost:        decimal.NewFromFloat(12.50),
	})
	if err != nil {
		t.Fatalf("AddPOItem: %v", err)
	}
	if item.Quantity != 50 {
		t.Errorf("expected qty 50, got %d", item.Quantity)
	}
}

func TestSupplier_AddPOItem_InvalidVariant(t *testing.T) {
	svc, _, _ := newTestSupplierService()

	po, _ := svc.CreatePurchaseOrder(context.Background(), service.CreatePOInput{})
	_, err := svc.AddPOItem(context.Background(), service.AddPOItemInput{
		PurchaseOrderID: po.ID,
		VariantID:       uuid.New(), // unknown
		Quantity:        10,
		UnitCost:        decimal.NewFromFloat(5),
	})
	if err == nil {
		t.Error("expected error for unknown variant")
	}
}

func TestSupplier_AddPOItem_NegativeQuantity(t *testing.T) {
	svc, _, _ := newTestSupplierService()

	po, _ := svc.CreatePurchaseOrder(context.Background(), service.CreatePOInput{})
	_, err := svc.AddPOItem(context.Background(), service.AddPOItemInput{
		PurchaseOrderID: po.ID,
		VariantID:       uuid.New(),
		Quantity:        -5,
		UnitCost:        decimal.NewFromFloat(10),
	})
	if err == nil {
		t.Error("expected error for negative quantity")
	}
}

func TestSupplier_ReceivePO_CreatesBatch(t *testing.T) {
	svc, sr, rr := newTestSupplierService()

	variantID := uuid.New()
	rr.variants[variantID] = &domain.Variant{ID: variantID, SKU: "JACKET-M-BLK"}

	po, _ := svc.CreatePurchaseOrder(context.Background(), service.CreatePOInput{ReferenceNumber: "PO-001"})

	item := &domain.POItem{
		ID:              uuid.New(),
		PurchaseOrderID: po.ID,
		VariantID:       variantID,
		Quantity:        100,
		UnitCost:        decimal.NewFromFloat(20),
	}
	sr.items[po.ID] = append(sr.items[po.ID], *item)

	batch, err := svc.ReceivePurchaseOrder(context.Background(), service.ReceiveInput{
		PurchaseOrderID: po.ID,
		ShippingTotal:   decimal.NewFromFloat(500),
		CustomsDutyPct:  decimal.NewFromFloat(0.05),
		InsuranceTotal:  decimal.NewFromFloat(100),
	})
	if err != nil {
		t.Fatalf("ReceivePurchaseOrder: %v", err)
	}
	if batch.ID == uuid.Nil {
		t.Error("expected batch to have an ID")
	}
	if len(rr.batchItems) != 1 {
		t.Errorf("expected 1 batch item, got %d", len(rr.batchItems))
	}
	// Verify landed cost components: unit=20, shipping=5, customs=1(5%), insurance=1
	// LandedCostPerUnit is a DB-computed column; we verify the components manually
	expectedShipping := decimal.NewFromFloat(5.0)   // 500/100
	expectedCustoms := decimal.NewFromFloat(1.0)    // 20 * 0.05
	expectedInsurance := decimal.NewFromFloat(1.0)  // 100/100
	actualShipping := rr.batchItems[0].ShippingAllocation
	actualCustoms := rr.batchItems[0].CustomsDuty
	actualInsurance := rr.batchItems[0].Insurance
	if !actualShipping.Equal(expectedShipping) {
		t.Errorf("expected shipping %s, got %s", expectedShipping, actualShipping)
	}
	if !actualCustoms.Equal(expectedCustoms) {
		t.Errorf("expected customs %s, got %s", expectedCustoms, actualCustoms)
	}
	if !actualInsurance.Equal(expectedInsurance) {
		t.Errorf("expected insurance %s, got %s", expectedInsurance, actualInsurance)
	}
	if sr.pos[po.ID].Status != domain.POStatusReceived {
		t.Errorf("expected PO status=received, got %s", sr.pos[po.ID].Status)
	}
}
