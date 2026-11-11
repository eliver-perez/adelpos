package kitchen

import (
	"bytes"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"local/ad-el-pos/server/internal/catalog"
	"local/ad-el-pos/server/internal/documents"
)

// Equivalencia exacta de la etapa 9B: la plantilla predeterminada de la comanda (y la de los avisos
// de modificación y cancelación y la prueba técnica de impresora) produce, byte a byte, el mismo
// texto que la plantilla anterior (legacyRenderText, copia literal), para ambos anchos y todas las
// opciones de Cocina.

func equivalenceIngredient(id, name, included, requested string) catalog.IngredientBreakdown {
	return catalog.IngredientBreakdown{IngredientID: id, Name: name, Included: included, Requested: requested}
}

// equivalenceDocuments son comandas con los casos que cambian el formato: salón y para llevar (con
// y sin cliente), datos vacíos, nombres largos que cortan, observaciones con controles, antes y
// ahora, motivo, cantidades mayores que uno con partes, complementos sin opciones, variantes,
// indicaciones rápidas con efecto e importes con miles.
func equivalenceDocuments() []Document {
	created := time.Date(2026, 11, 10, 19, 4, 5, 0, time.UTC)
	longName := strings.Repeat("Hamburguesa doble con tocino ", 3)
	quick := quickNotesTicket()
	quickParts := quick.Sections[0].Items[0].Parts
	quickParts[0].QuickNotes = []catalog.QuickNoteBreakdown{
		resolvedNote("n1", "Sin cebolla", "omit", "cebolla"),
		resolvedNote("n2", "Bien cocida", "none", ""),
	}
	quickParts[1].QuickNotes = []catalog.QuickNoteBreakdown{resolvedNote("n4", "Extra sal", "extra", "sal")}
	richItems := []Instructions{
		{
			ItemID: "partida-a", Name: "Hamburguesa", Quantity: 2, Price: "1250.00", Notes: "Sin cebolla\x1b\x40\x1d\x56\x00; café\tñ\nsegunda línea",
			Parts: []Part{
				{Path: "root", Name: "Hamburguesa", Variant: "Doble", Quantity: 1, Notes: "Nota de raíz",
					Ingredients: []catalog.IngredientBreakdown{
						equivalenceIngredient("cebolla", "Cebolla", "1", "0"),
						equivalenceIngredient("queso", "Queso", "1", "2"),
						equivalenceIngredient("pan", "Pan", "1", "1"),
					},
					Groups: []catalog.GroupBreakdown{
						{Name: "Salsas", Options: []catalog.OptionBreakdown{{Name: "Chipotle", Requested: "1"}, {Name: "Mayonesa", Requested: "0"}}},
						{Name: "Guarniciones", Options: []catalog.OptionBreakdown{{Name: "Papas", Requested: "0"}}},
					}},
				{Path: "root/0", Name: longName, Quantity: 3, Notes: "Componente con nota larga " + strings.Repeat("x", 50)},
			},
		},
		{ItemID: "partida-b", Name: longName, Quantity: 1, Price: "45.50", Reason: "Cliente cambió de opinión", Parts: []Part{}},
		{
			ItemID: "partida-c", Name: "Refresco", Quantity: 3, Price: "90.00", Notes: "Sin hielo",
			Before: &Instructions{Name: "Refresco", Quantity: 2, Notes: "Con hielo", Parts: []Part{{Path: "root", Name: "Refresco", Quantity: 1}}},
			Parts:  []Part{{Path: "root", Name: "Refresco", Quantity: 1}},
		},
	}
	sections := []Section{
		{Station: Station{Name: "Cocina caliente"}, Items: richItems},
		{Station: Station{Name: "Barra fría y postres"}, Items: []Instructions{{ItemID: "partida-d", Name: "Pastel", Quantity: 1, Parts: []Part{}}}},
	}
	return []Document{
		{
			ID: "documento-salon", Kind: "normal", CreatedAt: created, Sections: sections,
			Context: TicketContext{Establishment: "Restaurante de prueba", Timezone: "America/Monterrey", Folio: 12, AccountName: "Mesa 1",
				Area: "Terraza", Table: "Mesa 1", Mode: "dine_in", Employee: "Laura", OrderID: "pedido-1"},
		},
		{
			ID: "documento-llevar", Kind: "normal", CreatedAt: created, Sections: sections,
			Context: TicketContext{Establishment: "Restaurante con un nombre comercial bastante largo para cortar", Timezone: "America/Mexico_City",
				Folio: 1234567, AccountName: "Pedido para llevar", Mode: "takeaway", Origin: "Ventanilla", Customer: "Cliente con nombre largo " + longName,
				Employee: "Mesero", OrderID: "pedido-2"},
		},
		{
			ID: "documento-llevar-sin-cliente", Kind: "normal", CreatedAt: created, Sections: sections[:1],
			Context: TicketContext{Establishment: "ADEL POS", Timezone: "Zona/Invalida", Folio: 3, Mode: "takeaway", Origin: "Mostrador"},
		},
		{
			// Datos vacíos: la plantilla anterior imprimía igual las etiquetas y separadores.
			ID: "documento-vacio", Kind: "normal", CreatedAt: created, Sections: []Section{},
			Context: TicketContext{Timezone: "America/Monterrey"},
		},
		quick,
		quickNotesTicket(),
	}
}

func equivalenceSettings() []Settings {
	settings := []Settings{}
	for _, prices := range []bool{false, true} {
		for _, notes := range []bool{false, true} {
			for _, establishment := range []bool{false, true} {
				for _, size := range []string{"normal", "large"} {
					for _, spacing := range []int{0, 1, 2, 3} {
						for _, style := range []string{"prefix", "multiplier"} {
							settings = append(settings, Settings{
								ShowPrices: prices, ShowNotes: notes, ShowEstablishment: establishment, TextSize: size,
								LineSpacing: spacing, QuantityStyle: style,
							})
						}
					}
				}
			}
		}
	}
	return settings
}

func TestDefaultKitchenTemplatesMatchLegacyRenderer(t *testing.T) {
	cases := 0
	for _, base := range equivalenceDocuments() {
		for _, kind := range []string{"normal", "modification", "cancellation", "test"} {
			for _, settings := range equivalenceSettings() {
				for _, paper := range []int{58, 80} {
					document := base
					document.Kind = kind
					document.Settings = settings
					want := legacyRenderText(document, paper)
					if got := RenderText(document, paper); got != want {
						t.Fatalf("La comanda %s (%s, %d mm, %+v) cambió.\nAntes:\n%q\nAhora:\n%q", document.ID, kind, paper, settings, want, got)
					}
					cases++
				}
			}
		}
	}
	if cases < 3000 {
		t.Fatalf("La matriz de equivalencia es demasiado pequeña: %d casos", cases)
	}
}

// Las comandas emitidas desde la 9B guardan su versión, la identidad y el tamaño efectivo: con la
// versión del sistema el texto es idéntico al anterior y el layout es plano.
func TestKitchenDocumentWithStoredVersionIsPlainAndIdentical(t *testing.T) {
	document := equivalenceDocuments()[0]
	document.Settings = Settings{ShowNotes: true, ShowEstablishment: true, LineSpacing: 1, QuantityStyle: "prefix", TextSize: "large"}
	versionID := documents.SystemVersionID(documents.KindKitchenOrder)
	document.TemplateVersionID = &versionID
	document.Identity = &documents.Identity{Name: "Otro nombre que no se usa", Address: "Calle 1"}
	layout, err := RenderDocument(&documents.Version{Kind: documents.KindKitchenOrder, Template: documents.Default(documents.KindKitchenOrder)}, document, 80)
	if err != nil {
		t.Fatal(err)
	}
	if !layout.Plain() || layout.Text() != legacyRenderText(document, 80) || layout.TextSize != "large" || layout.Columns != 21 {
		t.Fatalf("La comanda con versión del sistema no es idéntica ni plana:\n%s", layout.Text())
	}
	if documents.EffectiveTextSize(documents.Default(documents.KindKitchenOrder), "large") != "large" {
		t.Fatal("El tamaño efectivo no hereda la configuración de Cocina")
	}
}

// legacyCUPSArguments es la construcción literal de los argumentos de lp de submitCUPS anterior a
// la 9B (commit fd1de81), para comprobar que el camino de texto no cambió.
func legacyCUPSArguments(job PrintJob, filename string) []string {
	printer := job.PrinterSnapshot
	arguments := []string{"-d", printer.Destination, "-n", fmt.Sprint(printer.Copies), "-t", "ADEL-" + job.ID,
		"-o", "document-format=text/plain", "-o", "job-sheets=none", "-o", "sides=one-sided"}
	cpi := 12
	lineMillimeters := 4
	if job.TextSize == "large" {
		cpi = 6
		lineMillimeters = 8
	}
	height := max(60, min(2000, (strings.Count(job.Content, "\n")+4)*lineMillimeters))
	arguments = append(arguments, "-o", fmt.Sprintf("cpi=%d", cpi), "-o", fmt.Sprintf("media=Custom.%dx%dmm", printer.PaperWidth, height),
		"-o", "page-left=0", "-o", "page-right=0", "-o", "page-top=0", "-o", "page-bottom=0")
	if printer.Cut {
		arguments = append(arguments, "-o", printer.CutterOption)
	}
	return append(arguments, filename)
}

// Un layout plano conserva los bytes ESC/POS y los argumentos de lp del texto anterior.
func TestPlainLayoutKeepsTransportBytesAndArguments(t *testing.T) {
	for _, base := range equivalenceDocuments()[:2] {
		for _, size := range []string{"normal", "large"} {
			for _, paper := range []int{58, 80} {
				document := base
				document.Settings = Settings{ShowNotes: true, ShowPrices: true, ShowEstablishment: true, LineSpacing: 1, QuantityStyle: "prefix", TextSize: size}
				layout, err := RenderDocument(&documents.Version{Kind: documents.KindKitchenOrder, Template: documents.Default(documents.KindKitchenOrder)}, document, paper)
				if err != nil {
					t.Fatal(err)
				}
				printer := Printer{Name: "Cocina", Transport: "cups", Destination: "COCINA", PaperWidth: paper, Copies: 2, Cut: true, SupportsCut: true, CutterOption: "CutMedia=1"}
				legacy := legacyRenderText(document, paper)
				if !bytes.Equal(ESCPosBytes(layout.Text(), printer, size), ESCPosBytes(legacy, printer, size)) {
					t.Fatal("Los bytes ESC/POS de un layout plano cambiaron")
				}
				job := PrintJob{ID: "trabajo", PrinterSnapshot: printer, Content: layout.Text(), TextSize: size}
				legacyJob := job
				legacyJob.Content = legacy
				if !slices.Equal(cupsTextArguments(job, "/tmp/archivo"), legacyCUPSArguments(legacyJob, "/tmp/archivo")) {
					t.Fatalf("Los argumentos de lp de un layout plano cambiaron: %v", cupsTextArguments(job, "/tmp/archivo"))
				}
			}
		}
	}
}
