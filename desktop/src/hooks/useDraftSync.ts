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
          // Etapa 7.1: la cuenta de la mesa se crea con su primer producto, con la misma apertura
          // sin formulario (Go asigna la lista predeterminada, el responsable y la referencia
          // neutra) y la misma clave de pendientes: un resultado incierto se recupera con el mismo
          // operation_id.
          create: (body) =>
            latest.current.execute<Bill>('restaurant_open_account', {
              body: {
                name: '',
                mode: 'dine_in',
                table_id: body.table_id,
                price_list_id: null,
                customer_name: '',
                phone: '',
                origin: '',
                notes: '',
                scheduled_local: '',
                schedule_kind: '',
                assigned_user_id: null,
                seen_account_ids: body.seen_account_ids,
                items: body.items,
              },
            }),
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
