import { useEffect, useState } from 'react';
import { useConfirmDialog } from '../../../components/ConfirmDialog';
import { Notice } from '../../../components/Forms';
import { useResource } from '../../../hooks/useResource';
import { documentVersion } from '../../../services/documents';
import type { TemplateSummary, VersionDetail, VersionSummary } from '../../../services/documents';
import { errorMessage } from '../../../services/notify';
import { serverDateTime } from '../../../services/restaurant';
import { ModalPanel } from '../ModalPanel';
import { LayoutPreview } from './LayoutPreview';

// Historial de versiones de una plantilla: número, fecha, autor y nota de cada versión, la vista
// previa de solo lectura de cualquiera de ellas y «Restaurar esta versión», que la guarda como una
// versión nueva (las versiones son inmutables: nada se sobrescribe).
export function HistoryDialog({
  template,
  timezone,
  canRestore,
  restoreBlocked,
  onRestore,
  onClose,
}: {
  template: TemplateSummary;
  timezone: string;
  // Con documents.templates.manage y en una plantilla del restaurante.
  canRestore: boolean;
  // Motivo por el que no se puede restaurar ahora (borrador sin guardar); vacío si se puede.
  restoreBlocked: string;
  onRestore: (version: VersionDetail) => Promise<void>;
  onClose: () => void;
}) {
  const versions = useResource<VersionSummary[]>('documents_template_versions', {
    id: template.id,
  });
  const [selectedID, setSelectedID] = useState(template.current_version.id);
  const [detail, setDetail] = useState<VersionDetail | null>(null);
  const [problem, setProblem] = useState('');
  const [busy, setBusy] = useState(false);
  const { confirm, dialog } = useConfirmDialog();

  useEffect(() => {
    let active = true;
    setProblem('');
    documentVersion(selectedID)
      .then((version) => {
        if (active) {
          setDetail(version);
        }
      })
      .catch((error: unknown) => {
        if (active) {
          setProblem(errorMessage(error, 'No se pudo consultar la versión.'));
        }
      });
    return () => {
      active = false;
    };
  }, [selectedID]);

  async function restore() {
    if (!detail) {
      return;
    }
    const confirmed = await confirm({
      title: `Restaurar la versión ${detail.number}`,
      message: (
        <>
          <p>
            Se guarda una versión nueva de «{template.name}» con el contenido de la versión{' '}
            {detail.number}. Las versiones anteriores se conservan.
          </p>
          {template.active && <p>La plantilla está activa: los documentos nuevos la usarán.</p>}
        </>
      ),
      confirmLabel: 'Restaurar esta versión',
    });
    if (!confirmed) {
      return;
    }
    setBusy(true);
    try {
      await onRestore(detail);
    } finally {
      setBusy(false);
    }
  }

  const current = detail?.id === template.current_version.id;
  // La confirmación va después del panel para quedar por encima de él.
  return (
    <>
      <ModalPanel title={`Historial de «${template.name}»`} wide onClose={onClose}>
        <div className="designer-history">
          <div className="designer-history-list">
            <Notice>{versions.error}</Notice>
            {versions.loading && !versions.data && (
              <p className="muted" role="status">
                Cargando versiones…
              </p>
            )}
            <ol aria-label="Versiones de la plantilla">
              {(versions.data ?? []).map((version) => (
                <li key={version.id}>
                  <button
                    type="button"
                    className={`designer-history-item ${version.id === selectedID ? 'designer-history-selected' : ''}`}
                    aria-pressed={version.id === selectedID}
                    onClick={() => setSelectedID(version.id)}
                  >
                    <strong>
                      Versión {version.number}
                      {version.id === template.current_version.id ? ' · vigente' : ''}
                    </strong>
                    <span className="muted text-xs">
                      {serverDateTime(version.created_at, timezone)} ·{' '}
                      {version.created_by_name ?? 'Sistema'}
                    </span>
                    {version.note && <span className="text-xs">{version.note}</span>}
                  </button>
                </li>
              ))}
            </ol>
          </div>
          <div className="designer-history-preview">
            {problem && (
              <p className="checkout-inline-note" role="alert">
                {problem}
              </p>
            )}
            {detail && (
              <>
                <h3>Versión {detail.number}</h3>
                <LayoutPreview
                  key={detail.id}
                  kind={detail.kind}
                  layout={detail.layout}
                  label={`Versión ${detail.number}`}
                />
                {canRestore && !current && (
                  <div className="button-row">
                    <button
                      type="button"
                      className="primary-button"
                      disabled={busy || restoreBlocked !== ''}
                      onClick={() => void restore()}
                    >
                      {busy ? 'Restaurando…' : 'Restaurar esta versión'}
                    </button>
                    {restoreBlocked && <span className="muted text-xs">{restoreBlocked}</span>}
                  </div>
                )}
              </>
            )}
          </div>
        </div>
      </ModalPanel>
      {dialog}
    </>
  );
}
