package documents

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// «Importes» de la lista de cocina (revisión de la etapa 9B, hallazgo medio 3). La comanda solo
// guarda los precios si Cocina tiene activado «Mostrar precios», así que la plantilla no puede
// ofrecer «Sí»: la opción queda en «Según la configuración de Cocina» u «Ocultar», y la muestra
// sigue la misma regla que el papel.

func TestKitchenPricesOnlyInheritOrHide(t *testing.T) {
	_, err := ParseTemplate(KindKitchenOrder, []byte(`{"blocks":[{"id":"lista","type":"kitchen_items","options":{"show_prices":"yes"}}]}`))
	var problem *ValidationError
	if !errors.As(err, &problem) || problem.BlockID != "lista" || !strings.Contains(problem.Message, "«Incluir importes comerciales en comandas»") {
		t.Fatalf("«Importes: Sí» debía rechazarse con el bloque y la explicación: %v", err)
	}
	for _, value := range []string{"inherit", "no"} {
		raw := `{"blocks":[{"id":"lista","type":"kitchen_items","options":{"show_prices":"` + value + `"}}]}`
		if _, err := ParseTemplate(KindKitchenOrder, []byte(raw)); err != nil {
			t.Fatalf("«Importes: %s» debía aceptarse: %v", value, err)
		}
	}
	// Las observaciones conservan sus tres valores.
	if _, err := ParseTemplate(KindKitchenOrder, []byte(`{"blocks":[{"id":"lista","type":"kitchen_items","options":{"show_notes":"yes"}}]}`)); err != nil {
		t.Fatalf("«Observaciones: Sí» debía aceptarse: %v", err)
	}
	catalog := BuildCatalog()
	for _, entry := range catalog.Kinds {
		if entry.Kind != KindKitchenOrder {
			continue
		}
		for _, block := range entry.Blocks {
			if block.Type != BlockKitchenItems {
				continue
			}
			for _, option := range block.Options {
				if option.Key != "show_prices" {
					continue
				}
				if len(option.Values) != 2 || option.Values[0].Value != Inherit || option.Values[1].Value != OptionNo {
					t.Fatalf("El catálogo debe ofrecer solo «Según Cocina» y «Ocultar»: %+v", option.Values)
				}
				// El texto de ayuda viaja en el catálogo («help») y explica cómo imprimir importes.
				encoded, err := json.Marshal(option)
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(string(encoded), `"help":"Para imprimir los importes, activa «Incluir importes comerciales en comandas» en Cocina`) {
					t.Fatalf("La opción no explica cómo imprimir importes: %s", encoded)
				}
				return
			}
		}
	}
	t.Fatal("El catálogo no ofrece la opción «Importes» de la lista de cocina")
}
