package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// PurchaseBatch represents a single import shipment from a supplier.
type PurchaseBatch struct {
	ID                 uuid.UUID       `db:"id"                   json:"id"`
	Reference          string          `db:"reference"            json:"reference"`
	SupplierName       string          `db:"supplier_name"        json:"supplier_name"`
	OriginCountry      string          `db:"origin_country"       json:"origin_country"`
	TotalShippingCost  decimal.Decimal `db:"total_shipping_cost"  json:"total_shipping_cost"`
	TotalInsurance     decimal.Decimal `db:"total_insurance"      json:"total_insurance"`
	CustomsDutyRate    decimal.Decimal `db:"customs_duty_rate"    json:"customs_duty_rate"`
	OrderedAt          time.Time       `db:"ordered_at"           json:"ordered_at"`
	ReceivedAt         *time.Time      `db:"received_at"          json:"received_at,omitempty"`
	Notes              *string         `db:"notes"                json:"notes,omitempty"`
	CreatedAt          time.Time       `db:"created_at"           json:"created_at"`
	UpdatedAt          time.Time       `db:"updated_at"           json:"updated_at"`

	Items []BatchItem `db:"-" json:"items,omitempty"`
}

// BatchItem is a single line within a purchase batch.
// landed_cost_per_unit is a PostgreSQL generated column; we read it but never write it.
type BatchItem struct {
	ID                 uuid.UUID       `db:"id"                    json:"id"`
	BatchID            uuid.UUID       `db:"batch_id"              json:"batch_id"`
	VariantID          uuid.UUID       `db:"variant_id"            json:"variant_id"`
	QuantityOrdered    int             `db:"quantity_ordered"      json:"quantity_ordered"`
	QuantityReceived   int             `db:"quantity_received"     json:"quantity_received"`
	UnitCost           decimal.Decimal `db:"unit_cost"             json:"unit_cost"`
	ShippingAllocation decimal.Decimal `db:"shipping_allocation"   json:"shipping_allocation"`
	CustomsDuty        decimal.Decimal `db:"customs_duty"          json:"customs_duty"`
	Insurance          decimal.Decimal `db:"insurance"             json:"insurance"`

	// Computed (STORED generated column) – read-only
	LandedCostPerUnit  decimal.Decimal `db:"landed_cost_per_unit"  json:"landed_cost_per_unit"`

	InputVATPerUnit    decimal.Decimal `db:"input_vat_per_unit"    json:"input_vat_per_unit"`
	CreatedAt          time.Time       `db:"created_at"            json:"created_at"`
	UpdatedAt          time.Time       `db:"updated_at"            json:"updated_at"`
}

// RemainingQty returns how much of this batch item is still available for FIFO deduction.
// The caller must supply the total already-deducted quantity from inventory_movements.
func (b *BatchItem) RemainingQty(deducted int) int {
	remaining := b.QuantityReceived - deducted
	if remaining < 0 {
		return 0
	}
	return remaining
}

// LandedCostEngine computes and stamps per-unit costs on a BatchItem given batch-level costs.
// Call this before inserting a new BatchItem.
func LandedCostEngine(item *BatchItem, batch *PurchaseBatch, totalUnits int) {
	if totalUnits == 0 {
		return
	}
	units := decimal.NewFromInt(int64(totalUnits))

	item.ShippingAllocation = batch.TotalShippingCost.Div(units).Round(4)
	item.Insurance = batch.TotalInsurance.Div(units).Round(4)
	item.CustomsDuty = item.UnitCost.Mul(batch.CustomsDutyRate).Round(4)

	// Input VAT (UAE FTA: paid on import, claimable)
	item.InputVATPerUnit = item.UnitCost.Add(item.CustomsDuty).Mul(decimal.NewFromFloat(0.05)).Round(4)
}
