import type {
  InventoryCategory,
  InventoryItem,
  InventoryMovement,
  InventorySettings,
  InventorySummary,
  InventoryUnit,
  ItemPage,
  MovementPage,
  StationWarehouse,
  StockPage,
  StockRow,
  UnitReference,
  Warehouse,
} from '../../services/inventory';

// Datos simulados de Go para las pruebas de la sección Inventarios. Solo el transporte nativo se
// sustituye: las cantidades, costos y etiquetas son las que entregaría Go, ya calculadas.

export const ids = {
  gram: '00000000-0000-4000-8000-0000000000a1',
  kilogram: '00000000-0000-4000-8000-0000000000a2',
  liter: '00000000-0000-4000-8000-0000000000a3',
  piece: '00000000-0000-4000-8000-0000000000a4',
  groceries: '00000000-0000-4000-8000-0000000000b1',
  dairy: '00000000-0000-4000-8000-0000000000b2',
  flour: '00000000-0000-4000-8000-0000000000c1',
  tomato: '00000000-0000-4000-8000-0000000000c2',
  created: '00000000-0000-4000-8000-0000000000c3',
  sack: '00000000-0000-4000-8000-0000000000d1',
  general: '00000000-0000-4000-8000-0000000000e1',
  bar: '00000000-0000-4000-8000-0000000000e2',
  hotStation: '00000000-0000-4000-8000-0000000000f1',
  barStation: '00000000-0000-4000-8000-0000000000f2',
  movement: '00000000-0000-4000-8000-000000000101',
  operation: '00000000-0000-4000-8000-000000000201',
};

const stamp = '2026-11-10T15:00:00Z';

function unit(
  id: string,
  code: string,
  name: string,
  dimension: InventoryUnit['dimension'],
  factor: string,
  label: string,
): InventoryUnit {
  return {
    id,
    code,
    name,
    dimension,
    dimension_label: label,
    factor,
    system: true,
    active: true,
    in_use: true,
    revision: 1,
    updated_at: stamp,
  };
}

export const units: InventoryUnit[] = [
  unit(ids.gram, 'g', 'Gramo', 'mass', '1', 'Masa'),
  unit(ids.kilogram, 'kg', 'Kilogramo', 'mass', '1000', 'Masa'),
  unit(ids.liter, 'l', 'Litro', 'volume', '1000', 'Volumen'),
  unit(ids.piece, 'pza', 'Pieza', 'count', '1', 'Piezas'),
];

export function reference(unitID: string): UnitReference {
  const found = units.find((entry) => entry.id === unitID)!;
  return { id: found.id, code: found.code, name: found.name, dimension: found.dimension };
}

export const categories: InventoryCategory[] = [
  {
    id: ids.groceries,
    name: 'Abarrotes',
    active: true,
    sort_order: 10,
    item_count: 2,
    revision: 1,
    updated_at: stamp,
  },
  {
    id: ids.dairy,
    name: 'Lácteos',
    active: true,
    sort_order: 20,
    item_count: 0,
    revision: 1,
    updated_at: stamp,
  },
];

export function item(extra: Partial<InventoryItem> = {}): InventoryItem {
  return {
    id: ids.flour,
    code: 'HAR-01',
    barcode: null,
    name: 'Harina',
    description: '',
    category_id: ids.groceries,
    category_name: 'Abarrotes',
    base_unit: reference(ids.kilogram),
    kind: 'purchased',
    kind_label: 'Comprado',
    active: true,
    costs_visible: true,
    reference_cost: '18.00',
    reference_cost_display: '18.0000',
    min_stock: '5',
    reorder_point: null,
    has_movements: false,
    presentations: [
      {
        id: ids.sack,
        item_id: ids.flour,
        name: 'Costal 25 kg',
        base_quantity: '25',
        barcode: null,
        active: true,
        purchase_default: true,
        revision: 1,
        updated_at: stamp,
      },
    ],
    revision: 3,
    created_at: stamp,
    updated_at: stamp,
    updated_by_name: 'Administrador',
    ...extra,
  };
}

export const tomato = item({
  id: ids.tomato,
  code: 'TOM-01',
  name: 'Tomate',
  reference_cost: null,
  reference_cost_display: null,
  min_stock: null,
  presentations: [],
  revision: 1,
});

export function itemPage(items: InventoryItem[], costsVisible = true): ItemPage {
  return {
    items: items.map((entry) => ({
      ...entry,
      costs_visible: costsVisible,
      reference_cost: costsVisible ? entry.reference_cost : null,
      reference_cost_display: costsVisible ? entry.reference_cost_display : null,
    })),
    total: items.length,
    page: 1,
    page_size: 25,
    costs_visible: costsVisible,
  };
}

export const warehouses: Warehouse[] = [
  {
    id: ids.general,
    name: 'Almacén general',
    description: 'Almacén predeterminado del inventario',
    active: true,
    is_default: true,
    stations: [{ id: ids.hotStation, name: 'Cocina caliente' }],
    revision: 1,
    updated_at: stamp,
  },
  {
    id: ids.bar,
    name: 'Barra',
    description: '',
    active: true,
    is_default: false,
    stations: [{ id: ids.barStation, name: 'Bebidas' }],
    revision: 2,
    updated_at: stamp,
  },
];

export const stations: StationWarehouse[] = [
  {
    station_id: ids.hotStation,
    station_name: 'Cocina caliente',
    station_active: true,
    warehouse_id: null,
    warehouse_name: 'Almacén general',
    uses_default: true,
    effective_from: null,
    assigned_by_name: null,
    revision: 0,
    history: [],
  },
  {
    station_id: ids.barStation,
    station_name: 'Bebidas',
    station_active: true,
    warehouse_id: ids.bar,
    warehouse_name: 'Barra',
    uses_default: false,
    effective_from: '2026-11-09T18:00:00Z',
    assigned_by_name: 'Administrador',
    revision: 1,
    history: [
      {
        warehouse_id: ids.bar,
        warehouse_name: 'Barra',
        effective_from: '2026-11-09T18:00:00Z',
        actor_name: 'Administrador',
      },
    ],
  },
];

function stockRow(extra: Partial<StockRow>): StockRow {
  return {
    item_id: ids.flour,
    item_code: 'HAR-01',
    item_name: 'Harina',
    category_name: 'Abarrotes',
    unit: reference(ids.kilogram),
    active: true,
    quantity: '12.5',
    total_quantity: '12.5',
    negative: false,
    min_stock: '5',
    reorder_point: null,
    below_minimum: false,
    at_reorder_point: false,
    warehouses: [
      {
        warehouse_id: ids.general,
        warehouse_name: 'Almacén general',
        quantity: '12.5',
        negative: false,
      },
    ],
    average_cost: '18.00',
    average_cost_display: '18.0000',
    value: '225.00',
    value_display: '225.00',
    cost_pending: false,
    updated_at: stamp,
    ...extra,
  };
}

export function stockPage(costsVisible: boolean, valuationVisible: boolean): StockPage {
  const rows = [
    stockRow({}),
    stockRow({
      item_id: ids.tomato,
      item_code: 'TOM-01',
      item_name: 'Tomate',
      quantity: '-1.5',
      total_quantity: '-1.5',
      negative: true,
      min_stock: null,
      warehouses: [
        {
          warehouse_id: ids.general,
          warehouse_name: 'Almacén general',
          quantity: '-1.5',
          negative: true,
        },
      ],
      average_cost: '20.00',
      average_cost_display: '20.0000',
      value: '-30.00',
      value_display: '-30.00',
    }),
  ];
  return {
    items: rows.map((row) => ({
      ...row,
      average_cost: costsVisible ? row.average_cost : null,
      average_cost_display: costsVisible ? row.average_cost_display : null,
      // Fase D: el valor de cada insumo solo con inventory.valuation.read.
      value: valuationVisible ? row.value : null,
      value_display: valuationVisible ? row.value_display : null,
      cost_pending: costsVisible ? row.cost_pending : null,
    })),
    total: rows.length,
    page: 1,
    page_size: 25,
    warehouse: null,
    costs_visible: costsVisible,
    valuation_visible: valuationVisible,
    total_value: valuationVisible ? '195.00' : null,
    total_value_display: valuationVisible ? '195.00' : null,
  };
}

// La valuación posterior al movimiento exige costos y valuación (fase D); por omisión, ambos con
// costsVisible.
export function movementPage(costsVisible: boolean, valuationVisible = costsVisible): MovementPage {
  const movement: InventoryMovement = {
    id: ids.movement,
    seq: 1,
    item_id: ids.flour,
    item_code: 'HAR-01',
    item_name: 'Harina',
    unit: reference(ids.kilogram),
    warehouse_id: ids.general,
    warehouse_name: 'Almacén general',
    kind: 'initial',
    kind_label: 'Existencia inicial',
    quantity: '12.5',
    stock_after: '12.5',
    valuation_quantity_after: '12.5',
    costs_visible: costsVisible,
    value: costsVisible ? '225.00' : null,
    value_display: costsVisible ? '225.00' : null,
    unit_cost: costsVisible ? '18.00' : null,
    unit_cost_display: costsVisible ? '18.0000' : null,
    cost_basis: costsVisible ? 'document' : null,
    cost_basis_label: costsVisible ? 'Costo del documento' : null,
    cost_pending: costsVisible ? false : null,
    valuation_visible: costsVisible && valuationVisible,
    valuation_value_after: costsVisible && valuationVisible ? '225.00' : null,
    occurred_at: '2026-11-10T14:30:00Z',
    recorded_at: '2026-11-10T14:31:00Z',
    source_kind: 'initial',
    source_kind_label: 'Existencia inicial',
    source_id: ids.operation,
    source_line: 1,
    restaurant_unit_id: null,
    related_movement_id: null,
    reason: 'Inventario de apertura',
    actor_name: 'Administrador',
    process: null,
    process_label: null,
    operation_id: ids.operation,
  };
  return {
    items: [movement],
    total: 1,
    page: 1,
    page_size: 50,
    costs_visible: costsVisible,
    valuation_visible: costsVisible && valuationVisible,
  };
}

export function settings(extra: Partial<InventorySettings> = {}): InventorySettings {
  return {
    purchase_tax_in_cost: true,
    operating_day_cutoff: '05:00',
    warn_products_without_recipe: true,
    count_frequency: 'none',
    count_frequency_label: 'Sin frecuencia',
    count_interval_days: null,
    revision: 1,
    updated_at: stamp,
    updated_by_name: 'Administrador',
    ...extra,
  };
}

export function summary(extra: Partial<InventorySummary> = {}): InventorySummary {
  return {
    feature: {
      code: 'inventory_control',
      name: 'Control de inventario',
      enabled: false,
      enabled_at: null,
      revision: 1,
    },
    active_items: 2,
    inactive_items: 1,
    active_warehouses: 2,
    negative_stock: 1,
    below_minimum: 0,
    at_reorder_point: 0,
    costs_visible: true,
    pending_cost_items: 0,
    valuation_visible: true,
    total_value: '195.00',
    total_value_display: '195.00',
    ...extra,
  };
}

// Todos los permisos de inventario de las fases A y B.
export const allInventoryPermissions = [
  'inventory.read',
  'inventory.items.manage',
  'inventory.costs.read',
  'inventory.costs.manage',
  'inventory.stock.read',
  'inventory.warehouses.manage',
  'inventory.movements.read',
  'inventory.adjustments.authorize',
  'inventory.valuation.read',
  'inventory.settings.manage',
  'inventory.recipes.manage',
  'inventory.subrecipes.manage',
  'inventory.production.record',
  'inventory.waste.record',
];

export interface RecordedRequest {
  operation: string;
  resourceId: string | null;
  body: Record<string, unknown> | null;
  query: Record<string, string> | null;
}

export type Responder = (request: RecordedRequest) => { status: number; body: unknown } | undefined;

// Transporte simulado: responde lecturas con los datos de arriba y registra cada solicitud. Cada
// prueba puede anteponer respuestas propias con «override».
export function inventoryTransport(override: Responder = () => undefined) {
  const requests: RecordedRequest[] = [];
  const handler = async (command: string, args: RecordedRequest) => {
    if (command !== 'api_request') {
      throw new Error(`Comando no esperado: ${command}`);
    }
    const request: RecordedRequest = {
      operation: args.operation,
      resourceId: args.resourceId ?? null,
      body: args.body ?? null,
      query: args.query ?? null,
    };
    requests.push(request);
    const custom = override(request);
    if (custom) {
      return custom;
    }
    switch (request.operation) {
      case 'inventory_summary':
        return { status: 200, body: summary() };
      case 'inventory_units':
        return { status: 200, body: units };
      case 'inventory_categories':
        return { status: 200, body: categories };
      case 'inventory_items':
        return { status: 200, body: itemPage([item(), tomato]) };
      case 'inventory_item':
        return { status: 200, body: item() };
      case 'inventory_warehouses':
        return { status: 200, body: warehouses };
      case 'inventory_station_warehouses':
        return { status: 200, body: stations };
      case 'inventory_stock':
        return { status: 200, body: stockPage(true, true) };
      case 'inventory_movements':
        return { status: 200, body: movementPage(true) };
      case 'inventory_settings':
        return { status: 200, body: settings() };
      // Fase B: listados vacíos salvo que la prueba los sustituya.
      case 'inventory_recipes':
      case 'inventory_recipe_targets':
      case 'inventory_ingredient_portions':
        return { status: 200, body: { items: [], total: 0, page: 1, page_size: 25 } };
      case 'inventory_cancellations':
        return {
          status: 200,
          body: { items: [], total: 0, pending: 0, page: 1, page_size: 25, costs_visible: true },
        };
      case 'inventory_consumption_requests':
        return {
          status: 200,
          body: { items: [], total: 0, pending: 0, failed: 0, page: 1, page_size: 25 },
        };
      case 'inventory_productions':
        return {
          status: 200,
          body: { items: [], total: 0, page: 1, page_size: 25, costs_visible: true },
        };
      // Fase D: listados vacíos y sin alertas salvo que la prueba los sustituya.
      case 'inventory_adjustments':
      case 'inventory_transfers':
        return {
          status: 200,
          body: { items: [], total: 0, page: 1, page_size: 25, costs_visible: true },
        };
      case 'inventory_counts':
      case 'inventory_stock_levels':
        return { status: 200, body: { items: [], total: 0, page: 1, page_size: 25 } };
      case 'inventory_alerts':
        return {
          status: 200,
          body: {
            items: [],
            total: 0,
            page: 1,
            page_size: 100,
            counts: { total: 0, critical: 0, warning: 0, info: 0, by_type: {} },
            costs_visible: true,
            stock_visible: true,
            purchasing_visible: false,
            generated_at: stamp,
          },
        };
      case 'inventory_purchase_orders':
        return { status: 200, body: { items: [], total: 0, page: 1, page_size: 25 } };
      case 'inventory_purchase_suggestions':
        return {
          status: 200,
          body: {
            items: [],
            total: 0,
            page: 1,
            page_size: 100,
            stock_visible: true,
            purchasing_visible: false,
          },
        };
      default:
        return { status: 200, body: {} };
    }
  };
  return { requests, handler };
}
