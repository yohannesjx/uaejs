package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// PlatformType identifies the external marketplace connector.
type PlatformType string

const (
	PlatformShopify   PlatformType = "shopify"
	PlatformAmazon    PlatformType = "amazon"
	PlatformInstagram PlatformType = "instagram"
	PlatformTikTok    PlatformType = "tiktok"
	PlatformNoon      PlatformType = "noon"
)

// ExternalPlatform is a registered external sales channel (e.g. "Shopify UAE").
// Disabled by default; must be explicitly activated.
type ExternalPlatform struct {
	ID        uuid.UUID    `json:"id"`
	Name      string       `json:"name"`
	Type      PlatformType `json:"type"`
	IsActive  bool         `json:"is_active"`
	CreatedAt time.Time    `json:"created_at"`
}

// PlatformAccount holds the credentials for one store on an external platform.
type PlatformAccount struct {
	ID         uuid.UUID       `json:"id"`
	PlatformID uuid.UUID       `json:"platform_id"`
	StoreName  string          `json:"store_name"`
	APIKey     string          `json:"-"` // never serialised
	APISecret  string          `json:"-"`
	Settings   json.RawMessage `json:"settings"`
	IsActive   bool            `json:"is_active"`
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at"`
}

// PlatformProduct links a local variant to an external listing.
type PlatformProduct struct {
	ID                 uuid.UUID  `json:"id"`
	PlatformAccountID  uuid.UUID  `json:"platform_account_id"`
	VariantID          uuid.UUID  `json:"variant_id"`
	ExternalProductID  string     `json:"external_product_id"`
	ExternalVariantID  string     `json:"external_variant_id"`
	LastSyncedAt       *time.Time `json:"last_synced_at,omitempty"`
	SyncStatus         string     `json:"sync_status"`
	SyncError          string     `json:"sync_error,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
}

// PlatformOrder records an order imported (or pending import) from an external platform.
type PlatformOrder struct {
	ID                uuid.UUID       `json:"id"`
	PlatformAccountID uuid.UUID       `json:"platform_account_id"`
	ExternalOrderID   string          `json:"external_order_id"`
	LocalOrderID      *uuid.UUID      `json:"local_order_id,omitempty"`
	Status            string          `json:"status"`
	RawPayload        json.RawMessage `json:"raw_payload,omitempty"`
	ErrorMessage      string          `json:"error_message,omitempty"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
}
