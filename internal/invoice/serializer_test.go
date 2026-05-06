package invoice_test

import (
	"strings"
	"testing"
	"time"

	"github.com/dubai-retail/os/internal/domain"
	"github.com/dubai-retail/os/internal/invoice"
	"github.com/shopspring/decimal"
)

func TestSerialize_ProducesValidUBLStructure(t *testing.T) {
	s := &invoice.Serializer{
		SellerProfile: invoice.SellerProfile{
			LegalName:  "Dubai Fashion House LLC",
			TRN:        "100123456789003",
			EndpointID: "0000000000128",
			Address: domain.AddressUBL{
				StreetName:  "Al Quoz Industrial Area 3",
				CityName:    "Dubai",
				CountryCode: "AE",
			},
		},
	}

	buyerName := "Al Majd Trading LLC"
	buyerTRN := "200987654321003"

	inv := &domain.EInvoice{
		InvoiceNumber: "INV-2026-0001",
		IssueDate:     time.Date(2026, 3, 7, 0, 0, 0, 0, time.UTC),
		InvoiceType:   invoice.InvoiceTypeCommercial,
		CurrencyCode:  "AED",
		SupplierTRN:   "100123456789003",
		SupplierName:  "Dubai Fashion House LLC",
		SupplierAddress: domain.AddressUBL{
			StreetName:  "Al Quoz Industrial Area 3",
			CityName:    "Dubai",
			CountryCode: "AE",
		},
		BuyerName: &buyerName,
		BuyerTRN:  &buyerTRN,
		Lines: []domain.EInvoiceLine{
			{
				LineID:          "1",
				Description:     "Blue Linen Dress - Size M",
				Quantity:        2,
				UnitCode:        invoice.UnitPiece,
				UnitPrice:       decimal.NewFromFloat(150.00),
				LineExtension:   decimal.NewFromFloat(300.00),
				TaxCategoryCode: invoice.TaxCatStandard,
				TaxRate:         decimal.NewFromFloat(0.05),
				TaxAmount:       decimal.NewFromFloat(15.00),
			},
			{
				LineID:          "2",
				Description:     "Beige Co-ord Set - Size S",
				Quantity:        1,
				UnitCode:        invoice.UnitPiece,
				UnitPrice:       decimal.NewFromFloat(220.00),
				LineExtension:   decimal.NewFromFloat(220.00),
				TaxCategoryCode: invoice.TaxCatStandard,
				TaxRate:         decimal.NewFromFloat(0.05),
				TaxAmount:       decimal.NewFromFloat(11.00),
			},
		},
		Subtotal:   decimal.NewFromFloat(520.00),
		TaxTotal:   decimal.NewFromFloat(26.00),
		GrandTotal: decimal.NewFromFloat(546.00),
	}

	xmlBytes, err := s.Serialize(inv, decimal.NewFromInt(1))
	if err != nil {
		t.Fatalf("Serialize returned error: %v", err)
	}

	xmlStr := string(xmlBytes)

	// -- Structural assertions --
	assertions := []struct {
		label   string
		contain string
	}{
		{"XML declaration", `<?xml version="1.0" encoding="UTF-8"?>`},
		{"UBL Invoice namespace", `xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2"`},
		{"CBC namespace", `xmlns:cbc="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2"`},
		{"CAC namespace", `xmlns:cac="urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2"`},
		{"PINT-AE CustomizationID", invoice.PINTAECustomizationID},
		{"PINT-AE ProfileID", invoice.PINTAEProfileID},
		{"Invoice number", "INV-2026-0001"},
		{"Issue date", "2026-03-07"},
		{"Invoice type code", "380"},
		{"Currency code", "AED"},
		{"Seller TRN", "100123456789003"},
		{"Seller TIN (first 10)", "1001234567"},
		{"Buyer TRN", "200987654321003"},
		{"Tax total", "26.00"},
		{"Payable amount", "546.00"},
		{"Line 1 description", "Blue Linen Dress"},
		{"Line 2 description", "Beige Co-ord Set"},
		{"VAT category S", `<cbc:ID>S</cbc:ID>`},
		{"VAT percent 5", "5.00"},
		{"AccountingSupplierParty", "AccountingSupplierParty"},
		{"AccountingCustomerParty", "AccountingCustomerParty"},
		{"LegalMonetaryTotal", "LegalMonetaryTotal"},
		{"TaxTotal element", "TaxTotal"},
		{"InvoiceLine element", "InvoiceLine"},
	}

	for _, a := range assertions {
		if !strings.Contains(xmlStr, a.contain) {
			t.Errorf("missing %s: expected to find %q in XML output", a.label, a.contain)
		}
	}
}

// TestTINDerivation verifies that the first 10 digits of TRN become the TIN.
func TestTINDerivation(t *testing.T) {
	tests := []struct {
		trn     string
		wantTIN string
	}{
		{"100123456789003", "1001234567"},
		{"200987654321003", "2009876543"},
		{"12345", "12345"}, // shorter than 10 — use as-is
	}
	for _, tt := range tests {
		sp := invoice.SellerProfile{TRN: tt.trn}
		got := sp.TIN()
		if got != tt.wantTIN {
			t.Errorf("TIN(%q) = %q, want %q", tt.trn, got, tt.wantTIN)
		}
	}
}

// TestZeroRatedExport verifies that export invoices use tax category Z with exemption reason.
func TestZeroRatedExport(t *testing.T) {
	s := &invoice.Serializer{
		SellerProfile: invoice.SellerProfile{
			LegalName:  "Dubai Fashion House LLC",
			TRN:        "100123456789003",
			EndpointID: "0000000000128",
			Address: domain.AddressUBL{
				CityName:    "Dubai",
				CountryCode: "AE",
			},
		},
	}

	inv := &domain.EInvoice{
		InvoiceNumber: "EXP-2026-0001",
		IssueDate:     time.Now().UTC(),
		InvoiceType:   invoice.InvoiceTypeCommercial,
		CurrencyCode:  "USD",
		Lines: []domain.EInvoiceLine{
			{
				LineID:          "1",
				Description:     "Fashion Export",
				Quantity:        10,
				UnitCode:        invoice.UnitPiece,
				UnitPrice:       decimal.NewFromFloat(100.00),
				LineExtension:   decimal.NewFromFloat(1000.00),
				TaxCategoryCode: invoice.TaxCatZeroRated,
				TaxRate:         decimal.Zero,
				TaxAmount:       decimal.Zero,
			},
		},
		Subtotal:   decimal.NewFromFloat(1000.00),
		TaxTotal:   decimal.Zero,
		GrandTotal: decimal.NewFromFloat(1000.00),
	}

	// USD order with 3.67 exchange rate to AED
	xmlBytes, err := s.Serialize(inv, decimal.NewFromFloat(3.67))
	if err != nil {
		t.Fatalf("Serialize returned error: %v", err)
	}

	xmlStr := string(xmlBytes)

	// Should include second TaxTotal in AED (BT-111)
	taxTotalCount := strings.Count(xmlStr, "TaxTotal>")
	if taxTotalCount < 2 {
		t.Errorf("expected 2 TaxTotal blocks for foreign currency order, got %d", taxTotalCount/2)
	}

	// Zero-rated category
	if !strings.Contains(xmlStr, `<cbc:ID>Z</cbc:ID>`) {
		t.Error("expected tax category Z for export invoice")
	}

	// Exemption reason code
	if !strings.Contains(xmlStr, "VATEX-AE-OOS") {
		t.Error("expected UAE FTA exemption reason code VATEX-AE-OOS")
	}
}
