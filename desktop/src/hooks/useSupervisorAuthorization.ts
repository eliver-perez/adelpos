import { useState } from 'react';
import { APIError } from '../services/api';
import type { AuthorizationInput } from '../services/payments';

// Códigos con los que Go pide o rechaza una autorización puntual de supervisor. Ninguno invalida
// la sesión del cajero: api.ts solo la cierra ante SESSION_* o DEVICE_*.
export const authorizationCodes = [
  'AUTHORIZATION_REQUIRED',
  'AUTHORIZATION_INVALID',
  'AUTHORIZATION_THROTTLED',
  'SELF_AUTHORIZATION',
  'AUTHORIZATION_INSUFFICIENT',
];

export const repeatAuthorizationMessage =
  'La operación anterior no llegó a registrarse y su autorización no se conserva. Solicita de nuevo la autorización del supervisor y vuelve a enviarla.';

export function isAuthorizationProblem(problem: unknown): problem is APIError {
  return problem instanceof APIError && authorizationCodes.includes(problem.code);
}

// Estado del formulario de autorización. La contraseña vive solo aquí y se borra en cuanto se
// toma para un envío, sea cual sea el resultado; nunca entra al almacén de pendientes.
//
// requiredByPlan es null mientras Go recalcula la previsualización: se conserva el último valor
// conocido para que el formulario no desaparezca y reaparezca en cada cálculo.
export function useSupervisorAuthorization(requiredByPlan: boolean | null) {
  const [requested, setRequested] = useState(false);
  const [known, setKnown] = useState(false);
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [message, setMessage] = useState('');
  if (requiredByPlan !== null && requiredByPlan !== known) {
    setKnown(requiredByPlan);
  }
  const visible = (requiredByPlan ?? known) || requested;
  const ready = !visible || (username.trim() !== '' && password !== '');

  // Devuelve la autorización para el cuerpo de la solicitud y elimina la contraseña del estado.
  function take(): AuthorizationInput | undefined {
    if (!visible) {
      return undefined;
    }
    const value =
      username.trim() !== '' && password !== ''
        ? { username: username.trim(), password }
        : undefined;
    setPassword('');
    setMessage('');
    return value;
  }

  // Muestra el formulario con el mensaje de Go cuando la respuesta corresponde a la autorización.
  function handleError(problem: unknown) {
    if (!isAuthorizationProblem(problem)) {
      return false;
    }
    setRequested(true);
    setPassword('');
    setMessage(problem.message);
    return true;
  }

  // Un reintento sin contraseña de una operación que no llegó a confirmarse exige repetirla.
  function requireRepeat() {
    setRequested(true);
    setPassword('');
    setMessage(repeatAuthorizationMessage);
  }

  function reset() {
    setRequested(false);
    setUsername('');
    setPassword('');
    setMessage('');
  }

  return {
    visible,
    ready,
    username,
    setUsername,
    password,
    setPassword,
    message,
    take,
    handleError,
    requireRepeat,
    reset,
  };
}

export type SupervisorAuthorizationState = ReturnType<typeof useSupervisorAuthorization>;
