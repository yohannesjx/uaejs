// Package service — Customer registry and Loyalty points engine.
//
// LoyaltyService manages points accumulation on completed orders and
// redemption at checkout. The earning rate and redemption rate are
// configurable per tenant via TenantSettings.
//
// Default rates (can be overridden via SaveSettings):
//   earn_rate   = 1 point per AED spent (integer floor)
//   redeem_rate = 100 points = 1 AED discount
//
// Tier thresholds (lifetime_points):
//   bronze  = 0 – 999
//   silver  = 1000 – 4999
//   gold    = 5000 – 19999
//   vip     = 20000+
package service

import (
	"context"
	"fmt"

	"github.com/dubai-retail/os/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// =============================================================================
// Repository interface
// =============================================================================

// CustomerRepo is the DB interface required by CustomerService and LoyaltyService.
type CustomerRepo interface {
	InsertCustomer(ctx context.Context, c *domain.Customer) error
	GetCustomerByID(ctx context.Context, id uuid.UUID) (*domain.Customer, error)
	GetCustomerByEmail(ctx context.Context, tenantID uuid.UUID, email string) (*domain.Customer, error)
	UpdateCustomer(ctx context.Context, c *domain.Customer) error
	UpdateCustomerTier(ctx context.Context, customerID uuid.UUID, tier domain.LoyaltyTier) error
	GetOrCreateAccount(ctx context.Context, customerID uuid.UUID) (*domain.LoyaltyAccount, error)
	GetAccountByCustomerID(ctx context.Context, customerID uuid.UUID) (*domain.LoyaltyAccount, error)
	GetAccountByCustomerIDForUpdate(ctx context.Context, tx pgx.Tx, customerID uuid.UUID) (*domain.LoyaltyAccount, error)
	AddPointsTx(ctx context.Context, tx pgx.Tx, accountID uuid.UUID, delta int) (int, error)
	InsertTransactionTx(ctx context.Context, tx pgx.Tx, t *domain.LoyaltyTransaction) error
	ListTransactions(ctx context.Context, accountID uuid.UUID, limit int) ([]domain.LoyaltyTransaction, error)
}

// =============================================================================
// CustomerService — CRUD
// =============================================================================

// CustomerService manages the customer registry.
type CustomerService struct {
	repo CustomerRepo
	pool TxBeginner
	log  *zap.Logger
}

// NewCustomerService creates a CustomerService.
func NewCustomerService(repo CustomerRepo, pool TxBeginner, log *zap.Logger) *CustomerService {
	return &CustomerService{repo: repo, pool: pool, log: log}
}

// CreateCustomerInput is the request to register a new customer.
type CreateCustomerInput struct {
	TenantID uuid.UUID `json:"tenant_id"`
	Email    string    `json:"email"`
	Phone    *string   `json:"phone,omitempty"`
	FullName string    `json:"full_name"`
	Notes    *string   `json:"notes,omitempty"`
}

// Create registers a new customer and seeds a loyalty account.
func (s *CustomerService) Create(ctx context.Context, in CreateCustomerInput) (*domain.Customer, error) {
	if in.Email == "" {
		return nil, fmt.Errorf("Create customer: email is required")
	}
	customer := &domain.Customer{
		TenantID:    in.TenantID,
		Email:       in.Email,
		Phone:       in.Phone,
		FullName:    in.FullName,
		LoyaltyTier: domain.LoyaltyTierBronze,
		IsActive:    true,
		Notes:       in.Notes,
	}
	if err := s.repo.InsertCustomer(ctx, customer); err != nil {
		return nil, fmt.Errorf("Create customer: %w", err)
	}
	// Seed a loyalty account immediately.
	if _, err := s.repo.GetOrCreateAccount(ctx, customer.ID); err != nil {
		s.log.Warn("loyalty.account_seed_failed",
			zap.String("customer_id", customer.ID.String()),
			zap.Error(err),
		)
	}
	s.log.Info("customer.created",
		zap.String("id", customer.ID.String()),
		zap.String("email", customer.Email),
	)
	return customer, nil
}

// GetByID returns a customer by primary key.
func (s *CustomerService) GetByID(ctx context.Context, id uuid.UUID) (*domain.Customer, error) {
	return s.repo.GetCustomerByID(ctx, id)
}

// GetByEmail returns a customer by tenant + email.
func (s *CustomerService) GetByEmail(ctx context.Context, tenantID uuid.UUID, email string) (*domain.Customer, error) {
	return s.repo.GetCustomerByEmail(ctx, tenantID, email)
}

func (s *CustomerService) List(ctx context.Context, filters domain.CustomerListFilters) (*domain.PageResponse[domain.CustomerListItem], error) {
	listRepo, ok := s.repo.(interface {
		ListCustomers(ctx context.Context, filters domain.CustomerListFilters) ([]domain.CustomerListItem, int, error)
	})
	if !ok {
		return nil, fmt.Errorf("ListCustomers: repository does not support list queries")
	}

	filters.Page = normalizePage(filters.Page)
	filters.PageSize = normalizePageSize(filters.PageSize)

	items, total, err := listRepo.ListCustomers(ctx, filters)
	if err != nil {
		return nil, fmt.Errorf("ListCustomers: %w", err)
	}

	return &domain.PageResponse[domain.CustomerListItem]{
		Items: items,
		Total: total,
	}, nil
}

// =============================================================================
// LoyaltyService — Points engine
// =============================================================================

// LoyaltyService manages point accumulation and redemption.
type LoyaltyService struct {
	repo CustomerRepo
	pool TxBeginner
	log  *zap.Logger
}

// NewLoyaltyService creates a LoyaltyService.
func NewLoyaltyService(repo CustomerRepo, pool TxBeginner, log *zap.Logger) *LoyaltyService {
	return &LoyaltyService{repo: repo, pool: pool, log: log}
}

// Earn/redeem rates.
const (
	defaultEarnRate   = 1    // 1 point per AED spent
	defaultRedeemRate = 100  // 100 points = 1 AED discount
)

// AwardPointsInput is the request to award points after an order is completed.
type AwardPointsInput struct {
	CustomerID uuid.UUID       `json:"customer_id"`
	OrderID    uuid.UUID       `json:"order_id"`
	TotalAED   decimal.Decimal `json:"total_aed"` // order total used to compute points
}

// AwardPoints adds points for a completed order and advances the loyalty tier.
func (s *LoyaltyService) AwardPoints(ctx context.Context, in AwardPointsInput) (*domain.LoyaltyTransaction, error) {
	if in.CustomerID == uuid.Nil {
		return nil, fmt.Errorf("AwardPoints: customer_id is required")
	}
	if in.TotalAED.LessThanOrEqual(decimal.Zero) {
		return nil, fmt.Errorf("AwardPoints: total_aed must be positive")
	}

	points := int(in.TotalAED.Floor().IntPart()) * defaultEarnRate
	if points <= 0 {
		return nil, nil // sub-AED orders earn nothing
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadWrite})
	if err != nil {
		return nil, fmt.Errorf("AwardPoints: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	acc, err := s.repo.GetAccountByCustomerIDForUpdate(ctx, tx, in.CustomerID)
	if err != nil {
		return nil, fmt.Errorf("AwardPoints: get account: %w", err)
	}

	balanceBefore := acc.PointsBalance
	newBalance, err := s.repo.AddPointsTx(ctx, tx, acc.ID, points)
	if err != nil {
		return nil, fmt.Errorf("AwardPoints: add points: %w", err)
	}

	orderID := in.OrderID
	note := fmt.Sprintf("earned for order %s", in.OrderID)
	ltx := &domain.LoyaltyTransaction{
		AccountID:     acc.ID,
		OrderID:       &orderID,
		TxType:        domain.LoyaltyTxEarned,
		Points:        points,
		BalanceBefore: balanceBefore,
		BalanceAfter:  newBalance,
		Note:          &note,
	}
	if err := s.repo.InsertTransactionTx(ctx, tx, ltx); err != nil {
		return nil, fmt.Errorf("AwardPoints: insert tx: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("AwardPoints: commit: %w", err)
	}

	// After commit: recalculate loyalty tier (non-critical, fire-and-forget).
	s.maybePromoteTier(ctx, in.CustomerID, newBalance)

	s.log.Info("loyalty.points_awarded",
		zap.String("customer", in.CustomerID.String()),
		zap.Int("points", points),
		zap.Int("new_balance", newBalance),
	)
	return ltx, nil
}

// RedeemInput is the request to redeem points for a discount.
type RedeemInput struct {
	CustomerID    uuid.UUID `json:"customer_id"`
	PointsToRedeem int     `json:"points_to_redeem"`
}

// RedeemPoints deducts loyalty points and returns the AED discount value.
func (s *LoyaltyService) RedeemPoints(ctx context.Context, in RedeemInput) (*domain.PointsRedemptionResult, error) {
	if in.CustomerID == uuid.Nil {
		return nil, fmt.Errorf("RedeemPoints: customer_id is required")
	}
	if in.PointsToRedeem <= 0 {
		return nil, fmt.Errorf("RedeemPoints: points_to_redeem must be positive")
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadWrite})
	if err != nil {
		return nil, fmt.Errorf("RedeemPoints: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	acc, err := s.repo.GetAccountByCustomerIDForUpdate(ctx, tx, in.CustomerID)
	if err != nil {
		return nil, fmt.Errorf("RedeemPoints: get account: %w", err)
	}
	if acc.PointsBalance < in.PointsToRedeem {
		return nil, fmt.Errorf("RedeemPoints: insufficient points (balance %d, requested %d)",
			acc.PointsBalance, in.PointsToRedeem)
	}

	balanceBefore := acc.PointsBalance
	newBalance, err := s.repo.AddPointsTx(ctx, tx, acc.ID, -in.PointsToRedeem)
	if err != nil {
		return nil, fmt.Errorf("RedeemPoints: deduct: %w", err)
	}

	note := fmt.Sprintf("redeemed %d points", in.PointsToRedeem)
	if err := s.repo.InsertTransactionTx(ctx, tx, &domain.LoyaltyTransaction{
		AccountID:     acc.ID,
		TxType:        domain.LoyaltyTxRedeemed,
		Points:        -in.PointsToRedeem,
		BalanceBefore: balanceBefore,
		BalanceAfter:  newBalance,
		Note:          &note,
	}); err != nil {
		return nil, fmt.Errorf("RedeemPoints: insert tx: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("RedeemPoints: commit: %w", err)
	}

	discountAED := decimal.NewFromInt(int64(in.PointsToRedeem)).
		Div(decimal.NewFromInt(defaultRedeemRate)).Round(2)

	s.log.Info("loyalty.points_redeemed",
		zap.String("customer", in.CustomerID.String()),
		zap.Int("points", in.PointsToRedeem),
		zap.String("discount_aed", discountAED.String()),
	)

	return &domain.PointsRedemptionResult{
		PointsRedeemed: in.PointsToRedeem,
		DiscountAED:    discountAED,
		BalanceAfter:   newBalance,
	}, nil
}

// GetBalance returns the current loyalty account for a customer.
func (s *LoyaltyService) GetBalance(ctx context.Context, customerID uuid.UUID) (*domain.LoyaltyAccount, error) {
	return s.repo.GetAccountByCustomerID(ctx, customerID)
}

// GetHistory returns recent point transactions for a customer.
func (s *LoyaltyService) GetHistory(ctx context.Context, customerID uuid.UUID, limit int) ([]domain.LoyaltyTransaction, error) {
	acc, err := s.repo.GetAccountByCustomerID(ctx, customerID)
	if err != nil {
		return nil, err
	}
	return s.repo.ListTransactions(ctx, acc.ID, limit)
}

// =============================================================================
// Tier management
// =============================================================================

// loyaltyTierThresholds maps lifetime point thresholds to tiers (ascending).
var loyaltyTierThresholds = []struct {
	min  int
	tier domain.LoyaltyTier
}{
	{20000, domain.LoyaltyTierVIP},
	{5000, domain.LoyaltyTierGold},
	{1000, domain.LoyaltyTierSilver},
	{0, domain.LoyaltyTierBronze},
}

// PointsToAED converts a points amount to its AED monetary value.
func PointsToAED(points int) decimal.Decimal {
	return decimal.NewFromInt(int64(points)).Div(decimal.NewFromInt(defaultRedeemRate)).Round(2)
}

// maybePromoteTier recalculates and updates the loyalty tier after a points change.
func (s *LoyaltyService) maybePromoteTier(ctx context.Context, customerID uuid.UUID, currentBalance int) {
	var newTier domain.LoyaltyTier
	for _, t := range loyaltyTierThresholds {
		if currentBalance >= t.min {
			newTier = t.tier
			break
		}
	}
	if err := s.repo.UpdateCustomerTier(ctx, customerID, newTier); err != nil {
		s.log.Warn("loyalty.tier_update_failed",
			zap.String("customer", customerID.String()),
			zap.Error(err),
		)
	}
}
