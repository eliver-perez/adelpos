import { api } from './api';
import type { Operation } from './api';

// Contratos del módulo financiero de Go (backend/internal/payments). El dinero viaja siempre como
// cadena con dos decimales: la interfaz lo muestra tal como lo entrega Go y nunca lo calcula.

export type PaymentState = 'not_recorded' | 'partial' | 'paid';
export type DeliveryState = 'not_delivered' | 'delivered';
export type MethodIcon = 'cash' | 'card' | 'transfer' | 'platform' | 'voucher' | 'other';
export type ReferenceMode = 'none' | 'optional' | 'required';
export type TaxRate = '16' | '8' | '0';
export type PaymentMode = 'total' | 'partial';

export interface PaymentMethod {
  id: string;
  code: string | null;
  name: string;
  icon: MethodIcon;
  active: boolean;
  sort_order: number;
  is_cash: boolean;
  reference_mode: ReferenceMode;
  allows_tip: boolean;
  allows_refund: boolean;
  revision: number;
  updated_at: string;
}

export interface MethodInput {
  revision: number;
  name: string;
  icon: MethodIcon;
  active: boolean;
  sort_order: number;
  is_cash: boolean;
  reference_mode: ReferenceMode;
  allows_tip: boolean;
  allows_refund: boolean;
  // Posición elegida para el orden visual; Go recalcula sort_order al guardar.
  position?: { after: string };
}

export interface PaymentSettings {
  tax_rate: TaxRate;
  tax_rates: TaxRate[];
  shared_shifts: boolean;
  print_receipt_default: boolean;
  receipt_printer_id: string | null;
  receipt_printer_name: string | null;
  refund_authorization_threshold: string | null;
  // Rango del tipo de cambio (MXN por USD) y porcentaje de diferencia que exige confirmación.
  usd_rate_min: string;
  usd_rate_max: string;
  usd_rate_alert_percent: string;
  revision: number;
  updated_at: string;
}

export interface USDSettingsInput {
  revision: number;
  usd_rate_min: string;
  usd_rate_max: string;
  usd_rate_alert_percent: string;
}

export interface SettingsInput {
  revision: number;
  shared_shifts: boolean;
  print_receipt_default: boolean;
  receipt_printer_id: string | null;
  refund_authorization_threshold: string | null;
}

export interface TaxInput {
  revision: number;
  tax_rate: TaxRate;
}

export interface TaxLine {
  rate: number;
  base: string;
  tax: string;
  total: string;
}

export interface FinanceUnit {
  id: string;
  ordinal: number;
  total: string;
  discount: string;
  due: string;
  paid: string;
  remaining: string;
  status: 'unpaid' | 'partial' | 'settled';
  tax_rate: number;
}

export interface FinanceLine {
  version_id: string;
  item_id: string;
  name: string;
  notes: string;
  quantity: number;
  settled_quantity: number;
  partial_quantity: number;
  due: string;
  remaining: string;
  units: FinanceUnit[];
}

export interface PaymentRecord {
  id: string;
  kind: string;
  method_id: string;
  method_name: string;
  is_cash: boolean;
  amount: string;
  tip: string;
  received: string | null;
  change: string;
  // Efectivo con dólares: pesos y dólares recibidos, tipo de cambio y moneda del cambio. Go los
  // envía siempre (null en pagos solo en pesos).
  received_mxn?: string | null;
  received_usd?: string | null;
  usd_rate?: string | null;
  change_currency?: ChangeCurrency | null;
  change_usd?: string | null;
  reference: string;
  corrects_payment_id: string | null;
  refunded_amount: string;
  refunded_tip: string;
  corrected: boolean;
  shift_id: string;
  register_name: string;
  actor_name: string;
  device_name: string;
  created_at: string;
  unit_ids: string[];
}

export interface RefundRecord {
  id: string;
  payment_id: string;
  kind: string;
  method_name: string;
  is_cash: boolean;
  amount: string;
  tip: string;
  reason: string;
  reference: string;
  actor_name: string;
  authorized_by_name: string | null;
  created_at: string;
  unit_ids: string[];
  tax: TaxLine[];
}

export interface Adjustment {
  id: string;
  kind: string;
  scope: string;
  method: string;
  value: string;
  amount: string;
  courtesy: boolean;
  tax_rate: number | null;
  concept: string;
  reason: string;
  actor_name: string;
  authorized_by_name: string | null;
  created_at: string;
  voided: boolean;
  voided_at: string | null;
  void_reason: string | null;
  unit_ids: string[];
}

export interface Delivery {
  delivered_at: string;
  actor_name: string;
  device_name: string;
}

// Precuentas y tickets ligados a la cuenta (se usan a partir de la fase 6D).
export interface DocumentRecord {
  ticket_id: string;
  kind: string;
  payment_id: string | null;
  created_at: string;
  job_id: string | null;
  job_state: string | null;
  printer_name: string | null;
  copies: number;
}

// Estado financiero completo y autoritativo de una cuenta (GET payments/accounts/{id}).
export interface Finance {
  account_id: string;
  folio: number;
  name: string;
  mode: 'dine_in' | 'takeaway';
  state: 'open' | 'closed' | 'merged' | 'voided';
  revision: number;
  table_name: string | null;
  items_total: string;
  item_discounts: string;
  account_discounts: string;
  charges: string;
  total: string;
  paid: string;
  refunded: string;
  tips: string;
  balance: string;
  payment_state: PaymentState;
  delivery_state: DeliveryState;
  settled: boolean;
  tax: TaxLine[];
  tax_base: string;
  tax_total: string;
  lines: FinanceLine[];
  payments: PaymentRecord[];
  refunds: RefundRecord[];
  adjustments: Adjustment[];
  delivery: Delivery | null;
  documents: DocumentRecord[];
  server_time: string;
}

// Resumen financiero que acompaña a cada cuenta del restaurante (Brief en Go).
export interface FinanceBrief {
  total: string;
  paid: string;
  balance: string;
  tips: string;
  payment_state: PaymentState;
  delivery_state: DeliveryState;
}

export interface Selection {
  version_id: string;
  quantity: number;
}

export type ChangeCurrency = 'MXN' | 'USD';

// Captura de un pago. La previsualización y el registro usan el mismo contrato; el registro agrega
// operation_id y expected_balance. Los campos en dólares solo se envían con la función usd_cash
// activa; con dólares, received son los pesos recibidos.
export interface PaymentInput {
  method_id: string;
  mode: PaymentMode;
  amount: string;
  received: string;
  received_usd?: string;
  change_currency?: ChangeCurrency;
  tip: string;
  reference: string;
  selections: Selection[];
  expected_balance?: string;
  print?: boolean;
  deliver?: boolean;
}

export interface SelectedUnit {
  unit_id: string;
  version_id: string;
  remaining: string;
  amount: string;
}

// Cálculo autoritativo de Go para una captura: importe, cambio, saldos y total de selección.
export interface PaymentPlan {
  mode: PaymentMode;
  target: string;
  amount: string;
  tip: string;
  received: string | null;
  change: string;
  balance_before: string;
  balance_after: string;
  settles: boolean;
  selection_total: string | null;
  units: SelectedUnit[];
  // Solo con la función usd_cash activa, método de efectivo y turno con tipo de cambio.
  usd?: USDPlan;
}

// Cifras en pesos y dólares de un cobro en efectivo, calculadas por Go con el tipo de cambio del
// turno. La interfaz nunca convierte ni redondea.
export interface USDPlan {
  rate: string;
  balance_usd: string;
  charged_usd: string;
  received_mxn: string;
  received_usd: string;
  received_usd_mxn: string;
  received_total: string;
  change_mxn: string;
  change_usd: string;
  change_currency: ChangeCurrency;
  covered: boolean;
  missing_mxn: string;
  missing_usd: string;
}

export type PrintJobState =
  'pending' | 'processing' | 'submitted' | 'error' | 'uncertain' | 'intervention' | 'cancelled';

// Resultado de la impresión solicitada con un pago o una precuenta. Un fallo de impresión nunca
// revierte el pago: el trabajo queda en la cola durable de Cocina.
export interface PrintingResult {
  requested: boolean;
  status?: PrintJobState | 'not_configured';
  job_id?: string;
  ticket_id?: string;
  message?: string;
}

export interface PaymentResult {
  payment: PaymentRecord | null;
  finance: Finance;
  closed: boolean;
  delivered: boolean;
  printing: PrintingResult;
}

export interface DeliveryResult {
  finance: Finance;
  closed: boolean;
  delivered: boolean;
}

export interface ReprintResult {
  status: PrintJobState;
  job_id: string;
  ticket_id: string;
}

export interface CommercialDocumentLine {
  quantity: number;
  name: string;
  notes: string;
  amount: string;
}

export interface CommercialDocumentPayment {
  method: string;
  amount: string;
  tip: string;
  reference: string;
  received: string | null;
  change: string;
  received_mxn?: string | null;
  received_usd?: string | null;
  usd_rate?: string | null;
  change_currency?: ChangeCurrency | null;
  change_usd?: string | null;
}

// Precuenta o ticket comercial. No es un CFDI; content es el texto ya formateado para imprimir.
export interface CommercialDocument {
  kind: 'precheck' | 'receipt';
  establishment: string;
  folio: number;
  account_name: string;
  context: string;
  issued_at: string;
  local_time: string;
  cashier: string;
  lines: CommercialDocumentLine[];
  adjustments: { concept: string; amount: string }[];
  tax: TaxLine[];
  tax_base: string;
  tax_total: string;
  total: string;
  payments: CommercialDocumentPayment[];
  paid: string;
  tips: string;
  balance: string;
  payment: CommercialDocumentPayment | null;
  content: string;
}

// Autorización puntual de un supervisor. La contraseña solo viaja en la solicitud que la usa.
export interface AuthorizationInput {
  username: string;
  password: string;
}

export type DiscountScope = 'account' | 'items';
export type DiscountMethod = 'percent' | 'amount';

export interface DiscountInput {
  scope: DiscountScope;
  method: DiscountMethod;
  value: string;
  courtesy: boolean;
  concept?: string;
  reason?: string;
  selections?: Selection[];
  expected_balance?: string;
  authorization?: AuthorizationInput;
}

export interface DiscountPlan {
  scope: DiscountScope;
  method: DiscountMethod;
  value: string;
  base: string;
  amount: string;
  percent: string;
  courtesy: boolean;
  actor_limit: string;
  requires_authorization: boolean;
  balance_after: string;
  settles: boolean;
  units: { unit_id: string; amount: string }[];
}

export interface ChargeInput {
  concept: string;
  amount: string;
  reason?: string;
  expected_balance: string;
}

export interface VoidAdjustmentInput {
  adjustment_id: string;
  reason: string;
  authorization?: AuthorizationInput;
}

export interface AdjustmentResult {
  adjustment_id: string;
  finance: Finance;
  closed: boolean;
}

export type RefundMode = 'total' | 'payment' | 'items';

export interface RefundInput {
  mode: RefundMode;
  payment_id?: string;
  amount?: string;
  tip?: string;
  selections?: Selection[];
  method_id?: string;
  reference?: string;
  reason?: string;
  authorization?: AuthorizationInput;
}

export interface RefundPart {
  payment_id: string;
  amount: string;
  tip: string;
  units: { unit_id: string; amount: string }[];
  tax: TaxLine[];
}

export interface RefundPlan {
  mode: RefundMode;
  amount: string;
  tip: string;
  parts: RefundPart[];
  requires_authorization: boolean;
  balance_after: string;
}

export interface RefundResult {
  refund_ids: string[];
  plan: RefundPlan;
  finance: Finance;
  reopened: boolean;
}

export interface CorrectionInput {
  payment_id: string;
  method_id: string;
  reference?: string;
  reason: string;
  authorization?: AuthorizationInput;
}

export interface CorrectionResult {
  reversal_id: string;
  payment_id: string;
  finance: Finance;
}

export interface CashRegister {
  id: string;
  name: string;
  active: boolean;
}

export interface Shift {
  id: string;
  register_id: string;
  register_name: string;
  state: string;
  opening_float: string;
  opened_by: string;
  opened_by_name: string;
  opened_device_name: string;
  opened_at: string;
  closed_at: string | null;
  revision: number;
  shared_allowed: boolean;
  // Tipo de cambio fijo del turno; null mientras no se captura.
  usd_rate?: string | null;
  usd_rate_set_by_name?: string | null;
  usd_rate_set_at?: string | null;
}

export interface CurrentShift {
  register: CashRegister | null;
  shift: Shift | null;
  shared_shifts: boolean;
  usable: boolean;
  reason: string;
  // Estado de la función usd_cash y si el turno abierto aún no tiene tipo de cambio.
  usd_cash?: boolean;
  usd_rate_required?: boolean;
}

export interface ShiftMovement {
  id: string;
  kind: string;
  method_name: string;
  is_cash: boolean;
  sale_amount: string;
  tip_amount: string;
  net_amount: string;
  cash_received: string;
  change_given: string;
  // Efecto físico por moneda; null en movimientos anteriores a la 0007 o no efectivo.
  mxn_in?: string | null;
  mxn_out?: string | null;
  usd_in?: string | null;
  usd_out?: string | null;
  usd_rate?: string | null;
  account_id: string;
  folio: number;
  payment_id: string | null;
  refund_id: string | null;
  actor_name: string;
  created_at: string;
}

export interface MethodTotal {
  method_name: string;
  is_cash: boolean;
  sales: string;
  tips: string;
  net: string;
}

export interface ShiftMovements {
  shift: Shift;
  movements: ShiftMovement[];
  by_method: MethodTotal[];
  cash_net: string;
  expected_cash: string;
  // Efectivo físico por moneda (no es arqueo: el cierre pertenece a la etapa 7).
  by_currency?: {
    mxn_in: string;
    mxn_out: string;
    usd_in: string;
    usd_out: string;
    expected_mxn: string;
    expected_usd: string;
  };
}

// Consultas finas sobre el puente nativo. Las escrituras pasan por el almacén de operaciones
// pendientes (useRestaurantMutation con clave) para conservar su operation_id.
export function readPaymentSettings() {
  return api<PaymentSettings>('payments_settings');
}

export function listPaymentMethods() {
  return api<PaymentMethod[]>('payments_methods');
}

export function readCurrentShift() {
  return api<CurrentShift>('payments_current_shift');
}

export function readShiftMovements(shiftID: string) {
  return api<ShiftMovements>('payments_shift_movements', { id: shiftID });
}

export function readAccountFinance(accountID: string) {
  return api<Finance>('payments_account', { id: accountID });
}

export function previewPayment(accountID: string, input: PaymentInput) {
  return api<PaymentPlan>('payments_preview', { id: accountID, body: input });
}

export function previewDiscount(accountID: string, input: DiscountInput) {
  return api<DiscountPlan>('payments_discount_preview', { id: accountID, body: input });
}

export function previewRefund(accountID: string, input: RefundInput) {
  return api<RefundPlan>('payments_refund_preview', { id: accountID, body: input });
}

// Clave del almacén global de pendientes para los cobros de una cuenta.
export function paymentPendingKey(accountID: string) {
  return `payments:${accountID}`;
}

// Las impresiones de precuentas y copias usan su propia clave: una impresión incierta no bloquea
// los cobros de la cuenta.
export function documentsPendingKey(accountID: string) {
  return `documents:${accountID}`;
}

export const documentKindLabels: Record<string, string> = {
  receipt: 'Ticket de pago',
  precheck: 'Precuenta',
};

// Estados del trabajo de impresión explicados al cajero; submitted significa aceptado por la
// impresora o CUPS, no confirmación física del papel.
export const printStatusLabels: Record<string, string> = {
  pending: 'Impresión pendiente',
  processing: 'Impresión en curso',
  submitted: 'Ticket enviado',
  error: 'Error de impresión',
  uncertain: 'Resultado de impresión incierto',
  intervention: 'La impresión requiere intervención',
  cancelled: 'Impresión cancelada',
  not_configured: 'Sin impresora configurada',
};

// Estados que se resuelven en Cocina → Cola de impresión.
export const printAttentionStates = ['error', 'uncertain', 'intervention'];

// Estado del último trabajo de un documento (ticket o precuenta) en la lista de documentos.
export const documentJobLabels: Record<string, string> = {
  pending: 'Pendiente de imprimir',
  processing: 'Imprimiendo',
  submitted: 'Enviado a la impresora',
  error: 'Error de impresión',
  uncertain: 'Resultado incierto',
  intervention: 'Requiere intervención',
  cancelled: 'Cancelado',
};

// Explica el resultado de una impresión solicitada. Nunca se presenta como fallo del pago: el
// trabajo queda en la cola durable y se resuelve en Cocina.
export function printingMessage(printing: PrintingResult | null | undefined, subject = 'pago') {
  if (!printing || !printing.requested || !printing.status) {
    return '';
  }
  const status = printing.status;
  if (status === 'not_configured') {
    return `Sin impresora configurada: el ${subject} quedó registrado.`;
  }
  if (status === 'pending' || status === 'processing') {
    return 'Impresión pendiente: el documento está en la cola de impresión.';
  }
  if (status === 'submitted') {
    return 'Ticket enviado a la impresora.';
  }
  if (status === 'cancelled') {
    return `Impresión cancelada; el ${subject} quedó registrado.`;
  }
  return `${printStatusLabels[status] ?? 'Impresión no confirmada'}: el ${subject} quedó registrado. Revísalo en Cocina → Cola de impresión.`;
}

export const paymentStateBadges: Record<PaymentState, string> = {
  not_recorded: 'Sin pago',
  partial: 'Abono',
  paid: 'Pagado',
};

export const paymentStateDescriptions: Record<PaymentState, string> = {
  not_recorded: 'sin registro de cobro',
  partial: 'abono registrado',
  paid: 'pagado por completo',
};

export const deliveryStateBadges: Record<DeliveryState, string> = {
  not_delivered: 'Sin entregar',
  delivered: 'Entregado',
};

export const deliveryStateDescriptions: Record<DeliveryState, string> = {
  not_delivered: 'no entregado',
  delivered: 'entregado',
};

export const methodIconLabels: Record<MethodIcon, string> = {
  cash: 'Efectivo',
  card: 'Tarjeta',
  transfer: 'Transferencia',
  platform: 'Plataforma',
  voucher: 'Vale',
  other: 'Otro',
};

export const methodIcons = Object.keys(methodIconLabels) as MethodIcon[];

export const referenceModeLabels: Record<ReferenceMode, string> = {
  none: 'No admitida',
  optional: 'Opcional',
  required: 'Obligatoria',
};

export const taxRateLabels: Record<TaxRate, string> = {
  '16': 'Tasa general',
  '8': 'Región fronteriza',
  '0': 'Tasa 0 %',
};

export const movementKindLabels: Record<string, string> = {
  payment: 'Cobro',
  refund: 'Devolución',
  correction: 'Corrección',
  correction_reversal: 'Reverso por corrección',
};

// Complemento «el resultado …» de cada escritura financiera, para describir una operación pendiente.
export const financialOperationLabels: Partial<Record<Operation, string>> = {
  payments_pay: 'del pago',
  payments_discount: 'del descuento',
  payments_charge: 'del cargo',
  payments_void_adjustment: 'de la cancelación del ajuste',
  payments_refund: 'de la devolución',
  payments_correct: 'de la corrección del pago',
  payments_deliver: 'de la entrega del pedido',
  payments_print_precheck: 'de la impresión de la precuenta',
  payments_reprint: 'de la copia del documento',
  payments_open_shift: 'de la apertura del turno',
  payments_shift_usd_rate: 'de la captura del tipo de cambio',
};

// Describe el resultado confirmado de cualquier escritura financiera; todas devuelven el estado
// financiero actualizado, que la interfaz muestra sin recalcularlo.
export function financialOutcome(operation: Operation, result: unknown) {
  const value = result as { finance?: Finance; closed?: boolean; reopened?: boolean } | null;
  const finance = value?.finance ?? null;
  let message = 'Operación registrada.';
  if (operation === 'payments_discount') {
    message = 'Descuento registrado.';
  } else if (operation === 'payments_charge') {
    message = 'Cargo registrado.';
  } else if (operation === 'payments_void_adjustment') {
    message = 'Ajuste cancelado; el registro original se conserva en el historial.';
  } else if (operation === 'payments_refund') {
    message = 'Devolución registrada.';
  } else if (operation === 'payments_correct') {
    message = 'Pago corregido con un reverso y un pago nuevo.';
  } else if (operation === 'payments_pay') {
    message = 'Pago registrado.';
  } else if (operation === 'payments_deliver') {
    message = 'Entrega realizada.';
  }
  if (value?.closed) {
    message += ' La cuenta quedó liquidada y se cerró.';
  }
  if (value?.reopened) {
    message += ' La cuenta se reabrió porque quedó saldo pendiente.';
  }
  return { finance, message };
}

// Nombres legibles de los permisos financieros, con la misma descripción registrada en Go.
export const paymentPermissionLabels: Record<string, string> = {
  'payments.read': 'Consultar información financiera y pagos de cuentas',
  'payments.collect': 'Registrar cobros',
  'payments.collect_partial': 'Registrar pagos parciales',
  'payments.collect_selection': 'Cobrar productos y cantidades seleccionados',
  'payments.methods.manage': 'Administrar métodos de pago',
  'payments.settings.manage': 'Configurar políticas de cobro e impresión de tickets',
  'payments.tax.manage': 'Configurar la tasa de IVA del establecimiento',
  'payments.discounts.apply': 'Aplicar descuentos manuales dentro del límite del rol',
  'payments.discounts.authorize': 'Autorizar descuentos superiores al límite',
  'payments.courtesy': 'Otorgar cortesías',
  'payments.charges.create': 'Registrar cargos extras generales',
  'payments.charges.manage': 'Modificar o cancelar cargos extras',
  'payments.tips.record': 'Registrar propinas',
  'payments.corrections': 'Anular o corregir pagos',
  'payments.refunds.execute': 'Ejecutar devoluciones',
  'payments.refunds.authorize': 'Autorizar devoluciones',
  'payments.tickets.print': 'Imprimir tickets y precuentas',
  'payments.tickets.reprint': 'Reimprimir tickets',
  'takeaway.deliver': 'Registrar entrega de pedidos para llevar',
  'cash.shifts.open': 'Abrir turno de caja',
  'cash.shifts.read': 'Consultar turnos y movimientos de caja',
  'payments.usd_rate.manage': 'Capturar el tipo de cambio del dólar del turno',
  'kitchen.printers.manage': 'Administrar impresoras',
};

export function permissionName(key: string) {
  return paymentPermissionLabels[key] ?? key;
}

// Antepone el símbolo de moneda sin convertir ni recalcular el importe que entregó Go.
export function formatMoney(value: string | null | undefined) {
  if (value === null || value === undefined || value === '') {
    return '—';
  }
  if (value.startsWith('-')) {
    return `−$${value.slice(1)}`;
  }
  return `$${value}`;
}

// Códigos de Go que indican que la cuenta cambió y debe consultarse de nuevo.
export const refreshCodes = [
  'BALANCE_CHANGED',
  'NOTHING_TO_COLLECT',
  'ACCOUNT_NOT_OPEN',
  'ACCOUNT_BELOW_PAYMENTS',
  'ADJUSTMENT_VOIDED',
  'NOTHING_TO_REFUND',
];

// Códigos que exigen revisar o abrir el turno de la caja de la terminal.
export const shiftCodes = [
  'SHIFT_REQUIRED',
  'REGISTER_REQUIRED',
  'SHIFT_NOT_SHARED',
  'USD_RATE_REQUIRED',
  'FEATURE_DISABLED',
];

// Antepone el símbolo de dólares a una cifra que ya calculó Go, sin convertirla.
export function formatDollars(value: string | null | undefined) {
  if (value === null || value === undefined || value === '') {
    return '—';
  }
  return `US$${value}`;
}

export const changeCurrencyLabels: Record<ChangeCurrency, string> = {
  MXN: 'Pesos',
  USD: 'Dólares',
};

// Longitud de un texto en caracteres (no en bytes), como la valida Go.
export function textLength(text: string) {
  return Array.from(text.trim()).length;
}

// Los motivos de descuentos, devoluciones, correcciones y cancelaciones miden de 5 a 500 caracteres.
export function validReason(text: string) {
  const length = textLength(text);
  return length >= 5 && length <= 500;
}

// Reconoce un importe nulo por su forma textual ("0", "0.00"); no realiza aritmética.
export function isZeroMoney(value: string) {
  return /^-?0+(\.0+)?$/.test(value);
}
