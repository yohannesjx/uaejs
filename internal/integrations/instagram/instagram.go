// Package instagram provides a stub ChannelConnector for Instagram Shopping (Meta Commerce API).
// Full implementation requires Meta Business SDK + Catalog API.
package instagram

import (
	"context"
	"time"

	"github.com/dubai-retail/os/internal/domain"
	"github.com/dubai-retail/os/internal/integrations"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

func init() {
	integrations.Register(domain.PlatformInstagram, New(zap.NewNop()))
}

// Connector is the Meta Commerce / Instagram Shopping stub.
type Connector struct{ log *zap.Logger }

func New(log *zap.Logger) *Connector { return &Connector{log: log} }

func (c *Connector) PlatformName() string { return string(domain.PlatformInstagram) }

func (c *Connector) PublishProduct(_ context.Context, _ *domain.PlatformAccount, v *domain.Variant, price decimal.Decimal, _ string) error {
	sku := ""
	if v.SKU != nil {
		sku = *v.SKU
	}
	c.log.Info("instagram.PublishProduct [stub]", zap.String("sku", sku))
	return nil
}

func (c *Connector) UpdateInventory(_ context.Context, _ *domain.PlatformAccount, extVarID string, qty int) error {
	c.log.Info("instagram.UpdateInventory [stub]", zap.String("ext_var_id", extVarID), zap.Int("qty", qty))
	return nil
}

func (c *Connector) UpdatePrice(_ context.Context, _ *domain.PlatformAccount, extVarID string, price decimal.Decimal, _ string) error {
	c.log.Info("instagram.UpdatePrice [stub]", zap.String("ext_var_id", extVarID), zap.String("price", price.String()))
	return nil
}

// Instagram Shopping does not support direct order polling via a public webhook-free API.
// Orders are pushed via webhooks from Meta; a future handler should accept the webhook
// and translate it into a platform_orders insert.
func (c *Connector) FetchOrders(_ context.Context, _ *domain.PlatformAccount, since time.Time) ([]integrations.ExternalOrder, error) {
	c.log.Info("instagram.FetchOrders [stub – use webhooks instead]", zap.Time("since", since))
	return nil, nil
}
