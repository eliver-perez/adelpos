import { IconLabel } from '../../../components/Icon';
import { useRestaurantMutation } from '../../../hooks/useRestaurantMutation';
import { notifySuccess } from '../../../services/notify';
import { documentJobLabels, documentsPendingKey } from '../../../services/payments';
import type { PrintingResult } from '../../../services/payments';
import {
  notifyPrinterRejection,
  openPrinterSetupScreen,
  printerSetupAvailable,
  printerSetupLabel,
} from '../../../services/printProblems';
import { notifyActionError } from './actionErrors';

// Ticket de pago que no se imprimió por falta de una impresora válida (etapa 9F, §4.1): el pago ya quedó
// registrado y el resultado lo dice; aquí, «Configurar impresora» (si el usuario puede elegirla) y
// «Reintentar», que imprime el ticket de ese pago una vez configurada (Go valida el permiso de imprimir y
// que el pago aún no tenga ticket).
export function ReceiptRecovery({
  accountID,
  paymentID,
}: {
  accountID: string;
  paymentID: string;
}) {
  const mutation = useRestaurantMutation((_operation, value) => {
    const state =
      documentJobLabels[(value as PrintingResult).status ?? ''] ?? 'en la cola de impresión';
    notifySuccess(`Ticket de pago enviado a la cola de impresión: ${state}.`);
    window.dispatchEvent(new CustomEvent('adel-server-change'));
  }, documentsPendingKey(accountID));
  const locked = mutation.busy || !!mutation.pending;

  async function print() {
    if (mutation.isPending()) {
      return;
    }
    try {
      await mutation.execute('payments_print_receipt', {
        id: accountID,
        body: { payment_id: paymentID },
      });
    } catch (problem) {
      if (!notifyPrinterRejection(problem, () => void print(), 'printer-missing:receipt')) {
        notifyActionError(problem, 'No se pudo imprimir el ticket de pago.');
      }
    }
  }

  return (
    <div className="button-row receipt-recovery" aria-label="Imprimir el ticket de pago">
      {printerSetupAvailable() && (
        <button type="button" className="secondary-button" onClick={openPrinterSetupScreen}>
          {printerSetupLabel}
        </button>
      )}
      <button
        type="button"
        className="secondary-button"
        disabled={locked}
        onClick={() => void print()}
      >
        <IconLabel icon="printer">Reintentar</IconLabel>
      </button>
    </div>
  );
}
