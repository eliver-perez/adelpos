import { useCallback, useRef } from 'react';
import { APIError } from '../../../services/api';

// Identificadores de operación de las escrituras del diseñador. Cada acción (guardar, activar,
// renombrar…) conserva su operation_id mientras su resultado sea incierto (sin conexión o error
// del servidor): si la persona vuelve a pulsar con los mismos datos, Go devuelve la respuesta ya
// guardada en lugar de registrar la acción dos veces. Un resultado confirmado o un rechazo
// definitivo libera el identificador.

export function uncertainResult(problem: unknown) {
  return (
    problem instanceof APIError && (problem.code === 'CONNECTION_ERROR' || problem.status >= 500)
  );
}

export function useOperations() {
  const pending = useRef(new Map<string, { body: string; id: string }>());

  const run = useCallback(
    async <T>(action: string, body: unknown, call: (operationID: string) => Promise<T>) => {
      const key = JSON.stringify(body);
      const previous = pending.current.get(action);
      const id = previous && previous.body === key ? previous.id : crypto.randomUUID();
      pending.current.set(action, { body: key, id });
      try {
        const result = await call(id);
        pending.current.delete(action);
        return result;
      } catch (problem) {
        if (!uncertainResult(problem)) {
          pending.current.delete(action);
        }
        throw problem;
      }
    },
    [],
  );

  return run;
}

export type RunOperation = ReturnType<typeof useOperations>;
