package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// =============================================================================
// Customer Tier  –  mirrors the PostgreSQL customer_tier enum
// =============================================================================

// CustomerTier controls which promotion tiers apply to a given buyer.
type CustomerTier string

const (
	CustomerTierStandard  CustomerTier = "standard"
	CustomerTierVIP       CustomerTier = "vip"
	CustomerTierWholesale CustomerTier = "wholesale"
	CustomerTierStaff     CustomerTier = "staff"
)

// =============================================================================
// PricePromotion  –  time-bounded promotional price
// =============================================================================

// PricePromotion represents a row in the price_promotions table.
// A nil CustomerTier means the promotion applies to all tiers.
type PricePromotion struct {
	ID             uuid.UUID     `db:"id"              json:"id"`
	VariantID      uuid.UUID     `db:"variant_id"      json:"variant_id"`
	ChannelID      uuid.UUID     `db:"channel_id"      json:"channel_id"`
	CustomerTier   *CustomerTier `db:"customer_tier"   json:"customer_tier,omitempty"`
	PromoPrice     decimal.Decimal `db:"promo_price"   json:"promo_price"`
	Currency       string        `db:"currency"        json:"currency"`
	EffectiveFrom  time.Time     `db:"effective_from"  json:"effective_from"`
	EffectiveUntil time.Time     `db:"effective_until" json:"effective_until"`
	IsActive       bool          `db:"is_active"       json:"is_active"`
	CreatedAt      time.Time     `db:"created_at"      json:"created_at"`
	UpdatedAt      time.Time     `db:"updated_at"      json:"updated_at"`
}

// IsCurrentlyActive reports whether the promotion window covers the given time.
func (p *PricePromotion) IsCurrentlyActive(now time.Time) bool {
	return p.IsActive &&
		!now.Before(p.EffectiveFrom) &&
		now.Before(p.EffectiveUntil)
}

// =============================================================================
// PriceResult  –  output of the PriceResolver
// =============================================================================

// PriceSource identifies how the price was determined.
type PriceSource string

const (
	PriceSourceStandard  PriceSource = "standard"
	PriceSourcePromotion PriceSource = "promotion"
)

// PriceResult is the fully-computed price for one variant on one channel.
// VATAmountAED always carries the VAT in AED regardless of the order currency.
type PriceResult struct {
	VariantID uuid.UUID `json:"variant_id"`
	ChannelID uuid.UUID `json:"channel_id"`

	// Core financials
	NetPrice   decimal.Decimal `json:"net_price"`   // price before VAT
	VATRate    decimal.Decimal `json:"vat_rate"`    // 0.05 or 0.00
	VATAmount  decimal.Decimal `json:"vat_amount"`  // NetPrice × VATRate, rounded to 2dp
	GrossPrice decimal.Decimal `json:"gross_price"` // NetPrice + VATAmount

	// Always in AED (BT-111 dual-currency compliance)
	VATAmountAED decimal.Decimal `json:"vat_amount_aed"`

	Currency string `json:"currency"`

	// Source metadata
	PriceSource    PriceSource  `json:"price_source"`
	PromotionID    *uuid.UUID   `json:"promotion_id,omitempty"`
	AppliedTier    *CustomerTier `json:"applied_tier,omitempty"`
}

// =============================================================================
// OrderInvoice  –  compliance archive row
// =============================================================================

// InvoiceDocType mirrors the PostgreSQL invoice_doc_type enum.
type InvoiceDocType string

const (
	InvoiceDocTypeEInvoice InvoiceDocType = "einvoice_ubl"
	InvoiceDocTypeReceipt  InvoiceDocType = "receipt"
)

// OrderInvoice is a single entry in the order_invoices compliance table.
type OrderInvoice struct {
	ID                uuid.UUID      `db:"id"                   json:"id"`
	OrderID           uuid.UUID      `db:"order_id"             json:"order_id"`
	InvoiceType       InvoiceDocType `db:"invoice_type"         json:"invoice_type"`
	InvoiceNumber     string         `db:"invoice_number"       json:"invoice_number"`
	XMLContent        *string        `db:"xml_content"          json:"xml_content,omitempty"`
	ExchangeRateToAED decimal.Decimal `db:"exchange_rate_to_aed" json:"exchange_rate_to_aed"`
	TriggerReason     string         `db:"trigger_reason"       json:"trigger_reason"`
	IssuedAt          time.Time      `db:"issued_at"            json:"issued_at"`
	CreatedAt         time.Time      `db:"created_at"           json:"created_at"`
}

// SandboxStatus represents the ASP validation outcome for an invoice.
type SandboxStatus string

const (
	SandboxStatusPending  SandboxStatus = "pending"
	SandboxStatusAccepted SandboxStatus = "accepted"
	SandboxStatusRejected SandboxStatus = "rejected"
	SandboxStatusError    SandboxStatus = "error"
)
