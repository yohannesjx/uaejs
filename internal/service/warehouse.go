// Package service — Multi-Warehouse / Location Management service.
//
// WarehouseService manages stock across multiple physical or logical locations.
// It is additive: tenants that don't configure warehouses are unaffected.
// All inventory deductions for existing channels continue to use the global
// inventory table; this service adds a per-location view on top.
package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/dubai-retail/os/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"go.uber.org/zap"
)

// =============================================================================
// Repository interface
// =============================================================================

// WarehouseRepo is the DB interface required by WarehouseService.
type WarehouseRepo interface {
	InsertWarehouse(ctx context.Context, w *domain.Warehouse) error
	UpdateWarehouse(ctx context.Context, w *domain.Warehouse) error
	GetWarehouseByID(ctx context.Context, id uuid.UUID) (*domain.Warehouse, error)
	ListWarehousesByTenant(ctx context.Context, tenantID uuid.UUID) ([]domain.Warehouse, error)
	GetStockForUpdate(ctx context.Context, tx pgx.Tx, warehouseID, variantID uuid.UUID) (*domain.WarehouseStock, error)
	UpsertStock(ctx context.Context, s *domain.WarehouseStock) error
	UpsertStockTx(ctx context.Context, tx pgx.Tx, s *domain.WarehouseStock) error
	AdjustStockTx(ctx context.Context, tx pgx.Tx, warehouseID, variantID uuid.UUID, delta int) (*domain.WarehouseStock, error)
	ListInventoryByWarehouse(ctx context.Context, warehouseID uuid.UUID) ([]domain.WarehouseStock, error)
	GetStockByWarehouseAndVariant(ctx context.Context, warehouseID, variantID uuid.UUID) (*domain.WarehouseStock, error)
	InsertMovementTx(ctx context.Context, tx pgx.Tx, m *domain.InventoryMovement) error
	EnsureDefaultWarehouse(ctx context.Context, tenantID uuid.UUID) (*domain.Warehouse, error)
	ListInventoryRows(ctx context.Context, tenantID uuid.UUID, warehouseID *uuid.UUID, product, category string, lowStockOnly bool) ([]domain.InventoryListItem, error)
	AdjustGlobalInventoryTx(ctx context.Context, tx pgx.Tx, variantID uuid.UUID, delta int) error
	InsertTransfer(ctx context.Context, t *domain.InventoryTransfer) error
	ReplaceTransferItems(ctx context.Context, transferID uuid.UUID, items []domain.TransferItem) error
	ListTransfers(ctx context.Context, tenantID uuid.UUID) ([]domain.InventoryTransfer, error)
	GetTransferByID(ctx context.Context, tenantID, transferID uuid.UUID) (*domain.InventoryTransfer, error)
	GetTransferItems(ctx context.Context, transferID uuid.UUID) ([]domain.TransferItem, error)
	UpdateTransferMeta(ctx context.Context, t *domain.InventoryTransfer) error
	UpdateTransferStatus(ctx context.Context, transferID uuid.UUID, status domain.TransferStatus) error
}

// =============================================================================
// Service
// =============================================================================

// WarehouseService manages warehouse CRUD, per-location stock, and transfers.
type WarehouseService struct {
	repo WarehouseRepo
	pool TxBeginner
	log  *zap.Logger
}

// NewWarehouseService creates a WarehouseService.
func NewWarehouseService(repo WarehouseRepo, pool TxBeginner, log *zap.Logger) *WarehouseService {
	return &WarehouseService{repo: repo, pool: pool, log: log}
}

// =============================================================================
// Warehouse CRUD
// =============================================================================

// CreateWarehouseInput is the request to create a new warehouse.
type CreateWarehouseInput struct {
	TenantID uuid.UUID            `json:"tenant_id"`
	Name     string               `json:"name"`
	Type     domain.WarehouseType `json:"type"`
	Address  string               `json:"address"`
	City     string               `json:"city"`
	Country  string               `json:"country"`
	Priority int                  `json:"priority"`
}

// CreateWarehouse provisions a new warehouse for a tenant.
func (s *WarehouseService) CreateWarehouse(ctx context.Context, in CreateWarehouseInput) (*domain.Warehouse, error) {
	if in.Name == "" {
		return nil, fmt.Errorf("CreateWarehouse: name is required")
	}
	if in.Type == "" {
		in.Type = domain.WarehouseTypeWarehouse
	}
	if in.Country == "" {
		in.Country = "AE"
	}
	if in.Priority == 0 {
		in.Priority = 100
	}

	w := &domain.Warehouse{
		TenantID: in.TenantID,
		Name:     in.Name,
		Type:     in.Type,
		Address:  in.Address,
		City:     in.City,
		Country:  in.Country,
		IsActive: true,
		Priority: in.Priority,
	}
	if err := s.repo.InsertWarehouse(ctx, w); err != nil {
		return nil, fmt.Errorf("CreateWarehouse: %w", err)
	}
	s.log.Info("warehouse.created",
		zap.String("id", w.ID.String()),
		zap.String("name", w.Name),
		zap.String("tenant", in.TenantID.String()),
	)
	return w, nil
}

// UpdateWarehouse persists mutable warehouse fields.
func (s *WarehouseService) UpdateWarehouse(ctx context.Context, w *domain.Warehouse) error {
	if err := s.repo.UpdateWarehouse(ctx, w); err != nil {
		return fmt.Errorf("UpdateWarehouse: %w", err)
	}
	return nil
}

// GetWarehouse returns a warehouse by ID.
func (s *WarehouseService) GetWarehouse(ctx context.Context, id uuid.UUID) (*domain.Warehouse, error) {
	return s.repo.GetWarehouseByID(ctx, id)
}

// ListWarehouses returns all warehouses for a tenant.
func (s *WarehouseService) ListWarehouses(ctx context.Context, tenantID uuid.UUID) ([]domain.Warehouse, error) {
	if _, err := s.repo.EnsureDefaultWarehouse(ctx, tenantID); err != nil {
		return nil, fmt.Errorf("ListWarehouses: ensure default warehouse: %w", err)
	}
	return s.repo.ListWarehousesByTenant(ctx, tenantID)
}

func (s *WarehouseService) EnsureDefaultWarehouse(ctx context.Context, tenantID uuid.UUID) (*domain.Warehouse, error) {
	return s.repo.EnsureDefaultWarehouse(ctx, tenantID)
}

// =============================================================================
// Stock management
// =============================================================================

// SetStockInput is the request to set absolute stock for a warehouse+variant.
type SetStockInput struct {
	WarehouseID  uuid.UUID `json:"warehouse_id"`
	VariantID    uuid.UUID `json:"variant_id"`
	QtyOnHand    int       `json:"qty_on_hand"`
	ReorderPoint int       `json:"reorder_point"`
	ReorderQty   int       `json:"reorder_qty"`
}

// SetStock creates or replaces the stock record for a warehouse+variant pair.
func (s *WarehouseService) SetStock(ctx context.Context, in SetStockInput) error {
	if in.QtyOnHand < 0 {
		return fmt.Errorf("SetStock: qty_on_hand cannot be negative")
	}
	return s.repo.UpsertStock(ctx, &domain.WarehouseStock{
		WarehouseID:  in.WarehouseID,
		VariantID:    in.VariantID,
		QtyOnHand:    in.QtyOnHand,
		ReorderPoint: in.ReorderPoint,
		ReorderQty:   in.ReorderQty,
	})
}

// GetInventory returns all stock rows for a warehouse.
func (s *WarehouseService) GetInventory(ctx context.Context, warehouseID uuid.UUID) ([]domain.WarehouseStock, error) {
	return s.repo.ListInventoryByWarehouse(ctx, warehouseID)
}

func (s *WarehouseService) ListInventoryRows(
	ctx context.Context,
	tenantID uuid.UUID,
	warehouseID *uuid.UUID,
	product, category string,
	lowStockOnly bool,
) ([]domain.InventoryListItem, error) {
	return s.repo.ListInventoryRows(ctx, tenantID, warehouseID, product, category, lowStockOnly)
}

type AdjustInventoryInput struct {
	WarehouseID    uuid.UUID
	VariantID      uuid.UUID
	AdjustmentType domain.InventoryAdjustmentType
	Quantity       int
	Reason         *string
}

func (s *WarehouseService) AdjustInventory(ctx context.Context, in AdjustInventoryInput) (*domain.WarehouseStock, error) {
	if in.Quantity <= 0 {
		return nil, fmt.Errorf("AdjustInventory: quantity must be positive")
	}
	delta := in.Quantity
	movementType := domain.MovementTypeAdjustmentIn
	if in.AdjustmentType == domain.InventoryAdjustmentDecrease {
		delta = -in.Quantity
		movementType = domain.MovementTypeAdjustmentOut
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel:   pgx.Serializable,
		AccessMode: pgx.ReadWrite,
	})
	if err != nil {
		return nil, fmt.Errorf("AdjustInventory: begin tx: %w", err)
	}
	defer func() {
		if rbErr := tx.Rollback(ctx); rbErr != nil && !errors.Is(rbErr, pgx.ErrTxClosed) {
			s.log.Error("AdjustInventory: rollback failed", zap.Error(rbErr))
		}
	}()

	seed := &domain.WarehouseStock{
		ID:           uuid.New(),
		WarehouseID:  in.WarehouseID,
		VariantID:    in.VariantID,
		QtyOnHand:    0,
		QtyReserved:  0,
		ReorderPoint: 0,
		ReorderQty:   0,
	}
	if err := s.repo.UpsertStockTx(ctx, tx, seed); err != nil {
		return nil, fmt.Errorf("AdjustInventory: ensure warehouse stock row: %w", err)
	}

	before, err := s.repo.GetStockForUpdate(ctx, tx, in.WarehouseID, in.VariantID)
	if err != nil {
		return nil, fmt.Errorf("AdjustInventory: lock stock: %w", err)
	}
	if delta < 0 && before.QtyAvailable < in.Quantity {
		return nil, fmt.Errorf("%w: warehouse %s has %d available, needs %d",
			ErrInsufficientStock, in.WarehouseID, before.QtyAvailable, in.Quantity)
	}

	after, err := s.repo.AdjustStockTx(ctx, tx, in.WarehouseID, in.VariantID, delta)
	if err != nil {
		return nil, fmt.Errorf("AdjustInventory: adjust stock: %w", err)
	}
	if err := s.repo.AdjustGlobalInventoryTx(ctx, tx, in.VariantID, delta); err != nil {
		return nil, fmt.Errorf("AdjustInventory: adjust global inventory: %w", err)
	}

	move := &domain.InventoryMovement{
		VariantID:      in.VariantID,
		MovementType:   movementType,
		Quantity:       delta,
		QuantityBefore: before.QtyOnHand,
		QuantityAfter:  after.QtyOnHand,
		Notes:          in.Reason,
	}
	if err := s.repo.InsertMovementTx(ctx, tx, move); err != nil {
		return nil, fmt.Errorf("AdjustInventory: insert movement: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("AdjustInventory: commit: %w", err)
	}
	return after, nil
}

// =============================================================================
// Warehouse Transfer
// =============================================================================

// Transfer moves stock from one warehouse to another.
//
// Safety guarantees:
//   - Serializable transaction with pessimistic row locks on both stock rows.
//   - Source must have sufficient available stock.
//   - Two inventory_movements rows are inserted: transfer_out and transfer_in.
//   - Global inventory.quantity_on_hand is NOT changed (net neutral).
func (s *WarehouseService) Transfer(ctx context.Context, req domain.TransferRequest) (*domain.TransferResult, error) {
	if req.Quantity <= 0 {
		return nil, fmt.Errorf("Transfer: quantity must be positive")
	}
	if req.FromWarehouseID == req.ToWarehouseID {
		return nil, fmt.Errorf("Transfer: source and destination must be different")
	}
	if req.VariantID == uuid.Nil {
		return nil, fmt.Errorf("Transfer: variant_id is required")
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel:   pgx.Serializable,
		AccessMode: pgx.ReadWrite,
	})
	if err != nil {
		return nil, fmt.Errorf("Transfer: begin tx: %w", err)
	}
	defer func() {
		if rbErr := tx.Rollback(ctx); rbErr != nil && !errors.Is(rbErr, pgx.ErrTxClosed) {
			s.log.Error("Transfer: rollback failed", zap.Error(rbErr))
		}
	}()

	// Lock source stock row.
	fromStock, err := s.repo.GetStockForUpdate(ctx, tx, req.FromWarehouseID, req.VariantID)
	if err != nil {
		return nil, fmt.Errorf("Transfer: lock source: %w", err)
	}
	if fromStock.QtyAvailable < req.Quantity {
		return nil, fmt.Errorf("%w: warehouse %s has %d available, needs %d",
			ErrInsufficientStock, req.FromWarehouseID, fromStock.QtyAvailable, req.Quantity)
	}

	// Deduct source.
	updatedFrom, err := s.repo.AdjustStockTx(ctx, tx, req.FromWarehouseID, req.VariantID, -req.Quantity)
	if err != nil {
		return nil, fmt.Errorf("Transfer: deduct source: %w", err)
	}

	// Add to destination (auto-creates row if missing by upserting first then adjusting).
	updatedTo, err := s.addToDestination(ctx, tx, req.ToWarehouseID, req.VariantID, req.Quantity)
	if err != nil {
		return nil, fmt.Errorf("Transfer: add destination: %w", err)
	}

	// Insert audit movements.
	noteStr := fmt.Sprintf("transfer from %s to %s", req.FromWarehouseID, req.ToWarehouseID)
	if req.Notes != nil {
		noteStr = *req.Notes
	}

	outMove := &domain.InventoryMovement{
		VariantID:      req.VariantID,
		MovementType:   domain.MovementTypeTransferOut,
		Quantity:       -req.Quantity,
		QuantityBefore: fromStock.QtyOnHand,
		QuantityAfter:  updatedFrom.QtyOnHand,
		Notes:          &noteStr,
	}
	inMove := &domain.InventoryMovement{
		VariantID:      req.VariantID,
		MovementType:   domain.MovementTypeTransferIn,
		Quantity:       req.Quantity,
		QuantityBefore: updatedTo.QtyOnHand - req.Quantity,
		QuantityAfter:  updatedTo.QtyOnHand,
		Notes:          &noteStr,
	}

	if err := s.repo.InsertMovementTx(ctx, tx, outMove); err != nil {
		return nil, fmt.Errorf("Transfer: insert out movement: %w", err)
	}
	if err := s.repo.InsertMovementTx(ctx, tx, inMove); err != nil {
		return nil, fmt.Errorf("Transfer: insert in movement: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("Transfer: commit: %w", err)
	}

	s.log.Info("warehouse.transfer_completed",
		zap.String("from", req.FromWarehouseID.String()),
		zap.String("to", req.ToWarehouseID.String()),
		zap.String("variant", req.VariantID.String()),
		zap.Int("qty", req.Quantity),
	)

	return &domain.TransferResult{
		FromStock: updatedFrom,
		ToStock:   updatedTo,
		Movements: []domain.InventoryMovement{*outMove, *inMove},
	}, nil
}

// addToDestination upserts a stock row (if needed) then adjusts by +qty.
func (s *WarehouseService) addToDestination(ctx context.Context, tx pgx.Tx, warehouseID, variantID uuid.UUID, qty int) (*domain.WarehouseStock, error) {
	updated, err := s.repo.AdjustStockTx(ctx, tx, warehouseID, variantID, qty)
	if err == nil {
		return updated, nil
	}
	// Row doesn't exist yet — insert it first.
	if err2 := s.repo.UpsertStockTx(ctx, tx, &domain.WarehouseStock{
		WarehouseID: warehouseID,
		VariantID:   variantID,
		QtyOnHand:   0,
	}); err2 != nil {
		return nil, fmt.Errorf("addToDestination: upsert: %w", err2)
	}
	return s.repo.AdjustStockTx(ctx, tx, warehouseID, variantID, qty)
}

type CreateTransferInput struct {
	TenantID               uuid.UUID
	Reference              string
	OriginWarehouseID      uuid.UUID
	DestinationWarehouseID uuid.UUID
	Notes                  *string
	Tags                   []string
	Items                  []domain.TransferItem
	Status                 domain.TransferStatus
}

func (s *WarehouseService) CreateTransfer(ctx context.Context, in CreateTransferInput) (*domain.InventoryTransfer, error) {
	if in.OriginWarehouseID == in.DestinationWarehouseID {
		return nil, fmt.Errorf("origin and destination cannot be the same")
	}
	if len(in.Items) == 0 {
		return nil, fmt.Errorf("at least one transfer item is required")
	}
	status := in.Status
	if status == "" {
		status = domain.TransferStatusDraft
	}
	tr := &domain.InventoryTransfer{
		ID:                     uuid.New(),
		TenantID:               in.TenantID,
		Reference:              strings.TrimSpace(in.Reference),
		OriginWarehouseID:      in.OriginWarehouseID,
		DestinationWarehouseID: in.DestinationWarehouseID,
		Status:                 status,
		Notes:                  in.Notes,
		Tags:                   in.Tags,
	}
	if err := s.repo.InsertTransfer(ctx, tr); err != nil {
		return nil, err
	}
	for i := range in.Items {
		in.Items[i].TransferID = tr.ID
		if in.Items[i].ID == uuid.Nil {
			in.Items[i].ID = uuid.New()
		}
	}
	if err := s.repo.ReplaceTransferItems(ctx, tr.ID, in.Items); err != nil {
		return nil, err
	}
	tr.Items = in.Items
	return tr, nil
}

func (s *WarehouseService) ListTransfers(ctx context.Context, tenantID uuid.UUID) ([]domain.InventoryTransfer, error) {
	return s.repo.ListTransfers(ctx, tenantID)
}

func (s *WarehouseService) GetTransfer(ctx context.Context, tenantID, transferID uuid.UUID) (*domain.InventoryTransfer, error) {
	tr, err := s.repo.GetTransferByID(ctx, tenantID, transferID)
	if err != nil {
		return nil, err
	}
	items, err := s.repo.GetTransferItems(ctx, transferID)
	if err != nil {
		return nil, err
	}
	tr.Items = items
	return tr, nil
}

func (s *WarehouseService) UpdateTransfer(ctx context.Context, tenantID, transferID uuid.UUID, in CreateTransferInput) (*domain.InventoryTransfer, error) {
	tr, err := s.repo.GetTransferByID(ctx, tenantID, transferID)
	if err != nil {
		return nil, err
	}
	if tr.Status == domain.TransferStatusCompleted || tr.Status == domain.TransferStatusCancelled {
		return nil, fmt.Errorf("cannot edit a completed or cancelled transfer")
	}
	tr.Reference = strings.TrimSpace(in.Reference)
	tr.OriginWarehouseID = in.OriginWarehouseID
	tr.DestinationWarehouseID = in.DestinationWarehouseID
	tr.Notes = in.Notes
	tr.Tags = in.Tags
	if in.OriginWarehouseID == in.DestinationWarehouseID {
		return nil, fmt.Errorf("origin and destination cannot be the same")
	}
	if len(in.Items) == 0 {
		return nil, fmt.Errorf("at least one transfer item is required")
	}
	if err := s.repo.UpdateTransferMeta(ctx, tr); err != nil {
		return nil, err
	}
	for i := range in.Items {
		in.Items[i].TransferID = tr.ID
		if in.Items[i].ID == uuid.Nil {
			in.Items[i].ID = uuid.New()
		}
	}
	if err := s.repo.ReplaceTransferItems(ctx, tr.ID, in.Items); err != nil {
		return nil, err
	}
	tr.Items = in.Items
	return tr, nil
}

func (s *WarehouseService) TransitionTransfer(ctx context.Context, tenantID, transferID uuid.UUID, target domain.TransferStatus) (*domain.InventoryTransfer, error) {
	tr, err := s.GetTransfer(ctx, tenantID, transferID)
	if err != nil {
		return nil, err
	}
	if tr.Status == target {
		return tr, nil
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable, AccessMode: pgx.ReadWrite})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	switch target {
	case domain.TransferStatusPending:
		if tr.Status != domain.TransferStatusDraft {
			return nil, fmt.Errorf("pending transition only allowed from draft")
		}
	case domain.TransferStatusInTransit:
		if tr.Status != domain.TransferStatusPending && tr.Status != domain.TransferStatusDraft {
			return nil, fmt.Errorf("in_transit transition only allowed from draft or pending")
		}
		for _, item := range tr.Items {
			from, err := s.repo.GetStockForUpdate(ctx, tx, tr.OriginWarehouseID, item.VariantID)
			if err != nil {
				return nil, err
			}
			if from.QtyAvailable < item.Quantity {
				return nil, fmt.Errorf("insufficient stock for variant %s", item.VariantID)
			}
			if _, err := s.repo.AdjustStockTx(ctx, tx, tr.OriginWarehouseID, item.VariantID, -item.Quantity); err != nil {
				return nil, err
			}
		}
	case domain.TransferStatusCompleted:
		switch tr.Status {
		case domain.TransferStatusInTransit:
			for _, item := range tr.Items {
				if _, err := s.addToDestination(ctx, tx, tr.DestinationWarehouseID, item.VariantID, item.Quantity); err != nil {
					return nil, err
				}
			}
		case domain.TransferStatusPending, domain.TransferStatusDraft:
			// Support direct completion from pending/draft by applying both legs at once.
			for _, item := range tr.Items {
				from, err := s.repo.GetStockForUpdate(ctx, tx, tr.OriginWarehouseID, item.VariantID)
				if err != nil {
					return nil, err
				}
				if from.QtyAvailable < item.Quantity {
					return nil, fmt.Errorf("insufficient stock for variant %s", item.VariantID)
				}
				if _, err := s.repo.AdjustStockTx(ctx, tx, tr.OriginWarehouseID, item.VariantID, -item.Quantity); err != nil {
					return nil, err
				}
				if _, err := s.addToDestination(ctx, tx, tr.DestinationWarehouseID, item.VariantID, item.Quantity); err != nil {
					return nil, err
				}
			}
		default:
			return nil, fmt.Errorf("completed transition not allowed from %s", tr.Status)
		}
	case domain.TransferStatusCancelled:
		if tr.Status == domain.TransferStatusCompleted {
			return nil, fmt.Errorf("cannot cancel completed transfer")
		}
		if tr.Status == domain.TransferStatusInTransit {
			for _, item := range tr.Items {
				if _, err := s.addToDestination(ctx, tx, tr.OriginWarehouseID, item.VariantID, item.Quantity); err != nil {
					return nil, err
				}
			}
		}
	default:
		return nil, fmt.Errorf("unsupported transfer status transition")
	}

	if err := s.repo.UpdateTransferStatus(ctx, transferID, target); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return s.GetTransfer(ctx, tenantID, transferID)
}
