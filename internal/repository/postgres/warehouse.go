package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/dubai-retail/os/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// WarehouseRepository handles all warehouse and per-location stock DB operations.
type WarehouseRepository struct {
	pool *pgxpool.Pool
}

// ── Warehouses ────────────────────────────────────────────────────────────────

// InsertWarehouse creates a new warehouse.
func (r *WarehouseRepository) InsertWarehouse(ctx context.Context, w *domain.Warehouse) error {
	w.ID = uuid.New()
	now := time.Now().UTC()
	w.CreatedAt = now
	w.UpdatedAt = now
	_, err := r.pool.Exec(ctx,
		`INSERT INTO warehouses (id, tenant_id, name, type, address, city, country, is_active, priority, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $10)`,
		w.ID, w.TenantID, w.Name, string(w.Type), w.Address, w.City, w.Country, w.IsActive, w.Priority, now,
	)
	return err
}

// UpdateWarehouse persists mutable warehouse fields.
func (r *WarehouseRepository) UpdateWarehouse(ctx context.Context, w *domain.Warehouse) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE warehouses
		    SET name = $2, type = $3, address = $4, city = $5, country = $6,
		        is_active = $7, priority = $8, updated_at = NOW()
		  WHERE id = $1 AND tenant_id = $9`,
		w.ID, w.Name, string(w.Type), w.Address, w.City, w.Country, w.IsActive, w.Priority, w.TenantID,
	)
	return err
}

// GetWarehouseByID returns a warehouse by primary key.
func (r *WarehouseRepository) GetWarehouseByID(ctx context.Context, id uuid.UUID) (*domain.Warehouse, error) {
	var w domain.Warehouse
	var wt string
	err := r.pool.QueryRow(ctx,
		`SELECT id, tenant_id, name, type, address, city, country, is_active, priority, created_at, updated_at
		   FROM warehouses WHERE id = $1`, id,
	).Scan(&w.ID, &w.TenantID, &w.Name, &wt, &w.Address, &w.City, &w.Country, &w.IsActive, &w.Priority, &w.CreatedAt, &w.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("warehouse not found")
	}
	if err != nil {
		return nil, fmt.Errorf("GetWarehouseByID: %w", err)
	}
	w.Type = domain.WarehouseType(wt)
	return &w, nil
}

// ListWarehousesByTenant returns all warehouses for a tenant ordered by priority.
func (r *WarehouseRepository) ListWarehousesByTenant(ctx context.Context, tenantID uuid.UUID) ([]domain.Warehouse, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, tenant_id, name, type, address, city, country, is_active, priority, created_at, updated_at
		   FROM warehouses WHERE tenant_id = $1 ORDER BY priority, name`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var warehouses []domain.Warehouse
	for rows.Next() {
		var w domain.Warehouse
		var wt string
		if err := rows.Scan(&w.ID, &w.TenantID, &w.Name, &wt, &w.Address, &w.City, &w.Country, &w.IsActive, &w.Priority, &w.CreatedAt, &w.UpdatedAt); err != nil {
			return nil, err
		}
		w.Type = domain.WarehouseType(wt)
		warehouses = append(warehouses, w)
	}
	return warehouses, rows.Err()
}

func (r *WarehouseRepository) EnsureDefaultWarehouse(ctx context.Context, tenantID uuid.UUID) (*domain.Warehouse, error) {
	var w domain.Warehouse
	var wt string
	err := r.pool.QueryRow(ctx,
		`SELECT id, tenant_id, name, type, address, city, country, is_active, priority, created_at, updated_at
		   FROM warehouses
		  WHERE tenant_id = $1
		  ORDER BY priority ASC, created_at ASC
		  LIMIT 1`,
		tenantID,
	).Scan(&w.ID, &w.TenantID, &w.Name, &wt, &w.Address, &w.City, &w.Country, &w.IsActive, &w.Priority, &w.CreatedAt, &w.UpdatedAt)
	if err == nil {
		w.Type = domain.WarehouseType(wt)
		return &w, nil
	}
	if err != pgx.ErrNoRows {
		return nil, fmt.Errorf("EnsureDefaultWarehouse lookup: %w", err)
	}

	input := &domain.Warehouse{
		TenantID: tenantID,
		Name:     "Default Warehouse",
		Type:     domain.WarehouseTypeWarehouse,
		Address:  "N/A",
		City:     "Dubai",
		Country:  "AE",
		IsActive: true,
		Priority: 1,
	}
	if err := r.InsertWarehouse(ctx, input); err != nil {
		return nil, fmt.Errorf("EnsureDefaultWarehouse insert: %w", err)
	}
	return input, nil
}

// ── Warehouse Stock ───────────────────────────────────────────────────────────

// GetStockForUpdate returns the warehouse_stock row with a pessimistic lock.
// Must be called inside a transaction.
func (r *WarehouseRepository) GetStockForUpdate(ctx context.Context, tx pgx.Tx, warehouseID, variantID uuid.UUID) (*domain.WarehouseStock, error) {
	var s domain.WarehouseStock
	err := tx.QueryRow(ctx,
		`SELECT id, warehouse_id, variant_id, qty_on_hand, qty_reserved, qty_available, reorder_point, reorder_qty, created_at, updated_at
		   FROM warehouse_stock
		  WHERE warehouse_id = $1 AND variant_id = $2
		  FOR UPDATE`,
		warehouseID, variantID,
	).Scan(&s.ID, &s.WarehouseID, &s.VariantID, &s.QtyOnHand, &s.QtyReserved, &s.QtyAvailable, &s.ReorderPoint, &s.ReorderQty, &s.CreatedAt, &s.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("warehouse stock not found for warehouse %s variant %s", warehouseID, variantID)
	}
	if err != nil {
		return nil, fmt.Errorf("GetStockForUpdate: %w", err)
	}
	return &s, nil
}

// UpsertStock creates or updates a warehouse_stock row.
func (r *WarehouseRepository) UpsertStock(ctx context.Context, s *domain.WarehouseStock) error {
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	_, err := r.pool.Exec(ctx,
		`INSERT INTO warehouse_stock (id, warehouse_id, variant_id, qty_on_hand, qty_reserved, reorder_point, reorder_qty)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 ON CONFLICT (warehouse_id, variant_id)
		 DO UPDATE SET qty_on_hand = EXCLUDED.qty_on_hand, qty_reserved = EXCLUDED.qty_reserved,
		               reorder_point = EXCLUDED.reorder_point, reorder_qty = EXCLUDED.reorder_qty,
		               updated_at = NOW()`,
		s.ID, s.WarehouseID, s.VariantID, s.QtyOnHand, s.QtyReserved, s.ReorderPoint, s.ReorderQty,
	)
	return err
}

func (r *WarehouseRepository) UpsertStockTx(ctx context.Context, tx pgx.Tx, s *domain.WarehouseStock) error {
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	_, err := tx.Exec(ctx,
		`INSERT INTO warehouse_stock (id, warehouse_id, variant_id, qty_on_hand, qty_reserved, reorder_point, reorder_qty)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 ON CONFLICT (warehouse_id, variant_id)
		 DO NOTHING`,
		s.ID, s.WarehouseID, s.VariantID, s.QtyOnHand, s.QtyReserved, s.ReorderPoint, s.ReorderQty,
	)
	return err
}

// AdjustStockTx adjusts qty_on_hand by delta inside a transaction (delta may be negative).
func (r *WarehouseRepository) AdjustStockTx(ctx context.Context, tx pgx.Tx, warehouseID, variantID uuid.UUID, delta int) (*domain.WarehouseStock, error) {
	var s domain.WarehouseStock
	err := tx.QueryRow(ctx,
		`UPDATE warehouse_stock
		    SET qty_on_hand = qty_on_hand + $3, updated_at = NOW()
		  WHERE warehouse_id = $1 AND variant_id = $2
		  RETURNING id, warehouse_id, variant_id, qty_on_hand, qty_reserved, qty_available, reorder_point, reorder_qty, created_at, updated_at`,
		warehouseID, variantID, delta,
	).Scan(&s.ID, &s.WarehouseID, &s.VariantID, &s.QtyOnHand, &s.QtyReserved, &s.QtyAvailable, &s.ReorderPoint, &s.ReorderQty, &s.CreatedAt, &s.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("no stock row for warehouse %s variant %s — call UpsertStock first", warehouseID, variantID)
	}
	return &s, err
}

// ListInventoryByWarehouse returns all stock rows for a warehouse.
func (r *WarehouseRepository) ListInventoryByWarehouse(ctx context.Context, warehouseID uuid.UUID) ([]domain.WarehouseStock, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, warehouse_id, variant_id, qty_on_hand, qty_reserved, qty_available, reorder_point, reorder_qty, created_at, updated_at
		   FROM warehouse_stock WHERE warehouse_id = $1 ORDER BY variant_id`, warehouseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var stocks []domain.WarehouseStock
	for rows.Next() {
		var s domain.WarehouseStock
		if err := rows.Scan(&s.ID, &s.WarehouseID, &s.VariantID, &s.QtyOnHand, &s.QtyReserved, &s.QtyAvailable, &s.ReorderPoint, &s.ReorderQty, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		stocks = append(stocks, s)
	}
	return stocks, rows.Err()
}

// GetStockByWarehouseAndVariant returns one stock row (no lock).
func (r *WarehouseRepository) GetStockByWarehouseAndVariant(ctx context.Context, warehouseID, variantID uuid.UUID) (*domain.WarehouseStock, error) {
	var s domain.WarehouseStock
	err := r.pool.QueryRow(ctx,
		`SELECT id, warehouse_id, variant_id, qty_on_hand, qty_reserved, qty_available, reorder_point, reorder_qty, created_at, updated_at
		   FROM warehouse_stock WHERE warehouse_id = $1 AND variant_id = $2`,
		warehouseID, variantID,
	).Scan(&s.ID, &s.WarehouseID, &s.VariantID, &s.QtyOnHand, &s.QtyReserved, &s.QtyAvailable, &s.ReorderPoint, &s.ReorderQty, &s.CreatedAt, &s.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("stock not found")
	}
	return &s, err
}

// InsertMovementTx inserts an inventory_movements row inside a transaction.
// Reuses the existing movement table so the full ledger stays in one place.
func (r *WarehouseRepository) InsertMovementTx(ctx context.Context, tx pgx.Tx, m *domain.InventoryMovement) error {
	m.ID = uuid.New()
	m.CreatedAt = time.Now().UTC()
	_, err := tx.Exec(ctx,
		`INSERT INTO inventory_movements
		    (id, variant_id, movement_type, quantity, quantity_before, quantity_after,
		     unit_cost_snapshot, notes, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		m.ID, m.VariantID, string(m.MovementType),
		m.Quantity, m.QuantityBefore, m.QuantityAfter,
		m.UnitCostSnapshot, m.Notes, m.CreatedAt,
	)
	return err
}

func (r *WarehouseRepository) AdjustGlobalInventoryTx(ctx context.Context, tx pgx.Tx, variantID uuid.UUID, delta int) error {
	tag, err := tx.Exec(ctx,
		`UPDATE inventory
		    SET quantity_on_hand = quantity_on_hand + $2,
		        updated_at = NOW()
		  WHERE variant_id = $1`,
		variantID, delta,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("inventory row not found for variant %s", variantID)
	}
	return nil
}

func (r *WarehouseRepository) ListInventoryRows(
	ctx context.Context,
	tenantID uuid.UUID,
	warehouseID *uuid.UUID,
	product, category string,
	lowStockOnly bool,
) ([]domain.InventoryListItem, error) {
	query := `
		SELECT
			p.id AS product_id,
			COALESCE(p.name, '') AS product_name,
			v.id AS variant_id,
			TRIM(BOTH ' ' FROM CONCAT(COALESCE(v.color, ''), ' ', COALESCE(v.size, ''))) AS variant_name,
			COALESCE(v.sku, '') AS sku,
			COALESCE(p.category, '') AS category,
			w.id AS warehouse_id,
			w.name AS warehouse_name,
			v.unit_cost::text AS unit_cost,
			(ws.qty_available::numeric * COALESCE(v.unit_cost, 0))::text AS stock_value_at_cost,
			ws.qty_available,
			ws.qty_reserved,
			0 AS incoming_quantity
		FROM warehouse_stock ws
		JOIN warehouses w ON w.id = ws.warehouse_id
		JOIN variants v ON v.id = ws.variant_id
		JOIN products p ON p.id = v.product_id
		WHERE w.tenant_id = $1`
	args := []any{tenantID}
	argIdx := 2
	if warehouseID != nil {
		query += fmt.Sprintf(" AND w.id = $%d", argIdx)
		args = append(args, *warehouseID)
		argIdx++
	}
	if product != "" {
		query += fmt.Sprintf(" AND (p.name ILIKE $%d OR v.sku ILIKE $%d)", argIdx, argIdx)
		args = append(args, "%"+product+"%")
		argIdx++
	}
	if category != "" {
		query += fmt.Sprintf(" AND p.category ILIKE $%d", argIdx)
		args = append(args, "%"+category+"%")
		argIdx++
	}
	if lowStockOnly {
		query += " AND ws.qty_available <= GREATEST(ws.reorder_point, 5)"
	}
	query += " ORDER BY p.name ASC, v.sku ASC, w.name ASC"

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]domain.InventoryListItem, 0)
	for rows.Next() {
		var it domain.InventoryListItem
		var unitCost sql.NullString
		if err := rows.Scan(
			&it.ProductID,
			&it.ProductName,
			&it.VariantID,
			&it.VariantName,
			&it.SKU,
			&it.Category,
			&it.WarehouseID,
			&it.WarehouseName,
			&unitCost,
			&it.StockValueAtCost,
			&it.AvailableQuantity,
			&it.ReservedQuantity,
			&it.IncomingQuantity,
		); err != nil {
			return nil, err
		}
		if unitCost.Valid && strings.TrimSpace(unitCost.String) != "" {
			s := strings.TrimSpace(unitCost.String)
			it.UnitCost = &s
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

func (r *WarehouseRepository) InsertTransfer(ctx context.Context, t *domain.InventoryTransfer) error {
	if t.ID == uuid.Nil {
		t.ID = uuid.New()
	}
	tagsJSON, _ := json.Marshal(t.Tags)
	_, err := r.pool.Exec(ctx, `
		INSERT INTO inventory_transfers
			(id, tenant_id, reference, origin_warehouse_id, destination_warehouse_id, status, notes, tags, created_at, updated_at)
		VALUES
			($1,$2,$3,$4,$5,$6,$7,$8,NOW(),NOW())
	`, t.ID, t.TenantID, t.Reference, t.OriginWarehouseID, t.DestinationWarehouseID, t.Status, t.Notes, tagsJSON)
	return err
}

func (r *WarehouseRepository) ReplaceTransferItems(ctx context.Context, transferID uuid.UUID, items []domain.TransferItem) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `DELETE FROM inventory_transfer_items WHERE transfer_id = $1`, transferID); err != nil {
		return err
	}
	for _, it := range items {
		id := it.ID
		if id == uuid.Nil {
			id = uuid.New()
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO inventory_transfer_items (id, transfer_id, variant_id, quantity, created_at)
			VALUES ($1,$2,$3,$4,NOW())
		`, id, transferID, it.VariantID, it.Quantity); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (r *WarehouseRepository) ListTransfers(ctx context.Context, tenantID uuid.UUID) ([]domain.InventoryTransfer, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT t.id, t.tenant_id, t.reference, t.origin_warehouse_id, t.destination_warehouse_id, t.status, t.notes, t.tags, t.created_at, t.updated_at,
		       COALESCE(i.total_items, 0) AS total_items
		  FROM inventory_transfers t
		  LEFT JOIN LATERAL (
		    SELECT COUNT(*)::int AS total_items
		      FROM inventory_transfer_items ti
		     WHERE ti.transfer_id = t.id
		  ) i ON TRUE
		 WHERE tenant_id = $1
		 ORDER BY t.created_at DESC
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.InventoryTransfer, 0)
	for rows.Next() {
		var tr domain.InventoryTransfer
		var tagsJSON []byte
		if err := rows.Scan(&tr.ID, &tr.TenantID, &tr.Reference, &tr.OriginWarehouseID, &tr.DestinationWarehouseID, &tr.Status, &tr.Notes, &tagsJSON, &tr.CreatedAt, &tr.UpdatedAt, &tr.TotalItems); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(tagsJSON, &tr.Tags)
		out = append(out, tr)
	}
	return out, rows.Err()
}

func (r *WarehouseRepository) GetTransferByID(ctx context.Context, tenantID, transferID uuid.UUID) (*domain.InventoryTransfer, error) {
	var tr domain.InventoryTransfer
	var tagsJSON []byte
	err := r.pool.QueryRow(ctx, `
		SELECT id, tenant_id, reference, origin_warehouse_id, destination_warehouse_id, status, notes, tags, created_at, updated_at
		  FROM inventory_transfers
		 WHERE tenant_id = $1 AND id = $2
	`, tenantID, transferID).Scan(&tr.ID, &tr.TenantID, &tr.Reference, &tr.OriginWarehouseID, &tr.DestinationWarehouseID, &tr.Status, &tr.Notes, &tagsJSON, &tr.CreatedAt, &tr.UpdatedAt)
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(tagsJSON, &tr.Tags)
	return &tr, nil
}

func (r *WarehouseRepository) GetTransferItems(ctx context.Context, transferID uuid.UUID) ([]domain.TransferItem, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, transfer_id, variant_id, quantity
		  FROM inventory_transfer_items
		 WHERE transfer_id = $1
		 ORDER BY created_at ASC
	`, transferID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.TransferItem, 0)
	for rows.Next() {
		var it domain.TransferItem
		if err := rows.Scan(&it.ID, &it.TransferID, &it.VariantID, &it.Quantity); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

func (r *WarehouseRepository) UpdateTransferMeta(ctx context.Context, t *domain.InventoryTransfer) error {
	tagsJSON, _ := json.Marshal(t.Tags)
	_, err := r.pool.Exec(ctx, `
		UPDATE inventory_transfers
		   SET reference = $2,
		       origin_warehouse_id = $3,
		       destination_warehouse_id = $4,
		       notes = $5,
		       tags = $6,
		       updated_at = NOW()
		 WHERE id = $1
	`, t.ID, t.Reference, t.OriginWarehouseID, t.DestinationWarehouseID, t.Notes, tagsJSON)
	return err
}

func (r *WarehouseRepository) UpdateTransferStatus(ctx context.Context, transferID uuid.UUID, status domain.TransferStatus) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE inventory_transfers
		   SET status = $2, updated_at = NOW()
		 WHERE id = $1
	`, transferID, status)
	return err
}
