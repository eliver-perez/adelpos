package businessday

import (
	"fmt"
	"regexp"
	"strconv"
	"time"
)

// Periodos de los reportes (§7 de la etapa 9A). Todos se expresan en días comerciales completos y se
// convierten a instantes [desde, hasta) con la hora de inicio del día y la zona horaria vigentes:
//
//   - today, yesterday: el día comercial en curso y el anterior;
//   - current_week, current_month, current_year: la semana ISO 8601 (lunes a domingo), el mes y el
//     año del día comercial en curso, completos (el periodo puede estar «en curso»);
//   - day (date=AAAA-MM-DD), iso_week (week=AAAA-Www), month (month=AAAA-MM), year (year=AAAA);
//   - range (from, until: AAAA-MM-DD, ambos incluidos), hasta MaxRangeDays días.
//
// La semana del año es ISO 8601: empieza en lunes y la semana 1 es la que contiene el primer jueves
// del año, por lo que el 1 de enero puede pertenecer a la última semana del año anterior y algunos
// años tienen semana 53.

// MaxRangeDays es la mayor duración de un periodo personalizado (un año bisiesto y holgura para
// comparar el mismo mes de dos años).
const MaxRangeDays = 400

// Clases de periodo admitidas.
const (
	KindToday        = "today"
	KindYesterday    = "yesterday"
	KindCurrentWeek  = "current_week"
	KindCurrentMonth = "current_month"
	KindCurrentYear  = "current_year"
	KindDay          = "day"
	KindISOWeek      = "iso_week"
	KindMonth        = "month"
	KindYear         = "year"
	KindRange        = "range"
)

// Spec es la solicitud de periodo tal como llega en los parámetros de consulta.
type Spec struct {
	Kind  string
	Date  string
	Week  string
	Month string
	Year  string
	From  string
	Until string
}

// Period es un periodo resuelto: días comerciales incluidos, instantes de filtrado y su etiqueta.
type Period struct {
	Kind       string    `json:"kind"`
	Label      string    `json:"label"`
	FirstDay   string    `json:"first_day"`
	LastDay    string    `json:"last_day"`
	From       time.Time `json:"from"`
	Until      time.Time `json:"until"`
	Days       int       `json:"days"`
	ISOYear    int       `json:"iso_year,omitempty"`
	ISOWeek    int       `json:"iso_week,omitempty"`
	InProgress bool      `json:"in_progress"`

	first time.Time
	last  time.Time
}

// First y Last son las fechas comerciales (medianoche UTC) del primer y último día del periodo.
func (period Period) First() time.Time {
	return period.first
}

func (period Period) Last() time.Time {
	return period.last
}

// SpecError es un periodo mal formado; su mensaje está en español y se muestra tal cual.
type SpecError struct {
	Message string
}

func (problem *SpecError) Error() string {
	return problem.Message
}

func specError(message string) error {
	return &SpecError{Message: message}
}

var (
	datePattern  = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}$`)
	weekPattern  = regexp.MustCompile(`^([0-9]{4})-W([0-9]{2})$`)
	monthPattern = regexp.MustCompile(`^([0-9]{4})-([0-9]{2})$`)
	yearPattern  = regexp.MustCompile(`^[0-9]{4}$`)
)

// ParseDate interpreta AAAA-MM-DD como fecha comercial (medianoche UTC). Rechaza fechas que no
// existen (por ejemplo, 2026-02-30) y años fuera de 2000–2100.
func ParseDate(text string) (time.Time, error) {
	if !datePattern.MatchString(text) {
		return time.Time{}, specError("La fecha debe tener el formato AAAA-MM-DD.")
	}
	date, err := time.Parse("2006-01-02", text)
	if err != nil || date.Format("2006-01-02") != text {
		return time.Time{}, specError("La fecha " + text + " no existe.")
	}
	if date.Year() < 2000 || date.Year() > 2100 {
		return time.Time{}, specError("La fecha debe estar entre los años 2000 y 2100.")
	}
	return date, nil
}

// ISOWeekStart devuelve el lunes (fecha comercial) de la semana ISO indicada.
func ISOWeekStart(year, week int) (time.Time, error) {
	if year < 2000 || year > 2100 {
		return time.Time{}, specError("El año debe estar entre 2000 y 2100.")
	}
	if week < 1 || week > WeeksInISOYear(year) {
		return time.Time{}, specError(fmt.Sprintf("El año %d tiene %d semanas.", year, WeeksInISOYear(year)))
	}
	// El 4 de enero siempre pertenece a la semana 1.
	january4 := time.Date(year, time.January, 4, 0, 0, 0, 0, time.UTC)
	offset := (int(january4.Weekday()) + 6) % 7
	monday := january4.AddDate(0, 0, -offset)
	return monday.AddDate(0, 0, (week-1)*7), nil
}

// WeeksInISOYear devuelve 52 o 53: el 28 de diciembre siempre está en la última semana ISO del año.
func WeeksInISOYear(year int) int {
	_, week := time.Date(year, time.December, 28, 0, 0, 0, 0, time.UTC).ISOWeek()
	return week
}

// Resolve convierte la solicitud en un periodo con la configuración y el instante actual.
func Resolve(spec Spec, now time.Time, config Config) (Period, error) {
	today := config.DayOf(now).Date
	kind := spec.Kind
	if kind == "" {
		kind = KindToday
	}
	switch kind {
	case KindToday:
		return build(KindToday, today, today, now, config), nil
	case KindYesterday:
		yesterday := today.AddDate(0, 0, -1)
		return build(KindYesterday, yesterday, yesterday, now, config), nil
	case KindCurrentWeek:
		year, week := today.ISOWeek()
		return weekPeriod(KindCurrentWeek, year, week, now, config)
	case KindCurrentMonth:
		return monthPeriod(KindCurrentMonth, today.Year(), today.Month(), now, config), nil
	case KindCurrentYear:
		return yearPeriod(KindCurrentYear, today.Year(), now, config), nil
	case KindDay:
		date, err := ParseDate(spec.Date)
		if err != nil {
			return Period{}, err
		}
		return build(KindDay, date, date, now, config), nil
	case KindISOWeek:
		match := weekPattern.FindStringSubmatch(spec.Week)
		if match == nil {
			return Period{}, specError("La semana debe tener el formato AAAA-Wss (por ejemplo, 2026-W46).")
		}
		year, _ := strconv.Atoi(match[1])
		week, _ := strconv.Atoi(match[2])
		return weekPeriod(KindISOWeek, year, week, now, config)
	case KindMonth:
		match := monthPattern.FindStringSubmatch(spec.Month)
		if match == nil {
			return Period{}, specError("El mes debe tener el formato AAAA-MM.")
		}
		year, _ := strconv.Atoi(match[1])
		month, _ := strconv.Atoi(match[2])
		if month < 1 || month > 12 || year < 2000 || year > 2100 {
			return Period{}, specError("El mes indicado no existe.")
		}
		return monthPeriod(KindMonth, year, time.Month(month), now, config), nil
	case KindYear:
		if !yearPattern.MatchString(spec.Year) {
			return Period{}, specError("El año debe tener cuatro dígitos.")
		}
		year, _ := strconv.Atoi(spec.Year)
		if year < 2000 || year > 2100 {
			return Period{}, specError("El año debe estar entre 2000 y 2100.")
		}
		return yearPeriod(KindYear, year, now, config), nil
	case KindRange:
		first, err := ParseDate(spec.From)
		if err != nil {
			return Period{}, err
		}
		last, err := ParseDate(spec.Until)
		if err != nil {
			return Period{}, err
		}
		if last.Before(first) {
			return Period{}, specError("La fecha final no puede ser anterior a la inicial.")
		}
		if int(last.Sub(first).Hours()/24)+1 > MaxRangeDays {
			return Period{}, specError(fmt.Sprintf("El periodo no puede exceder %d días.", MaxRangeDays))
		}
		return build(KindRange, first, last, now, config), nil
	}
	return Period{}, specError("El tipo de periodo no es válido.")
}

func weekPeriod(kind string, year, week int, now time.Time, config Config) (Period, error) {
	monday, err := ISOWeekStart(year, week)
	if err != nil {
		return Period{}, err
	}
	period := build(kind, monday, monday.AddDate(0, 0, 6), now, config)
	period.ISOYear, period.ISOWeek = year, week
	period.Label = WeekLabel(year, week, monday)
	return period, nil
}

func monthPeriod(kind string, year int, month time.Month, now time.Time, config Config) Period {
	first := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
	period := build(kind, first, first.AddDate(0, 1, -1), now, config)
	period.Label = MonthLabel(year, month)
	return period
}

func yearPeriod(kind string, year int, now time.Time, config Config) Period {
	first := time.Date(year, time.January, 1, 0, 0, 0, 0, time.UTC)
	period := build(kind, first, time.Date(year, time.December, 31, 0, 0, 0, 0, time.UTC), now, config)
	period.Label = "Año " + strconv.Itoa(year)
	return period
}

// build arma el periodo de los días first..last con sus instantes y una etiqueta por omisión.
func build(kind string, first, last, now time.Time, config Config) Period {
	from, until := config.Bounds(first, last)
	today := config.DayOf(now).Date
	period := Period{
		Kind:       kind,
		FirstDay:   first.Format("2006-01-02"),
		LastDay:    last.Format("2006-01-02"),
		From:       from.UTC(),
		Until:      until.UTC(),
		Days:       int(last.Sub(first).Hours()/24) + 1,
		InProgress: !today.Before(first) && !today.After(last),
		first:      first,
		last:       last,
	}
	switch {
	case kind == KindToday:
		period.Label = "Hoy, " + DayLabel(first, false)
	case kind == KindYesterday:
		period.Label = "Ayer, " + DayLabel(first, false)
	case first.Equal(last):
		period.Label = DayLabel(first, true)
	default:
		period.Label = RangeLabel(first, last)
	}
	return period
}

// Previous devuelve el periodo anterior equivalente: el día, la semana ISO, el mes o el año
// anteriores, o, en un rango, los mismos días inmediatamente antes.
func (period Period) Previous(now time.Time, config Config) Period {
	switch period.Kind {
	case KindToday, KindYesterday, KindDay:
		previous := period.first.AddDate(0, 0, -1)
		return build(KindDay, previous, previous, now, config)
	case KindCurrentWeek, KindISOWeek:
		year, week := period.first.AddDate(0, 0, -7).ISOWeek()
		previous, _ := weekPeriod(KindISOWeek, year, week, now, config)
		return previous
	case KindCurrentMonth, KindMonth:
		previous := period.first.AddDate(0, -1, 0)
		return monthPeriod(KindMonth, previous.Year(), previous.Month(), now, config)
	case KindCurrentYear, KindYear:
		return yearPeriod(KindYear, period.first.Year()-1, now, config)
	}
	last := period.first.AddDate(0, 0, -1)
	first := last.AddDate(0, 0, -(period.Days - 1))
	return build(KindRange, first, last, now, config)
}

var monthNames = []string{
	"enero", "febrero", "marzo", "abril", "mayo", "junio", "julio", "agosto", "septiembre", "octubre", "noviembre",
	"diciembre",
}

var weekdayNames = []string{"domingo", "lunes", "martes", "miércoles", "jueves", "viernes", "sábado"}

func capitalize(text string) string {
	if text == "" {
		return text
	}
	runes := []rune(text)
	first := string(runes[0])
	switch first {
	case "á":
		first = "Á"
	case "é":
		first = "É"
	default:
		if runes[0] >= 'a' && runes[0] <= 'z' {
			first = string(runes[0] - 'a' + 'A')
		}
	}
	return first + string(runes[1:])
}

// MonthLabel es «Noviembre de 2026».
func MonthLabel(year int, month time.Month) string {
	return capitalize(monthNames[month-1]) + " de " + strconv.Itoa(year)
}

// DayLabel es «martes 10 de noviembre de 2026» (con mayúscula inicial si se pide).
func DayLabel(date time.Time, capitalized bool) string {
	label := weekdayNames[date.Weekday()] + " " + strconv.Itoa(date.Day()) + " de " + monthNames[date.Month()-1] + " de " +
		strconv.Itoa(date.Year())
	if capitalized {
		return capitalize(label)
	}
	return label
}

// RangeLabel es «Del 1 al 15 de noviembre de 2026» o, entre meses o años distintos, «Del 28 de
// diciembre de 2026 al 3 de enero de 2027».
func RangeLabel(first, last time.Time) string {
	end := strconv.Itoa(last.Day()) + " de " + monthNames[last.Month()-1] + " de " + strconv.Itoa(last.Year())
	if first.Year() == last.Year() && first.Month() == last.Month() {
		return "Del " + strconv.Itoa(first.Day()) + " al " + end
	}
	start := strconv.Itoa(first.Day()) + " de " + monthNames[first.Month()-1]
	if first.Year() != last.Year() {
		start += " de " + strconv.Itoa(first.Year())
	}
	return "Del " + start + " al " + end
}

// WeekLabel es «Semana 46 de 2026 (del 9 al 15 de noviembre de 2026)».
func WeekLabel(year, week int, monday time.Time) string {
	sunday := monday.AddDate(0, 0, 6)
	rangeText := RangeLabel(monday, sunday)
	return fmt.Sprintf("Semana %d de %d (%s)", week, year, "d"+rangeText[1:])
}
