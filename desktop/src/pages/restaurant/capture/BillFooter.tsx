import { IconLabel } from '../../../components/Icon';
import { shortcutProps } from '../../../commands/hints';
import { usePendingEntry } from '../../../hooks/pendingOperations';
import { formatMoney } from '../../../services/money';
import { paymentPendingKey } from '../../../services/payments';
import { isPendingAccount } from '../../../services/restaurant';
import type { Account, DraftItem, OrderQuote } from '../../../services/restaurant';
import type { AccountAction } from '../AccountActions';
import { adjustmentsAvailable } from '../adjustments/AccountAdjustments';
import { cancelBlockerHints } from '../CancelAccount';
import { draftUnits } from './captureRules';
import { MoreActions } from './MoreActions';
import type { MoreAction } from './MoreActions';

export type BillAction =
  AccountAction | 'metadata' | 'history' | 'kitchen' | 'adjustments' | 'precheck' | 'cancelAccount';

// Acciones menos frecuentes del menú «Más…» (antes «Acciones de cuenta…»), con su permiso.
const moreActions: (MoreAction<BillAction> & { permission: string })[] = [
  { id: 'metadata', label: 'Datos de cuenta', permission: 'accounts.manage', icon: 'edit' },
  {
    id: 'history',
    label: 'Historial de cambios',
    permission: 'orders.history.read',
    icon: 'clock',
  },
  {
    id: 'kitchen',
    label: 'Preparación y comandas',
    permission: 'kitchen.states.read',
    icon: 'send',
  },
  { id: 'move', label: 'Cambiar mesa', permission: 'accounts.move', icon: 'move' },
  {
    id: 'transfer',
    label: 'Transferir productos',
    permission: 'accounts.transfer_items',
    icon: 'transfer',
  },
  { id: 'merge', label: 'Combinar cuentas', permission: 'accounts.merge', icon: 'merge' },
  {
    id: 'assign',
    label: 'Cambiar responsable',
    permission: 'accounts.transfer',
    icon: 'edit',
  },
  // Etapa 9D (§5): «Cancelar cuenta» sustituye a «Anular cuenta vacía». La ruta pide abrir cuentas
  // (para llevar, administrarlas); Go decide si basta el permiso propio o hace falta un supervisor.
  {
    id: 'cancelAccount',
    label: 'Cancelar cuenta',
    permission: 'accounts.open',
    icon: 'ban',
    tone: 'danger',
  },
];

// Acciones del menú «Más…» que aplican a la cuenta con los permisos del usuario. Las comparten la
// cuenta de la captura y el panel lateral de la mesa. Desde la etapa 9D los productos nuevos aún
// sin guardar de esta terminal ya no condicionan ninguna opción (antes ocultaban la anulación de
// cuentas vacías; «Cancelar cuenta» los descarta): el tercer parámetro se conserva para los
// llamadores.
export function accountMoreActions(
  account: Account,
  can: (permission: string) => boolean,
  _localDraftItems: number,
): MoreAction<BillAction>[] {
  // Una cuenta pendiente (etapa 7.1) aún no existe en Go: no tiene acciones propias.
  if (isPendingAccount(account)) {
    return [];
  }
  const writable =
    account.state === 'open' && (account.mode !== 'takeaway' || can('takeaway.manage'));
  return moreActions
    .filter(
      (action) =>
        can(action.permission) &&
        (action.id === 'history' || action.id === 'kitchen' || writable) &&
        (action.id !== 'move' || account.mode === 'dine_in'),
    )
    .map((action) =>
      // Con pagos o ajustes vigentes «Cancelar cuenta» sigue a la vista y dice qué hacer primero;
      // su panel lo explica con el acceso a la devolución o a los descuentos y cargos.
      action.id === 'cancelAccount' && account.cancel_blocker
        ? { ...action, hint: cancelBlockerHints[account.cancel_blocker] }
        : action,
    );
}

// Botón de cobro: Cobrar exige registrar cobros y consultar la información financiera; muestra el
// saldo que calculó Go. Sin permiso de cobro basta la consulta.
//
// Etapa 9D (§3.3 y decisión 7): un solo botón principal por panel. Dentro de la captura «Cobrar»
// es principal junto a «Enviar a cocina»; fuera de ella (panel de la mesa y detalle de la cuenta en
// Cuentas y Para llevar) el principal es «Capturar…» y «Cobrar» conserva su texto e importe con el
// estilo de contorno (tone «secondary»).
export function CheckoutButton({
  account,
  can,
  disabled,
  onCheckout,
  tone = 'primary',
}: {
  account: Account;
  can: (permission: string) => boolean;
  disabled: boolean;
  onCheckout: () => void;
  tone?: 'primary' | 'secondary';
}) {
  const pending = usePendingEntry(paymentPendingKey(account.id));
  if (!can('payments.read')) {
    return null;
  }
  const collectClass =
    tone === 'primary'
      ? 'primary-button capture-checkout'
      : 'secondary-button capture-checkout capture-checkout-outline';
  const collectable =
    account.state === 'open' && account.confirmed_quantity > 0 && account.payment_state !== 'paid';
  if (collectable && can('payments.collect')) {
    return (
      <button type="button" className={collectClass} disabled={disabled} onClick={onCheckout}>
        <IconLabel icon="cash">
          Cobrar{account.finance ? ` · ${formatMoney(account.finance.balance)}` : ''}
        </IconLabel>
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
        <button type="button" className={collectClass} disabled>
          <IconLabel icon="cash">Cobrar</IconLabel>
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
      <IconLabel icon="cash">Consultar pagos</IconLabel>
    </button>
  );
}

// Pie de la cuenta: totales de Go, acciones secundarias como botones y las dos acciones
// principales («Enviar a cocina» y «Cobrar»). Fuera de la captura (emphasis «overview»: el detalle
// de la cuenta en Cuentas y Para llevar) el botón principal es «Capturar productos en esta cuenta»
// y estas dos conservan su lugar con el estilo de contorno (etapa 9D, decisión 7).
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
  emphasis = 'capture',
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
  emphasis?: 'capture' | 'overview';
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
    adjustments: !isPendingAccount(account) && adjustmentsAvailable(account, can),
    precheck: can('payments.read'),
    precheckReady: account.confirmed_quantity > 0,
  };
  const more = accountMoreActions(account, can, localDraft.length);
  return (
    <footer className="capture-bill-footer" aria-live="polite">
      <dl className="capture-totals">
        <div>
          <dt>Enviado a cocina</dt>
          <dd>{formatMoney(account.totals.total)}</dd>
        </div>
        {localDraft.length > 0 && (
          <div>
            <dt>Nuevo sin enviar</dt>
            <dd>{clean && quote?.quote ? formatMoney(quote.quote.total) : 'Calculando…'}</dd>
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
            <IconLabel icon="split">Dividir</IconLabel>
          </button>
        )}
        {secondary.adjustments && (
          <button
            type="button"
            className="secondary-button"
            disabled={blocked}
            onClick={() => onAction('adjustments')}
          >
            <IconLabel icon="discount">Descuento</IconLabel>
          </button>
        )}
        {secondary.precheck && (
          <button
            type="button"
            className="secondary-button"
            disabled={blocked || !secondary.precheckReady}
            {...shortcutProps('file.print_precheck', 'Precuenta')}
            onClick={() => onAction('precheck')}
          >
            <IconLabel icon="receipt">Precuenta</IconLabel>
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
            className={`${
              emphasis === 'capture' ? 'primary-button' : 'secondary-button'
            } capture-send`}
            disabled={newUnits === 0 || blocked || sending || conflict || linesLocked}
            onClick={onSend}
          >
            <IconLabel icon="send">
              {sending ? 'Enviando…' : `Enviar a cocina · ${newUnits}`}
            </IconLabel>
          </button>
        )}
        <CheckoutButton
          account={account}
          can={can}
          disabled={blocked || sending}
          onCheckout={onCheckout}
          tone={emphasis === 'capture' ? 'primary' : 'secondary'}
        />
      </div>
    </footer>
  );
}
