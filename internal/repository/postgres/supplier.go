package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/dubai-retail/os/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SupplierRepository handles all supplier and purchase-order DB operations.
type SupplierRepository struct {
	pool *pgxpool.Pool
}

// ── Suppliers ─────────────────────────────────────────────────────────────────

func (r *SupplierRepository) InsertSupplier(ctx context.Context, s *domain.Supplier) error {
	now := time.Now().UTC()
	s.ID = uuid.New()
	s.CreatedAt = now
	s.UpdatedAt = now
	_, err := r.pool.Exec(ctx, `
		INSERT INTO suppliers
		    (id, name, contact_name, phone, email, country,
		     lead_time_days, minimum_order_qty, rating, notes, is_active, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$12)`,
		s.ID, s.Name, s.ContactName, s.Phone, s.Email, s.Country,
		s.LeadTimeDays, s.MinimumOrderQty, s.Rating, s.Notes, s.IsActive, now,
	)
	return err
}

func (r *SupplierRepository) UpdateSupplier(ctx context.Context, s *domain.Supplier) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE suppliers
		   SET name=$2, contact_name=$3, phone=$4, email=$5, country=$6,
		       lead_time_days=$7, minimum_order_qty=$8, rating=$9,
		       notes=$10, is_active=$11, updated_at=NOW()
		 WHERE id=$1`,
		s.ID, s.Name, s.ContactName, s.Phone, s.Email, s.Country,
		s.LeadTimeDays, s.MinimumOrderQty, s.Rating, s.Notes, s.IsActive,
	)
	return err
}

func (r *SupplierRepository) GetSupplierByID(ctx context.Context, id uuid.UUID) (*domain.Supplier, error) {
	var s domain.Supplier
	err := r.pool.QueryRow(ctx, `
		SELECT id, name, contact_name, phone, email, country,
		       lead_time_days, minimum_order_qty, rating, notes, is_active, created_at, updated_at
		  FROM suppliers WHERE id=$1`, id,
	).Scan(
		&s.ID, &s.Name, &s.ContactName, &s.Phone, &s.Email, &s.Country,
		&s.LeadTimeDays, &s.MinimumOrderQty, &s.Rating, &s.Notes, &s.IsActive,
		&s.CreatedAt, &s.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("supplier not found: %s", id)
	}
	return &s, err
}

func (r *SupplierRepository) ListSuppliers(ctx context.Context) ([]domain.Supplier, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, name, contact_name, phone, email, country,
		       lead_time_days, minimum_order_qty, rating, notes, is_active, created_at, updated_at
		  FROM suppliers WHERE is_active=TRUE ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Supplier
	for rows.Next() {
		var s domain.Supplier
		if err := rows.Scan(
			&s.ID, &s.Name, &s.ContactName, &s.Phone, &s.Email, &s.Country,
			&s.LeadTimeDays, &s.MinimumOrderQty, &s.Rating, &s.Notes, &s.IsActive,
			&s.CreatedAt, &s.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// ── Purchase Orders ───────────────────────────────────────────────────────────

func (r *SupplierRepository) InsertPurchaseOrder(ctx context.Context, tx pgx.Tx, po *domain.PurchaseOrder) error {
	now := time.Now().UTC()
	po.ID = uuid.New()
	po.CreatedAt = now
	po.UpdatedAt = now
	if po.Currency == "" {
		po.Currency = "AED"
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO purchase_orders
		    (id, supplier_id, status, reference_number, notes, total_cost, currency, expected_at, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$9)`,
		po.ID, po.SupplierID, po.Status, po.ReferenceNumber, po.Notes,
		po.TotalCost, po.Currency, po.ExpectedAt, now,
	)
	return err
}

func (r *SupplierRepository) InsertPOItem(ctx context.Context, tx pgx.Tx, item *domain.POItem) error {
	item.ID = uuid.New()
	item.CreatedAt = time.Now().UTC()
	_, err := tx.Exec(ctx, `
		INSERT INTO purchase_order_items
		    (id, purchase_order_id, variant_id, quantity, unit_cost, created_at)
		VALUES ($1,$2,$3,$4,$5,$6)`,
		item.ID, item.PurchaseOrderID, item.VariantID,
		item.Quantity, item.UnitCost, item.CreatedAt,
	)
	return err
}

func (r *SupplierRepository) GetPurchaseOrderByID(ctx context.Context, id uuid.UUID) (*domain.PurchaseOrder, error) {
	var po domain.PurchaseOrder
	err := r.pool.QueryRow(ctx, `
		SELECT id, supplier_id, status, reference_number, notes,
		       total_cost, currency, expected_at, received_at, created_at, updated_at
		  FROM purchase_orders WHERE id=$1`, id,
	).Scan(
		&po.ID, &po.SupplierID, &po.Status, &po.ReferenceNumber, &po.Notes,
		&po.TotalCost, &po.Currency, &po.ExpectedAt, &po.ReceivedAt,
		&po.CreatedAt, &po.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("purchase order not found: %s", id)
	}
	if err != nil {
		return nil, err
	}

	items, err := r.GetPOItems(ctx, id)
	if err != nil {
		return nil, err
	}
	po.Items = items
	return &po, nil
}

func (r *SupplierRepository) GetPOItems(ctx context.Context, poID uuid.UUID) ([]domain.POItem, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, purchase_order_id, variant_id, quantity, unit_cost, received_qty, created_at
		  FROM purchase_order_items WHERE purchase_order_id=$1`, poID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []domain.POItem
	for rows.Next() {
		var it domain.POItem
		if err := rows.Scan(
			&it.ID, &it.PurchaseOrderID, &it.VariantID,
			&it.Quantity, &it.UnitCost, &it.ReceivedQty, &it.CreatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

func (r *SupplierRepository) ListPurchaseOrders(ctx context.Context) ([]domain.PurchaseOrder, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, supplier_id, status, reference_number, notes,
		       total_cost, currency, expected_at, received_at, created_at, updated_at
		  FROM purchase_orders ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.PurchaseOrder
	for rows.Next() {
		var po domain.PurchaseOrder
		if err := rows.Scan(
			&po.ID, &po.SupplierID, &po.Status, &po.ReferenceNumber, &po.Notes,
			&po.TotalCost, &po.Currency, &po.ExpectedAt, &po.ReceivedAt,
			&po.CreatedAt, &po.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, po)
	}
	return out, rows.Err()
}

func (r *SupplierRepository) UpdatePOStatus(ctx context.Context, tx pgx.Tx, poID uuid.UUID, status domain.POStatus, receivedAt *time.Time) error {
	_, err := tx.Exec(ctx, `
		UPDATE purchase_orders
		   SET status=$2, received_at=$3, updated_at=NOW()
		 WHERE id=$1`, poID, status, receivedAt)
	return err
}

func (r *SupplierRepository) UpdatePOItemReceivedQty(ctx context.Context, tx pgx.Tx, itemID uuid.UUID, qty int) error {
	_, err := tx.Exec(ctx, `
		UPDATE purchase_order_items
		   SET received_qty = received_qty + $2
		 WHERE id=$1`, itemID, qty)
	return err
}
