// Package service: Omnichannel Sync
//
// ChannelSyncService orchestrates product, inventory, price, and order syncing
// between the local inventory system and external marketplace connectors.
//
// The module is entirely optional: if no platform accounts are active, all
// sync methods return immediately with no side-effects.
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/dubai-retail/os/internal/domain"
	"github.com/dubai-retail/os/internal/integrations"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// =============================================================================
// Repository interface
// =============================================================================

// ChannelSyncRepo is the DB interface required by ChannelSyncService.
type ChannelSyncRepo interface {
	InsertPlatform(ctx context.Context, p *domain.ExternalPlatform) error
	ListAllPlatforms(ctx context.Context) ([]domain.ExternalPlatform, error)
	ListActivePlatforms(ctx context.Context) ([]domain.ExternalPlatform, error)

	InsertPlatformAccount(ctx context.Context, a *domain.PlatformAccount) error
	GetPlatformAccountByID(ctx context.Context, id uuid.UUID) (*domain.PlatformAccount, error)
	GetActiveAccountsByPlatformType(ctx context.Context, t domain.PlatformType) ([]domain.PlatformAccount, error)
	ListAllActiveAccounts(ctx context.Context) ([]domain.PlatformAccount, error)

	UpsertPlatformProduct(ctx context.Context, pp *domain.PlatformProduct) error
	GetMappedProducts(ctx context.Context, accountID uuid.UUID) ([]domain.PlatformProduct, error)
	GetLocalVariantID(ctx context.Context, accountID uuid.UUID, externalVariantID string) (uuid.UUID, error)

	UpsertPlatformOrder(ctx context.Context, po *domain.PlatformOrder) error
	ListPlatformOrders(ctx context.Context, status string) ([]domain.PlatformOrder, error)
}

// InventorySyncQuerier is the subset needed to get current stock levels.
type InventorySyncQuerier interface {
	GetAvailableStock(ctx context.Context, variantID uuid.UUID) (int, error)
}

// =============================================================================
// Service
// =============================================================================

// ChannelSyncService orchestrates all omnichannel operations.
type ChannelSyncService struct {
	repo      ChannelSyncRepo
	inventory InventorySyncQuerier
	registry  map[domain.PlatformType]integrations.ChannelConnector
	log       *zap.Logger
}

// NewChannelSyncService creates a ChannelSyncService.
// Pass integrations.Registry as the connector registry for production use.
func NewChannelSyncService(
	repo ChannelSyncRepo,
	inventory InventorySyncQuerier,
	registry map[domain.PlatformType]integrations.ChannelConnector,
	log *zap.Logger,
) *ChannelSyncService {
	return &ChannelSyncService{
		repo:      repo,
		inventory: inventory,
		registry:  registry,
		log:       log,
	}
}

// =============================================================================
// Platform account management
// =============================================================================

// ConnectPlatformInput is the DTO for registering a new platform account.
type ConnectPlatformInput struct {
	Name      string              `json:"name"`
	Type      domain.PlatformType `json:"type"`
	StoreName string              `json:"store_name"`
	APIKey    string              `json:"api_key"`
	APISecret string              `json:"api_secret"`
	Settings  json.RawMessage     `json:"settings"`
}

// ConnectPlatform registers and enables a new platform account.
func (s *ChannelSyncService) ConnectPlatform(ctx context.Context, in ConnectPlatformInput) (*domain.PlatformAccount, error) {
	platform := &domain.ExternalPlatform{
		Name:     in.Name,
		Type:     in.Type,
		IsActive: true,
	}
	if err := s.repo.InsertPlatform(ctx, platform); err != nil {
		return nil, fmt.Errorf("ConnectPlatform: insert platform: %w", err)
	}

	account := &domain.PlatformAccount{
		PlatformID: platform.ID,
		StoreName:  in.StoreName,
		APIKey:     in.APIKey,
		APISecret:  in.APISecret,
		Settings:   in.Settings,
		IsActive:   true,
	}
	if in.Settings == nil {
		account.Settings = json.RawMessage("{}")
	}
	if err := s.repo.InsertPlatformAccount(ctx, account); err != nil {
		return nil, fmt.Errorf("ConnectPlatform: insert account: %w", err)
	}

	s.log.Info("channel.connected",
		zap.String("platform_type", string(in.Type)),
		zap.String("account_id", account.ID.String()),
	)
	// Never return credentials in response
	account.APIKey = ""
	account.APISecret = ""
	return account, nil
}

func (s *ChannelSyncService) ListPlatforms(ctx context.Context) ([]domain.ExternalPlatform, error) {
	return s.repo.ListAllPlatforms(ctx)
}

func (s *ChannelSyncService) ListPlatformOrders(ctx context.Context, status string) ([]domain.PlatformOrder, error) {
	return s.repo.ListPlatformOrders(ctx, status)
}

// =============================================================================
// Product sync
// =============================================================================

// SyncProductsInput carries the data needed to publish variants to a platform.
type SyncProductsInput struct {
	AccountID uuid.UUID `json:"account_id"`
	Variants  []domain.Variant
	Prices    map[uuid.UUID]domain.ChannelPrice // variantID → price
}

// SyncProducts publishes or updates all given variants on the target platform account.
// Already-mapped products are updated; new ones are created.
func (s *ChannelSyncService) SyncProducts(ctx context.Context, accountID uuid.UUID, variants []domain.Variant, prices map[uuid.UUID]domain.ChannelPrice) error {
	account, err := s.repo.GetPlatformAccountByID(ctx, accountID)
	if err != nil {
		return err
	}
	platform, connector := s.connectorForAccount(ctx, account)
	if connector == nil {
		s.log.Warn("channel.sync_products.no_connector", zap.String("account_id", accountID.String()))
		return nil
	}

	for _, v := range variants {
		price := prices[v.ID]
		sku := ""
		if v.SKU != nil {
			sku = *v.SKU
		}
		if err := connector.PublishProduct(ctx, account, &v, price.Price, "AED"); err != nil {
			s.log.Error("channel.sync_products.error",
				zap.String("platform", platform),
				zap.String("sku", sku),
				zap.Error(err),
			)
			_ = s.repo.UpsertPlatformProduct(ctx, &domain.PlatformProduct{
				PlatformAccountID: accountID,
				VariantID:         v.ID,
				SyncStatus:        "error",
				SyncError:         err.Error(),
			})
			continue
		}
		_ = s.repo.UpsertPlatformProduct(ctx, &domain.PlatformProduct{
			PlatformAccountID: accountID,
			VariantID:         v.ID,
			SyncStatus:        "synced",
		})
	}
	return nil
}

// =============================================================================
// Inventory sync
// =============================================================================

// SyncInventory pushes current stock levels for all mapped products to a platform.
func (s *ChannelSyncService) SyncInventory(ctx context.Context, accountID uuid.UUID) error {
	account, err := s.repo.GetPlatformAccountByID(ctx, accountID)
	if err != nil {
		return err
	}
	_, connector := s.connectorForAccount(ctx, account)
	if connector == nil {
		return nil
	}

	mappings, err := s.repo.GetMappedProducts(ctx, accountID)
	if err != nil {
		return fmt.Errorf("SyncInventory: get mappings: %w", err)
	}

	for _, m := range mappings {
		qty, err := s.inventory.GetAvailableStock(ctx, m.VariantID)
		if err != nil {
			s.log.Error("channel.sync_inventory.get_stock_error",
				zap.String("variant_id", m.VariantID.String()), zap.Error(err))
			continue
		}
		if err := connector.UpdateInventory(ctx, account, m.ExternalVariantID, qty); err != nil {
			s.log.Error("channel.sync_inventory.push_error",
				zap.String("variant_id", m.VariantID.String()), zap.Error(err))
		}
	}
	s.log.Info("channel.inventory_synced",
		zap.String("account_id", accountID.String()),
		zap.Int("variants_synced", len(mappings)),
	)
	return nil
}

// =============================================================================
// Order import
// =============================================================================

// ImportOrders pulls new orders from a platform account and stores them
// in platform_orders. Local order creation (reserve + process) is enqueued
// as an Asynq job so the worker can handle failures independently.
func (s *ChannelSyncService) ImportOrders(ctx context.Context, accountID uuid.UUID, since time.Time) (int, error) {
	account, err := s.repo.GetPlatformAccountByID(ctx, accountID)
	if err != nil {
		return 0, err
	}
	_, connector := s.connectorForAccount(ctx, account)
	if connector == nil {
		return 0, nil
	}

	orders, err := connector.FetchOrders(ctx, account, since)
	if err != nil {
		return 0, fmt.Errorf("ImportOrders: fetch: %w", err)
	}

	imported := 0
	for _, ext := range orders {
		raw, _ := json.Marshal(ext)
		po := &domain.PlatformOrder{
			PlatformAccountID: accountID,
			ExternalOrderID:   ext.ExternalOrderID,
			Status:            "pending",
			RawPayload:        raw,
		}
		if err := s.repo.UpsertPlatformOrder(ctx, po); err != nil {
			s.log.Error("channel.import_order.upsert_error",
				zap.String("external_order_id", ext.ExternalOrderID), zap.Error(err))
			continue
		}
		imported++
	}

	s.log.Info("channel.orders_imported",
		zap.String("account_id", accountID.String()),
		zap.Int("count", imported),
	)
	return imported, nil
}

// SyncAllInventory triggers an inventory sync for every active platform account.
// Called by the Asynq background worker.
func (s *ChannelSyncService) SyncAllInventory(ctx context.Context) error {
	accounts, err := s.repo.ListAllActiveAccounts(ctx)
	if err != nil {
		return err
	}
	for _, a := range accounts {
		if syncErr := s.SyncInventory(ctx, a.ID); syncErr != nil {
			s.log.Error("channel.sync_all_inventory.error",
				zap.String("account_id", a.ID.String()),
				zap.Error(syncErr),
			)
		}
	}
	return nil
}

// ImportAllOrders pulls orders from every active account.
func (s *ChannelSyncService) ImportAllOrders(ctx context.Context, since time.Time) error {
	accounts, err := s.repo.ListAllActiveAccounts(ctx)
	if err != nil {
		return err
	}
	for _, a := range accounts {
		if _, importErr := s.ImportOrders(ctx, a.ID, since); importErr != nil {
			s.log.Error("channel.import_all_orders.error",
				zap.String("account_id", a.ID.String()),
				zap.Error(importErr),
			)
		}
	}
	return nil
}

// =============================================================================
// Internal helper
// =============================================================================

func (s *ChannelSyncService) connectorForAccount(ctx context.Context, account *domain.PlatformAccount) (string, integrations.ChannelConnector) {
	// Fetch platform type from the associated platform record
	platforms, err := s.repo.ListActivePlatforms(ctx)
	if err != nil {
		return "", nil
	}
	for _, p := range platforms {
		if p.ID == account.PlatformID {
			connector := s.registry[p.Type]
			return string(p.Type), connector
		}
	}
	return "", nil
}
