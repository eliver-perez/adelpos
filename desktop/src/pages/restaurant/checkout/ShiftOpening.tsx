import { useState } from 'react';
import { AmountField, NumericKeypad } from '../../../components/NumericKeypad';
import { APIError } from '../../../services/api';
import { toWireAmount } from '../../../services/amountInput';
import { formatMoney, permissionName } from '../../../services/payments';
import type { CurrentShift } from '../../../services/payments';
import { useRestaurantMutation } from '../../../hooks/useRestaurantMutation';

export const shiftPendingKey = 'payments:shift';

// Turno no utilizable: muestra el motivo de Go y, si falta el turno y el usuario puede abrirlo,
// ofrece la apertura en la misma pantalla. El fondo inicial se captura siempre de forma explícita
// (se permite 0); nunca se abre un turno automáticamente.
export function ShiftOpening({
  current,
  can,
  active,
  onChanged,
}: {
  current: CurrentShift;
  can: (permission: string) => boolean;
  active: boolean;
  onChanged: () => void;
}) {
  const [opening, setOpening] = useState('');
  const mutation = useRestaurantMutation(() => {
    setOpening('');
    onChanged();
    window.dispatchEvent(new CustomEvent('adel-server-change'));
  }, shiftPendingKey);
  const register = current.register;
  const openable = !!register && !current.shift && can('cash.shifts.open');
  const wire = toWireAmount(opening);

  function openLabel() {
    if (mutation.busy) {
      return 'Abriendo turno…';
    }
    if (wire === '') {
      return 'Captura el fondo inicial';
    }
    return `Abrir turno con fondo de ${formatMoney(wire)}`;
  }

  async function open() {
    if (wire === '' || mutation.isPending()) {
      return;
    }
    try {
      await mutation.execute('payments_open_shift', { body: { opening_float: wire } });
    } catch (problem) {
      // Un turno abierto por otra terminal mientras se capturaba el fondo se refleja al consultar.
      if (problem instanceof APIError && problem.status === 409) {
        onChanged();
      }
    }
  }

  return (
    <section className="checkout-shift" aria-label="Turno de caja">
      <h3>{register ? `Turno de ${register.name}` : 'Caja no asignada'}</h3>
      <p className="checkout-inline-note">{current.reason}</p>
      {mutation.error && (
        <p className="checkout-inline-note" role="alert">
          {mutation.error}
        </p>
      )}
      {mutation.uncertain && (
        <div className="operation-recovery">
          <p>
            No se pudo confirmar la apertura del turno. Reintenta con el mismo identificador o
            consulta el turno actual.
          </p>
          <button
            type="button"
            className="primary-button"
            disabled={mutation.busy}
            onClick={() => void mutation.retry()}
          >
            Reintentar con el mismo identificador
          </button>
          <button type="button" className="secondary-button" onClick={onChanged}>
            Consultar turno
          </button>
        </div>
      )}
      {register && !current.shift && !can('cash.shifts.open') && (
        <p className="muted text-sm">
          Para abrir el turno se necesita el permiso «{permissionName('cash.shifts.open')}».
        </p>
      )}
      {openable && (
        <>
          <AmountField
            label="Fondo inicial"
            value={opening}
            onChange={setOpening}
            active
            emphasis
            placeholder="0.00"
            hint="Obligatorio. Captura 0 si la caja inicia sin efectivo."
          />
          <NumericKeypad
            value={opening}
            onChange={setOpening}
            label="Teclado numérico del fondo inicial"
            captureKeyboard={active}
            disabled={mutation.busy || mutation.uncertain}
          />
          <button
            type="button"
            className="primary-button checkout-register"
            disabled={wire === '' || mutation.busy || !!mutation.pending}
            onClick={() => void open()}
          >
            {openLabel()}
          </button>
        </>
      )}
    </section>
  );
}
