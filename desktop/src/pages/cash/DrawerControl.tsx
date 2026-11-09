import { useId, useState } from 'react';
import { IconLabel } from '../../components/Icon';
import type { DrawerOutcome } from '../../services/cash';
import { validReason } from '../../services/payments';
import { notifyDrawerOutcome } from './CashNotices';
import { PendingCashOperation } from './PendingCashOperation';
import { useCashMutation } from './useCashMutation';

// Clave del almacén de pendientes del cajón de una caja: un comando incierto se reintenta con el
// mismo identificador (Go devuelve el mismo comando, sin otro pulso) y no bloquea la caja.
export function drawerPendingKey(registerID: string) {
  return `cash-drawer:${registerID}`;
}

// Apertura manual del cajón de la caja de esta terminal (cash.drawer.open). Exige un motivo, es
// idempotente y Go la audita. El aviso solo informa que el comando se envió: nunca afirma que el
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
  // El resultado se avisa al confirmarse, también cuando llega por el reintento de un incierto.
  const mutation = useCashMutation(
    registerID,
    (_, result) => notifyDrawerOutcome(result as DrawerOutcome),
    drawerPendingKey(registerID),
  );
  const ready = validReason(reason) && !mutation.locked;

  async function send() {
    if (!ready || mutation.isPending()) {
      return;
    }
    try {
      await mutation.execute<DrawerOutcome>('payments_open_drawer', {
        body: { reason: reason.trim() },
      });
      setReason('');
      setCapturing(false);
    } catch (problem) {
      mutation.reportFailure(problem, 'No se pudo enviar el comando de apertura del cajón.');
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
            mutation.clearError();
          }}
        >
          <IconLabel icon={capturing ? 'close' : 'drawer'}>
            {capturing ? 'Cancelar apertura del cajón' : 'Abrir cajón'}
          </IconLabel>
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
            <IconLabel icon="drawer">
              {mutation.busy ? 'Enviando comando…' : 'Enviar comando de apertura'}
            </IconLabel>
          </button>
        </div>
      )}
      <PendingCashOperation mutation={mutation} />
    </section>
  );
}
