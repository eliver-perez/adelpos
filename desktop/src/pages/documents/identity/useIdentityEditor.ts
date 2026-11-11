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
//   reintentar: Go devuelve la respuesta guardada si ya se aplicó, sin repetir efectos. Mientras
//   la operación está en curso o pendiente, los textos y la presentación no se pueden cambiar
//   (`locked`): así el reintento envía exactamente lo que se ve y nada escrito se pierde.
// - Una escritura correcta aplica primero su propia respuesta (la identidad guardada, sin vistas
//   previas) y después recarga: si la recarga falla, la pantalla muestra lo guardado con su
//   revisión, avisa con `loadError` y la siguiente edición no provoca un falso conflicto.
// - La vista previa de otra presentación usa antirrebote y una sola petición en vuelo, y pertenece
//   al logotipo con el que se pidió: al reemplazarlo se vuelve a pedir.

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
  // Campos que envía un guardado (solo en «save»): tras el éxito, el borrador se borra solo si
  // coincide con ellos.
  sent?: IdentityFields;
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

// Una vista previa sin guardar depende del logotipo y de la presentación: la clave lleva ambos,
// de modo que reemplazar el logotipo invalida la vista previa anterior.
interface PreviewTarget {
  logoID: string;
  presentation: IdentityPresentation;
}

function previewKey(target: PreviewTarget) {
  return [
    target.logoID,
    target.presentation.width_percent,
    target.presentation.align,
    target.presentation.method,
    target.presentation.threshold,
  ].join('|');
}

// La respuesta de una escritura no trae vistas previas (las imágenes solo llegan en la lectura).
// Si el logotipo es el mismo, se conservan las del estado anterior para las conversiones que no
// cambiaron; si es otro, se muestran sin imagen hasta la recarga.
function withKnownPreviews(result: DocumentIdentity, previous: DocumentIdentity | null) {
  const logo = result.logo;
  const before = previous?.logo;
  if (!logo || !before || before.id !== logo.id) {
    return result;
  }
  const rasters = logo.rasters.map((raster) => {
    if (raster.data_uri || !raster.raster_id) {
      return raster;
    }
    const known = before.rasters.find((candidate) => candidate.raster_id === raster.raster_id);
    return known?.data_uri ? { ...raster, data_uri: known.data_uri } : raster;
  });
  return {
    ...result,
    logo: { ...logo, data_uri: logo.data_uri ?? before.data_uri, rasters },
  };
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
  const identityRef = useRef<DocumentIdentity | null>(null);
  identityRef.current = identity;
  // Con una escritura en curso o sin confirmar, lo escrito queda fijo hasta conocer su resultado.
  const locked = busy !== null || pending !== null;
  const lockedRef = useRef(false);
  lockedRef.current = locked;

  const dirty = draft !== null && !sameFields(draft.fields, draft.base);
  const fields = useMemo(() => {
    if (draft) {
      return draft.fields;
    }
    return identity ? identityFields(identity) : null;
  }, [draft, identity]);

  // Consulta la identidad. Sin cambios en edición, el panel pasa a mostrar la versión nueva; con
  // cambios y una revisión ajena posterior, la ofrece como conflicto sin tocar lo escrito. Si lo
  // escrito ya es exactamente lo guardado (por ejemplo, tras consultar una operación incierta que
  // sí se aplicó), el borrador se cierra sin conflicto.
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
      const editing = current !== null && !sameFields(current.fields, current.base);
      if (!editing || sameFields(current.fields, identityFields(result))) {
        setDraft(null);
      } else if (result.revision > current.baseRevision) {
        setConflict(result);
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
      // Durante una escritura en curso o pendiente los campos están en solo lectura; un cambio que
      // llegue de todos modos se ignora para que el reintento envíe lo que se ve.
      if (!canManage || !identity || lockedRef.current) {
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

  // Ejecuta una escritura con su operation_id. El éxito aplica primero la respuesta de la
  // escritura (revisión y datos guardados), después recarga la identidad con sus vistas previas
  // antes de liberar los controles y avisa a las demás pantallas. Si la recarga falla, se queda lo
  // que respondió la escritura y el panel muestra el error de carga.
  const execute = useCallback(
    async (operation: PendingOperation) => {
      const { action, run, operationID, sent } = operation;
      setBusy(action);
      try {
        const result = await run(operationID);
        setPending(null);
        // Una lectura que siguiera en vuelo traería una versión anterior: se descarta.
        generation.current++;
        setIdentity(withKnownPreviews(result, identityRef.current));
        setLoadError('');
        if (action === 'save') {
          setConflict(null);
          // El borrador se cierra solo si es exactamente lo que se guardó; si no, sigue abierto
          // sobre la revisión nueva.
          setDraft((current) => {
            if (!current || !sent || sameFields(current.fields, sent)) {
              return null;
            }
            return { ...current, base: identityFields(result), baseRevision: result.revision };
          });
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
          setPending(operation);
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
    if (!canManage || lockedRef.current || !current || sameFields(current.fields, current.base)) {
      return Promise.resolve();
    }
    const fieldsToSave = current.fields;
    const revision = current.baseRevision;
    return execute({
      action: 'save',
      run: (operationID) => saveDocumentIdentity(fieldsToSave, revision, operationID),
      operationID: crypto.randomUUID(),
      sent: fieldsToSave,
    });
  }, [canManage, execute]);

  const upload = useCallback(
    async (file: File) => {
      if (!canManage || !identity || lockedRef.current) {
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
      await execute({
        action: 'upload',
        run: (operationID) => uploadIdentityLogo(content, revision, operationID),
        operationID: crypto.randomUUID(),
      });
    },
    [canManage, execute, identity],
  );

  const remove = useCallback(async () => {
    if (!canManage || !identity?.logo || lockedRef.current) {
      return;
    }
    const revision = identity.revision;
    await execute({
      action: 'remove',
      run: (operationID) => removeIdentityLogo(revision, operationID),
      operationID: crypto.randomUUID(),
    });
  }, [canManage, execute, identity]);

  // Reintenta una operación incierta con el mismo operation_id y exactamente el mismo cuerpo.
  const retryPending = useCallback(() => {
    if (!pending) {
      return Promise.resolve();
    }
    return execute(pending);
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
    return execute({
      action: 'save',
      run: (operationID) => saveDocumentIdentity(fieldsToSave, latest.revision, operationID),
      operationID: crypto.randomUUID(),
      sent: fieldsToSave,
    });
  }, [conflict, execute]);

  const discardMine = useCallback(() => {
    if (conflict) {
      setIdentity(conflict);
    }
    setConflict(null);
    setDraft(null);
  }, [conflict]);

  const discardDraft = useCallback(() => {
    if (lockedRef.current) {
      return;
    }
    setDraft(null);
    setConflict(null);
  }, []);

  // Vista previa de una presentación sin guardar: solo con logotipo y si difiere de la guardada.
  const draftPresentation = fields?.presentation ?? null;
  const logoID = identity?.logo?.id ?? '';
  const needsPreview =
    canManage &&
    identity !== null &&
    identity.logo !== null &&
    draftPresentation !== null &&
    !samePresentation(draftPresentation, identity.presentation);
  const currentKey = draftPresentation
    ? previewKey({ logoID, presentation: draftPresentation })
    : '';
  const wanted = useRef<PreviewTarget | null>(null);
  const inFlight = useRef(false);

  const requestPreview = useCallback(() => {
    const target = wanted.current;
    if (inFlight.current || !target) {
      return;
    }
    inFlight.current = true;
    const key = previewKey(target);
    previewIdentityLogo(target.presentation)
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
        // La última presentación (o el último logotipo) pedida gana: si cambió mientras tanto, se
        // pide de nuevo.
        if (wanted.current && previewKey(wanted.current) !== key) {
          requestPreview();
        }
      });
  }, []);

  useEffect(() => {
    if (!needsPreview || !draftPresentation) {
      wanted.current = null;
      return;
    }
    wanted.current = { logoID, presentation: draftPresentation };
    const timer = window.setTimeout(requestPreview, previewDelay);
    return () => window.clearTimeout(timer);
    // La clave resume el logotipo y la presentación: un objeto nuevo con los mismos valores no
    // vuelve a pedirla, y otro logotipo sí.
  }, [needsPreview, currentKey, requestPreview]);

  const draftPreview = needsPreview && preview?.key === currentKey ? preview : null;

  return {
    identity,
    fields,
    loading,
    loadError,
    dirty,
    busy,
    locked,
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
