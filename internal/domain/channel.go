package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// ChannelType mirrors the PostgreSQL channel_type enum.
type ChannelType string

const (
	ChannelTypePOS        ChannelType = "pos"
	ChannelTypeEcommerce  ChannelType = "ecommerce"
	ChannelTypeWholesale  ChannelType = "wholesale"
)

// Channel represents a sales channel (POS, Website, Wholesale).
type Channel struct {
	ID          uuid.UUID   `db:"id"          json:"id"`
	Name        string      `db:"name"        json:"name"`
	Type        ChannelType `db:"type"        json:"type"`
	IsActive    bool        `db:"is_active"   json:"is_active"`
	Description *string     `db:"description" json:"description,omitempty"`
	CreatedAt   time.Time   `db:"created_at"  json:"created_at"`
	UpdatedAt   time.Time   `db:"updated_at"  json:"updated_at"`
}

// ChannelPrice stores the selling price for a variant on a given channel.
type ChannelPrice struct {
	ID             uuid.UUID       `db:"id"              json:"id"`
	VariantID      uuid.UUID       `db:"variant_id"      json:"variant_id"`
	ChannelID      uuid.UUID       `db:"channel_id"      json:"channel_id"`
	Price          decimal.Decimal `db:"price"           json:"price"`
	Currency       string          `db:"currency"        json:"currency"`
	IsActive       bool            `db:"is_active"       json:"is_active"`
	EffectiveFrom  time.Time       `db:"effective_from"  json:"effective_from"`
	EffectiveUntil *time.Time      `db:"effective_until" json:"effective_until,omitempty"`
	CreatedAt      time.Time       `db:"created_at"      json:"created_at"`
	UpdatedAt      time.Time       `db:"updated_at"      json:"updated_at"`
}
