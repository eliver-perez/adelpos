import { invoke, isTauri } from '@tauri-apps/api/core';
import defaultConnection from '../generated/connection.json';

// Modalidad de esta computadora (etapa 9G, fase A; §2.3). React solo elige una de las tres de la lista
// cerrada; Rust la guarda en la conexión de la terminal y, si el servicio del servidor está instalado, en
// su carpeta de datos con el archivo de habilitación (desktop/src-tauri/src/installation.rs). Después la
// aplicación se reinicia para conectarse con la configuración nueva. Dejar de ser servidor nunca borra
// la base: el servidor deja de atender y conserva los datos.

export type InstallationMode = 'server-terminal' | 'server-only' | 'terminal-only';

export const installationModes: InstallationMode[] = [
  'server-terminal',
  'server-only',
  'terminal-only',
];

export const modeLabels: Record<InstallationMode, string> = {
  'server-terminal': 'Servidor y terminal',
  'server-only': 'Solo servidor',
  'terminal-only': 'Solo terminal',
};

export const modeDescriptions: Record<InstallationMode, string> = {
  'server-terminal':
    'La computadora principal del restaurante: guarda los datos, atiende a las demás terminales y además se usa como caja.',
  'server-only':
    'Una computadora apartada que guarda los datos y atiende a las terminales. En ella no se vende.',
  'terminal-only':
    'Una caja o terminal de cocina que se conecta a la computadora principal del restaurante.',
};

export interface InstallationState {
  // Ya se eligió la modalidad (en macOS se elige en el primer arranque).
  chosen: boolean;
  mode: InstallationMode;
  // La terminal ya tiene un servidor al que conectarse.
  server_configured: boolean;
  // La modalidad se puede cambiar desde esta computadora.
  can_change: boolean;
  // El servidor del restaurante está instalado como servicio de esta computadora.
  service_installed: boolean;
  service_enabled: boolean;
  // Sin servidor en esta modalidad, pero la computadora conserva la base de cuando lo era.
  server_data_kept: boolean;
}

export function includesServer(mode: InstallationMode) {
  return mode === 'server-terminal' || mode === 'server-only';
}

// Fuera de la aplicación de escritorio (vista previa de desarrollo) no hay modalidad que elegir.
function browserState(): InstallationState {
  return {
    chosen: true,
    mode: defaultConnection.installation_mode as InstallationMode,
    server_configured: true,
    can_change: false,
    service_installed: false,
    service_enabled: false,
    server_data_kept: false,
  };
}

export async function readInstallation(): Promise<InstallationState> {
  if (!isTauri()) {
    return browserState();
  }
  return invoke<InstallationState>('installation_state');
}

export async function chooseInstallationMode(mode: InstallationMode) {
  await invoke('set_installation_mode', { mode });
}

export async function restartApplication() {
  await invoke('restart_application');
}

// Error del puente (texto de Rust) o cualquier otro, en un mensaje para la persona.
export function installationError(error: unknown) {
  return typeof error === 'string' && error.length > 0
    ? error
    : 'No se pudo cambiar la modalidad de esta computadora.';
}
