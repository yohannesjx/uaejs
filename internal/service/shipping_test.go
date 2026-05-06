package service_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/dubai-retail/os/internal/domain"
	"github.com/dubai-retail/os/internal/integrations/shipping"
	"github.com/dubai-retail/os/internal/service"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// =============================================================================
// Fake repository
// =============================================================================

type fakeShippingRepo struct {
	providers map[string]*domain.ShippingProvider
	accounts  map[uuid.UUID]*domain.ShippingAccount
	shipments map[uuid.UUID]*domain.Shipment
	events    map[uuid.UUID][]domain.ShipmentEvent
}

func newFakeShippingRepo() *fakeShippingRepo {
	providerID := uuid.MustParse("00000000-0000-0000-0000-000000000020")
	return &fakeShippingRepo{
		providers: map[string]*domain.ShippingProvider{
			"aramex": {ID: providerID, Name: "Aramex", Type: "aramex", IsActive: true},
			"dhl":    {ID: uuid.New(), Name: "DHL", Type: "dhl", IsActive: false},
		},
		accounts:  make(map[uuid.UUID]*domain.ShippingAccount),
		shipments: make(map[uuid.UUID]*domain.Shipment),
		events:    make(map[uuid.UUID][]domain.ShipmentEvent),
	}
}

func (r *fakeShippingRepo) ListActiveProviders(_ context.Context) ([]domain.ShippingProvider, error) {
	var out []domain.ShippingProvider
	for _, p := range r.providers {
		if p.IsActive {
			out = append(out, *p)
		}
	}
	return out, nil
}

func (r *fakeShippingRepo) GetProviderByType(_ context.Context, t string) (*domain.ShippingProvider, error) {
	p, ok := r.providers[t]
	if !ok {
		return nil, fmt.Errorf("provider not found: %s", t)
	}
	return p, nil
}

func (r *fakeShippingRepo) InsertAccount(_ context.Context, a *domain.ShippingAccount) error {
	a.ID = uuid.New()
	r.accounts[a.ID] = a
	return nil
}

func (r *fakeShippingRepo) GetActiveAccountForProvider(_ context.Context, providerID uuid.UUID) (*domain.ShippingAccount, error) {
	for _, a := range r.accounts {
		if a.ProviderID == providerID && a.IsActive {
			return a, nil
		}
	}
	return nil, fmt.Errorf("no active account for provider")
}

func (r *fakeShippingRepo) GetAccountByID(_ context.Context, id uuid.UUID) (*domain.ShippingAccount, error) {
	a, ok := r.accounts[id]
	if !ok {
		return nil, fmt.Errorf("account not found")
	}
	return a, nil
}

func (r *fakeShippingRepo) InsertShipment(_ context.Context, s *domain.Shipment) error {
	s.ID = uuid.New()
	copy := *s
	r.shipments[s.ID] = &copy
	return nil
}

func (r *fakeShippingRepo) UpdateShipmentStatus(_ context.Context, id uuid.UUID, status domain.ShipmentStatus, tn, ref *string) error {
	s, ok := r.shipments[id]
	if !ok {
		return fmt.Errorf("shipment not found")
	}
	s.Status = status
	if tn != nil {
		s.TrackingNumber = tn
	}
	if ref != nil {
		s.CarrierRef = ref
	}
	return nil
}

func (r *fakeShippingRepo) GetShipmentByID(_ context.Context, id uuid.UUID) (*domain.Shipment, error) {
	s, ok := r.shipments[id]
	if !ok {
		return nil, fmt.Errorf("shipment not found")
	}
	copy := *s
	copy.Events = r.events[id]
	return &copy, nil
}

func (r *fakeShippingRepo) GetShipmentByOrderID(_ context.Context, orderID uuid.UUID) (*domain.Shipment, error) {
	for _, s := range r.shipments {
		if s.OrderID == orderID {
			return s, nil
		}
	}
	return nil, fmt.Errorf("shipment not found")
}

func (r *fakeShippingRepo) InsertEvent(_ context.Context, e *domain.ShipmentEvent) error {
	e.ID = uuid.New()
	r.events[e.ShipmentID] = append(r.events[e.ShipmentID], *e)
	return nil
}

func (r *fakeShippingRepo) GetEventsByShipmentID(_ context.Context, id uuid.UUID) ([]domain.ShipmentEvent, error) {
	return r.events[id], nil
}

// =============================================================================
// Helper
// =============================================================================

func newTestShippingService(connector shipping.ShippingConnector) (*service.ShippingService, *fakeShippingRepo) {
	repo := newFakeShippingRepo()
	registry := map[string]shipping.ShippingConnector{}
	if connector != nil {
		registry[connector.ProviderType()] = connector
	}
	svc := service.NewShippingService(repo, registry, zap.NewNop())
	return svc, repo
}

// =============================================================================
// Tests
// =============================================================================

func TestShipping_AddAccount_Success(t *testing.T) {
	svc, repo := newTestShippingService(&shipping.MockConnector{ProviderTypeVal: "aramex"})
	ctx := context.Background()

	acc, err := svc.AddAccount(ctx, service.AddAccountInput{
		ProviderType: "aramex",
		Label:        "Production",
		APIKey:       "key123",
		APISecret:    "secret456",
		Settings:     map[string]any{"environment": "sandbox"},
	})
	if err != nil {
		t.Fatalf("AddAccount: %v", err)
	}
	if acc.ID == uuid.Nil {
		t.Error("expected non-nil account ID")
	}
	// Credentials must be scrubbed from the response.
	if acc.APIKey != "" || acc.APISecret != "" {
		t.Error("credentials should be scrubbed from AddAccount response")
	}
	// Verify stored in repo.
	if len(repo.accounts) != 1 {
		t.Errorf("expected 1 account in repo, got %d", len(repo.accounts))
	}
}

func TestShipping_AddAccount_UnknownProvider(t *testing.T) {
	svc, _ := newTestShippingService(nil)
	_, err := svc.AddAccount(context.Background(), service.AddAccountInput{
		ProviderType: "fedex",
		Label:        "Test",
		APIKey:       "k",
		APISecret:    "s",
	})
	if err == nil {
		t.Error("expected error for unknown provider")
	}
}

func TestShipping_CreateShipment_Success(t *testing.T) {
	mock := &shipping.MockConnector{
		ProviderTypeVal: "aramex",
		CreateResult: &shipping.CreateShipmentResult{
			TrackingNumber: "ARX-001",
			CarrierRef:     "REF-001",
		},
	}
	svc, repo := newTestShippingService(mock)
	ctx := context.Background()

	// Pre-seed an active account
	providerID := uuid.MustParse("00000000-0000-0000-0000-000000000020")
	repo.accounts[uuid.New()] = &domain.ShippingAccount{
		ID:         uuid.New(),
		ProviderID: providerID,
		Label:      "Sandbox",
		IsActive:   true,
		Settings:   map[string]any{},
	}

	orderID := uuid.New()
	shipment, err := svc.CreateShipment(ctx, service.CreateShipmentInput{
		OrderID:        orderID,
		ProviderType:   "aramex",
		RecipientName:  "Sarah Al-Mansouri",
		RecipientPhone: "+971501234567",
		Address: domain.ShippingAddress{
			Line1:   "Villa 5, Al Wasl Road",
			City:    "Dubai",
			Country: "AE",
		},
		WeightG:     850,
		Description: "Fashion clothing × 1",
	})
	if err != nil {
		t.Fatalf("CreateShipment: %v", err)
	}
	if shipment.TrackingNumber == nil || *shipment.TrackingNumber != "ARX-001" {
		t.Errorf("expected tracking number ARX-001, got %v", shipment.TrackingNumber)
	}
	if shipment.Status != domain.ShipmentBooked {
		t.Errorf("expected status booked, got %s", shipment.Status)
	}
}

func TestShipping_CreateShipment_NoAccount(t *testing.T) {
	mock := &shipping.MockConnector{ProviderTypeVal: "aramex"}
	svc, _ := newTestShippingService(mock)

	_, err := svc.CreateShipment(context.Background(), service.CreateShipmentInput{
		OrderID:      uuid.New(),
		ProviderType: "aramex",
	})
	if err == nil {
		t.Error("expected error when no active account is configured")
	}
}

func TestShipping_ConnectorInterface_Compliance(t *testing.T) {
	// Verify MockConnector satisfies ShippingConnector at compile time.
	var _ shipping.ShippingConnector = (*shipping.MockConnector)(nil)

	tests := []struct {
		name      string
		connector shipping.ShippingConnector
	}{
		{"mock connector", &shipping.MockConnector{ProviderTypeVal: "test"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.connector.ProviderType() == "" {
				t.Error("ProviderType() must return a non-empty string")
			}
			// CreateShipment, GetTracking, CancelShipment are tested via the mock.
			result, _ := tc.connector.CreateShipment(context.Background(), nil, shipping.CreateShipmentInput{})
			if result == nil {
				t.Error("mock CreateShipment should return non-nil result by default")
			}
		})
	}
}

func TestShipping_OrderShipmentMapping(t *testing.T) {
	mock := &shipping.MockConnector{ProviderTypeVal: "aramex"}
	svc, repo := newTestShippingService(mock)
	ctx := context.Background()

	providerID := uuid.MustParse("00000000-0000-0000-0000-000000000020")
	accID := uuid.New()
	repo.accounts[accID] = &domain.ShippingAccount{
		ID:         accID,
		ProviderID: providerID,
		IsActive:   true,
		Settings:   map[string]any{},
	}

	orderID := uuid.New()
	shipment, _ := svc.CreateShipment(ctx, service.CreateShipmentInput{
		OrderID:      orderID,
		ProviderType: "aramex",
	})

	// Retrieve by order ID
	found, err := svc.GetShipmentForOrder(ctx, orderID)
	if err != nil {
		t.Fatalf("GetShipmentForOrder: %v", err)
	}
	if found.ID != shipment.ID {
		t.Errorf("shipment ID mismatch: got %s want %s", found.ID, shipment.ID)
	}
}
