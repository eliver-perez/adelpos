import { useState } from 'react';
import { useConfirmDialog } from '../../components/ConfirmDialog';
import { Field, Form, Notice } from '../../components/Forms';
import { IconLabel } from '../../components/Icon';
import { PositionField } from '../../components/PositionField';
import { createInventoryCategory, saveInventoryCategory } from '../../services/inventory';
import type { CategoryInput, InventoryCategory, InventoryExecute } from '../../services/inventory';
import { notifyError, notifySuccess } from '../../services/notify';
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

// Nombre comparable para la búsqueda: sin distinguir mayúsculas ni acentos.
function searchable(text: string) {
  return text
    .normalize('NFD')
    .replace(/[\u0300-\u036f]/g, '')
    .toLocaleLowerCase('es-MX')
    .trim();
}

// Categorías para clasificar insumos (opcionales), en su orden visual (etapa 9A, §3.5): búsqueda por
// nombre (la lista es completa, se filtra en pantalla), conteo de insumos, estado, acceso a sus
// insumos (Insumos filtrado por la categoría), activar o desactivar y editar nombre y posición.
// Go evita duplicados (DUPLICATE_NAME) y conserva la categoría de los insumos al desactivarla.
export function CategoriesAdmin({
  categories,
  editable,
  execute,
  busy,
  onViewItems,
}: {
  categories: InventoryCategory[];
  editable: boolean;
  execute: InventoryExecute;
  busy: boolean;
  // Abre Insumos filtrado por la categoría.
  onViewItems?: (category: InventoryCategory) => void;
}) {
  const [editing, setEditing] = useState<string | null>(null);
  const [search, setSearch] = useState('');
  const { confirm, dialog } = useConfirmDialog();
  const ordered = orderSiblings(categories);
  const current = ordered.find((category) => category.id === editing) ?? null;
  const term = searchable(search);
  const shown =
    term === '' ? ordered : ordered.filter((category) => searchable(category.name).includes(term));

  async function toggle(category: InventoryCategory) {
    if (category.active) {
      const confirmed = await confirm({
        title: 'Desactivar categoría',
        message: `«${category.name}» dejará de ofrecerse al clasificar insumos. Los ${countLabel(category.item_count, 'insumo', 'insumos')} que ya la tienen la conservan.`,
        confirmLabel: 'Desactivar',
        cancelLabel: 'Mantener activa',
        tone: 'danger',
      });
      if (!confirmed) {
        return;
      }
    }
    try {
      await saveInventoryCategory(execute, category.id, {
        name: category.name,
        active: !category.active,
        revision: category.revision,
      });
      notifySuccess(
        category.active
          ? `Categoría «${category.name}» desactivada.`
          : `Categoría «${category.name}» activada.`,
      );
    } catch (error) {
      notifyError(error, 'No se pudo cambiar el estado de la categoría.');
    }
  }

  return (
    <div className="form-stack">
      {!editable && (
        <Notice>
          Modificar las categorías requiere el permiso «
          {inventoryPermissionName('inventory.items.manage')}».
        </Notice>
      )}
      <div className="section-heading">
        <Field label="Buscar categoría">
          <input
            type="search"
            maxLength={60}
            value={search}
            placeholder="Nombre de la categoría"
            onChange={(event) => setSearch(event.target.value)}
          />
        </Field>
        {editable && (
          <button type="button" className="primary-button" onClick={() => setEditing('')}>
            <IconLabel icon="plus">Nueva categoría</IconLabel>
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
      <div className="table-container cash-table">
        <table className="inventory-table">
          <caption className="sr-only">Categorías de insumos</caption>
          <thead>
            <tr>
              <th scope="col">Categoría</th>
              <th scope="col">Insumos</th>
              <th scope="col">Estado</th>
              <th scope="col">
                <span className="sr-only">Acciones</span>
              </th>
            </tr>
          </thead>
          <tbody>
            {shown.map((category) => (
              <tr key={category.id}>
                <td className="inventory-cell-name">
                  <strong>{category.name}</strong>
                </td>
                <td>{countLabel(category.item_count, 'insumo', 'insumos')}</td>
                <td>
                  <span className={category.active ? 'tag tag-good' : 'tag tag-muted'}>
                    {category.active ? 'Activa' : 'Inactiva'}
                  </span>
                </td>
                <td>
                  <div className="button-row">
                    {onViewItems && (
                      <button
                        type="button"
                        className="secondary-button"
                        aria-label={`Ver los insumos de ${category.name}`}
                        onClick={() => onViewItems(category)}
                      >
                        Ver insumos
                      </button>
                    )}
                    {editable && (
                      <button
                        type="button"
                        className="secondary-button"
                        aria-label={`${category.active ? 'Desactivar' : 'Activar'} la categoría ${category.name}`}
                        disabled={busy}
                        onClick={() => void toggle(category)}
                      >
                        {category.active ? 'Desactivar' : 'Activar'}
                      </button>
                    )}
                    {editable && (
                      <button
                        type="button"
                        className="secondary-button"
                        aria-label={`Editar la categoría ${category.name}`}
                        aria-pressed={editing === category.id}
                        onClick={() => setEditing(category.id)}
                      >
                        Editar
                      </button>
                    )}
                  </div>
                </td>
              </tr>
            ))}
            {shown.length === 0 && (
              <tr>
                <td colSpan={4} className="muted">
                  {ordered.length === 0
                    ? 'Todavía no hay categorías de insumos.'
                    : 'Ninguna categoría coincide con la búsqueda.'}
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
      {dialog}
    </div>
  );
}
