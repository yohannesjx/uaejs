package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/dubai-retail/os/internal/domain"
	"github.com/dubai-retail/os/internal/service"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// =============================================================================
// Fake PricingRepo
// =============================================================================

type fakePricingRepo struct {
	promotions map[string]*domain.PricePromotion // key: variantID:channelID
	prices     map[string]*domain.ChannelPrice   // key: variantID:channelID
	channels   map[uuid.UUID]*domain.Channel
}

func newFakePricingRepo() *fakePricingRepo {
	return &fakePricingRepo{
		promotions: make(map[string]*domain.PricePromotion),
		prices:     make(map[string]*domain.ChannelPrice),
		channels:   make(map[uuid.UUID]*domain.Channel),
	}
}

func pricingKey(v, c uuid.UUID) string { return v.String() + ":" + c.String() }

func (f *fakePricingRepo) GetActivePromotion(
	_ context.Context,
	variantID, channelID uuid.UUID,
	tier *domain.CustomerTier,
	now time.Time,
) (*domain.PricePromotion, error) {
	p, ok := f.promotions[pricingKey(variantID, channelID)]
	if !ok {
		return nil, nil
	}
	// Filter by active window
	if !p.IsCurrentlyActive(now) {
		return nil, nil
	}
	// Filter by tier: if promo has a tier, it must match
	if p.CustomerTier != nil && tier != nil && *p.CustomerTier != *tier {
		return nil, nil
	}
	if p.CustomerTier != nil && tier == nil {
		return nil, nil // tier-specific promo doesn't apply to nil (standard) tier
	}
	return p, nil
}

func (f *fakePricingRepo) GetChannelPrice(_ context.Context, variantID, channelID uuid.UUID) (*domain.ChannelPrice, error) {
	cp, ok := f.prices[pricingKey(variantID, channelID)]
	if !ok {
		return nil, errNoRows("channel price not found")
	}
	return cp, nil
}

func (f *fakePricingRepo) GetChannelByID(_ context.Context, id uuid.UUID) (*domain.Channel, error) {
	ch, ok := f.channels[id]
	if !ok {
		return nil, errNoRows("channel not found")
	}
	return ch, nil
}

type noRowsErr string

func (e noRowsErr) Error() string          { return string(e) }
func errNoRows(s string) error             { return noRowsErr(s) }

// =============================================================================
// Tests
// =============================================================================

var vatRate = decimal.NewFromFloat(0.05)

func TestPriceResolver_StandardPrice(t *testing.T) {
	repo := newFakePricingRepo()
	variantID := uuid.New()
	channelID := uuid.New()

	repo.prices[pricingKey(variantID, channelID)] = &domain.ChannelPrice{
		ID:        uuid.New(),
		VariantID: variantID,
		ChannelID: channelID,
		Price:     decimal.NewFromFloat(200.00),
		Currency:  "AED",
		IsActive:  true,
	}

	r := service.NewPriceResolver(repo, vatRate, zap.NewNop())
	res, err := r.Resolve(context.Background(), service.PriceResolveRequest{
		VariantID: variantID,
		ChannelID: channelID,
	})

	assertNoErr(t, err)
	assertDecimalEq(t, "NetPrice", decimal.NewFromFloat(200.00), res.NetPrice)
	assertDecimalEq(t, "VATAmount", decimal.NewFromFloat(10.00), res.VATAmount)
	assertDecimalEq(t, "GrossPrice", decimal.NewFromFloat(210.00), res.GrossPrice)
	assertDecimalEq(t, "VATAmountAED", decimal.NewFromFloat(10.00), res.VATAmountAED)
	if res.PriceSource != domain.PriceSourceStandard {
		t.Errorf("expected PriceSource=standard, got %s", res.PriceSource)
	}
	if res.PromotionID != nil {
		t.Error("expected no PromotionID for standard price")
	}
}

func TestPriceResolver_ActivePromotion_BeatsStandardPrice(t *testing.T) {
	repo := newFakePricingRepo()
	variantID := uuid.New()
	channelID := uuid.New()
	promoID := uuid.New()
	now := time.Now().UTC()

	repo.prices[pricingKey(variantID, channelID)] = &domain.ChannelPrice{
		Price:    decimal.NewFromFloat(300.00),
		Currency: "AED",
		IsActive: true,
	}
	repo.promotions[pricingKey(variantID, channelID)] = &domain.PricePromotion{
		ID:             promoID,
		VariantID:      variantID,
		ChannelID:      channelID,
		PromoPrice:     decimal.NewFromFloat(250.00),
		Currency:       "AED",
		EffectiveFrom:  now.Add(-24 * time.Hour),
		EffectiveUntil: now.Add(24 * time.Hour),
		IsActive:       true,
	}

	r := service.NewPriceResolver(repo, vatRate, zap.NewNop())
	res, err := r.Resolve(context.Background(), service.PriceResolveRequest{
		VariantID: variantID,
		ChannelID: channelID,
	})

	assertNoErr(t, err)
	assertDecimalEq(t, "NetPrice", decimal.NewFromFloat(250.00), res.NetPrice)
	assertDecimalEq(t, "VATAmount", decimal.NewFromFloat(12.50), res.VATAmount)
	assertDecimalEq(t, "GrossPrice", decimal.NewFromFloat(262.50), res.GrossPrice)
	if res.PriceSource != domain.PriceSourcePromotion {
		t.Errorf("expected PriceSource=promotion, got %s", res.PriceSource)
	}
	if res.PromotionID == nil || *res.PromotionID != promoID {
		t.Error("expected PromotionID to be set")
	}
}

func TestPriceResolver_ExpiredPromotion_FallsBackToStandard(t *testing.T) {
	repo := newFakePricingRepo()
	variantID := uuid.New()
	channelID := uuid.New()
	now := time.Now().UTC()

	repo.prices[pricingKey(variantID, channelID)] = &domain.ChannelPrice{
		Price:    decimal.NewFromFloat(300.00),
		Currency: "AED",
		IsActive: true,
	}
	// Promotion window already expired
	repo.promotions[pricingKey(variantID, channelID)] = &domain.PricePromotion{
		ID:             uuid.New(),
		PromoPrice:     decimal.NewFromFloat(200.00),
		Currency:       "AED",
		EffectiveFrom:  now.Add(-48 * time.Hour),
		EffectiveUntil: now.Add(-1 * time.Hour), // expired
		IsActive:       true,
	}

	r := service.NewPriceResolver(repo, vatRate, zap.NewNop())
	res, err := r.Resolve(context.Background(), service.PriceResolveRequest{
		VariantID: variantID,
		ChannelID: channelID,
	})

	assertNoErr(t, err)
	assertDecimalEq(t, "NetPrice", decimal.NewFromFloat(300.00), res.NetPrice)
	if res.PriceSource != domain.PriceSourceStandard {
		t.Errorf("expired promo should fall back to standard, got %s", res.PriceSource)
	}
}

func TestPriceResolver_TierSpecificPromotion_VIPGetsDiscount(t *testing.T) {
	repo := newFakePricingRepo()
	variantID := uuid.New()
	channelID := uuid.New()
	now := time.Now().UTC()
	vipTier := domain.CustomerTierVIP

	repo.prices[pricingKey(variantID, channelID)] = &domain.ChannelPrice{
		Price:    decimal.NewFromFloat(500.00),
		Currency: "AED",
		IsActive: true,
	}
	repo.promotions[pricingKey(variantID, channelID)] = &domain.PricePromotion{
		ID:             uuid.New(),
		CustomerTier:   &vipTier,
		PromoPrice:     decimal.NewFromFloat(400.00),
		Currency:       "AED",
		EffectiveFrom:  now.Add(-1 * time.Hour),
		EffectiveUntil: now.Add(24 * time.Hour),
		IsActive:       true,
	}

	r := service.NewPriceResolver(repo, vatRate, zap.NewNop())

	// VIP customer gets the discounted price
	vipRes, err := r.Resolve(context.Background(), service.PriceResolveRequest{
		VariantID:    variantID,
		ChannelID:    channelID,
		CustomerTier: &vipTier,
	})
	assertNoErr(t, err)
	assertDecimalEq(t, "VIP NetPrice", decimal.NewFromFloat(400.00), vipRes.NetPrice)
	if vipRes.PriceSource != domain.PriceSourcePromotion {
		t.Errorf("VIP should get promotion price, got %s", vipRes.PriceSource)
	}

	// Standard customer pays full price (no matching tier)
	stdTier := domain.CustomerTierStandard
	stdRes, err := r.Resolve(context.Background(), service.PriceResolveRequest{
		VariantID:    variantID,
		ChannelID:    channelID,
		CustomerTier: &stdTier,
	})
	assertNoErr(t, err)
	assertDecimalEq(t, "Standard NetPrice", decimal.NewFromFloat(500.00), stdRes.NetPrice)
	if stdRes.PriceSource != domain.PriceSourceStandard {
		t.Errorf("standard customer should get standard price, got %s", stdRes.PriceSource)
	}
}

func TestPriceResolver_MultiChannel_IndependentPrices(t *testing.T) {
	repo := newFakePricingRepo()
	variantID := uuid.New()
	posChannelID := uuid.New()
	webChannelID := uuid.New()
	wsChannelID := uuid.New()

	// POS: 199 AED, Web: 219 AED, Wholesale: 110 AED
	repo.prices[pricingKey(variantID, posChannelID)] = &domain.ChannelPrice{
		Price: decimal.NewFromFloat(199.00), Currency: "AED", IsActive: true,
	}
	repo.prices[pricingKey(variantID, webChannelID)] = &domain.ChannelPrice{
		Price: decimal.NewFromFloat(219.00), Currency: "AED", IsActive: true,
	}
	repo.prices[pricingKey(variantID, wsChannelID)] = &domain.ChannelPrice{
		Price: decimal.NewFromFloat(110.00), Currency: "AED", IsActive: true,
	}

	r := service.NewPriceResolver(repo, vatRate, zap.NewNop())
	ctx := context.Background()

	posRes, _ := r.Resolve(ctx, service.PriceResolveRequest{VariantID: variantID, ChannelID: posChannelID})
	webRes, _ := r.Resolve(ctx, service.PriceResolveRequest{VariantID: variantID, ChannelID: webChannelID})
	wsRes, _ := r.Resolve(ctx, service.PriceResolveRequest{VariantID: variantID, ChannelID: wsChannelID})

	assertDecimalEq(t, "POS price", decimal.NewFromFloat(199.00), posRes.NetPrice)
	assertDecimalEq(t, "Web price", decimal.NewFromFloat(219.00), webRes.NetPrice)
	assertDecimalEq(t, "Wholesale price", decimal.NewFromFloat(110.00), wsRes.NetPrice)

	// VAT on wholesale: 110 * 0.05 = 5.50
	assertDecimalEq(t, "Wholesale VAT", decimal.NewFromFloat(5.50), wsRes.VATAmount)
	assertDecimalEq(t, "Wholesale Gross", decimal.NewFromFloat(115.50), wsRes.GrossPrice)
}

func TestPriceResolver_DualCurrency_VATAlwaysInAED(t *testing.T) {
	repo := newFakePricingRepo()
	variantID := uuid.New()
	channelID := uuid.New()

	// Price in USD
	repo.prices[pricingKey(variantID, channelID)] = &domain.ChannelPrice{
		Price:    decimal.NewFromFloat(100.00),
		Currency: "USD",
		IsActive: true,
	}

	r := service.NewPriceResolver(repo, vatRate, zap.NewNop())
	res, err := r.Resolve(context.Background(), service.PriceResolveRequest{
		VariantID:         variantID,
		ChannelID:         channelID,
		ExchangeRateToAED: decimal.NewFromFloat(3.67), // 1 USD = 3.67 AED
	})

	assertNoErr(t, err)
	// VAT in USD: 100 * 0.05 = 5.00
	assertDecimalEq(t, "VATAmount USD", decimal.NewFromFloat(5.00), res.VATAmount)
	// VAT in AED: 5.00 * 3.67 = 18.35
	assertDecimalEq(t, "VATAmountAED", decimal.NewFromFloat(18.35), res.VATAmountAED)
}

func TestPriceResolver_ZeroRated_NoVAT(t *testing.T) {
	repo := newFakePricingRepo()
	variantID := uuid.New()
	channelID := uuid.New()

	repo.prices[pricingKey(variantID, channelID)] = &domain.ChannelPrice{
		Price:    decimal.NewFromFloat(300.00),
		Currency: "AED",
		IsActive: true,
	}

	r := service.NewPriceResolver(repo, vatRate, zap.NewNop())
	res, err := r.ResolveZeroRated(context.Background(), service.PriceResolveRequest{
		VariantID: variantID,
		ChannelID: channelID,
	})

	assertNoErr(t, err)
	assertDecimalEq(t, "NetPrice", decimal.NewFromFloat(300.00), res.NetPrice)
	assertDecimalEq(t, "VATAmount", decimal.Zero, res.VATAmount)
	assertDecimalEq(t, "GrossPrice", decimal.NewFromFloat(300.00), res.GrossPrice)
	assertDecimalEq(t, "VATAmountAED", decimal.Zero, res.VATAmountAED)
}

func TestPriceResolver_NoPriceConfigured_ReturnsError(t *testing.T) {
	repo := newFakePricingRepo()
	r := service.NewPriceResolver(repo, vatRate, zap.NewNop())

	_, err := r.Resolve(context.Background(), service.PriceResolveRequest{
		VariantID: uuid.New(),
		ChannelID: uuid.New(),
	})
	if err == nil {
		t.Error("expected error for missing price, got nil")
	}
}

// =============================================================================
// Shared test helpers
// =============================================================================

func assertNoErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func assertDecimalEq(t *testing.T, label string, want, got decimal.Decimal) {
	t.Helper()
	if !want.Equal(got) {
		t.Errorf("%s: want %s, got %s", label, want, got)
	}
}
