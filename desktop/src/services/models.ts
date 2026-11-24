import type { TerminalHome } from '../navigation/sections';
import type { LicenseSeat, LicenseSummary, SeatUsage } from './license';

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
  // Etapa 9H: si tiene contraseña (los importados del sistema anterior llegan sin ella y sin acceso). Solo lo
  // entrega la lista de Usuarios; la sesión no lo lleva.
  has_password?: boolean;
  // Etapa A1 (fase B): PIN para los teléfonos y tabletas de meseros (nunca el PIN), su nombre corto, el color de su
  // distintivo (de la paleta de Go) y hasta cuándo está bloqueado por intentos fallidos. La sesión solo lleva has_pin.
  has_pin?: boolean;
  short_name?: string | null;
  badge_color?: string | null;
  pin_locked_until?: string | null;
  // A1-D16: el PIN quedó bloqueado por tres bloqueos seguidos y espera a que un administrador lo desbloquee.
  pin_requires_unlock?: boolean;
}

export interface Identity {
  user: User & { permissions: string[]; support: boolean; session_id: string; device_id: string };
  preferences: Preferences;
  terminal: Device;
  // Desde la etapa 9F incluye el nombre visible de la precuenta (services/documentNames.ts).
  establishment: Pick<Settings, 'name' | 'timezone' | 'currency'> & { precheck_name?: string };
  // Etapa L1 (Fase C): el estado de la licencia y de la hora de la computadora principal. Un servidor anterior no lo
  // envía.
  license?: LicenseSummary;
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
  // Etapa A1 (fase B): segundos sin uso tras los que un teléfono o tableta de meseros vuelve a la pantalla de entrada.
  idle_lock_seconds?: number;
  // Etapa A2 (parte C): impresora de precuentas de un teléfono o tableta de meseros, con su nombre y el de la terminal
  // que la tiene conectada si es la USB de una terminal. Un servidor anterior no los informa.
  precheck_printer_id?: string | null;
  precheck_printer_name?: string | null;
  precheck_printer_terminal_name?: string | null;
  // Etapa A2 (parte D): avisos de platillos listos del teléfono o tableta (activados por omisión).
  ready_alerts_enabled?: boolean;
  ready_alerts_sound?: boolean;
  ready_alerts_vibration?: boolean;
  // Etapa L1 (Fase D): cupos de la licencia que ocupa la terminal en este momento (vacío si ninguno).
  seats?: LicenseSeat[];
}

// Impresora que se puede elegir para precuentas (etapa A2, parte C): Go dice por qué no sirve (problem: inactive o
// unsupported; vacío si sirve).
export interface PrecheckPrinterOption {
  id: string;
  name: string;
  transport: string;
  terminal_name: string;
  active: boolean;
  problem: string;
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
  // Revisión de la fase B: solicitudes canceladas, rechazadas o vencidas sin aprobarse en la última ventana (indicio
  // de un intento de suplantar a una terminal), si se alcanzó el tope de solicitudes y si se puede emparejar
  // (sin identidad cargada, no).
  recent_abandoned?: number;
  limit_reached?: boolean;
  window_minutes?: number;
  pairing_available?: boolean;
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
  // Etapa L1 (Fase D): cupos en uso y límite de la licencia (con limited: true).
  seats?: SeatUsage;
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
  // Etapa A2 (parte C): la impresora de precuentas de un teléfono o tableta y las que se le pueden elegir (vacía en
  // las demás terminales). Un servidor anterior no las informa.
  precheck_printer?: PrecheckPrinterOption | null;
  precheck_printer_options?: PrecheckPrinterOption[];
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
