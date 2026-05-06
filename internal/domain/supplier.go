package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// POStatus is the lifecycle state of a purchase order.
type POStatus string

const (
	POStatusDraft              POStatus = "draft"
	POStatusSent               POStatus = "sent"
	POStatusConfirmed          POStatus = "confirmed"
	POStatusPartiallyReceived  POStatus = "partially_received"
	POStatusReceived           POStatus = "received"
	POStatusCancelled          POStatus = "cancelled"
)

// Supplier is a goods supplier (e.g. a China factory).
type Supplier struct {
	ID              uuid.UUID `json:"id"`
	Name            string    `json:"name"`
	ContactName     string    `json:"contact_name"`
	Phone           string    `json:"phone"`
	Email           string    `json:"email"`
	Country         string    `json:"country"`
	LeadTimeDays    int       `json:"lead_time_days"`
	MinimumOrderQty int       `json:"minimum_order_qty"`
	Rating          int       `json:"rating"` // 1–5
	Notes           string    `json:"notes"`
	IsActive        bool      `json:"is_active"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// PurchaseOrder tracks a request for goods from a supplier.
type PurchaseOrder struct {
	ID              uuid.UUID        `json:"id"`
	SupplierID      *uuid.UUID       `json:"supplier_id,omitempty"`
	Status          POStatus         `json:"status"`
	ReferenceNumber string           `json:"reference_number"`
	Notes           string           `json:"notes"`
	TotalCost       decimal.Decimal  `json:"total_cost"`
	Currency        string           `json:"currency"`
	ExpectedAt      *time.Time       `json:"expected_at,omitempty"`
	ReceivedAt      *time.Time       `json:"received_at,omitempty"`
	Items           []POItem         `json:"items,omitempty"`
	CreatedAt       time.Time        `json:"created_at"`
	UpdatedAt       time.Time        `json:"updated_at"`
}

// POItem is one line on a purchase order.
type POItem struct {
	ID              uuid.UUID       `json:"id"`
	PurchaseOrderID uuid.UUID       `json:"purchase_order_id"`
	VariantID       uuid.UUID       `json:"variant_id"`
	Quantity        int             `json:"quantity"`
	UnitCost        decimal.Decimal `json:"unit_cost"`
	ReceivedQty     int             `json:"received_qty"`
	CreatedAt       time.Time       `json:"created_at"`
}
