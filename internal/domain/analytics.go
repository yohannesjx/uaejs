package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// WeeklySales is one ISO week of sales volume for a variant.
type WeeklySales struct {
	VariantID    uuid.UUID
	SKU          string
	ISOWeek      int
	Year         int
	UnitsSold    int
	GrossRevenue decimal.Decimal
}

// VariantStockRow combines variant identity with current stock level.
type VariantStockRow struct {
	VariantID uuid.UUID
	SKU       string
	ProductID uuid.UUID
	Name      string
	Stock     int
}

// PromotionStat aggregates usage metrics for a single promotion.
type PromotionStat struct {
	PromotionID    uuid.UUID
	VariantID      uuid.UUID
	SKU            string
	Channel        ChannelType
	PromoPrice     decimal.Decimal
	StandardPrice  decimal.Decimal
	HitCount       int
	TotalRevenue   decimal.Decimal
	AvgDiscount    decimal.Decimal
	EffectiveFrom  time.Time
	EffectiveUntil time.Time
}

// CustomerReturnStat tracks return behaviour per customer.
type CustomerReturnStat struct {
	CustomerEmail  string
	TotalOrders    int
	TotalReturns   int
	QCMismatches   int
	LastReturnDate time.Time
}
