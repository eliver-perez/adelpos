import { ComparisonBars } from '../../components/reports/ComparisonBars';
import { SegmentedBar, maxSegments } from '../../components/reports/SegmentedBar';
import { TrendChart } from '../../components/reports/TrendChart';
import type { TrendSeries } from '../../components/reports/TrendChart';
import {
  decimalSign,
  formatReportValue,
  isNumericType,
} from '../../components/reports/reportValues';
import type { ReportColumn, ReportResult, ReportValue } from '../../services/reports';

// Gráfico de un reporte elegido a partir de la forma del resultado de Go, para que los reportes
// nuevos del catálogo (fase B2) tengan su visualización sin escribir una pantalla por reporte:
//   - una secuencia (día, semana, mes, hora o periodo de la evolución) → líneas, con la serie del
//     periodo anterior si el reporte la trae;
//   - hasta cinco categorías con valores no negativos y todo el conjunto en la página → barra
//     segmentada (distribución);
//   - más categorías → barras horizontales con los primeros renglones en el orden de la tabla.
// Los listados de detalle (con fecha y hora o con muchas columnas descriptivas) no llevan gráfico:
// la tabla es su forma adecuada. El gráfico solo usa renglones que Go ya entregó; nunca suma.

// Medida principal en orden de preferencia; si ninguna está, la primera de importe con total.
const preferredMeasures = [
  'net',
  'total_discount',
  'collected',
  'amount',
  'total',
  'consumption_value',
  'released',
  'quantity',
];

const sequenceKeys = ['day', 'week', 'month', 'hour', 'bucket'];
const barLimit = 10;

function primaryMeasure(columns: ReportColumn[]) {
  for (const key of preferredMeasures) {
    const column = columns.find((entry) => entry.key === key && isNumericType(entry.type));
    if (column && column.type !== 'percent') {
      return column;
    }
  }
  return (
    columns.find((entry) => entry.type === 'money_mxn' && entry.total === 'sum') ??
    columns.find(
      (entry) => (entry.type === 'integer' || entry.type === 'quantity') && entry.total === 'sum',
    ) ??
    null
  );
}

export function ReportChart({
  result,
  timezone,
  busy,
}: {
  result: ReportResult;
  timezone: string;
  busy: boolean;
}) {
  const columns = result.columns;
  const rows = result.rows;
  const first = columns[0];
  const measure = primaryMeasure(columns);
  if (!first || !measure || rows.length < 2 || isNumericType(first.type)) {
    return null;
  }
  if (columns.some((column) => column.type === 'datetime')) {
    return null;
  }
  const descriptive = columns.filter(
    (column) =>
      (column.type === 'text' || column.type === 'code') && column.visibility === 'default',
  );
  // Más de dos columnas de texto (además de los códigos) indican un listado de detalle.
  if (descriptive.filter((column) => column.type === 'text').length > 2) {
    return null;
  }
  const format = (column: ReportColumn, value: ReportValue | undefined) =>
    formatReportValue(column.type, value ?? null, timezone);
  const sequence = first.type === 'date' || sequenceKeys.includes(first.key);

  if (sequence) {
    if (result.page.total_pages > 1) {
      return (
        <p className="muted text-sm report-chart-note">
          El gráfico de tendencia aparece cuando todo el periodo cabe en una página; usa un periodo
          más corto o el reporte por semana o por mes.
        </p>
      );
    }
    // La tendencia sigue el orden del tiempo: si la tabla se ordenó por otra columna no se dibuja.
    const dateKeys = columns.filter((column) => column.type === 'date').map((column) => column.key);
    const sortKey = result.sort.key;
    if (sortKey !== '' && sortKey !== first.key && !dateKeys.includes(sortKey)) {
      return (
        <p className="muted text-sm report-chart-note">
          La tendencia se muestra con la tabla ordenada por {first.label.toLowerCase()}.
        </p>
      );
    }
    const ordered = result.sort.direction === 'desc' && sortKey !== '' ? [...rows].reverse() : rows;
    // En el eje, los días de un mismo año se rotulan dd/mm (el año está en la descripción).
    const years = new Set(
      first.type === 'date' ? rows.map((row) => String(row[first.key] ?? '').slice(0, 4)) : [],
    );
    const categoryText = (column: ReportColumn, value: ReportValue | undefined) => {
      const text = format(column, value);
      return column.type === 'date' && years.size === 1 ? text.slice(0, 5) : text;
    };
    const previous = columns.find((column) => column.key === `previous_${measure.key}`);
    const series: TrendSeries[] = [
      {
        key: measure.key,
        label: previous ? `${measure.label} (periodo)` : measure.label,
        tone: 1,
        values: ordered.map((row) => row[measure.key] ?? null),
      },
    ];
    if (previous) {
      series.push({
        key: previous.key,
        label: previous.label,
        tone: 'previous',
        values: ordered.map((row) => row[previous.key] ?? null),
      });
    }
    return (
      <TrendChart
        title={`${measure.label} por ${first.label.toLowerCase()}`}
        description={result.meta.period?.label}
        categoryLabel={first.label}
        categories={ordered.map((row) => categoryText(first, row[first.key]))}
        labelPoint={result.meta.period?.in_progress ? 'max' : 'last'}
        series={series}
        type={measure.type}
        timezone={timezone}
        busy={busy}
      />
    );
  }

  const labelColumns = descriptive.filter((column) => column.type === 'text');
  const naming = labelColumns.length > 0 ? labelColumns : descriptive;
  const label = (row: ReportResult['rows'][number]) =>
    naming
      .map((column) => format(column, row[column.key]))
      .filter((text) => text !== '—')
      .join(' · ') || '—';
  const allNonNegative = rows.every((row) => decimalSign(row[measure.key] ?? null) >= 0);
  const anyPositive = rows.some((row) => decimalSign(row[measure.key] ?? null) > 0);

  if (
    rows.length <= maxSegments &&
    result.page.total_pages === 1 &&
    allNonNegative &&
    anyPositive &&
    measure.type !== 'percent'
  ) {
    return (
      <SegmentedBar
        title={`Distribución de ${measure.label.toLowerCase()}`}
        description={`Por ${naming[0]?.label.toLowerCase() ?? 'categoría'} · ${result.meta.period?.label ?? ''}`}
        categoryLabel={naming[0]?.label ?? first.label}
        valueLabel={measure.label}
        parts={rows.map((row, index) => ({
          key: `${label(row)}-${index}`,
          label: label(row),
          value: row[measure.key] ?? null,
        }))}
        type={measure.type}
        timezone={timezone}
        busy={busy}
      />
    );
  }

  const shown = rows.slice(0, barLimit);
  const partial = rows.length > barLimit || result.page.total_pages > 1;
  return (
    <ComparisonBars
      title={`${measure.label} por ${(naming[0]?.label ?? first.label).toLowerCase()}`}
      description={
        partial
          ? `Primeros ${shown.length} renglones de esta página, en el orden de la tabla.`
          : result.meta.period?.label
      }
      categoryLabel={naming[0]?.label ?? first.label}
      series={[{ label: measure.label, tone: 1 }]}
      items={shown.map((row, index) => ({
        key: `${label(row)}-${index}`,
        label: label(row),
        values: [row[measure.key] ?? null],
      }))}
      type={measure.type}
      timezone={timezone}
      busy={busy}
    />
  );
}
