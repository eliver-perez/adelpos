import { useId } from 'react';
import type { SupervisorAuthorizationState } from '../hooks/useSupervisorAuthorization';

// Autorización puntual en línea: usuario y contraseña de un supervisor para una sola operación.
// No inicia sesión ni afecta la sesión del supervisor en su propia terminal.
export function SupervisorAuthorization({
  auth,
  disabled = false,
  description,
}: {
  auth: SupervisorAuthorizationState;
  disabled?: boolean;
  description?: string;
}) {
  const identifier = useId();
  if (!auth.visible) {
    return null;
  }
  return (
    <fieldset className="supervisor-authorization" disabled={disabled}>
      <legend>Autorización de supervisor</legend>
      <p className="muted text-xs">
        {description ??
          'Esta operación requiere la autorización de un supervisor. Es puntual: no inicia sesión ni cambia la sesión del supervisor en su terminal.'}
      </p>
      <div className="supervisor-authorization-fields">
        <label className="field" htmlFor={`${identifier}-user`}>
          <span>Usuario del supervisor</span>
          <input
            id={`${identifier}-user`}
            autoComplete="off"
            autoCapitalize="none"
            spellCheck={false}
            maxLength={40}
            value={auth.username}
            onChange={(event) => auth.setUsername(event.target.value)}
          />
        </label>
        <label className="field" htmlFor={`${identifier}-password`}>
          <span>Contraseña del supervisor</span>
          <input
            id={`${identifier}-password`}
            type="password"
            autoComplete="off"
            maxLength={256}
            value={auth.password}
            onChange={(event) => auth.setPassword(event.target.value)}
          />
        </label>
      </div>
      {auth.message && (
        <p className="checkout-inline-note" role="alert">
          {auth.message}
        </p>
      )}
    </fieldset>
  );
}
