import { useState } from 'react';
import type { ReactNode } from 'react';
import { useConfirmDialog } from '../../../components/ConfirmDialog';
import {
  activateDocumentTemplate,
  createDocumentTemplate,
  renameDocumentTemplate,
  restoreDefaultTemplate,
} from '../../../services/documents';
import type { DocumentKind, TemplateDetail, TemplateSummary } from '../../../services/documents';
import { notifyError, notifySuccess } from '../../../services/notify';
import { useOperations } from '../designer/useOperations';
import { NameDialog } from './NameDialog';

// Acciones sobre plantillas que comparten la lista por tipo y el editor: duplicar, crear,
// renombrar, activar y restaurar la predeterminada. Activar y restaurar se confirman porque
// cambian lo que imprimen los documentos nuevos; Go valida permisos, revisión y nombre.

interface NameRequest {
  title: string;
  description: string;
  initialName: string;
  confirmLabel: string;
  submit: (name: string) => Promise<void>;
}

export function useTemplateActions({
  kindName,
  onChanged,
  onCreated,
}: {
  kindName: (kind: DocumentKind) => string;
  // Go confirmó un cambio: la lista vuelve a consultarse y el editor actualiza su base.
  onChanged: (template: TemplateDetail) => void;
  // Plantilla nueva (creada o duplicada): se abre en el editor.
  onCreated: (template: TemplateDetail) => void;
}) {
  const run = useOperations();
  const { confirm, dialog } = useConfirmDialog();
  const [nameRequest, setNameRequest] = useState<NameRequest | null>(null);

  function duplicate(template: TemplateSummary) {
    setNameRequest({
      title: 'Duplicar plantilla',
      description: `Se crea una plantilla nueva de ${kindName(template.kind).toLowerCase()} con la versión ${template.current_version.number} de «${template.name}». Queda inactiva hasta que la actives.`,
      initialName: `Copia de ${template.name}`.slice(0, 80),
      confirmLabel: 'Duplicar',
      submit: async (name) => {
        const created = await run(
          `duplicate:${template.id}`,
          { name, source: template.current_version.id },
          (operationID) =>
            createDocumentTemplate(operationID, template.kind, name, template.current_version.id),
        );
        setNameRequest(null);
        notifySuccess(`Plantilla «${created.name}» creada.`);
        onCreated(created);
      },
    });
  }

  function create(kind: DocumentKind) {
    setNameRequest({
      title: 'Nueva plantilla',
      description: `La plantilla nueva de ${kindName(kind).toLowerCase()} parte de la predeterminada del sistema. Queda inactiva hasta que la actives.`,
      initialName: '',
      confirmLabel: 'Crear plantilla',
      submit: async (name) => {
        const created = await run(`create:${kind}`, { name }, (operationID) =>
          createDocumentTemplate(operationID, kind, name),
        );
        setNameRequest(null);
        notifySuccess(`Plantilla «${created.name}» creada.`);
        onCreated(created);
      },
    });
  }

  function rename(template: TemplateSummary) {
    setNameRequest({
      title: 'Renombrar plantilla',
      description: 'El nombre no cambia el contenido ni las versiones de la plantilla.',
      initialName: template.name,
      confirmLabel: 'Renombrar',
      submit: async (name) => {
        const renamed = await run(
          `rename:${template.id}`,
          { name, revision: template.revision },
          (operationID) =>
            renameDocumentTemplate(operationID, template.id, template.revision, name),
        );
        setNameRequest(null);
        notifySuccess(`Plantilla renombrada: «${renamed.name}».`);
        onChanged(renamed);
      },
    });
  }

  async function activate(template: TemplateSummary, unsavedDraft = false) {
    const confirmed = await confirm({
      title: 'Activar plantilla',
      message: (
        <>
          <p>
            Los documentos nuevos de {kindName(template.kind).toLowerCase()} se imprimirán con «
            {template.name}» (versión {template.current_version.number}).
          </p>
          <p>Los documentos ya emitidos conservan su formato al reimprimirse.</p>
          {unsavedDraft && (
            <p>Tus cambios sin guardar no se incluyen: se activa la última versión guardada.</p>
          )}
        </>
      ),
      confirmLabel: 'Activar',
    });
    if (!confirmed) {
      return;
    }
    try {
      const activated = await run(
        `activate:${template.id}`,
        { revision: template.revision },
        (operationID) => activateDocumentTemplate(operationID, template.id, template.revision),
      );
      notifySuccess(`«${activated.name}» es ahora la plantilla activa.`);
      onChanged(activated);
    } catch (error) {
      notifyError(error, 'No se pudo activar la plantilla.');
    }
  }

  async function restoreDefault(kind: DocumentKind) {
    const confirmed = await confirm({
      title: 'Restaurar la plantilla predeterminada',
      message: (
        <>
          <p>
            Se activa la plantilla predeterminada del sistema para {kindName(kind).toLowerCase()}:
            los documentos nuevos vuelven al formato original.
          </p>
          <p>Las demás plantillas y sus versiones se conservan; puedes volver a activarlas.</p>
        </>
      ),
      confirmLabel: 'Restaurar predeterminada',
    });
    if (!confirmed) {
      return;
    }
    try {
      const restored = await run(`restore:${kind}`, { kind }, (operationID) =>
        restoreDefaultTemplate(operationID, kind),
      );
      notifySuccess('Plantilla predeterminada activa.');
      onChanged(restored);
    } catch (error) {
      notifyError(error, 'No se pudo restaurar la plantilla predeterminada.');
    }
  }

  const dialogs: ReactNode = (
    <>
      {dialog}
      {nameRequest && (
        <NameDialog
          title={nameRequest.title}
          description={nameRequest.description}
          initialName={nameRequest.initialName}
          confirmLabel={nameRequest.confirmLabel}
          onSubmit={nameRequest.submit}
          onClose={() => setNameRequest(null)}
        />
      )}
    </>
  );

  return { duplicate, create, rename, activate, restoreDefault, dialogs };
}
