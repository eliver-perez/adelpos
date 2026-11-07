import { invoke, isTauri } from '@tauri-apps/api/core';

export type Operation =
  | 'setup_status'
  | 'setup'
  | 'enroll'
  | 'enroll_local'
  | 'login'
  | 'logout'
  | 'me'
  | 'change_password'
  | 'users'
  | 'create_user'
  | 'update_user'
  | 'user_permissions'
  | 'save_user_permissions'
  | 'user_preferences'
  | 'save_user_preferences'
  | 'roles'
  | 'create_role'
  | 'update_role'
  | 'permissions'
  | 'settings'
  | 'save_settings'
  | 'registers'
  | 'create_register'
  | 'update_register'
  | 'devices'
  | 'invite_device'
  | 'update_device'
  | 'revoke_device'
  | 'sessions'
  | 'revoke_session'
  | 'audit'
  | 'preferences'
  | 'save_preferences'
  | 'save_terminal_preferences'
  | 'catalog_categories'
  | 'catalog_create_categories'
  | 'catalog_get_categories'
  | 'catalog_save_categories'
  | 'catalog_products'
  | 'catalog_create_products'
  | 'catalog_get_products'
  | 'catalog_save_products'
  | 'catalog_ingredients'
  | 'catalog_create_ingredients'
  | 'catalog_get_ingredients'
  | 'catalog_save_ingredients'
  | 'catalog_complements'
  | 'catalog_create_complements'
  | 'catalog_get_complements'
  | 'catalog_save_complements'
  | 'catalog_portions'
  | 'catalog_create_portions'
  | 'catalog_get_portions'
  | 'catalog_save_portions'
  | 'catalog_groups'
  | 'catalog_create_groups'
  | 'catalog_get_groups'
  | 'catalog_save_groups'
  | 'catalog_price_lists'
  | 'catalog_create_price_lists'
  | 'catalog_get_price_lists'
  | 'catalog_save_price_lists'
  | 'catalog_promotions'
  | 'catalog_create_promotions'
  | 'catalog_get_promotions'
  | 'catalog_save_promotions'
  | 'catalog_lookups'
  | 'catalog_preview'
  | 'catalog_upload_image'
  | 'catalog_reorder_categories'
  | 'catalog_image'
  | 'catalog_configuration'
  | 'catalog_product_state'
  | 'catalog_product_availability'
  | 'catalog_duplicate_product';

export class APIError extends Error {
  constructor(
    public code: string,
    message: string,
    public status = 0,
  ) {
    super(message);
    this.name = 'APIError';
  }
}

export interface LocalAuthentication {
  enrolled: boolean;
  has_session: boolean;
  local_setup_available: boolean;
  local_enrollment_available: boolean;
}

export async function localAuthentication(): Promise<LocalAuthentication> {
  if (!isTauri()) {
    throw new APIError(
      'DESKTOP_REQUIRED',
      'Abre la aplicación de escritorio para acceder de forma segura.',
    );
  }
  try {
    return await invoke<LocalAuthentication>('authentication_state');
  } catch (error) {
    throw new APIError(
      'SECURE_STORAGE_UNAVAILABLE',
      typeof error === 'string' ? error : 'El almacén seguro no está disponible.',
    );
  }
}

export async function api<T>(
  operation: Operation,
  options: {
    id?: string;
    body?: unknown;
    query?: Record<string, string>;
  } = {},
): Promise<T> {
  if (!isTauri()) {
    throw new APIError('DESKTOP_REQUIRED', 'Esta operación requiere la aplicación Tauri.');
  }
  let response: { status: number; body: unknown };
  try {
    response = await invoke('api_request', {
      operation,
      resourceId: options.id ?? null,
      body: options.body ?? null,
      query: options.query ?? null,
    });
  } catch (error) {
    const message = typeof error === 'string' ? error : 'No se pudo comunicar con el servidor.';
    throw new APIError('CONNECTION_ERROR', message);
  }
  if (response.status >= 400) {
    const body = response.body as { error?: { code?: string; message?: string } };
    const error = new APIError(
      body.error?.code ?? 'API_ERROR',
      body.error?.message ?? 'No se pudo completar la operación.',
      response.status,
    );
    const invalidatesSession =
      error.code.startsWith('SESSION_') || error.code.startsWith('DEVICE_');
    if (response.status === 401 && invalidatesSession) {
      window.dispatchEvent(new CustomEvent('session-invalid', { detail: error }));
    }
    throw error;
  }
  return response.body as T;
}
