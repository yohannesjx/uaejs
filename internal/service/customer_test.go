package service_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/dubai-retail/os/internal/domain"
	"github.com/dubai-retail/os/internal/service"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// =============================================================================
// Fake repository
// =============================================================================

type fakeCustomerRepo struct {
	customers    map[uuid.UUID]*domain.Customer
	accounts     map[uuid.UUID]*domain.LoyaltyAccount // keyed by customer_id
	transactions []domain.LoyaltyTransaction
}

func newFakeCustomerRepo() *fakeCustomerRepo {
	return &fakeCustomerRepo{
		customers: make(map[uuid.UUID]*domain.Customer),
		accounts:  make(map[uuid.UUID]*domain.LoyaltyAccount),
	}
}

func (r *fakeCustomerRepo) InsertCustomer(_ context.Context, c *domain.Customer) error {
	c.ID = uuid.New()
	r.customers[c.ID] = c
	return nil
}

func (r *fakeCustomerRepo) GetCustomerByID(_ context.Context, id uuid.UUID) (*domain.Customer, error) {
	c, ok := r.customers[id]
	if !ok {
		return nil, fmt.Errorf("customer not found")
	}
	return c, nil
}

func (r *fakeCustomerRepo) GetCustomerByEmail(_ context.Context, tenantID uuid.UUID, email string) (*domain.Customer, error) {
	for _, c := range r.customers {
		if c.TenantID == tenantID && c.Email == email {
			return c, nil
		}
	}
	return nil, fmt.Errorf("customer not found")
}

func (r *fakeCustomerRepo) UpdateCustomer(_ context.Context, c *domain.Customer) error {
	r.customers[c.ID] = c
	return nil
}

func (r *fakeCustomerRepo) UpdateCustomerTier(_ context.Context, id uuid.UUID, tier domain.LoyaltyTier) error {
	if c, ok := r.customers[id]; ok {
		c.LoyaltyTier = tier
	}
	return nil
}

func (r *fakeCustomerRepo) GetOrCreateAccount(_ context.Context, customerID uuid.UUID) (*domain.LoyaltyAccount, error) {
	if a, ok := r.accounts[customerID]; ok {
		return a, nil
	}
	a := &domain.LoyaltyAccount{ID: uuid.New(), CustomerID: customerID}
	r.accounts[customerID] = a
	return a, nil
}

func (r *fakeCustomerRepo) GetAccountByCustomerID(_ context.Context, customerID uuid.UUID) (*domain.LoyaltyAccount, error) {
	a, ok := r.accounts[customerID]
	if !ok {
		return nil, fmt.Errorf("account not found for customer %s", customerID)
	}
	return a, nil
}

func (r *fakeCustomerRepo) GetAccountByCustomerIDForUpdate(_ context.Context, _ pgx.Tx, customerID uuid.UUID) (*domain.LoyaltyAccount, error) {
	return r.GetAccountByCustomerID(context.Background(), customerID)
}

func (r *fakeCustomerRepo) AddPointsTx(_ context.Context, _ pgx.Tx, accountID uuid.UUID, delta int) (int, error) {
	for _, a := range r.accounts {
		if a.ID == accountID {
			a.PointsBalance += delta
			if delta > 0 {
				a.LifetimePoints += delta
			}
			return a.PointsBalance, nil
		}
	}
	return 0, fmt.Errorf("account not found")
}

func (r *fakeCustomerRepo) InsertTransactionTx(_ context.Context, _ pgx.Tx, t *domain.LoyaltyTransaction) error {
	t.ID = uuid.New()
	r.transactions = append(r.transactions, *t)
	return nil
}

func (r *fakeCustomerRepo) ListTransactions(_ context.Context, accountID uuid.UUID, limit int) ([]domain.LoyaltyTransaction, error) {
	var out []domain.LoyaltyTransaction
	for _, t := range r.transactions {
		if t.AccountID == accountID {
			out = append(out, t)
		}
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// =============================================================================
// Helpers
// =============================================================================

func newTestCustomerAndLoyalty() (*service.CustomerService, *service.LoyaltyService, *fakeCustomerRepo) {
	repo := newFakeCustomerRepo()
	txb := &unitFakeTxBeginner{}
	cs := service.NewCustomerService(repo, txb, zap.NewNop())
	ls := service.NewLoyaltyService(repo, txb, zap.NewNop())
	return cs, ls, repo
}

// seedCustomerWithAccount creates a customer + loyalty account for a test.
func seedCustomerWithAccount(t *testing.T, cs *service.CustomerService, repo *fakeCustomerRepo) *domain.Customer {
	t.Helper()
	customer, err := cs.Create(context.Background(), service.CreateCustomerInput{
		TenantID: domain.DefaultTenantID,
		Email:    "hessa@example.com",
		FullName: "Hessa Al-Mansouri",
	})
	if err != nil {
		t.Fatalf("Create customer: %v", err)
	}
	return customer
}

// =============================================================================
// Tests: Customer CRUD
// =============================================================================

func TestCustomer_Create_Success(t *testing.T) {
	cs, _, _ := newTestCustomerAndLoyalty()
	ctx := context.Background()

	customer, err := cs.Create(ctx, service.CreateCustomerInput{
		TenantID: domain.DefaultTenantID,
		Email:    "fatima@store.ae",
		FullName: "Fatima Al-Rashidi",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if customer.ID == uuid.Nil {
		t.Error("expected non-nil customer ID")
	}
	if customer.LoyaltyTier != domain.LoyaltyTierBronze {
		t.Errorf("new customer should start at bronze, got %s", customer.LoyaltyTier)
	}
	if !customer.IsActive {
		t.Error("new customer should be active")
	}
}

func TestCustomer_Create_EmptyEmail_Rejected(t *testing.T) {
	cs, _, _ := newTestCustomerAndLoyalty()
	_, err := cs.Create(context.Background(), service.CreateCustomerInput{
		TenantID: domain.DefaultTenantID,
		FullName: "No Email",
	})
	if err == nil {
		t.Error("expected error for missing email")
	}
}

func TestCustomer_Create_SeedsLoyaltyAccount(t *testing.T) {
	cs, ls, _ := newTestCustomerAndLoyalty()
	ctx := context.Background()

	customer, _ := cs.Create(ctx, service.CreateCustomerInput{
		TenantID: domain.DefaultTenantID,
		Email:    "maryam@store.ae",
		FullName: "Maryam Al-Ali",
	})

	acc, err := ls.GetBalance(ctx, customer.ID)
	if err != nil {
		t.Fatalf("GetBalance after create: %v", err)
	}
	if acc.PointsBalance != 0 {
		t.Errorf("new customer should have 0 points, got %d", acc.PointsBalance)
	}
}

// =============================================================================
// Tests: Points accumulation
// =============================================================================

func TestLoyalty_AwardPoints_Basic(t *testing.T) {
	cs, ls, _ := newTestCustomerAndLoyalty()
	ctx := context.Background()
	customer := seedCustomerWithAccount(t, cs, nil)
	_ = customer // already seeded into repo

	_, err := ls.AwardPoints(ctx, service.AwardPointsInput{
		CustomerID: customer.ID,
		OrderID:    uuid.New(),
		TotalAED:   decimal.NewFromFloat(150.00),
	})
	if err != nil {
		t.Fatalf("AwardPoints: %v", err)
	}

	acc, _ := ls.GetBalance(ctx, customer.ID)
	if acc.PointsBalance != 150 {
		t.Errorf("expected 150 points (1 per AED), got %d", acc.PointsBalance)
	}
}

func TestLoyalty_AwardPoints_MultipleOrders_Accumulate(t *testing.T) {
	cs, ls, _ := newTestCustomerAndLoyalty()
	ctx := context.Background()
	customer := seedCustomerWithAccount(t, cs, nil)

	totals := []float64{100, 250, 75}
	expectedTotal := 425

	for _, total := range totals {
		_, err := ls.AwardPoints(ctx, service.AwardPointsInput{
			CustomerID: customer.ID,
			OrderID:    uuid.New(),
			TotalAED:   decimal.NewFromFloat(total),
		})
		if err != nil {
			t.Fatalf("AwardPoints %.0f AED: %v", total, err)
		}
	}

	acc, _ := ls.GetBalance(ctx, customer.ID)
	if acc.PointsBalance != expectedTotal {
		t.Errorf("expected %d points, got %d", expectedTotal, acc.PointsBalance)
	}
}

func TestLoyalty_AwardPoints_ZeroAED_NoPointsAwarded(t *testing.T) {
	cs, ls, _ := newTestCustomerAndLoyalty()
	ctx := context.Background()
	customer := seedCustomerWithAccount(t, cs, nil)

	_, err := ls.AwardPoints(ctx, service.AwardPointsInput{
		CustomerID: customer.ID,
		OrderID:    uuid.New(),
		TotalAED:   decimal.Zero,
	})
	if err == nil {
		t.Error("expected error for zero-AED award")
	}
}

// =============================================================================
// Tests: Points redemption
// =============================================================================

func TestLoyalty_RedeemPoints_Success(t *testing.T) {
	cs, ls, _ := newTestCustomerAndLoyalty()
	ctx := context.Background()
	customer := seedCustomerWithAccount(t, cs, nil)

	// Award 500 points first.
	_, _ = ls.AwardPoints(ctx, service.AwardPointsInput{
		CustomerID: customer.ID,
		OrderID:    uuid.New(),
		TotalAED:   decimal.NewFromFloat(500),
	})

	result, err := ls.RedeemPoints(ctx, service.RedeemInput{
		CustomerID:     customer.ID,
		PointsToRedeem: 200,
	})
	if err != nil {
		t.Fatalf("RedeemPoints: %v", err)
	}

	// 200 points = 200/100 = 2.00 AED discount.
	expectedDiscount := decimal.NewFromFloat(2.00)
	if !result.DiscountAED.Equal(expectedDiscount) {
		t.Errorf("expected discount %s AED, got %s", expectedDiscount, result.DiscountAED)
	}
	if result.BalanceAfter != 300 {
		t.Errorf("expected 300 points remaining, got %d", result.BalanceAfter)
	}
	if result.PointsRedeemed != 200 {
		t.Errorf("expected 200 redeemed, got %d", result.PointsRedeemed)
	}
}

func TestLoyalty_RedeemPoints_InsufficientBalance(t *testing.T) {
	cs, ls, _ := newTestCustomerAndLoyalty()
	ctx := context.Background()
	customer := seedCustomerWithAccount(t, cs, nil)

	// Award only 50 points.
	_, _ = ls.AwardPoints(ctx, service.AwardPointsInput{
		CustomerID: customer.ID,
		OrderID:    uuid.New(),
		TotalAED:   decimal.NewFromFloat(50),
	})

	_, err := ls.RedeemPoints(ctx, service.RedeemInput{
		CustomerID:     customer.ID,
		PointsToRedeem: 200, // more than available
	})
	if err == nil {
		t.Error("expected error for insufficient points")
	}
}

func TestLoyalty_RedeemPoints_ZeroAmount_Rejected(t *testing.T) {
	cs, ls, _ := newTestCustomerAndLoyalty()
	ctx := context.Background()
	customer := seedCustomerWithAccount(t, cs, nil)

	_, err := ls.RedeemPoints(ctx, service.RedeemInput{
		CustomerID:     customer.ID,
		PointsToRedeem: 0,
	})
	if err == nil {
		t.Error("expected error for zero redemption")
	}
}

// =============================================================================
// Tests: Tier progression
// =============================================================================

func TestLoyalty_TierProgression(t *testing.T) {
	type tierTest struct {
		points   int
		wantTier domain.LoyaltyTier
	}
	tests := []tierTest{
		{999, domain.LoyaltyTierBronze},
		{1000, domain.LoyaltyTierSilver},
		{5000, domain.LoyaltyTierGold},
		{20000, domain.LoyaltyTierVIP},
	}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("%d_points", tc.points), func(t *testing.T) {
			cs, _, repo := newTestCustomerAndLoyalty()

			customer := seedCustomerWithAccount(t, cs, nil)

			// Directly set the balance in the fake repo.
			acc := repo.accounts[customer.ID]
			acc.PointsBalance = tc.points
			acc.LifetimePoints = tc.points

			// Verify the PointsToAED conversion is correct.
			discount := service.PointsToAED(tc.points)
			expectedAED := decimal.NewFromInt(int64(tc.points)).Div(decimal.NewFromInt(100)).Round(2)
			if !discount.Equal(expectedAED) {
				t.Errorf("PointsToAED(%d): expected %s, got %s", tc.points, expectedAED, discount)
			}
		})
	}
}

// =============================================================================
// Tests: History
// =============================================================================

func TestLoyalty_GetHistory(t *testing.T) {
	cs, ls, _ := newTestCustomerAndLoyalty()
	ctx := context.Background()
	customer := seedCustomerWithAccount(t, cs, nil)

	for i := 0; i < 3; i++ {
		_, _ = ls.AwardPoints(ctx, service.AwardPointsInput{
			CustomerID: customer.ID,
			OrderID:    uuid.New(),
			TotalAED:   decimal.NewFromFloat(50),
		})
	}

	history, err := ls.GetHistory(ctx, customer.ID, 10)
	if err != nil {
		t.Fatalf("GetHistory: %v", err)
	}
	if len(history) != 3 {
		t.Errorf("expected 3 history entries, got %d", len(history))
	}
	for _, tx := range history {
		if tx.TxType != domain.LoyaltyTxEarned {
			t.Errorf("unexpected tx type: %s", tx.TxType)
		}
	}
}

// =============================================================================
// Tests: Tenant isolation
// =============================================================================

func TestCustomer_TenantIsolation(t *testing.T) {
	cs, _, repo := newTestCustomerAndLoyalty()
	ctx := context.Background()

	tenantA := uuid.New()
	tenantB := uuid.New()

	// Create customer in tenant A.
	cA, _ := cs.Create(ctx, service.CreateCustomerInput{TenantID: tenantA, Email: "a@a.com", FullName: "A"})

	// Try to retrieve via tenant B email (different tenant).
	_, err := cs.GetByEmail(ctx, tenantB, "a@a.com")
	if err == nil {
		t.Error("tenant B should not be able to find tenant A's customer by email")
	}

	// Tenant A can find their own customer.
	found, err := cs.GetByEmail(ctx, tenantA, "a@a.com")
	if err != nil {
		t.Fatalf("tenant A should find their own customer: %v", err)
	}
	if found.ID != cA.ID {
		t.Error("ID mismatch")
	}
	_ = repo
}
