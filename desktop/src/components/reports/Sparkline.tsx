import { geometryNumber } from './reportValues';
import type { ReportValue } from '../../services/reports';

// Minigráfico de una tarjeta del resumen: solo la forma de la tendencia, sin ejes ni rótulos. Es
// decorativo (aria-hidden): la tarjeta ya dice el valor y la serie completa está en su gráfico y en
// su tabla alternativa. Línea en el tono atenuado y el último punto en el acento.

const width = 120;
const height = 32;
const padding = 4;

export function Sparkline({ values }: { values: ReportValue[] }) {
  const numbers = values.map(geometryNumber);
  const present = numbers.filter((value): value is number => value !== null);
  if (present.length < 2) {
    return null;
  }
  const low = Math.min(...present);
  const high = Math.max(...present);
  const span = high - low || 1;
  const step = (width - padding * 2) / Math.max(1, numbers.length - 1);
  const points = numbers
    .map((value, index) =>
      value === null
        ? null
        : {
            x: padding + index * step,
            y: height - padding - ((value - low) / span) * (height - padding * 2),
          },
    )
    .filter((point): point is { x: number; y: number } => point !== null);
  const last = points[points.length - 1]!;
  return (
    <svg
      className="sparkline"
      width={width}
      height={height}
      viewBox={`0 0 ${width} ${height}`}
      aria-hidden="true"
      focusable="false"
    >
      <polyline
        className="sparkline-line"
        points={points.map((point) => `${point.x},${point.y}`).join(' ')}
      />
      <circle className="sparkline-dot" cx={last.x} cy={last.y} r={3} />
    </svg>
  );
}
