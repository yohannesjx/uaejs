package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/dubai-retail/os/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
)

// AnalyticsRepository provides read-only analytics queries.
type AnalyticsRepository struct {
	pool *pgxpool.Pool
}

// GetWeeklySalesByVariant returns the last nWeeks weeks of sales grouped by ISO week.
func (r *AnalyticsRepository) GetWeeklySalesByVariant(
	ctx context.Context,
	sku string,
	nWeeks int,
) ([]domain.WeeklySales, error) {
	const q = `
		SELECT
		    v.id                              AS variant_id,
		    v.sku,
		    EXTRACT(ISOYEAR FROM o.created_at)::INT AS year,
		    EXTRACT(WEEK   FROM o.created_at)::INT AS iso_week,
		    COALESCE(SUM(oi.quantity), 0)    AS units_sold,
		    COALESCE(SUM(oi.line_total), 0)  AS gross_revenue
		  FROM order_items oi
		  JOIN variants v  ON v.id = oi.variant_id
		  JOIN orders   o  ON o.id = oi.order_id
		 WHERE v.sku = $1
		   AND o.created_at >= NOW() - ($2 || ' weeks')::INTERVAL
		   AND o.status NOT IN ('cancelled', 'pending')
		 GROUP BY v.id, v.sku, year, iso_week
		 ORDER BY year ASC, iso_week ASC
		 LIMIT $2`

	rows, err := r.pool.Query(ctx, q, sku, nWeeks)
	if err != nil {
		return nil, fmt.Errorf("GetWeeklySalesByVariant: %w", err)
	}
	defer rows.Close()

	var result []domain.WeeklySales
	for rows.Next() {
		var ws domain.WeeklySales
		var grossStr string
		if err := rows.Scan(&ws.VariantID, &ws.SKU, &ws.Year, &ws.ISOWeek, &ws.UnitsSold, &grossStr); err != nil {
			return nil, fmt.Errorf("GetWeeklySalesByVariant scan: %w", err)
		}
		ws.GrossRevenue, _ = decimal.NewFromString(grossStr)
		result = append(result, ws)
	}
	return result, rows.Err()
}

// GetAllVariantsWithStock returns every active variant with current stock level.
func (r *AnalyticsRepository) GetAllVariantsWithStock(ctx context.Context) ([]domain.VariantStockRow, error) {
	const q = `
		SELECT v.id, v.sku, v.product_id, p.name,
		       COALESCE(i.quantity_available, 0) AS stock
		  FROM variants v
		  JOIN products p ON p.id = v.product_id
		  LEFT JOIN inventory i ON i.variant_id = v.id
		 WHERE p.is_active = TRUE
		 ORDER BY stock ASC`

	rows, err := r.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("GetAllVariantsWithStock: %w", err)
	}
	defer rows.Close()

	var result []domain.VariantStockRow
	for rows.Next() {
		var row domain.VariantStockRow
		if err := rows.Scan(&row.VariantID, &row.SKU, &row.ProductID, &row.Name, &row.Stock); err != nil {
			return nil, fmt.Errorf("GetAllVariantsWithStock scan: %w", err)
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

// GetPromotionStats returns efficacy metrics for recent active/expired promotions.
func (r *AnalyticsRepository) GetPromotionStats(ctx context.Context) ([]domain.PromotionStat, error) {
	const q = `
		SELECT
		    pp.id, pp.variant_id, v.sku,
		    ch.channel_type,
		    pp.promo_price, cp.price AS standard_price,
		    COUNT(oi.id)            AS hit_count,
		    COALESCE(SUM(oi.line_total), 0) AS total_revenue,
		    pp.effective_from, pp.effective_until
		  FROM price_promotions pp
		  JOIN variants      v  ON v.id   = pp.variant_id
		  JOIN channels      ch ON ch.id  = pp.channel_id
		  JOIN channel_prices cp ON cp.variant_id = pp.variant_id AND cp.channel_id = pp.channel_id
		  LEFT JOIN order_items oi ON oi.variant_id = pp.variant_id
		                          AND oi.promotion_id = pp.id
		 WHERE pp.effective_from >= NOW() - INTERVAL '90 days'
		 GROUP BY pp.id, pp.variant_id, v.sku, ch.channel_type,
		          pp.promo_price, cp.price, pp.effective_from, pp.effective_until
		 ORDER BY hit_count DESC`

	rows, err := r.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("GetPromotionStats: %w", err)
	}
	defer rows.Close()

	var result []domain.PromotionStat
	for rows.Next() {
		var ps domain.PromotionStat
		var promoStr, standardStr, revenueStr string
		if err := rows.Scan(
			&ps.PromotionID, &ps.VariantID, &ps.SKU,
			&ps.Channel,
			&promoStr, &standardStr,
			&ps.HitCount, &revenueStr,
			&ps.EffectiveFrom, &ps.EffectiveUntil,
		); err != nil {
			return nil, fmt.Errorf("GetPromotionStats scan: %w", err)
		}
		ps.PromoPrice, _ = decimal.NewFromString(promoStr)
		ps.StandardPrice, _ = decimal.NewFromString(standardStr)
		ps.TotalRevenue, _ = decimal.NewFromString(revenueStr)
		discountAmt := ps.StandardPrice.Sub(ps.PromoPrice)
		if !ps.StandardPrice.IsZero() {
			ps.AvgDiscount = discountAmt.Div(ps.StandardPrice)
		}
		result = append(result, ps)
	}
	return result, rows.Err()
}

// GetCustomerReturnStats returns per-customer return counts and QC-fail totals.
func (r *AnalyticsRepository) GetCustomerReturnStats(ctx context.Context, since time.Time) ([]domain.CustomerReturnStat, error) {
	const q = `
		SELECT
		    o.customer_email,
		    COUNT(DISTINCT o.id)            AS total_orders,
		    COUNT(DISTINCT ret.id)          AS total_returns,
		    COALESCE(SUM(CASE WHEN ri.qc_passed = FALSE THEN 1 ELSE 0 END), 0) AS qc_mismatches,
		    MAX(ret.requested_at)           AS last_return_date
		  FROM orders o
		  LEFT JOIN returns ret ON ret.order_id = o.id AND ret.requested_at >= $1
		  LEFT JOIN return_items ri ON ri.return_id = ret.id
		 WHERE o.created_at >= $1
		 GROUP BY o.customer_email
		HAVING COUNT(DISTINCT ret.id) > 0
		 ORDER BY total_returns DESC`

	rows, err := r.pool.Query(ctx, q, since)
	if err != nil {
		return nil, fmt.Errorf("GetCustomerReturnStats: %w", err)
	}
	defer rows.Close()

	var result []domain.CustomerReturnStat
	for rows.Next() {
		var stat domain.CustomerReturnStat
		if err := rows.Scan(
			&stat.CustomerEmail,
			&stat.TotalOrders, &stat.TotalReturns,
			&stat.QCMismatches, &stat.LastReturnDate,
		); err != nil {
			return nil, fmt.Errorf("GetCustomerReturnStats scan: %w", err)
		}
		result = append(result, stat)
	}
	return result, rows.Err()
}

// ensure uuid is used (avoids import-not-used error from future refactors)
var _ = uuid.Nil
