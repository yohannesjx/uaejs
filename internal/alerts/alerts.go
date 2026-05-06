// Package alerts provides the internal alerting layer: it bridges Prometheus
// metric events with webhook notifications (Slack) for operational teams.
//
// Design: alerts.Manager is a thin wrapper that:
//   - Exposes helper methods that services call directly (e.g., FireLowStock).
//   - Emits a structured audit log entry via zap on every alert.
//   - Optionally delivers the same payload to a Slack Incoming Webhook.
//
// Prometheus / Alertmanager handles the primary alerting path; this package
// gives services the ability to fire *immediate* alerts for critical events
// (e.g. QC mismatch, pending-invoice accumulation) without waiting for a
// Prometheus scrape interval.
package alerts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"go.uber.org/zap"
)

// Severity levels map to Alertmanager / Slack colour coding.
type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityCritical Severity = "critical"
)

// Event is a structured alert payload.
type Event struct {
	AlertName   string
	Severity    Severity
	Team        string
	Summary     string
	Description string
	Labels      map[string]string // extra k/v attached to the alert
	FiredAt     time.Time
}

// Config holds Slack webhook URLs per channel.
// Leave empty to disable Slack delivery (alerts are still logged).
type Config struct {
	SlackWebhookOperations  string
	SlackWebhookCritical    string
	SlackWebhookCompliance  string
	SlackWebhookEngineering string
	HTTPTimeout             time.Duration
}

// Manager fires alerts through logging and optional Slack webhooks.
type Manager struct {
	cfg    Config
	http   *http.Client
	log    *zap.Logger
}

// New creates an alerts.Manager. Call with an empty Config to disable webhooks.
func New(cfg Config, log *zap.Logger) *Manager {
	timeout := cfg.HTTPTimeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	return &Manager{
		cfg:  cfg,
		http: &http.Client{Timeout: timeout},
		log:  log,
	}
}

// =============================================================================
// Pre-built alert methods – one per alerting rule in dubai_retail_alerts.yml
// =============================================================================

// FireLowStock fires when a variant's quantity_available drops to or below
// the low-stock threshold. Called from InventoryService after each FIFO deduction.
func (m *Manager) FireLowStock(ctx context.Context, variantID, sku string, quantityAvailable int) {
	m.fire(ctx, Event{
		AlertName:   "LowStockVariant",
		Severity:    SeverityWarning,
		Team:        "operations",
		Summary:     fmt.Sprintf("Low stock: %s", sku),
		Description: fmt.Sprintf("Variant %s (SKU %s) has %d units remaining.", variantID, sku, quantityAvailable),
		Labels:      map[string]string{"variant_id": variantID, "sku": sku},
	})
}

// FireStockout fires when a variant reaches exactly 0 units.
func (m *Manager) FireStockout(ctx context.Context, variantID, sku string) {
	m.fire(ctx, Event{
		AlertName:   "StockoutVariant",
		Severity:    SeverityCritical,
		Team:        "operations",
		Summary:     fmt.Sprintf("Stockout: %s", sku),
		Description: fmt.Sprintf("Variant %s (SKU %s) has reached zero stock. Sales will be rejected.", variantID, sku),
		Labels:      map[string]string{"variant_id": variantID, "sku": sku},
	})
}

// FireQCMismatch fires immediately when a QC photo comparison fails.
func (m *Manager) FireQCMismatch(ctx context.Context, returnItemID, variantID string) {
	m.fire(ctx, Event{
		AlertName:   "QCPhotoMismatch",
		Severity:    SeverityWarning,
		Team:        "operations",
		Summary:     "QC photo hash mismatch on return item",
		Description: fmt.Sprintf("Return item %s (variant %s) hash did not match outbound QC photo. Manual review required.", returnItemID, variantID),
		Labels:      map[string]string{"return_item_id": returnItemID, "variant_id": variantID},
	})
}

// FirePendingInvoice fires when an order invoice fails to generate.
func (m *Manager) FirePendingInvoice(ctx context.Context, orderID string, reason string) {
	m.fire(ctx, Event{
		AlertName:   "PendingInvoice",
		Severity:    SeverityCritical,
		Team:        "compliance",
		Summary:     fmt.Sprintf("Order %s has no invoice number", orderID),
		Description: fmt.Sprintf("ComplianceService failed for order %s: %s", orderID, reason),
		Labels:      map[string]string{"order_id": orderID},
	})
}

// FireASPRejection fires when the ASP sandbox rejects an invoice.
func (m *Manager) FireASPRejection(ctx context.Context, orderID, invoiceID string, errors []string) {
	m.fire(ctx, Event{
		AlertName:   "ASPSandboxRejection",
		Severity:    SeverityWarning,
		Team:        "compliance",
		Summary:     fmt.Sprintf("ASP rejected invoice for order %s", orderID),
		Description: fmt.Sprintf("Invoice %s was rejected by ASP sandbox. Errors: %v", invoiceID, errors),
		Labels:      map[string]string{"order_id": orderID, "invoice_id": invoiceID},
	})
}

// FireOrderFailure fires when a ProcessOrder call fails with a non-stock error.
func (m *Manager) FireOrderFailure(ctx context.Context, channelType, reason string) {
	m.fire(ctx, Event{
		AlertName:   "OrderFailure",
		Severity:    SeverityWarning,
		Team:        "engineering",
		Summary:     fmt.Sprintf("Order failure on %s channel: %s", channelType, reason),
		Description: fmt.Sprintf("A %s order failed with reason '%s'. Check application logs.", channelType, reason),
		Labels:      map[string]string{"channel_type": channelType, "reason": reason},
	})
}

// FireFraudSignal fires when the analytics service detects a fraud pattern.
func (m *Manager) FireFraudSignal(ctx context.Context, customerEmail, reason string, returnCount int) {
	m.fire(ctx, Event{
		AlertName:   "FraudSignalDetected",
		Severity:    SeverityCritical,
		Team:        "operations",
		Summary:     fmt.Sprintf("Potential fraud: %s", customerEmail),
		Description: fmt.Sprintf("Customer %s has %d returns with reason: %s", customerEmail, returnCount, reason),
		Labels:      map[string]string{"customer_email": customerEmail},
	})
}

// =============================================================================
// Internal dispatch
// =============================================================================

func (m *Manager) fire(ctx context.Context, evt Event) {
	evt.FiredAt = time.Now().UTC()

	// Always emit a structured log first (guaranteed delivery)
	m.log.Warn("alert.fired",
		zap.String("alert_name", evt.AlertName),
		zap.String("severity", string(evt.Severity)),
		zap.String("team", evt.Team),
		zap.String("summary", evt.Summary),
		zap.String("description", evt.Description),
		zap.Any("labels", evt.Labels),
		zap.Time("fired_at", evt.FiredAt),
	)

	// Deliver to Slack (non-blocking, best-effort)
	webhook := m.webhookFor(evt.Team, evt.Severity)
	if webhook == "" {
		return
	}
	go func() {
		if err := m.sendSlack(ctx, webhook, evt); err != nil {
			m.log.Error("alert.slack_delivery_failed",
				zap.String("alert", evt.AlertName),
				zap.Error(err),
			)
		}
	}()
}

// webhookFor selects the correct Slack webhook URL based on team and severity.
func (m *Manager) webhookFor(team string, severity Severity) string {
	if severity == SeverityCritical && m.cfg.SlackWebhookCritical != "" {
		return m.cfg.SlackWebhookCritical
	}
	switch team {
	case "compliance":
		return m.cfg.SlackWebhookCompliance
	case "engineering":
		return m.cfg.SlackWebhookEngineering
	default:
		return m.cfg.SlackWebhookOperations
	}
}

// slackPayload is the Slack Incoming Webhook JSON schema.
type slackPayload struct {
	Text        string            `json:"text"`
	Attachments []slackAttachment `json:"attachments"`
}

type slackAttachment struct {
	Color  string `json:"color"`
	Title  string `json:"title"`
	Text   string `json:"text"`
	Footer string `json:"footer"`
	Ts     int64  `json:"ts"`
}

func (m *Manager) sendSlack(ctx context.Context, webhookURL string, evt Event) error {
	color := "warning"
	switch evt.Severity {
	case SeverityCritical:
		color = "danger"
	case SeverityInfo:
		color = "good"
	}

	payload := slackPayload{
		Attachments: []slackAttachment{
			{
				Color:  color,
				Title:  fmt.Sprintf("[%s] %s", string(evt.Severity), evt.Summary),
				Text:   evt.Description,
				Footer: fmt.Sprintf("dubai-retail-os | team:%s | alert:%s", evt.Team, evt.AlertName),
				Ts:     evt.FiredAt.Unix(),
			},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal slack payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := m.http.Do(req)
	if err != nil {
		return fmt.Errorf("slack POST: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("slack returned %d", resp.StatusCode)
	}
	return nil
}
