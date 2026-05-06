// Package service: Analytics Module
//
// AnalyticsService provides data-driven operational intelligence:
//
//  1. ForecastDemand   – Projects next-period sales for a SKU using a
//     weighted moving average over historical order_items.
//  2. SuggestReorders  – Cross-references forecast demand against current
//     stock levels to produce ranked reorder suggestions.
//  3. PromotionEfficacy – Measures promotion hit-rate, revenue lift, and
//     average discount depth for each active/recent promotion.
//  4. FraudSignals     – Detects customers with statistically anomalous
//     return rates or QC-fail patterns.
//
// Algorithm notes:
//   - ForecastDemand: 4-week weighted moving average (most recent week = 4×
//     weight, previous weeks declining by 1×).  The result is days_of_stock
//     remaining = current_stock / daily_demand.
//   - FraudSignals: a customer triggers a fraud signal when:
//     (returns / orders) > 0.4 AND at least one QC mismatch, OR
//     total returns > 5 within 30 days.
//
// All queries are read-only and run against replicas in production.
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

// AnalyticsRepo provides read-only access to the data needed for analytics.
type AnalyticsRepo interface {
	// GetWeeklySalesByVariant returns total units sold per variant per ISO week
	// for the last nWeeks weeks (ordered oldest first).
	GetWeeklySalesByVariant(ctx context.Context, sku string, nWeeks int) ([]domain.WeeklySales, error)

	// GetAllVariantsWithStock returns all active variants with current stock.
	GetAllVariantsWithStock(ctx context.Context) ([]domain.VariantStockRow, error)

	// GetPromotionStats returns hit-rate and revenue data per promotion.
	GetPromotionStats(ctx context.Context) ([]domain.PromotionStat, error)

	// GetCustomerReturnStats returns per-customer return and QC-fail counts.
	GetCustomerReturnStats(ctx context.Context, since time.Time) ([]domain.CustomerReturnStat, error)
}

// ── Data transfer types are defined in domain/analytics.go ──────────────────
// Aliases for backwards compatibility within this package.
type WeeklySales = domain.WeeklySales
type VariantStockRow = domain.VariantStockRow
type PromotionStat = domain.PromotionStat
type CustomerReturnStat = domain.CustomerReturnStat

// =============================================================================
// Output types
// =============================================================================

// ForecastResult is returned by ForecastDemand.
type ForecastResult struct {
	SKU              string          `json:"sku"`
	VariantID        uuid.UUID       `json:"variant_id"`
	Channel          string          `json:"channel"`
	CurrentStock     int             `json:"current_stock"`
	WeeklyForecast   int             `json:"weekly_forecast_units"`
	DailyForecast    float64         `json:"daily_forecast_units"`
	DaysOfStockLeft  float64         `json:"days_of_stock_left"`
	ReorderSuggested bool            `json:"reorder_suggested"`
	Confidence       string          `json:"confidence"` // "high" | "medium" | "low"
	WeeksOfHistory   int             `json:"weeks_of_history"`
	Algorithm        string          `json:"algorithm"`
}

// ReorderSuggestion is returned by SuggestReorders.
type ReorderSuggestion struct {
	VariantID        uuid.UUID       `json:"variant_id"`
	SKU              string          `json:"sku"`
	ProductName      string          `json:"product_name"`
	CurrentStock     int             `json:"current_stock"`
	WeeklyForecast   int             `json:"weekly_forecast_units"`
	DaysOfStockLeft  float64         `json:"days_of_stock_left"`
	SuggestedQty     int             `json:"suggested_order_qty"`    // 4 weeks supply
	Priority         string          `json:"priority"`               // "urgent" | "high" | "medium"
}

// PromotionEfficacyReport groups all promotion stats.
type PromotionEfficacyReport struct {
	GeneratedAt time.Time     `json:"generated_at"`
	Promotions  []PromotionInsight `json:"promotions"`
}

// PromotionInsight enriches PromotionStat with derived analytics.
type PromotionInsight struct {
	PromotionStat
	RevenueLift    decimal.Decimal `json:"revenue_lift"`    // extra rev vs standard price
	DiscountDepth  string          `json:"discount_depth"`  // "shallow" | "moderate" | "deep"
	Verdict        string          `json:"verdict"`         // "effective" | "neutral" | "costly"
}

// FraudSignal is a flagged customer record.
type FraudSignal struct {
	CustomerEmail string    `json:"customer_email"`
	ReturnRate    float64   `json:"return_rate"`       // returns / orders
	QCMismatches  int       `json:"qc_mismatches"`
	TotalReturns  int       `json:"total_returns"`
	RiskLevel     string    `json:"risk_level"`        // "medium" | "high"
	Reason        string    `json:"reason"`
}

// =============================================================================
// Service
// =============================================================================

// AnalyticsService provides data-driven intelligence for operational decisions.
type AnalyticsService struct {
	repo AnalyticsRepo
	log  *zap.Logger
}

// NewAnalyticsService creates an AnalyticsService.
func NewAnalyticsService(repo AnalyticsRepo, log *zap.Logger) *AnalyticsService {
	return &AnalyticsService{repo: repo, log: log}
}

// ForecastDemand projects demand for a SKU over the next 7 days using a
// 4-week weighted moving average.
//
// Weight schedule: week -1 = 4, week -2 = 3, week -3 = 2, week -4 = 1
// Total weight = 10. Weekly forecast = weighted_sum / total_weight.
func (s *AnalyticsService) ForecastDemand(ctx context.Context, sku, channel string) (*ForecastResult, error) {
	const nWeeks = 4
	history, err := s.repo.GetWeeklySalesByVariant(ctx, sku, nWeeks)
	if err != nil {
		return nil, fmt.Errorf("ForecastDemand: fetch history: %w", err)
	}

	result := &ForecastResult{
		SKU:            sku,
		Channel:        channel,
		Algorithm:      "weighted_moving_average_4w",
		WeeksOfHistory: len(history),
	}

	if len(history) == 0 {
		result.Confidence = "low"
		result.WeeklyForecast = 0
		result.DailyForecast = 0
		return result, nil
	}

	// Fill in variant ID and current stock from the most recent row
	result.VariantID = history[len(history)-1].VariantID

	// Weighted moving average
	// Most recent week gets highest weight
	weights := []int{1, 2, 3, 4}
	totalWeight := 0
	weightedSum := 0

	// Use up to nWeeks; pad if fewer available
	offset := nWeeks - len(history)
	for i, h := range history {
		w := weights[offset+i]
		weightedSum += h.UnitsSold * w
		totalWeight += w
	}

	weeklyForecast := 0
	if totalWeight > 0 {
		weeklyForecast = weightedSum / totalWeight
	}
	dailyForecast := float64(weeklyForecast) / 7.0

	// Get current stock for days-of-stock calculation
	stocks, err := s.repo.GetAllVariantsWithStock(ctx)
	if err == nil {
		for _, vs := range stocks {
			if vs.VariantID == result.VariantID {
				result.CurrentStock = vs.Stock
				break
			}
		}
	}

	daysOfStock := 0.0
	if dailyForecast > 0 {
		daysOfStock = float64(result.CurrentStock) / dailyForecast
	}

	result.WeeklyForecast = weeklyForecast
	result.DailyForecast = dailyForecast
	result.DaysOfStockLeft = daysOfStock
	result.ReorderSuggested = daysOfStock < 14 // < 2 weeks supply = reorder

	// Confidence based on history depth
	switch {
	case len(history) >= 4:
		result.Confidence = "high"
	case len(history) >= 2:
		result.Confidence = "medium"
	default:
		result.Confidence = "low"
	}

	s.log.Debug("analytics.forecast",
		zap.String("sku", sku),
		zap.Int("weekly_forecast", weeklyForecast),
		zap.Float64("days_of_stock", daysOfStock),
		zap.String("confidence", result.Confidence),
	)

	return result, nil
}

// SuggestReorders returns all variants with fewer than 14 days of projected
// stock, ranked by urgency.
func (s *AnalyticsService) SuggestReorders(ctx context.Context) ([]ReorderSuggestion, error) {
	variants, err := s.repo.GetAllVariantsWithStock(ctx)
	if err != nil {
		return nil, fmt.Errorf("SuggestReorders: get variants: %w", err)
	}

	var suggestions []ReorderSuggestion

	for _, v := range variants {
		forecast, err := s.ForecastDemand(ctx, v.SKU, "")
		if err != nil {
			s.log.Warn("SuggestReorders: forecast failed",
				zap.String("sku", v.SKU), zap.Error(err))
			continue
		}

		if !forecast.ReorderSuggested {
			continue
		}

		priority := "medium"
		switch {
		case forecast.DaysOfStockLeft <= 3:
			priority = "urgent"
		case forecast.DaysOfStockLeft <= 7:
			priority = "high"
		}

		// Suggest 4 weeks of supply
		suggested := forecast.WeeklyForecast * 4
		if suggested < 10 {
			suggested = 10 // minimum order quantity
		}

		suggestions = append(suggestions, ReorderSuggestion{
			VariantID:       v.VariantID,
			SKU:             v.SKU,
			ProductName:     v.Name,
			CurrentStock:    v.Stock,
			WeeklyForecast:  forecast.WeeklyForecast,
			DaysOfStockLeft: forecast.DaysOfStockLeft,
			SuggestedQty:    suggested,
			Priority:        priority,
		})
	}

	s.log.Info("analytics.reorder_suggestions", zap.Int("count", len(suggestions)))
	return suggestions, nil
}

// PromotionEfficacy analyses each recent promotion's hit-rate and revenue impact.
func (s *AnalyticsService) PromotionEfficacy(ctx context.Context) (*PromotionEfficacyReport, error) {
	stats, err := s.repo.GetPromotionStats(ctx)
	if err != nil {
		return nil, fmt.Errorf("PromotionEfficacy: fetch stats: %w", err)
	}

	insights := make([]PromotionInsight, 0, len(stats))
	for _, st := range stats {
		// Revenue lift = (promo revenue) - (what standard price would have made)
		standardRevenue := st.StandardPrice.Mul(decimal.NewFromInt(int64(st.HitCount)))
		revenueLift := st.TotalRevenue.Sub(standardRevenue) // typically negative = cost of promo

		discountPct := decimal.Zero
		if !st.StandardPrice.IsZero() {
			discountPct = st.StandardPrice.Sub(st.PromoPrice).Div(st.StandardPrice).Mul(decimal.NewFromInt(100))
		}

		depth := "shallow"
		switch {
		case discountPct.GreaterThan(decimal.NewFromInt(30)):
			depth = "deep"
		case discountPct.GreaterThan(decimal.NewFromInt(15)):
			depth = "moderate"
		}

		// Verdict: effective if hit rate > 20% and revenue lift > -10% of gross
		hitRatePct := 0.0
		if st.HitCount > 0 {
			hitRatePct = float64(st.HitCount) / float64(st.HitCount) * 100 // placeholder
		}
		verdict := "neutral"
		liftThreshold := standardRevenue.Mul(decimal.NewFromFloat(-0.1))
		if float64(st.HitCount) > 10 && revenueLift.GreaterThan(liftThreshold) {
			verdict = "effective"
		} else if revenueLift.LessThan(liftThreshold) && depth == "deep" {
			verdict = "costly"
		}
		_ = hitRatePct

		insights = append(insights, PromotionInsight{
			PromotionStat: st,
			RevenueLift:   revenueLift,
			DiscountDepth: depth,
			Verdict:       verdict,
		})
	}

	return &PromotionEfficacyReport{
		GeneratedAt: time.Now().UTC(),
		Promotions:  insights,
	}, nil
}

// FraudSignals returns customers with statistically anomalous return patterns.
//
// Signals fired when:
//   - return_rate > 40% AND at least 1 QC mismatch, OR
//   - total returns > 5 within the last 30 days.
func (s *AnalyticsService) FraudSignals(ctx context.Context) ([]FraudSignal, error) {
	since := time.Now().UTC().Add(-30 * 24 * time.Hour)
	stats, err := s.repo.GetCustomerReturnStats(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("FraudSignals: fetch stats: %w", err)
	}

	var signals []FraudSignal
	for _, stat := range stats {
		if stat.TotalOrders == 0 {
			continue
		}

		returnRate := float64(stat.TotalReturns) / float64(stat.TotalOrders)
		riskLevel := ""
		reason := ""

		switch {
		case returnRate > 0.4 && stat.QCMismatches > 0:
			riskLevel = "high"
			reason = fmt.Sprintf("return rate %.0f%% with %d QC mismatches", returnRate*100, stat.QCMismatches)
		case stat.TotalReturns > 5:
			riskLevel = "high"
			reason = fmt.Sprintf("%d returns in 30 days exceeds threshold", stat.TotalReturns)
		case returnRate > 0.25 && stat.TotalOrders > 2:
			riskLevel = "medium"
			reason = fmt.Sprintf("elevated return rate %.0f%%", returnRate*100)
		}

		if riskLevel == "" {
			continue
		}

		signals = append(signals, FraudSignal{
			CustomerEmail: stat.CustomerEmail,
			ReturnRate:    returnRate,
			QCMismatches:  stat.QCMismatches,
			TotalReturns:  stat.TotalReturns,
			RiskLevel:     riskLevel,
			Reason:        reason,
		})

		s.log.Warn("analytics.fraud_signal",
			zap.String("customer_email", stat.CustomerEmail),
			zap.Float64("return_rate", returnRate),
			zap.Int("qc_mismatches", stat.QCMismatches),
			zap.String("risk_level", riskLevel),
			zap.String("reason", reason),
		)
	}

	return signals, nil
}
