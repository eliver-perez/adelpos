import { useState } from 'react';
import { Field, Notice } from '../../components/Forms';
import { IconLabel } from '../../components/Icon';
import { inventoryChangeEvent } from '../../hooks/useInventoryEvents';
import { useResource } from '../../hooks/useResource';
import type {
  InventoryCategory,
  InventoryExecute,
  InventoryItem,
  InventoryUnit,
  ItemPage,
} from '../../services/inventory';
import { formatMoney } from '../../services/money';
import { countLabel } from '../../services/plural';
import { Pagination } from './InventoryFields';
import { formatQuantity } from './inventoryFormat';
import { ItemEditorPanel } from './ItemEditor';

const pageSize = 25;

interface ItemFilter {
  search: string;
  category: string;
  active: string;
  kind: string;
  page: number;
}

function itemQuery(filter: ItemFilter) {
  const query: Record<string, string> = {
    page: String(filter.page),
    page_size: String(pageSize),
  };
  if (filter.search.trim() !== '') {
    query.search = filter.search.trim();
  }
  if (filter.category !== '') {
    query.category_id = filter.category;
  }
  if (filter.active !== '') {
    query.active = filter.active;
  }
  if (filter.kind !== '') {
    query.kind = filter.kind;
  }
  return query;
}

function presentationSummary(item: InventoryItem) {
  const active = item.presentations.filter((presentation) => presentation.active);
  if (active.length === 0) {
    return '—';
  }
  const preferred = active.find((presentation) => presentation.purchase_default);
  const count = countLabel(active.length, 'presentación', 'presentaciones');
  return preferred ? `${count} · compra: ${preferred.name}` : count;
}

function alertSummary(item: InventoryItem) {
  const parts: string[] = [];
  if (item.min_stock !== null) {
    parts.push(`Mín.\u00a0${formatQuantity(item.min_stock, item.base_unit)}`);
  }
  if (item.reorder_point !== null) {
    parts.push(`Reabastecer en ${formatQuantity(item.reorder_point, item.base_unit)}`);
  }
  return parts.length > 0 ? parts.join(' · ') : '—';
}

function ItemFilters({
  filter,
  categories,
  onChange,
}: {
  filter: ItemFilter;
  categories: InventoryCategory[];
  onChange: (filter: ItemFilter) => void;
}) {
  return (
    <section className="panel catalog-filters" aria-label="Filtros de insumos">
      <Field label="Buscar insumo">
        <input
          type="search"
          maxLength={100}
          value={filter.search}
          placeholder="Nombre, código o código de barras"
          onChange={(event) => onChange({ ...filter, search: event.target.value, page: 1 })}
        />
      </Field>
      <Field label="Categoría">
        <select
          value={filter.category}
          onChange={(event) => onChange({ ...filter, category: event.target.value, page: 1 })}
        >
          <option value="">Todas</option>
          <option value="none">Sin categoría</option>
          {categories.map((category) => (
            <option key={category.id} value={category.id}>
              {category.name}
            </option>
          ))}
        </select>
      </Field>
      <Field label="Estado">
        <select
          value={filter.active}
          onChange={(event) => onChange({ ...filter, active: event.target.value, page: 1 })}
        >
          <option value="">Todos</option>
          <option value="true">Activos</option>
          <option value="false">Inactivos</option>
        </select>
      </Field>
      <Field label="Tipo">
        <select
          value={filter.kind}
          onChange={(event) => onChange({ ...filter, kind: event.target.value, page: 1 })}
        >
          <option value="">Todos</option>
          <option value="purchased">Comprados</option>
          <option value="prepared">Preparaciones almacenables</option>
        </select>
      </Field>
    </section>
  );
}

function ItemsTable({
  page,
  editable,
  onOpen,
}: {
  page: ItemPage;
  editable: boolean;
  onOpen: (item: InventoryItem) => void;
}) {
  return (
    <div className="table-container cash-table">
      <table className="inventory-table">
        <thead>
          <tr>
            <th scope="col">Insumo</th>
            <th scope="col">Categoría</th>
            <th scope="col">Unidad base</th>
            <th scope="col">Presentaciones</th>
            <th scope="col">Alertas</th>
            {page.costs_visible && <th scope="col">Costo de referencia</th>}
            <th scope="col">Estado</th>
            <th scope="col">
              <span className="sr-only">Acciones</span>
            </th>
          </tr>
        </thead>
        <tbody>
          {page.items.map((item) => (
            <tr key={item.id}>
              <td className="inventory-cell-name">
                <strong>{item.name}</strong>
                <span className="muted text-xs inventory-cell-detail">
                  {item.code}
                  {item.barcode ? ` · ${item.barcode}` : ''}
                </span>
              </td>
              <td>{item.category_name ?? 'Sin categoría'}</td>
              <td>
                {item.base_unit.name}
                <span className="muted text-xs"> · {item.base_unit.code}</span>
              </td>
              <td>{presentationSummary(item)}</td>
              <td>{alertSummary(item)}</td>
              {page.costs_visible && (
                <td className="inventory-number">
                  {item.reference_cost_display
                    ? `${formatMoney(item.reference_cost_display)} / ${item.base_unit.code}`
                    : '—'}
                </td>
              )}
              <td>
                <div className="inventory-tags">
                  <span className={item.active ? 'tag tag-good' : 'tag'}>
                    {item.active ? 'Activo' : 'Inactivo'}
                  </span>
                  {item.kind === 'prepared' && <span className="tag">{item.kind_label}</span>}
                </div>
              </td>
              <td>
                <button
                  type="button"
                  className="secondary-button"
                  aria-label={`${editable ? 'Editar' : 'Ver'} el insumo ${item.name}`}
                  onClick={() => onOpen(item)}
                >
                  {editable ? 'Editar' : 'Ver'}
                </button>
              </td>
            </tr>
          ))}
          {page.items.length === 0 && (
            <tr>
              <td colSpan={page.costs_visible ? 8 : 7} className="muted">
                No hay insumos con estos filtros.
              </td>
            </tr>
          )}
        </tbody>
      </table>
    </div>
  );
}

// Sección «Insumos»: listado con búsqueda, filtros y paginación; alta y edición en un panel con
// sus presentaciones. Desde la etapa 9A las categorías y las unidades se administran en el grupo
// Configuración, que abre esta sección filtrada por una categoría («Ver insumos»).
export function ItemsView({
  can,
  execute,
  busy,
  initialItemID = null,
  initialCategoryID = '',
}: {
  can: (permission: string) => boolean;
  execute: InventoryExecute;
  busy: boolean;
  // Insumo que se abre al llegar desde una alerta del resumen.
  initialItemID?: string | null;
  // Categoría con la que se filtra el listado al llegar desde Categorías.
  initialCategoryID?: string;
}) {
  const [filter, setFilter] = useState<ItemFilter>({
    search: '',
    category: initialCategoryID,
    active: '',
    kind: '',
    page: 1,
  });
  // null: listado; '': insumo nuevo; id: insumo abierto.
  const [editing, setEditing] = useState<string | null>(initialItemID);
  const categories = useResource<InventoryCategory[]>('inventory_categories', {
    reloadEvent: inventoryChangeEvent,
  });
  const units = useResource<InventoryUnit[]>('inventory_units', {
    reloadEvent: inventoryChangeEvent,
  });
  const list = useResource<ItemPage>('inventory_items', {
    query: itemQuery(filter),
    reloadEvent: inventoryChangeEvent,
  });
  const editable = can('inventory.items.manage');
  const categoryList = categories.data ?? [];
  const unitList = units.data ?? [];

  if (editing !== null) {
    return (
      <ItemEditorPanel
        initialItemID={editing}
        categories={categoryList}
        units={unitList}
        costsVisible={list.data?.costs_visible ?? false}
        can={can}
        execute={execute}
        busy={busy}
        onClose={() => setEditing(null)}
      />
    );
  }

  return (
    <div className="form-stack">
      <div className="section-heading">
        <p className="muted text-sm">
          Insumos inventariables con su unidad base, presentaciones de compra y alertas de
          existencia.
        </p>
        {editable && (
          <button
            type="button"
            className="primary-button"
            disabled={!units.data}
            onClick={() => setEditing('')}
          >
            <IconLabel icon="plus">Nuevo insumo</IconLabel>
          </button>
        )}
      </div>
      <ItemFilters filter={filter} categories={categoryList} onChange={setFilter} />
      <Notice>{list.error || categories.error || units.error}</Notice>
      {!list.data && !list.error && <p className="muted">Consultando insumos…</p>}
      {list.data && (
        <section className="panel">
          <ItemsTable page={list.data} editable={editable} onOpen={(item) => setEditing(item.id)} />
          <Pagination
            page={list.data.page}
            pageSize={list.data.page_size}
            total={list.data.total}
            singular="insumo"
            plural="insumos"
            onPage={(page) => setFilter({ ...filter, page })}
          />
        </section>
      )}
    </div>
  );
}
