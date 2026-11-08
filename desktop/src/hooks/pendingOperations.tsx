import { createContext, useContext, useState, useSyncExternalStore } from 'react';
import type { ReactNode } from 'react';
import type { Operation } from '../services/api';

export interface PendingRequest {
  operation: Operation;
  id?: string;
  body: Record<string, unknown>;
}

export interface PendingEntry {
  request: PendingRequest;
  // La solicitud está en curso con este mismo identificador.
  running: boolean;
  // La última respuesta se perdió o fue un error del servidor: el resultado es desconocido.
  uncertain: boolean;
}

// Copia exacta del cuerpo para reintentos, sin la contraseña de una autorización de supervisor.
// Go excluye esa contraseña de la huella idempotente, por lo que el reintento sin ella recupera el
// resultado ya confirmado; nunca se conserva el secreto en memoria más allá del primer envío.
export function storableRequest(request: PendingRequest): PendingRequest {
  const body = JSON.parse(JSON.stringify(request.body)) as Record<string, unknown>;
  const authorization = body.authorization;
  if (authorization && typeof authorization === 'object' && !Array.isArray(authorization)) {
    delete (authorization as Record<string, unknown>).password;
  }
  const stored: PendingRequest = { operation: request.operation, body };
  if (request.id !== undefined) {
    stored.id = request.id;
  }
  return stored;
}

// Almacén de operaciones con resultado pendiente o incierto, indexado por una clave estable
// (por ejemplo, payments:<cuenta> o kitchen). Conserva operación, recurso, cuerpo y operation_id.
export class PendingOperationStore {
  private entries = new Map<string, PendingEntry>();
  private listeners = new Set<() => void>();

  subscribe = (listener: () => void) => {
    this.listeners.add(listener);
    return () => {
      this.listeners.delete(listener);
    };
  };

  get(key: string): PendingEntry | undefined {
    return this.entries.get(key);
  }

  // Inicia un envío. Un reintento conserva la marca de incertidumbre hasta conocer el resultado.
  begin(key: string, request: PendingRequest) {
    const previous = this.entries.get(key);
    this.entries.set(key, {
      request: storableRequest(request),
      running: true,
      uncertain: previous?.uncertain ?? false,
    });
    this.emit();
  }

  markUncertain(key: string) {
    const entry = this.entries.get(key);
    if (!entry) {
      return;
    }
    this.entries.set(key, { ...entry, running: false, uncertain: true });
    this.emit();
  }

  // El servidor respondió de forma definitiva (éxito o rechazo): el identificador ya no se reutiliza.
  resolve(key: string) {
    if (!this.entries.delete(key)) {
      return;
    }
    this.emit();
  }

  private emit() {
    for (const listener of this.listeners) {
      listener();
    }
  }
}

export const PendingOperationsContext = createContext<PendingOperationStore | null>(null);

// Proveedor a nivel de aplicación: vive fuera de las páginas, por lo que cambiar de pantalla no
// pierde el identificador de una operación incierta.
export function PendingOperationsProvider({ children }: { children: ReactNode }) {
  const [store] = useState(() => new PendingOperationStore());
  return (
    <PendingOperationsContext.Provider value={store}>{children}</PendingOperationsContext.Provider>
  );
}

const emptyStore = new PendingOperationStore();

// Consulta de solo lectura de un pendiente; sin proveedor no hay pendientes que mostrar.
export function usePendingEntry(key: string): PendingEntry | undefined {
  const store = useContext(PendingOperationsContext) ?? emptyStore;
  return useSyncExternalStore(
    store.subscribe,
    () => store.get(key),
    () => store.get(key),
  );
}
