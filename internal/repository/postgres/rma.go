package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/dubai-retail/os/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RMARepository handles all DB operations for the Returns / RMA module.
type RMARepository struct {
	pool *pgxpool.Pool
}

// InsertReturn persists the parent Return record (without items) inside tx.
func (r *RMARepository) InsertReturn(ctx context.Context, tx pgx.Tx, ret *domain.Return) error {
	const q = `
		INSERT INTO returns
		       (id, order_id, status, customer_name, customer_email, return_reason, notes, requested_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $9)`

	now := time.Now().UTC()
	ret.CreatedAt = now
	ret.UpdatedAt = now
	ret.RequestedAt = now

	_, err := tx.Exec(ctx, q,
		ret.ID, ret.OrderID, ret.Status,
		ret.CustomerName, ret.CustomerEmail, ret.ReturnReason,
		ret.Notes, now,
	)
	if err != nil {
		return fmt.Errorf("InsertReturn: %w", err)
	}
	return nil
}

// InsertReturnItem persists one line item (without photos) inside tx.
func (r *RMARepository) InsertReturnItem(ctx context.Context, tx pgx.Tx, item *domain.ReturnItem) error {
	const q = `
		INSERT INTO return_items
		       (id, return_id, order_item_id, variant_id, batch_item_id, quantity, condition,
		        qc_photo_hash_customer, qc_photo_hash_outbound, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $10)`

	now := time.Now().UTC()
	item.CreatedAt = now
	item.UpdatedAt = now

	_, err := tx.Exec(ctx, q,
		item.ID, item.ReturnID, item.OrderItemID, item.VariantID,
		item.BatchItemID, item.Quantity, item.Condition,
		item.QCPhotoHashCustomer, item.QCPhotoHashOutbound, now,
	)
	if err != nil {
		return fmt.Errorf("InsertReturnItem: %w", err)
	}
	return nil
}

// InsertReturnPhoto records a photo asset linked to a return item.
func (r *RMARepository) InsertReturnPhoto(ctx context.Context, photo *domain.ReturnPhoto) error {
	const q = `
		INSERT INTO return_photos
		       (id, return_item_id, photo_type, file_hash, file_size_bytes, mime_type, storage_path, uploaded_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`

	photo.UploadedAt = time.Now().UTC()
	_, err := r.pool.Exec(ctx, q,
		photo.ID, photo.ReturnItemID, photo.PhotoType,
		photo.FileHash, photo.FileSizeBytes, photo.MIMEType,
		photo.StoragePath, photo.UploadedAt,
	)
	return err
}

// GetReturnByID fetches a Return with all child ReturnItems.
func (r *RMARepository) GetReturnByID(ctx context.Context, id uuid.UUID) (*domain.Return, error) {
	const retQ = `
		SELECT id, order_id, status, customer_name, customer_email, return_reason,
		       rejection_reason, notes, requested_at, received_at, resolved_at,
		       created_at, updated_at
		  FROM returns WHERE id = $1`

	ret := &domain.Return{}
	err := r.pool.QueryRow(ctx, retQ, id).Scan(
		&ret.ID, &ret.OrderID, &ret.Status,
		&ret.CustomerName, &ret.CustomerEmail, &ret.ReturnReason,
		&ret.RejectionReason, &ret.Notes,
		&ret.RequestedAt, &ret.ReceivedAt, &ret.ResolvedAt,
		&ret.CreatedAt, &ret.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("GetReturnByID: %w", err)
	}

	// Load child items
	items, err := r.getReturnItems(ctx, id)
	if err != nil {
		return nil, err
	}
	ret.Items = items
	return ret, nil
}

func (r *RMARepository) getReturnItems(ctx context.Context, returnID uuid.UUID) ([]domain.ReturnItem, error) {
	const q = `
		SELECT id, return_id, order_item_id, variant_id, batch_item_id, quantity, condition,
		       qc_photo_hash_customer, qc_photo_hash_outbound,
		       qc_match_score, qc_passed, qc_reviewed_at, qc_reviewer_notes,
		       cogs_per_unit_reversed, created_at, updated_at
		  FROM return_items WHERE return_id = $1 ORDER BY created_at`

	rows, err := r.pool.Query(ctx, q, returnID)
	if err != nil {
		return nil, fmt.Errorf("getReturnItems: %w", err)
	}
	defer rows.Close()

	var items []domain.ReturnItem
	for rows.Next() {
		var it domain.ReturnItem
		if err := rows.Scan(
			&it.ID, &it.ReturnID, &it.OrderItemID, &it.VariantID, &it.BatchItemID,
			&it.Quantity, &it.Condition,
			&it.QCPhotoHashCustomer, &it.QCPhotoHashOutbound,
			&it.QCMatchScore, &it.QCPassed, &it.QCReviewedAt, &it.QCReviewerNotes,
			&it.COGSPerUnitReversed, &it.CreatedAt, &it.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("getReturnItems scan: %w", err)
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

// UpdateReturnItemQC persists the QC comparison result and marks qc_reviewed_at.
func (r *RMARepository) UpdateReturnItemQC(
	ctx context.Context,
	tx pgx.Tx,
	itemID uuid.UUID,
	score float64,
	passed bool,
	reviewerNotes string,
) error {
	now := time.Now().UTC()
	_, err := tx.Exec(ctx, `
		UPDATE return_items
		   SET qc_match_score = $2, qc_passed = $3,
		       qc_reviewer_notes = $4, qc_reviewed_at = $5,
		       updated_at = $5
		 WHERE id = $1`,
		itemID, score, passed, reviewerNotes, now,
	)
	return err
}

// UpdateReturnStatus advances the status and optionally sets lifecycle timestamps.
func (r *RMARepository) UpdateReturnStatus(
	ctx context.Context,
	tx pgx.Tx,
	returnID uuid.UUID,
	status domain.ReturnStatus,
	rejectionReason *string,
) error {
	now := time.Now().UTC()

	var receivedAt, resolvedAt *time.Time
	switch status {
	case domain.ReturnStatusReceived:
		receivedAt = &now
	case domain.ReturnStatusApproved, domain.ReturnStatusRejected, domain.ReturnStatusCompleted:
		resolvedAt = &now
	}

	_, err := tx.Exec(ctx, `
		UPDATE returns
		   SET status = $2, rejection_reason = COALESCE($3, rejection_reason),
		       received_at  = COALESCE($4, received_at),
		       resolved_at  = COALESCE($5, resolved_at),
		       updated_at   = $6
		 WHERE id = $1`,
		returnID, status, rejectionReason, receivedAt, resolvedAt, now,
	)
	return err
}

// SetReturnItemCOGS stamps the COGS per unit on the return item at approval time.
func (r *RMARepository) SetReturnItemCOGS(ctx context.Context, tx pgx.Tx, itemID uuid.UUID, cogsPerUnit string) error {
	_, err := tx.Exec(ctx, `
		UPDATE return_items
		   SET cogs_per_unit_reversed = $2, updated_at = NOW() AT TIME ZONE 'UTC'
		 WHERE id = $1`,
		itemID, cogsPerUnit,
	)
	return err
}

// ListReturns returns paginated returns (tenant-scoped via orders).
func (r *RMARepository) ListReturns(ctx context.Context, filters domain.ReturnListFilters) ([]domain.ReturnListItem, int, error) {
	if filters.Page < 1 {
		filters.Page = 1
	}
	if filters.PageSize < 1 || filters.PageSize > 100 {
		filters.PageSize = 25
	}
	offset := (filters.Page - 1) * filters.PageSize

	args := []any{filters.TenantID}
	where := []string{"o.tenant_id = $1"}
	argIdx := 2
	if filters.Status != "" {
		where = append(where, fmt.Sprintf("ret.status = $%d", argIdx))
		args = append(args, filters.Status)
		argIdx++
	}
	if filters.OrderID != nil {
		where = append(where, fmt.Sprintf("ret.order_id = $%d", argIdx))
		args = append(args, *filters.OrderID)
		argIdx++
	}
	if filters.CustomerID != nil {
		where = append(where, fmt.Sprintf("o.customer_id = $%d", argIdx))
		args = append(args, *filters.CustomerID)
		argIdx++
	}

	whereClause := strings.Join(where, " AND ")

	countQ := fmt.Sprintf(`
		SELECT COUNT(*) FROM returns ret
		JOIN orders o ON o.id = ret.order_id
		WHERE %s`, whereClause)
	var total int
	if err := r.pool.QueryRow(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("ListReturns count: %w", err)
	}

	args = append(args, filters.PageSize, offset)
	limitIdx := argIdx
	offsetIdx := argIdx + 1
	listQ := fmt.Sprintf(`
		SELECT ret.id, ret.order_id, o.customer_id, ret.customer_name, ret.customer_email,
		       ret.status::text, ret.return_reason,
		       (SELECT COUNT(*) FROM return_items ri WHERE ri.return_id = ret.id) AS item_count,
		       ret.requested_at
		  FROM returns ret
		  JOIN orders o ON o.id = ret.order_id
		 WHERE %s
		 ORDER BY ret.requested_at DESC
		 LIMIT $%d OFFSET $%d`, whereClause, limitIdx, offsetIdx)

	rows, err := r.pool.Query(ctx, listQ, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("ListReturns: %w", err)
	}
	defer rows.Close()

	var items []domain.ReturnListItem
	for rows.Next() {
		var it domain.ReturnListItem
		if err := rows.Scan(&it.ID, &it.OrderID, &it.CustomerID, &it.CustomerName, &it.CustomerEmail,
			&it.Status, &it.ReturnReason, &it.ItemCount, &it.RequestedAt); err != nil {
			return nil, 0, fmt.Errorf("ListReturns scan: %w", err)
		}
		items = append(items, it)
	}
	return items, total, rows.Err()
}

// GetOutboundQCHash returns the stored outbound QC photo hash for a return item.
// Returns ("", nil) when none has been stored yet.
func (r *RMARepository) GetOutboundQCHash(ctx context.Context, returnItemID uuid.UUID) (string, error) {
	var hash string
	err := r.pool.QueryRow(ctx, `
		SELECT file_hash FROM return_photos
		 WHERE return_item_id = $1 AND photo_type = 'outbound_qc'
		 ORDER BY uploaded_at DESC LIMIT 1`,
		returnItemID,
	).Scan(&hash)
	if err == pgx.ErrNoRows {
		return "", nil
	}
	return hash, err
}
