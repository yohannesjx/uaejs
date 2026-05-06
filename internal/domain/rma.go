package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// ReturnStatus lifecycle of a customer return request.
type ReturnStatus string

const (
	ReturnStatusPending   ReturnStatus = "pending"
	ReturnStatusReceived  ReturnStatus = "received"
	ReturnStatusQCReview  ReturnStatus = "qc_review"
	ReturnStatusApproved  ReturnStatus = "approved"
	ReturnStatusRejected  ReturnStatus = "rejected"
	ReturnStatusCompleted ReturnStatus = "completed"
)

// ItemCondition describes the physical state of a returned item.
type ItemCondition string

const (
	ItemConditionGood      ItemCondition = "good"
	ItemConditionDamaged   ItemCondition = "damaged"
	ItemConditionWrongItem ItemCondition = "wrong_item"
)

// PhotoType distinguishes the originator of a QC photo.
type PhotoType string

const (
	PhotoTypeOutboundQC        PhotoType = "outbound_qc"
	PhotoTypeCustomerSubmitted PhotoType = "customer_submitted"
	PhotoTypeWarehouseReceived PhotoType = "warehouse_received"
)

// Return is the top-level RMA request linked to one original order.
type Return struct {
	ID              uuid.UUID    `json:"id"`
	OrderID         uuid.UUID    `json:"order_id"`
	Status          ReturnStatus `json:"status"`
	CustomerName    string       `json:"customer_name"`
	CustomerEmail   string       `json:"customer_email"`
	ReturnReason    string       `json:"return_reason"`
	RejectionReason *string      `json:"rejection_reason,omitempty"`
	RequestedAt     time.Time    `json:"requested_at"`
	ReceivedAt      *time.Time   `json:"received_at,omitempty"`
	ResolvedAt      *time.Time   `json:"resolved_at,omitempty"`
	Notes           *string      `json:"notes,omitempty"`
	Items           []ReturnItem `json:"items"`
	CreatedAt       time.Time    `json:"created_at"`
	UpdatedAt       time.Time    `json:"updated_at"`
}

// ReturnItem is a line item within a Return, referencing the original order_item.
type ReturnItem struct {
	ID              uuid.UUID     `json:"id"`
	ReturnID        uuid.UUID     `json:"return_id"`
	OrderItemID     uuid.UUID     `json:"order_item_id"`
	VariantID       uuid.UUID     `json:"variant_id"`
	BatchItemID     *uuid.UUID    `json:"batch_item_id,omitempty"` // original FIFO batch
	Quantity        int           `json:"quantity"`
	Condition       ItemCondition `json:"condition"`

	// QC photo comparison fields
	QCPhotoHashCustomer *string  `json:"qc_photo_hash_customer,omitempty"`
	QCPhotoHashOutbound *string  `json:"qc_photo_hash_outbound,omitempty"`
	QCMatchScore        *float64 `json:"qc_match_score,omitempty"`
	QCPassed            *bool    `json:"qc_passed,omitempty"`
	QCReviewedAt        *time.Time `json:"qc_reviewed_at,omitempty"`
	QCReviewerNotes     *string  `json:"qc_reviewer_notes,omitempty"`

	// Copied at approval time for immutable COGS reversal audit trail
	COGSPerUnitReversed *decimal.Decimal `json:"cogs_per_unit_reversed,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ReturnPhoto is a QC or customer-submitted photo linked to a return line item.
type ReturnPhoto struct {
	ID            uuid.UUID `json:"id"`
	ReturnItemID  uuid.UUID `json:"return_item_id"`
	PhotoType     PhotoType `json:"photo_type"`
	FileHash      string    `json:"file_hash"`    // SHA-256 hex
	FileSizeBytes int64     `json:"file_size_bytes"`
	MIMEType      string    `json:"mime_type"`
	StoragePath   string    `json:"storage_path"` // s3://... or local
	UploadedAt    time.Time `json:"uploaded_at"`
}

// QCResult is the output of the photo comparison step.
type QCResult struct {
	ReturnItemID uuid.UUID
	MatchScore   float64 // 1.0 = identical, 0.0 = no match
	QCPassed     bool    // true if score >= 0.5 (configurable threshold)
	Reason       string  // human-readable explanation
}
