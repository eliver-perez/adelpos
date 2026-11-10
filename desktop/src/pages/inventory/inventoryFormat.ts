import type { MovementKind, StockState, UnitReference } from '../../services/inventory';

// Presentación de cantidades de inventario (etapa 8). Go entrega cada cantidad como cadena decimal
// en la unidad base del insumo («20», «0.25», «-1.5») y es la autoridad de toda conversión. Aquí
// solo se agrega el separador de miles, el menos tipográfico y la abreviatura de la unidad
// operando sobre el texto, igual que services/money.ts: nunca se convierte a number, no se
// redondea, no se suma ni se convierte entre unidades.

const quantityPattern = /^(-?)(\d+)(?:\.(\d+))?$/;
const screenMinus = '−';
const emptyQuantity = '—';

interface ParsedQuantity {
  negative: boolean;
  zero: boolean;
  text: string;
}

function groupThousands(integer: string) {
  return integer.replace(/\B(?=(\d{3})+(?!\d))/g, ',');
}

function parseQuantity(value: string): ParsedQuantity | null {
  const match = quantityPattern.exec(value);
  if (!match) {
    return null;
  }
  const integer = (match[2] ?? '').replace(/^0+(?=\d)/, '');
  const fraction = match[3] ?? '';
  const zero = /^0+$/.test(integer) && /^0*$/.test(fraction);
  const decimals = fraction === '' ? '' : `.${fraction}`;
  return {
    negative: match[1] === '-' && !zero,
    zero,
    text: `${groupThousands(integer)}${decimals}`,
  };
}

// La cifra y su unidad van unidas por un espacio de no separación para no partirse en dos líneas.
function withUnit(text: string, unit: Pick<UnitReference, 'code'> | null | undefined) {
  if (!unit) {
    return text;
  }
  return `${text}\u00a0${unit.code}`;
}

// Cantidad con su unidad: «25,000 g», «−1.5 kg», «—» si falta. Un texto que no es un decimal se
// muestra tal como llegó.
export function formatQuantity(
  value: string | null | undefined,
  unit?: Pick<UnitReference, 'code'> | null,
) {
  if (value === null || value === undefined || value === '') {
    return emptyQuantity;
  }
  const parsed = parseQuantity(value);
  if (!parsed) {
    return withUnit(value, unit);
  }
  const sign = parsed.negative ? screenMinus : '';
  return withUnit(`${sign}${parsed.text}`, unit);
}

// Cantidad de un movimiento con signo explícito: entradas «+20 kg», salidas «−3 kg» y «0 kg».
export function formatSignedQuantity(
  value: string | null | undefined,
  unit?: Pick<UnitReference, 'code'> | null,
) {
  if (value === null || value === undefined || value === '') {
    return emptyQuantity;
  }
  const parsed = parseQuantity(value);
  if (!parsed) {
    return withUnit(value, unit);
  }
  let sign = '+';
  if (parsed.negative) {
    sign = screenMinus;
  } else if (parsed.zero) {
    sign = '';
  }
  return withUnit(`${sign}${parsed.text}`, unit);
}

// Nombres de los permisos de inventario (fases A, B y D), iguales a las descripciones de Go
// (inventory.Permissions), para explicar en pantalla qué permiso falta.
const permissionLabels: Record<string, string> = {
  'inventory.read': 'Consultar inventario (sección y resumen)',
  'inventory.items.manage': 'Administrar insumos, unidades y presentaciones',
  'inventory.costs.read': 'Consultar costos (unitarios, valores, costo teórico)',
  'inventory.costs.manage': 'Administrar costos de referencia y costos de existencias iniciales',
  'inventory.stock.read': 'Consultar existencias (cantidades, sin costos)',
  'inventory.warehouses.manage': 'Administrar almacenes y su asignación a estaciones',
  'inventory.movements.read':
    'Consultar movimientos (cantidades; valores solo con consulta de costos)',
  'inventory.adjustments.authorize': 'Autorizar ajustes y corregir clasificaciones vencidas',
  'inventory.valuation.read':
    'Consultar valuaciones (valor total del inventario por insumo/almacén)',
  'inventory.settings.manage': 'Administrar parámetros de inventario',
  'inventory.recipes.manage': 'Administrar recetas y porciones habituales',
  'inventory.subrecipes.manage': 'Administrar subrecetas (preparaciones)',
  'inventory.production.record': 'Registrar elaboraciones de subrecetas almacenables',
  'inventory.waste.record': 'Registrar mermas y clasificar cancelaciones',
  'inventory.adjustments.record': 'Registrar ajustes de inventario',
  'inventory.counts.perform': 'Realizar conteos físicos',
  'inventory.counts.approve': 'Aprobar y aplicar conteos',
  'inventory.transfers.record': 'Transferir mercancía entre almacenes',
  'inventory.purchases.create': 'Crear órdenes de compra',
  'settings.features.manage': 'Activar o desactivar funciones del establecimiento',
};

export function inventoryPermissionName(key: string) {
  return permissionLabels[key] ?? key;
}

// Tipos de movimiento para el filtro del libro, con los mismos nombres que entrega Go en
// kind_label. El listado siempre muestra la etiqueta que trae cada movimiento.
export const movementKindOptions: { value: MovementKind; label: string }[] = [
  { value: 'initial', label: 'Existencia inicial' },
  { value: 'receipt', label: 'Entrada por compra' },
  { value: 'consumption', label: 'Consumo por preparación' },
  { value: 'consumption_reversal', label: 'Reversión de consumo' },
  { value: 'waste', label: 'Merma' },
  { value: 'adjustment_in', label: 'Ajuste positivo' },
  { value: 'adjustment_out', label: 'Ajuste negativo' },
  { value: 'transfer_out', label: 'Transferencia (salida)' },
  { value: 'transfer_in', label: 'Transferencia (entrada)' },
  { value: 'supplier_return', label: 'Devolución a proveedor' },
  { value: 'production_in', label: 'Elaboración (entrada)' },
  { value: 'production_out', label: 'Elaboración (consumo)' },
  { value: 'count_in', label: 'Sobrante de conteo' },
  { value: 'count_out', label: 'Faltante de conteo' },
  { value: 'cost_adjustment', label: 'Ajuste de costo' },
];

// Orígenes de movimiento (Go: sourceKindLabels), con los de la fase D: ajustes, mermas,
// transferencias y conteos.
export const sourceKindOptions: { value: string; label: string }[] = [
  { value: 'initial', label: 'Existencia inicial' },
  { value: 'consumption', label: 'Liberación a cocina' },
  { value: 'amendment', label: 'Modificación de partida' },
  { value: 'cancellation', label: 'Cancelación de partida' },
  { value: 'production', label: 'Elaboración' },
  { value: 'receipt', label: 'Recepción de mercancía' },
  { value: 'supplier_return', label: 'Devolución a proveedor' },
  { value: 'adjustment', label: 'Ajuste de inventario' },
  { value: 'waste', label: 'Merma operativa' },
  { value: 'transfer', label: 'Transferencia entre almacenes' },
  { value: 'count', label: 'Conteo físico' },
];

export const stockStateOptions: { value: StockState; label: string }[] = [
  { value: 'negative', label: 'Negativas' },
  { value: 'zero', label: 'En cero' },
  { value: 'positive', label: 'Positivas' },
  { value: 'below_minimum', label: 'Bajo mínimo' },
  { value: 'reorder', label: 'En punto de reabastecimiento' },
];
