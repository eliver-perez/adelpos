import type { Operation } from '../../services/api';
import { toWireAmount } from '../../services/amountInput';
import type { CashCurrency, CashOperationInput } from '../../services/cash';

// Ingresos, retiros y gastos del turno: definición de cada tipo y armado del cuerpo que valida Go.

export type OutflowKind = 'withdrawal' | 'expense';
export type CashEntryKind = 'income' | OutflowKind;

export interface CashEntryDefinition {
  title: string;
  verb: string;
  operation: Operation;
  permission: string;
  // Solo las salidas exigen autorización (el permiso propio basta, §0-B).
  outflow: boolean;
}

export const cashEntries: Record<CashEntryKind, CashEntryDefinition> = {
  income: {
    title: 'Ingreso de efectivo',
    verb: 'Registrar ingreso',
    operation: 'payments_shift_income',
    permission: 'cash.income.record',
    outflow: false,
  },
  withdrawal: {
    title: 'Retiro de efectivo',
    verb: 'Registrar retiro',
    operation: 'payments_shift_withdrawal',
    permission: 'cash.withdrawals.record',
    outflow: true,
  },
  expense: {
    title: 'Gasto pagado desde caja',
    verb: 'Registrar gasto',
    operation: 'payments_shift_expense',
    permission: 'cash.expenses.record',
    outflow: true,
  },
};

// Captura local de la operación: cadenas tal como se teclean.
export interface CashEntryCapture {
  choice: string;
  amount: string;
  currency: CashCurrency;
  concept: string;
  reference: string;
  notes: string;
  exceptional: boolean;
  reason: string;
}

export function initialEntry(): CashEntryCapture {
  return {
    choice: '',
    amount: '',
    currency: 'MXN',
    concept: '',
    reference: '',
    notes: '',
    exceptional: false,
    reason: '',
  };
}

// Cuerpo de la operación sin autorización: el tipo (ingreso o retiro) o la categoría (gasto).
export function entryBody(kind: CashEntryKind, capture: CashEntryCapture): CashOperationInput {
  const body: CashOperationInput = { amount: toWireAmount(capture.amount) };
  if (capture.currency === 'USD') {
    body.currency = 'USD';
  }
  if (kind === 'expense') {
    body.category_id = capture.choice;
  } else {
    body.type = capture.choice;
  }
  const concept = capture.concept.trim();
  if (concept) {
    body.concept = concept;
  }
  const reference = capture.reference.trim();
  if (kind === 'expense' && reference) {
    body.reference = reference;
  }
  const notes = capture.notes.trim();
  if (notes) {
    body.notes = notes;
  }
  if (kind !== 'income' && capture.exceptional) {
    body.exceptional = true;
    body.reason = capture.reason.trim();
  }
  return body;
}

// Permisos que bastan para autorizar la salida por cuenta propia. Una salida excepcional exige
// además poder consultar el efectivo esperado (lo valida Go).
export function outflowPermissions(exceptional: boolean) {
  if (exceptional) {
    return ['cash.outflows.authorize', 'cash.expected.read'];
  }
  return ['cash.outflows.authorize'];
}
