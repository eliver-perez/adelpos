import { useConfirmDialog } from '../../../components/ConfirmDialog';
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
import {
  renderCurrentConfirmation,
  renderCurrentLabel,
  renderCurrentPermission,
} from '../../documents/rerender';
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
  const mutation = useRestaurantMutation((operation, value) => {
    const result = value as ReprintResult;
    const state = documentJobLabels[result.status] ?? 'en la cola de impresión';
    // Etapa 9B: la impresión con la plantilla vigente es un documento nuevo, no una copia.
    notifySuccess(
      operation === 'payments_rerender'
        ? `Documento con la plantilla vigente solicitado: ${state}.`
        : `Copia solicitada: ${state}.`,
    );
    window.dispatchEvent(new CustomEvent('adel-server-change'));
  }, documentsPendingKey(accountID));
  const locked = mutation.busy || !!mutation.pending;
  const canReprint = can('payments.tickets.reprint');
  const canRenderCurrent = canReprint && can(renderCurrentPermission);
  const { confirm, dialog } = useConfirmDialog();

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

  // Etapa 9B: otra representación del mismo documento con la plantilla activa, confirmada antes.
  async function renderCurrent(ticketID: string, kind: string) {
    if (mutation.isPending()) {
      return;
    }
    const subject = kind === 'precheck' ? 'la precuenta' : 'el ticket';
    if (!(await confirm(renderCurrentConfirmation(subject)))) {
      return;
    }
    try {
      await mutation.execute('payments_rerender', {
        id: accountID,
        body: { ticket_id: ticketID },
      });
    } catch (problem) {
      notifyActionError(problem, 'No se pudo imprimir con la plantilla vigente.');
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
      {dialog}
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
                  {canRenderCurrent && (
                    <button
                      type="button"
                      className="secondary-button"
                      disabled={locked}
                      onClick={() => void renderCurrent(document.ticket_id, document.kind)}
                    >
                      {renderCurrentLabel}
                    </button>
                  )}
                </div>
              )}
            </li>
          );
        })}
      </ol>
    </section>
  );
}
