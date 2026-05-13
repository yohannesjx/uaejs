package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// PageResponse is the standard paginated API response shape.
type PageResponse[T any] struct {
	Items []T `json:"items"`
	Total int `json:"total"`
}

// ProductListFilters for GET /products list endpoint.
type ProductListFilters struct {
	TenantID       uuid.UUID
	Page           int
	PageSize       int
	Search         string
	Status         string
	Category       string
	WarehouseID    *uuid.UUID
	CollectionID   *uuid.UUID
	// AggregateVariantInventory sums quantity_available across active variants (admin picker UX).
	AggregateVariantInventory bool
	// PublicCatalog strips unit cost from each row (used by unauthenticated product list).
	PublicCatalog bool
}

// ProductListItem is a single row in the products list.
type ProductListItem struct {
	ID        uuid.UUID       `json:"id"`
	ProductID uuid.UUID       `json:"product_id"`
	Name      string          `json:"name"`
	Slug      string          `json:"slug"`
	SKU       string          `json:"sku"`
	Category  *string         `json:"category,omitempty"`
	Thumbnail *string         `json:"thumbnail,omitempty"`
	// Cost is unit cost (COGS) for the primary list variant, when set.
	Cost      *string         `json:"cost,omitempty"`
	Price     decimal.Decimal `json:"price"`
	Stock     int             `json:"stock"`
	Status    string          `json:"status"`
	CreatedAt time.Time       `json:"created_at"`
}

// OrderListFilters for GET /orders list endpoint.
type OrderListFilters struct {
	TenantID   uuid.UUID
	Page       int
	PageSize   int
	Status     string
	Channel    string
	DateFrom   *time.Time
	DateTo     *time.Time
	CustomerID *uuid.UUID
}

// OrderListItem is a single row in the orders list.
type OrderListItem struct {
	ID             uuid.UUID       `json:"id"`
	ChannelID      uuid.UUID       `json:"channel_id"`
	ChannelName    string          `json:"channel_name"`
	ChannelType    string          `json:"channel_type"`
	CustomerID     *uuid.UUID      `json:"customer_id,omitempty"`
	CustomerName   *string         `json:"customer_name,omitempty"`
	CustomerEmail  *string         `json:"customer_email,omitempty"`
	TotalAmount    decimal.Decimal `json:"total_amount"`
	Currency       string          `json:"currency"`
	Status         string          `json:"status"`
	PaymentStatus  string          `json:"payment_status"`
	CreatedAt      time.Time       `json:"created_at"`
}

// CustomerListFilters for GET /customers list endpoint.
type CustomerListFilters struct {
	TenantID uuid.UUID
	Page     int
	PageSize int
	Search   string
	Tier     string
	Email    string
}

// CustomerListItem is a single row in the customers list.
type CustomerListItem struct {
	ID             uuid.UUID `json:"id"`
	Email          string    `json:"email"`
	FullName       string    `json:"full_name"`
	Phone          *string   `json:"phone,omitempty"`
	LoyaltyTier    string    `json:"loyalty_tier"`
	PointsBalance  int       `json:"points_balance"`
	LifetimePoints int       `json:"lifetime_points"`
	IsActive       bool      `json:"is_active"`
	CreatedAt      time.Time `json:"created_at"`
}

// ReturnListFilters for GET /returns list endpoint.
type ReturnListFilters struct {
	TenantID   uuid.UUID
	Page       int
	PageSize   int
	Status     string
	OrderID    *uuid.UUID
	CustomerID *uuid.UUID
}

// ReturnListItem is a single row in the returns list.
type ReturnListItem struct {
	ID            uuid.UUID `json:"id"`
	OrderID       uuid.UUID `json:"order_id"`
	CustomerID    *uuid.UUID `json:"customer_id,omitempty"`
	CustomerName  string    `json:"customer_name"`
	CustomerEmail string    `json:"customer_email"`
	Status        string    `json:"status"`
	ReturnReason  string    `json:"return_reason"`
	ItemCount     int       `json:"item_count"`
	RequestedAt   time.Time `json:"requested_at"`
}

// ShipmentListFilters for GET /shipments list endpoint.
type ShipmentListFilters struct {
	TenantID    uuid.UUID
	Page        int
	PageSize    int
	Status      string
	Carrier     string
	WarehouseID *uuid.UUID
}

// ShipmentListItem is a single row in the shipments list.
type ShipmentListItem struct {
	ID             uuid.UUID  `json:"id"`
	OrderID        uuid.UUID  `json:"order_id"`
	AccountID      *uuid.UUID `json:"account_id,omitempty"`
	TrackingNumber *string    `json:"tracking_number,omitempty"`
	CarrierRef     *string    `json:"carrier_ref,omitempty"`
	Status         string     `json:"status"`
	Carrier        *string    `json:"carrier,omitempty"`
	WarehouseID    *uuid.UUID `json:"warehouse_id,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

// ActivityLogFilters for GET /activity-log endpoint.
type ActivityLogFilters struct {
	TenantID uuid.UUID
	Page     int
	PageSize int
	Search   string
}

// ActivityLogItem is a single row in the activity log.
type ActivityLogItem struct {
	ID          string         `json:"id"`
	EventType   string         `json:"event_type"`
	Title       string         `json:"title"`
	Description string         `json:"description"`
	Actor       string         `json:"actor"`
	SubjectID   string         `json:"subject_id"`
	SubjectType string         `json:"subject_type"`
	CreatedAt   time.Time      `json:"created_at"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}
