import { useRef, useState } from 'react';
import { api, APIError } from '../services/api';
import type { Operation } from '../services/api';

interface PendingOperation {
  operation: Operation;
  id?: string;
  body: Record<string, unknown>;
}

export function useRestaurantMutation(onResolved: (operation: Operation, result: unknown) => void) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [uncertain, setUncertain] = useState(false);
  const pending = useRef<PendingOperation | null>(null);
  const running = useRef(false);

  async function send<T>(request: PendingOperation): Promise<T> {
    if (running.current) throw new Error('Espera a que termine la operación actual.');
    running.current = true;
    setBusy(true);
    setError('');
    try {
      const result = await api<T>(request.operation, { id: request.id, body: request.body });
      pending.current = null;
      setUncertain(false);
      onResolved(request.operation, result);
      return result;
    } catch (problem) {
      const unknownResult =
        problem instanceof APIError &&
        (problem.code === 'CONNECTION_ERROR' || problem.status >= 500);
      if (!unknownResult) pending.current = null;
      setUncertain(unknownResult);
      setError(problem instanceof Error ? problem.message : 'No se pudo completar la operación.');
      throw problem;
    } finally {
      running.current = false;
      setBusy(false);
    }
  }

  async function execute<T>(
    operation: Operation,
    options: { id?: string; body?: Record<string, unknown> },
  ): Promise<T> {
    if (pending.current)
      throw new Error('Primero recupera el resultado de la operación pendiente.');
    const request: PendingOperation = {
      operation,
      id: options.id,
      body: { ...options.body, operation_id: crypto.randomUUID() },
    };
    pending.current = request;
    return send<T>(request);
  }

  async function retry() {
    if (!pending.current) return;
    try {
      await send(pending.current);
    } catch {
      // El mismo identificador permanece disponible si vuelve a fallar la comunicación.
    }
  }

  return { execute, retry, busy, error, uncertain, clearError: () => setError('') };
}
