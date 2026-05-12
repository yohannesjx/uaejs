package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/dubai-retail/os/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type CategoryRepository struct {
	pool *pgxpool.Pool
}

func NewCategoryRepository(pool *pgxpool.Pool) *CategoryRepository {
	return &CategoryRepository{pool: pool}
}

func (r *CategoryRepository) InsertCategory(ctx context.Context, tx pgx.Tx, c *domain.ProductCategory) error {
	conditionsJSON, err := json.Marshal(c.Conditions)
	if err != nil {
		return fmt.Errorf("serialize conditions: %w", err)
	}
	if len(c.Conditions) == 0 {
		conditionsJSON = []byte("[]")
	}

	query := `
		INSERT INTO product_categories (
			id, tenant_id, title, slug, description, type, image_url, conditions, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, NOW(), NOW()
		)
	`
	_, err = tx.Exec(ctx, query,
		c.ID, c.TenantID, c.Title, c.Slug, c.Description,
		c.Type, c.ImageURL, conditionsJSON,
	)
	return err
}

// categoryProductCountExpr counts manual categories via memberships and smart categories
// by matching active products against JSON conditions (title contains / equals).
const categoryProductCountExpr = `
COALESCE(
	CASE c.type
		WHEN 'manual' THEN (
			SELECT COUNT(*)::int FROM product_category_memberships pcm WHERE pcm.category_id = c.id
		)
		WHEN 'smart' THEN (
			SELECT COUNT(*)::int
			FROM products p
			WHERE p.tenant_id = c.tenant_id
				AND p.status = 'active'
				AND COALESCE(jsonb_array_length(COALESCE(c.conditions, '[]'::jsonb)), 0) > 0
				AND (
					SELECT COALESCE(bool_and(
						CASE
							WHEN (cond->>'field') = 'title' AND (cond->>'operator') = 'contains' THEN
								LOWER(COALESCE(p.name, '')) LIKE '%' || LOWER(TRIM(cond->>'value')) || '%'
							WHEN (cond->>'field') = 'title' AND (cond->>'operator') = 'equals' THEN
								LOWER(TRIM(COALESCE(p.name, ''))) = LOWER(TRIM(cond->>'value'))
							ELSE FALSE
						END
					), false)
					FROM jsonb_array_elements(COALESCE(c.conditions, '[]'::jsonb)) AS cond
				)
		)
		ELSE 0
	END,
	0)`

func (r *CategoryRepository) DeleteMembershipsForProduct(ctx context.Context, tx pgx.Tx, productID uuid.UUID) error {
	_, err := tx.Exec(ctx, `DELETE FROM product_category_memberships WHERE product_id = $1`, productID)
	return err
}

func (r *CategoryRepository) FirstCategoryIDForProduct(ctx context.Context, productID uuid.UUID) (*uuid.UUID, error) {
	var id uuid.UUID
	err := r.pool.QueryRow(ctx,
		`SELECT category_id FROM product_category_memberships WHERE product_id = $1 ORDER BY created_at ASC LIMIT 1`,
		productID,
	).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &id, nil
}

func (r *CategoryRepository) LinkProducts(ctx context.Context, tx pgx.Tx, categoryID uuid.UUID, productIDs []uuid.UUID) error {
	if len(productIDs) == 0 {
		return nil
	}

	for _, pid := range productIDs {
		query := `
			INSERT INTO product_category_memberships (category_id, product_id, created_at)
			VALUES ($1, $2, NOW())
			ON CONFLICT DO NOTHING
		`
		if _, err := tx.Exec(ctx, query, categoryID, pid); err != nil {
			return err
		}
	}
	return nil
}

func (r *CategoryRepository) ListCategories(ctx context.Context, tenantID uuid.UUID) ([]*domain.ProductCategory, error) {
	query := `
		SELECT
			c.id,
			c.tenant_id,
			c.title,
			c.slug,
			c.description,
			c.type,
			c.image_url,
			c.conditions,
			` + categoryProductCountExpr + ` AS product_count,
			c.created_at,
			c.updated_at
		FROM product_categories c
		WHERE tenant_id = $1
		ORDER BY c.title ASC
	`
	rows, err := r.pool.Query(ctx, query, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*domain.ProductCategory
	for rows.Next() {
		var c domain.ProductCategory
		var conditionsJSON []byte
		if err := rows.Scan(
			&c.ID, &c.TenantID, &c.Title, &c.Slug, &c.Description,
			&c.Type, &c.ImageURL, &conditionsJSON, &c.ProductCount, &c.CreatedAt, &c.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(conditionsJSON, &c.Conditions); err != nil {
			c.Conditions = []domain.SmartCollectionCondition{}
		}
		results = append(results, &c)
	}
	return results, nil
}

func (r *CategoryRepository) GetCategory(ctx context.Context, tenantID, categoryID uuid.UUID) (*domain.ProductCategory, error) {
	query := `
		SELECT
			c.id,
			c.tenant_id,
			c.title,
			c.slug,
			c.description,
			c.type,
			c.image_url,
			c.conditions,
			COALESCE(m.product_ids, ARRAY[]::uuid[]) AS product_ids,
			` + categoryProductCountExpr + ` AS product_count,
			c.created_at,
			c.updated_at
		FROM product_categories c
		LEFT JOIN LATERAL (
			SELECT ARRAY_AGG(pcm.product_id ORDER BY pcm.created_at DESC) AS product_ids
			FROM product_category_memberships pcm
			WHERE pcm.category_id = c.id
		) m ON TRUE
		WHERE tenant_id = $1 AND id = $2
	`
	var c domain.ProductCategory
	var conditionsJSON []byte
	err := r.pool.QueryRow(ctx, query, tenantID, categoryID).Scan(
		&c.ID, &c.TenantID, &c.Title, &c.Slug, &c.Description,
		&c.Type, &c.ImageURL, &conditionsJSON, &c.ProductIDs, &c.ProductCount, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(conditionsJSON, &c.Conditions); err != nil {
		c.Conditions = []domain.SmartCollectionCondition{}
	}
	return &c, nil
}

func (r *CategoryRepository) DeleteCategory(ctx context.Context, tenantID, categoryID uuid.UUID) error {
	query := `DELETE FROM product_categories WHERE tenant_id = $1 AND id = $2`
	_, err := r.pool.Exec(ctx, query, tenantID, categoryID)
	return err
}

func (r *CategoryRepository) PatchCategory(ctx context.Context, tenantID, categoryID uuid.UUID, c *domain.ProductCategory) error {
	conditionsJSON, _ := json.Marshal(c.Conditions)
	if len(c.Conditions) == 0 {
		conditionsJSON = []byte("[]")
	}

	query := `
		UPDATE product_categories 
		SET title = $1, slug = $2, description = $3, type = $4, image_url = $5, conditions = $6, updated_at = NOW()
		WHERE tenant_id = $7 AND id = $8
	`
	_, err := r.pool.Exec(ctx, query,
		c.Title, c.Slug, c.Description, c.Type, c.ImageURL, conditionsJSON,
		tenantID, categoryID,
	)
	return err
}

func (r *CategoryRepository) ClearMemberships(ctx context.Context, tx pgx.Tx, categoryID uuid.UUID) error {
	query := `DELETE FROM product_category_memberships WHERE category_id = $1`
	_, err := tx.Exec(ctx, query, categoryID)
	return err
}
