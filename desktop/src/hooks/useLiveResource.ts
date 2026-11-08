import { useCallback, useEffect, useRef, useState } from 'react';
import { api } from '../services/api';
import type { Operation } from '../services/api';

export function useLiveResource<T>(
  operation: Operation,
  options: {
    id?: string;
    query?: Record<string, string>;
    enabled?: boolean;
    interval?: number;
  } = {},
) {
  const { id, enabled = true, interval = 5000 } = options;
  const queryKey = JSON.stringify(options.query ?? {});
  const key = `${operation}|${id ?? ''}|${queryKey}`;
  const generation = useRef(0);
  const inFlight = useRef(false);
  const [state, setState] = useState<{
    key: string;
    data: T | null;
    error: string;
    loading: boolean;
  }>({ key, data: null, error: '', loading: false });

  const reload = useCallback(async () => {
    if (!enabled) return;
    const request = ++generation.current;
    inFlight.current = true;
    setState((previous) => ({
      key,
      data: previous.key === key ? previous.data : null,
      error: '',
      loading: true,
    }));
    try {
      const data = await api<T>(operation, {
        id,
        query: JSON.parse(queryKey) as Record<string, string>,
      });
      if (request === generation.current) setState({ key, data, error: '', loading: false });
    } catch (problem) {
      if (request === generation.current) {
        setState((previous) => ({
          key,
          data: previous.key === key ? previous.data : null,
          loading: false,
          error: problem instanceof Error ? problem.message : 'No se pudo actualizar.',
        }));
      }
    } finally {
      if (request === generation.current) inFlight.current = false;
    }
  }, [operation, id, queryKey, enabled, key]);

  useEffect(() => {
    void reload();
    if (!enabled) return;
    const timer = window.setInterval(() => {
      if (document.visibilityState !== 'hidden' && !inFlight.current) void reload();
    }, interval);
    return () => {
      generation.current++;
      window.clearInterval(timer);
    };
  }, [reload, enabled, interval]);

  useEffect(() => {
    if (!enabled) return;
    let timer: number | undefined;
    const invalidate = () => {
      window.clearTimeout(timer);
      timer = window.setTimeout(() => void reload(), 75);
    };
    window.addEventListener('adel-server-change', invalidate);
    return () => {
      window.clearTimeout(timer);
      window.removeEventListener('adel-server-change', invalidate);
    };
  }, [enabled, reload]);

  return {
    data: state.key === key ? state.data : null,
    error: state.key === key ? state.error : '',
    loading: enabled && (state.key !== key || state.loading),
    reload,
  };
}
