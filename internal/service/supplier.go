// Package service: Supplier + Procurement
//
// SupplierService handles:
//   - CRUD for suppliers
//   - Creating purchase orders and adding line items
//   - ReceivePurchaseOrder: creates a purchase_batch + batch_items,
//     updates inventory, and records purchase_in movements – reusing
//     the existing BatchImportRepository to keep batch logic in one place.
package service

import (
	"context"
	"fmt"
	"time"

	"github.com/dubai-retail/os/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// =============================================================================
// Repository interfaces
// =============================================================================

// SupplierRepo is the DB interface required by SupplierService.
type SupplierRepo interface {
	InsertSupplier(ctx context.Context, s *domain.Supplier) error
	UpdateSupplier(ctx context.Context, s *domain.Supplier) error
	GetSupplierByID(ctx context.Context, id uuid.UUID) (*domain.Supplier, error)
	ListSuppliers(ctx context.Context) ([]domain.Supplier, error)

	InsertPurchaseOrder(ctx context.Context, tx pgx.Tx, po *domain.PurchaseOrder) error
	InsertPOItem(ctx context.Context, tx pgx.Tx, item *domain.POItem) error
	GetPurchaseOrderByID(ctx context.Context, id uuid.UUID) (*domain.PurchaseOrder, error)
	GetPOItems(ctx context.Context, poID uuid.UUID) ([]domain.POItem, error)
	ListPurchaseOrders(ctx context.Context) ([]domain.PurchaseOrder, error)
	UpdatePOStatus(ctx context.Context, tx pgx.Tx, poID uuid.UUID, status domain.POStatus, receivedAt *time.Time) error
	UpdatePOItemReceivedQty(ctx context.Context, tx pgx.Tx, itemID uuid.UUID, qty int) error
}

// ReceiveRepo is the subset of BatchImportRepo used when receiving a PO.
// This reuses the existing batch persistence layer.
type ReceiveRepo interface {
	GetVariantByID(ctx context.Context, id uuid.UUID) (*domain.Variant, error)
	InsertPurchaseBatch(ctx context.Context, tx pgx.Tx, b *domain.PurchaseBatch) error
	InsertBatchItem(ctx context.Context, tx pgx.Tx, bi *domain.BatchItem) error
	UpsertInventory(ctx context.Context, tx pgx.Tx, variantID uuid.UUID, delta int) error
}

// =============================================================================
// Service
// =============================================================================

// SupplierService orchestrates supplier and procurement operations.
type SupplierService struct {
	supplierRepo SupplierRepo
	receiveRepo  ReceiveRepo
	pool         TxBeginner
	log          *zap.Logger
}

// NewSupplierService creates a SupplierService.
func NewSupplierService(
	supplierRepo SupplierRepo,
	receiveRepo ReceiveRepo,
	pool TxBeginner,
	log *zap.Logger,
) *SupplierService {
	return &SupplierService{
		supplierRepo: supplierRepo,
		receiveRepo:  receiveRepo,
		pool:         pool,
		log:          log,
	}
}

// =============================================================================
// Supplier CRUD
// =============================================================================

func (s *SupplierService) CreateSupplier(ctx context.Context, in domain.Supplier) (*domain.Supplier, error) {
	in.IsActive = true
	if err := s.supplierRepo.InsertSupplier(ctx, &in); err != nil {
		return nil, fmt.Errorf("CreateSupplier: %w", err)
	}
	s.log.Info("supplier.created", zap.String("id", in.ID.String()), zap.String("name", in.Name))
	return &in, nil
}

func (s *SupplierService) UpdateSupplier(ctx context.Context, in domain.Supplier) (*domain.Supplier, error) {
	existing, err := s.supplierRepo.GetSupplierByID(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	// Only update supplied non-zero fields
	if in.Name != "" {
		existing.Name = in.Name
	}
	if in.ContactName != "" {
		existing.ContactName = in.ContactName
	}
	if in.Phone != "" {
		existing.Phone = in.Phone
	}
	if in.Email != "" {
		existing.Email = in.Email
	}
	if in.Country != "" {
		existing.Country = in.Country
	}
	if in.LeadTimeDays > 0 {
		existing.LeadTimeDays = in.LeadTimeDays
	}
	if in.MinimumOrderQty > 0 {
		existing.MinimumOrderQty = in.MinimumOrderQty
	}
	if in.Rating > 0 {
		existing.Rating = in.Rating
	}
	if in.Notes != "" {
		existing.Notes = in.Notes
	}
	if err := s.supplierRepo.UpdateSupplier(ctx, existing); err != nil {
		return nil, fmt.Errorf("UpdateSupplier: %w", err)
	}
	return existing, nil
}

func (s *SupplierService) GetSupplier(ctx context.Context, id uuid.UUID) (*domain.Supplier, error) {
	return s.supplierRepo.GetSupplierByID(ctx, id)
}

func (s *SupplierService) ListSuppliers(ctx context.Context) ([]domain.Supplier, error) {
	return s.supplierRepo.ListSuppliers(ctx)
}

// =============================================================================
// Purchase Orders
// =============================================================================

// CreatePOInput is the DTO for opening a purchase order.
type CreatePOInput struct {
	SupplierID      *uuid.UUID `json:"supplier_id"`
	ReferenceNumber string     `json:"reference_number"`
	Notes           string     `json:"notes"`
	Currency        string     `json:"currency"`
	ExpectedAt      *time.Time `json:"expected_at"`
}

// CreatePurchaseOrder opens a new PO with status=draft.
func (s *SupplierService) CreatePurchaseOrder(ctx context.Context, in CreatePOInput) (*domain.PurchaseOrder, error) {
	if in.Currency == "" {
		in.Currency = "AED"
	}
	po := &domain.PurchaseOrder{
		SupplierID:      in.SupplierID,
		Status:          domain.POStatusDraft,
		ReferenceNumber: in.ReferenceNumber,
		Notes:           in.Notes,
		Currency:        in.Currency,
		ExpectedAt:      in.ExpectedAt,
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if err := s.supplierRepo.InsertPurchaseOrder(ctx, tx, po); err != nil {
		return nil, fmt.Errorf("CreatePurchaseOrder: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	s.log.Info("purchase_order.created",
		zap.String("po_id", po.ID.String()),
		zap.String("status", string(po.Status)),
	)
	return po, nil
}

// AddPOItemInput is the DTO for adding a line item to a PO.
type AddPOItemInput struct {
	PurchaseOrderID uuid.UUID       `json:"purchase_order_id"`
	VariantID       uuid.UUID       `json:"variant_id"`
	Quantity        int             `json:"quantity"`
	UnitCost        decimal.Decimal `json:"unit_cost"`
}

// AddPOItem appends a line item to an existing PO.
func (s *SupplierService) AddPOItem(ctx context.Context, in AddPOItemInput) (*domain.POItem, error) {
	if in.Quantity <= 0 {
		return nil, fmt.Errorf("quantity must be positive")
	}
	if in.UnitCost.IsNegative() || in.UnitCost.IsZero() {
		return nil, fmt.Errorf("unit_cost must be positive")
	}

	// Validate variant exists
	if _, err := s.receiveRepo.GetVariantByID(ctx, in.VariantID); err != nil {
		return nil, fmt.Errorf("AddPOItem: variant %s: %w", in.VariantID, err)
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	item := &domain.POItem{
		PurchaseOrderID: in.PurchaseOrderID,
		VariantID:       in.VariantID,
		Quantity:        in.Quantity,
		UnitCost:        in.UnitCost,
	}
	if err := s.supplierRepo.InsertPOItem(ctx, tx, item); err != nil {
		return nil, fmt.Errorf("AddPOItem: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	s.log.Info("purchase_order.item_added",
		zap.String("po_id", in.PurchaseOrderID.String()),
		zap.String("variant_id", in.VariantID.String()),
		zap.Int("qty", in.Quantity),
	)
	return item, nil
}

func (s *SupplierService) ListPurchaseOrders(ctx context.Context) ([]domain.PurchaseOrder, error) {
	return s.supplierRepo.ListPurchaseOrders(ctx)
}

func (s *SupplierService) GetPurchaseOrder(ctx context.Context, id uuid.UUID) (*domain.PurchaseOrder, error) {
	return s.supplierRepo.GetPurchaseOrderByID(ctx, id)
}

// =============================================================================
// Receive Purchase Order → creates inventory batch
// =============================================================================

// ReceiveInput carries optional per-item shipping/customs/insurance cost overrides.
type ReceiveInput struct {
	PurchaseOrderID uuid.UUID       `json:"purchase_order_id"`
	ShippingTotal   decimal.Decimal `json:"shipping_total"`
	CustomsDutyPct  decimal.Decimal `json:"customs_duty_pct"` // e.g. 0.05 = 5%
	InsuranceTotal  decimal.Decimal `json:"insurance_total"`
}

// ReceivePurchaseOrder marks the PO as received, creates a purchase_batch,
// inserts batch_items with landed costs, updates inventory, and records
// purchase_in inventory_movements.
//
// All writes occur inside a ReadCommitted transaction for safety.
func (s *SupplierService) ReceivePurchaseOrder(ctx context.Context, in ReceiveInput) (*domain.PurchaseBatch, error) {
	po, err := s.supplierRepo.GetPurchaseOrderByID(ctx, in.PurchaseOrderID)
	if err != nil {
		return nil, fmt.Errorf("ReceivePurchaseOrder: %w", err)
	}
	if po.Status == domain.POStatusReceived {
		return nil, fmt.Errorf("purchase order %s already received", po.ID)
	}
	if po.Status == domain.POStatusCancelled {
		return nil, fmt.Errorf("purchase order %s is cancelled", po.ID)
	}
	if len(po.Items) == 0 {
		return nil, fmt.Errorf("purchase order %s has no items", po.ID)
	}

	// Compute total units to allocate shipping + insurance per unit
	totalUnits := 0
	for _, item := range po.Items {
		totalUnits += item.Quantity
	}
	shippingPerUnit := decimal.Zero
	insurancePerUnit := decimal.Zero
	if totalUnits > 0 {
		shippingPerUnit = in.ShippingTotal.Div(decimal.NewFromInt(int64(totalUnits)))
		insurancePerUnit = in.InsuranceTotal.Div(decimal.NewFromInt(int64(totalUnits)))
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// --- Create the purchase batch header --------------------------------
	notes := fmt.Sprintf("Received from PO %s", po.ID)
	now := time.Now().UTC()
	batch := &domain.PurchaseBatch{
		ID:         uuid.New(),
		Reference:  po.ReferenceNumber,
		Notes:      &notes,
		ReceivedAt: &now,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := s.receiveRepo.InsertPurchaseBatch(ctx, tx, batch); err != nil {
		return nil, fmt.Errorf("ReceivePurchaseOrder: batch header: %w", err)
	}

	// --- Insert batch items and update inventory -------------------------
	for _, item := range po.Items {
		customsDuty := item.UnitCost.Mul(in.CustomsDutyPct)

		batchItem := &domain.BatchItem{
			ID:                 uuid.New(),
			BatchID:            batch.ID,
			VariantID:          item.VariantID,
			QuantityOrdered:    item.Quantity,
			QuantityReceived:   item.Quantity,
			UnitCost:           item.UnitCost,
			ShippingAllocation: shippingPerUnit,
			CustomsDuty:        customsDuty,
			Insurance:          insurancePerUnit,
			CreatedAt:          now,
			UpdatedAt:          now,
		}
		if err := s.receiveRepo.InsertBatchItem(ctx, tx, batchItem); err != nil {
			return nil, fmt.Errorf("ReceivePurchaseOrder: batch_item variant %s: %w", item.VariantID, err)
		}
		if err := s.receiveRepo.UpsertInventory(ctx, tx, item.VariantID, item.Quantity); err != nil {
			return nil, fmt.Errorf("ReceivePurchaseOrder: inventory variant %s: %w", item.VariantID, err)
		}
		// Update received quantity on PO item
		if err := s.supplierRepo.UpdatePOItemReceivedQty(ctx, tx, item.ID, item.Quantity); err != nil {
			return nil, err
		}
	}

	// --- Mark PO as received ---------------------------------------------
	receivedNow := time.Now().UTC()
	if err := s.supplierRepo.UpdatePOStatus(ctx, tx, po.ID, domain.POStatusReceived, &receivedNow); err != nil {
		return nil, fmt.Errorf("ReceivePurchaseOrder: update status: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("ReceivePurchaseOrder: commit: %w", err)
	}

	s.log.Info("purchase_order.received",
		zap.String("po_id", po.ID.String()),
		zap.String("batch_id", batch.ID.String()),
		zap.Int("total_units", totalUnits),
	)
	return batch, nil
}
