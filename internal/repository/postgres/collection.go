package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/dubai-retail/os/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type CollectionRepository struct {
	pool *pgxpool.Pool
}

func NewCollectionRepository(pool *pgxpool.Pool) *CollectionRepository {
	return &CollectionRepository{pool: pool}
}

func (r *CollectionRepository) InsertCollection(ctx context.Context, tx pgx.Tx, c *domain.ProductCollection) error {
	const q = `
		INSERT INTO product_collections (id, tenant_id, title, slug, description, image_url, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())`
	_, err := tx.Exec(ctx, q, c.ID, c.TenantID, c.Title, c.Slug, c.Description, c.ImageURL)
	return err
}

func (r *CollectionRepository) PatchCollection(ctx context.Context, tenantID, collectionID uuid.UUID, c *domain.ProductCollection) error {
	const q = `
		UPDATE product_collections
		   SET title = $1,
		       slug = $2,
		       description = $3,
		       image_url = $4,
		       updated_at = NOW()
		 WHERE tenant_id = $5 AND id = $6`
	tag, err := r.pool.Exec(ctx, q,
		c.Title, c.Slug, c.Description, c.ImageURL, tenantID, collectionID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (r *CollectionRepository) LinkProductsToCollection(ctx context.Context, tx pgx.Tx, collectionID uuid.UUID, productIDs []uuid.UUID) error {
	for _, pid := range productIDs {
		const insert = `
			INSERT INTO product_collection_memberships (collection_id, product_id, created_at)
			VALUES ($1, $2, NOW())
			ON CONFLICT DO NOTHING`
		if _, err := tx.Exec(ctx, insert, collectionID, pid); err != nil {
			return err
		}
	}
	return nil
}

func (r *CollectionRepository) ClearCollectionMemberships(ctx context.Context, tx pgx.Tx, collectionID uuid.UUID) error {
	_, err := tx.Exec(ctx, `DELETE FROM product_collection_memberships WHERE collection_id = $1`, collectionID)
	return err
}

func (r *CollectionRepository) DeleteCollection(ctx context.Context, tenantID, collectionID uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM product_collections WHERE tenant_id = $1 AND id = $2`, tenantID, collectionID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (r *CollectionRepository) ListCollections(ctx context.Context, tenantID uuid.UUID) ([]*domain.ProductCollection, error) {
	const q = `
		SELECT
			c.id,
			c.tenant_id,
			c.title,
			c.slug,
			c.description,
			c.image_url,
			COALESCE(m.cnt, 0),
			c.created_at,
			c.updated_at
		FROM product_collections c
		LEFT JOIN LATERAL (
			SELECT COUNT(*)::int AS cnt
			FROM product_collection_memberships pcm
			WHERE pcm.collection_id = c.id
		) m ON TRUE
		WHERE c.tenant_id = $1
		ORDER BY c.title ASC`

	rows, err := r.pool.Query(ctx, q, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*domain.ProductCollection
	for rows.Next() {
		col := &domain.ProductCollection{}
		if err := rows.Scan(&col.ID, &col.TenantID, &col.Title, &col.Slug, &col.Description, &col.ImageURL, &col.ProductCount, &col.CreatedAt, &col.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, col)
	}
	return out, rows.Err()
}

func (r *CollectionRepository) GetCollection(ctx context.Context, tenantID, collectionID uuid.UUID) (*domain.ProductCollection, error) {
	const q = `
		SELECT
			c.id,
			c.tenant_id,
			c.title,
			c.slug,
			c.description,
			c.image_url,
			COALESCE(m.product_ids, ARRAY[]::uuid[]) AS product_ids,
			COALESCE(m.product_count, 0) AS product_count,
			c.created_at,
			c.updated_at
		FROM product_collections c
		LEFT JOIN LATERAL (
			SELECT ARRAY_AGG(pcm.product_id ORDER BY pcm.created_at DESC) AS product_ids,
			       COUNT(*)::int AS product_count
			FROM product_collection_memberships pcm
			WHERE pcm.collection_id = c.id
		) m ON TRUE
		WHERE c.tenant_id = $1 AND c.id = $2`

	col := &domain.ProductCollection{}
	err := r.pool.QueryRow(ctx, q, tenantID, collectionID).Scan(
		&col.ID, &col.TenantID, &col.Title, &col.Slug, &col.Description, &col.ImageURL, &col.ProductIDs, &col.ProductCount, &col.CreatedAt, &col.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return col, nil
}

func (r *CollectionRepository) GetCollectionBySlug(ctx context.Context, tenantID uuid.UUID, slug string) (*domain.ProductCollection, error) {
	const q = `
		SELECT
			c.id,
			c.tenant_id,
			c.title,
			c.slug,
			c.description,
			c.image_url,
			COALESCE(m.product_ids, ARRAY[]::uuid[]) AS product_ids,
			COALESCE(m.product_count, 0) AS product_count,
			c.created_at,
			c.updated_at
		FROM product_collections c
		LEFT JOIN LATERAL (
			SELECT ARRAY_AGG(pcm.product_id ORDER BY pcm.created_at DESC) AS product_ids,
			       COUNT(*)::int AS product_count
			FROM product_collection_memberships pcm
			WHERE pcm.collection_id = c.id
		) m ON TRUE
		WHERE c.tenant_id = $1 AND c.slug = $2`

	col := &domain.ProductCollection{}
	err := r.pool.QueryRow(ctx, q, tenantID, slug).Scan(
		&col.ID, &col.TenantID, &col.Title, &col.Slug, &col.Description, &col.ImageURL, &col.ProductIDs, &col.ProductCount, &col.CreatedAt, &col.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return col, nil
}

// ListCollectionIDsForProduct implements product-collection membership reads.
func (r *CollectionRepository) ListCollectionIDsForProduct(ctx context.Context, productID uuid.UUID) ([]uuid.UUID, error) {
	const q = `
		SELECT collection_id
		  FROM product_collection_memberships
		 WHERE product_id = $1
		 ORDER BY created_at DESC`

	rows, err := r.pool.Query(ctx, q, productID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// ReplaceMembershipsForProduct replaces all collections for a single product after validating tenant parity.
func (r *CollectionRepository) ReplaceMembershipsForProduct(ctx context.Context, productID uuid.UUID, collectionIDs []uuid.UUID) error {
	var tenant uuid.UUID
	if err := r.pool.QueryRow(ctx, `SELECT tenant_id FROM products WHERE id = $1`, productID).Scan(&tenant); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("product not found")
		}
		return err
	}

	uniq := map[uuid.UUID]struct{}{}
	for _, id := range collectionIDs {
		if id == uuid.Nil {
			continue
		}
		uniq[id] = struct{}{}
	}

	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	for cid := range uniq {
		var cTenant uuid.UUID
		if err := tx.QueryRow(ctx,
			`SELECT tenant_id FROM product_collections WHERE id = $1`, cid,
		).Scan(&cTenant); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("collection %s not found", cid)
			}
			return err
		}
		if cTenant != tenant {
			return fmt.Errorf("collection tenant mismatch")
		}
	}

	if _, err := tx.Exec(ctx, `DELETE FROM product_collection_memberships WHERE product_id = $1`, productID); err != nil {
		return err
	}

	for cid := range uniq {
		if _, err := tx.Exec(ctx, `
			INSERT INTO product_collection_memberships (collection_id, product_id, created_at)
			VALUES ($1, $2, NOW())
			ON CONFLICT DO NOTHING
		`, cid, productID); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}
