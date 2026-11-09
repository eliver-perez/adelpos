import { api } from './api';
import type { Operation } from './api';
import type { AuthorizationInput, Shift } from './payments';

// Contratos de la administración de caja de Go (backend/internal/payments/cash_*.go, etapa 7).
// El dinero viaja como cadena con dos decimales y lo calcula Go: la interfaz nunca suma billetes,
// importes ni diferencias. El efectivo esperado y las diferencias solo llegan cuando Go lo permite
// (cash.expected.read y fuera del arqueo de cierre); su ausencia no es un error.

export type CashCurrency = 'MXN' | 'USD';
export type CashOperationKind = 'income' | 'withdrawal' | 'expense' | 'correction';
export type CashOperationStatus = 'recorded' | 'corrected' | 'correction' | 'regularization';
export type CashDifferenceResult = 'even' | 'over' | 'short';

export interface Denomination {
  id: string;
  currency: CashCurrency;
  value: string;
  kind: 'bill' | 'coin';
  active: boolean;
  revision: number;
  updated_at: string;
}

export interface ExpenseCategory {
  id: string;
  name: string;
  active: boolean;
  sort_order: number;
  revision: number;
  updated_at: string;
}

export interface CashTypeOption {
  code: string;
  label: string;
}

export interface CashCatalog {
  denominations: Denomination[];
  expense_categories: ExpenseCategory[];
  income_types: CashTypeOption[];
  withdrawal_types: CashTypeOption[];
}

export interface RegisterShift {
  register: { id: string; name: string; active: boolean };
  shift: Shift | null;
  own: boolean;
}

// Conteo por denominaciones: cantidades físicas capturadas; Go calcula importes y totales.
export interface CountLineInput {
  denomination_id: string;
  quantity: number;
}

export interface CountInput {
  lines: CountLineInput[];
  extra_mxn?: string;
  extra_usd?: string;
  extra_note?: string;
}

export interface CountLine {
  denomination_id: string;
  currency: CashCurrency;
  kind: 'bill' | 'coin';
  value: string;
  quantity: number;
  amount: string;
}

export interface CashCount {
  id: string;
  shift_id: string;
  purpose: 'opening' | 'closing';
  state: 'draft' | 'final';
  lines: CountLine[];
  extra_mxn: string;
  extra_usd: string;
  extra_note: string;
  notes: string;
  counted_mxn: string;
  counted_usd: string;
  revision: number;
  updated_by_name: string;
  updated_at: string;
}

export interface CashExpectation {
  mxn: string;
  usd: string;
  usd_rate: string | null;
  usd_equivalent_mxn: string | null;
  total_equivalent_mxn: string | null;
}

export interface CashDifferences {
  mxn: string;
  usd: string;
  result_mxn: CashDifferenceResult;
  result_usd: CashDifferenceResult;
  equivalent_mxn: string | null;
}

export interface CashOperation {
  id: string;
  shift_id: string;
  register_id: string;
  register_name: string;
  kind: CashOperationKind;
  currency: CashCurrency;
  amount: string;
  direction: 'in' | 'out';
  concept: string;
  type: string | null;
  type_label: string | null;
  category_id: string | null;
  category_name: string | null;
  reference: string;
  notes: string;
  reason: string;
  corrects_operation_id: string | null;
  corrected_by_operation_id: string | null;
  regularization: boolean;
  exceptional: boolean;
  usd_rate: string | null;
  equivalent_mxn: string;
  actor_name: string;
  device_name: string;
  authorized_by_name: string | null;
  authorization_mode: 'self' | 'supervisor' | null;
  status: CashOperationStatus;
  created_at: string;
}

// Captura de un ingreso, retiro o gasto. type aplica a ingresos y retiros; category_id a gastos.
// exceptional solicita una salida que excede el efectivo disponible (con motivo y autorización).
export interface CashOperationInput {
  currency?: CashCurrency;
  amount: string;
  concept?: string;
  type?: string;
  category_id?: string;
  reference?: string;
  notes?: string;
  exceptional?: boolean;
  reason?: string;
  authorization?: AuthorizationInput;
}

export interface CashCorrectionInput {
  corrects_operation_id: string;
  reason: string;
  authorization?: AuthorizationInput;
}

export interface CashCorrectionResult {
  correction: CashOperation;
  original: CashOperation;
}

export interface CashReportSummary {
  id: string;
  kind: 'partial' | 'final';
  sequence: number;
  covered_from: string;
  covered_to: string;
  created_by_name: string;
  created_at: string;
}

export interface ShiftDetail {
  shift: Shift;
  opening_count: CashCount | null;
  reports: CashReportSummary[];
  summary: { payments: number; refunds: number; cash_operations: number; partial_cuts: number };
  expected_visible: boolean;
  expected?: CashExpectation;
}

export interface MethodReportTotals {
  method_name: string;
  is_cash: boolean;
  sales: string;
  tips: string;
  refunds: string;
  net: string;
}

export interface CurrencyOperationTotals {
  currency: CashCurrency;
  income: string;
  withdrawals: string;
  expenses: string;
  corrections_in: string;
  corrections_out: string;
}

export interface ShiftTotals {
  payments: number;
  refunds: number;
  payment_corrections: number;
  cash_operations: number;
  regularizations: number;
  sales: string;
  tips: string;
  refunded: string;
  discounts: string;
  discount_count: number;
  courtesies: string;
  courtesy_count: number;
  methods: MethodReportTotals[];
  cash: CurrencyOperationTotals[];
}

export interface CashReport {
  id: string;
  shift_id: string;
  kind: 'partial' | 'final';
  sequence: number;
  previous_report_id: string | null;
  covered_from: string;
  covered_to: string;
  created_by_name: string;
  device_name: string;
  created_at: string;
  expected_visible: boolean;
  snapshot: {
    control: {
      register_name: string;
      business_date: string;
      opened_by_name: string;
      opening_float: string;
      opening_float_usd: string | null;
      usd_rate: string | null;
    };
    accumulated: ShiftTotals;
    since_previous: ShiftTotals;
    declared?: {
      counted_mxn: string;
      counted_usd: string;
      extra_mxn: string;
      extra_usd: string;
      cashier_notes: string;
    };
    // Solo con cash.expected.read y fuera del arqueo de cierre.
    sensitive?: { expected: CashExpectation; differences?: CashDifferences };
  };
}

// Arqueo en curso: solo lo declarado. Nunca contiene el efectivo esperado.
export interface ClosingView {
  shift: Shift;
  count: CashCount | null;
}

export interface ClosureComment {
  id: string;
  author_name: string;
  comment: string;
  created_at: string;
}

export interface ClosureView {
  shift: Shift;
  count: CashCount | null;
  cashier_notes: string;
  closed_by_name: string;
  device_name: string;
  closed_at: string;
  final_report_id: string;
  regularizations: CashOperation[];
  expected_visible: boolean;
  expected?: CashExpectation;
  differences?: CashDifferences;
  comments?: ClosureComment[];
}

export interface ClosureSummary {
  counted_mxn: string;
  counted_usd: string;
  expected_mxn?: string;
  expected_usd?: string;
  difference_mxn?: string;
  difference_usd?: string;
  result_mxn?: CashDifferenceResult;
  result_usd?: CashDifferenceResult;
  difference_equivalent_mxn?: string | null;
}

export interface ShiftHistory {
  items: { shift: Shift; closure: ClosureSummary | null }[];
  page: number;
  page_size: number;
  total: number;
}

export interface ShiftHistoryFilter {
  register_id?: string;
  from?: string;
  until?: string;
  state?: 'open' | 'closing' | 'closed';
  page?: string;
}

// Vista previa de un conteo por denominaciones (POST payments/cash/count-preview). Go valida y
// totaliza con la misma función del arqueo y de la apertura; nunca incluye el efectivo esperado.
export interface CountPreviewInput extends CountInput {
  // Turno del arqueo; se omite para el desglose del fondo de una apertura.
  shift_id?: string;
}

export interface CountPreview {
  lines: CountLine[];
  extra_mxn: string;
  extra_usd: string;
  extra_note: string;
  counted_mxn: string;
  counted_usd: string;
}

// Efectivo recibido y entregado en cobros y devoluciones por moneda. Solo llega con el permiso de
// consultar importes esperados y fuera del arqueo.
export interface PhysicalCash {
  mxn_in: string;
  mxn_out: string;
  usd_in: string;
  usd_out: string;
}

// Resumen en vivo del turno (GET payments/shifts/{id}/summary): los totales de un corte sin
// registrarlo. Durante el arqueo (blind) no trae totales, esperado ni efectivo físico.
export interface ShiftSummary {
  shift: Shift;
  generated_at: string;
  blind: boolean;
  control: CashReport['snapshot']['control'];
  partial_cuts: number;
  last_cut: CashReportSummary | null;
  accumulated?: ShiftTotals;
  since_last_cut?: ShiftTotals;
  expected_visible: boolean;
  expected?: CashExpectation;
  physical?: PhysicalCash;
  closure?: ClosureSummary;
}

export function previewCount(input: CountPreviewInput) {
  return api<CountPreview>('payments_cash_count_preview', { body: input });
}

// Consultas sobre el puente nativo. Las escrituras pasan por el almacén de operaciones pendientes
// (useRestaurantMutation con cashPendingKey) para conservar su operation_id ante una respuesta
// incierta; la contraseña de autorización nunca se guarda.
export function readCashCatalog() {
  return api<CashCatalog>('payments_cash_catalog');
}

export function readCashRegisters() {
  return api<RegisterShift[]>('payments_cash_registers');
}

export function readShiftDetail(shiftID: string) {
  return api<ShiftDetail>('payments_shift', { id: shiftID });
}

export function readShiftOperations(shiftID: string) {
  return api<{ shift: Shift; operations: CashOperation[] }>('payments_shift_operations', {
    id: shiftID,
  });
}

export function readClosing(shiftID: string) {
  return api<ClosingView>('payments_shift_closing', { id: shiftID });
}

export function readClosure(shiftID: string) {
  return api<ClosureView>('payments_shift_closure', { id: shiftID });
}

export function readCashReport(reportID: string) {
  return api<CashReport>('payments_cash_report', { id: reportID });
}

export function readShiftHistory(filter: ShiftHistoryFilter = {}) {
  const query: Record<string, string> = {};
  for (const [key, value] of Object.entries(filter)) {
    if (value) {
      query[key] = value;
    }
  }
  return api<ShiftHistory>('payments_shifts', { query });
}

export function readShiftSummary(shiftID: string) {
  return api<ShiftSummary>('payments_shift_summary', { id: shiftID });
}

// Clave del almacén global de pendientes para las escrituras de un turno.
export function cashPendingKey(shiftID: string) {
  return `cash:${shiftID}`;
}

// Complemento «el resultado …» de cada escritura de caja, para describir una operación pendiente.
export const cashOperationLabels: Partial<Record<Operation, string>> = {
  payments_shift_income: 'del ingreso de efectivo',
  payments_shift_withdrawal: 'del retiro de efectivo',
  payments_shift_expense: 'del gasto',
  payments_shift_correction: 'de la corrección del movimiento',
  payments_shift_partial_cut: 'del corte parcial',
  payments_shift_start_closing: 'del inicio del arqueo',
  payments_shift_save_count: 'del conteo del arqueo',
  payments_shift_cancel_closing: 'de la cancelación del arqueo',
  payments_shift_confirm_closing: 'del cierre del turno',
  payments_shift_comment_closure: 'del comentario del cierre',
};

// Rechazos de un reintento sobre un turno pausado, cerrado o sustituido. El servidor es la fuente
// de verdad: la operación no se registró ni se duplicó, y el pendiente se descarta.
export const shiftEndedCodes = ['SHIFT_CLOSING', 'SHIFT_CLOSED', 'SHIFT_CHANGED'];

export function retryRejectedMessage(code: string, serverMessage: string) {
  if (!shiftEndedCodes.includes(code)) {
    return '';
  }
  return `${serverMessage} La operación pendiente no se registró ni se duplicó; ya no se reintentará. Si sigue haciendo falta, regístrala como una operación nueva en un turno abierto.`;
}

// Etiquetas de un billete o una moneda del catálogo, con el valor tal como lo entrega Go.
export function denominationLabel(denomination: {
  currency: CashCurrency;
  kind: string;
  value: string;
}) {
  const symbol = denomination.currency === 'USD' ? 'US$' : '$';
  const value = denomination.value.endsWith('.00')
    ? denomination.value.slice(0, -3)
    : denomination.value;
  const kind = denomination.kind === 'bill' ? 'Billetes' : 'Monedas';
  return `${kind} de ${symbol}${value}`;
}

export const currencyLabels: Record<CashCurrency, string> = {
  MXN: 'Pesos',
  USD: 'Dólares',
};

export const cashOperationKindLabels: Record<CashOperationKind, string> = {
  income: 'Ingreso',
  withdrawal: 'Retiro',
  expense: 'Gasto',
  correction: 'Corrección',
};

export const cashOperationStatusLabels: Record<CashOperationStatus, string> = {
  recorded: 'Registrado',
  corrected: 'Corregido',
  correction: 'Corrección',
  regularization: 'Regularización',
};

export const cashDifferenceLabels: Record<CashDifferenceResult, string> = {
  even: 'Sin diferencia',
  over: 'Sobrante',
  short: 'Faltante',
};

export const shiftStateLabels: Record<string, string> = {
  open: 'Abierto',
  closing: 'Cierre en curso',
  closed: 'Cerrado',
};
