import { invoke, isTauri } from '@tauri-apps/api/core';
import { api, APIError, rejectionOf } from './api';
import type { LicenseClock, LicenseStatus, SeatUsage } from './license';

// Operaciones de la licencia del escritorio (etapa L1, Fase E). Las rutas están en la lista cerrada de api.rs; la
// solicitud sin internet y los archivos (.lic y la respuesta de soporte) los manejan las operaciones propias de Rust de
// license_files.rs: React nunca recibe una ruta ni el contenido de un archivo. Los textos de cada rechazo los pone
// services/api.ts con services/license.ts.

export function readLicenseStatus() {
  return api<LicenseStatus>('license_status');
}

// activateLicense es «Activar» en línea (§8.3 del contrato). Con ACTIVATION_LIMIT y transfer_available, la pantalla
// ofrece «Transferir a esta computadora».
export function activateLicense(licenseKey: string) {
  return api<LicenseStatus>('license_activate', { body: { license_key: licenseKey } });
}

// transferLicense se pide solo después de que el usuario lo confirmó (confirmed: true, también en Go).
export function transferLicense(licenseKey: string) {
  return api<LicenseStatus>('license_transfer', {
    body: { license_key: licenseKey, confirmed: true },
  });
}

export function refreshLicense() {
  return api<LicenseStatus>('license_refresh', { body: {} });
}

// deactivateLicense es «Liberar esta licencia», después de su confirmación.
export function deactivateLicense() {
  return api<LicenseStatus>('license_deactivate', { body: { confirmed: true } });
}

export interface IdentityRegeneration {
  server_id: string;
  revoked_terminals: number;
  restart_required: boolean;
  status: LicenseStatus;
}

// regenerateIdentity es «Crear una identidad nueva para esta computadora» en una instalación copiada (L1-D11).
export function regenerateIdentity() {
  return api<IdentityRegeneration>('license_identity_regenerate', { body: { confirmed: true } });
}

// --- La fecha y hora (Fase C) ----------------------------------------------------------------------------------------

export interface TimeCheck {
  matches: boolean;
  replaced: boolean;
  server_time_text: string;
  clock: LicenseClock;
}

export function checkTimeWithInternet() {
  return api<TimeCheck>('license_time_check', { body: {} });
}

export interface SupportRequest {
  request_code: string;
  expires_at: string;
  expires_at_text: string;
  last_activity_time_text: string;
  reused: boolean;
}

export function requestSupportCode(renew = false) {
  return api<SupportRequest>('license_support_request', { body: renew ? { renew: true } : {} });
}

export interface SupportUnlock {
  accepted: boolean;
  clock: LicenseClock;
}

export function unlockWithSupportText(response: string) {
  return api<SupportUnlock>('license_support_unlock', { body: { response, source: 'text' } });
}

// --- Cupos (Fase D) --------------------------------------------------------------------------------------------------

export function releaseDeviceSeat(deviceID: string) {
  return api<{ released: string[]; sessions_closed: number }>('devices_seat_release', {
    id: deviceID,
    body: {},
  });
}

export function setKitchenDisplayMode(active: boolean) {
  return api<{ active: boolean }>('kitchen_display_mode', { body: { active } });
}

export interface SessionsSeats {
  seats?: SeatUsage;
  limited?: boolean;
}

// --- Operaciones propias de Rust (license_files.rs) ------------------------------------------------------------------

function requireDesktop() {
  if (!isTauri()) {
    throw new APIError(
      'DESKTOP_REQUIRED',
      'Los archivos de la licencia requieren la aplicación de escritorio.',
    );
  }
}

// Un rechazo del puente llega como texto (Rust) y uno de Go como {status, body}, igual que api_request.
async function bridge(command: string, args: Record<string, unknown> = {}) {
  requireDesktop();
  let response: { status: number; body: unknown };
  try {
    response = await invoke<{ status: number; body: unknown }>(command, args);
  } catch (error) {
    const message =
      typeof error === 'string' && error.length > 0
        ? error
        : 'No se pudo completar la operación en esta computadora.';
    throw new APIError('BRIDGE_ERROR', message);
  }
  if (response.status >= 400) {
    throw rejectionOf(response);
  }
  return response.body;
}

export type OfflineAction = 'activate' | 'renew' | 'deactivate';

export type RequestSave = { result: 'cancelled' } | { result: 'saved'; file_name: string };

// saveOfflineRequest es «Solicitud sin internet»: Go arma la solicitud y Rust la guarda con el diálogo «Guardar como».
export async function saveOfflineRequest(action: OfflineAction, licenseKey?: string) {
  const args: Record<string, unknown> = { action };
  if (action === 'activate' && licenseKey && licenseKey.trim() !== '') {
    args.licenseKey = licenseKey.trim();
  }
  return (await bridge('license_save_request', args)) as RequestSave;
}

export type LicenseImport =
  { result: 'cancelled' } | { result: 'imported'; license: LicenseStatus };

// importLicenseFile es «Importar archivo de licencia»: Rust abre el diálogo, lee el .lic y lo envía a Go.
export async function importLicenseFile() {
  return (await bridge('license_import_file')) as LicenseImport;
}

export type UnlockImport = { result: 'cancelled' } | { result: 'unlocked'; unlock: SupportUnlock };

// importSupportResponse es «Importar la respuesta» del desbloqueo de soporte (archivo .desbloqueo).
export async function importSupportResponse() {
  return (await bridge('license_import_unlock_file')) as UnlockImport;
}
