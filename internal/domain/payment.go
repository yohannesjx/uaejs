package domain

import "github.com/shopspring/decimal"

// PaymentGateway identifies a payment processor.
type PaymentGateway string

const (
	GatewayStripe               PaymentGateway = "stripe"
	GatewayNetworkInternational PaymentGateway = "network_international"
	GatewayTabby                PaymentGateway = "tabby"   // UAE BNPL
	GatewayTamara               PaymentGateway = "tamara"  // UAE BNPL
)

// GatewayConfig is the admin-configurable gateway preference loaded from env.
type GatewayConfig struct {
	// Default gateway for all channels unless overridden
	Default  PaymentGateway
	// Fallback used when the primary gateway is unavailable
	Fallback PaymentGateway
	// POS can override per-terminal via ChannelOverrides
	ChannelOverrides map[ChannelType]PaymentGateway
	// WholesaleMinAED: orders above this amount on wholesale route to NI
	// (Network International handles large B2B transactions better)
	WholesaleMinAED decimal.Decimal
}
