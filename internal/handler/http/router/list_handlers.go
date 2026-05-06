package router

import (
	"net/http"
	"strconv"
	"time"

	"github.com/dubai-retail/os/internal/domain"
	"github.com/dubai-retail/os/internal/middleware"
	"github.com/dubai-retail/os/internal/service"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

func parsePageParams(r *http.Request) (page, pageSize int) {
	page, _ = strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ = strconv.Atoi(r.URL.Query().Get("page_size"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 25
	}
	return page, pageSize
}

func parseOptionalUUID(q string) *uuid.UUID {
	if q == "" {
		return nil
	}
	id, err := uuid.Parse(q)
	if err != nil {
		return nil
	}
	return &id
}

func parseOptionalDate(q string) *time.Time {
	if q == "" {
		return nil
	}
	t, err := time.Parse("2006-01-02", q)
	if err != nil {
		return nil
	}
	return &t
}

func listProductsHandler(svc *service.ProductService, log *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := middleware.TenantFromContext(r.Context())
		page, pageSize := parsePageParams(r)

		filters := domain.ProductListFilters{
			TenantID:    tenantID,
			Page:        page,
			PageSize:    pageSize,
			Search:      r.URL.Query().Get("search"),
			Status:      r.URL.Query().Get("status"),
			Category:    r.URL.Query().Get("category"),
			WarehouseID: parseOptionalUUID(r.URL.Query().Get("warehouse_id")),
		}
		if r.URL.Query().Get("inventory") == "product" {
			filters.AggregateVariantInventory = true
		}

		resp, err := svc.ListProducts(r.Context(), filters)
		if err != nil {
			log.Error("ListProducts failed", zap.Error(err))
			jsonError(w, "failed to list products", http.StatusInternalServerError)
			return
		}
		jsonOK(w, resp)
	}
}

// listPublicProductsHandler serves storefront product lists without admin auth.
// It defaults to active products only.
func listPublicProductsHandler(svcs *service.Services, log *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := middleware.TenantFromContext(r.Context())
		page, pageSize := parsePageParams(r)
		status := r.URL.Query().Get("status")
		if status == "" {
			status = "active"
		}

		filterCollectionID, err := resolveCollectionSlugToID(r, svcs.Collection)
		if err != nil {
			jsonError(w, "collection not found", http.StatusNotFound)
			return
		}

		filters := domain.ProductListFilters{
			TenantID:     tenantID,
			Page:         page,
			PageSize:     pageSize,
			Search:       r.URL.Query().Get("search"),
			Status:       status,
			Category:     r.URL.Query().Get("category"),
			WarehouseID:  parseOptionalUUID(r.URL.Query().Get("warehouse_id")),
			CollectionID: filterCollectionID,
		}

		resp, err := svcs.Product.ListProducts(r.Context(), filters)
		if err != nil {
			log.Error("ListPublicProducts failed", zap.Error(err))
			jsonError(w, "failed to list products", http.StatusInternalServerError)
			return
		}
		jsonOK(w, resp)
	}
}

func listOrdersHandler(svc *service.OrderService, log *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := middleware.TenantFromContext(r.Context())
		page, pageSize := parsePageParams(r)

		filters := domain.OrderListFilters{
			TenantID:   tenantID,
			Page:       page,
			PageSize:   pageSize,
			Status:     r.URL.Query().Get("status"),
			Channel:    r.URL.Query().Get("channel"),
			DateFrom:   parseOptionalDate(r.URL.Query().Get("date_from")),
			DateTo:     parseOptionalDate(r.URL.Query().Get("date_to")),
			CustomerID: parseOptionalUUID(r.URL.Query().Get("customer_id")),
		}

		resp, err := svc.ListOrders(r.Context(), filters)
		if err != nil {
			log.Error("ListOrders failed", zap.Error(err))
			jsonError(w, "failed to list orders", http.StatusInternalServerError)
			return
		}
		jsonOK(w, resp)
	}
}

func listCustomersHandler(svc *service.CustomerService, log *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := middleware.TenantFromContext(r.Context())
		page, pageSize := parsePageParams(r)

		filters := domain.CustomerListFilters{
			TenantID: tenantID,
			Page:     page,
			PageSize: pageSize,
			Search:   r.URL.Query().Get("search"),
			Tier:     r.URL.Query().Get("tier"),
			Email:    r.URL.Query().Get("email"),
		}

		resp, err := svc.List(r.Context(), filters)
		if err != nil {
			log.Error("ListCustomers failed", zap.Error(err))
			jsonError(w, "failed to list customers", http.StatusInternalServerError)
			return
		}
		jsonOK(w, resp)
	}
}

func listReturnsHandler(svc *service.RMAService, log *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := middleware.TenantFromContext(r.Context())
		page, pageSize := parsePageParams(r)

		filters := domain.ReturnListFilters{
			TenantID:   tenantID,
			Page:       page,
			PageSize:   pageSize,
			Status:     r.URL.Query().Get("status"),
			OrderID:    parseOptionalUUID(r.URL.Query().Get("order_id")),
			CustomerID: parseOptionalUUID(r.URL.Query().Get("customer_id")),
		}

		resp, err := svc.ListReturns(r.Context(), filters)
		if err != nil {
			log.Error("ListReturns failed", zap.Error(err))
			jsonError(w, "failed to list returns", http.StatusInternalServerError)
			return
		}
		jsonOK(w, resp)
	}
}

func listShipmentsHandler(svc *service.ShippingService, log *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := middleware.TenantFromContext(r.Context())
		page, pageSize := parsePageParams(r)

		filters := domain.ShipmentListFilters{
			TenantID:    tenantID,
			Page:        page,
			PageSize:    pageSize,
			Status:      r.URL.Query().Get("status"),
			Carrier:     r.URL.Query().Get("carrier"),
			WarehouseID: parseOptionalUUID(r.URL.Query().Get("warehouse_id")),
		}

		resp, err := svc.ListShipments(r.Context(), filters)
		if err != nil {
			log.Error("ListShipments failed", zap.Error(err))
			jsonError(w, "failed to list shipments", http.StatusInternalServerError)
			return
		}
		jsonOK(w, resp)
	}
}

func listActivityLogHandler(svc *service.ActivityLogService, log *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := middleware.TenantFromContext(r.Context())
		page, pageSize := parsePageParams(r)

		filters := domain.ActivityLogFilters{
			TenantID: tenantID,
			Page:     page,
			PageSize: pageSize,
			Search:   r.URL.Query().Get("search"),
		}

		resp, err := svc.List(r.Context(), filters)
		if err != nil {
			log.Error("ListActivityLog failed", zap.Error(err))
			jsonError(w, "failed to list activity log", http.StatusInternalServerError)
			return
		}
		jsonOK(w, resp)
	}
}
