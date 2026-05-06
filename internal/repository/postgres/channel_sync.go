package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/dubai-retail/os/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ChannelSyncRepository handles all platform/omnichannel DB operations.
type ChannelSyncRepository struct {
	pool *pgxpool.Pool
}

// ── Platforms ─────────────────────────────────────────────────────────────────

func (r *ChannelSyncRepository) InsertPlatform(ctx context.Context, p *domain.ExternalPlatform) error {
	p.ID = uuid.New()
	p.CreatedAt = time.Now().UTC()
	_, err := r.pool.Exec(ctx, `
		INSERT INTO external_platforms (id, name, type, is_active, created_at)
		VALUES ($1,$2,$3,$4,$5)`,
		p.ID, p.Name, p.Type, p.IsActive, p.CreatedAt)
	return err
}

func (r *ChannelSyncRepository) ListActivePlatforms(ctx context.Context) ([]domain.ExternalPlatform, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, name, type, is_active, created_at
		  FROM external_platforms WHERE is_active=TRUE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.ExternalPlatform
	for rows.Next() {
		var p domain.ExternalPlatform
		if err := rows.Scan(&p.ID, &p.Name, &p.Type, &p.IsActive, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *ChannelSyncRepository) ListAllPlatforms(ctx context.Context) ([]domain.ExternalPlatform, error) {
	rows, err := r.pool.Query(ctx, `SELECT id, name, type, is_active, created_at FROM external_platforms ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.ExternalPlatform
	for rows.Next() {
		var p domain.ExternalPlatform
		if err := rows.Scan(&p.ID, &p.Name, &p.Type, &p.IsActive, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ── Platform Accounts ─────────────────────────────────────────────────────────

func (r *ChannelSyncRepository) InsertPlatformAccount(ctx context.Context, a *domain.PlatformAccount) error {
	a.ID = uuid.New()
	now := time.Now().UTC()
	a.CreatedAt = now
	a.UpdatedAt = now
	if a.Settings == nil {
		a.Settings = json.RawMessage("{}")
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO platform_accounts (id, platform_id, store_name, api_key, api_secret, settings, is_active, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$8)`,
		a.ID, a.PlatformID, a.StoreName, a.APIKey, a.APISecret, a.Settings, a.IsActive, now)
	return err
}

func (r *ChannelSyncRepository) GetPlatformAccountByID(ctx context.Context, id uuid.UUID) (*domain.PlatformAccount, error) {
	var a domain.PlatformAccount
	err := r.pool.QueryRow(ctx, `
		SELECT id, platform_id, store_name, api_key, api_secret, settings, is_active, created_at, updated_at
		  FROM platform_accounts WHERE id=$1`, id,
	).Scan(&a.ID, &a.PlatformID, &a.StoreName, &a.APIKey, &a.APISecret, &a.Settings, &a.IsActive, &a.CreatedAt, &a.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("platform account not found: %s", id)
	}
	return &a, err
}

func (r *ChannelSyncRepository) GetActiveAccountsByPlatformType(ctx context.Context, platformType domain.PlatformType) ([]domain.PlatformAccount, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT pa.id, pa.platform_id, pa.store_name, pa.api_key, pa.api_secret, pa.settings, pa.is_active, pa.created_at, pa.updated_at
		  FROM platform_accounts pa
		  JOIN external_platforms ep ON ep.id = pa.platform_id
		 WHERE ep.type=$1 AND ep.is_active=TRUE AND pa.is_active=TRUE`, platformType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.PlatformAccount
	for rows.Next() {
		var a domain.PlatformAccount
		if err := rows.Scan(&a.ID, &a.PlatformID, &a.StoreName, &a.APIKey, &a.APISecret, &a.Settings, &a.IsActive, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *ChannelSyncRepository) ListAllActiveAccounts(ctx context.Context) ([]domain.PlatformAccount, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT pa.id, pa.platform_id, pa.store_name, pa.api_key, pa.api_secret, pa.settings, pa.is_active, pa.created_at, pa.updated_at
		  FROM platform_accounts pa
		  JOIN external_platforms ep ON ep.id = pa.platform_id
		 WHERE ep.is_active=TRUE AND pa.is_active=TRUE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.PlatformAccount
	for rows.Next() {
		var a domain.PlatformAccount
		if err := rows.Scan(&a.ID, &a.PlatformID, &a.StoreName, &a.APIKey, &a.APISecret, &a.Settings, &a.IsActive, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ── Platform Products ─────────────────────────────────────────────────────────

func (r *ChannelSyncRepository) UpsertPlatformProduct(ctx context.Context, pp *domain.PlatformProduct) error {
	if pp.ID == uuid.Nil {
		pp.ID = uuid.New()
		pp.CreatedAt = time.Now().UTC()
	}
	now := time.Now().UTC()
	pp.LastSyncedAt = &now
	_, err := r.pool.Exec(ctx, `
		INSERT INTO platform_products
		    (id, platform_account_id, variant_id, external_product_id, external_variant_id, last_synced_at, sync_status, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (platform_account_id, variant_id)
		DO UPDATE SET
		    external_product_id=$4,
		    external_variant_id=$5,
		    last_synced_at=$6,
		    sync_status=$7`,
		pp.ID, pp.PlatformAccountID, pp.VariantID,
		pp.ExternalProductID, pp.ExternalVariantID,
		pp.LastSyncedAt, pp.SyncStatus, pp.CreatedAt,
	)
	return err
}

func (r *ChannelSyncRepository) GetMappedProducts(ctx context.Context, accountID uuid.UUID) ([]domain.PlatformProduct, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, platform_account_id, variant_id, external_product_id, external_variant_id,
		       last_synced_at, sync_status, COALESCE(sync_error,''), created_at
		  FROM platform_products
		 WHERE platform_account_id=$1`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.PlatformProduct
	for rows.Next() {
		var pp domain.PlatformProduct
		if err := rows.Scan(
			&pp.ID, &pp.PlatformAccountID, &pp.VariantID,
			&pp.ExternalProductID, &pp.ExternalVariantID,
			&pp.LastSyncedAt, &pp.SyncStatus, &pp.SyncError, &pp.CreatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, pp)
	}
	return out, rows.Err()
}

func (r *ChannelSyncRepository) GetLocalVariantID(ctx context.Context, accountID uuid.UUID, externalVariantID string) (uuid.UUID, error) {
	var variantID uuid.UUID
	err := r.pool.QueryRow(ctx, `
		SELECT variant_id FROM platform_products
		 WHERE platform_account_id=$1 AND external_variant_id=$2`,
		accountID, externalVariantID,
	).Scan(&variantID)
	if err == pgx.ErrNoRows {
		return uuid.Nil, fmt.Errorf("no local variant mapped to external_variant_id %s", externalVariantID)
	}
	return variantID, err
}

// ── Platform Orders ───────────────────────────────────────────────────────────

func (r *ChannelSyncRepository) UpsertPlatformOrder(ctx context.Context, po *domain.PlatformOrder) error {
	if po.ID == uuid.Nil {
		po.ID = uuid.New()
		po.CreatedAt = time.Now().UTC()
	}
	po.UpdatedAt = time.Now().UTC()
	_, err := r.pool.Exec(ctx, `
		INSERT INTO platform_orders
		    (id, platform_account_id, external_order_id, local_order_id, status, raw_payload, error_message, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$8)
		ON CONFLICT (platform_account_id, external_order_id)
		DO UPDATE SET local_order_id=$4, status=$5, error_message=$7, updated_at=$8`,
		po.ID, po.PlatformAccountID, po.ExternalOrderID, po.LocalOrderID,
		po.Status, po.RawPayload, po.ErrorMessage, po.UpdatedAt,
	)
	return err
}

func (r *ChannelSyncRepository) ListPlatformOrders(ctx context.Context, status string) ([]domain.PlatformOrder, error) {
	q := `SELECT id, platform_account_id, external_order_id, local_order_id, status,
	             raw_payload, COALESCE(error_message,''), created_at, updated_at
	        FROM platform_orders`
	args := []interface{}{}
	if status != "" {
		q += " WHERE status=$1"
		args = append(args, status)
	}
	q += " ORDER BY created_at DESC LIMIT 500"

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.PlatformOrder
	for rows.Next() {
		var po domain.PlatformOrder
		if err := rows.Scan(
			&po.ID, &po.PlatformAccountID, &po.ExternalOrderID, &po.LocalOrderID,
			&po.Status, &po.RawPayload, &po.ErrorMessage, &po.CreatedAt, &po.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, po)
	}
	return out, rows.Err()
}
