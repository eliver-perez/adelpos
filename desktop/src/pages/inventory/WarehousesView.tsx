import { useState } from 'react';
import { Accordion } from '../../components/Accordion';
import { Field, Form, Notice } from '../../components/Forms';
import { IconLabel } from '../../components/Icon';
import { inventoryChangeEvent } from '../../hooks/useInventoryEvents';
import { useResource } from '../../hooks/useResource';
import { assignStationWarehouse, createWarehouse, saveWarehouse } from '../../services/inventory';
import type {
  InventoryExecute,
  StationWarehouse,
  Warehouse,
  WarehouseInput,
} from '../../services/inventory';
import { notifyError, notifySuccess } from '../../services/notify';
import { countLabel } from '../../services/plural';
import { serverDateTime } from '../../services/restaurant';
import { ActiveToggle } from './InventoryFields';
import { inventoryPermissionName } from './inventoryFormat';
import { Select } from '../../components/Select';

// Alta o edición de un almacén. El predeterminado («Almacén general») no se desactiva; Go además
// rechaza desactivar un almacén con existencias o asignado a estaciones.
function WarehouseEditor({
  warehouse,
  execute,
  busy,
  onClose,
}: {
  warehouse: Warehouse | null;
  execute: InventoryExecute;
  busy: boolean;
  onClose: () => void;
}) {
  const [name, setName] = useState(warehouse?.name ?? '');
  const [description, setDescription] = useState(warehouse?.description ?? '');
  const [active, setActive] = useState(warehouse?.active ?? true);
  return (
    <Form
      label={warehouse ? 'Guardar almacén' : 'Crear almacén'}
      successMessage={warehouse ? 'Almacén guardado.' : 'Almacén creado.'}
      disabled={busy}
      onSave={async () => {
        const input: WarehouseInput = { name, description, active };
        if (warehouse) {
          await saveWarehouse(execute, warehouse.id, { ...input, revision: warehouse.revision });
        } else {
          await createWarehouse(execute, input);
        }
        onClose();
      }}
    >
      <div className="form-grid">
        <Field label="Nombre del almacén">
          <input
            required
            maxLength={60}
            value={name}
            onChange={(event) => setName(event.target.value)}
          />
        </Field>
        <Field label="Descripción del almacén" hint="Opcional; por ejemplo, su ubicación.">
          <input
            maxLength={500}
            value={description}
            onChange={(event) => setDescription(event.target.value)}
          />
        </Field>
      </div>
      <div className="button-row">
        <ActiveToggle
          label="Almacén activo"
          pressed={active}
          onChange={setActive}
          disabled={warehouse?.is_default ?? false}
          confirmDeactivation={warehouse?.active ?? false}
          title="Desactivar almacén"
          message={`Al guardar, «${warehouse?.name ?? ''}» dejará de ofrecerse para nuevas operaciones. Debe estar sin existencias y sin estaciones asignadas.`}
        />
        <button type="button" className="secondary-button" onClick={onClose}>
          Cancelar
        </button>
      </div>
      {warehouse?.is_default && (
        <p className="muted text-xs">
          Es el almacén predeterminado: consumen de él las estaciones sin asignación y no se
          desactiva.
        </p>
      )}
    </Form>
  );
}

function WarehouseList({
  warehouses,
  editable,
  execute,
  busy,
}: {
  warehouses: Warehouse[];
  editable: boolean;
  execute: InventoryExecute;
  busy: boolean;
}) {
  // null: sin formulario; '': almacén nuevo; id: almacén en edición.
  const [editing, setEditing] = useState<string | null>(null);
  const current = warehouses.find((warehouse) => warehouse.id === editing) ?? null;
  return (
    <section className="panel form-stack" aria-labelledby="inventory-warehouses-title">
      <div className="section-heading">
        <div>
          <h2 id="inventory-warehouses-title">Almacenes</h2>
          <p className="muted text-xs">
            Las existencias se llevan por insumo y almacén. No se crean almacenes automáticamente.
          </p>
        </div>
        {editable && editing === null && (
          <button type="button" className="primary-button" onClick={() => setEditing('')}>
            <IconLabel icon="plus">Nuevo almacén</IconLabel>
          </button>
        )}
      </div>
      <ul className="inventory-list">
        {warehouses.map((warehouse) => (
          <li key={warehouse.id} className="inventory-list-row">
            <div className="inventory-list-text">
              <strong>{warehouse.name}</strong>
              {warehouse.description && (
                <span className="muted text-xs">{warehouse.description}</span>
              )}
              <span className="muted text-xs">
                {warehouse.stations.length === 0
                  ? 'Ninguna estación consume de este almacén'
                  : `Consumen: ${warehouse.stations.map((station) => station.name).join(', ')}`}
              </span>
            </div>
            <div className="button-row">
              {warehouse.is_default && <span className="tag tag-good">Predeterminado</span>}
              {!warehouse.active && <span className="tag">Inactivo</span>}
              {editable && (
                <button
                  type="button"
                  className="secondary-button"
                  aria-label={`Editar el almacén ${warehouse.name}`}
                  disabled={editing !== null}
                  onClick={() => setEditing(warehouse.id)}
                >
                  Editar
                </button>
              )}
            </div>
          </li>
        ))}
      </ul>
      {editable && editing !== null && (
        <WarehouseEditor
          key={current ? `${current.id}:${current.revision}` : 'new'}
          warehouse={current}
          execute={execute}
          busy={busy}
          onClose={() => setEditing(null)}
        />
      )}
    </section>
  );
}

// Una estación con su almacén vigente, el cambio de asignación y su historial plegable. El valor
// vacío vuelve al almacén predeterminado (Go guarda warehouse_id null).
function StationRow({
  station,
  warehouses,
  editable,
  execute,
  busy,
  timezone,
}: {
  station: StationWarehouse;
  warehouses: Warehouse[];
  editable: boolean;
  execute: InventoryExecute;
  busy: boolean;
  timezone: string;
}) {
  const [selected, setSelected] = useState(station.warehouse_id ?? '');
  const [saving, setSaving] = useState(false);
  const fallback = warehouses.find((warehouse) => warehouse.is_default);
  const choices = warehouses.filter(
    (warehouse) =>
      !warehouse.is_default && (warehouse.active || warehouse.id === station.warehouse_id),
  );
  const changed = selected !== (station.warehouse_id ?? '');

  async function assign() {
    setSaving(true);
    try {
      const saved = await assignStationWarehouse(execute, station.station_id, {
        revision: station.revision,
        warehouse_id: selected === '' ? null : selected,
      });
      notifySuccess(`${saved.station_name} consume ahora de «${saved.warehouse_name}».`);
    } catch (problem) {
      notifyError(problem, 'No se pudo asignar el almacén.');
    } finally {
      setSaving(false);
    }
  }

  let since = 'Sin asignación registrada';
  if (station.effective_from) {
    since = `Desde el ${serverDateTime(station.effective_from, timezone)}`;
    if (station.assigned_by_name) {
      since += ` · ${station.assigned_by_name}`;
    }
  }

  return (
    <li className="inventory-station">
      <div className="inventory-station-main">
        <div className="inventory-list-text">
          <strong>
            {station.station_name}
            {!station.station_active && <span className="tag inventory-inline-tag">Inactiva</span>}
          </strong>
          <span>
            {station.warehouse_name}
            {station.uses_default ? ' (predeterminado)' : ''}
          </span>
          <span className="muted text-xs">{since}</span>
        </div>
        {editable && (
          <div className="inventory-station-assign">
            <Field label={`Almacén de ${station.station_name}`}>
              <Select
                value={selected}
                disabled={busy || saving}
                options={[
                  {
                    value: '',
                    label: `Usar el predeterminado${fallback ? ` («${fallback.name}»)` : ''}`,
                  },
                  ...choices.map((warehouse) => ({
                    value: warehouse.id,
                    label: `${warehouse.name}${warehouse.active ? '' : ' (inactivo)'}`,
                  })),
                ]}
                onChange={setSelected}
              />
            </Field>
            <button
              type="button"
              className="secondary-button"
              disabled={!changed || busy || saving}
              onClick={() => void assign()}
            >
              {saving ? 'Asignando…' : 'Asignar'}
            </button>
          </div>
        )}
      </div>
      {station.history.length > 0 && (
        <Accordion
          title="Historial de asignaciones"
          summary={countLabel(station.history.length, 'asignación', 'asignaciones')}
          className="inventory-history"
        >
          <ol className="inventory-history-list">
            {station.history.map((entry) => (
              <li key={`${entry.effective_from}:${entry.warehouse_id ?? 'default'}`}>
                <strong>{entry.warehouse_name}</strong>
                {entry.warehouse_id === null ? ' (predeterminado)' : ''}
                <span className="muted text-xs">
                  {' '}
                  · {serverDateTime(entry.effective_from, timezone)}
                  {entry.actor_name ? ` · ${entry.actor_name}` : ''}
                </span>
              </li>
            ))}
          </ol>
        </Accordion>
      )}
    </li>
  );
}

// Pestaña «Almacenes»: almacenes y almacén de consumo de cada estación de preparación (§5).
export function WarehousesView({
  can,
  execute,
  busy,
  timezone,
}: {
  can: (permission: string) => boolean;
  execute: InventoryExecute;
  busy: boolean;
  timezone: string;
}) {
  const warehouses = useResource<Warehouse[]>('inventory_warehouses', {
    reloadEvent: inventoryChangeEvent,
  });
  const stations = useResource<StationWarehouse[]>('inventory_station_warehouses', {
    reloadEvent: inventoryChangeEvent,
  });
  const editable = can('inventory.warehouses.manage');
  const list = warehouses.data ?? [];
  return (
    <div className="form-stack">
      {!editable && (
        <Notice>
          Consulta disponible. Administrar almacenes requiere el permiso «
          {inventoryPermissionName('inventory.warehouses.manage')}».
        </Notice>
      )}
      <Notice>{warehouses.error || stations.error}</Notice>
      {warehouses.data && (
        <WarehouseList warehouses={list} editable={editable} execute={execute} busy={busy} />
      )}
      <section className="panel form-stack" aria-labelledby="inventory-stations-title">
        <div>
          <h2 id="inventory-stations-title">Almacén de consumo por estación</h2>
          <p className="muted text-xs">
            Cada estación de preparación consume del almacén asignado; sin asignación, del
            predeterminado. Varias estaciones pueden compartir un almacén y el consumo de un
            producto se calcula una sola vez.
          </p>
        </div>
        {stations.data && stations.data.length === 0 && (
          <p className="muted">
            Aún no hay estaciones de preparación. Se configuran en Cocina y preparación.
          </p>
        )}
        {stations.data && warehouses.data && (
          <ul className="inventory-stations">
            {stations.data.map((station) => (
              <StationRow
                key={`${station.station_id}:${station.revision}`}
                station={station}
                warehouses={list}
                editable={editable}
                execute={execute}
                busy={busy}
                timezone={timezone}
              />
            ))}
          </ul>
        )}
      </section>
    </div>
  );
}
