// Package shipping defines the ShippingConnector interface and a global
// registry for carrier adapters. Each connector is registered via its
// package's init() function, following the same pattern as the omnichannel
// integrations.
package shipping

import (
	"context"
	"fmt"
	"time"

	"github.com/dubai-retail/os/internal/domain"
	"github.com/google/uuid"
)

// =============================================================================
// Input / Output types
// =============================================================================

// CreateShipmentInput carries everything a carrier needs to book a shipment.
type CreateShipmentInput struct {
	ShipmentID    uuid.UUID
	OrderID       uuid.UUID
	RecipientName string
	RecipientPhone string
	Address       domain.ShippingAddress
	WeightG       float64
	Description   string // e.g. "Fashion clothing × 3"
	CODAmount     float64 // cash-on-delivery; 0 = prepaid
}

// CreateShipmentResult holds the carrier's response after booking.
type CreateShipmentResult struct {
	TrackingNumber string
	CarrierRef     string
	LabelURL       string // printable shipping label
}

// TrackingEvent is a single carrier tracking update.
type TrackingEvent struct {
	Status      string
	Location    string
	Description string
	EventTime   time.Time
}

// =============================================================================
// Connector interface
// =============================================================================

// ShippingConnector is the contract every carrier adapter must satisfy.
type ShippingConnector interface {
	// ProviderType returns the canonical provider key (matches shipping_providers.type).
	ProviderType() string

	// CreateShipment books a shipment with the carrier and returns tracking info.
	CreateShipment(ctx context.Context, account *domain.ShippingAccount, input CreateShipmentInput) (*CreateShipmentResult, error)

	// GetTracking fetches the latest tracking events for a shipment.
	GetTracking(ctx context.Context, account *domain.ShippingAccount, trackingNumber string) ([]TrackingEvent, error)

	// CancelShipment requests cancellation of a booked shipment.
	CancelShipment(ctx context.Context, account *domain.ShippingAccount, trackingNumber string) error
}

// =============================================================================
// Global registry
// =============================================================================

// Registry maps provider type keys to their connector implementations.
// Connectors register themselves in their package's init() function.
var Registry = map[string]ShippingConnector{}

// Register adds a connector to the global registry. Call from init().
func Register(c ShippingConnector) {
	Registry[c.ProviderType()] = c
}

// Get returns the connector for the given provider type, or an error if none
// is registered.
func Get(providerType string) (ShippingConnector, error) {
	c, ok := Registry[providerType]
	if !ok {
		return nil, fmt.Errorf("shipping: no connector registered for provider %q", providerType)
	}
	return c, nil
}

// =============================================================================
// Mock connector (used in tests)
// =============================================================================

// MockConnector is a no-op implementation for unit testing.
type MockConnector struct {
	ProviderTypeVal string
	CreateResult    *CreateShipmentResult
	CreateErr       error
	TrackingResult  []TrackingEvent
	TrackingErr     error
	CancelErr       error
}

func (m *MockConnector) ProviderType() string { return m.ProviderTypeVal }

func (m *MockConnector) CreateShipment(_ context.Context, _ *domain.ShippingAccount, _ CreateShipmentInput) (*CreateShipmentResult, error) {
	if m.CreateResult != nil {
		return m.CreateResult, m.CreateErr
	}
	return &CreateShipmentResult{
		TrackingNumber: "MOCK-TRK-001",
		CarrierRef:     "MOCK-REF-001",
	}, m.CreateErr
}

func (m *MockConnector) GetTracking(_ context.Context, _ *domain.ShippingAccount, _ string) ([]TrackingEvent, error) {
	return m.TrackingResult, m.TrackingErr
}

func (m *MockConnector) CancelShipment(_ context.Context, _ *domain.ShippingAccount, _ string) error {
	return m.CancelErr
}
