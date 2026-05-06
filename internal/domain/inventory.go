package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// MovementType mirrors the PostgreSQL movement_type enum.
type MovementType string

const (
	MovementTypePurchaseIn         MovementType = "purchase_in"
	MovementTypeSaleOut            MovementType = "sale_out"
	MovementTypeAdjustmentIn       MovementType = "adjustment_in"
	MovementTypeAdjustmentOut      MovementType = "adjustment_out"
	MovementTypeReservation        MovementType = "reservation"
	MovementTypeReservationRelease MovementType = "reservation_release"
	MovementTypeReturnIn           MovementType = "return_in"
	MovementTypeTransferIn         MovementType = "transfer_in"  // stock arriving at a warehouse
	MovementTypeTransferOut        MovementType = "transfer_out" // stock leaving a warehouse
)

// Inventory is the single global stock record per variant.
type Inventory struct {
	ID                uuid.UUID `db:"id"                 json:"id"`
	VariantID         uuid.UUID `db:"variant_id"         json:"variant_id"`
	QuantityOnHand    int       `db:"quantity_on_hand"   json:"quantity_on_hand"`
	QuantityReserved  int       `db:"quantity_reserved"  json:"quantity_reserved"`
	QuantityAvailable int       `db:"quantity_available" json:"quantity_available"` // computed column
	ReorderPoint      int       `db:"reorder_point"      json:"reorder_point"`
	ReorderQty        int       `db:"reorder_qty"        json:"reorder_qty"`
	CreatedAt         time.Time `db:"created_at"         json:"created_at"`
	UpdatedAt         time.Time `db:"updated_at"         json:"updated_at"`
}

// InventoryMovement is an immutable ledger entry for every stock change.
type InventoryMovement struct {
	ID               uuid.UUID        `db:"id"                  json:"id"`
	VariantID        uuid.UUID        `db:"variant_id"          json:"variant_id"`
	BatchItemID      *uuid.UUID       `db:"batch_item_id"       json:"batch_item_id,omitempty"`
	OrderID          *uuid.UUID       `db:"order_id"            json:"order_id,omitempty"`
	ReservationID    *uuid.UUID       `db:"reservation_id"      json:"reservation_id,omitempty"`
	MovementType     MovementType     `db:"movement_type"       json:"movement_type"`
	Quantity         int              `db:"quantity"            json:"quantity"`
	QuantityBefore   int              `db:"quantity_before"     json:"quantity_before"`
	QuantityAfter    int              `db:"quantity_after"      json:"quantity_after"`
	UnitCostSnapshot *decimal.Decimal `db:"unit_cost_snapshot"  json:"unit_cost_snapshot,omitempty"`
	ChannelID        *uuid.UUID       `db:"channel_id"          json:"channel_id,omitempty"`
	Reference        *string          `db:"reference"           json:"reference,omitempty"`
	Notes            *string          `db:"notes"               json:"notes,omitempty"`
	CreatedAt        time.Time        `db:"created_at"          json:"created_at"`
}
