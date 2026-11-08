import { useEffect, useRef, useState } from 'react';
import { APIError } from '../services/api';
import { previewPayment } from '../services/payments';
import type { PaymentInput, PaymentPlan } from '../services/payments';

interface PreviewState {
  key: string;
  plan: PaymentPlan | null;
  error: APIError | null;
}

const emptyState: PreviewState = { key: '', plan: null, error: null };

function asAPIError(problem: unknown) {
  if (problem instanceof APIError) {
    return problem;
  }
  return new APIError(
    'PREVIEW_ERROR',
    problem instanceof Error ? problem.message : 'No se pudo validar el cobro.',
  );
}

// Solicita a Go el cálculo de la captura (importe, cambio, saldo después y total de selección)
// con un pequeño retardo tras cada cambio. Una respuesta que llega después de otra captura más
// reciente se descarta: solo se muestra el cálculo que corresponde a lo que ve el cajero.
//
// financeKey identifica el estado financiero sobre el que se calculó; si la cuenta cambia, el
// cálculo anterior deja de ser válido y se vuelve a pedir.
export function usePaymentPreview(
  accountID: string,
  input: PaymentInput | null,
  financeKey: string,
  delay = 160,
) {
  const serialized = input ? JSON.stringify(input) : '';
  const [nonce, setNonce] = useState(0);
  const key = input ? `${accountID}|${financeKey}|${nonce}|${serialized}` : '';
  const [state, setState] = useState<PreviewState>(emptyState);
  const generation = useRef(0);

  useEffect(() => {
    if (!key) {
      return;
    }
    const request = ++generation.current;
    const body = JSON.parse(serialized) as PaymentInput;
    const timer = window.setTimeout(() => {
      previewPayment(accountID, body)
        .then((plan) => {
          if (request === generation.current) {
            setState({ key, plan, error: null });
          }
        })
        .catch((problem: unknown) => {
          if (request === generation.current) {
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
