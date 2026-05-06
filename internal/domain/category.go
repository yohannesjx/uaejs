package domain

import (
	"time"

	"github.com/google/uuid"
)

type CategoryType string

const (
	CategoryTypeManual CategoryType = "manual"
	CategoryTypeSmart  CategoryType = "smart"
)

type SmartCollectionCondition struct {
	Field    string `json:"field"`
	Operator string `json:"operator"`
	Value    string `json:"value"`
}

type ProductCategory struct {
	ID           uuid.UUID                  `json:"id"`
	TenantID     uuid.UUID                  `json:"tenant_id"`
	Title        string                     `json:"title"`
	Slug         string                     `json:"slug"`
	Description  *string                    `json:"description,omitempty"`
	Type         CategoryType               `json:"type"`
	ImageURL     *string                    `json:"image_url,omitempty"`
	ProductCount int                        `json:"product_count"`
	ProductIDs   []uuid.UUID                `json:"product_ids,omitempty"`
	Conditions   []SmartCollectionCondition `json:"conditions"`
	CreatedAt    time.Time                  `json:"created_at"`
	UpdatedAt    time.Time                  `json:"updated_at"`
}
