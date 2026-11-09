import { useState } from 'react';
import { AmountField, NumericKeypad } from '../../../components/NumericKeypad';
import { ConfirmDialog } from '../../../components/ConfirmDialog';
import { APIError } from '../../../services/api';
import { toWireAmount } from '../../../services/amountInput';
import { formatMoney, permissionName } from '../../../services/payments';
import type { CurrentShift, Shift } from '../../../services/payments';
import { useRestaurantMutation } from '../../../hooks/useRestaurantMutation';

export const shiftPendingKey = 'payments:shift';

type OpeningTarget = 'opening' | 'rate';

// Go pide confirmar un tipo de cambio que difiere mucho del turno anterior (evita 1.95 por 19.50).
const confirmationCode = 'USD_RATE_CONFIRMATION_REQUIRED';

// Diálogo de confirmación de un tipo de cambio con diferencia grande; el mensaje lo redacta Go.
function RateConfirmation({
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

// Turno no utilizable: muestra el motivo de Go y, si falta el turno y el usuario puede abrirlo,
// ofrece la apertura en la misma pantalla. El fondo inicial se captura siempre de forma explícita
// (se permite 0); nunca se abre un turno automáticamente. Con el pago en dólares activo y el
// permiso correspondiente, también se captura el tipo de cambio fijo del turno.
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
  const [rate, setRate] = useState('');
  const [target, setTarget] = useState<OpeningTarget>('opening');
  const [confirmation, setConfirmation] = useState('');
  const mutation = useRestaurantMutation(() => {
    setOpening('');
    setRate('');
    setTarget('opening');
    onChanged();
    window.dispatchEvent(new CustomEvent('adel-server-change'));
  }, shiftPendingKey);
  const register = current.register;
  const openable = !!register && !current.shift && can('cash.shifts.open');
  // Sin la función activa, la apertura queda exactamente como en la etapa 6.
  const dollars = !!current.usd_cash;
  const canSetRate = dollars && can('payments.usd_rate.manage');
  const wire = toWireAmount(opening);
  const rateWire = canSetRate ? toWireAmount(rate) : '';

  function openLabel() {
    if (mutation.busy) {
      return 'Abriendo turno…';
    }
    if (wire === '') {
      return 'Captura el fondo inicial';
    }
    if (rateWire !== '') {
      return `Abrir turno con fondo de ${formatMoney(wire)} y dólar a ${formatMoney(rateWire)}`;
    }
    return `Abrir turno con fondo de ${formatMoney(wire)}`;
  }

  async function open(confirmed = false) {
    if (wire === '' || mutation.isPending()) {
      return;
    }
    const body: Record<string, unknown> = { opening_float: wire };
    if (rateWire !== '') {
      body.usd_rate = rateWire;
      if (confirmed) {
        body.confirm_rate_difference = true;
      }
    }
    try {
      await mutation.execute('payments_open_shift', { body });
    } catch (problem) {
      if (problem instanceof APIError && problem.code === confirmationCode) {
        setConfirmation(problem.message);
        return;
      }
      // Un turno abierto por otra terminal mientras se capturaba el fondo se refleja al consultar.
      if (problem instanceof APIError && problem.status === 409) {
        onChanged();
      }
    }
  }

  const keypadValue = target === 'rate' ? rate : opening;
  const setKeypadValue = target === 'rate' ? setRate : setOpening;

  return (
    <section className="checkout-shift" aria-label="Turno de caja">
      <h3>{register ? `Turno de ${register.name}` : 'Caja no asignada'}</h3>
      <p className="checkout-inline-note">{current.reason}</p>
      {mutation.error && mutation.errorCode !== confirmationCode && (
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
            active={target === 'opening'}
            onActivate={() => setTarget('opening')}
            emphasis
            placeholder="0.00"
            hint="Obligatorio. Captura 0 si la caja inicia sin efectivo."
          />
          {canSetRate && (
            <AmountField
              label="Tipo de cambio del dólar"
              value={rate}
              onChange={setRate}
              active={target === 'rate'}
              onActivate={() => setTarget('rate')}
              placeholder="0.00"
              hint="Pesos por dólar. Queda fijo durante el turno; sin él, el turno no acepta dólares."
            />
          )}
          {dollars && !canSetRate && (
            <p className="muted text-sm">
              El tipo de cambio lo captura alguien con el permiso «
              {permissionName('payments.usd_rate.manage')}»; mientras falte, el turno no acepta
              dólares.
            </p>
          )}
          <NumericKeypad
            value={keypadValue}
            onChange={setKeypadValue}
            label={
              target === 'rate'
                ? 'Teclado numérico del tipo de cambio'
                : 'Teclado numérico del fondo inicial'
            }
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
      {confirmation && (
        <RateConfirmation
          message={confirmation}
          onConfirm={() => {
            setConfirmation('');
            void open(true);
          }}
          onCancel={() => {
            setConfirmation('');
            setTarget('rate');
          }}
        />
      )}
    </section>
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
      if (problem instanceof APIError && problem.status === 409) {
        onChanged();
      }
    }
  }

  return (
    <section className="checkout-area checkout-capture" aria-label="Tipo de cambio del turno">
      <header className="checkout-area-header">
        <h3>Tipo de cambio de {shift.register_name}</h3>
        <button type="button" className="secondary-button" onClick={onCancel}>
          Volver al cobro
        </button>
      </header>
      <div className="checkout-area-body checkout-capture-fields">
        <p className="checkout-inline-note">
          El turno no tiene tipo de cambio y no acepta dólares. Captúralo una sola vez: queda fijo
          hasta el cierre del turno.
        </p>
        {mutation.error && mutation.errorCode !== confirmationCode && (
          <p className="checkout-inline-note" role="alert">
            {mutation.error}
          </p>
        )}
        {mutation.uncertain && (
          <div className="operation-recovery">
            <p>No se pudo confirmar el tipo de cambio. Reintenta con el mismo identificador.</p>
            <button
              type="button"
              className="primary-button"
              disabled={mutation.busy}
              onClick={() => void mutation.retry()}
            >
              Reintentar con el mismo identificador
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
          {wire === '' ? 'Captura el tipo de cambio' : `Fijar el dólar a ${formatMoney(wire)}`}
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
