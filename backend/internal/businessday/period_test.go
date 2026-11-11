package businessday

import (
	"errors"
	"testing"
	"time"
)

func monterreyConfig(t *testing.T, hour int) Config {
	t.Helper()
	return Config{Timezone: "America/Monterrey", Location: mustLocation(t, "America/Monterrey"), Hour: hour}
}

// Semana ISO 8601: 2026 tiene 53 semanas (empieza en jueves); la semana 53 cruza al año 2027 y el
// 1 de enero de 2027 pertenece a ella; la semana 1 de 2025 empieza el 30 de diciembre de 2024.
func TestISOWeeksWithYearChange(t *testing.T) {
	if WeeksInISOYear(2026) != 53 || WeeksInISOYear(2027) != 52 || WeeksInISOYear(2020) != 53 || WeeksInISOYear(2025) != 52 {
		t.Fatalf("Semanas por año ISO: 2026=%d 2027=%d 2020=%d 2025=%d", WeeksInISOYear(2026), WeeksInISOYear(2027),
			WeeksInISOYear(2020), WeeksInISOYear(2025))
	}
	config := monterreyConfig(t, 5)
	now := mustTime(t, "2026-11-10T12:00:00-06:00")
	week53, err := Resolve(Spec{Kind: KindISOWeek, Week: "2026-W53"}, now, config)
	if err != nil {
		t.Fatal(err)
	}
	if week53.FirstDay != "2026-12-28" || week53.LastDay != "2027-01-03" || week53.Days != 7 || week53.ISOYear != 2026 || week53.ISOWeek != 53 {
		t.Fatalf("Semana 53 de 2026 inesperada: %+v", week53)
	}
	if week53.Label != "Semana 53 de 2026 (del 28 de diciembre de 2026 al 3 de enero de 2027)" {
		t.Fatalf("Etiqueta de la semana 53: %q", week53.Label)
	}
	if !week53.From.Equal(mustTime(t, "2026-12-28T11:00:00Z")) || !week53.Until.Equal(mustTime(t, "2027-01-04T11:00:00Z")) {
		t.Fatalf("Instantes de la semana 53: %s %s", week53.From, week53.Until)
	}
	if year, week := time.Date(2027, time.January, 1, 0, 0, 0, 0, time.UTC).ISOWeek(); year != 2026 || week != 53 {
		t.Fatal("El 1 de enero de 2027 debe pertenecer a la semana 53 de 2026")
	}
	first2025, err := Resolve(Spec{Kind: KindISOWeek, Week: "2025-W01"}, now, config)
	if err != nil || first2025.FirstDay != "2024-12-30" || first2025.LastDay != "2025-01-05" {
		t.Fatalf("Semana 1 de 2025: %+v %v", first2025, err)
	}
	// La semana siguiente a la 53 de 2026 es la 1 de 2027 y la anterior a la 1 de 2027 es la 53.
	first2027, err := Resolve(Spec{Kind: KindISOWeek, Week: "2027-W01"}, now, config)
	if err != nil || first2027.FirstDay != "2027-01-04" {
		t.Fatalf("Semana 1 de 2027: %+v %v", first2027, err)
	}
	previous := first2027.Previous(now, config)
	if previous.ISOYear != 2026 || previous.ISOWeek != 53 || previous.FirstDay != "2026-12-28" {
		t.Fatalf("La semana anterior a la 1 de 2027 debe ser la 53 de 2026: %+v", previous)
	}
	var problem *SpecError
	if _, err := Resolve(Spec{Kind: KindISOWeek, Week: "2027-W53"}, now, config); !errors.As(err, &problem) {
		t.Fatal("2027 no tiene semana 53")
	}
	if _, err := Resolve(Spec{Kind: KindISOWeek, Week: "2026-46"}, now, config); !errors.As(err, &problem) {
		t.Fatal("Se aceptó una semana sin el formato AAAA-Wss")
	}
}

func TestRelativePeriodsAndComparisons(t *testing.T) {
	config := monterreyConfig(t, 5)
	// A la 01:30 del 11 con inicio 05:00, «hoy» sigue siendo el día comercial 10.
	now := mustTime(t, "2026-11-11T01:30:00-06:00")
	today, err := Resolve(Spec{}, now, config)
	if err != nil {
		t.Fatal(err)
	}
	if today.Kind != KindToday || today.FirstDay != "2026-11-10" || !today.InProgress || today.Label != "Hoy, martes 10 de noviembre de 2026" {
		t.Fatalf("Hoy inesperado: %+v", today)
	}
	if !today.From.Equal(mustTime(t, "2026-11-10T11:00:00Z")) || !today.Until.Equal(mustTime(t, "2026-11-11T11:00:00Z")) {
		t.Fatalf("Instantes de hoy: %s %s", today.From, today.Until)
	}
	yesterday := today.Previous(now, config)
	if yesterday.FirstDay != "2026-11-09" || yesterday.InProgress {
		t.Fatalf("Ayer como periodo anterior: %+v", yesterday)
	}
	week, _ := Resolve(Spec{Kind: KindCurrentWeek}, now, config)
	if week.FirstDay != "2026-11-09" || week.LastDay != "2026-11-15" || week.ISOWeek != 46 || !week.InProgress {
		t.Fatalf("Semana actual: %+v", week)
	}
	if previousWeek := week.Previous(now, config); previousWeek.FirstDay != "2026-11-02" || previousWeek.ISOWeek != 45 {
		t.Fatalf("Semana anterior: %+v", previousWeek)
	}
	month, _ := Resolve(Spec{Kind: KindCurrentMonth}, now, config)
	if month.FirstDay != "2026-11-01" || month.LastDay != "2026-11-30" || month.Label != "Noviembre de 2026" || month.Days != 30 {
		t.Fatalf("Mes actual: %+v", month)
	}
	if previousMonth := month.Previous(now, config); previousMonth.FirstDay != "2026-10-01" || previousMonth.LastDay != "2026-10-31" {
		t.Fatalf("Mes anterior: %+v", previousMonth)
	}
	year, _ := Resolve(Spec{Kind: KindCurrentYear}, now, config)
	if year.Days != 365 || year.Label != "Año 2026" || year.Previous(now, config).FirstDay != "2025-01-01" {
		t.Fatalf("Año actual: %+v", year)
	}
	march, _ := Resolve(Spec{Kind: KindMonth, Month: "2025-03"}, now, config)
	if march.Previous(now, config).LastDay != "2025-02-28" || march.InProgress {
		t.Fatalf("El mes anterior a marzo de 2025 termina el 28 de febrero: %+v", march.Previous(now, config))
	}
	span, err := Resolve(Spec{Kind: KindRange, From: "2026-11-01", Until: "2026-11-15"}, now, config)
	if err != nil || span.Days != 15 || span.Label != "Del 1 al 15 de noviembre de 2026" {
		t.Fatalf("Rango: %+v %v", span, err)
	}
	if before := span.Previous(now, config); before.FirstDay != "2026-10-17" || before.LastDay != "2026-10-31" || before.Days != 15 {
		t.Fatalf("Rango anterior equivalente: %+v", before)
	}
	day, _ := Resolve(Spec{Kind: KindDay, Date: "2026-02-28"}, now, config)
	if day.Label != "Sábado 28 de febrero de 2026" || day.Previous(now, config).FirstDay != "2026-02-27" {
		t.Fatalf("Día: %+v", day)
	}
}

func TestInvalidPeriodsAreRejectedInSpanish(t *testing.T) {
	config := monterreyConfig(t, 0)
	now := mustTime(t, "2026-11-10T12:00:00-06:00")
	invalid := []Spec{
		{Kind: "fortnight"},
		{Kind: KindDay, Date: "2026-02-30"},
		{Kind: KindDay, Date: "10/11/2026"},
		{Kind: KindMonth, Month: "2026-13"},
		{Kind: KindYear, Year: "26"},
		{Kind: KindRange, From: "2026-11-15", Until: "2026-11-01"},
		{Kind: KindRange, From: "2025-01-01", Until: "2026-03-01"},
	}
	for _, spec := range invalid {
		_, err := Resolve(spec, now, config)
		var problem *SpecError
		if !errors.As(err, &problem) || problem.Message == "" {
			t.Fatalf("Se aceptó el periodo %+v", spec)
		}
	}
	// Con inicio a medianoche el día 10 completo es [06:00Z del 10, 06:00Z del 11).
	day, err := Resolve(Spec{Kind: KindDay, Date: "2026-11-10"}, now, config)
	if err != nil || !day.From.Equal(mustTime(t, "2026-11-10T06:00:00Z")) || !day.Until.Equal(mustTime(t, "2026-11-11T06:00:00Z")) {
		t.Fatalf("Día con inicio a medianoche: %+v %v", day, err)
	}
}
