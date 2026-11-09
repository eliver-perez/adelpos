import type { ReactNode } from 'react';
import { PaymentMethodIcon } from '../../../components/PaymentMethodIcon';
import { formatDollars, formatMoney, isZeroMoney } from '../../../services/payments';
import type { PaymentMethod, PaymentRecord, RefundRecord } from '../../../services/payments';
import { serverDateTime } from '../../../services/restaurant';

// Métodos como botones rectangulares: icono a la izquierda, nombre y estado seleccionado por borde
// y fondo. No se usa lista desplegable ni casilla de verificación.
export function PaymentMethodGrid({
  methods,
  selectedID,
  onSelect,
  disabled,
  label = 'Métodos de pago disponibles',
  empty = 'No hay métodos de pago activos.',
}: {
  methods: PaymentMethod[];
  selectedID: string;
  onSelect: (method: PaymentMethod) => void;
  disabled: boolean;
  label?: string;
  empty?: string;
}) {
  if (methods.length === 0) {
    return <p className="muted text-sm">{empty}</p>;
  }
  return (
    <div className="payment-method-grid" role="group" aria-label={label}>
      {methods.map((method) => (
        <button
          type="button"
          key={method.id}
          className={`payment-method-button ${selectedID === method.id ? 'payment-method-selected' : ''}`}
          aria-pressed={selectedID === method.id}
          disabled={disabled}
          onClick={() => onSelect(method)}
        >
          <PaymentMethodIcon icon={method.icon} className="payment-method-icon" />
          <span>{method.name}</span>
        </button>
      ))}
    </div>
  );
}

function PaymentHistoryItem({
  payment,
  timezone,
  original,
  actions,
}: {
  payment: PaymentRecord;
  timezone: string;
  original: PaymentRecord | undefined;
  actions: ReactNode;
}) {
  return (
    <li className={`payment-history-item ${payment.corrected ? 'payment-history-historic' : ''}`}>
      <div className="payment-history-heading">
        <strong>{payment.method_name}</strong>
        <strong>{formatMoney(payment.amount)}</strong>
      </div>
      <p className="muted text-xs">
        {serverDateTime(payment.created_at, timezone)} · {payment.actor_name} ·{' '}
        {payment.register_name}
      </p>
      <p className="payment-history-detail">
        {payment.kind === 'selection' ? 'Por productos seleccionados' : 'Abono a la cuenta'}
        {payment.is_cash && payment.received && (
          <>
            {' '}
            · Recibido {formatMoney(payment.received)} · Cambio {formatMoney(payment.change)}
          </>
        )}
        {payment.reference && <> · Ref. {payment.reference}</>}
      </p>
      {/* Un efectivo con dólares conserva lo recibido en cada moneda y el tipo de cambio aplicado. */}
      {payment.is_cash && payment.received_usd && (
        <p className="payment-history-detail">
          {formatMoney(payment.received_mxn)} y {formatDollars(payment.received_usd)} a{' '}
          {formatMoney(payment.usd_rate)} por dólar
          {payment.change_currency === 'USD' && payment.change_usd
            ? ` · cambio entregado en dólares ${formatDollars(payment.change_usd)}`
            : ''}
        </p>
      )}
      {/* La propina se muestra siempre aparte de la venta; el cambio nunca se convierte en propina. */}
      {!isZeroMoney(payment.tip) && (
        <p className="payment-history-detail">Propina aparte {formatMoney(payment.tip)}</p>
      )}
      {payment.corrected && (
        <p className="payment-history-detail payment-history-flag">
          Corregido: se revirtió y se registró de nuevo con el método correcto
        </p>
      )}
      {original && (
        <p className="payment-history-detail payment-history-flag">
          Corrige a un pago de {original.method_name}
        </p>
      )}
      {!payment.corrected && !isZeroMoney(payment.refunded_amount) && (
        <p className="payment-history-detail">
          Devuelto {formatMoney(payment.refunded_amount)}
          {!isZeroMoney(payment.refunded_tip) && ` · propina ${formatMoney(payment.refunded_tip)}`}
        </p>
      )}
      {actions && <div className="button-row payment-history-actions">{actions}</div>}
    </li>
  );
}

export function PaymentHistory({
  payments,
  timezone,
  actions,
}: {
  payments: PaymentRecord[];
  timezone: string;
  actions?: (payment: PaymentRecord) => ReactNode;
}) {
  if (payments.length === 0) {
    return <p className="muted text-sm">Aún no hay pagos registrados en esta cuenta.</p>;
  }
  return (
    <ol className="payment-history" aria-label="Pagos registrados">
      {payments.map((payment) => (
        <PaymentHistoryItem
          key={payment.id}
          payment={payment}
          timezone={timezone}
          original={payments.find((entry) => entry.id === payment.corrects_payment_id)}
          actions={actions?.(payment) ?? null}
        />
      ))}
    </ol>
  );
}

// Devoluciones y reversos de corrección: registros compensatorios que nunca editan el pago.
export function RefundHistory({
  refunds,
  timezone,
}: {
  refunds: RefundRecord[];
  timezone: string;
}) {
  if (refunds.length === 0) {
    return null;
  }
  return (
    <>
      <h3 className="bill-section-title">Devoluciones y reversos</h3>
      <ol className="payment-history" aria-label="Devoluciones registradas">
        {refunds.map((refund) => (
          <li className="payment-history-item payment-history-refund" key={refund.id}>
            <div className="payment-history-heading">
              <strong>
                {refund.kind === 'correction' ? 'Reverso por corrección' : 'Devolución'} ·{' '}
                {refund.method_name}
              </strong>
              <strong>−{formatMoney(refund.amount)}</strong>
            </div>
            <p className="muted text-xs">
              {serverDateTime(refund.created_at, timezone)} · {refund.actor_name}
              {refund.authorized_by_name && ` · autorizó ${refund.authorized_by_name}`}
            </p>
            <p className="payment-history-detail">
              {refund.reason}
              {refund.reference && ` · Ref. ${refund.reference}`}
            </p>
            {!isZeroMoney(refund.tip) && (
              <p className="payment-history-detail">Propina devuelta {formatMoney(refund.tip)}</p>
            )}
          </li>
        ))}
      </ol>
    </>
  );
}

export function CheckoutMethods({
  title,
  methods,
  selectedID,
  onSelect,
  disabled,
  payments,
  refunds,
  timezone,
  error,
  actions,
  emptyMethods,
  gridLabel,
  documents,
}: {
  title: string;
  gridLabel: string;
  documents?: ReactNode;
  methods: PaymentMethod[];
  selectedID: string;
  onSelect: (method: PaymentMethod) => void;
  disabled: boolean;
  payments: PaymentRecord[];
  refunds: RefundRecord[];
  timezone: string;
  error: string;
  actions?: (payment: PaymentRecord) => ReactNode;
  emptyMethods?: string;
}) {
  return (
    <section className="checkout-area checkout-methods" aria-label={title}>
      <header className="checkout-area-header">
        <h3>{title}</h3>
      </header>
      <div className="checkout-area-body">
        {error && <p className="checkout-inline-note">{error}</p>}
        <PaymentMethodGrid
          methods={methods}
          selectedID={selectedID}
          onSelect={onSelect}
          disabled={disabled}
          label={gridLabel}
          empty={emptyMethods}
        />
        <h3 className="bill-section-title">Pagos registrados</h3>
        <PaymentHistory payments={payments} timezone={timezone} actions={actions} />
        <RefundHistory refunds={refunds} timezone={timezone} />
        {documents}
      </div>
    </section>
  );
}
