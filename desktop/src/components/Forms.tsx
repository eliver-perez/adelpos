import { useState } from 'react';
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

export function Form({
  onSave,
  children,
  label = 'Guardar cambios',
  disabled = false,
}: {
  onSave: () => Promise<void>;
  children: ReactNode;
  label?: string;
  disabled?: boolean;
}) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [saved, setSaved] = useState(false);

  async function submit(event: FormEvent) {
    event.preventDefault();
    setError('');
    setSaved(false);
    setBusy(true);
    try {
      await onSave();
      setSaved(true);
    } catch (error) {
      setError(error instanceof Error ? error.message : 'No se pudo guardar.');
    } finally {
      setBusy(false);
    }
  }

  return (
    <form onSubmit={(event) => void submit(event)} className="form-stack">
      <fieldset disabled={busy || disabled} className="form-stack">
        {children}
      </fieldset>
      <Notice>{error}</Notice>
      {saved && <Notice success>Cambios guardados en el servidor.</Notice>}
      <div>
        <button type="submit" className="primary-button" disabled={busy || disabled}>
          {busy ? 'Guardando…' : label}
        </button>
      </div>
    </form>
  );
}

export function dateTime(value: string) {
  return new Date(value).toLocaleString('es-MX');
}
