package domain

import (
	"time"

	"github.com/google/uuid"
)

// ProductCollection is a flexible Shopify-style grouping (independent from categories).
type ProductCollection struct {
	ID           uuid.UUID   `json:"id"`
	TenantID     uuid.UUID   `json:"tenant_id"`
	Title        string      `json:"title"`
	Slug         string      `json:"slug"`
	Description  *string     `json:"description,omitempty"`
	ImageURL     *string     `json:"image_url,omitempty"`
	ProductCount int         `json:"product_count"`
	ProductIDs   []uuid.UUID `json:"product_ids,omitempty"`
	CreatedAt    time.Time   `json:"created_at"`
	UpdatedAt    time.Time   `json:"updated_at"`
}
