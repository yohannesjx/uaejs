package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/dubai-retail/os/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ShippingRepository handles all shipping DB operations.
type ShippingRepository struct {
	pool *pgxpool.Pool
}

// ── Providers ─────────────────────────────────────────────────────────────────

// ListActiveProviders returns all active shipping providers.
func (r *ShippingRepository) ListActiveProviders(ctx context.Context) ([]domain.ShippingProvider, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, name, type, is_active, created_at FROM shipping_providers WHERE is_active = TRUE ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var providers []domain.ShippingProvider
	for rows.Next() {
		var p domain.ShippingProvider
		if err := rows.Scan(&p.ID, &p.Name, &p.Type, &p.IsActive, &p.CreatedAt); err != nil {
			return nil, err
		}
		providers = append(providers, p)
	}
	return providers, rows.Err()
}

// GetProviderByType returns a provider by its connector key.
func (r *ShippingRepository) GetProviderByType(ctx context.Context, providerType string) (*domain.ShippingProvider, error) {
	var p domain.ShippingProvider
	err := r.pool.QueryRow(ctx,
		`SELECT id, name, type, is_active, created_at FROM shipping_providers WHERE type = $1`,
		providerType,
	).Scan(&p.ID, &p.Name, &p.Type, &p.IsActive, &p.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("shipping provider %q not found", providerType)
	}
	return &p, err
}

// ── Accounts ─────────────────────────────────────────────────────────────────

// InsertAccount persists a new shipping account.
func (r *ShippingRepository) InsertAccount(ctx context.Context, a *domain.ShippingAccount) error {
	a.ID = uuid.New()
	a.CreatedAt = time.Now().UTC()
	a.UpdatedAt = a.CreatedAt

	settingsJSON, _ := json.Marshal(a.Settings)
	_, err := r.pool.Exec(ctx,
		`INSERT INTO shipping_accounts (id, provider_id, label, api_key, api_secret, settings, is_active, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8)`,
		a.ID, a.ProviderID, a.Label, a.APIKey, a.APISecret, settingsJSON, a.IsActive, a.CreatedAt,
	)
	return err
}

// GetActiveAccountForProvider returns the first active account for a provider.
func (r *ShippingRepository) GetActiveAccountForProvider(ctx context.Context, providerID uuid.UUID) (*domain.ShippingAccount, error) {
	return r.scanAccount(ctx,
		`SELECT id, provider_id, label, api_key, api_secret, settings, is_active, created_at, updated_at
		   FROM shipping_accounts WHERE provider_id = $1 AND is_active = TRUE LIMIT 1`,
		providerID,
	)
}

// GetAccountByID returns a shipping account.
func (r *ShippingRepository) GetAccountByID(ctx context.Context, id uuid.UUID) (*domain.ShippingAccount, error) {
	return r.scanAccount(ctx,
		`SELECT id, provider_id, label, api_key, api_secret, settings, is_active, created_at, updated_at
		   FROM shipping_accounts WHERE id = $1`,
		id,
	)
}

func (r *ShippingRepository) scanAccount(ctx context.Context, q string, arg any) (*domain.ShippingAccount, error) {
	var a domain.ShippingAccount
	var settingsRaw []byte
	err := r.pool.QueryRow(ctx, q, arg).Scan(
		&a.ID, &a.ProviderID, &a.Label, &a.APIKey, &a.APISecret,
		&settingsRaw, &a.IsActive, &a.CreatedAt, &a.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("shipping account not found")
	}
	if err != nil {
		return nil, fmt.Errorf("ShippingAccount scan: %w", err)
	}
	_ = json.Unmarshal(settingsRaw, &a.Settings)
	return &a, nil
}

// ── Shipments ─────────────────────────────────────────────────────────────────

// InsertShipment creates a new shipment record.
func (r *ShippingRepository) InsertShipment(ctx context.Context, s *domain.Shipment) error {
	s.ID = uuid.New()
	s.Status = domain.ShipmentPending
	s.CreatedAt = time.Now().UTC()
	s.UpdatedAt = s.CreatedAt

	_, err := r.pool.Exec(ctx,
		`INSERT INTO shipments (id, order_id, account_id, tracking_number, carrier_ref, status, weight_g, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8)`,
		s.ID, s.OrderID, s.AccountID, s.TrackingNumber, s.CarrierRef,
		string(s.Status), s.WeightG, s.CreatedAt,
	)
	return err
}

// UpdateShipmentStatus updates the shipment status and optional tracking info.
func (r *ShippingRepository) UpdateShipmentStatus(ctx context.Context, shipmentID uuid.UUID, status domain.ShipmentStatus, trackingNumber, carrierRef *string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE shipments SET status = $2,
		    tracking_number = COALESCE($3, tracking_number),
		    carrier_ref = COALESCE($4, carrier_ref),
		    updated_at = NOW()
		  WHERE id = $1`,
		shipmentID, string(status), trackingNumber, carrierRef,
	)
	return err
}

// GetShipmentByID returns a shipment with its events.
func (r *ShippingRepository) GetShipmentByID(ctx context.Context, id uuid.UUID) (*domain.Shipment, error) {
	var s domain.Shipment
	err := r.pool.QueryRow(ctx,
		`SELECT id, order_id, account_id, tracking_number, carrier_ref, status, weight_g, created_at, updated_at
		   FROM shipments WHERE id = $1`, id,
	).Scan(&s.ID, &s.OrderID, &s.AccountID, &s.TrackingNumber, &s.CarrierRef,
		(*string)(&s.Status), &s.WeightG, &s.CreatedAt, &s.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("shipment not found")
	}
	if err != nil {
		return nil, fmt.Errorf("GetShipmentByID: %w", err)
	}
	events, err := r.GetEventsByShipmentID(ctx, id)
	if err != nil {
		return nil, err
	}
	s.Events = events
	return &s, nil
}

// ListShipments returns paginated shipments (tenant-scoped via orders).
func (r *ShippingRepository) ListShipments(ctx context.Context, filters domain.ShipmentListFilters) ([]domain.ShipmentListItem, int, error) {
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
		where = append(where, fmt.Sprintf("s.status = $%d", argIdx))
		args = append(args, filters.Status)
		argIdx++
	}
	if filters.Carrier != "" {
		where = append(where, fmt.Sprintf("sp.type = $%d", argIdx))
		args = append(args, filters.Carrier)
		argIdx++
	}

	whereClause := strings.Join(where, " AND ")

	countQ := fmt.Sprintf(`
		SELECT COUNT(*) FROM shipments s
		JOIN orders o ON o.id = s.order_id
		LEFT JOIN shipping_accounts sa ON sa.id = s.account_id
		LEFT JOIN shipping_providers sp ON sp.id = sa.provider_id
		WHERE %s`, whereClause)
	var total int
	if err := r.pool.QueryRow(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("ListShipments count: %w", err)
	}

	args = append(args, filters.PageSize, offset)
	limitIdx := argIdx
	offsetIdx := argIdx + 1
	listQ := fmt.Sprintf(`
		SELECT s.id, s.order_id, s.account_id, s.tracking_number, s.carrier_ref,
		       s.status, sp.type, s.created_at
		  FROM shipments s
		  JOIN orders o ON o.id = s.order_id
		  LEFT JOIN shipping_accounts sa ON sa.id = s.account_id
		  LEFT JOIN shipping_providers sp ON sp.id = sa.provider_id
		 WHERE %s
		 ORDER BY s.created_at DESC
		 LIMIT $%d OFFSET $%d`, whereClause, limitIdx, offsetIdx)

	rows, err := r.pool.Query(ctx, listQ, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("ListShipments: %w", err)
	}
	defer rows.Close()

	var items []domain.ShipmentListItem
	for rows.Next() {
		var it domain.ShipmentListItem
		var carrier *string
		if err := rows.Scan(&it.ID, &it.OrderID, &it.AccountID, &it.TrackingNumber, &it.CarrierRef,
			&it.Status, &carrier, &it.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("ListShipments scan: %w", err)
		}
		it.Carrier = carrier
		items = append(items, it)
	}
	return items, total, rows.Err()
}

// GetShipmentByOrderID returns the shipment for a given order.
func (r *ShippingRepository) GetShipmentByOrderID(ctx context.Context, orderID uuid.UUID) (*domain.Shipment, error) {
	var id uuid.UUID
	err := r.pool.QueryRow(ctx, `SELECT id FROM shipments WHERE order_id = $1 ORDER BY created_at DESC LIMIT 1`, orderID).Scan(&id)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("no shipment for order")
	}
	if err != nil {
		return nil, err
	}
	return r.GetShipmentByID(ctx, id)
}

// ── Events ────────────────────────────────────────────────────────────────────

// InsertEvent appends a tracking event to the log.
func (r *ShippingRepository) InsertEvent(ctx context.Context, e *domain.ShipmentEvent) error {
	e.ID = uuid.New()
	e.RecordedAt = time.Now().UTC()
	_, err := r.pool.Exec(ctx,
		`INSERT INTO shipment_events (id, shipment_id, status, location, description, event_time, recorded_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		e.ID, e.ShipmentID, e.Status, e.Location, e.Description, e.EventTime, e.RecordedAt,
	)
	return err
}

// GetEventsByShipmentID returns all tracking events for a shipment.
func (r *ShippingRepository) GetEventsByShipmentID(ctx context.Context, shipmentID uuid.UUID) ([]domain.ShipmentEvent, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, shipment_id, status, location, description, event_time, recorded_at
		   FROM shipment_events WHERE shipment_id = $1 ORDER BY event_time DESC`, shipmentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var events []domain.ShipmentEvent
	for rows.Next() {
		var e domain.ShipmentEvent
		if err := rows.Scan(&e.ID, &e.ShipmentID, &e.Status, &e.Location, &e.Description, &e.EventTime, &e.RecordedAt); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}
