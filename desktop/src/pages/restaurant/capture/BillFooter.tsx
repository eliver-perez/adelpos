import { usePendingEntry } from '../../../hooks/pendingOperations';
import { formatMoney, paymentPendingKey } from '../../../services/payments';
import type { Account, DraftItem, OrderQuote } from '../../../services/restaurant';
import type { AccountAction } from '../AccountActions';
import { adjustmentsAvailable } from '../adjustments/AccountAdjustments';
import { draftUnits } from './captureRules';
import { MoreActions } from './MoreActions';
import type { MoreAction } from './MoreActions';

export type BillAction =
  AccountAction | 'metadata' | 'history' | 'kitchen' | 'adjustments' | 'precheck';

// Acciones menos frecuentes del menú «Más…» (antes «Acciones de cuenta…»), con su permiso.
const moreActions: (MoreAction<BillAction> & { permission: string })[] = [
  { id: 'metadata', label: 'Datos de cuenta', permission: 'accounts.manage' },
  { id: 'history', label: 'Historial de cambios', permission: 'orders.history.read' },
  { id: 'kitchen', label: 'Preparación y comandas', permission: 'kitchen.states.read' },
  { id: 'move', label: 'Cambiar mesa', permission: 'accounts.move' },
  { id: 'transfer', label: 'Transferir productos', permission: 'accounts.transfer_items' },
  { id: 'merge', label: 'Combinar cuentas', permission: 'accounts.merge' },
  { id: 'assign', label: 'Cambiar responsable', permission: 'accounts.transfer' },
  { id: 'void', label: 'Anular cuenta vacía', permission: 'accounts.manage' },
];

// Botón de cobro: Cobrar exige registrar cobros y consultar la información financiera; muestra el
// saldo que calculó Go. Sin permiso de cobro basta la consulta.
function CheckoutButton({
  account,
  can,
  disabled,
  onCheckout,
}: {
  account: Account;
  can: (permission: string) => boolean;
  disabled: boolean;
  onCheckout: () => void;
}) {
  const pending = usePendingEntry(paymentPendingKey(account.id));
  if (!can('payments.read')) {
    return null;
  }
  const collectable =
    account.state === 'open' && account.confirmed_quantity > 0 && account.payment_state !== 'paid';
  if (collectable && can('payments.collect')) {
    return (
      <button
        type="button"
        className="primary-button capture-checkout"
        disabled={disabled}
        onClick={onCheckout}
      >
        Cobrar{account.finance ? ` · ${formatMoney(account.finance.balance)}` : ''}
      </button>
    );
  }
  const consultable =
    account.confirmed_quantity > 0 || account.payment_state !== 'not_recorded' || !!pending;
  if (!consultable) {
    // Cuenta abierta sin productos enviados: «Cobrar» conserva su lugar, desactivado, hasta que
    // haya algo confirmado que cobrar.
    if (account.state === 'open' && can('payments.collect')) {
      return (
        <button type="button" className="primary-button capture-checkout" disabled>
          Cobrar
        </button>
      );
    }
    return null;
  }
  return (
    <button
      type="button"
      className="secondary-button capture-checkout"
      disabled={disabled}
      onClick={onCheckout}
    >
      Consultar pagos
    </button>
  );
}

// Pie de la cuenta: totales de Go, acciones secundarias como botones y las dos acciones
// principales («Enviar a cocina» y «Cobrar»).
export function BillFooter({
  account,
  localDraft,
  activeLines,
  quote,
  clean,
  can,
  blocked,
  linesLocked,
  sending,
  conflict,
  paymentPending,
  onAction,
  onSend,
  onCheckout,
}: {
  account: Account;
  localDraft: DraftItem[];
  activeLines: number;
  quote: OrderQuote | null;
  // La cotización corresponde exactamente al borrador visible.
  clean: boolean;
  can: (permission: string) => boolean;
  blocked: boolean;
  linesLocked: boolean;
  sending: boolean;
  conflict: boolean;
  paymentPending: boolean;
  onAction: (action: BillAction) => void;
  onSend: () => void;
  onCheckout: () => void;
}) {
  const writable =
    account.state === 'open' && (account.mode !== 'takeaway' || can('takeaway.manage'));
  const newUnits = draftUnits(localDraft);
  const finance = account.finance;
  const confirmedTotal = finance?.total ?? account.totals.total;
  const adjusted = !!finance && finance.total !== account.totals.total;
  const grandTotal =
    localDraft.length > 0 ? (clean ? (quote?.combined.total ?? null) : null) : confirmedTotal;
  // Las acciones secundarias permanecen en su lugar mientras el usuario tenga el permiso; se
  // desactivan cuando todavía no aplican (sin productos enviados), para que la distribución no
  // cambie al enviar a cocina.
  const secondary = {
    split: writable && can('accounts.split'),
    splitReady: activeLines > 0,
    adjustments: adjustmentsAvailable(account, can),
    precheck: can('payments.read'),
    precheckReady: account.confirmed_quantity > 0,
  };
  const more = moreActions.filter(
    (action) =>
      can(action.permission) &&
      (action.id === 'history' || action.id === 'kitchen' || writable) &&
      (action.id !== 'move' || account.mode === 'dine_in') &&
      (action.id !== 'void' ||
        (account.confirmed_quantity === 0 &&
          account.draft_quantity === 0 &&
          localDraft.length === 0)),
  );
  return (
    <footer className="capture-bill-footer" aria-live="polite">
      <dl className="capture-totals">
        <div>
          <dt>Enviado a cocina</dt>
          <dd>${account.totals.total}</dd>
        </div>
        {localDraft.length > 0 && (
          <div>
            <dt>Nuevo sin enviar</dt>
            <dd>{clean && quote?.quote ? `$${quote.quote.total}` : 'Calculando…'}</dd>
          </div>
        )}
        {adjusted && localDraft.length > 0 && (
          <div>
            <dt>Enviado con descuentos y cargos</dt>
            <dd>{formatMoney(finance.total)}</dd>
          </div>
        )}
        {finance && account.payment_state !== 'not_recorded' && (
          <div>
            <dt>Pagado</dt>
            <dd>{formatMoney(finance.paid)}</dd>
          </div>
        )}
        <div className="capture-grand-total">
          <dt>Total</dt>
          <dd>{grandTotal ? formatMoney(grandTotal) : 'Calculando…'}</dd>
        </div>
      </dl>
      <p className="muted text-xs capture-tax-note">Precios con IVA incluido</p>
      <div className="capture-secondary-actions" role="group" aria-label="Acciones de la cuenta">
        {secondary.split && (
          <button
            type="button"
            className="secondary-button"
            disabled={blocked || !secondary.splitReady}
            onClick={() => onAction('split')}
          >
            Dividir
          </button>
        )}
        {secondary.adjustments && (
          <button
            type="button"
            className="secondary-button"
            disabled={blocked}
            onClick={() => onAction('adjustments')}
          >
            Descuento
          </button>
        )}
        {secondary.precheck && (
          <button
            type="button"
            className="secondary-button"
            disabled={blocked || !secondary.precheckReady}
            onClick={() => onAction('precheck')}
          >
            Precuenta
          </button>
        )}
        <MoreActions actions={more} disabled={blocked} onAction={onAction} />
      </div>
      {paymentPending && can('payments.read') && (
        <p className="checkout-pending-hint" role="alert">
          Hay una operación financiera sin resultado confirmado. Abre el cobro para recuperarla
          antes de registrar otra.
        </p>
      )}
      <div className="capture-primary-actions">
        {writable && can('orders.confirm') && (
          <button
            type="button"
            className="primary-button capture-send"
            disabled={newUnits === 0 || blocked || sending || conflict || linesLocked}
            onClick={onSend}
          >
            {sending ? 'Enviando…' : `Enviar a cocina · ${newUnits}`}
          </button>
        )}
        <CheckoutButton
          account={account}
          can={can}
          disabled={blocked || sending}
          onCheckout={onCheckout}
        />
      </div>
    </footer>
  );
}
