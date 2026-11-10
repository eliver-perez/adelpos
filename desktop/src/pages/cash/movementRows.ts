import { isoToDisplay } from '../../components/catalog/DateField';
import {
  cashOperationKindLabels,
  cashOperationStatusLabels,
  currencyLabels,
} from '../../services/cash';
import type { CashOperation } from '../../services/cash';
import {
  formatDollars,
  formatMoney,
  isZeroMoney,
  movementKindLabels,
} from '../../services/payments';
import type { ShiftMovement } from '../../services/payments';

// Filas de la vista de movimientos: cobros, devoluciones y correcciones de pago del turno junto
// con sus operaciones de efectivo. Solo se reordenan y se rotulan: los importes son las cadenas
// que entregó Go, sin sumas ni conversiones, y no se muestran identificadores internos.

export type MovementType =
  'payment' | 'refund' | 'payment_correction' | 'income' | 'withdrawal' | 'expense' | 'correction';

export const movementTypeLabels: Record<MovementType, string> = {
  payment: 'Cobros',
  refund: 'Devoluciones',
  payment_correction: 'Correcciones de pago',
  income: 'Ingresos',
  withdrawal: 'Retiros',
  expense: 'Gastos',
  correction: 'Correcciones de efectivo',
};

export interface MovementRow {
  key: string;
  at: string;
  type: MovementType;
  typeLabel: string;
  concept: string;
  method: string;
  amount: string;
  direction: string;
  user: string;
  reference: string;
  status: string;
  movement?: ShiftMovement;
  operation?: CashOperation;
}

function movementType(kind: string): MovementType {
  if (kind === 'refund') {
    return 'refund';
  }
  if (kind === 'correction_reversal') {
    return 'payment_correction';
  }
  return 'payment';
}

const movementConcepts: Record<MovementType, string> = {
  payment: 'Cobro de cuenta',
  refund: 'Devolución al cliente',
  payment_correction: 'Reverso por corrección del método',
  income: 'Ingreso',
  withdrawal: 'Retiro',
  expense: 'Gasto',
  correction: 'Corrección',
};

function movementRow(movement: ShiftMovement): MovementRow {
  const type = movementType(movement.kind);
  let method = movement.method_name;
  if (movement.usd_in && !isZeroMoney(movement.usd_in)) {
    method += ` · ${formatDollars(movement.usd_in)} recibidos`;
  }
  return {
    key: `movement-${movement.id}`,
    at: movement.created_at,
    type,
    typeLabel: movementKindLabels[movement.kind] ?? 'Cobro',
    concept: movementConcepts[type],
    method,
    amount: formatMoney(movement.net_amount),
    direction: '',
    user: movement.actor_name,
    reference: `Cuenta #${movement.folio}`,
    status: 'Registrado',
    movement,
  };
}

function operationRow(operation: CashOperation): MovementRow {
  const amount = formatMoney(operation.amount, operation.currency);
  return {
    key: `operation-${operation.id}`,
    at: operation.created_at,
    type: operation.kind,
    typeLabel: operation.regularization
      ? 'Regularización'
      : cashOperationKindLabels[operation.kind],
    concept: operation.concept,
    method: currencyLabels[operation.currency],
    amount,
    direction: operation.direction === 'in' ? 'Entrada' : 'Salida',
    user: operation.actor_name,
    reference: operation.reference || '—',
    status: cashOperationStatusLabels[operation.status],
    operation,
  };
}

// Une y ordena por fecha y hora reales (no es aritmética de dinero).
export function movementRows(
  movements: ShiftMovement[],
  operations: CashOperation[],
): MovementRow[] {
  const rows = [...movements.map(movementRow), ...operations.map(operationRow)];
  return rows.sort((first, second) => {
    const difference = Date.parse(first.at) - Date.parse(second.at);
    if (difference !== 0) {
      return difference;
    }
    return first.key.localeCompare(second.key);
  });
}

// «Intentos rechazados» (etapa 7.1) no es un tipo de movimiento: muestra los retiros y gastos
// rechazados por efectivo insuficiente, solo con cash.expected.read.
export const rejectedFilter = 'rejected';

export interface MovementFilters {
  type: MovementType | typeof rejectedFilter | '';
  user: string;
}

export function filterRows(rows: MovementRow[], filters: MovementFilters) {
  return rows.filter((row) => {
    if (filters.type && row.type !== filters.type) {
      return false;
    }
    return !filters.user || row.user === filters.user;
  });
}

// Usuarios presentes en las filas, para el filtro por usuario.
export function rowUsers(rows: MovementRow[]) {
  return [...new Set(rows.map((row) => row.user))].sort((first, second) =>
    first.localeCompare(second, 'es'),
  );
}

// Fecha comercial (AAAA-MM-DD que entrega Go) mostrada como dd/mm/aaaa.
export function businessDate(value: string | null | undefined) {
  if (!value) {
    return '—';
  }
  return isoToDisplay(value) || value;
}

export function serverDate(value: string, timezone: string) {
  return new Intl.DateTimeFormat('es-MX', { dateStyle: 'short', timeZone: timezone }).format(
    new Date(value),
  );
}

export function serverTime(value: string, timezone: string) {
  return new Intl.DateTimeFormat('es-MX', { timeStyle: 'short', timeZone: timezone }).format(
    new Date(value),
  );
}
