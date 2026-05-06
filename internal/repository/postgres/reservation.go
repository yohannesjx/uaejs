package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ReservationRepository handles stock_reservations queries needed by workers.
type ReservationRepository struct {
	pool *pgxpool.Pool
}

// ExpiredReservation is a lightweight projection returned by
// ListExpiredActive, containing only what the cleanup job needs.
type ExpiredReservation struct {
	ID        uuid.UUID
	VariantID uuid.UUID
	OrderID   uuid.UUID
	Quantity  int
	ExpiresAt time.Time
}

// ListExpiredActive returns all reservations that passed their expiry time
// but are still marked active. The caller should process and release each one.
// Batches results to avoid large memory allocations.
func (r *ReservationRepository) ListExpiredActive(
	ctx context.Context,
	batchSize int,
) ([]ExpiredReservation, error) {
	if batchSize <= 0 {
		batchSize = 100
	}

	const q = `
		SELECT id, variant_id, order_id, quantity, expires_at
		  FROM stock_reservations
		 WHERE is_active  = TRUE
		   AND expires_at < NOW() AT TIME ZONE 'UTC'
		 ORDER BY expires_at ASC
		 LIMIT $1`

	rows, err := r.pool.Query(ctx, q, batchSize)
	if err != nil {
		return nil, fmt.Errorf("ListExpiredActive: %w", err)
	}
	defer rows.Close()

	var results []ExpiredReservation
	for rows.Next() {
		var er ExpiredReservation
		if err := rows.Scan(&er.ID, &er.VariantID, &er.OrderID, &er.Quantity, &er.ExpiresAt); err != nil {
			return nil, fmt.Errorf("ListExpiredActive scan: %w", err)
		}
		results = append(results, er)
	}
	return results, rows.Err()
}

// CountActiveReservations returns the total unexpired active reservation count
// for Prometheus gauge update.
func (r *ReservationRepository) CountActiveReservations(ctx context.Context) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM stock_reservations
		 WHERE is_active = TRUE AND expires_at > NOW() AT TIME ZONE 'UTC'`,
	).Scan(&count)
	return count, err
}

// CountPendingInvoices returns orders that are confirmed but have no invoice_number.
func (r *ReservationRepository) CountPendingInvoices(ctx context.Context) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM orders
		 WHERE status = 'confirmed'
		   AND invoice_number IS NULL`,
	).Scan(&count)
	return count, err
}
