import type { ReactNode } from 'react';
import { Field } from '../../../components/Forms';
import { AmountField, NumericKeypad } from '../../../components/NumericKeypad';
import { SegmentedControl } from '../../../components/SegmentedControl';
import { SupervisorAuthorization } from '../../../components/SupervisorAuthorization';
import { formatMoney, isZeroMoney, permissionName } from '../../../services/payments';
import type { Finance, RefundMode, RefundPlan, RefundResult } from '../../../services/payments';
import { DrawerNotice } from '../../cash/CashNotices';
import type { CheckoutStatus } from './PaymentCapture';
import type { RefundFlow } from './useRefundFlow';

const modeDescriptions: Record<RefundMode, string> = {
  total: 'Devuelve todo lo neto de todos los pagos de la cuenta, incluidas sus propinas.',
  payment:
    'Devuelve parte de un pago: solo su parte general (no asignada a productos) y su propina. Lo asignado a productos se devuelve por productos.',
  items: 'Devuelve las unidades pagadas que selecciones; se pueden devolver cantidades parciales.',
};

function RefundSummary({ plan, finance }: { plan: RefundPlan; finance: Finance }) {
  return (
    <div className="refund-summary" aria-live="polite">
      <dl className="checkout-figures">
        <div className="checkout-figure checkout-figure-emphasis">
          <dt>Venta devuelta</dt>
          <dd>{formatMoney(plan.amount)}</dd>
        </div>
        {!isZeroMoney(plan.tip) && (
          <div className="checkout-figure">
            <dt>Propina devuelta (aparte)</dt>
            <dd>{formatMoney(plan.tip)}</dd>
          </div>
        )}
        <div className="checkout-figure">
          <dt>Saldo de la cuenta después</dt>
          <dd>{formatMoney(plan.balance_after)}</dd>
        </div>
      </dl>
      <ul className="refund-parts" aria-label="Devolución por pago original">
        {plan.parts.map((part) => {
          const payment = finance.payments.find((entry) => entry.id === part.payment_id);
          return (
            <li key={part.payment_id}>
              <span>De {payment ? payment.method_name : 'pago original'}</span>
              <strong>
                {formatMoney(part.amount)}
                {!isZeroMoney(part.tip) && ` + propina ${formatMoney(part.tip)}`}
              </strong>
            </li>
          );
        })}
      </ul>
    </div>
  );
}

function RefundRecorded({ result }: { result: RefundResult }) {
  return (
    <div className="checkout-result" role="status">
      <strong>Devolución registrada: {formatMoney(result.plan.amount)}</strong>
      {!isZeroMoney(result.plan.tip) && <p>Propina devuelta {formatMoney(result.plan.tip)}</p>}
      <p>
        Saldo de la cuenta {formatMoney(result.finance.balance)}.
        {result.reopened && ' La cuenta se reabrió porque quedó saldo pendiente.'}
      </p>
      <DrawerNotice outcome={result.drawer} />
    </div>
  );
}

// Captura de una devolución dentro de la pantalla de cobro. El método de salida se elige en el
// área de métodos y el pago de origen en el historial; Go calcula importes y reparto por pago.
export function RefundPanel({
  flow,
  finance,
  status,
  banner,
  keyboardActive,
  canExecute,
  registering,
}: {
  flow: RefundFlow;
  finance: Finance;
  status: CheckoutStatus;
  banner: ReactNode;
  keyboardActive: boolean;
  canExecute: boolean;
  registering: boolean;
}) {
  const { capture, method, plan } = flow;
  const payment = finance.payments.find((entry) => entry.id === capture.paymentID) ?? null;
  const amountsEditable = capture.mode === 'payment' && !!payment && !flow.locked;
  const keypadValue = capture.target === 'tip' ? capture.tip : capture.amount;
  let registerLabel = 'Elige el método de devolución';
  if (registering) {
    registerLabel = 'Registrando devolución…';
  } else if (method && plan) {
    registerLabel = `Registrar devolución ${formatMoney(plan.amount)} · ${method.name}`;
  } else if (method) {
    registerLabel = 'Registrar devolución';
  }

  return (
    <section className="checkout-area checkout-capture" aria-label="Devolución">
      <header className="checkout-area-header checkout-capture-header">
        <SegmentedControl<RefundMode>
          label="Tipo de devolución"
          value={capture.mode}
          onChange={flow.changeMode}
          disabled={flow.locked}
          options={[
            { value: 'total', label: 'Total' },
            { value: 'payment', label: 'De un pago' },
            { value: 'items', label: 'Por productos' },
          ]}
        />
        <span className={`checkout-status checkout-status-${status.state}`} role="status">
          {status.label}
        </span>
      </header>
      <div className="checkout-area-body checkout-capture-fields">
        {banner}
        {flow.result && <RefundRecorded result={flow.result} />}
        <p className="muted text-xs">{modeDescriptions[capture.mode]}</p>
        {!canExecute && (
          <p className="muted text-xs">
            Sin el permiso «{permissionName('payments.refunds.execute')}», la devolución requiere la
            autorización de un supervisor.
          </p>
        )}
        {capture.mode === 'payment' && payment && (
          <div className="refund-origin">
            <span>
              Pago de {payment.method_name} por {formatMoney(payment.amount)}
              {!isZeroMoney(payment.tip) && ` · propina ${formatMoney(payment.tip)}`}
            </span>
            <AmountField
              label="Venta a devolver"
              value={capture.amount}
              onChange={(amount) => flow.update({ amount })}
              active={capture.target === 'amount'}
              onActivate={() => flow.update({ target: 'amount' })}
              placeholder="0.00"
              disabled={flow.locked}
            />
            {!isZeroMoney(payment.tip) && (
              <AmountField
                label="Propina a devolver"
                value={capture.tip}
                onChange={(tip) => flow.update({ tip })}
                active={capture.target === 'tip'}
                onActivate={() => flow.update({ target: 'tip' })}
                placeholder="0.00"
                disabled={flow.locked}
              />
            )}
          </div>
        )}
        <p className="checkout-inline-note">
          {method
            ? `Sale por ${method.name}.`
            : 'Elige en «Método de devolución» por dónde sale el dinero; puede ser distinto del pago original.'}
          {method &&
            !method.is_cash &&
            ' Registra solo una devolución ya confirmada fuera del sistema; AD-EL POS no procesa reembolsos bancarios.'}
          {method &&
            method.is_cash &&
            finance.payments.some((payment) => !!payment.received_usd) &&
            ' Las devoluciones se entregan en pesos, aunque el pago se haya recibido en dólares.'}
        </p>
        {method && method.reference_mode !== 'none' && (
          <Field
            label={
              method.reference_mode === 'required'
                ? 'Referencia de la devolución (obligatoria)'
                : 'Referencia de la devolución (opcional)'
            }
          >
            <input
              type="text"
              maxLength={120}
              autoComplete="off"
              value={capture.reference}
              disabled={flow.locked}
              onChange={(event) => flow.update({ reference: event.target.value })}
            />
          </Field>
        )}
        <Field label="Motivo de la devolución" hint="Obligatorio, de 5 a 500 caracteres.">
          <input
            type="text"
            maxLength={500}
            autoComplete="off"
            value={capture.reason}
            disabled={flow.locked}
            onChange={(event) => flow.update({ reason: event.target.value })}
          />
        </Field>
        <SupervisorAuthorization
          auth={flow.auth}
          disabled={flow.locked}
          description="La devolución requiere la autorización de un supervisor con el permiso «Autorizar devoluciones» (por permiso propio o por superar el umbral configurado)."
        />
        {plan && <RefundSummary plan={plan} finance={finance} />}
        {flow.hint && (
          <p className="checkout-inline-note" role="alert">
            {flow.hint}
          </p>
        )}
      </div>
      <footer className="checkout-area-footer checkout-capture-footer">
        {capture.mode === 'payment' && (
          <NumericKeypad
            value={keypadValue}
            onChange={(value) =>
              flow.update(capture.target === 'tip' ? { tip: value } : { amount: value })
            }
            label="Teclado numérico de devolución"
            captureKeyboard={keyboardActive && amountsEditable}
            disabled={!amountsEditable}
          />
        )}
        <button
          type="button"
          className="primary-button checkout-register"
          disabled={!flow.canRegister}
          onClick={() => void flow.register()}
        >
          {registerLabel}
        </button>
      </footer>
    </section>
  );
}
