// Package integrations defines the ChannelConnector interface that every
// external platform adapter must implement.
//
// Each connector lives in its own sub-package (shopify, amazon, instagram, etc.)
// and is only used when the corresponding platform account is active.
// The entire omnichannel module is disabled by default and operates without
// any side-effects when no platform accounts are configured.
package integrations

import (
	"context"
	"time"

	"github.com/dubai-retail/os/internal/domain"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// ExternalOrder is a normalised order fetched from an external platform.
// Connectors translate platform-native schemas into this common struct.
type ExternalOrder struct {
	ExternalOrderID string          `json:"external_order_id"`
	CustomerEmail   string          `json:"customer_email"`
	CustomerName    string          `json:"customer_name"`
	Currency        string          `json:"currency"`
	TotalAmount     decimal.Decimal `json:"total_amount"`
	Lines           []ExternalLine  `json:"lines"`
	PlacedAt        time.Time       `json:"placed_at"`
}

// ExternalLine is one product line within an external order.
type ExternalLine struct {
	ExternalVariantID string          `json:"external_variant_id"`
	Quantity          int             `json:"quantity"`
	UnitPrice         decimal.Decimal `json:"unit_price"`
}

// ChannelConnector is the interface every platform adapter must satisfy.
// Methods are called by ChannelSyncService and Asynq workers.
type ChannelConnector interface {
	// PlatformName returns the platform type string (e.g. "shopify").
	PlatformName() string

	// PublishProduct pushes a local variant to the external platform.
	// Creates or updates the external listing.
	PublishProduct(ctx context.Context, account *domain.PlatformAccount, variant *domain.Variant, price decimal.Decimal, currency string) error

	// UpdateInventory syncs the available stock count for one variant.
	UpdateInventory(ctx context.Context, account *domain.PlatformAccount, externalVariantID string, qty int) error

	// UpdatePrice syncs the price of one variant.
	UpdatePrice(ctx context.Context, account *domain.PlatformAccount, externalVariantID string, price decimal.Decimal, currency string) error

	// FetchOrders pulls new orders placed since the given time.
	// Returns normalised ExternalOrder objects.
	FetchOrders(ctx context.Context, account *domain.PlatformAccount, since time.Time) ([]ExternalOrder, error)
}

// Registry maps platform type → connector implementation.
// Register connectors at init() time from each adapter package.
var Registry = map[domain.PlatformType]ChannelConnector{}

// Register adds a connector to the global registry.
// Call from adapter package init() functions.
func Register(platformType domain.PlatformType, c ChannelConnector) {
	Registry[platformType] = c
}

// Get returns the connector for a platform type, or nil if not registered.
func Get(platformType domain.PlatformType) ChannelConnector {
	return Registry[platformType]
}

// MockConnector is a no-op connector used in tests.
type MockConnector struct {
	Name     string
	Products []domain.Variant
	Orders   []ExternalOrder
}

func (m *MockConnector) PlatformName() string { return m.Name }
func (m *MockConnector) PublishProduct(_ context.Context, _ *domain.PlatformAccount, v *domain.Variant, _ decimal.Decimal, _ string) error {
	m.Products = append(m.Products, *v)
	return nil
}
func (m *MockConnector) UpdateInventory(_ context.Context, _ *domain.PlatformAccount, _ string, _ int) error {
	return nil
}
func (m *MockConnector) UpdatePrice(_ context.Context, _ *domain.PlatformAccount, _ string, _ decimal.Decimal, _ string) error {
	return nil
}
func (m *MockConnector) FetchOrders(_ context.Context, _ *domain.PlatformAccount, _ time.Time) ([]ExternalOrder, error) {
	return m.Orders, nil
}

// Ensure MockConnector satisfies the interface at compile time.
var _ ChannelConnector = (*MockConnector)(nil)

// ── Variant lookup helper (used by order import) ──────────────────────────────

// LocalVariantByExternalID resolves an external variant ID to a local variant UUID
// using the platform_products mapping table.
type VariantResolver interface {
	GetLocalVariantID(ctx context.Context, accountID uuid.UUID, externalVariantID string) (uuid.UUID, error)
}
