import { useState } from 'react';
import { Field, Form, Notice } from '../../components/Forms';
import { PositionField } from '../../components/PositionField';
import { createInventoryCategory, saveInventoryCategory } from '../../services/inventory';
import type { CategoryInput, InventoryCategory, InventoryExecute } from '../../services/inventory';
import { countLabel } from '../../services/plural';
import { orderSiblings, positionPayload } from '../../services/positions';
import { ActiveToggle } from './InventoryFields';
import { inventoryPermissionName } from './inventoryFormat';

// Alta o edición de una categoría de insumos: nombre, estado y posición en la lista (Go recalcula
// el orden con internal/ordering).
function CategoryEditor({
  category,
  categories,
  execute,
  busy,
  onClose,
}: {
  category: InventoryCategory | null;
  categories: InventoryCategory[];
  execute: InventoryExecute;
  busy: boolean;
  onClose: () => void;
}) {
  const [name, setName] = useState(category?.name ?? '');
  const [active, setActive] = useState(category?.active ?? true);
  const [after, setAfter] = useState<string | undefined>(undefined);
  const siblings = categories.map((item) => ({
    id: item.id,
    name: item.name,
    note: item.active ? undefined : 'inactiva',
  }));
  return (
    <Form
      label={category ? 'Guardar categoría' : 'Crear categoría'}
      successMessage={category ? 'Categoría de insumos guardada.' : 'Categoría de insumos creada.'}
      disabled={busy}
      onSave={async () => {
        const input: CategoryInput = { name, active };
        const position = positionPayload(categories, category?.id ?? '', after);
        if (position) {
          input.position = position;
        }
        if (category) {
          await saveInventoryCategory(execute, category.id, {
            ...input,
            revision: category.revision,
          });
        } else {
          await createInventoryCategory(execute, input);
        }
        onClose();
      }}
    >
      <div className="form-grid">
        <Field label="Nombre de la categoría">
          <input
            required
            maxLength={60}
            value={name}
            onChange={(event) => setName(event.target.value)}
          />
        </Field>
        <PositionField
          label="Posición de la categoría"
          siblings={siblings}
          recordID={category?.id ?? ''}
          value={after}
          onChange={setAfter}
        />
      </div>
      <div className="button-row">
        <ActiveToggle
          label="Categoría activa"
          pressed={active}
          onChange={setActive}
          confirmDeactivation={category?.active ?? false}
          title="Desactivar categoría"
          message={`Al guardar, «${category?.name ?? ''}» dejará de ofrecerse al clasificar insumos.`}
        />
        <button type="button" className="secondary-button" onClick={onClose}>
          Cancelar
        </button>
      </div>
      <p className="muted text-xs">
        Desactivar una categoría solo deja de ofrecerla: los insumos que ya la tienen la conservan.
      </p>
    </Form>
  );
}

// Categorías para clasificar insumos (opcionales), en su orden visual.
export function CategoriesAdmin({
  categories,
  editable,
  execute,
  busy,
}: {
  categories: InventoryCategory[];
  editable: boolean;
  execute: InventoryExecute;
  busy: boolean;
}) {
  const [editing, setEditing] = useState<string | null>(null);
  const ordered = orderSiblings(categories);
  const current = ordered.find((category) => category.id === editing) ?? null;
  return (
    <div className="form-stack">
      {!editable && (
        <Notice>
          Modificar las categorías requiere el permiso «
          {inventoryPermissionName('inventory.items.manage')}».
        </Notice>
      )}
      <div className="cash-choice-grid" role="group" aria-label="Categorías de insumos">
        {ordered.map((category) => (
          <button
            type="button"
            key={category.id}
            className={editing === category.id ? 'cash-choice-selected' : ''}
            aria-pressed={editing === category.id}
            disabled={!editable}
            onClick={() => setEditing(category.id)}
          >
            {category.name}
            <span className="muted text-xs">
              {' '}
              · {countLabel(category.item_count, 'insumo', 'insumos')}
            </span>
            {!category.active && <span className="muted text-xs"> · inactiva</span>}
          </button>
        ))}
        {editable && (
          <button type="button" onClick={() => setEditing('')}>
            + Nueva categoría
          </button>
        )}
      </div>
      {editable && editing !== null && (
        <CategoryEditor
          key={current ? `${current.id}:${current.revision}` : 'new'}
          category={current}
          categories={ordered}
          execute={execute}
          busy={busy}
          onClose={() => setEditing(null)}
        />
      )}
    </div>
  );
}
