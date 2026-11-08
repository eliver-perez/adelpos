import { preparationLabels } from '../../services/kitchen';
import { Accordion } from '../../components/Accordion';
import { Notice } from '../../components/Forms';
import { ProductDetails } from '../../components/catalog/QuoteBreakdown';
import type {
  Account,
  Bill,
  ConfirmedLine,
  DraftItem,
  OrderQuote,
} from '../../services/restaurant';
import type { Lookups } from '../../services/catalog';
import type { AccountAction } from './AccountActions';
import { adjustmentsAvailable } from './adjustments/AccountAdjustments';
import { usePendingEntry } from '../../hooks/pendingOperations';
import {
  deliveryStateDescriptions,
  formatMoney,
  paymentPendingKey,
  paymentStateDescriptions,
} from '../../services/payments';

type BillAction = AccountAction | 'metadata' | 'history' | 'kitchen' | 'adjustments';

const actions: {
  id: BillAction;
  label: string;
  permission: string;
}[] = [
  { id: 'metadata', label: 'Datos de cuenta', permission: 'accounts.manage' },
  { id: 'history', label: 'Historial de cambios', permission: 'orders.history.read' },
  { id: 'kitchen', label: 'Preparación y comandas', permission: 'kitchen.states.read' },
  { id: 'move', label: 'Cambiar mesa', permission: 'accounts.move' },
  { id: 'split', label: 'Dividir cuenta', permission: 'accounts.split' },
  { id: 'transfer', label: 'Transferir productos', permission: 'accounts.transfer_items' },
  { id: 'merge', label: 'Combinar cuentas', permission: 'accounts.merge' },
  { id: 'adjustments', label: 'Descuentos y cargos', permission: 'payments.read' },
  { id: 'assign', label: 'Cambiar responsable', permission: 'accounts.transfer' },
  { id: 'void', label: 'Anular cuenta vacía', permission: 'accounts.manage' },
];

// Descripción del estado de pago con el saldo que calculó Go; no se suman importes en la interfaz.
function paymentDescription(account: Account) {
  const label = paymentStateDescriptions[account.payment_state] ?? account.payment_state;
  if (account.payment_state === 'partial' && account.finance) {
    return `${label} · saldo ${formatMoney(account.finance.balance)}`;
  }
  return label;
}

function closedDescription(account: Account) {
  if (account.mode === 'takeaway') {
    return 'Pedido liquidado y entregado; la cuenta está cerrada.';
  }
  return 'Cuenta liquidada y cerrada; la mesa quedó libre.';
}

// Botones de cobro: Cobrar exige registrar cobros y consultar la información financiera; la
// consulta de pagos basta con el permiso de consulta.
function CheckoutButtons({
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
      <>
        {pending && (
          <p className="checkout-pending-hint" role="alert">
            Hay una operación financiera sin resultado confirmado. Abre el cobro para recuperarla
            antes de registrar otra.
          </p>
        )}
        <button className="primary-button" disabled={disabled} onClick={onCheckout}>
          Cobrar {account.finance ? formatMoney(account.finance.balance) : ''}
        </button>
      </>
    );
  }
  const consultable =
    account.confirmed_quantity > 0 || account.payment_state !== 'not_recorded' || !!pending;
  if (!consultable) {
    return null;
  }
  return (
    <button className="secondary-button" disabled={disabled} onClick={onCheckout}>
      Consultar pagos y saldo
    </button>
  );
}

export function BillPanel({
  bill,
  error,
  loading,
  lookups,
  quote,
  editing,
  busy,
  can,
  onAction,
  onCheckout,
  onDraftEdit,
  onDraftRemove,
  onLineEdit,
  onLineCancel,
  onConfirm,
  onRefresh,
  onMergedTarget,
}: {
  bill: Bill | null;
  error: string;
  loading: boolean;
  lookups: Lookups | null;
  quote: OrderQuote | null;
  editing: boolean;
  busy: boolean;
  can: (permission: string) => boolean;
  onAction: (action: BillAction) => void;
  onCheckout: () => void;
  onDraftEdit: (item: DraftItem) => void;
  onDraftRemove: (item: DraftItem) => void;
  onLineEdit: (item: ConfirmedLine) => void;
  onLineCancel: (item: ConfirmedLine) => void;
  onConfirm: () => void;
  onRefresh: () => void;
  onMergedTarget: (id: string) => void;
}) {
  if (!bill) {
    return (
      <aside className="operation-bill">
        <h2>Cuenta actual</h2>
        <Notice>{error}</Notice>
        <p className="muted">
          {loading
            ? 'Consultando cuenta…'
            : 'Selecciona una cuenta o abre una nueva para comenzar.'}
        </p>
      </aside>
    );
  }
  const account = bill.account;
  const writable =
    account.state === 'open' && (account.mode !== 'takeaway' || can('takeaway.manage'));
  const validQuote =
    quote?.revision === account.revision && quote.account_id === account.id ? quote : null;
  const availableActions = actions.filter(
    (action) =>
      can(action.permission) &&
      (action.id === 'history' ||
        action.id === 'kitchen' ||
        action.id === 'adjustments' ||
        writable) &&
      (action.id !== 'adjustments' || adjustmentsAvailable(account, can)) &&
      (action.id !== 'move' || account.mode === 'dine_in') &&
      (action.id !== 'void' || (account.confirmed_quantity === 0 && account.draft_quantity === 0)),
  );

  return (
    <aside className="operation-bill" aria-label="Cuenta actual">
      <div className="operation-bill-header">
        <div>
          <p className="eyebrow">
            {account.mode === 'takeaway' ? 'Para llevar · ' : ''}Cuenta #{account.folio}
          </p>
          <h2>{account.name}</h2>
          <p className="muted text-xs">
            {account.table_name ?? 'Sin mesa'} · {account.assigned_name} · r{account.revision}
          </p>
        </div>
        <button className="secondary-button" onClick={onRefresh} aria-label="Actualizar cuenta">
          ↻
        </button>
      </div>
      <p className="account-preparation-status">
        Preparación: {preparationLabels[account.preparation_state] ?? account.preparation_state}
      </p>
      {account.mode !== 'takeaway' && (
        <p className="account-payment-status">Pago: {paymentDescription(account)}</p>
      )}
      <select
        className="operation-account-actions"
        aria-label="Acciones de cuenta"
        value=""
        onChange={(event) => {
          if (event.target.value) {
            onAction(event.target.value as BillAction);
          }
        }}
        disabled={busy || editing}
      >
        <option value="">Acciones de cuenta…</option>
        {availableActions.map((action) => (
          <option value={action.id} key={action.id}>
            {action.label}
          </option>
        ))}
      </select>
      <div className="operation-bill-lines">
        <Notice>{error}</Notice>
        {account.state === 'merged' && (
          <Notice>
            Esta cuenta se combinó y conserva su historial.{' '}
            {account.merged_into_id && (
              <button
                className="secondary-button"
                onClick={() => onMergedTarget(account.merged_into_id!)}
              >
                Ver cuenta resultante
              </button>
            )}
          </Notice>
        )}
        {account.state === 'closed' && <Notice>{closedDescription(account)}</Notice>}
        {account.state === 'voided' && (
          <Notice>Cuenta anulada sin productos activos. Su historial permanece disponible.</Notice>
        )}
        {account.mode === 'takeaway' && (
          <div className="takeaway-dimensions">
            <span>
              Preparación:{' '}
              {preparationLabels[account.preparation_state] ?? account.preparation_state}
            </span>
            <span>Pago: {paymentDescription(account)}</span>
            <span>
              Entrega: {deliveryStateDescriptions[account.delivery_state] ?? account.delivery_state}
            </span>
            {account.customer_name && (
              <strong>
                {account.customer_name} · {account.phone}
              </strong>
            )}
            {account.scheduled_local && (
              <span>
                Programado: {account.scheduled_local.replace('T', ' ')} · {bill.timezone}
              </span>
            )}
          </div>
        )}
        {bill.draft.length > 0 && <h3 className="bill-section-title">Borrador · Sin confirmar</h3>}
        {bill.draft.map((item) => (
          <article className="bill-line bill-line-draft" key={item.id}>
            <div className="bill-line-title">
              <strong>
                {item.quantity} ×{' '}
                {lookups?.products.find((product) => product.id === item.product_id)?.name ??
                  'Producto del catálogo'}
              </strong>
              <span className="status-pill">Borrador</span>
            </div>
            {item.customization?.notes && (
              <p className="muted text-xs">{item.customization.notes}</p>
            )}
            <div className="button-row">
              {writable && can('orders.capture') && (
                <button
                  className="secondary-button"
                  disabled={busy || editing}
                  onClick={() => onDraftEdit(item)}
                >
                  Personalizar
                </button>
              )}
              {writable && can('orders.capture') && (
                <button
                  className="secondary-button"
                  disabled={busy || editing}
                  onClick={() => onDraftRemove(item)}
                >
                  Quitar del borrador
                </button>
              )}
            </div>
          </article>
        ))}
        {bill.lines.length > 0 && <h3 className="bill-section-title">Productos confirmados</h3>}
        {bill.lines.map((line) => (
          <article
            className={`bill-line ${line.state === 'cancelled' ? 'bill-line-cancelled' : ''}`}
            key={`${line.version_id}-${line.state}`}
          >
            <div className="bill-line-title">
              <strong>
                {line.quantity} × {line.quote_line.name}
              </strong>
              <strong>${line.totals.total}</strong>
            </div>
            <p className="muted text-xs">
              {line.state === 'cancelled'
                ? 'Cancelado · Historial conservado'
                : (preparationLabels[line.preparation_state ?? ''] ?? 'Confirmado')}
            </p>
            {line.request.customization?.notes && (
              <p className="text-xs">{line.request.customization.notes}</p>
            )}
            <Accordion
              title="Preparación y desglose"
              summary={`Versión ${line.version} · Descuento histórico $${line.totals.discount}`}
            >
              <ProductDetails value={line.quote_line} />
            </Accordion>
            {line.state === 'active' && writable && (
              <div className="button-row">
                {can('orders.edit') && (
                  <button
                    className="secondary-button"
                    disabled={busy || editing}
                    onClick={() => onLineEdit(line)}
                  >
                    Modificar
                  </button>
                )}
                {can('orders.cancel') && (
                  <button
                    className="secondary-button"
                    disabled={busy || editing}
                    onClick={() => onLineCancel(line)}
                  >
                    Cancelar unidades
                  </button>
                )}
              </div>
            )}
          </article>
        ))}
        {bill.lines.length === 0 && bill.draft.length === 0 && (
          <p className="muted bill-empty">La cuenta aún no tiene productos.</p>
        )}
        {account.notes && <p className="bill-account-notes">Observaciones: {account.notes}</p>}
      </div>
      <footer className="operation-bill-footer" aria-live="polite">
        <div className="bill-total-row">
          <span>Confirmado</span>
          <strong>${account.totals.total}</strong>
        </div>
        {account.finance && account.payment_state !== 'not_recorded' && (
          <>
            <div className="bill-total-row">
              <span>Pagado</span>
              <strong>{formatMoney(account.finance.paid)}</strong>
            </div>
            <div className="bill-total-row">
              <span>Saldo pendiente</span>
              <strong>{formatMoney(account.finance.balance)}</strong>
            </div>
          </>
        )}
        {(bill.draft.length > 0 || editing) && (
          <>
            <div className="bill-total-row">
              <span>
                {editing ? 'Resultado de la selección en edición' : 'Cuenta con borrador'}
              </span>
              <strong>{validQuote ? `$${validQuote.combined.total}` : 'Calculando…'}</strong>
            </div>
            {validQuote?.quote && (
              <p className="muted text-xs">
                Descuento automático del lote: ${validQuote.quote.discount}
              </p>
            )}
          </>
        )}
        {editing && (
          <p className="muted text-xs">
            Guarda la selección o confirma su modificación antes de confirmar otro pedido.
          </p>
        )}
        {writable && can('orders.confirm') && (
          <button
            className="primary-button"
            disabled={
              busy || !!error || editing || bill.draft.length === 0 || !validQuote?.fingerprint
            }
            onClick={onConfirm}
          >
            Confirmar pedido
          </button>
        )}
        <CheckoutButtons
          account={account}
          can={can}
          disabled={busy || editing}
          onCheckout={onCheckout}
        />
        <p className="muted text-xs">
          Confirmar registra el pedido y resuelve su liberación. Consulta Preparación para ver
          destinos y recepción. Cobrar abre la pantalla de cobro de esta cuenta.
        </p>
      </footer>
    </aside>
  );
}
