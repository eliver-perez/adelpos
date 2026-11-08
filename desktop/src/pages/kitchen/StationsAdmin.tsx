import { useState } from 'react';
import { Accordion } from '../../components/Accordion';
import { Field, Form, Notice } from '../../components/Forms';
import {
  Check,
  IntegerField,
  ReferenceField,
  TextField,
} from '../../components/catalog/EditorFields';
import { useLiveResource } from '../../hooks/useLiveResource';
import type { DeviceScope, ExecuteKitchen, Station } from '../../services/kitchen';

const emptyStation = (): Station => ({
  id: '',
  name: '',
  description: '',
  active: true,
  sort_order: 0,
  reception: 'printer',
  printer_id: null,
  target_minutes: 15,
  warning_minutes: 15,
  overdue_minutes: 25,
  grouping: 'inherit',
  tracking: 'inherit',
  revision: 0,
});

export function StationsAdmin({
  can,
  execute,
  busy,
}: {
  can: (key: string) => boolean;
  execute: ExecuteKitchen;
  busy: boolean;
}) {
  const stations = useLiveResource<Station[]>('kitchen_stations');
  const outputs = useLiveResource<{ id: string; name: string }[]>('kitchen_outputs', {
    enabled: can('kitchen.stations.manage'),
  });
  const [selected, setSelected] = useState<Station | null>(null);
  const current = stations.data?.find((station) => station.id === selected?.id);
  const stale = !!selected?.id && current?.revision !== selected.revision;

  return (
    <div className="kitchen-editor-layout">
      <section className="panel form-stack">
        <div className="section-heading">
          <h2>Estaciones de preparación</h2>
          {can('kitchen.stations.manage') && (
            <button className="primary-button" onClick={() => setSelected(emptyStation())}>
              Nueva estación
            </button>
          )}
        </div>
        <Notice>{stations.error || outputs.error}</Notice>
        {stations.data?.map((station) => (
          <button
            className="kitchen-list-button"
            key={station.id}
            onClick={() => setSelected(station)}
          >
            <strong>{station.name}</strong>
            <span>
              {station.active ? 'Activa' : 'Inactiva'} ·{' '}
              {station.reception === 'printer'
                ? 'Impresora'
                : station.reception === 'both'
                  ? 'Impresora y KDS'
                  : 'Solo KDS'}{' '}
              · objetivo {station.target_minutes} min
            </span>
          </button>
        ))}
        {stations.data?.length === 0 && (
          <p className="muted">Crea las estaciones reales del restaurante y configura sus rutas.</p>
        )}
        {can('kitchen.stations.manage') && stations.data && (
          <DeviceStations stations={stations.data} execute={execute} busy={busy} />
        )}
      </section>
      {selected && (
        <section className="panel form-stack">
          <div className="section-heading">
            <h2>{selected.id ? selected.name : 'Nueva estación'}</h2>
            <button className="secondary-button" onClick={() => setSelected(null)}>
              Cerrar editor
            </button>
          </div>
          <Notice>
            {stale &&
              'La estación cambió en otra terminal. Tus campos se conservan; revisa la versión vigente antes de guardar.'}
          </Notice>
          {stale && current && (
            <button
              className="secondary-button"
              onClick={() => setSelected({ ...selected, revision: current.revision })}
            >
              Usar revisión vigente y conservar mis campos
            </button>
          )}
          <Form
            disabled={busy || stale || !can('kitchen.stations.manage')}
            label="Guardar estación"
            onSave={async () => {
              const saved = await execute<Station>(
                selected.id ? 'kitchen_save_station' : 'kitchen_create_station',
                { id: selected.id || undefined, body: { ...selected } },
              );
              setSelected(saved);
            }}
          >
            <TextField
              label="Nombre de la estación"
              required
              value={selected.name}
              onChange={(name) => setSelected({ ...selected, name })}
            />
            <TextField
              label="Descripción de la estación"
              value={selected.description}
              onChange={(description) => setSelected({ ...selected, description })}
            />
            <IntegerField
              label="Orden de la estación"
              value={selected.sort_order}
              onChange={(sort_order) => setSelected({ ...selected, sort_order })}
            />
            <Field label="Recepción de comandas">
              <select
                value={selected.reception}
                onChange={(event) =>
                  setSelected({
                    ...selected,
                    reception: event.target.value as Station['reception'],
                  })
                }
              >
                <option value="printer">Solo impresora</option>
                <option value="kds">Solo KDS</option>
                <option value="both">Impresora y KDS</option>
              </select>
            </Field>
            {selected.reception !== 'kds' && (
              <ReferenceField
                label="Impresora compartida o exclusiva"
                value={selected.printer_id}
                options={outputs.data ?? []}
                onChange={(printer_id) =>
                  setSelected({ ...selected, printer_id: printer_id || null })
                }
              />
            )}
            {selected.reception !== 'printer' && (
              <p className="muted">
                La recepción KDS queda pendiente hasta que un cliente autorizado reciba y confirme
                la comanda.
              </p>
            )}
            <Accordion
              title="Agrupación y seguimiento"
              summary="Varias estaciones pueden compartir la misma impresora"
            >
              <Field label="Agrupación de comandas">
                <select
                  value={selected.grouping}
                  onChange={(event) =>
                    setSelected({
                      ...selected,
                      grouping: event.target.value as Station['grouping'],
                    })
                  }
                >
                  <option value="inherit">Usar configuración del establecimiento</option>
                  <option value="independent">Ticket independiente</option>
                  <option value="unified">Unificar con estaciones de la misma impresora</option>
                </select>
              </Field>
              <Field label="Seguimiento de la estación">
                <select
                  value={selected.tracking}
                  onChange={(event) =>
                    setSelected({
                      ...selected,
                      tracking: event.target.value as Station['tracking'],
                    })
                  }
                >
                  <option value="inherit">Usar configuración del establecimiento</option>
                  <option value="item">Por producto</option>
                  <option value="order">Por pedido</option>
                </select>
              </Field>
            </Accordion>
            <Accordion
              title="Tiempos y advertencias"
              summary={`Objetivo ${selected.target_minutes} min · atraso ${selected.overdue_minutes} min`}
            >
              <fieldset disabled={!can('kitchen.timing.manage')} className="form-grid">
                <IntegerField
                  label="Tiempo objetivo en minutos"
                  min={1}
                  max={1440}
                  value={selected.target_minutes}
                  onChange={(target_minutes) => setSelected({ ...selected, target_minutes })}
                />
                <IntegerField
                  label="Advertencia en minutos"
                  min={1}
                  max={1440}
                  value={selected.warning_minutes}
                  onChange={(warning_minutes) => setSelected({ ...selected, warning_minutes })}
                />
                <IntegerField
                  label="Atraso en minutos"
                  min={1}
                  max={2880}
                  value={selected.overdue_minutes}
                  onChange={(overdue_minutes) => setSelected({ ...selected, overdue_minutes })}
                />
              </fieldset>
            </Accordion>
            <Check
              label="Estación activa"
              value={selected.active}
              onChange={(active) => setSelected({ ...selected, active })}
            />
          </Form>
        </section>
      )}
    </div>
  );
}

function DeviceStations({
  stations,
  execute,
  busy,
}: {
  stations: Station[];
  execute: ExecuteKitchen;
  busy: boolean;
}) {
  const resource = useLiveResource<DeviceScope[]>('kitchen_access');
  const [selected, setSelected] = useState<DeviceScope | null>(null);
  const [expected, setExpected] = useState<string[]>([]);
  return (
    <Accordion
      title="Ámbitos de terminales y futuro KDS"
      summary="Las terminales sin permiso global solo operan sus estaciones asignadas"
    >
      <Notice>{resource.error}</Notice>
      <ReferenceField
        optional
        label="Terminal para asignar estaciones"
        value={selected?.device_id ?? ''}
        options={(resource.data ?? []).map((device) => ({
          id: device.device_id,
          name: device.device_name,
        }))}
        onChange={(id) => {
          const device = resource.data?.find((entry) => entry.device_id === id) ?? null;
          setSelected(device);
          setExpected(device?.station_ids ?? []);
        }}
      />
      {selected && (
        <Form
          disabled={busy}
          label="Guardar estaciones de terminal"
          onSave={async () => {
            await execute('kitchen_save_access', {
              body: {
                device_id: selected.device_id,
                station_ids: selected.station_ids,
                expected_station_ids: expected,
              },
            });
            setExpected(selected.station_ids);
          }}
        >
          {stations.map((station) => (
            <Check
              key={station.id}
              label={station.name}
              value={selected.station_ids.includes(station.id)}
              onChange={(checked) =>
                setSelected({
                  ...selected,
                  station_ids: checked
                    ? [...selected.station_ids, station.id]
                    : selected.station_ids.filter((id) => id !== station.id),
                })
              }
            />
          ))}
        </Form>
      )}
    </Accordion>
  );
}
