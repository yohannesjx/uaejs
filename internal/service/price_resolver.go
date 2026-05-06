package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/dubai-retail/os/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// =============================================================================
// Repository interface
// =============================================================================

// PricingRepo is the minimal read interface required by PriceResolver.
type PricingRepo interface {
	GetActivePromotion(
		ctx context.Context,
		variantID, channelID uuid.UUID,
		tier *domain.CustomerTier,
		now time.Time,
	) (*domain.PricePromotion, error)

	GetChannelPrice(ctx context.Context, variantID, channelID uuid.UUID) (*domain.ChannelPrice, error)
	GetChannelByID(ctx context.Context, id uuid.UUID) (*domain.Channel, error)
}

// =============================================================================
// DTOs
// =============================================================================

// PriceResolveRequest is the input to PriceResolver.Resolve.
type PriceResolveRequest struct {
	VariantID    uuid.UUID
	ChannelID    uuid.UUID
	CustomerTier *domain.CustomerTier // nil = no specific tier
	// ExchangeRateToAED is required when the channel's base currency is not AED.
	// Defaults to 1 (AED orders).
	ExchangeRateToAED decimal.Decimal
}

// =============================================================================
// PriceResolver
// =============================================================================

// PriceResolver resolves the effective selling price for a variant on a channel.
//
// Resolution order:
//  1. Active tier-specific promotion (if CustomerTier is supplied and matches)
//  2. Active channel-wide promotion  (customer_tier IS NULL in DB)
//  3. Standard channel price
//
// VAT is always computed and stored in AED regardless of order currency
// (required by UAE PINT-AE BT-111 dual-currency compliance).
type PriceResolver struct {
	repo    PricingRepo
	vatRate decimal.Decimal // standard UAE VAT: 0.05
	log     *zap.Logger
}

func NewPriceResolver(repo PricingRepo, vatRate decimal.Decimal, log *zap.Logger) *PriceResolver {
	return &PriceResolver{repo: repo, vatRate: vatRate, log: log}
}

// Resolve returns the effective PriceResult for a given variant + channel request.
func (r *PriceResolver) Resolve(ctx context.Context, req PriceResolveRequest) (*domain.PriceResult, error) {
	now := time.Now().UTC()

	if req.ExchangeRateToAED.IsZero() {
		req.ExchangeRateToAED = decimal.NewFromInt(1)
	}

	// -------------------------------------------------------------------------
	// Step 1: Check for an active promotion.
	// -------------------------------------------------------------------------
	promo, err := r.repo.GetActivePromotion(ctx, req.VariantID, req.ChannelID, req.CustomerTier, now)
	if err != nil {
		return nil, fmt.Errorf("PriceResolver: promotion lookup: %w", err)
	}

	if promo != nil {
		result := r.buildResult(
			req.VariantID, req.ChannelID,
			promo.PromoPrice, promo.Currency,
			r.vatRate,
			req.ExchangeRateToAED,
			domain.PriceSourcePromotion,
			&promo.ID,
			req.CustomerTier,
		)

		r.log.Debug("price resolved via promotion",
			zap.String("variant_id", req.VariantID.String()),
			zap.String("promo_id", promo.ID.String()),
			zap.String("net_price", result.NetPrice.String()),
		)
		return result, nil
	}

	// -------------------------------------------------------------------------
	// Step 2: Fall back to standard channel price.
	// -------------------------------------------------------------------------
	cp, err := r.repo.GetChannelPrice(ctx, req.VariantID, req.ChannelID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("no active price for variant %s on channel %s",
				req.VariantID, req.ChannelID)
		}
		return nil, fmt.Errorf("PriceResolver: channel price lookup: %w", err)
	}

	result := r.buildResult(
		req.VariantID, req.ChannelID,
		cp.Price, cp.Currency,
		r.vatRate,
		req.ExchangeRateToAED,
		domain.PriceSourceStandard,
		nil,
		nil,
	)

	r.log.Debug("price resolved via standard rate",
		zap.String("variant_id", req.VariantID.String()),
		zap.String("net_price", result.NetPrice.String()),
	)
	return result, nil
}

// =============================================================================
// ZeroRatedResolve resolves price with 0% VAT for export orders.
// =============================================================================

// ResolveZeroRated is identical to Resolve but forces VAT = 0 on the result.
// Use this for orders where VATType is zero_rated or exempt.
func (r *PriceResolver) ResolveZeroRated(ctx context.Context, req PriceResolveRequest) (*domain.PriceResult, error) {
	res, err := r.Resolve(ctx, req)
	if err != nil {
		return nil, err
	}
	// Override to zero-rated
	res.VATRate = decimal.Zero
	res.VATAmount = decimal.Zero
	res.VATAmountAED = decimal.Zero
	res.GrossPrice = res.NetPrice
	return res, nil
}

// =============================================================================
// Builder helper
// =============================================================================

func (r *PriceResolver) buildResult(
	variantID, channelID uuid.UUID,
	netPrice decimal.Decimal,
	currency string,
	vatRate decimal.Decimal,
	exchangeRateToAED decimal.Decimal,
	source domain.PriceSource,
	promoID *uuid.UUID,
	tier *domain.CustomerTier,
) *domain.PriceResult {
	vatAmount := netPrice.Mul(vatRate).Round(2)
	grossPrice := netPrice.Add(vatAmount)

	// VAT in AED: multiply by exchange rate when currency ≠ AED.
	vatAmountAED := vatAmount
	if currency != "AED" && !exchangeRateToAED.IsZero() {
		vatAmountAED = vatAmount.Mul(exchangeRateToAED).Round(2)
	}

	return &domain.PriceResult{
		VariantID:    variantID,
		ChannelID:    channelID,
		NetPrice:     netPrice,
		VATRate:      vatRate,
		VATAmount:    vatAmount,
		GrossPrice:   grossPrice,
		VATAmountAED: vatAmountAED,
		Currency:     currency,
		PriceSource:  source,
		PromotionID:  promoID,
		AppliedTier:  tier,
	}
}
