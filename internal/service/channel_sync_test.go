package service_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/dubai-retail/os/internal/domain"
	"github.com/dubai-retail/os/internal/integrations"
	"github.com/dubai-retail/os/internal/service"
	"github.com/shopspring/decimal"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// =============================================================================
// Fakes
// =============================================================================

type fakeChannelSyncRepo struct {
	platforms []domain.ExternalPlatform
	accounts  map[uuid.UUID]*domain.PlatformAccount
	products  []domain.PlatformProduct
	orders    []domain.PlatformOrder
}

func newFakeChannelSyncRepo() *fakeChannelSyncRepo {
	return &fakeChannelSyncRepo{
		accounts: make(map[uuid.UUID]*domain.PlatformAccount),
	}
}

func (r *fakeChannelSyncRepo) InsertPlatform(_ context.Context, p *domain.ExternalPlatform) error {
	p.ID = uuid.New()
	p.CreatedAt = time.Now()
	r.platforms = append(r.platforms, *p)
	return nil
}
func (r *fakeChannelSyncRepo) ListAllPlatforms(_ context.Context) ([]domain.ExternalPlatform, error) {
	return r.platforms, nil
}
func (r *fakeChannelSyncRepo) ListActivePlatforms(_ context.Context) ([]domain.ExternalPlatform, error) {
	var out []domain.ExternalPlatform
	for _, p := range r.platforms {
		if p.IsActive {
			out = append(out, p)
		}
	}
	return out, nil
}
func (r *fakeChannelSyncRepo) InsertPlatformAccount(_ context.Context, a *domain.PlatformAccount) error {
	a.ID = uuid.New()
	r.accounts[a.ID] = a
	return nil
}
func (r *fakeChannelSyncRepo) GetPlatformAccountByID(_ context.Context, id uuid.UUID) (*domain.PlatformAccount, error) {
	a := r.accounts[id]
	if a == nil {
		return nil, fmt.Errorf("account not found")
	}
	return a, nil
}
func (r *fakeChannelSyncRepo) GetActiveAccountsByPlatformType(_ context.Context, _ domain.PlatformType) ([]domain.PlatformAccount, error) {
	var out []domain.PlatformAccount
	for _, a := range r.accounts {
		if a.IsActive {
			out = append(out, *a)
		}
	}
	return out, nil
}
func (r *fakeChannelSyncRepo) ListAllActiveAccounts(_ context.Context) ([]domain.PlatformAccount, error) {
	var out []domain.PlatformAccount
	for _, a := range r.accounts {
		if a.IsActive {
			out = append(out, *a)
		}
	}
	return out, nil
}
func (r *fakeChannelSyncRepo) UpsertPlatformProduct(_ context.Context, pp *domain.PlatformProduct) error {
	if pp.ID == uuid.Nil {
		pp.ID = uuid.New()
	}
	r.products = append(r.products, *pp)
	return nil
}
func (r *fakeChannelSyncRepo) GetMappedProducts(_ context.Context, accountID uuid.UUID) ([]domain.PlatformProduct, error) {
	var out []domain.PlatformProduct
	for _, p := range r.products {
		if p.PlatformAccountID == accountID {
			out = append(out, p)
		}
	}
	return out, nil
}
func (r *fakeChannelSyncRepo) GetLocalVariantID(_ context.Context, accountID uuid.UUID, extVarID string) (uuid.UUID, error) {
	for _, p := range r.products {
		if p.PlatformAccountID == accountID && p.ExternalVariantID == extVarID {
			return p.VariantID, nil
		}
	}
	return uuid.Nil, fmt.Errorf("no mapping for ext_var_id %s", extVarID)
}
func (r *fakeChannelSyncRepo) UpsertPlatformOrder(_ context.Context, po *domain.PlatformOrder) error {
	if po.ID == uuid.Nil {
		po.ID = uuid.New()
	}
	r.orders = append(r.orders, *po)
	return nil
}
func (r *fakeChannelSyncRepo) ListPlatformOrders(_ context.Context, status string) ([]domain.PlatformOrder, error) {
	var out []domain.PlatformOrder
	for _, o := range r.orders {
		if status == "" || o.Status == status {
			out = append(out, o)
		}
	}
	return out, nil
}

type fakeInventorySyncQuerier struct {
	stock map[uuid.UUID]int
}

func (f *fakeInventorySyncQuerier) GetAvailableStock(_ context.Context, variantID uuid.UUID) (int, error) {
	return f.stock[variantID], nil
}

// =============================================================================
// Helper
// =============================================================================

func newTestChannelSyncService(connectors map[domain.PlatformType]integrations.ChannelConnector) (*service.ChannelSyncService, *fakeChannelSyncRepo, *fakeInventorySyncQuerier) {
	repo := newFakeChannelSyncRepo()
	inv := &fakeInventorySyncQuerier{stock: make(map[uuid.UUID]int)}
	svc := service.NewChannelSyncService(repo, inv, connectors, zap.NewNop())
	return svc, repo, inv
}

// =============================================================================
// Tests
// =============================================================================

func TestChannelSync_ConnectPlatform(t *testing.T) {
	svc, repo, _ := newTestChannelSyncService(nil)

	account, err := svc.ConnectPlatform(context.Background(), service.ConnectPlatformInput{
		Name:      "My Shopify",
		Type:      domain.PlatformShopify,
		StoreName: "dubai-fashion",
		APIKey:    "key123",
		APISecret: "secret123",
		Settings:  json.RawMessage(`{"shop_domain":"mystore.myshopify.com"}`),
	})
	if err != nil {
		t.Fatalf("ConnectPlatform: %v", err)
	}
	if account.ID == uuid.Nil {
		t.Error("expected non-nil account ID")
	}
	// Credentials must not be returned
	if account.APIKey != "" {
		t.Error("APIKey should be redacted in response")
	}
	if len(repo.platforms) != 1 {
		t.Errorf("expected 1 platform registered, got %d", len(repo.platforms))
	}
	if len(repo.accounts) != 1 {
		t.Errorf("expected 1 account, got %d", len(repo.accounts))
	}
}

func TestChannelSync_ProductMapping(t *testing.T) {
	mock := &integrations.MockConnector{Name: "shopify"}
	svc, repo, _ := newTestChannelSyncService(map[domain.PlatformType]integrations.ChannelConnector{
		domain.PlatformShopify: mock,
	})

	// Register platform and account
	account, _ := svc.ConnectPlatform(context.Background(), service.ConnectPlatformInput{
		Name: "Test", Type: domain.PlatformShopify, APIKey: "k",
	})

	variantID := uuid.New()
	variants := []domain.Variant{{ID: variantID, SKU: "DRESS-S-RED"}}
	prices := map[uuid.UUID]domain.ChannelPrice{
		variantID: {Price: decimal.NewFromFloat(199.00)},
	}

	err := svc.SyncProducts(context.Background(), account.ID, variants, prices)
	if err != nil {
		t.Fatalf("SyncProducts: %v", err)
	}
	if len(mock.Products) != 1 {
		t.Errorf("expected 1 product sent to connector, got %d", len(mock.Products))
	}
	if len(repo.products) == 0 {
		t.Error("expected platform_product mapping to be saved")
	}
}

func TestChannelSync_InventorySyncJob(t *testing.T) {
	variantID := uuid.New()
	mock := &integrations.MockConnector{Name: "shopify"}
	svc, repo, inv := newTestChannelSyncService(map[domain.PlatformType]integrations.ChannelConnector{
		domain.PlatformShopify: mock,
	})
	inv.stock[variantID] = 42

	account, _ := svc.ConnectPlatform(context.Background(), service.ConnectPlatformInput{
		Name: "Test", Type: domain.PlatformShopify, APIKey: "k",
	})

	// Seed a product mapping
	repo.products = append(repo.products, domain.PlatformProduct{
		ID:                uuid.New(),
		PlatformAccountID: account.ID,
		VariantID:         variantID,
		ExternalVariantID: "ext-123",
		SyncStatus:        "synced",
	})

	if err := svc.SyncInventory(context.Background(), account.ID); err != nil {
		t.Fatalf("SyncInventory: %v", err)
	}
	// The mock connector's UpdateInventory is a no-op; just verify no error
}

func TestChannelSync_OrderImportMapping(t *testing.T) {
	extOrderID := "EXT-ORDER-001"
	variantID := uuid.New()

	mock := &integrations.MockConnector{
		Name: "shopify",
		Orders: []integrations.ExternalOrder{
			{
				ExternalOrderID: extOrderID,
				CustomerEmail:   "buyer@example.com",
				Currency:        "AED",
				Lines: []integrations.ExternalLine{
					{ExternalVariantID: "ext-123", Quantity: 2, UnitPrice: decimal.NewFromFloat(99)},
				},
				PlacedAt: time.Now(),
			},
		},
	}

	svc, repo, _ := newTestChannelSyncService(map[domain.PlatformType]integrations.ChannelConnector{
		domain.PlatformShopify: mock,
	})

	account, _ := svc.ConnectPlatform(context.Background(), service.ConnectPlatformInput{
		Name: "Test", Type: domain.PlatformShopify, APIKey: "k",
	})

	// Seed variant mapping so we can resolve external_variant_id → local
	repo.products = append(repo.products, domain.PlatformProduct{
		PlatformAccountID: account.ID,
		VariantID:         variantID,
		ExternalVariantID: "ext-123",
	})

	count, err := svc.ImportOrders(context.Background(), account.ID, time.Now().Add(-1*time.Hour))
	if err != nil {
		t.Fatalf("ImportOrders: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 order imported, got %d", count)
	}
	if len(repo.orders) != 1 {
		t.Errorf("expected 1 platform_order row, got %d", len(repo.orders))
	}
	if repo.orders[0].ExternalOrderID != extOrderID {
		t.Errorf("expected external order ID %s, got %s", extOrderID, repo.orders[0].ExternalOrderID)
	}
	if repo.orders[0].Status != "pending" {
		t.Errorf("expected status=pending, got %s", repo.orders[0].Status)
	}
}

func TestChannelSync_ConnectorInterface(t *testing.T) {
	// Verify that MockConnector satisfies ChannelConnector
	var _ integrations.ChannelConnector = (*integrations.MockConnector)(nil)
	t.Log("connector interface satisfied")
}
