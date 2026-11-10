import type {
  InventoryItem,
  InventoryMovement,
  InventoryUnit,
  ItemPage,
} from '../../services/inventory';
import type {
  Cancellation,
  CancellationPage,
  ConsumptionRequest,
  ConsumptionRequestPage,
  IngredientPortion,
  IngredientPortionPage,
  ItemReference,
  Production,
  Recipe,
  RecipeLine,
  RecipePage,
  RecipeSummary,
  RecipeTarget,
  RecipeTargetPage,
  RecipeVersionDetail,
  UnitConsumptionTrace,
} from '../../services/inventoryRecipes';
import { ids, item, reference, units } from './inventoryTestData';

// Datos simulados de Go para las pruebas de la fase B (recetas, porciones, elaboraciones,
// cancelaciones y diagnóstico del consumo). Las cantidades, costos y etiquetas son las que
// entregaría Go, ya calculadas: la interfaz solo las muestra.

export const recipeIDs = {
  milliliter: '00000000-0000-4000-8000-0000000003a1',
  portion: '00000000-0000-4000-8000-0000000003a2',
  meat: '00000000-0000-4000-8000-0000000003b1',
  onion: '00000000-0000-4000-8000-0000000003b2',
  bag: '00000000-0000-4000-8000-0000000003b3',
  rice: '00000000-0000-4000-8000-0000000003b4',
  steak: '00000000-0000-4000-8000-0000000003c1',
  burger: '00000000-0000-4000-8000-0000000003c2',
  double: '00000000-0000-4000-8000-0000000003c3',
  riceComplement: '00000000-0000-4000-8000-0000000003c4',
  burgerRecipe: '00000000-0000-4000-8000-0000000003d1',
  sauceRecipe: '00000000-0000-4000-8000-0000000003d2',
  storedRiceRecipe: '00000000-0000-4000-8000-0000000003d3',
  versionOne: '00000000-0000-4000-8000-0000000003e1',
  versionTwo: '00000000-0000-4000-8000-0000000003e2',
  sauceVersion: '00000000-0000-4000-8000-0000000003e3',
  onionIngredient: '00000000-0000-4000-8000-0000000003f1',
  jalapenoIngredient: '00000000-0000-4000-8000-0000000003f2',
  pendingCancellation: '00000000-0000-4000-8000-000000000401',
  automaticCancellation: '00000000-0000-4000-8000-000000000402',
  manualCancellation: '00000000-0000-4000-8000-000000000403',
  unit: '00000000-0000-4000-8000-000000000501',
  otherUnit: '00000000-0000-4000-8000-000000000502',
  failedRequest: '00000000-0000-4000-8000-000000000601',
  production: '00000000-0000-4000-8000-000000000701',
  consumptionMovement: '00000000-0000-4000-8000-000000000801',
};

const stamp = '2026-11-10T15:00:00Z';

function systemUnit(
  id: string,
  code: string,
  name: string,
  dimension: InventoryUnit['dimension'],
  label: string,
): InventoryUnit {
  return {
    id,
    code,
    name,
    dimension,
    dimension_label: label,
    factor: '1',
    system: true,
    active: true,
    in_use: false,
    revision: 1,
    updated_at: stamp,
  };
}

// Unidades de la fase A más mililitro y porción (equivalencia 1 de su dimensión).
export const recipeUnits: InventoryUnit[] = [
  ...units,
  systemUnit(recipeIDs.milliliter, 'ml', 'Mililitro', 'volume', 'Volumen'),
  systemUnit(recipeIDs.portion, 'porcion', 'Porción', 'portion', 'Porciones'),
];

const milliliter = {
  id: recipeIDs.milliliter,
  code: 'ml',
  name: 'Mililitro',
  dimension: 'volume' as const,
};
const portion = {
  id: recipeIDs.portion,
  code: 'porcion',
  name: 'Porción',
  dimension: 'portion' as const,
};

export const meat: ItemReference = {
  id: recipeIDs.meat,
  code: 'CAR-01',
  name: 'Carne molida',
  kind: 'purchased',
  active: true,
  unit: reference(ids.kilogram),
};
export const onion: ItemReference = {
  id: recipeIDs.onion,
  code: 'CEB-01',
  name: 'Cebolla blanca',
  kind: 'purchased',
  active: true,
  unit: reference(ids.gram),
};
export const bag: ItemReference = {
  id: recipeIDs.bag,
  code: 'BOL-01',
  name: 'Bolsa para llevar',
  kind: 'purchased',
  active: true,
  unit: reference(ids.piece),
};
export const preparedRice: ItemReference = {
  id: recipeIDs.rice,
  code: 'ARR-PREP',
  name: 'Arroz rojo preparado',
  kind: 'prepared',
  active: true,
  unit: reference(ids.gram),
};

function inventoryItemFrom(entry: ItemReference): InventoryItem {
  return item({
    id: entry.id,
    code: entry.code,
    name: entry.name,
    kind: entry.kind,
    kind_label: entry.kind === 'prepared' ? 'Preparación almacenable' : 'Comprado',
    base_unit: entry.unit,
    presentations: [],
    min_stock: null,
    revision: 1,
  });
}

// Insumos que ofrecen los selectores de receta.
export function recipeItemPage(): ItemPage {
  const items = [meat, onion, bag, preparedRice].map(inventoryItemFrom);
  return { items, total: items.length, page: 1, page_size: 50, costs_visible: true };
}

function target(extra: Partial<RecipeTarget>): RecipeTarget {
  return {
    kind: 'product',
    product_id: recipeIDs.steak,
    variant_id: null,
    complement_id: null,
    name: 'Filete de prueba',
    variant_name: null,
    category_name: 'Platillos',
    product_kind: 'product',
    active: true,
    recipe_id: null,
    recipe_name: null,
    recipe_active: null,
    inherited_recipe_id: null,
    status: 'without_recipe',
    kitchen_route_mode: 'prepare',
    ingredients_total: 0,
    ingredients_with_portion: 0,
    linked_product_name: null,
    linked_ingredient_name: null,
    configured: false,
    ...extra,
  };
}

export function targetPage(): RecipeTargetPage {
  const items = [
    target({}),
    target({
      product_id: recipeIDs.burger,
      name: 'Hamburguesa de prueba',
      recipe_id: recipeIDs.burgerRecipe,
      recipe_name: 'Hamburguesa de prueba',
      recipe_active: true,
      status: 'with_recipe',
      ingredients_total: 2,
      ingredients_with_portion: 1,
      configured: true,
    }),
    target({
      kind: 'variant',
      product_id: recipeIDs.burger,
      variant_id: recipeIDs.double,
      name: 'Hamburguesa de prueba',
      variant_name: 'Doble',
      inherited_recipe_id: recipeIDs.burgerRecipe,
      status: 'with_recipe',
      ingredients_total: 2,
      ingredients_with_portion: 1,
      configured: true,
    }),
    target({
      kind: 'complement',
      product_id: null,
      complement_id: recipeIDs.riceComplement,
      name: 'Arroz',
      category_name: null,
      product_kind: null,
      kitchen_route_mode: null,
      linked_ingredient_name: 'Arroz',
      configured: true,
    }),
  ];
  return { items, total: items.length, page: 1, page_size: 25 };
}

function line(extra: Partial<RecipeLine>): RecipeLine {
  return {
    line_no: 1,
    component_kind: 'item',
    component_kind_label: 'Insumo',
    item: meat,
    preparation_recipe_id: null,
    preparation_name: null,
    ingredient_id: null,
    ingredient_name: null,
    quantity: '180',
    unit: reference(ids.gram),
    base_quantity: '0.18',
    base_unit: reference(ids.kilogram),
    takeaway_only: false,
    cost: null,
    cost_display: null,
    cost_pending: null,
    ...extra,
  };
}

function burgerLines(costsVisible: boolean): RecipeLine[] {
  const cost = (value: string) => ({
    cost: costsVisible ? value : null,
    cost_display: costsVisible ? value : null,
    cost_pending: costsVisible ? false : null,
  });
  return [
    line({ ...cost('32.40') }),
    line({
      line_no: 2,
      component_kind: 'preparation',
      component_kind_label: 'Preparación',
      item: null,
      preparation_recipe_id: recipeIDs.sauceRecipe,
      preparation_name: 'Salsa de la casa',
      quantity: '20',
      unit: milliliter,
      base_quantity: '20',
      base_unit: milliliter,
      ...cost('0.68'),
    }),
    line({
      line_no: 3,
      component_kind: 'ingredient',
      component_kind_label: 'Ingrediente del catálogo',
      item: onion,
      ingredient_id: recipeIDs.onionIngredient,
      ingredient_name: 'Cebolla',
      quantity: '20',
      unit: reference(ids.gram),
      base_quantity: '20',
      base_unit: reference(ids.gram),
      ...cost('0.60'),
    }),
    line({
      line_no: 4,
      item: bag,
      quantity: '1',
      unit: reference(ids.piece),
      base_quantity: '1',
      base_unit: reference(ids.piece),
      takeaway_only: true,
      ...cost('2.00'),
    }),
  ];
}

function version(extra: Partial<RecipeVersionDetail>): RecipeVersionDetail {
  return {
    id: recipeIDs.versionTwo,
    recipe_id: recipeIDs.burgerRecipe,
    version: 2,
    name: 'Hamburguesa de prueba',
    active: true,
    yield_quantity: '1',
    yield_unit: portion,
    yield_base_quantity: '1',
    production_waste_percent: '0',
    effective_yield: '1',
    effective_from: '2026-11-10T14:00:00Z',
    notes: '',
    created_by_name: 'Administrador',
    lines: [],
    ...extra,
  };
}

// Versión 1 del historial: 150 g de carne (la vigente usa 180 g).
export function burgerVersionOne(): RecipeVersionDetail {
  return version({
    id: recipeIDs.versionOne,
    version: 1,
    effective_from: '2026-11-09T14:00:00Z',
    lines: [line({ quantity: '150', base_quantity: '0.15' })],
  });
}

function summary(extra: Partial<RecipeSummary>): RecipeSummary {
  return {
    id: recipeIDs.burgerRecipe,
    owner_kind: 'product',
    owner_kind_label: 'Producto',
    name: 'Hamburguesa de prueba',
    target_name: 'Hamburguesa de prueba',
    product_id: recipeIDs.burger,
    variant_id: null,
    complement_id: null,
    preparation_mode: null,
    preparation_mode_label: null,
    preparation_item_id: null,
    active: true,
    revision: 2,
    effective_from: '2026-11-10T14:00:00Z',
    line_count: 4,
    yield_quantity: '1',
    yield_unit: portion,
    updated_at: stamp,
    updated_by_name: 'Administrador',
    ...extra,
  };
}

export function burgerRecipe(costsVisible = true, extra: Partial<Recipe> = {}): Recipe {
  return {
    ...summary({}),
    product_name: 'Hamburguesa de prueba',
    variant_name: null,
    complement_name: null,
    preparation_item: null,
    version: version({ lines: burgerLines(costsVisible) }),
    versions: [
      {
        id: recipeIDs.versionTwo,
        version: 2,
        active: true,
        effective_from: '2026-11-10T14:00:00Z',
        line_count: 4,
        created_by_name: 'Administrador',
      },
      {
        id: recipeIDs.versionOne,
        version: 1,
        active: true,
        effective_from: '2026-11-09T14:00:00Z',
        line_count: 1,
        created_by_name: 'Administrador',
      },
    ],
    ingredients: [
      {
        ingredient_id: recipeIDs.onionIngredient,
        name: 'Cebolla',
        included: '1',
        removable: true,
        addable: true,
        portion_source: 'recipe',
        item: onion,
        preparation_name: null,
        portion_quantity: '20',
        included_quantity: '20',
        cost: costsVisible ? '0.60' : null,
        cost_display: costsVisible ? '0.60' : null,
        cost_pending: costsVisible ? false : null,
      },
      {
        ingredient_id: recipeIDs.jalapenoIngredient,
        name: 'Jalapeño',
        included: '0',
        removable: false,
        addable: true,
        portion_source: 'none',
        item: null,
        preparation_name: null,
        portion_quantity: null,
        included_quantity: null,
        cost: null,
        cost_display: null,
        cost_pending: null,
      },
    ],
    used_by: [],
    cost: {
      costs_visible: costsVisible,
      unit_cost: costsVisible ? '35.68' : null,
      unit_cost_display: costsVisible ? '35.68' : null,
      takeaway_cost: costsVisible ? '2.00' : null,
      takeaway_cost_display: costsVisible ? '2.00' : null,
      batch_cost: null,
      batch_cost_display: null,
      yield_unit_cost: null,
      yield_unit_cost_display: null,
      cost_pending: costsVisible ? false : null,
    },
    ...extra,
  };
}

// Preparación inmediata «Salsa de la casa»: un lote de 500 ml.
export function sauceRecipe(): Recipe {
  return {
    ...summary({
      id: recipeIDs.sauceRecipe,
      owner_kind: 'preparation',
      owner_kind_label: 'Preparación',
      name: 'Salsa de la casa',
      target_name: 'Elaboración inmediata',
      product_id: null,
      preparation_mode: 'immediate',
      preparation_mode_label: 'Elaboración inmediata',
      revision: 1,
      line_count: 1,
      yield_quantity: '500',
      yield_unit: milliliter,
    }),
    product_name: null,
    variant_name: null,
    complement_name: null,
    preparation_item: null,
    version: version({
      id: recipeIDs.sauceVersion,
      recipe_id: recipeIDs.sauceRecipe,
      version: 1,
      name: 'Salsa de la casa',
      yield_quantity: '500',
      yield_unit: milliliter,
      yield_base_quantity: '500',
      effective_yield: '500',
      lines: [
        line({
          item: onion,
          quantity: '100',
          unit: reference(ids.gram),
          base_quantity: '100',
          base_unit: reference(ids.gram),
        }),
      ],
    }),
    versions: [
      {
        id: recipeIDs.sauceVersion,
        version: 1,
        active: true,
        effective_from: stamp,
        line_count: 1,
        created_by_name: 'Administrador',
      },
    ],
    ingredients: [],
    used_by: [
      {
        id: recipeIDs.burgerRecipe,
        name: 'Hamburguesa de prueba',
        owner_kind: 'product',
        active: true,
      },
    ],
    cost: {
      costs_visible: true,
      unit_cost: null,
      unit_cost_display: null,
      takeaway_cost: null,
      takeaway_cost_display: null,
      batch_cost: '17.00',
      batch_cost_display: '17.00',
      yield_unit_cost: '0.034',
      yield_unit_cost_display: '0.0340',
      cost_pending: false,
    },
  };
}

// Preparación almacenable «Arroz rojo»: lote de 2,500 g con 4 % de merma (rinde 2,400 g).
export function storedRiceRecipe(): Recipe {
  const base = sauceRecipe();
  return {
    ...base,
    id: recipeIDs.storedRiceRecipe,
    name: 'Arroz rojo',
    target_name: 'Preparada previamente (almacenable)',
    preparation_mode: 'stored',
    preparation_mode_label: 'Preparada previamente (almacenable)',
    preparation_item_id: recipeIDs.rice,
    yield_quantity: '2500',
    yield_unit: reference(ids.gram),
    preparation_item: preparedRice,
    version: {
      ...base.version,
      id: '00000000-0000-4000-8000-0000000003e4',
      recipe_id: recipeIDs.storedRiceRecipe,
      name: 'Arroz rojo',
      yield_quantity: '2500',
      yield_unit: reference(ids.gram),
      yield_base_quantity: '2500',
      production_waste_percent: '4',
      effective_yield: '2400',
    },
    used_by: [],
  };
}

export function preparationPage(): RecipePage {
  const sauce = sauceRecipe();
  const rice = storedRiceRecipe();
  const strip = (recipe: Recipe): RecipeSummary => ({
    id: recipe.id,
    owner_kind: recipe.owner_kind,
    owner_kind_label: recipe.owner_kind_label,
    name: recipe.name,
    target_name: recipe.target_name,
    product_id: recipe.product_id,
    variant_id: recipe.variant_id,
    complement_id: recipe.complement_id,
    preparation_mode: recipe.preparation_mode,
    preparation_mode_label: recipe.preparation_mode_label,
    preparation_item_id: recipe.preparation_item_id,
    active: recipe.active,
    revision: recipe.revision,
    effective_from: recipe.effective_from,
    line_count: recipe.line_count,
    yield_quantity: recipe.yield_quantity,
    yield_unit: recipe.yield_unit,
    updated_at: recipe.updated_at,
    updated_by_name: recipe.updated_by_name,
  });
  const items = [strip(sauce), strip(rice)];
  return { items, total: items.length, page: 1, page_size: 100 };
}

function portionRow(extra: Partial<IngredientPortion>): IngredientPortion {
  return {
    ingredient_id: recipeIDs.onionIngredient,
    ingredient_name: 'Cebolla',
    catalog_unit: 'porción',
    active: true,
    configured: true,
    item: onion,
    quantity: '20',
    unit: reference(ids.gram),
    base_quantity: '20',
    effective_from: '2026-11-10T13:00:00Z',
    updated_by_name: 'Administrador',
    revision: 2,
    history: [
      {
        item_name: 'Cebolla blanca',
        quantity: '20',
        unit_code: 'g',
        base_quantity: '20',
        effective_from: '2026-11-10T13:00:00Z',
        actor_name: 'Administrador',
      },
      {
        item_name: 'Cebolla blanca',
        quantity: '15',
        unit_code: 'g',
        base_quantity: '15',
        effective_from: '2026-11-09T13:00:00Z',
        actor_name: 'Administrador',
      },
    ],
    ...extra,
  };
}

export function onionPortion(extra: Partial<IngredientPortion> = {}) {
  return portionRow(extra);
}

export function portionPage(): IngredientPortionPage {
  const items = [
    portionRow({}),
    portionRow({
      ingredient_id: recipeIDs.jalapenoIngredient,
      ingredient_name: 'Jalapeño',
      configured: false,
      item: null,
      quantity: null,
      unit: null,
      base_quantity: null,
      effective_from: null,
      updated_by_name: null,
      revision: 0,
      history: [],
    }),
  ];
  return { items, total: items.length, page: 1, page_size: 25 };
}

export function production(): Production {
  return {
    id: recipeIDs.production,
    recipe_id: recipeIDs.storedRiceRecipe,
    recipe_name: 'Arroz rojo',
    recipe_version: 1,
    item: preparedRice,
    warehouse: { id: ids.general, name: 'Almacén general' },
    batches: '2',
    expected_quantity: '4800',
    produced_quantity: '4750',
    waste_quantity: '250',
    occurred_at: '2026-11-10T16:00:00Z',
    notes: '',
    actor_name: 'Administrador',
    costs_visible: true,
    consumed_value: '56.80',
    consumed_value_display: '56.80',
    unit_cost: '0.0119578947',
    unit_cost_display: '0.0120',
    lines: [
      {
        item: onion,
        quantity: '160',
        stock_after: '-40',
        value: '4.80',
        value_display: '4.80',
      },
    ],
    warnings: [
      {
        code: 'NEGATIVE_STOCK',
        message: 'La existencia de «Cebolla blanca» en «Almacén general» quedó negativa.',
        item_id: recipeIDs.onion,
      },
    ],
  };
}

function cancellation(extra: Partial<Cancellation>): Cancellation {
  return {
    id: recipeIDs.pendingCancellation,
    restaurant_unit_id: recipeIDs.unit,
    product_name: 'Hamburguesa de prueba',
    account_folio: 12,
    account_name: 'Mesa 4',
    cancelled_at: '2026-11-10T20:30:00Z',
    operating_date: '2026-11-10',
    deadline_at: '2026-11-11T11:00:00Z',
    expired: false,
    state: 'pending',
    state_label: 'Pendiente de clasificar',
    resolution: null,
    resolution_label: null,
    classified_by_name: null,
    classified_at: null,
    notes: null,
    revision: 1,
    required_permission: 'inventory.waste.record',
    can_classify: true,
    costs_visible: true,
    lines: [
      {
        item: meat,
        warehouse: { id: ids.general, name: 'Almacén general' },
        consumed_quantity: '0.18',
        net_quantity: '0.18',
        value: '32.40',
        value_display: '32.40',
      },
    ],
    ...extra,
  };
}

export function pendingCancellation(extra: Partial<Cancellation> = {}) {
  return cancellation(extra);
}

export function pendingPage(entries: Cancellation[]): CancellationPage {
  return {
    items: entries,
    total: entries.length,
    pending: entries.filter((entry) => entry.state === 'pending').length,
    page: 1,
    page_size: 25,
    costs_visible: true,
  };
}

export function automaticCancellation(extra: Partial<Cancellation> = {}) {
  return cancellation({
    id: recipeIDs.automaticCancellation,
    restaurant_unit_id: recipeIDs.otherUnit,
    product_name: 'Filete de prueba',
    account_folio: 7,
    account_name: 'Pedido para llevar',
    operating_date: '2026-11-09',
    deadline_at: '2026-11-10T11:00:00Z',
    expired: true,
    state: 'not_prepared',
    state_label: 'No preparado',
    resolution: 'automatic',
    resolution_label: 'Automática al cierre del día operativo',
    classified_at: '2026-11-10T11:00:01Z',
    revision: 2,
    required_permission: 'inventory.adjustments.authorize',
    ...extra,
  });
}

export function failedRequest(): ConsumptionRequest {
  return {
    id: recipeIDs.failedRequest,
    seq: 41,
    kind: 'release',
    kind_label: 'Liberación a cocina',
    restaurant_unit_id: recipeIDs.unit,
    product_name: 'Hamburguesa de prueba',
    account_folio: 12,
    service_mode: 'takeaway',
    released_at: '2026-11-10T20:00:00Z',
    occurred_at: '2026-11-10T20:00:00Z',
    state: 'failed',
    state_label: 'Con error (se reintenta)',
    attempts: 3,
    last_error: 'No se pudo calcular el consumo; se reintentará automáticamente.',
    next_attempt_at: '2026-11-10T20:05:00Z',
    processed_at: null,
    created_at: '2026-11-10T20:00:00Z',
    diagnostics: [
      {
        code: 'ingredient_without_portion',
        path: 'root',
        ingredient_id: recipeIDs.jalapenoIngredient,
      },
      { code: 'no_configuration', path: 'root/1', product_id: recipeIDs.steak },
    ],
  };
}

export function requestPage(): ConsumptionRequestPage {
  return { items: [failedRequest()], total: 1, pending: 2, failed: 1, page: 1, page_size: 25 };
}

export function consumptionMovement(costsVisible: boolean): InventoryMovement {
  return {
    id: recipeIDs.consumptionMovement,
    seq: 9,
    item_id: meat.id,
    item_code: meat.code,
    item_name: meat.name,
    unit: meat.unit,
    warehouse_id: ids.general,
    warehouse_name: 'Almacén general',
    kind: 'consumption',
    kind_label: 'Consumo por preparación',
    quantity: '-0.18',
    stock_after: '4.82',
    valuation_quantity_after: '4.82',
    costs_visible: costsVisible,
    value: costsVisible ? '-32.40' : null,
    value_display: costsVisible ? '-32.40' : null,
    unit_cost: costsVisible ? '180.00' : null,
    unit_cost_display: costsVisible ? '180.0000' : null,
    cost_basis: costsVisible ? 'average' : null,
    cost_basis_label: costsVisible ? 'Costo promedio' : null,
    cost_pending: costsVisible ? false : null,
    valuation_value_after: costsVisible ? '867.60' : null,
    occurred_at: '2026-11-10T20:00:00Z',
    recorded_at: '2026-11-10T20:00:01Z',
    source_kind: 'consumption',
    source_kind_label: 'Liberación a cocina',
    source_id: recipeIDs.failedRequest,
    source_line: null,
    restaurant_unit_id: recipeIDs.unit,
    related_movement_id: null,
    reason: 'Liberación a cocina · Estación: Cocina caliente',
    actor_name: null,
    process: 'inventory_worker',
    process_label: 'Proceso de consumo de inventario',
    operation_id: null,
  };
}

export function unitTrace(costsVisible: boolean): UnitConsumptionTrace {
  const request = { ...failedRequest(), state: 'processed' as const, state_label: 'Procesada' };
  return {
    restaurant_unit_id: recipeIDs.unit,
    consumed: true,
    state: 'consumed',
    released_at: '2026-11-10T20:00:00Z',
    service_mode: 'takeaway',
    costs_visible: costsVisible,
    requests: [{ ...request, processed_at: '2026-11-10T20:00:01Z', diagnostics: [] }],
    lines: [
      {
        item: meat,
        warehouse: { id: ids.general, name: 'Almacén general' },
        consumed_quantity: '0.18',
        net_quantity: '0.18',
        value: costsVisible ? '32.40' : null,
        value_display: costsVisible ? '32.40' : null,
      },
    ],
    movements: [consumptionMovement(costsVisible)],
    cancellation: null,
  };
}
