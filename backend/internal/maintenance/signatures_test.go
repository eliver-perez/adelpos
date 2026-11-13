package maintenance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sourceText lee un archivo de prueba del repositorio (las rutas de las firmas son relativas a la raíz).
func sourceText(t *testing.T, source string) string {
	t.Helper()
	if source == "" || filepath.IsAbs(source) || strings.Contains(source, "..") {
		t.Fatalf("ruta de prueba inválida en una firma: %q", source)
	}
	content, err := os.ReadFile(filepath.Join("..", "..", "..", filepath.FromSlash(source)))
	if err != nil {
		t.Fatalf("la firma cita un archivo de prueba que no existe: %s", source)
	}
	return string(content)
}

func requireLiterals(t *testing.T, source string, literals ...string) {
	t.Helper()
	text := sourceText(t, source)
	for _, literal := range literals {
		if !strings.Contains(text, "'"+literal+"'") {
			t.Errorf("%s ya no crea %q: actualiza la firma o la prueba", source, literal)
		}
	}
}

// Cada firma de la lista cerrada sigue correspondiendo exactamente a lo que crea su archivo de prueba.
func TestSignaturesMatchTheirTestFiles(t *testing.T) {
	if len(TestPrinters) == 0 || len(TestStations) == 0 || len(TestProducts) == 0 || len(TestCategories) == 0 {
		t.Fatal("la lista cerrada de firmas está incompleta")
	}
	for _, printer := range TestPrinters {
		agent := printer.Transport == "agent" && printer.Destination == acceptanceDestination
		missingQueue := printer.Transport == "cups" && printer.Destination == acceptanceQueue
		if !agent && !missingQueue {
			t.Errorf("la impresora %q no es un destino de aceptación", printer.Name)
		}
		requireLiterals(t, printer.Source, printer.Name, printer.Transport, printer.Destination)
	}
	for _, station := range TestStations {
		requireLiterals(t, station.Source, station.Name)
	}
	// Las estaciones de prueba solo cuentan con la impresora de prueba que les asigna su archivo.
	kitchenFlow := sourceText(t, "desktop/e2e/kitchen-flow.ts")
	if !strings.Contains(kitchenFlow, "'Impresora compartida o exclusiva'") || !strings.Contains(kitchenFlow, "'Destino controlado E2E'") {
		t.Error("kitchen-flow.ts ya no asigna «Destino controlado E2E» a sus estaciones")
	}
	for _, product := range TestProducts {
		requireLiterals(t, product.Source, product.Name, product.Code, product.Kind)
		requireLiterals(t, "desktop/e2e/native-driver.ts", product.Name, product.Code)
	}
	for _, category := range TestCategories {
		requireLiterals(t, category.Source, category.Name, category.Color)
		requireLiterals(t, "desktop/e2e/native-driver.ts", category.Name, category.Color)
	}
}

// Ninguna firma es un nombre suelto: todas llevan atributos que solo crea la prueba.
func TestSignaturesAreNotLooseNames(t *testing.T) {
	for _, printer := range TestPrinters {
		if printer.Name == "" || printer.Transport == "" || printer.Destination == "" {
			t.Errorf("firma de impresora incompleta: %+v", printer)
		}
	}
	for _, product := range TestProducts {
		if product.Name == "" || product.Code == "" || product.Kind == "" {
			t.Errorf("firma de producto incompleta: %+v", product)
		}
	}
	for _, category := range TestCategories {
		if category.Name == "" || category.Color == "" {
			t.Errorf("firma de categoría incompleta: %+v", category)
		}
	}
}
