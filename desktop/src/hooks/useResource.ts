import { useCallback, useEffect, useRef, useState } from 'react';
import { api } from '../services/api';
import type { Operation } from '../services/api';

export function useResource<T>(
  operation: Operation,
  options: {
    id?: string;
    query?: Record<string, string>;
    enabled?: boolean;
  } = {},
) {
  const [data, setData] = useState<T | null>(null);
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(true);
  const { id, enabled = true } = options;
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
    if (enabled) window.addEventListener('adel-server-change', invalidate);
    return () => {
      generation.current++;
      window.clearTimeout(timer);
      window.removeEventListener('adel-server-change', invalidate);
    };
  }, [reload, enabled]);

  return { data, error, loading, reload };
}
