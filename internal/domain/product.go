package domain

import (
	"time"

	"github.com/google/uuid"
)

// VATType mirrors the PostgreSQL vat_type enum.
type VATType string

const (
	VATTypeStandard  VATType = "standard"
	VATTypeZeroRated VATType = "zero_rated"
	VATTypeExempt    VATType = "exempt"
)

// ProductStatus mirrors the PostgreSQL product_status enum.
type ProductStatus string

const (
	StatusDraft    ProductStatus = "draft"
	StatusActive   ProductStatus = "active"
	StatusArchived ProductStatus = "archived"
)

// Product represents a merchandise line (e.g. "Linen Co-ord Set").
type Product struct {
	ID              uuid.UUID     `db:"id"               json:"id"`
	Name            *string       `db:"name"             json:"name,omitempty"`
	Slug            *string       `db:"slug"             json:"slug,omitempty"`
	NameAR          *string       `db:"name_ar"          json:"name_ar,omitempty"`
	Description     *string       `db:"description"      json:"description,omitempty"`
	Brand           *string       `db:"brand"            json:"brand,omitempty"`
	Category        *string       `db:"category"         json:"category,omitempty"`
	SubCategory     *string       `db:"sub_category"     json:"sub_category,omitempty"`
	Status          ProductStatus `db:"status"           json:"status"`
	VATType         VATType       `db:"vat_type"         json:"vat_type"`
	HSCode          *string       `db:"hs_code"          json:"hs_code,omitempty"`
	CountryOfOrigin string        `db:"country_of_origin" json:"country_of_origin"`
	CreatedAt       time.Time     `db:"created_at"       json:"created_at"`
	UpdatedAt       time.Time     `db:"updated_at"       json:"updated_at"`
}

// Variant represents a specific SKU within a product (size + colour combination).
type Variant struct {
	ID        uuid.UUID `db:"id"         json:"id"`
	ProductID uuid.UUID `db:"product_id" json:"product_id"`
	SKU       *string   `db:"sku"        json:"sku,omitempty"`
	Barcode   *string   `db:"barcode"    json:"barcode,omitempty"`
	Color     *string   `db:"color"      json:"color,omitempty"`
	Size      *string   `db:"size"       json:"size,omitempty"`
	WeightG   *float64  `db:"weight_g"   json:"weight_g,omitempty"`
	ImageURL  *string   `db:"image_url"  json:"image_url,omitempty"`
	Price     *string   `db:"price"      json:"price,omitempty"`
	SalePrice *string   `db:"sale_price" json:"sale_price,omitempty"`
	Quantity  *int      `db:"quantity"   json:"quantity,omitempty"`
	IsActive  bool      `db:"is_active"  json:"is_active"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
	MediaURLs []string  `db:"-"          json:"media_urls,omitempty"`

	// Eagerly loaded in API responses
	Product *Product `db:"-" json:"product,omitempty"`
}
