package documents

import (
	"strings"
	"testing"
)

// La muestra de una comanda aplica la configuración vigente de Cocina como la comanda real: sin
// «Mostrar precios» no hay importes (aunque la plantilla los herede) y con él sí, salvo «Ocultar».
func TestKitchenSampleFollowsKitchenSettings(t *testing.T) {
	inherit := mustParse(t, KindKitchenOrder, `{"blocks":[{"id":"lista","type":"kitchen_items"}]}`)
	hidden := mustParse(t, KindKitchenOrder, `{"blocks":[{"id":"lista","type":"kitchen_items","options":{"show_prices":"no"}}]}`)
	render := func(template *Template, settings KitchenSettings) string {
		t.Helper()
		data := SampleData(KindKitchenOrder, Identity{Name: "Restaurante"})
		data.Kitchen.UseSettings(settings)
		layout, err := Render(KindKitchenOrder, template, data, RenderOptions{PaperWidth: 80})
		if err != nil {
			t.Fatal(err)
		}
		return layout.Text()
	}
	cocina := KitchenSettings{ShowNotes: true, ShowEstablishment: true, TextSize: TextSizeNormal, LineSpacing: 1, QuantityStyle: QuantityPrefix}
	if text := render(inherit, cocina); strings.Contains(text, "Importe:") {
		t.Fatalf("Sin «Mostrar precios» la muestra no debe llevar importes:\n%s", text)
	}
	cocina.ShowPrices = true
	if text := render(inherit, cocina); !strings.Contains(text, "Importe: $250.00") || !strings.Contains(text, "Importe: $45.00") {
		t.Fatalf("Con «Mostrar precios» la muestra debe llevar los importes:\n%s", text)
	}
	if text := render(hidden, cocina); strings.Contains(text, "Importe:") {
		t.Fatalf("«Ocultar» no debe imprimir importes:\n%s", text)
	}
	// Las demás opciones heredadas también siguen a Cocina.
	cocina.ShowNotes = false
	cocina.QuantityStyle = QuantityMulti
	text := render(inherit, cocina)
	if strings.Contains(text, "NOTA:") || !strings.Contains(text, "Platillo de muestra × 2") {
		t.Fatalf("La muestra no siguió las observaciones y cantidades de Cocina:\n%s", text)
	}
	// Una comanda guardada con precios borrados no los imprime aunque la plantilla herede.
	data := SampleData(KindKitchenOrder, Identity{Name: "Restaurante"})
	data.Kitchen.UseSettings(KitchenSettings{ShowNotes: true, TextSize: TextSizeNormal, LineSpacing: 1, QuantityStyle: QuantityPrefix})
	for _, item := range data.Kitchen.Sections[0].Items {
		if item.Price != "" {
			t.Fatalf("Sin «Mostrar precios» la muestra no conserva importes: %+v", item)
		}
	}
}
