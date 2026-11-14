package backupschedule

import (
	"testing"
	"time"
)

func mustLocation(t *testing.T, name string) *time.Location {
	t.Helper()
	location, err := time.LoadLocation(name)
	if err != nil {
		t.Fatal(err)
	}
	return location
}

// El instante programado es la hora configurada del día local en la zona del establecimiento: antes de esa hora toca
// el del día anterior y después, el del mismo día.
func TestSlotsFollowTheLocalDay(t *testing.T) {
	monterrey := mustLocation(t, "America/Monterrey")
	cases := []struct {
		now        time.Time
		last, next time.Time
	}{
		{
			now:  time.Date(2026, 11, 14, 4, 59, 0, 0, monterrey),
			last: time.Date(2026, 11, 13, 5, 0, 0, 0, monterrey),
			next: time.Date(2026, 11, 14, 5, 0, 0, 0, monterrey),
		},
		{
			now:  time.Date(2026, 11, 14, 5, 0, 0, 0, monterrey),
			last: time.Date(2026, 11, 14, 5, 0, 0, 0, monterrey),
			next: time.Date(2026, 11, 15, 5, 0, 0, 0, monterrey),
		},
		{
			now:  time.Date(2026, 11, 14, 23, 30, 0, 0, monterrey),
			last: time.Date(2026, 11, 14, 5, 0, 0, 0, monterrey),
			next: time.Date(2026, 11, 15, 5, 0, 0, 0, monterrey),
		},
		// Fin de mes y de año.
		{
			now:  time.Date(2027, 1, 1, 2, 0, 0, 0, monterrey),
			last: time.Date(2026, 12, 31, 5, 0, 0, 0, monterrey),
			next: time.Date(2027, 1, 1, 5, 0, 0, 0, monterrey),
		},
	}
	for _, item := range cases {
		if got := LastSlot(item.now, monterrey, 5, 0); !got.Equal(item.last) {
			t.Fatalf("último instante para %v: %v, esperado %v", item.now, got, item.last)
		}
		if got := NextSlot(item.now, monterrey, 5, 0); !got.Equal(item.next) {
			t.Fatalf("siguiente instante para %v: %v, esperado %v", item.now, got, item.next)
		}
	}
}

// La zona horaria es la del establecimiento, no la del proceso: el mismo instante UTC da otro día local.
func TestSlotsUseTheEstablishmentTimezone(t *testing.T) {
	instant := time.Date(2026, 11, 14, 10, 30, 0, 0, time.UTC)
	tokyo := mustLocation(t, "Asia/Tokyo")            // 19:30 del 14
	monterrey := mustLocation(t, "America/Monterrey") // 04:30 del 14
	if got := LastSlot(instant, tokyo, 5, 0); !got.Equal(time.Date(2026, 11, 14, 5, 0, 0, 0, tokyo)) {
		t.Fatalf("Tokio: %v", got)
	}
	if got := LastSlot(instant, monterrey, 5, 0); !got.Equal(time.Date(2026, 11, 13, 5, 0, 0, 0, monterrey)) {
		t.Fatalf("Monterrey: %v", got)
	}
}

// Cambio de horario (adelanto): una hora que no existe ese día pasa al primer instante después del salto; los días
// vecinos conservan su hora local.
func TestSlotsOnSpringForward(t *testing.T) {
	tijuana := mustLocation(t, "America/Tijuana")
	// El 8 de marzo de 2026 a las 02:00 los relojes de Tijuana pasan a las 03:00.
	before := time.Date(2026, 3, 8, 1, 0, 0, 0, tijuana)
	next := NextSlot(before, tijuana, 2, 30)
	if want := time.Date(2026, 3, 8, 10, 0, 0, 0, time.UTC); !next.Equal(want) {
		t.Fatalf("02:30 inexistente: %v, esperado %v (03:00 local)", next.UTC(), want)
	}
	if local := next.In(tijuana); local.Hour() != 3 || local.Minute() != 0 {
		t.Fatalf("hora local después del salto: %v", local)
	}
	after := NextSlot(next, tijuana, 2, 30)
	if local := after.In(tijuana); local.Day() != 9 || local.Hour() != 2 || local.Minute() != 30 {
		t.Fatalf("el día siguiente vuelve a las 02:30: %v", local)
	}
	// Una hora que sí existe se conserva el mismo día del cambio.
	if local := NextSlot(before, tijuana, 5, 0).In(tijuana); local.Day() != 8 || local.Hour() != 5 {
		t.Fatalf("05:00 del día del cambio: %v", local)
	}
	if gap := NextSlot(before, tijuana, 5, 0).Sub(LastSlot(before, tijuana, 5, 0)); gap != 23*time.Hour {
		t.Fatalf("el día del adelanto dura 23 h entre respaldos: %v", gap)
	}
}

// Cambio de horario (atraso): la hora que ocurre dos veces se toma la primera vez; la segunda vez el último instante
// programado sigue siendo el mismo, así que no se repite el respaldo.
func TestSlotsOnFallBack(t *testing.T) {
	tijuana := mustLocation(t, "America/Tijuana")
	// El 1 de noviembre de 2026 a las 02:00 los relojes de Tijuana vuelven a la 01:00.
	first := time.Date(2026, 11, 1, 8, 30, 0, 0, time.UTC) // 01:30 PDT
	second := first.Add(time.Hour)                         // 01:30 PST
	if local := second.In(tijuana); local.Hour() != 1 || local.Minute() != 30 {
		t.Fatalf("la 01:30 se repite: %v", local)
	}
	slot := NextSlot(first.Add(-time.Minute), tijuana, 1, 30)
	if !slot.Equal(first) {
		t.Fatalf("primera 01:30: %v, esperado %v", slot, first)
	}
	if last := LastSlot(second.Add(time.Minute), tijuana, 1, 30); !last.Equal(first) {
		t.Fatalf("en la segunda 01:30 el último instante sigue siendo la primera: %v", last)
	}
	if next := NextSlot(second, tijuana, 1, 30); next.In(tijuana).Day() != 2 {
		t.Fatalf("el siguiente es al día siguiente: %v", next.In(tijuana))
	}
	if gap := NextSlot(first, tijuana, 5, 0).Sub(LastSlot(first, tijuana, 5, 0)); gap != 25*time.Hour {
		t.Fatalf("el día del atraso dura 25 h entre respaldos: %v", gap)
	}
}

// Toca respaldar si no hay un automático correcto desde el último instante programado; un fallo se reintenta
// después de RetryDelay y como máximo MaxAttempts veces; nunca con uno en curso.
func TestDue(t *testing.T) {
	now := time.Date(2026, 11, 14, 9, 0, 0, 0, time.UTC)
	recent := now.Add(-10 * time.Minute)
	old := now.Add(-2 * time.Hour)
	cases := []struct {
		name    string
		history SlotHistory
		due     bool
	}{
		{"sin respaldos (la computadora estaba apagada)", SlotHistory{}, true},
		{"ya hay uno correcto", SlotHistory{Succeeded: 1, Attempts: 1, LastFinished: &recent}, false},
		{"uno en curso", SlotHistory{Running: true}, false},
		{"falló hace poco", SlotHistory{Attempts: 1, LastFinished: &recent}, false},
		{"falló hace más de una hora", SlotHistory{Attempts: 1, LastFinished: &old}, true},
		{"sin intentos restantes", SlotHistory{Attempts: MaxAttempts, LastFinished: &old}, false},
	}
	for _, item := range cases {
		if got := Due(now, item.history); got != item.due {
			t.Fatalf("%s: %v, esperado %v", item.name, got, item.due)
		}
	}
}

func TestParseTime(t *testing.T) {
	hour, minute, err := ParseTime("23:45")
	if err != nil || hour != 23 || minute != 45 {
		t.Fatalf("23:45: %d %d %v", hour, minute, err)
	}
	for _, invalid := range []string{"", "24:00", "5:00", "05:60", "05:00:00", "aa:bb", " 05:00"} {
		if _, _, err := ParseTime(invalid); err == nil {
			t.Fatalf("hora aceptada: %q", invalid)
		}
	}
	if !ValidTime("") || !ValidTime("00:00") || ValidTime("25:00") {
		t.Fatal("ValidTime")
	}
}
