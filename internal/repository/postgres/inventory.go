package postgres

import (
	"context"
	"fmt"

	"github.com/dubai-retail/os/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)


// InventoryRepository performs all inventory and movement DB operations.
// It holds a *pgxpool.Pool so individual transactions can be started per service call.
type InventoryRepository struct {
	pool *pgxpool.Pool
}

// -----------------------------------------------------------------------------
// Reads
// -----------------------------------------------------------------------------

// GetByVariantIDForUpdate fetches the inventory row with a pessimistic lock.
// Must be called inside a transaction.
func (r *InventoryRepository) GetByVariantIDForUpdate(ctx context.Context, tx pgx.Tx, variantID uuid.UUID) (*domain.Inventory, error) {
	const q = `
		SELECT id, variant_id, quantity_on_hand, quantity_reserved, quantity_available,
		       reorder_point, reorder_qty, created_at, updated_at
		  FROM inventory
		 WHERE variant_id = $1
		   FOR UPDATE`

	row := tx.QueryRow(ctx, q, variantID)
	inv := &domain.Inventory{}
	err := row.Scan(
		&inv.ID, &inv.VariantID, &inv.QuantityOnHand,
		&inv.QuantityReserved, &inv.QuantityAvailable,
		&inv.ReorderPoint, &inv.ReorderQty,
		&inv.CreatedAt, &inv.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("inventory.GetByVariantIDForUpdate(%s): %w", variantID, err)
	}
	return inv, nil
}

// -----------------------------------------------------------------------------
// FIFO – Batch Reads
// -----------------------------------------------------------------------------

// GetFIFOBatchItems returns all batch items for a variant ordered oldest-received first,
// together with how many units have already been deducted (via inventory_movements).
// Must be called inside a transaction with FOR UPDATE on inventory.
func (r *InventoryRepository) GetFIFOBatchItems(
	ctx context.Context,
	tx pgx.Tx,
	variantID uuid.UUID,
) ([]FIFOBatchItemRow, error) {
	const q = `
		SELECT
		    bi.id                   AS batch_item_id,
		    pb.received_at          AS batch_received_at,
		    bi.landed_cost_per_unit,
		    bi.quantity_received,
		    COALESCE(
		        SUM(CASE WHEN im.movement_type = 'sale_out' THEN ABS(im.quantity) ELSE 0 END),
		        0
		    )                       AS total_deducted
		  FROM batch_items bi
		  JOIN purchase_batches pb ON pb.id = bi.batch_id
		  LEFT JOIN inventory_movements im
		         ON im.batch_item_id = bi.id
		        AND im.movement_type = 'sale_out'
		 WHERE bi.variant_id = $1
		   AND pb.received_at IS NOT NULL
		 GROUP BY bi.id, pb.received_at, bi.landed_cost_per_unit, bi.quantity_received
		HAVING bi.quantity_received > COALESCE(
		    SUM(CASE WHEN im.movement_type = 'sale_out' THEN ABS(im.quantity) ELSE 0 END), 0
		)
		 ORDER BY pb.received_at ASC
		   FOR UPDATE OF bi`

	rows, err := tx.Query(ctx, q, variantID)
	if err != nil {
		return nil, fmt.Errorf("GetFIFOBatchItems(%s): %w", variantID, err)
	}
	defer rows.Close()

	var result []FIFOBatchItemRow
	for rows.Next() {
		var row FIFOBatchItemRow
		if err := rows.Scan(
			&row.BatchItemID,
			&row.BatchReceivedAt,
			&row.LandedCostPerUnit,
			&row.QuantityReceived,
			&row.TotalDeducted,
		); err != nil {
			return nil, fmt.Errorf("GetFIFOBatchItems scan: %w", err)
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

// -----------------------------------------------------------------------------
// Writes
// -----------------------------------------------------------------------------

// DeductOnHand subtracts quantity from quantity_on_hand and returns updated row.
func (r *InventoryRepository) DeductOnHand(
	ctx context.Context,
	tx pgx.Tx,
	variantID uuid.UUID,
	qty int,
) (*domain.Inventory, error) {
	const q = `
		UPDATE inventory
		   SET quantity_on_hand  = quantity_on_hand - $2,
		       updated_at        = NOW() AT TIME ZONE 'UTC'
		 WHERE variant_id = $1
		   AND quantity_on_hand - $2 >= 0
		RETURNING id, variant_id, quantity_on_hand, quantity_reserved, quantity_available,
		          reorder_point, reorder_qty, created_at, updated_at`

	row := tx.QueryRow(ctx, q, variantID, qty)
	inv := &domain.Inventory{}
	err := row.Scan(
		&inv.ID, &inv.VariantID, &inv.QuantityOnHand,
		&inv.QuantityReserved, &inv.QuantityAvailable,
		&inv.ReorderPoint, &inv.ReorderQty,
		&inv.CreatedAt, &inv.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("insufficient stock for variant %s", variantID)
		}
		return nil, fmt.Errorf("DeductOnHand(%s, %d): %w", variantID, qty, err)
	}
	return inv, nil
}

// ReleaseReservation decrements quantity_reserved and marks the reservation inactive.
func (r *InventoryRepository) ReleaseReservation(
	ctx context.Context,
	tx pgx.Tx,
	reservationID uuid.UUID,
) error {
	const q = `
		WITH res AS (
		    UPDATE stock_reservations
		       SET is_active   = FALSE,
		           released_at = NOW() AT TIME ZONE 'UTC',
		           updated_at  = NOW() AT TIME ZONE 'UTC'
		     WHERE id        = $1
		       AND is_active = TRUE
		 RETURNING variant_id, quantity
		)
		UPDATE inventory i
		   SET quantity_reserved = quantity_reserved - res.quantity,
		       updated_at        = NOW() AT TIME ZONE 'UTC'
		  FROM res
		 WHERE i.variant_id = res.variant_id`

	_, err := tx.Exec(ctx, q, reservationID)
	return err
}

// InsertMovement writes an immutable ledger row.
func (r *InventoryRepository) InsertMovement(
	ctx context.Context,
	tx pgx.Tx,
	m *domain.InventoryMovement,
) error {
	const q = `
		INSERT INTO inventory_movements
		    (id, variant_id, batch_item_id, order_id, reservation_id,
		     movement_type, quantity, quantity_before, quantity_after,
		     unit_cost_snapshot, channel_id, reference, notes, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13, NOW() AT TIME ZONE 'UTC')`

	_, err := tx.Exec(ctx, q,
		m.ID, m.VariantID, m.BatchItemID, m.OrderID, m.ReservationID,
		m.MovementType, m.Quantity, m.QuantityBefore, m.QuantityAfter,
		m.UnitCostSnapshot, m.ChannelID, m.Reference, m.Notes,
	)
	return err
}

// InsertReservation creates a new stock reservation row and increments quantity_reserved.
func (r *InventoryRepository) InsertReservation(
	ctx context.Context,
	tx pgx.Tx,
	res *domain.StockReservation,
) error {
	const insertRes = `
		INSERT INTO stock_reservations
		    (id, order_id, variant_id, quantity, expires_at, is_active, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,TRUE, NOW() AT TIME ZONE 'UTC', NOW() AT TIME ZONE 'UTC')`

	const updateInv = `
		UPDATE inventory
		   SET quantity_reserved = quantity_reserved + $2,
		       updated_at        = NOW() AT TIME ZONE 'UTC'
		 WHERE variant_id = $1
		   AND (quantity_on_hand - quantity_reserved - $2) >= 0`

	if _, err := tx.Exec(ctx, insertRes,
		res.ID, res.OrderID, res.VariantID, res.Quantity, res.ExpiresAt,
	); err != nil {
		return fmt.Errorf("InsertReservation insert: %w", err)
	}

	tag, err := tx.Exec(ctx, updateInv, res.VariantID, res.Quantity)
	if err != nil {
		return fmt.Errorf("InsertReservation update inventory: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("insufficient available stock for variant %s", res.VariantID)
	}
	return nil
}

// AdjustStock applies a manual stock delta for returns and adjustments.
// movementType is one of: adjustment_in, adjustment_out, return_in.
func (r *InventoryRepository) AdjustStock(ctx context.Context, tx pgx.Tx, variantID uuid.UUID, delta int, movementType string, note string) error {
	// Update on-hand quantity
	_, err := tx.Exec(ctx, `
		UPDATE inventory
		   SET quantity_on_hand = quantity_on_hand + $2,
		       updated_at = NOW()
		 WHERE variant_id = $1`, variantID, delta)
	if err != nil {
		return fmt.Errorf("AdjustStock: update inventory: %w", err)
	}
	// Record the movement
	_, err = tx.Exec(ctx, `
		INSERT INTO inventory_movements
		    (id, variant_id, movement_type, quantity_change, note, created_at)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, NOW())`,
		variantID, movementType, delta, note)
	return err
}

// GetAvailableStock returns quantity_on_hand - quantity_reserved for a variant.
// Used by the omnichannel inventory sync to push live stock counts to platforms.
func (r *InventoryRepository) GetAvailableStock(ctx context.Context, variantID uuid.UUID) (int, error) {
	var available int
	err := r.pool.QueryRow(ctx, `
		SELECT GREATEST(0, quantity_on_hand - quantity_reserved)
		  FROM inventory
		 WHERE variant_id = $1`, variantID,
	).Scan(&available)
	if err == pgx.ErrNoRows {
		return 0, nil
	}
	return available, err
}
