// Package noon provides a stub ChannelConnector for Noon.com Seller API.
package noon

import (
	"context"
	"time"

	"github.com/dubai-retail/os/internal/domain"
	"github.com/dubai-retail/os/internal/integrations"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

func init() {
	integrations.Register(domain.PlatformNoon, New(zap.NewNop()))
}

// Connector is the Noon.com stub adapter.
type Connector struct{ log *zap.Logger }

func New(log *zap.Logger) *Connector { return &Connector{log: log} }

func (c *Connector) PlatformName() string { return string(domain.PlatformNoon) }

func (c *Connector) PublishProduct(_ context.Context, _ *domain.PlatformAccount, v *domain.Variant, price decimal.Decimal, _ string) error {
	sku := ""
	if v.SKU != nil {
		sku = *v.SKU
	}
	c.log.Info("noon.PublishProduct [stub]", zap.String("sku", sku))
	return nil
}

func (c *Connector) UpdateInventory(_ context.Context, _ *domain.PlatformAccount, extVarID string, qty int) error {
	c.log.Info("noon.UpdateInventory [stub]", zap.String("ext_var_id", extVarID), zap.Int("qty", qty))
	return nil
}

func (c *Connector) UpdatePrice(_ context.Context, _ *domain.PlatformAccount, extVarID string, price decimal.Decimal, _ string) error {
	c.log.Info("noon.UpdatePrice [stub]", zap.String("ext_var_id", extVarID), zap.String("price", price.String()))
	return nil
}

func (c *Connector) FetchOrders(_ context.Context, _ *domain.PlatformAccount, since time.Time) ([]integrations.ExternalOrder, error) {
	c.log.Info("noon.FetchOrders [stub]", zap.Time("since", since))
	return nil, nil
}
