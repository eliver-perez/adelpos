package payments

import (
	"strings"

	"local/ad-el-pos/server/internal/documents"
)

// Envolturas finas que conservan, para las pruebas anteriores a la etapa 9B, los nombres de las
// plantillas de texto que reemplazó el renderizador único. Renderizan el documento completo con la
// plantilla predeterminada (la versión 1 del sistema) y el ancho en caracteres indicado.

// renderDocument renderiza una precuenta o ticket con la plantilla predeterminada.
func renderDocument(document *CommercialDocument, width int) string {
	kind := documents.Kind(document.Kind)
	data := CommercialData(document)
	layout, err := documents.Render(kind, documents.Default(kind), documents.Data{Commercial: &data}, documents.RenderOptions{
		PaperWidth: paperForColumns(width), Columns: width,
	})
	if err != nil {
		panic(err)
	}
	return layout.Text()
}

// cashWithDollars renderiza un ticket con el pago indicado y devuelve sus líneas (incluyen el
// detalle de efectivo con dólares del bloque «Este pago»).
func cashWithDollars(payment DocumentPayment, width int) []string {
	copied := payment
	document := &CommercialDocument{
		Kind: "receipt", Total: payment.Amount, Paid: payment.Amount, Tips: payment.Tip, Balance: "0.00", Payment: &copied,
	}
	return strings.Split(renderDocument(document, width), "\n")
}

func paperForColumns(width int) int {
	if width <= 30 {
		return 58
	}
	return 80
}

// documentLines acumula líneas con un ancho en caracteres, como la plantilla anterior de caja.
type documentLines struct {
	width int
	lines []string
}

func (document *documentLines) text(value string) {
	document.lines = append(document.lines, legacyWrapWords(value, document.width)...)
}

func (document *documentLines) String() string {
	return strings.Join(document.lines, "\n")
}

// renderDefaultCash renderiza un comprobante o corte completo con la plantilla predeterminada.
func renderDefaultCash(header *documentHeader, source cashDocumentSource, includeExpected bool, width int) []string {
	kind, _ := documents.CashKind(source.kind)
	data := cashData(header, source, includeExpected)
	layout, err := documents.Render(kind, documents.Default(kind), documents.Data{Cash: &data}, documents.RenderOptions{
		PaperWidth: paperForColumns(width), Columns: width, IncludeExpected: includeExpected,
	})
	if err != nil {
		panic(err)
	}
	return strings.Split(layout.Text(), "\n")
}

// renderReport agrega las líneas de un corte renderizado con la plantilla predeterminada.
func renderReport(document *documentLines, header *documentHeader, source cashDocumentSource, includeExpected bool) {
	document.lines = append(document.lines, renderDefaultCash(header, source, includeExpected, document.width)...)
}

// renderVoucher agrega las líneas de un comprobante renderizado con la plantilla predeterminada.
func renderVoucher(document *documentLines, header *documentHeader, source cashDocumentSource) {
	document.lines = append(document.lines, renderDefaultCash(header, source, false, document.width)...)
}
