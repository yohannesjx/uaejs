// Package invoice implements the UAE PINT-AE 2026 e-invoicing standard.
//
// Specification reference:
//   - Peppol PINT Billing 1.0 for UAE (pint:billing-1@ae-1)
//   - UBL 2.1 (ISO/IEC 19845)
//   - UAE FTA E-Invoicing Technical Guidelines February 2026
//   - EN 16931-1:2017 semantic model
//
// UBL namespaces used:
//   xmlns     = urn:oasis:names:specification:ubl:schema:xsd:Invoice-2
//   xmlns:cac = urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2
//   xmlns:cbc = urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2
//   xmlns:ext = urn:oasis:names:specification:ubl:schema:xsd:CommonExtensionComponents-2
package invoice

// =============================================================================
// Namespace constants (used as xml.Name.Space values)
// =============================================================================

const (
	NSInvoice = "urn:oasis:names:specification:ubl:schema:xsd:Invoice-2"
	NSCAC     = "urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2"
	NSCBC     = "urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2"
	NSEXT     = "urn:oasis:names:specification:ubl:schema:xsd:CommonExtensionComponents-2"

	// UAE PINT-AE specification identifiers (BT-24 / BT-23)
	PINTAECustomizationID = "urn:peppol:pint:billing-1@ae-1"
	PINTAEProfileID       = "urn:peppol:bis:billing"

	// UAE VAT rate
	UAEStandardVATRate = "5.00"
	UAEZeroVATRate     = "0.00"
)

// =============================================================================
// Invoice type codes (BT-3)
// =============================================================================

const (
	InvoiceTypeCommercial = "380" // Standard commercial invoice
	InvoiceTypeCreditNote = "381" // Credit note
	InvoiceTypeDebitNote  = "383" // Debit note
	InvoiceTypeSelfBilled = "389" // Self-billed invoice
)

// =============================================================================
// Tax category codes (BT-95 / BT-151)
// =============================================================================

const (
	TaxCatStandard  = "S" // UAE 5% VAT
	TaxCatZeroRated = "Z" // 0% (exports, international)
	TaxCatExempt    = "E" // Exempt
)

// =============================================================================
// Unit codes (BT-130) — UN/ECE Recommendation 20
// =============================================================================

const (
	UnitPiece    = "PCE"
	UnitKilogram = "KGM"
	UnitMetre    = "MTR"
)

// =============================================================================
// UBL Invoice — root document
// =============================================================================

// UBLInvoice is the Go representation of a UBL 2.1 Invoice document.
// All 51 mandatory PINT-AE fields are represented; optional fields use pointers.
//
// Serialization: this struct is rendered via the token-based encoder in
// serializer.go, which guarantees correct namespace prefix output (cbc:/cac:).
type UBLInvoice struct {
	// BT-24: Specification identifier
	CustomizationID string // "urn:peppol:pint:billing-1@ae-1"
	// BT-23: Business process type
	ProfileID string // "urn:peppol:bis:billing"
	// BT-1: Invoice number
	ID string
	// BT-2: Invoice issue date (YYYY-MM-DD)
	IssueDate string
	// BT-9: Payment due date (optional)
	DueDate *string
	// BT-3: Invoice type code
	InvoiceTypeCode string // "380"
	// BT-22: Invoice note (optional)
	Note *string
	// BT-5: Invoice currency code
	DocumentCurrencyCode string // "AED"
	// BT-6: VAT accounting currency code (AED for UAE)
	TaxCurrencyCode string // "AED"
	// BT-19: Buyer reference / purchase order (optional)
	BuyerReference *string
	// BT-13: Order reference (optional)
	OrderReference *OrderReference

	// BT-10..BT-40: Seller (Accounting Supplier Party)
	Supplier SupplierParty
	// BT-44..BT-53: Buyer (Accounting Customer Party)
	Buyer BuyerParty

	// Tax total (can be multiple for different currencies)
	TaxTotal        TaxTotal
	TaxTotalInAED   *TaxTotal // populated only when DocumentCurrencyCode != AED

	// BT-106..BT-115: Legal monetary total
	LegalMonetaryTotal LegalMonetaryTotal

	// Invoice lines
	Lines []InvoiceLine
}

// =============================================================================
// Parties
// =============================================================================

// SupplierParty represents the seller (cac:AccountingSupplierParty).
type SupplierParty struct {
	// BT-29: Seller endpoint ID (GLN or Peppol participant ID)
	EndpointID       string // schemeID = "0088" (GLN) or "9915" (Peppol)
	EndpointSchemeID string
	// BT-27: Seller trading name
	TradeName string
	// BT-28: Seller name (legal)
	LegalName string
	// BT-28-AR: Seller name in Arabic (UAE bilingual requirement)
	LegalNameAR *string
	// BT-35..BT-38: Seller postal address
	Address PostalAddress
	// BT-31: Seller VAT number (TRN — 15-digit UAE Tax Registration Number)
	TRN string
	// BT-32: Seller Tax ID (first 10 digits of TRN per UAE PINT-AE spec)
	TIN string
	// BT-30: Seller legal registration identifier (Trade License number)
	TradeLicenseNumber *string
	// BT-41: Seller contact (optional)
	Contact *Contact
}

// BuyerParty represents the customer (cac:AccountingCustomerParty).
type BuyerParty struct {
	// BT-49: Buyer electronic address (email)
	EndpointID       *string
	EndpointSchemeID string // "EM" for email
	// BT-44: Buyer name
	Name string
	// BT-44-AR: Buyer name in Arabic (optional)
	NameAR *string
	// BT-50..BT-53: Buyer postal address
	Address *PostalAddress
	// BT-48: Buyer VAT number (TRN — required for B2B invoices)
	TRN *string
	// BT-46: Buyer ID (optional internal reference)
	ID *string
}

// PostalAddress represents a UBL cac:PostalAddress.
type PostalAddress struct {
	// BT-35 / BT-50
	StreetName *string
	// BT-36 / BT-51
	AdditionalStreetName *string
	// BT-37 / BT-52
	CityName string
	// BT-38 / BT-53
	PostalZone *string
	// BT-39
	CountrySubentity *string // e.g. "Dubai", "Abu Dhabi"
	// BT-40 / BT-55: ISO 3166-1 alpha-2 country code
	CountryCode string // "AE"
}

// Contact holds optional seller contact details.
type Contact struct {
	Name  *string
	Phone *string
	Email *string
}

// OrderReference is an optional purchase order reference (BT-13).
type OrderReference struct {
	ID string
}

// =============================================================================
// Tax structures
// =============================================================================

// TaxTotal represents a cac:TaxTotal element.
type TaxTotal struct {
	// Total VAT amount in the stated currency
	TaxAmount  string
	CurrencyID string
	// Tax subtotals (one per tax category)
	TaxSubtotals []TaxSubtotal
}

// TaxSubtotal represents a single cac:TaxSubtotal breakdown.
type TaxSubtotal struct {
	TaxableAmount string // Net amount subject to this tax category
	TaxAmount     string // Tax amount for this category
	CurrencyID    string
	TaxCategory   TaxCategory
}

// TaxCategory describes the VAT category applied (BT-95..BT-100).
type TaxCategory struct {
	// BT-95: Tax category code (S, Z, E)
	ID string
	// BT-96: VAT rate percentage (e.g. "5.00")
	Percent string
	// BT-99/BT-100: Exemption reason code and text (required when ID = E or Z)
	TaxExemptionReasonCode *string
	TaxExemptionReason     *string
}

// =============================================================================
// Monetary totals
// =============================================================================

// LegalMonetaryTotal holds the cac:LegalMonetaryTotal element (BT-106..BT-115).
type LegalMonetaryTotal struct {
	CurrencyID string
	// BT-106: Sum of invoice line net amounts
	LineExtensionAmount string
	// BT-109: Invoice total amount without VAT
	TaxExclusiveAmount string
	// BT-112: Invoice total amount with VAT
	TaxInclusiveAmount string
	// BT-107: Sum of document-level allowances (optional)
	AllowanceTotalAmount *string
	// BT-108: Sum of document-level charges (optional)
	ChargeTotalAmount *string
	// BT-113: Paid amount (optional, for partial payments)
	PrepaidAmount *string
	// BT-114: Rounding amount (optional)
	PayableRoundingAmount *string
	// BT-115: Amount due for payment
	PayableAmount string
}

// =============================================================================
// Invoice lines
// =============================================================================

// InvoiceLine represents a cac:InvoiceLine element (BT-25..BT-161).
type InvoiceLine struct {
	// BT-126: Invoice line identifier
	ID string
	// BT-127: Invoice line note (optional)
	Note *string
	// BT-129: Invoiced quantity
	InvoicedQuantity int
	// BT-130: Unit of measure code
	UnitCode string
	// BT-131: Invoice line net amount (excl. VAT)
	LineExtensionAmount string
	CurrencyID          string
	// BT-133: Buyer accounting reference (optional)
	AccountingCost *string
	// Item details
	Item Item
	// BT-146: Item net price (per unit)
	Price LinePrice
	// Line-level tax
	TaxTotal *LineTaxTotal
}

// Item holds the cac:Item sub-element of InvoiceLine.
type Item struct {
	// BT-154: Item description (max 500 chars)
	Description *string
	// BT-154-AR: Arabic description (UAE bilingual support)
	DescriptionAR *string
	// BT-153: Item name
	Name string
	// BT-153-AR: Arabic name
	NameAR *string
	// BT-155: Seller's item identifier (SKU)
	SellerItemID *string
	// BT-157: Item standard identifier (GTIN/barcode)
	StandardItemID *StandardItemID
	// BT-158: Item classification identifier (HS code)
	CommodityClassification *CommodityClassification
	// BT-159: Country of origin
	OriginCountry *string
	// BT-151/BT-152: Tax category on this item
	ClassifiedTaxCategory TaxCategory
}

// StandardItemID represents a BT-157 standard item identifier with scheme.
type StandardItemID struct {
	ID       string
	SchemeID string // "0160" for GTIN, "AE_CUSTOMS" for UAE HS code
}

// CommodityClassification holds the HS/UNSPSC code (BT-158).
type CommodityClassification struct {
	ItemClassificationCode string
	ListID                 string // "HS" for Harmonised System
}

// LinePrice is the cac:Price element within an InvoiceLine.
type LinePrice struct {
	// BT-146: Item net price
	PriceAmount string
	CurrencyID  string
	// BT-149: Item price base quantity (optional, defaults to 1)
	BaseQuantity *int
	BaseUnitCode string
	// BT-147: Item price discount (optional)
	AllowanceCharge *PriceAllowanceCharge
}

// PriceAllowanceCharge represents a discount on the line price.
type PriceAllowanceCharge struct {
	ChargeIndicator     bool   // false = allowance (discount)
	Amount              string
	BaseAmount          string
	CurrencyID          string
	AllowanceChargeReasonCode *string
}

// LineTaxTotal is the optional per-line tax total.
type LineTaxTotal struct {
	TaxAmount  string
	CurrencyID string
}
