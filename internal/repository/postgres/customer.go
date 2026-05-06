package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/dubai-retail/os/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// CustomerRepository handles all customer and loyalty DB operations.
type CustomerRepository struct {
	pool *pgxpool.Pool
}

// ── Customers ─────────────────────────────────────────────────────────────────

// InsertCustomer creates a new customer record.
func (r *CustomerRepository) InsertCustomer(ctx context.Context, c *domain.Customer) error {
	c.ID = uuid.New()
	now := time.Now().UTC()
	c.CreatedAt = now
	c.UpdatedAt = now
	_, err := r.pool.Exec(ctx,
		`INSERT INTO customers (id, tenant_id, email, phone, full_name, loyalty_tier, is_active, notes, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $9)`,
		c.ID, c.TenantID, c.Email, c.Phone, c.FullName, string(c.LoyaltyTier), c.IsActive, c.Notes, now,
	)
	return err
}

// GetCustomerByID returns a customer by primary key.
func (r *CustomerRepository) GetCustomerByID(ctx context.Context, id uuid.UUID) (*domain.Customer, error) {
	return r.scanCustomer(ctx, `WHERE id = $1`, id)
}

// GetCustomerByEmail returns a customer by tenant + email.
func (r *CustomerRepository) GetCustomerByEmail(ctx context.Context, tenantID uuid.UUID, email string) (*domain.Customer, error) {
	var c domain.Customer
	var tier string
	err := r.pool.QueryRow(ctx,
		`SELECT id, tenant_id, email, phone, full_name, loyalty_tier, is_active, notes, created_at, updated_at
		   FROM customers WHERE tenant_id = $1 AND email = $2`,
		tenantID, email,
	).Scan(&c.ID, &c.TenantID, &c.Email, &c.Phone, &c.FullName, &tier, &c.IsActive, &c.Notes, &c.CreatedAt, &c.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("customer not found")
	}
	if err != nil {
		return nil, err
	}
	c.LoyaltyTier = domain.LoyaltyTier(tier)
	return &c, nil
}

// UpdateCustomerTier updates a customer's loyalty tier.
func (r *CustomerRepository) UpdateCustomerTier(ctx context.Context, customerID uuid.UUID, tier domain.LoyaltyTier) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE customers SET loyalty_tier = $2, updated_at = NOW() WHERE id = $1`,
		customerID, string(tier),
	)
	return err
}

// UpdateCustomer persists mutable customer fields.
func (r *CustomerRepository) UpdateCustomer(ctx context.Context, c *domain.Customer) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE customers SET email = $2, phone = $3, full_name = $4, loyalty_tier = $5, is_active = $6, notes = $7, updated_at = NOW()
		  WHERE id = $1`,
		c.ID, c.Email, c.Phone, c.FullName, string(c.LoyaltyTier), c.IsActive, c.Notes,
	)
	return err
}

func (r *CustomerRepository) scanCustomer(ctx context.Context, where string, arg any) (*domain.Customer, error) {
	var c domain.Customer
	var tier string
	q := `SELECT id, tenant_id, email, phone, full_name, loyalty_tier, is_active, notes, created_at, updated_at FROM customers ` + where
	err := r.pool.QueryRow(ctx, q, arg).Scan(
		&c.ID, &c.TenantID, &c.Email, &c.Phone, &c.FullName, &tier, &c.IsActive, &c.Notes, &c.CreatedAt, &c.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("customer not found")
	}
	if err != nil {
		return nil, fmt.Errorf("scanCustomer: %w", err)
	}
	c.LoyaltyTier = domain.LoyaltyTier(tier)
	return &c, nil
}

// ── Loyalty Accounts ─────────────────────────────────────────────────────────

// GetOrCreateAccount returns the loyalty account for a customer, creating it if absent.
func (r *CustomerRepository) GetOrCreateAccount(ctx context.Context, customerID uuid.UUID) (*domain.LoyaltyAccount, error) {
	acc, err := r.GetAccountByCustomerID(ctx, customerID)
	if err == nil {
		return acc, nil
	}
	// Create fresh account.
	acc = &domain.LoyaltyAccount{
		ID:         uuid.New(),
		CustomerID: customerID,
	}
	_, err = r.pool.Exec(ctx,
		`INSERT INTO loyalty_accounts (id, customer_id, points_balance, lifetime_points, updated_at)
		 VALUES ($1, $2, 0, 0, NOW()) ON CONFLICT (customer_id) DO NOTHING`,
		acc.ID, customerID,
	)
	if err != nil {
		return nil, fmt.Errorf("GetOrCreateAccount: %w", err)
	}
	return r.GetAccountByCustomerID(ctx, customerID)
}

// GetAccountByCustomerID returns a loyalty account.
func (r *CustomerRepository) GetAccountByCustomerID(ctx context.Context, customerID uuid.UUID) (*domain.LoyaltyAccount, error) {
	var a domain.LoyaltyAccount
	err := r.pool.QueryRow(ctx,
		`SELECT id, customer_id, points_balance, lifetime_points, updated_at
		   FROM loyalty_accounts WHERE customer_id = $1`, customerID,
	).Scan(&a.ID, &a.CustomerID, &a.PointsBalance, &a.LifetimePoints, &a.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("loyalty account not found for customer %s", customerID)
	}
	if err != nil {
		return nil, fmt.Errorf("GetAccountByCustomerID: %w", err)
	}
	return &a, nil
}

// GetAccountByCustomerIDForUpdate returns the loyalty account with a row lock.
func (r *CustomerRepository) GetAccountByCustomerIDForUpdate(ctx context.Context, tx pgx.Tx, customerID uuid.UUID) (*domain.LoyaltyAccount, error) {
	var a domain.LoyaltyAccount
	err := tx.QueryRow(ctx,
		`SELECT id, customer_id, points_balance, lifetime_points, updated_at
		   FROM loyalty_accounts WHERE customer_id = $1 FOR UPDATE`, customerID,
	).Scan(&a.ID, &a.CustomerID, &a.PointsBalance, &a.LifetimePoints, &a.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("loyalty account not found for customer %s", customerID)
	}
	return &a, err
}

// AddPointsTx adds (or subtracts) points within a transaction.
func (r *CustomerRepository) AddPointsTx(ctx context.Context, tx pgx.Tx, accountID uuid.UUID, delta int) (newBalance int, err error) {
	lifetimeDelta := 0
	if delta > 0 {
		lifetimeDelta = delta
	}
	err = tx.QueryRow(ctx,
		`UPDATE loyalty_accounts
		    SET points_balance  = points_balance  + $2,
		        lifetime_points = lifetime_points + $3,
		        updated_at      = NOW()
		  WHERE id = $1
		  RETURNING points_balance`,
		accountID, delta, lifetimeDelta,
	).Scan(&newBalance)
	return newBalance, err
}

// InsertTransactionTx appends an immutable loyalty_transactions row.
func (r *CustomerRepository) InsertTransactionTx(ctx context.Context, tx pgx.Tx, t *domain.LoyaltyTransaction) error {
	t.ID = uuid.New()
	t.CreatedAt = time.Now().UTC()
	_, err := tx.Exec(ctx,
		`INSERT INTO loyalty_transactions (id, account_id, order_id, tx_type, points, balance_before, balance_after, note, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		t.ID, t.AccountID, t.OrderID, string(t.TxType), t.Points, t.BalanceBefore, t.BalanceAfter, t.Note, t.CreatedAt,
	)
	return err
}

// ListCustomers returns paginated customers with loyalty balance.
func (r *CustomerRepository) ListCustomers(ctx context.Context, filters domain.CustomerListFilters) ([]domain.CustomerListItem, int, error) {
	if filters.Page < 1 {
		filters.Page = 1
	}
	if filters.PageSize < 1 || filters.PageSize > 100 {
		filters.PageSize = 25
	}
	offset := (filters.Page - 1) * filters.PageSize

	args := []any{filters.TenantID}
	where := []string{"c.tenant_id = $1"}
	argIdx := 2
	if filters.Search != "" {
		where = append(where, fmt.Sprintf("(c.email ILIKE $%d OR c.full_name ILIKE $%d OR c.phone ILIKE $%d)", argIdx, argIdx, argIdx))
		args = append(args, "%"+filters.Search+"%")
		argIdx++
	}
	if filters.Tier != "" {
		where = append(where, fmt.Sprintf("c.loyalty_tier = $%d", argIdx))
		args = append(args, filters.Tier)
		argIdx++
	}
	if filters.Email != "" {
		where = append(where, fmt.Sprintf("c.email ILIKE $%d", argIdx))
		args = append(args, "%"+filters.Email+"%")
		argIdx++
	}

	whereClause := strings.Join(where, " AND ")

	countQ := fmt.Sprintf(`SELECT COUNT(*) FROM customers c WHERE %s`, whereClause)
	var total int
	if err := r.pool.QueryRow(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("ListCustomers count: %w", err)
	}

	args = append(args, filters.PageSize, offset)
	limitIdx := argIdx
	offsetIdx := argIdx + 1
	listQ := fmt.Sprintf(`
		SELECT c.id, c.email, c.full_name, c.phone, c.loyalty_tier::text,
		       COALESCE(la.points_balance, 0), COALESCE(la.lifetime_points, 0),
		       c.is_active, c.created_at
		  FROM customers c
		  LEFT JOIN loyalty_accounts la ON la.customer_id = c.id
		 WHERE %s
		 ORDER BY c.created_at DESC
		 LIMIT $%d OFFSET $%d`, whereClause, limitIdx, offsetIdx)

	rows, err := r.pool.Query(ctx, listQ, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("ListCustomers: %w", err)
	}
	defer rows.Close()

	var items []domain.CustomerListItem
	for rows.Next() {
		var it domain.CustomerListItem
		if err := rows.Scan(&it.ID, &it.Email, &it.FullName, &it.Phone, &it.LoyaltyTier,
			&it.PointsBalance, &it.LifetimePoints, &it.IsActive, &it.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("ListCustomers scan: %w", err)
		}
		items = append(items, it)
	}
	return items, total, rows.Err()
}

// ListTransactions returns recent loyalty transactions for an account.
func (r *CustomerRepository) ListTransactions(ctx context.Context, accountID uuid.UUID, limit int) ([]domain.LoyaltyTransaction, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := r.pool.Query(ctx,
		`SELECT id, account_id, order_id, tx_type, points, balance_before, balance_after, note, created_at
		   FROM loyalty_transactions WHERE account_id = $1 ORDER BY created_at DESC LIMIT $2`,
		accountID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var txs []domain.LoyaltyTransaction
	for rows.Next() {
		var t domain.LoyaltyTransaction
		var txType string
		if err := rows.Scan(&t.ID, &t.AccountID, &t.OrderID, &txType, &t.Points, &t.BalanceBefore, &t.BalanceAfter, &t.Note, &t.CreatedAt); err != nil {
			return nil, err
		}
		t.TxType = domain.LoyaltyTxType(txType)
		txs = append(txs, t)
	}
	return txs, rows.Err()
}
