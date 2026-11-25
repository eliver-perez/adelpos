import { useEffect, useRef, useState } from 'react';
import { Channel, invoke } from '@tauri-apps/api/core';
import { cashChangeEvent } from './useCashEvents';
import type { CashChange } from './useCashEvents';
import { inventoryChangeEvent } from './useInventoryEvents';
import type { InventoryChange } from './useInventoryEvents';
import { identityChangeEvent } from '../services/documentIdentity';
import { templatesChangeEvent } from '../services/documents';
import type { TemplatesChange } from '../services/documents';
import { licenseChangeEvent } from '../services/license';
import { serverLink } from '../navigation/serverLink';

interface Change {
  id: string;
  sequence: number;
  kind: string;
  scope: string;
  version: number;
  aggregate_id?: string | null;
}
interface StreamMessage {
  connection?: 'connected' | 'reconnecting' | 'unauthorized';
  cursor?: number;
  reset?: boolean;
  events?: Change[];
  error?: string;
  status?: number;
}

// Textos del estado del canal de eventos. La barra superior (etapa 9C) los traduce a sus tres
// estados del servidor (navigation/serverState.ts) a partir de estas mismas constantes.
export const realtimeLabels = {
  periodic: 'Actualización periódica',
  connected: 'En tiempo real',
  reconnecting: 'Recuperando eventos · sondeo activo',
  interrupted: 'Canal interrumpido · sondeo activo',
  unavailable: 'Sondeo activo · canal no disponible',
} as const;

export function useRealtime(identityKey: string, enabled: boolean) {
  const [status, setStatus] = useState<string>(realtimeLabels.periodic);
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
      setStatus(realtimeLabels.periodic);
      serverLink.channelClosed();
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
            ? realtimeLabels.connected
            : realtimeLabels.reconnecting,
        );
      if (message.error) setStatus(realtimeLabels.interrupted);
      // Etapa K1 (Fase E, K1-D10): el canal alimenta el estado único de conexión. Rust manda «reconnecting» solo con una
      // falla real (la renovación normal del canal cada 30 s no manda nada) y «connected» solo al volver de una falla;
      // cada mensaje de cambios dice que el canal está vivo.
      if (message.connection === 'reconnecting') {
        serverLink.channelFailure();
      } else if (message.connection === 'connected' || message.cursor !== undefined) {
        serverLink.channelAlive();
      }
      let changed = !!message.reset || message.connection === 'connected';
      // Los eventos de caja (etapa 7) se publican además con su tipo y su turno o caja, para que
      // Caja y el cobro vuelvan a consultar de inmediato. Un reinicio del cursor también los avisa.
      const cash: CashChange = { kinds: [], aggregates: [] };
      let cashChanged = !!message.reset;
      // Etapa 8: los eventos de inventario recargan solo la pestaña visible de Inventarios, que no
      // se recarga con los cambios de otras áreas. Solo llevan el tipo, nunca cantidades ni costos.
      const inventory: InventoryChange = { kinds: [] };
      let inventoryChanged = !!message.reset;
      // Etapa 9B: la identidad del establecimiento (logotipo y textos) vuelve a consultarse solo
      // con su propio evento, porque su lectura incluye las imágenes.
      let identityChanged = !!message.reset;
      // Etapa 9B (fase C): el editor de plantillas avisa si otra persona guardó la plantilla que se
      // edita; el evento solo lleva el identificador de la plantilla.
      const templates: TemplatesChange = { templates: [] };
      let templatesChanged = !!message.reset;
      // Etapa L1 (Fase C): license.changed avisa que cambió la licencia o el bloqueo por la hora; la sesión vuelve a
      // leer su estado.
      let licenseChanged = !!message.reset;
      for (const event of message.events ?? []) {
        if (event.version !== 1) {
          changed = true;
          continue;
        }
        if (seen.current.has(event.id)) continue;
        seen.current.add(event.id);
        changed = true;
        if (event.scope === 'cash') {
          cashChanged = true;
          cash.kinds.push(event.kind);
          if (event.aggregate_id) cash.aggregates.push(event.aggregate_id);
        }
        if (event.scope === 'documents' && event.kind === 'documents.identity.changed') {
          identityChanged = true;
        }
        if (event.scope === 'documents' && event.kind === 'documents.templates.changed') {
          templatesChanged = true;
          if (event.aggregate_id) templates.templates.push(event.aggregate_id);
        }
        if (event.scope === 'inventory') {
          inventoryChanged = true;
          inventory.kinds.push(event.kind);
        }
        if (event.kind === 'license.changed') {
          licenseChanged = true;
        }
      }
      if (seen.current.size > 5000) seen.current.clear();
      if (message.cursor !== undefined) cursor.current = message.cursor;
      // El evento invalida consultas; nunca sustituye el estado autoritativo del servidor.
      if (changed) window.dispatchEvent(new CustomEvent('adel-server-change'));
      if (cashChanged) window.dispatchEvent(new CustomEvent(cashChangeEvent, { detail: cash }));
      if (identityChanged) {
        window.dispatchEvent(new CustomEvent(identityChangeEvent));
      }
      if (templatesChanged) {
        window.dispatchEvent(new CustomEvent(templatesChangeEvent, { detail: templates }));
      }
      if (inventoryChanged) {
        window.dispatchEvent(new CustomEvent(inventoryChangeEvent, { detail: inventory }));
      }
      if (licenseChanged) {
        window.dispatchEvent(new CustomEvent(licenseChangeEvent));
      }
    };
    void invoke<number>('events_start', { channel, after: cursor.current, stationId: null })
      .then((identifier) => {
        subscriptionID = identifier;
        if (!active) void invoke('events_stop', { subscriptionId: identifier });
      })
      .catch(() => {
        if (active) setStatus(realtimeLabels.unavailable);
      });
    return () => {
      active = false;
      serverLink.channelClosed();
      if (subscriptionID !== null)
        void invoke('events_stop', { subscriptionId: subscriptionID }).catch(() => undefined);
    };
  }, [enabled, identityKey]);
  return status;
}
