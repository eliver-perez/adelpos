import { useCallback, useEffect, useRef, useState } from 'react';
import { api, APIError, clientUpdateEvent, localAuthentication } from '../services/api';
import { clientUpdateRequired, serverOutdated } from '../services/compatibility';
import type { LocalAuthentication } from '../services/api';
import { reconnectDelay } from '../services/reconnection';
import type { Identity } from '../services/models';

// update-terminal y update-server (etapa 9G, §5.1): la aplicación o el servidor son demasiado viejos para trabajar
// juntos; la terminal lo dice en lugar de fallar en cada operación.
type Phase =
  | 'loading'
  | 'offline'
  | 'secure-storage'
  | 'update-terminal'
  | 'update-server'
  | 'setup'
  | 'enroll'
  | 'login'
  | 'authenticated';

export function useSession() {
  const [phase, setPhase] = useState<Phase>('loading');
  const [identity, setIdentity] = useState<Identity | null>(null);
  const [local, setLocal] = useState<LocalAuthentication | null>(null);
  const [message, setMessage] = useState('');
  const [connectionError, setConnectionError] = useState('');
  // Etapa 9G (fase C, D11): consultas fallidas seguidas sin conexión (la espera crece con ellas) y cuándo es el
  // próximo intento automático, para la capa «Sin conexión con el servidor».
  const [failures, setFailures] = useState(0);
  const [retryAt, setRetryAt] = useState<number | null>(null);
  const busy = useRef(false);
  const versionChecked = useRef(false);

  const refresh = useCallback(async () => {
    if (busy.current) {
      return;
    }
    busy.current = true;
    try {
      const credentials = await localAuthentication();
      setLocal(credentials);
      // Etapa 9G: un servidor anterior a la versión que necesita esta aplicación se avisa antes de cualquier otra
      // consulta (sus rutas nuevas no existirían). Se comprueba al conectar y otra vez después de perder la conexión.
      if (!versionChecked.current) {
        const version = await api<{ server_version?: unknown }>('version');
        if (serverOutdated(version.server_version)) {
          setConnectionError('');
          setPhase('update-server');
          return;
        }
        versionChecked.current = true;
      }
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
      setFailures(0);
    } catch (error) {
      if (clientUpdateRequired(error)) {
        setConnectionError('');
        setPhase('update-terminal');
      } else if (error instanceof APIError && error.status === 401) {
        setMessage(error.message);
        setIdentity(null);
        const credentials = await localAuthentication();
        setLocal(credentials);
        setPhase(credentials.enrolled ? 'login' : 'enroll');
      } else {
        versionChecked.current = false;
        setConnectionError(error instanceof Error ? error.message : 'No se pudo conectar.');
        setFailures((count) => count + 1);
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
    // Sin conexión (etapa 9G, D11): se reintenta sola con espera creciente (1, 2, 4, 8 y después cada 10 s).
    if (connectionError) {
      const delay = reconnectDelay(failures);
      setRetryAt(Date.now() + delay);
      const timer = window.setTimeout(() => {
        void refresh();
      }, delay);
      return () => window.clearTimeout(timer);
    }
    setRetryAt(null);
    // La consulta periódica notifica reemplazos/revocaciones; Go rechaza la sesión inmediatamente.
    const timer = window.setInterval(() => {
      void refresh();
    }, 5000);
    return () => window.clearInterval(timer);
  }, [phase, refresh, connectionError, failures]);

  // Cualquier operación rechazada con «Actualiza esta terminal» lleva a su aviso (services/api.ts lo anuncia).
  useEffect(() => {
    const outdated = () => setPhase('update-terminal');
    window.addEventListener(clientUpdateEvent, outdated);
    return () => window.removeEventListener(clientUpdateEvent, outdated);
  }, []);

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

  return { phase, identity, local, message, connectionError, retryAt, refresh, logout };
}
