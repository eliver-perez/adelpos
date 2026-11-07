import { useCallback, useEffect, useRef, useState } from 'react';
import { connectionConfig, getDiagnostics } from '../services/diagnostics';
import type { ConnectionConfig, Diagnostics } from '../services/diagnostics';

export function useConnection() {
  const [config, setConfig] = useState<ConnectionConfig | null>(null);
  const [diagnostics, setDiagnostics] = useState<Diagnostics | null>(null);
  const [configError, setConfigError] = useState(false);
  const [refreshing, setRefreshing] = useState(false);
  const busy = useRef(false);
  const alive = useRef(false);

  const refresh = useCallback(async () => {
    if (!config || busy.current) {
      return;
    }
    busy.current = true;
    setRefreshing(true);
    try {
      const next = await getDiagnostics(config);
      if (alive.current) {
        setDiagnostics(next);
      }
    } finally {
      busy.current = false;
      if (alive.current) {
        setRefreshing(false);
      }
    }
  }, [config]);

  useEffect(() => {
    alive.current = true;
    let cancelled = false;
    void connectionConfig()
      .then((value) => {
        if (!cancelled) {
          setConfig(value);
        }
      })
      .catch(() => {
        if (!cancelled) {
          setConfigError(true);
        }
      });
    return () => {
      cancelled = true;
      alive.current = false;
    };
  }, []);

  useEffect(() => {
    void refresh();
    const timer = window.setInterval(() => {
      void refresh();
    }, 5000);
    return () => window.clearInterval(timer);
  }, [refresh]);

  return { config, diagnostics, configError, refreshing, refresh };
}
