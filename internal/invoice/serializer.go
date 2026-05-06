package invoice

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/dubai-retail/os/internal/domain"
	"github.com/shopspring/decimal"
)

// =============================================================================
// Public API
// =============================================================================

// Serializer converts domain order data into a UAE PINT-AE UBL 2.1 XML invoice.
type Serializer struct {
	// SellerProfile holds static supplier information configured once at startup.
	SellerProfile SellerProfile
}

// SellerProfile holds the fixed seller-side data required in every invoice.
type SellerProfile struct {
	LegalName          string
	LegalNameAR        *string
	TRN                string  // 15-digit UAE TRN, e.g. "100123456789003"
	TradeLicenseNumber *string
	EndpointID         string  // GLN or Peppol ID
	Address            domain.AddressUBL
	ContactEmail       *string
}

// TIN derives the 10-digit Tax Identification Number from the 15-digit TRN.
// Per UAE PINT-AE spec BT-32: TIN = first 10 digits of TRN.
func (sp *SellerProfile) TIN() string {
	trn := strings.ReplaceAll(sp.TRN, "-", "")
	if len(trn) >= 10 {
		return trn[:10]
	}
	return trn
}

// =============================================================================
// Serialize  –  main entry point
// =============================================================================

// Serialize converts a domain.EInvoice into a valid UBL 2.1 XML byte slice.
//
// The output is ready for submission to a UAE FTA-Accredited Service Provider (ASP).
// Currency conversion: if the order is not in AED, a second cac:TaxTotal block
// with AED amounts is included (BT-111 requirement).
func (s *Serializer) Serialize(inv *domain.EInvoice, exchangeRateToAED decimal.Decimal) ([]byte, error) {
	ubl, err := s.buildUBL(inv, exchangeRateToAED)
	if err != nil {
		return nil, fmt.Errorf("invoice.Serialize: build UBL: %w", err)
	}
	return encodeUBL(ubl)
}

// =============================================================================
// Builder:  domain.EInvoice  →  UBLInvoice
// =============================================================================

func (s *Serializer) buildUBL(inv *domain.EInvoice, exchangeRateToAED decimal.Decimal) (*UBLInvoice, error) {
	currency := inv.CurrencyCode
	if currency == "" {
		currency = "AED"
	}

	// --- Supplier party ---
	tin := s.SellerProfile.TIN()
	supplier := SupplierParty{
		EndpointID:         s.SellerProfile.EndpointID,
		EndpointSchemeID:   "0088",
		LegalName:          s.SellerProfile.LegalName,
		LegalNameAR:        s.SellerProfile.LegalNameAR,
		TradeLicenseNumber: s.SellerProfile.TradeLicenseNumber,
		TRN:                s.SellerProfile.TRN,
		TIN:                tin,
		Address: PostalAddress{
			StreetName:  strPtr(s.SellerProfile.Address.StreetName),
			CityName:    s.SellerProfile.Address.CityName,
			PostalZone:  nilIfEmpty(s.SellerProfile.Address.PostalZone),
			CountryCode: s.SellerProfile.Address.CountryCode,
		},
	}
	if s.SellerProfile.ContactEmail != nil {
		supplier.Contact = &Contact{Email: s.SellerProfile.ContactEmail}
	}

	// --- Buyer party ---
	buyer := BuyerParty{
		Name:   derefStr(inv.BuyerName, "Consumer"),
		NameAR: nil,
		TRN:    inv.BuyerTRN,
	}
	if inv.BuyerAddress != nil {
		buyer.Address = &PostalAddress{
			StreetName:  strPtr(inv.BuyerAddress.StreetName),
			CityName:    inv.BuyerAddress.CityName,
			PostalZone:  nilIfEmpty(inv.BuyerAddress.PostalZone),
			CountryCode: inv.BuyerAddress.CountryCode,
		}
	}

	// --- Lines ---
	ublLines := make([]InvoiceLine, 0, len(inv.Lines))
	for i, l := range inv.Lines {
		ublLines = append(ublLines, buildLine(i+1, l, currency))
	}

	// --- Tax total ---
	taxTotal := buildTaxTotal(inv, currency)

	// --- AED tax total (BT-111) when order is in a foreign currency ---
	var taxTotalAED *TaxTotal
	if currency != "AED" && !exchangeRateToAED.IsZero() {
		vatAED := inv.TaxTotal.Mul(exchangeRateToAED).Round(2)
		t := buildTaxTotalFromAmount(vatAED.String(), "AED", getFirstTaxCat(inv))
		taxTotalAED = &t
	}

	// --- Legal monetary total ---
	total := buildMonetaryTotal(inv, currency)

	ubl := &UBLInvoice{
		CustomizationID:      PINTAECustomizationID,
		ProfileID:            PINTAEProfileID,
		ID:                   inv.InvoiceNumber,
		IssueDate:            inv.IssueDate.UTC().Format("2006-01-02"),
		InvoiceTypeCode:      inv.InvoiceType,
		DocumentCurrencyCode: currency,
		TaxCurrencyCode:      "AED",
		Supplier:             supplier,
		Buyer:                buyer,
		TaxTotal:             taxTotal,
		TaxTotalInAED:        taxTotalAED,
		LegalMonetaryTotal:   total,
		Lines:                ublLines,
	}

	return ubl, nil
}

// =============================================================================
// Sub-builders
// =============================================================================

func buildLine(seq int, l domain.EInvoiceLine, currency string) InvoiceLine {
	lineNet := l.LineExtension.String()

	taxCat := TaxCategory{
		ID:      l.TaxCategoryCode,
		Percent: l.TaxRate.Mul(decimal.NewFromInt(100)).String(),
	}
	if l.TaxCategoryCode == TaxCatZeroRated || l.TaxCategoryCode == TaxCatExempt {
		reason := "Export or VAT-exempt supply"
		code := "VATEX-AE-OOS" // UAE FTA exemption code
		taxCat.TaxExemptionReasonCode = &code
		taxCat.TaxExemptionReason = &reason
	}

	item := Item{
		Name:                  l.Description,
		ClassifiedTaxCategory: taxCat,
	}

	price := LinePrice{
		PriceAmount: l.UnitPrice.String(),
		CurrencyID:  currency,
	}

	return InvoiceLine{
		ID:                  strconv.Itoa(seq),
		InvoicedQuantity:    l.Quantity,
		UnitCode:            UnitPiece,
		LineExtensionAmount: lineNet,
		CurrencyID:          currency,
		Item:                item,
		Price:               price,
	}
}

func buildTaxTotal(inv *domain.EInvoice, currency string) TaxTotal {
	// Group lines by tax category
	type catKey struct{ code, rate string }
	type catData struct {
		taxable decimal.Decimal
		tax     decimal.Decimal
	}
	cats := make(map[catKey]*catData)

	for _, l := range inv.Lines {
		k := catKey{
			code: l.TaxCategoryCode,
			rate: l.TaxRate.Mul(decimal.NewFromInt(100)).String(),
		}
		if _, ok := cats[k]; !ok {
			cats[k] = &catData{}
		}
		cats[k].taxable = cats[k].taxable.Add(l.LineExtension)
		cats[k].tax = cats[k].tax.Add(l.TaxAmount)
	}

	subtotals := make([]TaxSubtotal, 0, len(cats))
	for k, d := range cats {
		pct := k.rate
		cat := TaxCategory{ID: k.code, Percent: pct}
		if k.code == TaxCatZeroRated || k.code == TaxCatExempt {
			reason := "Export or VAT-exempt supply"
			code := "VATEX-AE-OOS"
			cat.TaxExemptionReasonCode = &code
			cat.TaxExemptionReason = &reason
		}
		subtotals = append(subtotals, TaxSubtotal{
			TaxableAmount: d.taxable.Round(2).String(),
			TaxAmount:     d.tax.Round(2).String(),
			CurrencyID:    currency,
			TaxCategory:   cat,
		})
	}

	return TaxTotal{
		TaxAmount:    inv.TaxTotal.Round(2).String(),
		CurrencyID:   currency,
		TaxSubtotals: subtotals,
	}
}

func buildTaxTotalFromAmount(amount, currency, taxCatCode string) TaxTotal {
	return TaxTotal{
		TaxAmount:  amount,
		CurrencyID: currency,
		TaxSubtotals: []TaxSubtotal{{
			TaxableAmount: "0.00",
			TaxAmount:     amount,
			CurrencyID:    currency,
			TaxCategory:   TaxCategory{ID: taxCatCode, Percent: UAEStandardVATRate},
		}},
	}
}

func buildMonetaryTotal(inv *domain.EInvoice, currency string) LegalMonetaryTotal {
	payable := inv.GrandTotal.Round(2).String()
	taxExcl := inv.Subtotal.Round(2).String()
	taxIncl := inv.GrandTotal.Round(2).String()
	lineExt := inv.Subtotal.Round(2).String()

	return LegalMonetaryTotal{
		CurrencyID:          currency,
		LineExtensionAmount: lineExt,
		TaxExclusiveAmount:  taxExcl,
		TaxInclusiveAmount:  taxIncl,
		PayableAmount:       payable,
	}
}

func getFirstTaxCat(inv *domain.EInvoice) string {
	if len(inv.Lines) > 0 {
		return inv.Lines[0].TaxCategoryCode
	}
	return TaxCatStandard
}

// =============================================================================
// UBL XML encoder  –  token-based for canonical prefix output
// =============================================================================
//
// Go's encoding/xml encoder respects namespace prefix declarations when you:
//  1. Declare xmlns:cbc / xmlns:cac as Attr on the root element.
//  2. Use xml.Name{Space: NSCBC, Local: "..."} for child elements.
// The encoder reuses the declared prefix rather than generating ns0:/ns1:.

func encodeUBL(inv *UBLInvoice) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	buf.WriteByte('\n')

	enc := xml.NewEncoder(&buf)
	enc.Indent("", "  ")

	rootStart := xml.StartElement{
		Name: xml.Name{Space: NSInvoice, Local: "Invoice"},
		Attr: []xml.Attr{
			{Name: xml.Name{Local: "xmlns"}, Value: NSInvoice},
			{Name: xml.Name{Local: "xmlns:cac"}, Value: NSCAC},
			{Name: xml.Name{Local: "xmlns:cbc"}, Value: NSCBC},
			{Name: xml.Name{Local: "xmlns:ext"}, Value: NSEXT},
		},
	}
	must(enc.EncodeToken(rootStart))

	// BT-24 / BT-23
	encText(enc, NSCBC, "CustomizationID", inv.CustomizationID)
	encText(enc, NSCBC, "ProfileID", inv.ProfileID)

	// BT-1
	encText(enc, NSCBC, "ID", inv.ID)
	// BT-2
	encText(enc, NSCBC, "IssueDate", inv.IssueDate)
	// BT-3
	encText(enc, NSCBC, "InvoiceTypeCode", inv.InvoiceTypeCode)
	// Optional note
	if inv.Note != nil {
		encText(enc, NSCBC, "Note", *inv.Note)
	}
	// BT-5
	encText(enc, NSCBC, "DocumentCurrencyCode", inv.DocumentCurrencyCode)
	// BT-6
	encText(enc, NSCBC, "TaxCurrencyCode", inv.TaxCurrencyCode)
	// BT-19 Buyer reference
	if inv.BuyerReference != nil {
		encText(enc, NSCBC, "BuyerReference", *inv.BuyerReference)
	}
	// BT-13 Order reference
	if inv.OrderReference != nil {
		open := startElem(NSCAC, "OrderReference")
		must(enc.EncodeToken(open))
		encText(enc, NSCBC, "ID", inv.OrderReference.ID)
		must(enc.EncodeToken(open.End()))
	}

	// cac:AccountingSupplierParty
	encodeSupplier(enc, inv.Supplier)

	// cac:AccountingCustomerParty
	encodeBuyer(enc, inv.Buyer)

	// cac:TaxTotal (invoice currency)
	encodeTaxTotal(enc, inv.TaxTotal)

	// cac:TaxTotal (AED, BT-111 – only when currency != AED)
	if inv.TaxTotalInAED != nil {
		encodeTaxTotal(enc, *inv.TaxTotalInAED)
	}

	// cac:LegalMonetaryTotal
	encodeMonetaryTotal(enc, inv.LegalMonetaryTotal)

	// cac:InvoiceLine (×N)
	for _, line := range inv.Lines {
		encodeInvoiceLine(enc, line)
	}

	must(enc.EncodeToken(rootStart.End()))
	if err := enc.Flush(); err != nil {
		return nil, fmt.Errorf("encodeUBL: flush: %w", err)
	}

	return buf.Bytes(), nil
}

// =============================================================================
// Element encoders
// =============================================================================

func encodeSupplier(enc *xml.Encoder, s SupplierParty) {
	asp := startElem(NSCAC, "AccountingSupplierParty")
	must(enc.EncodeToken(asp))
	party := startElem(NSCAC, "Party")
	must(enc.EncodeToken(party))

	// BT-29: Endpoint ID
	ep := startElemAttrs(NSCBC, "EndpointID", []xml.Attr{
		{Name: xml.Name{Local: "schemeID"}, Value: s.EndpointSchemeID},
	})
	must(enc.EncodeToken(ep))
	must(enc.EncodeToken(xml.CharData(s.EndpointID)))
	must(enc.EncodeToken(ep.End()))

	// BT-27: Party name
	pn := startElem(NSCAC, "PartyName")
	must(enc.EncodeToken(pn))
	encText(enc, NSCBC, "Name", s.LegalName)
	// Bilingual: append Arabic name as a second cbc:Name if present
	if s.LegalNameAR != nil {
		encText(enc, NSCBC, "Name", *s.LegalNameAR)
	}
	must(enc.EncodeToken(pn.End()))

	// Postal address
	encodeAddress(enc, s.Address)

	// cac:PartyTaxScheme (TRN + TIN)
	pts := startElem(NSCAC, "PartyTaxScheme")
	must(enc.EncodeToken(pts))
	encText(enc, NSCBC, "CompanyID", s.TRN) // BT-31: full TRN
	ts := startElem(NSCAC, "TaxScheme")
	must(enc.EncodeToken(ts))
	encText(enc, NSCBC, "ID", "VAT")
	must(enc.EncodeToken(ts.End()))
	must(enc.EncodeToken(pts.End()))

	// cac:PartyLegalEntity
	ple := startElem(NSCAC, "PartyLegalEntity")
	must(enc.EncodeToken(ple))
	encText(enc, NSCBC, "RegistrationName", s.LegalName)
	if s.TradeLicenseNumber != nil {
		encText(enc, NSCBC, "CompanyID", *s.TradeLicenseNumber) // Trade License
	}
	must(enc.EncodeToken(ple.End()))

	// BT-32: TIN (first 10 digits of TRN) in cac:PartyIdentification
	pi := startElem(NSCAC, "PartyIdentification")
	must(enc.EncodeToken(pi))
	tinEl := startElemAttrs(NSCBC, "ID", []xml.Attr{
		{Name: xml.Name{Local: "schemeID"}, Value: "AE:TIN"},
	})
	must(enc.EncodeToken(tinEl))
	must(enc.EncodeToken(xml.CharData(s.TIN)))
	must(enc.EncodeToken(tinEl.End()))
	must(enc.EncodeToken(pi.End()))

	// Optional contact
	if s.Contact != nil {
		ct := startElem(NSCAC, "Contact")
		must(enc.EncodeToken(ct))
		if s.Contact.Name != nil {
			encText(enc, NSCBC, "Name", *s.Contact.Name)
		}
		if s.Contact.Phone != nil {
			encText(enc, NSCBC, "Telephone", *s.Contact.Phone)
		}
		if s.Contact.Email != nil {
			encText(enc, NSCBC, "ElectronicMail", *s.Contact.Email)
		}
		must(enc.EncodeToken(ct.End()))
	}

	must(enc.EncodeToken(party.End()))
	must(enc.EncodeToken(asp.End()))
}

func encodeBuyer(enc *xml.Encoder, b BuyerParty) {
	acp := startElem(NSCAC, "AccountingCustomerParty")
	must(enc.EncodeToken(acp))
	party := startElem(NSCAC, "Party")
	must(enc.EncodeToken(party))

	// Endpoint (email)
	if b.EndpointID != nil {
		schemeID := b.EndpointSchemeID
		if schemeID == "" {
			schemeID = "EM"
		}
		ep := startElemAttrs(NSCBC, "EndpointID", []xml.Attr{
			{Name: xml.Name{Local: "schemeID"}, Value: schemeID},
		})
		must(enc.EncodeToken(ep))
		must(enc.EncodeToken(xml.CharData(*b.EndpointID)))
		must(enc.EncodeToken(ep.End()))
	}

	// Name
	pn := startElem(NSCAC, "PartyName")
	must(enc.EncodeToken(pn))
	encText(enc, NSCBC, "Name", b.Name)
	if b.NameAR != nil {
		encText(enc, NSCBC, "Name", *b.NameAR)
	}
	must(enc.EncodeToken(pn.End()))

	// Address
	if b.Address != nil {
		encodeAddress(enc, *b.Address)
	}

	// TRN (B2B only)
	if b.TRN != nil {
		pts := startElem(NSCAC, "PartyTaxScheme")
		must(enc.EncodeToken(pts))
		encText(enc, NSCBC, "CompanyID", *b.TRN)
		ts := startElem(NSCAC, "TaxScheme")
		must(enc.EncodeToken(ts))
		encText(enc, NSCBC, "ID", "VAT")
		must(enc.EncodeToken(ts.End()))
		must(enc.EncodeToken(pts.End()))
	}

	// Legal entity (buyer name)
	ple := startElem(NSCAC, "PartyLegalEntity")
	must(enc.EncodeToken(ple))
	encText(enc, NSCBC, "RegistrationName", b.Name)
	if b.ID != nil {
		encText(enc, NSCBC, "CompanyID", *b.ID)
	}
	must(enc.EncodeToken(ple.End()))

	must(enc.EncodeToken(party.End()))
	must(enc.EncodeToken(acp.End()))
}

func encodeAddress(enc *xml.Encoder, a PostalAddress) {
	pa := startElem(NSCAC, "PostalAddress")
	must(enc.EncodeToken(pa))
	if a.StreetName != nil {
		encText(enc, NSCBC, "StreetName", *a.StreetName)
	}
	if a.AdditionalStreetName != nil {
		encText(enc, NSCBC, "AdditionalStreetName", *a.AdditionalStreetName)
	}
	encText(enc, NSCBC, "CityName", a.CityName)
	if a.PostalZone != nil {
		encText(enc, NSCBC, "PostalZone", *a.PostalZone)
	}
	if a.CountrySubentity != nil {
		encText(enc, NSCBC, "CountrySubentity", *a.CountrySubentity)
	}
	country := startElem(NSCAC, "Country")
	must(enc.EncodeToken(country))
	encText(enc, NSCBC, "IdentificationCode", a.CountryCode)
	must(enc.EncodeToken(country.End()))
	must(enc.EncodeToken(pa.End()))
}

func encodeTaxTotal(enc *xml.Encoder, t TaxTotal) {
	tt := startElem(NSCAC, "TaxTotal")
	must(enc.EncodeToken(tt))

	ta := startElemAttrs(NSCBC, "TaxAmount", []xml.Attr{
		{Name: xml.Name{Local: "currencyID"}, Value: t.CurrencyID},
	})
	must(enc.EncodeToken(ta))
	must(enc.EncodeToken(xml.CharData(t.TaxAmount)))
	must(enc.EncodeToken(ta.End()))

	for _, sub := range t.TaxSubtotals {
		st := startElem(NSCAC, "TaxSubtotal")
		must(enc.EncodeToken(st))

		txbl := startElemAttrs(NSCBC, "TaxableAmount", []xml.Attr{
			{Name: xml.Name{Local: "currencyID"}, Value: sub.CurrencyID},
		})
		must(enc.EncodeToken(txbl))
		must(enc.EncodeToken(xml.CharData(sub.TaxableAmount)))
		must(enc.EncodeToken(txbl.End()))

		taxAmt := startElemAttrs(NSCBC, "TaxAmount", []xml.Attr{
			{Name: xml.Name{Local: "currencyID"}, Value: sub.CurrencyID},
		})
		must(enc.EncodeToken(taxAmt))
		must(enc.EncodeToken(xml.CharData(sub.TaxAmount)))
		must(enc.EncodeToken(taxAmt.End()))

		encodeTaxCategory(enc, sub.TaxCategory)
		must(enc.EncodeToken(st.End()))
	}

	must(enc.EncodeToken(tt.End()))
}

func encodeTaxCategory(enc *xml.Encoder, cat TaxCategory) {
	tc := startElem(NSCAC, "TaxCategory")
	must(enc.EncodeToken(tc))
	encText(enc, NSCBC, "ID", cat.ID)
	encText(enc, NSCBC, "Percent", cat.Percent)
	if cat.TaxExemptionReasonCode != nil {
		encText(enc, NSCBC, "TaxExemptionReasonCode", *cat.TaxExemptionReasonCode)
	}
	if cat.TaxExemptionReason != nil {
		encText(enc, NSCBC, "TaxExemptionReason", *cat.TaxExemptionReason)
	}
	ts := startElem(NSCAC, "TaxScheme")
	must(enc.EncodeToken(ts))
	encText(enc, NSCBC, "ID", "VAT")
	must(enc.EncodeToken(ts.End()))
	must(enc.EncodeToken(tc.End()))
}

func encodeMonetaryTotal(enc *xml.Encoder, m LegalMonetaryTotal) {
	lmt := startElem(NSCAC, "LegalMonetaryTotal")
	must(enc.EncodeToken(lmt))

	encDecimal(enc, NSCBC, "LineExtensionAmount", m.LineExtensionAmount, m.CurrencyID)
	encDecimal(enc, NSCBC, "TaxExclusiveAmount", m.TaxExclusiveAmount, m.CurrencyID)
	encDecimal(enc, NSCBC, "TaxInclusiveAmount", m.TaxInclusiveAmount, m.CurrencyID)
	if m.AllowanceTotalAmount != nil {
		encDecimal(enc, NSCBC, "AllowanceTotalAmount", *m.AllowanceTotalAmount, m.CurrencyID)
	}
	if m.PrepaidAmount != nil {
		encDecimal(enc, NSCBC, "PrepaidAmount", *m.PrepaidAmount, m.CurrencyID)
	}
	if m.PayableRoundingAmount != nil {
		encDecimal(enc, NSCBC, "PayableRoundingAmount", *m.PayableRoundingAmount, m.CurrencyID)
	}
	encDecimal(enc, NSCBC, "PayableAmount", m.PayableAmount, m.CurrencyID)

	must(enc.EncodeToken(lmt.End()))
}

func encodeInvoiceLine(enc *xml.Encoder, l InvoiceLine) {
	il := startElem(NSCAC, "InvoiceLine")
	must(enc.EncodeToken(il))

	// BT-126
	encText(enc, NSCBC, "ID", l.ID)
	if l.Note != nil {
		encText(enc, NSCBC, "Note", *l.Note)
	}

	// BT-129/BT-130: Invoiced quantity
	iq := startElemAttrs(NSCBC, "InvoicedQuantity", []xml.Attr{
		{Name: xml.Name{Local: "unitCode"}, Value: l.UnitCode},
	})
	must(enc.EncodeToken(iq))
	must(enc.EncodeToken(xml.CharData(strconv.Itoa(l.InvoicedQuantity))))
	must(enc.EncodeToken(iq.End()))

	// BT-131
	encDecimal(enc, NSCBC, "LineExtensionAmount", l.LineExtensionAmount, l.CurrencyID)

	// Optional accounting cost
	if l.AccountingCost != nil {
		encText(enc, NSCBC, "AccountingCost", *l.AccountingCost)
	}

	// cac:Item
	item := startElem(NSCAC, "Item")
	must(enc.EncodeToken(item))
	if l.Item.Description != nil {
		encText(enc, NSCBC, "Description", *l.Item.Description)
	}
	if l.Item.DescriptionAR != nil {
		// Bilingual: Arabic description in a second cbc:Description element
		encText(enc, NSCBC, "Description", *l.Item.DescriptionAR)
	}
	encText(enc, NSCBC, "Name", l.Item.Name)
	if l.Item.NameAR != nil {
		encText(enc, NSCBC, "Name", *l.Item.NameAR)
	}
	if l.Item.SellerItemID != nil {
		si := startElem(NSCAC, "SellersItemIdentification")
		must(enc.EncodeToken(si))
		encText(enc, NSCBC, "ID", *l.Item.SellerItemID)
		must(enc.EncodeToken(si.End()))
	}
	if l.Item.StandardItemID != nil {
		stdi := startElem(NSCAC, "StandardItemIdentification")
		must(enc.EncodeToken(stdi))
		sid := startElemAttrs(NSCBC, "ID", []xml.Attr{
			{Name: xml.Name{Local: "schemeID"}, Value: l.Item.StandardItemID.SchemeID},
		})
		must(enc.EncodeToken(sid))
		must(enc.EncodeToken(xml.CharData(l.Item.StandardItemID.ID)))
		must(enc.EncodeToken(sid.End()))
		must(enc.EncodeToken(stdi.End()))
	}
	if l.Item.CommodityClassification != nil {
		cc := startElem(NSCAC, "CommodityClassification")
		must(enc.EncodeToken(cc))
		icc := startElemAttrs(NSCBC, "ItemClassificationCode", []xml.Attr{
			{Name: xml.Name{Local: "listID"}, Value: l.Item.CommodityClassification.ListID},
		})
		must(enc.EncodeToken(icc))
		must(enc.EncodeToken(xml.CharData(l.Item.CommodityClassification.ItemClassificationCode)))
		must(enc.EncodeToken(icc.End()))
		must(enc.EncodeToken(cc.End()))
	}
	if l.Item.OriginCountry != nil {
		oc := startElem(NSCAC, "OriginCountry")
		must(enc.EncodeToken(oc))
		encText(enc, NSCBC, "IdentificationCode", *l.Item.OriginCountry)
		must(enc.EncodeToken(oc.End()))
	}
	encodeTaxCategory(enc, l.Item.ClassifiedTaxCategory)
	must(enc.EncodeToken(item.End()))

	// cac:Price
	price := startElem(NSCAC, "Price")
	must(enc.EncodeToken(price))
	encDecimal(enc, NSCBC, "PriceAmount", l.Price.PriceAmount, l.Price.CurrencyID)
	if l.Price.BaseQuantity != nil {
		bq := startElemAttrs(NSCBC, "BaseQuantity", []xml.Attr{
			{Name: xml.Name{Local: "unitCode"}, Value: l.Price.BaseUnitCode},
		})
		must(enc.EncodeToken(bq))
		must(enc.EncodeToken(xml.CharData(strconv.Itoa(*l.Price.BaseQuantity))))
		must(enc.EncodeToken(bq.End()))
	}
	if l.Price.AllowanceCharge != nil {
		ac := startElem(NSCAC, "AllowanceCharge")
		must(enc.EncodeToken(ac))
		chargeInd := "false"
		if l.Price.AllowanceCharge.ChargeIndicator {
			chargeInd = "true"
		}
		encText(enc, NSCBC, "ChargeIndicator", chargeInd)
		encDecimal(enc, NSCBC, "Amount", l.Price.AllowanceCharge.Amount, l.Price.AllowanceCharge.CurrencyID)
		encDecimal(enc, NSCBC, "BaseAmount", l.Price.AllowanceCharge.BaseAmount, l.Price.AllowanceCharge.CurrencyID)
		must(enc.EncodeToken(ac.End()))
	}
	must(enc.EncodeToken(price.End()))

	must(enc.EncodeToken(il.End()))
}

// =============================================================================
// Conversion helper:  domain.EInvoice  →  domain.EInvoice (no-op, just validation)
// =============================================================================

// BuildFromOrder converts a domain.Order + items into the domain.EInvoice
// intermediate representation, which Serializer.Serialize then renders to XML.
func BuildFromOrder(
	order *domain.Order,
	sellerName, sellerTRN string,
	sellerAddr domain.AddressUBL,
	exchangeRateToAED decimal.Decimal,
) (*domain.EInvoice, error) {
	if order.InvoiceNumber == nil {
		return nil, fmt.Errorf("BuildFromOrder: order has no invoice number")
	}

	vatType := TaxCatStandard
	if order.VATType == domain.VATTypeZeroRated {
		vatType = TaxCatZeroRated
	} else if order.VATType == domain.VATTypeExempt {
		vatType = TaxCatExempt
	}

	invType := InvoiceTypeCommercial

	lines := make([]domain.EInvoiceLine, 0, len(order.Items))
	for i, item := range order.Items {
		vatRate := item.VATRate
		lines = append(lines, domain.EInvoiceLine{
			LineID:          strconv.Itoa(i + 1),
			Description:     item.VariantID.String(), // caller should replace with variant name
			Quantity:        item.Quantity,
			UnitCode:        UnitPiece,
			UnitPrice:       item.UnitPrice,
			LineExtension:   item.LineTotal.Sub(item.VATAmount),
			TaxCategoryCode: vatType,
			TaxRate:         vatRate,
			TaxAmount:       item.VATAmount,
		})
	}

	inv := &domain.EInvoice{
		InvoiceNumber:   *order.InvoiceNumber,
		IssueDate:       time.Now().UTC(),
		InvoiceType:     invType,
		CurrencyCode:    order.Currency,
		SupplierTRN:     sellerTRN,
		SupplierName:    sellerName,
		SupplierAddress: sellerAddr,
		BuyerTRN:        order.CustomerTRN,
		BuyerName:       order.CustomerName,
		Lines:           lines,
		Subtotal:        order.Subtotal,
		TaxTotal:        order.VATAmount,
		GrandTotal:      order.TotalAmount,
	}

	if order.CustomerEmail != nil {
		inv.BuyerAddress = nil // populated from shipping address if available
	}

	return inv, nil
}

// =============================================================================
// Low-level token helpers
// =============================================================================

func startElem(ns, local string) xml.StartElement {
	return xml.StartElement{Name: xml.Name{Space: ns, Local: local}}
}

func startElemAttrs(ns, local string, attrs []xml.Attr) xml.StartElement {
	return xml.StartElement{Name: xml.Name{Space: ns, Local: local}, Attr: attrs}
}

func encText(enc *xml.Encoder, ns, local, value string) {
	el := startElem(ns, local)
	must(enc.EncodeToken(el))
	must(enc.EncodeToken(xml.CharData(value)))
	must(enc.EncodeToken(el.End()))
}

func encDecimal(enc *xml.Encoder, ns, local, value, currencyID string) {
	el := startElemAttrs(ns, local, []xml.Attr{
		{Name: xml.Name{Local: "currencyID"}, Value: currencyID},
	})
	must(enc.EncodeToken(el))
	must(enc.EncodeToken(xml.CharData(value)))
	must(enc.EncodeToken(el.End()))
}

// must panics on xml.Encoder errors – these only occur on OOM or closed writers.
func must(err error) {
	if err != nil {
		panic(fmt.Sprintf("invoice.encodeUBL internal error: %v", err))
	}
}

// =============================================================================
// String helpers
// =============================================================================

func strPtr(s string) *string { return &s }

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func derefStr(s *string, fallback string) string {
	if s == nil {
		return fallback
	}
	return *s
}
