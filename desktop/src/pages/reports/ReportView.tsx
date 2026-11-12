import { useMemo, useState } from 'react';
import { Icon, IconLabel } from '../../components/Icon';
import { ExportButtons } from '../../components/reports/ExportButtons';
import { PeriodFilter } from '../../components/reports/PeriodFilter';
import { periodQuery } from '../../components/reports/period';
import type { PeriodSelection } from '../../components/reports/period';
import { ReportTable } from '../../components/reports/ReportTable';
import type { SortState } from '../../components/reports/ReportTable';
import { formatReportValue } from '../../components/reports/reportValues';
import { serverDateTime } from '../../services/restaurant';
import { runReport } from '../../services/reports';
import type {
  CatalogReport,
  ReportCatalog,
  ReportQuery,
  ReportResult,
} from '../../services/reports';
import { ReportChart } from './ReportChart';
import { ReportFilterFields, filterStateKey } from './ReportFilterFields';
import type { OptionLoader } from './ReportFilterFields';
import { useReportData } from './useReportData';

// Un reporte del catálogo de Go, dibujado de forma genérica a partir de su declaración (filtros,
// columnas tipadas, periodo y comparación) y de su resultado (metadatos, cifras, renglones de la
// página, totales y avisos). Un solo panel reúne periodo y filtros; cambiar cualquiera vuelve a la
// primera página y consulta una vez. Las columnas opcionales se muestran a pedido.

const pageSize = '50';

export function ReportWarnings({ warnings }: { warnings: ReportResult['meta']['warnings'] }) {
  if (warnings.length === 0) {
    return null;
  }
  return (
    <div className="report-warnings">
      {warnings.map((warning) => (
        <p
          key={`${warning.code}-${warning.message}`}
          className="report-warning"
          role="status"
          data-warning-code={warning.code}
        >
          <Icon name="alert" size={18} />
          <span>{warning.message}</span>
        </p>
      ))}
    </div>
  );
}

function ReportFigures({ result, timezone }: { result: ReportResult; timezone: string }) {
  if (result.summary.length === 0) {
    return null;
  }
  return (
    <div className="report-figures" role="list" aria-label="Cifras del reporte">
      {result.summary.map((figure) => (
        <div key={figure.key} className="metric-card" role="listitem">
          <span className="metric-label">{figure.label}</span>
          <strong className="metric-value">
            {formatReportValue(figure.type, figure.value, timezone)}
          </strong>
          {figure.hint && <span className="metric-hint muted text-xs">{figure.hint}</span>}
        </div>
      ))}
    </div>
  );
}

function ReportMetadata({ result, timezone }: { result: ReportResult; timezone: string }) {
  const meta = result.meta;
  return (
    <dl className="report-meta text-xs">
      <div>
        <dt>Fecha usada</dt>
        <dd>{meta.date_basis}</dd>
      </div>
      {meta.filters.length > 0 && (
        <div>
          <dt>Filtros aplicados</dt>
          <dd>{meta.filters.map((filter) => `${filter.label}: ${filter.display}`).join(' · ')}</dd>
        </div>
      )}
      <div>
        <dt>Generado</dt>
        <dd>{serverDateTime(meta.generated_at, timezone)}</dd>
      </div>
    </dl>
  );
}

export function ReportView({
  report,
  catalog,
  period,
  onPeriod,
  filters,
  onFilter,
  loadOptions,
  can,
  timezone,
}: {
  report: CatalogReport;
  catalog: ReportCatalog;
  period: PeriodSelection;
  onPeriod: (next: PeriodSelection) => void;
  filters: Record<string, string>;
  onFilter: (key: string, value: string) => void;
  loadOptions: OptionLoader;
  can: (permission: string) => boolean;
  timezone: string;
}) {
  const [sort, setSort] = useState<SortState | null>(null);
  const [page, setPage] = useState(1);
  const [showOptional, setShowOptional] = useState(false);
  const usesPeriod = report.period === 'required';

  const query = useMemo(() => {
    const next: ReportQuery = usesPeriod ? periodQuery(period, report.comparison) : {};
    next.page = String(page);
    next.page_size = pageSize;
    if (sort) {
      next.sort = sort.key;
      next.direction = sort.direction;
    }
    for (const filter of report.filters) {
      const value = filters[filterStateKey(filter)] ?? '';
      if (value !== '') {
        next[filter.key] = value;
      }
    }
    return next;
  }, [usesPeriod, period, report, page, sort, filters]);
  const request = `${report.id}?${new URLSearchParams(query).toString()}`;
  const { data, error, loading, reload } = useReportData<ReportResult>(request, report.id, () =>
    runReport(report.id, query),
  );

  const columns = data?.columns ?? report.columns;
  const optionalCount = columns.filter((column) => column.visibility === 'optional').length;
  const visibleColumns = showOptional
    ? columns
    : columns.filter((column) => column.visibility !== 'optional');
  const periodLabel = data?.meta.period?.label;
  const emptyMessage = usesPeriod
    ? `No hay datos en el periodo seleccionado${periodLabel ? ` (${periodLabel})` : ''}.`
    : 'No hay registros en este momento.';

  return (
    <section className="report-view" aria-labelledby="report-title">
      <header className="report-header">
        <div>
          <h2 id="report-title">{report.title}</h2>
          <p className="muted text-sm">{report.description}</p>
        </div>
        <div className="button-row">
          <button
            type="button"
            className="secondary-button"
            disabled={loading}
            onClick={() => void reload()}
          >
            <IconLabel icon="refresh">{loading ? 'Actualizando…' : 'Actualizar'}</IconLabel>
          </button>
          <ExportButtons
            reportID={report.id}
            title={report.title}
            query={query}
            allowPDF={report.export.pdf && can('reports.export.pdf')}
            allowExcel={report.export.excel && can('reports.export.excel')}
            disabled={!data}
          />
        </div>
      </header>
      <div className="panel report-filters" role="region" aria-label="Filtros del reporte">
        {usesPeriod ? (
          <PeriodFilter
            periods={catalog.periods}
            value={period}
            onChange={(next) => {
              setPage(1);
              onPeriod(next);
            }}
            resolved={data?.meta.period ?? null}
            comparison={data?.meta.comparison ?? null}
            allowCompare={report.comparison}
            dayStart={data?.meta.day_start ?? catalog.day_start}
          />
        ) : (
          <p className="muted text-sm">
            Situación actual: este reporte no usa periodo y muestra lo vigente al consultar.
          </p>
        )}
        <ReportFilterFields
          filters={report.filters}
          values={filters}
          loadOptions={loadOptions}
          onChange={(key, value) => {
            setPage(1);
            onFilter(key, value);
          }}
        />
      </div>
      {data && <ReportMetadata result={data} timezone={timezone} />}
      {data && <ReportWarnings warnings={data.meta.warnings} />}
      {data && <ReportFigures result={data} timezone={timezone} />}
      {data && <ReportChart result={data} timezone={timezone} busy={loading} />}
      {optionalCount > 0 && (
        <div className="report-columns">
          <button
            type="button"
            className="secondary-button"
            aria-pressed={showOptional}
            onClick={() => setShowOptional((current) => !current)}
          >
            {showOptional
              ? 'Ocultar columnas adicionales'
              : `Mostrar columnas adicionales (${optionalCount})`}
          </button>
        </div>
      )}
      <ReportTable
        title={report.title}
        columns={visibleColumns}
        result={data}
        loading={loading}
        error={error}
        onRetry={() => void reload()}
        onSort={(next) => {
          setPage(1);
          setSort(next);
        }}
        onPage={setPage}
        timezone={timezone}
        emptyMessage={emptyMessage}
      />
      {data && data.meta.notes.length > 0 && (
        <ul className="report-notes muted text-xs" aria-label="Notas del reporte">
          {data.meta.notes.map((note) => (
            <li key={note}>{note}</li>
          ))}
        </ul>
      )}
    </section>
  );
}
