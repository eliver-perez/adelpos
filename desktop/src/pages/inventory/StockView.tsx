import { useState } from 'react';
import { Field, Notice } from '../../components/Forms';
import { Icon, IconLabel } from '../../components/Icon';
import { inventoryChangeEvent } from '../../hooks/useInventoryEvents';
import { useResource } from '../../hooks/useResource';
import type {
  InventoryCategory,
  InventoryExecute,
  StockPage,
  StockQuery,
  StockRow,
  StockState,
  Warehouse,
} from '../../services/inventory';
import { formatMoney } from '../../services/money';
import { countLabel } from '../../services/plural';
import { InitialBalanceForm } from './InitialBalanceForm';
import { Pagination } from './InventoryFields';
import { formatQuantity, stockStateOptions } from './inventoryFormat';
import { Select } from '../../components/Select';

const pageSize = 25;

interface StockFilter {
  warehouse: string;
  category: string;
  state: StockState | '';
  search: string;
  active: string;
  page: number;
}

function stockQuery(filter: StockFilter): Record<string, string> {
  const query: Record<string, string> & StockQuery = {
    page: String(filter.page),
    page_size: String(pageSize),
  };
  if (filter.warehouse !== '') {
    query.warehouse_id = filter.warehouse;
  }
  if (filter.category !== '') {
    query.category_id = filter.category;
  }
  if (filter.state !== '') {
    query.state = filter.state;
  }
  if (filter.search.trim() !== '') {
    query.search = filter.search.trim();
  }
  if (filter.active === 'true' || filter.active === 'false') {
    query.active = filter.active;
  }
  return query;
}

function StockFilters({
  filter,
  warehouses,
  categories,
  onChange,
}: {
  filter: StockFilter;
  warehouses: Warehouse[];
  categories: InventoryCategory[];
  onChange: (filter: StockFilter) => void;
}) {
  return (
    <section className="panel catalog-filters" aria-label="Filtros de existencias">
      <Field label="Buscar insumo">
        <input
          type="search"
          maxLength={100}
          value={filter.search}
          placeholder="Nombre, código o código de barras"
          onChange={(event) => onChange({ ...filter, search: event.target.value, page: 1 })}
        />
      </Field>
      <Field label="Almacén">
        <Select
          value={filter.warehouse}
          options={[
            { value: '', label: 'Todos (existencia total)' },
            ...warehouses.map((warehouse) => ({
              value: warehouse.id,
              label: `${warehouse.name}${warehouse.active ? '' : ' (inactivo)'}`,
            })),
          ]}
          onChange={(warehouse) => onChange({ ...filter, warehouse, page: 1 })}
        />
      </Field>
      <Field label="Categoría">
        <Select
          value={filter.category}
          options={[
            { value: '', label: 'Todas' },
            { value: 'none', label: 'Sin categoría' },
            ...categories.map((category) => ({ value: category.id, label: category.name })),
          ]}
          onChange={(category) => onChange({ ...filter, category, page: 1 })}
        />
      </Field>
      <Field label="Existencia">
        <Select<StockState | ''>
          value={filter.state}
          options={[{ value: '', label: 'Todas' }, ...stockStateOptions]}
          onChange={(state) => onChange({ ...filter, state, page: 1 })}
        />
      </Field>
      <Field label="Insumos">
        <Select
          value={filter.active}
          options={[
            { value: 'true', label: 'Activos' },
            { value: 'false', label: 'Inactivos' },
            { value: '', label: 'Todos' },
          ]}
          onChange={(active) => onChange({ ...filter, active, page: 1 })}
        />
      </Field>
    </section>
  );
}

function StockTags({ row }: { row: StockRow }) {
  const flagged =
    row.negative || row.below_minimum || row.at_reorder_point || !!row.cost_pending || !row.active;
  if (!flagged) {
    return <span className="muted">—</span>;
  }
  return (
    <div className="inventory-tags">
      {row.negative && (
        <span className="tag tag-warning">
          <Icon name="alert" size={14} />
          Existencia negativa
        </span>
      )}
      {row.below_minimum && <span className="tag tag-warning">Bajo mínimo</span>}
      {row.at_reorder_point && <span className="tag tag-warning">Reabastecer</span>}
      {row.cost_pending && <span className="tag">Costo pendiente</span>}
      {!row.active && <span className="tag">Inactivo</span>}
    </div>
  );
}

// Origen del nivel comparado (fase D): con el filtro de almacén, el nivel propio del almacén
// («warehouse») o el del insumo contra la existencia total («item»).
function levelScope(row: StockRow) {
  return (row as StockRow & { level_scope?: 'item' | 'warehouse' }).level_scope ?? 'item';
}

// Desglose por almacén que entrega Go (solo almacenes con existencia distinta de cero).
function WarehouseBreakdown({ row }: { row: StockRow }) {
  if (row.warehouses.length === 0) {
    return <span className="muted">Sin existencias registradas</span>;
  }
  return (
    <ul className="inventory-breakdown">
      {row.warehouses.map((entry) => (
        <li
          key={entry.warehouse_id}
          className={entry.negative ? 'inventory-negative-text' : undefined}
        >
          {entry.warehouse_name}: {formatQuantity(entry.quantity, row.unit)}
        </li>
      ))}
    </ul>
  );
}

function StockTable({ page }: { page: StockPage }) {
  const scoped = page.warehouse !== null;
  // El costo promedio llega con inventory.costs.read; el valor del insumo exige además
  // inventory.valuation.read (Go lo envía en null sin ese permiso aunque haya costos).
  const valueVisible = page.costs_visible && page.valuation_visible;
  const columns = 6 + (scoped ? 1 : 0) + (page.costs_visible ? 1 : 0) + (valueVisible ? 1 : 0);
  return (
    <div className="table-container cash-table">
      <table className="inventory-table">
        <thead>
          <tr>
            <th scope="col">Insumo</th>
            <th scope="col">{scoped ? `En ${page.warehouse?.name}` : 'Existencia total'}</th>
            {scoped && <th scope="col">Existencia total</th>}
            <th scope="col">Por almacén</th>
            <th scope="col">Mínimo y reabastecimiento</th>
            {page.costs_visible && <th scope="col">Costo promedio</th>}
            {valueVisible && <th scope="col">Valor del insumo</th>}
            <th scope="col">Situación</th>
            <th scope="col">Categoría</th>
          </tr>
        </thead>
        <tbody>
          {page.items.map((row) => (
            <tr key={row.item_id} className={row.negative ? 'inventory-row-negative' : undefined}>
              <td className="inventory-cell-name">
                <strong>{row.item_name}</strong>
                <span className="muted text-xs inventory-cell-detail">{row.item_code}</span>
              </td>
              <td className="inventory-number">
                <strong className={row.negative ? 'inventory-negative-text' : undefined}>
                  {formatQuantity(row.quantity, row.unit)}
                </strong>
              </td>
              {scoped && (
                <td className="inventory-number">{formatQuantity(row.total_quantity, row.unit)}</td>
              )}
              <td>
                <WarehouseBreakdown row={row} />
              </td>
              <td>
                {row.min_stock === null && row.reorder_point === null ? (
                  '—'
                ) : (
                  <span className="inventory-cell-stack">
                    {row.min_stock !== null && (
                      <span>Mín.&nbsp;{formatQuantity(row.min_stock, row.unit)}</span>
                    )}
                    {row.reorder_point !== null && (
                      <span>Reabastecer en {formatQuantity(row.reorder_point, row.unit)}</span>
                    )}
                    {scoped && (
                      <span className="muted text-xs">
                        {levelScope(row) === 'warehouse'
                          ? `Nivel de ${page.warehouse?.name}`
                          : 'Nivel del insumo (existencia total)'}
                      </span>
                    )}
                  </span>
                )}
              </td>
              {page.costs_visible && (
                <td className="inventory-number">
                  {row.average_cost_display
                    ? `${formatMoney(row.average_cost_display)} / ${row.unit.code}`
                    : '—'}
                </td>
              )}
              {valueVisible && (
                <td className="inventory-number">{formatMoney(row.value_display)}</td>
              )}
              <td>
                <StockTags row={row} />
              </td>
              <td>{row.category_name ?? 'Sin categoría'}</td>
            </tr>
          ))}
          {page.items.length === 0 && (
            <tr>
              <td colSpan={columns} className="muted">
                No hay existencias con estos filtros.
              </td>
            </tr>
          )}
        </tbody>
      </table>
    </div>
  );
}

// Pestaña «Existencias» (inventory.stock.read): existencia total y por almacén de cada insumo.
// El costo promedio solo con inventory.costs.read; el valor de cada insumo y el total del
// inventario solo con inventory.valuation.read; Go los omite en los demás casos. Una existencia negativa se resalta,
// pero nunca bloquea nada (§6).
export function StockView({
  can,
  execute,
  busy,
  initialState,
  initialSearch = '',
  initialWarehouse = '',
  timezone,
}: {
  can: (permission: string) => boolean;
  execute: InventoryExecute;
  busy: boolean;
  initialState: StockState | '';
  // Insumo (por su código) y almacén con los que se abre desde una alerta del resumen.
  initialSearch?: string;
  initialWarehouse?: string;
  timezone: string;
}) {
  const [filter, setFilter] = useState<StockFilter>({
    warehouse: initialWarehouse,
    category: '',
    state: initialState,
    search: initialSearch,
    active: 'true',
    page: 1,
  });
  const [recording, setRecording] = useState(false);
  const warehouses = useResource<Warehouse[]>('inventory_warehouses', {
    reloadEvent: inventoryChangeEvent,
  });
  const categories = useResource<InventoryCategory[]>('inventory_categories', {
    reloadEvent: inventoryChangeEvent,
  });
  const stock = useResource<StockPage>('inventory_stock', {
    query: stockQuery(filter),
    reloadEvent: inventoryChangeEvent,
  });
  const canRecordInitial = can('inventory.adjustments.authorize') && can('inventory.costs.manage');
  const negatives = stock.data?.items.filter((row) => row.negative).length ?? 0;

  if (recording && warehouses.data) {
    return (
      <InitialBalanceForm
        warehouses={warehouses.data}
        execute={execute}
        busy={busy}
        timezone={timezone}
        onClose={() => setRecording(false)}
      />
    );
  }

  return (
    <div className="form-stack">
      <div className="section-heading">
        <p className="muted text-sm">
          Existencias por insumo y almacén. Solo cambian con movimientos registrados; nunca se
          editan a mano.
        </p>
        {canRecordInitial && (
          <button
            type="button"
            className="primary-button"
            disabled={!warehouses.data}
            onClick={() => setRecording(true)}
          >
            <IconLabel icon="inventory">Existencia inicial</IconLabel>
          </button>
        )}
      </div>
      <StockFilters
        filter={filter}
        warehouses={warehouses.data ?? []}
        categories={categories.data ?? []}
        onChange={setFilter}
      />
      <Notice>{stock.error || warehouses.error || categories.error}</Notice>
      {stock.data?.valuation_visible && stock.data.total_value_display !== null && (
        <div className="inventory-total">
          <span className="muted text-sm">Valor total del inventario</span>
          <strong>{formatMoney(stock.data.total_value_display)}</strong>
        </div>
      )}
      {negatives > 0 && (
        <div className="inventory-negative-notice" role="status">
          <Icon name="alert" />
          <p>
            {countLabel(negatives, 'insumo', 'insumos')} de esta página con existencia negativa: se
            registraron salidas mayores que la existencia. No bloquea ninguna operación; se corrige
            con las entradas siguientes.
          </p>
        </div>
      )}
      {!stock.data && !stock.error && <p className="muted">Consultando existencias…</p>}
      {stock.data && (
        <section className="panel">
          <StockTable page={stock.data} />
          <Pagination
            page={stock.data.page}
            pageSize={stock.data.page_size}
            total={stock.data.total}
            singular="insumo"
            plural="insumos"
            onPage={(page) => setFilter({ ...filter, page })}
          />
        </section>
      )}
    </div>
  );
}
