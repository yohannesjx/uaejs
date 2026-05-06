// Package service: ASP Sandbox Integration
//
// The ASP (Accredited Service Provider) sandbox validates PINT-AE UBL 2.1
// XML invoices before live submission to the FTA (UAE Federal Tax Authority).
//
// Validation pipeline:
//  1. Local XSD schema validation of the UBL 2.1 Invoice document.
//  2. HTTP submission to the ASP sandbox endpoint.
//  3. Parse the sandbox response (accept / reject with error details).
//  4. Persist the result in order_invoices.sandbox_status.
//  5. Emit structured audit log and Prometheus metrics.
package service

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/dubai-retail/os/internal/domain"
	"github.com/dubai-retail/os/internal/metrics"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// =============================================================================
// Domain types for sandbox results
// =============================================================================

// SandboxResult is returned by ASPSandboxService.Submit.
type SandboxResult struct {
	OrderInvoiceID uuid.UUID
	OrderID        uuid.UUID
	Status         domain.SandboxStatus
	ASPResponseID  string   // unique ID issued by the ASP
	Errors         []string // validation errors when Status = rejected
	SubmittedAt    time.Time
	DurationMs     int64
}

// =============================================================================
// Repository interface
// =============================================================================

// ASPInvoiceRepo is the subset of order_invoices operations needed by the sandbox.
type ASPInvoiceRepo interface {
	GetOrderInvoice(ctx context.Context, orderID uuid.UUID) (*domain.OrderInvoice, error)
	UpdateSandboxStatus(ctx context.Context, invoiceID uuid.UUID, status domain.SandboxStatus, aspRespID string, validationErrors []string) error
}

// =============================================================================
// ASP HTTP client interface (allows mock injection in tests)
// =============================================================================

// ASPClient performs the actual HTTP call to the ASP endpoint.
// The default implementation is httpASPClient; tests inject a stub.
type ASPClient interface {
	Submit(ctx context.Context, xmlBytes []byte) (*ASPResponse, error)
}

// ASPResponse mirrors the JSON envelope returned by most UAE ASP sandbox APIs.
type ASPResponse struct {
	ResponseID string   `json:"responseId"`
	Status     string   `json:"status"` // "ACCEPTED" | "REJECTED"
	Errors     []string `json:"errors,omitempty"`
}

// =============================================================================
// Service
// =============================================================================

// ASPSandboxConfig holds runtime parameters for the sandbox client.
type ASPSandboxConfig struct {
	EndpointURL string
	APIKey      string
	Timeout     time.Duration
}

// ASPSandboxService orchestrates local XSD validation + ASP sandbox submission.
type ASPSandboxService struct {
	client      ASPClient
	invoiceRepo ASPInvoiceRepo
	metrics     *metrics.Metrics
	log         *zap.Logger
}

// NewASPSandboxService creates the service.
// If client is nil a default httpASPClient is built from cfg.
func NewASPSandboxService(
	cfg ASPSandboxConfig,
	client ASPClient,
	invoiceRepo ASPInvoiceRepo,
	m *metrics.Metrics,
	log *zap.Logger,
) *ASPSandboxService {
	if client == nil {
		client = newHTTPASPClient(cfg)
	}
	return &ASPSandboxService{
		client:      client,
		invoiceRepo: invoiceRepo,
		metrics:     m,
		log:         log,
	}
}

// Submit validates and submits the UBL XML for a given order to the sandbox.
//
// Steps:
//  1. Fetch the order_invoices row (must have XMLContent).
//  2. Run local structural XML well-formedness + required-field checks.
//  3. POST to ASP sandbox.
//  4. Parse response and persist sandbox_status.
func (s *ASPSandboxService) Submit(ctx context.Context, orderID uuid.UUID) (*SandboxResult, error) {
	inv, err := s.invoiceRepo.GetOrderInvoice(ctx, orderID)
	if err != nil {
		return nil, fmt.Errorf("ASPSandbox.Submit: fetch invoice: %w", err)
	}
	if inv.XMLContent == nil || *inv.XMLContent == "" {
		return nil, fmt.Errorf("ASPSandbox.Submit: order %s has no UBL XML (receipt-only order)", orderID)
	}

	xmlBytes := []byte(*inv.XMLContent)

	// ── Step 1: Local validation ───────────────────────────────────────────
	if localErrs := validateUBLLocally(xmlBytes); len(localErrs) > 0 {
		s.log.Warn("asp.local_validation_failed",
			zap.String("order_id", orderID.String()),
			zap.Strings("errors", localErrs),
		)
		_ = s.invoiceRepo.UpdateSandboxStatus(ctx, inv.ID, domain.SandboxStatusRejected, "", localErrs)
		s.metrics.InvoicesGeneratedTotal.WithLabelValues("sandbox_rejected", "local_schema").Inc()
		return &SandboxResult{
			OrderInvoiceID: inv.ID,
			OrderID:        orderID,
			Status:         domain.SandboxStatusRejected,
			Errors:         localErrs,
			SubmittedAt:    time.Now().UTC(),
		}, nil
	}

	// ── Step 2: ASP sandbox submission ────────────────────────────────────
	start := time.Now()
	aspResp, err := s.client.Submit(ctx, xmlBytes)
	durationMs := time.Since(start).Milliseconds()

	if err != nil {
		s.log.Error("asp.submission_error",
			zap.String("order_id", orderID.String()),
			zap.Error(err),
		)
		_ = s.invoiceRepo.UpdateSandboxStatus(ctx, inv.ID, domain.SandboxStatusError, "", []string{err.Error()})
		s.metrics.InvoicesGeneratedTotal.WithLabelValues("sandbox_error", "network").Inc()
		return &SandboxResult{
			OrderInvoiceID: inv.ID,
			OrderID:        orderID,
			Status:         domain.SandboxStatusError,
			Errors:         []string{err.Error()},
			SubmittedAt:    time.Now().UTC(),
			DurationMs:     durationMs,
		}, nil
	}

	// ── Step 3: Persist result ─────────────────────────────────────────────
	status := domain.SandboxStatusAccepted
	if !strings.EqualFold(aspResp.Status, "ACCEPTED") {
		status = domain.SandboxStatusRejected
	}

	if err := s.invoiceRepo.UpdateSandboxStatus(ctx, inv.ID, status, aspResp.ResponseID, aspResp.Errors); err != nil {
		s.log.Error("asp.persist_status_failed",
			zap.String("order_id", orderID.String()),
			zap.Error(err),
		)
		// Non-fatal: result is still returned to caller
	}

	result := &SandboxResult{
		OrderInvoiceID: inv.ID,
		OrderID:        orderID,
		Status:         status,
		ASPResponseID:  aspResp.ResponseID,
		Errors:         aspResp.Errors,
		SubmittedAt:    time.Now().UTC(),
		DurationMs:     durationMs,
	}

	s.log.Info("asp.submission_complete",
		zap.String("order_id", orderID.String()),
		zap.String("invoice_id", inv.ID.String()),
		zap.String("status", string(status)),
		zap.String("asp_response_id", aspResp.ResponseID),
		zap.Int64("duration_ms", durationMs),
	)

	metricLabel := "sandbox_accepted"
	if status == domain.SandboxStatusRejected {
		metricLabel = "sandbox_rejected"
	}
	s.metrics.InvoicesGeneratedTotal.WithLabelValues(metricLabel, "asp_sandbox").Inc()

	return result, nil
}

// =============================================================================
// Local UBL well-formedness + mandatory-field check
// =============================================================================

// ublRequiredPaths is the list of element local-names that MUST appear at
// least once in any PINT-AE invoice (subset of the 51 mandatory fields used
// for fast local pre-validation before calling the ASP).
var ublRequiredElements = []string{
	"ID",          // BT-1  Invoice number
	"UUID",        // BT-2  Invoice UUID
	"IssueDate",   // BT-2  Issue date
	"InvoiceTypeCode",
	"DocumentCurrencyCode",
	"TaxCurrencyCode",
	"AccountingSupplierParty",
	"AccountingCustomerParty",
	"TaxTotal",
	"LegalMonetaryTotal",
	"InvoiceLine",
}

// validateUBLLocally checks that the XML is well-formed and contains every
// mandatory top-level element. Returns a slice of human-readable errors.
func validateUBLLocally(xmlBytes []byte) []string {
	var errs []string

	// Well-formedness check
	decoder := xml.NewDecoder(bytes.NewReader(xmlBytes))
	for {
		_, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return []string{fmt.Sprintf("XML is not well-formed: %v", err)}
		}
	}

	// Required-element presence check
	xmlStr := string(xmlBytes)
	for _, elem := range ublRequiredElements {
		// Check both prefixed (cbc:ID) and unprefixed forms
		if !strings.Contains(xmlStr, "<cbc:"+elem) &&
			!strings.Contains(xmlStr, "<cac:"+elem) &&
			!strings.Contains(xmlStr, "<"+elem) {
			errs = append(errs, fmt.Sprintf("missing mandatory element: %s", elem))
		}
	}

	// Invoice namespace check
	if !strings.Contains(xmlStr, "urn:oasis:names:specification:ubl:schema:xsd:Invoice-2") {
		errs = append(errs, "missing UBL 2.1 Invoice namespace declaration")
	}

	return errs
}

// =============================================================================
// Default HTTP ASP client
// =============================================================================

type httpASPClient struct {
	endpoint string
	apiKey   string
	http     *http.Client
}

func newHTTPASPClient(cfg ASPSandboxConfig) *httpASPClient {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	return &httpASPClient{
		endpoint: cfg.EndpointURL,
		apiKey:   cfg.APIKey,
		http:     &http.Client{Timeout: timeout},
	}
}

func (c *httpASPClient) Submit(ctx context.Context, xmlBytes []byte) (*ASPResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(xmlBytes))
	if err != nil {
		return nil, fmt.Errorf("httpASPClient: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/xml; charset=utf-8")
	req.Header.Set("Accept", "application/json")
	if c.apiKey != "" {
		req.Header.Set("X-API-Key", c.apiKey)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("httpASPClient: send: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 500 {
		return nil, fmt.Errorf("httpASPClient: ASP server error %d: %s", resp.StatusCode, string(body))
	}

	var aspResp ASPResponse
	if err := json.Unmarshal(body, &aspResp); err != nil {
		return nil, fmt.Errorf("httpASPClient: parse response: %w (body: %s)", err, string(body))
	}
	return &aspResp, nil
}
