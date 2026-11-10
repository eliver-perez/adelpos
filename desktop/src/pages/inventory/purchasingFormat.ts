import { isoToDisplay } from '../../components/catalog/DateField';
import { APIError } from '../../services/api';
import type { InventoryItem } from '../../services/inventory';
import type {
  CashLinkStatus,
  PurchaseLine,
  PurchaseLineInput,
  PurchaseOrderStateFilter,
  PurchaseTaxRate,
} from '../../services/inventoryPurchasing';
import { isZeroMoney } from '../../services/payments';
import type { DecimalShape } from '../../services/numericInput';

// Textos y cuerpos de la fase C de Inventarios (proveedores, compras, recepciones, devoluciones y
// vínculos con gastos de caja). Solo traducen claves cerradas de Go, arman referencias legibles y
// copian las cadenas capturadas al cuerpo de la solicitud: ninguna función suma, prorratea,
// compara ni redondea cantidades o importes. Go calcula importes, IVA, totales y costos.

// Leer compras: cualquiera de los seis permisos de compras (Go no tiene «purchases.read»).
export const purchasingReadPermissions = [
  'inventory.suppliers.manage',
  'inventory.purchases.create',
  'inventory.purchases.authorize',
  'inventory.purchases.receive',
  'inventory.supplier_returns.record',
  'inventory.purchases.link_cash',
];

export function canReadPurchasing(can: (permission: string) => boolean) {
  return purchasingReadPermissions.some((permission) => can(permission));
}

// Vincular o desvincular gastos exige además leer movimientos de efectivo (Go aplica también las
// reglas de turno de otra caja y de turnos cerrados).
export function canLinkCash(can: (permission: string) => boolean) {
  return can('inventory.purchases.link_cash') && can('cash.movements.read');
}

// Nombres de los permisos de compras (descripciones de Go) para explicar qué permiso falta.
const purchasingPermissionLabels: Record<string, string> = {
  'inventory.suppliers.manage': 'Administrar proveedores',
  'inventory.purchases.create': 'Crear órdenes de compra',
  'inventory.purchases.authorize': 'Emitir (autorizar), cancelar y cerrar órdenes de compra',
  'inventory.purchases.receive': 'Recibir mercancía',
  'inventory.supplier_returns.record': 'Registrar devoluciones a proveedores',
  'inventory.purchases.link_cash': 'Vincular compras con gastos de caja',
  'cash.movements.read': 'Consultar movimientos de efectivo',
};

export function purchasingPermissionName(key: string) {
  return purchasingPermissionLabels[key] ?? key;
}

// Precio unitario sin IVA: hasta 6 decimales y 1,000,000 (límites de Go).
export const purchasePriceShape: DecimalShape = { decimals: 6, integerDigits: 7 };

export const taxRateOptions: { value: `${PurchaseTaxRate}`; label: string }[] = [
  { value: '16', label: 'IVA 16 %' },
  { value: '8', label: 'IVA 8 %' },
  { value: '0', label: 'IVA 0 %' },
];

export function taxRateValue(text: string): PurchaseTaxRate {
  if (text === '8') {
    return 8;
  }
  if (text === '0') {
    return 0;
  }
  return 16;
}

// Folios legibles (Go nombra igual las recepciones en los movimientos: «Recepción 12 · …»).
export function orderFolio(folio: number | null | undefined) {
  return folio === null || folio === undefined ? 'Orden sin folio' : `Orden ${folio}`;
}

export function receiptFolio(folio: number) {
  return `Recepción ${folio}`;
}

export function returnFolio(folio: number) {
  return `Devolución ${folio}`;
}

// Fecha de la orden (AAAA-MM-DD) como dd/mm/aaaa.
export function orderDateText(value: string | null | undefined) {
  if (!value) {
    return '—';
  }
  return isoToDisplay(value) || value;
}

export const orderStateOptions: { value: PurchaseOrderStateFilter; label: string }[] = [
  { value: 'open', label: 'Abiertas (emitidas y parciales)' },
  { value: 'draft', label: 'Borrador' },
  { value: 'issued', label: 'Emitida' },
  { value: 'partially_received', label: 'Parcialmente recibida' },
  { value: 'received', label: 'Recibida' },
  { value: 'cancelled', label: 'Cancelada' },
  { value: 'closed', label: 'Cerrada administrativamente' },
];

// Etiqueta del estado de una orden con su tono (el texto es el state_label de Go).
export function orderStateTone(state: string) {
  if (state === 'received') {
    return 'tag tag-good';
  }
  if (state === 'partially_received' || state === 'issued') {
    return 'tag tag-info';
  }
  if (state === 'cancelled' || state === 'closed') {
    return 'tag tag-muted';
  }
  return 'tag';
}

// Un vínculo cuyo gasto se corrigió o regularizó después requiere revisión.
export function cashLinkTone(status: CashLinkStatus) {
  if (status === 'active') {
    return 'tag tag-good';
  }
  if (status === 'unlinked') {
    return 'tag tag-muted';
  }
  return 'tag tag-warning';
}

// Un texto decimal que vale cero («0», «0.00», «0.000000»), reconocido por su forma.
export function isZeroText(value: string | null | undefined) {
  if (value === null || value === undefined) {
    return true;
  }
  return isZeroMoney(value.trim() === '' ? '0' : value.trim());
}

// Resultado incierto de una escritura (sin conexión o error del servidor): el pendiente se
// recupera con el mismo identificador desde el aviso de la sección; mientras tanto el formulario
// no vuelve a enviar con otro identificador.
export function uncertainResult(problem: unknown) {
  return (
    problem instanceof APIError && (problem.code === 'CONNECTION_ERROR' || problem.status >= 500)
  );
}

// Renglón capturado de una orden o de una recepción directa. measure vacío es la unidad base del
// insumo; «unit:<id>» otra unidad de su dimensión y «presentation:<id>» una presentación.
// computed guarda los importes que Go calculó para el renglón guardado; se borra al editarlo.
export interface PurchaseLineDraft {
  key: number;
  itemID: string;
  item: InventoryItem | null;
  // Nombre del insumo del renglón guardado mientras se consulta el insumo completo.
  itemLabel: string;
  measure: string;
  quantity: string;
  unitPrice: string;
  discount: string;
  taxRate: PurchaseTaxRate;
  computed: PurchaseLine | null;
}

export function emptyPurchaseLine(key: number): PurchaseLineDraft {
  return {
    key,
    itemID: '',
    item: null,
    itemLabel: '',
    measure: '',
    quantity: '',
    unitPrice: '',
    discount: '',
    taxRate: 16,
    computed: null,
  };
}

// Borrador de un renglón guardado, con los importes de Go.
export function purchaseLineDraft(line: PurchaseLine, key: number): PurchaseLineDraft {
  let measure = '';
  if (line.presentation) {
    measure = `presentation:${line.presentation.id}`;
  } else if (line.unit && line.unit.id !== line.item.unit.id) {
    measure = `unit:${line.unit.id}`;
  }
  return {
    key,
    itemID: line.item.id,
    item: null,
    itemLabel: `${line.item.name} · ${line.item.code}`,
    measure,
    quantity: line.quantity,
    unitPrice: line.unit_price,
    discount: isZeroText(line.discount) ? '' : line.discount,
    taxRate: line.tax_rate,
    computed: line,
  };
}

// Cuerpo del renglón para Go con las cadenas tal como se capturaron; el descuento vacío es «0».
export function purchaseLineInput(line: PurchaseLineDraft): PurchaseLineInput {
  const input: PurchaseLineInput = {
    item_id: line.itemID,
    quantity: line.quantity,
    unit_price: line.unitPrice,
    tax_rate: line.taxRate,
  };
  if (line.discount.trim() !== '') {
    input.discount = line.discount;
  }
  if (line.measure.startsWith('presentation:')) {
    input.presentation_id = line.measure.slice('presentation:'.length);
  } else if (line.measure.startsWith('unit:')) {
    input.unit_id = line.measure.slice('unit:'.length);
  }
  return input;
}

// Lo que identifica un renglón capturado (sin los importes calculados), para saber si cambió.
export function lineSignature(line: PurchaseLineDraft) {
  return JSON.stringify([
    line.itemID,
    line.measure,
    line.quantity,
    line.unitPrice,
    line.discount,
    line.taxRate,
  ]);
}

export function optionalText(value: string) {
  return value.trim() === '' ? undefined : value;
}
