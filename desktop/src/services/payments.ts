import { api } from './api';

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
}

export interface PaymentSettings {
  tax_rate: TaxRate;
  tax_rates: TaxRate[];
  shared_shifts: boolean;
  print_receipt_default: boolean;
  receipt_printer_id: string | null;
  receipt_printer_name: string | null;
  refund_authorization_threshold: string | null;
  revision: number;
  updated_at: string;
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

// Captura de un pago. La previsualización y el registro usan el mismo contrato; el registro agrega
// operation_id y expected_balance.
export interface PaymentInput {
  method_id: string;
  mode: PaymentMode;
  amount: string;
  received: string;
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
}

export interface PaymentResult {
  payment: PaymentRecord | null;
  finance: Finance;
  closed: boolean;
  delivered: boolean;
  printing: { requested: boolean } & Record<string, unknown>;
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
}

export interface CurrentShift {
  register: CashRegister | null;
  shift: Shift | null;
  shared_shifts: boolean;
  usable: boolean;
  reason: string;
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

// Clave del almacén global de pendientes para los cobros de una cuenta.
export function paymentPendingKey(accountID: string) {
  return `payments:${accountID}`;
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
};

// Nombres legibles de los permisos financieros, con la misma descripción registrada en Go.
export const paymentPermissionLabels: Record<string, string> = {
  'payments.read': 'Consultar información financiera y pagos de cuentas',
  'payments.collect': 'Registrar cobros',
  'payments.collect_partial': 'Registrar pagos parciales',
  'payments.collect_selection': 'Cobrar productos y cantidades seleccionados',
  'payments.methods.manage': 'Administrar métodos de pago',
  'payments.settings.manage': 'Configurar políticas de cobro e impresión de tickets',
  'payments.tax.manage': 'Configurar la tasa de IVA del establecimiento',
  'payments.tips.record': 'Registrar propinas',
  'cash.shifts.open': 'Abrir turno de caja',
  'cash.shifts.read': 'Consultar turnos y movimientos de caja',
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

// Reconoce un importe nulo por su forma textual ("0", "0.00"); no realiza aritmética.
export function isZeroMoney(value: string) {
  return /^-?0+(\.0+)?$/.test(value);
}
