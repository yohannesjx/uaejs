// Package emiratespost is a stub ShippingConnector for Emirates Post.
package emiratespost

import (
	"context"
	"fmt"

	"github.com/dubai-retail/os/internal/domain"
	"github.com/dubai-retail/os/internal/integrations/shipping"
	"go.uber.org/zap"
)

func init() {
	shipping.Register(&Connector{log: zap.NewNop()})
}

// Connector is the Emirates Post stub implementing ShippingConnector.
type Connector struct{ log *zap.Logger }

func (c *Connector) ProviderType() string { return "emiratespost" }

func (c *Connector) CreateShipment(_ context.Context, _ *domain.ShippingAccount, in shipping.CreateShipmentInput) (*shipping.CreateShipmentResult, error) {
	c.log.Info("emiratespost.CreateShipment stub", zap.String("order_id", in.OrderID.String()))
	return nil, fmt.Errorf("emirates post connector not yet configured")
}

func (c *Connector) GetTracking(_ context.Context, _ *domain.ShippingAccount, trackingNumber string) ([]shipping.TrackingEvent, error) {
	c.log.Info("emiratespost.GetTracking stub", zap.String("tracking", trackingNumber))
	return nil, fmt.Errorf("emirates post connector not yet configured")
}

func (c *Connector) CancelShipment(_ context.Context, _ *domain.ShippingAccount, trackingNumber string) error {
	c.log.Info("emiratespost.CancelShipment stub", zap.String("tracking", trackingNumber))
	return fmt.Errorf("emirates post connector not yet configured")
}
