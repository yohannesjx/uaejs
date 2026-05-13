package router

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"

	"github.com/dubai-retail/os/internal/metrics"
	appMiddleware "github.com/dubai-retail/os/internal/middleware"
	"github.com/dubai-retail/os/internal/service"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
)

// corsAllowedOrigins merges built-in dev origins with CORS_ALLOWED_ORIGINS (comma-separated).
// Set e.g. CORS_ALLOWED_ORIGINS=http://203.0.113.10:3000 for remote admin dashboards.
func corsAllowedOrigins() []string {
	origins := []string{
		"http://localhost:3000", "http://localhost:3002", "http://localhost:5173",
		"http://127.0.0.1:3000", "http://127.0.0.1:3002", "http://127.0.0.1:5173",
	}
	if extra := strings.TrimSpace(os.Getenv("CORS_ALLOWED_ORIGINS")); extra != "" {
		for _, o := range strings.Split(extra, ",") {
			o = strings.TrimSpace(o)
			if o == "" {
				continue
			}
			origins = append(origins, o)
		}
	}
	return origins
}

func corsOptionsFromEnv() cors.Options {
	o := cors.Options{
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"*"},
		AllowCredentials: true,
		MaxAge:           300,
	}
	relaxed := os.Getenv("CORS_ALLOW_ANY_ORIGIN")
	if relaxed == "1" || strings.EqualFold(relaxed, "true") {
		// Reflects the request Origin (works with AllowCredentials). Do not use on untrusted public APIs.
		o.AllowOriginFunc = func(_ *http.Request, origin string) bool {
			return strings.TrimSpace(origin) != ""
		}
		return o
	}
	o.AllowedOrigins = corsAllowedOrigins()
	return o
}

// New builds and returns the root HTTP router.
func New(
	svcs *service.Services,
	adminSvcs *service.AdminServices,
	m *metrics.Metrics,
	reg *prometheus.Registry,
	log *zap.Logger,
) http.Handler {
	r := chi.NewRouter()

	tenantMW := appMiddleware.NewTenantMiddleware(adminSvcs.Tenant, log)

	// ── CORS (admin + storefront origins via CORS_ALLOWED_ORIGINS; optional CORS_ALLOW_ANY_ORIGIN) ──
	r.Use(cors.Handler(corsOptionsFromEnv()))

	// ── Global middleware ────────────────────────────────────────────────────
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(zapMiddleware(log))
	r.Use(middleware.Recoverer)
	r.Use(middleware.StripSlashes)
	r.Use(tenantMW.Resolve)
	r.Use(m.HTTPMiddleware)

	// ── Observability ────────────────────────────────────────────────────────
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	r.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{Registry: reg}))

	// ── Serve Local Uploads (for dev) ─────────────────────────────────────────
	// Downscaled JPEGs for admin grids; register before the /uploads/* wildcard.
	r.Get("/uploads/thumb", serveUploadThumbnail(log))
	r.Get("/uploads/*", http.StripPrefix("/uploads/", http.FileServer(http.Dir("./storage/uploads"))).ServeHTTP)

	// ── API v1 ───────────────────────────────────────────────────────────────
	r.Route("/api/v1", func(r chi.Router) {

		r.Route("/products", func(r chi.Router) {
			ph := newProductHandler(svcs.Product, log)
			r.Post("/", ph.CreateProduct)
			r.Get("/{id}", ph.GetProduct)
			r.Put("/{id}/prices", ph.SetPrice)
			r.Get("/{id}/prices", ph.GetPrice)
		})

		r.Route("/orders", func(r chi.Router) {
			oh := newOrderHandler(svcs.Order, log)
			ih := newInvoiceHandler(svcs.Order, log)
			r.Post("/", oh.ProcessOrder)
			r.Get("/{id}", oh.GetOrder)
			r.Post("/{id}/invoice", ih.IssueInvoice)
			r.Get("/{id}/invoice", ih.GetInvoiceXML)
		})

		r.Route("/inventory", func(r chi.Router) {
			ih := newInventoryHandler(svcs.Inventory, log)
			r.Post("/deduct", ih.DeductStock)
			r.Post("/reserve", ih.ReserveStock)
		})

		r.Route("/returns", func(r chi.Router) {
			rh := newRMAHandler(svcs.RMA)
			r.Post("/", rh.CreateReturn)
			r.Get("/{id}", rh.GetReturn)
			r.Post("/{id}/approve", rh.ApproveReturn)
			r.Post("/{id}/reject", rh.RejectReturn)
			r.Post("/{id}/items/{itemId}/photos", rh.UploadPhoto)
		})
	})

	// ── Public storefront routes (no JWT required) ───────────────────────────
	r.Get("/store/collections", listPublicCollectionsHandler(svcs.Collection, log))
	r.Get("/store/products", listPublicProductsHandler(svcs, log))

	// ── POS endpoints (JWT required, no specific RBAC permission needed) ─────
	posH := newPOSHandler(adminSvcs.POS)
	authMwPOS := appMiddleware.New(adminSvcs.Auth, log)
	r.Route("/pos", func(r chi.Router) {
		r.Use(authMwPOS.Authenticate)
		r.Post("/sessions/open", posH.OpenSession)
		r.Post("/sessions/close", posH.CloseSession)
		r.Post("/orders", posH.CreateOrder)
		r.Post("/orders/{id}/pay", posH.PayOrder)
		r.Get("/orders/{id}/receipt", posH.GetReceipt)
		r.Get("/products/scan", posH.ScanBarcode)
	})

	// ── Public auth endpoints (no JWT required) ──────────────────────────────
	authH := newAuthHandler(adminSvcs.Auth)
	authMw := appMiddleware.New(adminSvcs.Auth, log)

	r.Route("/auth", func(r chi.Router) {
		r.Post("/login", authH.Login)
		r.Post("/refresh", authH.Refresh)
		r.Post("/logout", authH.Logout)
		r.With(authMw.Authenticate).Get("/me", authH.Me)
	})

	// ── Admin routes — JWT required ───────────────────────────────────────────
	r.Route("/admin", func(r chi.Router) {
		r.Use(authMw.Authenticate) // all /admin routes require a valid token

		// Batch operations
		ah := newAdminHandler(adminSvcs.BatchImport, adminSvcs.ASPSandbox)
		r.With(appMiddleware.RequirePermission("inventory.manage")).Post("/batches/import", ah.ImportBatch)
		r.With(appMiddleware.RequirePermission("invoices.sandbox")).Post("/invoices/{orderId}/sandbox", ah.SubmitToSandbox)

		// Analytics
		r.With(appMiddleware.RequirePermission("analytics.view")).Get("/analytics/forecast", analyticsHandler(adminSvcs.Analytics))
		r.With(appMiddleware.RequirePermission("analytics.view")).Get("/analytics/reorder", reorderHandler(adminSvcs.Analytics))
		r.With(appMiddleware.RequirePermission("analytics.view")).Get("/analytics/promotions", promotionEfficacyHandler(adminSvcs.Analytics))
		r.With(appMiddleware.RequirePermission("analytics.view")).Get("/analytics/fraud", fraudSignalsHandler(adminSvcs.Analytics))

		// Auth management
		// POST /admin/auth/revoke-all — emergency global session revocation
		r.With(appMiddleware.RequirePermission("users.manage")).Post("/auth/revoke-all", authH.RevokeAll)

		// User management
		r.With(appMiddleware.RequirePermission("users.manage")).Get("/users", authH.ListUsers)
		r.With(appMiddleware.RequirePermission("users.manage")).Post("/users", authH.CreateUser)
		r.With(appMiddleware.RequirePermission("users.manage")).Patch("/users/{id}", authH.UpdateUser)
		r.With(appMiddleware.RequirePermission("users.manage")).Post("/users/{id}/roles", authH.AssignRole)

		// Supplier module
		sh := newSupplierHandler(adminSvcs.Supplier)
		r.With(appMiddleware.RequirePermission("suppliers.manage")).Get("/suppliers", sh.ListSuppliers)
		r.With(appMiddleware.RequirePermission("suppliers.manage")).Post("/suppliers", sh.CreateSupplier)
		r.With(appMiddleware.RequirePermission("suppliers.manage")).Patch("/suppliers/{id}", sh.UpdateSupplier)
		r.With(appMiddleware.RequirePermission("suppliers.manage")).Get("/purchase-orders", sh.ListPOs)
		r.With(appMiddleware.RequirePermission("suppliers.manage")).Post("/purchase-orders", sh.CreatePO)
		r.With(appMiddleware.RequirePermission("suppliers.manage")).Post("/purchase-orders/{id}/items", sh.AddPOItem)
		r.With(appMiddleware.RequirePermission("suppliers.manage")).Post("/purchase-orders/{id}/receive", sh.ReceivePO)

		// Omnichannel
		ch := newChannelHandler(adminSvcs.ChannelSync)
		r.With(appMiddleware.RequirePermission("channels.manage")).Get("/channels", ch.ListPlatforms)
		r.With(appMiddleware.RequirePermission("channels.manage")).Post("/channels/connect", ch.ConnectPlatform)
		r.With(appMiddleware.RequirePermission("channels.manage")).Post("/channels/{id}/sync-products", ch.SyncProducts)
		r.With(appMiddleware.RequirePermission("channels.manage")).Post("/channels/{id}/sync-inventory", ch.SyncInventory)
		r.With(appMiddleware.RequirePermission("channels.manage")).Get("/channels/orders", ch.ListPlatformOrders)

		// Shipping
		sh2 := newShippingHandler(adminSvcs.Shipping)
		r.With(appMiddleware.RequirePermission("inventory.manage")).Post("/shipments/create", sh2.CreateShipment)
		r.With(appMiddleware.RequirePermission("inventory.manage")).Get("/shipments/{id}", sh2.GetShipment)
		r.With(appMiddleware.RequirePermission("inventory.manage")).Get("/shipments/{id}/tracking", sh2.GetTracking)
		r.With(appMiddleware.RequirePermission("inventory.manage")).Post("/shipping/accounts", sh2.AddAccount)

		// Tenant management
		th := newTenantHandler(adminSvcs.Tenant)
		r.With(appMiddleware.RequirePermission("users.manage")).Get("/tenants", th.List)
		r.With(appMiddleware.RequirePermission("users.manage")).Post("/tenants", th.Create)
		r.With(appMiddleware.RequirePermission("users.manage")).Get("/tenants/{id}", th.Get)
		r.With(appMiddleware.RequirePermission("users.manage")).Get("/tenants/{id}/settings", th.GetSettings)
		r.With(appMiddleware.RequirePermission("users.manage")).Put("/tenants/{id}/settings", th.SaveSettings)
		r.With(appMiddleware.RequirePermission("users.manage")).Post("/tenants/{id}/users", th.AddUser)

		// Products
		ph := newProductHandler(svcs.Product, log)
		r.With(appMiddleware.RequirePermission("products.read")).Get("/products", listProductsHandler(svcs.Product, log))
		r.With(appMiddleware.RequirePermission("products.read")).Post("/products/drafts", ph.CreateDraft)

		// Categories (Must be defined before parameterized /products/{id} to prevent 405 shadowing)
		catH := newCategoryHandler(svcs.Category, log)
		r.With(appMiddleware.RequirePermission("products.read")).Post("/products/categories", catH.CreateCategory)
		r.With(appMiddleware.RequirePermission("products.read")).Get("/products/categories", catH.ListCategories)
		r.With(appMiddleware.RequirePermission("products.read")).Get("/products/categories/{id}", catH.GetCategory)
		r.With(appMiddleware.RequirePermission("products.read")).Patch("/products/categories/{id}", catH.PatchCategory)
		r.With(appMiddleware.RequirePermission("products.read")).Delete("/products/categories/{id}", catH.DeleteCategory)

		colH := newCollectionHandler(svcs.Collection, log)
		r.With(appMiddleware.RequirePermission("products.read")).Post("/collections", colH.Create)
		r.With(appMiddleware.RequirePermission("products.read")).Get("/collections", colH.List)
		r.With(appMiddleware.RequirePermission("products.read")).Get("/collections/{id}", colH.Get)
		r.With(appMiddleware.RequirePermission("products.write")).Patch("/collections/{id}", colH.Patch)
		r.With(appMiddleware.RequirePermission("products.write")).Delete("/collections/{id}", colH.Delete)

		r.With(appMiddleware.RequirePermission("products.read")).Patch("/products/{id}", ph.UpdateProduct)
		r.With(appMiddleware.RequirePermission("products.write")).Delete("/products/{id}", ph.DeleteProduct)
		r.With(appMiddleware.RequirePermission("products.write")).Post("/products/{id}/variants/default", ph.UpsertProductDefaultVariant)
		r.With(appMiddleware.RequirePermission("products.write")).Post("/products/{id}/variants", ph.CreateProductVariant)
		r.With(appMiddleware.RequirePermission("products.write")).Patch("/products/variants/{id}", ph.PatchVariant)
		r.With(appMiddleware.RequirePermission("products.write")).Delete("/products/variants/{id}", ph.DeleteVariant)
		r.With(appMiddleware.RequirePermission("products.write")).Post("/products/{id}/duplicate", ph.DuplicateProduct)

		// Media
		mh := newMediaHandler(svcs.Media, log)
		r.With(appMiddleware.RequirePermission("products.read")).Post("/media/upload", mh.UploadMedia)
		r.With(appMiddleware.RequirePermission("products.read")).Post("/media/import-url", mh.ImportMediaFromURL)
		r.With(appMiddleware.RequirePermission("products.read")).Get("/media", mh.ListMedia)
		r.With(appMiddleware.RequirePermission("products.read")).Patch("/media/{id}", mh.PatchMedia)
		r.With(appMiddleware.RequirePermission("products.read")).Delete("/media/{id}", mh.DeleteMedia)

		// Orders list (paginated)
		r.With(appMiddleware.RequirePermission("orders.manage")).Get("/orders", listOrdersHandler(svcs.Order, log))

		// Returns list (paginated)
		r.With(appMiddleware.RequirePermission("returns.approve")).Get("/returns", listReturnsHandler(svcs.RMA, log))

		// Shipments list (paginated)
		r.With(appMiddleware.RequirePermission("inventory.manage")).Get("/shipments", listShipmentsHandler(adminSvcs.Shipping, log))

		// Activity log (paginated)
		r.With(appMiddleware.RequirePermission("analytics.view")).Get("/activity-log", listActivityLogHandler(adminSvcs.ActivityLog, log))

		// Warehouses
		wh2 := newWarehouseHandler(adminSvcs.Warehouse, adminSvcs.ActivityLog)
		r.With(appMiddleware.RequirePermission("inventory.manage")).Get("/warehouses", wh2.List)
		r.With(appMiddleware.RequirePermission("inventory.manage")).Get("/inventory", wh2.ListInventoryRows)
		r.With(appMiddleware.RequirePermission("inventory.manage")).Post("/inventory/adjust", wh2.AdjustStock)
		r.With(appMiddleware.RequirePermission("inventory.manage")).Post("/warehouses", wh2.Create)
		r.With(appMiddleware.RequirePermission("inventory.manage")).Put("/warehouses/{id}", wh2.Update)
		r.With(appMiddleware.RequirePermission("inventory.manage")).Get("/warehouses/{id}/inventory", wh2.GetInventory)
		r.With(appMiddleware.RequirePermission("inventory.manage")).Post("/warehouses/{id}/stock", wh2.SetStock)
		r.With(appMiddleware.RequirePermission("inventory.manage")).Post("/warehouses/transfer", wh2.Transfer)
		r.With(appMiddleware.RequirePermission("inventory.manage")).Get("/transfers", wh2.ListTransfers)
		r.With(appMiddleware.RequirePermission("inventory.manage")).Post("/transfers", wh2.CreateTransfer)
		r.With(appMiddleware.RequirePermission("inventory.manage")).Get("/transfers/{id}", wh2.GetTransfer)
		r.With(appMiddleware.RequirePermission("inventory.manage")).Patch("/transfers/{id}", wh2.UpdateTransfer)
		r.With(appMiddleware.RequirePermission("inventory.manage")).Post("/transfers/{id}/status", wh2.TransitionTransfer)
	})

	// ── Customer-facing routes (JWT required) ─────────────────────────────────
	customerAuthMw := appMiddleware.New(adminSvcs.Auth, log)
	ch2 := newCustomerHandler(adminSvcs.Customer, adminSvcs.Loyalty)
	r.Route("/customers", func(r chi.Router) {
		r.Use(customerAuthMw.Authenticate)
		r.Get("/", listCustomersHandler(adminSvcs.Customer, log))
		r.Post("/", ch2.Create)
		r.Get("/{id}", ch2.Get)
		r.Post("/{id}/points/add", ch2.AddPoints)
		r.Post("/{id}/points/redeem", ch2.RedeemPoints)
		r.Get("/{id}/points/history", ch2.PointsHistory)
	})

	return r
}

// ── Analytics handler shims ──────────────────────────────────────────────────
// These delegate to service.AnalyticsService methods and render JSON.

func analyticsHandler(svc *service.AnalyticsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sku := r.URL.Query().Get("sku")
		channel := r.URL.Query().Get("channel")
		if sku == "" {
			http.Error(w, "sku query param is required", http.StatusBadRequest)
			return
		}
		result, err := svc.ForecastDemand(r.Context(), sku, channel)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	}
}

func reorderHandler(svc *service.AnalyticsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		suggestions, err := svc.SuggestReorders(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(suggestions)
	}
}

func promotionEfficacyHandler(svc *service.AnalyticsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		report, err := svc.PromotionEfficacy(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(report)
	}
}

func fraudSignalsHandler(svc *service.AnalyticsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		signals, err := svc.FraudSignals(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(signals)
	}
}

// zapMiddleware logs each request with method, path, and request ID.
func zapMiddleware(log *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			log.Info("http.request",
				zap.String("method", r.Method),
				zap.String("path", r.URL.Path),
				zap.String("remote", r.RemoteAddr),
				zap.String("request_id", middleware.GetReqID(r.Context())),
			)
			next.ServeHTTP(w, r)
		})
	}
}
