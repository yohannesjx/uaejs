package service

import (
	"github.com/dubai-retail/os/internal/domain"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// GatewaySelector determines which payment gateway to use for a given order.
//
// Selection rules (in priority order):
//  1. If the channel has an explicit override in GatewayConfig.ChannelOverrides → use it.
//  2. Wholesale orders above WholesaleMinAED → route to Network International.
//  3. Fall through to the configured default gateway.
//
// The selected gateway is advisory: the caller decides whether to initiate a
// payment session; GatewaySelector never makes network calls itself.
type GatewaySelector struct {
	cfg domain.GatewayConfig
	log *zap.Logger
}

// NewGatewaySelector creates a GatewaySelector with the provided config.
// If cfg.Default is empty it defaults to stripe.
func NewGatewaySelector(cfg domain.GatewayConfig, log *zap.Logger) *GatewaySelector {
	if cfg.Default == "" {
		cfg.Default = domain.GatewayStripe
	}
	if cfg.Fallback == "" {
		cfg.Fallback = domain.GatewayNetworkInternational
	}
	if cfg.WholesaleMinAED.IsZero() {
		// Default: wholesale orders >= 5,000 AED route to Network International
		cfg.WholesaleMinAED = decimal.NewFromInt(5000)
	}
	return &GatewaySelector{cfg: cfg, log: log}
}

// SelectRequest carries the contextual information needed to pick a gateway.
type SelectRequest struct {
	ChannelType       domain.ChannelType
	OrderTotalAED     decimal.Decimal
	PreferredGateway  *domain.PaymentGateway // optional: client-supplied preference
}

// SelectResult is returned by Select.
type SelectResult struct {
	Gateway  domain.PaymentGateway
	Reason   string
	Fallback domain.PaymentGateway
}

// Select applies the gateway selection rules and returns the chosen gateway
// plus the fallback to use if that gateway is unavailable.
func (s *GatewaySelector) Select(req SelectRequest) SelectResult {
	// Rule 1: Honour client-supplied preference if configured as a valid gateway
	if req.PreferredGateway != nil && s.isKnown(*req.PreferredGateway) {
		s.log.Debug("gateway.client_preference",
			zap.String("gateway", string(*req.PreferredGateway)),
			zap.String("channel", string(req.ChannelType)),
		)
		return SelectResult{
			Gateway:  *req.PreferredGateway,
			Reason:   "client_preferred",
			Fallback: s.cfg.Fallback,
		}
	}

	// Rule 2: Channel-level override (e.g. POS terminals may be configured for NI)
	if override, ok := s.cfg.ChannelOverrides[req.ChannelType]; ok {
		s.log.Debug("gateway.channel_override",
			zap.String("gateway", string(override)),
			zap.String("channel", string(req.ChannelType)),
		)
		return SelectResult{
			Gateway:  override,
			Reason:   "channel_override",
			Fallback: s.cfg.Fallback,
		}
	}

	// Rule 3: Large wholesale orders → Network International
	if req.ChannelType == domain.ChannelTypeWholesale &&
		req.OrderTotalAED.GreaterThanOrEqual(s.cfg.WholesaleMinAED) {
		s.log.Debug("gateway.wholesale_large_order",
			zap.String("gateway", string(domain.GatewayNetworkInternational)),
			zap.String("total_aed", req.OrderTotalAED.String()),
		)
		return SelectResult{
			Gateway:  domain.GatewayNetworkInternational,
			Reason:   "wholesale_large_order",
			Fallback: s.cfg.Default,
		}
	}

	// Rule 4: Default
	s.log.Debug("gateway.default_selected",
		zap.String("gateway", string(s.cfg.Default)),
	)
	return SelectResult{
		Gateway:  s.cfg.Default,
		Reason:   "default",
		Fallback: s.cfg.Fallback,
	}
}

func (s *GatewaySelector) isKnown(g domain.PaymentGateway) bool {
	switch g {
	case domain.GatewayStripe,
		domain.GatewayNetworkInternational,
		domain.GatewayTabby,
		domain.GatewayTamara:
		return true
	}
	return false
}
