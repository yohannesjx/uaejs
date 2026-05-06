package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// LoyaltyTier mirrors the loyalty_tier DB enum.
// This is separate from the pricing CustomerTier (standard/vip/wholesale/staff).
type LoyaltyTier string

const (
	LoyaltyTierBronze LoyaltyTier = "bronze"
	LoyaltyTierSilver LoyaltyTier = "silver"
	LoyaltyTierGold   LoyaltyTier = "gold"
	LoyaltyTierVIP    LoyaltyTier = "vip"
)

// LoyaltyTxType mirrors the loyalty_tx_type DB enum.
type LoyaltyTxType string

const (
	LoyaltyTxEarned   LoyaltyTxType = "earned"
	LoyaltyTxRedeemed LoyaltyTxType = "redeemed"
	LoyaltyTxExpired  LoyaltyTxType = "expired"
	LoyaltyTxAdjusted LoyaltyTxType = "adjusted"
	LoyaltyTxRefunded LoyaltyTxType = "refunded"
)

// Customer is a registered buyer with a loyalty profile.
type Customer struct {
	ID          uuid.UUID   `json:"id"`
	TenantID    uuid.UUID   `json:"tenant_id"`
	Email       string      `json:"email"`
	Phone       *string     `json:"phone,omitempty"`
	FullName    string      `json:"full_name"`
	LoyaltyTier LoyaltyTier `json:"loyalty_tier"`
	IsActive    bool        `json:"is_active"`
	Notes       *string     `json:"notes,omitempty"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
}

// LoyaltyAccount holds the live points balance for a customer.
type LoyaltyAccount struct {
	ID             uuid.UUID `json:"id"`
	CustomerID     uuid.UUID `json:"customer_id"`
	PointsBalance  int       `json:"points_balance"`
	LifetimePoints int       `json:"lifetime_points"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// LoyaltyTransaction is one immutable row in the points ledger.
type LoyaltyTransaction struct {
	ID            uuid.UUID     `json:"id"`
	AccountID     uuid.UUID     `json:"account_id"`
	OrderID       *uuid.UUID    `json:"order_id,omitempty"`
	TxType        LoyaltyTxType `json:"tx_type"`
	Points        int           `json:"points"`         // positive = earned, negative = redeemed
	BalanceBefore int           `json:"balance_before"`
	BalanceAfter  int           `json:"balance_after"`
	Note          *string       `json:"note,omitempty"`
	CreatedAt     time.Time     `json:"created_at"`
}

// PointsRedemptionResult is returned when a customer redeems loyalty points.
type PointsRedemptionResult struct {
	PointsRedeemed   int             `json:"points_redeemed"`
	DiscountAED      decimal.Decimal `json:"discount_aed"`
	BalanceAfter     int             `json:"balance_after"`
}
