package inventory

import (
	"testing"
	"time"
)

// Día operativo con zona horaria y hora de corte, incluidos los cambios de horario.

func mustLocation(t *testing.T, name string) *time.Location {
	t.Helper()
	location, err := time.LoadLocation(name)
	if err != nil {
		t.Fatal(err)
	}
	return location
}

func TestOperatingDayWithCutoff(t *testing.T) {
	// Monterrey no tiene horario de verano (UTC−6).
	monterrey := mustLocation(t, "America/Monterrey")
	cases := []struct {
		name     string
		instant  string
		hour     int
		minute   int
		date     string
		deadline string
	}{
		// 02:00 del 11 con corte a las 05:00 pertenece al día 10 y vence a las 05:00 del 11.
		{"antes del corte", "2026-11-11T02:00:00-06:00", 5, 0, "2026-11-10", "2026-11-11T11:00:00Z"},
		// Exactamente en el corte ya es el día nuevo.
		{"en el corte", "2026-11-11T05:00:00-06:00", 5, 0, "2026-11-11", "2026-11-12T11:00:00Z"},
		{"un minuto antes", "2026-11-11T04:59:00-06:00", 5, 0, "2026-11-10", "2026-11-11T11:00:00Z"},
		{"noche", "2026-11-10T23:59:00-06:00", 5, 0, "2026-11-10", "2026-11-11T11:00:00Z"},
		// Corte a medianoche: el día operativo es la fecha local.
		{"corte a medianoche", "2026-11-10T23:59:00-06:00", 0, 0, "2026-11-10", "2026-11-11T06:00:00Z"},
		// Corte con minutos (04:30).
		{"corte con minutos", "2026-11-11T04:29:00-06:00", 4, 30, "2026-11-10", "2026-11-11T10:30:00Z"},
		// Fin de mes y de año.
		{"fin de año", "2027-01-01T03:00:00-06:00", 5, 0, "2026-12-31", "2027-01-01T11:00:00Z"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			instant, err := time.Parse(time.RFC3339, testCase.instant)
			if err != nil {
				t.Fatal(err)
			}
			day := operatingDayOf(instant, monterrey, testCase.hour, testCase.minute)
			if day.Date.Format("2006-01-02") != testCase.date || !day.Deadline.Equal(mustTime(t, testCase.deadline)) {
				t.Fatalf("día %s y corte %s; esperado %s y %s", day.Date.Format("2006-01-02"), day.Deadline.UTC().Format(time.RFC3339),
					testCase.date, testCase.deadline)
			}
			if !day.Deadline.After(instant) {
				t.Fatal("el corte debe ser posterior al instante")
			}
		})
	}
}

func mustTime(t *testing.T, text string) time.Time {
	t.Helper()
	value, err := time.Parse(time.RFC3339, text)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestOperatingDayAcrossDaylightSaving(t *testing.T) {
	// Tijuana sigue el horario de verano de Estados Unidos: en 2026 adelanta el 8 de marzo a las
	// 02:00 (PST −8 → PDT −7) y atrasa el 1 de noviembre a las 02:00 (PDT → PST).
	tijuana := mustLocation(t, "America/Tijuana")
	cases := []struct {
		name     string
		instant  string
		hour     int
		minute   int
		date     string
		deadline string
	}{
		// Corte a las 05:00 el día del adelanto: el día operativo del 7 dura 23 horas.
		{"adelanto con corte 05:00", "2026-03-08T01:30:00-08:00", 5, 0, "2026-03-07", "2026-03-08T12:00:00Z"},
		// Corte a las 02:30, hora que no existe el 8 de marzo: el corte es el primer instante
		// después del salto (03:00 PDT = 10:00 UTC).
		{"corte inexistente antes", "2026-03-08T01:59:00-08:00", 2, 30, "2026-03-07", "2026-03-08T10:00:00Z"},
		{"corte inexistente después", "2026-03-08T03:10:00-07:00", 2, 30, "2026-03-08", "2026-03-09T09:30:00Z"},
		// Corte a la 01:30 el día del atraso (la hora ocurre dos veces): cuenta la primera.
		{"atraso primera pasada", "2026-11-01T01:45:00-07:00", 1, 30, "2026-11-01", "2026-11-02T09:30:00Z"},
		{"atraso antes del corte", "2026-11-01T01:15:00-07:00", 1, 30, "2026-10-31", "2026-11-01T08:30:00Z"},
		// 01:15 de la segunda pasada (PST) es posterior a la primera 01:30: ya es el día 1.
		{"atraso segunda pasada", "2026-11-01T01:15:00-08:00", 1, 30, "2026-11-01", "2026-11-02T09:30:00Z"},
		// Corte a las 05:00 el día del atraso: el día operativo del 31 dura 25 horas.
		{"atraso con corte 05:00", "2026-11-01T04:00:00-08:00", 5, 0, "2026-10-31", "2026-11-01T13:00:00Z"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			instant := mustTime(t, testCase.instant)
			day := operatingDayOf(instant, tijuana, testCase.hour, testCase.minute)
			if day.Date.Format("2006-01-02") != testCase.date || !day.Deadline.Equal(mustTime(t, testCase.deadline)) {
				t.Fatalf("día %s y corte %s; esperado %s y %s", day.Date.Format("2006-01-02"), day.Deadline.UTC().Format(time.RFC3339),
					testCase.date, testCase.deadline)
			}
			if !day.Deadline.After(instant) {
				t.Fatal("el corte debe ser posterior al instante")
			}
		})
	}
}
