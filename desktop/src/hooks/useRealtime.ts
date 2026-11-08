import { useEffect, useRef, useState } from 'react';
import { Channel, invoke } from '@tauri-apps/api/core';

interface Change {
  id: string;
  sequence: number;
  kind: string;
  scope: string;
  version: number;
}
interface StreamMessage {
  connection?: 'connected' | 'reconnecting' | 'unauthorized';
  cursor?: number;
  reset?: boolean;
  events?: Change[];
  error?: string;
  status?: number;
}

export function useRealtime(identityKey: string, enabled: boolean) {
  const [status, setStatus] = useState('Actualización periódica');
  const cursor = useRef(0);
  const seen = useRef(new Set<string>());
  const previousIdentity = useRef(identityKey);

  useEffect(() => {
    if (previousIdentity.current !== identityKey) {
      cursor.current = 0;
      seen.current.clear();
      previousIdentity.current = identityKey;
    }
    if (!enabled) {
      setStatus('Actualización periódica');
      return;
    }
    let active = true;
    let subscriptionID: number | null = null;
    const channel = new Channel<StreamMessage>();
    channel.onmessage = (message) => {
      if (!active) return;
      if (message.connection)
        setStatus(
          message.connection === 'connected'
            ? 'En tiempo real'
            : 'Recuperando eventos · sondeo activo',
        );
      if (message.error) setStatus('Canal interrumpido · sondeo activo');
      let changed = !!message.reset || message.connection === 'connected';
      for (const event of message.events ?? []) {
        if (event.version !== 1) {
          changed = true;
          continue;
        }
        if (seen.current.has(event.id)) continue;
        seen.current.add(event.id);
        changed = true;
      }
      if (seen.current.size > 5000) seen.current.clear();
      if (message.cursor !== undefined) cursor.current = message.cursor;
      // El evento invalida consultas; nunca sustituye el estado autoritativo del servidor.
      if (changed) window.dispatchEvent(new CustomEvent('adel-server-change'));
    };
    void invoke<number>('events_start', { channel, after: cursor.current, stationId: null })
      .then((identifier) => {
        subscriptionID = identifier;
        if (!active) void invoke('events_stop', { subscriptionId: identifier });
      })
      .catch(() => {
        if (active) setStatus('Sondeo activo · canal no disponible');
      });
    return () => {
      active = false;
      if (subscriptionID !== null)
        void invoke('events_stop', { subscriptionId: subscriptionID }).catch(() => undefined);
    };
  }, [enabled, identityKey]);
  return status;
}
