import { useEffect, useRef, useState } from 'react';
import { APIError } from '../services/api';
import { previewDiscount, previewPayment, previewRefund } from '../services/payments';
import type {
  DiscountInput,
  DiscountPlan,
  PaymentInput,
  PaymentPlan,
  RefundInput,
  RefundPlan,
} from '../services/payments';

interface PreviewState<TPlan> {
  key: string;
  plan: TPlan | null;
  error: APIError | null;
}

function asAPIError(problem: unknown) {
  if (problem instanceof APIError) {
    return problem;
  }
  return new APIError(
    'PREVIEW_ERROR',
    problem instanceof Error ? problem.message : 'No se pudo validar la operación.',
  );
}

// Solicita a Go el cálculo de una captura financiera con un pequeño retardo tras cada cambio. Una
// respuesta que llega después de otra captura más reciente se descarta: solo se muestra el
// cálculo que corresponde a lo que ve el cajero.
//
// financeKey identifica el estado financiero sobre el que se calculó; si la cuenta cambia, el
// cálculo anterior deja de ser válido y se vuelve a pedir.
export function useFinancePreview<TInput, TPlan>(
  request: (accountID: string, input: TInput) => Promise<TPlan>,
  accountID: string,
  input: TInput | null,
  financeKey: string,
  delay = 160,
) {
  const serialized = input ? JSON.stringify(input) : '';
  const [nonce, setNonce] = useState(0);
  const key = input ? `${accountID}|${financeKey}|${nonce}|${serialized}` : '';
  const [state, setState] = useState<PreviewState<TPlan>>({ key: '', plan: null, error: null });
  const generation = useRef(0);
  // La función de consulta es estable por tipo de previsualización; se lee desde una referencia.
  const requester = useRef(request);
  requester.current = request;

  useEffect(() => {
    if (!key) {
      return;
    }
    const current = ++generation.current;
    const body = JSON.parse(serialized) as TInput;
    const timer = window.setTimeout(() => {
      requester
        .current(accountID, body)
        .then((plan) => {
          if (current === generation.current) {
            setState({ key, plan, error: null });
          }
        })
        .catch((problem: unknown) => {
          if (current === generation.current) {
            setState({ key, plan: null, error: asAPIError(problem) });
          }
        });
    }, delay);
    return () => {
      window.clearTimeout(timer);
      // Invalida la solicitud en curso: su respuesta ya no corresponde a la captura vigente.
      generation.current++;
    };
  }, [accountID, key, serialized, delay]);

  const current = key !== '' && state.key === key;
  return {
    plan: current ? state.plan : null,
    error: current ? state.error : null,
    validating: key !== '' && !current,
    revalidate: () => setNonce((value) => value + 1),
  };
}

export function usePaymentPreview(
  accountID: string,
  input: PaymentInput | null,
  financeKey: string,
  delay = 160,
) {
  return useFinancePreview<PaymentInput, PaymentPlan>(
    previewPayment,
    accountID,
    input,
    financeKey,
    delay,
  );
}

export function useDiscountPreview(
  accountID: string,
  input: DiscountInput | null,
  financeKey: string,
) {
  return useFinancePreview<DiscountInput, DiscountPlan>(
    previewDiscount,
    accountID,
    input,
    financeKey,
  );
}

export function useRefundPreview(accountID: string, input: RefundInput | null, financeKey: string) {
  return useFinancePreview<RefundInput, RefundPlan>(previewRefund, accountID, input, financeKey);
}
