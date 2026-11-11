import { useEffect, useMemo } from 'react';
import { IconLabel } from '../../components/Icon';
import { ComparisonBars } from '../../components/reports/ComparisonBars';
import { MetricCard } from '../../components/reports/MetricCard';
import type { MetricPolarity } from '../../components/reports/MetricCard';
import { PeriodFilter } from '../../components/reports/PeriodFilter';
import { periodQuery } from '../../components/reports/period';
import type { PeriodSelection } from '../../components/reports/period';
import { TrendChart } from '../../components/reports/TrendChart';
import { decimalSign, formatBusinessDate } from '../../components/reports/reportValues';
import { fetchDashboard } from '../../services/reports';
import type { Dashboard, DashboardIndicator, ReportCatalog } from '../../services/reports';
import { ReportWarnings } from './ReportView';
import { useReportData } from './useReportData';

// Resumen ejecutivo (§5). Go calcula cada indicador con su permiso, para el periodo elegido y el
// anterior equivalente, y solo envía las secciones que el usuario puede ver: lo que no llega no se
// muestra. **No se recalcula con cada evento SSE** (§0-B, «Rendimiento»): se consulta al abrir, al
// cambiar el periodo, con «Actualizar» y con un temporizador de 5 minutos solo mientras la pantalla
// está visible. Cada tarjeta abre su reporte detallado con el mismo periodo.

export const dashboardRefreshMilliseconds = 5 * 60 * 1000;

// Polaridad de los indicadores conocidos; los demás (compras, consumo…) se muestran sin tono.
const upIsGood = [
  'sales',
  'net_sales',
  'collected',
  'payments',
  'accounts',
  'average_ticket',
  'units',
  'tips',
  'margin',
  'estimated_margin',
  'margin_percent',
  'firm_cost_percent',
];
const upIsBad = [
  'refunds',
  'discounts',
  'courtesies',
  'discount_percent',
  'waste',
  'waste_cost',
  'cost_adjustments',
];

function polarityOf(key: string): MetricPolarity {
  if (upIsGood.includes(key)) {
    return 'up-good';
  }
  if (upIsBad.includes(key)) {
    return 'up-bad';
  }
  return 'neutral';
}

// Días del eje: dd/mm si todos son del mismo año (el año está en la descripción del periodo).
function dayLabels(days: string[]) {
  const sameYear = new Set(days.map((day) => day.slice(0, 4))).size === 1;
  return days.map((day) => {
    const text = formatBusinessDate(day);
    return sameYear ? text.slice(0, 5) : text;
  });
}

function timeOf(value: string, timezone: string) {
  return new Intl.DateTimeFormat('es-MX', { timeStyle: 'short', timeZone: timezone }).format(
    new Date(value),
  );
}

// Barras del periodo actual frente al anterior para los importes en pesos de una sección (los
// indicadores de situación actual, sin valor anterior, no entran).
// Si todos los importes son cero en ambos periodos no hay nada que comparar y no se dibuja.
function comparableMoney(indicators: DashboardIndicator[]) {
  const comparable = indicators.filter(
    (indicator) =>
      indicator.type === 'money_mxn' && indicator.value !== null && indicator.previous !== null,
  );
  const anyAmount = comparable.some(
    (indicator) => decimalSign(indicator.value) !== 0 || decimalSign(indicator.previous) !== 0,
  );
  return anyAmount ? comparable : [];
}

export function DashboardView({
  catalog,
  period,
  onPeriod,
  onOpenReport,
  timezone,
}: {
  catalog: ReportCatalog;
  period: PeriodSelection;
  onPeriod: (next: PeriodSelection) => void;
  onOpenReport: (reportID: string) => void;
  timezone: string;
}) {
  const query = useMemo(() => periodQuery(period, true), [period]);
  const request = `dashboard?${new URLSearchParams(query).toString()}`;
  const { data, error, loading, reload } = useReportData<Dashboard>(request, 'dashboard', () =>
    fetchDashboard(query),
  );

  // Temporizador moderado: solo consulta si la ventana está visible (la vista solo existe mientras
  // la pantalla Reportes muestra el resumen).
  useEffect(() => {
    const timer = window.setInterval(() => {
      if (document.visibilityState === 'visible') {
        void reload();
      }
    }, dashboardRefreshMilliseconds);
    return () => window.clearInterval(timer);
  }, [reload]);

  const reportIDs = new Set(
    catalog.groups.flatMap((group) => group.reports.map((report) => report.id)),
  );
  const compared = period.compare && data?.comparison !== null && data?.comparison !== undefined;
  const series = data?.series ?? [];

  function trendFor(indicator: DashboardIndicator) {
    const match = series.find(
      (entry) => entry.key === indicator.key || entry.key.startsWith(`${indicator.key}_by_`),
    );
    return match ? match.points.map((point) => point.value) : undefined;
  }

  return (
    <section className="report-view dashboard-view" aria-labelledby="dashboard-title">
      <header className="report-header">
        <div>
          <h2 id="dashboard-title">Resumen ejecutivo</h2>
          <p className="muted text-sm">
            Indicadores del periodo con su comparación contra el periodo anterior equivalente. Se
            actualiza cada 5 minutos o con «Actualizar»; no cambia con cada movimiento de la
            operación.
          </p>
        </div>
        <div className="button-row">
          {data && (
            <span className="muted text-xs" role="status">
              Actualizado a las {timeOf(data.generated_at, timezone)}
            </span>
          )}
          <button
            type="button"
            className="secondary-button"
            disabled={loading}
            onClick={() => void reload()}
          >
            <IconLabel icon="refresh">{loading ? 'Actualizando…' : 'Actualizar'}</IconLabel>
          </button>
        </div>
      </header>
      <div className="panel report-filters" role="region" aria-label="Periodo del resumen">
        <PeriodFilter
          periods={catalog.periods}
          value={period}
          onChange={onPeriod}
          resolved={data?.period ?? null}
          comparison={data?.comparison ?? null}
          allowCompare
          dayStart={data?.day_start ?? catalog.day_start}
          timezone={data?.timezone ?? catalog.timezone}
        />
      </div>
      {error && (
        <div className="report-state report-state-error" role="alert">
          <p>{error}</p>
          <button type="button" className="secondary-button" onClick={() => void reload()}>
            Reintentar
          </button>
        </div>
      )}
      {!data && !error && (
        <div className="report-state" role="status" aria-busy="true">
          Cargando indicadores…
        </div>
      )}
      {data && <ReportWarnings warnings={data.warnings} />}
      {data && data.sections.length === 0 && (
        <div className="report-state" role="status">
          Tu usuario no tiene permiso para ver ningún indicador del resumen. Los indicadores de
          ventas, cobros, descuentos y demás áreas requieren su permiso de reportes.
        </div>
      )}
      {data?.sections.map((section) => {
        const accounts = section.indicators.find((indicator) => indicator.key === 'accounts');
        const noSales = section.id === 'sales' && accounts !== undefined && accounts.value === 0;
        const money = comparableMoney(section.indicators);
        return (
          <section
            key={section.id}
            className={`dashboard-section ${loading ? 'report-refreshing' : ''}`}
            aria-labelledby={`dashboard-section-${section.id}`}
          >
            <h3 id={`dashboard-section-${section.id}`}>{section.title}</h3>
            {noSales && (
              <p className="muted text-sm">
                No hay ventas cerradas en este periodo. Las cuentas abiertas se ven en «En curso» y
                no cuentan como venta hasta cerrarse.
              </p>
            )}
            <div className="metric-grid">
              {section.indicators.map((indicator) => {
                const target =
                  indicator.report_id && reportIDs.has(indicator.report_id)
                    ? indicator.report_id
                    : null;
                return (
                  <MetricCard
                    key={indicator.key}
                    indicator={indicator}
                    timezone={timezone}
                    compared={compared}
                    trend={trendFor(indicator)}
                    polarity={polarityOf(indicator.key)}
                    onOpen={target ? () => onOpenReport(target) : undefined}
                  />
                );
              })}
            </div>
            {compared && money.length >= 2 && (
              <ComparisonBars
                title={`${section.title}: periodo actual frente al anterior`}
                description={`${data.period.label} · ${data.comparison?.label ?? ''}`}
                categoryLabel="Indicador"
                series={[
                  { label: 'Periodo actual', tone: 1 },
                  { label: 'Periodo anterior', tone: 'previous' },
                ]}
                items={money.map((indicator) => ({
                  key: indicator.key,
                  label: indicator.label,
                  values: [indicator.value, indicator.previous],
                }))}
                type="money_mxn"
                timezone={timezone}
                busy={loading}
              />
            )}
          </section>
        );
      })}
      {data &&
        series
          .filter((entry) => entry.points.length >= 2)
          .map((entry) => (
            <div key={entry.key} className="dashboard-trend">
              <TrendChart
                title={entry.label}
                description={data.period.label}
                categoryLabel="Día"
                categories={dayLabels(entry.points.map((point) => point.date))}
                labelPoint={data.period.in_progress ? 'max' : 'last'}
                series={[
                  {
                    key: entry.key,
                    label: entry.label,
                    tone: 1,
                    values: entry.points.map((point) => point.value),
                  },
                ]}
                type={entry.type}
                timezone={timezone}
                busy={loading}
              />
              {entry.report_id && reportIDs.has(entry.report_id) && (
                <button
                  type="button"
                  className="secondary-button"
                  onClick={() => onOpenReport(entry.report_id!)}
                >
                  Ver reporte detallado
                </button>
              )}
            </div>
          ))}
    </section>
  );
}
