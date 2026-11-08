import { financialOperationLabels, formatMoney } from '../../../services/payments';
import type { PaymentMethod } from '../../../services/payments';
import type { PendingRequest } from '../../../hooks/pendingOperations';

// Resume la operación pendiente con lo que conserva el almacén (sin contraseña).
function describe(request: PendingRequest, methods: PaymentMethod[]) {
  const body = request.body;
  const label = financialOperationLabels[request.operation] ?? 'de la operación';
  const details: string[] = [];
  const amount = typeof body.amount === 'string' ? body.amount : '';
  if (amount) {
    details.push(formatMoney(amount));
  }
  const methodID = typeof body.method_id === 'string' ? body.method_id : '';
  const method = methods.find((entry) => entry.id === methodID);
  if (method) {
    details.push(`con ${method.name}`);
  }
  return [label, ...details].join(' ');
}

// Operación financiera con resultado incierto. Mientras exista, la cuenta no admite otra escritura
// financiera: se reintenta con el mismo identificador o se consulta el resultado.
export function UncertainOperation({
  request,
  methods,
  busy,
  onRetry,
  onConsult,
}: {
  request: PendingRequest;
  methods: PaymentMethod[];
  busy: boolean;
  onRetry: () => void;
  onConsult: () => void;
}) {
  const authorized = typeof request.body.authorization === 'object';
  return (
    <div className="checkout-uncertain" role="alert">
      <strong>No se pudo confirmar el resultado {describe(request, methods)}.</strong>
      <p>
        No registres otra operación financiera en esta cuenta. Reintenta con el mismo identificador:
        si ya llegó al servidor se recupera sin duplicarse. También puedes consultar el resultado.
      </p>
      {authorized && (
        <p>
          La contraseña del supervisor no se conserva: si la operación no llegó a registrarse, se
          pedirá de nuevo su autorización.
        </p>
      )}
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
