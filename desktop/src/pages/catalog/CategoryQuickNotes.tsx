import { useState } from 'react';
import { Accordion } from '../../components/Accordion';
import { Field } from '../../components/Forms';
import { PositionField } from '../../components/PositionField';
import { ToggleButton } from '../../components/ToggleButton';
import { api } from '../../services/api';
import type { Category, CategoryQuickNote } from '../../services/catalog';
import { notifyError, notifySuccess } from '../../services/notify';
import { countLabel } from '../../services/plural';
import { orderSiblings, positionPayload } from '../../services/positions';

// Las indicaciones de una partida se guardan unidas por comas: una indicación rápida no puede
// contenerlas. Go aplica la misma regla y el límite de 80 caracteres.
export function quickNoteProblem(text: string): string {
  const trimmed = text.trim();
  if (trimmed.length === 0) {
    return 'Escribe la indicación.';
  }
  if (trimmed.includes(',')) {
    return 'Sin comas: la captura separa las indicaciones con comas.';
  }
  if (Array.from(trimmed).length > 80) {
    return 'Hasta 80 caracteres.';
  }
  return '';
}

// Captura o edición de una indicación rápida: texto, estado y posición dentro de la categoría.
function QuickNoteEditor({
  category,
  note,
  notes,
  onSaved,
  onCancel,
}: {
  category: Category;
  note: CategoryQuickNote | null;
  notes: CategoryQuickNote[];
  onSaved: () => Promise<void>;
  onCancel?: () => void;
}) {
  const [text, setText] = useState(note?.text ?? '');
  const [active, setActive] = useState(note?.active ?? true);
  const [after, setAfter] = useState<string | undefined>(undefined);
  const [busy, setBusy] = useState(false);
  const problem = quickNoteProblem(text);
  const siblings = notes.map((item) => ({
    id: item.id,
    name: item.text,
    note: item.active ? undefined : 'inactiva',
  }));

  async function save() {
    if (problem || busy) {
      return;
    }
    const body: Record<string, unknown> = {
      category_id: category.id,
      text: text.trim(),
      active,
    };
    const position = positionPayload(notes, note?.id ?? '', after);
    if (position) {
      body.position = position;
    }
    setBusy(true);
    try {
      if (note) {
        await api('catalog_save_quick_note', {
          id: note.id,
          body: { ...body, revision: note.revision },
        });
        notifySuccess(`Indicación «${text.trim()}» guardada.`);
      } else {
        await api('catalog_create_quick_note', { body: { ...body, revision: 0 } });
        notifySuccess(`Indicación «${text.trim()}» agregada a ${category.name}.`);
        setText('');
        setAfter(undefined);
      }
      await onSaved();
    } catch (failure) {
      notifyError(failure, 'No se pudo guardar la indicación rápida.');
    } finally {
      setBusy(false);
    }
  }

  return (
    <form
      className="form-stack"
      aria-label={note ? `Editar indicación ${note.text}` : 'Nueva indicación rápida'}
      onSubmit={(event) => {
        event.preventDefault();
        void save();
      }}
    >
      <div className="form-grid">
        <Field
          label={note ? 'Texto de la indicación' : 'Nueva indicación'}
          hint={problem && text.length > 0 ? problem : 'Hasta 80 caracteres, sin comas.'}
        >
          <input
            type="text"
            maxLength={80}
            autoComplete="off"
            value={text}
            placeholder="Por ejemplo: Sin cebolla"
            aria-invalid={!!problem && text.length > 0}
            onChange={(event) => setText(event.target.value)}
          />
        </Field>
        <PositionField
          label="Posición"
          siblings={siblings}
          recordID={note?.id ?? ''}
          value={after}
          onChange={setAfter}
        />
      </div>
      <div className="button-row">
        <ToggleButton label="Indicación activa" pressed={active} onChange={setActive} />
        <button type="submit" className="primary-button" disabled={!!problem || busy}>
          {note ? 'Guardar indicación' : 'Agregar indicación'}
        </button>
        {onCancel && (
          <button type="button" className="secondary-button" onClick={onCancel}>
            Cancelar
          </button>
        )}
      </div>
    </form>
  );
}

// Indicaciones rápidas de una categoría (etapa 7.1): la captura las ofrece como botones al
// personalizar sus productos, antes de «Escribir otra», y llegan a la comanda como las escritas.
// Cada indicación se guarda por separado, con su revisión, fuera del formulario de la categoría.
export function CategoryQuickNotes({
  category,
  notes,
  editable,
  onChanged,
}: {
  category: Category;
  notes: CategoryQuickNote[];
  editable: boolean;
  onChanged: () => Promise<void>;
}) {
  const [editing, setEditing] = useState<string | null>(null);
  const ordered = orderSiblings(notes.filter((note) => note.category_id === category.id));
  const activeCount = ordered.filter((note) => note.active).length;
  const current = ordered.find((note) => note.id === editing) ?? null;
  return (
    <Accordion
      title="Indicaciones rápidas para cocina"
      summary={
        ordered.length === 0
          ? 'Sin indicaciones configuradas'
          : `${countLabel(activeCount, 'activa', 'activas')} de ${ordered.length}`
      }
      defaultOpen
    >
      <p className="muted text-sm">
        Aparecen como botones al personalizar un producto de «{category.name}», junto a «Escribir
        otra». No tienen precio: lo que se cobra sigue siendo un complemento o un ingrediente.
      </p>
      {ordered.length > 0 && (
        <div
          className="cash-choice-grid"
          role="group"
          aria-label={`Indicaciones de ${category.name}`}
        >
          {ordered.map((note) => (
            <button
              type="button"
              key={note.id}
              className={editing === note.id ? 'cash-choice-selected' : ''}
              aria-pressed={editing === note.id}
              disabled={!editable}
              onClick={() => setEditing(note.id)}
            >
              {note.text}
              {!note.active && <span className="muted text-xs"> · inactiva</span>}
            </button>
          ))}
        </div>
      )}
      {editable && current && (
        <QuickNoteEditor
          key={`${current.id}-${current.revision}`}
          category={category}
          note={current}
          notes={ordered}
          onSaved={async () => {
            setEditing(null);
            await onChanged();
          }}
          onCancel={() => setEditing(null)}
        />
      )}
      {editable && !current && (
        <QuickNoteEditor category={category} note={null} notes={ordered} onSaved={onChanged} />
      )}
    </Accordion>
  );
}
