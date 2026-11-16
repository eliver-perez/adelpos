package legacyimport

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"codeberg.org/go-pdf/fpdf"
	"github.com/jackc/pgx/v5"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"local/ad-el-pos/server/internal/moneytext"
)

// Informe final en PDF (con codeberg.org/go-pdf/fpdf y las fuentes Go en UTF-8, como las exportaciones de
// reportes): resumen del lote, validación §4.4 y la lista de datos incoherentes (folio original, fecha, marca e
// importes). Esa lista sale de las tablas del historial del lote y solo existe en este PDF y en la base: nunca en
// bitácoras ni en las respuestas JSON.

const (
	pdfFamily = "go"
	pdfMargin = 12.0
	// pdfMaxIncoherent acota la lista detallada (el respaldo real tiene unos cientos de casos).
	pdfMaxIncoherent = 20000
)

// File es un archivo para descargar.
type File struct {
	FileName    string
	ContentType string
	Content     []byte
}

// incoherentRow es un renglón de la lista de datos incoherentes.
type incoherentRow struct {
	entity  string
	key     string
	at      *time.Time
	flags   []string
	amounts string
}

// RenderReport arma el PDF del informe de un lote.
func RenderReport(ctx context.Context, tx pgx.Tx, batchID string) (*File, error) {
	var status, timezone, username, fingerprint string
	var startedAt time.Time
	var completedAt, backupTakenAt *time.Time
	var reportJSON []byte
	var establishment string
	err := tx.QueryRow(ctx, `SELECT b.status, b.timezone, b.imported_by_username, b.package_fingerprint, b.started_at,
            b.completed_at, b.backup_taken_at, b.report, coalesce((SELECT name FROM app.establishment), '')
        FROM app.legacy_import_batches b WHERE b.id = $1`, batchID).Scan(&status, &timezone, &username, &fingerprint,
		&startedAt, &completedAt, &backupTakenAt, &reportJSON, &establishment)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, &Error{Code: CodeUploadMissing, Message: "El lote no existe."}
	}
	if err != nil {
		return nil, err
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		location = time.UTC
	}
	var report Report
	if len(reportJSON) > 0 {
		if err := json.Unmarshal(reportJSON, &report); err != nil {
			return nil, err
		}
	}
	rows := []incoherentRow{}
	truncated := false
	if status == "completed" {
		rows, truncated, err = incoherentRows(ctx, tx, batchID)
		if err != nil {
			return nil, err
		}
	}
	document := newReportPDF()
	writer := &pdfWriter{pdf: document, location: location}
	writer.title(establishment)
	writer.field("Lote", batchID)
	writer.field("Estado", map[string]string{"completed": "Vigente", "replaced": "Reemplazado", "importing": "En curso"}[status])
	writer.field("Importado", startedAt.In(location).Format("02/01/2006 15:04")+" por "+username)
	if backupTakenAt != nil {
		writer.field("Fecha del respaldo", backupTakenAt.In(location).Format("02/01/2006 15:04"))
	}
	writer.field("Huella del paquete", fingerprint)
	if report.BackupFile != "" {
		writer.field("Respaldo previo de AD-EL POS", report.BackupFile)
	}
	if report.SalesPeriod.Label != "" {
		writer.field("Ventas", report.SalesPeriod.Label)
	}
	writer.section("Validación posterior")
	if report.Validation.OK {
		writer.paragraph("Todo cuadra: lo importado coincide con el paquete.")
	} else {
		writer.warning("Hay diferencias entre lo importado y el paquete. Revisa los renglones marcados.")
	}
	writer.validation(report.Validation)
	for _, note := range report.Validation.Notes {
		writer.paragraph("• " + note)
	}
	writer.section("Datos incoherentes del sistema anterior")
	writer.paragraph("Se importaron tal como estaban, con su marca; no se corrigieron.")
	writer.flagCounts(report.Flags)
	if len(report.Notes) > 0 {
		writer.section("Catálogo, mesas y usuarios")
		for _, note := range report.Notes {
			writer.paragraph("• " + note.Message)
		}
	}
	if len(report.Warnings) > 0 {
		writer.section("Avisos")
		for _, warning := range report.Warnings {
			writer.paragraph("• " + warning)
		}
	}
	writer.section("Lista de datos incoherentes")
	if status != "completed" {
		writer.paragraph("Este lote ya se reemplazó: sus filas del historial se borraron y la lista detallada ya no está disponible.")
	} else if len(rows) == 0 {
		writer.paragraph("Ningún dato incoherente.")
	} else {
		writer.incoherent(rows)
		if truncated {
			writer.warning(fmt.Sprintf("La lista se cortó en %d renglones.", pdfMaxIncoherent))
		}
	}
	var output bytes.Buffer
	if err := document.Output(&output); err != nil {
		return nil, err
	}
	name := "migracion-sistema-anterior-" + startedAt.In(location).Format("20060102-1504") + ".pdf"
	return &File{FileName: name, ContentType: "application/pdf", Content: output.Bytes()}, nil
}

// listedFlags son las marcas de cada entidad que van renglón por renglón en la lista.
func listedFlags(entity string) []string {
	flags := []string{}
	for _, info := range Flags {
		if info.Entity == entity && info.Listed {
			flags = append(flags, info.Code)
		}
	}
	return flags
}

// incoherentRows lee de las tablas del lote las filas con marcas de la lista.
func incoherentRows(ctx context.Context, tx pgx.Tx, batchID string) ([]incoherentRow, bool, error) {
	queries := []struct {
		entity string
		sql    string
	}{
		{EntitySale, `SELECT 'Folio ' || folio, opened_at, flags,
                'Total ' || total::text || ' · debe ' || amount_due::text || ' · renglones ' || lines_amount::text ||
                ' · pagos ' || payments_amount::text
            FROM app.legacy_sales WHERE batch_id = $1 AND flags && $2 ORDER BY opened_at, legacy_id`},
		{EntityPayment, `SELECT 'Pago ' || p.legacy_id || ' · folio ' || coalesce(s.folio, '?'), p.paid_at, p.flags,
                p.method_name || ' ' || p.amount::text
            FROM app.legacy_payments p LEFT JOIN app.legacy_sales s ON s.batch_id = p.batch_id AND s.legacy_id = p.legacy_sale_id
            WHERE p.batch_id = $1 AND p.flags && $2 ORDER BY p.paid_at, p.legacy_id`},
		{EntityDiscount, `SELECT 'Descuento · folio ' || coalesce(s.folio, '?'), s.opened_at, d.flags,
                'Descuento ' || d.amount::text || ' · total ' || coalesce(s.total::text, '?')
            FROM app.legacy_discounts d LEFT JOIN app.legacy_sales s ON s.batch_id = d.batch_id AND s.legacy_id = d.legacy_sale_id
            WHERE d.batch_id = $1 AND d.flags && $2 ORDER BY d.position`},
		{EntityLine, `SELECT 'Renglón ' || l.legacy_id || ' · folio ' || coalesce(s.folio, '?'), s.opened_at, l.flags,
                'Total ' || l.total::text
            FROM app.legacy_sale_lines l LEFT JOIN app.legacy_sales s ON s.batch_id = l.batch_id AND s.legacy_id = l.legacy_sale_id
            WHERE l.batch_id = $1 AND l.flags && $2 ORDER BY l.legacy_id`},
		{EntityComplement, `SELECT 'Complemento del renglón ' || c.legacy_line_id, NULL::timestamptz, c.flags,
                'Total ' || c.total::text
            FROM app.legacy_line_complements c WHERE c.batch_id = $1 AND c.flags && $2 ORDER BY c.position`},
		{EntityShift, `SELECT 'Corte ' || legacy_id || coalesce(' · ' || register_name, ''), opened_at, flags,
                'Venta guardada ' || coalesce(sales_amount::text, '-') || ' · retiros ' || coalesce(withdrawals_amount::text, '-')
            FROM app.legacy_cash_shifts WHERE batch_id = $1 AND flags && $2 ORDER BY legacy_id`},
		{EntityWithdrawal, `SELECT 'Retiro ' || legacy_id || coalesce(' · corte ' || legacy_shift_id, ''), withdrawn_at, flags,
                'Importe ' || amount::text
            FROM app.legacy_cash_withdrawals WHERE batch_id = $1 AND flags && $2 ORDER BY legacy_id`},
	}
	result := []incoherentRow{}
	for _, query := range queries {
		rows, err := tx.Query(ctx, query.sql, batchID, listedFlags(query.entity))
		if err != nil {
			return nil, false, err
		}
		for rows.Next() {
			row := incoherentRow{entity: query.entity}
			if err := rows.Scan(&row.key, &row.at, &row.flags, &row.amounts); err != nil {
				rows.Close()
				return nil, false, err
			}
			if len(result) >= pdfMaxIncoherent {
				rows.Close()
				return result, true, nil
			}
			result = append(result, row)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, false, err
		}
	}
	return result, false, nil
}

func newReportPDF() *fpdf.Fpdf {
	document := fpdf.New("P", "mm", "Letter", "")
	document.AddUTF8FontFromBytes(pdfFamily, "", goregular.TTF)
	document.AddUTF8FontFromBytes(pdfFamily, "B", gobold.TTF)
	document.SetMargins(pdfMargin, pdfMargin, pdfMargin)
	document.SetAutoPageBreak(true, 14)
	document.AliasNbPages("{nb}")
	document.SetFooterFunc(func() {
		document.SetY(-10)
		document.SetFont(pdfFamily, "", 7)
		document.SetTextColor(88, 98, 110)
		document.CellFormat(0, 4, "Página "+strconv.Itoa(document.PageNo())+" de {nb}", "", 0, "R", false, 0, "")
	})
	document.AddPage()
	return document
}

type pdfWriter struct {
	pdf      *fpdf.Fpdf
	location *time.Location
}

func (writer *pdfWriter) width() float64 {
	page, _ := writer.pdf.GetPageSize()
	return page - 2*pdfMargin
}

func (writer *pdfWriter) title(establishment string) {
	writer.pdf.SetFont(pdfFamily, "B", 15)
	writer.pdf.SetTextColor(28, 34, 42)
	writer.pdf.MultiCell(writer.width(), 7, "Migración del sistema anterior", "", "L", false)
	if establishment != "" {
		writer.pdf.SetFont(pdfFamily, "", 9)
		writer.pdf.MultiCell(writer.width(), 5, establishment, "", "L", false)
	}
	writer.pdf.Ln(2)
}

func (writer *pdfWriter) field(label, value string) {
	writer.pdf.SetFont(pdfFamily, "B", 8)
	writer.pdf.CellFormat(45, 4.4, label, "", 0, "L", false, 0, "")
	writer.pdf.SetFont(pdfFamily, "", 8)
	writer.pdf.MultiCell(writer.width()-45, 4.4, value, "", "L", false)
}

func (writer *pdfWriter) section(title string) {
	writer.pdf.Ln(3)
	writer.pdf.SetFont(pdfFamily, "B", 11)
	writer.pdf.SetTextColor(28, 34, 42)
	writer.pdf.MultiCell(writer.width(), 6, title, "", "L", false)
}

func (writer *pdfWriter) paragraph(text string) {
	writer.pdf.SetFont(pdfFamily, "", 8)
	writer.pdf.MultiCell(writer.width(), 4.2, text, "", "L", false)
}

func (writer *pdfWriter) warning(text string) {
	writer.pdf.SetFont(pdfFamily, "B", 8.5)
	writer.pdf.SetFillColor(253, 243, 222)
	writer.pdf.MultiCell(writer.width(), 5, text, "", "L", true)
}

// pesos escribe un importe exacto del historial («1234.5000») como pesos («$1,234.50»).
func pesos(text string) string {
	value, err := ParseDisplayMoney(text)
	if err != nil {
		return text
	}
	return moneytext.Format("MXN", value.Display())
}

func (writer *pdfWriter) row(widths []float64, cells []string, bold, fill bool) {
	style := ""
	if bold {
		style = "B"
	}
	writer.pdf.SetFont(pdfFamily, style, 7.5)
	if fill {
		writer.pdf.SetFillColor(253, 243, 222)
	} else {
		writer.pdf.SetFillColor(255, 255, 255)
	}
	for index, cell := range cells {
		align := "R"
		if index == 0 {
			align = "L"
		}
		writer.pdf.CellFormat(widths[index], 4.4, cell, "B", 0, align, fill, 0, "")
	}
	writer.pdf.Ln(-1)
}

func (writer *pdfWriter) validation(validation Validation) {
	total := writer.width()
	widths := []float64{total * 0.43, total * 0.19, total * 0.19, total * 0.19}
	writer.row(widths, []string{"Concepto", "Paquete", "Importado", "Diferencia"}, true, false)
	for _, check := range validation.Checks {
		expected, actual, difference := check.Expected, check.Actual, check.Difference
		if check.Kind == KindMoney {
			expected, actual, difference = pesos(expected), pesos(actual), pesos(difference)
		}
		writer.row(widths, []string{check.Label, expected, actual, difference}, false, !check.OK)
	}
}

func (writer *pdfWriter) flagCounts(flags []FlagCount) {
	if len(flags) == 0 {
		writer.paragraph("Ninguna marca.")
		return
	}
	total := writer.width()
	widths := []float64{total * 0.85, total * 0.15}
	writer.row(widths, []string{"Marca", "Filas"}, true, false)
	for _, flag := range flags {
		writer.row(widths, []string{flag.Label, strconv.FormatInt(flag.Count, 10)}, false, false)
	}
}

func (writer *pdfWriter) incoherent(rows []incoherentRow) {
	total := writer.width()
	widths := []float64{total * 0.24, total * 0.14, total * 0.34, total * 0.28}
	writer.pdf.SetFont(pdfFamily, "B", 7.5)
	headers := []string{"Registro", "Fecha", "Marca", "Importes"}
	for index, header := range headers {
		writer.pdf.CellFormat(widths[index], 4.4, header, "B", 0, "L", false, 0, "")
	}
	writer.pdf.Ln(-1)
	writer.pdf.SetFont(pdfFamily, "", 7)
	for _, row := range rows {
		date := ""
		if row.at != nil {
			date = row.at.In(writer.location).Format("02/01/2006 15:04")
		}
		labels := []string{}
		for _, flag := range row.flags {
			if info, found := flagInfo(row.entity, flag); found && info.Listed {
				labels = append(labels, info.Label)
			}
		}
		cells := []string{row.key, date, strings.Join(labels, "; "), row.amounts}
		height := 3.8 * float64(writer.lines(cells, widths))
		if writer.pdf.GetY()+height > 264 {
			writer.pdf.AddPage()
		}
		x, y := writer.pdf.GetXY()
		for index, cell := range cells {
			writer.pdf.SetXY(x, y)
			writer.pdf.MultiCell(widths[index], 3.8, cell, "", "L", false)
			x += widths[index]
		}
		writer.pdf.SetXY(pdfMargin, y+height)
		writer.pdf.Line(pdfMargin, y+height, pdfMargin+total, y+height)
	}
}

// lines es cuántas líneas ocupa el renglón más alto de una fila.
func (writer *pdfWriter) lines(cells []string, widths []float64) int {
	maximum := 1
	for index, cell := range cells {
		count := len(writer.pdf.SplitText(cell, widths[index]-2))
		if count > maximum {
			maximum = count
		}
	}
	return maximum
}
