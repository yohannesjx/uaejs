package service_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dubai-retail/os/internal/domain"
	"github.com/dubai-retail/os/internal/metrics"
	"github.com/dubai-retail/os/internal/service"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
)

// ── Fake invoice repo for sandbox tests ──────────────────────────────────────

type fakeASPInvoiceRepo struct {
	invoices       map[uuid.UUID]*domain.OrderInvoice
	sandboxUpdates []sandboxUpdate
}

type sandboxUpdate struct {
	InvoiceID uuid.UUID
	Status    domain.SandboxStatus
	RespID    string
	Errors    []string
}

func (r *fakeASPInvoiceRepo) GetOrderInvoice(ctx context.Context, orderID uuid.UUID) (*domain.OrderInvoice, error) {
	for _, inv := range r.invoices {
		if inv.OrderID == orderID {
			return inv, nil
		}
	}
	return nil, fmt.Errorf("invoice not found for order %s", orderID)
}

func (r *fakeASPInvoiceRepo) UpdateSandboxStatus(ctx context.Context, invoiceID uuid.UUID, status domain.SandboxStatus, aspRespID string, errs []string) error {
	r.sandboxUpdates = append(r.sandboxUpdates, sandboxUpdate{
		InvoiceID: invoiceID,
		Status:    status,
		RespID:    aspRespID,
		Errors:    errs,
	})
	return nil
}

// ── Stub ASP client ────────────────────────────────────────────────────────

type stubASPClient struct {
	resp *service.ASPResponse
	err  error
}

func (c *stubASPClient) Submit(_ context.Context, _ []byte) (*service.ASPResponse, error) {
	return c.resp, c.err
}

// ── Helpers ───────────────────────────────────────────────────────────────

func validUBLXML(invoiceID, orderID uuid.UUID) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2"
         xmlns:cbc="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2"
         xmlns:cac="urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2">
  <cbc:UBLVersionID>2.1</cbc:UBLVersionID>
  <cbc:ID>INV-2026-000001</cbc:ID>
  <cbc:UUID>` + orderID.String() + `</cbc:UUID>
  <cbc:IssueDate>2026-03-07</cbc:IssueDate>
  <cbc:InvoiceTypeCode>388</cbc:InvoiceTypeCode>
  <cbc:DocumentCurrencyCode>AED</cbc:DocumentCurrencyCode>
  <cbc:TaxCurrencyCode>AED</cbc:TaxCurrencyCode>
  <cac:AccountingSupplierParty><cac:Party><cbc:EndpointID>1234567890</cbc:EndpointID></cac:Party></cac:AccountingSupplierParty>
  <cac:AccountingCustomerParty><cac:Party><cbc:EndpointID>buyer</cbc:EndpointID></cac:Party></cac:AccountingCustomerParty>
  <cac:TaxTotal><cbc:TaxAmount currencyID="AED">5.00</cbc:TaxAmount></cac:TaxTotal>
  <cac:LegalMonetaryTotal><cbc:TaxExclusiveAmount currencyID="AED">100.00</cbc:TaxExclusiveAmount></cac:LegalMonetaryTotal>
  <cac:InvoiceLine><cbc:ID>1</cbc:ID></cac:InvoiceLine>
</Invoice>`
}

func newTestMetrics() *metrics.Metrics {
	return metrics.New(prometheus.NewRegistry())
}

func newFakeASPRepo(orderID uuid.UUID, xml string) *fakeASPInvoiceRepo {
	invID := uuid.New()
	xmlCopy := xml
	return &fakeASPInvoiceRepo{
		invoices: map[uuid.UUID]*domain.OrderInvoice{
			invID: {
				ID:          invID,
				OrderID:     orderID,
				InvoiceType: domain.InvoiceDocTypeEInvoice,
				XMLContent:  &xmlCopy,
				IssuedAt:    time.Now(),
			},
		},
	}
}

// =============================================================================
// Tests
// =============================================================================

// TestASPSandbox_AcceptedResponse verifies that a valid XML document
// submitted to an ASP that returns ACCEPTED is persisted and returned correctly.
func TestASPSandbox_AcceptedResponse(t *testing.T) {
	orderID := uuid.New()
	xmlContent := validUBLXML(uuid.New(), orderID)
	repo := newFakeASPRepo(orderID, xmlContent)

	aspClient := &stubASPClient{
		resp: &service.ASPResponse{
			ResponseID: "ASP-RESP-001",
			Status:     "ACCEPTED",
		},
	}

	svc := service.NewASPSandboxService(
		service.ASPSandboxConfig{},
		aspClient,
		repo,
		newTestMetrics(),
		zap.NewNop(),
	)

	result, err := svc.Submit(context.Background(), orderID)
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}

	if result.Status != domain.SandboxStatusAccepted {
		t.Errorf("expected ACCEPTED, got %s", result.Status)
	}
	if result.ASPResponseID != "ASP-RESP-001" {
		t.Errorf("expected ASP-RESP-001, got %s", result.ASPResponseID)
	}
	if len(repo.sandboxUpdates) != 1 {
		t.Fatalf("expected 1 DB update, got %d", len(repo.sandboxUpdates))
	}
	if repo.sandboxUpdates[0].Status != domain.SandboxStatusAccepted {
		t.Errorf("DB status: want accepted, got %s", repo.sandboxUpdates[0].Status)
	}
}

// TestASPSandbox_RejectedByASP verifies that ASP-returned errors are
// stored and surfaced in the result.
func TestASPSandbox_RejectedByASP(t *testing.T) {
	orderID := uuid.New()
	xmlContent := validUBLXML(uuid.New(), orderID)
	repo := newFakeASPRepo(orderID, xmlContent)

	aspClient := &stubASPClient{
		resp: &service.ASPResponse{
			ResponseID: "ASP-RESP-002",
			Status:     "REJECTED",
			Errors:     []string{"BT-5: DocumentCurrencyCode must be a valid ISO 4217 code"},
		},
	}

	svc := service.NewASPSandboxService(
		service.ASPSandboxConfig{},
		aspClient,
		repo,
		newTestMetrics(),
		zap.NewNop(),
	)

	result, err := svc.Submit(context.Background(), orderID)
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}

	if result.Status != domain.SandboxStatusRejected {
		t.Errorf("expected REJECTED, got %s", result.Status)
	}
	if len(result.Errors) == 0 {
		t.Error("expected validation errors in result")
	}
}

// TestASPSandbox_LocalValidation_MissingElement verifies that a malformed
// XML (missing mandatory element) fails local validation before the ASP call.
func TestASPSandbox_LocalValidation_MissingElement(t *testing.T) {
	orderID := uuid.New()
	// XML missing TaxTotal and LegalMonetaryTotal
	malformedXML := `<?xml version="1.0"?>
<Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2"
         xmlns:cbc="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2">
  <cbc:ID>INV-001</cbc:ID>
  <cbc:UUID>` + orderID.String() + `</cbc:UUID>
  <cbc:IssueDate>2026-03-07</cbc:IssueDate>
  <cbc:InvoiceTypeCode>388</cbc:InvoiceTypeCode>
  <cbc:DocumentCurrencyCode>AED</cbc:DocumentCurrencyCode>
  <cbc:TaxCurrencyCode>AED</cbc:TaxCurrencyCode>
</Invoice>`

	xmlCopy := malformedXML
	invID := uuid.New()
	repo := &fakeASPInvoiceRepo{
		invoices: map[uuid.UUID]*domain.OrderInvoice{
			invID: {ID: invID, OrderID: orderID, XMLContent: &xmlCopy},
		},
	}

	// ASP client should NOT be called; use a stub that panics if called
	aspClient := &stubASPClient{err: fmt.Errorf("ASP should not have been called")}

	svc := service.NewASPSandboxService(
		service.ASPSandboxConfig{},
		aspClient,
		repo,
		newTestMetrics(),
		zap.NewNop(),
	)

	result, err := svc.Submit(context.Background(), orderID)
	// Submit should NOT return an error; instead it returns a rejected result
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != domain.SandboxStatusRejected {
		t.Errorf("expected REJECTED from local validation, got %s", result.Status)
	}

	// Verify missing elements are reported
	combined := strings.Join(result.Errors, " ")
	if !strings.Contains(combined, "AccountingSupplierParty") {
		t.Errorf("expected missing AccountingSupplierParty in errors: %v", result.Errors)
	}

	// ASP must not have been called (the stub error was not triggered)
	if len(repo.sandboxUpdates) != 1 {
		t.Errorf("expected 1 DB update, got %d", len(repo.sandboxUpdates))
	}
	if repo.sandboxUpdates[0].Status != domain.SandboxStatusRejected {
		t.Errorf("DB status should be rejected, got %s", repo.sandboxUpdates[0].Status)
	}
}

// TestASPSandbox_MissingXML verifies that Submit returns an error when the
// order invoice has no XML content (receipt-only order).
func TestASPSandbox_MissingXML(t *testing.T) {
	orderID := uuid.New()
	invID := uuid.New()
	repo := &fakeASPInvoiceRepo{
		invoices: map[uuid.UUID]*domain.OrderInvoice{
			invID: {ID: invID, OrderID: orderID, XMLContent: nil}, // no XML
		},
	}

	svc := service.NewASPSandboxService(
		service.ASPSandboxConfig{},
		nil,
		repo,
		newTestMetrics(),
		zap.NewNop(),
	)

	_, err := svc.Submit(context.Background(), orderID)
	if err == nil {
		t.Error("expected error for invoice without XML, got nil")
	}
}

// TestASPSandbox_NetworkError verifies graceful handling of ASP network failures.
func TestASPSandbox_NetworkError(t *testing.T) {
	orderID := uuid.New()
	xmlContent := validUBLXML(uuid.New(), orderID)
	repo := newFakeASPRepo(orderID, xmlContent)

	aspClient := &stubASPClient{err: fmt.Errorf("connection refused")}

	svc := service.NewASPSandboxService(
		service.ASPSandboxConfig{},
		aspClient,
		repo,
		newTestMetrics(),
		zap.NewNop(),
	)

	result, err := svc.Submit(context.Background(), orderID)
	if err != nil {
		t.Fatalf("unexpected error (should be captured in result): %v", err)
	}
	if result.Status != domain.SandboxStatusError {
		t.Errorf("expected error status, got %s", result.Status)
	}
}

// TestASPSandbox_HTTPServer is an integration-style test that spins up a real
// HTTP test server to validate the full httpASPClient request/response cycle.
func TestASPSandbox_HTTPServer(t *testing.T) {
	orderID := uuid.New()
	xmlContent := validUBLXML(uuid.New(), orderID)
	repo := newFakeASPRepo(orderID, xmlContent)

	// Spin up a local test server that returns ACCEPTED
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") == "" {
			http.Error(w, "missing Content-Type", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(service.ASPResponse{
			ResponseID: "TEST-ASP-LIVE",
			Status:     "ACCEPTED",
		})
	}))
	defer ts.Close()

	svc := service.NewASPSandboxService(
		service.ASPSandboxConfig{
			EndpointURL: ts.URL,
			Timeout:     5 * time.Second,
		},
		nil, // use default httpASPClient
		repo,
		newTestMetrics(),
		zap.NewNop(),
	)

	result, err := svc.Submit(context.Background(), orderID)
	if err != nil {
		t.Fatalf("Submit to test server failed: %v", err)
	}
	if result.Status != domain.SandboxStatusAccepted {
		t.Errorf("expected ACCEPTED from test server, got %s", result.Status)
	}
	if result.ASPResponseID != "TEST-ASP-LIVE" {
		t.Errorf("expected TEST-ASP-LIVE response ID, got %s", result.ASPResponseID)
	}
}
