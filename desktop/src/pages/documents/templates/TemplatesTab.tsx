import { useState } from 'react';
import { Notice } from '../../../components/Forms';
import { useResource } from '../../../hooks/useResource';
import { documentTemplate } from '../../../services/documents';
import type {
  DocumentCatalog,
  DocumentKind,
  PrinterCompatibility,
  TemplateDetail,
  TemplateSummary,
  VersionDetail,
} from '../../../services/documents';
import { notifyError, notifySuccess } from '../../../services/notify';
import { familyGroups } from '../designer/blockLabels';
import { Designer } from '../designer/Designer';
import { HistoryDialog } from '../designer/HistoryDialog';
import { restoreVersion } from '../designer/useDesignerSave';
import { useOperations } from '../designer/useOperations';
import { KindTemplates } from './KindTemplates';
import { useTemplateActions } from './useTemplateActions';

// Pestaña «Plantillas»: los diez tipos agrupados (Venta, Cocina, Caja) con sus plantillas y, al
// abrir una, el editor visual. Mientras se edita, el editor ocupa toda la pestaña.
export function TemplatesTab({
  catalog,
  permissions,
  timezone,
  onEditing,
}: {
  catalog: DocumentCatalog;
  permissions: string[];
  timezone: string;
  // Avisa a la página si el editor está abierto (oculta el título y las pestañas).
  onEditing: (editing: boolean) => void;
}) {
  const [kind, setKind] = useState<DocumentKind>('receipt');
  const [editing, setEditing] = useState<TemplateDetail | null>(null);
  const [history, setHistory] = useState<TemplateSummary | null>(null);
  const [reloadKey, setReloadKey] = useState(0);
  const [opening, setOpening] = useState(false);
  const run = useOperations();
  const printers = useResource<PrinterCompatibility[]>('documents_printers');
  const catalogKind = catalog.kinds.find((entry) => entry.kind === kind);
  const kindName = (value: DocumentKind) =>
    catalog.kinds.find((entry) => entry.kind === value)?.name ?? 'Documento';

  function changed() {
    setReloadKey((value) => value + 1);
  }

  function edit(template: TemplateDetail) {
    setKind(template.kind);
    setEditing(template);
    onEditing(true);
  }

  const actions = useTemplateActions({
    kindName,
    onChanged: (template) => {
      changed();
      setEditing((current) => (current && current.id === template.id ? template : current));
    },
    onCreated: (template) => {
      changed();
      edit(template);
    },
  });

  async function open(template: TemplateSummary) {
    setOpening(true);
    try {
      edit(await documentTemplate(template.id));
    } catch (error) {
      notifyError(error, 'No se pudo abrir la plantilla.');
    } finally {
      setOpening(false);
    }
  }

  async function restoreFromList(template: TemplateSummary, version: VersionDetail) {
    try {
      const saved = await restoreVersion(run, template, version);
      notifySuccess(
        `Versión ${version.number} restaurada como versión ${saved.current_version.number}.`,
      );
      setHistory(null);
      changed();
    } catch (error) {
      notifyError(error, 'No se pudo restaurar la versión.');
    }
  }

  const editingKind = editing
    ? catalog.kinds.find((entry) => entry.kind === editing.kind)
    : undefined;
  if (editing && editingKind) {
    return (
      <>
        <Designer
          key={editing.id}
          catalog={catalog}
          kind={editingKind}
          template={editing}
          permissions={permissions}
          timezone={timezone}
          printers={printers.data}
          actions={{
            activate: actions.activate,
            duplicate: actions.duplicate,
          }}
          onClose={() => {
            setEditing(null);
            onEditing(false);
            changed();
          }}
          onTemplateChanged={changed}
        />
        {actions.dialogs}
      </>
    );
  }

  return (
    <div className="documents-browser">
      <nav className="documents-kinds panel" aria-label="Tipos de documento">
        {familyGroups.map((group) => (
          <div key={group.family} className="documents-kind-group">
            <h2>{group.label}</h2>
            {group.kinds.map((value) => (
              <button
                type="button"
                key={value}
                className={`documents-kind-button ${value === kind ? 'nav-selected' : ''}`}
                aria-current={value === kind ? 'true' : undefined}
                onClick={() => setKind(value)}
              >
                {kindName(value)}
              </button>
            ))}
          </div>
        ))}
      </nav>
      <div className="documents-kind-detail">
        {opening && (
          <p className="muted" role="status">
            Abriendo la plantilla…
          </p>
        )}
        {catalogKind ? (
          <KindTemplates
            key={kind}
            kind={catalogKind}
            permissions={permissions}
            timezone={timezone}
            reloadKey={reloadKey}
            onOpen={(template) => void open(template)}
            onDuplicate={actions.duplicate}
            onRename={actions.rename}
            onActivate={(template) => void actions.activate(template)}
            onHistory={setHistory}
            onCreate={() => actions.create(kind)}
            onRestoreDefault={() => void actions.restoreDefault(kind)}
          />
        ) : (
          <Notice>Este tipo de documento no está en el catálogo del servidor.</Notice>
        )}
      </div>
      {history && (
        <HistoryDialog
          template={history}
          timezone={timezone}
          canRestore={!history.system && permissions.includes('documents.templates.manage')}
          restoreBlocked=""
          onRestore={(version) => restoreFromList(history, version)}
          onClose={() => setHistory(null)}
        />
      )}
      {actions.dialogs}
    </div>
  );
}
