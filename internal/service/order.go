package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/dubai-retail/os/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// =============================================================================
// Repository interface
// =============================================================================

type OrderRepo interface {
	InsertOrder(ctx context.Context, tx pgx.Tx, o *domain.Order) error
	InsertOrderItem(ctx context.Context, tx pgx.Tx, item *domain.OrderItem) error
	GetOrderByID(ctx context.Context, id uuid.UUID) (*domain.Order, error)
	UpdateOrderStatus(ctx context.Context, tx pgx.Tx, orderID uuid.UUID, status domain.OrderStatus) error
	StampInvoiceNumber(ctx context.Context, tx pgx.Tx, orderID uuid.UUID, invoiceNumber string) error
}

// ChannelRepo is the subset of lookups needed by OrderService for compliance.
type ChannelRepo interface {
	GetChannelByID(ctx context.Context, id uuid.UUID) (*domain.Channel, error)
}

// =============================================================================
// DTOs
// =============================================================================

// OrderLineInput is one item in a process-order request.
type OrderLineInput struct {
	VariantID      uuid.UUID       `json:"variant_id"`
	Quantity       int             `json:"quantity"`
	DiscountAmount decimal.Decimal `json:"discount_amount"`
}

// ProcessOrderInput is the full request to create and confirm a new order.
type ProcessOrderInput struct {
	ChannelID     uuid.UUID              `json:"channel_id"`
	// CustomerID links the order to a registered customer (optional; nil = guest).
	// When set, the order row records customer_id and LoyaltyService can be called
	// post-commit to award points for the completed order.
	CustomerID    *uuid.UUID             `json:"customer_id,omitempty"`
	CustomerName  *string                `json:"customer_name,omitempty"`
	CustomerEmail *string                `json:"customer_email,omitempty"`
	CustomerPhone *string                `json:"customer_phone,omitempty"`
	// CustomerTRN triggers B2B e-invoice when present.
	CustomerTRN   *string                `json:"customer_trn,omitempty"`
	// CustomerTier drives tiered promotion selection.
	CustomerTier  *domain.CustomerTier   `json:"customer_tier,omitempty"`
	// VATType: empty = standard (5%), "zero_rated" = exports.
	VATType       domain.VATType         `json:"vat_type"`
	// ExchangeRateToAED: set to the UAE Central Bank rate when billing in a
	// foreign currency. Defaults to 1.0 (AED orders).
	ExchangeRateToAED decimal.Decimal    `json:"exchange_rate_to_aed"`
	ShippingAddr  *domain.ShippingAddress `json:"shipping_address,omitempty"`
	Lines         []OrderLineInput        `json:"lines"`
	Notes         *string                 `json:"notes,omitempty"`
}

// ProcessOrderResult is returned after a successful order creation.
type ProcessOrderResult struct {
	Order         *domain.Order         `json:"order"`
	FIFOResults   []DeductionResult     `json:"fifo_results"`
	Invoice       *domain.OrderInvoice  `json:"invoice,omitempty"`
}

// =============================================================================
// OrderService
// =============================================================================

type OrderService struct {
	orderRepo    OrderRepo
	channelRepo  ChannelRepo
	inventorySvc *InventoryService
	priceResolver *PriceResolver
	compliance   *ComplianceService
	pool         TxBeginner
	vatRate      decimal.Decimal
	log          *zap.Logger
}

func NewOrderService(
	orderRepo OrderRepo,
	channelRepo ChannelRepo,
	inventorySvc *InventoryService,
	priceResolver *PriceResolver,
	compliance *ComplianceService,
	pool TxBeginner,
	vatRate decimal.Decimal,
	log *zap.Logger,
) *OrderService {
	return &OrderService{
		orderRepo:     orderRepo,
		channelRepo:   channelRepo,
		inventorySvc:  inventorySvc,
		priceResolver: priceResolver,
		compliance:    compliance,
		pool:          pool,
		vatRate:       vatRate,
		log:           log,
	}
}

// =============================================================================
// ProcessOrder  –  Full order lifecycle with compliance trigger
// =============================================================================
//
// Transaction design:
//   Tx 1 [Serializable]  – SubtractStock (FIFO stock deduction)
//   Tx 2 [ReadCommitted] – InsertOrder + InsertOrderItems +
//                          compliance.Execute (invoice_number seq + order_invoices)
//
// All three are atomic within Tx 2; Tx 1 is independent but runs first.
// On Tx 2 failure after Tx 1 succeeds a compensation log is emitted.

func (s *OrderService) ProcessOrder(
	ctx context.Context,
	input ProcessOrderInput,
) (*ProcessOrderResult, error) {
	if err := validateOrderInput(input); err != nil {
		return nil, err
	}

	// Normalise exchange rate.
	exchangeRate := input.ExchangeRateToAED
	if exchangeRate.IsZero() {
		exchangeRate = decimal.NewFromInt(1)
	}

	// -------------------------------------------------------------------------
	// 1. Fetch channel (needed for compliance trigger + audit log).
	// -------------------------------------------------------------------------
	ch, err := s.channelRepo.GetChannelByID(ctx, input.ChannelID)
	if err != nil {
		return nil, fmt.Errorf("ProcessOrder: channel lookup: %w", err)
	}

	// -------------------------------------------------------------------------
	// 2. Resolve effective prices via PriceResolver
	//    (promotion → standard channel price fallback, VAT in AED).
	// -------------------------------------------------------------------------
	type resolvedLine struct {
		variantID uuid.UUID
		quantity  int
		unitPrice decimal.Decimal
		discount  decimal.Decimal
		vatRate   decimal.Decimal
		vatAED    decimal.Decimal
		source    domain.PriceSource
	}

	orderID := uuid.New()
	lines := make([]resolvedLine, 0, len(input.Lines))
	deductItems := make([]DeductionItem, 0, len(input.Lines))

	isZeroRated := input.VATType == domain.VATTypeZeroRated || input.VATType == domain.VATTypeExempt

	for _, li := range input.Lines {
		req := PriceResolveRequest{
			VariantID:         li.VariantID,
			ChannelID:         input.ChannelID,
			CustomerTier:      input.CustomerTier,
			ExchangeRateToAED: exchangeRate,
		}

		var pr *domain.PriceResult
		if isZeroRated {
			pr, err = s.priceResolver.ResolveZeroRated(ctx, req)
		} else {
			pr, err = s.priceResolver.Resolve(ctx, req)
		}
		if err != nil {
			return nil, fmt.Errorf("ProcessOrder: price resolve variant %s: %w", li.VariantID, err)
		}

		lines = append(lines, resolvedLine{
			variantID: li.VariantID,
			quantity:  li.Quantity,
			unitPrice: pr.NetPrice,
			discount:  li.DiscountAmount,
			vatRate:   pr.VATRate,
			vatAED:    pr.VATAmountAED,
			source:    pr.PriceSource,
		})

		deductItems = append(deductItems, DeductionItem{
			VariantID: li.VariantID,
			OrderID:   orderID,
			ChannelID: input.ChannelID,
			Quantity:  li.Quantity,
		})
	}

	// -------------------------------------------------------------------------
	// 3. FIFO stock deduction (Serializable tx inside SubtractStock).
	// -------------------------------------------------------------------------
	fifoResults, err := s.inventorySvc.SubtractStock(ctx, deductItems)
	if err != nil {
		return nil, err
	}

	fifoByVariant := make(map[uuid.UUID]DeductionResult, len(fifoResults))
	for _, r := range fifoResults {
		fifoByVariant[r.VariantID] = r
	}

	// -------------------------------------------------------------------------
	// 4. Build order + line items (financials, COGS stamping).
	// -------------------------------------------------------------------------
	vatType := input.VATType
	if vatType == "" {
		vatType = domain.VATTypeStandard
	}

	order := &domain.Order{
		ID:              orderID,
		ChannelID:       input.ChannelID,
		CustomerID:      input.CustomerID,
		CustomerName:    input.CustomerName,
		CustomerEmail:   input.CustomerEmail,
		CustomerPhone:   input.CustomerPhone,
		CustomerTRN:     input.CustomerTRN,
		ShippingAddress: input.ShippingAddr,
		Currency:        "AED",
		VATType:         vatType,
		Status:          domain.OrderStatusConfirmed,
		PaymentStatus:   domain.PaymentStatusUnpaid,
		Notes:           input.Notes,
	}

	orderItems := make([]domain.OrderItem, 0, len(lines))
	var totalSubtotal, totalVAT, totalDiscount decimal.Decimal

	for _, l := range lines {
		qty := decimal.NewFromInt(int64(l.quantity))
		lineNet := l.unitPrice.Mul(qty).Sub(l.discount)
		vatAmount := lineNet.Mul(l.vatRate).Round(2)
		lineTotal := lineNet.Add(vatAmount)

		var cogsPerUnit, totalCOGS *decimal.Decimal
		if fr, ok := fifoByVariant[l.variantID]; ok {
			cogs := fr.WeightedCOGS
			cogsPerUnit = &cogs
			t := cogs.Mul(qty)
			totalCOGS = &t
		}

		item := domain.OrderItem{
			ID:             uuid.New(),
			OrderID:        orderID,
			VariantID:      l.variantID,
			Quantity:       l.quantity,
			UnitPrice:      l.unitPrice,
			DiscountAmount: l.discount,
			VATRate:        l.vatRate,
			VATAmount:      vatAmount,
			LineTotal:      lineTotal,
			COGSPerUnit:    cogsPerUnit,
			TotalCOGS:      totalCOGS,
		}
		orderItems = append(orderItems, item)

		totalSubtotal = totalSubtotal.Add(lineNet)
		totalVAT = totalVAT.Add(vatAmount)
		totalDiscount = totalDiscount.Add(l.discount)
	}

	order.Subtotal = totalSubtotal
	order.VATAmount = totalVAT
	order.DiscountAmount = totalDiscount
	order.TotalAmount = totalSubtotal.Add(totalVAT)
	order.Items = orderItems

	// -------------------------------------------------------------------------
	// 5. Persist order + items + compliance document in one ReadCommitted tx.
	// -------------------------------------------------------------------------
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel:   pgx.ReadCommitted,
		AccessMode: pgx.ReadWrite,
	})
	if err != nil {
		_ = s.compensate(ctx, deductItems, orderID)
		return nil, fmt.Errorf("ProcessOrder: begin tx: %w", err)
	}
	defer func() {
		if rbErr := tx.Rollback(ctx); rbErr != nil && !errors.Is(rbErr, pgx.ErrTxClosed) {
			s.log.Error("ProcessOrder: rollback", zap.Error(rbErr))
		}
	}()

	if err := s.orderRepo.InsertOrder(ctx, tx, order); err != nil {
		_ = s.compensate(ctx, deductItems, orderID)
		return nil, fmt.Errorf("ProcessOrder: insert order: %w", err)
	}

	for i := range orderItems {
		if err := s.orderRepo.InsertOrderItem(ctx, tx, &orderItems[i]); err != nil {
			_ = s.compensate(ctx, deductItems, orderID)
			return nil, fmt.Errorf("ProcessOrder: insert order item %d: %w", i, err)
		}
	}

	// 5c. Compliance trigger: generate + persist invoice or receipt.
	orderInvoice, err := s.compliance.Execute(ctx, tx, order, ch, exchangeRate)
	if err != nil {
		_ = s.compensate(ctx, deductItems, orderID)
		return nil, fmt.Errorf("ProcessOrder: compliance.Execute: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		_ = s.compensate(ctx, deductItems, orderID)
		return nil, fmt.Errorf("ProcessOrder: commit: %w", err)
	}

	// -------------------------------------------------------------------------
	// 6. Audit log (after commit so log reflects committed state).
	// -------------------------------------------------------------------------
	s.log.Info("order.processed",
		zap.String("order_id", orderID.String()),
		zap.String("channel_id", input.ChannelID.String()),
		zap.String("channel_type", string(ch.Type)),
		zap.String("subtotal_aed", order.Subtotal.String()),
		zap.String("vat_aed", order.VATAmount.String()),
		zap.String("total_aed", order.TotalAmount.String()),
		zap.String("exchange_rate", exchangeRate.String()),
		zap.Int("line_count", len(orderItems)),
		zap.String("invoice_type", string(orderInvoice.InvoiceType)),
		zap.String("invoice_number", orderInvoice.InvoiceNumber),
		zap.String("trigger_reason", orderInvoice.TriggerReason),
	)

	for _, fr := range fifoResults {
		s.log.Info("stock.movement.cogs",
			zap.String("order_id", orderID.String()),
			zap.String("variant_id", fr.VariantID.String()),
			zap.Int("qty_deducted", fr.TotalDeducted),
			zap.String("weighted_cogs_per_unit", fr.WeightedCOGS.String()),
			zap.Int("batches_touched", len(fr.Movements)),
		)
	}

	return &ProcessOrderResult{
		Order:       order,
		FIFOResults: fifoResults,
		Invoice:     orderInvoice,
	}, nil
}

// GetOrder fetches an order with all line items.
func (s *OrderService) GetOrder(ctx context.Context, id uuid.UUID) (*domain.Order, error) {
	o, err := s.orderRepo.GetOrderByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("order %s not found", id)
		}
		return nil, err
	}
	return o, nil
}

func (s *OrderService) ListOrders(ctx context.Context, filters domain.OrderListFilters) (*domain.PageResponse[domain.OrderListItem], error) {
	listRepo, ok := s.orderRepo.(interface {
		ListOrders(ctx context.Context, filters domain.OrderListFilters) ([]domain.OrderListItem, int, error)
	})
	if !ok {
		return nil, fmt.Errorf("ListOrders: repository does not support list queries")
	}

	filters.Page = normalizePage(filters.Page)
	filters.PageSize = normalizePageSize(filters.PageSize)

	items, total, err := listRepo.ListOrders(ctx, filters)
	if err != nil {
		return nil, fmt.Errorf("ListOrders: %w", err)
	}

	return &domain.PageResponse[domain.OrderListItem]{
		Items: items,
		Total: total,
	}, nil
}

// =============================================================================
// Compensation
// =============================================================================

func (s *OrderService) compensate(ctx context.Context, items []DeductionItem, orderID uuid.UUID) error {
	_, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	s.log.Error("stock.compensation_required",
		zap.String("order_id", orderID.String()),
		zap.Int("line_count", len(items)),
		zap.String("action", "order_persist_failed_after_fifo_deduction"),
	)
	return nil
}

// =============================================================================
// Input validation
// =============================================================================

func validateOrderInput(input ProcessOrderInput) error {
	if len(input.Lines) == 0 {
		return fmt.Errorf("order must have at least one line item")
	}
	seen := make(map[uuid.UUID]struct{}, len(input.Lines))
	for i, l := range input.Lines {
		if l.Quantity <= 0 {
			return fmt.Errorf("line %d: quantity must be positive", i+1)
		}
		if l.DiscountAmount.IsNegative() {
			return fmt.Errorf("line %d: discount cannot be negative", i+1)
		}
		if _, dup := seen[l.VariantID]; dup {
			return fmt.Errorf("line %d: duplicate variant %s in same order", i+1, l.VariantID)
		}
		seen[l.VariantID] = struct{}{}
	}
	return nil
}
