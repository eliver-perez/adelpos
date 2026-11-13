package payments

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"local/ad-el-pos/server/internal/documents"
	"local/ad-el-pos/server/internal/kitchen"
)

// Revisión 9F (M2): al reintentar o reimprimir en una impresora que cambió de papel, la precuenta, el ticket o
// el documento de caja se preparan de nuevo con los datos que guardó su ticket y la misma versión de
// plantilla (sin versión guardada, la predeterminada, que no consulta la base): con el mismo papel el texto es
// idéntico al emitido y con otro solo cambia el ancho.

func storedCommercial(t *testing.T) (*CommercialDocument, []byte) {
	t.Helper()
	document := &CommercialDocument{
		Kind: "precheck", Establishment: "ADEL POS", Folio: 31, AccountName: "Mesa 4", Context: "Mesa 4",
		LocalTime: "13/11/2026 14:00", Cashier: "Cajera",
		Lines:    []DocumentLine{{Quantity: 2, Name: "Corte de res con guarnición de temporada", Amount: "1250.00"}},
		Tax:      []TaxLine{{Rate: 16, Base: "1077.59", Tax: "172.41", Total: "1250.00"}},
		Total:    "1250.00",
		Payments: []DocumentPayment{}, Paid: "0.00", Tips: "0.00", Balance: "1250.00",
	}
	version, err := documents.VersionByID(context.Background(), nil, documents.KindPrecheck, nil)
	if err != nil {
		t.Fatal(err)
	}
	layout, err := renderCommercial(version, document, nil, documents.RenderOptions{PaperWidth: 80})
	if err != nil {
		t.Fatal(err)
	}
	document.Content = layout.Text()
	stored, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	return document, stored
}

func TestRenderStoredCommercialDocument(t *testing.T) {
	document, stored := storedCommercial(t)
	same, err := RenderStoredDocument(context.Background(), nil, "precheck", stored, nil, documents.RenderOptions{PaperWidth: 80})
	if err != nil {
		t.Fatal(err)
	}
	if same.Text() != document.Content {
		t.Fatalf("Con el mismo papel el documento cambió:\n%s\n---\n%s", same.Text(), document.Content)
	}
	narrow, err := RenderStoredDocument(context.Background(), nil, "precheck", stored, nil, documents.RenderOptions{PaperWidth: 58})
	if err != nil {
		t.Fatal(err)
	}
	text := narrow.Text()
	if text == document.Content || !strings.Contains(text, "$1,250.00") || !strings.Contains(text, "CFDI") {
		t.Fatalf("La precuenta no se preparó para 58 mm con los mismos datos:\n%s", text)
	}
	for _, row := range strings.Split(text, "\n") {
		if len([]rune(row)) > 30 {
			t.Fatalf("Un renglón no cabe en 58 mm: %q", row)
		}
	}
}

func TestRenderStoredCashDocument(t *testing.T) {
	data := documents.SampleData(documents.KindCashIncome, documents.Identity{Name: "ADEL POS"}).Cash
	stored, err := json.Marshal(map[string]any{"kind": kitchen.KindCashVoucher, "data": data, "includes_expected": false})
	if err != nil {
		t.Fatal(err)
	}
	wide, err := RenderStoredDocument(context.Background(), nil, kitchen.KindCashVoucher, stored, nil, documents.RenderOptions{PaperWidth: 80})
	if err != nil {
		t.Fatal(err)
	}
	narrow, err := RenderStoredDocument(context.Background(), nil, kitchen.KindCashVoucher, stored, nil, documents.RenderOptions{PaperWidth: 58})
	if err != nil {
		t.Fatal(err)
	}
	if narrow.Text() == wide.Text() || narrow.PaperWidth != 58 {
		t.Fatal("El comprobante no se preparó para 58 mm")
	}
	// Un documento de caja emitido antes de las plantillas solo guarda su texto: no se puede preparar.
	var problem *kitchen.Error
	_, err = RenderStoredDocument(context.Background(), nil, kitchen.KindCashReport, []byte(`{"content":"CORTE"}`), nil,
		documents.RenderOptions{PaperWidth: 58})
	if !errors.As(err, &problem) || problem.Code != "PRINT_RENDER_UNAVAILABLE" {
		t.Fatalf("Un documento sin datos guardados no se rechazó: %v", err)
	}
	_, err = RenderStoredDocument(context.Background(), nil, documents.TicketKindSample, []byte(`{}`), nil, documents.RenderOptions{PaperWidth: 58})
	if !errors.As(err, &problem) {
		t.Fatalf("Una muestra del diseñador no se rechazó: %v", err)
	}
}
