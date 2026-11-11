import type { PeriodKind, ReportQuery, ResolvedPeriod } from '../../services/reports';

// Selección de periodo de los reportes (etapa 9A, §7 y §0-B). La pantalla solo arma los parámetros;
// Go resuelve el periodo con la zona horaria y la hora de inicio del día del establecimiento y
// devuelve en los metadatos los días comerciales, los instantes y su etiqueta, que es lo que se
// muestra como periodo aplicado.

export interface PeriodSelection {
  period: PeriodKind;
  // Día comercial (AAAA-MM-DD) del periodo «day».
  date: string;
  // Semana ISO 8601 (año ISO y número) del periodo «iso_week».
  isoYear: string;
  isoWeek: string;
  // Mes (1–12) y año del periodo «month»; año del periodo «year».
  month: string;
  year: string;
  // Rango personalizado, días comerciales incluidos (AAAA-MM-DD).
  from: string;
  until: string;
  // Comparar con el periodo anterior equivalente (solo si el reporte lo admite).
  compare: boolean;
}

export const defaultPeriod: PeriodSelection = {
  period: 'today',
  date: '',
  isoYear: '',
  isoWeek: '',
  month: '',
  year: '',
  from: '',
  until: '',
  compare: true,
};

// Periodos que se aplican con un toque; los demás necesitan elegir un día, una semana, un mes, un
// año o un rango.
export const immediatePeriods: PeriodKind[] = [
  'today',
  'yesterday',
  'current_week',
  'current_month',
  'current_year',
];

// Rótulos cortos para el control segmentado; el resto usa la etiqueta que envía Go en el catálogo.
export const shortPeriodLabels: Partial<Record<PeriodKind, string>> = {
  today: 'Hoy',
  yesterday: 'Ayer',
  current_week: 'Semana actual',
  current_month: 'Mes actual',
  current_year: 'Año actual',
  day: 'Día',
  iso_week: 'Semana del año',
  month: 'Mes',
  year: 'Año',
  range: 'Personalizado',
};

export const monthNames = [
  'enero',
  'febrero',
  'marzo',
  'abril',
  'mayo',
  'junio',
  'julio',
  'agosto',
  'septiembre',
  'octubre',
  'noviembre',
  'diciembre',
];

function twoDigits(value: number) {
  return String(value).padStart(2, '0');
}

function isoDate(date: Date) {
  return `${date.getUTCFullYear()}-${twoDigits(date.getUTCMonth() + 1)}-${twoDigits(date.getUTCDate())}`;
}

function parseISODate(value: string) {
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(value);
  if (!match) {
    return null;
  }
  return new Date(Date.UTC(Number(match[1]), Number(match[2]) - 1, Number(match[3])));
}

function yearNumber(text: string) {
  if (!/^\d{4}$/.test(text)) {
    return null;
  }
  const year = Number(text);
  return year >= 2000 && year <= 2100 ? year : null;
}

// Lunes de la semana 1 de un año ISO: la semana que contiene el 4 de enero.
function isoWeekOneMonday(year: number) {
  const fourth = new Date(Date.UTC(year, 0, 4));
  const weekday = (fourth.getUTCDay() + 6) % 7;
  return new Date(Date.UTC(year, 0, 4 - weekday));
}

// Semanas del año ISO (52 o 53): tiene 53 si el 28 de diciembre cae en la semana 53.
export function isoWeeksInYear(year: number) {
  const nextMonday = isoWeekOneMonday(year + 1);
  const monday = isoWeekOneMonday(year);
  return Math.round((nextMonday.getTime() - monday.getTime()) / (7 * 86400000));
}

// Días de calendario de una semana ISO 8601 (lunes a domingo). Solo describe la elección antes de
// consultar: el periodo que vale es el que resuelve Go con la hora de inicio del día.
export function isoWeekRange(yearText: string, weekText: string) {
  const year = yearNumber(yearText);
  if (year === null || !/^\d{1,2}$/.test(weekText)) {
    return null;
  }
  const week = Number(weekText);
  if (week < 1 || week > isoWeeksInYear(year)) {
    return null;
  }
  const monday = isoWeekOneMonday(year);
  monday.setUTCDate(monday.getUTCDate() + (week - 1) * 7);
  const sunday = new Date(monday.getTime());
  sunday.setUTCDate(sunday.getUTCDate() + 6);
  return { first: isoDate(monday), last: isoDate(sunday) };
}

// Semana ISO de un día comercial (AAAA-MM-DD).
export function isoWeekOf(day: string) {
  const date = parseISODate(day);
  if (!date) {
    return null;
  }
  const weekday = (date.getUTCDay() + 6) % 7;
  const thursday = new Date(date.getTime());
  thursday.setUTCDate(thursday.getUTCDate() - weekday + 3);
  const year = thursday.getUTCFullYear();
  const monday = isoWeekOneMonday(year);
  const week = Math.round((thursday.getTime() - monday.getTime()) / (7 * 86400000)) + 1;
  return { year, week };
}

// Semana anterior o siguiente, con el cambio de año ISO.
export function shiftIsoWeek(yearText: string, weekText: string, offset: number) {
  const range = isoWeekRange(yearText, weekText);
  if (!range) {
    return null;
  }
  const monday = parseISODate(range.first)!;
  monday.setUTCDate(monday.getUTCDate() + offset * 7);
  return isoWeekOf(isoDate(monday));
}

// «Del 9 al 15 de noviembre de 2026» o «Del 28 de diciembre de 2026 al 3 de enero de 2027», con
// las mismas reglas que la etiqueta de Go (businessday.RangeLabel).
export function rangeText(first: string, last: string) {
  const start = parseISODate(first);
  const end = parseISODate(last);
  if (!start || !end) {
    return '';
  }
  const endText = `${end.getUTCDate()} de ${monthNames[end.getUTCMonth()]} de ${end.getUTCFullYear()}`;
  if (
    start.getUTCFullYear() === end.getUTCFullYear() &&
    start.getUTCMonth() === end.getUTCMonth()
  ) {
    return `Del ${start.getUTCDate()} al ${endText}`;
  }
  let startText = `${start.getUTCDate()} de ${monthNames[start.getUTCMonth()]}`;
  if (start.getUTCFullYear() !== end.getUTCFullYear()) {
    startText += ` de ${start.getUTCFullYear()}`;
  }
  return `Del ${startText} al ${endText}`;
}

// Completa los campos de un periodo con el último periodo que resolvió Go (el día comercial del
// servidor, no la fecha local de la terminal).
export function prefillPeriod(
  selection: PeriodSelection,
  kind: PeriodKind,
  resolved: ResolvedPeriod | null,
): PeriodSelection {
  const next: PeriodSelection = { ...selection, period: kind };
  const reference = resolved?.last_day ?? '';
  const first = resolved?.first_day ?? '';
  if (kind === 'day' && next.date === '') {
    next.date = reference;
  }
  if (kind === 'iso_week' && (next.isoYear === '' || next.isoWeek === '')) {
    if (resolved?.iso_year && resolved.iso_week) {
      next.isoYear = String(resolved.iso_year);
      next.isoWeek = String(resolved.iso_week);
    } else {
      const week = isoWeekOf(reference);
      if (week) {
        next.isoYear = String(week.year);
        next.isoWeek = String(week.week);
      }
    }
  }
  if (kind === 'month' && (next.month === '' || next.year === '') && reference !== '') {
    next.year = reference.slice(0, 4);
    next.month = String(Number(reference.slice(5, 7)));
  }
  if (kind === 'year' && next.year === '' && reference !== '') {
    next.year = reference.slice(0, 4);
  }
  if (kind === 'range' && next.from === '' && next.until === '') {
    next.from = first;
    next.until = reference;
  }
  return next;
}

// Indica si la selección tiene los datos que exige su clase de periodo (Go valida el resto).
export function periodComplete(selection: PeriodSelection) {
  switch (selection.period) {
    case 'day':
      return parseISODate(selection.date) !== null;
    case 'iso_week':
      return isoWeekRange(selection.isoYear, selection.isoWeek) !== null;
    case 'month':
      return (
        yearNumber(selection.year) !== null &&
        /^\d{1,2}$/.test(selection.month) &&
        Number(selection.month) >= 1 &&
        Number(selection.month) <= 12
      );
    case 'year':
      return yearNumber(selection.year) !== null;
    case 'range':
      return parseISODate(selection.from) !== null && parseISODate(selection.until) !== null;
    default:
      return true;
  }
}

// Parámetros de periodo para reports/run y reports/dashboard.
export function periodQuery(selection: PeriodSelection, compareAllowed: boolean): ReportQuery {
  const query: ReportQuery = { period: selection.period };
  switch (selection.period) {
    case 'day':
      query.date = selection.date;
      break;
    case 'iso_week':
      query.week = `${selection.isoYear}-W${selection.isoWeek.padStart(2, '0')}`;
      break;
    case 'month':
      query.month = `${selection.year}-${selection.month.padStart(2, '0')}`;
      break;
    case 'year':
      query.year = selection.year;
      break;
    case 'range':
      query.from = selection.from;
      query.until = selection.until;
      break;
  }
  if (compareAllowed) {
    query.compare = selection.compare ? 'previous' : 'none';
  }
  return query;
}
