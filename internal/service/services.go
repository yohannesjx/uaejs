package service

import (
	"os"
	"time"

	"github.com/dubai-retail/os/internal/domain"
	"github.com/dubai-retail/os/internal/integrations"
	"github.com/dubai-retail/os/internal/integrations/shipping"
	"github.com/dubai-retail/os/internal/invoice"
	"github.com/dubai-retail/os/internal/metrics"
	"github.com/dubai-retail/os/internal/repository/postgres"
	"github.com/dubai-retail/os/internal/storage"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// Deps groups all external dependencies injected into the service layer.
type Deps struct {
	Repos          *postgres.Repositories
	Pool           *pgxpool.Pool
	RDB            *redis.Client
	Log            *zap.Logger
	ReservationTTL time.Duration
	VATRate        decimal.Decimal
	Metrics        *metrics.Metrics
	PhotoStorage   PhotoStorage // optional; if nil, RMA photo uploads are disabled
	ObjectStore    storage.ObjectStore
}

// AdminServices holds services only accessible to admin/internal callers.
type AdminServices struct {
	BatchImport *BatchImportService
	ASPSandbox  *ASPSandboxService
	Analytics   *AnalyticsService
	Auth        *AuthService
	Supplier    *SupplierService
	ChannelSync *ChannelSyncService
	POS         *POSService
	Shipping    *ShippingService
	Tenant      *TenantService
	Warehouse   *WarehouseService
	Customer    *CustomerService
	Loyalty     *LoyaltyService
	ActivityLog *ActivityLogService
}

// Services bundles all business logic instances.
type Services struct {
	Inventory     *InventoryService
	Product       *ProductService
	PriceResolver *PriceResolver
	Order         *OrderService
	Compliance    *ComplianceService
	RMA           *RMAService
	Media         *MediaService
	Category      *CategoryService
	Collection    *CollectionService
}

// NewServices wires all services from the dependency bag.
func NewServices(d Deps) *Services {
	inventorySvc := NewInventoryService(
		d.Repos.Inventory,
		d.Pool,
		d.Log,
		d.VATRate,
	)

	collectionRepo := postgres.NewCollectionRepository(d.Pool)

	productSvc := NewProductService(
		d.Repos.Product,
		d.Pool,
		d.Log,
		collectionRepo,
	)

	collectionSvc := NewCollectionService(
		collectionRepo,
		d.Pool,
		d.Log,
	)

	priceResolver := NewPriceResolver(
		d.Repos.Pricing,
		d.VATRate,
		d.Log,
	)

	complianceSvc := buildComplianceService(
		d.Repos.InvoiceStore,
		d.Repos.Order,
		d.Log,
	)

	orderSvc := NewOrderService(
		d.Repos.Order,
		d.Repos.Pricing,
		inventorySvc,
		priceResolver,
		complianceSvc,
		d.Pool,
		d.VATRate,
		d.Log,
	)

	return &Services{
		Inventory:     inventorySvc,
		Product:       productSvc,
		PriceResolver: priceResolver,
		Order:         orderSvc,
		Compliance:    complianceSvc,
		RMA: NewRMAService(
			d.Repos.RMA,
			d.Repos.Inventory,
			d.PhotoStorage,
			d.Pool,
			d.Metrics,
			d.Log,
		),
		Media: NewMediaService(
			d.Repos.Media,
			d.ObjectStore,
			d.Log,
		),
		Category: NewCategoryService(
			postgres.NewCategoryRepository(d.Pool),
			d.Pool,
			d.Log,
		),
		Collection: collectionSvc,
	}
}

// buildComplianceService constructs the ComplianceService using seller identity
// loaded from environment variables (set in .env / docker-compose).
func buildComplianceService(
	invoiceStore InvoiceStoreRepo,
	orderRepo OrderRepo,
	log *zap.Logger,
) *ComplianceService {
	nameAR := os.Getenv("SELLER_NAME_AR")
	var nameARPtr *string
	if nameAR != "" {
		nameARPtr = &nameAR
	}

	license := os.Getenv("SELLER_TRADE_LICENSE")
	var licensePtr *string
	if license != "" {
		licensePtr = &license
	}

	contact := os.Getenv("SELLER_EMAIL")
	var contactPtr *string
	if contact != "" {
		contactPtr = &contact
	}

	sellerName := envOrDefault("SELLER_NAME", "Dubai Fashion House LLC")
	sellerTRN := envOrDefault("SELLER_TRN", "100000000000003")
	endpointID := envOrDefault("SELLER_GLN", "0000000000000")

	sellerProfile := invoice.SellerProfile{
		LegalName:          sellerName,
		LegalNameAR:        nameARPtr,
		TRN:                sellerTRN,
		TradeLicenseNumber: licensePtr,
		EndpointID:         endpointID,
		Address: domain.AddressUBL{
			StreetName:  envOrDefault("SELLER_STREET", "Al Quoz Industrial Area 3"),
			CityName:    envOrDefault("SELLER_CITY", "Dubai"),
			PostalZone:  os.Getenv("SELLER_POSTAL"),
			CountryCode: "AE",
		},
		ContactEmail: contactPtr,
	}

	serializer := &invoice.Serializer{SellerProfile: sellerProfile}

	return NewComplianceService(
		serializer,
		invoiceStore,
		orderRepo,
		sellerName,
		sellerTRN,
		sellerProfile.Address,
		log,
	)
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// NewAdminServices constructs the admin-only service bundle.
func NewAdminServices(d Deps, invoiceRepo ASPInvoiceRepo) *AdminServices {
	aspEndpoint := envOrDefault("ASP_SANDBOX_ENDPOINT", "https://sandbox.asp.example.ae/validate")
	aspAPIKey := os.Getenv("ASP_API_KEY")

	aspSvc := NewASPSandboxService(
		ASPSandboxConfig{
			EndpointURL: aspEndpoint,
			APIKey:      aspAPIKey,
		},
		nil, // use default httpASPClient
		invoiceRepo,
		d.Metrics,
		d.Log,
	)

	batchImportSvc := NewBatchImportService(
		d.Repos.BatchImport,
		d.Pool,
		d.Metrics,
		d.Log,
	)

	analyticsSvc := NewAnalyticsService(
		d.Repos.Analytics,
		d.Log,
	)

	authSvc := NewAuthService(
		d.Repos.Auth,
		d.RDB,
		AuthConfig{
			JWTSecret: envOrDefault("JWT_SECRET", "change_me_to_a_32_byte_secret!!!"),
		},
		d.Pool,
		d.Log,
	)

	supplierSvc := NewSupplierService(
		d.Repos.Supplier,
		d.Repos.BatchImport,
		d.Pool,
		d.Log,
	)

	channelSyncSvc := NewChannelSyncService(
		d.Repos.ChannelSync,
		d.Repos.Inventory,
		integrations.Registry,
		d.Log,
	)

	// Build the core order service so POS can reuse it.
	orderSvc := NewOrderService(
		d.Repos.Order,
		d.Repos.Pricing,
		NewInventoryService(d.Repos.Inventory, d.Pool, d.Log, d.VATRate),
		NewPriceResolver(d.Repos.Pricing, d.VATRate, d.Log),
		buildComplianceService(d.Repos.InvoiceStore, d.Repos.Order, d.Log),
		d.Pool,
		d.VATRate,
		d.Log,
	)

	posSvc := NewPOSService(d.Repos.POS, orderSvc, d.Log)
	shippingSvc := NewShippingService(d.Repos.Shipping, shipping.Registry, d.Log)
	tenantSvc := NewTenantService(d.Repos.Tenant, d.Log)
	warehouseSvc := NewWarehouseService(d.Repos.Warehouse, d.Pool, d.Log)
	customerSvc := NewCustomerService(d.Repos.Customer, d.Pool, d.Log)
	loyaltySvc := NewLoyaltyService(d.Repos.Customer, d.Pool, d.Log)
	activityLogSvc := NewActivityLogService(d.Repos.ActivityLog)

	return &AdminServices{
		BatchImport: batchImportSvc,
		ASPSandbox:  aspSvc,
		Analytics:   analyticsSvc,
		Auth:        authSvc,
		Supplier:    supplierSvc,
		ChannelSync: channelSyncSvc,
		POS:         posSvc,
		Shipping:    shippingSvc,
		Tenant:      tenantSvc,
		Warehouse:   warehouseSvc,
		Customer:    customerSvc,
		Loyalty:     loyaltySvc,
		ActivityLog: activityLogSvc,
	}
}
