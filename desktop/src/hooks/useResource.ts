import { useCallback, useEffect, useState } from 'react';
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

  const reload = useCallback(async () => {
    if (!enabled) {
      setLoading(false);
      return;
    }
    setError('');
    setLoading(true);
    try {
      const result = await api<T>(operation, {
        id,
        query: JSON.parse(queryKey) as Record<string, string>,
      });
      setData(result);
    } catch (error) {
      setError(error instanceof Error ? error.message : 'No se pudo cargar la información.');
    } finally {
      setLoading(false);
    }
  }, [operation, id, queryKey, enabled]);

  useEffect(() => {
    void reload();
  }, [reload]);

  return { data, error, loading, reload };
}
