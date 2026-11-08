import { toWireAmount } from '../../../services/amountInput';
import { isZeroMoney } from '../../../services/payments';
import type {
  Finance,
  FinanceLine,
  PaymentInput,
  PaymentMethod,
  PaymentMode,
  PaymentPlan,
  Selection,
} from '../../../services/payments';

// Campo de importe al que escribe el teclado numérico fijo.
export type AmountTarget = 'amount' | 'received' | 'tip';

// Captura local del cajero. Son cadenas tal como se teclean: Go valida y calcula cada importe.
export interface CaptureState {
  methodID: string;
  mode: PaymentMode;
  amount: string;
  received: string;
  tip: string;
  reference: string;
  // Cantidad seleccionada por versión de partida (unidades enteras, no dinero).
  selections: Record<string, number>;
  target: AmountTarget;
  // Imprimir ticket: null sigue la preferencia del establecimiento hasta que el cajero la cambie.
  print: boolean | null;
  // Marcar como entregado al registrar (solo pedidos para llevar).
  deliver: boolean;
}

export function initialCapture(deliver = false): CaptureState {
  return {
    methodID: '',
    mode: 'total',
    amount: '',
    received: '',
    tip: '',
    reference: '',
    selections: {},
    target: 'amount',
    print: null,
    deliver,
  };
}

// Unidades de una partida que aún pueden cobrarse por selección: las que conservan importe
// pendiente según Go (sin pago o con pago parcial).
export function selectableUnits(line: FinanceLine) {
  return line.units.filter((unit) => unit.status !== 'settled' && !isZeroMoney(unit.remaining))
    .length;
}

// Convierte la selección local en el contrato de Go, ajustada a las unidades que hoy siguen
// pendientes: si otra terminal cobró parte de una partida, la cantidad se limita sin descartarla.
export function selectionList(selections: Record<string, number>, finance: Finance): Selection[] {
  const result: Selection[] = [];
  for (const line of finance.lines) {
    const requested = selections[line.version_id] ?? 0;
    const quantity = Math.min(requested, selectableUnits(line));
    if (quantity > 0) {
      result.push({ version_id: line.version_id, quantity });
    }
  }
  return result;
}

export function tipAllowed(method: PaymentMethod | null, canRecordTips: boolean) {
  return !!method && method.allows_tip && canRecordTips;
}

// Captura completa para previsualizar con el método elegido. Un abono sin importe todavía no se
// valida: el cajero aún lo está capturando.
export function buildPaymentInput(
  capture: CaptureState,
  method: PaymentMethod,
  finance: Finance,
  permissions: { tips: boolean; selection: boolean },
): PaymentInput | null {
  const amount = toWireAmount(capture.amount);
  if (capture.mode === 'partial' && amount === '') {
    return null;
  }
  let reference = '';
  if (method.reference_mode !== 'none') {
    reference = capture.reference.trim();
  }
  let tip = '';
  if (tipAllowed(method, permissions.tips)) {
    tip = toWireAmount(capture.tip);
  }
  let received = '';
  if (method.is_cash) {
    received = toWireAmount(capture.received);
  }
  let selections: Selection[] = [];
  if (permissions.selection) {
    selections = selectionList(capture.selections, finance);
  }
  return {
    method_id: method.id,
    mode: capture.mode,
    amount: capture.mode === 'partial' ? amount : '',
    received,
    tip,
    reference,
    selections,
  };
}

// Método usado solo para que Go calcule el total de una selección antes de que el cajero elija
// cómo pagar. Se prefiere efectivo porque no exige referencia; nunca se usa para registrar.
export function calculationMethod(methods: PaymentMethod[]) {
  const cash = methods.find((method) => method.active && method.is_cash);
  if (cash) {
    return cash;
  }
  return methods.find((method) => method.active && method.reference_mode !== 'required') ?? null;
}

export function calculationInput(method: PaymentMethod, selections: Selection[]): PaymentInput {
  return {
    method_id: method.id,
    mode: 'total',
    amount: '',
    received: '',
    tip: '',
    reference: '',
    selections,
  };
}

// Cuerpo del registro: el importe total que se mostró (calculado por Go) y el saldo mostrado como
// saldo esperado. Si cualquiera cambió en otra terminal, Go responde BALANCE_CHANGED. Impresión y
// entrega se envían siempre explícitas para no depender de la preferencia del servidor.
export function registerInput(
  input: PaymentInput,
  plan: PaymentPlan,
  finance: Finance,
  options: { print: boolean; deliver: boolean },
): PaymentInput {
  return {
    ...input,
    amount: input.mode === 'total' ? plan.amount : input.amount,
    expected_balance: finance.balance,
    print: options.print,
    deliver: options.deliver,
  };
}

// Identifica el estado financiero que sustenta un cálculo; cambia cuando otra operación altera la
// cuenta, pero no con cada sondeo idéntico.
export function financeKey(finance: Finance) {
  return [
    finance.balance,
    finance.revision,
    finance.state,
    finance.payments.length,
    finance.refunds.length,
    finance.adjustments.length,
  ].join('|');
}

// Elige el estado financiero más reciente entre la consulta periódica y la respuesta del último
// pago, para no mostrar un saldo anterior mientras llega la siguiente consulta.
export function newestFinance(
  polled: Finance | null,
  recorded: Finance | null,
  accountID: string,
): Finance | null {
  const candidates = [polled, recorded].filter(
    (value): value is Finance => !!value && value.account_id === accountID,
  );
  if (candidates.length === 0) {
    return null;
  }
  return candidates.reduce((newest, candidate) =>
    Date.parse(candidate.server_time) > Date.parse(newest.server_time) ? candidate : newest,
  );
}

// Campo que recibe el teclado al elegir método o tipo de pago.
export function defaultTarget(
  mode: PaymentMode,
  method: PaymentMethod | null,
  canRecordTips: boolean,
): AmountTarget {
  if (mode === 'partial') {
    return 'amount';
  }
  if (method?.is_cash) {
    return 'received';
  }
  if (tipAllowed(method, canRecordTips)) {
    return 'tip';
  }
  return 'amount';
}

// Cambio parcial de la captura para el campo que recibe el teclado.
export function targetPatch(target: AmountTarget, value: string): Partial<CaptureState> {
  if (target === 'received') {
    return { received: value };
  }
  if (target === 'tip') {
    return { tip: value };
  }
  return { amount: value };
}

// Solo es editable el campo que corresponde a la captura vigente.
export function targetEditable(
  target: AmountTarget,
  capture: CaptureState,
  method: PaymentMethod | null,
  canRecordTips: boolean,
) {
  if (target === 'amount') {
    return capture.mode === 'partial';
  }
  if (target === 'received') {
    return !!method?.is_cash;
  }
  return tipAllowed(method, canRecordTips);
}
