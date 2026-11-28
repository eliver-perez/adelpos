import { useRef, useState, useSyncExternalStore } from 'react';
import { DraftSyncEngine } from '../pages/restaurant/capture/draftSync';
import { api, APIError } from '../services/api';
import { defaultTakeawayOrigin } from '../services/restaurant';
import type { Bill, TableAccountRef } from '../services/restaurant';
import { useRestaurantMutation } from './useRestaurantMutation';

// Etapa M1 (M1-D25): con TABLE_IN_UNION Go manda solo los identificadores de las cuentas de la unión en que quedó la
// mesa. Para ofrecer «Agregar a la Cuenta #folio» como con TABLE_HAS_ACCOUNT se lee cada una (como mucho seis, como
// Android); el rechazo sigue siendo el de Go, con su código y su texto.
const maxUnionAccounts = 6;

async function withUnionAccounts(problem: unknown): Promise<unknown> {
  if (!(problem instanceof APIError) || problem.code !== 'TABLE_IN_UNION') {
    return problem;
  }
  const details = (problem.details ?? {}) as { accounts?: unknown[] };
  const ids = (details.accounts ?? [])
    .filter((entry): entry is string => typeof entry === 'string')
    .slice(0, maxUnionAccounts);
  const accounts: TableAccountRef[] = [];
  for (const id of ids) {
    try {
      const bill = await api<Bill>('restaurant_account', { id });
      if (bill.account.state === 'open') {
        accounts.push({ id, folio: bill.account.folio, name: bill.account.name });
      }
    } catch {
      // Una cuenta que no se pudo leer no se ofrece; las demás sí.
    }
  }
  return new APIError(problem.code, problem.message, problem.status, { ...details, accounts });
}

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
          // operation_id. Etapa 9D: igual el pedido para llevar (con el origen de «Nuevo pedido»)
          // y la cuenta sin mesa.
          //
          // Etapa M1 (M1-D9): con una unión en pantalla viajan union_table_ids o union_group_id, y la unión que se
          // veía en la mesa en seen_union_id (solo si el motor los trae: un servidor sin uniones los rechazaría).
          create: async (body) => {
            const { union_table_ids, union_group_id, seen_union_id, ...rest } = body;
            const union: Record<string, unknown> = {};
            if (union_table_ids) union.union_table_ids = union_table_ids;
            if (union_group_id) union.union_group_id = union_group_id;
            if (seen_union_id !== undefined) union.seen_union_id = seen_union_id;
            try {
              return await latest.current.execute<Bill>('restaurant_open_account', {
                body: {
                  name: '',
                  mode: rest.mode,
                  table_id: rest.table_id,
                  price_list_id: null,
                  customer_name: '',
                  phone: '',
                  origin: rest.mode === 'takeaway' ? defaultTakeawayOrigin : '',
                  notes: '',
                  scheduled_local: '',
                  schedule_kind: '',
                  assigned_user_id: null,
                  seen_account_ids: rest.seen_account_ids,
                  items: rest.items,
                  ...union,
                },
              });
            } catch (problem) {
              throw await withUnionAccounts(problem);
            }
          },
          retry: async () => {
            lastResult.current = null;
            const problem = await latest.current.retry();
            if (problem) {
              throw await withUnionAccounts(problem);
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
