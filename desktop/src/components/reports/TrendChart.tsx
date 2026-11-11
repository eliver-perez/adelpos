import { useRef, useState } from 'react';
import type { KeyboardEvent, PointerEvent } from 'react';
import type { ColumnType, ReportValue } from '../../services/reports';
import { ChartFrame, toneClass } from './ChartFrame';
import type { SeriesTone } from './ChartFrame';
import { axisLabel, niceTicks, useElementWidth } from './chartScale';
import { formatReportValue, geometryNumber } from './reportValues';

// Gráfico de líneas para tendencias (§5: «líneas para tendencias»). Una sola escala Y (nunca doble
// eje): solo se combinan series del mismo tipo, como la venta del periodo y la del anterior. Línea
// de 2 px, punto final con anillo del color de la superficie y rótulo directo solo en el último
// valor de la serie principal; la cruceta y el recuadro muestran todas las series del punto y la
// tabla alternativa tiene todos los valores.

export interface TrendSeries {
  key: string;
  label: string;
  tone: SeriesTone;
  // Un valor por categoría (null = sin dato).
  values: ReportValue[];
}

const height = 260;
const top = 16;
const bottomBand = 34;
const leftBand = 72;
const rightBand = 96;

export function TrendChart({
  title,
  description,
  categoryLabel,
  categories,
  series,
  type,
  timezone,
  busy = false,
  labelPoint = 'last',
}: {
  title: string;
  description?: string;
  categoryLabel: string;
  categories: string[];
  series: TrendSeries[];
  type: ColumnType;
  timezone: string;
  busy?: boolean;
  labelPoint?: 'last' | 'max';
}) {
  const container = useRef<HTMLDivElement>(null);
  const width = Math.max(320, useElementWidth(container, 720));
  const [active, setActive] = useState<number | null>(null);
  const plotWidth = width - leftBand - rightBand;
  const plotHeight = height - top - bottomBand;
  const numbers = series.flatMap((entry) =>
    entry.values.map(geometryNumber).filter((value): value is number => value !== null),
  );
  const scale = niceTicks(Math.min(...numbers, 0), Math.max(...numbers, 0), 4);
  const count = categories.length;
  const step = count > 1 ? plotWidth / (count - 1) : 0;
  const x = (index: number) => (count > 1 ? leftBand + index * step : leftBand + plotWidth / 2);
  const y = (value: number) =>
    top + plotHeight - ((value - scale.low) / (scale.high - scale.low)) * plotHeight;
  const format = (value: ReportValue) => formatReportValue(type, value, timezone);

  // Rótulos del eje X sin encimarse: como máximo uno cada 84 px, siempre con el último.
  const labelEvery = Math.max(1, Math.ceil(count / Math.max(1, Math.floor(plotWidth / 84))));
  const labeled = categories
    .map((label, index) => ({ label, index }))
    .filter(({ index }) => index % labelEvery === 0 || index === count - 1)
    .filter(({ index }) => index === count - 1 || count - 1 - index >= labelEvery / 2);

  function linePath(values: ReportValue[]) {
    let path = '';
    let drawing = false;
    values.forEach((value, index) => {
      const number = geometryNumber(value);
      if (number === null) {
        drawing = false;
        return;
      }
      path += `${drawing ? 'L' : 'M'}${x(index)},${y(number)} `;
      drawing = true;
    });
    return path.trim();
  }

  function areaPath(values: ReportValue[]) {
    const points = values
      .map((value, index) => ({ index, number: geometryNumber(value) }))
      .filter((point): point is { index: number; number: number } => point.number !== null);
    if (points.length < 2) {
      return '';
    }
    const baseline = y(Math.max(scale.low, Math.min(0, scale.high)));
    const first = points[0]!;
    const last = points[points.length - 1]!;
    const body = points.map((point) => `L${x(point.index)},${y(point.number)}`).join(' ');
    return `M${x(first.index)},${baseline} ${body} L${x(last.index)},${baseline} Z`;
  }

  const primary = series[0];
  // Rótulo directo en un solo punto de la serie principal: el último o, en un periodo en curso
  // (cuyos días siguientes aún no ocurren), el valor más alto.
  const lastIndex = primary
    ? primary.values.reduce<number>((found, value, index) => {
        const number = geometryNumber(value);
        if (number === null) {
          return found;
        }
        if (labelPoint === 'max' && found >= 0) {
          const best = geometryNumber(primary.values[found] ?? null) ?? -Infinity;
          return number > best ? index : found;
        }
        return index;
      }, -1)
    : -1;
  const lastValue = primary && lastIndex >= 0 ? primary.values[lastIndex] : null;
  const lastNumber = geometryNumber(lastValue ?? null);

  // El área sensible es más ancha que la línea: media separación a cada lado del primer y del
  // último punto (al menos 12 px), y el puntero se ajusta al punto más cercano.
  const hitPadding = Math.max(step / 2, 12);

  function pointFromEvent(event: PointerEvent<SVGRectElement>) {
    const box = event.currentTarget.getBoundingClientRect();
    const offset = event.clientX - box.left - hitPadding;
    if (count <= 1) {
      return 0;
    }
    return Math.max(0, Math.min(count - 1, Math.round(offset / step)));
  }

  function keyboard(event: KeyboardEvent<HTMLDivElement>) {
    if (count === 0) {
      return;
    }
    const current = active ?? count - 1;
    let next = current;
    if (event.key === 'ArrowLeft') {
      next = Math.max(0, current - 1);
    } else if (event.key === 'ArrowRight') {
      next = Math.min(count - 1, current + 1);
    } else if (event.key === 'Home') {
      next = 0;
    } else if (event.key === 'End') {
      next = count - 1;
    } else if (event.key === 'Escape') {
      setActive(null);
      return;
    } else {
      return;
    }
    event.preventDefault();
    setActive(next);
  }

  const labeledText =
    labelPoint === 'max'
      ? `valor más alto ${format(lastValue ?? null)} (${categories[lastIndex] ?? ''})`
      : `último valor ${format(lastValue ?? null)}`;
  const summary =
    count > 0
      ? `${title}: ${count} puntos, de ${categories[0]} a ${categories[count - 1]}; ${labeledText}.`
      : `${title}: sin datos.`;
  const table = {
    columns: [categoryLabel, ...series.map((entry) => entry.label)],
    rows: categories.map((label, index) => [
      label,
      ...series.map((entry) => format(entry.values[index] ?? null)),
    ]),
  };
  const tooltipLeft = active !== null ? Math.min(Math.max(x(active), 120), width - 120) : 0;

  return (
    <ChartFrame
      title={title}
      description={description}
      legend={series.map((entry) => ({ label: entry.label, tone: entry.tone, shape: 'line' }))}
      table={table}
      busy={busy}
    >
      <div
        ref={container}
        className="chart-canvas"
        tabIndex={0}
        role="group"
        aria-label={`${title}. Usa las flechas izquierda y derecha para recorrer los puntos.`}
        onKeyDown={keyboard}
        onFocus={() => setActive((current) => current ?? (count > 0 ? count - 1 : null))}
        onBlur={() => setActive(null)}
      >
        <svg width={width} height={height} role="img" aria-label={summary}>
          {scale.ticks.map((tick) => (
            <g key={tick}>
              <line
                className="chart-grid"
                x1={leftBand}
                x2={leftBand + plotWidth}
                y1={y(tick)}
                y2={y(tick)}
              />
              <text
                className="chart-axis-label"
                x={leftBand - 10}
                y={y(tick)}
                textAnchor="end"
                dominantBaseline="middle"
              >
                {axisLabel(tick, type)}
              </text>
            </g>
          ))}
          {labeled.map(({ label, index }) => (
            <text
              key={`${label}-${index}`}
              className="chart-axis-label"
              x={x(index)}
              y={height - 12}
              textAnchor={
                index === 0 && count > 1
                  ? 'start'
                  : index === count - 1 && count > 1
                    ? 'end'
                    : 'middle'
              }
            >
              {label}
            </text>
          ))}
          {primary && series.length === 1 && (
            <path
              className={`chart-area ${toneClass(primary.tone)}`}
              d={areaPath(primary.values)}
            />
          )}
          {[...series].reverse().map((entry) => (
            <path
              key={entry.key}
              className={`chart-line ${toneClass(entry.tone)}`}
              d={linePath(entry.values)}
            />
          ))}
          {active !== null && (
            <line
              className="chart-crosshair"
              x1={x(active)}
              x2={x(active)}
              y1={top}
              y2={top + plotHeight}
            />
          )}
          {active !== null &&
            series.map((entry) => {
              const number = geometryNumber(entry.values[active] ?? null);
              if (number === null) {
                return null;
              }
              return (
                <circle
                  key={entry.key}
                  className={`chart-dot ${toneClass(entry.tone)}`}
                  cx={x(active)}
                  cy={y(number)}
                  r={4}
                />
              );
            })}
          {primary && lastNumber !== null && (
            <g>
              <circle
                className={`chart-dot ${toneClass(primary.tone)}`}
                cx={x(lastIndex)}
                cy={y(lastNumber)}
                r={4}
              />
              <text
                className="chart-value-label"
                x={x(lastIndex) + 10}
                y={y(lastNumber)}
                dominantBaseline="middle"
              >
                {format(lastValue ?? null)}
              </text>
            </g>
          )}
          <rect
            className="chart-hit"
            x={leftBand - hitPadding}
            y={top}
            width={plotWidth + hitPadding * 2}
            height={plotHeight}
            onPointerMove={(event) => setActive(pointFromEvent(event))}
            onPointerLeave={() => setActive(null)}
          />
        </svg>
        {active !== null && (
          <div className="chart-tooltip" style={{ left: tooltipLeft }} role="status">
            <span className="chart-tooltip-title">{categories[active]}</span>
            {series.map((entry) => (
              <span key={entry.key} className="chart-tooltip-row">
                <span
                  className={`chart-key chart-key-line ${toneClass(entry.tone)}`}
                  aria-hidden="true"
                />
                <strong>{format(entry.values[active] ?? null)}</strong>
                <span className="muted">{entry.label}</span>
              </span>
            ))}
          </div>
        )}
      </div>
    </ChartFrame>
  );
}
