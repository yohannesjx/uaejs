package postgres

import "github.com/jackc/pgx/v5/pgxpool"

// Repositories bundles all postgres repository instances.
type Repositories struct {
	Inventory    *InventoryRepository
	Product      *ProductRepository
	Order        *OrderRepository
	Pricing      *PricingRepository
	InvoiceStore *InvoiceStoreRepository
	RMA          *RMARepository
	Reservation  *ReservationRepository
	BatchImport  *BatchImportRepository
	Analytics    *AnalyticsRepository
	Auth         *AuthRepository
	Supplier     *SupplierRepository
	ChannelSync  *ChannelSyncRepository
	POS          *POSRepository
	Shipping     *ShippingRepository
	Tenant       *TenantRepository
	Warehouse    *WarehouseRepository
	Customer     *CustomerRepository
	ActivityLog  *ActivityLogRepository
	Media        *MediaRepository
}

// NewRepositories wires all repositories to the shared connection pool.
func NewRepositories(pool *pgxpool.Pool) *Repositories {
	return &Repositories{
		Inventory:    &InventoryRepository{pool: pool},
		Product:      &ProductRepository{pool: pool},
		Order:        &OrderRepository{pool: pool},
		Pricing:      &PricingRepository{pool: pool},
		InvoiceStore: &InvoiceStoreRepository{pool: pool},
		RMA:          &RMARepository{pool: pool},
		Reservation:  &ReservationRepository{pool: pool},
		BatchImport:  &BatchImportRepository{pool: pool},
		Analytics:    &AnalyticsRepository{pool: pool},
		Auth:         &AuthRepository{pool: pool},
		Supplier:     &SupplierRepository{pool: pool},
		ChannelSync:  &ChannelSyncRepository{pool: pool},
		POS:          &POSRepository{pool: pool},
		Shipping:     &ShippingRepository{pool: pool},
		Tenant:       &TenantRepository{pool: pool},
		Warehouse:    &WarehouseRepository{pool: pool},
		Customer:     &CustomerRepository{pool: pool},
		ActivityLog:  &ActivityLogRepository{pool: pool},
		Media:        &MediaRepository{pool: pool},
	}
}
