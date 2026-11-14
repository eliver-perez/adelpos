import { invoke } from '@tauri-apps/api/core';
import type { PairingView } from './pairing';

// Búsqueda de la computadora principal en la red local (etapa 9G, fase C; §4.1 y decisión D2). Rust hace la
// consulta mDNS (desktop/src-tauri/src/discovery.rs) y entrega solo el nombre, el identificador y si ya es el
// servidor emparejado; las direcciones se quedan en Rust. Para emparejarse, React manda el identificador del
// servidor elegido, nunca un destino.

export interface DiscoveredServer {
  id: string;
  // «Fonda Doña Ñeli — Mac de caja»: establecimiento y computadora.
  name: string;
  paired: boolean;
}

export async function discoverServers() {
  return invoke<DiscoveredServer[]>('discovery_search');
}

export async function startPairingFound(serverID: string, name: string) {
  return invoke<PairingView>('pairing_start_found', { serverId: serverID, name });
}

// Error del puente (texto de Rust) o el general.
export function discoveryError(error: unknown) {
  return typeof error === 'string' && error.length > 0
    ? error
    : 'No se pudo buscar en la red local. Puedes escribir la dirección de la computadora principal.';
}
