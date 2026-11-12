//go:build integration

package designer_test

import (
	"encoding/json"
	"strings"
	"testing"

	"local/ad-el-pos/server/internal/documents"
	"local/ad-el-pos/server/internal/kitchen"
)

// Etapa 9C (§5.3): la vista previa del diseñador permite elegir una impresora activa y muestra su
// ancho (papel y columnas de Cocina → Impresoras), además de 58 y 80 mm con sus columnas
// predeterminadas; documents/printers informa las columnas de cada impresora y una muestra impresa
// sale con las columnas de la impresora elegida.
func testPrinterPreview(t *testing.T, value *fixture) {
	wide, narrow := 48, 32
	create := func(name string, paper int, columns *int, active bool) kitchen.Printer {
		t.Helper()
		return decodeAs[kitchen.Printer](t, value.mutate(t, value.admin, "POST", "kitchen/printers", printerBody(kitchen.Printer{
			Name: name, Transport: "cups", Destination: "ADEL_TEST_USB", Active: active, PaperWidth: paper, Copies: 1, Columns: columns,
		}), 200))
	}
	wideTickets := create("Tickets 48 columnas", 80, &wide, true)
	narrowTickets := create("Tickets 32 columnas", 58, &narrow, true)
	inactive := create("Tickets inactiva", 80, &wide, false)
	if wideTickets.Columns == nil || *wideTickets.Columns != wide || narrowTickets.Columns == nil || *narrowTickets.Columns != narrow {
		t.Fatalf("No se guardaron las columnas: %+v %+v", wideTickets, narrowTickets)
	}
	listed := decodeAs[[]struct {
		ID             string `json:"id"`
		PaperWidth     int    `json:"paper_width"`
		Columns        int    `json:"columns"`
		DefaultColumns int    `json:"default_columns"`
	}](t, value.call(t, value.admin, "GET", "documents/printers", nil, 200))
	columns := map[string][3]int{}
	for _, item := range listed {
		columns[item.ID] = [3]int{item.PaperWidth, item.Columns, item.DefaultColumns}
	}
	if columns[wideTickets.ID] != [3]int{80, 48, 42} || columns[narrowTickets.ID] != [3]int{58, 32, 30} {
		t.Fatalf("documents/printers no informa las columnas de cada impresora: %+v", columns)
	}
	if _, found := columns[inactive.ID]; found {
		t.Fatal("Una impresora inactiva no se ofrece para la vista previa")
	}
	type printerPreview struct {
		PaperWidth  int    `json:"paper_width"`
		Columns     int    `json:"columns"`
		PrinterID   string `json:"printer_id"`
		PrinterName string `json:"printer_name"`
		Text        string `json:"text"`
	}
	layout := json.RawMessage(documents.Default(documents.KindReceipt).Encode())
	preview := func(client terminal, body map[string]any, status int) []byte {
		t.Helper()
		body["kind"], body["layout"], body["source"] = "receipt", layout, map[string]any{"sample": true}
		return value.call(t, client, "POST", "documents/preview", body, status)
	}
	widest := func(text string) int {
		result := 0
		for _, line := range strings.Split(text, "\n") {
			result = max(result, len([]rune(line)))
		}
		return result
	}
	standard := decodeAs[printerPreview](t, preview(value.admin, map[string]any{"paper_width": 80}, 200))
	if standard.Columns != 42 || standard.PrinterID != "" || widest(standard.Text) != 42 {
		t.Fatalf("Sin impresora la vista previa usa el ancho predeterminado: %+v", standard)
	}
	for printer, expected := range map[string][2]int{wideTickets.ID: {80, 48}, narrowTickets.ID: {58, 32}} {
		result := decodeAs[printerPreview](t, preview(value.admin, map[string]any{"printer_id": printer}, 200))
		if result.PaperWidth != expected[0] || result.Columns != expected[1] || result.PrinterID != printer || result.PrinterName == "" ||
			widest(result.Text) != expected[1] {
			t.Fatalf("La vista previa con la impresora no usa su ancho %v: %+v", expected, result)
		}
	}
	// El papel indicado debe coincidir con el de la impresora; una inactiva o inexistente se rechaza.
	mismatch := errorOf(t, preview(value.admin, map[string]any{"printer_id": wideTickets.ID, "paper_width": 58}, 400))
	if !strings.Contains(mismatch.Message, "no corresponde a la impresora") {
		t.Fatalf("Papel distinto del de la impresora: %+v", mismatch)
	}
	preview(value.admin, map[string]any{"printer_id": wideTickets.ID, "paper_width": 80}, 200)
	preview(value.admin, map[string]any{"printer_id": inactive.ID}, 400)
	preview(value.admin, map[string]any{"printer_id": randomID()}, 400)
	preview(value.admin, map[string]any{"printer_id": "no-es-uuid"}, 400)
	// El mismo permiso de lectura que documents/printers basta para elegir la impresora.
	reader := value.user(t, "lector_impresoras", []string{documents.PermissionTemplatesRead}, false)
	if result := decodeAs[printerPreview](t, preview(reader, map[string]any{"printer_id": wideTickets.ID}, 200)); result.Columns != 48 {
		t.Fatalf("Vista previa del lector: %+v", result)
	}
	// La muestra impresa sale con las columnas de la impresora elegida.
	sample := decodeAs[struct {
		JobID string `json:"job_id"`
	}](t, value.mutate(t, value.admin, "POST", "documents/samples", map[string]any{"kind": "receipt", "layout": layout, "printer_id": wideTickets.ID}, 200))
	var content, snapshot string
	if err := value.pool.QueryRow(value.ctx, "SELECT content,printer_snapshot->>'columns' FROM app.kitchen_print_jobs WHERE id=$1",
		sample.JobID).Scan(&content, &snapshot); err != nil {
		t.Fatal(err)
	}
	if widest(content) != 48 || snapshot != "48" || !strings.Contains(content, "MUESTRA") {
		t.Fatalf("La muestra no salió a 48 columnas:\n%s", content)
	}
	if _, err := value.pool.Exec(value.ctx, "UPDATE app.kitchen_print_jobs SET state='cancelled' WHERE id=$1", sample.JobID); err != nil {
		t.Fatal(err)
	}
}
