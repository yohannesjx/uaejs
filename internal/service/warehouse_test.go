package service_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/dubai-retail/os/internal/domain"
	"github.com/dubai-retail/os/internal/service"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"go.uber.org/zap"
)

// ensure sync is used (mutex in fakeWarehouseRepo)
var _ sync.Locker = &sync.Mutex{}

// =============================================================================
// Fake repository
// =============================================================================

type fakeWarehouseRepo struct {
	mu         sync.Mutex
	warehouses map[uuid.UUID]*domain.Warehouse
	stock      map[string]*domain.WarehouseStock // key = "warehouseID:variantID"
	movements  []*domain.InventoryMovement
}

func newFakeWarehouseRepo() *fakeWarehouseRepo {
	r := &fakeWarehouseRepo{
		warehouses: make(map[uuid.UUID]*domain.Warehouse),
		stock:      make(map[string]*domain.WarehouseStock),
	}
	// Seed two warehouses.
	w1 := &domain.Warehouse{ID: uuid.New(), TenantID: domain.DefaultTenantID, Name: "Dubai Main", Type: domain.WarehouseTypeWarehouse, IsActive: true, Priority: 10}
	w2 := &domain.Warehouse{ID: uuid.New(), TenantID: domain.DefaultTenantID, Name: "Abu Dhabi Store", Type: domain.WarehouseTypeStore, IsActive: true, Priority: 20}
	r.warehouses[w1.ID] = w1
	r.warehouses[w2.ID] = w2
	return r
}

func stockKey(wid, vid uuid.UUID) string { return wid.String() + ":" + vid.String() }

func (r *fakeWarehouseRepo) InsertWarehouse(_ context.Context, w *domain.Warehouse) error {
	w.ID = uuid.New()
	r.warehouses[w.ID] = w
	return nil
}

func (r *fakeWarehouseRepo) UpdateWarehouse(_ context.Context, w *domain.Warehouse) error {
	if _, ok := r.warehouses[w.ID]; !ok {
		return fmt.Errorf("warehouse not found")
	}
	r.warehouses[w.ID] = w
	return nil
}

func (r *fakeWarehouseRepo) GetWarehouseByID(_ context.Context, id uuid.UUID) (*domain.Warehouse, error) {
	w, ok := r.warehouses[id]
	if !ok {
		return nil, fmt.Errorf("warehouse not found")
	}
	return w, nil
}

func (r *fakeWarehouseRepo) ListWarehousesByTenant(_ context.Context, tenantID uuid.UUID) ([]domain.Warehouse, error) {
	var out []domain.Warehouse
	for _, w := range r.warehouses {
		if w.TenantID == tenantID {
			out = append(out, *w)
		}
	}
	return out, nil
}

func (r *fakeWarehouseRepo) GetStockForUpdate(_ context.Context, _ pgx.Tx, wid, vid uuid.UUID) (*domain.WarehouseStock, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.stock[stockKey(wid, vid)]
	if !ok {
		return nil, fmt.Errorf("stock not found")
	}
	return s, nil
}

func (r *fakeWarehouseRepo) UpsertStock(_ context.Context, s *domain.WarehouseStock) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	key := stockKey(s.WarehouseID, s.VariantID)
	existing, ok := r.stock[key]
	if ok {
		existing.QtyOnHand = s.QtyOnHand
		existing.QtyReserved = s.QtyReserved
		existing.QtyAvailable = s.QtyOnHand - s.QtyReserved
	} else {
		s.QtyAvailable = s.QtyOnHand - s.QtyReserved
		r.stock[key] = s
	}
	return nil
}

func (r *fakeWarehouseRepo) AdjustStockTx(_ context.Context, _ pgx.Tx, wid, vid uuid.UUID, delta int) (*domain.WarehouseStock, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := stockKey(wid, vid)
	s, ok := r.stock[key]
	if !ok {
		return nil, fmt.Errorf("no stock row for warehouse %s variant %s", wid, vid)
	}
	s.QtyOnHand += delta
	s.QtyAvailable = s.QtyOnHand - s.QtyReserved
	return s, nil
}

func (r *fakeWarehouseRepo) ListInventoryByWarehouse(_ context.Context, wid uuid.UUID) ([]domain.WarehouseStock, error) {
	var out []domain.WarehouseStock
	for _, s := range r.stock {
		if s.WarehouseID == wid {
			out = append(out, *s)
		}
	}
	return out, nil
}

func (r *fakeWarehouseRepo) GetStockByWarehouseAndVariant(_ context.Context, wid, vid uuid.UUID) (*domain.WarehouseStock, error) {
	s, ok := r.stock[stockKey(wid, vid)]
	if !ok {
		return nil, fmt.Errorf("stock not found")
	}
	return s, nil
}

func (r *fakeWarehouseRepo) InsertMovementTx(_ context.Context, _ pgx.Tx, m *domain.InventoryMovement) error {
	m.ID = uuid.New()
	r.movements = append(r.movements, m)
	return nil
}

// =============================================================================
// Fake TxBeginner (reuses unitFakeTxBeginner from testhelpers_test.go)
// =============================================================================

func newTestWarehouseService() (*service.WarehouseService, *fakeWarehouseRepo) {
	repo := newFakeWarehouseRepo()
	txb := &unitFakeTxBeginner{}
	svc := service.NewWarehouseService(repo, txb, zap.NewNop())
	return svc, repo
}

// =============================================================================
// Tests: Warehouse CRUD
// =============================================================================

func TestWarehouse_Create_Success(t *testing.T) {
	svc, _ := newTestWarehouseService()
	ctx := context.Background()

	w, err := svc.CreateWarehouse(ctx, service.CreateWarehouseInput{
		TenantID: domain.DefaultTenantID,
		Name:     "Sharjah Hub",
		Type:     domain.WarehouseTypeWarehouse,
		City:     "Sharjah",
		Country:  "AE",
		Priority: 30,
	})
	if err != nil {
		t.Fatalf("CreateWarehouse: %v", err)
	}
	if w.ID == uuid.Nil {
		t.Error("expected non-nil warehouse ID")
	}
	if w.Name != "Sharjah Hub" {
		t.Errorf("unexpected name: %s", w.Name)
	}
}

func TestWarehouse_Create_EmptyName_Rejected(t *testing.T) {
	svc, _ := newTestWarehouseService()
	_, err := svc.CreateWarehouse(context.Background(), service.CreateWarehouseInput{
		TenantID: domain.DefaultTenantID,
		Name:     "",
	})
	if err == nil {
		t.Error("expected error for empty name")
	}
}

func TestWarehouse_List_TenantScoped(t *testing.T) {
	svc, _ := newTestWarehouseService()
	ctx := context.Background()

	// Default tenant already has 2 seeded warehouses.
	list, err := svc.ListWarehouses(ctx, domain.DefaultTenantID)
	if err != nil {
		t.Fatalf("ListWarehouses: %v", err)
	}
	if len(list) < 2 {
		t.Errorf("expected ≥2 warehouses, got %d", len(list))
	}

	// Different tenant should see 0.
	otherTenant := uuid.New()
	list2, _ := svc.ListWarehouses(ctx, otherTenant)
	if len(list2) != 0 {
		t.Errorf("expected 0 warehouses for new tenant, got %d", len(list2))
	}
}

func TestWarehouse_SetStock_And_GetInventory(t *testing.T) {
	svc, repo := newTestWarehouseService()
	ctx := context.Background()

	// Pick an existing warehouse.
	var whID uuid.UUID
	for id := range repo.warehouses {
		whID = id
		break
	}
	variantID := uuid.New()

	if err := svc.SetStock(ctx, service.SetStockInput{
		WarehouseID:  whID,
		VariantID:    variantID,
		QtyOnHand:    100,
		ReorderPoint: 10,
	}); err != nil {
		t.Fatalf("SetStock: %v", err)
	}

	stocks, err := svc.GetInventory(ctx, whID)
	if err != nil {
		t.Fatalf("GetInventory: %v", err)
	}
	if len(stocks) != 1 || stocks[0].QtyOnHand != 100 {
		t.Errorf("unexpected stock: %+v", stocks)
	}
}

func TestWarehouse_SetStock_NegativeQty_Rejected(t *testing.T) {
	svc, _ := newTestWarehouseService()
	err := svc.SetStock(context.Background(), service.SetStockInput{
		WarehouseID: uuid.New(),
		VariantID:   uuid.New(),
		QtyOnHand:   -5,
	})
	if err == nil {
		t.Error("expected error for negative qty_on_hand")
	}
}

// =============================================================================
// Tests: Transfer
// =============================================================================

func TestWarehouse_Transfer_Success(t *testing.T) {
	svc, repo := newTestWarehouseService()
	ctx := context.Background()

	// Get the two seeded warehouses.
	ids := make([]uuid.UUID, 0, 2)
	for id := range repo.warehouses {
		ids = append(ids, id)
	}
	from, to := ids[0], ids[1]
	variantID := uuid.New()

	// Seed 50 units in the source warehouse.
	_ = repo.UpsertStock(ctx, &domain.WarehouseStock{
		WarehouseID: from,
		VariantID:   variantID,
		QtyOnHand:   50,
	})
	// Seed 10 units in the destination.
	_ = repo.UpsertStock(ctx, &domain.WarehouseStock{
		WarehouseID: to,
		VariantID:   variantID,
		QtyOnHand:   10,
	})

	result, err := svc.Transfer(ctx, domain.TransferRequest{
		FromWarehouseID: from,
		ToWarehouseID:   to,
		VariantID:       variantID,
		Quantity:        20,
	})
	if err != nil {
		t.Fatalf("Transfer: %v", err)
	}

	if result.FromStock.QtyOnHand != 30 {
		t.Errorf("expected source 30, got %d", result.FromStock.QtyOnHand)
	}
	if result.ToStock.QtyOnHand != 30 {
		t.Errorf("expected dest 30, got %d", result.ToStock.QtyOnHand)
	}
	if len(result.Movements) != 2 {
		t.Errorf("expected 2 movements, got %d", len(result.Movements))
	}
	if result.Movements[0].MovementType != domain.MovementTypeTransferOut {
		t.Errorf("expected transfer_out, got %s", result.Movements[0].MovementType)
	}
	if result.Movements[1].MovementType != domain.MovementTypeTransferIn {
		t.Errorf("expected transfer_in, got %s", result.Movements[1].MovementType)
	}
}

func TestWarehouse_Transfer_InsufficientStock(t *testing.T) {
	svc, repo := newTestWarehouseService()
	ctx := context.Background()

	ids := make([]uuid.UUID, 0, 2)
	for id := range repo.warehouses {
		ids = append(ids, id)
	}
	from, to := ids[0], ids[1]
	variantID := uuid.New()

	_ = repo.UpsertStock(ctx, &domain.WarehouseStock{
		WarehouseID: from,
		VariantID:   variantID,
		QtyOnHand:   5,
	})

	_, err := svc.Transfer(ctx, domain.TransferRequest{
		FromWarehouseID: from,
		ToWarehouseID:   to,
		VariantID:       variantID,
		Quantity:        10, // more than available
	})
	if err == nil {
		t.Error("expected error for insufficient stock")
	}
	if !errors.Is(err, service.ErrInsufficientStock) {
		t.Errorf("expected ErrInsufficientStock, got: %v", err)
	}
}

func TestWarehouse_Transfer_SameWarehouse_Rejected(t *testing.T) {
	svc, _ := newTestWarehouseService()
	id := uuid.New()
	_, err := svc.Transfer(context.Background(), domain.TransferRequest{
		FromWarehouseID: id,
		ToWarehouseID:   id,
		VariantID:       uuid.New(),
		Quantity:        1,
	})
	if err == nil {
		t.Error("expected error for same-warehouse transfer")
	}
}

// TestWarehouse_Transfer_StockExhaustion verifies the sequential safety of the
// Transfer guard: repeated transfers drain the source to exactly 0 and then
// subsequent attempts are rejected with ErrInsufficientStock.
//
// Concurrent safety is guaranteed in production by PostgreSQL SERIALIZABLE
// transactions + SELECT FOR UPDATE on both warehouse_stock rows.
// An in-memory fake cannot replicate MVCC, so the race-condition guarantee is
// documented as a DB contract, not tested here with goroutines.
func TestWarehouse_Transfer_StockExhaustion(t *testing.T) {
	svc, repo := newTestWarehouseService()
	ctx := context.Background()

	ids := make([]uuid.UUID, 0, 2)
	for id := range repo.warehouses {
		ids = append(ids, id)
	}
	from, to := ids[0], ids[1]
	variantID := uuid.New()

	// Seed 9 units in source, 0 in destination.
	_ = repo.UpsertStock(ctx, &domain.WarehouseStock{WarehouseID: from, VariantID: variantID, QtyOnHand: 9})
	_ = repo.UpsertStock(ctx, &domain.WarehouseStock{WarehouseID: to, VariantID: variantID, QtyOnHand: 0})

	// Three transfers of 3 units each should succeed.
	for i := 0; i < 3; i++ {
		_, err := svc.Transfer(ctx, domain.TransferRequest{
			FromWarehouseID: from, ToWarehouseID: to, VariantID: variantID, Quantity: 3,
		})
		if err != nil {
			t.Fatalf("transfer %d/3 failed unexpectedly: %v", i+1, err)
		}
	}

	// Source must be at 0.
	stocks, _ := svc.GetInventory(ctx, from)
	for _, s := range stocks {
		if s.VariantID == variantID && s.QtyOnHand != 0 {
			t.Errorf("expected source at 0, got %d", s.QtyOnHand)
		}
	}

	// 4th transfer must fail.
	_, err := svc.Transfer(ctx, domain.TransferRequest{
		FromWarehouseID: from, ToWarehouseID: to, VariantID: variantID, Quantity: 1,
	})
	if !errors.Is(err, service.ErrInsufficientStock) {
		t.Errorf("expected ErrInsufficientStock after exhaustion, got: %v", err)
	}
}

func TestWarehouse_Transfer_AutoCreatesDestinationStock(t *testing.T) {
	svc, repo := newTestWarehouseService()
	ctx := context.Background()

	ids := make([]uuid.UUID, 0, 2)
	for id := range repo.warehouses {
		ids = append(ids, id)
	}
	from, to := ids[0], ids[1]
	variantID := uuid.New()

	// Only seed source — destination has no stock row.
	_ = repo.UpsertStock(ctx, &domain.WarehouseStock{
		WarehouseID: from,
		VariantID:   variantID,
		QtyOnHand:   30,
	})

	result, err := svc.Transfer(ctx, domain.TransferRequest{
		FromWarehouseID: from,
		ToWarehouseID:   to,
		VariantID:       variantID,
		Quantity:        15,
	})
	if err != nil {
		t.Fatalf("Transfer with auto-create: %v", err)
	}
	if result.ToStock.QtyOnHand != 15 {
		t.Errorf("expected destination to have 15 units, got %d", result.ToStock.QtyOnHand)
	}
}
