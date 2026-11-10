import type { InventoryItem } from '../../services/inventory';
import type {
  CashExpense,
  CashLink,
  PurchaseOrder,
  PurchaseOrderLine,
  Receipt,
  ReceiptLine,
  Supplier,
  SupplierDetail,
  SupplierItem,
} from '../../services/inventoryPurchasing';
import type { ItemReference } from '../../services/inventoryRecipes';
import { allInventoryPermissions, ids, item, reference } from './inventoryTestData';

// Datos simulados de Go para las pruebas de la fase C (proveedores, órdenes de compra,
// recepciones, devoluciones y vínculos con gastos de caja). Los importes, cantidades y estados son
// los que entregaría Go, ya calculados: la interfaz solo los muestra. Algunos totales se eligieron
// a propósito sin relación aritmética con los renglones para comprobar que la pantalla muestra lo
// que llega y no suma por su cuenta.

export const purchasingIDs = {
  supplier: '00000000-0000-4000-8000-000000000901',
  otherSupplier: '00000000-0000-4000-8000-000000000902',
  soda: '00000000-0000-4000-8000-000000000911',
  box: '00000000-0000-4000-8000-000000000912',
  order: '00000000-0000-4000-8000-000000000921',
  draft: '00000000-0000-4000-8000-000000000922',
  receipt: '00000000-0000-4000-8000-000000000931',
  direct: '00000000-0000-4000-8000-000000000932',
  expense: '00000000-0000-4000-8000-000000000941',
  dollarExpense: '00000000-0000-4000-8000-000000000942',
  link: '00000000-0000-4000-8000-000000000951',
  shift: '00000000-0000-4000-8000-000000000961',
  register: '00000000-0000-4000-8000-000000000962',
  movement: '00000000-0000-4000-8000-000000000971',
};

const stamp = '2026-11-10T15:00:00Z';

// Lectura de compras con todos los permisos de la fase C y lectura de movimientos de efectivo.
export const purchasingPermissions = [
  ...allInventoryPermissions,
  'inventory.suppliers.manage',
  'inventory.purchases.create',
  'inventory.purchases.authorize',
  'inventory.purchases.receive',
  'inventory.supplier_returns.record',
  'inventory.purchases.link_cash',
  'cash.movements.read',
];

export const soda: InventoryItem = item({
  id: purchasingIDs.soda,
  code: 'REF-01',
  name: 'Refresco',
  category_id: null,
  category_name: null,
  base_unit: reference(ids.piece),
  reference_cost: null,
  reference_cost_display: null,
  min_stock: null,
  presentations: [
    {
      id: purchasingIDs.box,
      item_id: purchasingIDs.soda,
      name: 'Caja con 24',
      base_quantity: '24',
      barcode: null,
      active: true,
      purchase_default: true,
      revision: 1,
      updated_at: stamp,
    },
  ],
});

export const sodaReference: ItemReference = {
  id: purchasingIDs.soda,
  code: 'REF-01',
  name: 'Refresco',
  kind: 'purchased',
  active: true,
  unit: reference(ids.piece),
};

const supplierReference = {
  id: purchasingIDs.supplier,
  trade_name: 'Distribuidora del Norte',
  active: true,
};

const boxReference = {
  id: purchasingIDs.box,
  name: 'Caja con 24',
  base_quantity: '24',
  active: true,
};

export function supplierSummary(extra: Partial<Supplier> = {}): Supplier {
  return {
    id: purchasingIDs.supplier,
    trade_name: 'Distribuidora del Norte',
    legal_name: 'Distribuidora del Norte, S.A. de C.V.',
    rfc: 'DNO010203AB1',
    phone: '868 555 0101',
    email: 'ventas@dnorte.mx',
    address: 'Calle 5 #120',
    contact: 'Laura Pérez',
    notes: '',
    active: true,
    item_count: 1,
    has_purchases: true,
    last_receipt_at: stamp,
    revision: 4,
    created_at: stamp,
    updated_at: stamp,
    updated_by_name: 'Administrador',
    ...extra,
  };
}

export function supplierItem(extra: Partial<SupplierItem> = {}): SupplierItem {
  return {
    item: sodaReference,
    presentation: boxReference,
    supplier_code: 'R-24',
    preferred: true,
    last_unit_price: '250.000000',
    last_price_unit_label: 'Caja con 24',
    last_price_at: stamp,
    last_receipt_folio: 7,
    revision: 2,
    updated_at: stamp,
    ...extra,
  };
}

export function supplierDetail(extra: Partial<SupplierDetail> = {}): SupplierDetail {
  return {
    ...supplierSummary(),
    items: [supplierItem()],
    history: {
      order_count: 1,
      receipt_count: 2,
      return_count: 0,
      receipts_total: '6846.00',
      returns_value: '0.00',
      orders: [],
      receipts: [],
      returns: [],
    },
    ...extra,
  };
}

export function supplierPage(suppliers: Supplier[]) {
  return { items: suppliers, total: suppliers.length, page: 1, page_size: 25 };
}

export function orderLine(extra: Partial<PurchaseOrderLine> = {}): PurchaseOrderLine {
  return {
    line_no: 1,
    item: sodaReference,
    presentation: boxReference,
    unit: null,
    capture_label: 'Caja con 24',
    quantity: '20',
    base_quantity: '480',
    unit_price: '250.000000',
    gross: '5000.00',
    discount: '50.00',
    subtotal: '4950.00',
    tax_rate: 16,
    tax: '792.00',
    total: '5742.00',
    received_quantity: '0',
    pending_quantity: '20',
    excess_quantity: '0',
    received_base_quantity: '0',
    pending_base_quantity: '480',
    fully_received: false,
    ...extra,
  };
}

export function purchaseOrder(extra: Partial<PurchaseOrder> = {}): PurchaseOrder {
  return {
    id: purchasingIDs.order,
    folio: 12,
    supplier: supplierReference,
    state: 'issued',
    state_label: 'Emitida',
    order_date: '2026-11-10',
    expected_warehouse: { id: ids.general, name: 'Almacén general' },
    subtotal: '4950.00',
    discount: '50.00',
    tax: '792.00',
    total: '5742.00',
    line_count: 1,
    receipt_count: 0,
    created_by_name: 'Administrador',
    issued_at: stamp,
    revision: 2,
    created_at: stamp,
    updated_at: stamp,
    notes: '',
    updated_by_name: 'Administrador',
    issued_by_name: 'Administrador',
    cancelled_at: null,
    cancelled_by_name: null,
    cancel_reason: null,
    closed_at: null,
    closed_by_name: null,
    close_reason: null,
    editable: false,
    issuable: false,
    cancellable: true,
    closable: true,
    receivable: true,
    lines: [orderLine()],
    receipts: [],
    ...extra,
  };
}

// Borrador recién guardado: los totales son los que calculó Go (deliberadamente distintos de
// cualquier suma que pudiera hacer la pantalla).
export function draftOrder(extra: Partial<PurchaseOrder> = {}): PurchaseOrder {
  return purchaseOrder({
    id: purchasingIDs.draft,
    folio: 13,
    state: 'draft',
    state_label: 'Borrador',
    issued_at: null,
    issued_by_name: null,
    revision: 1,
    editable: true,
    issuable: true,
    cancellable: true,
    closable: false,
    receivable: false,
    subtotal: '1234.56',
    discount: '7.89',
    tax: '98.76',
    total: '1333.32',
    lines: [
      orderLine({
        quantity: '3',
        base_quantity: '72',
        unit_price: '410.000000',
        gross: '1230.00',
        discount: '0.00',
        subtotal: '1230.00',
        tax_rate: 8,
        tax: '98.40',
        total: '1328.40',
        pending_quantity: '3',
        pending_base_quantity: '72',
      }),
    ],
    ...extra,
  });
}

export function orderPage(orders: PurchaseOrder[]) {
  return { items: orders, total: orders.length, page: 1, page_size: 25 };
}

export function receiptLine(extra: Partial<ReceiptLine> = {}): ReceiptLine {
  return {
    line_no: 1,
    item: sodaReference,
    presentation: boxReference,
    unit: null,
    capture_label: 'Caja con 24',
    quantity: '12',
    base_quantity: '288',
    unit_price: '250.000000',
    gross: '3000.00',
    discount: '30.00',
    subtotal: '2970.00',
    tax_rate: 16,
    tax: '475.20',
    total: '3445.20',
    order_line_no: 1,
    ordered_quantity: '20',
    received_before_quantity: '0',
    pending_after_quantity: '8',
    returned_quantity: '0',
    returnable_quantity: '12',
    returned_base_quantity: '0',
    returnable_base_quantity: '288',
    movement_id: purchasingIDs.movement,
    stock_after: '288',
    costs_visible: true,
    inventory_value: '3445.200000',
    inventory_value_display: '3445.20',
    unit_cost: '11.9625000000',
    unit_cost_display: '11.9625',
    ...extra,
  };
}

export function cashExpense(extra: Partial<CashExpense> = {}): CashExpense {
  return {
    id: purchasingIDs.expense,
    shift_id: purchasingIDs.shift,
    register_id: purchasingIDs.register,
    register_name: 'Caja 1',
    shift_state: 'open',
    shift_business_date: '2026-11-10',
    currency: 'MXN',
    amount: '850.00',
    concept: 'Compra de suministros',
    category_name: 'Proveedores',
    reference: 'Factura 88',
    created_at: stamp,
    actor_name: 'Cajero',
    status: 'recorded',
    status_label: 'Registrado',
    corrected_at: null,
    linked_amount: '0.00',
    available_amount: '850.00',
    linked_to_receipt: '0.00',
    linkable: true,
    unavailable_reason: null,
    unavailable_label: null,
    ...extra,
  };
}

export function cashLink(extra: Partial<CashLink> = {}): CashLink {
  return {
    id: purchasingIDs.link,
    receipt_id: purchasingIDs.receipt,
    receipt_folio: 5,
    cash_operation_id: purchasingIDs.expense,
    amount: '500.00',
    notes: '',
    linked_at: stamp,
    linked_by_name: 'Administrador',
    status: 'active',
    status_label: 'Vigente',
    active: true,
    unlinked_at: null,
    unlinked_by_name: null,
    unlink_reason: null,
    expense_visible: true,
    expense: cashExpense({ linked_amount: '500.00', available_amount: '350.00' }),
    ...extra,
  };
}

export function receipt(extra: Partial<Receipt> = {}): Receipt {
  return {
    id: purchasingIDs.receipt,
    folio: 5,
    supplier: supplierReference,
    purchase_order_id: purchasingIDs.order,
    purchase_order_folio: 12,
    direct: false,
    warehouse: { id: ids.general, name: 'Almacén general' },
    occurred_at: stamp,
    received_by_name: 'Almacenista',
    supplier_reference: 'F-1001',
    tax_in_cost: true,
    subtotal: '2970.00',
    discount: '30.00',
    tax: '475.20',
    total: '3445.20',
    line_count: 1,
    excess: false,
    linked_amount: '0.00',
    linkable_amount: '3445.20',
    cash_link_attention: false,
    returned_value: '0.00',
    costs_visible: true,
    inventory_value: '3445.200000',
    inventory_value_display: '3445.20',
    created_at: stamp,
    notes: '',
    lines: [receiptLine()],
    returns: [],
    cash_links: [],
    warnings: [],
    ...extra,
  };
}

export function receiptPage(receipts: Receipt[], costsVisible = true) {
  return {
    items: receipts,
    total: receipts.length,
    page: 1,
    page_size: 25,
    costs_visible: costsVisible,
  };
}

// Recepción vista sin inventory.costs.read: Go entrega el costeo en null.
export function receiptWithoutCosts(extra: Partial<Receipt> = {}): Receipt {
  return receipt({
    costs_visible: false,
    inventory_value: null,
    inventory_value_display: null,
    lines: [
      receiptLine({
        costs_visible: false,
        inventory_value: null,
        inventory_value_display: null,
        unit_cost: null,
        unit_cost_display: null,
      }),
    ],
    ...extra,
  });
}
