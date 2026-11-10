import { api } from './api';
import type {
  InventoryExecute,
  InventorySummary,
  InventoryWarning,
  MovementKind,
  Page,
  UnitReference,
} from './inventory';
import type {
  PresentationReference,
  PurchaseOrder,
  SupplierReference,
} from './inventoryPurchasing';
import type { ItemReference, WarehouseReference } from './inventoryRecipes';

// Contratos de la fase D de inventario (backend/internal/inventory, etapa 8): ajustes de
// discrepancia y mermas operativas, transferencias entre almacenes, conteos físicos, niveles por
// almacén, alertas y sugerencias de compra. Go es la autoridad: cantidades y valores viajan como
// cadenas decimales y la interfaz los muestra tal como llegan (sin sumar, convertir ni redondear).
//   - Cantidades con signo (ajustes) en la unidad base del insumo: «-1000», «200».
//   - Valores (value, value_display) solo con inventory.costs.read; si falta, llegan en null.
//   - Ninguna existencia se edita: cada documento escribe movimientos del libro. Una existencia
//     negativa nunca bloquea: llega como aviso NEGATIVE_STOCK en warnings.
// Toda escritura lleva operation_id (lo agrega useRestaurantMutation); las autorizaciones,
// rechazos, transiciones de conteos y niveles llevan además revision (409 REVISION_CONFLICT).

// Códigos de rechazo propios de la fase D (además de INVENTORY_VALIDATION, PERMISSION_DENIED,
// NOT_FOUND, REVISION_CONFLICT, IDEMPOTENCY_CONFLICT y WAREHOUSE_INACTIVE).
export type InventoryControlErrorCode =
  | 'ADJUSTMENT_NOT_PENDING'
  | 'SELF_AUTHORIZATION_FORBIDDEN'
  | 'COUNT_ITEM_IN_PROGRESS'
  | 'COUNT_EMPTY'
  | 'COUNT_NOT_OPEN'
  | 'COUNT_NOT_SUBMITTED'
  | 'COUNT_NOT_APPROVED'
  | 'COUNT_ALREADY_APPLIED'
  | 'COUNT_NOT_CANCELLABLE'
  | 'COUNT_NOTHING_COUNTED'
  | 'COUNT_CONSUMPTION_PENDING'
  | 'SUGGESTION_CHANGED';

// Avisos que no bloquean: existencia negativa y, en un conteo sin terminar, renglones sin contar,
// consumos anteriores pendientes de calcular, cancelaciones sin clasificar y ajustes por autorizar.
export type InventoryControlWarningCode =
  | 'NEGATIVE_STOCK'
  | 'COUNT_UNCOUNTED_LINES'
  | 'COUNT_PENDING_CONSUMPTION'
  | 'COUNT_PENDING_CANCELLATIONS'
  | 'COUNT_PENDING_ADJUSTMENTS';

// Ajustes y mermas.

export type AdjustmentKind = 'discrepancy' | 'waste';
export type AdjustmentState = 'pending_authorization' | 'applied' | 'rejected';
export type AdjustmentReason =
  | 'breakage'
  | 'spill'
  | 'expiry'
  | 'physical_shortage'
  | 'surplus'
  | 'entry_error'
  | 'extraordinary_use'
  | 'other';

// Etiquetas de interfaz (Go también entrega kind_label, reason_label y state_label).
export const adjustmentReasonLabels: Record<AdjustmentReason, string> = {
  breakage: 'Rotura',
  spill: 'Derrame',
  expiry: 'Caducidad',
  physical_shortage: 'Faltante físico',
  surplus: 'Sobrante',
  entry_error: 'Error de registro',
  extraordinary_use: 'Consumo extraordinario',
  other: 'Otro',
};

// Sentido que admite cada motivo (Go lo valida): salidas (cantidad negativa), entradas o ambos.
export const adjustmentReasonDirection: Record<AdjustmentReason, 'out' | 'in' | 'both'> = {
  breakage: 'out',
  spill: 'out',
  expiry: 'out',
  physical_shortage: 'out',
  surplus: 'in',
  entry_error: 'both',
  extraordinary_use: 'out',
  other: 'both',
};

// Motivos que admite una merma operativa (solo salidas).
export const wasteReasons: AdjustmentReason[] = [
  'breakage',
  'spill',
  'expiry',
  'extraordinary_use',
  'other',
];

export interface AdjustmentSummary {
  id: string;
  folio: number;
  kind: AdjustmentKind;
  kind_label: string;
  reason: AdjustmentReason;
  reason_label: string;
  // Texto del motivo (obligatorio con «Otro», de 3 a 200 caracteres).
  reason_detail: string;
  warehouse: WarehouseReference;
  state: AdjustmentState;
  state_label: string;
  notes: string;
  line_count: number;
  requested_by_name: string;
  requested_at: string;
  // Solo cuando otra persona autorizó un ajuste pendiente; un ajuste aplicado al registrarlo no
  // tiene autorizador.
  authorized_by_name: string | null;
  authorized_at: string | null;
  // Hora de negocio de sus movimientos (registro o autorización).
  applied_at: string | null;
  rejected_by_name: string | null;
  rejected_at: string | null;
  rejection_reason: string | null;
  revision: number;
  updated_at: string;
  costs_visible: boolean;
  // Valor neto de los movimientos aplicados (negativo si salió más de lo que entró).
  value: string | null;
  value_display: string | null;
}

export interface AdjustmentLine {
  line_no: number;
  item: ItemReference;
  presentation: PresentationReference | null;
  unit: UnitReference | null;
  capture_label: string;
  captured_quantity: string;
  quantity: string;
  direction: 'in' | 'out';
  // Vacíos mientras el ajuste está pendiente o si se rechazó.
  movement_id: string | null;
  movement_kind: MovementKind | null;
  movement_label: string | null;
  stock_after: string | null;
  value: string | null;
  value_display: string | null;
}

export interface Adjustment extends AdjustmentSummary {
  // La sesión puede autorizarlo o rechazarlo: pendiente, con el permiso y registrado por otra persona.
  authorizable: boolean;
  lines: AdjustmentLine[];
  warnings: InventoryWarning[];
}

export interface AdjustmentPage extends Page<AdjustmentSummary> {
  costs_visible: boolean;
}

export interface AdjustmentQuery {
  kind?: AdjustmentKind | '';
  state?: AdjustmentState | '';
  reason?: AdjustmentReason | '';
  warehouse_id?: string;
  item_id?: string;
  from?: string;
  until?: string;
  // Folio exacto.
  search?: string;
  page?: string;
  page_size?: string;
}

// Renglón capturado: cantidad (con signo «-» en los ajustes) en una presentación, en una unidad de
// la dimensión del insumo o, sin ninguna, en su unidad base.
export interface ControlLineInput {
  item_id: string;
  quantity: string;
  presentation_id?: string | null;
  unit_id?: string | null;
}

export interface AdjustmentInput {
  kind: AdjustmentKind;
  reason: AdjustmentReason;
  reason_detail?: string;
  warehouse_id: string;
  notes?: string;
  lines: ControlLineInput[];
}

export interface AdjustmentAuthorizationInput {
  revision: number;
}

export interface AdjustmentRejectionInput {
  revision: number;
  // De 5 a 500 caracteres.
  reason: string;
}

// Transferencias.

export interface TransferSummary {
  id: string;
  folio: number;
  from_warehouse: WarehouseReference;
  to_warehouse: WarehouseReference;
  occurred_at: string;
  actor_name: string;
  notes: string;
  line_count: number;
  created_at: string;
  costs_visible: boolean;
  // Valor trasladado (el costo del insumo no cambia).
  value: string | null;
  value_display: string | null;
}

export interface TransferLine {
  line_no: number;
  item: ItemReference;
  presentation: PresentationReference | null;
  unit: UnitReference | null;
  capture_label: string;
  captured_quantity: string;
  quantity: string;
  out_movement_id: string;
  in_movement_id: string;
  from_stock_after: string;
  to_stock_after: string;
  value: string | null;
  value_display: string | null;
}

export interface Transfer extends TransferSummary {
  lines: TransferLine[];
  warnings: InventoryWarning[];
}

export interface TransferPage extends Page<TransferSummary> {
  costs_visible: boolean;
}

export interface TransferQuery {
  // Origen o destino.
  warehouse_id?: string;
  item_id?: string;
  from?: string;
  until?: string;
  search?: string;
  page?: string;
  page_size?: string;
}

export interface TransferInput {
  from_warehouse_id: string;
  to_warehouse_id: string;
  notes?: string;
  lines: ControlLineInput[];
}

// Conteos físicos.

export type CountState = 'open' | 'submitted' | 'approved' | 'applied' | 'cancelled';
// Filtro de conteos: un estado o «active» (abiertos, enviados y aprobados).
export type CountStateFilter = CountState | 'active';
export type CountScope = 'all' | 'category' | 'items';
export type CountDifferenceKind = 'surplus' | 'shortage' | 'none';

export interface CountSummary {
  id: string;
  folio: number;
  warehouse: WarehouseReference;
  scope: CountScope;
  scope_label: string;
  category: { id: string; name: string } | null;
  state: CountState;
  state_label: string;
  notes: string;
  line_count: number;
  counted_lines: number;
  // Solo en un conteo aplicado y con theoretical_visible.
  difference_lines: number | null;
  created_by_name: string;
  created_at: string;
  updated_by_name: string;
  updated_at: string;
  submitted_by_name: string | null;
  submitted_at: string | null;
  approved_by_name: string | null;
  approved_at: string | null;
  applied_by_name: string | null;
  applied_at: string | null;
  // Momento de referencia aplicado: la hora máxima de los renglones contados.
  reference_at: string | null;
  cancelled_by_name: string | null;
  cancelled_at: string | null;
  cancel_reason: string | null;
  revision: number;
  // Existencia teórica y diferencias visibles (inventory.counts.approve o inventory.stock.read);
  // quien solo cuenta captura a ciegas.
  theoretical_visible: boolean;
}

export interface CountLine {
  line_no: number;
  item: ItemReference;
  category_name: string | null;
  counted: boolean;
  // En la unidad base del insumo; captured_quantity, tal como se capturó (capture_label).
  counted_quantity: string | null;
  captured_quantity: string | null;
  presentation: PresentationReference | null;
  unit: UnitReference | null;
  capture_label: string | null;
  captured_at: string | null;
  captured_by_name: string | null;
  // Con theoretical_visible: existencia teórica a la hora del renglón (calculada al consultar en un
  // conteo sin aplicar; la aplicada en uno aplicado), diferencia (contado − teórico) y existencia
  // actual del almacén.
  theoretical_quantity: string | null;
  difference: string | null;
  difference_kind: CountDifferenceKind | null;
  current_quantity: string | null;
  movement_id: string | null;
  movement_kind: MovementKind | null;
  movement_label: string | null;
  value: string | null;
  value_display: string | null;
}

export interface Count extends CountSummary {
  editable: boolean;
  submittable: boolean;
  approvable: boolean;
  applicable: boolean;
  cancellable: boolean;
  costs_visible: boolean;
  value: string | null;
  value_display: string | null;
  lines: CountLine[];
  warnings: InventoryWarning[];
}

export interface CountQuery {
  warehouse_id?: string;
  state?: CountStateFilter | '';
  from?: string;
  until?: string;
  search?: string;
  page?: string;
  page_size?: string;
}

export interface CountInput {
  warehouse_id: string;
  scope: CountScope;
  // Solo con scope «category».
  category_id?: string | null;
  // Solo con scope «items».
  item_ids?: string[];
  notes?: string;
}

export interface CountUpdateInput {
  revision: number;
  notes?: string;
  add_item_ids?: string[];
  remove_item_ids?: string[];
}

// Captura: cantidad física (cero admitido) o null para borrar la captura. Sin revisión: varias
// terminales capturan a la vez; la hora del renglón la fija Go.
export interface CountCaptureInput {
  lines: {
    item_id: string;
    counted_quantity: string | null;
    presentation_id?: string | null;
    unit_id?: string | null;
  }[];
}

export interface CountTransitionInput {
  revision: number;
}

export interface CountCancellationInput {
  revision: number;
  // De 5 a 500 caracteres.
  reason: string;
}

// Niveles por almacén.

export interface StockLevel {
  item: ItemReference;
  warehouse: WarehouseReference;
  min_stock: string | null;
  reorder_point: string | null;
  // Niveles del insumo (comparados con la existencia total).
  item_min_stock: string | null;
  item_reorder_point: string | null;
  stock_visible: boolean;
  quantity: string | null;
  below_minimum: boolean | null;
  at_reorder_point: boolean | null;
  configured: boolean;
  // 0 sin nivel configurado.
  revision: number;
  updated_at: string | null;
  updated_by_name: string | null;
}

export interface StockLevelQuery {
  warehouse_id?: string;
  item_id?: string;
  search?: string;
  page?: string;
  page_size?: string;
}

// Los dos valores vacíos (null) quitan el nivel del almacén.
export interface StockLevelInput {
  item_id: string;
  warehouse_id: string;
  revision: number;
  min_stock: string | null;
  reorder_point: string | null;
}

// Alertas.

export type AlertType =
  | 'negative_stock'
  | 'consumption_failed'
  | 'below_minimum'
  | 'cost_pending'
  | 'count_difference_pending'
  | 'cancellations_unclassified'
  | 'cash_link_expense_corrected'
  | 'reorder_point'
  | 'count_due'
  | 'purchase_order_partial'
  | 'item_without_supplier'
  | 'product_without_recipe';
export type AlertSeverity = 'critical' | 'warning' | 'info';
// Destino navegable de una alerta (id null en las que resumen una lista).
export type AlertReferenceKind =
  | 'stock'
  | 'item'
  | 'product'
  | 'count'
  | 'warehouse'
  | 'purchase_order'
  | 'receipt'
  | 'cancellations'
  | 'consumption_requests';

export interface InventoryAlert {
  type: AlertType;
  type_label: string;
  severity: AlertSeverity;
  severity_label: string;
  message: string;
  reference: { kind: AlertReferenceKind; id: string | null; label: string };
  item: ItemReference | null;
  // Null en las alertas de existencia total.
  warehouse: WarehouseReference | null;
  // Solo con inventory.stock.read.
  quantity: string | null;
  threshold: string | null;
  // Registros que resume (1 en las de un solo registro).
  count: number;
  occurred_at: string | null;
}

export interface AlertCounts {
  total: number;
  critical: number;
  warning: number;
  info: number;
  by_type: Record<AlertType, number>;
}

export interface AlertPage extends Page<InventoryAlert> {
  // Conteos de todas las alertas visibles, sin filtros.
  counts: AlertCounts;
  costs_visible: boolean;
  stock_visible: boolean;
  purchasing_visible: boolean;
  generated_at: string;
}

export interface AlertQuery {
  type?: AlertType | '';
  severity?: AlertSeverity | '';
  page?: string;
  page_size?: string;
}

// El resumen de la fase A con los conteos de alertas de la fase D.
export interface InventorySummaryWithAlerts extends InventorySummary {
  alerts: AlertCounts;
}

// Sugerencias de compra.

export interface PurchaseSuggestion {
  item: ItemReference;
  category_name: string | null;
  // Punto de reabastecimiento usado: el del insumo o la suma de los de sus almacenes.
  reorder_point: string;
  reorder_source: 'item' | 'warehouses';
  min_stock: string | null;
  // Existencia total, pendiente de órdenes emitidas, en borradores y necesidad (unidad base): con
  // inventory.stock.read o inventory.purchases.create.
  stock_visible: boolean;
  stock: string | null;
  pending_quantity: string | null;
  draft_quantity: string | null;
  need_quantity: string | null;
  below_minimum: boolean | null;
  // Proveedor preferido y último precio: con algún permiso de compras.
  purchasing_visible: boolean;
  supplier: SupplierReference | null;
  supplier_code: string | null;
  // Presentación de compra (o, sin presentación, la unidad base en unit).
  presentation: PresentationReference | null;
  unit: UnitReference | null;
  capture_label: string;
  // Redondeado hacia arriba a presentaciones completas (o unidades base enteras).
  suggested_quantity: string;
  suggested_base_quantity: string;
  last_unit_price: string | null;
  last_price_label: string | null;
  last_price_at: string | null;
}

export interface PurchaseSuggestionPage extends Page<PurchaseSuggestion> {
  stock_visible: boolean;
  purchasing_visible: boolean;
}

export interface PurchaseSuggestionQuery {
  // Proveedor preferido o «none» (sin proveedor preferido); requiere permisos de compras.
  supplier_id?: string;
  category_id?: string;
  search?: string;
  page?: string;
  page_size?: string;
}

// Crea un borrador (nunca lo emite) con las sugerencias vigentes de los insumos elegidos para un
// proveedor; 409 SUGGESTION_CHANGED si alguna ya no aplica.
export interface DraftOrderFromSuggestionsInput {
  supplier_id: string;
  item_ids: string[];
  expected_warehouse_id?: string | null;
  notes?: string;
}

// Lecturas.

export function listAdjustments(query: AdjustmentQuery = {}) {
  return api<AdjustmentPage>('inventory_adjustments', { query: compactQuery(query) });
}

export function readAdjustment(id: string) {
  return api<Adjustment>('inventory_adjustment', { id });
}

export function listTransfers(query: TransferQuery = {}) {
  return api<TransferPage>('inventory_transfers', { query: compactQuery(query) });
}

export function readTransfer(id: string) {
  return api<Transfer>('inventory_transfer', { id });
}

export function listCounts(query: CountQuery = {}) {
  return api<Page<CountSummary>>('inventory_counts', { query: compactQuery(query) });
}

export function readCount(id: string) {
  return api<Count>('inventory_count', { id });
}

export function listStockLevels(query: StockLevelQuery = {}) {
  return api<Page<StockLevel>>('inventory_stock_levels', { query: compactQuery(query) });
}

export function listAlerts(query: AlertQuery = {}) {
  return api<AlertPage>('inventory_alerts', { query: compactQuery(query) });
}

export function listPurchaseSuggestions(query: PurchaseSuggestionQuery = {}) {
  return api<PurchaseSuggestionPage>('inventory_purchase_suggestions', {
    query: compactQuery(query),
  });
}

// Escrituras: reciben execute de useRestaurantMutation (operation_id y reintento con el mismo
// identificador).

export function recordAdjustment(execute: InventoryExecute, input: AdjustmentInput) {
  return execute<Adjustment>('inventory_record_adjustment', { body: { ...input } });
}

export function authorizeAdjustment(
  execute: InventoryExecute,
  id: string,
  input: AdjustmentAuthorizationInput,
) {
  return execute<Adjustment>('inventory_authorize_adjustment', { id, body: { ...input } });
}

export function rejectAdjustment(
  execute: InventoryExecute,
  id: string,
  input: AdjustmentRejectionInput,
) {
  return execute<Adjustment>('inventory_reject_adjustment', { id, body: { ...input } });
}

export function recordTransfer(execute: InventoryExecute, input: TransferInput) {
  return execute<Transfer>('inventory_record_transfer', { body: { ...input } });
}

export function createCount(execute: InventoryExecute, input: CountInput) {
  return execute<Count>('inventory_create_count', { body: { ...input } });
}

export function saveCount(execute: InventoryExecute, id: string, input: CountUpdateInput) {
  return execute<Count>('inventory_save_count', { id, body: { ...input } });
}

export function captureCountLines(execute: InventoryExecute, id: string, input: CountCaptureInput) {
  return execute<Count>('inventory_capture_count_lines', { id, body: { ...input } });
}

export function submitCount(execute: InventoryExecute, id: string, input: CountTransitionInput) {
  return execute<Count>('inventory_submit_count', { id, body: { ...input } });
}

export function approveCount(execute: InventoryExecute, id: string, input: CountTransitionInput) {
  return execute<Count>('inventory_approve_count', { id, body: { ...input } });
}

export function applyCount(execute: InventoryExecute, id: string, input: CountTransitionInput) {
  return execute<Count>('inventory_apply_count', { id, body: { ...input } });
}

export function cancelCount(execute: InventoryExecute, id: string, input: CountCancellationInput) {
  return execute<Count>('inventory_cancel_count', { id, body: { ...input } });
}

export function saveStockLevel(execute: InventoryExecute, input: StockLevelInput) {
  return execute<StockLevel>('inventory_save_stock_level', { body: { ...input } });
}

export function draftOrderFromSuggestions(
  execute: InventoryExecute,
  input: DraftOrderFromSuggestionsInput,
) {
  return execute<PurchaseOrder>('inventory_draft_order_from_suggestions', {
    body: { ...input },
  });
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
