package service

import (
	"context"
	"fmt"

	"github.com/dubai-retail/os/internal/domain"
	"github.com/dubai-retail/os/internal/invoice"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// =============================================================================
// Interfaces
// =============================================================================

// InvoiceGenerator is satisfied by *invoice.Serializer in production
// and by fakes in unit tests.
type InvoiceGenerator interface {
	Serialize(inv *domain.EInvoice, exchangeRateToAED decimal.Decimal) ([]byte, error)
}

// InvoiceStoreRepo is satisfied by *postgres.InvoiceStoreRepository.
type InvoiceStoreRepo interface {
	NextInvoiceNumber(ctx context.Context, tx pgx.Tx) (string, error)
	InsertOrderInvoice(ctx context.Context, tx pgx.Tx, oi *domain.OrderInvoice) error
	GetOrderInvoice(ctx context.Context, orderID uuid.UUID) (*domain.OrderInvoice, error)
}

// SellerInfoProvider supplies static seller identity for invoice generation.
type SellerInfoProvider interface {
	LegalName() string
	TRN() string
	Address() domain.AddressUBL
}

// =============================================================================
// Trigger reasons (audit trail labels)
// =============================================================================

const (
	TriggerReasonWholesale    = "wholesale_order"
	TriggerReasonB2BTRN       = "b2b_customer_trn"
	TriggerReasonRetailReceipt = "retail_pos_no_trn"
	TriggerReasonEcomReceipt  = "ecommerce_no_trn"
)

// =============================================================================
// TriggerDecision – result of the compliance check
// =============================================================================

// TriggerDecision describes whether and how to generate an invoice document.
type TriggerDecision struct {
	// ShouldGenerateEInvoice is true for wholesale and B2B orders.
	ShouldGenerateEInvoice bool
	// InvoiceDocType is the type of document to persist.
	InvoiceDocType domain.InvoiceDocType
	// TriggerReason is a stable string used in the audit log and DB row.
	TriggerReason string
}

// =============================================================================
// ComplianceService
// =============================================================================

// ComplianceService evaluates whether a given order requires a PINT-AE UBL
// e-invoice or an internal receipt, then generates and persists the document.
//
// Rules (UAE Phase 2, effective July 1, 2027 with early-adoption from March 2026):
//   - WHOLESALE channel  → always generate e-invoice (B2B by definition)
//   - Any channel + customer TRN present → generate e-invoice (B2B transaction)
//   - RETAIL POS / ECOMMERCE without TRN → internal receipt only (B2C)
type ComplianceService struct {
	invoiceGen   InvoiceGenerator
	invoiceStore InvoiceStoreRepo
	orderRepo    OrderRepo
	sellerName   string
	sellerTRN    string
	sellerAddr   domain.AddressUBL
	log          *zap.Logger
}

func NewComplianceService(
	invoiceGen InvoiceGenerator,
	invoiceStore InvoiceStoreRepo,
	orderRepo OrderRepo,
	sellerName, sellerTRN string,
	sellerAddr domain.AddressUBL,
	log *zap.Logger,
) *ComplianceService {
	return &ComplianceService{
		invoiceGen:   invoiceGen,
		invoiceStore: invoiceStore,
		orderRepo:    orderRepo,
		sellerName:   sellerName,
		sellerTRN:    sellerTRN,
		sellerAddr:   sellerAddr,
		log:          log,
	}
}

// =============================================================================
// Evaluate – pure decision logic (no DB side effects)
// =============================================================================

// Evaluate returns the TriggerDecision for an order without touching the DB.
// This is intentionally side-effect-free so it can be tested independently.
func (s *ComplianceService) Evaluate(order *domain.Order, channel *domain.Channel) TriggerDecision {
	// B2B criteria
	isWholesale := channel.Type == domain.ChannelTypeWholesale
	hasB2BTRN := order.CustomerTRN != nil && *order.CustomerTRN != ""

	if isWholesale {
		return TriggerDecision{
			ShouldGenerateEInvoice: true,
			InvoiceDocType:         domain.InvoiceDocTypeEInvoice,
			TriggerReason:          TriggerReasonWholesale,
		}
	}
	if hasB2BTRN {
		return TriggerDecision{
			ShouldGenerateEInvoice: true,
			InvoiceDocType:         domain.InvoiceDocTypeEInvoice,
			TriggerReason:          TriggerReasonB2BTRN,
		}
	}

	// B2C: POS or ecommerce without TRN
	reason := TriggerReasonRetailReceipt
	if channel.Type == domain.ChannelTypeEcommerce {
		reason = TriggerReasonEcomReceipt
	}
	return TriggerDecision{
		ShouldGenerateEInvoice: false,
		InvoiceDocType:         domain.InvoiceDocTypeReceipt,
		TriggerReason:          reason,
	}
}

// =============================================================================
// Execute – generates + persists the invoice document inside the order tx
// =============================================================================

// Execute is called inside the order's ReadCommitted transaction after the
// order row is inserted. It:
//  1. Determines the trigger decision.
//  2. Allocates an invoice number (from the DB sequence).
//  3. Serializes UBL XML if required.
//  4. Inserts the order_invoices row.
//  5. Stamps the invoice number on the order row.
//
// All DB operations use the caller's transaction (tx), ensuring atomicity with
// the order + order_items inserts.
func (s *ComplianceService) Execute(
	ctx context.Context,
	tx pgx.Tx,
	order *domain.Order,
	channel *domain.Channel,
	exchangeRateToAED decimal.Decimal,
) (*domain.OrderInvoice, error) {
	decision := s.Evaluate(order, channel)

	// Allocate invoice number from DB sequence (inside tx = atomically unique).
	invNumber, err := s.invoiceStore.NextInvoiceNumber(ctx, tx)
	if err != nil {
		return nil, fmt.Errorf("ComplianceService.Execute: get invoice number: %w", err)
	}

	oi := &domain.OrderInvoice{
		ID:                uuid.New(),
		OrderID:           order.ID,
		InvoiceType:       decision.InvoiceDocType,
		InvoiceNumber:     invNumber,
		ExchangeRateToAED: exchangeRateToAED,
		TriggerReason:     decision.TriggerReason,
	}

	// Generate UBL XML for B2B / wholesale orders.
	if decision.ShouldGenerateEInvoice {
		xmlBytes, err := s.generateXML(order, invNumber, exchangeRateToAED)
		if err != nil {
			return nil, fmt.Errorf("ComplianceService.Execute: generate XML: %w", err)
		}
		xmlStr := string(xmlBytes)
		oi.XMLContent = &xmlStr
	}

	// Persist the invoice record.
	if err := s.invoiceStore.InsertOrderInvoice(ctx, tx, oi); err != nil {
		return nil, fmt.Errorf("ComplianceService.Execute: insert invoice: %w", err)
	}

	// Stamp invoice number on the order row.
	if err := s.orderRepo.StampInvoiceNumber(ctx, tx, order.ID, invNumber); err != nil {
		return nil, fmt.Errorf("ComplianceService.Execute: stamp invoice number: %w", err)
	}

	s.log.Info("invoice document generated",
		zap.String("order_id", order.ID.String()),
		zap.String("invoice_number", invNumber),
		zap.String("invoice_type", string(decision.InvoiceDocType)),
		zap.String("trigger_reason", decision.TriggerReason),
		zap.Bool("xml_generated", oi.XMLContent != nil),
		zap.String("exchange_rate_aed", exchangeRateToAED.String()),
	)

	return oi, nil
}

// generateXML builds the domain.EInvoice and serialises it to UBL 2.1 XML.
func (s *ComplianceService) generateXML(
	order *domain.Order,
	invNumber string,
	exchangeRateToAED decimal.Decimal,
) ([]byte, error) {
	// Temporarily stamp the invoice number so BuildFromOrder can read it.
	order.InvoiceNumber = &invNumber

	einv, err := invoice.BuildFromOrder(
		order,
		s.sellerName,
		s.sellerTRN,
		s.sellerAddr,
		exchangeRateToAED,
	)
	if err != nil {
		return nil, err
	}

	return s.invoiceGen.Serialize(einv, exchangeRateToAED)
}
