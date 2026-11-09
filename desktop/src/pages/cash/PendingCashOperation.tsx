import { cashOperationLabels } from '../../services/cash';
import { formatDollars, formatMoney } from '../../services/payments';
import type { CashMutation } from './useCashMutation';

// Describe la operación pendiente con lo que conserva el almacén (nunca la contraseña).
function describe(mutation: CashMutation) {
  const pending = mutation.pending;
  if (!pending) {
    return '';
  }
  const label = cashOperationLabels[pending.operation] ?? 'de la operación de caja';
  const amount = typeof pending.body.amount === 'string' ? pending.body.amount : '';
  if (!amount) {
    return label;
  }
  const currency = pending.body.currency === 'USD' ? formatDollars(amount) : formatMoney(amount);
  return `${label} por ${currency}`;
}

// Aviso de una escritura de caja con resultado incierto, y explicación de un reintento rechazado
// porque el turno ya no admite la operación.
export function PendingCashOperation({ mutation }: { mutation: CashMutation }) {
  const authorized = typeof mutation.pending?.body.authorization === 'object';
  return (
    <>
      {mutation.retryNotice && (
        <div className="cash-retry-rejected" role="alert">
          <strong>El reintento no se registró.</strong>
          <p>{mutation.retryNotice}</p>
          <button
            type="button"
            className="secondary-button"
            onClick={() => mutation.clearRetryNotice()}
          >
            Entendido
          </button>
        </div>
      )}
      {mutation.uncertain && mutation.pending && (
        <div className="checkout-uncertain" role="alert">
          <strong>No se pudo confirmar el resultado {describe(mutation)}.</strong>
          <p>
            No registres otra operación en este turno hasta conocer el resultado. Reintenta con el
            mismo identificador: si ya llegó al servidor se recupera sin duplicarse.
          </p>
          {authorized && (
            <p>
              La contraseña del supervisor no se conserva: si la operación no llegó a registrarse,
              se pedirá de nuevo su autorización.
            </p>
          )}
          <div className="button-row">
            <button
              type="button"
              className="primary-button"
              disabled={mutation.busy}
              onClick={() => void mutation.retry()}
            >
              {mutation.busy ? 'Reintentando…' : 'Reintentar con el mismo identificador'}
            </button>
          </div>
        </div>
      )}
    </>
  );
}
