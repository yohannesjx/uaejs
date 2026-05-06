package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dubai-retail/os/internal/domain"
	"github.com/dubai-retail/os/internal/repository/postgres"
	"github.com/dubai-retail/os/internal/service"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// =============================================================================
// Fakes
// =============================================================================

// fakeTxBeginner returns a fake transaction.
type fakeTxBeginner struct{ tx *fakeTx }

func (f *fakeTxBeginner) BeginTx(_ context.Context, _ pgx.TxOptions) (pgx.Tx, error) {
	return f.tx, nil
}

// fakeTx is a no-op pgx.Tx implementation.
type fakeTx struct{ committed bool }

func (f *fakeTx) Begin(_ context.Context) (pgx.Tx, error) { return f, nil }
func (f *fakeTx) Commit(_ context.Context) error          { f.committed = true; return nil }
func (f *fakeTx) Rollback(_ context.Context) error        { return pgx.ErrTxClosed }
func (f *fakeTx) CopyFrom(_ context.Context, _ pgx.Identifier, _ []string, _ pgx.CopyFromSource) (int64, error) {
	return 0, nil
}
func (f *fakeTx) SendBatch(_ context.Context, _ *pgx.Batch) pgx.BatchResults { return nil }
func (f *fakeTx) LargeObjects() pgx.LargeObjects                             { return pgx.LargeObjects{} }
func (f *fakeTx) Prepare(_ context.Context, _, _ string) (*pgconn.StatementDescription, error) {
	return nil, nil
}
func (f *fakeTx) Exec(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}
func (f *fakeTx) Query(_ context.Context, _ string, _ ...any) (pgx.Rows, error) { return nil, nil }
func (f *fakeTx) QueryRow(_ context.Context, _ string, _ ...any) pgx.Row        { return nil }
func (f *fakeTx) Conn() *pgx.Conn                                               { return nil }

// fakeInventoryRepo implements service.InventoryRepo using in-memory state.
type fakeInventoryRepo struct {
	inventory  map[uuid.UUID]*domain.Inventory
	batches    map[uuid.UUID][]postgres.FIFOBatchItemRow
	movements  []domain.InventoryMovement
	deductions map[uuid.UUID]int
}

func newFakeRepo() *fakeInventoryRepo {
	return &fakeInventoryRepo{
		inventory:  make(map[uuid.UUID]*domain.Inventory),
		batches:    make(map[uuid.UUID][]postgres.FIFOBatchItemRow),
		deductions: make(map[uuid.UUID]int),
	}
}

func (r *fakeInventoryRepo) GetByVariantIDForUpdate(_ context.Context, _ pgx.Tx, id uuid.UUID) (*domain.Inventory, error) {
	inv, ok := r.inventory[id]
	if !ok {
		return nil, errors.New("not found")
	}
	return inv, nil
}

func (r *fakeInventoryRepo) GetFIFOBatchItems(_ context.Context, _ pgx.Tx, id uuid.UUID) ([]postgres.FIFOBatchItemRow, error) {
	return r.batches[id], nil
}

func (r *fakeInventoryRepo) DeductOnHand(_ context.Context, _ pgx.Tx, id uuid.UUID, qty int) (*domain.Inventory, error) {
	inv := r.inventory[id]
	inv.QuantityOnHand -= qty
	r.deductions[id] += qty
	return inv, nil
}

func (r *fakeInventoryRepo) InsertMovement(_ context.Context, _ pgx.Tx, m *domain.InventoryMovement) error {
	r.movements = append(r.movements, *m)
	return nil
}

func (r *fakeInventoryRepo) InsertReservation(_ context.Context, _ pgx.Tx, _ *domain.StockReservation) error {
	return nil
}

func (r *fakeInventoryRepo) ReleaseReservation(_ context.Context, _ pgx.Tx, _ uuid.UUID) error {
	return nil
}

// =============================================================================
// Tests
// =============================================================================

func TestSubtractStock_FIFO_SingleBatch(t *testing.T) {
	repo := newFakeRepo()
	variantID := uuid.New()
	orderID := uuid.New()
	channelID := uuid.New()
	batchItemID := uuid.New()

	repo.inventory[variantID] = &domain.Inventory{
		ID:               uuid.New(),
		VariantID:        variantID,
		QuantityOnHand:   100,
		QuantityReserved: 0,
	}
	repo.batches[variantID] = []postgres.FIFOBatchItemRow{
		{
			BatchItemID:       batchItemID,
			BatchReceivedAt:   time.Now().Add(-24 * time.Hour),
			LandedCostPerUnit: decimal.NewFromFloat(45.50),
			QuantityReceived:  100,
			TotalDeducted:     0,
		},
	}

	svc := service.NewInventoryService(repo, &fakeTxBeginner{tx: &fakeTx{}}, zap.NewNop(), decimal.NewFromFloat(0.05))

	results, err := svc.SubtractStock(context.Background(), []service.DeductionItem{
		{VariantID: variantID, OrderID: orderID, ChannelID: channelID, Quantity: 10},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	res := results[0]
	if res.TotalDeducted != 10 {
		t.Errorf("expected 10 deducted, got %d", res.TotalDeducted)
	}
	if !res.WeightedCOGS.Equal(decimal.NewFromFloat(45.50)) {
		t.Errorf("expected COGS 45.50, got %s", res.WeightedCOGS)
	}
	if len(res.Movements) != 1 {
		t.Errorf("expected 1 movement, got %d", len(res.Movements))
	}
	if repo.movements[0].Quantity != -10 {
		t.Errorf("expected movement qty -10, got %d", repo.movements[0].Quantity)
	}
}

func TestSubtractStock_FIFO_SpansMultipleBatches(t *testing.T) {
	repo := newFakeRepo()
	variantID := uuid.New()
	orderID := uuid.New()
	channelID := uuid.New()

	repo.inventory[variantID] = &domain.Inventory{
		ID:               uuid.New(),
		VariantID:        variantID,
		QuantityOnHand:   30,
		QuantityReserved: 0,
	}

	now := time.Now()
	repo.batches[variantID] = []postgres.FIFOBatchItemRow{
		{
			BatchItemID:       uuid.New(),
			BatchReceivedAt:   now.Add(-48 * time.Hour), // oldest
			LandedCostPerUnit: decimal.NewFromFloat(40.00),
			QuantityReceived:  15,
			TotalDeducted:     5, // 10 remaining
		},
		{
			BatchItemID:       uuid.New(),
			BatchReceivedAt:   now.Add(-24 * time.Hour), // newer
			LandedCostPerUnit: decimal.NewFromFloat(50.00),
			QuantityReceived:  20,
			TotalDeducted:     0, // 20 remaining
		},
	}

	svc := service.NewInventoryService(repo, &fakeTxBeginner{tx: &fakeTx{}}, zap.NewNop(), decimal.NewFromFloat(0.05))

	results, err := svc.SubtractStock(context.Background(), []service.DeductionItem{
		{VariantID: variantID, OrderID: orderID, ChannelID: channelID, Quantity: 25},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	res := results[0]
	if res.TotalDeducted != 25 {
		t.Errorf("expected 25 deducted, got %d", res.TotalDeducted)
	}

	// 10 units @ 40 + 15 units @ 50  = 400 + 750 = 1150 / 25 = 46.00
	expectedCOGS := decimal.NewFromFloat(46.00)
	if !res.WeightedCOGS.Equal(expectedCOGS) {
		t.Errorf("expected weighted COGS %s, got %s", expectedCOGS, res.WeightedCOGS)
	}

	if len(res.Movements) != 2 {
		t.Errorf("expected 2 movements (one per batch), got %d", len(res.Movements))
	}
}

func TestSubtractStock_InsufficientStock(t *testing.T) {
	repo := newFakeRepo()
	variantID := uuid.New()

	repo.inventory[variantID] = &domain.Inventory{
		ID:               uuid.New(),
		VariantID:        variantID,
		QuantityOnHand:   5,
		QuantityReserved: 0,
	}
	repo.batches[variantID] = []postgres.FIFOBatchItemRow{
		{
			BatchItemID:       uuid.New(),
			BatchReceivedAt:   time.Now(),
			LandedCostPerUnit: decimal.NewFromFloat(30.00),
			QuantityReceived:  5,
			TotalDeducted:     0,
		},
	}

	svc := service.NewInventoryService(repo, &fakeTxBeginner{tx: &fakeTx{}}, zap.NewNop(), decimal.NewFromFloat(0.05))

	_, err := svc.SubtractStock(context.Background(), []service.DeductionItem{
		{VariantID: variantID, Quantity: 10},
	})
	if !errors.Is(err, service.ErrInsufficientStock) {
		t.Errorf("expected ErrInsufficientStock, got %v", err)
	}
}
