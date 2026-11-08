import { useEffect, useState } from 'react';
import type { ReactNode } from 'react';
import { Notice } from '../../../components/Forms';
import { useLiveResource } from '../../../hooks/useLiveResource';
import { usePaymentPreview } from '../../../hooks/usePaymentPreview';
import { useRestaurantMutation } from '../../../hooks/useRestaurantMutation';
import { APIError } from '../../../services/api';
import { formatMoney, paymentPendingKey, permissionName } from '../../../services/payments';
import type {
  CurrentShift,
  Finance,
  PaymentInput,
  PaymentMethod,
  PaymentMode,
  PaymentResult,
} from '../../../services/payments';
import type { Bill } from '../../../services/restaurant';
import { CheckoutAccount } from './CheckoutAccount';
import type { CheckoutAccountAction } from './CheckoutAccount';
import { PaymentCapture, RecordedPayment, UncertainPayment, statusTitle } from './PaymentCapture';
import type { CheckoutStatus } from './PaymentCapture';
import { CheckoutMethods } from './PaymentMethods';
import { ShiftOpening } from './ShiftOpening';
import {
  buildPaymentInput,
  calculationInput,
  calculationMethod,
  defaultTarget,
  financeKey,
  initialCapture,
  newestFinance,
  registerInput,
  selectionList,
} from './captureState';
import type { CaptureState } from './captureState';

// Códigos de Go que indican que la cuenta cambió y debe consultarse de nuevo.
const refreshCodes = ['BALANCE_CHANGED', 'NOTHING_TO_COLLECT', 'ACCOUNT_NOT_OPEN'];
const shiftCodes = ['SHIFT_REQUIRED', 'REGISTER_REQUIRED', 'SHIFT_NOT_SHARED'];

const cashQuickAmounts = [
  { label: 'Exacto', value: '' },
  { label: '$100', value: '100' },
  { label: '$200', value: '200' },
  { label: '$500', value: '500' },
  { label: '$1000', value: '1000' },
];

// Pantalla de cobro integrada en la operación: cuenta y productos, métodos e historial, y
// captura con teclado fijo. Go calcula toda cifra; la interfaz solo envía la captura.
export function CheckoutPanel({
  bill,
  active,
  can,
  onClose,
  onAccountAction,
}: {
  bill: Bill;
  active: boolean;
  can: (permission: string) => boolean;
  onClose: () => void;
  onAccountAction: (action: CheckoutAccountAction) => void;
}) {
  const accountID = bill.account.id;
  const permissions = {
    collect: can('payments.collect'),
    partial: can('payments.collect_partial'),
    selection: can('payments.collect_selection'),
    tips: can('payments.tips.record'),
  };
  const polled = useLiveResource<Finance>('payments_account', { id: accountID, enabled: active });
  const methods = useLiveResource<PaymentMethod[]>('payments_methods', {
    enabled: active,
    interval: 30000,
  });
  const shift = useLiveResource<CurrentShift>('payments_current_shift', {
    enabled: active,
    interval: 15000,
  });
  const [recorded, setRecorded] = useState<Finance | null>(null);
  const [capture, setCapture] = useState<CaptureState>(initialCapture);
  const [lastResult, setLastResult] = useState<PaymentResult | null>(null);
  const [conflict, setConflict] = useState(false);
  const mutation = useRestaurantMutation((operation, result) => {
    if (operation !== 'payments_pay') {
      return;
    }
    const value = result as PaymentResult;
    // Pagos mixtos = pagos sucesivos: tras cada registro la captura se reinicia por completo.
    setLastResult(value);
    setRecorded(value.finance);
    setCapture(initialCapture());
    setConflict(false);
    window.dispatchEvent(new CustomEvent('adel-server-change'));
  }, paymentPendingKey(accountID));

  const finance = newestFinance(polled.data, recorded, accountID);
  const activeMethods = (methods.data ?? []).filter((method) => method.active);
  const method = activeMethods.find((entry) => entry.id === capture.methodID) ?? null;
  const collectable = !!finance && finance.state === 'open' && !finance.settled;
  const shiftUsable = shift.data?.usable ?? false;
  const pendingPayment = mutation.pending;
  const locked = mutation.busy || !!pendingPayment;
  const mode: PaymentMode = capture.mode === 'partial' && permissions.partial ? 'partial' : 'total';
  const effectiveCapture = { ...capture, mode };

  let previewInput: PaymentInput | null = null;
  if (finance && permissions.collect && collectable) {
    if (method) {
      previewInput = buildPaymentInput(effectiveCapture, method, finance, permissions);
    } else if (permissions.selection) {
      // Antes de elegir método, Go calcula igualmente el total de la selección.
      const selections = selectionList(capture.selections, finance);
      const calculator = calculationMethod(activeMethods);
      if (selections.length > 0 && calculator) {
        previewInput = calculationInput(calculator, selections);
      }
    }
  }
  const preview = usePaymentPreview(accountID, previewInput, finance ? financeKey(finance) : '');
  const plan = preview.plan;
  const previewCode = preview.error?.code ?? '';
  const reloadFinance = polled.reload;
  const reloadShift = shift.reload;

  // Un cálculo hecho sobre otro saldo o un rechazo por cambio de la cuenta obliga a consultarla.
  useEffect(() => {
    if (plan && finance && plan.balance_before !== finance.balance) {
      void reloadFinance();
    }
  }, [plan, finance, reloadFinance]);

  useEffect(() => {
    if (refreshCodes.includes(previewCode)) {
      void reloadFinance();
    }
  }, [previewCode, reloadFinance]);

  function updateCapture(patch: Partial<CaptureState>) {
    if (locked) {
      return;
    }
    setCapture((current) => ({ ...current, ...patch }));
    setLastResult(null);
    setConflict(false);
    mutation.clearError();
  }

  function selectMethod(selected: PaymentMethod) {
    updateCapture({
      methodID: selected.id,
      target: defaultTarget(mode, selected, permissions.tips),
    });
  }

  function changeMode(next: PaymentMode) {
    updateCapture({
      mode: next,
      amount: '',
      target: defaultTarget(next, method, permissions.tips),
    });
  }

  function changeSelection(versionID: string, quantity: number) {
    updateCapture({ selections: { ...capture.selections, [versionID]: quantity } });
  }

  const canRegister =
    permissions.collect &&
    collectable &&
    shiftUsable &&
    !!method &&
    !!plan &&
    !preview.validating &&
    !locked;

  async function register() {
    if (!finance || !method || !plan || !previewInput || !canRegister || mutation.isPending()) {
      return;
    }
    setConflict(false);
    setLastResult(null);
    try {
      await mutation.execute<PaymentResult>('payments_pay', {
        id: accountID,
        body: { ...registerInput(previewInput, plan, finance) },
      });
    } catch (problem) {
      if (!(problem instanceof APIError)) {
        return;
      }
      if (problem.code === 'BALANCE_CHANGED') {
        // Conflicto de cobro: se consulta el saldo real y se conserva la captura para revisarla.
        setConflict(true);
      }
      if (refreshCodes.includes(problem.code)) {
        void reloadFinance();
      }
      if (shiftCodes.includes(problem.code)) {
        void reloadShift();
      }
    }
  }

  if (!finance) {
    return (
      <div className="checkout-workspace checkout-loading">
        <section className="checkout-area">
          <header className="checkout-area-header">
            <h2>Cobro · Cuenta #{bill.account.folio}</h2>
            <button type="button" className="secondary-button" onClick={onClose}>
              Volver a la cuenta
            </button>
          </header>
          <div className="checkout-area-body">
            <Notice>{polled.error}</Notice>
            {!polled.error && <p className="muted">Consultando el estado financiero…</p>}
          </div>
        </section>
      </div>
    );
  }

  const status = checkoutStatus();
  const pendingMethod = activeMethods.find(
    (entry) => entry.id === (pendingPayment?.body.method_id as string | undefined),
  );
  const pendingAmount = pendingPayment?.body.amount as string | undefined;
  const pendingDescription = [
    pendingAmount ? formatMoney(pendingAmount) : '',
    pendingMethod ? `con ${pendingMethod.name}` : '',
  ]
    .filter(Boolean)
    .join(' ');

  function checkoutStatus(): CheckoutStatus {
    if (!finance) {
      return { state: 'loading', label: statusTitle('loading') };
    }
    if (mutation.uncertain) {
      return { state: 'uncertain', label: statusTitle('uncertain') };
    }
    if (mutation.busy) {
      return { state: 'registering', label: 'Registrando pago…' };
    }
    if (conflict) {
      return { state: 'conflict', label: statusTitle('conflict') };
    }
    if (mutation.error) {
      return { state: 'error', label: statusTitle('error') };
    }
    if (finance.settled) {
      return { state: 'settled', label: statusTitle('settled') };
    }
    if (!permissions.collect || finance.state !== 'open') {
      return { state: 'readonly', label: statusTitle('readonly') };
    }
    if (preview.validating) {
      return { state: 'validating', label: 'Validando…' };
    }
    if (lastResult) {
      return { state: 'recorded', label: statusTitle('recorded') };
    }
    return { state: 'capturing', label: statusTitle('capturing') };
  }

  function banner() {
    return (
      <>
        {mainBanner()}
        {shift.error && (
          <p className="checkout-inline-note" role="alert">
            {shift.error}
          </p>
        )}
      </>
    );
  }

  function mainBanner() {
    if (pendingPayment && mutation.uncertain) {
      return (
        <UncertainPayment
          description={pendingDescription}
          busy={mutation.busy}
          onRetry={() => void mutation.retry()}
          onConsult={() => void reloadFinance()}
        />
      );
    }
    if (conflict && finance) {
      return (
        <div className="checkout-conflict" role="alert">
          <strong>El saldo de la cuenta cambió en otra terminal.</strong>
          <p>
            Saldo real: {formatMoney(finance.balance)}. Revisa la captura antes de registrar el
            pago.
          </p>
        </div>
      );
    }
    if (mutation.error) {
      return (
        <p className="checkout-inline-note" role="alert">
          {mutation.error}
        </p>
      );
    }
    return null;
  }

  // Mensaje de validación de Go sobre la captura vigente (efectivo insuficiente, referencia…).
  let hint = '';
  if (preview.error && !refreshCodes.includes(previewCode)) {
    hint = preview.error.message;
  } else if (method && mode === 'partial' && !previewInput) {
    hint = 'Captura el importe del abono.';
  }

  const accountActions: { id: CheckoutAccountAction; label: string }[] = [];
  const writable =
    bill.account.state === 'open' && (bill.account.mode !== 'takeaway' || can('takeaway.manage'));
  if (writable && can('accounts.split')) {
    accountActions.push({ id: 'split', label: 'Dividir cuenta' });
  }
  if (writable && can('accounts.transfer_items')) {
    accountActions.push({ id: 'transfer', label: 'Transferir productos' });
  }

  const selectionTotal = plan?.selection_total ?? null;
  let quickAmounts: { label: string; value: string }[] = [];
  if (capture.target === 'received') {
    quickAmounts = cashQuickAmounts;
  } else if (capture.target === 'amount' && mode === 'partial') {
    const target = selectionTotal ?? (plan ? plan.target : finance.balance);
    quickAmounts = [{ label: `Saldo ${formatMoney(target)}`, value: target }];
  }

  return (
    <div className="checkout-workspace" aria-label={`Cobro de la cuenta #${finance.folio}`}>
      <CheckoutAccount
        finance={finance}
        bill={bill}
        selections={capture.selections}
        onSelectionChange={changeSelection}
        onClearSelection={() => updateCapture({ selections: {} })}
        selectionEnabled={permissions.collect && permissions.selection && collectable}
        selectionLocked={locked}
        selectionTotal={selectionTotal}
        selectionValidating={preview.validating}
        accountActions={locked ? [] : accountActions}
        onAccountAction={onAccountAction}
        onClose={onClose}
      />
      <CheckoutMethods
        methods={activeMethods}
        selectedID={capture.methodID}
        onSelect={selectMethod}
        disabled={!permissions.collect || !collectable || !shiftUsable || locked}
        payments={finance.payments}
        timezone={bill.timezone}
        error={methods.error}
      />
      {permissions.collect && collectable && shift.data && !shiftUsable ? (
        <section className="checkout-area checkout-capture" aria-label="Turno de caja">
          <div className="checkout-area-body">
            <ShiftOpening
              current={shift.data}
              can={can}
              active={active}
              onChanged={() => void reloadShift()}
            />
          </div>
        </section>
      ) : (
        <CheckoutCaptureOrSummary
          finance={finance}
          status={status}
          banner={banner()}
          lastResult={lastResult}
          collect={permissions.collect && collectable}
        >
          <PaymentCapture
            finance={finance}
            method={method}
            capture={effectiveCapture}
            onCapture={updateCapture}
            onModeChange={changeMode}
            plan={plan}
            hint={hint}
            canPartial={permissions.partial}
            canTip={permissions.tips}
            locked={locked}
            status={status}
            banner={banner()}
            lastResult={lastResult}
            canRegister={canRegister}
            registering={mutation.busy}
            onRegister={() => void register()}
            keyboardActive={active}
            quickAmounts={quickAmounts}
          />
        </CheckoutCaptureOrSummary>
      )}
    </div>
  );
}

// Sin permiso de cobro o con la cuenta liquidada, la tercera área resume el estado sin teclado.
function CheckoutCaptureOrSummary({
  finance,
  status,
  banner,
  lastResult,
  collect,
  children,
}: {
  finance: Finance;
  status: CheckoutStatus;
  banner: ReactNode;
  lastResult: PaymentResult | null;
  collect: boolean;
  children: ReactNode;
}) {
  if (collect) {
    return children;
  }
  return (
    <section className="checkout-area checkout-capture" aria-label="Estado del cobro">
      <header className="checkout-area-header">
        <h3>Estado del cobro</h3>
        <span className={`checkout-status checkout-status-${status.state}`} role="status">
          {status.label}
        </span>
      </header>
      <div className="checkout-area-body checkout-capture-fields">
        {banner}
        {lastResult && <RecordedPayment result={lastResult} />}
        <dl className="checkout-figures">
          <div className="checkout-figure checkout-figure-emphasis">
            <dt>Saldo pendiente</dt>
            <dd>{formatMoney(finance.balance)}</dd>
          </div>
          <div className="checkout-figure">
            <dt>Total</dt>
            <dd>{formatMoney(finance.total)}</dd>
          </div>
          <div className="checkout-figure">
            <dt>Pagado</dt>
            <dd>{formatMoney(finance.paid)}</dd>
          </div>
        </dl>
        <p className="muted text-sm">{summaryMessage(finance)}</p>
      </div>
    </section>
  );
}

function summaryMessage(finance: Finance) {
  if (finance.settled && finance.state === 'closed') {
    return 'La cuenta está liquidada y cerrada. Su historial de pagos permanece disponible.';
  }
  if (finance.settled && finance.mode === 'takeaway') {
    return 'El pedido está liquidado; la entrega se registra por separado.';
  }
  if (finance.settled) {
    return 'La cuenta está liquidada.';
  }
  if (finance.state !== 'open') {
    return 'La cuenta no está abierta; no admite cobros.';
  }
  return `Consulta disponible. Registrar cobros requiere el permiso «${permissionName('payments.collect')}».`;
}
