package legacyimport

import (
	"slices"
	"strings"
	"testing"

	"local/ad-el-pos/server/internal/businessday"
)

// Con un lote vigente, el resumen previo avisa en lenguaje sencillo qué pasa al repetir la migración; sin él, no
// (revisión de la Fase C, aviso de reemplazo).
func TestPreviewWarnsWhatReplacingDoes(t *testing.T) {
	model := loadSynthetic(t, standardSynthetic())
	environment := Environment{Config: businessday.Config{Timezone: "America/Mexico_City", Location: mustLocation(t), Hour: 5}}
	_, first := Analyze(model, environment, 1234, strings.Repeat("cd", 32))
	for _, warning := range replaceWarnings() {
		if slices.Contains(first.Warnings, warning) {
			t.Fatalf("sin lote vigente no hay aviso de reemplazo: %q", first.Warnings)
		}
	}
	environment.CurrentBatch = &BatchView{ID: "00000000-0000-4000-8000-000000000001", Status: "completed"}
	_, again := Analyze(model, environment, 1234, strings.Repeat("cd", 32))
	if !again.WillReplace {
		t.Fatal("con lote vigente el análisis debe ofrecer reemplazarlo")
	}
	joined := strings.Join(again.Warnings, "\n")
	for _, fragment := range []string{"nombre visible, el rol y el estado (activo o inactivo)", "lo que hayas cambiado en ellos se pierde",
		"las contraseñas que les asignaste se conservan", "ya no viene en este paquete se borra",
		"se desactiva y queda como retirado"} {
		if !strings.Contains(joined, fragment) {
			t.Fatalf("falta «%s» en el aviso de reemplazo: %q", fragment, again.Warnings)
		}
	}
	if len(again.Warnings) != len(first.Warnings)+len(replaceWarnings()) {
		t.Fatalf("avisos: %q", again.Warnings)
	}
}

// Las ventas pagadas sin renglones ni importes no quedan en el libro: la validación lo explica con su conteo y, sin
// ninguna, no dice nada (revisión de la Fase C, M-7).
func TestValidationExplainsPaidSalesWithoutLedger(t *testing.T) {
	expected := Expected{Counts: map[string]int64{}}
	note := "no quedan en el libro de ventas"
	for _, item := range []struct {
		count    int64
		expected string
	}{
		{0, ""},
		{1, "1 venta pagada no tiene renglones ni importes (total 0): no queda en el libro de ventas, así que «Cuentas cerradas» y el conteo de cuentas de los reportes dan 1 menos que «Ventas pagadas»."},
		{22, "22 ventas pagadas no tienen renglones ni importes (total 0): no quedan en el libro de ventas, así que «Cuentas cerradas» y el conteo de cuentas de los reportes dan 22 menos que «Ventas pagadas»."},
	} {
		validation := compare(expected, map[string]int64{}, writtenMoney{withoutLedger: item.count})
		found := ""
		for _, line := range validation.Notes {
			if strings.Contains(line, "renglones ni importes") {
				found = line
			}
		}
		if found != item.expected {
			t.Fatalf("%d ventas sin libro: %q", item.count, validation.Notes)
		}
		if item.count == 0 && strings.Contains(strings.Join(validation.Notes, " "), note) {
			t.Fatalf("sin ventas así no hay nota: %q", validation.Notes)
		}
		if !validation.OK {
			t.Fatalf("la nota no es una diferencia: %+v", validation)
		}
	}
}
