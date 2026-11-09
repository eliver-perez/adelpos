import type { Customization, PreviewLine, ProductBreakdown, Quote } from './catalog';
import type { DeliveryState, FinanceBrief, PaymentState } from './payments';

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
  // Estado calculado por Go (etapa 7.1); con varias cuentas, el más avanzado.
  state: TableState;
  // Apertura de la cuenta abierta más antigua y segundos transcurridos según el servidor.
  occupied_since: string | null;
  occupied_seconds: number;
  // Suma de los saldos de las cuentas abiertas, calculada por Go.
  balance: string;
}

export type TableState =
  'free' | 'occupied' | 'awaiting_payment' | 'partial_payment' | 'out_of_service';

// Nombre de cada estado de mesa: el mismo texto en el mapa, su leyenda y el panel de la mesa.
export const tableStateLabels: Record<TableState, string> = {
  free: 'Libre',
  occupied: 'Ocupada',
  awaiting_payment: 'Esperando pago',
  partial_payment: 'Pago parcial',
  out_of_service: 'Fuera de servicio',
};

// Cuenta abierta que otra terminal creó en la mesa (detalle del rechazo TABLE_HAS_ACCOUNT).
export interface TableAccountRef {
  id: string;
  folio: number;
  name: string;
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
  state: 'open' | 'closed' | 'merged' | 'voided';
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
  // Pago y entrega se derivan en Go de los registros financieros reales; son independientes de
  // la preparación.
  payment_state: PaymentState;
  delivery_state: DeliveryState;
  finance: FinanceBrief;
  confirmed_quantity: number;
  draft_quantity: number;
  totals: Totals;
  // Go la marca liberable cuando está abierta y vacía en sentido estricto (etapa 7.1).
  releasable: boolean;
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

// Personalización en curso en el panel central de la captura. Para una partida nueva o del
// borrador, la cotización tentativa se calcula sobre el borrador local vigente; para modificar una
// partida confirmada se conserva la revisión en la que se abrió, como exige `restaurant_edit`.
export interface EditorState {
  accountID: string;
  mode: Account['mode'];
  tableID: string | null;
  kind: 'new' | 'draft' | 'confirmed';
  revision: number;
  localID: string;
  draftID: string | null;
  confirmedVersionID: string | null;
  line: PreviewLine;
  // Selección al abrir: «Volver» solo pide confirmación si hubo cambios.
  initialLine: PreviewLine;
  reason: string;
  // El listado de captura informó que el producto base no tiene precio en la lista de la cuenta.
  basePriceUnavailable: boolean;
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

// Cuenta pendiente (etapa 7.1): la captura de una mesa sin cuenta trabaja con un identificador
// local; la cuenta se crea en Go con el primer producto guardado. Este prefijo nunca coincide con
// un UUID, por lo que el puente nativo rechazaría cualquier consulta que lo usara por error.
export const pendingAccountPrefix = 'pendiente:';

export function isPendingAccount(account: Pick<Account, 'id'> | null | undefined): boolean {
  return !!account && account.id.startsWith(pendingAccountPrefix);
}

// Cuenta mostrada mientras la mesa aún no tiene cuenta: abierta, sin productos ni importes. Usa la
// lista de precios predeterminada (la misma que Go asignará al crearla) para los precios del
// catálogo.
export function pendingAccountBill({
  localID,
  table,
  priceListID,
  createdAt,
  timezone,
}: {
  localID: string;
  table: Pick<RestaurantTable, 'id' | 'name'>;
  priceListID: string;
  createdAt: string;
  timezone: string;
}): Bill {
  const zero: Totals = { subtotal: '0.00', discount: '0.00', total: '0.00' };
  return {
    account: {
      id: localID,
      folio: 0,
      name: table.name,
      mode: 'dine_in',
      state: 'open',
      table_id: table.id,
      table_name: table.name,
      price_list_id: priceListID,
      customer_name: '',
      phone: '',
      origin: '',
      notes: '',
      scheduled_at: null,
      scheduled_local: '',
      schedule_kind: null,
      assigned_user_id: '',
      assigned_name: '',
      created_at: createdAt,
      updated_at: createdAt,
      revision: 0,
      merged_into_id: null,
      elapsed_seconds: 0,
      urgency: 'normal',
      preparation_state: 'without_items',
      payment_state: 'not_recorded',
      delivery_state: 'not_delivered',
      finance: {
        total: '0.00',
        paid: '0.00',
        balance: '0.00',
        tips: '0.00',
        payment_state: 'not_recorded',
        delivery_state: 'not_delivered',
      },
      confirmed_quantity: 0,
      draft_quantity: 0,
      totals: zero,
      releasable: false,
    },
    draft: [],
    lines: [],
    server_time: createdAt,
    timezone,
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

// Referencias neutras (etapa 6.2): Go asigna a una cuenta abierta o editada sin nombre el nombre
// de su mesa («Cuenta» sin mesa) o «Pedido para llevar». También se tratan como neutras las que
// proponía el formulario anterior («Cuenta», «Para llevar»). Una cuenta con referencia neutra se
// considera «sin nombre»: la interfaz la identifica por su folio y no muestra la referencia como
// nombre capturado.
const neutralAccountNames = ['Cuenta', 'Pedido para llevar', 'Para llevar'];

export function accountCustomName(account: Pick<Account, 'name' | 'table_name'>): string {
  const name = account.name.trim();
  if (!name || neutralAccountNames.includes(name) || name === account.table_name) {
    return '';
  }
  return name;
}

// Nombre con que se identifica a quien recoge un pedido para llevar: el cliente o, en pedidos
// anteriores, el nombre capturado de la cuenta. Vacío si el pedido no tiene nombre.
export function takeawayCustomerName(
  account: Pick<Account, 'name' | 'table_name' | 'customer_name'>,
): string {
  return account.customer_name.trim() || accountCustomName(account);
}

// Identificación corta de una cuenta: «Cuenta #folio» o «Pedido #folio», con el nombre capturado
// si lo tiene.
export function accountLabel(account: Account): string {
  if (isPendingAccount(account)) {
    return 'Cuenta nueva';
  }
  if (account.mode === 'takeaway') {
    const customer = takeawayCustomerName(account);
    return customer ? `Pedido #${account.folio} · ${customer}` : `Pedido #${account.folio}`;
  }
  const custom = accountCustomName(account);
  return custom ? `Cuenta #${account.folio} · ${custom}` : `Cuenta #${account.folio}`;
}

// Orígenes sugeridos de un pedido para llevar (todos son la modalidad takeaway). Un origen
// distinto capturado antes se conserva como «Otro origen».
export const takeawayOrigins = ['Para llevar', 'Mostrador', 'Teléfono', 'Ventanilla'];

// Origen con que se abre un pedido nuevo desde «Nuevo pedido».
export const defaultTakeawayOrigin = 'Para llevar';

export const eventLabels: Record<string, string> = {
  'account.opened': 'Cuenta abierta',
  'account.updated': 'Datos actualizados',
  'account.move': 'Cambio de mesa',
  'account.assign': 'Cambio de responsable',
  'account.void': 'Cuenta vacía anulada',
  'account.released': 'Cuenta vacía liberada',
  'account.split': 'División de cuenta',
  'account.merged': 'Combinación de cuentas',
  'items.transferred': 'Transferencia de unidades',
  'draft.saved': 'Borrador guardado',
  'order.confirmed': 'Pedido confirmado · Pendiente de envío a cocina',
  'item.modified': 'Producto modificado',
  'item.cancelled': 'Producto cancelado',
};
