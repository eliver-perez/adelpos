import { useState } from 'react';
import { Accordion } from '../../components/Accordion';
import { Field } from '../../components/Forms';
import { PositionField } from '../../components/PositionField';
import { SegmentedControl } from '../../components/SegmentedControl';
import { ToggleButton } from '../../components/ToggleButton';
import { ReferenceField } from '../../components/catalog/EditorFields';
import { api } from '../../services/api';
import { quickNoteEffect } from '../../services/catalog';
import type {
  Category,
  CategoryQuickNote,
  Ingredient,
  QuickNoteEffect,
} from '../../services/catalog';
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

// Tipos de efecto de la etapa 8, con botones. Ninguno pide gramos: la cantidad la fija cada
// producto (lo incluido, el máximo y la porción habitual del inventario).
const effectOptions: { value: QuickNoteEffect; label: string }[] = [
  { value: 'none', label: 'Solo instrucción' },
  { value: 'omit', label: 'Omitir' },
  { value: 'normal', label: 'Porción habitual' },
  { value: 'extra', label: 'Porción extra' },
];

const effectHints: Record<QuickNoteEffect, string> = {
  none: 'Solo llega a cocina; no cambia ingredientes, precio ni inventario.',
  omit: 'Al tocarla en la captura retira el ingrediente, como «Sin …» del personalizador.',
  normal:
    'Deja la porción incluida; si el producto no lo incluye, agrega una porción (con su cargo, si existe).',
  extra: 'Aplica la porción extra del personalizador (una más de lo incluido), con su cargo.',
};

// Resumen del efecto para el listado: «Omitir: Cebolla».
export function quickNoteEffectLabel(note: CategoryQuickNote): string {
  const effect = quickNoteEffect(note);
  if (effect === 'none') {
    return '';
  }
  const label = effectOptions.find((option) => option.value === effect)?.label ?? effect;
  return `${label}: ${note.ingredient_name ?? 'ingrediente'}`;
}

// Captura o edición de una indicación rápida: texto, efecto con su ingrediente, estado y posición
// dentro de la categoría.
function QuickNoteEditor({
  category,
  note,
  notes,
  ingredients,
  onSaved,
  onCancel,
}: {
  category: Category;
  note: CategoryQuickNote | null;
  notes: CategoryQuickNote[];
  ingredients: Ingredient[];
  onSaved: () => Promise<void>;
  onCancel?: () => void;
}) {
  const [text, setText] = useState(note?.text ?? '');
  const [active, setActive] = useState(note?.active ?? true);
  const [effect, setEffect] = useState<QuickNoteEffect>(note ? quickNoteEffect(note) : 'none');
  const [ingredientID, setIngredientID] = useState(note?.ingredient_id ?? '');
  const [after, setAfter] = useState<string | undefined>(undefined);
  const [busy, setBusy] = useState(false);
  const missingIngredient = effect !== 'none' && ingredientID === '';
  const problem =
    quickNoteProblem(text) || (missingIngredient ? 'Elige el ingrediente del efecto.' : '');
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
      effect,
      ingredient_id: effect === 'none' ? null : ingredientID,
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
        setEffect('none');
        setIngredientID('');
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
          hint={
            quickNoteProblem(text) && text.length > 0
              ? quickNoteProblem(text)
              : 'Hasta 80 caracteres, sin comas.'
          }
        >
          <input
            type="text"
            maxLength={80}
            autoComplete="off"
            value={text}
            placeholder="Por ejemplo: Sin cebolla"
            aria-invalid={!!quickNoteProblem(text) && text.length > 0}
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
      <div className="form-grid">
        <div className="field">
          <span>Efecto en el pedido</span>
          <SegmentedControl<QuickNoteEffect>
            label="Tipo de efecto"
            value={effect}
            options={effectOptions}
            onChange={setEffect}
          />
          <small className="muted">{effectHints[effect]}</small>
        </div>
        {effect !== 'none' && (
          <div className="form-stack">
            <ReferenceField
              label="Ingrediente vinculado"
              value={ingredientID || null}
              options={ingredients}
              onChange={setIngredientID}
            />
            <small className="muted">
              En la captura solo aparece en los productos que tienen este ingrediente con esa
              opción.
            </small>
          </div>
        )}
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
  ingredients = [],
  editable,
  onChanged,
}: {
  category: Category;
  notes: CategoryQuickNote[];
  ingredients?: Ingredient[];
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
        otra». No tienen precio propio: una indicación con efecto (omitir, porción habitual o extra)
        aplica la misma selección del ingrediente que el personalizador, con su cargo.
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
              {quickNoteEffectLabel(note) && (
                <span className="muted text-xs"> · {quickNoteEffectLabel(note)}</span>
              )}
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
          ingredients={ingredients}
          onSaved={async () => {
            setEditing(null);
            await onChanged();
          }}
          onCancel={() => setEditing(null)}
        />
      )}
      {editable && !current && (
        <QuickNoteEditor
          category={category}
          note={null}
          notes={ordered}
          ingredients={ingredients}
          onSaved={onChanged}
        />
      )}
    </Accordion>
  );
}
