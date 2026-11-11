package businessday

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

func mustTime(t *testing.T, text string) time.Time {
	t.Helper()
	value, err := time.Parse(time.RFC3339, text)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

// §0-B: con 00:00 el día comercial es la fecha del calendario; con 05:00, una venta de la 01:30
// pertenece al día anterior.
func TestSaleAtHalfPastOneWithBothCutoffs(t *testing.T) {
	monterrey := mustLocation(t, "America/Monterrey")
	sale := mustTime(t, "2026-11-11T01:30:00-06:00")
	midnight := Config{Timezone: "America/Monterrey", Location: monterrey, Hour: 0, Minute: 0}
	five := Config{Timezone: "America/Monterrey", Location: monterrey, Hour: 5, Minute: 0}
	if day := midnight.DayOf(sale).Date.Format("2006-01-02"); day != "2026-11-11" {
		t.Fatalf("Con inicio 00:00 la venta de la 01:30 debe ser del día 11: %s", day)
	}
	if day := five.DayOf(sale).Date.Format("2006-01-02"); day != "2026-11-10" {
		t.Fatalf("Con inicio 05:00 la venta de la 01:30 debe ser del día 10: %s", day)
	}
	// El periodo «día 10» con inicio 05:00 incluye la venta; con 00:00, no.
	date := time.Date(2026, time.November, 10, 0, 0, 0, 0, time.UTC)
	from, until := five.Bounds(date, date)
	if sale.Before(from) || !sale.Before(until) {
		t.Fatalf("El día 10 con inicio 05:00 debe incluir la venta: [%s, %s)", from, until)
	}
	from, until = midnight.Bounds(date, date)
	if !sale.Before(from) && sale.Before(until) {
		t.Fatal("El día 10 con inicio 00:00 no debe incluir la venta de la 01:30 del 11")
	}
	if five.Cutoff() != "05:00" || five.Interval() != "05:00:00" || midnight.Cutoff() != "00:00" {
		t.Fatal("La hora de inicio no se expresa como HH:MM y como intervalo")
	}
}

func TestDayOfWithCutoff(t *testing.T) {
	monterrey := mustLocation(t, "America/Monterrey")
	cases := []struct {
		name     string
		instant  string
		hour     int
		minute   int
		date     string
		deadline string
	}{
		{"antes del corte", "2026-11-11T02:00:00-06:00", 5, 0, "2026-11-10", "2026-11-11T11:00:00Z"},
		{"en el corte", "2026-11-11T05:00:00-06:00", 5, 0, "2026-11-11", "2026-11-12T11:00:00Z"},
		{"un minuto antes", "2026-11-11T04:59:00-06:00", 5, 0, "2026-11-10", "2026-11-11T11:00:00Z"},
		{"corte a medianoche", "2026-11-10T23:59:00-06:00", 0, 0, "2026-11-10", "2026-11-11T06:00:00Z"},
		{"corte con minutos", "2026-11-11T04:29:00-06:00", 4, 30, "2026-11-10", "2026-11-11T10:30:00Z"},
		{"fin de año", "2027-01-01T03:00:00-06:00", 5, 0, "2026-12-31", "2027-01-01T11:00:00Z"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			instant := mustTime(t, testCase.instant)
			day := DayOf(instant, monterrey, testCase.hour, testCase.minute)
			if day.Date.Format("2006-01-02") != testCase.date || !day.Deadline.Equal(mustTime(t, testCase.deadline)) {
				t.Fatalf("día %s y fin %s; esperado %s y %s", day.Date.Format("2006-01-02"), day.Deadline.UTC().Format(time.RFC3339),
					testCase.date, testCase.deadline)
			}
		})
	}
}

// Cambio de horario: los límites del día comercial siguen al reloj local, por lo que el día del
// adelanto dura 23 horas y el del atraso 25.
func TestBoundsAcrossDaylightSaving(t *testing.T) {
	tijuana := Config{Timezone: "America/Tijuana", Location: mustLocation(t, "America/Tijuana"), Hour: 5}
	cases := []struct {
		date  time.Time
		from  string
		until string
		hours float64
	}{
		{time.Date(2026, time.March, 7, 0, 0, 0, 0, time.UTC), "2026-03-07T13:00:00Z", "2026-03-08T12:00:00Z", 23},
		{time.Date(2026, time.October, 31, 0, 0, 0, 0, time.UTC), "2026-10-31T12:00:00Z", "2026-11-01T13:00:00Z", 25},
		{time.Date(2026, time.November, 10, 0, 0, 0, 0, time.UTC), "2026-11-10T13:00:00Z", "2026-11-11T13:00:00Z", 24},
	}
	for _, testCase := range cases {
		from, until := tijuana.Bounds(testCase.date, testCase.date)
		if !from.Equal(mustTime(t, testCase.from)) || !until.Equal(mustTime(t, testCase.until)) || until.Sub(from).Hours() != testCase.hours {
			t.Fatalf("%s: [%s, %s)", testCase.date.Format("2006-01-02"), from.UTC().Format(time.RFC3339), until.UTC().Format(time.RFC3339))
		}
	}
	// Un instante de la hora repetida del atraso (01:30 PST, segunda pasada) con inicio a la 01:30
	// pertenece ya al día 1 (el inicio cuenta en su primera aparición).
	early := Config{Timezone: "America/Tijuana", Location: tijuana.Location, Hour: 1, Minute: 30}
	if day := early.DayOf(mustTime(t, "2026-11-01T01:15:00-08:00")).Date.Format("2006-01-02"); day != "2026-11-01" {
		t.Fatalf("La segunda pasada de la hora repetida debe ser del día 1: %s", day)
	}
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()
	if config.Timezone != "UTC" || config.Hour != 5 || config.Minute != 0 || config.Location != time.UTC {
		t.Fatalf("Configuración por omisión inesperada: %+v", config)
	}
}
