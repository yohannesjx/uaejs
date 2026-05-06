package domain

import (
	"time"

	"github.com/google/uuid"
)

// WarehouseType mirrors the warehouse_type DB enum.
type WarehouseType string

const (
	WarehouseTypeWarehouse WarehouseType = "warehouse"
	WarehouseTypeStore     WarehouseType = "store"
	WarehouseTypeDropship  WarehouseType = "dropship"
	WarehouseTypeVirtual   WarehouseType = "virtual"
)

// DefaultWarehouseID is the well-known UUID seeded for single-store deployments.
var DefaultWarehouseID = uuid.MustParse("00000000-0000-0000-0000-000000000030")

// Warehouse is a physical or logical stock location.
type Warehouse struct {
	ID        uuid.UUID     `json:"id"`
	TenantID  uuid.UUID     `json:"tenant_id"`
	Name      string        `json:"name"`
	Type      WarehouseType `json:"type"`
	Address   string        `json:"address"`
	City      string        `json:"city"`
	Country   string        `json:"country"`
	IsActive  bool          `json:"is_active"`
	Priority  int           `json:"priority"` // lower = higher fulfillment priority
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
}

// WarehouseStock is the per-location stock record for a single variant.
type WarehouseStock struct {
	ID           uuid.UUID `json:"id"`
	WarehouseID  uuid.UUID `json:"warehouse_id"`
	VariantID    uuid.UUID `json:"variant_id"`
	QtyOnHand    int       `json:"qty_on_hand"`
	QtyReserved  int       `json:"qty_reserved"`
	QtyAvailable int       `json:"qty_available"` // computed: on_hand - reserved
	ReorderPoint int       `json:"reorder_point"`
	ReorderQty   int       `json:"reorder_qty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// TransferRequest describes the movement of stock from one warehouse to another.
type TransferRequest struct {
	FromWarehouseID uuid.UUID `json:"from_warehouse_id"`
	ToWarehouseID   uuid.UUID `json:"to_warehouse_id"`
	VariantID       uuid.UUID `json:"variant_id"`
	Quantity        int       `json:"quantity"`
	Notes           *string   `json:"notes,omitempty"`
}

// TransferResult is returned after a successful warehouse transfer.
type TransferResult struct {
	FromStock *WarehouseStock     `json:"from_stock"`
	ToStock   *WarehouseStock     `json:"to_stock"`
	Movements []InventoryMovement `json:"movements"`
}

// InventoryListItem is a flattened row for inventory management UI.
type InventoryListItem struct {
	ProductID         uuid.UUID `json:"product_id"`
	ProductName       string    `json:"product_name"`
	VariantID         uuid.UUID `json:"variant_id"`
	VariantName       string    `json:"variant_name"`
	SKU               string    `json:"sku"`
	Category          string    `json:"category"`
	WarehouseID       uuid.UUID `json:"warehouse_id"`
	WarehouseName     string    `json:"warehouse"`
	AvailableQuantity int       `json:"available_quantity"`
	ReservedQuantity  int       `json:"reserved_quantity"`
	IncomingQuantity  int       `json:"incoming_quantity"`
}

type InventoryAdjustmentType string

const (
	InventoryAdjustmentIncrease InventoryAdjustmentType = "increase"
	InventoryAdjustmentDecrease InventoryAdjustmentType = "decrease"
)

type TransferStatus string

const (
	TransferStatusDraft     TransferStatus = "draft"
	TransferStatusPending   TransferStatus = "pending"
	TransferStatusInTransit TransferStatus = "in_transit"
	TransferStatusCompleted TransferStatus = "completed"
	TransferStatusCancelled TransferStatus = "cancelled"
)

type InventoryTransfer struct {
	ID                     uuid.UUID      `json:"id"`
	TenantID               uuid.UUID      `json:"tenant_id"`
	Reference              string         `json:"reference"`
	OriginWarehouseID      uuid.UUID      `json:"origin_warehouse_id"`
	DestinationWarehouseID uuid.UUID      `json:"destination_warehouse_id"`
	Status                 TransferStatus `json:"status"`
	Notes                  *string        `json:"notes,omitempty"`
	Tags                   []string       `json:"tags,omitempty"`
	CreatedAt              time.Time      `json:"created_at"`
	UpdatedAt              time.Time      `json:"updated_at"`
	TotalItems             int            `json:"total_items"`
	Items                  []TransferItem `json:"items,omitempty"`
}

type TransferItem struct {
	ID         uuid.UUID `json:"id"`
	TransferID uuid.UUID `json:"transfer_id"`
	VariantID  uuid.UUID `json:"variant_id"`
	Quantity   int       `json:"quantity"`
}
