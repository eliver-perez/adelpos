import { useState } from 'react';
import { useFinancialMutation } from '../../../hooks/useFinancialMutation';
import { useLiveResource } from '../../../hooks/useLiveResource';
import { APIError } from '../../../services/api';
import type { Finance } from '../../../services/payments';
import type { Account } from '../../../services/restaurant';
import { serverDateTime } from '../../../services/restaurant';
import { UncertainOperation } from '../checkout/UncertainOperation';

// Entrega de un pedido para llevar desde la cuenta o el tablero. Preparación, pago y entrega son
// independientes: un pedido liquidado se entrega directamente; con saldo, se abre el cobro con la
// entrega activada para registrar ambos en la misma operación.
export function TakeawayDelivery({
  account,
  timezone,
  can,
  disabled,
  onCollect,
}: {
  account: Account;
  timezone: string;
  can: (permission: string) => boolean;
  disabled: boolean;
  onCollect: () => void;
}) {
  const [confirming, setConfirming] = useState(false);
  const [notice, setNotice] = useState('');
  const delivered = account.delivery_state === 'delivered';
  // Datos de la entrega (hora, usuario y terminal) desde el estado financiero de Go.
  const finance = useLiveResource<Finance>('payments_account', {
    id: account.id,
    enabled: delivered && can('payments.read'),
    interval: 30000,
  });
  const mutation = useFinancialMutation(account.id, () => {
    setConfirming(false);
    setNotice('Entrega realizada.');
  });
  const locked = mutation.busy || !!mutation.pending;

  if (account.mode !== 'takeaway') {
    return null;
  }
  if (delivered) {
    const delivery = finance.data?.delivery;
    return (
      <p className="takeaway-delivered" role="status">
        Entregado
        {delivery &&
          ` · ${serverDateTime(delivery.delivered_at, timezone)} · ${delivery.actor_name} · ${delivery.device_name}`}
      </p>
    );
  }
  // Sin productos confirmados no hay nada que entregar (ni que cobrar).
  if (!can('takeaway.deliver') || account.confirmed_quantity === 0 || account.state !== 'open') {
    return null;
  }
  const settled = account.payment_state === 'paid';

  async function deliver() {
    if (mutation.isPending()) {
      return;
    }
    setNotice('');
    try {
      await mutation.execute('payments_deliver', { id: account.id, body: {} });
    } catch (problem) {
      if (!(problem instanceof APIError)) {
        return;
      }
      if (problem.code === 'BALANCE_PENDING') {
        // Otra terminal dejó saldo: la entrega se registra junto con el cobro.
        setConfirming(false);
        onCollect();
      } else if (problem.code === 'ALREADY_DELIVERED') {
        setConfirming(false);
        setNotice('El pedido ya estaba entregado.');
        window.dispatchEvent(new CustomEvent('adel-server-change'));
      }
    }
  }

  return (
    <div className="takeaway-delivery">
      {mutation.pending && mutation.uncertain && (
        <UncertainOperation
          request={mutation.pending}
          methods={[]}
          busy={mutation.busy}
          onRetry={() => void mutation.retry()}
          onConsult={() => window.dispatchEvent(new CustomEvent('adel-server-change'))}
        />
      )}
      {mutation.error &&
        !mutation.uncertain &&
        mutation.errorCode !== 'ALREADY_DELIVERED' &&
        mutation.errorCode !== 'BALANCE_PENDING' && (
          <p className="checkout-inline-note" role="alert">
            {mutation.error}
          </p>
        )}
      {notice && (
        <p className="checkout-result" role="status">
          {notice}
        </p>
      )}
      {!confirming && (
        <button
          type="button"
          className="secondary-button"
          disabled={disabled || locked}
          onClick={() => {
            if (settled) {
              setConfirming(true);
            } else {
              onCollect();
            }
          }}
        >
          Marcar como entregado
        </button>
      )}
      {!confirming && !settled && (
        <small className="muted">
          Tiene saldo pendiente: se abrirá el cobro con la entrega activada.
        </small>
      )}
      {confirming && (
        <div className="takeaway-delivery-confirm" role="group" aria-label="Confirmar entrega">
          <span>¿El cliente recibió el pedido #{account.folio}?</span>
          <div className="button-row">
            <button
              type="button"
              className="primary-button"
              disabled={locked}
              onClick={() => void deliver()}
            >
              {mutation.busy ? 'Registrando entrega…' : 'Confirmar entrega'}
            </button>
            <button
              type="button"
              className="secondary-button"
              disabled={mutation.busy}
              onClick={() => setConfirming(false)}
            >
              Cancelar
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
