package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// POSPaymentMethod mirrors the pos_payment_method DB enum.
type POSPaymentMethod string

const (
	POSPaymentCash  POSPaymentMethod = "cash"
	POSPaymentCard  POSPaymentMethod = "card"
	POSPaymentSplit POSPaymentMethod = "split"
)

// POSRegister is a physical checkout terminal.
type POSRegister struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Location  string    `json:"location"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// POSSession represents an open cashier shift on a register.
type POSSession struct {
	ID          uuid.UUID  `json:"id"`
	RegisterID  uuid.UUID  `json:"register_id"`
	OpenedBy    uuid.UUID  `json:"opened_by"`
	OpenedAt    time.Time  `json:"opened_at"`
	ClosedAt    *time.Time `json:"closed_at,omitempty"`
	OpeningCash decimal.Decimal `json:"opening_cash"`
	ClosingCash *decimal.Decimal `json:"closing_cash,omitempty"`
	Notes       *string    `json:"notes,omitempty"`
}

// POSPayment records a single payment transaction for a POS order.
type POSPayment struct {
	ID            uuid.UUID        `json:"id"`
	OrderID       uuid.UUID        `json:"order_id"`
	SessionID     *uuid.UUID       `json:"session_id,omitempty"`
	PaymentMethod POSPaymentMethod `json:"payment_method"`
	Amount        decimal.Decimal  `json:"amount"`
	Currency      string           `json:"currency"`
	Reference     *string          `json:"reference,omitempty"`
	PaidAt        time.Time        `json:"paid_at"`
}

// BarcodeResult is returned by the barcode scan endpoint.
type BarcodeResult struct {
	Variant   *Variant        `json:"variant"`
	Product   *Product        `json:"product"`
	Price     decimal.Decimal `json:"price"`
	Currency  string          `json:"currency"`
	Stock     int             `json:"stock_available"`
}

// POSReceipt is the receipt data structure for JSON output and HTML printing.
type POSReceipt struct {
	ReceiptID     string           `json:"receipt_id"`
	StoreName     string           `json:"store_name"`
	RegisterName  string           `json:"register_name"`
	CashierName   string           `json:"cashier_name"`
	OrderID       uuid.UUID        `json:"order_id"`
	Items         []POSReceiptItem `json:"items"`
	Subtotal      decimal.Decimal  `json:"subtotal"`
	DiscountTotal decimal.Decimal  `json:"discount_total"`
	VATAmount     decimal.Decimal  `json:"vat_amount"`
	Total         decimal.Decimal  `json:"total"`
	PaymentMethod POSPaymentMethod `json:"payment_method"`
	AmountPaid    decimal.Decimal  `json:"amount_paid"`
	Change        decimal.Decimal  `json:"change"`
	Currency      string           `json:"currency"`
	IssuedAt      time.Time        `json:"issued_at"`
}

// POSReceiptItem is one line on the receipt.
type POSReceiptItem struct {
	SKU      string          `json:"sku"`
	Name     string          `json:"name"`
	Qty      int             `json:"qty"`
	UnitPrice decimal.Decimal `json:"unit_price"`
	LineTotal decimal.Decimal `json:"line_total"`
}
