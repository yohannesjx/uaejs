package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/dubai-retail/os/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
)

// POSRepository handles all POS DB operations.
type POSRepository struct {
	pool *pgxpool.Pool
}

// ── Registers ─────────────────────────────────────────────────────────────────

// GetRegisterByID returns a POS register.
func (r *POSRepository) GetRegisterByID(ctx context.Context, id uuid.UUID) (*domain.POSRegister, error) {
	var reg domain.POSRegister
	err := r.pool.QueryRow(ctx,
		`SELECT id, name, location, is_active, created_at, updated_at FROM pos_registers WHERE id = $1`,
		id,
	).Scan(&reg.ID, &reg.Name, &reg.Location, &reg.IsActive, &reg.CreatedAt, &reg.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("register not found")
		}
		return nil, fmt.Errorf("GetRegisterByID: %w", err)
	}
	return &reg, nil
}

// ── Sessions ─────────────────────────────────────────────────────────────────

// InsertSession creates a new POS session.
func (r *POSRepository) InsertSession(ctx context.Context, s *domain.POSSession) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO pos_sessions (id, register_id, opened_by, opened_at, opening_cash)
		 VALUES ($1, $2, $3, $4, $5)`,
		s.ID, s.RegisterID, s.OpenedBy, s.OpenedAt, s.OpeningCash,
	)
	return err
}

// CloseSession stamps the closed_at and closing_cash on a session.
func (r *POSRepository) CloseSession(ctx context.Context, sessionID uuid.UUID, closingCash decimal.Decimal) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE pos_sessions SET closed_at = NOW(), closing_cash = $2 WHERE id = $1`,
		sessionID, closingCash,
	)
	return err
}

// GetSessionByID returns a session.
func (r *POSRepository) GetSessionByID(ctx context.Context, id uuid.UUID) (*domain.POSSession, error) {
	var s domain.POSSession
	err := r.pool.QueryRow(ctx,
		`SELECT id, register_id, opened_by, opened_at, closed_at, opening_cash, closing_cash
		   FROM pos_sessions WHERE id = $1`, id,
	).Scan(&s.ID, &s.RegisterID, &s.OpenedBy, &s.OpenedAt, &s.ClosedAt, &s.OpeningCash, &s.ClosingCash)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("session not found")
		}
		return nil, fmt.Errorf("GetSessionByID: %w", err)
	}
	return &s, nil
}

// ── Payments ─────────────────────────────────────────────────────────────────

// InsertPayment records a POS payment.
func (r *POSRepository) InsertPayment(ctx context.Context, p *domain.POSPayment) error {
	p.ID = uuid.New()
	p.PaidAt = time.Now().UTC()
	_, err := r.pool.Exec(ctx,
		`INSERT INTO pos_payments (id, order_id, session_id, payment_method, amount, currency, reference, paid_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		p.ID, p.OrderID, p.SessionID, string(p.PaymentMethod), p.Amount, p.Currency, p.Reference, p.PaidAt,
	)
	return err
}

// GetPaymentsByOrderID fetches all payments for an order.
func (r *POSRepository) GetPaymentsByOrderID(ctx context.Context, orderID uuid.UUID) ([]domain.POSPayment, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, order_id, session_id, payment_method, amount, currency, reference, paid_at
		   FROM pos_payments WHERE order_id = $1 ORDER BY paid_at`, orderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var payments []domain.POSPayment
	for rows.Next() {
		var p domain.POSPayment
		var pm string
		if err := rows.Scan(&p.ID, &p.OrderID, &p.SessionID, &pm, &p.Amount, &p.Currency, &p.Reference, &p.PaidAt); err != nil {
			return nil, err
		}
		p.PaymentMethod = domain.POSPaymentMethod(pm)
		payments = append(payments, p)
	}
	return payments, rows.Err()
}

// ── Barcode lookup ────────────────────────────────────────────────────────────

// GetVariantByBarcode returns the variant + product info for a scanned barcode.
func (r *POSRepository) GetVariantByBarcode(ctx context.Context, barcode string) (*domain.Variant, *domain.Product, error) {
	var v domain.Variant
	var p domain.Product
	var vatType string
	err := r.pool.QueryRow(ctx, `
		SELECT v.id, v.sku, v.barcode, v.color, v.size, v.image_url, v.is_active,
		       p.id, p.name, p.name_ar, p.brand, p.category, p.vat_type, p.status
		  FROM variants v
		  JOIN products p ON p.id = v.product_id
		 WHERE v.barcode = $1 AND v.is_active = TRUE AND p.status = 'active'`,
		barcode,
	).Scan(
		&v.ID, &v.SKU, &v.Barcode, &v.Color, &v.Size, &v.ImageURL, &v.IsActive,
		&p.ID, &p.Name, &p.NameAR, &p.Brand, &p.Category, &vatType, &p.Status,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil, fmt.Errorf("barcode not found")
		}
		return nil, nil, fmt.Errorf("GetVariantByBarcode: %w", err)
	}
	p.VATType = domain.VATType(vatType)
	v.ProductID = p.ID
	return &v, &p, nil
}

// GetAvailableStockForVariant returns quantity_available for a variant.
func (r *POSRepository) GetAvailableStockForVariant(ctx context.Context, variantID uuid.UUID) (int, error) {
	var qty int
	err := r.pool.QueryRow(ctx,
		`SELECT COALESCE(quantity_available, 0) FROM inventory WHERE variant_id = $1`,
		variantID,
	).Scan(&qty)
	if err == pgx.ErrNoRows {
		return 0, nil
	}
	return qty, err
}

// GetPOSChannelID returns the channel_id of the first active POS-type channel.
func (r *POSRepository) GetPOSChannelID(ctx context.Context) (uuid.UUID, error) {
	var id uuid.UUID
	err := r.pool.QueryRow(ctx,
		`SELECT id FROM channels WHERE type = 'pos' AND is_active = TRUE LIMIT 1`,
	).Scan(&id)
	if err != nil {
		if err == pgx.ErrNoRows {
			return uuid.Nil, fmt.Errorf("no active POS channel found — ensure a 'pos' channel is seeded")
		}
		return uuid.Nil, fmt.Errorf("GetPOSChannelID: %w", err)
	}
	return id, nil
}

// GetPOSChannelPriceForVariant returns the POS price for a variant.
func (r *POSRepository) GetPOSChannelPriceForVariant(ctx context.Context, variantID, channelID uuid.UUID) (decimal.Decimal, string, error) {
	var price decimal.Decimal
	var currency string
	err := r.pool.QueryRow(ctx,
		`SELECT price, currency FROM channel_prices
		  WHERE variant_id = $1 AND channel_id = $2 AND is_active = TRUE
		  ORDER BY effective_from DESC LIMIT 1`,
		variantID, channelID,
	).Scan(&price, &currency)
	if err == pgx.ErrNoRows {
		return decimal.Zero, "AED", fmt.Errorf("no POS price set for this variant")
	}
	return price, currency, err
}

// ── Receipt helper ────────────────────────────────────────────────────────────

// GetOrderItemsForReceipt returns the line items needed for a POS receipt.
func (r *POSRepository) GetOrderItemsForReceipt(ctx context.Context, orderID uuid.UUID) ([]domain.POSReceiptItem, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT oi.quantity, oi.unit_price, oi.line_total, v.sku, p.name
		  FROM order_items oi
		  JOIN variants v ON v.id = oi.variant_id
		  JOIN products p ON p.id = v.product_id
		 WHERE oi.order_id = $1 ORDER BY oi.created_at`, orderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []domain.POSReceiptItem
	for rows.Next() {
		var ri domain.POSReceiptItem
		if err := rows.Scan(&ri.Qty, &ri.UnitPrice, &ri.LineTotal, &ri.SKU, &ri.Name); err != nil {
			return nil, err
		}
		items = append(items, ri)
	}
	return items, rows.Err()
}
