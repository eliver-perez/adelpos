import type { ReactNode } from 'react';
import { AmountField, NumericKeypad, QuickAmounts } from '../../../components/NumericKeypad';
import { SegmentedControl } from '../../../components/SegmentedControl';
import { Field } from '../../../components/Forms';
import {
  changeCurrencyLabels,
  formatDollars,
  formatMoney,
  isZeroMoney,
  permissionName,
  printAttentionStates,
  printingMessage,
} from '../../../services/payments';
import { DeliveryIcon, PrinterIcon, ToggleButton } from '../../../components/ToggleButton';
import { DrawerNotice } from '../../cash/CashNotices';
import type {
  ChangeCurrency,
  Finance,
  PaymentMethod,
  PaymentMode,
  PaymentPlan,
  PaymentRecord,
  PaymentResult,
} from '../../../services/payments';
import type { AmountTarget, CaptureState } from './captureState';
import { targetEditable, targetPatch, targetValue, tipAllowed } from './captureState';

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
  detail,
}: {
  label: string;
  value: string;
  emphasis?: boolean;
  // Segunda línea opcional, por ejemplo el cambio expresado en la otra moneda.
  detail?: string;
}) {
  return (
    <div className={`checkout-figure ${emphasis ? 'checkout-figure-emphasis' : ''}`}>
      <dt>{label}</dt>
      <dd>
        {value}
        {detail && <small className="checkout-figure-detail">{detail}</small>}
      </dd>
    </div>
  );
}

// Resultado del último pago: el cambio sigue visible después de registrar para entregarlo.
export function RecordedPayment({
  result,
  deliverRequested = false,
}: {
  result: PaymentResult;
  deliverRequested?: boolean;
}) {
  const payment = result.payment;
  const printing = printingMessage(result.printing);
  const printingAttention =
    !!result.printing?.status && printAttentionStates.includes(result.printing.status);
  return (
    <div className="checkout-result" role="status">
      <strong>
        Pago registrado
        {payment ? `: ${formatMoney(payment.amount)} con ${payment.method_name}` : ''}
      </strong>
      {payment && payment.is_cash && !isZeroMoney(payment.change) && (
        <p className="checkout-result-change">{changeMessage(payment)}</p>
      )}
      {payment?.received_usd && (
        <p>
          Recibido {formatMoney(payment.received_mxn)} y {formatDollars(payment.received_usd)} ·
          tipo de cambio {formatMoney(payment.usd_rate)} por dólar
        </p>
      )}
      {payment && !isZeroMoney(payment.tip) && <p>Propina {formatMoney(payment.tip)}</p>}
      <p>
        {result.finance.settled
          ? 'Cuenta liquidada.'
          : `Saldo pendiente ${formatMoney(result.finance.balance)}. Puedes registrar otro pago.`}
        {result.closed && ' La cuenta se cerró.'}
        {!result.closed &&
          !result.delivered &&
          result.finance.settled &&
          result.finance.mode === 'takeaway' &&
          ' El pedido queda pendiente de entrega.'}
      </p>
      {result.delivered && <p className="checkout-result-flag">Entrega realizada.</p>}
      {deliverRequested && !result.delivered && (
        <p className="checkout-result-warning">
          El pedido no se marcó entregado porque queda saldo pendiente.
        </p>
      )}
      {printing && (
        <p className={printingAttention ? 'checkout-result-warning' : 'checkout-result-flag'}>
          {printing}
        </p>
      )}
      <DrawerNotice outcome={result.drawer} />
    </div>
  );
}

// Cambio que el cajero entrega: en dólares si así se registró, con su equivalente en pesos.
function changeMessage(payment: PaymentRecord) {
  if (payment.change_currency === 'USD' && payment.change_usd) {
    return `Entrega cambio ${formatDollars(payment.change_usd)} (equivale a ${formatMoney(payment.change)})`;
  }
  return `Entrega cambio ${formatMoney(payment.change)}`;
}

// Tipo de cambio fijo del turno y equivalente en dólares del saldo y de este pago, tal como los
// calculó Go.
function DollarSummary({ rate, plan }: { rate: string; plan: PaymentPlan | null }) {
  const usd = plan?.usd ?? null;
  return (
    <div className="checkout-usd" aria-label="Pago en dólares">
      <div className="checkout-usd-rate">
        <span>Tipo de cambio del turno</span>
        <strong>{formatMoney(rate)} por dólar</strong>
      </div>
      {usd && plan && (
        <p className="checkout-usd-equivalent">
          Saldo {formatMoney(plan.balance_before)} · equivale a {formatDollars(usd.balance_usd)}
        </p>
      )}
      {usd && plan && !isZeroMoney(usd.received_usd) && (
        <p className="checkout-usd-equivalent">
          Recibido {formatMoney(usd.received_mxn)} y {formatDollars(usd.received_usd)} (
          {formatMoney(usd.received_usd_mxn)}) · total {formatMoney(usd.received_total)}
        </p>
      )}
      {usd && plan && usd.charged_usd !== usd.balance_usd && (
        <p className="checkout-usd-equivalent">
          Este pago {formatMoney(plan.amount)}
          {isZeroMoney(plan.tip) ? '' : ` más propina ${formatMoney(plan.tip)}`} · equivale a{' '}
          {formatDollars(usd.charged_usd)}
        </p>
      )}
    </div>
  );
}

// Interruptores del registro: imprimir ticket (con su permiso) y, en para llevar, entregar.
function RegisterOptions({
  showPrint,
  print,
  onPrint,
  printerConfigured,
  locked,
}: {
  showPrint: boolean;
  print: boolean;
  onPrint: (value: boolean) => void;
  printerConfigured: boolean;
  locked: boolean;
}) {
  if (!showPrint) {
    return null;
  }
  return (
    <>
      <ToggleButton
        label="Imprimir ticket"
        pressed={print}
        onChange={onPrint}
        disabled={locked}
        icon={<PrinterIcon />}
        describedBy={!printerConfigured ? 'checkout-printer-missing' : undefined}
      />
      {!printerConfigured && print && (
        <small id="checkout-printer-missing" className="checkout-printer-note">
          Sin impresora de tickets configurada: el pago se registrará sin imprimir.
        </small>
      )}
    </>
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
  dollars,
  dollarRate,
}: {
  capture: CaptureState;
  method: PaymentMethod | null;
  plan: PaymentPlan | null;
  finance: Finance;
  canTip: boolean;
  locked: boolean;
  onCapture: (patch: Partial<CaptureState>) => void;
  quickAmounts: { label: string; value: string }[];
  dollars: boolean;
  dollarRate: string | null;
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
  // Con dólares, un campo de pesos vacío significa cero pesos; sin ellos, importe exacto.
  const pesosPlaceholder = dollars && capture.receivedUSD !== '' ? '0.00' : 'Exacto';
  return (
    <div className="checkout-amounts">
      {dollarRate && <DollarSummary rate={dollarRate} plan={plan} />}
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
          label={dollars ? 'Pesos recibidos' : 'Efectivo recibido'}
          value={capture.received}
          onChange={(received) => onCapture({ received })}
          active={capture.target === 'received'}
          onActivate={activate('received')}
          placeholder={pesosPlaceholder}
          disabled={locked}
        />
      )}
      {method?.is_cash && quick('received')}
      {dollars && method?.is_cash && (
        <>
          <AmountField
            label="Dólares recibidos"
            currency="US$"
            value={capture.receivedUSD}
            onChange={(receivedUSD) => onCapture({ receivedUSD })}
            active={capture.target === 'received_usd'}
            onActivate={activate('received_usd')}
            placeholder="0.00"
            disabled={locked}
          />
          {quick('received_usd')}
          <div className="field">
            <span>Cambio entregado en</span>
            <SegmentedControl<ChangeCurrency>
              label="Cambio entregado en"
              value={capture.changeCurrency}
              onChange={(changeCurrency) => onCapture({ changeCurrency })}
              disabled={locked}
              options={[
                { value: 'MXN', label: changeCurrencyLabels.MXN },
                { value: 'USD', label: changeCurrencyLabels.USD },
              ]}
            />
          </div>
        </>
      )}
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
  const usd = method?.is_cash ? (plan.usd ?? null) : null;
  return (
    <dl className="checkout-figures" aria-live="polite">
      {method?.is_cash && !usd && (
        <Figure label="Cambio" value={formatMoney(plan.change)} emphasis />
      )}
      {/* El cambio se muestra en ambas monedas: destacado en la que se entrega y, debajo, su
          equivalente en la otra, ambos calculados por Go. */}
      {usd && usd.change_currency === 'USD' && (
        <Figure
          label="Cambio en dólares"
          value={formatDollars(usd.change_usd)}
          detail={`En pesos: ${formatMoney(usd.change_mxn)}`}
          emphasis
        />
      )}
      {usd && usd.change_currency !== 'USD' && (
        <Figure
          label="Cambio en pesos"
          value={formatMoney(usd.change_mxn)}
          detail={`En dólares: ${formatDollars(usd.change_usd)}`}
          emphasis
        />
      )}
      <Figure label="Aplicado" value={formatMoney(plan.amount)} />
      {!isZeroMoney(plan.tip) && <Figure label="Propina" value={formatMoney(plan.tip)} />}
      <Figure
        label={plan.settles ? 'Saldo restante (liquida)' : 'Saldo restante'}
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
  showPrint,
  printEnabled,
  printerConfigured,
  deliverAvailable,
  deliverEnabled,
  deliverRequested,
  dollars = false,
  dollarRate = null,
  dollarNotice = null,
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
  showPrint: boolean;
  printEnabled: boolean;
  printerConfigured: boolean;
  deliverAvailable: boolean;
  deliverEnabled: boolean;
  deliverRequested: boolean;
  // Pago en dólares: activo con la función, efectivo y turno con tipo de cambio.
  dollars?: boolean;
  dollarRate?: string | null;
  // Aviso del turno sin tipo de cambio (y su captura) cuando la función está activa.
  dollarNotice?: ReactNode;
}) {
  const editable = targetEditable(capture.target, capture, method, canTip, dollars);
  const keypadValue = targetValue(capture, capture.target);
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
        {lastResult && <RecordedPayment result={lastResult} deliverRequested={deliverRequested} />}
        {dollarNotice}
        {deliverAvailable && (
          <div className="checkout-deliver-option">
            <ToggleButton
              label="Marcar como entregado"
              pressed={deliverEnabled}
              onChange={(deliver) => onCapture({ deliver })}
              disabled={locked}
              icon={<DeliveryIcon />}
            />
            <small className="muted">Solo se registra si este pago liquida el pedido.</small>
          </div>
        )}
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
          dollars={dollars}
          dollarRate={dollarRate}
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
        <div className="checkout-register-row">
          <RegisterOptions
            showPrint={showPrint}
            print={printEnabled}
            onPrint={(print) => onCapture({ print })}
            printerConfigured={printerConfigured}
            locked={locked}
          />
          <button
            type="button"
            className="primary-button checkout-register"
            disabled={!canRegister}
            onClick={onRegister}
          >
            {registerLabel}
          </button>
        </div>
      </footer>
    </section>
  );
}
