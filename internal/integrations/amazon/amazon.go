// Package amazon provides a stub ChannelConnector for Amazon Seller Central (SP-API).
// Full SP-API OAuth + signing requires the amazon-sp-api SDK.
// This stub satisfies the interface and logs calls; replace with real HTTP calls when ready.
package amazon

import (
	"context"
	"time"

	"github.com/dubai-retail/os/internal/domain"
	"github.com/dubai-retail/os/internal/integrations"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

func init() {
	integrations.Register(domain.PlatformAmazon, New(zap.NewNop()))
}

// Connector is the Amazon SP-API adapter stub.
type Connector struct{ log *zap.Logger }

// New creates an Amazon Connector.
func New(log *zap.Logger) *Connector { return &Connector{log: log} }

func (c *Connector) PlatformName() string { return string(domain.PlatformAmazon) }

func (c *Connector) PublishProduct(_ context.Context, acc *domain.PlatformAccount, v *domain.Variant, price decimal.Decimal, currency string) error {
	pkgSKU := ""
	if v.SKU != nil {
		pkgSKU = *v.SKU
	}
	c.log.Info("Syncing to Amazon", zap.String("sku", pkgSKU), zap.String("price", price.String()))
	return nil
}

func (c *Connector) UpdateInventory(_ context.Context, acc *domain.PlatformAccount, extVarID string, qty int) error {
	c.log.Info("amazon.UpdateInventory [stub]", zap.String("ext_var_id", extVarID), zap.Int("qty", qty))
	return nil
}

func (c *Connector) UpdatePrice(_ context.Context, acc *domain.PlatformAccount, extVarID string, price decimal.Decimal, currency string) error {
	c.log.Info("amazon.UpdatePrice [stub]", zap.String("ext_var_id", extVarID), zap.String("price", price.String()))
	return nil
}

func (c *Connector) FetchOrders(_ context.Context, acc *domain.PlatformAccount, since time.Time) ([]integrations.ExternalOrder, error) {
	c.log.Info("amazon.FetchOrders [stub]", zap.Time("since", since))
	return nil, nil
}
