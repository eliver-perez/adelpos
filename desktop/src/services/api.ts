import { invoke, isTauri } from '@tauri-apps/api/core';

export type Operation =
  | 'kitchen_outputs'
  | 'kitchen_stations'
  | 'kitchen_create_station'
  | 'kitchen_printers'
  | 'kitchen_create_printer'
  | 'kitchen_discovery'
  | 'kitchen_routes'
  | 'kitchen_targets'
  | 'kitchen_create_route'
  | 'kitchen_settings'
  | 'kitchen_save_settings'
  | 'kitchen_save_timing'
  | 'kitchen_access'
  | 'kitchen_save_access'
  | 'kitchen_board'
  | 'kitchen_releases'
  | 'kitchen_tickets'
  | 'kitchen_jobs'
  | 'kitchen_history'
  | 'kitchen_save_states'
  | 'kitchen_test_print'
  | 'kitchen_reprint'
  | 'kitchen_save_station'
  | 'kitchen_save_printer'
  | 'kitchen_save_route'
  | 'kitchen_release'
  | 'kitchen_schedule'
  | 'kitchen_retry_change'
  | 'kitchen_ticket'
  | 'kitchen_ack_ticket'
  | 'kitchen_job'
  | 'kitchen_retry_job'
  | 'kitchen_resolve_job'
  | 'restaurant_draft_preview'
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
  | 'settings_features'
  | 'save_settings_feature'
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
  | 'catalog_duplicate_product'
  | 'payments_settings'
  | 'payments_save_settings'
  | 'payments_save_tax'
  | 'payments_methods'
  | 'payments_create_method'
  | 'payments_current_shift'
  | 'payments_open_shift'
  | 'payments_shift_usd_rate'
  | 'payments_save_usd_settings'
  | 'payments_save_method'
  | 'payments_shift_movements'
  | 'payments_account'
  | 'payments_preview'
  | 'payments_pay'
  | 'payments_deliver'
  // Operaciones de fases posteriores (6C y 6D): declaradas para el puente, aún sin uso en la interfaz.
  | 'payments_discount_preview'
  | 'payments_discount'
  | 'payments_charge'
  | 'payments_void_adjustment'
  | 'payments_refund_preview'
  | 'payments_refund'
  | 'payments_correct'
  | 'payments_precheck'
  | 'payments_print_precheck'
  | 'payments_reprint'
  // Administración de caja (etapa 7, fase A): contratos para las pantallas de la fase B.
  | 'payments_shifts'
  | 'payments_cash_catalog'
  | 'payments_cash_registers'
  | 'payments_create_denomination'
  | 'payments_create_category'
  | 'payments_shift'
  | 'payments_shift_operations'
  | 'payments_shift_income'
  | 'payments_shift_withdrawal'
  | 'payments_shift_expense'
  | 'payments_shift_correction'
  | 'payments_shift_partial_cut'
  | 'payments_shift_start_closing'
  | 'payments_shift_closing'
  | 'payments_shift_save_count'
  | 'payments_shift_cancel_closing'
  | 'payments_shift_confirm_closing'
  | 'payments_shift_closure'
  | 'payments_shift_comment_closure'
  | 'payments_cash_report'
  | 'payments_save_denomination'
  | 'payments_save_category'
  // Consultas de solo lectura de la interfaz de caja (etapa 7, fase B).
  | 'payments_cash_count_preview'
  | 'payments_shift_summary'
  // Impresión de documentos de caja, cajón de dinero y tope de caja (etapa 7, fase C).
  | 'payments_cash_limit'
  | 'payments_save_cash_limit'
  | 'payments_cash_drawers'
  | 'payments_save_cash_drawer'
  | 'payments_open_drawer'
  | 'payments_cash_report_preview'
  | 'payments_print_cash_report'
  | 'payments_cash_operation_preview'
  | 'payments_print_cash_operation'
  | 'payments_reprint_cash_document'
  | 'payments_shift_documents'
  | 'restaurant_overview'
  | 'restaurant_areas'
  | 'restaurant_create_area'
  | 'restaurant_tables'
  | 'restaurant_create_table'
  | 'restaurant_save_layout'
  | 'restaurant_settings'
  | 'restaurant_save_settings'
  | 'restaurant_staff'
  | 'restaurant_accounts'
  | 'restaurant_open_account'
  | 'restaurant_save_area'
  | 'restaurant_save_table'
  | 'restaurant_account'
  | 'restaurant_save_account'
  | 'restaurant_save_draft'
  | 'restaurant_preview'
  | 'restaurant_history'
  | 'restaurant_confirm'
  | 'restaurant_edit_preview'
  | 'restaurant_edit'
  | 'restaurant_cancel'
  | 'restaurant_split'
  | 'restaurant_transfer'
  | 'restaurant_merge'
  | 'restaurant_move'
  | 'restaurant_assign'
  | 'restaurant_void';

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
