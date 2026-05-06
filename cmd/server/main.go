package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dubai-retail/os/internal/alerts"
	"github.com/dubai-retail/os/internal/config"
	"github.com/dubai-retail/os/internal/handler/http/router"
	"github.com/dubai-retail/os/internal/metrics"
	"github.com/dubai-retail/os/internal/repository/postgres"
	"github.com/dubai-retail/os/internal/repository/redis"
	"github.com/dubai-retail/os/internal/service"
	"github.com/dubai-retail/os/internal/storage"
	"github.com/dubai-retail/os/internal/worker"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"

	// Register omnichannel connector adapters (init() calls integrations.Register)
	_ "github.com/dubai-retail/os/internal/integrations/amazon"
	_ "github.com/dubai-retail/os/internal/integrations/instagram"
	_ "github.com/dubai-retail/os/internal/integrations/noon"
	_ "github.com/dubai-retail/os/internal/integrations/shopify"
	_ "github.com/dubai-retail/os/internal/integrations/tiktok"

	// Register shipping connector adapters
	_ "github.com/dubai-retail/os/internal/integrations/shipping/aramex"
	_ "github.com/dubai-retail/os/internal/integrations/shipping/dhl"
	_ "github.com/dubai-retail/os/internal/integrations/shipping/emiratespost"
)

func main() {
	// ── Logger ────────────────────────────────────────────────────────────────
	log, _ := zap.NewProduction()
	defer log.Sync() //nolint:errcheck

	// ── Config ────────────────────────────────────────────────────────────────
	cfg, err := config.Load()
	if err != nil {
		log.Fatal("failed to load config", zap.Error(err))
	}

	// ── Database ──────────────────────────────────────────────────────────────
	db, err := postgres.NewPool(cfg.DatabaseURL)
	if err != nil {
		log.Fatal("failed to connect to postgres", zap.Error(err))
	}
	defer db.Close()

	// ── Redis ─────────────────────────────────────────────────────────────────
	rdb := redis.NewClient(cfg.RedisURL)
	defer rdb.Close()

	// ── Observability ─────────────────────────────────────────────────────────
	reg := prometheus.NewRegistry()
	m := metrics.New(reg)

	_ = alerts.New(alerts.Config{
		SlackWebhookOperations:  os.Getenv("SLACK_WEBHOOK_OPS"),
		SlackWebhookCritical:    os.Getenv("SLACK_WEBHOOK_CRITICAL"),
		SlackWebhookCompliance:  os.Getenv("SLACK_WEBHOOK_COMPLIANCE"),
		SlackWebhookEngineering: os.Getenv("SLACK_WEBHOOK_ENGINEERING"),
	}, log)

	// ── Storage ───────────────────────────────────────────────────────────────
	localStore, err := storage.NewLocalStore("./storage/uploads", "http://localhost:8080/uploads")
	if err != nil {
		log.Fatal("failed to initialize local store", zap.Error(err))
	}

	// ── Repositories ──────────────────────────────────────────────────────────
	repos := postgres.NewRepositories(db)

	// ── Services ──────────────────────────────────────────────────────────────
	deps := service.Deps{
		Repos:          repos,
		Pool:           db,
		RDB:            rdb,
		Log:            log,
		ReservationTTL: cfg.ReservationTTL,
		VATRate:        cfg.VATRate,
		Metrics:        m,
		ObjectStore:    localStore,
	}
	svcs := service.NewServices(deps)
	adminSvcs := service.NewAdminServices(deps, repos.InvoiceStore)

	// ── Background Workers (Asynq) ────────────────────────────────────────────
	wrk := worker.NewServer(cfg, rdb, svcs, repos.Reservation, adminSvcs.ChannelSync, adminSvcs.Shipping, m, log)
	go wrk.Start()
	defer wrk.Stop()

	// ── HTTP Server ───────────────────────────────────────────────────────────
	r := router.New(svcs, adminSvcs, m, reg, log)

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Info("server starting", zap.Int("port", cfg.Port))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("server error", zap.Error(err))
		}
	}()

	// ── Graceful shutdown ─────────────────────────────────────────────────────
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("shutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Error("server forced to shutdown", zap.Error(err))
	}
	log.Info("server exited")
}
