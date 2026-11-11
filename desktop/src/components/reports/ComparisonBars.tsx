import { useRef } from 'react';
import type { ColumnType, ReportValue } from '../../services/reports';
import { ChartFrame, toneClass } from './ChartFrame';
import type { SeriesTone } from './ChartFrame';
import { barPath, niceTicks, useElementWidth } from './chartScale';
import { formatReportValue, geometryNumber } from './reportValues';

// Barras horizontales para comparar (§5: «barras para comparaciones»): un ranking (una serie) o el
// periodo actual frente al anterior (dos series, la anterior en gris). Las etiquetas largas en
// español caben mejor en horizontal. Barras de 20 px como máximo (12 px cada una con dos series),
// extremo de datos redondeado y línea base recta en cero; el valor de Go se rotula en la punta.

export interface BarItem {
  key: string;
  label: string;
  // Un valor por serie, en el orden de «series».
  values: ReportValue[];
}

const labelGap = 12;
const valueBand = 104;

export function ComparisonBars({
  title,
  description,
  categoryLabel,
  series,
  items,
  type,
  timezone,
  busy = false,
}: {
  title: string;
  description?: string;
  categoryLabel: string;
  series: { label: string; tone: SeriesTone }[];
  items: BarItem[];
  type: ColumnType;
  timezone: string;
  busy?: boolean;
}) {
  const container = useRef<HTMLDivElement>(null);
  const width = Math.max(320, useElementWidth(container, 720));
  const labelWidth = Math.min(240, Math.round(width * 0.32));
  const trackWidth = width - labelWidth - labelGap;
  const plotWidth = Math.max(40, trackWidth - valueBand * (items.length > 0 ? 1 : 0));
  const numbers = items.flatMap((item) =>
    item.values.map(geometryNumber).filter((value): value is number => value !== null),
  );
  const minimum = Math.min(0, ...numbers);
  const maximum = Math.max(0, ...numbers);
  const scale = niceTicks(minimum, maximum, 4);
  // Con valores negativos, la línea base se desplaza y los rótulos de esas barras van a su izquierda.
  const negativeBand = minimum < 0 ? valueBand : 0;
  const domainWidth = plotWidth - negativeBand;
  const position = (value: number) =>
    negativeBand + ((value - scale.low) / (scale.high - scale.low)) * domainWidth;
  const baseline = position(0);
  const barHeight = series.length > 1 ? 12 : 20;
  const barGap = 2;
  const rowHeight = series.length * barHeight + (series.length - 1) * barGap + 16;
  const format = (value: ReportValue) => formatReportValue(type, value, timezone);
  const table = {
    columns: [categoryLabel, ...series.map((entry) => entry.label)],
    rows: items.map((item) => [item.label, ...item.values.map(format)]),
  };

  return (
    <ChartFrame
      title={title}
      description={description}
      legend={series.map((entry) => ({ label: entry.label, tone: entry.tone, shape: 'bar' }))}
      table={table}
      busy={busy}
    >
      <div
        ref={container}
        className="chart-canvas bar-chart"
        role="img"
        aria-label={`${title}: ${items
          .map((item) => `${item.label} ${item.values.map(format).join(' / ')}`)
          .join('; ')}.`}
      >
        {items.map((item) => (
          <div
            key={item.key}
            className="bar-row"
            style={{ gridTemplateColumns: `${labelWidth}px ${trackWidth}px` }}
          >
            <span className="bar-label">{item.label}</span>
            <svg width={trackWidth} height={rowHeight} aria-hidden="true">
              <line className="chart-axis" x1={baseline} x2={baseline} y1={4} y2={rowHeight - 4} />
              {item.values.map((value, index) => {
                const number = geometryNumber(value);
                const tone = series[index]?.tone ?? 1;
                const barTop = 8 + index * (barHeight + barGap);
                const end = number === null ? baseline : position(number);
                const negative = number !== null && number < 0;
                return (
                  <g key={index}>
                    <title>{`${item.label} · ${series[index]?.label ?? ''}: ${format(value)}`}</title>
                    {number !== null && number !== 0 && (
                      <path
                        className={`chart-bar ${toneClass(tone)}`}
                        d={barPath(baseline, end, barTop, barHeight)}
                      />
                    )}
                    <text
                      className={index === 0 ? 'chart-value-label' : 'chart-axis-label'}
                      x={negative ? end - 6 : end + 6}
                      y={barTop + barHeight / 2}
                      textAnchor={negative ? 'end' : 'start'}
                      dominantBaseline="middle"
                    >
                      {format(value)}
                    </text>
                  </g>
                );
              })}
            </svg>
          </div>
        ))}
      </div>
    </ChartFrame>
  );
}
