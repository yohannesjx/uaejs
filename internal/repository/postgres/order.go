package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dubai-retail/os/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// OrderRepository handles order and order item persistence.
type OrderRepository struct {
	pool *pgxpool.Pool
}

// =============================================================================
// Orders
// =============================================================================

// InsertOrder persists a new order row inside the given transaction.
func (r *OrderRepository) InsertOrder(ctx context.Context, tx pgx.Tx, o *domain.Order) error {
	addrJSON, err := marshalNullableJSON(o.ShippingAddress)
	if err != nil {
		return fmt.Errorf("InsertOrder: marshal address: %w", err)
	}

	const q = `
		INSERT INTO orders
		    (id, channel_id, customer_name, customer_email, customer_phone,
		     customer_trn, shipping_address, subtotal, discount_amount,
		     vat_amount, total_amount, currency, vat_type, status,
		     payment_status, notes, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,
		        NOW() AT TIME ZONE 'UTC', NOW() AT TIME ZONE 'UTC')`

	_, err = tx.Exec(ctx, q,
		o.ID, o.ChannelID, o.CustomerName, o.CustomerEmail, o.CustomerPhone,
		o.CustomerTRN, addrJSON, o.Subtotal, o.DiscountAmount,
		o.VATAmount, o.TotalAmount, o.Currency, o.VATType, o.Status,
		o.PaymentStatus, o.Notes,
	)
	if err != nil {
		return fmt.Errorf("InsertOrder(%s): %w", o.ID, err)
	}
	return nil
}

// InsertOrderItem persists one order line inside the given transaction.
func (r *OrderRepository) InsertOrderItem(ctx context.Context, tx pgx.Tx, item *domain.OrderItem) error {
	const q = `
		INSERT INTO order_items
		    (id, order_id, variant_id, quantity, unit_price, discount_amount,
		     vat_rate, vat_amount, line_total, cogs_per_unit, total_cogs,
		     created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,
		        NOW() AT TIME ZONE 'UTC', NOW() AT TIME ZONE 'UTC')`

	_, err := tx.Exec(ctx, q,
		item.ID, item.OrderID, item.VariantID, item.Quantity,
		item.UnitPrice, item.DiscountAmount, item.VATRate,
		item.VATAmount, item.LineTotal, item.COGSPerUnit, item.TotalCOGS,
	)
	if err != nil {
		return fmt.Errorf("InsertOrderItem(%s): %w", item.ID, err)
	}
	return nil
}

// GetOrderByID fetches an order and all its line items.
func (r *OrderRepository) GetOrderByID(ctx context.Context, id uuid.UUID) (*domain.Order, error) {
	const oq = `
		SELECT id, channel_id, customer_name, customer_email, customer_phone,
		       customer_trn, shipping_address, subtotal, discount_amount,
		       vat_amount, total_amount, currency, vat_type,
		       invoice_number, invoice_issued_at, status, payment_status,
		       notes, created_at, updated_at
		  FROM orders
		 WHERE id = $1`

	o := &domain.Order{}
	var addrJSON []byte
	err := r.pool.QueryRow(ctx, oq, id).Scan(
		&o.ID, &o.ChannelID, &o.CustomerName, &o.CustomerEmail, &o.CustomerPhone,
		&o.CustomerTRN, &addrJSON, &o.Subtotal, &o.DiscountAmount,
		&o.VATAmount, &o.TotalAmount, &o.Currency, &o.VATType,
		&o.InvoiceNumber, &o.InvoiceIssuedAt, &o.Status, &o.PaymentStatus,
		&o.Notes, &o.CreatedAt, &o.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("GetOrderByID(%s): %w", id, err)
	}
	if len(addrJSON) > 0 {
		if err := json.Unmarshal(addrJSON, &o.ShippingAddress); err != nil {
			return nil, fmt.Errorf("GetOrderByID: unmarshal address: %w", err)
		}
	}

	items, err := r.listOrderItems(ctx, id)
	if err != nil {
		return nil, err
	}
	o.Items = items
	return o, nil
}

// UpdateOrderStatus updates the status of an order atomically.
func (r *OrderRepository) UpdateOrderStatus(
	ctx context.Context, tx pgx.Tx,
	orderID uuid.UUID, status domain.OrderStatus,
) error {
	const q = `
		UPDATE orders
		   SET status     = $2,
		       updated_at = NOW() AT TIME ZONE 'UTC'
		 WHERE id = $1`
	_, err := tx.Exec(ctx, q, orderID, status)
	return err
}

// StampInvoiceNumber sets the invoice number and issued_at timestamp.
func (r *OrderRepository) StampInvoiceNumber(
	ctx context.Context, tx pgx.Tx,
	orderID uuid.UUID, invoiceNumber string,
) error {
	const q = `
		UPDATE orders
		   SET invoice_number   = $2,
		       invoice_issued_at = NOW() AT TIME ZONE 'UTC',
		       updated_at        = NOW() AT TIME ZONE 'UTC'
		 WHERE id = $1`
	_, err := tx.Exec(ctx, q, orderID, invoiceNumber)
	return err
}

// ListOrders returns paginated orders with channel info.
func (r *OrderRepository) ListOrders(ctx context.Context, filters domain.OrderListFilters) ([]domain.OrderListItem, int, error) {
	if filters.Page < 1 {
		filters.Page = 1
	}
	if filters.PageSize < 1 || filters.PageSize > 100 {
		filters.PageSize = 25
	}
	offset := (filters.Page - 1) * filters.PageSize

	args := []any{filters.TenantID}
	where := []string{"o.tenant_id = $1"}
	argIdx := 2
	if filters.Status != "" {
		where = append(where, fmt.Sprintf("o.status = $%d", argIdx))
		args = append(args, filters.Status)
		argIdx++
	}
	if filters.Channel != "" {
		where = append(where, fmt.Sprintf("c.type::text = $%d", argIdx))
		args = append(args, filters.Channel)
		argIdx++
	}
	if filters.DateFrom != nil {
		where = append(where, fmt.Sprintf("o.created_at >= $%d", argIdx))
		args = append(args, *filters.DateFrom)
		argIdx++
	}
	if filters.DateTo != nil {
		where = append(where, fmt.Sprintf("o.created_at <= $%d", argIdx))
		args = append(args, *filters.DateTo)
		argIdx++
	}
	if filters.CustomerID != nil {
		where = append(where, fmt.Sprintf("o.customer_id = $%d", argIdx))
		args = append(args, *filters.CustomerID)
		argIdx++
	}

	whereClause := strings.Join(where, " AND ")

	countQ := fmt.Sprintf(`
		SELECT COUNT(*) FROM orders o
		JOIN channels c ON c.id = o.channel_id
		WHERE %s`, whereClause)
	var total int
	if err := r.pool.QueryRow(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("ListOrders count: %w", err)
	}

	args = append(args, filters.PageSize, offset)
	limitIdx := argIdx
	offsetIdx := argIdx + 1
	listQ := fmt.Sprintf(`
		SELECT o.id, o.channel_id, c.name, c.type::text, o.customer_id,
		       o.customer_name, o.customer_email, o.total_amount, o.currency,
		       o.status::text, o.payment_status::text, o.created_at
		  FROM orders o
		  JOIN channels c ON c.id = o.channel_id
		 WHERE %s
		 ORDER BY o.created_at DESC
		 LIMIT $%d OFFSET $%d`, whereClause, limitIdx, offsetIdx)

	rows, err := r.pool.Query(ctx, listQ, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("ListOrders: %w", err)
	}
	defer rows.Close()

	var items []domain.OrderListItem
	for rows.Next() {
		var it domain.OrderListItem
		if err := rows.Scan(&it.ID, &it.ChannelID, &it.ChannelName, &it.ChannelType,
			&it.CustomerID, &it.CustomerName, &it.CustomerEmail,
			&it.TotalAmount, &it.Currency, &it.Status, &it.PaymentStatus, &it.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("ListOrders scan: %w", err)
		}
		items = append(items, it)
	}
	return items, total, rows.Err()
}

// listOrderItems is a private helper that fetches all items for an order.
func (r *OrderRepository) listOrderItems(ctx context.Context, orderID uuid.UUID) ([]domain.OrderItem, error) {
	const q = `
		SELECT id, order_id, variant_id, quantity, unit_price, discount_amount,
		       vat_rate, vat_amount, line_total, cogs_per_unit, total_cogs,
		       created_at, updated_at
		  FROM order_items
		 WHERE order_id = $1
		 ORDER BY created_at`

	rows, err := r.pool.Query(ctx, q, orderID)
	if err != nil {
		return nil, fmt.Errorf("listOrderItems(%s): %w", orderID, err)
	}
	defer rows.Close()

	var items []domain.OrderItem
	for rows.Next() {
		item := domain.OrderItem{}
		if err := rows.Scan(
			&item.ID, &item.OrderID, &item.VariantID, &item.Quantity,
			&item.UnitPrice, &item.DiscountAmount, &item.VATRate,
			&item.VATAmount, &item.LineTotal, &item.COGSPerUnit, &item.TotalCOGS,
			&item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// =============================================================================
// Helpers
// =============================================================================

func marshalNullableJSON(v any) ([]byte, error) {
	if v == nil {
		return nil, nil
	}
	return json.Marshal(v)
}
