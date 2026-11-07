import { useCallback, useEffect, useRef, useState } from 'react';
import { api, APIError, localAuthentication } from '../services/api';
import type { LocalAuthentication } from '../services/api';
import type { Identity } from '../services/models';

type Phase =
  'loading' | 'offline' | 'secure-storage' | 'setup' | 'enroll' | 'login' | 'authenticated';

export function useSession() {
  const [phase, setPhase] = useState<Phase>('loading');
  const [identity, setIdentity] = useState<Identity | null>(null);
  const [local, setLocal] = useState<LocalAuthentication | null>(null);
  const [message, setMessage] = useState('');
  const [connectionError, setConnectionError] = useState('');
  const busy = useRef(false);

  const refresh = useCallback(async () => {
    if (busy.current) {
      return;
    }
    busy.current = true;
    try {
      const credentials = await localAuthentication();
      setLocal(credentials);
      const setup = await api<{ configured: boolean }>('setup_status');
      if (!setup.configured) {
        setPhase('setup');
      } else if (!credentials.enrolled) {
        setIdentity(null);
        setPhase('enroll');
      } else if (!credentials.has_session) {
        setIdentity(null);
        setPhase('login');
      } else {
        const next = await api<Identity>('me');
        setIdentity(next);
        setPhase('authenticated');
      }
      setConnectionError('');
    } catch (error) {
      if (error instanceof APIError && error.status === 401) {
        setMessage(error.message);
        setIdentity(null);
        const credentials = await localAuthentication();
        setLocal(credentials);
        setPhase(credentials.enrolled ? 'login' : 'enroll');
      } else {
        setConnectionError(error instanceof Error ? error.message : 'No se pudo conectar.');
        const waitingForStorage =
          error instanceof APIError && error.code === 'SECURE_STORAGE_UNAVAILABLE';
        setPhase((current) => {
          if (current === 'authenticated') return current;
          return waitingForStorage ? 'secure-storage' : 'offline';
        });
      }
    } finally {
      busy.current = false;
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  useEffect(() => {
    if (phase === 'loading') {
      return;
    }
    // La consulta periódica notifica reemplazos/revocaciones; Go rechaza la sesión inmediatamente.
    const timer = window.setInterval(() => {
      void refresh();
    }, 5000);
    return () => window.clearInterval(timer);
  }, [phase, refresh]);

  useEffect(() => {
    const invalid = (event: Event) => {
      const error = (event as CustomEvent<APIError>).detail;
      setMessage(error.message);
      void refresh();
    };
    window.addEventListener('session-invalid', invalid);
    return () => window.removeEventListener('session-invalid', invalid);
  }, [refresh]);

  const logout = useCallback(async () => {
    await api('logout');
    setIdentity(null);
    setMessage('Sesión cerrada correctamente.');
    setPhase('login');
  }, []);

  return { phase, identity, local, message, connectionError, refresh, logout };
}
