import { useState } from 'react';
import type { FormEvent } from 'react';
import { ModalPanel } from '../ModalPanel';

// Guardar crea una versión nueva e inmutable de la plantilla; la nota es opcional (≤ 200
// caracteres) y aparece en el historial.
export function SaveDialog({
  nextVersion,
  active,
  onSave,
  onClose,
}: {
  nextVersion: number;
  active: boolean;
  onSave: (note: string) => Promise<void>;
  onClose: () => void;
}) {
  const [note, setNote] = useState('');
  const [busy, setBusy] = useState(false);
  const formID = 'documents-save-form';

  async function submit(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    try {
      await onSave(note.trim());
    } finally {
      setBusy(false);
    }
  }

  return (
    <ModalPanel
      title={`Guardar la versión ${nextVersion}`}
      onClose={onClose}
      actions={
        <>
          <button
            type="button"
            className="secondary-button dialog-button"
            disabled={busy}
            onClick={onClose}
          >
            Seguir editando
          </button>
          <button
            type="submit"
            form={formID}
            className="primary-button dialog-button"
            disabled={busy}
          >
            {busy ? 'Guardando…' : 'Guardar versión'}
          </button>
        </>
      }
    >
      <form id={formID} className="form-stack" onSubmit={(event) => void submit(event)}>
        <p className="muted text-sm">
          {active
            ? 'La plantilla está activa: los documentos nuevos usarán esta versión. Los ya emitidos conservan su formato al reimprimirse.'
            : 'La plantilla no está activa: guardar no cambia ningún documento hasta que la actives.'}
        </p>
        <label className="field">
          <span>Nota de la versión (opcional)</span>
          <textarea
            aria-label="Nota de la versión (opcional)"
            value={note}
            maxLength={200}
            rows={3}
            placeholder="Por ejemplo: agregamos el horario en el pie"
            onChange={(event) => setNote(event.target.value)}
          />
          <small className="muted">{note.length} de 200 caracteres</small>
        </label>
      </form>
    </ModalPanel>
  );
}
