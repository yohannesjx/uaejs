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
// Fake analytics repo with synthetic data
// =============================================================================

type fakeAnalyticsRepo struct {
	salesHistory    []service.WeeklySales
	variantStocks   []service.VariantStockRow
	promotionStats  []service.PromotionStat
	returnStats     []service.CustomerReturnStat
}

func (r *fakeAnalyticsRepo) GetWeeklySalesByVariant(_ context.Context, sku string, nWeeks int) ([]service.WeeklySales, error) {
	var result []service.WeeklySales
	for _, s := range r.salesHistory {
		if s.SKU == sku {
			result = append(result, s)
		}
	}
	if len(result) > nWeeks {
		result = result[len(result)-nWeeks:]
	}
	return result, nil
}

func (r *fakeAnalyticsRepo) GetAllVariantsWithStock(_ context.Context) ([]service.VariantStockRow, error) {
	return r.variantStocks, nil
}

func (r *fakeAnalyticsRepo) GetPromotionStats(_ context.Context) ([]service.PromotionStat, error) {
	return r.promotionStats, nil
}

func (r *fakeAnalyticsRepo) GetCustomerReturnStats(_ context.Context, since time.Time) ([]service.CustomerReturnStat, error) {
	return r.returnStats, nil
}

// =============================================================================
// Tests
// =============================================================================

// TestAnalytics_ForecastDemand_WMA verifies the 4-week weighted moving average.
//
// Synthetic data: sales over 4 weeks = [10, 20, 30, 40]
// Weights = [1, 2, 3, 4], total weight = 10
// Expected weekly forecast = (10×1 + 20×2 + 30×3 + 40×4) / 10 = 300 / 10 = 30
func TestAnalytics_ForecastDemand_WMA(t *testing.T) {
	variantID := uuid.New()
	repo := &fakeAnalyticsRepo{
		salesHistory: []service.WeeklySales{
			{VariantID: variantID, SKU: "DRESS-001", ISOWeek: 1, Year: 2026, UnitsSold: 10},
			{VariantID: variantID, SKU: "DRESS-001", ISOWeek: 2, Year: 2026, UnitsSold: 20},
			{VariantID: variantID, SKU: "DRESS-001", ISOWeek: 3, Year: 2026, UnitsSold: 30},
			{VariantID: variantID, SKU: "DRESS-001", ISOWeek: 4, Year: 2026, UnitsSold: 40},
		},
		variantStocks: []service.VariantStockRow{
			{VariantID: variantID, SKU: "DRESS-001", Stock: 200},
		},
	}
	svc := service.NewAnalyticsService(repo, zap.NewNop())

	result, err := svc.ForecastDemand(context.Background(), "DRESS-001", "pos")
	if err != nil {
		t.Fatalf("ForecastDemand failed: %v", err)
	}

	if result.WeeklyForecast != 30 {
		t.Errorf("WeeklyForecast: want 30 got %d", result.WeeklyForecast)
	}
	if result.Confidence != "high" {
		t.Errorf("Confidence: want 'high' got %s", result.Confidence)
	}

	// DaysOfStockLeft = 200 / (30/7) ≈ 46.67 days → no reorder needed
	if result.ReorderSuggested {
		t.Error("reorder should NOT be suggested with 46 days of stock")
	}
}

// TestAnalytics_ForecastDemand_LowStock verifies reorder trigger when days < 14.
func TestAnalytics_ForecastDemand_LowStock(t *testing.T) {
	variantID := uuid.New()
	repo := &fakeAnalyticsRepo{
		salesHistory: []service.WeeklySales{
			{VariantID: variantID, SKU: "HANDBAG-002", ISOWeek: 1, Year: 2026, UnitsSold: 70},
			{VariantID: variantID, SKU: "HANDBAG-002", ISOWeek: 2, Year: 2026, UnitsSold: 70},
			{VariantID: variantID, SKU: "HANDBAG-002", ISOWeek: 3, Year: 2026, UnitsSold: 70},
			{VariantID: variantID, SKU: "HANDBAG-002", ISOWeek: 4, Year: 2026, UnitsSold: 70},
		},
		variantStocks: []service.VariantStockRow{
			{VariantID: variantID, SKU: "HANDBAG-002", Stock: 50}, // only 5 days at 70/week
		},
	}
	svc := service.NewAnalyticsService(repo, zap.NewNop())

	result, err := svc.ForecastDemand(context.Background(), "HANDBAG-002", "wholesale")
	if err != nil {
		t.Fatalf("ForecastDemand failed: %v", err)
	}

	if !result.ReorderSuggested {
		t.Error("reorder SHOULD be suggested (< 14 days of stock)")
	}
	if result.DaysOfStockLeft >= 14 {
		t.Errorf("DaysOfStockLeft should be < 14, got %.1f", result.DaysOfStockLeft)
	}
}

// TestAnalytics_ForecastDemand_NoHistory verifies graceful handling of a SKU
// with no sales history (new product).
func TestAnalytics_ForecastDemand_NoHistory(t *testing.T) {
	repo := &fakeAnalyticsRepo{}
	svc := service.NewAnalyticsService(repo, zap.NewNop())

	result, err := svc.ForecastDemand(context.Background(), "NEW-SKU-999", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.WeeklyForecast != 0 {
		t.Errorf("expected 0 forecast for no history, got %d", result.WeeklyForecast)
	}
	if result.Confidence != "low" {
		t.Errorf("expected 'low' confidence, got %s", result.Confidence)
	}
}

// TestAnalytics_ForecastDemand_PartialHistory verifies medium confidence
// when only 2 weeks of data are available.
func TestAnalytics_ForecastDemand_PartialHistory(t *testing.T) {
	vid := uuid.New()
	repo := &fakeAnalyticsRepo{
		salesHistory: []service.WeeklySales{
			{VariantID: vid, SKU: "SKU-NEW", ISOWeek: 5, Year: 2026, UnitsSold: 25},
			{VariantID: vid, SKU: "SKU-NEW", ISOWeek: 6, Year: 2026, UnitsSold: 35},
		},
	}
	svc := service.NewAnalyticsService(repo, zap.NewNop())
	result, err := svc.ForecastDemand(context.Background(), "SKU-NEW", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Confidence != "medium" {
		t.Errorf("expected 'medium' confidence for 2-week history, got %s", result.Confidence)
	}
	// Weights for 2 weeks (offset=2): [3,4] → (25×3 + 35×4) / 7 = 215/7 = 30
	if result.WeeklyForecast != 30 {
		t.Errorf("WeeklyForecast: want 30 got %d", result.WeeklyForecast)
	}
}

// TestAnalytics_FraudSignals_HighRisk verifies that a customer with >40%
// return rate AND QC mismatches is flagged as high risk.
func TestAnalytics_FraudSignals_HighRisk(t *testing.T) {
	repo := &fakeAnalyticsRepo{
		returnStats: []service.CustomerReturnStat{
			{
				CustomerEmail: "fraud@example.com",
				TotalOrders:   10,
				TotalReturns:  5,   // 50% return rate
				QCMismatches:  2,
				LastReturnDate: time.Now(),
			},
			{
				CustomerEmail: "normal@example.com",
				TotalOrders:   20,
				TotalReturns:  1, // 5% return rate
				QCMismatches:  0,
			},
		},
	}
	svc := service.NewAnalyticsService(repo, zap.NewNop())

	signals, err := svc.FraudSignals(context.Background())
	if err != nil {
		t.Fatalf("FraudSignals failed: %v", err)
	}

	if len(signals) != 1 {
		t.Fatalf("expected 1 fraud signal, got %d", len(signals))
	}
	if signals[0].CustomerEmail != "fraud@example.com" {
		t.Errorf("wrong customer flagged: %s", signals[0].CustomerEmail)
	}
	if signals[0].RiskLevel != "high" {
		t.Errorf("expected high risk, got %s", signals[0].RiskLevel)
	}
}

// TestAnalytics_FraudSignals_VolumeThreshold verifies that > 5 returns in
// 30 days triggers a high-risk signal regardless of rate.
func TestAnalytics_FraudSignals_VolumeThreshold(t *testing.T) {
	repo := &fakeAnalyticsRepo{
		returnStats: []service.CustomerReturnStat{
			{
				CustomerEmail: "bulk-returner@example.com",
				TotalOrders:   100,
				TotalReturns:  6, // only 6% rate but volume > 5
				QCMismatches:  0,
				LastReturnDate: time.Now(),
			},
		},
	}
	svc := service.NewAnalyticsService(repo, zap.NewNop())

	signals, err := svc.FraudSignals(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(signals) != 1 {
		t.Fatalf("expected 1 signal for volume threshold, got %d", len(signals))
	}
	if signals[0].RiskLevel != "high" {
		t.Errorf("expected high risk, got %s", signals[0].RiskLevel)
	}
}

// TestAnalytics_PromotionEfficacy_Verdict verifies that deep-discount promotions
// with negative revenue lift are classified as "costly".
func TestAnalytics_PromotionEfficacy_Verdict(t *testing.T) {
	vid := uuid.New()
	pid := uuid.New()
	repo := &fakeAnalyticsRepo{
		promotionStats: []service.PromotionStat{
			{
				PromotionID:   pid,
				VariantID:     vid,
				SKU:           "DRESS-RED-S",
				Channel:       domain.ChannelTypePOS,
				PromoPrice:    decimal.NewFromFloat(50),
				StandardPrice: decimal.NewFromFloat(200), // 75% discount = "deep"
				HitCount:      100,
				TotalRevenue:  decimal.NewFromFloat(5000), // 100 × 50
			},
		},
	}
	svc := service.NewAnalyticsService(repo, zap.NewNop())

	report, err := svc.PromotionEfficacy(context.Background())
	if err != nil {
		t.Fatalf("PromotionEfficacy failed: %v", err)
	}
	if len(report.Promotions) != 1 {
		t.Fatalf("expected 1 promotion insight, got %d", len(report.Promotions))
	}

	insight := report.Promotions[0]
	if insight.DiscountDepth != "deep" {
		t.Errorf("expected 'deep' discount, got %s", insight.DiscountDepth)
	}
	// Revenue lift: 5000 - (200×100 = 20000) = -15000 → costly
	if insight.Verdict != "costly" {
		t.Errorf("expected 'costly' verdict for deep-discount promo, got %s", insight.Verdict)
	}
}

// TestAnalytics_SuggestReorders_Priority verifies that urgency ordering is correct.
func TestAnalytics_SuggestReorders_Priority(t *testing.T) {
	v1 := uuid.New()
	v2 := uuid.New()
	repo := &fakeAnalyticsRepo{
		variantStocks: []service.VariantStockRow{
			{VariantID: v1, SKU: "SKU-URGENT", Stock: 5},   // very low stock
			{VariantID: v2, SKU: "SKU-MEDIUM", Stock: 40},  // moderate stock
		},
		salesHistory: []service.WeeklySales{
			{VariantID: v1, SKU: "SKU-URGENT", ISOWeek: 1, Year: 2026, UnitsSold: 30},
			{VariantID: v1, SKU: "SKU-URGENT", ISOWeek: 2, Year: 2026, UnitsSold: 30},
			{VariantID: v1, SKU: "SKU-URGENT", ISOWeek: 3, Year: 2026, UnitsSold: 30},
			{VariantID: v1, SKU: "SKU-URGENT", ISOWeek: 4, Year: 2026, UnitsSold: 30},
			{VariantID: v2, SKU: "SKU-MEDIUM", UnitsSold: 0},
			{VariantID: v2, SKU: "SKU-MEDIUM", ISOWeek: 1, Year: 2026, UnitsSold: 20},
			{VariantID: v2, SKU: "SKU-MEDIUM", ISOWeek: 2, Year: 2026, UnitsSold: 20},
			{VariantID: v2, SKU: "SKU-MEDIUM", ISOWeek: 3, Year: 2026, UnitsSold: 20},
			{VariantID: v2, SKU: "SKU-MEDIUM", ISOWeek: 4, Year: 2026, UnitsSold: 20},
		},
	}
	svc := service.NewAnalyticsService(repo, zap.NewNop())

	suggestions, err := svc.SuggestReorders(context.Background())
	if err != nil {
		t.Fatalf("SuggestReorders failed: %v", err)
	}

	// SKU-URGENT should be urgent, SKU-MEDIUM should be medium or high
	urgent := false
	for _, s := range suggestions {
		if s.SKU == "SKU-URGENT" && s.Priority == "urgent" {
			urgent = true
		}
	}
	if !urgent {
		t.Errorf("SKU-URGENT should have priority=urgent; suggestions: %+v", suggestions)
	}
}
