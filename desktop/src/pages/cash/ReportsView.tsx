import { useState } from 'react';
import { Accordion } from '../../components/Accordion';
import { Notice } from '../../components/Forms';
import { SegmentedControl } from '../../components/SegmentedControl';
import { useLiveResource } from '../../hooks/useLiveResource';
import { cashDifferenceLabels } from '../../services/cash';
import type { ShiftHistory, ShiftSummary } from '../../services/cash';
import { formatDollars, formatMoney, isZeroMoney, permissionName } from '../../services/payments';
import type { Shift } from '../../services/payments';
import { serverDateTime } from '../../services/restaurant';
import { ExpectedFigures } from './ExpectedBlock';
import { businessDate } from './movementRows';
import { PhysicalFigures, TotalsTables } from './TotalsTables';

type SummaryView = 'accumulated' | 'since';

// Resumen del turno actual con el cálculo de los cortes (Go), sin registrar un corte.
function CurrentSummary({ shift, timezone }: { shift: Shift; timezone: string }) {
  const [view, setView] = useState<SummaryView>('accumulated');
  const summary = useLiveResource<ShiftSummary>('payments_shift_summary', {
    id: shift.id,
    interval: 15000,
  });
  if (summary.error) {
    return <Notice>{summary.error}</Notice>;
  }
  if (!summary.data) {
    return <p className="muted">Consultando el resumen del turno…</p>;
  }
  const data = summary.data;
  if (data.blind || !data.accumulated || !data.since_last_cut) {
    return (
      <Notice>
        Arqueo de cierre en curso en {shift.register_name}: la captura es ciega y el resumen del
        turno se consulta al terminar el arqueo.
      </Notice>
    );
  }
  const totals = view === 'since' ? data.since_last_cut : data.accumulated;
  return (
    <div className="form-stack">
      <p className="muted text-sm">
        Resumen al {serverDateTime(data.generated_at, timezone)} · fondo inicial{' '}
        {formatMoney(data.control.opening_float)}
        {data.control.opening_float_usd
          ? ` + ${formatDollars(data.control.opening_float_usd)}`
          : ''}{' '}
        · {data.partial_cuts} cortes parciales
        {data.last_cut ? ` (último: ${serverDateTime(data.last_cut.created_at, timezone)})` : ''}.
      </p>
      {data.last_cut && (
        <SegmentedControl<SummaryView>
          label="Periodo del resumen"
          value={view}
          onChange={setView}
          options={[
            { value: 'accumulated', label: 'Acumulado del turno' },
            { value: 'since', label: 'Desde el último corte' },
          ]}
        />
      )}
      <TotalsTables totals={totals} />
      {data.physical && <PhysicalFigures physical={data.physical} />}
      {data.expected_visible && data.expected && <ExpectedFigures expected={data.expected} />}
      {!data.expected_visible && (
        <p className="muted text-xs">
          El efectivo esperado y el efectivo recibido y entregado por moneda se muestran con el
          permiso «{permissionName('cash.expected.read')}».
        </p>
      )}
    </div>
  );
}

// Cierres recientes de la caja con lo declarado y, si Go las entrega, sus diferencias.
function RecentClosures({ registerID, timezone }: { registerID: string; timezone: string }) {
  const history = useLiveResource<ShiftHistory>('payments_shifts', {
    query: { register_id: registerID, state: 'closed', page: '1' },
    interval: 60000,
  });
  if (history.error) {
    return <Notice>{history.error}</Notice>;
  }
  const items = history.data?.items.slice(0, 10) ?? [];
  return (
    <div className="table-container cash-table">
      <table>
        <thead>
          <tr>
            <th scope="col">Fecha comercial</th>
            <th scope="col">Cierre</th>
            <th scope="col">Declarado</th>
            <th scope="col">Esperado</th>
            <th scope="col">Diferencia</th>
          </tr>
        </thead>
        <tbody>
          {items.map(({ shift, closure }) => (
            <tr key={shift.id}>
              <td>{businessDate(shift.business_date)}</td>
              <td>{shift.closed_at ? serverDateTime(shift.closed_at, timezone) : '—'}</td>
              <td>
                {formatMoney(closure?.counted_mxn)}
                {closure && !isZeroMoney(closure.counted_usd)
                  ? ` + ${formatDollars(closure.counted_usd)}`
                  : ''}
              </td>
              <td>{closure?.expected_mxn ? formatMoney(closure.expected_mxn) : '—'}</td>
              <td>
                {closure?.difference_mxn && closure.result_mxn
                  ? `${formatMoney(closure.difference_mxn)} · ${cashDifferenceLabels[closure.result_mxn]}`
                  : '—'}
              </td>
            </tr>
          ))}
          {items.length === 0 && (
            <tr>
              <td colSpan={5} className="muted">
                Sin cierres registrados.
              </td>
            </tr>
          )}
        </tbody>
      </table>
    </div>
  );
}

// Reportes operativos de caja (§21): resumen del turno actual y cierres recientes, siempre con
// cifras de Go y respetando permisos. El tablero completo pertenece a la etapa 9.
export function ReportsView({
  registerID,
  shift,
  can,
  timezone,
}: {
  registerID: string;
  shift: Shift | null;
  can: (permission: string) => boolean;
  timezone: string;
}) {
  return (
    <div className="form-stack">
      <Accordion
        title="Resumen del turno actual"
        summary="Ventas, propinas, devoluciones, ingresos, retiros, gastos y cortes"
        defaultOpen
      >
        {shift ? (
          <CurrentSummary key={shift.id} shift={shift} timezone={timezone} />
        ) : (
          <p className="muted text-sm">La caja no tiene un turno activo.</p>
        )}
      </Accordion>
      {can('cash.history.read') && registerID && (
        <Accordion
          title="Cierres recientes"
          summary="Lo declarado y, con permiso, esperado y diferencias"
        >
          <RecentClosures registerID={registerID} timezone={timezone} />
        </Accordion>
      )}
    </div>
  );
}
