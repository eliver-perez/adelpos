import { api } from './api';
import type { Operation } from './api';
import type { AuthorizationInput, Shift } from './payments';
import { formatDenomination } from './money';
import { isWindows } from '../commands/platform';

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
  // Fase C: la caja tiene un cajón de dinero activado (sin datos de su impresora).
  drawer_enabled?: boolean;
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
  // Aviso del tope de caja: solo llega con el esperado visible y la función activa (fase C).
  cash_limit_warning?: CashLimitWarning;
}

// Aviso de Go cuando el efectivo esperado supera el tope de caja. Nunca bloquea cobros y solo
// llega a quien puede consultar el esperado, fuera del arqueo de cierre.
export interface CashLimitWarning {
  currencies: CashCurrency[];
  message: string;
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
  cash_limit_warning?: CashLimitWarning;
}

// Estado de una impresión solicitada (fase C). status es el estado del trabajo en la cola o
// not_configured (sin impresora de tickets) o failed (no se pudo encolar). Nunca significa que el
// papel salió: la aceptación de CUPS o TCP solo confirma el envío.
export interface PrintOutcome {
  requested: boolean;
  status?: string;
  document_id?: string;
  ticket_id?: string;
  job_id?: string;
  includes_expected: boolean;
  message?: string;
  // Etapa 9F (decisión D4): por qué no se encoló (sin impresora de tickets, desactivada o de otra
  // terminal) y la impresora configurada.
  problem?: 'missing' | 'inactive' | 'unsupported';
  printer_name?: string;
}

// Estado de un comando de apertura del cajón. status es el del trabajo en la cola o disabled,
// unavailable o failed. Nunca significa que el cajón se abrió físicamente.
export interface DrawerOutcome {
  requested: boolean;
  status?: string;
  job_id?: string;
  event_id?: string;
  message?: string;
}

// Ingreso, retiro o gasto registrado con el estado de su comprobante y del cajón.
export type RecordedCashOperation = CashOperation & {
  printing?: PrintOutcome;
  drawer?: DrawerOutcome;
};

// Corte parcial creado: sin bloque sensible y con el estado de su impresión si se pidió.
export type CreatedCashReport = CashReport & { printing?: PrintOutcome };

export type CashDocumentKind = 'partial_cut' | 'final_cut' | 'income' | 'withdrawal' | 'expense';

export interface CashDocumentRecord {
  id: string;
  kind: CashDocumentKind;
  shift_id: string;
  report_id: string | null;
  cash_operation_id: string | null;
  ticket_id: string;
  includes_expected: boolean;
  created_by_name: string;
  created_at: string;
  job_id: string | null;
  job_state: string | null;
  // Clave del problema del último trabajo si exige atención (etapa 9F; services/printProblems.ts).
  job_problem?: string | null;
  printer_name: string | null;
  copies: number;
}

// Vista previa renderizada por Go con la misma plantilla y los mismos permisos que la impresión.
export interface CashDocumentPreview {
  kind: CashDocumentKind;
  title: string;
  content: string;
  includes_expected: boolean;
  paper_width: number;
  document: CashDocumentRecord | null;
}

// Origen de un documento de caja: un corte o una operación de efectivo.
export interface CashDocumentSource {
  kind: 'report' | 'operation';
  id: string;
}

export interface DrawerSettings {
  register_id: string;
  register_name: string;
  register_active: boolean;
  printer_id: string | null;
  printer_name: string | null;
  pulse_pin: number;
  enabled: boolean;
  revision: number;
  updated_at: string | null;
}

// Impresora candidata para el cajón. Para una cola CUPS, Go la consulta con lpstat y lpoptions
// (solo lecturas, etapa 7.1): connection es la conexión que informa CUPS y reason explica por qué
// no admite datos raw.
export interface DrawerPrinter {
  id: string;
  name: string;
  transport: string;
  active: boolean;
  compatible: boolean;
  connection?: string;
  reason?: string;
}

// Conexión de la impresora del cajón tal como la describe la configuración.
export function drawerConnectionLabel(printer: DrawerPrinter) {
  if (printer.transport === 'escpos_tcp') {
    return 'ESC/POS por red';
  }
  if (printer.transport === 'cups' && isWindows()) {
    // Etapa 9E: en Windows la cola del equipo es la de la impresora instalada en Windows.
    const connection = printer.connection ? windowsConnectionNames[printer.connection] : '';
    return connection ? `Impresora de Windows · ${connection}` : 'Impresora de Windows';
  }
  if (printer.transport === 'cups') {
    const connection = printer.connection ? drawerConnectionNames[printer.connection] : '';
    return connection ? `CUPS · ${connection}` : 'CUPS';
  }
  return 'Agente de impresión';
}

// Puertos de una impresora instalada en Windows que entregan los datos sin convertir.
const windowsConnectionNames: Record<string, string> = {
  usb: 'USB',
  serial: 'puerto serie',
  parallel: 'puerto paralelo',
  socket: 'TCP/IP estándar',
};

const drawerConnectionNames: Record<string, string> = {
  usb: 'USB',
  serial: 'puerto serie',
  parallel: 'puerto paralelo',
  socket: 'AppSocket',
  ipp: 'IPP',
  ipps: 'IPPS',
  dnssd: 'Bonjour',
  lpd: 'LPD',
};

// Retiro o gasto rechazado por efectivo insuficiente (etapa 7.1). Solo lo consulta quien puede ver
// el efectivo esperado; nunca incluye el esperado ni cuánto faltó.
export interface RejectedOutflow {
  id: string;
  occurred_at: string;
  kind: 'withdrawal' | 'expense';
  currency: CashCurrency;
  attempted_amount: string;
  actor_name: string;
  device_name: string;
  register_name: string;
  authorization_mode: 'self' | 'supervisor' | null;
  authorized_by_name: string | null;
}

export interface RejectedOutflows {
  shift_id: string;
  // Falso durante el arqueo de cierre: la consulta es ciega, como el esperado.
  visible: boolean;
  message?: string;
  rejections: RejectedOutflow[];
}

export interface CashLimitSettings {
  enabled: boolean;
  usd_cash: boolean;
  cash_limit_mxn: string | null;
  cash_limit_usd: string | null;
  revision: number;
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

// Vista previa de solo lectura de un documento de caja (fase C).
export function previewCashDocument(source: CashDocumentSource) {
  const operation: Operation =
    source.kind === 'report' ? 'payments_cash_report_preview' : 'payments_cash_operation_preview';
  return api<CashDocumentPreview>(operation, { id: source.id });
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
  payments_print_cash_report: 'de la impresión del corte',
  payments_print_cash_operation: 'de la impresión del comprobante',
  payments_reprint_cash_document: 'de la reimpresión del documento',
  payments_retry_cash_document: 'del reintento de impresión del documento',
  payments_open_drawer: 'del comando de apertura del cajón',
  payments_test_cash_drawer: 'de la prueba de apertura del cajón',
};

export const cashDocumentKindLabels: Record<CashDocumentKind, string> = {
  partial_cut: 'Corte parcial',
  final_cut: 'Corte definitivo',
  income: 'Comprobante de ingreso',
  withdrawal: 'Comprobante de retiro',
  expense: 'Comprobante de gasto',
};

// Estado del último trabajo de un documento de caja en la cola de impresión. «Enviado» significa
// que CUPS o la impresora aceptaron el trabajo, no que el papel salió.
export const cashJobLabels: Record<string, string> = {
  pending: 'En la cola de impresión',
  processing: 'Enviándose a la impresora',
  submitted: 'Enviado a la impresora',
  error: 'Error de impresión',
  uncertain: 'Resultado incierto',
  intervention: 'Requiere intervención',
  cancelled: 'Cancelado',
};

// Explica el resultado de una impresión de caja. Nunca afirma que el papel salió ni presenta un
// fallo de impresión como fallo de la operación: el trabajo queda en la cola durable.
export function cashPrintMessage(outcome: PrintOutcome | null | undefined, subject: string) {
  if (!outcome || !outcome.requested || !outcome.status) {
    return '';
  }
  switch (outcome.status) {
    case 'not_configured':
      // Etapa 9F: Go dice por qué (sin impresora de tickets, desactivada o de otra terminal).
      return (
        outcome.message ||
        `Sin impresora de tickets configurada: ${subject} quedó registrado sin imprimir.`
      );
    case 'failed':
      return `No se pudo enviar el documento a la cola: ${subject} quedó registrado. Imprímelo después desde Caja.`;
    case 'pending':
    case 'processing':
      return 'Documento enviado a la cola de impresión.';
    case 'submitted':
      return 'Documento enviado a la impresora.';
    case 'cancelled':
      return `Impresión cancelada; ${subject} quedó registrado.`;
  }
  return `${cashJobLabels[outcome.status] ?? 'Impresión no confirmada'}: ${subject} quedó registrado. Reinténtalo desde el documento en Caja.`;
}

export const printAttentionStatuses = [
  'error',
  'uncertain',
  'intervention',
  'failed',
  'unavailable',
];

// Explica el resultado de un comando del cajón. Solo informa que el comando se envió: nunca que el
// cajón se abrió físicamente.
export function drawerMessage(outcome: DrawerOutcome | null | undefined) {
  if (!outcome || !outcome.requested || !outcome.status) {
    return '';
  }
  switch (outcome.status) {
    case 'pending':
    case 'processing':
    case 'submitted':
      return 'Comando de apertura enviado.';
    case 'uncertain':
      return 'Resultado incierto del comando de apertura: revisa el cajón. No se reintenta automáticamente.';
    case 'unavailable':
    case 'failed':
      return outcome.message || 'No se pudo enviar el comando de apertura del cajón.';
  }
  return 'El comando de apertura no se envió: revísalo en la cola de impresión.';
}

// Rechazos de un reintento sobre un turno pausado, cerrado o sustituido. El servidor es la fuente
// de verdad: la operación no se registró ni se duplicó, y el pendiente se descarta.
export const shiftEndedCodes = ['SHIFT_CLOSING', 'SHIFT_CLOSED', 'SHIFT_CHANGED'];

export function retryRejectedMessage(code: string, serverMessage: string) {
  if (!shiftEndedCodes.includes(code)) {
    return '';
  }
  return `${serverMessage} La operación pendiente no se registró ni se duplicó; ya no se reintentará. Si sigue haciendo falta, regístrala como una operación nueva en un turno abierto.`;
}

// Etiquetas de un billete o una moneda del catálogo, con el valor que entrega Go en el formato
// único de denominaciones: sin decimales si es entero («Billetes de $1,000») y con dos si no
// («Monedas de $0.50»).
export function denominationLabel(denomination: {
  currency: CashCurrency;
  kind: string;
  value: string;
}) {
  const kind = denomination.kind === 'bill' ? 'Billetes' : 'Monedas';
  return `${kind} de ${formatDenomination(denomination.value, denomination.currency)}`;
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
