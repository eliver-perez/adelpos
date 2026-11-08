import { useState } from 'react';
import type { ReactNode } from 'react';
import { Notice } from '../../../components/Forms';
import { useFinancialMutation } from '../../../hooks/useFinancialMutation';
import { useLiveResource } from '../../../hooks/useLiveResource';
import {
  authorizationCodes,
  isAuthorizationProblem,
  repeatAuthorizationMessage,
} from '../../../hooks/useSupervisorAuthorization';
import type { Operation } from '../../../services/api';
import { financialOutcome, formatMoney, permissionName } from '../../../services/payments';
import type {
  CorrectionResult,
  CurrentShift,
  Finance,
  PaymentMethod,
  PaymentRecord,
  PaymentResult,
  RefundResult,
} from '../../../services/payments';
import type { Bill } from '../../../services/restaurant';
import { CheckoutAccount } from './CheckoutAccount';
import type { LineSelection } from './CheckoutAccount';
import { CorrectionPanel } from './CorrectionPanel';
import { PaymentCapture, RecordedPayment, statusTitle } from './PaymentCapture';
import type { CheckoutStatus } from './PaymentCapture';
import { CheckoutMethods } from './PaymentMethods';
import { RefundPanel } from './RefundPanel';
import { ShiftOpening } from './ShiftOpening';
import { UncertainOperation } from './UncertainOperation';
import { newestFinance, selectableUnits } from './captureState';
import { useCollectFlow } from './useCollectFlow';
import { correctable, useCorrectionFlow } from './useCorrectionFlow';
import { refundableUnits, useRefundFlow } from './useRefundFlow';

export type CheckoutMode = 'collect' | 'refund';

// Errores que se muestran junto al formulario que los provocó, no en el aviso general.
const contextualCodes = [...authorizationCodes, 'SHIFT_CLOSED', 'PAYMENT_NOT_CORRECTABLE'];

// Pantalla de cobro integrada en la operación: cuenta y productos, métodos e historial, y
// captura con teclado fijo. Reúne cobros, devoluciones y correcciones de una cuenta; Go calcula
// toda cifra y la interfaz solo envía la captura. Todas sus escrituras comparten la clave de
// pendientes de la cuenta.
export function CheckoutPanel({
  bill,
  active,
  can,
  mode,
  onModeChange,
  onClose,
}: {
  bill: Bill;
  active: boolean;
  can: (permission: string) => boolean;
  mode: CheckoutMode;
  onModeChange: (mode: CheckoutMode) => void;
  onClose: () => void;
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
  const [notice, setNotice] = useState('');
  const [retryNotice, setRetryNotice] = useState('');
  const mutation = useFinancialMutation(accountID, handleResult);
  const finance = newestFinance(polled.data, recorded, accountID);
  const activeMethods = (methods.data ?? []).filter((method) => method.active);
  const shiftUsable = shift.data?.usable ?? false;
  const reloadFinance = polled.reload;
  const reloadShift = shift.reload;
  const locked = mutation.busy || !!mutation.pending;

  const correction = useCorrectionFlow({
    accountID,
    finance,
    methods: activeMethods,
    mutation,
    canCorrect: can('payments.corrections'),
    reloadFinance,
  });
  const correcting = !!correction.original;
  const collect = useCollectFlow({
    accountID,
    finance,
    methods: activeMethods,
    shiftUsable,
    permissions,
    mutation,
    enabled: mode === 'collect' && !correcting,
    reloadFinance,
    reloadShift,
  });
  const refund = useRefundFlow({
    accountID,
    finance,
    methods: activeMethods,
    mutation,
    enabled: mode === 'refund' && !correcting,
    reloadFinance,
    reloadShift,
  });

  // Toda escritura financiera devuelve el estado actualizado; cada flujo reinicia su captura.
  function handleResult(operation: Operation, result: unknown) {
    const outcome = financialOutcome(operation, result);
    if (outcome.finance) {
      setRecorded(outcome.finance);
    }
    setRetryNotice('');
    setNotice('');
    if (operation === 'payments_pay') {
      collect.onRecorded(result as PaymentResult);
    } else if (operation === 'payments_refund') {
      refund.onRecorded(result as RefundResult);
    } else if (operation === 'payments_correct') {
      correction.onRecorded(result as CorrectionResult);
      setNotice(outcome.message);
    } else {
      setNotice(outcome.message);
    }
  }

  // Un reintento se envía sin la contraseña del supervisor. Si la operación no llegó a
  // confirmarse, Go la rechaza y la autorización debe repetirse con una captura nueva.
  async function retryPending() {
    const pending = mutation.pending;
    const authorized = typeof pending?.body.authorization === 'object';
    const problem = await mutation.retry();
    if (!pending || !authorized || !isAuthorizationProblem(problem)) {
      return;
    }
    setRetryNotice(repeatAuthorizationMessage);
    if (pending.operation === 'payments_refund') {
      refund.auth.requireRepeat();
    } else if (pending.operation === 'payments_correct') {
      correction.auth.requireRepeat();
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

  const status = checkoutStatus(finance);

  function registeringLabel() {
    const operation = mutation.pending?.operation;
    if (operation === 'payments_refund') {
      return 'Registrando devolución…';
    }
    if (operation === 'payments_correct') {
      return 'Registrando corrección…';
    }
    if (operation === 'payments_pay') {
      return 'Registrando pago…';
    }
    return 'Registrando…';
  }

  function checkoutStatus(current: Finance): CheckoutStatus {
    if (mutation.uncertain) {
      return { state: 'uncertain', label: statusTitle('uncertain') };
    }
    if (mutation.busy) {
      return { state: 'registering', label: registeringLabel() };
    }
    if (collect.conflict && mode === 'collect') {
      return { state: 'conflict', label: statusTitle('conflict') };
    }
    if (mutation.error && !contextualCodes.includes(mutation.errorCode)) {
      return { state: 'error', label: statusTitle('error') };
    }
    if (correcting) {
      return { state: 'capturing', label: 'Capturando corrección' };
    }
    if (mode === 'refund') {
      if (refund.preview.validating) {
        return { state: 'validating', label: 'Validando…' };
      }
      if (refund.result) {
        return { state: 'recorded', label: 'Devolución registrada' };
      }
      return { state: 'capturing', label: 'Capturando devolución' };
    }
    if (current.settled) {
      return { state: 'settled', label: statusTitle('settled') };
    }
    if (!permissions.collect || current.state !== 'open') {
      return { state: 'readonly', label: statusTitle('readonly') };
    }
    if (collect.preview.validating) {
      return { state: 'validating', label: 'Validando…' };
    }
    if (collect.lastResult) {
      return { state: 'recorded', label: statusTitle('recorded') };
    }
    return { state: 'capturing', label: statusTitle('capturing') };
  }

  function mainBanner(current: Finance) {
    const pending = mutation.pending;
    if (pending && mutation.uncertain) {
      return (
        <UncertainOperation
          request={pending}
          methods={activeMethods}
          busy={mutation.busy}
          onRetry={() => void retryPending()}
          onConsult={() => void reloadFinance()}
        />
      );
    }
    if (collect.conflict && mode === 'collect') {
      return (
        <div className="checkout-conflict" role="alert">
          <strong>El saldo de la cuenta cambió en otra terminal.</strong>
          <p>
            Saldo real: {formatMoney(current.balance)}. Revisa la captura antes de registrar el
            pago.
          </p>
        </div>
      );
    }
    if (mutation.error && !contextualCodes.includes(mutation.errorCode)) {
      return (
        <p className="checkout-inline-note" role="alert">
          {mutation.error}
        </p>
      );
    }
    return null;
  }

  const banner: ReactNode = (
    <>
      {mainBanner(finance)}
      {retryNotice && (
        <p className="checkout-inline-note" role="alert">
          {retryNotice}
        </p>
      )}
      {notice && (
        <p className="checkout-result" role="status">
          {notice}
        </p>
      )}
      {shift.error && (
        <p className="checkout-inline-note" role="alert">
          {shift.error}
        </p>
      )}
    </>
  );

  const offerCorrection = can('payments.corrections') || permissions.collect;

  function paymentActions(payment: PaymentRecord) {
    const actions: ReactNode[] = [];
    const choosingPayment = mode === 'refund' && refund.capture.mode === 'payment' && !correcting;
    if (choosingPayment && !payment.corrected) {
      const chosen = refund.capture.paymentID === payment.id;
      actions.push(
        <button
          type="button"
          key="refund"
          className={`secondary-button ${chosen ? 'payment-history-chosen' : ''}`}
          aria-pressed={chosen}
          disabled={locked}
          onClick={() => refund.choosePayment(payment.id)}
        >
          {chosen ? 'Pago elegido para devolver' : 'Devolver de este pago'}
        </button>,
      );
    }
    if (offerCorrection && correctable(payment) && !correcting && !locked) {
      actions.push(
        <button
          type="button"
          key="correct"
          className="secondary-button"
          onClick={() => {
            setNotice('');
            correction.start(payment);
          }}
        >
          Corregir método
        </button>,
      );
    }
    return actions.length > 0 ? actions : null;
  }

  let selection: LineSelection | null = null;
  if (
    !correcting &&
    mode === 'collect' &&
    permissions.collect &&
    permissions.selection &&
    collect.collectable
  ) {
    selection = {
      verb: 'Cobrar',
      available: selectableUnits,
      selections: collect.capture.selections,
      onChange: collect.changeSelection,
      onClear: () => collect.updateCapture({ selections: {} }),
      locked,
      totalLabel: 'Total de la selección',
      total: collect.selectionTotal,
      validating: collect.preview.validating,
    };
  } else if (!correcting && mode === 'refund' && refund.capture.mode === 'items') {
    selection = {
      verb: 'Devolver',
      available: refundableUnits,
      selections: refund.capture.selections,
      onChange: refund.changeSelection,
      onClear: () => refund.update({ selections: {} }),
      locked,
      totalLabel: 'Devolución de la selección',
      total: refund.plan?.amount ?? null,
      validating: refund.preview.validating,
    };
  }

  function methodsColumn() {
    const common = {
      payments: finance?.payments ?? [],
      refunds: finance?.refunds ?? [],
      timezone: bill.timezone,
      error: methods.error,
      actions: paymentActions,
    };
    if (correcting) {
      return (
        <CheckoutMethods
          {...common}
          title="Método correcto"
          gridLabel="Métodos correctos disponibles"
          methods={correction.replacements}
          selectedID={correction.method?.id ?? ''}
          onSelect={correction.chooseMethod}
          disabled={locked}
          emptyMethods="No hay otro método activo compatible con este pago."
        />
      );
    }
    if (mode === 'refund') {
      return (
        <CheckoutMethods
          {...common}
          title="Método de devolución"
          gridLabel="Métodos de devolución disponibles"
          methods={refund.outlets}
          selectedID={refund.capture.methodID}
          onSelect={(method) => refund.update({ methodID: method.id })}
          disabled={locked}
          emptyMethods="Ningún método activo admite devoluciones."
        />
      );
    }
    return (
      <CheckoutMethods
        {...common}
        title="Método de pago"
        gridLabel="Métodos de pago disponibles"
        methods={activeMethods}
        selectedID={collect.capture.methodID}
        onSelect={collect.selectMethod}
        disabled={!permissions.collect || !collect.collectable || !shiftUsable || locked}
      />
    );
  }

  function shiftColumn(current: CurrentShift) {
    return (
      <section className="checkout-area checkout-capture" aria-label="Turno de caja">
        <div className="checkout-area-body">
          {banner}
          <ShiftOpening
            current={current}
            can={can}
            active={active}
            onChanged={() => void reloadShift()}
          />
        </div>
      </section>
    );
  }

  function operationColumn(current: Finance) {
    if (correcting) {
      return (
        <CorrectionPanel
          flow={correction}
          status={status}
          banner={banner}
          canCorrect={can('payments.corrections')}
          registering={mutation.busy}
          onRefundInstead={() => {
            correction.cancel();
            onModeChange('refund');
          }}
        />
      );
    }
    if (mode === 'refund') {
      // Devolver exige un turno utilizable en la caja de esta terminal.
      if (shift.data && !shiftUsable) {
        return shiftColumn(shift.data);
      }
      return (
        <RefundPanel
          flow={refund}
          finance={current}
          status={status}
          banner={banner}
          keyboardActive={active}
          canExecute={can('payments.refunds.execute')}
          registering={mutation.busy}
        />
      );
    }
    if (permissions.collect && collect.collectable && shift.data && !shiftUsable) {
      return shiftColumn(shift.data);
    }
    return (
      <CheckoutCaptureOrSummary
        finance={current}
        status={status}
        banner={banner}
        lastResult={collect.lastResult}
        collect={permissions.collect && collect.collectable}
      >
        <PaymentCapture
          finance={current}
          method={collect.method}
          capture={collect.effectiveCapture}
          onCapture={collect.updateCapture}
          onModeChange={collect.changeMode}
          plan={collect.plan}
          hint={collect.hint}
          canPartial={permissions.partial}
          canTip={permissions.tips}
          locked={locked}
          status={status}
          banner={banner}
          lastResult={collect.lastResult}
          canRegister={collect.canRegister}
          registering={mutation.busy}
          onRegister={() => void collect.register()}
          keyboardActive={active}
          quickAmounts={collect.quickAmounts}
        />
      </CheckoutCaptureOrSummary>
    );
  }

  return (
    <div className="checkout-workspace" aria-label={`Cobro de la cuenta #${finance.folio}`}>
      <CheckoutAccount finance={finance} bill={bill} selection={selection} onClose={onClose} />
      {methodsColumn()}
      {operationColumn(finance)}
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
    return 'La cuenta está liquidada y cerrada. Su historial de pagos permanece disponible; las devoluciones se registran en «Devoluciones».';
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
