import { invoke, isTauri } from '@tauri-apps/api/core';
import { api, APIError } from './api';
import { countLabel } from './plural';

// Respaldos programados (etapa 9G, fase F; §7): el servidor respalda la base de datos todos los días a la hora
// configurada, revisa cada respaldo y conserva los últimos; si la carpeta elegida no está disponible usa la
// predeterminada y lo avisa. Go decide todo (cuándo toca, qué carpeta, qué se borra y qué se avisa); la pantalla solo
// lo muestra y pide los cambios. La carpeta se elige con el diálogo nativo de la computadora principal (Rust envía la
// ruta a Go); la interfaz nunca envía una ruta.

export type BackupKind = 'automatic' | 'manual';
export type BackupStatus = 'running' | 'succeeded' | 'failed';
export type BackupFallbackReason = '' | 'missing' | 'not_directory' | 'not_writable';
export type BackupFailureReason =
  '' | 'directory_unavailable' | 'dump_failed' | 'verify_failed' | 'interrupted';

export interface BackupRun {
  id: string;
  kind: BackupKind;
  status: BackupStatus;
  started_at: string;
  finished_at: string | null;
  file_name: string;
  size_bytes: number | null;
  // «default» = carpeta predeterminada; «custom» = la elegida.
  location: 'default' | 'custom' | '';
  folder_name: string;
  volume_name: string;
  // Ruta completa: solo para «Información para soporte».
  directory: string;
  used_default_directory: boolean;
  fallback_reason: BackupFallbackReason;
  verified: boolean;
  failure_reason: BackupFailureReason;
  // La retención lo borró (solo se conservan los últimos automáticos).
  removed: boolean;
  // Solo en la lista: el archivo sigue en su carpeta.
  available?: boolean;
}

export interface BackupAlert {
  kind: 'failed' | 'fallback';
  reason: string;
  since: string;
  failures: number;
}

export interface BackupDirectory {
  custom: boolean;
  path: string;
  folder_name: string;
  volume_name: string;
  available: boolean;
}

export interface BackupSettings {
  enabled: boolean;
  // null = la «Hora de inicio del día».
  time: string | null;
  effective_time: string;
  business_day_cutoff: string;
  keep: number;
  directory: BackupDirectory;
  default_directory: BackupDirectory;
}

export interface BackupState {
  settings: BackupSettings;
  next_run_at: string | null;
  pending: boolean;
  running: boolean;
  last_backup: BackupRun | null;
  last_success: BackupRun | null;
  alert: BackupAlert | null;
  runs: BackupRun[];
  // La carpeta solo se cambia desde la computadora principal.
  directory_change_available: boolean;
}

export interface BackupSummary {
  last_backup: BackupRun | null;
  alert: BackupAlert | null;
  running: boolean;
}

export interface BackupSettingsInput {
  enabled: boolean;
  time: string | null;
  keep: number;
}

export const minimumKeep = 1;
export const maximumKeep = 365;

export function readBackups() {
  return api<BackupState>('maintenance_backups');
}

export function saveBackupSettings(input: BackupSettingsInput) {
  return api<BackupState>('maintenance_save_backup_settings', { body: input });
}

export function restoreDefaultBackupDirectory() {
  return api<BackupState>('maintenance_backup_default_directory', { body: {} });
}

export function backupNow(operationID: string) {
  return api<{ run: BackupRun }>('maintenance_backup_now', {
    body: { operation_id: operationID },
  });
}

export type DirectoryChoice = { result: 'cancelled' } | { result: 'saved'; state: BackupState };

// Abre el diálogo nativo de carpeta (operación cerrada choose_backup_directory de Rust). Rust envía a Go la carpeta
// elegida; aquí solo llega si se canceló o el estado nuevo de los respaldos.
export async function chooseBackupDirectory(): Promise<DirectoryChoice> {
  if (!isTauri()) {
    throw new APIError(
      'DESKTOP_REQUIRED',
      'Elegir la carpeta requiere la aplicación de escritorio.',
    );
  }
  let response: { status: number; body: unknown };
  try {
    response = await invoke('choose_backup_directory');
  } catch (error) {
    const message =
      typeof error === 'string' ? error : 'No se pudo abrir el diálogo de la carpeta.';
    throw new APIError('DIRECTORY_CHOICE_FAILED', message);
  }
  if (response.status >= 400) {
    const body = response.body as { error?: { code?: string; message?: string } };
    const error = new APIError(
      body.error?.code ?? 'API_ERROR',
      body.error?.message ?? 'No se pudo guardar la carpeta de los respaldos.',
      response.status,
    );
    if (
      response.status === 401 &&
      (error.code.startsWith('SESSION_') || error.code.startsWith('DEVICE_'))
    ) {
      window.dispatchEvent(new CustomEvent('session-invalid', { detail: error }));
    }
    throw error;
  }
  const outcome = response.body as DirectoryChoice;
  if (outcome.result !== 'saved' && outcome.result !== 'cancelled') {
    throw new APIError(
      'DIRECTORY_CHOICE_FAILED',
      'El puente nativo no respondió con un resultado válido.',
    );
  }
  return outcome;
}

// Avisa a las pantallas abiertas (el aviso de la pantalla de inicio y el estado) que los respaldos cambiaron.
export function announceBackupChange() {
  window.dispatchEvent(new CustomEvent('adel-server-change'));
}

// Fecha y hora de un respaldo, sin segundos.
export function backupDateTime(value: string) {
  return new Intl.DateTimeFormat('es-MX', { dateStyle: 'medium', timeStyle: 'short' }).format(
    new Date(value),
  );
}

// Tamaño en palabras del restaurante (KB, MB o GB con un decimal).
export function sizeLabel(bytes: number | null) {
  if (bytes === null || bytes <= 0) {
    return '';
  }
  const units = ['KB', 'MB', 'GB'];
  let value = bytes / 1024;
  let unit = 0;
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024;
    unit += 1;
  }
  const decimals = value >= 100 || unit === 0 ? 0 : 1;
  return `${value.toFixed(decimals)} ${units[unit]}`;
}

// Dónde está una carpeta: la predeterminada o la elegida por su nombre (y el del disco, si está en otro).
export function folderLabel(folder: {
  location?: string;
  custom?: boolean;
  folder_name: string;
  volume_name: string;
}) {
  const custom = folder.custom ?? folder.location === 'custom';
  if (!custom) {
    return 'la carpeta predeterminada';
  }
  if (folder.folder_name && folder.volume_name) {
    return `«${folder.folder_name}» en «${folder.volume_name}»`;
  }
  if (folder.volume_name) {
    return `el disco «${folder.volume_name}»`;
  }
  return `la carpeta «${folder.folder_name}»`;
}

export const failureLabels: Record<Exclude<BackupFailureReason, ''>, string> = {
  directory_unavailable: 'No se pudo guardar el archivo en ninguna carpeta.',
  dump_failed: 'No se pudo copiar la base de datos.',
  verify_failed: 'Se creó, pero al revisarlo no se pudo leer; se descartó.',
  interrupted: 'Se interrumpió porque el servidor se detuvo.',
};

export const fallbackLabels: Record<Exclude<BackupFallbackReason, ''>, string> = {
  missing: 'no se encontró (¿se desconectó el disco o la USB?)',
  not_directory: 'ya no es una carpeta',
  not_writable: 'no permite guardar archivos',
};

export function failureLabel(reason: string) {
  return (
    failureLabels[reason as Exclude<BackupFailureReason, ''>] ?? 'No se pudo crear el respaldo.'
  );
}

export function fallbackLabel(reason: string) {
  return fallbackLabels[reason as Exclude<BackupFallbackReason, ''>] ?? 'no estaba disponible';
}

export const kindLabels: Record<BackupKind, string> = {
  automatic: 'Automático',
  manual: 'Manual',
};

// Resultado de un respaldo en una frase corta.
export function runResult(run: BackupRun) {
  if (run.status === 'running') {
    return 'En curso';
  }
  if (run.status === 'failed') {
    return 'Falló';
  }
  return run.verified ? 'Correcto y revisado' : 'Correcto';
}

// Texto del aviso persistente al administrador (pantalla de inicio, Mantenimiento y «Solo servidor»).
export function alertMessage(alert: BackupAlert) {
  if (alert.kind === 'failed') {
    const repeated =
      alert.failures > 1
        ? ` Van ${countLabel(alert.failures, 'intento fallido', 'intentos fallidos')} desde el ${backupDateTime(alert.since)}.`
        : '';
    return {
      title: 'No se pudo crear el respaldo de la base de datos',
      description: `${failureLabel(alert.reason)}${repeated} Revisa los respaldos en Sistema › Mantenimiento o usa «Respaldar ahora».`,
    };
  }
  return {
    title: 'La carpeta de los respaldos no estaba disponible',
    description: `La carpeta elegida ${fallbackLabel(alert.reason)}, así que el último respaldo se guardó en la carpeta predeterminada. Conecta el disco o elige otra carpeta en Sistema › Mantenimiento.`,
  };
}

// «Último respaldo» de las tarjetas de estado.
export function lastBackupCard(run: BackupRun | null | undefined): {
  value: string;
  description: string;
  state: 'good' | 'warning' | 'neutral';
} {
  if (!run) {
    return {
      value: 'Sin registro',
      description: 'Todavía no hay respaldos automáticos ni de «Respaldar ahora».',
      state: 'neutral',
    };
  }
  const when = backupDateTime(run.finished_at ?? run.started_at);
  if (run.status === 'failed') {
    return {
      value: 'Falló',
      description: `${when}. ${failureLabel(run.failure_reason)}`,
      state: 'warning',
    };
  }
  if (run.status === 'running') {
    return { value: 'En curso', description: `Empezó el ${when}.`, state: 'neutral' };
  }
  const size = sizeLabel(run.size_bytes);
  return {
    value: 'Correcto',
    description: `${when}${size ? ` · ${size}` : ''}. Revisado y guardado en ${folderLabel(run)}.`,
    state: run.used_default_directory && run.fallback_reason ? 'warning' : 'good',
  };
}
