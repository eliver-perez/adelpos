import { useState } from 'react';
import type { FormEvent } from 'react';
import { errorMessage } from '../../../services/notify';
import { ModalPanel } from '../ModalPanel';

// Nombre de una plantilla nueva, duplicada o renombrada. Go valida el nombre (1–80 caracteres,
// único por tipo sin distinguir mayúsculas); su rechazo se muestra aquí mismo, sin cerrar el
// diálogo ni perder lo escrito.
export function NameDialog({
  title,
  description,
  initialName,
  confirmLabel,
  onSubmit,
  onClose,
}: {
  title: string;
  description: string;
  initialName: string;
  confirmLabel: string;
  onSubmit: (name: string) => Promise<void>;
  onClose: () => void;
}) {
  const [name, setName] = useState(initialName);
  const [busy, setBusy] = useState(false);
  const [problem, setProblem] = useState('');
  const formID = 'documents-name-form';

  async function submit(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    setProblem('');
    try {
      await onSubmit(name.trim());
    } catch (error) {
      setProblem(errorMessage(error, 'No se pudo guardar el nombre.'));
      setBusy(false);
    }
  }

  return (
    <ModalPanel
      title={title}
      onClose={onClose}
      actions={
        <>
          <button type="button" className="secondary-button dialog-button" onClick={onClose}>
            Cancelar
          </button>
          <button
            type="submit"
            form={formID}
            className="primary-button dialog-button"
            disabled={busy || name.trim() === ''}
          >
            {busy ? 'Guardando…' : confirmLabel}
          </button>
        </>
      }
    >
      <form id={formID} className="form-stack" onSubmit={(event) => void submit(event)}>
        <p className="muted text-sm">{description}</p>
        <label className="field">
          <span>Nombre de la plantilla</span>
          <input
            type="text"
            value={name}
            maxLength={80}
            required
            onChange={(event) => setName(event.target.value)}
          />
        </label>
        {problem && (
          <p className="checkout-inline-note" role="alert">
            {problem}
          </p>
        )}
      </form>
    </ModalPanel>
  );
}
