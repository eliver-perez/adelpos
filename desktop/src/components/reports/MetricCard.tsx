import type { DashboardIndicator, ReportValue } from '../../services/reports';
import { Sparkline } from './Sparkline';
import {
  decimalSign,
  formatReportValue,
  formatSignedPercent,
  formatSignedValue,
} from './reportValues';

// Tarjeta de un indicador del resumen ejecutivo (§5): valor del periodo, variación contra el
// periodo anterior equivalente y valor anterior, todo calculado por Go. La variación se dice con
// texto y flecha, nunca solo con color. El tono depende de la polaridad: subir es favorable en
// ventas o cobros, desfavorable en descuentos, cortesías, devoluciones o mermas, y neutral (sin
// tono) en lo demás, como compras o consumo. Con un reporte detallado disponible, la tarjeta es un
// botón que lo abre con el mismo periodo.

export type MetricPolarity = 'up-good' | 'up-bad' | 'neutral';

export function MetricCard({
  indicator,
  timezone,
  compared,
  trend,
  polarity = 'neutral',
  onOpen,
}: {
  indicator: DashboardIndicator;
  timezone: string;
  compared: boolean;
  trend?: ReportValue[];
  polarity?: MetricPolarity;
  onOpen?: () => void;
}) {
  const value = formatReportValue(indicator.type, indicator.value, timezone);
  const hasValue = indicator.value !== null && indicator.value !== undefined;
  const hasPrevious = compared && indicator.previous !== null && indicator.previous !== undefined;
  const direction = decimalSign(indicator.change);
  let tone = 'metric-delta-flat';
  if (direction !== 0 && polarity !== 'neutral') {
    const rising = direction > 0;
    const risingIsGood = polarity === 'up-good';
    const favorable = rising === risingIsGood;
    tone = favorable ? 'metric-delta-good' : 'metric-delta-bad';
  }
  const arrow = direction > 0 ? '▲' : direction < 0 ? '▼' : '■';
  const change =
    indicator.change_percent !== null
      ? formatSignedPercent(indicator.change_percent)
      : formatSignedValue(indicator.type, indicator.change);
  const body = (
    <>
      <span className="metric-label">{indicator.label}</span>
      <strong className="metric-value">{value}</strong>
      {!hasValue && <span className="muted text-xs">Sin datos suficientes en el periodo.</span>}
      {hasValue && hasPrevious && (
        <span className={`metric-delta ${tone}`}>
          <span aria-hidden="true">{arrow}</span>
          {direction === 0
            ? ' Sin cambio frente al periodo anterior'
            : ` ${change} frente al periodo anterior`}
        </span>
      )}
      {hasPrevious && (
        <span className="muted text-xs">
          Anterior: {formatReportValue(indicator.type, indicator.previous, timezone)}
        </span>
      )}
      {indicator.hint && <span className="metric-hint muted text-xs">{indicator.hint}</span>}
      {trend && <Sparkline values={trend} />}
      {onOpen && <span className="metric-open text-xs">Ver reporte detallado</span>}
    </>
  );
  if (!onOpen) {
    return <div className="metric-card">{body}</div>;
  }
  return (
    <button type="button" className="metric-card metric-card-link" onClick={onOpen}>
      {body}
    </button>
  );
}
