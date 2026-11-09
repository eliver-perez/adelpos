import { useEffect, useRef, useState } from 'react';
import type { FormEvent, ReactNode } from 'react';
import { notifyError, notifySuccess } from '../services/notify';

// Estado en pantalla, en su lugar: errores al cargar un panel o avisos que describen la situación
// vigente. Los resultados de una acción (éxito o error al guardar, enviar, cobrar…) no usan este
// componente sino services/notify (sistema único de mensajes, etapa 7.1).
export function Notice({ children }: { children?: ReactNode }) {
  if (!children) {
    return null;
  }
  return (
    <div className="notice" role="status">
      {children}
    </div>
  );
}

export function Field({
  label,
  children,
  hint,
}: {
  label: string;
  children: ReactNode;
  hint?: string;
}) {
  return (
    <label className="field">
      <span>{label}</span>
      {children}
      {hint && <small className="muted">{hint}</small>}
    </label>
  );
}

export function PageTitle({
  title,
  description,
  children,
}: {
  title: string;
  description: string;
  children?: ReactNode;
}) {
  return (
    <div className="page-title">
      <div>
        <h1>{title}</h1>
        <p className="muted">{description}</p>
      </div>
      {children}
    </div>
  );
}

type SubmitIntent = 'save' | 'new';

export function Form({
  onSave,
  children,
  label = 'Guardar cambios',
  disabled = false,
  onSaveAndNew,
  saveAndNewLabel = 'Guardar y crear otra',
  successMessage = 'Cambios guardados en el servidor.',
}: {
  onSave: () => Promise<void>;
  children: ReactNode;
  label?: string;
  disabled?: boolean;
  // Alta continua: guarda y deja el formulario listo para el siguiente registro. Si se indica,
  // aparece un segundo botón de envío; ambos pasan por la misma validación nativa del formulario.
  // Puede devolver el texto del aviso de confirmación (por ejemplo, con el nombre guardado).
  onSaveAndNew?: () => Promise<string | void>;
  saveAndNewLabel?: string;
  // Confirmación que se muestra como aviso al guardar con el botón principal.
  successMessage?: string;
}) {
  const [busy, setBusy] = useState(false);
  const intent = useRef<SubmitIntent>('save');
  const fields = useRef<HTMLFieldSetElement>(null);
  const [focusRequest, setFocusRequest] = useState(0);
  const locked = busy || disabled;

  // Después de «Guardar y crear otra» el cursor vuelve al primer campo (el nombre) en cuanto el
  // formulario deja de estar bloqueado por la solicitud.
  useEffect(() => {
    if (focusRequest === 0 || locked || !fields.current) {
      return;
    }
    const first = fields.current.querySelector<HTMLElement>(
      'input:not([type="hidden"]):not([disabled]), select:not([disabled]), textarea:not([disabled])',
    );
    first?.focus();
  }, [focusRequest, locked]);

  async function submit(event: FormEvent) {
    event.preventDefault();
    const form = event.currentTarget;
    // El botón pulsado fija la intención en su onClick, que ocurre antes del envío; Intro envía
    // con el botón principal.
    const createAnother = intent.current === 'new' && onSaveAndNew !== undefined;
    intent.current = 'save';
    setBusy(true);
    try {
      if (createAnother) {
        const message = await onSaveAndNew();
        notifySuccess(message || 'Registro guardado en el servidor. Captura el siguiente.');
        setFocusRequest((current) => current + 1);
      } else {
        await onSave();
        notifySuccess(successMessage);
      }
    } catch (error) {
      // El error de Go permanece como aviso hasta que se cierra. Además se revelan las secciones
      // cerradas del formulario por si el problema está en uno de sus campos.
      notifyError(error, 'No se pudo guardar.');
      form.dispatchEvent(new CustomEvent('form-validation-error', { bubbles: true }));
    } finally {
      setBusy(false);
    }
  }

  return (
    <form onSubmit={(event) => void submit(event)} className="form-stack">
      <fieldset ref={fields} disabled={locked} className="form-stack">
        {children}
      </fieldset>
      {/* La barra queda fija al pie del área con desplazamiento: la acción principal siempre está
          visible sin subir hasta el inicio del formulario. */}
      <div className="form-actions">
        <button
          type="submit"
          className="primary-button"
          disabled={locked}
          onClick={() => {
            intent.current = 'save';
          }}
        >
          {busy ? 'Guardando…' : label}
        </button>
        {onSaveAndNew && (
          <button
            type="submit"
            className="secondary-button"
            disabled={locked}
            onClick={() => {
              intent.current = 'new';
            }}
          >
            {saveAndNewLabel}
          </button>
        )}
      </div>
    </form>
  );
}

export function dateTime(value: string) {
  return new Date(value).toLocaleString('es-MX');
}
