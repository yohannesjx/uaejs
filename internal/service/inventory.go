package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/dubai-retail/os/internal/domain"
	"github.com/dubai-retail/os/internal/repository/postgres"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// =============================================================================
// Errors
// =============================================================================

// InsufficientStockError is a typed error that carries the Available quantity so
// callers can surface a precise "needs X, only Y available" message.
type InsufficientStockError struct {
	VariantID uuid.UUID
	Needed    int
	Available int
}

func (e *InsufficientStockError) Error() string {
	return fmt.Sprintf("insufficient stock: variant %s needs %d, available %d",
		e.VariantID, e.Needed, e.Available)
}

// Is makes errors.Is(err, ErrInsufficientStock) return true for any
// *InsufficientStockError, preserving backward compatibility.
func (e *InsufficientStockError) Is(target error) bool {
	return target == ErrInsufficientStock
}

// Sentinel kept for backward-compat errors.Is checks.
var ErrInsufficientStock = errors.New("insufficient stock")

var ErrNoFIFOBatches = errors.New("no received purchase batches found for variant")

// =============================================================================
// Interfaces (depend on abstractions, not concretions)
// =============================================================================

// InventoryRepo is the subset of postgres.InventoryRepository the service needs.
type InventoryRepo interface {
	GetByVariantIDForUpdate(ctx context.Context, tx pgx.Tx, variantID uuid.UUID) (*domain.Inventory, error)
	GetFIFOBatchItems(ctx context.Context, tx pgx.Tx, variantID uuid.UUID) ([]postgres.FIFOBatchItemRow, error)
	DeductOnHand(ctx context.Context, tx pgx.Tx, variantID uuid.UUID, qty int) (*domain.Inventory, error)
	InsertMovement(ctx context.Context, tx pgx.Tx, m *domain.InventoryMovement) error
	InsertReservation(ctx context.Context, tx pgx.Tx, res *domain.StockReservation) error
	ReleaseReservation(ctx context.Context, tx pgx.Tx, reservationID uuid.UUID) error
}

// TxBeginner abstracts pgxpool so we can test without a real DB.
type TxBeginner interface {
	BeginTx(ctx context.Context, txOptions pgx.TxOptions) (pgx.Tx, error)
}

// =============================================================================
// DTOs
// =============================================================================

// DeductionItem is a single order line passed to SubtractStock.
type DeductionItem struct {
	VariantID uuid.UUID
	OrderID   uuid.UUID
	ChannelID uuid.UUID
	Quantity  int
}

// DeductionResult is returned per variant after a successful FIFO deduction.
type DeductionResult struct {
	VariantID    uuid.UUID
	TotalDeducted int
	WeightedCOGS decimal.Decimal // weighted average landed cost across all batches touched
	Movements    []domain.InventoryMovement
}

// ReserveRequest is a single line for a stock reservation.
type ReserveRequest struct {
	OrderID        uuid.UUID
	VariantID      uuid.UUID
	ChannelID      uuid.UUID
	Quantity       int
	ReservationTTL time.Duration
}

// =============================================================================
// InventoryService
// =============================================================================

type InventoryService struct {
	repo    InventoryRepo
	pool    TxBeginner
	log     *zap.Logger
	vatRate decimal.Decimal
}

func NewInventoryService(repo InventoryRepo, pool TxBeginner, log *zap.Logger, vatRate decimal.Decimal) *InventoryService {
	return &InventoryService{
		repo:    repo,
		pool:    pool,
		log:     log,
		vatRate: vatRate,
	}
}

// =============================================================================
// SubtractStock  –  FIFO stock deduction for a completed/confirmed order
// =============================================================================
//
// Algorithm:
//  1. Open a serializable transaction.
//  2. For each order item:
//     a. Lock the inventory row (SELECT FOR UPDATE).
//     b. Verify sufficient on-hand stock.
//     c. Fetch all available batch items ordered oldest-received-first
//        (SELECT … FOR UPDATE OF batch_items).
//     d. Walk batches, consuming as many units as needed (FIFO waterfall).
//     e. For each batch consumed: insert an inventory_movement row with the
//        batch's landed_cost as unit_cost_snapshot.
//     f. UPDATE inventory.quantity_on_hand in a single atomic statement.
//  3. Commit — any error triggers a full rollback.

func (s *InventoryService) SubtractStock(
	ctx context.Context,
	items []DeductionItem,
) ([]DeductionResult, error) {
	if len(items) == 0 {
		return nil, nil
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel:   pgx.Serializable,
		AccessMode: pgx.ReadWrite,
	})
	if err != nil {
		return nil, fmt.Errorf("SubtractStock: begin tx: %w", err)
	}
	defer func() {
		// Rollback is a no-op if the transaction was already committed.
		if rbErr := tx.Rollback(ctx); rbErr != nil && !errors.Is(rbErr, pgx.ErrTxClosed) {
			s.log.Error("SubtractStock: rollback failed", zap.Error(rbErr))
		}
	}()

	results := make([]DeductionResult, 0, len(items))

	for _, item := range items {
		result, err := s.deductSingleVariant(ctx, tx, item)
		if err != nil {
			return nil, fmt.Errorf("SubtractStock: variant %s: %w", item.VariantID, err)
		}
		results = append(results, *result)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("SubtractStock: commit: %w", err)
	}

	s.log.Info("SubtractStock completed",
		zap.Int("variants_processed", len(results)),
	)
	return results, nil
}

// deductSingleVariant runs the FIFO waterfall for one variant inside an existing tx.
func (s *InventoryService) deductSingleVariant(
	ctx context.Context,
	tx pgx.Tx,
	item DeductionItem,
) (*DeductionResult, error) {
	// -------------------------------------------------------------------------
	// 1. Lock inventory row – prevents concurrent deductions for the same SKU.
	// -------------------------------------------------------------------------
	inv, err := s.repo.GetByVariantIDForUpdate(ctx, tx, item.VariantID)
	if err != nil {
		return nil, err
	}

	// -------------------------------------------------------------------------
	// 2. Pre-flight availability check.
	// -------------------------------------------------------------------------
	available := inv.QuantityOnHand - inv.QuantityReserved
	if available < item.Quantity {
		return nil, &InsufficientStockError{
			VariantID: item.VariantID,
			Needed:    item.Quantity,
			Available: available,
		}
	}

	// -------------------------------------------------------------------------
	// 3. Fetch FIFO batch items (oldest received first, still-unsold only).
	// -------------------------------------------------------------------------
	batches, err := s.repo.GetFIFOBatchItems(ctx, tx, item.VariantID)
	if err != nil {
		return nil, err
	}
	if len(batches) == 0 {
		return nil, fmt.Errorf("%w: %s", ErrNoFIFOBatches, item.VariantID)
	}

	// -------------------------------------------------------------------------
	// 4. FIFO waterfall – consume batches until the required quantity is met.
	// -------------------------------------------------------------------------
	remaining := item.Quantity
	var movements []domain.InventoryMovement
	var totalCOGSValue decimal.Decimal // sum of (qty * landed_cost) across touched batches

	currentQtyBefore := inv.QuantityOnHand

	for _, batch := range batches {
		if remaining == 0 {
			break
		}

		available := batch.Remaining()
		if available == 0 {
			continue
		}

		consume := min(remaining, available)
		remaining -= consume

		// Cost for this deduction slice
		costSlice := batch.LandedCostPerUnit.Mul(decimal.NewFromInt(int64(consume)))
		totalCOGSValue = totalCOGSValue.Add(costSlice)

		qtyAfter := currentQtyBefore - consume

		chID := &item.ChannelID
		batchItemID := batch.BatchItemID
		orderID := item.OrderID

		movement := domain.InventoryMovement{
			ID:               uuid.New(),
			VariantID:        item.VariantID,
			BatchItemID:      &batchItemID,
			OrderID:          &orderID,
			MovementType:     domain.MovementTypeSaleOut,
			Quantity:         -consume, // negative = stock leaving warehouse
			QuantityBefore:   currentQtyBefore,
			QuantityAfter:    qtyAfter,
			UnitCostSnapshot: &batch.LandedCostPerUnit,
			ChannelID:        chID,
		}

		if err := s.repo.InsertMovement(ctx, tx, &movement); err != nil {
			return nil, fmt.Errorf("InsertMovement (batch %s, consume %d): %w",
				batch.BatchItemID, consume, err)
		}

		movements = append(movements, movement)
		currentQtyBefore = qtyAfter // slide the window for the next batch slice
	}

	// -------------------------------------------------------------------------
	// 5. Sanity check: FIFO batches must fully cover the requested quantity.
	// -------------------------------------------------------------------------
	if remaining > 0 {
		return nil, &InsufficientStockError{
			VariantID: item.VariantID,
			Needed:    item.Quantity,
			Available: item.Quantity - remaining,
		}
	}

	// -------------------------------------------------------------------------
	// 6. Atomic on-hand deduction.
	// -------------------------------------------------------------------------
	if _, err := s.repo.DeductOnHand(ctx, tx, item.VariantID, item.Quantity); err != nil {
		return nil, err
	}

	// -------------------------------------------------------------------------
	// 7. Weighted average COGS per unit.
	// -------------------------------------------------------------------------
	weightedCOGS := totalCOGSValue.Div(decimal.NewFromInt(int64(item.Quantity))).Round(4)

	s.log.Info("FIFO deduction",
		zap.String("variant_id", item.VariantID.String()),
		zap.Int("quantity", item.Quantity),
		zap.String("weighted_cogs", weightedCOGS.String()),
		zap.Int("batches_touched", len(movements)),
	)

	return &DeductionResult{
		VariantID:     item.VariantID,
		TotalDeducted: item.Quantity,
		WeightedCOGS:  weightedCOGS,
		Movements:     movements,
	}, nil
}

// =============================================================================
// ReserveStock  –  pre-payment reservation (prevents overselling)
// =============================================================================

func (s *InventoryService) ReserveStock(
	ctx context.Context,
	requests []ReserveRequest,
) ([]*domain.StockReservation, error) {
	if len(requests) == 0 {
		return nil, nil
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel:   pgx.RepeatableRead,
		AccessMode: pgx.ReadWrite,
	})
	if err != nil {
		return nil, fmt.Errorf("ReserveStock: begin tx: %w", err)
	}
	defer func() {
		if rbErr := tx.Rollback(ctx); rbErr != nil && !errors.Is(rbErr, pgx.ErrTxClosed) {
			s.log.Error("ReserveStock: rollback failed", zap.Error(rbErr))
		}
	}()

	var reservations []*domain.StockReservation

	for _, req := range requests {
		// Lock the inventory row before reading available qty.
		inv, err := s.repo.GetByVariantIDForUpdate(ctx, tx, req.VariantID)
		if err != nil {
			return nil, fmt.Errorf("ReserveStock: lock variant %s: %w", req.VariantID, err)
		}

		if inv.QuantityAvailable < req.Quantity {
			return nil, &InsufficientStockError{
				VariantID: req.VariantID,
				Needed:    req.Quantity,
				Available: inv.QuantityAvailable,
			}
		}

		now := time.Now().UTC()
		res := &domain.StockReservation{
			ID:        uuid.New(),
			OrderID:   req.OrderID,
			VariantID: req.VariantID,
			Quantity:  req.Quantity,
			ExpiresAt: now.Add(req.ReservationTTL),
			IsActive:  true,
			CreatedAt: now,
			UpdatedAt: now,
		}

		if err := s.repo.InsertReservation(ctx, tx, res); err != nil {
			return nil, fmt.Errorf("ReserveStock: insert reservation for variant %s: %w", req.VariantID, err)
		}

		// Record the reservation in the movement ledger.
		chID := req.ChannelID
		movement := &domain.InventoryMovement{
			ID:             uuid.New(),
			VariantID:      req.VariantID,
			OrderID:        &req.OrderID,
			ReservationID:  &res.ID,
			MovementType:   domain.MovementTypeReservation,
			Quantity:       -req.Quantity,
			QuantityBefore: inv.QuantityAvailable,
			QuantityAfter:  inv.QuantityAvailable - req.Quantity,
			ChannelID:      &chID,
		}
		if err := s.repo.InsertMovement(ctx, tx, movement); err != nil {
			return nil, fmt.Errorf("ReserveStock: movement for variant %s: %w", req.VariantID, err)
		}

		reservations = append(reservations, res)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("ReserveStock: commit: %w", err)
	}

	return reservations, nil
}

// =============================================================================
// ReleaseExpiredReservations  –  called by the Asynq background job
// =============================================================================

func (s *InventoryService) ReleaseExpiredReservations(
	ctx context.Context,
	reservationIDs []uuid.UUID,
) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel:   pgx.RepeatableRead,
		AccessMode: pgx.ReadWrite,
	})
	if err != nil {
		return fmt.Errorf("ReleaseExpiredReservations: begin tx: %w", err)
	}
	defer func() {
		if rbErr := tx.Rollback(ctx); rbErr != nil && !errors.Is(rbErr, pgx.ErrTxClosed) {
			s.log.Error("ReleaseExpiredReservations: rollback", zap.Error(rbErr))
		}
	}()

	for _, id := range reservationIDs {
		if err := s.repo.ReleaseReservation(ctx, tx, id); err != nil {
			return fmt.Errorf("ReleaseExpiredReservations: release %s: %w", id, err)
		}
	}

	return tx.Commit(ctx)
}

// =============================================================================
// Helper
// =============================================================================

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
