import { useState } from 'react';
import { useConfirmDialog } from '../../components/ConfirmDialog';
import { IconLabel } from '../../components/Icon';
import { ReprintReason } from '../../components/ReprintReason';
import { useLiveResource } from '../../hooks/useLiveResource';
import { APIError } from '../../services/api';
import { cashDocumentKindLabels, cashJobLabels, previewCashDocument } from '../../services/cash';
import type {
  CashDocumentPreview,
  CashDocumentRecord,
  CashDocumentSource,
  PrintOutcome,
} from '../../services/cash';
import { permissionName } from '../../services/payments';
import { countLabel } from '../../services/plural';
import {
  notifyPrinterRejection,
  openPrinterSetupScreen,
  printerSetupAvailable,
  printerSetupLabel,
  printerSetupProblem,
  printProblemHint,
  printProblemMessage,
  retryableProblem,
} from '../../services/printProblems';
import {
  renderCurrentConfirmation,
  renderCurrentLabel,
  renderCurrentPermission,
} from '../documents/rerender';
import { notifyPrintOutcome } from './CashNotices';
import { PendingCashOperation } from './PendingCashOperation';
import { useCashMutation } from './useCashMutation';

// Clave del almacén de pendientes para imprimir documentos de un turno: una impresión incierta no
// bloquea las operaciones de efectivo de la caja.
export function cashDocumentPendingKey(shiftID: string) {
  return `cash-documents:${shiftID}`;
}

// Documento ya registrado de un corte o de una operación, según la lista del turno que entrega Go.
export function documentFor(
  documents: CashDocumentRecord[],
  source: CashDocumentSource,
): CashDocumentRecord | null {
  const found = documents.find((document) =>
    source.kind === 'report'
      ? document.report_id === source.id
      : document.cash_operation_id === source.id,
  );
  return found ?? null;
}

// Opciones de impresión según permisos. Imprimir crea el único documento del corte u operación;
// después solo se reimprime. Un documento con el efectivo esperado exige también poder consultarlo
// (Go lo vuelve a validar al reimprimir).
export function documentOptions(
  can: (permission: string) => boolean,
  document: CashDocumentRecord | null,
) {
  const print = !document && can('cash.reports.print');
  const reprintAllowed = can('cash.reports.reprint');
  const expectedBlocked = !!document?.includes_expected && !can('cash.expected.read');
  const reprint = !!document && reprintAllowed && !expectedBlocked;
  return {
    print,
    reprint,
    // Etapa 9F (§4.2): un documento que nunca salió en papel se reintenta sin motivo con el permiso de
    // imprimir documentos de caja (Go valida además el turno y el esperado).
    retry:
      !!document &&
      retryableProblem(document.job_state) &&
      can('cash.reports.print') &&
      !expectedBlocked,
    // Etapa 9B: otra representación con la plantilla activa, con el mismo permiso de reimprimir
    // más documents.history.render_current (Go valida además el turno y el esperado).
    renderCurrent: reprint && can(renderCurrentPermission),
    expectedBlocked: !!document && reprintAllowed && expectedBlocked,
  };
}

// Estado visible del documento: tipo, último trabajo y copias, sin identificadores internos.
function documentSummary(document: CashDocumentRecord) {
  const state = document.job_state ? (cashJobLabels[document.job_state] ?? document.job_state) : '';
  const copies = countLabel(document.copies, 'envío', 'envíos');
  const parts = [cashDocumentKindLabels[document.kind], state, copies];
  if (document.printer_name) {
    parts.push(document.printer_name);
  }
  if (document.includes_expected) {
    parts.push('incluye el efectivo esperado');
  }
  return parts.filter(Boolean).join(' · ');
}

// Vista previa de solo lectura (renderizada por Go), impresión y reimpresión de un documento de
// caja. Nunca crea movimientos ni toca el turno: Go solo registra el documento y su trabajo.
export function CashDocumentActions({
  source,
  shiftID,
  can,
}: {
  source: CashDocumentSource;
  shiftID: string;
  can: (permission: string) => boolean;
}) {
  const documents = useLiveResource<{ documents: CashDocumentRecord[] }>(
    'payments_shift_documents',
    { id: shiftID, interval: 15000 },
  );
  const [preview, setPreview] = useState<CashDocumentPreview | null>(null);
  const [previewError, setPreviewError] = useState('');
  const [loadingPreview, setLoadingPreview] = useState(false);
  // El resultado de imprimir o reimprimir se avisa al confirmarse, también tras un reintento.
  const mutation = useCashMutation(
    shiftID,
    (_, result) => notifyPrintOutcome(result as PrintOutcome, 'el documento'),
    cashDocumentPendingKey(shiftID),
  );
  const document = documentFor(documents.data?.documents ?? [], source);
  const options = documentOptions(can, document);
  const { confirm, dialog } = useConfirmDialog();
  const [askingReason, setAskingReason] = useState(false);
  // Motivo de «Imprimir con la plantilla vigente» (revisión 9F, B2): otra copia en papel, como reimprimir.
  const [askingRenderReason, setAskingRenderReason] = useState(false);
  const attention = !!document && !!document.job_problem;

  async function togglePreview() {
    if (preview) {
      setPreview(null);
      return;
    }
    setLoadingPreview(true);
    setPreviewError('');
    try {
      setPreview(await previewCashDocument(source));
    } catch (problem) {
      setPreviewError(
        problem instanceof APIError ? problem.message : 'No se pudo consultar la vista previa.',
      );
    } finally {
      setLoadingPreview(false);
    }
  }

  async function print() {
    if (mutation.isPending()) {
      return;
    }
    const operation =
      source.kind === 'report' ? 'payments_print_cash_report' : 'payments_print_cash_operation';
    try {
      await mutation.execute<PrintOutcome>(operation, { id: source.id, body: {} });
      void documents.reload();
    } catch (problem) {
      // Etapa 9F (§4.1): sin impresora válida, «Configurar impresora» y «Reintentar» en el aviso.
      if (!notifyPrinterRejection(problem, () => void print(), 'printer-missing:cash')) {
        mutation.reportFailure(problem, 'No se pudo enviar el documento a imprimir.');
      }
    }
  }

  // Reimprimir un documento que ya salió pide motivo (etapa 9F, decisión D3).
  async function reprint(reason: string) {
    if (!document || mutation.isPending()) {
      return;
    }
    try {
      await mutation.execute<PrintOutcome>('payments_reprint_cash_document', {
        id: document.id,
        body: { reason },
      });
      setAskingReason(false);
      void documents.reload();
    } catch (problem) {
      mutation.reportFailure(problem, 'No se pudo enviar el documento a reimprimir.');
    }
  }

  // Reintentar un documento que nunca salió en papel: un toque, sin motivo.
  async function retryPrint() {
    if (!document || mutation.isPending()) {
      return;
    }
    try {
      await mutation.execute<PrintOutcome>('payments_retry_cash_document', {
        id: document.id,
        body: { job_id: document.job_id ?? '' },
      });
      void documents.reload();
    } catch (problem) {
      if (!notifyPrinterRejection(problem, () => void retryPrint(), 'printer-missing:cash')) {
        mutation.reportFailure(problem, 'No se pudo reintentar la impresión del documento.');
      }
    }
  }

  async function renderCurrent(reason: string) {
    if (!document || mutation.isPending()) {
      return;
    }
    if (!(await confirm(renderCurrentConfirmation('el documento de caja')))) {
      return;
    }
    try {
      await mutation.execute<PrintOutcome>('payments_rerender_cash_document', {
        id: document.id,
        body: { reason },
      });
      setAskingRenderReason(false);
      void documents.reload();
    } catch (problem) {
      mutation.reportFailure(problem, 'No se pudo imprimir con la plantilla vigente.');
    }
  }

  return (
    <div className="cash-document" aria-label="Documento de caja">
      {dialog}
      {document && <p className="muted text-xs">{documentSummary(document)}</p>}
      {attention && document && (
        <p className="checkout-result-warning">
          {printProblemMessage(document.job_problem, document.printer_name, 'cash')}{' '}
          {printProblemHint(document.job_problem, document.printer_name)}
        </p>
      )}
      <div className="button-row">
        <button
          type="button"
          className="secondary-button"
          aria-expanded={!!preview}
          disabled={loadingPreview}
          onClick={() => void togglePreview()}
        >
          {preview ? 'Ocultar vista previa' : loadingPreview ? 'Consultando…' : 'Vista previa'}
        </button>
        {options.print && (
          <button
            type="button"
            className="secondary-button"
            disabled={mutation.locked}
            onClick={() => void print()}
          >
            <IconLabel icon="printer">Imprimir</IconLabel>
          </button>
        )}
        {options.retry && printerSetupProblem(document?.job_problem) && printerSetupAvailable() && (
          <button type="button" className="secondary-button" onClick={openPrinterSetupScreen}>
            {printerSetupLabel}
          </button>
        )}
        {options.retry && (
          <button
            type="button"
            className="secondary-button"
            disabled={mutation.locked}
            onClick={() => void retryPrint()}
          >
            <IconLabel icon="printer">Reintentar</IconLabel>
          </button>
        )}
        {options.reprint && !askingReason && !askingRenderReason && (
          <button
            type="button"
            className="secondary-button"
            disabled={mutation.locked}
            onClick={() => setAskingReason(true)}
          >
            <IconLabel icon="printer">Reimprimir</IconLabel>
          </button>
        )}
        {options.renderCurrent && !askingReason && !askingRenderReason && (
          <button
            type="button"
            className="secondary-button"
            disabled={mutation.locked}
            onClick={() => setAskingRenderReason(true)}
          >
            {renderCurrentLabel}
          </button>
        )}
      </div>
      {options.reprint && askingReason && document && (
        <ReprintReason
          subject={`del ${cashDocumentKindLabels[document.kind].toLowerCase()}`}
          busy={mutation.locked}
          onSubmit={(reason) => void reprint(reason)}
          onCancel={() => setAskingReason(false)}
        />
      )}
      {options.renderCurrent && askingRenderReason && document && (
        <ReprintReason
          action="renderCurrent"
          subject={`del ${cashDocumentKindLabels[document.kind].toLowerCase()}`}
          busy={mutation.locked}
          onSubmit={(reason) => void renderCurrent(reason)}
          onCancel={() => setAskingRenderReason(false)}
        />
      )}
      {options.expectedBlocked && (
        <p className="muted text-xs">
          Este documento incluye el efectivo esperado: reimprimirlo requiere el permiso «
          {permissionName('cash.expected.read')}».
        </p>
      )}
      <PendingCashOperation mutation={mutation} />
      {previewError && (
        <p className="checkout-inline-note" role="alert">
          {previewError}
        </p>
      )}
      {preview && (
        <figure className="cash-document-preview">
          <figcaption className="muted text-xs">
            Vista previa · {preview.title} · papel de {preview.paper_width} mm
            {preview.includes_expected ? ' · incluye el efectivo esperado' : ''}
          </figcaption>
          <pre aria-label={`Vista previa de ${preview.title}`}>{preview.content}</pre>
        </figure>
      )}
    </div>
  );
}
