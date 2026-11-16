package legacyimport

import (
	"slices"
	"testing"
	"time"
)

// Los cortes del anterior sin fecha_inicio toman la de su primera venta o pago y, cerrados sin fecha_fin, la de la
// última; sin ventas ni pagos quedan sin fecha (revisión de la Fase C, M-8). Una fecha guardada nunca se cambia.
func TestShiftDatesFallBackToTheirSalesAndPayments(t *testing.T) {
	location := mustLocation(t)
	at := func(text string) time.Time {
		parsed, err := ParseLocalTime(text, location)
		if err != nil {
			t.Fatal(err)
		}
		return parsed
	}
	shift := func(id int32, status, start, end string) Shift {
		value := Shift{ID: id, Status: status, StatusSet: true}
		if start != "" {
			value.OpenedAt = at(start)
		}
		if end != "" {
			value.ClosedAt = at(end)
		}
		return value
	}
	inShift := func(id int32) NullInt {
		return NullInt{Value: id, Valid: true}
	}
	model := &Model{Location: location, Info: PackageInfo{Tables: map[string]int64{}}, TableRows: map[string]int64{},
		Shifts: []Shift{
			shift(1, "C", "", ""),                                       // sin fechas, con una venta y dos pagos
			shift(2, "A", "", ""),                                       // abierto sin fecha de inicio: sin fin estimado
			shift(3, "C", "", ""),                                       // sin fechas ni ventas o pagos
			shift(4, "C", "2023-05-01T08:00:00", ""),                    // con inicio guardado y sin fin
			shift(5, "C", "2023-06-01T08:00:00", "2023-06-01T22:00:00"), // completo: no cambia
			shift(6, "C", "", "2023-07-01T09:00:00"),                    // fin guardado antes de su primera venta
		},
		Sales: []Sale{
			{ID: 1, Folio: "1", OpenedAt: at("2023-05-10T13:00:00"), Shift: inShift(1)},
			{ID: 2, Folio: "2", OpenedAt: at("2023-05-11T09:00:00"), Shift: inShift(2)},
			{ID: 3, Folio: "3", OpenedAt: at("2023-05-02T10:00:00"), Shift: inShift(4)},
			{ID: 4, Folio: "4", OpenedAt: at("2023-06-01T12:00:00"), Shift: inShift(5)},
			{ID: 5, Folio: "5", OpenedAt: at("2023-07-02T12:00:00"), Shift: inShift(6)},
		},
		Payments: []Payment{
			{ID: 1, Sale: 1, Amount: NullMoney{Value: 100, Valid: true}, PaidAt: at("2023-05-10T13:40:00"), Shift: inShift(1)},
			// Un pago del corte 1 registrado antes de la apertura de su venta: es la primera fecha del corte.
			{ID: 2, Sale: 1, Amount: NullMoney{Value: 100, Valid: true}, PaidAt: at("2023-05-10T12:50:00"), Shift: inShift(1)},
			{ID: 3, Sale: 3, Amount: NullMoney{Value: 100, Valid: true}, PaidAt: at("2023-05-03T18:30:00"), Shift: inShift(4)},
		},
	}
	history := buildHistory(model)
	cases := []struct {
		label         string
		opened, ended string
		flags         []string
	}{
		{"sin fechas", "2023-05-10T12:50:00", "2023-05-10T13:40:00", []string{FlagStartEstimated, FlagEndEstimated}},
		{"abierto", "2023-05-11T09:00:00", "", []string{FlagStartEstimated}},
		{"sin ventas ni pagos", "", "", []string{FlagNoActivity, FlagNoStart}},
		{"sin fin", "2023-05-01T08:00:00", "2023-05-03T18:30:00", []string{FlagEndEstimated}},
		{"completo", "2023-06-01T08:00:00", "2023-06-01T22:00:00", nil},
		{"fin anterior a su venta", "2023-07-01T09:00:00", "2023-07-01T09:00:00", []string{FlagStartEstimated}},
	}
	for index, item := range cases {
		state := history.Shifts[index]
		expectedOpened, expectedEnded := time.Time{}, time.Time{}
		if item.opened != "" {
			expectedOpened = at(item.opened)
		}
		if item.ended != "" {
			expectedEnded = at(item.ended)
		}
		if !state.OpenedAt.Equal(expectedOpened) || !state.ClosedAt.Equal(expectedEnded) {
			t.Fatalf("%s: %s a %s; se esperaba %s a %s", item.label, state.OpenedAt, state.ClosedAt, item.opened, item.ended)
		}
		for _, flag := range []string{FlagStartEstimated, FlagEndEstimated, FlagNoStart} {
			if state.Flags.has(flag) != slices.Contains(item.flags, flag) {
				t.Fatalf("%s: marcas %v; se esperaban %v", item.label, state.Flags, item.flags)
			}
		}
	}
	// El resumen lo cuenta en lenguaje sencillo (y la lista cerrada de marcas lo describe).
	warnings := shiftDateWarnings(history)
	expected := []string{
		"3 cortes del sistema anterior no tenían fecha de inicio: se usa la de su primera venta o pago para que aparezcan en «Turnos de caja».",
		"2 cortes cerrados del sistema anterior no tenían fecha de fin: se usa la de su última venta o pago.",
		"1 corte del sistema anterior sin fecha de inicio ni ventas o pagos queda sin fecha y no aparece en «Turnos de caja».",
	}
	if !slices.Equal(warnings, expected) {
		t.Fatalf("avisos de los cortes: %q", warnings)
	}
	for _, flag := range []string{FlagStartEstimated, FlagEndEstimated, FlagNoStart} {
		if _, found := flagInfo(EntityShift, flag); !found {
			t.Fatalf("la marca %s no está en la lista cerrada", flag)
		}
	}
	// Sin cortes así, el resumen no dice nada de ellos.
	if warnings := shiftDateWarnings(buildHistory(loadSynthetic(t, standardSynthetic()))); len(warnings) != 0 {
		t.Fatalf("avisos sin cortes sin fecha: %q", warnings)
	}
}
