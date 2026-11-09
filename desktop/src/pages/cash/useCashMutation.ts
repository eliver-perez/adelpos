import { useRestaurantMutation } from '../../hooks/useRestaurantMutation';
import { APIError } from '../../services/api';
import type { Operation } from '../../services/api';
import { cashPendingKey, retryRejectedMessage } from '../../services/cash';
import { notifyError } from '../../services/notify';

// Escrituras de caja de un turno. Todas comparten la clave cashPendingKey(turno) del almacén global
// de pendientes: una operación incierta sobrevive a la navegación, se reintenta con el mismo
// operation_id y nunca guarda la contraseña de la autorización. Un resultado confirmado invalida
// las consultas de la aplicación (el mismo evento que usa el canal en tiempo real).
//
// pendingKey permite separar escrituras que no deben bloquear las operaciones de efectivo del
// turno, como la impresión de documentos o el comando del cajón (fase C).
//
// Los errores definitivos se avisan con el sistema único de mensajes (etapa 7.1): un aviso
// persistente con el texto de Go. Un resultado incierto no se avisa así: lo explica el panel de
// recuperación (PendingCashOperation), que sigue en pantalla hasta resolverse.
export function useCashMutation(
  shiftID: string,
  onResult?: (operation: Operation, result: unknown) => void,
  pendingKey: string = cashPendingKey(shiftID),
) {
  const mutation = useRestaurantMutation((operation, result) => {
    window.dispatchEvent(new CustomEvent('adel-server-change'));
    onResult?.(operation, result);
  }, pendingKey);

  // Aviso del error definitivo de una escritura. Si la operación quedó incierta, el almacén la
  // conserva y el panel de recuperación ya la explica: no se duplica con otro aviso.
  function reportFailure(
    problem: unknown,
    fallback = 'No se pudo registrar la operación de caja.',
  ) {
    if (mutation.isPending()) {
      return;
    }
    notifyError(problem, fallback);
  }

  // Reintenta con el mismo identificador. Si Go rechaza el reintento porque el turno ya está en
  // arqueo, cerrado o sustituido, la operación no se registró ni se duplicó: el almacén ya la
  // descartó y se explica al cajero que, si sigue haciendo falta, la registre como nueva. Ese
  // resultado del «Reintentar» es un aviso persistente que conserva el texto completo.
  async function retry() {
    const problem = await mutation.retry();
    if (problem instanceof APIError) {
      const message = retryRejectedMessage(problem.code, problem.message);
      if (message) {
        mutation.clearError();
        notifyError('El reintento no se registró.', undefined, { description: message });
        window.dispatchEvent(new CustomEvent('adel-server-change'));
        return problem;
      }
    }
    if (problem) {
      reportFailure(problem);
    }
    return problem;
  }

  return {
    ...mutation,
    retry,
    reportFailure,
    locked: mutation.busy || !!mutation.pending,
  };
}

export type CashMutation = ReturnType<typeof useCashMutation>;
