import { IconLabel } from '../../../components/Icon';
import { useRestaurantMutation } from '../../../hooks/useRestaurantMutation';
import { notifySuccess } from '../../../services/notify';
import {
  documentJobLabels,
  documentKindLabels,
  documentsPendingKey,
  printAttentionStates,
} from '../../../services/payments';
import type { DocumentRecord, PaymentMethod, ReprintResult } from '../../../services/payments';
import { countLabel, pluralWord } from '../../../services/plural';
import { serverDateTime } from '../../../services/restaurant';
import { UncertainOperation } from '../checkout/UncertainOperation';
import { notifyActionError } from '../checkout/actionErrors';

function copiesLabel(copies: number) {
  return `${countLabel(copies, 'copia', 'copias')} ${pluralWord(copies, 'solicitada', 'solicitadas')}`;
}

// Tickets y precuentas de la cuenta con el estado de su último trabajo de impresión. Reimprimir
// solicita otra copia idéntica sin duplicar ningún cobro; usa una clave propia de pendientes para
// que una impresión incierta no bloquee los cobros de la cuenta.
export function DocumentList({
  accountID,
  documents,
  timezone,
  can,
  methods = [],
}: {
  accountID: string;
  documents: DocumentRecord[];
  timezone: string;
  can: (permission: string) => boolean;
  methods?: PaymentMethod[];
}) {
  const mutation = useRestaurantMutation((_operation, value) => {
    const result = value as ReprintResult;
    notifySuccess(
      `Copia solicitada: ${documentJobLabels[result.status] ?? 'en la cola de impresión'}.`,
    );
    window.dispatchEvent(new CustomEvent('adel-server-change'));
  }, documentsPendingKey(accountID));
  const locked = mutation.busy || !!mutation.pending;
  const canReprint = can('payments.tickets.reprint');

  async function reprint(ticketID: string) {
    if (mutation.isPending()) {
      return;
    }
    try {
      await mutation.execute('payments_reprint', {
        id: accountID,
        body: { ticket_id: ticketID },
      });
    } catch (problem) {
      // Go explica el motivo (copia pendiente o trabajo sin resolver).
      notifyActionError(problem, 'No se pudo solicitar la copia.');
    }
  }

  async function retry() {
    const problem = await mutation.retry();
    if (problem) {
      notifyActionError(problem, 'No se pudo solicitar la copia.');
    }
  }

  if (documents.length === 0) {
    return null;
  }
  return (
    <section className="document-list" aria-label="Documentos de la cuenta">
      <h3 className="bill-section-title">Documentos</h3>
      {mutation.pending && mutation.uncertain && (
        <UncertainOperation
          request={mutation.pending}
          methods={methods}
          busy={mutation.busy}
          onRetry={() => void retry()}
          onConsult={() => window.dispatchEvent(new CustomEvent('adel-server-change'))}
        />
      )}
      <ol className="payment-history">
        {documents.map((document) => {
          const state = document.job_state ?? '';
          const attention = printAttentionStates.includes(state);
          return (
            <li
              key={document.ticket_id}
              className={`payment-history-item document-item ${attention ? 'document-attention' : ''}`}
            >
              <div className="payment-history-heading">
                <strong>{documentKindLabels[document.kind] ?? 'Documento'}</strong>
                <span className="document-state">
                  {state ? (documentJobLabels[state] ?? state) : 'Sin trabajo de impresión'}
                </span>
              </div>
              <p className="muted text-xs">
                {serverDateTime(document.created_at, timezone)}
                {document.printer_name && ` · ${document.printer_name}`} ·{' '}
                {copiesLabel(document.copies)}
              </p>
              {attention && (
                <p className="payment-history-detail payment-history-flag">
                  Se resuelve en Cocina → Cola de impresión.
                </p>
              )}
              {canReprint && (
                <div className="button-row payment-history-actions">
                  <button
                    type="button"
                    className="secondary-button"
                    disabled={locked}
                    onClick={() => void reprint(document.ticket_id)}
                  >
                    <IconLabel icon="printer">
                      Reimprimir {documentKindLabels[document.kind]?.toLowerCase() ?? 'documento'}
                    </IconLabel>
                  </button>
                </div>
              )}
            </li>
          );
        })}
      </ol>
    </section>
  );
}
