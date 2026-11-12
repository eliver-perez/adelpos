import { useState } from 'react';
import { ChoiceDialog } from '../../../components/ConfirmDialog';
import type { TemplateDetail } from '../../../services/documents';
import { serverDateTime } from '../../../services/restaurant';
import { ModalPanel } from '../ModalPanel';
import { LayoutPreview } from './LayoutPreview';

// Conflicto de revisión: otra persona guardó la plantilla mientras se editaba. Nunca se sobrescribe
// en silencio ni se pierde el borrador: se muestran opciones explícitas. «Seguir editando» cierra
// el aviso; el siguiente guardado volverá a preguntar.
export function ConflictDialog({
  latest,
  timezone,
  busy,
  onSaveMine,
  onDiscardMine,
  onKeepEditing,
}: {
  latest: TemplateDetail;
  timezone: string;
  busy: boolean;
  onSaveMine: () => void;
  onDiscardMine: () => void;
  onKeepEditing: () => void;
}) {
  const [viewing, setViewing] = useState(false);
  const version = latest.current_version;
  const author = version.created_by_name ?? latest.updated_by_name ?? 'otra persona';

  if (viewing) {
    return (
      <ModalPanel
        title={`Versión ${version.number} guardada por ${author}`}
        wide
        closeLabel="Volver a las opciones"
        onClose={() => setViewing(false)}
        actions={
          <button
            type="button"
            className="primary-button dialog-button"
            onClick={() => setViewing(false)}
          >
            Volver a las opciones
          </button>
        }
      >
        <p className="muted text-sm">
          Así quedó la plantilla que guardó {author}. Tu borrador sigue intacto en el editor.
        </p>
        <LayoutPreview
          kind={latest.kind}
          layout={latest.layout}
          label={`Versión ${version.number}`}
        />
      </ModalPanel>
    );
  }

  return (
    <ChoiceDialog
      title="Otra persona guardó esta plantilla"
      busy={busy}
      message={
        <>
          <p>
            {author} guardó la versión {version.number} de «{latest.name}» el{' '}
            {serverDateTime(version.created_at, timezone)}, mientras editabas.
          </p>
          <p>Tu borrador no se perdió y nada se sobrescribió. Elige qué hacer con tus cambios.</p>
        </>
      }
      choices={[
        { label: 'Ver la versión nueva', onSelect: () => setViewing(true) },
        {
          label: 'Guardar mi borrador como versión más reciente',
          onSelect: onSaveMine,
        },
        { label: 'Descartar mi borrador', onSelect: onDiscardMine, tone: 'danger' },
        { label: 'Seguir editando sin guardar', onSelect: onKeepEditing },
      ]}
    />
  );
}
