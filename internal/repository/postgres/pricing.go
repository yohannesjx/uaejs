package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/dubai-retail/os/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PricingRepository handles promotions and channel lookup queries
// that sit between the raw price tables and the PriceResolver service.
type PricingRepository struct {
	pool *pgxpool.Pool
}

// =============================================================================
// Channel lookup (needed by ComplianceService to determine invoice trigger)
// =============================================================================

// GetChannelByID returns a channel by its primary key.
func (r *PricingRepository) GetChannelByID(ctx context.Context, id uuid.UUID) (*domain.Channel, error) {
	const q = `
		SELECT id, name, type, is_active, description, created_at, updated_at
		  FROM channels
		 WHERE id = $1`

	ch := &domain.Channel{}
	err := r.pool.QueryRow(ctx, q, id).Scan(
		&ch.ID, &ch.Name, &ch.Type, &ch.IsActive,
		&ch.Description, &ch.CreatedAt, &ch.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("GetChannelByID(%s): %w", id, err)
	}
	return ch, nil
}

// =============================================================================
// Promotion queries
// =============================================================================

// GetActivePromotion returns the best active promotion for a variant + channel,
// respecting customer tier priority:
//   1. Tier-specific promotion (exact match on customer_tier)
//   2. Channel-wide promotion (customer_tier IS NULL)
//
// Returns nil, nil when no promotion is active.
func (r *PricingRepository) GetActivePromotion(
	ctx context.Context,
	variantID, channelID uuid.UUID,
	tier *domain.CustomerTier,
	now time.Time,
) (*domain.PricePromotion, error) {
	// The query returns the best applicable promotion.
	// Tier-specific rows are ranked first (NULL tier ranked last).
	const q = `
		SELECT id, variant_id, channel_id, customer_tier,
		       promo_price, currency, effective_from, effective_until,
		       is_active, created_at, updated_at
		  FROM price_promotions
		 WHERE variant_id      = $1
		   AND channel_id      = $2
		   AND is_active       = TRUE
		   AND effective_from  <= $3
		   AND effective_until > $3
		   AND (customer_tier  = $4::customer_tier OR customer_tier IS NULL)
		 ORDER BY
		   -- Prefer exact tier match; fall back to universal (NULL tier) promotion.
		   (customer_tier IS NULL) ASC,
		   -- Among equally ranked promos, pick the lowest price (best for customer).
		   promo_price ASC
		 LIMIT 1`

	p := &domain.PricePromotion{}
	err := r.pool.QueryRow(ctx, q, variantID, channelID, now, tier).Scan(
		&p.ID, &p.VariantID, &p.ChannelID, &p.CustomerTier,
		&p.PromoPrice, &p.Currency, &p.EffectiveFrom, &p.EffectiveUntil,
		&p.IsActive, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		// pgx.ErrNoRows → no active promotion; return (nil, nil)
		if isNoRows(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("GetActivePromotion(variant=%s, channel=%s): %w",
			variantID, channelID, err)
	}
	return p, nil
}

// GetChannelPrice fetches the standard (non-promotional) active price for a
// variant on a channel. Used as the fallback inside PriceResolver.
// Delegates to the same SQL used by ProductRepository.GetChannelPrice to avoid
// duplication while keeping PricingRepository self-contained.
func (r *PricingRepository) GetChannelPrice(ctx context.Context, variantID, channelID uuid.UUID) (*domain.ChannelPrice, error) {
	const q = `
		SELECT id, variant_id, channel_id, price, currency, is_active,
		       effective_from, effective_until, created_at, updated_at
		  FROM channel_prices
		 WHERE variant_id = $1
		   AND channel_id = $2
		   AND is_active  = TRUE
		   AND (effective_until IS NULL OR effective_until > NOW() AT TIME ZONE 'UTC')`

	cp := &domain.ChannelPrice{}
	err := r.pool.QueryRow(ctx, q, variantID, channelID).Scan(
		&cp.ID, &cp.VariantID, &cp.ChannelID, &cp.Price, &cp.Currency, &cp.IsActive,
		&cp.EffectiveFrom, &cp.EffectiveUntil, &cp.CreatedAt, &cp.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("PricingRepository.GetChannelPrice(variant=%s, channel=%s): %w",
			variantID, channelID, err)
	}
	return cp, nil
}

// InsertPromotion creates a new promotion record.
func (r *PricingRepository) InsertPromotion(ctx context.Context, p *domain.PricePromotion) error {
	const q = `
		INSERT INTO price_promotions
		    (id, variant_id, channel_id, customer_tier, promo_price, currency,
		     effective_from, effective_until, is_active, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,
		        NOW() AT TIME ZONE 'UTC', NOW() AT TIME ZONE 'UTC')`

	_, err := r.pool.Exec(ctx, q,
		p.ID, p.VariantID, p.ChannelID, p.CustomerTier, p.PromoPrice, p.Currency,
		p.EffectiveFrom, p.EffectiveUntil, p.IsActive,
	)
	if err != nil {
		return fmt.Errorf("InsertPromotion(%s): %w", p.ID, err)
	}
	return nil
}

// =============================================================================
// Helper: detect no-rows error without importing pgx at domain boundary
// =============================================================================

func isNoRows(err error) bool {
	return err != nil && err.Error() == "no rows in result set"
}
