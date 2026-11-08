import { useContext, useState, useSyncExternalStore } from 'react';
import { api, APIError } from '../services/api';
import type { Operation } from '../services/api';
import { PendingOperationStore, PendingOperationsContext } from './pendingOperations';
import type { PendingRequest } from './pendingOperations';

const localKey = 'local';

// Ejecuta escrituras idempotentes con operation_id. Ante un resultado incierto (sin conexión o
// error ≥ 500) conserva la operación y su cuerpo exacto para reintentarla con el mismo UUID.
//
// Sin clave, el pendiente pertenece a la instancia del hook (comportamiento original). Con clave,
// se guarda en el almacén global de la aplicación y sobrevive al desmontaje de la página.
export function useRestaurantMutation(
  onResolved: (operation: Operation, result: unknown) => void,
  key?: string,
) {
  const shared = useContext(PendingOperationsContext);
  const [local] = useState(() => new PendingOperationStore());
  if (key !== undefined && !shared) {
    throw new Error('Las operaciones con clave requieren PendingOperationsProvider.');
  }
  const store = key !== undefined && shared ? shared : local;
  const storeKey = key ?? localKey;
  const entry = useSyncExternalStore(
    store.subscribe,
    () => store.get(storeKey),
    () => store.get(storeKey),
  );
  const [error, setError] = useState('');
  const [errorCode, setErrorCode] = useState('');

  async function send<T>(request: PendingRequest): Promise<T> {
    if (store.get(storeKey)?.running) {
      throw new Error('Espera a que termine la operación actual.');
    }
    store.begin(storeKey, request);
    setError('');
    setErrorCode('');
    try {
      const result = await api<T>(request.operation, { id: request.id, body: request.body });
      store.resolve(storeKey);
      onResolved(request.operation, result);
      return result;
    } catch (problem) {
      const unknownResult =
        problem instanceof APIError &&
        (problem.code === 'CONNECTION_ERROR' || problem.status >= 500);
      if (unknownResult) {
        store.markUncertain(storeKey);
      } else {
        store.resolve(storeKey);
      }
      setError(problem instanceof Error ? problem.message : 'No se pudo completar la operación.');
      setErrorCode(problem instanceof APIError ? problem.code : '');
      throw problem;
    }
  }

  async function execute<T>(
    operation: Operation,
    options: { id?: string; body?: Record<string, unknown> },
  ): Promise<T> {
    if (store.get(storeKey)) {
      throw new Error('Primero recupera el resultado de la operación pendiente.');
    }
    const request: PendingRequest = {
      operation,
      id: options.id,
      body: { ...options.body, operation_id: crypto.randomUUID() },
    };
    return send<T>(request);
  }

  // Reintenta con el mismo operation_id y cuerpo. Devuelve el error (o null si se confirmó) para
  // que la pantalla distinga, por ejemplo, una autorización que debe repetirse.
  async function retry(): Promise<unknown> {
    const pending = store.get(storeKey);
    if (!pending) {
      return null;
    }
    try {
      await send(pending.request);
      return null;
    } catch (problem) {
      // El mismo identificador permanece disponible si vuelve a fallar la comunicación.
      return problem;
    }
  }

  return {
    execute,
    retry,
    busy: entry?.running ?? false,
    error,
    errorCode,
    uncertain: entry?.uncertain ?? false,
    pending: entry?.request ?? null,
    // Lectura sincrónica del almacén: evita un segundo envío por doble toque antes de renderizar.
    isPending: () => store.get(storeKey) !== undefined,
    clearError: () => {
      setError('');
      setErrorCode('');
    },
  };
}
