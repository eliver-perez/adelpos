import { api } from './api';
import type { InventoryExecute, InventoryWarning, Page, UnitReference } from './inventory';
import type { ItemReference, WarehouseReference } from './inventoryRecipes';

// Contratos de la fase C de inventario (backend/internal/inventory, etapa 8): proveedores, órdenes
// de compra, recepciones de mercancía, devoluciones a proveedor y vínculos opcionales con gastos de
// caja. Go es la autoridad: cantidades e importes viajan como cadenas decimales y la interfaz los
// muestra tal como llegan (sin sumar, convertir, prorratear ni redondear).
//   - Importes de compra (unit_price, gross, discount, subtotal, tax, total, value…): los ve quien
//     tiene cualquier permiso de compras; siempre con dos decimales («850.00»), salvo el precio
//     unitario sin IVA, que admite hasta seis («33.333333»).
//   - Costeo de inventario (inventory_value, unit_cost, outflow_value, cost_adjustment, unabsorbed):
//     solo con inventory.costs.read; si falta, llegan en null y costs_visible es false. Los
//     *_display son la versión para mostrar (valores a centavos y costos a cuatro decimales).
//   - Cantidades: quantity está en la presentación o unidad capturada (capture_label) y
//     base_quantity en la unidad base del insumo.
// Toda escritura lleva operation_id (lo agrega useRestaurantMutation); proveedores, insumos de
// proveedor y órdenes llevan además revision (409 REVISION_CONFLICT). Una recepción, una devolución
// o un vínculo repetidos con el mismo operation_id devuelven la misma respuesta sin duplicar nada.

export type PurchaseOrderState =
  'draft' | 'issued' | 'partially_received' | 'received' | 'cancelled' | 'closed';
// Filtro de órdenes: un estado o «open» (emitidas y parcialmente recibidas).
export type PurchaseOrderStateFilter = PurchaseOrderState | 'open';
export type PurchaseTaxRate = 16 | 8 | 0;
export type CashLinkStatus = 'active' | 'expense_corrected' | 'expense_regularized' | 'unlinked';
export type CashExpenseStatus = 'recorded' | 'corrected' | 'regularized';
export type CashExpenseUnavailableReason =
  'not_expense' | 'currency' | 'corrected' | 'fully_linked';
export type CashShiftState = 'open' | 'closing' | 'closed';

// Códigos de rechazo propios de compras (además de INVENTORY_VALIDATION, PERMISSION_DENIED,
// NOT_FOUND, REVISION_CONFLICT e IDEMPOTENCY_CONFLICT).
export type PurchasingErrorCode =
  | 'DUPLICATE_NAME'
  | 'SUPPLIER_INACTIVE'
  | 'ITEM_INACTIVE'
  | 'PRESENTATION_INACTIVE'
  | 'WAREHOUSE_INACTIVE'
  | 'ORDER_NOT_EDITABLE'
  | 'ORDER_NOT_DRAFT'
  | 'ORDER_EMPTY'
  | 'ORDER_NOT_CANCELLABLE'
  | 'ORDER_NOT_CLOSABLE'
  | 'ORDER_NOT_RECEIVABLE'
  | 'RECEIPT_EXCEEDS_PENDING'
  | 'RETURN_EXCEEDS_RECEIVED'
  | 'CASH_OPERATION_NOT_EXPENSE'
  | 'CASH_EXPENSE_CURRENCY'
  | 'CASH_EXPENSE_CORRECTED'
  | 'CASH_EXPENSE_OVERLINKED'
  | 'RECEIPT_OVERLINKED'
  | 'CASH_LINK_EXISTS'
  | 'CASH_LINK_INACTIVE';

export interface SupplierReference {
  id: string;
  trade_name: string;
  active: boolean;
}

export interface PresentationReference {
  id: string;
  name: string;
  base_quantity: string;
  active: boolean;
}

// Proveedores.

export interface Supplier {
  id: string;
  trade_name: string;
  legal_name: string;
  // RFC en mayúsculas o null; no es único.
  rfc: string | null;
  phone: string;
  email: string;
  address: string;
  contact: string;
  notes: string;
  active: boolean;
  item_count: number;
  // Con compras registradas: nunca se borra, solo se desactiva.
  has_purchases: boolean;
  last_receipt_at: string | null;
  revision: number;
  created_at: string;
  updated_at: string;
  updated_by_name: string | null;
}

export interface SupplierItem {
  item: ItemReference;
  presentation: PresentationReference | null;
  supplier_code: string;
  preferred: boolean;
  // Último precio recibido, sin IVA y antes del descuento, por last_price_unit_label.
  last_unit_price: string | null;
  last_price_unit_label: string | null;
  last_price_at: string | null;
  last_receipt_folio: number | null;
  revision: number;
  updated_at: string;
}

export interface SupplierHistory {
  order_count: number;
  receipt_count: number;
  return_count: number;
  receipts_total: string;
  returns_value: string;
  // Las 20 más recientes de cada tipo.
  orders: PurchaseOrderSummary[];
  receipts: ReceiptSummary[];
  returns: SupplierReturnSummary[];
}

export interface SupplierDetail extends Supplier {
  items: SupplierItem[];
  history: SupplierHistory;
}

export interface SupplierQuery {
  search?: string;
  active?: 'true' | 'false';
  item_id?: string;
  page?: string;
  page_size?: string;
}

export interface SupplierInput {
  revision?: number;
  trade_name: string;
  legal_name?: string;
  rfc?: string | null;
  phone?: string;
  email?: string;
  address?: string;
  contact?: string;
  notes?: string;
  // Ausente: activo al crear y sin cambio al editar.
  active?: boolean;
}

export interface SupplierItemInput {
  // 0 si el proveedor todavía no surte el insumo.
  revision: number;
  item_id: string;
  presentation_id?: string | null;
  supplier_code?: string;
  preferred?: boolean;
}

export interface SupplierItemRemoval {
  revision: number;
  item_id: string;
}

// Renglones comunes de órdenes y recepciones.

export interface PurchaseLine {
  line_no: number;
  item: ItemReference;
  presentation: PresentationReference | null;
  // Unidad de captura (null si se capturó en una presentación).
  unit: UnitReference | null;
  capture_label: string;
  quantity: string;
  base_quantity: string;
  unit_price: string;
  gross: string;
  discount: string;
  subtotal: string;
  tax_rate: PurchaseTaxRate;
  tax: string;
  total: string;
}

// Renglón capturado de una orden o de una recepción directa: presentación o unidad (por omisión,
// la unidad base del insumo), nunca ambas.
export interface PurchaseLineInput {
  item_id: string;
  presentation_id?: string | null;
  unit_id?: string | null;
  quantity: string;
  unit_price: string;
  discount?: string;
  tax_rate: PurchaseTaxRate;
}

// Órdenes de compra.

export interface PurchaseOrderSummary {
  id: string;
  folio: number;
  supplier: SupplierReference;
  state: PurchaseOrderState;
  state_label: string;
  // AAAA-MM-DD.
  order_date: string;
  expected_warehouse: WarehouseReference;
  subtotal: string;
  discount: string;
  tax: string;
  total: string;
  line_count: number;
  receipt_count: number;
  created_by_name: string;
  issued_at: string | null;
  revision: number;
  created_at: string;
  updated_at: string;
}

export interface PurchaseOrderLine extends PurchaseLine {
  received_quantity: string;
  pending_quantity: string;
  // Lo recibido de más con allow_excess.
  excess_quantity: string;
  received_base_quantity: string;
  pending_base_quantity: string;
  fully_received: boolean;
}

export interface PurchaseOrder extends PurchaseOrderSummary {
  notes: string;
  updated_by_name: string;
  issued_by_name: string | null;
  cancelled_at: string | null;
  cancelled_by_name: string | null;
  cancel_reason: string | null;
  closed_at: string | null;
  closed_by_name: string | null;
  close_reason: string | null;
  // Acciones que admite el estado (los permisos los valida Go).
  editable: boolean;
  issuable: boolean;
  cancellable: boolean;
  closable: boolean;
  receivable: boolean;
  lines: PurchaseOrderLine[];
  receipts: ReceiptSummary[];
}

export interface PurchaseOrderQuery {
  search?: string;
  state?: PurchaseOrderStateFilter;
  supplier_id?: string;
  from?: string;
  until?: string;
  page?: string;
  page_size?: string;
}

export interface PurchaseOrderInput {
  revision?: number;
  supplier_id: string;
  // AAAA-MM-DD; vacía es hoy. No puede ser futura.
  order_date?: string;
  // Vacío es el almacén predeterminado.
  expected_warehouse_id?: string | null;
  notes?: string;
  // Un borrador puede no tener renglones; emitirlo exige al menos uno (hasta 200).
  lines: PurchaseLineInput[];
}

export interface PurchaseOrderTransitionInput {
  revision: number;
  // Obligatorio (5–500) para cancelar y cerrar; vacío para emitir.
  reason?: string;
}

// Recepciones.

export interface ReceiptSummary {
  id: string;
  folio: number;
  supplier: SupplierReference;
  purchase_order_id: string | null;
  purchase_order_folio: number | null;
  direct: boolean;
  warehouse: WarehouseReference;
  occurred_at: string;
  received_by_name: string;
  supplier_reference: string;
  // Copiado de la configuración al recibir: el valor de inventario es el total con IVA o el subtotal.
  tax_in_cost: boolean;
  subtotal: string;
  discount: string;
  tax: string;
  total: string;
  line_count: number;
  excess: boolean;
  linked_amount: string;
  linkable_amount: string;
  // Algún gasto vinculado se corrigió o regularizó después del vínculo.
  cash_link_attention: boolean;
  returned_value: string;
  costs_visible: boolean;
  inventory_value: string | null;
  inventory_value_display: string | null;
  created_at: string;
}

export interface ReceiptLine extends PurchaseLine {
  order_line_no: number | null;
  // Con orden: solicitado, recibido por recepciones anteriores y pendiente después de esta.
  ordered_quantity: string | null;
  received_before_quantity: string | null;
  pending_after_quantity: string | null;
  returned_quantity: string;
  returnable_quantity: string;
  returned_base_quantity: string;
  returnable_base_quantity: string;
  movement_id: string;
  stock_after: string;
  costs_visible: boolean;
  inventory_value: string | null;
  inventory_value_display: string | null;
  // Costo por unidad base.
  unit_cost: string | null;
  unit_cost_display: string | null;
}

export interface Receipt extends ReceiptSummary {
  notes: string;
  lines: ReceiptLine[];
  returns: SupplierReturnSummary[];
  cash_links: CashLink[];
  // Solo en la respuesta del registro: RECEIPT_EXCESS por renglón recibido de más.
  warnings: InventoryWarning[];
}

export interface ReceiptQuery {
  search?: string;
  supplier_id?: string;
  purchase_order_id?: string;
  warehouse_id?: string;
  direct?: 'true' | 'false';
  // Días de negocio AAAA-MM-DD en la zona del establecimiento.
  from?: string;
  until?: string;
  page?: string;
  page_size?: string;
}

// Renglón recibido de una orden: order_line_no y quantity en la presentación o unidad de la orden;
// precio, descuento y tasa opcionales (por omisión, los de la orden; el descuento prorrateado por
// lo recibido).
export interface OrderReceiptLineInput {
  order_line_no: number;
  quantity: string;
  unit_price?: string;
  discount?: string;
  tax_rate?: PurchaseTaxRate;
}

export interface OrderReceiptInput {
  purchase_order_id: string;
  // Por omisión, el almacén previsto de la orden.
  warehouse_id?: string | null;
  // ISO 8601, no futura; por omisión, ahora.
  occurred_at?: string;
  supplier_reference?: string;
  notes?: string;
  // Recibir más de lo pendiente exige confirmarlo (409 RECEIPT_EXCEEDS_PENDING sin él).
  allow_excess?: boolean;
  lines: OrderReceiptLineInput[];
}

// Recepción directa (sin orden): exige además inventory.purchases.create y un proveedor activo.
export interface DirectReceiptInput {
  supplier_id: string;
  warehouse_id?: string | null;
  occurred_at?: string;
  supplier_reference?: string;
  notes?: string;
  lines: PurchaseLineInput[];
}

// Devoluciones a proveedor.

export interface SupplierReturnSummary {
  id: string;
  folio: number;
  receipt_id: string;
  receipt_folio: number;
  supplier: SupplierReference;
  warehouse: WarehouseReference;
  occurred_at: string;
  reason: string;
  actor_name: string;
  // Valor asociado a costo original, a centavos.
  value: string;
  line_count: number;
  costs_visible: boolean;
  unabsorbed: string | null;
  unabsorbed_display: string | null;
  value_exact: string | null;
  created_at: string;
  receipt_supplier_reference: string;
}

export interface SupplierReturnLine {
  line_no: number;
  receipt_line_no: number;
  item: ItemReference;
  capture_label: string;
  quantity: string;
  base_quantity: string;
  value: string;
  movement_id: string;
  stock_after: string;
  costs_visible: boolean;
  original_unit_cost: string | null;
  original_unit_cost_display: string | null;
  outflow_value: string | null;
  outflow_value_display: string | null;
  cost_adjustment: string | null;
  cost_adjustment_display: string | null;
  unabsorbed: string | null;
  unabsorbed_display: string | null;
  cost_adjustment_movement_id: string | null;
}

export interface SupplierReturn extends SupplierReturnSummary {
  lines: SupplierReturnLine[];
  // Solo en la respuesta del registro: NEGATIVE_STOCK si la existencia quedó negativa.
  warnings: InventoryWarning[];
}

export interface SupplierReturnQuery {
  receipt_id?: string;
  supplier_id?: string;
  from?: string;
  until?: string;
  page?: string;
  page_size?: string;
}

export interface SupplierReturnInput {
  receipt_id: string;
  // Por omisión, el almacén de la recepción.
  warehouse_id?: string | null;
  occurred_at?: string;
  // 5–500 caracteres.
  reason: string;
  // quantity en la presentación o unidad del renglón recibido; nunca más de returnable_quantity.
  lines: { receipt_line_no: number; quantity: string }[];
}

// Gastos de caja (solo lectura) y vínculos.

export interface CashExpense {
  id: string;
  shift_id: string;
  register_id: string;
  register_name: string;
  shift_state: CashShiftState;
  shift_business_date: string | null;
  currency: 'MXN' | 'USD';
  amount: string;
  concept: string;
  category_name: string | null;
  reference: string;
  created_at: string;
  actor_name: string;
  status: CashExpenseStatus;
  status_label: string;
  corrected_at: string | null;
  linked_amount: string;
  available_amount: string;
  // Lo vinculado con la recepción indicada en receipt_id (si se indicó).
  linked_to_receipt: string | null;
  linkable: boolean;
  unavailable_reason: CashExpenseUnavailableReason | null;
  unavailable_label: string | null;
}

export interface CashLink {
  id: string;
  receipt_id: string;
  receipt_folio: number;
  cash_operation_id: string;
  amount: string;
  notes: string;
  linked_at: string;
  linked_by_name: string;
  status: CashLinkStatus;
  status_label: string;
  active: boolean;
  unlinked_at: string | null;
  unlinked_by_name: string | null;
  unlink_reason: string | null;
  // Los datos del gasto solo llegan si la sesión puede leerlo con las reglas de caja.
  expense_visible: boolean;
  expense: CashExpense | null;
}

export interface CashExpenseQuery {
  // Días de negocio del registro del gasto, AAAA-MM-DD.
  from?: string;
  until?: string;
  min_amount?: string;
  max_amount?: string;
  register_id?: string;
  receipt_id?: string;
  page?: string;
  page_size?: string;
}

export interface CashLinkInput {
  cash_operation_id: string;
  amount: string;
  notes?: string;
}

export interface CashUnlinkInput {
  // 5–500 caracteres.
  reason: string;
}

// Lecturas.

export function listSuppliers(query: SupplierQuery = {}) {
  return api<Page<Supplier>>('inventory_suppliers', { query: compactQuery(query) });
}

export function readSupplier(id: string) {
  return api<SupplierDetail>('inventory_supplier', { id });
}

export function listPurchaseOrders(query: PurchaseOrderQuery = {}) {
  return api<Page<PurchaseOrderSummary>>('inventory_purchase_orders', {
    query: compactQuery(query),
  });
}

export function readPurchaseOrder(id: string) {
  return api<PurchaseOrder>('inventory_purchase_order', { id });
}

export function listReceipts(query: ReceiptQuery = {}) {
  return api<Page<ReceiptSummary> & { costs_visible: boolean }>('inventory_receipts', {
    query: compactQuery(query),
  });
}

export function readReceipt(id: string) {
  return api<Receipt>('inventory_receipt', { id });
}

export function listSupplierReturns(query: SupplierReturnQuery = {}) {
  return api<Page<SupplierReturnSummary> & { costs_visible: boolean }>(
    'inventory_supplier_returns',
    { query: compactQuery(query) },
  );
}

export function readSupplierReturn(id: string) {
  return api<SupplierReturn>('inventory_supplier_return', { id });
}

export function listCashExpenses(query: CashExpenseQuery = {}) {
  return api<Page<CashExpense>>('inventory_cash_expenses', { query: compactQuery(query) });
}

// Escrituras: reciben execute de useRestaurantMutation (operation_id y reintento con el mismo
// identificador).

export function createSupplier(execute: InventoryExecute, input: SupplierInput) {
  return execute<SupplierDetail>('inventory_create_supplier', { body: { ...input } });
}

export function saveSupplier(execute: InventoryExecute, id: string, input: SupplierInput) {
  return execute<SupplierDetail>('inventory_save_supplier', { id, body: { ...input } });
}

export function saveSupplierItem(
  execute: InventoryExecute,
  supplierID: string,
  input: SupplierItemInput,
) {
  return execute<SupplierDetail>('inventory_save_supplier_item', {
    id: supplierID,
    body: { ...input },
  });
}

export function removeSupplierItem(
  execute: InventoryExecute,
  supplierID: string,
  input: SupplierItemRemoval,
) {
  return execute<SupplierDetail>('inventory_remove_supplier_item', {
    id: supplierID,
    body: { ...input },
  });
}

export function createPurchaseOrder(execute: InventoryExecute, input: PurchaseOrderInput) {
  return execute<PurchaseOrder>('inventory_create_purchase_order', { body: { ...input } });
}

export function savePurchaseOrder(
  execute: InventoryExecute,
  id: string,
  input: PurchaseOrderInput,
) {
  return execute<PurchaseOrder>('inventory_save_purchase_order', { id, body: { ...input } });
}

export function issuePurchaseOrder(
  execute: InventoryExecute,
  id: string,
  input: PurchaseOrderTransitionInput,
) {
  return execute<PurchaseOrder>('inventory_issue_purchase_order', { id, body: { ...input } });
}

export function cancelPurchaseOrder(
  execute: InventoryExecute,
  id: string,
  input: PurchaseOrderTransitionInput,
) {
  return execute<PurchaseOrder>('inventory_cancel_purchase_order', { id, body: { ...input } });
}

export function closePurchaseOrder(
  execute: InventoryExecute,
  id: string,
  input: PurchaseOrderTransitionInput,
) {
  return execute<PurchaseOrder>('inventory_close_purchase_order', { id, body: { ...input } });
}

export function recordReceipt(
  execute: InventoryExecute,
  input: OrderReceiptInput | DirectReceiptInput,
) {
  return execute<Receipt>('inventory_record_receipt', { body: { ...input } });
}

export function recordSupplierReturn(execute: InventoryExecute, input: SupplierReturnInput) {
  return execute<SupplierReturn>('inventory_record_supplier_return', { body: { ...input } });
}

export function linkCashExpense(
  execute: InventoryExecute,
  receiptID: string,
  input: CashLinkInput,
) {
  return execute<Receipt>('inventory_link_cash_expense', { id: receiptID, body: { ...input } });
}

export function unlinkCashExpense(
  execute: InventoryExecute,
  linkID: string,
  input: CashUnlinkInput,
) {
  return execute<Receipt>('inventory_unlink_cash_expense', { id: linkID, body: { ...input } });
}

// compactQuery quita los filtros vacíos: Go y el puente nativo rechazan parámetros desconocidos.
function compactQuery(query: object): Record<string, string> {
  const result: Record<string, string> = {};
  for (const [key, value] of Object.entries(query)) {
    if (typeof value === 'string' && value !== '') {
      result[key] = value;
    }
  }
  return result;
}
