import { Notice } from '../../components/Forms';
import { useLiveResource } from '../../hooks/useLiveResource';
import {
  formatDollars,
  formatMoney,
  isZeroMoney,
  movementKindLabels,
  permissionName,
} from '../../services/payments';
import type { CurrentShift, Shift, ShiftMovements } from '../../services/payments';
import { serverDateTime } from '../../services/restaurant';

function ShiftMovementsTable({ shift, timezone }: { shift: Shift; timezone: string }) {
  const movements = useLiveResource<ShiftMovements>('payments_shift_movements', {
    id: shift.id,
    interval: 10000,
  });
  if (movements.error) {
    return <Notice>{movements.error}</Notice>;
  }
  if (!movements.data) {
    return <p className="muted">Consultando movimientos del turno…</p>;
  }
  const data = movements.data;
  const currency = data.by_currency;
  // Go solo entrega el efectivo esperado con el permiso correspondiente y nunca durante el arqueo
  // de cierre: la interfaz no lo calcula ni lo deduce.
  const expectedShown = data.expected_cash !== undefined;
  // Los totales por moneda solo se muestran cuando el turno movió dólares o tiene tipo de cambio.
  const dollars =
    !!currency &&
    (!!shift.usd_rate || !isZeroMoney(currency.usd_in) || !isZeroMoney(currency.usd_out));
  return (
    <div className="form-stack">
      <div className="shift-summary">
        <div>
          Efectivo neto cobrado
          <strong>{formatMoney(data.cash_net)}</strong>
        </div>
        {expectedShown && (
          <div>
            Pesos esperados en caja
            <strong>{formatMoney(data.expected_cash)}</strong>
          </div>
        )}
      </div>
      {!expectedShown && (
        <p className="muted text-sm">
          {shift.state === 'closing'
            ? 'Arqueo de cierre en curso: el efectivo esperado no se muestra durante el conteo.'
            : `El efectivo esperado se muestra con el permiso «${permissionName('cash.expected.read')}».`}
        </p>
      )}
      {dollars && currency && (
        <div className="shift-summary" aria-label="Efectivo físico por moneda">
          <div>
            Pesos físicos esperados
            <strong>{formatMoney(currency.expected_mxn)}</strong>
          </div>
          <div>
            Dólares físicos esperados
            <strong>{formatDollars(currency.expected_usd)}</strong>
          </div>
          <div>
            Dólares recibidos / entregados
            <strong>
              {formatDollars(currency.usd_in)} / {formatDollars(currency.usd_out)}
            </strong>
          </div>
        </div>
      )}
      <h3 className="bill-section-title">Totales por método</h3>
      <div className="table-container">
        <table>
          <thead>
            <tr>
              <th>Método</th>
              <th>Ventas</th>
              <th>Propinas</th>
              <th>Neto</th>
            </tr>
          </thead>
          <tbody>
            {data.by_method.map((total) => (
              <tr key={total.method_name}>
                <td>
                  <strong>{total.method_name}</strong>
                  {total.is_cash && <span className="muted"> · efectivo</span>}
                </td>
                <td>{formatMoney(total.sales)}</td>
                <td>{formatMoney(total.tips)}</td>
                <td>{formatMoney(total.net)}</td>
              </tr>
            ))}
            {data.by_method.length === 0 && (
              <tr>
                <td colSpan={4} className="muted">
                  Sin movimientos todavía.
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
      <h3 className="bill-section-title">Movimientos</h3>
      <div className="table-container">
        <table>
          <thead>
            <tr>
              <th>Fecha</th>
              <th>Cuenta</th>
              <th>Tipo</th>
              <th>Método</th>
              <th>Venta</th>
              <th>Propina</th>
              <th>Recibido</th>
              <th>Cambio</th>
              <th>Usuario</th>
            </tr>
          </thead>
          <tbody>
            {data.movements.map((movement) => (
              <tr key={movement.id}>
                <td>{serverDateTime(movement.created_at, timezone)}</td>
                <td>#{movement.folio}</td>
                <td>{movementKindLabels[movement.kind] ?? 'Movimiento'}</td>
                <td>{movement.method_name}</td>
                <td>{formatMoney(movement.sale_amount)}</td>
                <td>{formatMoney(movement.tip_amount)}</td>
                <td>{movement.is_cash ? formatMoney(movement.cash_received) : '—'}</td>
                <td>{movement.is_cash ? formatMoney(movement.change_given) : '—'}</td>
                <td>{movement.actor_name}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

// Turno actual de la caja asociada a esta terminal, con sus movimientos y totales por método.
export function ShiftOverview({
  can,
  timezone,
}: {
  can: (permission: string) => boolean;
  timezone: string;
}) {
  const current = useLiveResource<CurrentShift>('payments_current_shift', {
    enabled: can('payments.read'),
    interval: 15000,
  });
  if (!can('payments.read')) {
    return (
      <p className="muted text-sm">
        Consultar el turno requiere el permiso «{permissionName('payments.read')}».
      </p>
    );
  }
  if (current.error) {
    return <Notice>{current.error}</Notice>;
  }
  if (!current.data) {
    return <p className="muted">Consultando turno…</p>;
  }
  const shift = current.data.shift;
  return (
    <div className="form-stack">
      {!shift && <Notice>{current.data.reason}</Notice>}
      {shift && (
        <div className="shift-summary">
          <div>
            Caja
            <strong>{shift.register_name}</strong>
          </div>
          <div>
            Fondo inicial
            <strong>{formatMoney(shift.opening_float)}</strong>
          </div>
          <div>
            Abierto por
            <strong>{shift.opened_by_name}</strong>
          </div>
          <div>
            Apertura
            <strong>{serverDateTime(shift.opened_at, timezone)}</strong>
          </div>
          <div>
            Uso del turno
            <strong>{current.data.shared_shifts ? 'Compartido' : 'Solo quien lo abrió'}</strong>
          </div>
          {(current.data.usd_cash || shift.usd_rate) && (
            <div>
              Tipo de cambio del dólar
              <strong>
                {shift.usd_rate ? `${formatMoney(shift.usd_rate)} por dólar` : 'Sin capturar'}
              </strong>
            </div>
          )}
        </div>
      )}
      {shift && shift.state === 'closing' && (
        <div className="shift-summary" aria-label="Arqueo de cierre en curso">
          <div>
            Estado
            <strong>Cierre en curso</strong>
          </div>
          {shift.closing_started_by_name && (
            <div>
              Arqueo iniciado por
              <strong>{shift.closing_started_by_name}</strong>
            </div>
          )}
          {shift.closing_started_at && (
            <div>
              Inicio del arqueo
              <strong>{serverDateTime(shift.closing_started_at, timezone)}</strong>
            </div>
          )}
        </div>
      )}
      {shift && current.data.reason && <Notice>{current.data.reason}</Notice>}
      {shift && can('cash.shifts.read') && (
        <ShiftMovementsTable key={shift.id} shift={shift} timezone={timezone} />
      )}
      {shift && !can('cash.shifts.read') && (
        <p className="muted text-sm">
          Consultar movimientos requiere el permiso «{permissionName('cash.shifts.read')}».
        </p>
      )}
      <p className="muted text-xs">
        La apertura del turno se realiza desde la pantalla de cobro. Cortes y cierres de caja
        pertenecen a la siguiente etapa.
      </p>
    </div>
  );
}
