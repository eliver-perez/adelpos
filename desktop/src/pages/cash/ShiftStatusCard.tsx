import { shiftStateLabels } from '../../services/cash';
import { formatDollars, formatMoney } from '../../services/payments';
import type { Shift } from '../../services/payments';
import { serverDateTime } from '../../services/restaurant';
import { businessDate } from './movementRows';

// Aviso destacado de «Cierre en curso»: la caja está pausada por el arqueo. Indica quién lo
// inició, cuándo y desde qué terminal.
export function ClosingNotice({ shift, timezone }: { shift: Shift; timezone: string }) {
  if (shift.state !== 'closing') {
    return null;
  }
  return (
    <div className="cash-closing-banner" role="status" aria-label="Cierre en curso">
      <strong>Cierre en curso · {shift.register_name}</strong>
      <p>
        Arqueo iniciado por {shift.closing_started_by_name ?? 'otro usuario'}
        {shift.closing_device_name ? ` desde ${shift.closing_device_name}` : ''}
        {shift.closing_started_at ? ` (${serverDateTime(shift.closing_started_at, timezone)})` : ''}
        . La caja no acepta cobros, devoluciones, ingresos, retiros, gastos ni correcciones hasta
        que se confirme el cierre o se cancele el arqueo.
      </p>
    </div>
  );
}

// Datos del turno: estado, responsable, apertura, fecha comercial y fondo inicial.
export function ShiftStatusCard({ shift, timezone }: { shift: Shift; timezone: string }) {
  return (
    <div className="shift-summary" aria-label={`Turno de ${shift.register_name}`}>
      <div>
        Estado
        <strong className={`cash-state cash-state-${shift.state}`}>
          {shiftStateLabels[shift.state] ?? shift.state}
        </strong>
      </div>
      <div>
        Responsable
        <strong>{shift.opened_by_name}</strong>
      </div>
      <div>
        Apertura
        <strong>
          {serverDateTime(shift.opened_at, timezone)} · {shift.opened_device_name}
        </strong>
      </div>
      <div>
        Fecha comercial
        <strong>{businessDate(shift.business_date)}</strong>
      </div>
      <div>
        Fondo inicial
        <strong>
          {formatMoney(shift.opening_float)}
          {shift.opening_float_usd ? ` + ${formatDollars(shift.opening_float_usd)}` : ''}
        </strong>
      </div>
      {shift.usd_rate && (
        <div>
          Tipo de cambio del turno
          <strong>{formatMoney(shift.usd_rate)} por dólar</strong>
        </div>
      )}
      {shift.closed_at && (
        <div>
          Cierre
          <strong>
            {serverDateTime(shift.closed_at, timezone)} · {shift.closed_by_name ?? ''}
          </strong>
        </div>
      )}
      {shift.opening_notes && (
        <div>
          Observaciones de apertura
          <strong>{shift.opening_notes}</strong>
        </div>
      )}
    </div>
  );
}
