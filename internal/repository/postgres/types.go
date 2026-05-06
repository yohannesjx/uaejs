package postgres

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// FIFOBatchItemRow is the raw scan target returned by GetFIFOBatchItems.
// Using concrete types here avoids interface{} and blank-import tricks.
type FIFOBatchItemRow struct {
	BatchItemID       uuid.UUID
	BatchReceivedAt   time.Time
	LandedCostPerUnit decimal.Decimal
	QuantityReceived  int
	TotalDeducted     int
}

// Remaining returns how many units are still available in this batch for FIFO.
func (r *FIFOBatchItemRow) Remaining() int {
	rem := r.QuantityReceived - r.TotalDeducted
	if rem < 0 {
		return 0
	}
	return rem
}
