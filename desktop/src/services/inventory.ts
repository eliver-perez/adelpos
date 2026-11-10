import { api } from './api';
import type { Operation } from './api';

// Contratos del módulo de inventario de Go (backend/internal/inventory, etapa 8 fase A). Go es la
// autoridad de cantidades, existencias y costos: las cantidades y el dinero viajan como cadenas
// decimales y la interfaz los muestra tal como llegan, sin sumar, restar, convertir ni redondear.
//   - Cantidades: hasta 6 decimales en la unidad base del insumo, sin ceros finales («20», «0.25»).
//   - Valores y costos exactos (value, unit_cost, average_cost…) y su versión para mostrar
//     (*_display): valores a centavos y costos unitarios a cuatro decimales.
//   - Los campos de costo llegan en null cuando la sesión no tiene inventory.costs.read
//     (costs_visible: false) y la valuación total cuando no tiene inventory.valuation.read.
// Toda escritura lleva operation_id (lo agrega useRestaurantMutation) y las entidades
// administrables, además, su revision (409 REVISION_CONFLICT si otra terminal la cambió).

export type Dimension = 'mass' | 'volume' | 'count' | 'portion';
export type ItemKind = 'purchased' | 'prepared';
export type CountFrequency = 'none' | 'daily' | 'weekly' | 'monthly' | 'custom';
export type StockState = 'negative' | 'zero' | 'positive' | 'below_minimum' | 'reorder';
export type MovementKind =
  | 'initial'
  | 'receipt'
  | 'consumption'
  | 'consumption_reversal'
  | 'waste'
  | 'adjustment_in'
  | 'adjustment_out'
  | 'transfer_out'
  | 'transfer_in'
  | 'supplier_return'
  | 'production_in'
  | 'production_out'
  | 'count_in'
  | 'count_out'
  | 'cost_adjustment';
export type CostBasis =
  | 'average'
  | 'reference'
  | 'configured'
  | 'pending'
  | 'document'
  | 'original'
  | 'provisional'
  | 'difference';

// Operaciones de escritura de inventario: la clave de pendientes permite que un resultado incierto
// sobreviva a la navegación entre pestañas (PendingOperationsProvider).
export const inventoryPendingKey = 'inventory';

export type InventoryExecute = <T>(
  operation: Operation,
  options: { id?: string; body?: Record<string, unknown> },
) => Promise<T>;

export interface Page<T> {
  items: T[];
  total: number;
  page: number;
  page_size: number;
}

export interface UnitReference {
  id: string;
  code: string;
  name: string;
  dimension: Dimension;
}

export interface InventoryUnit {
  id: string;
  code: string;
  name: string;
  dimension: Dimension;
  dimension_label: string;
  // Equivalencia con la unidad base de su dimensión (g, ml, pieza o porción = 1).
  factor: string;
  // Las unidades del sistema (g, kg, ml, l, pza, porción) no se modifican.
  system: boolean;
  active: boolean;
  // Es la unidad base de algún insumo: su equivalencia ya no cambia.
  in_use: boolean;
  revision: number;
  updated_at: string;
}

export interface UnitInput {
  revision?: number;
  // Abreviatura (solo al crear; no cambia después).
  code?: string;
  name: string;
  // Dimensión (solo al crear; no cambia después).
  dimension?: Dimension;
  factor: string;
  active: boolean;
}

export interface InventoryCategory {
  id: string;
  name: string;
  active: boolean;
  sort_order: number;
  item_count: number;
  revision: number;
  updated_at: string;
}

export interface CategoryInput {
  revision?: number;
  name: string;
  active: boolean;
  // Orden visual: '' al inicio o el id de la categoría anterior; Go recalcula sort_order.
  position?: { after: string };
}

export interface InventoryPresentation {
  id: string;
  item_id: string;
  name: string;
  // Equivalencia en la unidad base del insumo.
  base_quantity: string;
  barcode: string | null;
  active: boolean;
  purchase_default: boolean;
  revision: number;
  updated_at: string;
}

export interface PresentationInput {
  revision?: number;
  name: string;
  // Equivalencia capturada en unit_id (por omisión, la unidad base del insumo).
  quantity: string;
  unit_id?: string | null;
  barcode?: string | null;
  active: boolean;
  purchase_default: boolean;
}

export interface InventoryItem {
  id: string;
  code: string;
  barcode: string | null;
  name: string;
  description: string;
  category_id: string | null;
  category_name: string | null;
  base_unit: UnitReference;
  kind: ItemKind;
  kind_label: string;
  active: boolean;
  costs_visible: boolean;
  // Costo de referencia por unidad base (null sin costo configurado o sin inventory.costs.read).
  reference_cost: string | null;
  reference_cost_display: string | null;
  min_stock: string | null;
  reorder_point: string | null;
  // Con movimientos, la unidad base ya no cambia.
  has_movements: boolean;
  presentations: InventoryPresentation[];
  revision: number;
  created_at: string;
  updated_at: string;
  updated_by_name: string | null;
}

export interface ItemPage extends Page<InventoryItem> {
  costs_visible: boolean;
}

export interface ItemInput {
  revision?: number;
  code: string;
  barcode?: string | null;
  name: string;
  description?: string;
  category_id?: string | null;
  base_unit_id: string;
  kind?: ItemKind;
  active: boolean;
  // Omitir para conservar el costo vigente. Enviarlo (cadena o null) exige inventory.costs.manage.
  reference_cost?: string | null;
  min_stock?: string | null;
  reorder_point?: string | null;
}

export interface StationReference {
  id: string;
  name: string;
}

export interface Warehouse {
  id: string;
  name: string;
  description: string;
  active: boolean;
  // «Almacén general»: no se desactiva y es el de las estaciones sin asignación.
  is_default: boolean;
  // Estaciones activas que consumen hoy de este almacén.
  stations: StationReference[];
  revision: number;
  updated_at: string;
}

export interface WarehouseInput {
  revision?: number;
  name: string;
  description?: string;
  active: boolean;
}

export interface StationAssignment {
  warehouse_id: string | null;
  warehouse_name: string;
  effective_from: string;
  actor_name: string | null;
}

export interface StationWarehouse {
  station_id: string;
  station_name: string;
  station_active: boolean;
  // null: usa el almacén predeterminado (warehouse_name trae su nombre).
  warehouse_id: string | null;
  warehouse_name: string;
  uses_default: boolean;
  effective_from: string | null;
  assigned_by_name: string | null;
  // Número de asignaciones registradas (0 si nunca se asignó); se envía como revision.
  revision: number;
  history: StationAssignment[];
}

export interface StationWarehouseInput {
  revision: number;
  warehouse_id: string | null;
}

export interface StockByWarehouse {
  warehouse_id: string;
  warehouse_name: string;
  quantity: string;
  negative: boolean;
}

export interface StockRow {
  item_id: string;
  item_code: string;
  item_name: string;
  category_name: string | null;
  unit: UnitReference;
  active: boolean;
  // Existencia en el almacén filtrado (o total sin filtro) y existencia total del insumo.
  quantity: string;
  total_quantity: string;
  negative: boolean;
  min_stock: string | null;
  reorder_point: string | null;
  below_minimum: boolean;
  at_reorder_point: boolean;
  warehouses: StockByWarehouse[];
  // Solo con inventory.costs.read. value es el valor total del insumo (el costo es uno por insumo).
  average_cost: string | null;
  average_cost_display: string | null;
  value: string | null;
  value_display: string | null;
  cost_pending: boolean | null;
  updated_at: string | null;
}

export interface StockPage extends Page<StockRow> {
  warehouse: { id: string; name: string } | null;
  costs_visible: boolean;
  valuation_visible: boolean;
  // Valor total del inventario (todos los insumos), solo con inventory.valuation.read.
  total_value: string | null;
  total_value_display: string | null;
}

export interface StockQuery {
  warehouse_id?: string;
  // Identificador de categoría o 'none' para insumos sin categoría.
  category_id?: string;
  search?: string;
  state?: StockState;
  active?: 'true' | 'false';
  page?: string;
  page_size?: string;
}

export interface InventoryMovement {
  id: string;
  seq: number;
  item_id: string;
  item_code: string;
  item_name: string;
  unit: UnitReference;
  // null solo en ajustes de costo (que solo se listan con inventory.costs.read).
  warehouse_id: string | null;
  warehouse_name: string | null;
  kind: MovementKind;
  kind_label: string;
  // Con signo: entradas positivas, salidas negativas, 0 en ajustes de costo.
  quantity: string;
  stock_after: string | null;
  valuation_quantity_after: string;
  costs_visible: boolean;
  value: string | null;
  value_display: string | null;
  unit_cost: string | null;
  unit_cost_display: string | null;
  cost_basis: CostBasis | null;
  cost_basis_label: string | null;
  cost_pending: boolean | null;
  valuation_value_after: string | null;
  // Hora de negocio (liberación, recepción, conteo) y hora de registro.
  occurred_at: string;
  recorded_at: string;
  source_kind: string;
  source_kind_label: string;
  source_id: string | null;
  source_line: number | null;
  restaurant_unit_id: string | null;
  related_movement_id: string | null;
  reason: string | null;
  actor_name: string | null;
  process: string | null;
  process_label: string | null;
  operation_id: string | null;
}

export interface MovementPage extends Page<InventoryMovement> {
  costs_visible: boolean;
}

export interface MovementQuery {
  item_id?: string;
  warehouse_id?: string;
  kind?: MovementKind;
  source_kind?: string;
  source_id?: string;
  // Fechas de negocio AAAA-MM-DD en la zona horaria del establecimiento (días completos).
  from?: string;
  until?: string;
  page?: string;
  page_size?: string;
}

export interface InitialBalanceLineInput {
  item_id: string;
  quantity: string;
  // Opcionales y excluyentes: unidad de la misma dimensión o presentación del insumo.
  unit_id?: string | null;
  presentation_id?: string | null;
  // Costo por unidad capturada (por caja si se captura en cajas), o el de referencia confirmado.
  unit_cost?: string | null;
  use_reference_cost?: boolean;
}

export interface InitialBalanceInput {
  warehouse_id: string;
  // RFC 3339; por omisión, la hora del servidor. No puede ser futura.
  occurred_at?: string;
  reason?: string | null;
  lines: InitialBalanceLineInput[];
}

export interface InventoryWarning {
  code: string;
  message: string;
  item_id: string | null;
}

export interface InitialBalanceLine {
  line: number;
  item_id: string;
  item_code: string;
  item_name: string;
  unit: UnitReference;
  captured_quantity: string;
  captured_unit_label: string;
  quantity: string;
  stock_after: string;
  movement_id: string;
  use_reference_cost: boolean;
  unit_cost: string | null;
  unit_cost_display: string | null;
  value: string | null;
  value_display: string | null;
}

export interface InitialBalanceResult {
  operation_id: string;
  warehouse: { id: string; name: string };
  occurred_at: string;
  reason: string | null;
  costs_visible: boolean;
  lines: InitialBalanceLine[];
  warnings: InventoryWarning[];
}

export interface InventorySettings {
  purchase_tax_in_cost: boolean;
  // HH:MM en la zona horaria del establecimiento.
  operating_day_cutoff: string;
  warn_products_without_recipe: boolean;
  count_frequency: CountFrequency;
  count_frequency_label: string;
  count_interval_days: number | null;
  revision: number;
  updated_at: string;
  updated_by_name: string | null;
}

export interface InventorySettingsInput {
  revision: number;
  purchase_tax_in_cost: boolean;
  operating_day_cutoff: string;
  warn_products_without_recipe: boolean;
  count_frequency: CountFrequency;
  // Obligatorio (1–365) solo con count_frequency 'custom'; null en los demás casos.
  count_interval_days: number | null;
}

export interface InventorySummary {
  feature: {
    code: string;
    name: string;
    enabled: boolean;
    enabled_at: string | null;
    revision: number;
  };
  active_items: number;
  inactive_items: number;
  active_warehouses: number;
  negative_stock: number;
  below_minimum: number;
  at_reorder_point: number;
  costs_visible: boolean;
  pending_cost_items: number | null;
  valuation_visible: boolean;
  total_value: string | null;
  total_value_display: string | null;
}

// Etiquetas de interfaz (Go también entrega kind_label, dimension_label y cost_basis_label).
export const dimensionLabels: Record<Dimension, string> = {
  mass: 'Masa',
  volume: 'Volumen',
  count: 'Piezas',
  portion: 'Porciones',
};

export const countFrequencyLabels: Record<CountFrequency, string> = {
  none: 'Sin frecuencia',
  daily: 'Diaria',
  weekly: 'Semanal',
  monthly: 'Mensual',
  custom: 'Cada cierto número de días',
};

// Códigos de Go que indican que el registro cambió en otra terminal y debe recargarse.
export const inventoryRefreshCodes = ['REVISION_CONFLICT', 'IDEMPOTENCY_CONFLICT'];

// Lecturas.

export function readInventorySummary() {
  return api<InventorySummary>('inventory_summary');
}

export function readInventorySettings() {
  return api<InventorySettings>('inventory_settings');
}

export function listInventoryUnits() {
  return api<InventoryUnit[]>('inventory_units');
}

export function listInventoryCategories() {
  return api<InventoryCategory[]>('inventory_categories');
}

export function listInventoryItems(
  query: {
    search?: string;
    active?: 'true' | 'false';
    category_id?: string;
    kind?: ItemKind;
    page?: string;
    page_size?: string;
  } = {},
) {
  return api<ItemPage>('inventory_items', { query: compactQuery(query) });
}

export function readInventoryItem(id: string) {
  return api<InventoryItem>('inventory_item', { id });
}

export function listWarehouses() {
  return api<Warehouse[]>('inventory_warehouses');
}

export function listStationWarehouses() {
  return api<StationWarehouse[]>('inventory_station_warehouses');
}

export function listStock(query: StockQuery = {}) {
  return api<StockPage>('inventory_stock', { query: compactQuery(query) });
}

export function listMovements(query: MovementQuery = {}) {
  return api<MovementPage>('inventory_movements', { query: compactQuery(query) });
}

// Escrituras: reciben execute de useRestaurantMutation, que agrega operation_id y conserva el
// pendiente para reintentarlo con el mismo identificador.

export function saveInventorySettings(execute: InventoryExecute, input: InventorySettingsInput) {
  return execute<InventorySettings>('inventory_save_settings', { body: { ...input } });
}

export function createInventoryUnit(execute: InventoryExecute, input: UnitInput) {
  return execute<InventoryUnit>('inventory_create_unit', { body: { ...input } });
}

export function saveInventoryUnit(execute: InventoryExecute, id: string, input: UnitInput) {
  return execute<InventoryUnit>('inventory_save_unit', { id, body: { ...input } });
}

export function createInventoryCategory(execute: InventoryExecute, input: CategoryInput) {
  return execute<InventoryCategory>('inventory_create_category', { body: { ...input } });
}

export function saveInventoryCategory(execute: InventoryExecute, id: string, input: CategoryInput) {
  return execute<InventoryCategory>('inventory_save_category', { id, body: { ...input } });
}

export function createInventoryItem(execute: InventoryExecute, input: ItemInput) {
  return execute<InventoryItem>('inventory_create_item', { body: { ...input } });
}

export function saveInventoryItem(execute: InventoryExecute, id: string, input: ItemInput) {
  return execute<InventoryItem>('inventory_save_item', { id, body: { ...input } });
}

// Devuelve el insumo completo con todas sus presentaciones.
export function createPresentation(
  execute: InventoryExecute,
  itemID: string,
  input: PresentationInput,
) {
  return execute<InventoryItem>('inventory_create_presentation', {
    id: itemID,
    body: { ...input },
  });
}

// Devuelve el insumo completo con todas sus presentaciones.
export function savePresentation(execute: InventoryExecute, id: string, input: PresentationInput) {
  return execute<InventoryItem>('inventory_save_presentation', { id, body: { ...input } });
}

export function createWarehouse(execute: InventoryExecute, input: WarehouseInput) {
  return execute<Warehouse>('inventory_create_warehouse', { body: { ...input } });
}

export function saveWarehouse(execute: InventoryExecute, id: string, input: WarehouseInput) {
  return execute<Warehouse>('inventory_save_warehouse', { id, body: { ...input } });
}

export function assignStationWarehouse(
  execute: InventoryExecute,
  stationID: string,
  input: StationWarehouseInput,
) {
  return execute<StationWarehouse>('inventory_assign_station_warehouse', {
    id: stationID,
    body: { ...input },
  });
}

export function recordInitialBalances(execute: InventoryExecute, input: InitialBalanceInput) {
  return execute<InitialBalanceResult>('inventory_record_initial_balances', {
    body: { ...input },
  });
}

// compactQuery quita los filtros vacíos: Go y el puente nativo rechazan parámetros desconocidos,
// y un filtro vacío equivale a no filtrar.
function compactQuery(query: object): Record<string, string> {
  const result: Record<string, string> = {};
  for (const [key, value] of Object.entries(query)) {
    if (typeof value === 'string' && value !== '') {
      result[key] = value;
    }
  }
  return result;
}
