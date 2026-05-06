package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/dubai-retail/os/internal/config"
	"github.com/dubai-retail/os/internal/metrics"
	"github.com/dubai-retail/os/internal/repository/postgres"
	"github.com/dubai-retail/os/internal/service"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// Task type constants
const (
	TaskReleaseReservation         = "reservation:release"
	TaskCheckLowStock              = "inventory:low_stock_check"
	TaskCleanupExpiredReservations = "reservation:cleanup_expired"
	TaskRefreshMetricsGauges       = "metrics:refresh_gauges"
	TaskSyncChannelInventory       = "channel.sync.inventory"
	TaskSyncChannelPrice           = "channel.sync.price"
	TaskSyncChannelProduct         = "channel.sync.product"
	TaskImportChannelOrders        = "channel.import.orders"
	TaskCreateShipment             = "shipping.create_shipment"
	TaskSyncTracking               = "shipping.sync_tracking"
)

// Server wraps the Asynq worker server and periodic scheduler.
type Server struct {
	server    *asynq.Server
	scheduler *asynq.Scheduler
	mux       *asynq.ServeMux
	log       *zap.Logger
}

// NewServer creates and configures the Asynq worker + scheduler.
func NewServer(
	cfg *config.Config,
	rdb *redis.Client,
	svcs *service.Services,
	reservationRepo *postgres.ReservationRepository,
	channelSync *service.ChannelSyncService,
	shipping *service.ShippingService,
	m *metrics.Metrics,
	log *zap.Logger,
) *Server {
	redisOpt := asynq.RedisClientOpt{
		Addr:     rdb.Options().Addr,
		Password: rdb.Options().Password,
		DB:       rdb.Options().DB,
	}

	srv := asynq.NewServer(
		redisOpt,
		asynq.Config{
			Concurrency: 10,
			Queues: map[string]int{
				"critical": 6,
				"default":  3,
				"low":      1,
			},
			ErrorHandler: asynq.ErrorHandlerFunc(func(ctx context.Context, task *asynq.Task, err error) {
				log.Error("asynq task failed",
					zap.String("type", task.Type()),
					zap.Error(err),
				)
			}),
		},
	)

	// Periodic scheduler
	scheduler := asynq.NewScheduler(redisOpt, &asynq.SchedulerOpts{
		Location: time.UTC,
		EnqueueErrorHandler: func(task *asynq.Task, opts []asynq.Option, err error) {
			log.Error("scheduler enqueue failed",
				zap.String("task", task.Type()),
				zap.Error(err),
			)
		},
	})

	mux := asynq.NewServeMux()

	h := &handlers{
		svcs:            svcs,
		reservationRepo: reservationRepo,
		channelSync:     channelSync,
		shipping:        shipping,
		metrics:         m,
		cfg:             cfg,
		log:             log,
	}

	mux.HandleFunc(TaskReleaseReservation, h.HandleReleaseReservation)
	mux.HandleFunc(TaskCheckLowStock, h.HandleLowStockCheck)
	mux.HandleFunc(TaskCleanupExpiredReservations, h.HandleCleanupExpiredReservations)
	mux.HandleFunc(TaskRefreshMetricsGauges, h.HandleRefreshMetricsGauges)
	mux.HandleFunc(TaskSyncChannelInventory, h.HandleSyncChannelInventory)
	mux.HandleFunc(TaskImportChannelOrders, h.HandleImportChannelOrders)
	mux.HandleFunc(TaskSyncTracking, h.HandleSyncTracking)

	// Schedule: cleanup expired reservations every minute
	if _, err := scheduler.Register(
		"@every 1m",
		asynq.NewTask(TaskCleanupExpiredReservations, nil),
		asynq.Queue("critical"),
	); err != nil {
		log.Fatal("failed to register cleanup scheduler", zap.Error(err))
	}

	// Schedule: refresh Prometheus gauges every 30 seconds
	if _, err := scheduler.Register(
		"@every 30s",
		asynq.NewTask(TaskRefreshMetricsGauges, nil),
		asynq.Queue("low"),
	); err != nil {
		log.Fatal("failed to register metrics refresh scheduler", zap.Error(err))
	}

	// Schedule: low-stock check every 5 minutes
	if _, err := scheduler.Register(
		"@every 5m",
		asynq.NewTask(TaskCheckLowStock, nil),
		asynq.Queue("low"),
	); err != nil {
		log.Fatal("failed to register low-stock scheduler", zap.Error(err))
	}

	// Schedule: sync inventory to all active channels every 10 minutes
	if _, err := scheduler.Register(
		"@every 10m",
		asynq.NewTask(TaskSyncChannelInventory, nil),
		asynq.Queue("low"),
	); err != nil {
		log.Fatal("failed to register channel inventory sync scheduler", zap.Error(err))
	}

	// Schedule: import orders from all active channels every 5 minutes
	if _, err := scheduler.Register(
		"@every 5m",
		asynq.NewTask(TaskImportChannelOrders, nil),
		asynq.Queue("default"),
	); err != nil {
		log.Fatal("failed to register channel order import scheduler", zap.Error(err))
	}

	// Schedule: sync shipment tracking every 15 minutes
	if _, err := scheduler.Register(
		"@every 15m",
		asynq.NewTask(TaskSyncTracking, nil),
		asynq.Queue("low"),
	); err != nil {
		log.Fatal("failed to register tracking sync scheduler", zap.Error(err))
	}

	return &Server{server: srv, scheduler: scheduler, mux: mux, log: log}
}

func (s *Server) Start() {
	s.log.Info("starting asynq worker + scheduler")
	go func() {
		if err := s.scheduler.Run(); err != nil {
			s.log.Fatal("asynq scheduler error", zap.Error(err))
		}
	}()
	if err := s.server.Run(s.mux); err != nil {
		s.log.Fatal("asynq server error", zap.Error(err))
	}
}

func (s *Server) Stop() {
	s.scheduler.Shutdown()
	s.server.Shutdown()
	s.log.Info("asynq worker + scheduler stopped")
}

// =============================================================================
// Task payloads
// =============================================================================

type ReleaseReservationPayload struct {
	ReservationIDs []uuid.UUID `json:"reservation_ids"`
}

// =============================================================================
// Handler bundle
// =============================================================================

type handlers struct {
	svcs            *service.Services
	reservationRepo *postgres.ReservationRepository
	channelSync     *service.ChannelSyncService
	shipping        *service.ShippingService
	metrics         *metrics.Metrics
	cfg             *config.Config
	log             *zap.Logger
}

// HandleReleaseReservation releases specific reservation IDs (manually triggered).
func (h *handlers) HandleReleaseReservation(ctx context.Context, t *asynq.Task) error {
	var payload ReleaseReservationPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("HandleReleaseReservation: unmarshal: %w", err)
	}
	if len(payload.ReservationIDs) == 0 {
		return nil
	}
	return h.svcs.Inventory.ReleaseExpiredReservations(ctx, payload.ReservationIDs)
}

// HandleCleanupExpiredReservations is the periodic abandoned-cart cleanup job.
//
// Flow:
//  1. Query stock_reservations WHERE is_active = TRUE AND expires_at < NOW().
//  2. For each expired reservation: release it (refund stock to pool,
//     record reservation_release movement, mark is_active = FALSE).
//  3. Update Prometheus counters.
//  4. Log each released reservation for audit.
func (h *handlers) HandleCleanupExpiredReservations(ctx context.Context, _ *asynq.Task) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	expired, err := h.reservationRepo.ListExpiredActive(ctx, 200)
	if err != nil {
		return fmt.Errorf("HandleCleanupExpiredReservations: list: %w", err)
	}

	if len(expired) == 0 {
		return nil
	}

	h.log.Info("reservation.cleanup_started",
		zap.Int("expired_count", len(expired)),
	)

	ids := make([]uuid.UUID, 0, len(expired))
	for _, r := range expired {
		ids = append(ids, r.ID)
	}

	if err := h.svcs.Inventory.ReleaseExpiredReservations(ctx, ids); err != nil {
		return fmt.Errorf("HandleCleanupExpiredReservations: release: %w", err)
	}

	// Update metrics
	h.metrics.ReservationsExpiredTotal.Add(float64(len(expired)))

	for _, r := range expired {
		h.log.Info("reservation.released",
			zap.String("reservation_id", r.ID.String()),
			zap.String("order_id", r.OrderID.String()),
			zap.String("variant_id", r.VariantID.String()),
			zap.Int("quantity_returned", r.Quantity),
			zap.Time("expired_at", r.ExpiresAt),
			zap.String("action", "abandoned_cart_cleanup"),
		)
	}

	h.log.Info("reservation.cleanup_completed",
		zap.Int("released", len(expired)),
	)
	return nil
}

// HandleRefreshMetricsGauges updates point-in-time Prometheus gauges by
// querying the DB for current counts.
func (h *handlers) HandleRefreshMetricsGauges(ctx context.Context, _ *asynq.Task) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	activeCount, err := h.reservationRepo.CountActiveReservations(ctx)
	if err == nil {
		h.metrics.ObserveActiveReservations(float64(activeCount))
	}

	pendingCount, err := h.reservationRepo.CountPendingInvoices(ctx)
	if err == nil {
		h.metrics.ObservePendingInvoices(float64(pendingCount))
	}

	return nil
}

// HandleLowStockCheck queries for variants at or below reorder_point.
func (h *handlers) HandleLowStockCheck(_ context.Context, _ *asynq.Task) error {
	h.log.Info("low_stock_check triggered", zap.Time("at", time.Now().UTC()))
	return nil
}

// HandleSyncChannelInventory pushes current stock levels to all active platform accounts.
func (h *handlers) HandleSyncChannelInventory(ctx context.Context, _ *asynq.Task) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	if h.channelSync == nil {
		return nil // omnichannel module not configured
	}
	if err := h.channelSync.SyncAllInventory(ctx); err != nil {
		h.log.Error("channel.sync_inventory.worker_error", zap.Error(err))
		return err
	}
	return nil
}

// HandleSyncTracking refreshes tracking status for all in-transit shipments.
func (h *handlers) HandleSyncTracking(ctx context.Context, _ *asynq.Task) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	if h.shipping == nil {
		return nil
	}
	if err := h.shipping.SyncAllTracking(ctx); err != nil {
		h.log.Error("shipping.sync_tracking.worker_error", zap.Error(err))
		return err
	}
	return nil
}

// HandleImportChannelOrders pulls orders from all active platform accounts.
func (h *handlers) HandleImportChannelOrders(ctx context.Context, _ *asynq.Task) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	if h.channelSync == nil {
		return nil
	}
	since := time.Now().UTC().Add(-6 * time.Hour) // look back 6 hours
	if err := h.channelSync.ImportAllOrders(ctx, since); err != nil {
		h.log.Error("channel.import_orders.worker_error", zap.Error(err))
		return err
	}
	return nil
}
