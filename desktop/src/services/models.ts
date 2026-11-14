import type { TerminalHome } from '../navigation/sections';

export interface Preferences {
  theme: 'light' | 'dark';
  home_screen: string;
}

export interface User {
  id: string;
  username: string;
  display_name: string;
  role_id: string;
  role_name: string;
  active: boolean;
  initial_password: boolean;
}

export interface Identity {
  user: User & { permissions: string[]; support: boolean; session_id: string; device_id: string };
  preferences: Preferences;
  terminal: Device;
  // Desde la etapa 9F incluye el nombre visible de la precuenta (services/documentNames.ts).
  establishment: Pick<Settings, 'name' | 'timezone' | 'currency'> & { precheck_name?: string };
}

export interface Role {
  id: string;
  code: string | null;
  name: string;
  permissions: string[];
  // Porcentaje máximo de descuento sin autorización (cadena de Go); null equivale a 0 %.
  discount_limit?: string | null;
}

export interface Permission {
  key: string;
  description: string;
}

export interface Settings {
  name: string;
  timezone: string;
  currency: string;
  phone: string;
  email: string;
  address: string;
  theme: 'light' | 'dark';
}

export interface Register {
  id: string;
  name: string;
  description: string;
  active: boolean;
}

export interface Device {
  id: string;
  name: string;
  register_id: string | null;
  register_name?: string | null;
  keyboard_mode: 'auto' | 'always' | 'disabled';
  // Pantalla de inicio de la terminal (etapa 9C, §1.5); Go la informa desde la migración 0020.
  home_screen?: TerminalHome;
  last_seen_at?: string;
  revoked_at?: string | null;
  // Etapa 9G (fase B): tipo de dispositivo, sistema y versión de la aplicación que informó al
  // emparejarse, cuándo se emparejó, si está en línea (petición reciente) y el usuario con sesión activa.
  kind?: DeviceKind;
  os?: string;
  app_version?: string;
  paired_at?: string | null;
  online?: boolean;
  current_user?: string | null;
}

// Tipos de dispositivo (etapa 9G, decisión D9): lista cerrada de Go.
export type DeviceKind = 'desktop' | 'android' | 'kitchen';

// Solicitud pendiente de una terminal nueva (etapa 9G, §3.2). El código no viaja: el administrador
// escribe el que muestra la terminal.
export interface PairingRequest {
  id: string;
  name: string;
  device_kind: DeviceKind;
  os: string;
  app_version: string;
  created_at: string;
  expires_at: string;
  // La terminal ya envió su parte del código.
  ready: boolean;
  attempts_left: number;
}

export interface PairingRequests {
  requests: PairingRequest[];
  // Solo en la computadora principal se aprueban las terminales.
  approve_here: boolean;
  // Solo en la computadora principal: si recibe terminales de la red y cómo escribir su dirección.
  network?: {
    available: boolean;
    port: number;
    addresses: string[];
  };
}

// Sesiones activas por tipo de dispositivo (etapa 9G, §3.4): solo se cuentan, no se limita nada.
export interface SessionsSummary {
  by_kind: Record<DeviceKind, number>;
  total: number;
  limited: boolean;
}

// Configuración de una terminal (etapa 9G, §3.3). Desde la fase E, sus impresoras: las de AD-EL POS conectadas a
// ella (printers) y las que informó su computadora para su agente de impresión (local_printers; null si es una
// terminal revocada).
export interface DeviceConfiguration {
  device: Device;
  kitchen_all_stations: boolean;
  kitchen_sound_alert: boolean;
  kitchen_stations: { id: string; name: string; active: boolean }[];
  printers: { id: string; name: string; destination?: string; active?: boolean }[];
  local_printers?: {
    agent_online: boolean;
    agent_seen_at: string | null;
    reported_at: string | null;
    queues: {
      queue: string;
      model: string;
      connection: string;
      usable: boolean;
      reason?: string;
    }[];
  } | null;
}

export interface Session {
  id: string;
  username: string;
  device_name: string;
  device_id: string;
  created_at: string;
  expires_at: string;
  last_seen_at: string;
  revoked_at: string | null;
  revoke_reason: string | null;
}

export interface AuditEvent {
  id: string;
  occurred_at: string;
  actor_username: string;
  actor_kind: string;
  device_id: string | null;
  action: string;
  entity_type: string;
  entity_id: string | null;
  success: boolean;
  before_state: unknown;
  after_state: unknown;
}

export interface AuditPage {
  items: AuditEvent[];
  total: number;
  page: number;
  page_size: number;
}
