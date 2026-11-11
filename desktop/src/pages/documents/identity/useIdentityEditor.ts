import { useCallback, useEffect, useId, useMemo, useRef, useState } from 'react';
import { APIError } from '../../../services/api';
import {
  identityChangeEvent,
  identityFields,
  previewIdentityLogo,
  readDocumentIdentity,
  readFileAsBase64,
  removeIdentityLogo,
  sameFields,
  samePresentation,
  saveDocumentIdentity,
  uploadIdentityLogo,
} from '../../../services/documentIdentity';
import type {
  DocumentIdentity,
  IdentityFields,
  IdentityPresentation,
  LogoPreview,
} from '../../../services/documentIdentity';
import { errorMessage, notifyError, notifySuccess } from '../../../services/notify';

// Estado del panel «Identidad del establecimiento».
//
// - `identity` es la última versión del servidor; `draft` lo que la persona escribe, con la versión
//   sobre la que lo empezó (baseRevision). Mientras no hay cambios, el panel muestra el servidor.
// - Guardar envía la revisión de origen del borrador: si otra persona guardó antes, Go responde
//   409 y el panel conserva lo escrito y muestra la versión guardada con dos opciones explícitas.
// - Un resultado incierto (sin respuesta del servidor) conserva el mismo operation_id para
//   reintentar: Go devuelve la respuesta guardada si ya se aplicó, sin repetir efectos.
// - La vista previa de otra presentación usa antirrebote y una sola petición en vuelo.

export interface IdentityDraft {
  base: IdentityFields;
  baseRevision: number;
  fields: IdentityFields;
}

export type IdentityAction = 'save' | 'upload' | 'remove';

export interface PendingOperation {
  action: IdentityAction;
  operationID: string;
  run: (operationID: string) => Promise<DocumentIdentity>;
}

export interface PreviewState {
  key: string;
  result: LogoPreview | null;
  error: string;
}

const previewDelay = 300;
const successMessages: Record<IdentityAction, string> = {
  save: 'Identidad del establecimiento guardada.',
  upload: 'Logotipo cargado. Revisa cómo saldrá en papel de 58 y 80 mm.',
  remove: 'Logotipo retirado. Los documentos nuevos ya no lo imprimen.',
};

function presentationKey(presentation: IdentityPresentation) {
  return [
    presentation.width_percent,
    presentation.align,
    presentation.method,
    presentation.threshold,
  ].join('|');
}

function isConnectionError(error: unknown) {
  return error instanceof APIError && error.code === 'CONNECTION_ERROR';
}

function isRevisionConflict(error: unknown) {
  return error instanceof APIError && error.code === 'REVISION_CONFLICT';
}

export function useIdentityEditor(canManage: boolean) {
  const source = useId();
  const [identity, setIdentity] = useState<DocumentIdentity | null>(null);
  const [loadError, setLoadError] = useState('');
  const [loading, setLoading] = useState(true);
  const [draft, setDraft] = useState<IdentityDraft | null>(null);
  const [busy, setBusy] = useState<IdentityAction | null>(null);
  const [conflict, setConflict] = useState<DocumentIdentity | null>(null);
  const [pending, setPending] = useState<PendingOperation | null>(null);
  const [preview, setPreview] = useState<PreviewState | null>(null);
  const generation = useRef(0);
  const mounted = useRef(true);
  const draftRef = useRef<IdentityDraft | null>(null);
  draftRef.current = draft;

  const dirty = draft !== null && !sameFields(draft.fields, draft.base);
  const fields = useMemo(() => {
    if (draft) {
      return draft.fields;
    }
    return identity ? identityFields(identity) : null;
  }, [draft, identity]);

  // Consulta la identidad. Sin cambios en edición, el panel pasa a mostrar la versión nueva; con
  // cambios y una revisión ajena posterior, la ofrece como conflicto sin tocar lo escrito.
  const reload = useCallback(async () => {
    const request = ++generation.current;
    setLoadError('');
    try {
      const result = await readDocumentIdentity();
      if (!mounted.current || request !== generation.current) {
        return result;
      }
      setIdentity(result);
      const current = draftRef.current;
      if (current && !sameFields(current.fields, current.base)) {
        if (result.revision > current.baseRevision) {
          setConflict(result);
        }
      } else {
        setDraft(null);
      }
      return result;
    } catch (error) {
      if (mounted.current && request === generation.current) {
        setLoadError(errorMessage(error, 'No se pudo cargar la identidad del establecimiento.'));
      }
      return null;
    } finally {
      if (mounted.current && request === generation.current) {
        setLoading(false);
      }
    }
  }, []);

  useEffect(() => {
    mounted.current = true;
    void reload();
    let timer: number | undefined;
    const invalidate = (event: Event) => {
      const detail = (event as CustomEvent<{ source?: string }>).detail;
      if (detail?.source === source) {
        return;
      }
      window.clearTimeout(timer);
      timer = window.setTimeout(() => void reload(), 75);
    };
    window.addEventListener(identityChangeEvent, invalidate);
    return () => {
      mounted.current = false;
      generation.current++;
      window.clearTimeout(timer);
      window.removeEventListener(identityChangeEvent, invalidate);
    };
  }, [reload, source]);

  const change = useCallback(
    (patch: Partial<IdentityFields>) => {
      if (!canManage || !identity) {
        return;
      }
      setDraft((current) => {
        const base = current ?? {
          base: identityFields(identity),
          baseRevision: identity.revision,
          fields: identityFields(identity),
        };
        return { ...base, fields: { ...base.fields, ...patch } };
      });
    },
    [canManage, identity],
  );

  const changePresentation = useCallback(
    (patch: Partial<IdentityPresentation>) => {
      if (!fields) {
        return;
      }
      change({ presentation: { ...fields.presentation, ...patch } });
    },
    [change, fields],
  );

  // Ejecuta una escritura con su operation_id. El éxito recarga la identidad (con sus vistas
  // previas) antes de liberar los controles y avisa a las demás pantallas.
  const execute = useCallback(
    async (
      action: IdentityAction,
      run: (operationID: string) => Promise<DocumentIdentity>,
      operationID: string,
    ) => {
      setBusy(action);
      try {
        const result = await run(operationID);
        setPending(null);
        if (action === 'save') {
          setDraft(null);
          setConflict(null);
        } else {
          // Cargar o retirar el logotipo no cambia los textos ni la presentación en edición: el
          // borrador pasa a partir de la revisión nueva que dejó esta misma terminal.
          setDraft((current) => {
            if (!current || current.baseRevision !== result.revision - 1) {
              return current;
            }
            return { ...current, baseRevision: result.revision };
          });
        }
        await reload();
        window.dispatchEvent(new CustomEvent(identityChangeEvent, { detail: { source } }));
        notifySuccess(successMessages[action]);
      } catch (error) {
        if (isConnectionError(error)) {
          setPending({ action, operationID, run });
          return;
        }
        setPending(null);
        if (isRevisionConflict(error)) {
          const latest = await reload();
          if (action === 'save' && latest) {
            setConflict(latest);
            return;
          }
          notifyError(
            'Otra persona cambió la identidad del establecimiento. Se cargó la versión más reciente: vuelve a intentarlo.',
          );
          return;
        }
        notifyError(error, 'No se pudo completar la operación.');
      } finally {
        if (mounted.current) {
          setBusy(null);
        }
      }
    },
    [reload, source],
  );

  const save = useCallback(() => {
    const current = draftRef.current;
    if (!canManage || !current || sameFields(current.fields, current.base)) {
      return Promise.resolve();
    }
    const fieldsToSave = current.fields;
    const revision = current.baseRevision;
    return execute(
      'save',
      (operationID) => saveDocumentIdentity(fieldsToSave, revision, operationID),
      crypto.randomUUID(),
    );
  }, [canManage, execute]);

  const upload = useCallback(
    async (file: File) => {
      if (!canManage || !identity) {
        return;
      }
      if (file.size > identity.limits.logo_max_bytes) {
        notifyError('El logotipo debe ocupar como máximo 1 MiB.');
        return;
      }
      let content = '';
      try {
        content = await readFileAsBase64(file);
      } catch (error) {
        notifyError(error, 'No se pudo leer el archivo del logotipo.');
        return;
      }
      const revision = identity.revision;
      await execute(
        'upload',
        (operationID) => uploadIdentityLogo(content, revision, operationID),
        crypto.randomUUID(),
      );
    },
    [canManage, execute, identity],
  );

  const remove = useCallback(async () => {
    if (!canManage || !identity?.logo) {
      return;
    }
    const revision = identity.revision;
    await execute(
      'remove',
      (operationID) => removeIdentityLogo(revision, operationID),
      crypto.randomUUID(),
    );
  }, [canManage, execute, identity]);

  // Reintenta una operación incierta con el mismo operation_id.
  const retryPending = useCallback(() => {
    if (!pending) {
      return Promise.resolve();
    }
    return execute(pending.action, pending.run, pending.operationID);
  }, [execute, pending]);

  const dismissPending = useCallback(() => {
    setPending(null);
    void reload();
  }, [reload]);

  // Conflicto: conservar lo escrito sobre la versión nueva y guardarlo, o descartarlo.
  const keepMine = useCallback(() => {
    if (!conflict) {
      return Promise.resolve();
    }
    const latest = conflict;
    setConflict(null);
    setDraft((current) => {
      if (!current) {
        return current;
      }
      return { ...current, base: identityFields(latest), baseRevision: latest.revision };
    });
    const current = draftRef.current;
    if (!current) {
      return Promise.resolve();
    }
    const fieldsToSave = current.fields;
    return execute(
      'save',
      (operationID) => saveDocumentIdentity(fieldsToSave, latest.revision, operationID),
      crypto.randomUUID(),
    );
  }, [conflict, execute]);

  const discardMine = useCallback(() => {
    if (conflict) {
      setIdentity(conflict);
    }
    setConflict(null);
    setDraft(null);
  }, [conflict]);

  const discardDraft = useCallback(() => {
    setDraft(null);
    setConflict(null);
  }, []);

  // Vista previa de una presentación sin guardar: solo con logotipo y si difiere de la guardada.
  const draftPresentation = fields?.presentation ?? null;
  const needsPreview =
    canManage &&
    identity !== null &&
    identity.logo !== null &&
    draftPresentation !== null &&
    !samePresentation(draftPresentation, identity.presentation);
  const currentKey = draftPresentation ? presentationKey(draftPresentation) : '';
  const wanted = useRef<IdentityPresentation | null>(null);
  const inFlight = useRef(false);

  const requestPreview = useCallback(() => {
    const target = wanted.current;
    if (inFlight.current || !target) {
      return;
    }
    inFlight.current = true;
    const key = presentationKey(target);
    previewIdentityLogo(target)
      .then((result) => {
        if (mounted.current) {
          setPreview({ key, result, error: '' });
        }
      })
      .catch((error: unknown) => {
        if (mounted.current) {
          setPreview({
            key,
            result: null,
            error: errorMessage(error, 'No se pudo preparar la vista previa.'),
          });
        }
      })
      .finally(() => {
        inFlight.current = false;
        // La última presentación pedida gana: si cambió mientras tanto, se pide de nuevo.
        if (wanted.current && presentationKey(wanted.current) !== key) {
          requestPreview();
        }
      });
  }, []);

  useEffect(() => {
    if (!needsPreview || !draftPresentation) {
      wanted.current = null;
      return;
    }
    wanted.current = draftPresentation;
    const timer = window.setTimeout(requestPreview, previewDelay);
    return () => window.clearTimeout(timer);
    // La clave resume la presentación: un objeto nuevo con los mismos valores no vuelve a pedirla.
  }, [needsPreview, currentKey, requestPreview]);

  const draftPreview = needsPreview && preview?.key === currentKey ? preview : null;

  return {
    identity,
    fields,
    loading,
    loadError,
    dirty,
    busy,
    conflict,
    pending,
    needsPreview,
    draftPreview,
    reload,
    change,
    changePresentation,
    save,
    upload,
    remove,
    retryPending,
    dismissPending,
    keepMine,
    discardMine,
    discardDraft,
  };
}
