package licensing

import (
	"testing"
	"time"
)

// La regla de la §6.2 con sus límites exactos: el bloqueo empieza cuando el reloj queda más de 10 minutos antes de la
// última hora confiable, y sin hora confiable nunca se bloquea.
func TestCheckClockMargin(t *testing.T) {
	trusted := time.Date(2026, 11, 23, 18, 40, 0, 0, time.UTC)
	cases := []struct {
		name    string
		local   time.Time
		blocked bool
	}{
		{"igual", trusted, false},
		{"adelantado", trusted.Add(48 * time.Hour), false},
		{"9 min antes", trusted.Add(-9 * time.Minute), false},
		{"exactamente 10 min antes", trusted.Add(-ClockMargin), false},
		{"10 min y 1 ns antes", trusted.Add(-ClockMargin - time.Nanosecond), true},
		{"2 h 25 min antes", trusted.Add(-145 * time.Minute), true},
	}
	for _, item := range cases {
		status := CheckClock(item.local, trusted)
		if status.Blocked != item.blocked || !status.LastActivity.Equal(trusted) || status.Margin != ClockMargin {
			t.Errorf("%s: %+v", item.name, status)
		}
	}
	if CheckClock(trusted, time.Time{}).Blocked {
		t.Fatal("sin hora confiable no se bloquea")
	}
}

// El gestor revisa con su reloj (el simulado) y su última hora confiable; al corregir el reloj el bloqueo se quita
// solo, sin guardar nada.
func TestManagerClockBlocksAndUnblocks(t *testing.T) {
	h := newHarness(t)
	manager := h.start()
	h.accept(manager, "perpetual-0-months")
	manager.ObserveTime(h.clock.now(), TimeFromClock)
	trusted := manager.TrustedTime()
	if manager.Clock().Blocked {
		t.Fatal("con el reloj correcto no hay bloqueo")
	}
	h.clock.set("2026-11-21T09:00:00Z")
	blocked := manager.Clock()
	if !blocked.Blocked || !blocked.LastActivity.Equal(trusted) || !blocked.LocalTime.Equal(h.clock.now()) {
		t.Fatalf("con el reloj 3 h atrás se bloquea: %+v", blocked)
	}
	// Atrasar el reloj nunca baja la última hora confiable (ObserveTime solo sube).
	manager.ObserveTime(h.clock.now(), TimeFromClock)
	if !manager.TrustedTime().Equal(trusted) {
		t.Fatal("la última hora confiable bajó con el reloj atrasado")
	}
	h.clock.set("2026-11-21T11:55:00Z")
	if manager.Clock().Blocked {
		t.Fatal("dentro del margen el bloqueo se quita solo")
	}
}

// Los textos de la pantalla de la §6.2: fecha completa en la zona horaria del restaurante.
func TestScreenTime(t *testing.T) {
	monterrey, err := time.LoadLocation("America/Monterrey")
	if err != nil {
		t.Skip("sin la base de zonas horarias")
	}
	instant := time.Date(2026, 11, 23, 16, 15, 0, 0, time.UTC)
	if got := ScreenTime(instant, monterrey); got != "lunes 23 nov 2026, 10:15" {
		t.Fatalf("texto: %q", got)
	}
	if got := ScreenTime(time.Date(2027, 1, 3, 9, 5, 0, 0, time.UTC), nil); got != "domingo 3 ene 2027, 09:05" {
		t.Fatalf("texto en UTC: %q", got)
	}
	if ScreenTime(time.Time{}, monterrey) != "" {
		t.Fatal("sin hora no hay texto")
	}
}

// §6.3: coinciden dentro del margen, en las dos direcciones.
func TestServerTimeMatches(t *testing.T) {
	server := time.Date(2026, 11, 23, 18, 0, 0, 0, time.UTC)
	for _, item := range []struct {
		offset  time.Duration
		matches bool
	}{
		{0, true}, {9 * time.Minute, true}, {-ClockMargin, true}, {ClockMargin, true},
		{ClockMargin + time.Second, false}, {-ClockMargin - time.Second, false}, {-48 * time.Hour, false},
	} {
		if ServerTimeMatches(server.Add(item.offset), server) != item.matches {
			t.Errorf("diferencia %v: se esperaba %v", item.offset, item.matches)
		}
	}
}
