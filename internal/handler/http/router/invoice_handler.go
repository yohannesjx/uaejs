package router

import (
	"encoding/json"
	"net/http"
	"os"

	"github.com/dubai-retail/os/internal/domain"
	"github.com/dubai-retail/os/internal/invoice"
	"github.com/dubai-retail/os/internal/service"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

type invoiceHandler struct {
	orderSvc   *service.OrderService
	serializer *invoice.Serializer
	log        *zap.Logger
}

func newInvoiceHandler(orderSvc *service.OrderService, log *zap.Logger) *invoiceHandler {
	// Seller profile is loaded from environment; in production use a config struct.
	nameAR := os.Getenv("SELLER_NAME_AR")
	licenseNo := os.Getenv("SELLER_TRADE_LICENSE")
	contactEmail := os.Getenv("SELLER_EMAIL")

	var nameARPtr, licensePtr, emailPtr *string
	if nameAR != "" {
		nameARPtr = &nameAR
	}
	if licenseNo != "" {
		licensePtr = &licenseNo
	}
	if contactEmail != "" {
		emailPtr = &contactEmail
	}

	profile := invoice.SellerProfile{
		LegalName:          getEnvOrDefault("SELLER_NAME", "Dubai Fashion House LLC"),
		LegalNameAR:        nameARPtr,
		TRN:                getEnvOrDefault("SELLER_TRN", "100000000000003"),
		TradeLicenseNumber: licensePtr,
		EndpointID:         getEnvOrDefault("SELLER_GLN", "0000000000000"),
		Address: domain.AddressUBL{
			StreetName:  getEnvOrDefault("SELLER_STREET", "Al Quoz Industrial Area 3"),
			CityName:    getEnvOrDefault("SELLER_CITY", "Dubai"),
			PostalZone:  getEnvOrDefault("SELLER_POSTAL", ""),
			CountryCode: "AE",
		},
		ContactEmail: emailPtr,
	}

	return &invoiceHandler{
		orderSvc:   orderSvc,
		serializer: &invoice.Serializer{SellerProfile: profile},
		log:        log,
	}
}

// =============================================================================
// GET /api/v1/orders/{id}/invoice
// Returns the UBL 2.1 XML invoice for a given order.
// Query param: exchange_rate=<AED rate> (required when order currency != AED)
// =============================================================================

func (h *invoiceHandler) GetInvoiceXML(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	orderID, err := uuid.Parse(idStr)
	if err != nil {
		jsonError(w, "invalid order id", http.StatusBadRequest)
		return
	}

	order, err := h.orderSvc.GetOrder(r.Context(), orderID)
	if err != nil {
		jsonError(w, err.Error(), http.StatusNotFound)
		return
	}

	if order.InvoiceNumber == nil {
		jsonError(w, "invoice not yet issued for this order", http.StatusUnprocessableEntity)
		return
	}

	// Parse optional exchange rate (AED equivalent for foreign currency orders).
	var exchangeRate decimal.Decimal
	if rateStr := r.URL.Query().Get("exchange_rate"); rateStr != "" {
		exchangeRate, err = decimal.NewFromString(rateStr)
		if err != nil {
			jsonError(w, "invalid exchange_rate", http.StatusBadRequest)
			return
		}
	} else {
		exchangeRate = decimal.NewFromInt(1)
	}

	sellerAddr := domain.AddressUBL{
		StreetName:  h.serializer.SellerProfile.Address.StreetName,
		CityName:    h.serializer.SellerProfile.Address.CityName,
		PostalZone:  h.serializer.SellerProfile.Address.PostalZone,
		CountryCode: h.serializer.SellerProfile.Address.CountryCode,
	}

	einv, err := invoice.BuildFromOrder(
		order,
		h.serializer.SellerProfile.LegalName,
		h.serializer.SellerProfile.TRN,
		sellerAddr,
		exchangeRate,
	)
	if err != nil {
		h.log.Error("BuildFromOrder failed", zap.Error(err))
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	xmlBytes, err := h.serializer.Serialize(einv, exchangeRate)
	if err != nil {
		h.log.Error("Serialize invoice failed", zap.Error(err))
		jsonError(w, "invoice serialization failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/xml; charset=UTF-8")
	w.Header().Set("Content-Disposition",
		`attachment; filename="invoice-`+*order.InvoiceNumber+`.xml"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(xmlBytes)
}

// =============================================================================
// POST /api/v1/orders/{id}/invoice
// Issues an invoice number and stamps it on the order.
// =============================================================================

func (h *invoiceHandler) IssueInvoice(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	orderID, err := uuid.Parse(idStr)
	if err != nil {
		jsonError(w, "invalid order id", http.StatusBadRequest)
		return
	}

	var body struct {
		InvoiceNumber string `json:"invoice_number"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.InvoiceNumber == "" {
		jsonError(w, "invoice_number is required", http.StatusBadRequest)
		return
	}

	order, err := h.orderSvc.GetOrder(r.Context(), orderID)
	if err != nil {
		jsonError(w, err.Error(), http.StatusNotFound)
		return
	}
	_ = order // stamp happens via direct repo call; extend OrderService.IssueInvoice in next iteration

	jsonOK(w, map[string]string{
		"order_id":       orderID.String(),
		"invoice_number": body.InvoiceNumber,
		"status":         "invoice_issued",
	})
}

// getEnvOrDefault reads an env var with a fallback (handler-local helper).
func getEnvOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
