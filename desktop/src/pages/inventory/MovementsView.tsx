import { useState } from 'react';
import { OptionalDateField } from '../../components/catalog/DateField';
import { Field, Notice } from '../../components/Forms';
import { inventoryChangeEvent } from '../../hooks/useInventoryEvents';
import { useResource } from '../../hooks/useResource';
import type {
  InventoryItem,
  InventoryMovement,
  MovementKind,
  MovementPage,
  Warehouse,
} from '../../services/inventory';
import { formatMoney, formatSignedMoney } from '../../services/money';
import { serverDateTime } from '../../services/restaurant';
import { ConsumptionRequestsSection } from './ConsumptionRequestsSection';
import { ConsumptionTraceView } from './ConsumptionTrace';
import { ItemPicker, Pagination } from './InventoryFields';
import {
  formatQuantity,
  formatSignedQuantity,
  movementKindOptions,
  sourceKindOptions,
} from './inventoryFormat';

const pageSize = 50;

interface MovementFilter {
  item: InventoryItem | null;
  warehouse: string;
  kind: MovementKind | '';
  source: string;
  from: string;
  until: string;
  page: number;
}

function movementQuery(filter: MovementFilter) {
  const query: Record<string, string> = {
    page: String(filter.page),
    page_size: String(pageSize),
  };
  if (filter.item) {
    query.item_id = filter.item.id;
  }
  if (filter.warehouse !== '') {
    query.warehouse_id = filter.warehouse;
  }
  if (filter.kind !== '') {
    query.kind = filter.kind;
  }
  if (filter.source !== '') {
    query.source_kind = filter.source;
  }
  if (filter.from !== '') {
    query.from = filter.from;
  }
  if (filter.until !== '') {
    query.until = filter.until;
  }
  return query;
}

function MovementFilters({
  filter,
  warehouses,
  onChange,
}: {
  filter: MovementFilter;
  warehouses: Warehouse[];
  onChange: (filter: MovementFilter) => void;
}) {
  return (
    <section className="panel catalog-filters" aria-label="Filtros de movimientos">
      <ItemPicker
        label="Insumo"
        value={filter.item}
        activeOnly={false}
        emptyLabel="Todos los insumos"
        onChange={(item) => onChange({ ...filter, item, page: 1 })}
      />
      <Field label="Almacén">
        <select
          value={filter.warehouse}
          onChange={(event) => onChange({ ...filter, warehouse: event.target.value, page: 1 })}
        >
          <option value="">Todos</option>
          {warehouses.map((warehouse) => (
            <option key={warehouse.id} value={warehouse.id}>
              {warehouse.name}
            </option>
          ))}
        </select>
      </Field>
      <Field label="Tipo de movimiento">
        <select
          value={filter.kind}
          onChange={(event) =>
            onChange({ ...filter, kind: event.target.value as MovementKind | '', page: 1 })
          }
        >
          <option value="">Todos</option>
          {movementKindOptions.map((option) => (
            <option key={option.value} value={option.value}>
              {option.label}
            </option>
          ))}
        </select>
      </Field>
      <Field label="Origen">
        <select
          value={filter.source}
          onChange={(event) => onChange({ ...filter, source: event.target.value, page: 1 })}
        >
          <option value="">Todos</option>
          {sourceKindOptions.map((option) => (
            <option key={option.value} value={option.value}>
              {option.label}
            </option>
          ))}
        </select>
      </Field>
      <OptionalDateField
        label="Fecha de negocio desde"
        value={filter.from}
        onChange={(from) => onChange({ ...filter, from, page: 1 })}
      />
      <OptionalDateField
        label="Fecha de negocio hasta"
        value={filter.until}
        onChange={(until) => onChange({ ...filter, until, page: 1 })}
      />
    </section>
  );
}

// Responsable legible del movimiento: la persona o el proceso de Go que lo registró.
function responsible(movement: InventoryMovement) {
  if (movement.actor_name) {
    return movement.actor_name;
  }
  return movement.process_label ?? '—';
}

function MovementRow({
  movement,
  costsVisible,
  timezone,
  onShowTrace,
}: {
  movement: InventoryMovement;
  costsVisible: boolean;
  timezone: string;
  onShowTrace: (unitID: string) => void;
}) {
  const unitID = movement.restaurant_unit_id;
  const quantity =
    movement.kind === 'cost_adjustment'
      ? '—'
      : formatSignedQuantity(movement.quantity, movement.unit);
  return (
    <tr>
      <td>
        {serverDateTime(movement.occurred_at, timezone)}
        <span className="muted text-xs inventory-cell-detail">
          Registrado: {serverDateTime(movement.recorded_at, timezone)}
        </span>
      </td>
      <td className="inventory-cell-name">
        <strong>{movement.item_name}</strong>
        <span className="muted text-xs inventory-cell-detail">{movement.item_code}</span>
      </td>
      <td>{movement.warehouse_name ?? 'Valuación del insumo'}</td>
      <td>{movement.kind_label}</td>
      <td className="inventory-number">
        <strong>{quantity}</strong>
      </td>
      <td className="inventory-number">
        {movement.stock_after === null ? '—' : formatQuantity(movement.stock_after, movement.unit)}
      </td>
      <td>
        {movement.source_kind_label}
        {movement.reason && (
          <span className="muted text-xs inventory-cell-detail">{movement.reason}</span>
        )}
        {unitID && (
          <button
            type="button"
            className="secondary-button inventory-trace-button"
            aria-label={`Ver consumo de la partida del movimiento de ${movement.item_name}`}
            onClick={() => onShowTrace(unitID)}
          >
            Ver consumo de la partida
          </button>
        )}
      </td>
      <td>{responsible(movement)}</td>
      {costsVisible && (
        <td className="inventory-number">
          {formatSignedMoney(movement.value_display)}
          {movement.unit_cost_display && (
            <span className="muted text-xs inventory-cell-detail">
              {formatMoney(movement.unit_cost_display)} / {movement.unit.code}
            </span>
          )}
          {movement.cost_basis_label && (
            <span className="muted text-xs inventory-cell-detail">
              {movement.cost_basis_label}
              {movement.cost_pending ? ' · costo pendiente' : ''}
            </span>
          )}
        </td>
      )}
    </tr>
  );
}

// Pestaña «Movimientos» (inventory.movements.read): libro de solo inserción, del más reciente al
// más antiguo por hora de negocio. Valores y costos solo con inventory.costs.read; sin ese permiso
// Go tampoco lista los ajustes de costo. No se muestran identificadores técnicos. Desde la fase B
// también muestra las solicitudes de consumo pendientes o con error y, desde un movimiento de una
// partida, la traza de su consumo.
export function MovementsView({ timezone }: { timezone: string }) {
  const [trace, setTrace] = useState<string | null>(null);
  const [filter, setFilter] = useState<MovementFilter>({
    item: null,
    warehouse: '',
    kind: '',
    source: '',
    from: '',
    until: '',
    page: 1,
  });
  const warehouses = useResource<Warehouse[]>('inventory_warehouses', {
    reloadEvent: inventoryChangeEvent,
  });
  const movements = useResource<MovementPage>('inventory_movements', {
    query: movementQuery(filter),
    reloadEvent: inventoryChangeEvent,
  });
  const page = movements.data;
  const costsVisible = page?.costs_visible ?? false;
  if (trace) {
    return (
      <ConsumptionTraceView
        unitID={trace}
        timezone={timezone}
        closeLabel="Volver a movimientos"
        onClose={() => setTrace(null)}
      />
    );
  }
  return (
    <div className="form-stack">
      <p className="muted text-sm">
        Cada cambio de existencia queda en este libro con su origen y responsable. Los movimientos
        no se editan ni se borran: una corrección es otro movimiento.
      </p>
      <ConsumptionRequestsSection timezone={timezone} onShowTrace={setTrace} />
      <MovementFilters filter={filter} warehouses={warehouses.data ?? []} onChange={setFilter} />
      <Notice>{movements.error || warehouses.error}</Notice>
      {!page && !movements.error && <p className="muted">Consultando movimientos…</p>}
      {page && (
        <section className="panel">
          <div className="table-container cash-table">
            <table className="inventory-table">
              <thead>
                <tr>
                  <th scope="col">Fecha de negocio</th>
                  <th scope="col">Insumo</th>
                  <th scope="col">Almacén</th>
                  <th scope="col">Tipo</th>
                  <th scope="col">Cantidad</th>
                  <th scope="col">Existencia después</th>
                  <th scope="col">Origen</th>
                  <th scope="col">Responsable</th>
                  {costsVisible && <th scope="col">Valor</th>}
                </tr>
              </thead>
              <tbody>
                {page.items.map((movement) => (
                  <MovementRow
                    key={movement.id}
                    movement={movement}
                    costsVisible={costsVisible}
                    timezone={timezone}
                    onShowTrace={setTrace}
                  />
                ))}
                {page.items.length === 0 && (
                  <tr>
                    <td colSpan={costsVisible ? 9 : 8} className="muted">
                      No hay movimientos con estos filtros.
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
            singular="movimiento"
            plural="movimientos"
            onPage={(next) => setFilter({ ...filter, page: next })}
          />
        </section>
      )}
    </div>
  );
}
