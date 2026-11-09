import { useState } from 'react';
import { useRestaurantMutation } from '../../hooks/useRestaurantMutation';
import { APIError } from '../../services/api';
import type { Operation } from '../../services/api';
import { cashPendingKey, retryRejectedMessage } from '../../services/cash';

// Escrituras de caja de un turno. Todas comparten la clave cashPendingKey(turno) del almacén global
// de pendientes: una operación incierta sobrevive a la navegación, se reintenta con el mismo
// operation_id y nunca guarda la contraseña de la autorización. Un resultado confirmado invalida
// las consultas de la aplicación (el mismo evento que usa el canal en tiempo real).
export function useCashMutation(
  shiftID: string,
  onResult?: (operation: Operation, result: unknown) => void,
) {
  const [retryNotice, setRetryNotice] = useState('');
  const mutation = useRestaurantMutation((operation, result) => {
    setRetryNotice('');
    window.dispatchEvent(new CustomEvent('adel-server-change'));
    onResult?.(operation, result);
  }, cashPendingKey(shiftID));

  // Reintenta con el mismo identificador. Si Go rechaza el reintento porque el turno ya está en
  // arqueo, cerrado o sustituido, la operación no se registró ni se duplicó: el almacén ya la
  // descartó y se explica al cajero que, si sigue haciendo falta, la registre como nueva.
  async function retry() {
    const problem = await mutation.retry();
    if (problem instanceof APIError) {
      const message = retryRejectedMessage(problem.code, problem.message);
      if (message) {
        setRetryNotice(message);
        mutation.clearError();
        window.dispatchEvent(new CustomEvent('adel-server-change'));
      }
    }
    return problem;
  }

  return {
    ...mutation,
    retry,
    retryNotice,
    clearRetryNotice: () => setRetryNotice(''),
    locked: mutation.busy || !!mutation.pending,
  };
}

export type CashMutation = ReturnType<typeof useCashMutation>;
