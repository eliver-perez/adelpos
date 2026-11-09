import type { Schedule } from './catalog';

// Días de Go: 0 = domingo … 6 = sábado. La interfaz los presenta de lunes a domingo.
export const weekOrder = [1, 2, 3, 4, 5, 6, 0];
export const dayShortNames = ['Dom', 'Lun', 'Mar', 'Mié', 'Jue', 'Vie', 'Sáb'];

function displayDate(iso: string): string {
  const match = /^([0-9]{4})-([0-9]{2})-([0-9]{2})$/.exec(iso);
  return match ? `${match[3]}/${match[2]}/${match[1]}` : iso;
}

// «07:00» se muestra como «7:00»; «24:00» se conserva.
export function displayHour(value: string): string {
  return value.replace(/^0([0-9]):/, '$1:');
}

// Agrupa días consecutivos (de lunes a domingo): «Lun a vie», «Lun, mié, vie», «Sáb y dom».
function daysSummary(days: number[]): string {
  const positions = weekOrder
    .map((day, position) => (days.includes(day) ? position : -1))
    .filter((position) => position >= 0);
  if (positions.length === 0 || positions.length === 7) {
    return '';
  }
  const runs: number[][] = [];
  for (const position of positions) {
    const last = runs[runs.length - 1];
    if (last && last[last.length - 1] === position - 1) {
      last.push(position);
    } else {
      runs.push([position]);
    }
  }
  const name = (position: number) => (dayShortNames[weekOrder[position] ?? 0] ?? '').toLowerCase();
  const parts = runs.map((run) => {
    const first = run[0] ?? 0;
    const last = run[run.length - 1] ?? first;
    if (run.length >= 3) {
      return `${name(first)} a ${name(last)}`;
    }
    return run.map(name).join(', ');
  });
  const text = parts.join(', ');
  return text.charAt(0).toUpperCase() + text.slice(1);
}

function datesSummary(schedule: Schedule): string {
  const starts = schedule.starts_on ?? '';
  const ends = schedule.ends_on ?? '';
  if (starts && ends) {
    return `Del ${displayDate(starts)} al ${displayDate(ends)}`;
  }
  if (starts) {
    return `Desde ${displayDate(starts)}`;
  }
  if (ends) {
    return `Hasta ${displayDate(ends)}`;
  }
  return '';
}

// Resumen breve para el acordeón cerrado: «Disponible siempre» o, por ejemplo,
// «Desde 15/11/2026 · Lun a vie · 7:00–12:00».
export function scheduleSummary(schedule: Schedule): string {
  const parts: string[] = [];
  const dates = datesSummary(schedule);
  if (dates) {
    parts.push(dates);
  }
  const days = daysSummary(schedule.days ?? []);
  const windows = [...(schedule.windows ?? [])]
    .sort((first, second) => first.start.localeCompare(second.start))
    .map((window) => `${displayHour(window.start)}–${displayHour(window.end)}`);
  if (days) {
    parts.push(days);
  } else if (windows.length > 0) {
    parts.push('Todos los días');
  }
  if (windows.length > 0) {
    parts.push(windows.join(', '));
  }
  return parts.length > 0 ? parts.join(' · ') : 'Disponible siempre';
}
