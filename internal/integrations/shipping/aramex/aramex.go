// Package aramex implements the ShippingConnector for Aramex UAE.
// The real Aramex API uses SOAP/XML; this implementation provides the
// interface contract and a working HTTP stub that can be upgraded to the
// full WSDL client when needed.
package aramex

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/dubai-retail/os/internal/domain"
	"github.com/dubai-retail/os/internal/integrations/shipping"
)

func init() {
	shipping.Register(&Connector{})
}

const (
	aramexSandboxURL    = "https://ws.dev.aramex.net/ShippingAPI.V2/Shipping/Service_1_0.svc/json"
	aramexProductionURL = "https://ws.aramex.net/ShippingAPI.V2/Shipping/Service_1_0.svc/json"
)

// Connector is the Aramex implementation of ShippingConnector.
type Connector struct {
	client *http.Client
}

func (c *Connector) ProviderType() string { return "aramex" }

func (c *Connector) httpClient() *http.Client {
	if c.client != nil {
		return c.client
	}
	return &http.Client{Timeout: 30 * time.Second}
}

// CreateShipment books a shipment via the Aramex Rate+Ship API.
// Currently sends to the sandbox endpoint; switch baseURL to production
// via account.Settings["environment"] = "production".
func (c *Connector) CreateShipment(
	ctx context.Context,
	account *domain.ShippingAccount,
	input shipping.CreateShipmentInput,
) (*shipping.CreateShipmentResult, error) {
	baseURL := aramexSandboxURL
	if env, ok := account.Settings["environment"].(string); ok && env == "production" {
		baseURL = aramexProductionURL
	}

	// Build a simplified Aramex CreateShipments request body.
	payload := map[string]any{
		"ClientInfo": map[string]any{
			"AccountCountryCode": "AE",
			"AccountEntity":      account.Settings["account_entity"],
			"AccountNumber":      account.Settings["account_number"],
			"AccountPin":         account.APISecret,
			"UserName":           account.APIKey,
			"Password":           account.APISecret,
			"Version":            "v1.0",
		},
		"Shipments": []map[string]any{
			{
				"Reference1":      input.OrderID.String(),
				"Reference2":      input.ShipmentID.String(),
				"Consignee": map[string]any{
					"Reference1":   input.RecipientName,
					"ContactName":  input.RecipientName,
					"PhoneNumber1": input.RecipientPhone,
					"CellPhone":    input.RecipientPhone,
					"Address": map[string]any{
						"Line1":       input.Address.Line1,
						"City":        input.Address.City,
						"CountryCode": input.Address.Country,
					},
				},
				"Details": map[string]any{
					"Dimensions":         nil,
					"ActualWeight":       map[string]any{"Unit": "G", "Value": input.WeightG},
					"ProductType":        "DDU",
					"ProductGroup":       "EXP",
					"PayType":            "P",
					"NumberOfPieces":     1,
					"DescriptionOfGoods": input.Description,
					"CashOnDeliveryAmount": map[string]any{
						"CurrencyCode": "AED",
						"Value":        input.CODAmount,
					},
				},
			},
		},
		"LabelInfo": map[string]any{
			"ReportID":   9201,
			"ReportType": "URL",
		},
	}

	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/CreateShipments", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("aramex.CreateShipment: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("aramex.CreateShipment: http: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		HasErrors bool `json:"HasErrors"`
		Shipments []struct {
			ID             string `json:"ID"`
			AirwayBillNumber string `json:"AirwayBillNumber"`
			ShipmentLabel  struct {
				LabelURL string `json:"LabelURL"`
			} `json:"ShipmentLabel"`
		} `json:"Shipments"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("aramex.CreateShipment: decode: %w", err)
	}
	if result.HasErrors || len(result.Shipments) == 0 {
		return nil, fmt.Errorf("aramex.CreateShipment: carrier returned error or empty response")
	}

	s := result.Shipments[0]
	return &shipping.CreateShipmentResult{
		TrackingNumber: s.AirwayBillNumber,
		CarrierRef:     s.ID,
		LabelURL:       s.ShipmentLabel.LabelURL,
	}, nil
}

// GetTracking fetches tracking events from the Aramex TrackShipments endpoint.
func (c *Connector) GetTracking(
	ctx context.Context,
	account *domain.ShippingAccount,
	trackingNumber string,
) ([]shipping.TrackingEvent, error) {
	baseURL := aramexSandboxURL
	if env, ok := account.Settings["environment"].(string); ok && env == "production" {
		baseURL = aramexProductionURL
	}

	payload := map[string]any{
		"ClientInfo": map[string]any{
			"UserName": account.APIKey,
			"Password": account.APISecret,
			"Version":  "v1.0",
		},
		"Shipments": []map[string]any{
			{"WaybillNumber": trackingNumber},
		},
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/TrackShipments", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("aramex.GetTracking: build: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("aramex.GetTracking: http: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		TrackingResults []struct {
			WaybillNumber  string `json:"WaybillNumber"`
			TrackingResult struct {
				TrackingEvents []struct {
					EventCode    string `json:"EventCode"`
					EventDescription string `json:"EventDescription"`
					Location      string `json:"Location"`
					Date          string `json:"Date"`
					Time          string `json:"Time"`
				} `json:"TrackingEvents"`
			} `json:"TrackingResult"`
		} `json:"TrackingResults"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("aramex.GetTracking: decode: %w", err)
	}

	var events []shipping.TrackingEvent
	for _, tr := range result.TrackingResults {
		for _, e := range tr.TrackingResult.TrackingEvents {
			t, _ := time.Parse("2006-01-02 15:04:05", e.Date+" "+e.Time)
			events = append(events, shipping.TrackingEvent{
				Status:      e.EventCode,
				Location:    e.Location,
				Description: e.EventDescription,
				EventTime:   t,
			})
		}
	}
	return events, nil
}

// CancelShipment requests cancellation from Aramex.
func (c *Connector) CancelShipment(
	ctx context.Context,
	account *domain.ShippingAccount,
	trackingNumber string,
) error {
	baseURL := aramexSandboxURL
	if env, ok := account.Settings["environment"].(string); ok && env == "production" {
		baseURL = aramexProductionURL
	}

	payload := map[string]any{
		"ClientInfo":     map[string]any{"UserName": account.APIKey, "Password": account.APISecret, "Version": "v1.0"},
		"WaybillNumbers": []string{trackingNumber},
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/DeleteShipments", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("aramex.CancelShipment: build: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return fmt.Errorf("aramex.CancelShipment: http: %w", err)
	}
	defer resp.Body.Close()
	return nil
}
