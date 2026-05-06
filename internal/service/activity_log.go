package service

import (
	"context"

	"github.com/dubai-retail/os/internal/domain"
	"github.com/dubai-retail/os/internal/repository/postgres"
	"github.com/google/uuid"
)

// ActivityLogService handles activity log recording and listing.
type ActivityLogService struct {
	repo *postgres.ActivityLogRepository
}

// NewActivityLogService creates an ActivityLogService.
func NewActivityLogService(repo *postgres.ActivityLogRepository) *ActivityLogService {
	return &ActivityLogService{repo: repo}
}

// RecordInput is the input for recording an activity.
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

// Record writes an activity log entry. Best-effort; errors are logged but not returned.
func (s *ActivityLogService) Record(ctx context.Context, in RecordInput) error {
	return s.repo.Record(ctx, postgres.RecordInput{
		TenantID:    in.TenantID,
		ActorID:     in.ActorID,
		ActorEmail:  in.ActorEmail,
		EventType:   in.EventType,
		Title:       in.Title,
		Description: in.Description,
		SubjectID:   in.SubjectID,
		SubjectType: in.SubjectType,
		Metadata:    in.Metadata,
	})
}

// List returns paginated activity log entries.
func (s *ActivityLogService) List(ctx context.Context, filters domain.ActivityLogFilters) (*domain.PageResponse[domain.ActivityLogItem], error) {
	return s.repo.List(ctx, filters)
}
