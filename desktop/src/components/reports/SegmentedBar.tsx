import { useRef, useState } from 'react';
import type { ColumnType, ReportValue } from '../../services/reports';
import { ChartFrame, toneClass } from './ChartFrame';
import type { SeriesTone } from './ChartFrame';
import { useElementWidth } from './chartScale';
import { formatReportValue, geometryNumber } from './reportValues';

// Barra segmentada para una distribución (métodos de pago, modalidades, tasas de IVA…) en lugar de
// un pastel: un todo de 28 px dividido en hasta cinco partes con 2 px de separación del color de la
// superficie. El importe de Go se imprime dentro de la parte solo si cabe; la leyenda nombra las
// partes en su orden y la tabla alternativa tiene todos los valores. React no calcula porcentajes:
// la proporción de cada parte es solo geometría.

export interface SegmentPart {
  key: string;
  label: string;
  value: ReportValue;
}

const barHeight = 28;
const gap = 2;
export const maxSegments = 5;

export function SegmentedBar({
  title,
  description,
  categoryLabel,
  valueLabel,
  parts,
  type,
  timezone,
  busy = false,
}: {
  title: string;
  description?: string;
  categoryLabel: string;
  valueLabel: string;
  parts: SegmentPart[];
  type: ColumnType;
  timezone: string;
  busy?: boolean;
}) {
  const container = useRef<HTMLDivElement>(null);
  const width = Math.max(240, useElementWidth(container, 720));
  const [active, setActive] = useState<number | null>(null);
  const visible = parts.slice(0, maxSegments);
  const numbers = visible.map((part) => Math.max(0, geometryNumber(part.value) ?? 0));
  const whole = numbers.reduce((sum, number) => sum + number, 0);
  const drawn = numbers.filter((number) => number > 0).length;
  const available = width - gap * Math.max(0, drawn - 1);
  const format = (value: ReportValue) => formatReportValue(type, value, timezone);
  // El color sigue a la parte (por su nombre en orden alfabético), no a su posición en el orden de
  // la tabla: si cambia el ranking, cada método o categoría conserva su color.
  const byName = visible
    .map((part) => part.label)
    .sort((left, right) => left.localeCompare(right, 'es'));
  let cursor = 0;
  const blocks = visible.map((part, index) => {
    const number = numbers[index] ?? 0;
    const length = whole > 0 ? (number / whole) * available : 0;
    const tone = (byName.indexOf(part.label) + 1) as SeriesTone;
    const block = { part, index, x: cursor, length, tone };
    if (length > 0) {
      cursor += length + gap;
    }
    return block;
  });
  const table = {
    columns: [categoryLabel, valueLabel],
    rows: visible.map((part) => [part.label, format(part.value)]),
  };

  return (
    <ChartFrame title={title} description={description} table={table} busy={busy}>
      <div ref={container} className="chart-canvas segmented-chart">
        <svg
          width={width}
          height={barHeight}
          role="img"
          aria-label={`${title}: ${visible.map((part) => `${part.label} ${format(part.value)}`).join('; ')}.`}
        >
          {blocks.map((block) => {
            if (block.length <= 0) {
              return null;
            }
            const text = format(block.part.value);
            // Cabe el rótulo si hay al menos 7 px por carácter y 8 px de margen a cada lado.
            const fits = block.length >= text.length * 7 + 16;
            return (
              <g
                key={block.part.key}
                onPointerEnter={() => setActive(block.index)}
                onPointerLeave={() => setActive(null)}
              >
                <title>{`${block.part.label}: ${text}`}</title>
                <rect
                  className={`chart-segment ${toneClass(block.tone)} ${active === block.index ? 'chart-segment-active' : ''}`}
                  x={block.x}
                  y={0}
                  width={block.length}
                  height={barHeight}
                  rx={4}
                />
                {fits && (
                  <text
                    className={`chart-segment-label ${block.tone === 4 ? 'chart-segment-label-dark' : ''}`}
                    x={block.x + 8}
                    y={barHeight / 2}
                    dominantBaseline="middle"
                  >
                    {text}
                  </text>
                )}
              </g>
            );
          })}
        </svg>
        <ul className="chart-legend" aria-label="Leyenda">
          {blocks.map((block) => (
            <li
              key={block.part.key}
              className={active === block.index ? 'chart-legend-active' : ''}
            >
              <span
                className={`chart-key chart-key-bar ${toneClass(block.tone)}`}
                aria-hidden="true"
              />
              {block.part.label}
            </li>
          ))}
        </ul>
      </div>
    </ChartFrame>
  );
}
