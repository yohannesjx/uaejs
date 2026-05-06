package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/dubai-retail/os/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type MediaRepository struct {
	pool *pgxpool.Pool
}

func (r *MediaRepository) InsertMedia(ctx context.Context, item domain.MediaAsset) error {
	query := `
		INSERT INTO media_assets (id, tenant_id, url, mime_type, size_bytes, alt, tags, sort_order, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`
	_, err := r.pool.Exec(ctx, query,
		item.ID,
		item.TenantID,
		item.URL,
		item.MimeType,
		item.SizeBytes,
		item.Alt,
		item.Tags,
		item.SortOrder,
		item.CreatedAt,
		item.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("InsertMedia: %w", err)
	}
	return nil
}

func (r *MediaRepository) ListMedia(ctx context.Context, f domain.MediaFilter) (domain.MediaPage, error) {
	// Build cursor-based query
	query := `
		SELECT id, tenant_id, url, mime_type, size_bytes, alt, tags, sort_order, created_at, updated_at
		FROM media_assets
		WHERE tenant_id = $1
	`
	args := []interface{}{f.TenantID}
	argID := 2

	if f.Type != "" {
		query += fmt.Sprintf(" AND mime_type LIKE $%d", argID)
		args = append(args, f.Type+"/%")
		argID++
	}

	if f.Search != "" {
		query += fmt.Sprintf(" AND (alt ILIKE $%d OR $%d = ANY(tags))", argID, argID)
		searchPattern := "%" + f.Search + "%"
		args = append(args, searchPattern)
		argID++
	}

	if f.Cursor != nil {
		query += fmt.Sprintf(" AND created_at < $%d", argID)
		args = append(args, *f.Cursor)
		argID++
	}

	query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d", argID)
	args = append(args, f.Limit+1) // Fetch one extra to determine NextCursor

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return domain.MediaPage{}, fmt.Errorf("ListMedia query: %w", err)
	}
	defer rows.Close()

	var items []domain.MediaAsset
	for rows.Next() {
		var i domain.MediaAsset
		if err := rows.Scan(
			&i.ID, &i.TenantID, &i.URL, &i.MimeType, &i.SizeBytes,
			&i.Alt, &i.Tags, &i.SortOrder, &i.CreatedAt, &i.UpdatedAt,
		); err != nil {
			return domain.MediaPage{}, fmt.Errorf("ListMedia scan: %w", err)
		}
		items = append(items, i)
	}
	if err := rows.Err(); err != nil {
		return domain.MediaPage{}, fmt.Errorf("ListMedia rows: %w", err)
	}

	var nextCursor *time.Time
	if len(items) > f.Limit {
		// We have more results
		nextCursor = &items[f.Limit-1].CreatedAt
		items = items[:f.Limit]
	}

	return domain.MediaPage{
		Items:      items,
		NextCursor: nextCursor,
	}, nil
}

func (r *MediaRepository) PatchMedia(ctx context.Context, id string, alt *string, tags []string) error {
	query := `
		UPDATE media_assets
		SET alt = COALESCE($2, alt),
		    tags = COALESCE($3, tags),
		    updated_at = NOW()
		WHERE id = $1
	`
	cmd, err := r.pool.Exec(ctx, query, id, alt, tags)
	if err != nil {
		return fmt.Errorf("PatchMedia: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (r *MediaRepository) DeleteMedia(ctx context.Context, id string) error {
	query := `DELETE FROM media_assets WHERE id = $1`
	cmd, err := r.pool.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("DeleteMedia: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (r *MediaRepository) GetMedia(ctx context.Context, id string) (*domain.MediaAsset, error) {
	query := `
		SELECT id, tenant_id, url, mime_type, size_bytes, alt, tags, sort_order, created_at, updated_at
		FROM media_assets
		WHERE id = $1
	`
	var i domain.MediaAsset
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&i.ID, &i.TenantID, &i.URL, &i.MimeType, &i.SizeBytes,
		&i.Alt, &i.Tags, &i.SortOrder, &i.CreatedAt, &i.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil // Return nil, nil for not found (or return err, handled by svc)
		}
		return nil, fmt.Errorf("GetMedia: %w", err)
	}
	return &i, nil
}
