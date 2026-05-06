// Package service — POS checkout service.
//
// POSService wraps the existing OrderService and adds POS-specific operations:
// cashier session management, barcode scanning, payment recording, and
// receipt generation. All inventory deduction and VAT logic reuses the
// existing order pipeline unchanged.
package service

import (
	"context"
	"fmt"
	"time"

	"github.com/dubai-retail/os/internal/domain"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// =============================================================================
// Repository interface
// =============================================================================

// POSRepo is the subset of POSRepository required by POSService.
type POSRepo interface {
	GetRegisterByID(ctx context.Context, id uuid.UUID) (*domain.POSRegister, error)
	InsertSession(ctx context.Context, s *domain.POSSession) error
	CloseSession(ctx context.Context, sessionID uuid.UUID, closingCash decimal.Decimal) error
	GetSessionByID(ctx context.Context, id uuid.UUID) (*domain.POSSession, error)
	InsertPayment(ctx context.Context, p *domain.POSPayment) error
	GetPaymentsByOrderID(ctx context.Context, orderID uuid.UUID) ([]domain.POSPayment, error)
	GetVariantByBarcode(ctx context.Context, barcode string) (*domain.Variant, *domain.Product, error)
	GetAvailableStockForVariant(ctx context.Context, variantID uuid.UUID) (int, error)
	GetPOSChannelID(ctx context.Context) (uuid.UUID, error)
	GetPOSChannelPriceForVariant(ctx context.Context, variantID, channelID uuid.UUID) (decimal.Decimal, string, error)
	GetOrderItemsForReceipt(ctx context.Context, orderID uuid.UUID) ([]domain.POSReceiptItem, error)
}

// =============================================================================
// Service
// =============================================================================

// OrderQuerier is the minimal interface POSService needs from the order subsystem.
// *OrderService satisfies this interface; it can be mocked in tests.
type OrderQuerier interface {
	ProcessOrder(ctx context.Context, in ProcessOrderInput) (*ProcessOrderResult, error)
	GetOrder(ctx context.Context, id uuid.UUID) (*domain.Order, error)
}

// POSService provides POS-specific operations layered on top of OrderService.
type POSService struct {
	repo     POSRepo
	orderSvc OrderQuerier
	receipt  *ReceiptService
	log      *zap.Logger
}

// NewPOSService creates a POSService.
// orderSvc must implement OrderQuerier; pass nil to disable order + receipt features
// (barcode scan and session management still work).
func NewPOSService(repo POSRepo, orderSvc OrderQuerier, log *zap.Logger) *POSService {
	return &POSService{
		repo:     repo,
		orderSvc: orderSvc,
		receipt:  NewReceiptService(),
		log:      log,
	}
}

// =============================================================================
// Sessions
// =============================================================================

// OpenSessionInput is the request body for opening a cashier session.
type OpenSessionInput struct {
	RegisterID  uuid.UUID       `json:"register_id"`
	OpenedBy    uuid.UUID       `json:"opened_by"`
	OpeningCash decimal.Decimal `json:"opening_cash"`
}

// OpenSession starts a cashier shift on a register.
func (s *POSService) OpenSession(ctx context.Context, in OpenSessionInput) (*domain.POSSession, error) {
	if _, err := s.repo.GetRegisterByID(ctx, in.RegisterID); err != nil {
		return nil, fmt.Errorf("OpenSession: %w", err)
	}
	session := &domain.POSSession{
		ID:          uuid.New(),
		RegisterID:  in.RegisterID,
		OpenedBy:    in.OpenedBy,
		OpenedAt:    time.Now().UTC(),
		OpeningCash: in.OpeningCash,
	}
	if err := s.repo.InsertSession(ctx, session); err != nil {
		return nil, fmt.Errorf("OpenSession: %w", err)
	}
	s.log.Info("pos.session_opened",
		zap.String("session_id", session.ID.String()),
		zap.String("register_id", in.RegisterID.String()),
	)
	return session, nil
}

// CloseSessionInput is the request body for closing a cashier session.
type CloseSessionInput struct {
	SessionID   uuid.UUID       `json:"session_id"`
	ClosingCash decimal.Decimal `json:"closing_cash"`
}

// CloseSession ends a cashier shift.
func (s *POSService) CloseSession(ctx context.Context, in CloseSessionInput) error {
	if err := s.repo.CloseSession(ctx, in.SessionID, in.ClosingCash); err != nil {
		return fmt.Errorf("CloseSession: %w", err)
	}
	s.log.Info("pos.session_closed", zap.String("session_id", in.SessionID.String()))
	return nil
}

// =============================================================================
// Barcode scan
// =============================================================================

// ScanBarcode looks up a variant by barcode and returns the POS price + stock.
func (s *POSService) ScanBarcode(ctx context.Context, barcode string) (*domain.BarcodeResult, error) {
	if barcode == "" {
		return nil, fmt.Errorf("barcode is required")
	}

	variant, product, err := s.repo.GetVariantByBarcode(ctx, barcode)
	if err != nil {
		return nil, fmt.Errorf("ScanBarcode: %w", err)
	}

	channelID, err := s.repo.GetPOSChannelID(ctx)
	if err != nil {
		return nil, fmt.Errorf("ScanBarcode: get POS channel: %w", err)
	}

	price, currency, err := s.repo.GetPOSChannelPriceForVariant(ctx, variant.ID, channelID)
	if err != nil {
		// Price not yet configured; return zero so the cashier can override.
		price = decimal.Zero
		currency = "AED"
	}

	stock, _ := s.repo.GetAvailableStockForVariant(ctx, variant.ID)

	return &domain.BarcodeResult{
		Variant:  variant,
		Product:  product,
		Price:    price,
		Currency: currency,
		Stock:    stock,
	}, nil
}

// =============================================================================
// POS order creation
// =============================================================================

// CreatePOSOrderInput wraps a standard ProcessOrderInput for POS checkout.
// The channel_id is resolved automatically from the active POS channel.
type CreatePOSOrderInput struct {
	SessionID     *uuid.UUID       `json:"session_id,omitempty"`
	CustomerName  *string          `json:"customer_name,omitempty"`
	CustomerPhone *string          `json:"customer_phone,omitempty"`
	Lines         []OrderLineInput `json:"lines"`
	Notes         *string          `json:"notes,omitempty"`
}

// CreatePOSOrder creates an order via the existing order pipeline using the
// POS channel. Returns the full result including FIFO deductions and invoice.
func (s *POSService) CreatePOSOrder(ctx context.Context, in CreatePOSOrderInput) (*ProcessOrderResult, error) {
	if s.orderSvc == nil {
		return nil, fmt.Errorf("CreatePOSOrder: order service not configured")
	}
	if len(in.Lines) == 0 {
		return nil, fmt.Errorf("CreatePOSOrder: at least one item is required")
	}

	channelID, err := s.repo.GetPOSChannelID(ctx)
	if err != nil {
		return nil, fmt.Errorf("CreatePOSOrder: %w", err)
	}

	result, err := s.orderSvc.ProcessOrder(ctx, ProcessOrderInput{
		ChannelID:     channelID,
		CustomerName:  in.CustomerName,
		CustomerPhone: in.CustomerPhone,
		VATType:       domain.VATTypeStandard,
		Lines:         in.Lines,
		Notes:         in.Notes,
	})
	if err != nil {
		return nil, fmt.Errorf("CreatePOSOrder: %w", err)
	}

	s.log.Info("pos.order_created",
		zap.String("order_id", result.Order.ID.String()),
		zap.Int("items", len(in.Lines)),
	)
	return result, nil
}

// =============================================================================
// Payment
// =============================================================================

// RecordPaymentInput is the request to record a payment for a POS order.
type RecordPaymentInput struct {
	OrderID       uuid.UUID               `json:"order_id"`
	SessionID     *uuid.UUID              `json:"session_id,omitempty"`
	PaymentMethod domain.POSPaymentMethod `json:"payment_method"`
	AmountPaid    decimal.Decimal         `json:"amount_paid"`
	Currency      string                  `json:"currency"`
	Reference     *string                 `json:"reference,omitempty"`
}

// RecordPayment stores a POS payment and returns the receipt.
func (s *POSService) RecordPayment(ctx context.Context, in RecordPaymentInput) (*domain.POSReceipt, error) {
	if in.AmountPaid.LessThanOrEqual(decimal.Zero) {
		return nil, fmt.Errorf("RecordPayment: amount must be positive")
	}
	if in.Currency == "" {
		in.Currency = "AED"
	}

	if err := s.repo.InsertPayment(ctx, &domain.POSPayment{
		OrderID:       in.OrderID,
		SessionID:     in.SessionID,
		PaymentMethod: in.PaymentMethod,
		Amount:        in.AmountPaid,
		Currency:      in.Currency,
		Reference:     in.Reference,
	}); err != nil {
		return nil, fmt.Errorf("RecordPayment: %w", err)
	}

	s.log.Info("pos.payment_recorded",
		zap.String("order_id", in.OrderID.String()),
		zap.String("method", string(in.PaymentMethod)),
		zap.String("amount", in.AmountPaid.String()),
	)

	receipt, err := s.buildReceipt(ctx, in.OrderID, in.PaymentMethod, in.AmountPaid)
	if err != nil {
		s.log.Warn("pos.receipt_build_failed", zap.Error(err))
		return nil, nil
	}
	return receipt, nil
}

// =============================================================================
// Receipt
// =============================================================================

// buildReceipt constructs a POSReceipt for a completed order.
func (s *POSService) buildReceipt(ctx context.Context, orderID uuid.UUID, method domain.POSPaymentMethod, amountPaid decimal.Decimal) (*domain.POSReceipt, error) {
	if s.orderSvc == nil {
		return nil, fmt.Errorf("buildReceipt: order service not configured")
	}
	order, err := s.orderSvc.GetOrder(ctx, orderID)
	if err != nil {
		return nil, fmt.Errorf("buildReceipt: %w", err)
	}

	items, err := s.repo.GetOrderItemsForReceipt(ctx, orderID)
	if err != nil {
		return nil, fmt.Errorf("buildReceipt: %w", err)
	}

	change := amountPaid.Sub(order.TotalAmount)
	if change.IsNegative() {
		change = decimal.Zero
	}

	return &domain.POSReceipt{
		ReceiptID:     "RCP-" + orderID.String()[:8],
		StoreName:     "Dubai Fashion House",
		RegisterName:  "POS Register",
		OrderID:       orderID,
		Items:         items,
		Subtotal:      order.Subtotal,
		DiscountTotal: order.DiscountAmount,
		VATAmount:     order.VATAmount,
		Total:         order.TotalAmount,
		PaymentMethod: method,
		AmountPaid:    amountPaid,
		Change:        change,
		Currency:      order.Currency,
		IssuedAt:      time.Now().UTC(),
	}, nil
}

// GetReceiptHTML returns a printable HTML receipt for an existing paid order.
func (s *POSService) GetReceiptHTML(ctx context.Context, orderID uuid.UUID) (string, error) {
	payments, err := s.repo.GetPaymentsByOrderID(ctx, orderID)
	if err != nil || len(payments) == 0 {
		return "", fmt.Errorf("GetReceiptHTML: no payment found for order")
	}
	p := payments[0]
	receipt, err := s.buildReceipt(ctx, orderID, p.PaymentMethod, p.Amount)
	if err != nil {
		return "", err
	}
	return s.receipt.RenderHTML(receipt)
}
