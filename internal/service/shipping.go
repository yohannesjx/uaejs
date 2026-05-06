// Package service — Shipping / fulfillment service.
//
// ShippingService orchestrates shipment booking, tracking sync, and event
// recording. It delegates carrier I/O to ShippingConnector adapters (Aramex,
// DHL, Emirates Post) looked up from the global shipping registry.
package service

import (
	"context"
	"fmt"
	"time"

	"github.com/dubai-retail/os/internal/domain"
	"github.com/dubai-retail/os/internal/integrations/shipping"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// =============================================================================
// Repository interface
// =============================================================================

// ShippingRepo is the DB interface required by ShippingService.
type ShippingRepo interface {
	ListActiveProviders(ctx context.Context) ([]domain.ShippingProvider, error)
	GetProviderByType(ctx context.Context, providerType string) (*domain.ShippingProvider, error)
	InsertAccount(ctx context.Context, a *domain.ShippingAccount) error
	GetActiveAccountForProvider(ctx context.Context, providerID uuid.UUID) (*domain.ShippingAccount, error)
	GetAccountByID(ctx context.Context, id uuid.UUID) (*domain.ShippingAccount, error)
	InsertShipment(ctx context.Context, s *domain.Shipment) error
	UpdateShipmentStatus(ctx context.Context, shipmentID uuid.UUID, status domain.ShipmentStatus, trackingNumber, carrierRef *string) error
	GetShipmentByID(ctx context.Context, id uuid.UUID) (*domain.Shipment, error)
	GetShipmentByOrderID(ctx context.Context, orderID uuid.UUID) (*domain.Shipment, error)
	InsertEvent(ctx context.Context, e *domain.ShipmentEvent) error
	GetEventsByShipmentID(ctx context.Context, shipmentID uuid.UUID) ([]domain.ShipmentEvent, error)
}

// =============================================================================
// Service
// =============================================================================

// ShippingService manages shipment lifecycle and carrier interactions.
type ShippingService struct {
	repo     ShippingRepo
	registry map[string]shipping.ShippingConnector
	log      *zap.Logger
}

// NewShippingService creates a ShippingService.
// registry should be shipping.Registry for production; pass a custom map for tests.
func NewShippingService(repo ShippingRepo, registry map[string]shipping.ShippingConnector, log *zap.Logger) *ShippingService {
	return &ShippingService{repo: repo, registry: registry, log: log}
}

// =============================================================================
// Accounts
// =============================================================================

// AddAccountInput is the request to configure a carrier account.
type AddAccountInput struct {
	ProviderType string         `json:"provider_type"`
	Label        string         `json:"label"`
	APIKey       string         `json:"api_key"`
	APISecret    string         `json:"api_secret"`
	Settings     map[string]any `json:"settings"`
}

// AddAccount registers a new shipping account for a provider.
func (s *ShippingService) AddAccount(ctx context.Context, in AddAccountInput) (*domain.ShippingAccount, error) {
	provider, err := s.repo.GetProviderByType(ctx, in.ProviderType)
	if err != nil {
		return nil, fmt.Errorf("AddAccount: %w", err)
	}
	if !provider.IsActive {
		return nil, fmt.Errorf("AddAccount: provider %q is disabled", in.ProviderType)
	}

	if _, ok := s.registry[in.ProviderType]; !ok {
		return nil, fmt.Errorf("AddAccount: no connector registered for provider %q", in.ProviderType)
	}

	acc := &domain.ShippingAccount{
		ProviderID: provider.ID,
		Label:      in.Label,
		APIKey:     in.APIKey,
		APISecret:  in.APISecret,
		Settings:   in.Settings,
		IsActive:   true,
	}
	if err := s.repo.InsertAccount(ctx, acc); err != nil {
		return nil, fmt.Errorf("AddAccount: %w", err)
	}

	s.log.Info("shipping.account_added",
		zap.String("provider", in.ProviderType),
		zap.String("account_id", acc.ID.String()),
	)
	acc.APIKey = ""
	acc.APISecret = ""
	return acc, nil
}

// =============================================================================
// Shipment creation
// =============================================================================

// CreateShipmentInput is the request to book a shipment for an order.
type CreateShipmentInput struct {
	OrderID        uuid.UUID              `json:"order_id"`
	ProviderType   string                 `json:"provider_type"`
	RecipientName  string                 `json:"recipient_name"`
	RecipientPhone string                 `json:"recipient_phone"`
	Address        domain.ShippingAddress `json:"address"`
	WeightG        float64                `json:"weight_g"`
	Description    string                 `json:"description"`
	CODAmount      float64                `json:"cod_amount"`
}

// CreateShipment books a shipment with the carrier and stores the tracking number.
func (s *ShippingService) CreateShipment(ctx context.Context, in CreateShipmentInput) (*domain.Shipment, error) {
	if in.OrderID == uuid.Nil {
		return nil, fmt.Errorf("CreateShipment: order_id is required")
	}

	connector, ok := s.registry[in.ProviderType]
	if !ok {
		return nil, fmt.Errorf("CreateShipment: no connector for provider %q", in.ProviderType)
	}

	provider, err := s.repo.GetProviderByType(ctx, in.ProviderType)
	if err != nil {
		return nil, fmt.Errorf("CreateShipment: %w", err)
	}
	account, err := s.repo.GetActiveAccountForProvider(ctx, provider.ID)
	if err != nil {
		return nil, fmt.Errorf("CreateShipment: no active account for %q: %w", in.ProviderType, err)
	}

	shipment := &domain.Shipment{
		OrderID:   in.OrderID,
		AccountID: &account.ID,
		WeightG:   &in.WeightG,
	}
	if err := s.repo.InsertShipment(ctx, shipment); err != nil {
		return nil, fmt.Errorf("CreateShipment: insert: %w", err)
	}

	// Call carrier API
	result, err := connector.CreateShipment(ctx, account, shipping.CreateShipmentInput{
		ShipmentID:     shipment.ID,
		OrderID:        in.OrderID,
		RecipientName:  in.RecipientName,
		RecipientPhone: in.RecipientPhone,
		Address:        in.Address,
		WeightG:        in.WeightG,
		Description:    in.Description,
		CODAmount:      in.CODAmount,
	})
	if err != nil {
		// Mark as pending; retry can be triggered manually.
		s.log.Error("shipping.carrier_booking_failed",
			zap.String("shipment_id", shipment.ID.String()),
			zap.Error(err),
		)
		return shipment, nil
	}

	// Persist tracking info and advance status.
	if err := s.repo.UpdateShipmentStatus(ctx, shipment.ID, domain.ShipmentBooked, &result.TrackingNumber, &result.CarrierRef); err != nil {
		s.log.Error("shipping.update_status_failed", zap.Error(err))
	}

	shipment.TrackingNumber = &result.TrackingNumber
	shipment.CarrierRef = &result.CarrierRef
	shipment.Status = domain.ShipmentBooked

	s.log.Info("shipping.shipment_booked",
		zap.String("shipment_id", shipment.ID.String()),
		zap.String("tracking", result.TrackingNumber),
	)
	return shipment, nil
}

// =============================================================================
// Tracking
// =============================================================================

// SyncTracking fetches the latest events from the carrier and appends any new
// ones to the shipment_events log. Called by the Asynq worker.
func (s *ShippingService) SyncTracking(ctx context.Context, shipmentID uuid.UUID) error {
	shipment, err := s.repo.GetShipmentByID(ctx, shipmentID)
	if err != nil {
		return fmt.Errorf("SyncTracking: %w", err)
	}
	if shipment.TrackingNumber == nil || shipment.AccountID == nil {
		return nil // nothing to sync yet
	}

	account, err := s.repo.GetAccountByID(ctx, *shipment.AccountID)
	if err != nil {
		return fmt.Errorf("SyncTracking: %w", err)
	}

	// The provider type is stored in account settings or looked up via the provider registry.
	// We search the registry for the connector whose provider ID matches.
	providerType, _ := account.Settings["provider_type"].(string)
	if providerType == "" {
		// Fall back: check all registered connectors (safe for single-provider setups).
		for pt := range s.registry {
			providerType = pt
			break
		}
	}
	connector, ok := s.registry[providerType]
	if !ok {
		return fmt.Errorf("SyncTracking: no connector for provider type %q", providerType)
	}

	events, err := connector.GetTracking(ctx, account, *shipment.TrackingNumber)
	if err != nil {
		return fmt.Errorf("SyncTracking: %w", err)
	}

	// Determine latest known event to avoid duplicates.
	latestKnown := time.Time{}
	for _, e := range shipment.Events {
		if e.EventTime.After(latestKnown) {
			latestKnown = e.EventTime
		}
	}

	for _, e := range events {
		if !e.EventTime.After(latestKnown) {
			continue
		}
		if err := s.repo.InsertEvent(ctx, &domain.ShipmentEvent{
			ShipmentID:  shipmentID,
			Status:      e.Status,
			Location:    e.Location,
			Description: e.Description,
			EventTime:   e.EventTime,
		}); err != nil {
			s.log.Warn("shipping.insert_event_failed", zap.Error(err))
		}
	}

	// Advance status if the latest event indicates delivery.
	for _, e := range events {
		if e.Status == "DL" || e.Status == "DELIVERED" {
			_ = s.repo.UpdateShipmentStatus(ctx, shipmentID, domain.ShipmentDelivered, nil, nil)
		}
	}

	return nil
}

// GetShipment returns a shipment with tracking events.
func (s *ShippingService) GetShipment(ctx context.Context, id uuid.UUID) (*domain.Shipment, error) {
	return s.repo.GetShipmentByID(ctx, id)
}

// GetShipmentForOrder returns the shipment for an order.
func (s *ShippingService) GetShipmentForOrder(ctx context.Context, orderID uuid.UUID) (*domain.Shipment, error) {
	return s.repo.GetShipmentByOrderID(ctx, orderID)
}

// ListShipments returns paginated shipments with optional filters.
func (s *ShippingService) ListShipments(ctx context.Context, filters domain.ShipmentListFilters) (*domain.PageResponse[domain.ShipmentListItem], error) {
	listRepo, ok := s.repo.(interface {
		ListShipments(ctx context.Context, filters domain.ShipmentListFilters) ([]domain.ShipmentListItem, int, error)
	})
	if !ok {
		return nil, fmt.Errorf("ListShipments: repository does not support list queries")
	}

	filters.Page = normalizePage(filters.Page)
	filters.PageSize = normalizePageSize(filters.PageSize)

	items, total, err := listRepo.ListShipments(ctx, filters)
	if err != nil {
		return nil, fmt.Errorf("ListShipments: %w", err)
	}

	return &domain.PageResponse[domain.ShipmentListItem]{
		Items: items,
		Total: total,
	}, nil
}

// SyncAllTracking is called by the Asynq worker to refresh all in-transit shipments.
func (s *ShippingService) SyncAllTracking(ctx context.Context) error {
	// For now this is a no-op stub; the full implementation would query
	// shipments WHERE status IN ('booked','picked_up','in_transit') and call SyncTracking for each.
	s.log.Info("shipping.sync_all_tracking_triggered")
	return nil
}
