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
// Fakes for ComplianceService
// =============================================================================

// fakeInvoiceGen always returns a simple XML stub.
type fakeInvoiceGen struct{ callCount int }

func (f *fakeInvoiceGen) Serialize(_ *domain.EInvoice, _ decimal.Decimal) ([]byte, error) {
	f.callCount++
	return []byte(`<?xml version="1.0"?><Invoice><cbc:ID>TEST</cbc:ID></Invoice>`), nil
}

// fakeInvoiceStore records calls in memory.
type fakeInvoiceStore struct {
	invoices      []*domain.OrderInvoice
	invoiceNumSeq int
}

func (f *fakeInvoiceStore) NextInvoiceNumber(_ context.Context, _ pgx.Tx) (string, error) {
	f.invoiceNumSeq++
	return fmt.Sprintf("INV-2026-%06d", f.invoiceNumSeq), nil
}

func (f *fakeInvoiceStore) InsertOrderInvoice(_ context.Context, _ pgx.Tx, oi *domain.OrderInvoice) error {
	f.invoices = append(f.invoices, oi)
	return nil
}

func (f *fakeInvoiceStore) GetOrderInvoice(_ context.Context, orderID uuid.UUID) (*domain.OrderInvoice, error) {
	for _, oi := range f.invoices {
		if oi.OrderID == orderID {
			return oi, nil
		}
	}
	return nil, fmt.Errorf("not found")
}

// fakeOrderRepoCompliance just records StampInvoiceNumber calls.
type fakeOrderRepoCompliance struct{ stamped map[uuid.UUID]string }

func newFakeOrderRepoCompliance() *fakeOrderRepoCompliance {
	return &fakeOrderRepoCompliance{stamped: make(map[uuid.UUID]string)}
}
func (f *fakeOrderRepoCompliance) InsertOrder(_ context.Context, _ pgx.Tx, _ *domain.Order) error {
	return nil
}
func (f *fakeOrderRepoCompliance) InsertOrderItem(_ context.Context, _ pgx.Tx, _ *domain.OrderItem) error {
	return nil
}
func (f *fakeOrderRepoCompliance) GetOrderByID(_ context.Context, _ uuid.UUID) (*domain.Order, error) {
	return nil, nil
}
func (f *fakeOrderRepoCompliance) UpdateOrderStatus(_ context.Context, _ pgx.Tx, _ uuid.UUID, _ domain.OrderStatus) error {
	return nil
}
func (f *fakeOrderRepoCompliance) StampInvoiceNumber(_ context.Context, _ pgx.Tx, id uuid.UUID, num string) error {
	f.stamped[id] = num
	return nil
}

// =============================================================================
// Helpers
// =============================================================================

func newComplianceSvc(gen service.InvoiceGenerator, store service.InvoiceStoreRepo, orderRepo service.OrderRepo) *service.ComplianceService {
	addr := domain.AddressUBL{CityName: "Dubai", CountryCode: "AE"}
	return service.NewComplianceService(gen, store, orderRepo, "Test Seller LLC", "100000000000003", addr, zap.NewNop())
}

func orderWithTRN(trn string) *domain.Order {
	return &domain.Order{
		ID:          uuid.New(),
		ChannelID:   uuid.New(),
		CustomerTRN: &trn,
		Currency:    "AED",
		VATType:     domain.VATTypeStandard,
		Status:      domain.OrderStatusConfirmed,
	}
}

func orderWithoutTRN() *domain.Order {
	return &domain.Order{
		ID:        uuid.New(),
		ChannelID: uuid.New(),
		Currency:  "AED",
		VATType:   domain.VATTypeStandard,
		Status:    domain.OrderStatusConfirmed,
	}
}

func channel(t domain.ChannelType) *domain.Channel {
	return &domain.Channel{ID: uuid.New(), Type: t, IsActive: true}
}

// =============================================================================
// Evaluate() – pure decision tests (no DB calls)
// =============================================================================

func TestComplianceService_Evaluate_Wholesale_AlwaysEInvoice(t *testing.T) {
	svc := newComplianceSvc(&fakeInvoiceGen{}, &fakeInvoiceStore{}, newFakeOrderRepoCompliance())

	// Wholesale channel without TRN still triggers e-invoice
	d := svc.Evaluate(orderWithoutTRN(), channel(domain.ChannelTypeWholesale))

	if !d.ShouldGenerateEInvoice {
		t.Error("wholesale order must always generate e-invoice")
	}
	if d.InvoiceDocType != domain.InvoiceDocTypeEInvoice {
		t.Errorf("expected einvoice_ubl, got %s", d.InvoiceDocType)
	}
	if d.TriggerReason != service.TriggerReasonWholesale {
		t.Errorf("expected trigger wholesale_order, got %s", d.TriggerReason)
	}
}

func TestComplianceService_Evaluate_B2B_TRN_Triggers_EInvoice(t *testing.T) {
	svc := newComplianceSvc(&fakeInvoiceGen{}, &fakeInvoiceStore{}, newFakeOrderRepoCompliance())

	// POS channel but customer supplies TRN → B2B → e-invoice
	d := svc.Evaluate(orderWithTRN("200987654321003"), channel(domain.ChannelTypePOS))

	if !d.ShouldGenerateEInvoice {
		t.Error("POS order with B2B TRN must generate e-invoice")
	}
	if d.TriggerReason != service.TriggerReasonB2BTRN {
		t.Errorf("expected trigger b2b_customer_trn, got %s", d.TriggerReason)
	}
}

func TestComplianceService_Evaluate_Ecommerce_TRN_Triggers_EInvoice(t *testing.T) {
	svc := newComplianceSvc(&fakeInvoiceGen{}, &fakeInvoiceStore{}, newFakeOrderRepoCompliance())

	d := svc.Evaluate(orderWithTRN("200987654321003"), channel(domain.ChannelTypeEcommerce))

	if !d.ShouldGenerateEInvoice {
		t.Error("ecommerce order with TRN must generate e-invoice")
	}
}

func TestComplianceService_Evaluate_RetailPOS_NoTRN_ReceiptOnly(t *testing.T) {
	svc := newComplianceSvc(&fakeInvoiceGen{}, &fakeInvoiceStore{}, newFakeOrderRepoCompliance())

	d := svc.Evaluate(orderWithoutTRN(), channel(domain.ChannelTypePOS))

	if d.ShouldGenerateEInvoice {
		t.Error("POS without TRN should generate receipt, not e-invoice")
	}
	if d.InvoiceDocType != domain.InvoiceDocTypeReceipt {
		t.Errorf("expected receipt doc type, got %s", d.InvoiceDocType)
	}
	if d.TriggerReason != service.TriggerReasonRetailReceipt {
		t.Errorf("expected retail_pos_no_trn, got %s", d.TriggerReason)
	}
}

func TestComplianceService_Evaluate_Ecommerce_NoTRN_ReceiptOnly(t *testing.T) {
	svc := newComplianceSvc(&fakeInvoiceGen{}, &fakeInvoiceStore{}, newFakeOrderRepoCompliance())

	d := svc.Evaluate(orderWithoutTRN(), channel(domain.ChannelTypeEcommerce))

	if d.ShouldGenerateEInvoice {
		t.Error("ecommerce without TRN should generate receipt, not e-invoice")
	}
	if d.TriggerReason != service.TriggerReasonEcomReceipt {
		t.Errorf("expected ecommerce_no_trn, got %s", d.TriggerReason)
	}
}

// =============================================================================
// Execute() – integration: decision + XML + DB persistence
// =============================================================================

func TestComplianceService_Execute_Wholesale_PersistsXML(t *testing.T) {
	gen := &fakeInvoiceGen{}
	store := &fakeInvoiceStore{}
	orderRepo := newFakeOrderRepoCompliance()
	svc := newComplianceSvc(gen, store, orderRepo)

	order := orderWithoutTRN()
	order.Items = []domain.OrderItem{
		{
			ID: uuid.New(), OrderID: order.ID, VariantID: uuid.New(),
			Quantity: 5, UnitPrice: decimal.NewFromFloat(100),
			VATRate: decimal.NewFromFloat(0.05), VATAmount: decimal.NewFromFloat(25),
			LineTotal: decimal.NewFromFloat(525),
		},
	}
	order.Subtotal = decimal.NewFromFloat(500)
	order.VATAmount = decimal.NewFromFloat(25)
	order.TotalAmount = decimal.NewFromFloat(525)

	oi, err := svc.Execute(
		context.Background(),
		&fakeTx{}, // transaction fake from inventory_test.go (same package)
		order,
		channel(domain.ChannelTypeWholesale),
		decimal.NewFromInt(1),
	)

	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if oi.InvoiceType != domain.InvoiceDocTypeEInvoice {
		t.Errorf("expected einvoice_ubl, got %s", oi.InvoiceType)
	}
	if oi.XMLContent == nil || len(*oi.XMLContent) == 0 {
		t.Error("expected XML content to be populated")
	}
	if gen.callCount != 1 {
		t.Errorf("expected InvoiceGenerator.Serialize to be called once, got %d", gen.callCount)
	}
	if len(store.invoices) != 1 {
		t.Errorf("expected 1 invoice persisted, got %d", len(store.invoices))
	}
	if _, stamped := orderRepo.stamped[order.ID]; !stamped {
		t.Error("expected invoice number to be stamped on order")
	}
}

func TestComplianceService_Execute_RetailPOS_NoXMLGenerated(t *testing.T) {
	gen := &fakeInvoiceGen{}
	store := &fakeInvoiceStore{}
	svc := newComplianceSvc(gen, store, newFakeOrderRepoCompliance())

	order := orderWithoutTRN()
	oi, err := svc.Execute(
		context.Background(),
		&fakeTx{},
		order,
		channel(domain.ChannelTypePOS),
		decimal.NewFromInt(1),
	)

	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if oi.InvoiceType != domain.InvoiceDocTypeReceipt {
		t.Errorf("expected receipt, got %s", oi.InvoiceType)
	}
	if oi.XMLContent != nil {
		t.Error("receipt document must not contain XML")
	}
	if gen.callCount != 0 {
		t.Errorf("InvoiceGenerator should NOT be called for retail receipt, got %d calls", gen.callCount)
	}
}

func TestComplianceService_Execute_ZeroRated_Exchange_AEDTaxTotal(t *testing.T) {
	gen := &fakeInvoiceGen{}
	store := &fakeInvoiceStore{}
	svc := newComplianceSvc(gen, store, newFakeOrderRepoCompliance())

	trn := "200100200300003"
	order := orderWithTRN(trn)
	order.VATType = domain.VATTypeZeroRated
	order.Currency = "USD"
	order.Subtotal = decimal.NewFromFloat(1000)
	order.VATAmount = decimal.Zero
	order.TotalAmount = decimal.NewFromFloat(1000)

	oi, err := svc.Execute(
		context.Background(),
		&fakeTx{},
		order,
		channel(domain.ChannelTypeEcommerce),
		decimal.NewFromFloat(3.67), // USD → AED
	)

	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if oi.ExchangeRateToAED.String() != "3.67" {
		t.Errorf("expected exchange rate 3.67, got %s", oi.ExchangeRateToAED)
	}
	if oi.InvoiceType != domain.InvoiceDocTypeEInvoice {
		t.Errorf("B2B TRN must trigger e-invoice even for zero-rated, got %s", oi.InvoiceType)
	}
}

// fakeInvoiceStore needs fmt — add a small import helper used across compliance_test.go
var _ = time.Now // keep time import used
