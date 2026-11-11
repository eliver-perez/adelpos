import { useRef, useState } from 'react';
import { APIError } from '../../../services/api';
import { documentTemplate, saveDocumentTemplate } from '../../../services/documents';
import type { TemplateDetail, TemplateLayout, VersionDetail } from '../../../services/documents';
import { errorMessage, notifyError, notifySuccess } from '../../../services/notify';
import type { RunOperation } from './useOperations';
import type { DesignerDispatch } from './useDesignerDraft';

// Guardar una versión nueva y resolver el conflicto de revisión. Go compara la revisión: si otra
// persona guardó antes responde 409 REVISION_CONFLICT y nada se sobrescribe; el borrador queda
// intacto y la persona elige explícitamente qué hacer (ver la versión nueva, guardar su borrador
// como la versión más reciente o descartarlo). Un 422 TEMPLATE_INVALID señala el bloque.

export interface SaveProblem {
  message: string;
  blockID: string;
}

// Restaurar una versión anterior = guardar su layout como versión nueva con la revisión vigente.
export async function restoreVersion(
  run: RunOperation,
  template: TemplateDetail | { id: string; revision: number },
  version: VersionDetail,
) {
  const note = `Restaurada desde la versión ${version.number}`;
  return run(
    `restore-version:${template.id}`,
    { revision: template.revision, version: version.id },
    (operationID) =>
      saveDocumentTemplate(operationID, template.id, template.revision, version.layout, note),
  );
}

export function useDesignerSave({
  base,
  draft,
  run,
  dispatch,
  onSaved,
}: {
  base: TemplateDetail;
  draft: TemplateLayout;
  run: RunOperation;
  dispatch: DesignerDispatch;
  onSaved: (template: TemplateDetail) => void;
}) {
  const [problem, setProblem] = useState<SaveProblem | null>(null);
  // Versión que guardó otra persona; mientras exista se muestra el diálogo del conflicto.
  const [conflict, setConflict] = useState<TemplateDetail | null>(null);
  const [pendingNote, setPendingNote] = useState('');
  // Mientras se guarda, el aviso de cambios de otra persona no se evalúa (el evento de la propia
  // versión puede llegar antes que la respuesta).
  const saving = useRef(false);

  function finish(saved: TemplateDetail) {
    dispatch({ type: 'saved', template: saved });
    setProblem(null);
    setConflict(null);
    notifySuccess(`Versión ${saved.current_version.number} guardada.`);
    onSaved(saved);
  }

  async function handleFailure(error: unknown, note: string) {
    if (error instanceof APIError && error.code === 'REVISION_CONFLICT') {
      setPendingNote(note);
      try {
        setConflict(await documentTemplate(base.id));
      } catch (reload) {
        notifyError(reload, 'Otra persona guardó la plantilla y no se pudo consultar su versión.');
      }
      return;
    }
    if (error instanceof APIError && error.code === 'TEMPLATE_INVALID') {
      const details = error.details as { block_id?: unknown } | undefined;
      const blockID = typeof details?.block_id === 'string' ? details.block_id : '';
      setProblem({ message: errorMessage(error, 'La plantilla no es válida.'), blockID });
      if (blockID) {
        dispatch({ type: 'select', id: blockID });
      }
    }
    notifyError(error, 'No se pudo guardar la plantilla.');
  }

  async function save(note: string, revision = base.revision) {
    saving.current = true;
    try {
      const saved = await run(`save:${base.id}`, { revision, layout: draft, note }, (operationID) =>
        saveDocumentTemplate(operationID, base.id, revision, draft, note),
      );
      finish(saved);
      return true;
    } catch (error) {
      await handleFailure(error, note);
      return false;
    } finally {
      saving.current = false;
    }
  }

  // «Guardar mi borrador como versión más reciente»: se guarda sobre la revisión que Go reportó.
  async function saveOverLatest() {
    if (!conflict) {
      return;
    }
    const latest = conflict;
    setConflict(null);
    await save(pendingNote, latest.revision);
  }

  function discardDraft() {
    if (!conflict) {
      return;
    }
    dispatch({ type: 'reset', template: conflict });
    setConflict(null);
    setProblem(null);
    onSaved(conflict);
  }

  return {
    problem,
    conflict,
    saving,
    save,
    saveOverLatest,
    discardDraft,
    setConflict,
    clearProblem: () => setProblem(null),
  };
}
