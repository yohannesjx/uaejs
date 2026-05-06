package service_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/dubai-retail/os/internal/domain"
	"github.com/dubai-retail/os/internal/service"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// =============================================================================
// Fake repository
// =============================================================================

type fakePOSRepo struct {
	registers  map[uuid.UUID]*domain.POSRegister
	sessions   map[uuid.UUID]*domain.POSSession
	payments   map[uuid.UUID][]domain.POSPayment
	variants   map[string]*domain.Variant  // keyed by barcode
	products   map[uuid.UUID]*domain.Product
	stock      map[uuid.UUID]int
	prices     map[uuid.UUID]decimal.Decimal
	posChannel uuid.UUID
	receiptRows map[uuid.UUID][]domain.POSReceiptItem
}

var defaultRegisterID = uuid.MustParse("00000000-0000-0000-0000-000000000010")
var defaultPOSChannelID = uuid.MustParse("00000000-0000-0000-0000-000000000011")

func newFakePOSRepo() *fakePOSRepo {
	variantID := uuid.New()
	productID := uuid.New()
	r := &fakePOSRepo{
		registers: map[uuid.UUID]*domain.POSRegister{
			defaultRegisterID: {ID: defaultRegisterID, Name: "Main Register", Location: "Floor 1", IsActive: true},
		},
		sessions:   make(map[uuid.UUID]*domain.POSSession),
		payments:   make(map[uuid.UUID][]domain.POSPayment),
		posChannel: defaultPOSChannelID,
		variants: map[string]*domain.Variant{
			"123456789": {ID: variantID, SKU: "DSS-001-M-BK", Barcode: strPtr("123456789"), IsActive: true, ProductID: productID},
		},
		products: map[uuid.UUID]*domain.Product{
			productID: {ID: productID, Name: "Classic Abaya", VATType: domain.VATTypeStandard, IsActive: true},
		},
		stock:       map[uuid.UUID]int{variantID: 15},
		prices:      map[uuid.UUID]decimal.Decimal{variantID: decimal.NewFromFloat(150.00)},
		receiptRows: map[uuid.UUID][]domain.POSReceiptItem{},
	}
	return r
}

func strPtr(s string) *string { return &s }

func (r *fakePOSRepo) GetRegisterByID(_ context.Context, id uuid.UUID) (*domain.POSRegister, error) {
	reg, ok := r.registers[id]
	if !ok {
		return nil, fmt.Errorf("register not found")
	}
	return reg, nil
}

func (r *fakePOSRepo) InsertSession(_ context.Context, s *domain.POSSession) error {
	r.sessions[s.ID] = s
	return nil
}

func (r *fakePOSRepo) CloseSession(_ context.Context, sessionID uuid.UUID, cash decimal.Decimal) error {
	s, ok := r.sessions[sessionID]
	if !ok {
		return fmt.Errorf("session not found")
	}
	now := time.Now()
	s.ClosedAt = &now
	s.ClosingCash = &cash
	return nil
}

func (r *fakePOSRepo) GetSessionByID(_ context.Context, id uuid.UUID) (*domain.POSSession, error) {
	s, ok := r.sessions[id]
	if !ok {
		return nil, fmt.Errorf("session not found")
	}
	return s, nil
}

func (r *fakePOSRepo) InsertPayment(_ context.Context, p *domain.POSPayment) error {
	p.ID = uuid.New()
	p.PaidAt = time.Now().UTC()
	r.payments[p.OrderID] = append(r.payments[p.OrderID], *p)
	return nil
}

func (r *fakePOSRepo) GetPaymentsByOrderID(_ context.Context, orderID uuid.UUID) ([]domain.POSPayment, error) {
	return r.payments[orderID], nil
}

func (r *fakePOSRepo) GetVariantByBarcode(_ context.Context, barcode string) (*domain.Variant, *domain.Product, error) {
	v, ok := r.variants[barcode]
	if !ok {
		return nil, nil, fmt.Errorf("barcode not found")
	}
	p := r.products[v.ProductID]
	return v, p, nil
}

func (r *fakePOSRepo) GetAvailableStockForVariant(_ context.Context, id uuid.UUID) (int, error) {
	return r.stock[id], nil
}

func (r *fakePOSRepo) GetPOSChannelID(_ context.Context) (uuid.UUID, error) {
	return r.posChannel, nil
}

func (r *fakePOSRepo) GetPOSChannelPriceForVariant(_ context.Context, variantID, _ uuid.UUID) (decimal.Decimal, string, error) {
	p, ok := r.prices[variantID]
	if !ok {
		return decimal.Zero, "AED", fmt.Errorf("price not found")
	}
	return p, "AED", nil
}

func (r *fakePOSRepo) GetOrderItemsForReceipt(_ context.Context, orderID uuid.UUID) ([]domain.POSReceiptItem, error) {
	return r.receiptRows[orderID], nil
}

// =============================================================================
// Fake order querier (implements service.OrderQuerier)
// =============================================================================

type fakeOrderQuerier struct {
	orders map[uuid.UUID]*domain.Order
}

func newFakeOrderQuerier() *fakeOrderQuerier {
	return &fakeOrderQuerier{orders: make(map[uuid.UUID]*domain.Order)}
}

func (f *fakeOrderQuerier) ProcessOrder(_ context.Context, in service.ProcessOrderInput) (*service.ProcessOrderResult, error) {
	orderID := uuid.New()
	order := &domain.Order{
		ID:             orderID,
		ChannelID:      in.ChannelID,
		Subtotal:       decimal.NewFromFloat(142.86),
		DiscountAmount: decimal.Zero,
		VATAmount:      decimal.NewFromFloat(7.14),
		TotalAmount:    decimal.NewFromFloat(150.00),
		Currency:       "AED",
		Status:         domain.OrderStatusReserved,
	}
	f.orders[orderID] = order
	return &service.ProcessOrderResult{Order: order}, nil
}

func (f *fakeOrderQuerier) GetOrder(_ context.Context, id uuid.UUID) (*domain.Order, error) {
	if o, ok := f.orders[id]; ok {
		return o, nil
	}
	// Return a plausible order for receipt tests.
	return &domain.Order{
		ID:             id,
		Subtotal:       decimal.NewFromFloat(142.86),
		VATAmount:      decimal.NewFromFloat(7.14),
		TotalAmount:    decimal.NewFromFloat(150.00),
		DiscountAmount: decimal.Zero,
		Currency:       "AED",
	}, nil
}

// =============================================================================
// Helper to build POSService with fakes
// =============================================================================

func newTestPOSService() (*service.POSService, *fakePOSRepo) {
	repo := newFakePOSRepo()
	orderQ := newFakeOrderQuerier()
	svc := service.NewPOSService(repo, orderQ, zap.NewNop())
	return svc, repo
}

// =============================================================================
// Tests
// =============================================================================

func TestPOS_OpenSession_Success(t *testing.T) {
	svc, _ := newTestPOSService()
	ctx := context.Background()

	session, err := svc.OpenSession(ctx, service.OpenSessionInput{
		RegisterID:  defaultRegisterID,
		OpenedBy:    uuid.New(),
		OpeningCash: decimal.NewFromFloat(500),
	})
	if err != nil {
		t.Fatalf("OpenSession: %v", err)
	}
	if session.ID == uuid.Nil {
		t.Error("expected non-nil session ID")
	}
	if session.RegisterID != defaultRegisterID {
		t.Errorf("register mismatch: got %s", session.RegisterID)
	}
	if !session.OpeningCash.Equal(decimal.NewFromFloat(500)) {
		t.Errorf("opening cash mismatch: got %s", session.OpeningCash)
	}
}

func TestPOS_OpenSession_InvalidRegister(t *testing.T) {
	svc, _ := newTestPOSService()
	_, err := svc.OpenSession(context.Background(), service.OpenSessionInput{
		RegisterID: uuid.New(), // unknown register
		OpenedBy:   uuid.New(),
	})
	if err == nil {
		t.Error("expected error for unknown register")
	}
}

func TestPOS_CloseSession(t *testing.T) {
	svc, repo := newTestPOSService()
	ctx := context.Background()

	session, _ := svc.OpenSession(ctx, service.OpenSessionInput{
		RegisterID:  defaultRegisterID,
		OpenedBy:    uuid.New(),
		OpeningCash: decimal.NewFromFloat(500),
	})

	err := svc.CloseSession(ctx, service.CloseSessionInput{
		SessionID:   session.ID,
		ClosingCash: decimal.NewFromFloat(730),
	})
	if err != nil {
		t.Fatalf("CloseSession: %v", err)
	}

	stored := repo.sessions[session.ID]
	if stored.ClosedAt == nil {
		t.Error("expected closed_at to be set")
	}
	if stored.ClosingCash == nil || !stored.ClosingCash.Equal(decimal.NewFromFloat(730)) {
		t.Errorf("closing cash mismatch")
	}
}

func TestPOS_ScanBarcode_Found(t *testing.T) {
	svc, _ := newTestPOSService()
	result, err := svc.ScanBarcode(context.Background(), "123456789")
	if err != nil {
		t.Fatalf("ScanBarcode: %v", err)
	}
	if result.Variant == nil {
		t.Fatal("expected variant in result")
	}
	if result.Variant.SKU != "DSS-001-M-BK" {
		t.Errorf("unexpected SKU: %s", result.Variant.SKU)
	}
	if !result.Price.Equal(decimal.NewFromFloat(150.00)) {
		t.Errorf("unexpected price: %s", result.Price)
	}
	if result.Stock != 15 {
		t.Errorf("unexpected stock: %d", result.Stock)
	}
	if result.Currency != "AED" {
		t.Errorf("unexpected currency: %s", result.Currency)
	}
}

func TestPOS_ScanBarcode_NotFound(t *testing.T) {
	svc, _ := newTestPOSService()
	_, err := svc.ScanBarcode(context.Background(), "999999999")
	if err == nil {
		t.Error("expected error for unknown barcode")
	}
}

func TestPOS_ScanBarcode_EmptyBarcode(t *testing.T) {
	svc, _ := newTestPOSService()
	_, err := svc.ScanBarcode(context.Background(), "")
	if err == nil {
		t.Error("expected error for empty barcode")
	}
}

func TestPOS_RecordPayment_Cash(t *testing.T) {
	svc, repo := newTestPOSService()
	ctx := context.Background()

	orderID := uuid.New()
	// Pre-seed receipt data
	repo.receiptRows[orderID] = []domain.POSReceiptItem{
		{SKU: "DSS-001", Name: "Classic Abaya", Qty: 1, UnitPrice: decimal.NewFromFloat(142.86), LineTotal: decimal.NewFromFloat(142.86)},
	}

	_, err := svc.RecordPayment(ctx, service.RecordPaymentInput{
		OrderID:       orderID,
		PaymentMethod: domain.POSPaymentCash,
		AmountPaid:    decimal.NewFromFloat(200),
		Currency:      "AED",
	})
	if err != nil {
		t.Fatalf("RecordPayment: %v", err)
	}

	payments := repo.payments[orderID]
	if len(payments) != 1 {
		t.Fatalf("expected 1 payment, got %d", len(payments))
	}
	if payments[0].PaymentMethod != domain.POSPaymentCash {
		t.Errorf("expected cash, got %s", payments[0].PaymentMethod)
	}
	if !payments[0].Amount.Equal(decimal.NewFromFloat(200)) {
		t.Errorf("amount mismatch: %s", payments[0].Amount)
	}
}

func TestPOS_RecordPayment_Card(t *testing.T) {
	svc, repo := newTestPOSService()
	ctx := context.Background()

	orderID := uuid.New()
	repo.receiptRows[orderID] = []domain.POSReceiptItem{}
	ref := "CARD-TXN-001"

	_, err := svc.RecordPayment(ctx, service.RecordPaymentInput{
		OrderID:       orderID,
		PaymentMethod: domain.POSPaymentCard,
		AmountPaid:    decimal.NewFromFloat(150),
		Currency:      "AED",
		Reference:     &ref,
	})
	if err != nil {
		t.Fatalf("RecordPayment card: %v", err)
	}

	payments := repo.payments[orderID]
	if payments[0].PaymentMethod != domain.POSPaymentCard {
		t.Errorf("expected card payment, got %s", payments[0].PaymentMethod)
	}
	if payments[0].Reference == nil || *payments[0].Reference != "CARD-TXN-001" {
		t.Error("expected reference to be preserved")
	}
}

func TestPOS_RecordPayment_ZeroAmount_Rejected(t *testing.T) {
	svc, _ := newTestPOSService()
	_, err := svc.RecordPayment(context.Background(), service.RecordPaymentInput{
		OrderID:       uuid.New(),
		PaymentMethod: domain.POSPaymentCash,
		AmountPaid:    decimal.Zero,
		Currency:      "AED",
	})
	if err == nil {
		t.Error("expected error for zero payment amount")
	}
}

func TestPOS_ReceiptService_RenderHTML(t *testing.T) {
	rs := service.NewReceiptService()
	receipt := &domain.POSReceipt{
		ReceiptID:     "RCP-12345678",
		StoreName:     "Dubai Fashion House",
		RegisterName:  "Main Register",
		OrderID:       uuid.New(),
		Items: []domain.POSReceiptItem{
			{SKU: "ABC-001", Name: "Test Item", Qty: 2, UnitPrice: decimal.NewFromFloat(50), LineTotal: decimal.NewFromFloat(100)},
		},
		Subtotal:      decimal.NewFromFloat(95.24),
		DiscountTotal: decimal.Zero,
		VATAmount:     decimal.NewFromFloat(4.76),
		Total:         decimal.NewFromFloat(100),
		PaymentMethod: domain.POSPaymentCash,
		AmountPaid:    decimal.NewFromFloat(100),
		Change:        decimal.Zero,
		Currency:      "AED",
		IssuedAt:      time.Now(),
	}

	html, err := rs.RenderHTML(receipt)
	if err != nil {
		t.Fatalf("RenderHTML: %v", err)
	}
	if html == "" {
		t.Error("expected non-empty HTML")
	}
	// Verify key content is present
	for _, want := range []string{"Dubai Fashion House", "RCP-12345678", "Test Item", "4.76", "100"} {
		if !contains(html, want) {
			t.Errorf("HTML missing expected content: %q", want)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
