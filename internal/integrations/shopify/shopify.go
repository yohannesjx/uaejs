// Package shopify implements the ChannelConnector interface for Shopify stores.
// It uses the Shopify REST Admin API v2024-01.
//
// Configuration (stored in platform_accounts.settings JSONB):
//
//	{
//	  "shop_domain": "mystore.myshopify.com",
//	  "api_version": "2024-01",
//	  "location_id": "gid://shopify/Location/12345"
//	}
package shopify

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/dubai-retail/os/internal/domain"
	"github.com/dubai-retail/os/internal/integrations"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

func init() {
	integrations.Register(domain.PlatformShopify, &Connector{
		client: &http.Client{Timeout: 30 * time.Second},
		log:    zap.NewNop(),
	})
}

// Connector implements integrations.ChannelConnector for Shopify.
type Connector struct {
	client *http.Client
	log    *zap.Logger
}

// New creates a Connector with a custom logger.
func New(log *zap.Logger) *Connector {
	return &Connector{
		client: &http.Client{Timeout: 30 * time.Second},
		log:    log,
	}
}

func (c *Connector) PlatformName() string { return string(domain.PlatformShopify) }

// settings extracted from account JSONB
type shopifySettings struct {
	ShopDomain string `json:"shop_domain"`
	APIVersion string `json:"api_version"`
	LocationID string `json:"location_id"`
}

func (c *Connector) settings(account *domain.PlatformAccount) (shopifySettings, error) {
	var s shopifySettings
	if err := json.Unmarshal(account.Settings, &s); err != nil {
		return s, fmt.Errorf("shopify: invalid settings: %w", err)
	}
	if s.APIVersion == "" {
		s.APIVersion = "2024-01"
	}
	return s, nil
}

func (c *Connector) baseURL(account *domain.PlatformAccount) (string, error) {
	cfg, err := c.settings(account)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("https://%s/admin/api/%s", cfg.ShopDomain, cfg.APIVersion), nil
}

func (c *Connector) newRequest(ctx context.Context, method, url string, body interface{}, account *domain.PlatformAccount) (*http.Request, error) {
	var payload *strings.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		payload = strings.NewReader(string(b))
	} else {
		payload = strings.NewReader("")
	}
	req, err := http.NewRequestWithContext(ctx, method, url, payload)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Shopify-Access-Token", account.APIKey)
	return req, nil
}

// PublishProduct creates or updates a product variant on Shopify.
func (c *Connector) PublishProduct(ctx context.Context, account *domain.PlatformAccount, variant *domain.Variant, price decimal.Decimal, currency string) error {
	base, err := c.baseURL(account)
	if err != nil {
		return err
	}
	sku := ""
	if variant.SKU != nil {
		sku = *variant.SKU
	}

	payload := map[string]interface{}{
		"product": map[string]interface{}{
			"title": sku,
			"variants": []map[string]interface{}{
				{
					"sku":   sku,
					"price": price.StringFixed(2),
				},
			},
		},
	}
	req, err := c.newRequest(ctx, http.MethodPost, base+"/products.json", payload, account)
	if err != nil {
		return err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("shopify.PublishProduct: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("shopify.PublishProduct: HTTP %d", resp.StatusCode)
	}
	sku = ""
	if variant.SKU != nil {
		sku = *variant.SKU
	}
	c.log.Info("shopify.product_published", zap.String("sku", sku))
	return nil
}

// UpdateInventory sets the inventory level for a variant at the configured location.
func (c *Connector) UpdateInventory(ctx context.Context, account *domain.PlatformAccount, externalVariantID string, qty int) error {
	base, err := c.baseURL(account)
	if err != nil {
		return err
	}
	cfg, _ := c.settings(account)
	payload := map[string]interface{}{
		"location_id":       cfg.LocationID,
		"inventory_item_id": externalVariantID,
		"available":         qty,
	}
	req, err := c.newRequest(ctx, http.MethodPost, base+"/inventory_levels/set.json", payload, account)
	if err != nil {
		return err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("shopify.UpdateInventory: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("shopify.UpdateInventory: HTTP %d", resp.StatusCode)
	}
	return nil
}

// UpdatePrice updates the price on a Shopify variant.
func (c *Connector) UpdatePrice(ctx context.Context, account *domain.PlatformAccount, externalVariantID string, price decimal.Decimal, currency string) error {
	base, err := c.baseURL(account)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/variants/%s.json", base, externalVariantID)
	payload := map[string]interface{}{
		"variant": map[string]interface{}{
			"id":    externalVariantID,
			"price": price.StringFixed(2),
		},
	}
	req, err := c.newRequest(ctx, http.MethodPut, url, payload, account)
	if err != nil {
		return err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("shopify.UpdatePrice: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("shopify.UpdatePrice: HTTP %d", resp.StatusCode)
	}
	return nil
}

// FetchOrders pulls unfulfilled orders created since the given time.
func (c *Connector) FetchOrders(ctx context.Context, account *domain.PlatformAccount, since time.Time) ([]integrations.ExternalOrder, error) {
	base, err := c.baseURL(account)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/orders.json?status=open&created_at_min=%s&limit=250",
		base, since.UTC().Format(time.RFC3339))
	req, err := c.newRequest(ctx, http.MethodGet, url, nil, account)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("shopify.FetchOrders: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("shopify.FetchOrders: HTTP %d", resp.StatusCode)
	}

	var raw struct {
		Orders []struct {
			ID         int64  `json:"id"`
			Email      string `json:"email"`
			Currency   string `json:"currency"`
			TotalPrice string `json:"total_price"`
			CreatedAt  string `json:"created_at"`
			Customer   struct{ FirstName, LastName string }
			LineItems  []struct {
				VariantID int64  `json:"variant_id"`
				Quantity  int    `json:"quantity"`
				Price     string `json:"price"`
			} `json:"line_items"`
		} `json:"orders"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("shopify.FetchOrders: decode: %w", err)
	}

	var out []integrations.ExternalOrder
	for _, o := range raw.Orders {
		total, _ := decimal.NewFromString(o.TotalPrice)
		placed, _ := time.Parse(time.RFC3339, o.CreatedAt)
		ext := integrations.ExternalOrder{
			ExternalOrderID: fmt.Sprintf("%d", o.ID),
			CustomerEmail:   o.Email,
			CustomerName:    o.Customer.FirstName + " " + o.Customer.LastName,
			Currency:        o.Currency,
			TotalAmount:     total,
			PlacedAt:        placed,
		}
		for _, li := range o.LineItems {
			lp, _ := decimal.NewFromString(li.Price)
			ext.Lines = append(ext.Lines, integrations.ExternalLine{
				ExternalVariantID: fmt.Sprintf("%d", li.VariantID),
				Quantity:          li.Quantity,
				UnitPrice:         lp,
			})
		}
		out = append(out, ext)
	}
	return out, nil
}
