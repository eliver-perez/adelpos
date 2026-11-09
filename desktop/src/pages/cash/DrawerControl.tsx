import { useId, useState } from 'react';
import type { DrawerOutcome } from '../../services/cash';
import { validReason } from '../../services/payments';
import { DrawerNotice } from './CashNotices';
import { PendingCashOperation } from './PendingCashOperation';
import { useCashMutation } from './useCashMutation';

// Clave del almacén de pendientes del cajón de una caja: un comando incierto se reintenta con el
// mismo identificador (Go devuelve el mismo comando, sin otro pulso) y no bloquea la caja.
export function drawerPendingKey(registerID: string) {
  return `cash-drawer:${registerID}`;
}

// Apertura manual del cajón de la caja de esta terminal (cash.drawer.open). Exige un motivo, es
// idempotente y Go la audita. La pantalla solo informa que el comando se envió: nunca afirma que el
// cajón se abrió físicamente.
export function DrawerControl({
  registerID,
  registerName,
}: {
  registerID: string;
  registerName: string;
}) {
  const reasonID = useId();
  const [capturing, setCapturing] = useState(false);
  const [reason, setReason] = useState('');
  const [outcome, setOutcome] = useState<DrawerOutcome | null>(null);
  const mutation = useCashMutation(registerID, undefined, drawerPendingKey(registerID));
  const ready = validReason(reason) && !mutation.locked;

  async function send() {
    if (!ready || mutation.isPending()) {
      return;
    }
    try {
      const result = await mutation.execute<DrawerOutcome>('payments_open_drawer', {
        body: { reason: reason.trim() },
      });
      setOutcome(result);
      setReason('');
      setCapturing(false);
    } catch {
      // El mensaje de Go queda visible junto al formulario.
    }
  }

  return (
    <section className="cash-drawer" aria-label="Cajón de dinero">
      <div className="button-row">
        <button
          type="button"
          className="secondary-button"
          aria-expanded={capturing}
          onClick={() => {
            setCapturing(!capturing);
            setOutcome(null);
            mutation.clearError();
          }}
        >
          {capturing ? 'Cancelar apertura del cajón' : 'Abrir cajón'}
        </button>
      </div>
      {capturing && (
        <div className="form-stack">
          <div className="field">
            <label htmlFor={reasonID}>Motivo de la apertura del cajón de {registerName}</label>
            <textarea
              id={reasonID}
              rows={2}
              maxLength={500}
              aria-describedby={`${reasonID}-hint`}
              value={reason}
              onChange={(event) => setReason(event.target.value)}
            />
            <small id={`${reasonID}-hint`} className="muted">
              Entre 5 y 500 caracteres. La apertura manual queda registrada y auditada.
            </small>
          </div>
          <button
            type="button"
            className="primary-button"
            disabled={!ready}
            onClick={() => void send()}
          >
            {mutation.busy ? 'Enviando comando…' : 'Enviar comando de apertura'}
          </button>
        </div>
      )}
      <PendingCashOperation mutation={mutation} />
      {mutation.error && !mutation.uncertain && (
        <p className="checkout-inline-note" role="alert">
          {mutation.error}
        </p>
      )}
      <DrawerNotice outcome={outcome} />
    </section>
  );
}
