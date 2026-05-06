package service

import (
	"github.com/dubai-retail/os/internal/domain"
)

const (
	defaultPage     = 1
	defaultPageSize = 25
	maxPageSize     = 100
)

func normalizePage(page int) int {
	if page < 1 {
		return defaultPage
	}
	return page
}

func normalizePageSize(pageSize int) int {
	if pageSize < 1 {
		return defaultPageSize
	}
	if pageSize > maxPageSize {
		return maxPageSize
	}
	return pageSize
}

func pageOffset(page, pageSize int) int {
	return (normalizePage(page) - 1) * normalizePageSize(pageSize)
}

// Re-export domain types for service layer convenience.
type (
	PageResponse[T any]  = domain.PageResponse[T]
	ProductListFilters   = domain.ProductListFilters
	ProductListItem      = domain.ProductListItem
	OrderListFilters     = domain.OrderListFilters
	OrderListItem        = domain.OrderListItem
	CustomerListFilters  = domain.CustomerListFilters
	CustomerListItem     = domain.CustomerListItem
	ReturnListFilters    = domain.ReturnListFilters
	ReturnListItem       = domain.ReturnListItem
	ShipmentListFilters  = domain.ShipmentListFilters
	ShipmentListItem     = domain.ShipmentListItem
	ActivityLogFilters   = domain.ActivityLogFilters
	ActivityLogItem      = domain.ActivityLogItem
)
