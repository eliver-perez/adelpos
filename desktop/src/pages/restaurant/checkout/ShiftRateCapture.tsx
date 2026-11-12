import { useState } from 'react';
import { AmountField, NumericKeypad } from '../../../components/NumericKeypad';
import { ConfirmDialog } from '../../../components/ConfirmDialog';
import { IconLabel } from '../../../components/Icon';
import { APIError } from '../../../services/api';
import { toWireAmount } from '../../../services/amountInput';
import { notifySuccess } from '../../../services/notify';
import { formatMoney } from '../../../services/payments';
import type { Shift } from '../../../services/payments';
import { useRestaurantMutation } from '../../../hooks/useRestaurantMutation';
import { notifyActionError } from './actionErrors';

// Clave compartida de las escrituras del turno desde el cobro y desde Caja: una apertura o un
// tipo de cambio incierto se recupera en cualquiera de las dos pantallas.
export const shiftPendingKey = 'payments:shift';

// Go pide confirmar un tipo de cambio que difiere mucho del turno anterior (evita 1.95 por 19.50).
export const confirmationCode = 'USD_RATE_CONFIRMATION_REQUIRED';

// Diálogo de confirmación de un tipo de cambio con diferencia grande; el mensaje lo redacta Go.
export function RateConfirmation({
  message,
  onConfirm,
  onCancel,
}: {
  message: string;
  onConfirm: () => void;
  onCancel: () => void;
}) {
  return (
    <ConfirmDialog
      title="Confirmar tipo de cambio"
      message={message}
      confirmLabel="Sí, el tipo de cambio es correcto"
      cancelLabel="Revisar el tipo de cambio"
      onConfirm={onConfirm}
      onCancel={onCancel}
    />
  );
}

// Captura única del tipo de cambio de un turno abierto que no lo tiene (por ejemplo, porque la
// función se activó con el turno ya abierto). Queda fijo hasta el cierre del turno.
export function ShiftRateCapture({
  shift,
  active,
  onChanged,
  onCancel,
}: {
  shift: Shift;
  active: boolean;
  onChanged: () => void;
  onCancel: () => void;
}) {
  const [rate, setRate] = useState('');
  const [confirmation, setConfirmation] = useState('');
  const mutation = useRestaurantMutation(() => {
    notifySuccess(`Tipo de cambio del turno de ${shift.register_name} registrado.`);
    setRate('');
    onChanged();
    window.dispatchEvent(new CustomEvent('adel-server-change'));
  }, shiftPendingKey);
  const wire = toWireAmount(rate);

  async function save(confirmed = false) {
    if (wire === '' || mutation.isPending()) {
      return;
    }
    const body: Record<string, unknown> = { usd_rate: wire };
    if (confirmed) {
      body.confirm_rate_difference = true;
    }
    try {
      await mutation.execute('payments_shift_usd_rate', { id: shift.id, body });
    } catch (problem) {
      if (problem instanceof APIError && problem.code === confirmationCode) {
        setConfirmation(problem.message);
        return;
      }
      notifyActionError(problem, 'No se pudo registrar el tipo de cambio.');
      if (problem instanceof APIError && problem.status === 409) {
        onChanged();
      }
    }
  }

  async function retry() {
    const problem = await mutation.retry();
    if (problem) {
      notifyActionError(problem, 'No se pudo registrar el tipo de cambio.');
    }
  }

  return (
    <section className="checkout-area checkout-capture" aria-label="Tipo de cambio del turno">
      <header className="checkout-area-header">
        <h3>Tipo de cambio de {shift.register_name}</h3>
        <button type="button" className="secondary-button" onClick={onCancel}>
          <IconLabel icon="back">Volver al cobro</IconLabel>
        </button>
      </header>
      <div className="checkout-area-body checkout-capture-fields">
        <p className="checkout-inline-note">
          El turno no tiene tipo de cambio y no acepta dólares. Captúralo una sola vez: queda fijo
          hasta el cierre del turno.
        </p>
        {mutation.uncertain && (
          <div className="operation-recovery">
            <p>No se pudo confirmar el tipo de cambio. Usa «Reintentar sin duplicar».</p>
            <button
              type="button"
              className="primary-button"
              disabled={mutation.busy}
              onClick={() => void retry()}
            >
              <IconLabel icon="refresh">Reintentar sin duplicar</IconLabel>
            </button>
          </div>
        )}
        <AmountField
          label="Tipo de cambio del dólar"
          value={rate}
          onChange={setRate}
          active
          emphasis
          placeholder="0.00"
          hint="Pesos por dólar, por ejemplo 19.50."
        />
      </div>
      <footer className="checkout-area-footer checkout-capture-footer">
        <NumericKeypad
          value={rate}
          onChange={setRate}
          label="Teclado numérico del tipo de cambio"
          captureKeyboard={active}
          disabled={mutation.busy || mutation.uncertain}
        />
        <button
          type="button"
          className="primary-button checkout-register"
          disabled={wire === '' || mutation.busy || !!mutation.pending}
          onClick={() => void save()}
        >
          <IconLabel icon="check">
            {wire === '' ? 'Captura el tipo de cambio' : `Fijar el dólar a ${formatMoney(wire)}`}
          </IconLabel>
        </button>
      </footer>
      {confirmation && (
        <RateConfirmation
          message={confirmation}
          onConfirm={() => {
            setConfirmation('');
            void save(true);
          }}
          onCancel={() => setConfirmation('')}
        />
      )}
    </section>
  );
}
