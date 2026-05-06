package domain

import (
	"time"

	"github.com/google/uuid"
)

type MediaAsset struct {
	ID        uuid.UUID `json:"id"`
	TenantID  uuid.UUID `json:"tenant_id"`
	URL       string    `json:"url"`
	MimeType  string    `json:"mime_type"`
	SizeBytes int64     `json:"size_bytes"`
	Alt       *string   `json:"alt,omitempty"`
	Tags      []string  `json:"tags,omitempty"`
	SortOrder int       `json:"sort_order"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type MediaFilter struct {
	TenantID uuid.UUID
	Cursor   *time.Time // For keyset pagination based on created_at DESC
	Limit    int
	Type     string // e.g., "image", "video"
	Search   string // Search in alt or tags
}

type MediaPage struct {
	Items      []MediaAsset `json:"items"`
	NextCursor *time.Time   `json:"next_cursor,omitempty"`
}
