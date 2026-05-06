package postgres

import (
	"context"
	"fmt"

	"github.com/dubai-retail/os/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
)

// InvoiceStoreRepository handles persistence of the order_invoices compliance table.
type InvoiceStoreRepository struct {
	pool *pgxpool.Pool
}

// =============================================================================
// Invoice number generation
// =============================================================================

// NextInvoiceNumber calls the PostgreSQL function next_invoice_number() inside
// the given transaction to guarantee uniqueness with the order record.
func (r *InvoiceStoreRepository) NextInvoiceNumber(ctx context.Context, tx pgx.Tx) (string, error) {
	var num string
	err := tx.QueryRow(ctx, `SELECT next_invoice_number()`).Scan(&num)
	if err != nil {
		return "", fmt.Errorf("NextInvoiceNumber: %w", err)
	}
	return num, nil
}

// =============================================================================
// Writes
// =============================================================================

// InsertOrderInvoice persists a compliance record within the order transaction.
func (r *InvoiceStoreRepository) InsertOrderInvoice(
	ctx context.Context,
	tx pgx.Tx,
	oi *domain.OrderInvoice,
) error {
	const q = `
		INSERT INTO order_invoices
		    (id, order_id, invoice_type, invoice_number, xml_content,
		     exchange_rate_to_aed, trigger_reason, issued_at, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,
		        NOW() AT TIME ZONE 'UTC',
		        NOW() AT TIME ZONE 'UTC')`

	_, err := tx.Exec(ctx, q,
		oi.ID, oi.OrderID, oi.InvoiceType, oi.InvoiceNumber,
		oi.XMLContent, oi.ExchangeRateToAED, oi.TriggerReason,
	)
	if err != nil {
		return fmt.Errorf("InsertOrderInvoice(order=%s): %w", oi.OrderID, err)
	}
	return nil
}

// =============================================================================
// Reads
// =============================================================================

// GetOrderInvoice fetches the invoice record for a given order.
func (r *InvoiceStoreRepository) GetOrderInvoice(
	ctx context.Context,
	orderID uuid.UUID,
) (*domain.OrderInvoice, error) {
	const q = `
		SELECT id, order_id, invoice_type, invoice_number, xml_content,
		       exchange_rate_to_aed, trigger_reason, issued_at, created_at
		  FROM order_invoices
		 WHERE order_id = $1
		 ORDER BY created_at DESC
		 LIMIT 1`

	oi := &domain.OrderInvoice{}
	var exchangeRateStr string
	err := r.pool.QueryRow(ctx, q, orderID).Scan(
		&oi.ID, &oi.OrderID, &oi.InvoiceType, &oi.InvoiceNumber, &oi.XMLContent,
		&exchangeRateStr, &oi.TriggerReason, &oi.IssuedAt, &oi.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("GetOrderInvoice(order=%s): %w", orderID, err)
	}
	oi.ExchangeRateToAED, _ = decimal.NewFromString(exchangeRateStr)
	return oi, nil
}

// UpdateSandboxStatus persists the ASP sandbox result on an order_invoices row.
func (r *InvoiceStoreRepository) UpdateSandboxStatus(
	ctx context.Context,
	invoiceID uuid.UUID,
	status domain.SandboxStatus,
	aspRespID string,
	validationErrors []string,
) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE order_invoices
		   SET sandbox_status = $2, sandbox_resp_id = $3,
		       sandbox_errors = $4, sandbox_submitted_at = NOW()
		 WHERE id = $1`,
		invoiceID, string(status), aspRespID, validationErrors,
	)
	return err
}
