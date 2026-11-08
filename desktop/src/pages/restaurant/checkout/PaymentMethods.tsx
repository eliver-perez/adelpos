import { PaymentMethodIcon } from '../../../components/PaymentMethodIcon';
import { formatMoney, isZeroMoney } from '../../../services/payments';
import type { PaymentMethod, PaymentRecord } from '../../../services/payments';
import { serverDateTime } from '../../../services/restaurant';

// Métodos como botones rectangulares: icono a la izquierda, nombre y estado seleccionado por borde
// y fondo. No se usa lista desplegable ni casilla de verificación.
export function PaymentMethodGrid({
  methods,
  selectedID,
  onSelect,
  disabled,
}: {
  methods: PaymentMethod[];
  selectedID: string;
  onSelect: (method: PaymentMethod) => void;
  disabled: boolean;
}) {
  if (methods.length === 0) {
    return <p className="muted text-sm">No hay métodos de pago activos.</p>;
  }
  return (
    <div className="payment-method-grid" role="group" aria-label="Métodos de pago disponibles">
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

function PaymentHistoryItem({ payment, timezone }: { payment: PaymentRecord; timezone: string }) {
  return (
    <li className="payment-history-item">
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
        {!isZeroMoney(payment.tip) && <> · Propina {formatMoney(payment.tip)}</>}
        {payment.reference && <> · Ref. {payment.reference}</>}
      </p>
      {(!isZeroMoney(payment.refunded_amount) || payment.corrected) && (
        <p className="payment-history-detail">
          {payment.corrected ? 'Pago corregido' : 'Devuelto'} {formatMoney(payment.refunded_amount)}
        </p>
      )}
    </li>
  );
}

export function PaymentHistory({
  payments,
  timezone,
}: {
  payments: PaymentRecord[];
  timezone: string;
}) {
  if (payments.length === 0) {
    return <p className="muted text-sm">Aún no hay pagos registrados en esta cuenta.</p>;
  }
  return (
    <ol className="payment-history" aria-label="Pagos registrados">
      {payments.map((payment) => (
        <PaymentHistoryItem key={payment.id} payment={payment} timezone={timezone} />
      ))}
    </ol>
  );
}

export function CheckoutMethods({
  methods,
  selectedID,
  onSelect,
  disabled,
  payments,
  timezone,
  error,
}: {
  methods: PaymentMethod[];
  selectedID: string;
  onSelect: (method: PaymentMethod) => void;
  disabled: boolean;
  payments: PaymentRecord[];
  timezone: string;
  error: string;
}) {
  return (
    <section className="checkout-area checkout-methods" aria-label="Métodos de pago">
      <header className="checkout-area-header">
        <h3>Método de pago</h3>
      </header>
      <div className="checkout-area-body">
        {error && <p className="checkout-inline-note">{error}</p>}
        <PaymentMethodGrid
          methods={methods}
          selectedID={selectedID}
          onSelect={onSelect}
          disabled={disabled}
        />
        <h3 className="bill-section-title">Pagos registrados</h3>
        <PaymentHistory payments={payments} timezone={timezone} />
      </div>
    </section>
  );
}
