package service

import (
	"bytes"
	"fmt"
	"html/template"
	"time"

	"github.com/dubai-retail/os/internal/domain"
)

// ReceiptService generates JSON-ready structs and printable HTML receipts.
type ReceiptService struct {
	tmpl *template.Template
}

// NewReceiptService parses the built-in receipt template.
func NewReceiptService() *ReceiptService {
	tmpl := template.Must(template.New("receipt").Funcs(template.FuncMap{
		"fmtTime": func(t time.Time) string {
			return t.Format("02 Jan 2006 15:04:05")
		},
	}).Parse(receiptHTMLTemplate))
	return &ReceiptService{tmpl: tmpl}
}

// RenderHTML produces a minimal printable HTML receipt string.
func (rs *ReceiptService) RenderHTML(r *domain.POSReceipt) (string, error) {
	var buf bytes.Buffer
	if err := rs.tmpl.Execute(&buf, r); err != nil {
		return "", fmt.Errorf("RenderHTML: %w", err)
	}
	return buf.String(), nil
}

// receiptHTMLTemplate is a minimal 80mm-width POS receipt.
const receiptHTMLTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<title>Receipt {{ .ReceiptID }}</title>
<style>
  body { font-family: monospace; font-size:12px; width:300px; margin:0 auto; }
  h1   { font-size:14px; text-align:center; margin:4px 0; }
  .sep { border-top:1px dashed #000; margin:4px 0; }
  .row { display:flex; justify-content:space-between; }
  .total { font-weight:bold; font-size:13px; }
</style>
</head>
<body>
<h1>{{ .StoreName }}</h1>
<p style="text-align:center">{{ fmtTime .IssuedAt }}</p>
<div class="sep"></div>
<p>Order: {{ .OrderID }}</p>
<p>Receipt: {{ .ReceiptID }}</p>
<div class="sep"></div>
{{ range .Items }}
<div class="row"><span>{{ .Name }} ({{ .SKU }})</span></div>
<div class="row"><span>&nbsp;&nbsp;{{ .Qty }} × {{ .UnitPrice }}</span><span>{{ .LineTotal }}</span></div>
{{ end }}
<div class="sep"></div>
<div class="row"><span>Subtotal</span><span>{{ .Subtotal }}</span></div>
{{ if .DiscountTotal.IsPositive }}<div class="row"><span>Discount</span><span>-{{ .DiscountTotal }}</span></div>{{ end }}
<div class="row"><span>VAT (5%)</span><span>{{ .VATAmount }}</span></div>
<div class="sep"></div>
<div class="row total"><span>TOTAL {{ .Currency }}</span><span>{{ .Total }}</span></div>
<div class="sep"></div>
<div class="row"><span>Paid ({{ .PaymentMethod }})</span><span>{{ .AmountPaid }}</span></div>
{{ if .Change.IsPositive }}<div class="row"><span>Change</span><span>{{ .Change }}</span></div>{{ end }}
<div class="sep"></div>
<p style="text-align:center;font-size:10px">Thank you for shopping with us!</p>
</body>
</html>`
