import { useEffect, useRef, useState } from 'react';
import type { FormEvent, ReactNode } from 'react';

export function Notice({ children, success = false }: { children?: ReactNode; success?: boolean }) {
  if (!children) {
    return null;
  }
  return (
    <div className={`notice ${success ? 'notice-success' : ''}`} role="status">
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
}: {
  onSave: () => Promise<void>;
  children: ReactNode;
  label?: string;
  disabled?: boolean;
  // Alta continua: guarda y deja el formulario listo para el siguiente registro. Si se indica,
  // aparece un segundo botón de envío; ambos pasan por la misma validación nativa del formulario.
  onSaveAndNew?: () => Promise<void>;
  saveAndNewLabel?: string;
}) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [saved, setSaved] = useState('');
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
    setError('');
    setSaved('');
    setBusy(true);
    try {
      if (createAnother) {
        await onSaveAndNew();
        setSaved('Registro guardado en el servidor. Captura el siguiente.');
        setFocusRequest((current) => current + 1);
      } else {
        await onSave();
        setSaved('Cambios guardados en el servidor.');
      }
    } catch (error) {
      setError(error instanceof Error ? error.message : 'No se pudo guardar.');
      // Si se cerró una sección durante la solicitud, el error debe quedar visible.
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
      <Notice>{error}</Notice>
      {saved && <Notice success>{saved}</Notice>}
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
