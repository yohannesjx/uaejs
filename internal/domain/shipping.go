package domain

import (
	"time"

	"github.com/google/uuid"
)

// ShipmentStatus mirrors the shipment_status DB enum.
type ShipmentStatus string

const (
	ShipmentPending         ShipmentStatus = "pending"
	ShipmentBooked          ShipmentStatus = "booked"
	ShipmentPickedUp        ShipmentStatus = "picked_up"
	ShipmentInTransit       ShipmentStatus = "in_transit"
	ShipmentOutForDelivery  ShipmentStatus = "out_for_delivery"
	ShipmentDelivered       ShipmentStatus = "delivered"
	ShipmentFailed          ShipmentStatus = "failed"
	ShipmentCancelled       ShipmentStatus = "cancelled"
	ShipmentReturned        ShipmentStatus = "returned"
)

// ShippingProvider is an entry in the shipping_providers registry.
type ShippingProvider struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Type      string    `json:"type"` // connector key: "aramex", "dhl", etc.
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
}

// ShippingAccount holds API credentials for a specific provider.
type ShippingAccount struct {
	ID         uuid.UUID         `json:"id"`
	ProviderID uuid.UUID         `json:"provider_id"`
	Label      string            `json:"label"`
	APIKey     string            `json:"-"` // never serialised
	APISecret  string            `json:"-"` // never serialised
	Settings   map[string]any    `json:"settings"`
	IsActive   bool              `json:"is_active"`
	CreatedAt  time.Time         `json:"created_at"`
	UpdatedAt  time.Time         `json:"updated_at"`
}

// Shipment links an order to a carrier booking.
type Shipment struct {
	ID             uuid.UUID      `json:"id"`
	OrderID        uuid.UUID      `json:"order_id"`
	AccountID      *uuid.UUID     `json:"account_id,omitempty"`
	TrackingNumber *string        `json:"tracking_number,omitempty"`
	CarrierRef     *string        `json:"carrier_ref,omitempty"`
	Status         ShipmentStatus `json:"status"`
	WeightG        *float64       `json:"weight_g,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`

	Events []ShipmentEvent `json:"events,omitempty"`
}

// ShipmentEvent is one immutable entry in the tracking log.
type ShipmentEvent struct {
	ID          uuid.UUID `json:"id"`
	ShipmentID  uuid.UUID `json:"shipment_id"`
	Status      string    `json:"status"`
	Location    string    `json:"location"`
	Description string    `json:"description"`
	EventTime   time.Time `json:"event_time"`
	RecordedAt  time.Time `json:"recorded_at"`
}
