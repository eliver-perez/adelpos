import { IconLabel } from '../../components/Icon';
import { cashOperationLabels } from '../../services/cash';
import { formatMoney } from '../../services/money';
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
  const currency = pending.body.currency === 'USD' ? 'USD' : 'MXN';
  return `${label} por ${formatMoney(amount, currency)}`;
}

// Panel persistente de una escritura de caja con resultado incierto: sigue en pantalla hasta que
// se recupera el resultado con el mismo identificador. El rechazo de un reintento porque el turno
// ya no admite la operación se avisa desde useCashMutation con el sistema de mensajes.
export function PendingCashOperation({ mutation }: { mutation: CashMutation }) {
  const authorized = typeof mutation.pending?.body.authorization === 'object';
  if (!mutation.uncertain || !mutation.pending) {
    return null;
  }
  return (
    <div className="checkout-uncertain" role="alert">
      <strong>No se pudo confirmar el resultado {describe(mutation)}.</strong>
      <p>
        No registres otra operación en este turno hasta conocer el resultado. Reintenta con el mismo
        identificador: si ya llegó al servidor se recupera sin duplicarse.
      </p>
      {authorized && (
        <p>
          La contraseña del supervisor no se conserva: si la operación no llegó a registrarse, se
          pedirá de nuevo su autorización.
        </p>
      )}
      <div className="button-row">
        <button
          type="button"
          className="primary-button"
          disabled={mutation.busy}
          onClick={() => void mutation.retry()}
        >
          <IconLabel icon="refresh">
            {mutation.busy ? 'Reintentando…' : 'Reintentar con el mismo identificador'}
          </IconLabel>
        </button>
      </div>
    </div>
  );
}
