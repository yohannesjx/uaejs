// Package metrics centralises all Prometheus metric definitions for the
// Dubai Retail OS. All metrics are registered with a custom registry so that
// they can be tested in isolation and exported on /metrics.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const namespace = "dubai_retail"

// =============================================================================
// Metrics – single struct injected into every service that needs instrumentation
// =============================================================================

type Metrics struct {
	// ── Orders ───────────────────────────────────────────────────────────────
	// OrdersProcessedTotal counts successfully completed orders.
	// Labels: channel_type (pos|ecommerce|wholesale), invoice_type (einvoice_ubl|receipt)
	OrdersProcessedTotal *prometheus.CounterVec

	// OrdersFailedTotal counts orders that hit an error before committing.
	// Labels: channel_type, reason (insufficient_stock|price_missing|db_error|compliance_error)
	OrdersFailedTotal *prometheus.CounterVec

	// OrderProcessingDuration measures end-to-end ProcessOrder latency.
	// Labels: channel_type
	OrderProcessingDuration *prometheus.HistogramVec

	// ── Inventory ────────────────────────────────────────────────────────────
	// StockLevel is a gauge set after every FIFO deduction or stock movement.
	// Labels: variant_id, sku
	StockLevel *prometheus.GaugeVec

	// LowStockEventsTotal counts transitions from normal to low-stock state.
	// Labels: variant_id
	LowStockEventsTotal *prometheus.CounterVec

	// FIFOBatchesConsumedTotal counts individual batch_item rows depleted per order.
	// Labels: variant_id
	FIFOBatchesConsumedTotal *prometheus.CounterVec

	// StockMovementsTotal counts inventory_movements rows inserted.
	// Labels: movement_type
	StockMovementsTotal *prometheus.CounterVec

	// ── Reservations ─────────────────────────────────────────────────────────
	// ReservationsCreatedTotal counts new reservation rows.
	ReservationsCreatedTotal prometheus.Counter

	// ReservationsExpiredTotal counts reservations released by the cleanup job.
	ReservationsExpiredTotal prometheus.Counter

	// ActiveReservations tracks the current count of unexpired active reservations.
	ActiveReservations prometheus.Gauge

	// ── Invoices / Compliance ─────────────────────────────────────────────────
	// InvoicesGeneratedTotal counts order_invoices rows inserted.
	// Labels: invoice_type (einvoice_ubl|receipt), trigger_reason
	InvoicesGeneratedTotal *prometheus.CounterVec

	// InvoiceSerializationDuration measures UBL XML generation time.
	InvoiceSerializationDuration prometheus.Histogram

	// PendingInvoices is set by a background collector; orders missing invoice_number.
	PendingInvoices prometheus.Gauge

	// ── Pricing ──────────────────────────────────────────────────────────────
	// PriceResolveDuration measures PriceResolver.Resolve latency.
	// Labels: price_source (standard|promotion)
	PriceResolveDuration *prometheus.HistogramVec

	// PromotionHitsTotal counts successful promotion price applications.
	// Labels: channel_type, customer_tier
	PromotionHitsTotal *prometheus.CounterVec

	// ── Returns / RMA ─────────────────────────────────────────────────────────
	// ReturnsCreatedTotal counts new return requests.
	// Labels: channel_type, condition (good|damaged|wrong_item)
	ReturnsCreatedTotal *prometheus.CounterVec

	// QCMismatchTotal counts QC photo comparisons that failed (potential fraud).
	QCMismatchTotal prometheus.Counter

	// ── Warehouses ────────────────────────────────────────────────────────────
	// WarehouseTransfersTotal counts stock transfers between warehouse locations.
	// Labels: from_type (warehouse|store|dropship), to_type
	WarehouseTransfersTotal *prometheus.CounterVec

	// WarehouseTransferQtyTotal tracks cumulative units moved between warehouses.
	WarehouseTransferQtyTotal prometheus.Counter

	// ── Loyalty ───────────────────────────────────────────────────────────────
	// LoyaltyPointsEarnedTotal counts points awarded across all customers.
	LoyaltyPointsEarnedTotal prometheus.Counter

	// LoyaltyPointsRedeemedTotal counts points redeemed across all customers.
	LoyaltyPointsRedeemedTotal prometheus.Counter

	// LoyaltyTierChangesTotal counts tier promotions.
	// Labels: from_tier, to_tier
	LoyaltyTierChangesTotal *prometheus.CounterVec

	// ── HTTP ──────────────────────────────────────────────────────────────────
	// HTTPRequestsTotal counts all HTTP requests by method + path + status.
	HTTPRequestsTotal *prometheus.CounterVec

	// HTTPRequestDuration measures per-endpoint latency.
	HTTPRequestDuration *prometheus.HistogramVec
}

// New creates and registers all metrics with the provided Prometheus registry.
// Pass prometheus.DefaultRegisterer for production; use a fresh registry for tests.
func New(reg prometheus.Registerer) *Metrics {
	factory := promauto.With(reg)

	return &Metrics{
		OrdersProcessedTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "orders_processed_total",
			Help:      "Total number of orders successfully processed.",
		}, []string{"channel_type", "invoice_type"}),

		OrdersFailedTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "orders_failed_total",
			Help:      "Total number of failed order attempts.",
		}, []string{"channel_type", "reason"}),

		OrderProcessingDuration: factory.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "order_processing_duration_seconds",
			Help:      "End-to-end latency of ProcessOrder including FIFO and DB writes.",
			Buckets:   []float64{0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0},
		}, []string{"channel_type"}),

		StockLevel: factory.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "stock_level",
			Help:      "Current quantity_available per variant.",
		}, []string{"variant_id", "sku"}),

		LowStockEventsTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "low_stock_events_total",
			Help:      "Number of times a variant dropped to or below its reorder_point.",
		}, []string{"variant_id"}),

		FIFOBatchesConsumedTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "fifo_batches_consumed_total",
			Help:      "Number of purchase batch slices consumed by FIFO deductions.",
		}, []string{"variant_id"}),

		StockMovementsTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "stock_movements_total",
			Help:      "Total inventory_movements rows inserted, by movement type.",
		}, []string{"movement_type"}),

		ReservationsCreatedTotal: factory.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "reservations_created_total",
			Help:      "Total stock reservations created.",
		}),

		ReservationsExpiredTotal: factory.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "reservations_expired_total",
			Help:      "Total stock reservations released by the cleanup job.",
		}),

		ActiveReservations: factory.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "active_reservations",
			Help:      "Current count of unexpired active stock reservations.",
		}),

		InvoicesGeneratedTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "invoices_generated_total",
			Help:      "Total order_invoices rows created, by type and trigger reason.",
		}, []string{"invoice_type", "trigger_reason"}),

		InvoiceSerializationDuration: factory.NewHistogram(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "invoice_serialization_duration_seconds",
			Help:      "Time taken to serialize a UBL 2.1 XML invoice.",
			Buckets:   []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1},
		}),

		PendingInvoices: factory.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "pending_invoices",
			Help:      "Orders in confirmed state that have no invoice_number yet.",
		}),

		PriceResolveDuration: factory.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "price_resolve_duration_seconds",
			Help:      "Latency of PriceResolver.Resolve calls.",
			Buckets:   []float64{0.001, 0.005, 0.01, 0.025, 0.05},
		}, []string{"price_source"}),

		PromotionHitsTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "promotion_hits_total",
			Help:      "Number of orders where a promotion price was applied.",
		}, []string{"channel_type", "customer_tier"}),

		ReturnsCreatedTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "returns_created_total",
			Help:      "Total return (RMA) requests created.",
		}, []string{"channel_type", "condition"}),

		QCMismatchTotal: factory.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "qc_photo_mismatch_total",
			Help:      "QC comparisons where customer photo hash did not match outbound.",
		}),

		HTTPRequestsTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "http_requests_total",
			Help:      "Total HTTP requests handled.",
		}, []string{"method", "path", "status"}),

		HTTPRequestDuration: factory.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "http_request_duration_seconds",
			Help:      "HTTP handler latency.",
			Buckets:   prometheus.DefBuckets,
		}, []string{"method", "path"}),

		WarehouseTransfersTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "warehouse_transfers_total",
			Help:      "Total stock transfers between warehouse locations.",
		}, []string{"from_type", "to_type"}),

		WarehouseTransferQtyTotal: factory.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "warehouse_transfer_qty_total",
			Help:      "Cumulative units moved in warehouse transfers.",
		}),

		LoyaltyPointsEarnedTotal: factory.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "loyalty_points_earned_total",
			Help:      "Total loyalty points awarded to customers.",
		}),

		LoyaltyPointsRedeemedTotal: factory.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "loyalty_points_redeemed_total",
			Help:      "Total loyalty points redeemed by customers.",
		}),

		LoyaltyTierChangesTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "loyalty_tier_changes_total",
			Help:      "Number of times a customer's loyalty tier was promoted.",
		}, []string{"from_tier", "to_tier"}),
	}
}
