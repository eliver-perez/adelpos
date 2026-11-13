import { useEffect, useRef, useState } from 'react';
import { useConfirmDialog } from '../../../components/ConfirmDialog';
import { IconLabel } from '../../../components/Icon';
import { ReprintReason } from '../../../components/ReprintReason';
import { useRestaurantMutation } from '../../../hooks/useRestaurantMutation';
import { notifySuccess, notifyWarning } from '../../../services/notify';
import {
  documentJobLabels,
  documentKindLabels,
  documentsPendingKey,
  printAttentionStates,
} from '../../../services/payments';
import type { DocumentRecord, PaymentMethod, ReprintResult } from '../../../services/payments';
import { countLabel, pluralWord } from '../../../services/plural';
import {
  notifyPrinterRejection,
  printProblemHint,
  printProblemMessage,
  printerActions,
  retryableProblem,
} from '../../../services/printProblems';
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

// Nombre del documento con «de» y su artículo («de la precuenta», «del ticket de pago»).
function documentSubject(document: DocumentRecord) {
  const label = documentKindLabels[document.kind] ?? 'Documento';
  const masculine = document.kind === 'receipt' || label.startsWith('Ticket');
  return `${masculine ? 'del' : 'de la'} ${label.toLowerCase()}`;
}

// Qué pasó con la impresión de un documento, en palabras del restaurante (etapa 9F, §4.2): la impresora
// de caja es la que imprime precuentas y tickets.
function problemText(document: DocumentRecord) {
  return printProblemMessage(document.job_problem, document.printer_name, 'cash');
}

// Tickets y precuentas de la cuenta con el estado de su último trabajo de impresión. Reimprimir
// solicita otra copia idéntica sin duplicar ningún cobro y, desde la etapa 9F, pide motivo; un
// documento que nunca salió en papel se reintenta con un toque, sin motivo. Usa una clave propia de
// pendientes para que una impresión incierta no bloquee los cobros de la cuenta.
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
    if (operation === 'payments_rerender') {
      notifySuccess(`Documento con la plantilla vigente solicitado: ${state}.`);
    } else if (operation === 'payments_retry_document') {
      notifySuccess(`Reintento enviado a la impresora: ${state}.`);
    } else {
      notifySuccess(`Copia solicitada: ${state}.`);
    }
    window.dispatchEvent(new CustomEvent('adel-server-change'));
  }, documentsPendingKey(accountID));
  const locked = mutation.busy || !!mutation.pending;
  const canReprint = can('payments.tickets.reprint');
  const canRetry = can('payments.tickets.print');
  const canRenderCurrent = canReprint && can(renderCurrentPermission);
  const { confirm, dialog } = useConfirmDialog();
  const [reprinting, setReprinting] = useState<string | null>(null);
  const retryRef = useRef<(document: DocumentRecord) => void>(() => undefined);

  async function reprint(ticketID: string, reason: string) {
    if (mutation.isPending()) {
      return;
    }
    try {
      await mutation.execute('payments_reprint', {
        id: accountID,
        body: { ticket_id: ticketID, reason },
      });
      setReprinting(null);
    } catch (problem) {
      // Go explica el motivo (copia pendiente o trabajo sin resolver).
      notifyActionError(problem, 'No se pudo solicitar la copia.');
    }
  }

  // Etapa 9F (§4.2): un toque, sin motivo, con el permiso de imprimir tickets y precuentas.
  async function retryDocument(document: DocumentRecord) {
    if (mutation.isPending()) {
      return;
    }
    try {
      await mutation.execute('payments_retry_document', {
        id: accountID,
        body: { ticket_id: document.ticket_id, job_id: document.job_id ?? '' },
      });
    } catch (problem) {
      if (!notifyPrinterRejection(problem, () => retryRef.current(document))) {
        notifyActionError(problem, 'No se pudo reintentar la impresión.');
      }
    }
  }
  retryRef.current = (document) => void retryDocument(document);

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

  // Aviso del error en el momento (etapa 9F, §4.2): cuando un documento que esta pantalla veía en
  // camino a la impresora pasa a error o a intervención, se avisa una vez qué pasó y se ofrece
  // «Reintentar» (los reintentos automáticos del mismo trabajo no repiten el aviso). El aviso se
  // cierra solo; la explicación y «Reintentar» siguen en el documento mientras no salga.
  const seen = useRef(new Map<string, string>());
  const notified = useRef(new Set<string>());
  useEffect(() => {
    for (const document of documents) {
      const key = `${document.ticket_id}:${document.job_id ?? ''}`;
      const state = document.job_state ?? '';
      const before = seen.current.get(key);
      seen.current.set(key, state);
      const wasOnItsWay = before === 'pending' || before === 'processing';
      if (!wasOnItsWay || !retryableProblem(state) || notified.current.has(key)) {
        continue;
      }
      notified.current.add(key);
      const subject = documentKindLabels[document.kind] ?? 'Documento';
      const retryAction = canRetry ? () => retryRef.current(document) : undefined;
      notifyWarning(`${subject}: ${problemText(document) || 'la impresión no salió.'}`, {
        id: `print-failed:${document.ticket_id}`,
        description: printProblemHint(document.job_problem, document.printer_name),
        actions:
          document.job_problem === 'unreachable'
            ? retryAction
              ? [{ label: 'Reintentar', onClick: retryAction }]
              : []
            : printerActions(retryAction),
      });
    }
  }, [documents, canRetry]);

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
          const label = documentKindLabels[document.kind] ?? 'Documento';
          return (
            <li
              key={document.ticket_id}
              className={`payment-history-item document-item ${attention ? 'document-attention' : ''}`}
            >
              <div className="payment-history-heading">
                <strong>{label}</strong>
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
                  {problemText(document)}{' '}
                  {printProblemHint(document.job_problem, document.printer_name)}
                </p>
              )}
              {retryableProblem(state) && canRetry && (
                <div className="button-row payment-history-actions">
                  <button
                    type="button"
                    className="secondary-button"
                    disabled={locked}
                    onClick={() => void retryDocument(document)}
                  >
                    <IconLabel icon="printer">Reintentar</IconLabel>
                  </button>
                </div>
              )}
              {canReprint && reprinting !== document.ticket_id && (
                <div className="button-row payment-history-actions">
                  <button
                    type="button"
                    className="secondary-button"
                    disabled={locked}
                    onClick={() => setReprinting(document.ticket_id)}
                  >
                    <IconLabel icon="printer">Reimprimir {label.toLowerCase()}</IconLabel>
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
              {canReprint && reprinting === document.ticket_id && (
                <ReprintReason
                  subject={documentSubject(document)}
                  busy={locked}
                  onSubmit={(reason) => void reprint(document.ticket_id, reason)}
                  onCancel={() => setReprinting(null)}
                />
              )}
            </li>
          );
        })}
      </ol>
    </section>
  );
}
