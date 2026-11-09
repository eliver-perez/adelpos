import { useState } from 'react';
import { SegmentedControl } from '../../components/SegmentedControl';
import type { CashReport } from '../../services/cash';
import { formatDollars, formatMoney, permissionName } from '../../services/payments';
import { serverDateTime } from '../../services/restaurant';
import { DifferenceFigures, ExpectedFigures } from './ExpectedBlock';
import { businessDate } from './movementRows';
import { TotalsTables } from './TotalsTables';

type TotalsView = 'accumulated' | 'since';

export function reportTitle(report: { kind: string; sequence: number }) {
  return report.kind === 'final' ? 'Corte definitivo' : `Corte parcial #${report.sequence}`;
}

// Datos generales de control del corte.
function ReportControl({ report, timezone }: { report: CashReport; timezone: string }) {
  const control = report.snapshot.control;
  return (
    <div className="shift-summary" aria-label="Datos de control del corte">
      <div>
        Caja
        <strong>{control.register_name}</strong>
      </div>
      <div>
        Fecha comercial
        <strong>{businessDate(control.business_date)}</strong>
      </div>
      <div>
        Turno abierto por
        <strong>{control.opened_by_name}</strong>
      </div>
      <div>
        Fondo inicial
        <strong>
          {formatMoney(control.opening_float)}
          {control.opening_float_usd ? ` + ${formatDollars(control.opening_float_usd)}` : ''}
        </strong>
      </div>
      <div>
        Realizado por
        <strong>
          {report.created_by_name} · {report.device_name}
        </strong>
      </div>
      <div>
        Intervalo cubierto
        <strong>
          {serverDateTime(report.covered_from, timezone)} –{' '}
          {serverDateTime(report.covered_to, timezone)}
        </strong>
      </div>
    </div>
  );
}

// Lo declarado en el arqueo (solo en el corte definitivo).
function DeclaredCash({ report }: { report: CashReport }) {
  const declared = report.snapshot.declared;
  if (!declared) {
    return null;
  }
  return (
    <div className="shift-summary" aria-label="Efectivo declarado">
      <div>
        Pesos declarados
        <strong>{formatMoney(declared.counted_mxn)}</strong>
      </div>
      <div>
        Dólares declarados
        <strong>{formatDollars(declared.counted_usd)}</strong>
      </div>
      {declared.cashier_notes && (
        <div>
          Observaciones del cajero
          <strong>{declared.cashier_notes}</strong>
        </div>
      )}
    </div>
  );
}

// Corte parcial o definitivo tal como lo guardó Go. El bloque del efectivo esperado y las
// diferencias solo aparece si Go lo entregó y la pantalla no está en un arqueo (blind).
export function CashReportView({
  report,
  timezone,
  blind = false,
}: {
  report: CashReport;
  timezone: string;
  blind?: boolean;
}) {
  const [view, setView] = useState<TotalsView>('accumulated');
  const sensitive = !blind && report.expected_visible ? report.snapshot.sensitive : undefined;
  const totals = view === 'since' ? report.snapshot.since_previous : report.snapshot.accumulated;
  return (
    <article className="cash-report" aria-label={reportTitle(report)}>
      <header className="cash-panel-header">
        <h4>{reportTitle(report)}</h4>
        <span className="muted text-xs">{serverDateTime(report.created_at, timezone)}</span>
      </header>
      {report.kind === 'partial' && (
        <p className="muted text-sm">
          Informe del turno hasta este momento. Un corte parcial no retira dinero, no cierra el
          turno ni pausa la caja.
        </p>
      )}
      <ReportControl report={report} timezone={timezone} />
      {report.previous_report_id && (
        <SegmentedControl<TotalsView>
          label="Totales del corte"
          value={view}
          onChange={setView}
          options={[
            { value: 'accumulated', label: 'Acumulado del turno' },
            { value: 'since', label: 'Desde el corte anterior' },
          ]}
        />
      )}
      <TotalsTables totals={totals} />
      <DeclaredCash report={report} />
      {sensitive && <ExpectedFigures expected={sensitive.expected} />}
      {sensitive?.differences && <DifferenceFigures differences={sensitive.differences} />}
      {!sensitive && !blind && (
        <p className="muted text-xs">
          El efectivo esperado y las diferencias se muestran con el permiso «
          {permissionName('cash.expected.read')}».
        </p>
      )}
    </article>
  );
}
