import { useState } from 'react';
import type { FinancialMutation } from '../../../hooks/useFinancialMutation';
import { useSupervisorAuthorization } from '../../../hooks/useSupervisorAuthorization';
import { APIError } from '../../../services/api';
import { isZeroMoney, validReason } from '../../../services/payments';
import { notifyActionError } from './actionErrors';
import type {
  CorrectionInput,
  CorrectionResult,
  Finance,
  PaymentMethod,
  PaymentRecord,
} from '../../../services/payments';

// Solo se corrige el método de un pago sin corrección previa y sin devoluciones; lo demás exige
// una devolución. Go confirma además que el turno original siga abierto.
export function correctable(payment: PaymentRecord) {
  return (
    !payment.corrected && isZeroMoney(payment.refunded_amount) && isZeroMoney(payment.refunded_tip)
  );
}

const closedShiftMessage =
  'El turno del pago ya está cerrado: su método no puede corregirse aquí. Usa una regularización posterior (etapa 7) o, si corresponde, una devolución.';

// Corrección de un método capturado por error: Go registra un reverso y un pago nuevo enlazados;
// el pago original no se edita.
export function useCorrectionFlow({
  accountID,
  finance,
  methods,
  mutation,
  canCorrect,
  reloadFinance,
}: {
  accountID: string;
  finance: Finance | null;
  methods: PaymentMethod[];
  mutation: FinancialMutation;
  canCorrect: boolean;
  reloadFinance: () => Promise<void>;
}) {
  const [paymentID, setPaymentID] = useState('');
  const [methodID, setMethodID] = useState('');
  const [reference, setReference] = useState('');
  const [reason, setReason] = useState('');
  const [notice, setNotice] = useState('');
  const [suggestRefund, setSuggestRefund] = useState(false);
  const [result, setResult] = useState<CorrectionResult | null>(null);
  // Sin permiso propio, la corrección requiere desde el inicio la autorización de un supervisor.
  const auth = useSupervisorAuthorization(!canCorrect);
  const original = finance?.payments.find((payment) => payment.id === paymentID) ?? null;
  const replacements = original
    ? methods.filter(
        (method) =>
          method.active &&
          method.id !== original.method_id &&
          (isZeroMoney(original.tip) || method.allows_tip),
      )
    : [];
  const method = replacements.find((entry) => entry.id === methodID) ?? null;
  const locked = mutation.busy || !!mutation.pending;
  const referenceMissing = method?.reference_mode === 'required' && reference.trim() === '';
  const canRegister =
    !!original && !!method && validReason(reason) && !referenceMissing && auth.ready && !locked;

  function clearNotices() {
    setNotice('');
    setSuggestRefund(false);
    mutation.clearError();
  }

  function start(payment: PaymentRecord) {
    if (locked) {
      return;
    }
    setPaymentID(payment.id);
    setMethodID('');
    setReference('');
    setReason('');
    setResult(null);
    clearNotices();
  }

  function cancel() {
    setPaymentID('');
    clearNotices();
  }

  function chooseMethod(selected: PaymentMethod) {
    if (locked) {
      return;
    }
    setMethodID(selected.id);
    clearNotices();
  }

  function onRecorded(value: CorrectionResult) {
    setResult(value);
    setPaymentID('');
    auth.reset();
  }

  async function register() {
    if (!canRegister || !original || !method || mutation.isPending()) {
      return;
    }
    const authorization = auth.take();
    const body: CorrectionInput = {
      payment_id: original.id,
      method_id: method.id,
      reference: method.reference_mode === 'none' ? '' : reference.trim(),
      reason: reason.trim(),
    };
    if (authorization) {
      body.authorization = authorization;
    }
    clearNotices();
    try {
      await mutation.execute<CorrectionResult>('payments_correct', {
        id: accountID,
        body: { ...body },
      });
    } catch (problem) {
      if (auth.handleError(problem)) {
        return;
      }
      // Estos dos rechazos se explican en el propio panel de corrección, con la alternativa que
      // corresponde (regularización o devolución); los demás se avisan como mensaje.
      if (problem instanceof APIError && problem.code === 'SHIFT_CLOSED') {
        setNotice(closedShiftMessage);
      } else if (problem instanceof APIError && problem.code === 'PAYMENT_NOT_CORRECTABLE') {
        setNotice(`${problem.message} Registra una devolución en su lugar.`);
        setSuggestRefund(true);
        void reloadFinance();
      } else {
        notifyActionError(problem, 'No se pudo registrar la corrección.');
      }
    }
  }

  return {
    paymentID,
    original,
    replacements,
    method,
    reference,
    setReference,
    reason,
    setReason,
    referenceMissing,
    notice,
    suggestRefund,
    result,
    auth,
    locked,
    canRegister,
    start,
    cancel,
    chooseMethod,
    onRecorded,
    register,
  };
}

export type CorrectionFlow = ReturnType<typeof useCorrectionFlow>;
