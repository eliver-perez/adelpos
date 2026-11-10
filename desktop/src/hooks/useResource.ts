import { useCallback, useEffect, useRef, useState } from 'react';
import { api } from '../services/api';
import type { Operation } from '../services/api';

export function useResource<T>(
  operation: Operation,
  options: {
    id?: string;
    query?: Record<string, string>;
    enabled?: boolean;
    // Evento de la aplicación que vuelve a consultar el recurso. Por omisión, cualquier cambio del
    // servidor; la sección Inventarios usa solo sus propios cambios (hooks/useInventoryEvents.ts).
    reloadEvent?: string;
  } = {},
) {
  const [data, setData] = useState<T | null>(null);
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(true);
  const { id, enabled = true, reloadEvent = 'adel-server-change' } = options;
  const queryKey = JSON.stringify(options.query ?? {});
  const generation = useRef(0);

  const reload = useCallback(async () => {
    if (!enabled) {
      setLoading(false);
      return;
    }
    setError('');
    setLoading(true);
    const request = ++generation.current;
    try {
      const result = await api<T>(operation, {
        id,
        query: JSON.parse(queryKey) as Record<string, string>,
      });
      if (request === generation.current) setData(result);
    } catch (error) {
      if (request === generation.current)
        setError(error instanceof Error ? error.message : 'No se pudo cargar la información.');
    } finally {
      if (request === generation.current) setLoading(false);
    }
  }, [operation, id, queryKey, enabled]);

  useEffect(() => {
    void reload();
    let timer: number | undefined;
    const invalidate = () => {
      window.clearTimeout(timer);
      timer = window.setTimeout(() => void reload(), 75);
    };
    if (enabled) window.addEventListener(reloadEvent, invalidate);
    return () => {
      generation.current++;
      window.clearTimeout(timer);
      window.removeEventListener(reloadEvent, invalidate);
    };
  }, [reload, enabled, reloadEvent]);

  return { data, error, loading, reload };
}
