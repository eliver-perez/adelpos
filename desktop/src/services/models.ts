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
