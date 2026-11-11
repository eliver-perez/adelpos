import type {
  Adjustment,
  AdjustmentLine,
  AlertPage,
  Count,
  CountLine,
  InventoryAlert,
  PurchaseSuggestion,
  PurchaseSuggestionPage,
  StockLevel,
  Transfer,
} from '../../services/inventoryControl';
import type { ItemReference } from '../../services/inventoryRecipes';
import { purchasingPermissions } from './inventoryPurchasingTestData';
import { ids, reference } from './inventoryTestData';

// Datos simulados de Go para las pruebas de la fase D (ajustes, mermas, transferencias, conteos,
// niveles por almacén, alertas y sugerencias de compra). Las cantidades, diferencias, teóricos y
// valores son los que entregaría Go, ya calculados: algunos se eligieron sin relación aritmética con
// lo capturado para comprobar que la pantalla muestra lo que llega y no calcula por su cuenta.

export const controlIDs = {
  adjustment: '00000000-0000-4000-8000-000000000a01',
  waste: '00000000-0000-4000-8000-000000000a02',
  transfer: '00000000-0000-4000-8000-000000000a11',
  count: '00000000-0000-4000-8000-000000000a21',
  product: '00000000-0000-4000-8000-000000000a31',
  draftOrder: '00000000-0000-4000-8000-000000000a41',
  supplier: '00000000-0000-4000-8000-000000000a51',
};

const stamp = '2026-11-10T15:00:00Z';

// Permisos de la fase D además de los de las fases A–C.
export const controlPermissions = [
  ...purchasingPermissions,
  'inventory.adjustments.record',
  'inventory.counts.perform',
  'inventory.counts.approve',
  'inventory.transfers.record',
];

export const flourReference: ItemReference = {
  id: ids.flour,
  code: 'HAR-01',
  name: 'Harina',
  kind: 'purchased',
  active: true,
  unit: reference(ids.kilogram),
};

export const tomatoReference: ItemReference = {
  id: ids.tomato,
  code: 'TOM-01',
  name: 'Tomate',
  kind: 'purchased',
  active: true,
  unit: reference(ids.kilogram),
};

const general = { id: ids.general, name: 'Almacén general' };
const bar = { id: ids.bar, name: 'Barra' };

export function adjustmentLine(extra: Partial<AdjustmentLine> = {}): AdjustmentLine {
  return {
    line_no: 1,
    item: flourReference,
    presentation: null,
    unit: null,
    capture_label: '',
    captured_quantity: '-2',
    quantity: '-2',
    direction: 'out',
    movement_id: null,
    movement_kind: null,
    movement_label: null,
    stock_after: null,
    value: null,
    value_display: null,
    ...extra,
  };
}

export function adjustment(extra: Partial<Adjustment> = {}): Adjustment {
  return {
    id: controlIDs.adjustment,
    folio: 12,
    kind: 'discrepancy',
    kind_label: 'Ajuste de discrepancia',
    reason: 'physical_shortage',
    reason_label: 'Faltante físico',
    reason_detail: '',
    warehouse: general,
    state: 'pending_authorization',
    state_label: 'Pendiente de autorización',
    notes: '',
    line_count: 1,
    requested_by_name: 'Cajera',
    requested_at: stamp,
    authorized_by_name: null,
    authorized_at: null,
    applied_at: null,
    rejected_by_name: null,
    rejected_at: null,
    rejection_reason: null,
    revision: 1,
    updated_at: stamp,
    costs_visible: true,
    value: null,
    value_display: null,
    authorizable: false,
    lines: [adjustmentLine()],
    warnings: [],
    ...extra,
  };
}

// El mismo ajuste ya aplicado (por otra persona) con el movimiento y la existencia que calculó Go.
export function appliedAdjustment(extra: Partial<Adjustment> = {}): Adjustment {
  return adjustment({
    state: 'applied',
    state_label: 'Aplicado',
    authorized_by_name: 'Gerente',
    authorized_at: stamp,
    applied_at: stamp,
    revision: 2,
    value: '-36.00',
    value_display: '-36.00',
    lines: [
      adjustmentLine({
        movement_id: '00000000-0000-4000-8000-000000000a09',
        movement_kind: 'adjustment_out',
        movement_label: 'Ajuste negativo',
        stock_after: '10.5',
        value: '-36.00',
        value_display: '-36.00',
      }),
    ],
    ...extra,
  });
}

export function adjustmentPage(items: Adjustment[]) {
  return { items, total: items.length, page: 1, page_size: 25, costs_visible: true };
}

export function transfer(extra: Partial<Transfer> = {}): Transfer {
  return {
    id: controlIDs.transfer,
    folio: 4,
    from_warehouse: general,
    to_warehouse: bar,
    occurred_at: stamp,
    actor_name: 'Administrador',
    notes: '',
    line_count: 1,
    created_at: stamp,
    costs_visible: true,
    value: '450.00',
    value_display: '450.00',
    warnings: [
      {
        code: 'NEGATIVE_STOCK',
        message: 'Harina quedó con existencia negativa en «Almacén general» (−37.5 kg).',
        item_id: ids.flour,
      },
    ],
    lines: [
      {
        line_no: 1,
        item: flourReference,
        presentation: { id: ids.sack, name: 'Costal 25 kg', base_quantity: '25', active: true },
        unit: null,
        capture_label: 'Costal 25 kg',
        captured_quantity: '2',
        quantity: '50',
        out_movement_id: '00000000-0000-4000-8000-000000000a12',
        in_movement_id: '00000000-0000-4000-8000-000000000a13',
        from_stock_after: '-37.5',
        to_stock_after: '50',
        value: '450.00',
        value_display: '450.00',
      },
    ],
    ...extra,
  };
}

export function countLine(extra: Partial<CountLine> = {}): CountLine {
  return {
    line_no: 1,
    item: flourReference,
    category_name: 'Abarrotes',
    counted: false,
    counted_quantity: null,
    captured_quantity: null,
    presentation: null,
    unit: null,
    capture_label: null,
    captured_at: null,
    captured_by_name: null,
    theoretical_quantity: null,
    difference: null,
    difference_kind: null,
    current_quantity: null,
    movement_id: null,
    movement_kind: null,
    movement_label: null,
    value: null,
    value_display: null,
    ...extra,
  };
}

// Renglón contado con la diferencia que calculó Go con la existencia teórica a su hora.
export function countedTomato(extra: Partial<CountLine> = {}): CountLine {
  return countLine({
    line_no: 2,
    item: tomatoReference,
    counted: true,
    counted_quantity: '3.25',
    captured_quantity: '3.25',
    capture_label: '',
    captured_at: '2026-11-10T14:05:00Z',
    captured_by_name: 'Ana',
    theoretical_quantity: '4.75',
    difference: '-1.5',
    difference_kind: 'shortage',
    current_quantity: '4.1',
    ...extra,
  });
}

export function count(extra: Partial<Count> = {}): Count {
  return {
    id: controlIDs.count,
    folio: 7,
    warehouse: general,
    scope: 'all',
    scope_label: 'Todos los insumos',
    category: null,
    state: 'open',
    state_label: 'Abierto',
    notes: '',
    line_count: 2,
    counted_lines: 1,
    difference_lines: null,
    created_by_name: 'Ana',
    created_at: stamp,
    updated_by_name: 'Ana',
    updated_at: stamp,
    submitted_by_name: null,
    submitted_at: null,
    approved_by_name: null,
    approved_at: null,
    applied_by_name: null,
    applied_at: null,
    reference_at: null,
    cancelled_by_name: null,
    cancelled_at: null,
    cancel_reason: null,
    revision: 3,
    theoretical_visible: true,
    editable: true,
    submittable: true,
    approvable: false,
    applicable: false,
    cancellable: true,
    costs_visible: false,
    value: null,
    value_display: null,
    lines: [countLine(), countedTomato()],
    warnings: [
      {
        code: 'COUNT_UNCOUNTED_LINES',
        message: '1 insumo no se ha contado: no se ajustará al aplicar el conteo.',
        item_id: null,
      },
    ],
    ...extra,
  };
}

// Lo que ve quien solo cuenta: Go omite teórico, diferencias y existencia actual.
export function blindCount(extra: Partial<Count> = {}): Count {
  const base = count();
  return count({
    theoretical_visible: false,
    lines: base.lines.map((line) => ({
      ...line,
      theoretical_quantity: null,
      difference: null,
      difference_kind: null,
      current_quantity: null,
    })),
    ...extra,
  });
}

export function countPage(counts: Count[]) {
  return { items: counts, total: counts.length, page: 1, page_size: 25 };
}

export function stockLevel(extra: Partial<StockLevel> = {}): StockLevel {
  return {
    item: flourReference,
    warehouse: bar,
    min_stock: '2',
    reorder_point: '4',
    item_min_stock: '5',
    item_reorder_point: null,
    stock_visible: true,
    quantity: '1.5',
    below_minimum: true,
    at_reorder_point: true,
    configured: true,
    revision: 2,
    updated_at: stamp,
    updated_by_name: 'Administrador',
    ...extra,
  };
}

export function alert(extra: Partial<InventoryAlert> = {}): InventoryAlert {
  return {
    type: 'negative_stock',
    type_label: 'Existencia negativa',
    severity: 'critical',
    severity_label: 'Crítica',
    message: 'Tomate tiene existencia negativa en «Almacén general»: −1.5 kg.',
    reference: { kind: 'stock', id: ids.tomato, label: 'Tomate' },
    item: tomatoReference,
    warehouse: general,
    quantity: '-1.5',
    threshold: null,
    count: 1,
    occurred_at: null,
    ...extra,
  };
}

export const sampleAlerts: InventoryAlert[] = [
  alert(),
  alert({
    type: 'count_difference_pending',
    type_label: 'Diferencia de conteo pendiente',
    severity: 'warning',
    severity_label: 'Advertencia',
    message: 'El conteo 7 de «Almacén general» tiene 1 diferencia por aplicar.',
    reference: { kind: 'count', id: controlIDs.count, label: 'Conteo 7' },
    item: null,
    quantity: null,
  }),
  alert({
    type: 'item_without_supplier',
    type_label: 'Insumo sin proveedor',
    severity: 'info',
    severity_label: 'Aviso',
    message: 'Harina no tiene un proveedor activo que lo surta.',
    reference: { kind: 'item', id: ids.flour, label: 'Harina' },
    item: flourReference,
    warehouse: null,
    quantity: null,
  }),
  alert({
    type: 'product_without_recipe',
    type_label: 'Producto sin receta',
    severity: 'info',
    severity_label: 'Aviso',
    message: '«Agua fresca» se vende sin receta: no descuenta inventario.',
    reference: { kind: 'product', id: controlIDs.product, label: 'Agua fresca' },
    item: null,
    warehouse: null,
    quantity: null,
  }),
];

export function alertPage(alerts: InventoryAlert[] = sampleAlerts): AlertPage {
  return {
    items: alerts,
    total: alerts.length,
    page: 1,
    page_size: 100,
    // Conteos de Go (de todas las alertas visibles), no recalculados por la pantalla.
    counts: {
      total: 4,
      critical: 1,
      warning: 1,
      info: 2,
      by_type: {
        negative_stock: 1,
        consumption_failed: 0,
        below_minimum: 0,
        cost_pending: 0,
        count_difference_pending: 1,
        cancellations_unclassified: 0,
        cash_link_expense_corrected: 0,
        reorder_point: 0,
        count_due: 0,
        purchase_order_partial: 0,
        item_without_supplier: 1,
        product_without_recipe: 1,
      },
    },
    costs_visible: true,
    stock_visible: true,
    purchasing_visible: true,
    generated_at: stamp,
  };
}

export function suggestion(extra: Partial<PurchaseSuggestion> = {}): PurchaseSuggestion {
  return {
    item: flourReference,
    category_name: 'Abarrotes',
    reorder_point: '60',
    reorder_source: 'item',
    min_stock: '5',
    stock_visible: true,
    stock: '12.5',
    pending_quantity: '0',
    draft_quantity: '0',
    need_quantity: '47.5',
    below_minimum: false,
    purchasing_visible: true,
    supplier: {
      id: controlIDs.supplier,
      trade_name: 'Molinos del Norte',
      kind: 'formal',
      active: true,
    },
    supplier_code: 'H-25',
    presentation: { id: ids.sack, name: 'Costal 25 kg', base_quantity: '25', active: true },
    unit: null,
    capture_label: 'Costal 25 kg',
    suggested_quantity: '2',
    suggested_base_quantity: '50',
    last_unit_price: '450.000000',
    last_price_label: 'Costal 25 kg',
    last_price_at: stamp,
    ...extra,
  };
}

export function suggestionPage(items: PurchaseSuggestion[]): PurchaseSuggestionPage {
  return {
    items,
    total: items.length,
    page: 1,
    page_size: 100,
    stock_visible: true,
    purchasing_visible: true,
  };
}
