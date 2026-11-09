import { useConfirmDialog } from '../../../components/ConfirmDialog';
import { IconLabel } from '../../../components/Icon';
import { useFinancialMutation } from '../../../hooks/useFinancialMutation';
import { useLiveResource } from '../../../hooks/useLiveResource';
import { APIError } from '../../../services/api';
import { notifyInfo, notifySuccess } from '../../../services/notify';
import type { Finance } from '../../../services/payments';
import type { Account } from '../../../services/restaurant';
import { serverDateTime } from '../../../services/restaurant';
import { UncertainOperation } from '../checkout/UncertainOperation';
import { notifyActionError } from '../checkout/actionErrors';

// Entrega de un pedido para llevar desde la cuenta o el tablero. Preparación, pago y entrega son
// independientes: un pedido liquidado se entrega directamente, tras confirmarlo en el diálogo
// estándar; con saldo, se abre el cobro con la entrega activada para registrar ambos en la misma
// operación.
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
  const { confirm, dialog } = useConfirmDialog();
  const delivered = account.delivery_state === 'delivered';
  // Datos de la entrega (hora, usuario y terminal) desde el estado financiero de Go.
  const finance = useLiveResource<Finance>('payments_account', {
    id: account.id,
    enabled: delivered && can('payments.read'),
    interval: 30000,
  });
  const mutation = useFinancialMutation(account.id, () => {
    notifySuccess(`Pedido #${account.folio} entregado.`);
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

  // Rechazos de la entrega: con saldo pendiente se abre el cobro; si ya estaba entregado se
  // informa y se actualiza la vista; los demás se avisan como error.
  function handleFailure(problem: unknown) {
    if (problem instanceof APIError && problem.code === 'BALANCE_PENDING') {
      // Otra terminal dejó saldo: la entrega se registra junto con el cobro.
      onCollect();
      return;
    }
    if (problem instanceof APIError && problem.code === 'ALREADY_DELIVERED') {
      notifyInfo('El pedido ya estaba entregado.');
      window.dispatchEvent(new CustomEvent('adel-server-change'));
      return;
    }
    notifyActionError(problem, 'No se pudo registrar la entrega.');
  }

  async function deliver() {
    if (mutation.isPending()) {
      return;
    }
    const confirmed = await confirm({
      title: 'Confirmar entrega',
      message: `¿El cliente recibió el pedido #${account.folio}?`,
      confirmLabel: 'Confirmar entrega',
      cancelLabel: 'Cancelar',
    });
    if (!confirmed || mutation.isPending()) {
      return;
    }
    try {
      await mutation.execute('payments_deliver', { id: account.id, body: {} });
    } catch (problem) {
      handleFailure(problem);
    }
  }

  async function retry() {
    const problem = await mutation.retry();
    if (problem) {
      handleFailure(problem);
    }
  }

  return (
    <div className="takeaway-delivery">
      {mutation.pending && mutation.uncertain && (
        <UncertainOperation
          request={mutation.pending}
          methods={[]}
          busy={mutation.busy}
          onRetry={() => void retry()}
          onConsult={() => window.dispatchEvent(new CustomEvent('adel-server-change'))}
        />
      )}
      <button
        type="button"
        className="secondary-button"
        disabled={disabled || locked}
        onClick={() => {
          if (settled) {
            void deliver();
          } else {
            onCollect();
          }
        }}
      >
        <IconLabel icon="delivery">
          {mutation.busy ? 'Registrando entrega…' : 'Marcar como entregado'}
        </IconLabel>
      </button>
      {!settled && (
        <small className="muted">
          Tiene saldo pendiente: se abrirá el cobro con la entrega activada.
        </small>
      )}
      {dialog}
    </div>
  );
}
