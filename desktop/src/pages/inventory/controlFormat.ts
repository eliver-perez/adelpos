import type { InventoryItem, UnitReference } from '../../services/inventory';
import {
  adjustmentReasonDirection,
  adjustmentReasonLabels,
  wasteReasons,
} from '../../services/inventoryControl';
import type {
  AdjustmentKind,
  AdjustmentReason,
  AdjustmentState,
  AlertReferenceKind,
  AlertSeverity,
  ControlLineInput,
  CountDifferenceKind,
  CountScope,
  CountState,
  CountStateFilter,
} from '../../services/inventoryControl';
import type { PresentationReference } from '../../services/inventoryPurchasing';
import { formatQuantity, formatSignedQuantity } from './inventoryFormat';

// Textos y cuerpos de la fase D de Inventarios (ajustes, mermas, transferencias, conteos, niveles
// por almacén, alertas y sugerencias de compra). Solo traducen claves cerradas de Go, arman
// referencias legibles y copian las cadenas capturadas al cuerpo de la solicitud: ninguna función
// suma, resta, compara ni redondea cantidades o importes. Go calcula existencias, diferencias,
// teóricos, valores y sugerencias.

// Folios legibles, como los nombra Go en sus textos («Conteo 7», «Merma 3»).
export function adjustmentFolio(kind: AdjustmentKind, folio: number) {
  return kind === 'waste' ? `Merma ${folio}` : `Ajuste ${folio}`;
}

export function transferFolio(folio: number) {
  return `Transferencia ${folio}`;
}

export function countFolio(folio: number) {
  return `Conteo ${folio}`;
}

// Tono de la etiqueta de estado (el texto es el state_label de Go).
export function adjustmentStateTone(state: AdjustmentState) {
  if (state === 'applied') {
    return 'tag tag-good';
  }
  if (state === 'pending_authorization') {
    return 'tag tag-warning';
  }
  return 'tag tag-muted';
}

export function countStateTone(state: CountState) {
  if (state === 'applied') {
    return 'tag tag-good';
  }
  if (state === 'submitted' || state === 'approved') {
    return 'tag tag-warning';
  }
  if (state === 'open') {
    return 'tag tag-info';
  }
  return 'tag tag-muted';
}

export const adjustmentStateOptions: { value: AdjustmentState; label: string }[] = [
  { value: 'pending_authorization', label: 'Pendientes de autorización' },
  { value: 'applied', label: 'Aplicados' },
  { value: 'rejected', label: 'Rechazados' },
];

export const adjustmentKindOptions: { value: AdjustmentKind; label: string }[] = [
  { value: 'discrepancy', label: 'Ajustes de discrepancia' },
  { value: 'waste', label: 'Mermas operativas' },
];

export const countStateOptions: { value: CountStateFilter; label: string }[] = [
  { value: 'active', label: 'En curso (abiertos, enviados y aprobados)' },
  { value: 'open', label: 'Abiertos' },
  { value: 'submitted', label: 'Enviados a revisión' },
  { value: 'approved', label: 'Aprobados' },
  { value: 'applied', label: 'Aplicados' },
  { value: 'cancelled', label: 'Cancelados' },
];

export const countScopeOptions: { value: CountScope; label: string }[] = [
  { value: 'all', label: 'Todos los insumos' },
  { value: 'category', label: 'Una categoría' },
  { value: 'items', label: 'Selección de insumos' },
];

// Motivos de lista cerrada que admite cada tipo: una merma solo los de pérdida.
export function reasonOptions(kind: AdjustmentKind): { value: AdjustmentReason; label: string }[] {
  const reasons =
    kind === 'waste' ? wasteReasons : (Object.keys(adjustmentReasonLabels) as AdjustmentReason[]);
  return reasons.map((reason) => ({ value: reason, label: adjustmentReasonLabels[reason] }));
}

export type LineDirection = 'out' | 'in';

// Sentido de un renglón: el que impone el motivo o, si admite ambos, el elegido. Una merma siempre
// es salida.
export function effectiveDirection(
  kind: AdjustmentKind,
  reason: AdjustmentReason | '',
  chosen: LineDirection,
): LineDirection {
  if (kind === 'waste') {
    return 'out';
  }
  if (reason === '') {
    return chosen;
  }
  const allowed = adjustmentReasonDirection[reason];
  return allowed === 'both' ? chosen : allowed;
}

export const directionLabels: Record<LineDirection, string> = {
  out: 'Faltante (salida)',
  in: 'Sobrante (entrada)',
};

// Renglón capturado de un ajuste, una merma o una transferencia. measure vacío es la unidad base
// del insumo; «unit:<id>» otra unidad de su dimensión y «presentation:<id>» una presentación. La
// cantidad se captura siempre sin signo: el sentido lo indica «Faltante» o «Sobrante».
export interface ControlLineDraft {
  key: number;
  item: InventoryItem | null;
  measure: string;
  quantity: string;
  direction: LineDirection;
}

export function emptyControlLine(key: number): ControlLineDraft {
  return { key, item: null, measure: '', quantity: '', direction: 'out' };
}

// Unidad o presentación capturada para el cuerpo de Go.
export function measureInput(measure: string): { unit_id?: string; presentation_id?: string } {
  if (measure.startsWith('presentation:')) {
    return { presentation_id: measure.slice('presentation:'.length) };
  }
  if (measure.startsWith('unit:')) {
    return { unit_id: measure.slice('unit:'.length) };
  }
  return {};
}

// Cuerpo del renglón para Go con la cadena tal como se capturó. En un ajuste la salida antepone
// el signo «-» al texto (Go valida el sentido contra el motivo); una transferencia no lleva signo.
export function controlLineInput(line: ControlLineDraft, signed: boolean): ControlLineInput {
  const quantity = signed && line.direction === 'out' ? `-${line.quantity}` : line.quantity;
  return { item_id: line.item?.id ?? '', quantity, ...measureInput(line.measure) };
}

// Lo capturado en un renglón tal como lo guardó Go: «2 · Caja con 6», «1.5 kg», «−300 ml».
export function capturedText(
  quantity: string | null,
  presentation: PresentationReference | null,
  unit: UnitReference | null,
  baseUnit: UnitReference,
  signed = false,
) {
  const format = signed ? formatSignedQuantity : formatQuantity;
  if (presentation) {
    return `${format(quantity)} · ${presentation.name}`;
  }
  return format(quantity, unit ?? baseUnit);
}

export const differenceLabels: Record<CountDifferenceKind, string> = {
  surplus: 'Sobrante',
  shortage: 'Faltante',
  none: 'Sin diferencia',
};

export function differenceTone(kind: CountDifferenceKind | null) {
  if (kind === 'surplus') {
    return 'tag tag-info';
  }
  if (kind === 'shortage') {
    return 'tag tag-warning';
  }
  return 'tag tag-good';
}

export function severityTone(severity: AlertSeverity) {
  if (severity === 'critical') {
    return 'tag tag-danger';
  }
  if (severity === 'warning') {
    return 'tag tag-warning';
  }
  return 'tag tag-info';
}

export const severityOptions: { value: AlertSeverity | ''; label: string }[] = [
  { value: '', label: 'Todas' },
  { value: 'critical', label: 'Críticas' },
  { value: 'warning', label: 'Advertencias' },
  { value: 'info', label: 'Avisos' },
];

// Texto del botón que abre el destino de una alerta.
export const alertActionLabels: Record<AlertReferenceKind, string> = {
  stock: 'Ver existencias',
  item: 'Abrir insumo',
  product: 'Ver recetas',
  count: 'Abrir conteo',
  warehouse: 'Ver conteos del almacén',
  purchase_order: 'Abrir orden',
  receipt: 'Abrir recepción',
  cancellations: 'Clasificar',
  consumption_requests: 'Ver solicitudes',
};
