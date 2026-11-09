import { useState } from 'react';
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
import { PrintNotice } from './CashNotices';
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
  return {
    print,
    reprint: !!document && reprintAllowed && !expectedBlocked,
    expectedBlocked: !!document && reprintAllowed && expectedBlocked,
  };
}

// Estado visible del documento: tipo, último trabajo y copias, sin identificadores internos.
function documentSummary(document: CashDocumentRecord) {
  const state = document.job_state ? (cashJobLabels[document.job_state] ?? document.job_state) : '';
  const copies = document.copies === 1 ? '1 envío' : `${document.copies} envíos`;
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
  const [outcome, setOutcome] = useState<PrintOutcome | null>(null);
  const mutation = useCashMutation(shiftID, undefined, cashDocumentPendingKey(shiftID));
  const document = documentFor(documents.data?.documents ?? [], source);
  const options = documentOptions(can, document);

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
      setOutcome(await mutation.execute<PrintOutcome>(operation, { id: source.id, body: {} }));
      void documents.reload();
    } catch {
      // El mensaje de Go queda visible junto a las acciones.
    }
  }

  async function reprint() {
    if (!document || mutation.isPending()) {
      return;
    }
    try {
      setOutcome(
        await mutation.execute<PrintOutcome>('payments_reprint_cash_document', {
          id: document.id,
          body: {},
        }),
      );
      void documents.reload();
    } catch {
      // El mensaje de Go queda visible junto a las acciones.
    }
  }

  return (
    <div className="cash-document" aria-label="Documento de caja">
      {document && <p className="muted text-xs">{documentSummary(document)}</p>}
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
            Imprimir
          </button>
        )}
        {options.reprint && (
          <button
            type="button"
            className="secondary-button"
            disabled={mutation.locked}
            onClick={() => void reprint()}
          >
            Reimprimir
          </button>
        )}
      </div>
      {options.expectedBlocked && (
        <p className="muted text-xs">
          Este documento incluye el efectivo esperado: reimprimirlo requiere el permiso «
          {permissionName('cash.expected.read')}».
        </p>
      )}
      <PendingCashOperation mutation={mutation} />
      {mutation.error && !mutation.uncertain && (
        <p className="checkout-inline-note" role="alert">
          {mutation.error}
        </p>
      )}
      {previewError && (
        <p className="checkout-inline-note" role="alert">
          {previewError}
        </p>
      )}
      <PrintNotice outcome={outcome} subject="el documento" />
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
