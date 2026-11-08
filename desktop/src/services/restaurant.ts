import type { Customization, PreviewLine, ProductBreakdown, Quote } from './catalog';

export interface Area {
  id: string;
  name: string;
  description: string;
  sort_order: number;
  active: boolean;
  revision: number;
  table_count: number;
}

export interface RestaurantTable {
  id: string;
  area_id: string;
  name: string;
  shape: 'rectangle' | 'round';
  x: number;
  y: number;
  width: number;
  height: number;
  seats: number;
  active: boolean;
  out_of_service: boolean;
  revision: number;
  account_count: number;
  state: 'free' | 'occupied' | 'out_of_service';
}

export interface RestaurantSettings {
  warning_minutes: number;
  overdue_minutes: number;
  revision: number;
}

export interface Overview {
  areas: Area[];
  tables: RestaurantTable[];
  settings: RestaurantSettings;
  timezone: string;
}

export interface Totals {
  subtotal: string;
  discount: string;
  total: string;
}

export interface Account {
  id: string;
  folio: number;
  name: string;
  mode: 'dine_in' | 'takeaway';
  state: 'open' | 'merged' | 'voided';
  table_id: string | null;
  table_name: string | null;
  price_list_id: string;
  customer_name: string;
  phone: string;
  origin: string;
  notes: string;
  scheduled_at: string | null;
  scheduled_local: string;
  schedule_kind: 'preparation' | 'delivery' | null;
  assigned_user_id: string;
  assigned_name: string;
  created_at: string;
  updated_at: string;
  revision: number;
  merged_into_id: string | null;
  elapsed_seconds: number;
  urgency: 'normal' | 'warning' | 'overdue';
  preparation_state:
    | 'without_items'
    | 'draft'
    | 'confirmed_pending_dispatch'
    | 'pending_release'
    | 'scheduled'
    | 'released'
    | 'preparing'
    | 'ready'
    | 'change_pending'
    | 'cancelled';
  payment_state: 'not_recorded';
  delivery_state: 'not_delivered';
  confirmed_quantity: number;
  draft_quantity: number;
  totals: Totals;
}

export interface AccountInput {
  name: string;
  mode: Account['mode'];
  table_id: string | null;
  price_list_id: string | null;
  customer_name: string;
  phone: string;
  origin: string;
  notes: string;
  scheduled_local: string;
  schedule_kind: string;
  assigned_user_id: string | null;
}

export interface DraftItem extends PreviewLine {
  id: string;
}

export interface ConfirmedLine {
  preparation_state?: string;
  version_id: string;
  item_id: string;
  original_order_id: string;
  version: number;
  state: 'active' | 'cancelled';
  quantity: number;
  request: PreviewLine;
  quote_line: ProductBreakdown & {
    quantity: number;
    subtotal: string;
    discount: string;
    total: string;
  };
  totals: Totals;
  created_at: string;
}

export interface Bill {
  account: Account;
  draft: DraftItem[];
  lines: ConfirmedLine[];
  server_time: string;
  timezone: string;
}

export interface AccountPage {
  items: Account[];
  total: number;
  page: number;
  page_size: number;
  server_time: string;
  timezone: string;
  settings: RestaurantSettings;
}

export interface OrderQuote {
  account_id: string;
  revision: number;
  fingerprint: string;
  quote: Quote | null;
  combined: Totals;
  pricing_policy: string;
}

export interface HistoryEvent {
  id: string;
  sequence: number;
  account_id: string;
  related_account_id: string | null;
  kind: string;
  payload: { before: unknown; after: unknown };
  author_name: string;
  device_name: string;
  created_at: string;
  integration_state: 'pending_integration';
}

export interface HistoryPage {
  items: HistoryEvent[];
  page: number;
  total: number;
}

export interface EditorState {
  accountID: string;
  mode: Account['mode'];
  tableID: string | null;
  revision: number;
  baseDraft: DraftItem[];
  localID: string;
  draftID: string | null;
  confirmedVersionID: string | null;
  line: PreviewLine;
  reason: string;
}

export function emptyCustomization(): Customization {
  return { ingredients: [], groups: [], components: [], notes: '' };
}

export function normalizeSelection(line: PreviewLine): PreviewLine {
  const normalize = (value: Customization): Customization => ({
    ingredients: value.ingredients ?? [],
    groups: (value.groups ?? []).map((group) => ({ ...group, options: group.options ?? [] })),
    components: (value.components ?? []).map((component) => ({
      ...component,
      customization: normalize(component.customization),
    })),
    notes: value.notes ?? '',
  });
  return {
    product_id: line.product_id,
    variant_id: line.variant_id ?? null,
    quantity: line.quantity,
    customization: normalize(line.customization),
  };
}

export function serverDateTime(value: string, timezone: string) {
  return new Intl.DateTimeFormat('es-MX', {
    dateStyle: 'short',
    timeStyle: 'short',
    timeZone: timezone,
  }).format(new Date(value));
}

export function elapsedTime(seconds: number) {
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes} min`;
  return `${Math.floor(minutes / 60)} h ${minutes % 60} min`;
}

export const eventLabels: Record<string, string> = {
  'account.opened': 'Cuenta abierta',
  'account.updated': 'Datos actualizados',
  'account.move': 'Cambio de mesa',
  'account.assign': 'Cambio de responsable',
  'account.void': 'Cuenta vacía anulada',
  'account.split': 'División de cuenta',
  'account.merged': 'Combinación de cuentas',
  'items.transferred': 'Transferencia de unidades',
  'draft.saved': 'Borrador guardado',
  'order.confirmed': 'Pedido confirmado · Pendiente de envío a cocina',
  'item.modified': 'Producto modificado',
  'item.cancelled': 'Producto cancelado',
};
