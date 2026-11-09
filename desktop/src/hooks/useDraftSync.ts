import { useRef, useState, useSyncExternalStore } from 'react';
import { DraftSyncEngine } from '../pages/restaurant/capture/draftSync';
import type { Bill } from '../services/restaurant';
import { useRestaurantMutation } from './useRestaurantMutation';

// Clave del almacén global de operaciones pendientes para el guardado del borrador: una respuesta
// incierta conserva su operation_id y cuerpo aunque se cambie de pantalla.
export const draftPendingKey = 'restaurant:draft';

// Conecta el motor de sincronización del borrador con el mecanismo existente de escrituras
// idempotentes (useRestaurantMutation + almacén de pendientes).
export function useDraftSync(delay = 700) {
  // Resultado de la última respuesta confirmada; lo usa la recuperación de una operación incierta.
  const lastResult = useRef<Bill | null>(null);
  const mutation = useRestaurantMutation((_operation, result) => {
    lastResult.current = result as Bill;
  }, draftPendingKey);
  const latest = useRef(mutation);
  latest.current = mutation;
  const [engine] = useState(
    () =>
      new DraftSyncEngine(
        {
          save: (accountID, body) =>
            latest.current.execute<Bill>('restaurant_save_draft', { id: accountID, body }),
          retry: async () => {
            lastResult.current = null;
            const problem = await latest.current.retry();
            if (problem) {
              throw problem;
            }
            if (!lastResult.current) {
              throw new Error('No se pudo recuperar el resultado del borrador.');
            }
            return lastResult.current;
          },
        },
        delay,
      ),
  );
  const version = useSyncExternalStore(engine.subscribe, engine.getVersion, engine.getVersion);
  return { engine, version };
}
