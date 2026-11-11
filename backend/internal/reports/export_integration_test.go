//go:build integration

package reports_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
	"github.com/xuri/excelize/v2"
	"local/ad-el-pos/server/internal/moneytext"
	"local/ad-el-pos/server/internal/payments"
	"local/ad-el-pos/server/internal/reports"
)

// Exportación PDF y Excel por la API real (fase C1): misma consulta, filtros, orden y totales que
// reports/run; tipo y nombre del archivo; permisos de exportación por formato (403 auditado);
// columnas protegidas ausentes del archivo aunque se llame a la API directamente (efectivo físico
// sin cash.expected.read y, para todo reporte registrado, las columnas cuyo permiso falte); datos
// históricos (motivo renombrado); textos de usuarios que parecen fórmulas; archivo grande; límite
// de 50 000 renglones (422 sin archivo ni auditoría) y auditoría report.exported sin datos
// personales.
//
// La prueba usa su propia base desde cero (borra y vuelve a crear el esquema) y la deja vacía al
// terminar, para no alterar las demás pruebas del paquete, que esperan una base nueva.

type exportResponse struct {
	status int
	header http.Header
	body   []byte
}

func (fixture *reportsFixture) export(t *testing.T, member terminal, id, query string) exportResponse {
	t.Helper()
	request := httptest.NewRequest("GET", "/api/v1/reports/export/"+id+"?"+query, nil)
	request.RemoteAddr = "127.0.0.1:52000"
	request.Header.Set("X-Device-ID", member.deviceID)
	request.Header.Set("X-Device-Token", member.deviceToken)
	request.Header.Set("Authorization", "Bearer "+member.sessionToken)
	response := httptest.NewRecorder()
	fixture.service.ServeHTTP(response, request)
	return exportResponse{status: response.Code, header: response.Header(), body: append([]byte{}, response.Body.Bytes()...)}
}

func (fixture *reportsFixture) exportOK(t *testing.T, member terminal, id, query string) exportResponse {
	t.Helper()
	response := fixture.export(t, member, id, query)
	if response.status != 200 {
		t.Fatalf("Exportar %s?%s: %d %s", id, query, response.status, response.body)
	}
	return response
}

// screen es el resultado JSON de reports/run con las etiquetas de columnas y cifras.
type screen struct {
	Report struct {
		Title       string `json:"title"`
		Description string `json:"description"`
	} `json:"report"`
	Meta struct {
		DateBasis string   `json:"date_basis"`
		Notes     []string `json:"notes"`
		Warnings  []struct {
			Message string `json:"message"`
		} `json:"warnings"`
	} `json:"meta"`
	Columns []struct {
		Key   string `json:"key"`
		Label string `json:"label"`
		Type  string `json:"type"`
	} `json:"columns"`
	Rows    []map[string]any `json:"rows"`
	Totals  map[string]any   `json:"totals"`
	Summary []struct {
		Label string `json:"label"`
	} `json:"summary"`
	Page struct {
		TotalRows int `json:"total_rows"`
	} `json:"page"`
}

func (fixture *reportsFixture) screen(t *testing.T, member terminal, id, query string) screen {
	t.Helper()
	route := "reports/run/" + id + "?page_size=200"
	if query != "" {
		route += "&" + query
	}
	return decode[screen](t, fixture.get(t, member, route, 200))
}

func workbook(t *testing.T, content []byte) *excelize.File {
	t.Helper()
	file, err := excelize.OpenReader(bytes.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = file.Close() })
	return file
}

func sheetRows(t *testing.T, file *excelize.File, sheet string, raw bool) [][]string {
	t.Helper()
	rows, err := file.GetRows(sheet, excelize.Options{RawCellValue: raw})
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

// workbookText concatena todas las partes del libro descomprimido.
func workbookText(t *testing.T, content []byte) string {
	t.Helper()
	archive, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		t.Fatal(err)
	}
	var builder strings.Builder
	for _, entry := range archive.File {
		reader, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, _ := io.ReadAll(reader)
		_ = reader.Close()
		builder.Write(data)
	}
	return builder.String()
}

// summaryLabels son las etiquetas de la sección «Resumen» de la hoja de filtros.
func summaryLabels(t *testing.T, file *excelize.File) []string {
	t.Helper()
	labels := []string{}
	inside := false
	for _, row := range sheetRows(t, file, "Filtros", false) {
		if len(row) == 0 {
			continue
		}
		switch row[0] {
		case "Filtros aplicados", "Resumen", "Avisos", "Notas", "Precisión":
			inside = row[0] == "Resumen"
			continue
		}
		if inside {
			labels = append(labels, row[0])
		}
	}
	return labels
}

// assertSameAsScreen comprueba que el libro tiene exactamente las columnas, los renglones, los
// totales y las cifras de la pantalla.
func assertSameAsScreen(t *testing.T, id string, result screen, content []byte) {
	t.Helper()
	file := workbook(t, content)
	rows := sheetRows(t, file, "Datos", true)
	labels := []string{}
	for _, column := range result.Columns {
		labels = append(labels, column.Label)
	}
	if len(rows) == 0 || !slices.Equal(rows[0], labels) {
		t.Fatalf("%s: columnas del libro %v ≠ pantalla %v", id, rows, labels)
	}
	expectedRows := 1 + result.Page.TotalRows
	if result.Page.TotalRows > 0 {
		expectedRows++
	}
	if len(rows) != expectedRows {
		t.Fatalf("%s: renglones del libro %d ≠ pantalla %d (+ encabezado y totales)", id, len(rows), result.Page.TotalRows)
	}
	if result.Page.TotalRows > 0 {
		totals := rows[len(rows)-1]
		for index, column := range result.Columns {
			value, found := result.Totals[column.Key]
			if !found || value == nil {
				continue
			}
			cell := ""
			if index < len(totals) {
				cell = totals[index]
			}
			expected := fmt.Sprint(value)
			got, err := decimal.NewFromString(cell)
			want, wantErr := decimal.NewFromString(expected)
			if column.Type == "percent" {
				want = want.Shift(-2)
			}
			if err != nil || wantErr != nil || !got.Equal(want) {
				t.Fatalf("%s: total de %s en el libro %q ≠ pantalla %q", id, column.Key, cell, expected)
			}
		}
	}
	screenSummary := []string{}
	for _, figure := range result.Summary {
		screenSummary = append(screenSummary, figure.Label)
	}
	if got := summaryLabels(t, file); !slices.Equal(got, screenSummary) {
		t.Fatalf("%s: cifras del libro %v ≠ pantalla %v", id, got, screenSummary)
	}
}

func dropReportsSchema(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, "DROP SCHEMA IF EXISTS app CASCADE; DROP TABLE IF EXISTS public.schema_migrations")
	return err
}

func TestReportExports(t *testing.T) {
	ctx, pool := openPool(t)
	if err := dropReportsSchema(ctx, pool); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := dropReportsSchema(context.Background(), pool); err != nil {
			t.Error(err)
		}
	})
	fixture := newReportsFixture(t)
	fixture.seedCatalog(t)
	shift := decode[payments.Shift](t, fixture.mutate(t, "POST", "payments/shifts", map[string]any{"opening_float": "500"}, 200))
	fixture.shiftID = shift.ID
	cash, debit := fixture.methods["cash"], fixture.methods["debit"]
	first := fixture.account(t, "Mesa uno", "dine_in", &fixture.table, line(fixture.burger, 2), line(fixture.soda, 1))
	fixture.pay(t, first.Account.ID, map[string]any{"method_id": cash, "mode": "total", "received": "300"})
	// La referencia de un pago con tarjeta la escribe una persona: aquí parece una fórmula de Excel.
	injection := `=HYPERLINK("http://example.invalid/robo","Abrir")`
	second := fixture.account(t, "Mesa dos", "dine_in", &fixture.secondTable, line(fixture.salad, 1), line(fixture.soda, 2))
	fixture.pay(t, second.Account.ID, map[string]any{"method_id": debit, "mode": "total", "reference": injection})
	// Descuento con motivo del catálogo que después se renombra (dato histórico).
	reasons := decode[[]struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Revision int    `json:"revision"`
	}](t, fixture.get(t, fixture.admin, "payments/discount-reasons", 200))
	var attention string
	var revision int
	for _, reason := range reasons {
		if reason.Name == "Atención" {
			attention, revision = reason.ID, reason.Revision
		}
	}
	discounted := fixture.account(t, "Con descuentos", "dine_in", &fixture.table, line(fixture.burger, 2))
	burger := fixture.versionOf(t, discounted.Account.ID, fixture.burger)
	fixture.post(t, discounted.Account.ID, "discounts", map[string]any{
		"scope": "items", "method": "percent", "value": "10", "reason_id": attention, "reason": "",
		"selections": []map[string]any{{"version_id": burger, "quantity": 1}},
	}, 200)
	fixture.post(t, discounted.Account.ID, "discounts", map[string]any{"scope": "account", "method": "amount", "value": "20",
		"reason": "Ajuste"}, 200)
	fixture.pay(t, discounted.Account.ID, map[string]any{"method_id": debit, "mode": "total"})
	fixture.call(t, fixture.admin, "PATCH", "payments/discount-reasons/"+attention, map[string]any{
		"operation_id": randomID(), "name": "Atención renombrada", "requires_description": false, "active": true, "revision": revision,
	}, 200)
	// Pesos y dólares físicos en el mismo cobro.
	features := decode[[]struct {
		Code     string `json:"code"`
		Revision int    `json:"revision"`
	}](t, fixture.get(t, fixture.admin, "settings/features", 200))
	for _, feature := range features {
		if feature.Code == "usd_cash" {
			fixture.call(t, fixture.admin, "PUT", "settings/features", map[string]any{"code": "usd_cash", "enabled": true, "revision": feature.Revision}, 200)
		}
	}
	fixture.call(t, fixture.admin, "POST", "payments/shifts/"+fixture.shiftID+"/usd-rate", map[string]any{
		"operation_id": randomID(), "usd_rate": "18.50",
	}, 200)
	dollars := fixture.account(t, "Pesos y dólares", "dine_in", &fixture.table, line(fixture.burger, 2), line(fixture.soda, 2))
	fixture.pay(t, dollars.Account.ID, map[string]any{"method_id": cash, "mode": "total", "received": "100", "received_usd": "20"})

	exportCount := func() int {
		count, _ := strconv.Atoi(fixture.scalar(t, "SELECT count(*)::text FROM app.audit_events WHERE action='report.exported'"))
		return count
	}

	t.Run("PDF y Excel iguales a la pantalla, con tipo y nombre del archivo", func(t *testing.T) {
		result := fixture.screen(t, fixture.admin, "sales.by_product", "")
		if result.Page.TotalRows < 3 {
			t.Fatalf("Ventas por producto: %+v", result.Rows)
		}
		pdf := fixture.exportOK(t, fixture.admin, "sales.by_product", "format=pdf")
		if pdf.header.Get("Content-Type") != reports.ContentTypePDF || !bytes.HasPrefix(pdf.body, []byte("%PDF-")) ||
			pdf.header.Get("Content-Length") != strconv.Itoa(len(pdf.body)) {
			t.Fatalf("Respuesta PDF: %v", pdf.header)
		}
		location, err := time.LoadLocation("America/Monterrey")
		if err != nil {
			t.Fatal(err)
		}
		// Día comercial de hoy con la hora de inicio por omisión (05:00) en la zona del establecimiento.
		today := time.Now().In(location).Add(-5 * time.Hour).Format("2006-01-02")
		if disposition := pdf.header.Get("Content-Disposition"); disposition != "attachment; filename=ventas-por-producto_"+today+".pdf" {
			t.Fatalf("Nombre sugerido: %s", disposition)
		}
		text := strings.Join(reports.PDFPagesForTest(t, pdf.body), "\n")
		for _, expected := range []string{"Restaurante de reportes", "Ventas por producto", "Hamburguesa", "Refresco", "Ensalada",
			"Página 1 de 1", "Total"} {
			if !strings.Contains(text, expected) {
				t.Fatalf("El PDF no contiene «%s»", expected)
			}
		}
		for _, column := range result.Columns {
			if value, ok := result.Totals[column.Key].(string); ok && column.Type == "money_mxn" {
				if formatted := moneytext.Format("MXN", value); !strings.Contains(text, formatted) {
					t.Fatalf("El PDF no tiene el total de %s de la pantalla (%s)", column.Label, formatted)
				}
			}
		}
		xlsx := fixture.exportOK(t, fixture.admin, "sales.by_product", "format=xlsx")
		if xlsx.header.Get("Content-Type") != reports.ContentTypeXLSX || !bytes.HasPrefix(xlsx.body, []byte("PK\x03\x04")) ||
			!strings.HasSuffix(xlsx.header.Get("Content-Disposition"), ".xlsx") {
			t.Fatalf("Respuesta XLSX: %v", xlsx.header)
		}
		assertSameAsScreen(t, "sales.by_product", result, xlsx.body)
		if strings.Contains(workbookText(t, xlsx.body), "<f>") {
			t.Fatal("El libro contiene fórmulas")
		}
	})

	t.Run("filtros, orden, comparación y papel", func(t *testing.T) {
		result := fixture.screen(t, fixture.admin, "payments.list", "method_id="+cash+"&sort=amount&direction=desc")
		xlsx := fixture.exportOK(t, fixture.admin, "payments.list", "format=xlsx&method_id="+cash+"&sort=amount&direction=desc")
		assertSameAsScreen(t, "payments.list", result, xlsx.body)
		file := workbook(t, xlsx.body)
		rows := sheetRows(t, file, "Datos", true)
		for index, row := range result.Rows {
			amount := decimal.RequireFromString(fmt.Sprint(row["amount"]))
			if got := decimal.RequireFromString(rows[index+1][4]); !got.Equal(amount) {
				t.Fatalf("El orden del libro no es el de la pantalla: renglón %d %s ≠ %s", index+1, got, amount)
			}
		}
		info := map[string]string{}
		for _, row := range sheetRows(t, file, "Filtros", false) {
			if len(row) > 1 {
				info[row[0]] = row[1]
			}
		}
		if info["Método de pago"] != "Efectivo" || info["Orden"] != "Importe, descendente" {
			t.Fatalf("Hoja de filtros: %+v", info)
		}
		if response := fixture.export(t, fixture.admin, "payments.by_method", "format=xlsx&sort=no_existe"); response.status != 400 {
			t.Fatalf("Un orden por una columna inexistente debe rechazarse como en la pantalla: %d", response.status)
		}
		evolution := fixture.exportOK(t, fixture.admin, "sales.evolution", "format=pdf&compare=previous&period=current_month&paper=a4")
		text := strings.Join(reports.PDFPagesForTest(t, evolution.body), "\n")
		if !strings.Contains(text, "Comparación") || !bytes.Contains(evolution.body, []byte("/MediaBox [0 0 595.28 841.89]")) {
			t.Fatal("La comparación y el papel A4 deben respetarse")
		}
		for query, code := range map[string]string{
			"format=csv": "VALIDATION_ERROR", "format=pdf&page=2": "VALIDATION_ERROR", "format=xlsx&paper=a4": "VALIDATION_ERROR",
			"format=pdf&no_existe=1": "VALIDATION_ERROR", "format=pdf&period=month&month=2026-13": "VALIDATION_ERROR",
		} {
			response := fixture.export(t, fixture.admin, "sales.by_day", query)
			if response.status != 400 || !strings.Contains(string(response.body), code) {
				t.Fatalf("%s: %d %s", query, response.status, response.body)
			}
		}
		if response := fixture.export(t, fixture.admin, "no.existe", "format=pdf"); response.status != 404 {
			t.Fatalf("Reporte inexistente: %d", response.status)
		}
	})

	t.Run("datos históricos y textos que parecen fórmulas", func(t *testing.T) {
		pdf := fixture.exportOK(t, fixture.admin, "discounts.by_reason", "format=pdf")
		text := strings.Join(reports.PDFPagesForTest(t, pdf.body), "\n")
		if !strings.Contains(text, "Atención") || strings.Contains(text, "Atención renombrada") {
			t.Fatal("El PDF debe mostrar el motivo histórico, no el nombre actual")
		}
		xlsx := fixture.exportOK(t, fixture.admin, "payments.list", "format=xlsx")
		file := workbook(t, xlsx.body)
		found := false
		for _, row := range sheetRows(t, file, "Datos", true) {
			for _, cell := range row {
				if strings.Contains(cell, "HYPERLINK") {
					found = true
					if cell != "'"+injection {
						t.Fatalf("La referencia que parece fórmula debe llevar apóstrofo: %q", cell)
					}
				}
			}
		}
		content := workbookText(t, xlsx.body)
		if !found {
			t.Fatalf("La referencia del pago no aparece en el libro: %v", sheetRows(t, file, "Datos", true))
		}
		if strings.Contains(content, "<f>") || strings.Contains(content, "<hyperlink") || strings.Contains(content, "TargetMode") {
			t.Fatal("El libro no debe contener fórmulas ni hipervínculos")
		}
	})

	t.Run("monedas separadas", func(t *testing.T) {
		result := fixture.screen(t, fixture.admin, "payments.currency", "group_by=shift")
		xlsx := fixture.exportOK(t, fixture.admin, "payments.currency", "format=xlsx&group_by=shift")
		assertSameAsScreen(t, "payments.currency", result, xlsx.body)
		file := workbook(t, xlsx.body)
		header := sheetRows(t, file, "Datos", true)[0]
		for index, label := range header {
			column := result.Columns[index]
			style, _ := file.GetCellStyle("Datos", fmt.Sprintf("%c2", 'A'+index))
			definition, _ := file.GetStyle(style)
			format := ""
			if definition.CustomNumFmt != nil {
				format = *definition.CustomNumFmt
			}
			if column.Type == "money_usd" && !strings.Contains(format, "US$") || column.Type == "money_mxn" && strings.Contains(format, "US$") {
				t.Fatalf("%s (%s) con formato %s", label, column.Type, format)
			}
		}
		pdf := fixture.exportOK(t, fixture.admin, "payments.currency", "format=pdf&group_by=shift")
		text := strings.Join(reports.PDFPagesForTest(t, pdf.body), "\n")
		if !strings.Contains(text, "US$20.00") || !strings.Contains(text, "Dólares") {
			t.Fatal("Los dólares deben ir en su propia columna con US$")
		}
	})

	t.Run("permisos de exportación por formato, auditados", func(t *testing.T) {
		viewer := fixture.staff(t, "sinexportar", reports.PermissionRead, reports.PermissionSales, reports.PermissionPayments)
		excel := fixture.staff(t, "soloexcel", reports.PermissionRead, reports.PermissionSales, reports.PermissionExportExcel)
		denials := func() string {
			return fixture.scalar(t, "SELECT count(*)::text FROM app.audit_events WHERE action='access.denied'")
		}
		before := denials()
		for _, format := range []string{"pdf", "xlsx"} {
			if response := fixture.export(t, viewer, "sales.by_day", "format="+format); response.status != 403 {
				t.Fatalf("Sin reports.export.* la exportación %s debe responder 403: %d", format, response.status)
			}
		}
		if response := fixture.export(t, excel, "sales.by_day", "format=pdf"); response.status != 403 {
			t.Fatalf("Con solo Excel el PDF debe responder 403: %d", response.status)
		}
		if response := fixture.export(t, excel, "payments.by_method", "format=xlsx"); response.status != 403 {
			t.Fatalf("El permiso de exportar no da acceso a reportes sin su permiso: %d", response.status)
		}
		fixture.exportOK(t, excel, "sales.by_day", "format=xlsx")
		after, _ := strconv.Atoi(denials())
		previous, _ := strconv.Atoi(before)
		if after-previous != 4 {
			t.Fatalf("Cada denegación debe auditarse: %d", after-previous)
		}
	})

	t.Run("columnas protegidas fuera del archivo aunque se llame a la API", func(t *testing.T) {
		// Detalle de cobros sin cash.expected.read: ni lo recibido, ni el cambio, ni los dólares ni el
		// tipo de cambio (efectivo físico, paridad con Caja).
		// Con las consultas de Caja (todas las cajas e historial) para que el detalle tenga los cobros.
		cashier := fixture.staff(t, "cobrosinesperado", reports.PermissionRead, reports.PermissionPayments, reports.PermissionExportPDF,
			reports.PermissionExportExcel, "cash.shifts.read", "cash.shifts.read_all", "cash.history.read")
		forbidden := []string{"Recibido", "Cambio", "Dólares recibidos", "Tipo de cambio"}
		xlsx := fixture.exportOK(t, cashier, "payments.list", "format=xlsx")
		listed := fixture.screen(t, cashier, "payments.list", "")
		if listed.Page.TotalRows == 0 {
			t.Fatal("El detalle de cobros del cajero debía tener renglones")
		}
		assertSameAsScreen(t, "payments.list", listed, xlsx.body)
		content := workbookText(t, xlsx.body)
		pdf := fixture.exportOK(t, cashier, "payments.list", "format=pdf")
		text := strings.Join(reports.PDFPagesForTest(t, pdf.body), "\n")
		for _, label := range forbidden {
			if strings.Contains(content, label) || strings.Contains(text, label) {
				t.Fatalf("El archivo de un usuario sin cash.expected.read contiene «%s»", label)
			}
		}
		if strings.Contains(content, "18.5") || strings.Contains(text, "18.50") {
			t.Fatal("El archivo contiene el tipo de cambio de un cobro en efectivo")
		}
		if response := fixture.export(t, cashier, "payments.currency", "format=pdf"); response.status != 403 {
			t.Fatalf("Efectivo por moneda exige cash.expected.read también al exportar: %d", response.status)
		}
		// Todo reporte registrado: un usuario con exactamente los permisos del reporte (sin
		// cash.expected.read, inventory.costs.read, inventory.valuation.read ni inventory.stock.read
		// salvo que el reporte los exija) recibe en el archivo las mismas columnas y cifras que en la
		// pantalla, y ninguna columna protegida.
		users := map[string]terminal{}
		checked := 0
		for _, definition := range reports.Definitions() {
			base := append([]string{reports.PermissionRead}, definition.Permissions...)
			protected := []string{}
			for _, column := range definition.Columns {
				for _, permission := range column.Permissions {
					if !slices.Contains(base, permission) {
						protected = append(protected, column.Label)
						break
					}
				}
			}
			if len(protected) == 0 {
				continue
			}
			key := append([]string{}, base...)
			// Los reportes de caja y de cobros se limitan a los turnos que el usuario puede consultar en
			// Caja: con todas las cajas y el historial tienen renglones que comprobar.
			if definition.Group == "cash" || definition.Group == "payments" {
				key = append(key, "cash.shifts.read", "cash.shifts.read_all", "cash.history.read")
			}
			sort.Strings(key)
			member, found := users[strings.Join(key, ",")]
			if !found {
				member = fixture.staff(t, "limitado"+strconv.Itoa(len(users)+1),
					append(key, reports.PermissionExportPDF, reports.PermissionExportExcel)...)
				users[strings.Join(key, ",")] = member
			}
			status, body := fixture.request(member, "GET", "reports/run/"+definition.ID+"?page_size=200", nil)
			if status != 200 {
				t.Logf("%s necesita parámetros para consultarse (%d %s); se omite", definition.ID, status, body)
				continue
			}
			result := decode[screen](t, body)
			xlsx := fixture.exportOK(t, member, definition.ID, "format=xlsx")
			assertSameAsScreen(t, definition.ID, result, xlsx.body)
			pdf := fixture.exportOK(t, member, definition.ID, "format=pdf")
			text := strings.Join(reports.PDFPagesForTest(t, pdf.body), "\n")
			visible := map[string]bool{}
			visibleLabels := []string{}
			for _, column := range result.Columns {
				visible[column.Label] = true
				visibleLabels = append(visibleLabels, column.Label)
			}
			// Texto del PDF que no es la tabla: título, descripción, fecha usada, cifras, avisos y notas.
			visibleText := strings.Join(visibleLabels, " ") + " " + result.Report.Title + " " + result.Report.Description + " " +
				result.Meta.DateBasis + " " + strings.Join(result.Meta.Notes, " ")
			for _, figure := range result.Summary {
				visibleText += " " + figure.Label
			}
			for _, warning := range result.Meta.Warnings {
				visibleText += " " + warning.Message
			}
			for _, label := range protected {
				if visible[label] {
					t.Fatalf("%s: la pantalla muestra la columna protegida «%s»", definition.ID, label)
				}
				// Las palabras propias de la etiqueta protegida (no compartidas con una columna visible)
				// no aparecen en el PDF.
				for _, word := range strings.Fields(label) {
					if len([]rune(word)) > 5 && strings.Contains(text, word) && !strings.Contains(visibleText, word) {
						t.Fatalf("%s: el PDF contiene «%s» de la columna protegida «%s»", definition.ID, word, label)
					}
				}
			}
			checked++
		}
		t.Logf("Reportes con columnas protegidas comprobados en PDF y Excel: %d", checked)
		if checked == 0 {
			t.Fatal("Debe comprobarse al menos un reporte con columnas protegidas")
		}
	})

	t.Run("auditoría report.exported sin datos personales", func(t *testing.T) {
		before := exportCount()
		fixture.exportOK(t, fixture.admin, "payments.by_user", "format=xlsx&user_id="+fixture.adminID(t))
		if exportCount() != before+1 {
			t.Fatal("La exportación debe auditarse una vez")
		}
		var after json.RawMessage
		var actor, entity string
		if err := fixture.pool.QueryRow(fixture.ctx, `SELECT after_state,actor_username,entity_type FROM app.audit_events
            WHERE action='report.exported' ORDER BY occurred_at DESC,id DESC LIMIT 1`).Scan(&after, &actor, &entity); err != nil {
			t.Fatal(err)
		}
		var details map[string]any
		if err := json.Unmarshal(after, &details); err != nil {
			t.Fatal(err)
		}
		filters, _ := details["filters"].(map[string]any)
		if details["report"] != "payments.by_user" || details["format"] != "xlsx" || actor != "admin" || entity != "report" ||
			filters["user_id"] != fixture.adminID(t) || details["rows"] == nil || details["period"] == nil {
			t.Fatalf("Auditoría de la exportación: %s", after)
		}
		for _, personal := range []string{"Administrador", "display_name", "$", "Efectivo"} {
			if strings.Contains(string(after), personal) {
				t.Fatalf("La auditoría contiene «%s»: %s", personal, after)
			}
		}
	})

	t.Run("archivo grande y límite de 50 000 renglones", func(t *testing.T) {
		insert := func(count int, seed string) {
			t.Helper()
			if _, err := fixture.pool.Exec(fixture.ctx, `INSERT INTO app.restaurant_operations(id,actor_id,device_id,route,fingerprint,response)
                SELECT md5($2::text||n::text)::uuid,p.actor_id,p.device_id,'POST exportación','huella','{}'
                FROM generate_series(1,$1::int) n CROSS JOIN LATERAL (SELECT actor_id,device_id FROM app.payments LIMIT 1) p`,
				count, seed); err != nil {
				t.Fatal(err)
			}
			if _, err := fixture.pool.Exec(fixture.ctx, `INSERT INTO app.payments(id,account_id,shift_id,register_id,method_id,method_name,
                    is_cash,kind,amount,tip,actor_id,device_id,operation_id,created_at)
                SELECT gen_random_uuid(),p.account_id,p.shift_id,p.register_id,p.method_id,p.method_name,false,'general',1.25,0,p.actor_id,
                    p.device_id,md5($2::text||n::text)::uuid,now() - make_interval(secs => (n % 1000) / 1000.0)
                FROM generate_series(1,$1::int) n CROSS JOIN LATERAL (SELECT * FROM app.payments WHERE NOT is_cash LIMIT 1) p`,
				count, seed); err != nil {
				t.Fatal(err)
			}
		}
		insert(3000, "grande")
		result := fixture.screen(t, fixture.admin, "payments.list", "")
		started := time.Now()
		pdf := fixture.exportOK(t, fixture.admin, "payments.list", "format=pdf")
		pdfTime := time.Since(started)
		pages := reports.PDFPagesForTest(t, pdf.body)
		if len(pages) < 50 {
			t.Fatalf("Miles de renglones deben ocupar muchas páginas: %d", len(pages))
		}
		for number, page := range pages {
			if !strings.Contains(page, fmt.Sprintf("Página %d de %d", number+1, len(pages))) || !strings.Contains(page, "Método") {
				t.Fatalf("Página %d sin número o sin encabezados repetidos", number+1)
			}
		}
		if !strings.Contains(pages[len(pages)-1], moneytext.Format("MXN", fmt.Sprint(result.Totals["amount"]))) {
			t.Fatal("El total del PDF grande debe ser el de la pantalla")
		}
		started = time.Now()
		xlsx := fixture.exportOK(t, fixture.admin, "payments.list", "format=xlsx")
		xlsxTime := time.Since(started)
		assertSameAsScreen(t, "payments.list", result, xlsx.body)
		t.Logf("payments.list con %d renglones: PDF %d páginas %d bytes en %s; XLSX %d bytes en %s", result.Page.TotalRows, len(pages),
			len(pdf.body), pdfTime.Round(time.Millisecond), len(xlsx.body), xlsxTime.Round(time.Millisecond))
		// Exactamente 50 000 renglones se exportan; uno más se rechaza con 422 sin archivo ni auditoría.
		insert(reports.MaxExportRows-result.Page.TotalRows, "limite")
		if total := fixture.screen(t, fixture.admin, "payments.list", "").Page.TotalRows; total != reports.MaxExportRows {
			t.Fatalf("Renglones al límite: %d", total)
		}
		for _, format := range []string{"pdf", "xlsx"} {
			started = time.Now()
			response := fixture.exportOK(t, fixture.admin, "payments.list", "format="+format)
			t.Logf("payments.list con 50 000 renglones (%s): %d bytes en %s", format, len(response.body), time.Since(started).Round(time.Millisecond))
			if len(response.body) > reports.MaxExportBytes {
				t.Fatalf("El archivo de 50 000 renglones excede 32 MiB: %d", len(response.body))
			}
		}
		insert(1, "excedente")
		before := exportCount()
		response := fixture.export(t, fixture.admin, "payments.list", "format=xlsx")
		if response.status != 422 || !strings.Contains(string(response.body), "EXPORT_TOO_LARGE") ||
			!strings.Contains(string(response.body), "50,001 renglones") || strings.HasPrefix(string(response.body), "PK") {
			t.Fatalf("Más de 50 000 renglones: %d %s", response.status, response.body)
		}
		if exportCount() != before {
			t.Fatal("Una exportación rechazada no se audita como exportada")
		}
		// La pantalla sigue respondiendo con el mismo conjunto (paginado).
		if total := fixture.screen(t, fixture.admin, "payments.list", "").Page.TotalRows; total != reports.MaxExportRows+1 {
			t.Fatalf("La pantalla debe seguir consultando el conjunto completo: %d", total)
		}
	})
}

// adminID es el usuario administrador de la base de prueba.
func (fixture *reportsFixture) adminID(t *testing.T) string {
	t.Helper()
	return fixture.scalar(t, "SELECT id::text FROM app.users WHERE username='admin'")
}
