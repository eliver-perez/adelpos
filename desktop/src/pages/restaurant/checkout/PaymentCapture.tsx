import type { ReactNode } from 'react';
import { AmountField, NumericKeypad, QuickAmounts } from '../../../components/NumericKeypad';
import { SegmentedControl } from '../../../components/SegmentedControl';
import { Field } from '../../../components/Forms';
import { formatMoney, isZeroMoney, permissionName } from '../../../services/payments';
import type {
  Finance,
  PaymentMethod,
  PaymentMode,
  PaymentPlan,
  PaymentResult,
} from '../../../services/payments';
import type { AmountTarget, CaptureState } from './captureState';
import { targetEditable, targetPatch, tipAllowed } from './captureState';

export type CheckoutState =
  | 'loading'
  | 'capturing'
  | 'validating'
  | 'registering'
  | 'recorded'
  | 'settled'
  | 'error'
  | 'conflict'
  | 'uncertain'
  | 'readonly';

export interface CheckoutStatus {
  state: CheckoutState;
  label: string;
}

const statusTitles: Record<CheckoutState, string> = {
  loading: 'Consultando cuenta',
  capturing: 'Capturando pago',
  validating: 'Validando',
  registering: 'Registrando pago',
  recorded: 'Pago registrado',
  settled: 'Cuenta liquidada',
  error: 'Error recuperable',
  conflict: 'Conflicto de cobro',
  uncertain: 'Operación incierta',
  readonly: 'Solo consulta',
};

export function statusTitle(state: CheckoutState) {
  return statusTitles[state];
}

function Figure({
  label,
  value,
  emphasis = false,
}: {
  label: string;
  value: string;
  emphasis?: boolean;
}) {
  return (
    <div className={`checkout-figure ${emphasis ? 'checkout-figure-emphasis' : ''}`}>
      <dt>{label}</dt>
      <dd>{value}</dd>
    </div>
  );
}

// Resultado del último pago: el cambio sigue visible después de registrar para entregarlo.
export function RecordedPayment({ result }: { result: PaymentResult }) {
  const payment = result.payment;
  return (
    <div className="checkout-result" role="status">
      <strong>
        Pago registrado
        {payment ? `: ${formatMoney(payment.amount)} con ${payment.method_name}` : ''}
      </strong>
      {payment && payment.is_cash && !isZeroMoney(payment.change) && (
        <p className="checkout-result-change">Entrega cambio {formatMoney(payment.change)}</p>
      )}
      {payment && !isZeroMoney(payment.tip) && <p>Propina {formatMoney(payment.tip)}</p>}
      <p>
        {result.finance.settled
          ? 'Cuenta liquidada.'
          : `Saldo pendiente ${formatMoney(result.finance.balance)}. Puedes registrar otro pago.`}
        {result.closed && ' La cuenta se cerró.'}
        {!result.closed &&
          result.finance.settled &&
          result.finance.mode === 'takeaway' &&
          ' El pedido queda pendiente de entrega.'}
      </p>
    </div>
  );
}

export function UncertainPayment({
  description,
  busy,
  onRetry,
  onConsult,
}: {
  description: string;
  busy: boolean;
  onRetry: () => void;
  onConsult: () => void;
}) {
  return (
    <div className="checkout-uncertain" role="alert">
      <strong>No se pudo confirmar el resultado del pago {description}.</strong>
      <p>
        No registres otro cobro en esta cuenta. Reintenta con el mismo identificador: si el pago ya
        llegó al servidor se recupera sin duplicarse. También puedes consultar el resultado.
      </p>
      <div className="button-row">
        <button type="button" className="primary-button" disabled={busy} onClick={onRetry}>
          {busy ? 'Reintentando…' : 'Reintentar con el mismo identificador'}
        </button>
        <button type="button" className="secondary-button" onClick={onConsult}>
          Consultar resultado
        </button>
      </div>
    </div>
  );
}

function AmountSection({
  capture,
  method,
  plan,
  finance,
  canTip,
  locked,
  onCapture,
  quickAmounts,
}: {
  capture: CaptureState;
  method: PaymentMethod | null;
  plan: PaymentPlan | null;
  finance: Finance;
  canTip: boolean;
  locked: boolean;
  onCapture: (patch: Partial<CaptureState>) => void;
  quickAmounts: { label: string; value: string }[];
}) {
  const activate = (target: AmountTarget) => () => onCapture({ target });
  // Los importes sugeridos acompañan al campo que reciben, justo debajo de él.
  const quick = (target: AmountTarget) => {
    if (capture.target !== target || locked) {
      return null;
    }
    return (
      <QuickAmounts
        amounts={quickAmounts}
        onSelect={(value) => onCapture(targetPatch(target, value))}
      />
    );
  };
  const selectionActive = Object.values(capture.selections).some((quantity) => quantity > 0);
  let proposed = 'Calculando…';
  if (plan) {
    proposed = formatMoney(plan.amount);
  } else if (!selectionActive) {
    // Sin selección, el pago total propone el saldo que entregó Go.
    proposed = formatMoney(finance.balance);
  }
  return (
    <div className="checkout-amounts">
      {capture.mode === 'total' ? (
        <div className="amount-field amount-field-readonly">
          <span className="amount-field-label">
            {selectionActive ? 'Importe de la selección' : 'Saldo a cobrar'}
          </span>
          <strong className="amount-field-value">{proposed}</strong>
        </div>
      ) : (
        <AmountField
          label={selectionActive ? 'Abono a la selección' : 'Importe del abono'}
          value={capture.amount}
          onChange={(amount) => onCapture({ amount })}
          active={capture.target === 'amount'}
          onActivate={activate('amount')}
          placeholder="0.00"
          disabled={locked}
          emphasis
        />
      )}
      {capture.mode === 'partial' && quick('amount')}
      {method?.is_cash && (
        <AmountField
          label="Efectivo recibido"
          value={capture.received}
          onChange={(received) => onCapture({ received })}
          active={capture.target === 'received'}
          onActivate={activate('received')}
          placeholder="Exacto"
          disabled={locked}
        />
      )}
      {method?.is_cash && quick('received')}
      {tipAllowed(method, canTip) && (
        <AmountField
          label="Propina (opcional)"
          value={capture.tip}
          onChange={(tip) => onCapture({ tip })}
          active={capture.target === 'tip'}
          onActivate={activate('tip')}
          placeholder="Sin propina"
          disabled={locked}
        />
      )}
    </div>
  );
}

function ReferenceInput({
  method,
  value,
  onChange,
  disabled,
}: {
  method: PaymentMethod;
  value: string;
  onChange: (value: string) => void;
  disabled: boolean;
}) {
  if (method.reference_mode === 'none') {
    return null;
  }
  const required = method.reference_mode === 'required';
  // La referencia sí admite el teclado flotante de la terminal cuando está habilitado.
  return (
    <Field label={required ? 'Referencia (obligatoria)' : 'Referencia (opcional)'}>
      <input
        type="text"
        maxLength={120}
        autoComplete="off"
        value={value}
        required={required}
        disabled={disabled}
        placeholder="Folio o autorización de la operación confirmada"
        onChange={(event) => onChange(event.target.value)}
      />
    </Field>
  );
}

function PlanFigures({ plan, method }: { plan: PaymentPlan; method: PaymentMethod | null }) {
  return (
    <dl className="checkout-figures" aria-live="polite">
      {method?.is_cash && <Figure label="Cambio" value={formatMoney(plan.change)} emphasis />}
      <Figure label="Aplicado a la cuenta" value={formatMoney(plan.amount)} />
      {!isZeroMoney(plan.tip) && <Figure label="Propina" value={formatMoney(plan.tip)} />}
      <Figure
        label={plan.settles ? 'Saldo restante · liquida la cuenta' : 'Saldo restante'}
        value={formatMoney(plan.balance_after)}
      />
    </dl>
  );
}

// Área de captura del pago. Todas las cifras mostradas provienen de la previsualización de Go.
export function PaymentCapture({
  finance,
  method,
  capture,
  onCapture,
  onModeChange,
  plan,
  hint,
  canPartial,
  canTip,
  locked,
  status,
  banner,
  lastResult,
  canRegister,
  registering,
  onRegister,
  keyboardActive,
  quickAmounts,
}: {
  finance: Finance;
  method: PaymentMethod | null;
  capture: CaptureState;
  onCapture: (patch: Partial<CaptureState>) => void;
  onModeChange: (mode: PaymentMode) => void;
  plan: PaymentPlan | null;
  hint: string;
  canPartial: boolean;
  canTip: boolean;
  locked: boolean;
  status: CheckoutStatus;
  banner: ReactNode;
  lastResult: PaymentResult | null;
  canRegister: boolean;
  registering: boolean;
  onRegister: () => void;
  keyboardActive: boolean;
  quickAmounts: { label: string; value: string }[];
}) {
  const editable = targetEditable(capture.target, capture, method, canTip);
  const keypadValue = capture[capture.target];
  let registerLabel = 'Selecciona un método de pago';
  if (registering) {
    registerLabel = 'Registrando pago…';
  } else if (method && plan) {
    registerLabel = `Registrar ${formatMoney(plan.amount)} · ${method.name}`;
  } else if (method) {
    registerLabel = 'Registrar pago';
  }

  return (
    <section className="checkout-area checkout-capture" aria-label="Captura del pago">
      <header className="checkout-area-header checkout-capture-header">
        <SegmentedControl
          label="Tipo de pago"
          value={capture.mode}
          onChange={onModeChange}
          disabled={locked}
          options={[
            { value: 'total', label: 'Pago total' },
            { value: 'partial', label: 'Pago parcial', disabled: !canPartial },
          ]}
        />
        <span className={`checkout-status checkout-status-${status.state}`} role="status">
          {status.label}
        </span>
      </header>
      <div className="checkout-area-body checkout-capture-fields">
        {banner}
        {lastResult && <RecordedPayment result={lastResult} />}
        {!canPartial && (
          <p className="muted text-xs">
            Pago parcial requiere el permiso «{permissionName('payments.collect_partial')}».
          </p>
        )}
        <AmountSection
          capture={capture}
          method={method}
          plan={plan}
          finance={finance}
          canTip={canTip}
          locked={locked}
          onCapture={onCapture}
          quickAmounts={quickAmounts}
        />
        {method && (
          <ReferenceInput
            method={method}
            value={capture.reference}
            onChange={(reference) => onCapture({ reference })}
            disabled={locked}
          />
        )}
        {hint && (
          <p className="checkout-inline-note" role="alert">
            {hint}
          </p>
        )}
      </div>
      <footer className="checkout-area-footer checkout-capture-footer">
        {/* Cambio, importe aplicado y saldo restante permanecen visibles junto al botón. */}
        {method && plan && <PlanFigures plan={plan} method={method} />}
        <NumericKeypad
          value={keypadValue}
          onChange={(value) => onCapture(targetPatch(capture.target, value))}
          label="Teclado numérico de cobro"
          captureKeyboard={keyboardActive && editable && !locked}
          disabled={!editable || locked}
        />
        <button
          type="button"
          className="primary-button checkout-register"
          disabled={!canRegister}
          onClick={onRegister}
        >
          {registerLabel}
        </button>
      </footer>
    </section>
  );
}
