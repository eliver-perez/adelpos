import { cashOperationKindLabels } from '../../services/cash';
import type { CashOperation, RecordedCashOperation } from '../../services/cash';
import { DrawerNotice, PrintNotice } from './CashNotices';
import { currencyAmount } from './DenominationCount';

// Describe quién autorizó una operación de efectivo sin mostrar identificadores internos.
export function authorizationText(operation: CashOperation) {
  if (operation.authorization_mode === 'self') {
    return `Autorizado con el permiso propio de ${operation.authorized_by_name ?? operation.actor_name}`;
  }
  if (operation.authorization_mode === 'supervisor') {
    return `Autorizado por ${operation.authorized_by_name ?? 'un supervisor'}`;
  }
  return 'Sin autorización adicional';
}

// Resultado confirmado de un ingreso, retiro, gasto o corrección, con lo que devolvió Go: también
// el estado del comprobante y del comando del cajón, que nunca revierten la operación.
export function RecordedOperation({ operation }: { operation: RecordedCashOperation }) {
  const kind = operation.regularization
    ? 'Regularización'
    : cashOperationKindLabels[operation.kind];
  // Corrección y regularización son femeninas; ingreso, retiro y gasto, masculinos.
  const feminine = operation.regularization || operation.kind === 'correction';
  return (
    <div className="checkout-result" role="status">
      <strong>
        {kind} {feminine ? 'registrada' : 'registrado'}:{' '}
        {currencyAmount(operation.currency, operation.amount)} · {operation.concept}
      </strong>
      <p>
        {authorizationText(operation)}
        {operation.exceptional ? ' · Salida excepcional' : ''}
      </p>
      <PrintNotice outcome={operation.printing} subject="la operación" />
      <DrawerNotice outcome={operation.drawer} />
    </div>
  );
}
