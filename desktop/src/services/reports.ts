import { api } from './api';

// Contratos del módulo Reportes (etapa 9A, backend/internal/reports). Go es la autoridad: resuelve
// el periodo con la hora de inicio del día, filtra, ordena, pagina y calcula los totales de todo el
// conjunto filtrado. Los importes, porcentajes y cantidades llegan como cadenas decimales: React no
// suma, resta ni redondea; los muestra con services/money.ts. Las columnas protegidas (efectivo
// físico, costos) no llegan si el usuario no tiene su permiso.

export type ColumnType =
  | 'text'
  | 'code'
  | 'integer'
  | 'quantity'
  | 'money_mxn'
  | 'money_usd'
  | 'percent'
  | 'date'
  | 'datetime';

export type ReportValue = string | number | null;

export interface ReportColumn {
  key: string;
  label: string;
  type: ColumnType;
  visibility: 'default' | 'optional';
  total: 'none' | 'sum' | 'custom';
  sortable: boolean;
  description?: string;
}

export interface FilterOption {
  value: string;
  label: string;
  detail?: string;
}

export interface ReportFilter {
  key: string;
  label: string;
  type: 'id' | 'choice' | 'text';
  // Para los filtros «id» (y el de origen), la fuente de reports_filters.
  source?: string;
  options?: FilterOption[];
  default?: string;
}

export type PeriodKind =
  | 'today'
  | 'yesterday'
  | 'current_week'
  | 'current_month'
  | 'current_year'
  | 'day'
  | 'iso_week'
  | 'month'
  | 'year'
  | 'range';

// Periodo resuelto por Go: días comerciales incluidos e instantes [from, until) en UTC.
export interface ResolvedPeriod {
  kind: PeriodKind;
  label: string;
  first_day: string;
  last_day: string;
  from: string;
  until: string;
  days: number;
  iso_year?: number;
  iso_week?: number;
  in_progress: boolean;
}

export interface CatalogReport {
  id: string;
  title: string;
  description: string;
  date_basis: string;
  period: 'required' | 'none';
  comparison: boolean;
  filters: ReportFilter[];
  columns: ReportColumn[];
  default_sort: { key: string; direction: 'asc' | 'desc' };
  export: { pdf: boolean; excel: boolean };
}

export interface ReportCatalog {
  groups: { id: string; title: string; reports: CatalogReport[] }[];
  periods: FilterOption[];
  dashboard: boolean;
  timezone: string;
  day_start: string;
}

export interface ReportWarning {
  code: string;
  message: string;
}

export interface ReportResult {
  report: { id: string; group: string; title: string; description: string };
  meta: {
    title: string;
    period: ResolvedPeriod | null;
    comparison: ResolvedPeriod | null;
    filters: { key: string; label: string; value: string; display: string }[];
    generated_at: string;
    timezone: string;
    day_start: string;
    date_basis: string;
    warnings: ReportWarning[];
    notes: string[];
  };
  columns: ReportColumn[];
  rows: Record<string, ReportValue>[];
  // Totales de todo el conjunto filtrado (no de la página); null en las columnas sin total.
  totals: Record<string, ReportValue> | null;
  summary: { key: string; label: string; type: ColumnType; value: ReportValue; hint?: string }[];
  page: { page: number; page_size: number; total_rows: number; total_pages: number };
  sort: { key: string; direction: 'asc' | 'desc' };
}

export interface DashboardIndicator {
  key: string;
  label: string;
  type: ColumnType;
  value: ReportValue;
  previous: ReportValue;
  change: ReportValue;
  change_percent: string | null;
  report_id?: string;
  hint?: string;
}

export interface Dashboard {
  period: ResolvedPeriod;
  comparison: ResolvedPeriod | null;
  generated_at: string;
  timezone: string;
  day_start: string;
  sections: { id: string; title: string; indicators: DashboardIndicator[] }[];
  series: {
    key: string;
    label: string;
    type: ColumnType;
    report_id?: string;
    points: { date: string; value: string }[];
  }[];
  warnings: ReportWarning[];
}

// Parámetros de un reporte: periodo, comparación, página, orden y filtros declarados por el reporte.
// El puente nativo solo admite las claves de su lista blanca.
export type ReportQuery = Record<string, string>;

export function fetchReportCatalog() {
  return api<ReportCatalog>('reports_catalog');
}

export function runReport(id: string, query: ReportQuery = {}) {
  return api<ReportResult>('reports_run', { id, query });
}

export function fetchDashboard(query: ReportQuery = {}) {
  return api<Dashboard>('reports_dashboard', { query });
}

export function fetchFilterOptions(source: string, search = '') {
  const query: ReportQuery = {};
  if (search !== '') {
    query.search = search;
  }
  return api<{ source: string; options: FilterOption[] }>('reports_filters', { id: source, query });
}

// «Hora de inicio del día» (configuración general, settings.manage). Es la misma hora de corte que
// usa inventario; Inventarios → Parámetros la muestra en solo lectura.
export interface BusinessDaySettings {
  operating_day_cutoff: string;
  timezone: string;
  revision: number;
  updated_at: string;
  updated_by_name: string | null;
}

export function fetchBusinessDay() {
  return api<BusinessDaySettings>('settings_business_day');
}

export function saveBusinessDay(input: { operating_day_cutoff: string; revision: number }) {
  return api<BusinessDaySettings>('save_settings_business_day', { body: input });
}

// Reconstruye HH:MM a partir de los dígitos tecleados (como el campo de fechas dd/mm/aaaa). Solo
// acomoda texto: la validación es de Go.
export function formatDayStartDigits(text: string): string | null {
  if (!/^[0-9:]*$/.test(text)) {
    return null;
  }
  const digits = text.replaceAll(':', '').slice(0, 4);
  if (digits.length <= 2) {
    return digits;
  }
  return `${digits.slice(0, 2)}:${digits.slice(2)}`;
}
