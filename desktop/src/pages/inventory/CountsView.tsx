import { useState } from 'react';
import type { FormEvent } from 'react';
import { OptionalDateField } from '../../components/catalog/DateField';
import { Field, Notice } from '../../components/Forms';
import { IconLabel } from '../../components/Icon';
import { SegmentedControl } from '../../components/SegmentedControl';
import { inventoryChangeEvent } from '../../hooks/useInventoryEvents';
import { useResource } from '../../hooks/useResource';
import type {
  InventoryCategory,
  InventoryExecute,
  InventoryItem,
  Page,
  Warehouse,
} from '../../services/inventory';
import { createCount } from '../../services/inventoryControl';
import type {
  Count,
  CountInput,
  CountScope,
  CountStateFilter,
  CountSummary,
} from '../../services/inventoryControl';
import { notifyError, notifySuccess } from '../../services/notify';
import { acceptIntegerText } from '../../services/numericInput';
import { countLabel } from '../../services/plural';
import { serverDateTime } from '../../services/restaurant';
import { countFolio, countScopeOptions, countStateOptions, countStateTone } from './controlFormat';
import { defaultWarehouseID, WarehouseSelect } from './ControlShared';
import { CountPanel } from './CountPanel';
import { ItemPicker, Pagination } from './InventoryFields';
import { inventoryPermissionName } from './inventoryFormat';
import type { InventoryNavigate } from './inventoryNavigation';
import { Select } from '../../components/Select';

const pageSize = 25;

interface CountFilter {
  state: CountStateFilter | '';
  warehouse: string;
  from: string;
  until: string;
  search: string;
  page: number;
}

function countQuery(filter: CountFilter) {
  const query: Record<string, string> = {
    page: String(filter.page),
    page_size: String(pageSize),
  };
  const optional: [string, string][] = [
    ['state', filter.state],
    ['warehouse_id', filter.warehouse],
    ['from', filter.from],
    ['until', filter.until],
    ['search', filter.search.trim()],
  ];
  for (const [key, value] of optional) {
    if (value !== '') {
      query[key] = value;
    }
  }
  return query;
}

function CountFilters({
  filter,
  warehouses,
  onChange,
}: {
  filter: CountFilter;
  warehouses: Warehouse[];
  onChange: (filter: CountFilter) => void;
}) {
  return (
    <section className="panel catalog-filters" aria-label="Filtros de conteos">
      <Field label="Folio">
        <input
          type="search"
          inputMode="numeric"
          autoComplete="off"
          value={filter.search}
          placeholder="Folio exacto"
          onChange={(event) =>
            onChange({
              ...filter,
              search: acceptIntegerText(event.target.value, filter.search, 9),
              page: 1,
            })
          }
        />
      </Field>
      <Field label="Estado del conteo">
        <Select<CountStateFilter | ''>
          value={filter.state}
          options={[{ value: '', label: 'Todos' }, ...countStateOptions]}
          onChange={(state) => onChange({ ...filter, state, page: 1 })}
        />
      </Field>
      <Field label="Almacén">
        <Select
          value={filter.warehouse}
          options={[
            { value: '', label: 'Todos' },
            ...warehouses.map((warehouse) => ({
              value: warehouse.id,
              label: `${warehouse.name}${warehouse.active ? '' : ' (inactivo)'}`,
            })),
          ]}
          onChange={(warehouse) => onChange({ ...filter, warehouse, page: 1 })}
        />
      </Field>
      <OptionalDateField
        label="Creado desde"
        value={filter.from}
        onChange={(from) => onChange({ ...filter, from, page: 1 })}
      />
      <OptionalDateField
        label="Creado hasta"
        value={filter.until}
        onChange={(until) => onChange({ ...filter, until, page: 1 })}
      />
    </section>
  );
}

function CountsTable({
  page,
  timezone,
  onOpen,
}: {
  page: Page<CountSummary>;
  timezone: string;
  onOpen: (count: CountSummary) => void;
}) {
  return (
    <div className="table-container cash-table">
      <table className="inventory-table">
        <thead>
          <tr>
            <th scope="col">Conteo</th>
            <th scope="col">Almacén</th>
            <th scope="col">Alcance</th>
            <th scope="col">Avance</th>
            <th scope="col">Creado</th>
            <th scope="col">Estado</th>
            <th scope="col">
              <span className="sr-only">Acciones</span>
            </th>
          </tr>
        </thead>
        <tbody>
          {page.items.map((count) => (
            <tr key={count.id}>
              <td className="inventory-cell-name">
                <strong>{countFolio(count.folio)}</strong>
              </td>
              <td>{count.warehouse.name}</td>
              <td>
                {count.scope_label}
                {count.category && (
                  <span className="muted text-xs inventory-cell-detail">{count.category.name}</span>
                )}
              </td>
              <td>
                {count.counted_lines} de {countLabel(count.line_count, 'insumo', 'insumos')}
                {count.difference_lines !== null && (
                  <span className="muted text-xs inventory-cell-detail">
                    {countLabel(
                      count.difference_lines,
                      'diferencia aplicada',
                      'diferencias aplicadas',
                    )}
                  </span>
                )}
              </td>
              <td>
                {serverDateTime(count.created_at, timezone)}
                <span className="muted text-xs inventory-cell-detail">{count.created_by_name}</span>
              </td>
              <td>
                <span className={countStateTone(count.state)}>{count.state_label}</span>
              </td>
              <td>
                <button
                  type="button"
                  className="secondary-button"
                  aria-label={`Abrir el ${countFolio(count.folio).toLowerCase()} de ${count.warehouse.name}`}
                  onClick={() => onOpen(count)}
                >
                  Abrir
                </button>
              </td>
            </tr>
          ))}
          {page.items.length === 0 && (
            <tr>
              <td colSpan={7} className="muted">
                No hay conteos con estos filtros.
              </td>
            </tr>
          )}
        </tbody>
      </table>
    </div>
  );
}

// Alta de un conteo (inventory.counts.perform): almacén y alcance (todos los insumos activos, los
// de una categoría o una selección). Go arma la hoja en orden de categoría y nombre y rechaza un
// insumo que ya está en otro conteo sin terminar del mismo almacén. Crear el conteo no bloquea la
// venta ni toca existencias.
function CountForm({
  initialWarehouseID,
  execute,
  busy,
  onCreated,
  onCancel,
}: {
  initialWarehouseID: string;
  execute: InventoryExecute;
  busy: boolean;
  onCreated: (count: Count) => void;
  onCancel: () => void;
}) {
  const warehouses = useResource<Warehouse[]>('inventory_warehouses', {
    reloadEvent: inventoryChangeEvent,
  });
  const categories = useResource<InventoryCategory[]>('inventory_categories', {
    reloadEvent: inventoryChangeEvent,
  });
  const [warehouseID, setWarehouseID] = useState(initialWarehouseID);
  const [scope, setScope] = useState<CountScope>('all');
  const [categoryID, setCategoryID] = useState('');
  const [picked, setPicked] = useState<InventoryItem | null>(null);
  const [selection, setSelection] = useState<InventoryItem[]>([]);
  const [notes, setNotes] = useState('');
  const [sending, setSending] = useState(false);
  const warehouseList = warehouses.data ?? [];
  const selectedWarehouse = warehouseID || defaultWarehouseID(warehouseList);
  const activeCategories = (categories.data ?? []).filter((category) => category.active);
  const locked = busy || sending;
  let ready = selectedWarehouse !== '';
  if (scope === 'category') {
    ready = ready && categoryID !== '';
  }
  if (scope === 'items') {
    ready = ready && selection.length > 0;
  }

  function addPicked() {
    if (!picked || selection.some((item) => item.id === picked.id)) {
      setPicked(null);
      return;
    }
    setSelection((current) => [...current, picked]);
    setPicked(null);
  }

  async function submit(event: FormEvent) {
    event.preventDefault();
    if (!ready) {
      return;
    }
    const input: CountInput = { warehouse_id: selectedWarehouse, scope };
    if (scope === 'category') {
      input.category_id = categoryID;
    }
    if (scope === 'items') {
      input.item_ids = selection.map((item) => item.id);
    }
    if (notes.trim() !== '') {
      input.notes = notes.trim();
    }
    setSending(true);
    try {
      const created = await createCount(execute, input);
      notifySuccess(
        `${countFolio(created.folio)} abierto en «${created.warehouse.name}» con ${countLabel(created.line_count, 'insumo', 'insumos')}.`,
      );
      onCreated(created);
    } catch (problem) {
      notifyError(problem, 'No se pudo crear el conteo.');
    } finally {
      setSending(false);
    }
  }

  return (
    <section className="panel form-stack" aria-label="Nuevo conteo">
      <div className="section-heading">
        <div>
          <h2>Nuevo conteo</h2>
          <p className="muted text-xs">
            La venta sigue mientras se cuenta: cada cantidad guarda la hora en que se captura.
          </p>
        </div>
        <button type="button" className="secondary-button" onClick={onCancel}>
          Volver al listado
        </button>
      </div>
      <Notice>{warehouses.error || categories.error}</Notice>
      <form className="form-stack" onSubmit={(event) => void submit(event)}>
        <fieldset className="form-stack" disabled={locked}>
          <div className="form-grid">
            <WarehouseSelect
              label="Almacén que se cuenta"
              warehouses={warehouseList}
              value={selectedWarehouse}
              onChange={setWarehouseID}
            />
          </div>
          <div className="field">
            <span>Insumos que se cuentan</span>
            <SegmentedControl
              label="Alcance del conteo"
              value={scope}
              options={countScopeOptions}
              onChange={setScope}
            />
          </div>
          {scope === 'all' && (
            <p className="muted text-xs">
              Todos los insumos activos, en orden de categoría y nombre.
            </p>
          )}
          {scope === 'category' && (
            <div className="form-grid">
              <Field label="Categoría que se cuenta" hint="Sus insumos activos.">
                <Select
                  required
                  value={categoryID}
                  placeholder="Elige la categoría"
                  options={activeCategories.map((category) => ({
                    value: category.id,
                    label: category.name,
                  }))}
                  onChange={setCategoryID}
                />
              </Field>
            </div>
          )}
          {scope === 'items' && (
            <div className="form-stack">
              <ItemPicker
                label="Insumo"
                value={picked}
                activeOnly={false}
                emptyLabel="Elige un insumo para agregar"
                onChange={setPicked}
              />
              <div className="button-row">
                <button
                  type="button"
                  className="secondary-button"
                  disabled={!picked}
                  onClick={addPicked}
                >
                  <IconLabel icon="plus">Agregar a la selección</IconLabel>
                </button>
                <span className="muted text-xs">
                  {countLabel(selection.length, 'insumo elegido', 'insumos elegidos')}
                </span>
              </div>
              {selection.length > 0 && (
                <ul className="inventory-list" aria-label="Insumos elegidos">
                  {selection.map((item) => (
                    <li key={item.id} className="inventory-list-row">
                      <div className="inventory-list-text">
                        <strong>{item.name}</strong>
                        <span className="muted text-xs">
                          {item.code}
                          {item.category_name ? ` · ${item.category_name}` : ''}
                        </span>
                      </div>
                      <button
                        type="button"
                        className="secondary-button"
                        aria-label={`Quitar ${item.name} de la selección`}
                        onClick={() =>
                          setSelection((current) => current.filter((entry) => entry.id !== item.id))
                        }
                      >
                        Quitar
                      </button>
                    </li>
                  ))}
                </ul>
              )}
            </div>
          )}
          <Field label="Notas del conteo" hint="Opcional; hasta 1000 caracteres.">
            <textarea
              rows={2}
              maxLength={1000}
              value={notes}
              onChange={(event) => setNotes(event.target.value)}
            />
          </Field>
        </fieldset>
        <div className="form-actions">
          <button type="submit" className="primary-button" disabled={locked || !ready}>
            {sending ? 'Creando…' : 'Crear conteo'}
          </button>
        </div>
      </form>
    </section>
  );
}

type CountMode =
  | { kind: 'list' }
  | { kind: 'new'; warehouseID: string }
  | { kind: 'open'; id: string; initial: Count | null };

// Pestaña «Conteos» (inventory.counts.perform o inventory.counts.approve): conteos físicos por
// almacén con su estado (abierto, enviado, aprobado, aplicado o cancelado) y quién hizo cada paso.
// Quien realiza conteos los crea, captura y envía; quien los aprueba los revisa con las
// diferencias de Go, los aprueba y los aplica. Ningún paso bloquea la venta.
export function CountsView({
  can,
  execute,
  busy,
  timezone,
  initialCountID,
  initialWarehouseID,
  onNavigate,
}: {
  can: (permission: string) => boolean;
  execute: InventoryExecute;
  busy: boolean;
  timezone: string;
  // Conteo que se abre al llegar desde una alerta.
  initialCountID: string | null;
  // Almacén con el que se propone un conteo nuevo al llegar desde la alerta «Conteo pendiente».
  initialWarehouseID: string | null;
  onNavigate?: InventoryNavigate;
}) {
  const canPerform = can('inventory.counts.perform');
  const [mode, setMode] = useState<CountMode>(() => {
    if (initialCountID) {
      return { kind: 'open', id: initialCountID, initial: null };
    }
    if (initialWarehouseID && canPerform) {
      return { kind: 'new', warehouseID: initialWarehouseID };
    }
    return { kind: 'list' };
  });
  const [filter, setFilter] = useState<CountFilter>({
    state: 'active',
    warehouse: initialWarehouseID ?? '',
    from: '',
    until: '',
    search: '',
    page: 1,
  });
  const warehouses = useResource<Warehouse[]>('inventory_warehouses', {
    reloadEvent: inventoryChangeEvent,
  });
  const list = useResource<Page<CountSummary>>('inventory_counts', {
    query: countQuery(filter),
    enabled: mode.kind === 'list',
    reloadEvent: inventoryChangeEvent,
  });

  if (mode.kind === 'new') {
    return (
      <CountForm
        initialWarehouseID={mode.warehouseID}
        execute={execute}
        busy={busy}
        onCreated={(created) => setMode({ kind: 'open', id: created.id, initial: created })}
        onCancel={() => setMode({ kind: 'list' })}
      />
    );
  }
  if (mode.kind === 'open') {
    return (
      <CountPanel
        key={mode.id}
        countID={mode.id}
        initial={mode.initial}
        can={can}
        execute={execute}
        busy={busy}
        timezone={timezone}
        onClose={() => setMode({ kind: 'list' })}
        onNavigate={onNavigate}
      />
    );
  }

  return (
    <div className="form-stack">
      <div className="section-heading">
        <p className="muted text-sm">
          Conteos físicos por almacén. La venta sigue durante el conteo: la diferencia de cada
          renglón se calcula con la existencia teórica a la hora en que se capturó.
        </p>
        {canPerform && (
          <button
            type="button"
            className="primary-button"
            onClick={() => setMode({ kind: 'new', warehouseID: '' })}
          >
            <IconLabel icon="count">Nuevo conteo</IconLabel>
          </button>
        )}
      </div>
      {!canPerform && (
        <Notice>
          Revisión de conteos. Crearlos y capturarlos requiere el permiso «
          {inventoryPermissionName('inventory.counts.perform')}».
        </Notice>
      )}
      <CountFilters filter={filter} warehouses={warehouses.data ?? []} onChange={setFilter} />
      <Notice>{list.error}</Notice>
      {!list.data && !list.error && <p className="muted">Consultando conteos…</p>}
      {list.data && (
        <section className="panel" aria-label="Conteos registrados">
          <CountsTable
            page={list.data}
            timezone={timezone}
            onOpen={(count) => setMode({ kind: 'open', id: count.id, initial: null })}
          />
          <Pagination
            page={list.data.page}
            pageSize={list.data.page_size}
            total={list.data.total}
            singular="conteo"
            plural="conteos"
            onPage={(page) => setFilter({ ...filter, page })}
          />
        </section>
      )}
    </div>
  );
}
