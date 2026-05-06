package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dubai-retail/os/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ActivityLogRepository handles activity log DB operations.
type ActivityLogRepository struct {
	pool *pgxpool.Pool
}

// RecordInput is the input for inserting an activity log entry.
type RecordInput struct {
	TenantID    uuid.UUID
	ActorID     uuid.UUID
	ActorEmail  string
	EventType   string
	Title       string
	Description string
	SubjectID   string
	SubjectType string
	Metadata    map[string]any
}

// Record inserts a new activity log entry.
func (r *ActivityLogRepository) Record(ctx context.Context, in RecordInput) error {
	metaJSON, _ := json.Marshal(in.Metadata)
	if metaJSON == nil {
		metaJSON = []byte("{}")
	}
	_, err := r.pool.Exec(ctx,
		`INSERT INTO activity_log (tenant_id, actor_id, actor_email, event_type, title, description, subject_id, subject_type, metadata)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		in.TenantID, in.ActorID, in.ActorEmail, in.EventType, in.Title, in.Description,
		in.SubjectID, in.SubjectType, metaJSON,
	)
	return err
}

// List returns paginated activity log entries for a tenant.
func (r *ActivityLogRepository) List(ctx context.Context, filters domain.ActivityLogFilters) (*domain.PageResponse[domain.ActivityLogItem], error) {
	page := filters.Page
	pageSize := filters.PageSize
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 25
	}
	offset := (page - 1) * pageSize

	args := []any{filters.TenantID}
	argIdx := 2
	var where []string

	if filters.Search != "" {
		where = append(where, fmt.Sprintf("(title ILIKE $%d OR description ILIKE $%d OR actor_email ILIKE $%d)", argIdx, argIdx, argIdx))
		args = append(args, "%"+strings.TrimSpace(filters.Search)+"%")
		argIdx++
	}

	whereClause := ""
	if len(where) > 0 {
		whereClause = " AND " + strings.Join(where, " AND ")
	}

	var total int
	err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM activity_log WHERE tenant_id = $1`+whereClause,
		args...,
	).Scan(&total)
	if err != nil {
		return nil, fmt.Errorf("activity log count: %w", err)
	}

	orderBy := "ORDER BY created_at DESC"
	limitOffset := fmt.Sprintf(" LIMIT %d OFFSET %d", pageSize, offset)

	rows, err := r.pool.Query(ctx,
		`SELECT id, event_type, title, description, actor_email, subject_id, subject_type, created_at, metadata
		   FROM activity_log WHERE tenant_id = $1`+whereClause+" "+orderBy+limitOffset,
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("activity log list: %w", err)
	}
	defer rows.Close()

	var items []domain.ActivityLogItem
	for rows.Next() {
		var item domain.ActivityLogItem
		var id uuid.UUID
		var metaRaw []byte
		err := rows.Scan(
			&id, &item.EventType, &item.Title, &item.Description,
			&item.Actor, &item.SubjectID, &item.SubjectType, &item.CreatedAt, &metaRaw,
		)
		if err != nil {
			return nil, err
		}
		if len(metaRaw) > 0 {
			_ = json.Unmarshal(metaRaw, &item.Metadata)
		}
		item.ID = id.String()
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return &domain.PageResponse[domain.ActivityLogItem]{
		Items: items,
		Total: total,
	}, nil
}
