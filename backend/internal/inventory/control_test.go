package inventory

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

// Pruebas unitarias de la fase D: redondeo de sugerencias de compra, vencimiento de conteos por
// frecuencia, clasificación de diferencias, motivos de ajuste y rutas.

func TestSuggestedQuantityRoundsUpToFullPresentations(t *testing.T) {
	base := func(text string) *decimal.Decimal {
		value := decimal.RequireFromString(text)
		return &value
	}
	for _, entry := range []struct {
		need, presentation, packages, quantity string
	}{
		// Faltan 30 piezas en cajas de 24: 2 cajas (48 piezas), nunca 1.
		{"30", "24", "2", "48"},
		// Exactamente dos cajas: sin redondeo.
		{"48", "24", "2", "48"},
		// Una millonésima arriba de dos cajas: 3 cajas.
		{"48.000001", "24", "3", "72"},
		// Costal de 25 kg (25000 g) para 1250.5 g: un costal.
		{"1250.5", "25000", "1", "25000"},
		// Presentación fraccionaria de 0.5 l en mililitros (500): 700 ml → 2 botellas.
		{"700", "500", "2", "1000"},
	} {
		packages, quantity := suggestedQuantity(decimal.RequireFromString(entry.need), base(entry.presentation))
		if packages.String() != entry.packages || quantity.String() != entry.quantity {
			t.Fatalf("Necesidad %s en presentaciones de %s: %s (%s), se esperaba %s (%s)", entry.need, entry.presentation, packages,
				quantity, entry.packages, entry.quantity)
		}
	}
	// Sin presentación: unidades base enteras hacia arriba.
	for need, expected := range map[string]string{"2.5": "3", "3": "3", "0.000001": "1"} {
		packages, quantity := suggestedQuantity(decimal.RequireFromString(need), nil)
		if packages.String() != expected || quantity.String() != expected {
			t.Fatalf("Necesidad %s en la unidad base: %s, se esperaba %s", need, packages, expected)
		}
	}
}

func TestCountDueByFrequency(t *testing.T) {
	day := func(text string) *time.Time {
		value, err := time.Parse("2006-01-02", text)
		if err != nil {
			t.Fatal(err)
		}
		return &value
	}
	today := *day("2026-11-10")
	interval := 3
	for _, entry := range []struct {
		frequency string
		interval  *int
		last      *time.Time
		due       bool
	}{
		{"none", nil, nil, false},
		{"daily", nil, nil, true},
		{"daily", nil, day("2026-11-10"), false},
		{"daily", nil, day("2026-11-09"), true},
		{"weekly", nil, day("2026-11-04"), false},
		{"weekly", nil, day("2026-11-03"), true},
		{"monthly", nil, day("2026-10-11"), false},
		{"monthly", nil, day("2026-10-10"), true},
		{"custom", &interval, day("2026-11-08"), false},
		{"custom", &interval, day("2026-11-07"), true},
		{"custom", nil, day("2026-01-01"), false},
	} {
		if got := countDue(entry.frequency, entry.interval, entry.last, today); got != entry.due {
			t.Fatalf("Frecuencia %s con último conteo %v: %v, se esperaba %v", entry.frequency, entry.last, got, entry.due)
		}
	}
	// Mensual al final de mes: el 31 de marzo menos un mes normaliza al 3 de marzo; un conteo del 2 de
	// marzo ya venció y uno del 3 no.
	march := *day("2026-03-31")
	if !countDue("monthly", nil, day("2026-03-02"), march) || countDue("monthly", nil, day("2026-03-04"), march) {
		t.Fatal("La frecuencia mensual al final de mes no coincide con AddDate")
	}
}

func TestDifferenceKind(t *testing.T) {
	for text, expected := range map[string]string{"3": "surplus", "-0.5": "shortage", "0": "none"} {
		if got := differenceKind(decimal.RequireFromString(text)); got != expected {
			t.Fatalf("Diferencia %s: %s, se esperaba %s", text, got, expected)
		}
	}
}

func TestAdjustmentReasonsAreConsistent(t *testing.T) {
	if len(adjustmentReasonLabels) != 8 || len(adjustmentReasonDirection) != 8 {
		t.Fatalf("Se esperaban 8 motivos de ajuste con sentido: %d y %d", len(adjustmentReasonLabels), len(adjustmentReasonDirection))
	}
	for reason := range adjustmentReasonLabels {
		if _, found := adjustmentReasonDirection[reason]; !found {
			t.Fatalf("El motivo %s no tiene sentido definido", reason)
		}
	}
	for reason := range wasteReasons {
		if adjustmentReasonDirection[reason] > 0 {
			t.Fatalf("Una merma no puede usar el motivo de entrada %s", reason)
		}
	}
	if wasteReasons["surplus"] || wasteReasons["entry_error"] || wasteReasons["physical_shortage"] {
		t.Fatal("Sobrante, error de registro y faltante físico son discrepancias, no mermas")
	}
	reason := adjustmentMovementReason("other", "Botella abierta para cata")
	if reason == nil || *reason != "Otro: Botella abierta para cata" {
		t.Fatalf("Motivo del movimiento: %v", reason)
	}
}

func TestControlRoutes(t *testing.T) {
	for _, entry := range []struct {
		method string
		parts  []string
		static bool
		found  bool
	}{
		{"POST", []string{"adjustments"}, false, true},
		{"POST", []string{"adjustments", "x", "authorize"}, false, true},
		{"POST", []string{"adjustments", "x", "reject"}, false, true},
		{"PATCH", []string{"adjustments", "x"}, false, false},
		{"POST", []string{"transfers"}, false, true},
		{"PUT", []string{"counts", "x", "lines"}, false, true},
		{"POST", []string{"counts", "x", "apply"}, false, true},
		{"DELETE", []string{"counts", "x"}, false, false},
		{"PUT", []string{"stock-levels"}, false, true},
		{"GET", []string{"alerts"}, false, true},
		{"POST", []string{"alerts"}, false, false},
		{"GET", []string{"purchase-suggestions"}, false, true},
		{"POST", []string{"purchase-suggestions", "draft-order"}, true, true},
		{"POST", []string{"purchase-suggestions", "issue-order"}, false, false},
	} {
		selected := resolve(entry.method, entry.parts)
		if (selected != nil) != entry.found {
			t.Fatalf("%s %v: encontrada %v, se esperaba %v", entry.method, entry.parts, selected != nil, entry.found)
		}
		if selected != nil && selected.staticPath != entry.static {
			t.Fatalf("%s %v: ruta fija %v, se esperaba %v", entry.method, entry.parts, selected.staticPath, entry.static)
		}
	}
	// Las lecturas de compras y de inventario anteriores siguen resolviéndose igual.
	if selected := resolve("GET", []string{"purchase-orders"}); selected == nil || len(selected.anyPermission) != 6 {
		t.Fatal("La lectura de órdenes de compra cambió")
	}
	if selected := resolve("GET", []string{"stock"}); selected == nil || selected.permissions[0] != permissionStockRead {
		t.Fatal("La consulta de existencias cambió")
	}
}

func TestAlertTypesAreUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, entry := range alertTypes {
		if seen[entry.code] || entry.label == "" || alertSeverityLabels[entry.severity] == "" {
			t.Fatalf("Tipo de alerta incompleto o repetido: %+v", entry)
		}
		seen[entry.code] = true
	}
	if len(seen) != 12 {
		t.Fatalf("Se esperaban 12 tipos de alerta: %d", len(seen))
	}
}
