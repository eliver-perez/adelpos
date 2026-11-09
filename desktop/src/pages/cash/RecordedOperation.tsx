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

// Título del resultado: tipo, importe con el formato único y concepto. Es el mismo texto del aviso
// de confirmación; nunca incluye el efectivo esperado.
export function recordedOperationTitle(operation: CashOperation) {
  const kind = operation.regularization
    ? 'Regularización'
    : cashOperationKindLabels[operation.kind];
  // Corrección y regularización son femeninas; ingreso, retiro y gasto, masculinos.
  const feminine = operation.regularization || operation.kind === 'correction';
  const verb = feminine ? 'registrada' : 'registrado';
  return `${kind} ${verb}: ${currencyAmount(operation.currency, operation.amount)} · ${operation.concept}`;
}

// Resultado confirmado de un ingreso, retiro, gasto o corrección, con lo que devolvió Go: también
// el estado del comprobante y del comando del cajón, que nunca revierten la operación. En la fase A
// de la etapa 7.1 sigue en pantalla como resultado de la operación (además del aviso de
// confirmación); la fase C cerrará el panel y llevará estos estados al aviso.
export function RecordedOperation({ operation }: { operation: RecordedCashOperation }) {
  return (
    <div className="checkout-result" role="status">
      <strong>{recordedOperationTitle(operation)}</strong>
      <p>
        {authorizationText(operation)}
        {operation.exceptional ? ' · Salida excepcional' : ''}
      </p>
      <PrintNotice outcome={operation.printing} subject="la operación" />
      <DrawerNotice outcome={operation.drawer} />
    </div>
  );
}
