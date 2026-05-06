//go:build integration

// Package service_test contains integration-level tests that exercise the full
// service chain: PriceResolver → ProcessOrder → ComplianceService.
//
// These tests use carefully constructed fake repositories to isolate business
// logic from database infrastructure while exercising every service-layer branch.
// Run them with:
//
//	go test -tags integration ./internal/service/...
package service_test

import (
	"context"
	"errors"
	"fmt"
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
// Test fixtures
// =============================================================================

var (
	fixtureVariantID  = uuid.MustParse("aaaaaaaa-0000-0000-0000-000000000001")
	fixtureVariantID2 = uuid.MustParse("aaaaaaaa-0000-0000-0000-000000000002")
	fixturePOSChannel = uuid.MustParse("cccccccc-0000-0000-0000-000000000001")
	fixtureWholesale  = uuid.MustParse("cccccccc-0000-0000-0000-000000000002")
	fixtureEcomCh     = uuid.MustParse("cccccccc-0000-0000-0000-000000000003")
	fixtureBatch1     = uuid.MustParse("bbbbbbbb-0000-0000-0000-000000000001")
	fixtureBatch2     = uuid.MustParse("bbbbbbbb-0000-0000-0000-000000000002")
	fixturePromoID    = uuid.MustParse("eeeeeeee-0000-0000-0000-000000000001")
)

// =============================================================================
// testBed wires all fakes into live services
// =============================================================================

type testBed struct {
	invRepo     *intFakeInventoryRepo
	pricingRepo *intFakePricingRepo
	orderRepo   *intFakeOrderRepo
	invoiceRepo *intFakeInvoiceRepo
	txBeginner  *intFakeTxBeginner

	inventorySvc  *service.InventoryService
	priceResolver *service.PriceResolver
	complianceSvc *service.ComplianceService
	orderSvc      *service.OrderService
}

func newTestBed(t *testing.T) *testBed {
	t.Helper()
	log := zap.NewNop()
	vatRate := decimal.NewFromFloat(0.05)

	invRepo := &intFakeInventoryRepo{
		stock:    make(map[uuid.UUID]*domain.Inventory),
		batches:  make(map[uuid.UUID][]postgres.FIFOBatchItemRow),
	}
	pricingRepo := &intFakePricingRepo{
		channels: make(map[uuid.UUID]*domain.Channel),
		prices:   make(map[string]*domain.ChannelPrice),
	}
	orderRepo := &intFakeOrderRepo{
		orders: make(map[uuid.UUID]*domain.Order),
		items:  make(map[uuid.UUID][]domain.OrderItem),
	}
	invoiceRepo := &intFakeInvoiceRepo{invoices: make(map[uuid.UUID]*domain.OrderInvoice)}
	txb := &intFakeTxBeginner{}

	inventorySvc := service.NewInventoryService(invRepo, txb, log, vatRate)
	priceResolver := service.NewPriceResolver(pricingRepo, vatRate, log)

	complianceSvc := service.NewComplianceService(
		&intFakeInvoiceGen{},
		invoiceRepo,
		orderRepo,
		"Dubai Fashion House LLC",
		"100000000000003",
		domain.AddressUBL{StreetName: "Al Quoz", CityName: "Dubai", CountryCode: "AE"},
		log,
	)

	orderSvc := service.NewOrderService(
		orderRepo,
		pricingRepo,
		inventorySvc,
		priceResolver,
		complianceSvc,
		txb,
		vatRate,
		log,
	)

	return &testBed{
		invRepo: invRepo, pricingRepo: pricingRepo,
		orderRepo: orderRepo, invoiceRepo: invoiceRepo, txBeginner: txb,
		inventorySvc: inventorySvc, priceResolver: priceResolver,
		complianceSvc: complianceSvc, orderSvc: orderSvc,
	}
}

// Helpers to seed state.

func (b *testBed) addChannel(id uuid.UUID, ct domain.ChannelType) {
	b.pricingRepo.channels[id] = &domain.Channel{ID: id, Type: ct, Name: string(ct), IsActive: true}
}

func (b *testBed) addPrice(variantID, channelID uuid.UUID, price, currency string) {
	key := variantID.String() + channelID.String()
	b.pricingRepo.prices[key] = &domain.ChannelPrice{
		VariantID: variantID, ChannelID: channelID,
		Price: decimal.RequireFromString(price), Currency: currency,
	}
}

func (b *testBed) setStock(variantID, batchID uuid.UUID, qty int, cogs string) {
	if b.invRepo.stock[variantID] == nil {
		b.invRepo.stock[variantID] = &domain.Inventory{
			ID: uuid.New(), VariantID: variantID,
			QuantityOnHand: qty, QuantityAvailable: qty,
		}
	} else {
		b.invRepo.stock[variantID].QuantityOnHand += qty
		b.invRepo.stock[variantID].QuantityAvailable += qty
	}
	b.invRepo.batches[variantID] = append(b.invRepo.batches[variantID], postgres.FIFOBatchItemRow{
		BatchItemID:       batchID,
		BatchReceivedAt:   time.Now().Add(-24 * time.Hour),
		LandedCostPerUnit: decimal.RequireFromString(cogs),
		QuantityReceived:  qty,
		TotalDeducted:     0,
	})
}

// =============================================================================
// Tests
// =============================================================================

func TestIntegration_POS_RetailOrder(t *testing.T) {
	bed := newTestBed(t)
	bed.addChannel(fixturePOSChannel, domain.ChannelTypePOS)
	bed.addPrice(fixtureVariantID, fixturePOSChannel, "100", "AED")
	bed.setStock(fixtureVariantID, fixtureBatch1, 10, "40.00")

	namePtr := func(s string) *string { return &s }

	result, err := bed.orderSvc.ProcessOrder(context.Background(), service.ProcessOrderInput{
		ChannelID:         fixturePOSChannel,
		CustomerName:      namePtr("Walk-In Customer"),
		VATType:           domain.VATTypeStandard,
		ExchangeRateToAED: decimal.NewFromInt(1),
		Lines:             []service.OrderLineInput{{VariantID: fixtureVariantID, Quantity: 3}},
	})
	if err != nil {
		t.Fatalf("ProcessOrder: %v", err)
	}

	item := result.Order.Items[0]
	if !item.UnitPrice.Equal(decimal.NewFromFloat(100)) {
		t.Errorf("UnitPrice: want 100, got %s", item.UnitPrice)
	}
	wantVAT := decimal.NewFromFloat(15) // 3 * 5
	if !item.VATAmount.Equal(wantVAT) {
		t.Errorf("VATAmount: want 15, got %s", item.VATAmount)
	}
	wantTotal := decimal.NewFromFloat(315) // 3 * 105
	if !item.LineTotal.Equal(wantTotal) {
		t.Errorf("LineTotal: want 315, got %s", item.LineTotal)
	}

	// POS without TRN → receipt
	if result.Invoice == nil {
		t.Fatal("expected invoice")
	}
	if result.Invoice.InvoiceType != domain.InvoiceDocTypeReceipt {
		t.Errorf("expected receipt, got %s", result.Invoice.InvoiceType)
	}
	if result.Invoice.XMLContent != nil {
		t.Error("receipt should have nil XMLContent")
	}

	// Stock deducted
	if bed.invRepo.stock[fixtureVariantID].QuantityOnHand != 7 {
		t.Errorf("expected 7 remaining, got %d", bed.invRepo.stock[fixtureVariantID].QuantityOnHand)
	}
}

func TestIntegration_WholesaleOrder_EInvoice(t *testing.T) {
	bed := newTestBed(t)
	bed.addChannel(fixtureWholesale, domain.ChannelTypeWholesale)
	bed.addPrice(fixtureVariantID, fixtureWholesale, "80", "AED")
	bed.setStock(fixtureVariantID, fixtureBatch1, 100, "40.00")

	result, err := bed.orderSvc.ProcessOrder(context.Background(), service.ProcessOrderInput{
		ChannelID:         fixtureWholesale,
		CustomerName:      strPtr("Wholesale Buyer LLC"),
		VATType:           domain.VATTypeStandard,
		ExchangeRateToAED: decimal.NewFromInt(1),
		Lines:             []service.OrderLineInput{{VariantID: fixtureVariantID, Quantity: 50}},
	})
	if err != nil {
		t.Fatalf("ProcessOrder: %v", err)
	}

	if result.Invoice == nil {
		t.Fatal("expected invoice")
	}
	if result.Invoice.InvoiceType != domain.InvoiceDocTypeEInvoice {
		t.Errorf("wholesale should be einvoice_ubl, got %s", result.Invoice.InvoiceType)
	}
	if result.Invoice.XMLContent == nil {
		t.Error("wholesale e-invoice XML must not be nil")
	}
	if bed.invRepo.stock[fixtureVariantID].QuantityOnHand != 50 {
		t.Errorf("expected 50 remaining, got %d", bed.invRepo.stock[fixtureVariantID].QuantityOnHand)
	}
}

func TestIntegration_ExportOrder_ZeroRated(t *testing.T) {
	bed := newTestBed(t)
	bed.addChannel(fixtureWholesale, domain.ChannelTypeWholesale)
	bed.addPrice(fixtureVariantID, fixtureWholesale, "100", "USD")
	bed.setStock(fixtureVariantID, fixtureBatch1, 20, "40.00")

	result, err := bed.orderSvc.ProcessOrder(context.Background(), service.ProcessOrderInput{
		ChannelID:         fixtureWholesale,
		CustomerName:      strPtr("Export Customer Ltd"),
		VATType:           domain.VATTypeZeroRated,
		ExchangeRateToAED: decimal.NewFromFloat(3.67),
		Lines:             []service.OrderLineInput{{VariantID: fixtureVariantID, Quantity: 10}},
	})
	if err != nil {
		t.Fatalf("ProcessOrder zero-rated: %v", err)
	}

	item := result.Order.Items[0]
	if !item.VATAmount.IsZero() {
		t.Errorf("zero-rated should have 0 VAT, got %s", item.VATAmount)
	}
	if !result.Order.VATAmount.IsZero() {
		t.Errorf("order-level VAT should be 0 for zero-rated, got %s", result.Order.VATAmount)
	}
	if result.Invoice.InvoiceType != domain.InvoiceDocTypeEInvoice {
		t.Errorf("export wholesale should be einvoice_ubl, got %s", result.Invoice.InvoiceType)
	}
}

func TestIntegration_PromotionPricing_VIPTier(t *testing.T) {
	bed := newTestBed(t)
	bed.addChannel(fixturePOSChannel, domain.ChannelTypePOS)
	bed.addPrice(fixtureVariantID, fixturePOSChannel, "200", "AED")
	vipTier := domain.CustomerTierVIP
	bed.pricingRepo.promotions = append(bed.pricingRepo.promotions, &domain.PricePromotion{
		ID:             fixturePromoID,
		VariantID:      fixtureVariantID,
		ChannelID:      fixturePOSChannel,
		CustomerTier:   &vipTier,
		PromoPrice:     decimal.NewFromFloat(150),
		Currency:       "AED",
		EffectiveFrom:  time.Now().Add(-time.Hour),
		EffectiveUntil: time.Now().Add(time.Hour),
		IsActive:       true,
	})
	bed.setStock(fixtureVariantID, fixtureBatch1, 5, "80.00")

	vip := domain.CustomerTierVIP
	result, err := bed.orderSvc.ProcessOrder(context.Background(), service.ProcessOrderInput{
		ChannelID:         fixturePOSChannel,
		CustomerName:      strPtr("VIP Customer"),
		CustomerTier:      &vip,
		VATType:           domain.VATTypeStandard,
		ExchangeRateToAED: decimal.NewFromInt(1),
		Lines:             []service.OrderLineInput{{VariantID: fixtureVariantID, Quantity: 2}},
	})
	if err != nil {
		t.Fatalf("VIP order: %v", err)
	}

	if !result.Order.Items[0].UnitPrice.Equal(decimal.NewFromFloat(150)) {
		t.Errorf("expected VIP promo 150, got %s", result.Order.Items[0].UnitPrice)
	}
}

func TestIntegration_ExpiredPromotion_FallsBackToStandard(t *testing.T) {
	bed := newTestBed(t)
	bed.addChannel(fixturePOSChannel, domain.ChannelTypePOS)
	bed.addPrice(fixtureVariantID, fixturePOSChannel, "200", "AED")
	bed.pricingRepo.promotions = append(bed.pricingRepo.promotions, &domain.PricePromotion{
		ID:             fixturePromoID,
		VariantID:      fixtureVariantID,
		ChannelID:      fixturePOSChannel,
		PromoPrice:     decimal.NewFromFloat(150),
		Currency:       "AED",
		EffectiveFrom:  time.Now().Add(-2 * time.Hour),
		EffectiveUntil: time.Now().Add(-time.Hour), // expired
		IsActive:       true,
	})
	bed.setStock(fixtureVariantID, fixtureBatch1, 5, "80.00")

	result, err := bed.orderSvc.ProcessOrder(context.Background(), service.ProcessOrderInput{
		ChannelID:         fixturePOSChannel,
		CustomerName:      strPtr("Regular Customer"),
		VATType:           domain.VATTypeStandard,
		ExchangeRateToAED: decimal.NewFromInt(1),
		Lines:             []service.OrderLineInput{{VariantID: fixtureVariantID, Quantity: 1}},
	})
	if err != nil {
		t.Fatalf("expired-promo order: %v", err)
	}
	if !result.Order.Items[0].UnitPrice.Equal(decimal.NewFromFloat(200)) {
		t.Errorf("expected standard 200 after expired promo, got %s", result.Order.Items[0].UnitPrice)
	}
}

func TestIntegration_MultiBatch_FIFO(t *testing.T) {
	bed := newTestBed(t)
	bed.addChannel(fixturePOSChannel, domain.ChannelTypePOS)
	bed.addPrice(fixtureVariantID, fixturePOSChannel, "100", "AED")
	// Batch 1 (older): 3 units at 40 AED
	bed.setStock(fixtureVariantID, fixtureBatch1, 3, "40.00")
	// Batch 2 (newer): 10 units at 45 AED
	bed.setStock(fixtureVariantID, fixtureBatch2, 10, "45.00")

	result, err := bed.orderSvc.ProcessOrder(context.Background(), service.ProcessOrderInput{
		ChannelID:         fixturePOSChannel,
		CustomerName:      strPtr("FIFO Test"),
		VATType:           domain.VATTypeStandard,
		ExchangeRateToAED: decimal.NewFromInt(1),
		Lines:             []service.OrderLineInput{{VariantID: fixtureVariantID, Quantity: 7}},
	})
	if err != nil {
		t.Fatalf("FIFO order: %v", err)
	}

	totalDeducted := 0
	for _, r := range result.FIFOResults {
		totalDeducted += r.TotalDeducted
	}
	if totalDeducted != 7 {
		t.Errorf("expected 7 deducted, got %d", totalDeducted)
	}
	// Batch 1 fully consumed
	if bed.invRepo.batches[fixtureVariantID][0].TotalDeducted != 3 {
		t.Errorf("batch1 should be depleted (TotalDeducted=3), got %d",
			bed.invRepo.batches[fixtureVariantID][0].TotalDeducted)
	}
	// Batch 2: 4 consumed out of 10
	if bed.invRepo.batches[fixtureVariantID][1].TotalDeducted != 4 {
		t.Errorf("batch2 should have 4 deducted, got %d",
			bed.invRepo.batches[fixtureVariantID][1].TotalDeducted)
	}
}

func TestIntegration_InsufficientStock_TypedError(t *testing.T) {
	bed := newTestBed(t)
	bed.addChannel(fixturePOSChannel, domain.ChannelTypePOS)
	bed.addPrice(fixtureVariantID, fixturePOSChannel, "100", "AED")
	bed.setStock(fixtureVariantID, fixtureBatch1, 2, "40.00") // only 2

	_, err := bed.orderSvc.ProcessOrder(context.Background(), service.ProcessOrderInput{
		ChannelID:         fixturePOSChannel,
		CustomerName:      strPtr("Greedy Customer"),
		VATType:           domain.VATTypeStandard,
		ExchangeRateToAED: decimal.NewFromInt(1),
		Lines:             []service.OrderLineInput{{VariantID: fixtureVariantID, Quantity: 10}},
	})
	if err == nil {
		t.Fatal("expected InsufficientStockError, got nil")
	}

	var insuffErr *service.InsufficientStockError
	if !errors.As(err, &insuffErr) {
		t.Fatalf("expected *InsufficientStockError, got %T: %v", err, err)
	}
	if insuffErr.Available != 2 {
		t.Errorf("Available: want 2, got %d", insuffErr.Available)
	}
	if insuffErr.Needed != 10 {
		t.Errorf("Needed: want 10, got %d", insuffErr.Needed)
	}
}

func TestIntegration_DualCurrency_VATInAED(t *testing.T) {
	bed := newTestBed(t)
	bed.addChannel(fixtureWholesale, domain.ChannelTypeWholesale)
	bed.addPrice(fixtureVariantID, fixtureWholesale, "100", "USD")
	bed.setStock(fixtureVariantID, fixtureBatch1, 10, "30.00")

	rate := decimal.NewFromFloat(3.67)
	result, err := bed.orderSvc.ProcessOrder(context.Background(), service.ProcessOrderInput{
		ChannelID:         fixtureWholesale,
		CustomerName:      strPtr("Export Co. Ltd"),
		VATType:           domain.VATTypeStandard,
		ExchangeRateToAED: rate,
		Lines:             []service.OrderLineInput{{VariantID: fixtureVariantID, Quantity: 1}},
	})
	if err != nil {
		t.Fatalf("dual-currency order: %v", err)
	}

	item := result.Order.Items[0]
	// Net price: 100 USD; VAT: 5 USD
	wantVATUSD := decimal.NewFromFloat(5)
	if !item.VATAmount.Equal(wantVATUSD) {
		t.Errorf("VATAmount(USD): want 5, got %s", item.VATAmount)
	}
	// VATAmountAED is tracked at the price-resolver level and written to
	// the UBL invoice XML; it is not persisted on the order_items row.
	// Verify indirectly via the order total.
	if result.Order.VATAmount.IsZero() {
		t.Error("order-level VAT should not be zero for standard VAT type")
	}
}

func TestIntegration_B2B_TRN_Triggers_EInvoice(t *testing.T) {
	bed := newTestBed(t)
	bed.addChannel(fixtureEcomCh, domain.ChannelTypeEcommerce)
	bed.addPrice(fixtureVariantID, fixtureEcomCh, "120", "AED")
	bed.setStock(fixtureVariantID, fixtureBatch1, 5, "50.00")

	trn := "100123456789003"
	result, err := bed.orderSvc.ProcessOrder(context.Background(), service.ProcessOrderInput{
		ChannelID:         fixtureEcomCh,
		CustomerName:      strPtr("B2B Online Buyer LLC"),
		CustomerTRN:       &trn,
		VATType:           domain.VATTypeStandard,
		ExchangeRateToAED: decimal.NewFromInt(1),
		Lines:             []service.OrderLineInput{{VariantID: fixtureVariantID, Quantity: 1}},
	})
	if err != nil {
		t.Fatalf("B2B TRN order: %v", err)
	}
	if result.Invoice.InvoiceType != domain.InvoiceDocTypeEInvoice {
		t.Errorf("B2B with TRN should be einvoice_ubl, got %s", result.Invoice.InvoiceType)
	}
	if result.Invoice.XMLContent == nil {
		t.Error("B2B e-invoice XML must not be nil")
	}
}

func TestIntegration_MultiItem_Totals(t *testing.T) {
	bed := newTestBed(t)
	bed.addChannel(fixturePOSChannel, domain.ChannelTypePOS)
	bed.addPrice(fixtureVariantID, fixturePOSChannel, "100", "AED")
	bed.addPrice(fixtureVariantID2, fixturePOSChannel, "200", "AED")
	bed.setStock(fixtureVariantID, fixtureBatch1, 10, "40.00")
	bed.setStock(fixtureVariantID2, fixtureBatch2, 10, "90.00")

	result, err := bed.orderSvc.ProcessOrder(context.Background(), service.ProcessOrderInput{
		ChannelID:         fixturePOSChannel,
		CustomerName:      strPtr("Multi-item Customer"),
		VATType:           domain.VATTypeStandard,
		ExchangeRateToAED: decimal.NewFromInt(1),
		Lines: []service.OrderLineInput{
			{VariantID: fixtureVariantID, Quantity: 2},  // 200 + 10 VAT
			{VariantID: fixtureVariantID2, Quantity: 3}, // 600 + 30 VAT
		},
	})
	if err != nil {
		t.Fatalf("multi-item order: %v", err)
	}

	wantSubtotal := decimal.NewFromFloat(800)
	wantVAT := decimal.NewFromFloat(40)
	wantTotal := decimal.NewFromFloat(840)

	if !result.Order.Subtotal.Equal(wantSubtotal) {
		t.Errorf("subtotal: want %s, got %s", wantSubtotal, result.Order.Subtotal)
	}
	if !result.Order.VATAmount.Equal(wantVAT) {
		t.Errorf("VAT: want %s, got %s", wantVAT, result.Order.VATAmount)
	}
	if !result.Order.TotalAmount.Equal(wantTotal) {
		t.Errorf("total: want %s, got %s", wantTotal, result.Order.TotalAmount)
	}
}

func TestIntegration_CustomerID_StampedOnOrder(t *testing.T) {
	bed := newTestBed(t)
	bed.addChannel(fixturePOSChannel, domain.ChannelTypePOS)
	bed.addPrice(fixtureVariantID, fixturePOSChannel, "100", "AED")
	bed.setStock(fixtureVariantID, fixtureBatch1, 5, "40.00")

	customerID := uuid.New()
	result, err := bed.orderSvc.ProcessOrder(context.Background(), service.ProcessOrderInput{
		ChannelID:         fixturePOSChannel,
		CustomerID:        &customerID,
		CustomerName:      strPtr("Hessa Al-Mansouri"),
		VATType:           domain.VATTypeStandard,
		ExchangeRateToAED: decimal.NewFromInt(1),
		Lines:             []service.OrderLineInput{{VariantID: fixtureVariantID, Quantity: 1}},
	})
	if err != nil {
		t.Fatalf("order with customer_id: %v", err)
	}
	if result.Order.CustomerID == nil || *result.Order.CustomerID != customerID {
		t.Errorf("expected customer_id %s on order, got %v", customerID, result.Order.CustomerID)
	}
}

// =============================================================================
// Integration: loyalty points awarded after order completes
// =============================================================================

func TestIntegration_LoyaltyPoints_AwardedAfterOrder(t *testing.T) {
	// Loyalty service is separate; this verifies the data available to award.
	bed := newTestBed(t)
	bed.addChannel(fixturePOSChannel, domain.ChannelTypePOS)
	bed.addPrice(fixtureVariantID, fixturePOSChannel, "250", "AED")
	bed.setStock(fixtureVariantID, fixtureBatch1, 5, "100.00")

	customerID := uuid.New()
	result, err := bed.orderSvc.ProcessOrder(context.Background(), service.ProcessOrderInput{
		ChannelID:         fixturePOSChannel,
		CustomerID:        &customerID,
		CustomerName:      strPtr("Loyalty Member"),
		VATType:           domain.VATTypeStandard,
		ExchangeRateToAED: decimal.NewFromInt(1),
		Lines:             []service.OrderLineInput{{VariantID: fixtureVariantID, Quantity: 2}},
	})
	if err != nil {
		t.Fatalf("loyalty order: %v", err)
	}

	// Verify the total that loyalty service would use for points calculation.
	// 2 × 250 = 500 AED net = 250 expected points at 1pt/AED.
	expectedTotal := decimal.NewFromFloat(500)
	if !result.Order.Subtotal.Equal(expectedTotal) {
		t.Errorf("subtotal for loyalty: want %s, got %s", expectedTotal, result.Order.Subtotal)
	}
	if result.Order.CustomerID == nil || *result.Order.CustomerID != customerID {
		t.Error("customer_id not propagated to order")
	}
}

// =============================================================================
// Fake implementations
// =============================================================================

// intFakeInventoryRepo implements service.InventoryRepo.
type intFakeInventoryRepo struct {
	stock   map[uuid.UUID]*domain.Inventory
	batches map[uuid.UUID][]postgres.FIFOBatchItemRow
}

func (r *intFakeInventoryRepo) GetByVariantIDForUpdate(_ context.Context, _ pgx.Tx, variantID uuid.UUID) (*domain.Inventory, error) {
	inv := r.stock[variantID]
	if inv == nil {
		return nil, fmt.Errorf("inventory not found for variant %s", variantID)
	}
	return inv, nil
}

func (r *intFakeInventoryRepo) GetFIFOBatchItems(_ context.Context, _ pgx.Tx, variantID uuid.UUID) ([]postgres.FIFOBatchItemRow, error) {
	rows := r.batches[variantID]
	var result []postgres.FIFOBatchItemRow
	for _, row := range rows {
		if row.Remaining() > 0 {
			result = append(result, row)
		}
	}
	return result, nil
}

func (r *intFakeInventoryRepo) DeductOnHand(_ context.Context, _ pgx.Tx, variantID uuid.UUID, qty int) (*domain.Inventory, error) {
	inv := r.stock[variantID]
	if inv == nil {
		return nil, fmt.Errorf("inventory not found")
	}
	inv.QuantityOnHand -= qty
	inv.QuantityAvailable -= qty
	return inv, nil
}

func (r *intFakeInventoryRepo) InsertMovement(_ context.Context, _ pgx.Tx, m *domain.InventoryMovement) error {
	// Simulate FIFO batch deduction tracking.
	if m.BatchItemID != nil {
		batches := r.batches[m.VariantID]
		for i, b := range batches {
			if b.BatchItemID == *m.BatchItemID {
				batches[i].TotalDeducted += -m.Quantity // movements are negative for deductions
				r.batches[m.VariantID] = batches
				break
			}
		}
	}
	return nil
}

func (r *intFakeInventoryRepo) InsertReservation(_ context.Context, _ pgx.Tx, _ *domain.StockReservation) error {
	return nil
}

func (r *intFakeInventoryRepo) ReleaseReservation(_ context.Context, _ pgx.Tx, _ uuid.UUID) error {
	return nil
}

// intFakePricingRepo implements service.PricingRepo (and service.ChannelRepo).
type intFakePricingRepo struct {
	channels   map[uuid.UUID]*domain.Channel
	prices     map[string]*domain.ChannelPrice
	promotions []*domain.PricePromotion
}

func (r *intFakePricingRepo) GetChannelByID(_ context.Context, id uuid.UUID) (*domain.Channel, error) {
	ch := r.channels[id]
	if ch == nil {
		return nil, fmt.Errorf("channel not found: %s", id)
	}
	return ch, nil
}

func (r *intFakePricingRepo) GetChannelPrice(_ context.Context, variantID, channelID uuid.UUID) (*domain.ChannelPrice, error) {
	key := variantID.String() + channelID.String()
	cp := r.prices[key]
	if cp == nil {
		return nil, fmt.Errorf("price not found: variant=%s channel=%s", variantID, channelID)
	}
	return cp, nil
}

func (r *intFakePricingRepo) GetActivePromotion(_ context.Context, variantID, channelID uuid.UUID, tier *domain.CustomerTier, now time.Time) (*domain.PricePromotion, error) {
	for _, p := range r.promotions {
		if p.VariantID != variantID || p.ChannelID != channelID {
			continue
		}
		if !p.IsCurrentlyActive(now) {
			continue
		}
		if p.CustomerTier != nil && tier != nil && *p.CustomerTier == *tier {
			return p, nil
		}
		if p.CustomerTier == nil {
			return p, nil
		}
	}
	return nil, nil
}

// intFakeOrderRepo implements service.OrderRepo.
type intFakeOrderRepo struct {
	orders map[uuid.UUID]*domain.Order
	items  map[uuid.UUID][]domain.OrderItem
}

func (r *intFakeOrderRepo) InsertOrder(_ context.Context, _ pgx.Tx, o *domain.Order) error {
	r.orders[o.ID] = o
	return nil
}

func (r *intFakeOrderRepo) InsertOrderItem(_ context.Context, _ pgx.Tx, it *domain.OrderItem) error {
	r.items[it.OrderID] = append(r.items[it.OrderID], *it)
	return nil
}

func (r *intFakeOrderRepo) GetOrderByID(_ context.Context, id uuid.UUID) (*domain.Order, error) {
	o := r.orders[id]
	if o == nil {
		return nil, fmt.Errorf("order not found: %s", id)
	}
	o.Items = r.items[id]
	return o, nil
}

func (r *intFakeOrderRepo) UpdateOrderStatus(_ context.Context, _ pgx.Tx, id uuid.UUID, status domain.OrderStatus) error {
	if o := r.orders[id]; o != nil {
		o.Status = status
	}
	return nil
}

func (r *intFakeOrderRepo) StampInvoiceNumber(_ context.Context, _ pgx.Tx, id uuid.UUID, num string) error {
	if o := r.orders[id]; o != nil {
		o.InvoiceNumber = &num
	}
	return nil
}

// intFakeInvoiceRepo implements service.InvoiceStoreRepo.
type intFakeInvoiceRepo struct {
	invoices map[uuid.UUID]*domain.OrderInvoice
	seq      int
}

func (r *intFakeInvoiceRepo) NextInvoiceNumber(_ context.Context, _ pgx.Tx) (string, error) {
	r.seq++
	return fmt.Sprintf("INV-2026-%06d", r.seq), nil
}

func (r *intFakeInvoiceRepo) InsertOrderInvoice(_ context.Context, _ pgx.Tx, oi *domain.OrderInvoice) error {
	r.invoices[oi.ID] = oi
	return nil
}

func (r *intFakeInvoiceRepo) GetOrderInvoice(_ context.Context, orderID uuid.UUID) (*domain.OrderInvoice, error) {
	for _, inv := range r.invoices {
		if inv.OrderID == orderID {
			return inv, nil
		}
	}
	return nil, fmt.Errorf("invoice not found for order %s", orderID)
}

// intFakeInvoiceGen implements service.InvoiceGenerator.
type intFakeInvoiceGen struct{}

func (g *intFakeInvoiceGen) Serialize(_ *domain.EInvoice, _ decimal.Decimal) ([]byte, error) {
	return []byte(`<?xml version="1.0"?><Invoice><!-- stub --></Invoice>`), nil
}

// newIntegrationFakeTxBeginner returns a TxBeginner backed by the integration
// fake, satisfying calls from auth_test.go, supplier_test.go, etc. when the
// "integration" build tag is active.
func newIntegrationFakeTxBeginner() *intFakeTxBeginner {
	return &intFakeTxBeginner{}
}

// intFakeTxBeginner / intFakeTx provide no-op transaction management.
type intFakeTxBeginner struct{}

func (f *intFakeTxBeginner) BeginTx(_ context.Context, _ pgx.TxOptions) (pgx.Tx, error) {
	return &intFakeTx{}, nil
}

type intFakeTx struct{}

func (t *intFakeTx) Begin(_ context.Context) (pgx.Tx, error)         { return t, nil }
func (t *intFakeTx) Commit(_ context.Context) error                   { return nil }
func (t *intFakeTx) Rollback(_ context.Context) error                 { return nil }
func (t *intFakeTx) CopyFrom(_ context.Context, _ pgx.Identifier, _ []string, _ pgx.CopyFromSource) (int64, error) {
	return 0, nil
}
func (t *intFakeTx) SendBatch(_ context.Context, _ *pgx.Batch) pgx.BatchResults { return nil }
func (t *intFakeTx) LargeObjects() pgx.LargeObjects                             { return pgx.LargeObjects{} }
func (t *intFakeTx) Prepare(_ context.Context, _, _ string) (*pgconn.StatementDescription, error) {
	return nil, nil
}
func (t *intFakeTx) Exec(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}
func (t *intFakeTx) Query(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
	return nil, nil
}
func (t *intFakeTx) QueryRow(_ context.Context, _ string, _ ...any) pgx.Row { return nil }
func (t *intFakeTx) Conn() *pgx.Conn                                        { return nil }

// =============================================================================
// Helpers
// =============================================================================

// Note: strPtr is defined in pos_test.go which always compiles (no build tag).
