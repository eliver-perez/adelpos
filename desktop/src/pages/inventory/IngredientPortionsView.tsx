import { useState } from 'react';
import { Accordion } from '../../components/Accordion';
import { useConfirmDialog } from '../../components/ConfirmDialog';
import { Field, Form, Notice } from '../../components/Forms';
import { inventoryChangeEvent } from '../../hooks/useInventoryEvents';
import { useResource } from '../../hooks/useResource';
import type { InventoryExecute, InventoryUnit } from '../../services/inventory';
import { saveIngredientPortion } from '../../services/inventoryRecipes';
import type {
  IngredientPortion,
  IngredientPortionPage,
  ItemReference,
} from '../../services/inventoryRecipes';
import { notifyError, notifySuccess } from '../../services/notify';
import { inventoryQuantityShape } from '../../services/numericInput';
import { countLabel } from '../../services/plural';
import { serverDateTime } from '../../services/restaurant';
import { DecimalTextField, Pagination } from './InventoryFields';
import { formatQuantity, inventoryPermissionName } from './inventoryFormat';
import { DimensionUnitSelect, RecipeItemPicker } from './RecipePickers';

const pageSize = 25;

interface PortionFilter {
  search: string;
  configured: string;
  active: string;
  page: number;
}

function portionQuery(filter: PortionFilter) {
  const query: Record<string, string> = {
    page: String(filter.page),
    page_size: String(pageSize),
  };
  if (filter.search.trim() !== '') {
    query.search = filter.search.trim();
  }
  if (filter.configured !== '') {
    query.configured = filter.configured;
  }
  if (filter.active !== '') {
    query.active = filter.active;
  }
  return query;
}

// Porción vigente legible: «20 g de Cebolla blanca» o «Sin porción».
export function portionText(portion: IngredientPortion) {
  if (!portion.configured || !portion.item) {
    return 'Sin porción: no consume inventario';
  }
  return `${formatQuantity(portion.quantity, portion.unit)} de ${portion.item.name}`;
}

function PortionHistory({ portion, timezone }: { portion: IngredientPortion; timezone: string }) {
  return (
    <Accordion
      title="Historial de la porción"
      summary={countLabel(portion.revision, 'cambio', 'cambios')}
      className="inventory-history"
    >
      {portion.history.length === 0 ? (
        <p className="muted text-sm">Nunca se ha configurado una porción.</p>
      ) : (
        <ol className="inventory-history-list">
          {portion.history.map((entry) => (
            <li
              key={`${entry.effective_from}:${entry.item_name ?? 'none'}:${entry.quantity ?? ''}`}
            >
              <strong>
                {entry.item_name
                  ? `${formatQuantity(entry.quantity, entry.unit_code ? { code: entry.unit_code } : null)} de ${entry.item_name}`
                  : 'Porción retirada'}
              </strong>
              <span className="muted text-xs">
                {' '}
                · {serverDateTime(entry.effective_from, timezone)}
                {entry.actor_name ? ` · ${entry.actor_name}` : ''}
              </span>
            </li>
          ))}
        </ol>
      )}
    </Accordion>
  );
}

function PortionForm({
  portion,
  units,
  execute,
  busy,
  onSaved,
}: {
  portion: IngredientPortion;
  units: InventoryUnit[];
  execute: InventoryExecute;
  busy: boolean;
  onSaved: (portion: IngredientPortion) => void;
}) {
  const [item, setItem] = useState<ItemReference | null>(portion.item);
  const [quantity, setQuantity] = useState(portion.quantity ?? '');
  const [unitID, setUnitID] = useState(portion.unit?.id ?? portion.item?.unit.id ?? '');
  const [removing, setRemoving] = useState(false);
  const { confirm, dialog } = useConfirmDialog();

  async function remove() {
    const confirmed = await confirm({
      title: 'Quitar la porción habitual',
      message: `«${portion.ingredient_name}» dejará de consumir inventario en las liberaciones siguientes, salvo en los productos cuya receta defina su propia porción. El historial se conserva.`,
      confirmLabel: 'Quitar porción',
      cancelLabel: 'Conservar porción',
      tone: 'danger',
    });
    if (!confirmed) {
      return;
    }
    setRemoving(true);
    try {
      const saved = await saveIngredientPortion(execute, portion.ingredient_id, {
        revision: portion.revision,
        item_id: null,
      });
      notifySuccess(`Porción de «${saved.ingredient_name}» retirada.`);
      onSaved(saved);
    } catch (problem) {
      notifyError(problem, 'No se pudo quitar la porción.');
    } finally {
      setRemoving(false);
    }
  }

  return (
    <>
      <Form
        label="Guardar porción"
        successMessage="Porción habitual guardada."
        disabled={busy || removing}
        onSave={async () => {
          const saved = await saveIngredientPortion(execute, portion.ingredient_id, {
            revision: portion.revision,
            item_id: item?.id ?? null,
            quantity,
            unit_id: unitID === '' ? null : unitID,
          });
          onSaved(saved);
        }}
      >
        <RecipeItemPicker
          label="Insumo por porción"
          value={item}
          required
          onChange={(next) => {
            setItem(next);
            setUnitID(next?.unit.id ?? '');
          }}
        />
        <div className="form-grid">
          <DecimalTextField
            label="Cantidad por porción"
            value={quantity}
            onChange={setQuantity}
            shape={inventoryQuantityShape}
            required
            hint="Lo que se usa en una porción normal; «Extra» consume dos y «Sin» ninguna."
          />
          <DimensionUnitSelect
            label="Unidad de la porción"
            units={units}
            dimension={item?.unit.dimension ?? null}
            value={unitID}
            onChange={setUnitID}
            hint={
              item
                ? `Go la convierte a ${item.unit.name.toLowerCase()} (${item.unit.code}).`
                : undefined
            }
          />
        </div>
        {portion.configured && (
          <div className="button-row">
            <button
              type="button"
              className="secondary-button"
              disabled={busy || removing}
              onClick={() => void remove()}
            >
              {removing ? 'Quitando…' : 'Quitar porción'}
            </button>
          </div>
        )}
      </Form>
      {dialog}
    </>
  );
}

// Panel de la porción de un ingrediente: edición (inventory.recipes.manage) e historial.
function PortionPanel({
  initial,
  units,
  editable,
  execute,
  busy,
  timezone,
  onClose,
}: {
  initial: IngredientPortion;
  units: InventoryUnit[];
  editable: boolean;
  execute: InventoryExecute;
  busy: boolean;
  timezone: string;
  onClose: () => void;
}) {
  const [portion, setPortion] = useState(initial);
  return (
    <section className="panel form-stack" aria-label={`Porción de ${portion.ingredient_name}`}>
      <div className="section-heading">
        <div>
          <h2>{portion.ingredient_name}</h2>
          <p className="muted text-xs">
            Vigente: {portionText(portion)}
            {portion.base_quantity !== null && portion.item
              ? ` (${formatQuantity(portion.base_quantity, portion.item.unit)} en la unidad base)`
              : ''}
          </p>
        </div>
        <button type="button" className="secondary-button" onClick={onClose}>
          Cerrar
        </button>
      </div>
      {editable && (
        <PortionForm
          key={`${portion.ingredient_id}:${portion.revision}`}
          portion={portion}
          units={units}
          execute={execute}
          busy={busy}
          onSaved={setPortion}
        />
      )}
      <PortionHistory portion={portion} timezone={timezone} />
    </section>
  );
}

// Apartado «Porciones habituales»: la cantidad de un insumo que representa una porción de cada
// ingrediente del catálogo. Una receta de producto puede sobrescribirla con un renglón de
// ingrediente. Cada cambio queda en el historial (solo inserción).
export function IngredientPortionsView({
  editable,
  units,
  execute,
  busy,
  timezone,
}: {
  editable: boolean;
  units: InventoryUnit[];
  execute: InventoryExecute;
  busy: boolean;
  timezone: string;
}) {
  const [filter, setFilter] = useState<PortionFilter>({
    search: '',
    configured: '',
    active: 'true',
    page: 1,
  });
  const [selected, setSelected] = useState<IngredientPortion | null>(null);
  const list = useResource<IngredientPortionPage>('inventory_ingredient_portions', {
    query: portionQuery(filter),
    reloadEvent: inventoryChangeEvent,
  });
  const page = list.data;
  return (
    <div className="form-stack">
      <p className="muted text-sm">
        Porción habitual de cada ingrediente del catálogo: el consumo es porciones pedidas ×
        porción. Un ingrediente sin porción no inventa consumo.
      </p>
      {!editable && (
        <Notice>
          Consulta disponible. Editar porciones requiere el permiso «
          {inventoryPermissionName('inventory.recipes.manage')}».
        </Notice>
      )}
      {selected && (
        <PortionPanel
          key={selected.ingredient_id}
          initial={selected}
          units={units}
          editable={editable}
          execute={execute}
          busy={busy}
          timezone={timezone}
          onClose={() => setSelected(null)}
        />
      )}
      <section className="panel catalog-filters" aria-label="Filtros de porciones">
        <Field label="Buscar ingrediente">
          <input
            type="search"
            maxLength={100}
            value={filter.search}
            placeholder="Nombre del ingrediente del catálogo"
            onChange={(event) => setFilter({ ...filter, search: event.target.value, page: 1 })}
          />
        </Field>
        <Field label="Porción">
          <select
            value={filter.configured}
            onChange={(event) => setFilter({ ...filter, configured: event.target.value, page: 1 })}
          >
            <option value="">Todas</option>
            <option value="true">Con porción</option>
            <option value="false">Sin porción</option>
          </select>
        </Field>
        <Field label="Estado del ingrediente">
          <select
            value={filter.active}
            onChange={(event) => setFilter({ ...filter, active: event.target.value, page: 1 })}
          >
            <option value="true">Activos</option>
            <option value="false">Inactivos</option>
            <option value="">Todos</option>
          </select>
        </Field>
      </section>
      <Notice>{list.error}</Notice>
      {!page && !list.error && <p className="muted">Consultando porciones…</p>}
      {page && (
        <section className="panel">
          <div className="table-container cash-table">
            <table className="inventory-table">
              <thead>
                <tr>
                  <th scope="col">Ingrediente del catálogo</th>
                  <th scope="col">Porción vigente</th>
                  <th scope="col" className="inventory-number">
                    En unidad base
                  </th>
                  <th scope="col">Último cambio</th>
                  <th scope="col">
                    <span className="sr-only">Acciones</span>
                  </th>
                </tr>
              </thead>
              <tbody>
                {page.items.map((portion) => (
                  <tr key={portion.ingredient_id}>
                    <td className="inventory-cell-name">
                      <strong>{portion.ingredient_name}</strong>
                      <span className="muted text-xs inventory-cell-detail">
                        Unidad en el catálogo: {portion.catalog_unit}
                        {portion.active ? '' : ' · inactivo'}
                      </span>
                    </td>
                    <td>
                      {portion.configured ? (
                        portionText(portion)
                      ) : (
                        <span className="tag tag-warning">Sin porción</span>
                      )}
                    </td>
                    <td className="inventory-number">
                      {formatQuantity(portion.base_quantity, portion.item?.unit)}
                    </td>
                    <td>
                      {portion.effective_from
                        ? serverDateTime(portion.effective_from, timezone)
                        : '—'}
                      {portion.updated_by_name && (
                        <span className="muted text-xs inventory-cell-detail">
                          {portion.updated_by_name}
                        </span>
                      )}
                    </td>
                    <td>
                      <button
                        type="button"
                        className="secondary-button"
                        aria-label={`${editable ? 'Editar' : 'Ver'} la porción de ${portion.ingredient_name}`}
                        onClick={() => setSelected(portion)}
                      >
                        {editable ? 'Editar porción' : 'Ver historial'}
                      </button>
                    </td>
                  </tr>
                ))}
                {page.items.length === 0 && (
                  <tr>
                    <td colSpan={5} className="muted">
                      No hay ingredientes del catálogo con estos filtros.
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
          <Pagination
            page={page.page}
            pageSize={page.page_size}
            total={page.total}
            singular="ingrediente"
            plural="ingredientes"
            onPage={(next) => setFilter({ ...filter, page: next })}
          />
        </section>
      )}
    </div>
  );
}
