import { useState } from 'react';
import type { FinancialMutation } from '../../../hooks/useFinancialMutation';
import { useRefundPreview } from '../../../hooks/usePaymentPreview';
import { useSupervisorAuthorization } from '../../../hooks/useSupervisorAuthorization';
import { APIError } from '../../../services/api';
import { toWireAmount } from '../../../services/amountInput';
import { isZeroMoney, refreshCodes, shiftCodes, validReason } from '../../../services/payments';
import type {
  Finance,
  FinanceLine,
  PaymentMethod,
  RefundInput,
  RefundMode,
  RefundResult,
  Selection,
} from '../../../services/payments';
import { financeKey } from './captureState';

export type RefundTarget = 'amount' | 'tip';

// Captura local de una devolución: cadenas tal como se teclean; Go calcula y valida el importe.
export interface RefundCapture {
  mode: RefundMode;
  paymentID: string;
  amount: string;
  tip: string;
  selections: Record<string, number>;
  methodID: string;
  reference: string;
  reason: string;
  target: RefundTarget;
}

export function initialRefundCapture(mode: RefundMode = 'total'): RefundCapture {
  return {
    mode,
    paymentID: '',
    amount: '',
    tip: '',
    selections: {},
    methodID: '',
    reference: '',
    reason: '',
    target: 'amount',
  };
}

// Unidades de una partida con pago asignado que todavía pueden devolverse (pago neto mayor que 0).
export function refundableUnits(line: FinanceLine) {
  return line.units.filter((unit) => !isZeroMoney(unit.paid) && !unit.paid.startsWith('-')).length;
}

function refundSelections(selections: Record<string, number>, finance: Finance): Selection[] {
  const result: Selection[] = [];
  for (const line of finance.lines) {
    const quantity = Math.min(selections[line.version_id] ?? 0, refundableUnits(line));
    if (quantity > 0) {
      result.push({ version_id: line.version_id, quantity });
    }
  }
  return result;
}

// Cuerpo de la previsualización. Método, referencia y motivo no cambian el cálculo de Go.
export function refundPreviewInput(capture: RefundCapture, finance: Finance): RefundInput | null {
  if (capture.mode === 'total') {
    return { mode: 'total' };
  }
  if (capture.mode === 'payment') {
    const amount = toWireAmount(capture.amount);
    const tip = toWireAmount(capture.tip);
    if (!capture.paymentID || (amount === '' && tip === '')) {
      return null;
    }
    return { mode: 'payment', payment_id: capture.paymentID, amount, tip };
  }
  const selections = refundSelections(capture.selections, finance);
  if (selections.length === 0) {
    return null;
  }
  return { mode: 'items', selections };
}

// Devoluciones totales, de un pago o por productos, con el método real de salida del dinero.
export function useRefundFlow({
  accountID,
  finance,
  methods,
  mutation,
  enabled,
  reloadFinance,
  reloadShift,
  shiftID = null,
}: {
  accountID: string;
  finance: Finance | null;
  methods: PaymentMethod[];
  mutation: FinancialMutation;
  enabled: boolean;
  reloadFinance: () => Promise<void>;
  reloadShift: () => Promise<void>;
  // Turno que muestra la terminal; forma parte de la huella de la devolución (etapa 7).
  shiftID?: string | null;
}) {
  const [capture, setCapture] = useState<RefundCapture>(() => initialRefundCapture());
  const [result, setResult] = useState<RefundResult | null>(null);
  const input = enabled && finance ? refundPreviewInput(capture, finance) : null;
  const preview = useRefundPreview(accountID, input, finance ? financeKey(finance) : '');
  const plan = preview.plan;
  const auth = useSupervisorAuthorization(plan ? plan.requires_authorization : null);
  const outlets = methods.filter((method) => method.active && method.allows_refund);
  const method = outlets.find((entry) => entry.id === capture.methodID) ?? null;
  const locked = mutation.busy || !!mutation.pending;
  const referenceMissing = method?.reference_mode === 'required' && capture.reference.trim() === '';
  const canRegister =
    enabled &&
    !!plan &&
    !preview.validating &&
    !!method &&
    validReason(capture.reason) &&
    !referenceMissing &&
    auth.ready &&
    !locked;

  function update(patch: Partial<RefundCapture>) {
    if (locked) {
      return;
    }
    setCapture((current) => ({ ...current, ...patch }));
    setResult(null);
    mutation.clearError();
  }

  function changeMode(mode: RefundMode) {
    update({ mode, paymentID: '', amount: '', tip: '', selections: {}, target: 'amount' });
  }

  function choosePayment(paymentID: string) {
    update({ paymentID, amount: '', tip: '', target: 'amount' });
  }

  function changeSelection(versionID: string, quantity: number) {
    update({ selections: { ...capture.selections, [versionID]: quantity } });
  }

  function onRecorded(value: RefundResult) {
    setResult(value);
    setCapture((current) => initialRefundCapture(current.mode));
    auth.reset();
  }

  async function register() {
    if (!canRegister || !input || !method || mutation.isPending()) {
      return;
    }
    // La contraseña se retira del formulario antes de enviar, sea cual sea el resultado.
    const authorization = auth.take();
    const body: RefundInput = {
      ...input,
      method_id: method.id,
      reference: method.reference_mode === 'none' ? '' : capture.reference.trim(),
      reason: capture.reason.trim(),
    };
    if (authorization) {
      body.authorization = authorization;
    }
    if (shiftID) {
      body.shift_id = shiftID;
    }
    setResult(null);
    try {
      await mutation.execute<RefundResult>('payments_refund', { id: accountID, body: { ...body } });
    } catch (problem) {
      if (auth.handleError(problem)) {
        return;
      }
      if (!(problem instanceof APIError)) {
        return;
      }
      if (refreshCodes.includes(problem.code)) {
        void reloadFinance();
      }
      if (shiftCodes.includes(problem.code)) {
        void reloadShift();
      }
    }
  }

  let hint = '';
  if (preview.error) {
    hint = preview.error.message;
  } else if (capture.mode === 'payment' && !capture.paymentID) {
    hint = 'Elige en el historial el pago del que sale la devolución.';
  } else if (capture.mode === 'payment' && !input) {
    hint = 'Captura el importe o la propina que se devuelve.';
  } else if (capture.mode === 'items' && !input) {
    hint = 'Selecciona las unidades pagadas que se devuelven.';
  }

  return {
    capture,
    update,
    changeMode,
    choosePayment,
    changeSelection,
    input,
    preview,
    plan,
    auth,
    outlets,
    method,
    locked,
    referenceMissing,
    canRegister,
    register,
    result,
    onRecorded,
    hint,
  };
}

export type RefundFlow = ReturnType<typeof useRefundFlow>;
