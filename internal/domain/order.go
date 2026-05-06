package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// OrderStatus mirrors the PostgreSQL order_status enum.
type OrderStatus string

const (
	OrderStatusPending   OrderStatus = "pending"
	OrderStatusReserved  OrderStatus = "reserved"
	OrderStatusConfirmed OrderStatus = "confirmed"
	OrderStatusShipped   OrderStatus = "shipped"
	OrderStatusCompleted OrderStatus = "completed"
	OrderStatusCancelled OrderStatus = "cancelled"
	OrderStatusRefunded  OrderStatus = "refunded"
)

// PaymentStatus mirrors the PostgreSQL payment_status enum.
type PaymentStatus string

const (
	PaymentStatusUnpaid   PaymentStatus = "unpaid"
	PaymentStatusPartial  PaymentStatus = "partial"
	PaymentStatusPaid     PaymentStatus = "paid"
	PaymentStatusRefunded PaymentStatus = "refunded"
)

// ShippingAddress holds structured address data stored as JSONB.
type ShippingAddress struct {
	Line1      string `json:"line1"`
	Line2      string `json:"line2,omitempty"`
	City       string `json:"city"`
	Emirate    string `json:"emirate"`
	Country    string `json:"country"`
	PostalCode string `json:"postal_code,omitempty"`
}

// Order is the top-level sales transaction.
type Order struct {
	ID              uuid.UUID        `db:"id"               json:"id"`
	ChannelID       uuid.UUID        `db:"channel_id"       json:"channel_id"`
	CustomerID      *uuid.UUID       `db:"customer_id"      json:"customer_id,omitempty"` // FK → customers.id (nullable)
	CustomerName    *string          `db:"customer_name"    json:"customer_name,omitempty"`
	CustomerEmail   *string          `db:"customer_email"   json:"customer_email,omitempty"`
	CustomerPhone   *string          `db:"customer_phone"   json:"customer_phone,omitempty"`
	CustomerTRN     *string          `db:"customer_trn"     json:"customer_trn,omitempty"`
	ShippingAddress *ShippingAddress `db:"shipping_address" json:"shipping_address,omitempty"`
	Subtotal        decimal.Decimal  `db:"subtotal"         json:"subtotal"`
	DiscountAmount  decimal.Decimal  `db:"discount_amount"  json:"discount_amount"`
	VATAmount       decimal.Decimal  `db:"vat_amount"       json:"vat_amount"`
	TotalAmount     decimal.Decimal  `db:"total_amount"     json:"total_amount"`
	Currency        string           `db:"currency"         json:"currency"`
	VATType         VATType          `db:"vat_type"         json:"vat_type"`
	InvoiceNumber   *string          `db:"invoice_number"   json:"invoice_number,omitempty"`
	InvoiceIssuedAt *time.Time       `db:"invoice_issued_at" json:"invoice_issued_at,omitempty"`
	Status          OrderStatus      `db:"status"           json:"status"`
	PaymentStatus   PaymentStatus    `db:"payment_status"   json:"payment_status"`
	Notes           *string          `db:"notes"            json:"notes,omitempty"`
	CreatedAt       time.Time        `db:"created_at"       json:"created_at"`
	UpdatedAt       time.Time        `db:"updated_at"       json:"updated_at"`

	Items []OrderItem `db:"-" json:"items,omitempty"`
}

// OrderItem is a single line within an order.
type OrderItem struct {
	ID             uuid.UUID       `db:"id"              json:"id"`
	OrderID        uuid.UUID       `db:"order_id"        json:"order_id"`
	VariantID      uuid.UUID       `db:"variant_id"      json:"variant_id"`
	Quantity       int             `db:"quantity"        json:"quantity"`
	UnitPrice      decimal.Decimal `db:"unit_price"      json:"unit_price"`
	DiscountAmount decimal.Decimal `db:"discount_amount" json:"discount_amount"`
	VATRate        decimal.Decimal `db:"vat_rate"        json:"vat_rate"`
	VATAmount      decimal.Decimal `db:"vat_amount"      json:"vat_amount"`
	LineTotal      decimal.Decimal `db:"line_total"      json:"line_total"`
	COGSPerUnit    *decimal.Decimal `db:"cogs_per_unit"  json:"cogs_per_unit,omitempty"`
	TotalCOGS      *decimal.Decimal `db:"total_cogs"     json:"total_cogs,omitempty"`
	CreatedAt      time.Time       `db:"created_at"      json:"created_at"`
	UpdatedAt      time.Time       `db:"updated_at"      json:"updated_at"`
}

// StockReservation holds stock against an order until payment is confirmed.
type StockReservation struct {
	ID         uuid.UUID  `db:"id"          json:"id"`
	OrderID    uuid.UUID  `db:"order_id"    json:"order_id"`
	VariantID  uuid.UUID  `db:"variant_id"  json:"variant_id"`
	Quantity   int        `db:"quantity"    json:"quantity"`
	ExpiresAt  time.Time  `db:"expires_at"  json:"expires_at"`
	ReleasedAt *time.Time `db:"released_at" json:"released_at,omitempty"`
	IsActive   bool       `db:"is_active"   json:"is_active"`
	CreatedAt  time.Time  `db:"created_at"  json:"created_at"`
	UpdatedAt  time.Time  `db:"updated_at"  json:"updated_at"`
}

// =============================================================================
// UAE PINT-AE E-Invoice (UBL-compatible) data structures
// =============================================================================

// EInvoice encapsulates all fields required by UAE FTA PINT-AE 2026.
type EInvoice struct {
	InvoiceNumber   string          `json:"invoice_number"`
	IssueDate       time.Time       `json:"issue_date"`
	InvoiceType     string          `json:"invoice_type"` // "380" commercial, "381" credit note
	CurrencyCode    string          `json:"currency_code"`
	SupplierTRN     string          `json:"supplier_trn"`
	SupplierName    string          `json:"supplier_name"`
	SupplierAddress AddressUBL      `json:"supplier_address"`
	BuyerTRN        *string         `json:"buyer_trn,omitempty"`
	BuyerName       *string         `json:"buyer_name,omitempty"`
	BuyerAddress    *AddressUBL     `json:"buyer_address,omitempty"`
	Lines           []EInvoiceLine  `json:"lines"`
	Subtotal        decimal.Decimal `json:"subtotal"`
	TaxTotal        decimal.Decimal `json:"tax_total"`
	GrandTotal      decimal.Decimal `json:"grand_total"`
}

// AddressUBL mirrors the UBL PostalAddress structure.
type AddressUBL struct {
	StreetName   string `json:"street_name"`
	CityName     string `json:"city_name"`
	PostalZone   string `json:"postal_zone,omitempty"`
	CountryCode  string `json:"country_code"` // "AE"
}

// EInvoiceLine is a UBL InvoiceLine item.
type EInvoiceLine struct {
	LineID          string          `json:"line_id"`
	Description     string          `json:"description"`
	Quantity        int             `json:"quantity"`
	UnitCode        string          `json:"unit_code"` // "PCE"
	UnitPrice       decimal.Decimal `json:"unit_price"`
	LineExtension   decimal.Decimal `json:"line_extension_amount"`
	TaxCategoryCode string          `json:"tax_category_code"` // "S", "Z", "E"
	TaxRate         decimal.Decimal `json:"tax_rate"`
	TaxAmount       decimal.Decimal `json:"tax_amount"`
}
