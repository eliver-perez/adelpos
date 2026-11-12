import { api } from './api';
import type {
  InventoryExecute,
  InventoryMovement,
  InventoryWarning,
  ItemKind,
  Page,
  UnitReference,
} from './inventory';

// Contratos de la fase B de inventario (backend/internal/inventory, etapa 8): recetas versionadas,
// subrecetas, porciones habituales de los ingredientes del catálogo, cancelaciones por clasificar,
// elaboraciones y diagnóstico del consumo. Go es la autoridad: cantidades y dinero viajan como
// cadenas decimales y la interfaz los muestra tal como llegan (sin sumar, convertir ni redondear).
//   - Las cantidades de captura (quantity + unit_id) se guardan también convertidas a la unidad
//     base (base_quantity); las cantidades base no tienen ceros finales («150», «0.25»).
//   - Costos teóricos y valores solo con inventory.costs.read: si falta, llegan en null y
//     costs_visible es false. Los *_display son la versión para mostrar (valores a centavos, costos
//     unitarios a cuatro decimales).
// Toda escritura lleva operation_id (lo agrega useRestaurantMutation); las recetas, porciones y
// cancelaciones llevan además revision (409 REVISION_CONFLICT si otra terminal la cambió).

export type RecipeOwnerKind = 'product' | 'complement' | 'preparation';
export type PreparationMode = 'immediate' | 'stored';
export type RecipeComponentKind = 'item' | 'preparation' | 'ingredient';
export type RecipeTargetKind = 'product' | 'variant' | 'complement';
export type RecipeTargetStatus = 'with_recipe' | 'without_recipe' | 'no_preparation';
export type PortionSource = 'recipe' | 'habitual' | 'none';
export type CancellationState = 'pending' | 'not_prepared' | 'prepared_waste';
export type CancellationResolution = 'manual' | 'automatic' | 'correction';
export type ConsumptionRequestKind = 'release' | 'amend' | 'cancel';
export type ConsumptionRequestState = 'pending' | 'processed' | 'failed';

// Códigos de diagnóstico de la resolución del consumo (nunca inventa cantidades).
export type ConsumptionDiagnosticCode =
  | 'no_configuration'
  | 'ingredient_without_portion'
  | 'complement_without_configuration'
  | 'preparation_unavailable'
  | 'depth_exceeded'
  | 'preparation_cycle'
  | 'unit_not_consumed'
  | 'nothing_consumed'
  | 'unit_reversed'
  | 'unit_already_registered';

export interface ItemReference {
  id: string;
  code: string;
  name: string;
  kind: ItemKind;
  active: boolean;
  unit: UnitReference;
}

export interface WarehouseReference {
  id: string;
  name: string;
}

// Recetas.

export interface RecipeSummary {
  id: string;
  owner_kind: RecipeOwnerKind;
  owner_kind_label: string;
  name: string;
  // Nombre del producto (· variante), del complemento o la modalidad de la preparación.
  target_name: string;
  product_id: string | null;
  variant_id: string | null;
  complement_id: string | null;
  preparation_mode: PreparationMode | null;
  preparation_mode_label: string | null;
  preparation_item_id: string | null;
  active: boolean;
  // Número de la versión vigente (revisión optimista del PUT).
  revision: number;
  effective_from: string;
  line_count: number;
  yield_quantity: string;
  yield_unit: UnitReference;
  updated_at: string;
  updated_by_name: string | null;
}

export type RecipePage = Page<RecipeSummary>;

export interface RecipeLine {
  line_no: number;
  component_kind: RecipeComponentKind;
  component_kind_label: string;
  item: ItemReference | null;
  preparation_recipe_id: string | null;
  preparation_name: string | null;
  ingredient_id: string | null;
  ingredient_name: string | null;
  // Captura (cantidad y unidad) y su conversión a la unidad base del insumo o de la preparación.
  quantity: string;
  unit: UnitReference;
  base_quantity: string;
  base_unit: UnitReference;
  takeaway_only: boolean;
  cost: string | null;
  cost_display: string | null;
  cost_pending: boolean | null;
}

export interface RecipeVersionDetail {
  id: string;
  recipe_id: string;
  version: number;
  name: string;
  active: boolean;
  yield_quantity: string;
  yield_unit: UnitReference;
  yield_base_quantity: string;
  // Porcentaje (solo preparaciones). El rendimiento efectivo ya lo descuenta.
  production_waste_percent: string;
  effective_yield: string;
  effective_from: string;
  notes: string;
  created_by_name: string | null;
  lines: RecipeLine[];
}

export interface RecipeVersionEntry {
  id: string;
  version: number;
  active: boolean;
  effective_from: string;
  line_count: number;
  created_by_name: string | null;
}

// Ingrediente del catálogo del producto con la porción que se le aplica: el renglón de
// ingrediente de esta receta (recipe), la porción habitual (habitual) o ninguna (none: no consume).
export interface RecipeIngredient {
  ingredient_id: string;
  name: string;
  included: string;
  removable: boolean;
  addable: boolean;
  portion_source: PortionSource;
  item: ItemReference | null;
  preparation_name: string | null;
  portion_quantity: string | null;
  included_quantity: string | null;
  cost: string | null;
  cost_display: string | null;
  cost_pending: boolean | null;
}

export interface RecipeUse {
  id: string;
  name: string;
  owner_kind: RecipeOwnerKind;
  active: boolean;
}

// Costo teórico de la versión vigente (solo con inventory.costs.read): unidad vendida (renglones
// fijos e ingredientes incluidos), adicional «solo para llevar» y, en preparaciones, lote y costo
// por unidad base del rendimiento. cost_pending: algún componente no tiene costo.
export interface RecipeCost {
  costs_visible: boolean;
  unit_cost: string | null;
  unit_cost_display: string | null;
  takeaway_cost: string | null;
  takeaway_cost_display: string | null;
  batch_cost: string | null;
  batch_cost_display: string | null;
  yield_unit_cost: string | null;
  yield_unit_cost_display: string | null;
  cost_pending: boolean | null;
}

export interface Recipe extends RecipeSummary {
  product_name: string | null;
  variant_name: string | null;
  complement_name: string | null;
  preparation_item: ItemReference | null;
  version: RecipeVersionDetail;
  versions: RecipeVersionEntry[];
  ingredients: RecipeIngredient[];
  used_by: RecipeUse[];
  cost: RecipeCost;
}

export interface RecipeLineInput {
  component_kind: RecipeComponentKind;
  item_id?: string | null;
  preparation_recipe_id?: string | null;
  // Solo en recetas de producto: sobrescribe la porción habitual de ese ingrediente del catálogo.
  ingredient_id?: string | null;
  quantity: string;
  // Por omisión, la unidad base del insumo o de la preparación.
  unit_id?: string | null;
  takeaway_only?: boolean;
}

// Alta: el destino (owner_kind con producto/variante, complemento o modalidad e insumo de la
// preparación) se fija aquí y no cambia después.
export interface RecipeCreateInput {
  owner_kind: RecipeOwnerKind;
  product_id?: string | null;
  variant_id?: string | null;
  complement_id?: string | null;
  preparation_mode?: PreparationMode | null;
  preparation_item_id?: string | null;
  // Vacío: el nombre del producto, complemento o insumo preparado.
  name?: string;
  active: boolean;
  // Por omisión «1» porción (productos y complementos) o la unidad base del insumo preparado.
  yield_quantity?: string;
  yield_unit_id?: string | null;
  production_waste_percent?: string;
  notes?: string;
  lines: RecipeLineInput[];
}

// Nueva versión: revision es la versión vigente; desactivar también crea una versión.
export interface RecipeVersionInput {
  revision: number;
  name?: string;
  active: boolean;
  yield_quantity?: string;
  yield_unit_id?: string | null;
  production_waste_percent?: string;
  notes?: string;
  lines: RecipeLineInput[];
}

export interface RecipeQuery {
  search?: string;
  owner_kind?: RecipeOwnerKind;
  active?: 'true' | 'false';
  product_id?: string;
  page?: string;
  page_size?: string;
}

// Destinos de receta (productos, variantes y complementos del catálogo).

export interface RecipeTarget {
  kind: RecipeTargetKind;
  product_id: string | null;
  variant_id: string | null;
  complement_id: string | null;
  name: string;
  variant_name: string | null;
  category_name: string | null;
  product_kind: 'product' | 'combo' | null;
  active: boolean;
  recipe_id: string | null;
  recipe_name: string | null;
  recipe_active: boolean | null;
  // En una variante, la receta del producto que aplica si la variante no tiene la suya.
  inherited_recipe_id: string | null;
  status: RecipeTargetStatus;
  kitchen_route_mode: 'prepare' | 'none' | null;
  ingredients_total: number;
  ingredients_with_portion: number;
  linked_product_name: string | null;
  linked_ingredient_name: string | null;
  // Consumiría algo aunque no tenga receta (porciones o vínculo del complemento).
  configured: boolean;
}

export type RecipeTargetPage = Page<RecipeTarget>;

export interface RecipeTargetQuery {
  search?: string;
  kind?: RecipeTargetKind;
  state?: RecipeTargetStatus;
  active?: 'true' | 'false';
  page?: string;
  page_size?: string;
}

// Porciones habituales.

export interface PortionHistoryRow {
  item_name: string | null;
  quantity: string | null;
  unit_code: string | null;
  base_quantity: string | null;
  effective_from: string;
  actor_name: string | null;
}

export interface IngredientPortion {
  ingredient_id: string;
  ingredient_name: string;
  catalog_unit: string;
  active: boolean;
  configured: boolean;
  item: ItemReference | null;
  quantity: string | null;
  unit: UnitReference | null;
  base_quantity: string | null;
  effective_from: string | null;
  updated_by_name: string | null;
  // Número de filas del historial (revisión optimista del PUT).
  revision: number;
  history: PortionHistoryRow[];
}

export type IngredientPortionPage = Page<IngredientPortion>;

// item_id null retira la porción (sin cantidad ni unidad).
export interface IngredientPortionInput {
  revision: number;
  item_id: string | null;
  quantity?: string;
  unit_id?: string | null;
}

export interface IngredientPortionQuery {
  search?: string;
  active?: 'true' | 'false';
  configured?: 'true' | 'false';
  page?: string;
  page_size?: string;
}

// Cancelaciones por clasificar.

export interface ConsumptionLine {
  item: ItemReference;
  warehouse: WarehouseReference;
  // Lo consumido por la liberación y sus modificaciones, y lo neto vigente tras clasificar.
  consumed_quantity: string;
  net_quantity: string;
  value: string | null;
  value_display: string | null;
}

export interface Cancellation {
  id: string;
  restaurant_unit_id: string;
  product_name: string;
  account_folio: number | null;
  account_name: string | null;
  cancelled_at: string;
  // Día operativo AAAA-MM-DD (hora de corte y zona horaria del establecimiento).
  operating_date: string;
  deadline_at: string;
  // Pasó el corte: solo se corrige con inventory.adjustments.authorize.
  expired: boolean;
  state: CancellationState;
  state_label: string;
  resolution: CancellationResolution | null;
  resolution_label: string | null;
  // Etapa 9D: «Preparado / merma» que Go asignó al cancelar la cuenta con un motivo que marca merma
  // y que nadie ha corregido. Entonces `classified_by_name` es quien canceló la cuenta (no la
  // clasificó el encargado) y `resolution_label` dice «Preclasificada al cancelar la cuenta».
  preclassified: boolean;
  classified_by_name: string | null;
  classified_at: string | null;
  notes: string | null;
  revision: number;
  required_permission: 'inventory.waste.record' | 'inventory.adjustments.authorize';
  can_classify: boolean;
  costs_visible: boolean;
  lines: ConsumptionLine[];
}

export interface CancellationPage extends Page<Cancellation> {
  // Pendientes en total (para el aviso de la pestaña).
  pending: number;
  costs_visible: boolean;
}

export interface CancellationQuery {
  state?: CancellationState;
  // Días operativos AAAA-MM-DD.
  from?: string;
  until?: string;
  page?: string;
  page_size?: string;
}

export interface ClassifyCancellationInput {
  revision: number;
  state: Exclude<CancellationState, 'pending'>;
  notes?: string | null;
}

// Elaboraciones de subrecetas almacenables.

export interface ProductionLine {
  item: ItemReference;
  quantity: string;
  stock_after: string;
  value: string | null;
  value_display: string | null;
}

export interface Production {
  id: string;
  recipe_id: string;
  recipe_name: string;
  recipe_version: number;
  item: ItemReference;
  warehouse: WarehouseReference;
  batches: string;
  // Lotes × rendimiento efectivo; producido real; merma = lotes × rendimiento − producido.
  expected_quantity: string;
  produced_quantity: string;
  waste_quantity: string;
  occurred_at: string;
  notes: string;
  actor_name: string | null;
  costs_visible: boolean;
  consumed_value: string | null;
  consumed_value_display: string | null;
  unit_cost: string | null;
  unit_cost_display: string | null;
  lines: ProductionLine[];
  warnings: InventoryWarning[];
}

export interface ProductionPage extends Page<Production> {
  costs_visible: boolean;
}

export interface ProductionInput {
  recipe_id: string;
  // Por omisión, «Almacén general».
  warehouse_id?: string | null;
  batches: string;
  // Por omisión, lo esperado.
  produced_quantity?: string | null;
  // RFC 3339, no futura; por omisión la hora del servidor.
  occurred_at?: string | null;
  notes?: string;
}

export interface ProductionQuery {
  item_id?: string;
  warehouse_id?: string;
  from?: string;
  until?: string;
  page?: string;
  page_size?: string;
}

// Diagnóstico del consumo.

export interface ConsumptionDiagnostic {
  code: ConsumptionDiagnosticCode;
  path: string;
  product_id?: string;
  ingredient_id?: string;
  complement_id?: string;
  recipe_id?: string;
}

export interface ConsumptionRequest {
  id: string;
  seq: number;
  kind: ConsumptionRequestKind;
  kind_label: string;
  restaurant_unit_id: string;
  product_name: string;
  account_folio: number | null;
  service_mode: 'dine_in' | 'takeaway';
  released_at: string;
  occurred_at: string;
  state: ConsumptionRequestState;
  state_label: string;
  attempts: number;
  // Mensaje genérico (sin detalles internos ni costos).
  last_error: string | null;
  next_attempt_at: string;
  processed_at: string | null;
  created_at: string;
  diagnostics: ConsumptionDiagnostic[];
}

export interface ConsumptionRequestPage extends Page<ConsumptionRequest> {
  pending: number;
  failed: number;
}

export interface UnitConsumptionTrace {
  restaurant_unit_id: string;
  consumed: boolean;
  state: 'consumed' | 'reversed' | null;
  released_at: string | null;
  service_mode: 'dine_in' | 'takeaway' | null;
  costs_visible: boolean;
  requests: ConsumptionRequest[];
  lines: ConsumptionLine[];
  movements: InventoryMovement[];
  cancellation: Cancellation | null;
}

// Etiquetas fijas para la interfaz (Go también las entrega en cada registro).
export const recipeOwnerKindLabels: Record<RecipeOwnerKind, string> = {
  product: 'Producto',
  complement: 'Complemento',
  preparation: 'Preparación',
};

export const preparationModeLabels: Record<PreparationMode, string> = {
  immediate: 'Elaboración inmediata',
  stored: 'Preparada previamente (almacenable)',
};

export const recipeTargetStatusLabels: Record<RecipeTargetStatus, string> = {
  with_recipe: 'Con receta',
  without_recipe: 'Sin receta',
  no_preparation: 'Sin preparación',
};

export const cancellationStateLabels: Record<CancellationState, string> = {
  pending: 'Pendiente de clasificar',
  not_prepared: 'No preparado',
  prepared_waste: 'Preparado / merma',
};

// Lecturas.

export function listRecipes(query: RecipeQuery = {}) {
  return api<RecipePage>('inventory_recipes', { query: compactQuery(query) });
}

export function readRecipe(id: string) {
  return api<Recipe>('inventory_recipe', { id });
}

export function readRecipeVersion(versionID: string) {
  return api<RecipeVersionDetail>('inventory_recipe_version', { id: versionID });
}

export function listRecipeTargets(query: RecipeTargetQuery = {}) {
  return api<RecipeTargetPage>('inventory_recipe_targets', { query: compactQuery(query) });
}

export function listIngredientPortions(query: IngredientPortionQuery = {}) {
  return api<IngredientPortionPage>('inventory_ingredient_portions', {
    query: compactQuery(query),
  });
}

export function listCancellations(query: CancellationQuery = {}) {
  return api<CancellationPage>('inventory_cancellations', { query: compactQuery(query) });
}

export function listProductions(query: ProductionQuery = {}) {
  return api<ProductionPage>('inventory_productions', { query: compactQuery(query) });
}

// state vacío: las no procesadas; «all»: todas.
export function listConsumptionRequests(
  query: { state?: ConsumptionRequestState | 'all'; page?: string; page_size?: string } = {},
) {
  return api<ConsumptionRequestPage>('inventory_consumption_requests', {
    query: compactQuery(query),
  });
}

export function readUnitConsumption(unitID: string) {
  return api<UnitConsumptionTrace>('inventory_unit_consumption', { id: unitID });
}

// Escrituras: reciben execute de useRestaurantMutation (operation_id y reintento con el mismo
// identificador).

export function createRecipe(execute: InventoryExecute, input: RecipeCreateInput) {
  return execute<Recipe>('inventory_create_recipe', { body: { ...input } });
}

export function saveRecipeVersion(
  execute: InventoryExecute,
  id: string,
  input: RecipeVersionInput,
) {
  return execute<Recipe>('inventory_save_recipe', { id, body: { ...input } });
}

export function saveIngredientPortion(
  execute: InventoryExecute,
  ingredientID: string,
  input: IngredientPortionInput,
) {
  return execute<IngredientPortion>('inventory_save_ingredient_portion', {
    id: ingredientID,
    body: { ...input },
  });
}

export function classifyCancellation(
  execute: InventoryExecute,
  id: string,
  input: ClassifyCancellationInput,
) {
  return execute<Cancellation>('inventory_classify_cancellation', { id, body: { ...input } });
}

export function recordProduction(execute: InventoryExecute, input: ProductionInput) {
  return execute<Production>('inventory_record_production', { body: { ...input } });
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
