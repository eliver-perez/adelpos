import { useEffect, useState } from 'react';
import type { FinancialMutation } from '../../../hooks/useFinancialMutation';
import { usePaymentPreview } from '../../../hooks/usePaymentPreview';
import { APIError } from '../../../services/api';
import { formatMoney, refreshCodes, shiftCodes } from '../../../services/payments';
import type {
  Finance,
  PaymentInput,
  PaymentMethod,
  PaymentMode,
  PaymentResult,
} from '../../../services/payments';
import {
  buildPaymentInput,
  calculationInput,
  calculationMethod,
  defaultTarget,
  financeKey,
  initialCapture,
  registerInput,
  selectionList,
} from './captureState';
import type { CaptureState } from './captureState';

const cashQuickAmounts = [
  { label: 'Exacto', value: '' },
  { label: '$100', value: '100' },
  { label: '$200', value: '200' },
  { label: '$500', value: '500' },
  { label: '$1000', value: '1000' },
];

export interface CollectPermissions {
  collect: boolean;
  partial: boolean;
  selection: boolean;
  tips: boolean;
  print: boolean;
  deliver: boolean;
}

// Captura y registro de pagos (fase 6B) separados de la composición de la pantalla de cobro.
export function useCollectFlow({
  accountID,
  finance,
  methods,
  shiftUsable,
  permissions,
  mutation,
  enabled,
  printDefault,
  initialDeliver,
  reloadFinance,
  reloadShift,
}: {
  accountID: string;
  finance: Finance | null;
  methods: PaymentMethod[];
  shiftUsable: boolean;
  permissions: CollectPermissions;
  mutation: FinancialMutation;
  enabled: boolean;
  printDefault: boolean;
  initialDeliver: boolean;
  reloadFinance: () => Promise<void>;
  reloadShift: () => Promise<void>;
}) {
  const [capture, setCapture] = useState<CaptureState>(() => initialCapture(initialDeliver));
  const [lastResult, setLastResult] = useState<PaymentResult | null>(null);
  // Si se pidió la entrega con el último pago, para explicar cuando no se registró por saldo.
  const [deliverRequested, setDeliverRequested] = useState(false);
  const [conflict, setConflict] = useState(false);
  const method = methods.find((entry) => entry.id === capture.methodID) ?? null;
  const collectable = !!finance && finance.state === 'open' && !finance.settled;
  const locked = mutation.busy || !!mutation.pending;
  const mode: PaymentMode = capture.mode === 'partial' && permissions.partial ? 'partial' : 'total';
  const effectiveCapture = { ...capture, mode };
  // La impresión solo se ofrece con su permiso; sin decisión del cajero rige la preferencia.
  const printEnabled = permissions.print && (capture.print ?? printDefault);
  // La entrega solo aplica a pedidos para llevar aún no entregados.
  const deliverAvailable = permissions.deliver && finance?.mode === 'takeaway' && !finance.delivery;
  const deliverEnabled = deliverAvailable && capture.deliver;

  let previewInput: PaymentInput | null = null;
  if (enabled && finance && permissions.collect && collectable) {
    if (method) {
      previewInput = buildPaymentInput(effectiveCapture, method, finance, permissions);
    } else if (permissions.selection) {
      // Antes de elegir método, Go calcula igualmente el total de la selección.
      const selections = selectionList(capture.selections, finance);
      const calculator = calculationMethod(methods);
      if (selections.length > 0 && calculator) {
        previewInput = calculationInput(calculator, selections);
      }
    }
  }
  const preview = usePaymentPreview(accountID, previewInput, finance ? financeKey(finance) : '');
  const plan = preview.plan;
  const previewCode = preview.error?.code ?? '';

  // Un cálculo hecho sobre otro saldo o un rechazo por cambio de la cuenta obliga a consultarla.
  useEffect(() => {
    if (plan && finance && plan.balance_before !== finance.balance) {
      void reloadFinance();
    }
  }, [plan, finance, reloadFinance]);

  useEffect(() => {
    if (refreshCodes.includes(previewCode)) {
      void reloadFinance();
    }
  }, [previewCode, reloadFinance]);

  function updateCapture(patch: Partial<CaptureState>) {
    if (locked) {
      return;
    }
    setCapture((current) => ({ ...current, ...patch }));
    setLastResult(null);
    setConflict(false);
    mutation.clearError();
  }

  function selectMethod(selected: PaymentMethod) {
    updateCapture({
      methodID: selected.id,
      target: defaultTarget(mode, selected, permissions.tips),
    });
  }

  function changeMode(next: PaymentMode) {
    updateCapture({
      mode: next,
      amount: '',
      target: defaultTarget(next, method, permissions.tips),
    });
  }

  function changeSelection(versionID: string, quantity: number) {
    updateCapture({ selections: { ...capture.selections, [versionID]: quantity } });
  }

  // Pagos mixtos = pagos sucesivos: tras cada registro la captura se reinicia. La intención de
  // entregar se conserva mientras el pedido siga sin entregarse.
  function onRecorded(result: PaymentResult) {
    setLastResult(result);
    setCapture((current) => initialCapture(current.deliver && !result.delivered));
    setConflict(false);
  }

  const canRegister =
    enabled &&
    permissions.collect &&
    collectable &&
    shiftUsable &&
    !!method &&
    !!plan &&
    !preview.validating &&
    !locked;

  async function register() {
    if (!finance || !method || !plan || !previewInput || !canRegister || mutation.isPending()) {
      return;
    }
    setConflict(false);
    setLastResult(null);
    setDeliverRequested(deliverEnabled);
    try {
      await mutation.execute<PaymentResult>('payments_pay', {
        id: accountID,
        body: {
          ...registerInput(previewInput, plan, finance, {
            print: printEnabled,
            deliver: deliverEnabled,
          }),
        },
      });
    } catch (problem) {
      if (!(problem instanceof APIError)) {
        return;
      }
      if (problem.code === 'BALANCE_CHANGED') {
        // Conflicto de cobro: se consulta el saldo real y se conserva la captura para revisarla.
        setConflict(true);
      }
      if (refreshCodes.includes(problem.code)) {
        void reloadFinance();
      }
      if (shiftCodes.includes(problem.code)) {
        void reloadShift();
      }
    }
  }

  // Mensaje de validación de Go sobre la captura vigente (efectivo insuficiente, referencia…).
  let hint = '';
  if (preview.error && !refreshCodes.includes(previewCode)) {
    hint = preview.error.message;
  } else if (method && mode === 'partial' && !previewInput) {
    hint = 'Captura el importe del abono.';
  }

  const selectionTotal = plan?.selection_total ?? null;
  let quickAmounts: { label: string; value: string }[] = [];
  if (capture.target === 'received') {
    quickAmounts = cashQuickAmounts;
  } else if (capture.target === 'amount' && mode === 'partial' && finance) {
    const target = selectionTotal ?? (plan ? plan.target : finance.balance);
    quickAmounts = [{ label: `Saldo ${formatMoney(target)}`, value: target }];
  }

  return {
    capture,
    effectiveCapture,
    method,
    mode,
    collectable,
    preview,
    plan,
    canRegister,
    register,
    updateCapture,
    selectMethod,
    changeMode,
    changeSelection,
    conflict,
    lastResult,
    onRecorded,
    printEnabled,
    deliverAvailable,
    deliverEnabled,
    deliverRequested,
    setDeliverRequested,
    hint,
    quickAmounts,
    selectionTotal,
  };
}

export type CollectFlow = ReturnType<typeof useCollectFlow>;
