import { SupervisorAuthorization } from '../../components/SupervisorAuthorization';
import type { SupervisorAuthorizationState } from '../../hooks/useSupervisorAuthorization';
import { permissionName } from '../../services/payments';

// Autorización de una salida o corrección con la regla de §0-B: si quien registra tiene el
// permiso de autorizarla, su propio permiso basta y no se pide otra contraseña; si no, la
// autorización puntual de otra persona con ese permiso.
export function OutflowAuthorization({
  selfAuthorized,
  exceptional = false,
  auth,
  disabled,
  permission = 'cash.outflows.authorize',
}: {
  selfAuthorized: boolean;
  exceptional?: boolean;
  auth: SupervisorAuthorizationState;
  disabled: boolean;
  permission?: string;
}) {
  if (selfAuthorized) {
    return (
      <p className="cash-self-authorized" role="status">
        Autorización propia: tu permiso «{permissionName(permission)}»
        {exceptional ? ` y «${permissionName('cash.expected.read')}»` : ''} autoriza esta operación.
        No se pide otra contraseña y el registro queda autorizado por ti.
      </p>
    );
  }
  return (
    <SupervisorAuthorization
      auth={auth}
      disabled={disabled}
      description={`Esta operación requiere la autorización puntual de otra persona con el permiso «${permissionName(permission)}». No inicia sesión ni cambia la sesión del supervisor en su terminal.`}
    />
  );
}
