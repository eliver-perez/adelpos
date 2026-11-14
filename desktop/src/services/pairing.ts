import { invoke } from '@tauri-apps/api/core';

// Emparejamiento de esta terminal con la computadora principal (etapa 9G, fase B; §3.2 y decisiones D2 y
// D5). React solo propone la dirección escrita a mano y el nombre: Rust la valida (red local del
// restaurante), fija el certificado del servidor, pide el código, espera la aprobación y guarda la
// credencial en el almacén seguro (desktop/src-tauri/src/pairing.rs). El secreto de la solicitud y los
// números del código nunca llegan aquí.

export type PairingState = 'pending' | 'approved' | 'paired' | 'rejected' | 'expired' | 'cancelled';

export interface PairingView {
  state: PairingState | 'claimed';
  // Código de 6 dígitos que se escribe en la computadora principal.
  code: string;
  name: string;
  // Nombre del restaurante que informó el servidor.
  server_name: string;
  expires_at: string;
}

// Nombre propuesto por omisión (editable en la pantalla).
export const defaultTerminalName = 'Caja 2';

export async function startPairing(address: string, name: string) {
  return invoke<PairingView>('pairing_start', { address, name });
}

export async function readPairing() {
  return invoke<PairingView>('pairing_status');
}

export async function renamePairing(name: string) {
  return invoke<PairingView>('pairing_rename', { name });
}

export async function cancelPairing() {
  await invoke('pairing_cancel');
}

// Deja de usar el servidor emparejado para emparejar de nuevo (terminal revocada o servidor cambiado).
export async function forgetPairing() {
  await invoke('pairing_forget');
}

// Error del puente (texto de Rust, en lenguaje del restaurante) o cualquier otro.
export function pairingError(error: unknown, fallback = 'No se pudo emparejar esta terminal.') {
  return typeof error === 'string' && error.length > 0 ? error : fallback;
}

// El código se muestra en dos grupos de tres para leerlo en voz alta sin errores.
export function groupedCode(code: string) {
  return code.length === 6 ? `${code.slice(0, 3)} ${code.slice(3)}` : code;
}
