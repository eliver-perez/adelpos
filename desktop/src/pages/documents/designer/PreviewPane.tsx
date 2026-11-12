import { SegmentedControl } from '../../../components/SegmentedControl';
import { useResource } from '../../../hooks/useResource';
import type {
  DocumentKind,
  DocumentPreview,
  PaperWidth,
  PrinterCompatibility,
  RecentDocument,
} from '../../../services/documents';
import { serverDateTime } from '../../../services/restaurant';
import { PaperStrip } from './PaperStrip';
import type { PreviewProblem } from './usePreview';
import { Select } from '../../../components/Select';

// Área central del editor: la tira de papel con la IR de Go, el ancho (58/80 mm con sus columnas
// predeterminadas o, desde la etapa 9C, el de una impresora activa con sus columnas), el origen de
// los datos (muestra o documento real) y los avisos de Go. La marca «Vista previa con datos de
// muestra» deja claro que nada de lo que se ve es una venta ni un movimiento.

export type PreviewMode = 'sample' | 'real';

export function PreviewPane({
  kind,
  preview,
  problem,
  loading,
  paperWidth,
  printers,
  printerID,
  mode,
  ticketID,
  canPreviewReal,
  selectedID,
  timezone,
  blockName,
  onPaperWidth,
  onPrinter,
  onMode,
  onTicket,
  onSelect,
}: {
  kind: DocumentKind;
  preview: DocumentPreview | null;
  problem: PreviewProblem | null;
  loading: boolean;
  paperWidth: PaperWidth;
  printers: PrinterCompatibility[] | null;
  printerID: string;
  mode: PreviewMode;
  ticketID: string;
  canPreviewReal: boolean;
  selectedID: string | null;
  timezone: string;
  blockName: (blockID: string) => string;
  onPaperWidth: (width: PaperWidth) => void;
  onPrinter: (printerID: string) => void;
  onMode: (mode: PreviewMode) => void;
  onTicket: (ticketID: string) => void;
  onSelect: (blockID: string) => void;
}) {
  const recent = useResource<RecentDocument[]>('documents_recent', {
    query: { kind },
    enabled: canPreviewReal && mode === 'real',
  });
  const shown = preview;
  const realWithoutTicket = mode === 'real' && !ticketID;
  const sourceLabel =
    mode === 'sample' ? 'Vista previa con datos de muestra' : 'Vista previa con un documento real';
  const printer = (printers ?? []).find((entry) => entry.id === printerID);
  // El papel que se ve: el de la impresora elegida o el ancho predeterminado elegido.
  const shownPaper = printer ? printer.paper_width : paperWidth;
  const widthLabel = shown
    ? shown.printer_name
      ? `${shown.printer_name} · ${shown.paper_width} mm · ${shown.columns} columnas`
      : `${shown.columns} columnas`
    : '';

  return (
    <section className="designer-preview" aria-label="Vista previa">
      <div className="designer-preview-controls">
        <SegmentedControl
          label="Ancho del papel"
          value={String(shownPaper) as '58' | '80'}
          options={[
            { value: '58', label: '58 mm' },
            { value: '80', label: '80 mm' },
          ]}
          onChange={(value) => onPaperWidth(Number(value) as PaperWidth)}
        />
        {canPreviewReal && (
          <SegmentedControl
            label="Datos de la vista previa"
            value={mode}
            options={[
              { value: 'sample', label: 'Datos de muestra' },
              { value: 'real', label: 'Documento real' },
            ]}
            onChange={onMode}
          />
        )}
      </div>
      {(printers ?? []).length > 0 && (
        <label className="field">
          <span>Ver con el ancho de una impresora</span>
          <Select
            value={printer ? printerID : ''}
            options={[
              { value: '', label: 'Ancho predeterminado del papel' },
              ...(printers ?? []).map((entry) => ({
                value: entry.id,
                label: `${entry.name} · ${entry.paper_width} mm · columnas: ${entry.columns}${
                  entry.columns === entry.default_columns ? ' (predeterminado)' : ''
                }`,
              })),
            ]}
            onChange={onPrinter}
          />
          <small className="muted">
            Las columnas de cada impresora se eligen en Cocina → Impresoras.
          </small>
        </label>
      )}
      {mode === 'real' && (
        <label className="field">
          <span>Documento reciente</span>
          <Select
            value={ticketID}
            options={[
              { value: '', label: 'Elige un documento' },
              ...(recent.data ?? []).map((document) => ({
                value: document.ticket_id,
                label: `${document.label} · ${serverDateTime(document.created_at, timezone)}`,
              })),
            ]}
            onChange={onTicket}
          />
          {recent.data?.length === 0 && (
            <small className="muted">
              No hay documentos recientes de este tipo que puedas ver.
            </small>
          )}
          {recent.error && <small className="muted">{recent.error}</small>}
        </label>
      )}
      <div
        className={`designer-preview-mark ${mode === 'real' ? 'designer-preview-mark-real' : ''}`}
        role="note"
      >
        <strong>{sourceLabel}</strong>
        <span>
          {mode === 'sample'
            ? 'Datos ficticios: no son ventas, pagos ni movimientos.'
            : 'Datos históricos del documento elegido con esta plantilla; no se imprime ni se guarda nada.'}
        </span>
        <span className="designer-preview-status">{loading ? 'Actualizando…' : widthLabel}</span>
      </div>
      {problem && (
        <div className="designer-preview-problem" role="alert">
          <strong>No se pudo armar esta vista previa.</strong>
          <span>{problem.message}</span>
          {problem.blockID && (
            <button
              type="button"
              className="secondary-button"
              onClick={() => onSelect(problem.blockID)}
            >
              Ir a {blockName(problem.blockID)}
            </button>
          )}
        </div>
      )}
      <div className="designer-paper-stage">
        {realWithoutTicket && (
          <p className="muted">
            Elige un documento reciente para ver cómo saldría con esta plantilla.
          </p>
        )}
        {!realWithoutTicket && shown && (
          <PaperStrip
            preview={shown}
            selectedID={selectedID}
            blockName={blockName}
            onSelect={onSelect}
            label={`${sourceLabel}, papel de ${shown.paper_width} mm, ${shown.columns} columnas`}
          />
        )}
        {!realWithoutTicket && !shown && !problem && (
          <p className="muted" role="status">
            Generando la vista previa…
          </p>
        )}
      </div>
      {shown && !realWithoutTicket && (
        <PreviewNotes preview={shown} blockName={blockName} onSelect={onSelect} />
      )}
    </section>
  );
}

// Avisos de Go: advertencias del documento y bloques visibles que no imprimieron nada.
function PreviewNotes({
  preview,
  blockName,
  onSelect,
}: {
  preview: DocumentPreview;
  blockName: (blockID: string) => string;
  onSelect: (blockID: string) => void;
}) {
  if (preview.warnings.length === 0 && preview.empty_blocks.length === 0) {
    return null;
  }
  return (
    <div className="designer-preview-notes">
      {preview.warnings.map((warning) => (
        <p key={warning} className="designer-preview-warning">
          {warning}
        </p>
      ))}
      {preview.empty_blocks.length > 0 && (
        <>
          <h3>No imprimen nada con estos datos</h3>
          <ul>
            {preview.empty_blocks.map((empty) => (
              <li key={empty.block_id}>
                <button
                  type="button"
                  className="designer-link"
                  onClick={() => onSelect(empty.block_id)}
                >
                  {blockName(empty.block_id)}
                </button>
                <span className="muted"> · {empty.reason}</span>
              </li>
            ))}
          </ul>
        </>
      )}
    </div>
  );
}
