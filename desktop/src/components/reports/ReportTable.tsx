import { Pagination } from '../../pages/inventory/InventoryFields';
import type { ReportColumn, ReportResult, ReportValue } from '../../services/reports';
import { formatReportValue, isNumericType, statusTone } from './reportValues';

// Tabla de un reporte (§26): columnas tipadas tal como las entrega Go (las que el usuario no puede
// ver no llegan y no se dibujan), totales de Go de todo el conjunto filtrado en <tfoot>, paginación
// del detalle en Go y orden por columna en Go (aria-sort). React solo da formato: no suma, no
// ordena ni pagina en el cliente. Estados de carga, vacío y error en su lugar.

export type SortState = ReportResult['sort'];

function nextSort(column: ReportColumn, current: SortState | null): SortState {
  if (current && current.key === column.key) {
    return { key: column.key, direction: current.direction === 'asc' ? 'desc' : 'asc' };
  }
  // Primer toque: de mayor a menor en cifras y de A a Z (o del más antiguo) en lo demás.
  return { key: column.key, direction: isNumericType(column.type) ? 'desc' : 'asc' };
}

// Clase de una celda según su tipo: cifras alineadas a la derecha; códigos y fechas sin partir.
function cellClass(column: ReportColumn) {
  if (isNumericType(column.type)) {
    return 'numeric-cell';
  }
  if (column.type === 'code' || column.type === 'date' || column.type === 'datetime') {
    return 'report-nowrap';
  }
  return undefined;
}

function ariaSort(column: ReportColumn, sort: SortState | null) {
  if (!column.sortable) {
    return undefined;
  }
  if (!sort || sort.key !== column.key) {
    return 'none' as const;
  }
  return sort.direction === 'asc' ? ('ascending' as const) : ('descending' as const);
}

function Cell({
  column,
  value,
  timezone,
}: {
  column: ReportColumn;
  value: ReportValue;
  timezone: string;
}) {
  const text = formatReportValue(column.type, value, timezone);
  const tone = column.type === 'text' ? statusTone(value) : undefined;
  if (tone) {
    return <span className={`tag tag-${tone}`}>{text}</span>;
  }
  if (column.type === 'code' && value !== null && value !== '') {
    return <code className="report-code">{text}</code>;
  }
  if (column.type === 'text') {
    return <span className="report-text">{text}</span>;
  }
  return <>{text}</>;
}

export function ReportTable({
  title,
  columns,
  result,
  loading,
  error,
  onRetry,
  onSort,
  onPage,
  timezone,
  emptyMessage,
}: {
  title: string;
  columns: ReportColumn[];
  result: ReportResult | null;
  loading: boolean;
  error: string;
  onRetry: () => void;
  onSort?: (sort: SortState) => void;
  onPage: (page: number) => void;
  timezone: string;
  emptyMessage: string;
}) {
  if (error && !result) {
    return (
      <div className="report-state report-state-error" role="alert">
        <p>{error}</p>
        <button type="button" className="secondary-button" onClick={onRetry}>
          Reintentar
        </button>
      </div>
    );
  }
  if (!result) {
    return (
      <div className="report-state" role="status" aria-busy="true">
        Cargando reporte…
      </div>
    );
  }
  const rows = result.rows;
  const totals = result.totals;
  const sort = result.sort.key ? result.sort : null;
  const showTotals = totals !== null && rows.length > 0;
  const firstHasTotal = columns[0] ? columns[0].total !== 'none' : false;
  return (
    <div className={`report-table ${loading ? 'report-refreshing' : ''}`}>
      {error && (
        <div className="report-state report-state-error" role="alert">
          <p>{error}</p>
          <button type="button" className="secondary-button" onClick={onRetry}>
            Reintentar
          </button>
        </div>
      )}
      <div
        className="table-container report-table-scroll"
        role="region"
        aria-label={`Tabla: ${title}`}
        aria-busy={loading}
        tabIndex={0}
      >
        <table>
          <caption className="visually-hidden">
            {title}
            {result.meta.period ? ` · ${result.meta.period.label}` : ''}
          </caption>
          <thead>
            <tr>
              {columns.map((column) => (
                <th
                  key={column.key}
                  scope="col"
                  aria-sort={onSort ? ariaSort(column, sort) : undefined}
                  className={isNumericType(column.type) ? 'numeric-cell' : undefined}
                  title={column.description}
                >
                  {onSort && column.sortable ? (
                    <button
                      type="button"
                      className="report-sort"
                      onClick={() => onSort(nextSort(column, sort))}
                    >
                      <span>{column.label}</span>
                      <span className="report-sort-mark" aria-hidden="true">
                        {sort?.key === column.key ? (sort.direction === 'asc' ? '▲' : '▼') : '↕'}
                      </span>
                    </button>
                  ) : (
                    column.label
                  )}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {rows.length === 0 && (
              <tr>
                <td colSpan={Math.max(1, columns.length)} className="report-empty">
                  {emptyMessage}
                </td>
              </tr>
            )}
            {rows.map((row, index) => (
              <tr key={index}>
                {columns.map((column) => (
                  <td key={column.key} className={cellClass(column)}>
                    <Cell column={column} value={row[column.key] ?? null} timezone={timezone} />
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
          {showTotals && (
            <tfoot>
              <tr>
                {columns.map((column, index) => {
                  if (index === 0 && !firstHasTotal) {
                    return (
                      <th key={column.key} scope="row">
                        Total
                      </th>
                    );
                  }
                  const value = column.total === 'none' ? null : (totals?.[column.key] ?? null);
                  return (
                    <td
                      key={column.key}
                      className={isNumericType(column.type) ? 'numeric-cell' : undefined}
                    >
                      {value === null ? '' : formatReportValue(column.type, value, timezone)}
                    </td>
                  );
                })}
              </tr>
            </tfoot>
          )}
        </table>
      </div>
      {result.page.total_rows > 0 && (
        <Pagination
          page={result.page.page}
          pageSize={result.page.page_size}
          total={result.page.total_rows}
          singular="renglón"
          plural="renglones"
          onPage={onPage}
        />
      )}
    </div>
  );
}
