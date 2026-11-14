import { useState } from 'react';
import { Accordion } from '../../components/Accordion';
import { Field, Form, Notice } from '../../components/Forms';
import {
  Check,
  IntegerField,
  ReferenceField,
  TextField,
} from '../../components/catalog/EditorFields';
import { PositionField } from '../../components/PositionField';
import { useLiveResource } from '../../hooks/useLiveResource';
import { printerLabel } from '../../services/kitchen';
import { positionPayload } from '../../services/positions';
import type { Position } from '../../services/catalog';
import type { ExecuteKitchen, Station } from '../../services/kitchen';
import { TerminalStations } from './TerminalStations';
import { Select } from '../../components/Select';

// Borrador local de la estación: la posición elegida solo viaja al guardar.
type StationDraft = Station & { position?: Position };

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
  // Etapa 9G (fase E): cada impresora dice en qué terminal está conectada, si es la USB de una terminal.
  const outputs = useLiveResource<{ id: string; name: string; terminal_name?: string | null }[]>(
    'kitchen_outputs',
    {
      enabled: can('kitchen.stations.manage'),
    },
  );
  const [selected, setSelected] = useState<StationDraft | null>(null);
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
                  ? 'Impresora y pantalla de cocina'
                  : 'Solo pantalla de cocina'}{' '}
              · objetivo {station.target_minutes} min
            </span>
          </button>
        ))}
        {stations.data?.length === 0 && (
          <p className="muted">Crea las estaciones reales del restaurante y configura sus rutas.</p>
        )}
        {can('kitchen.stations.manage') && stations.data && (
          <TerminalStations stations={stations.data} execute={execute} busy={busy} />
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
              Usar la versión vigente y conservar mis campos
            </button>
          )}
          <Form
            disabled={busy || stale || !can('kitchen.stations.manage')}
            label="Guardar estación"
            successMessage="Estación guardada."
            onSave={async () => {
              const body: Record<string, unknown> = { ...selected };
              delete body.position;
              const position = positionPayload(
                stations.data ?? [],
                selected.id,
                selected.position?.after,
              );
              if (position) {
                body.position = position;
              }
              const saved = await execute<Station>(
                selected.id ? 'kitchen_save_station' : 'kitchen_create_station',
                { id: selected.id || undefined, body },
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
            <PositionField
              label="Posición de la estación"
              siblings={stations.data ?? []}
              recordID={selected.id}
              value={selected.position?.after}
              onChange={(after) => setSelected({ ...selected, position: { after } })}
            />
            <Field label="Recepción de comandas">
              <Select<Station['reception']>
                value={selected.reception}
                options={[
                  { value: 'printer', label: 'Solo impresora' },
                  { value: 'kds', label: 'Solo pantalla de cocina' },
                  { value: 'both', label: 'Impresora y pantalla de cocina' },
                ]}
                onChange={(reception) => setSelected({ ...selected, reception })}
              />
            </Field>
            {selected.reception !== 'kds' && (
              <ReferenceField
                label="Impresora compartida o exclusiva"
                value={selected.printer_id}
                options={(outputs.data ?? []).map((output) => ({
                  ...output,
                  name: printerLabel(output),
                }))}
                onChange={(printer_id) =>
                  setSelected({ ...selected, printer_id: printer_id || null })
                }
              />
            )}
            {selected.reception !== 'printer' && (
              <p className="muted">
                La recepción en la pantalla de cocina queda pendiente hasta que una pantalla de
                cocina autorizada reciba y confirme la comanda.
              </p>
            )}
            <Accordion
              title="Agrupación y seguimiento"
              summary="Varias estaciones pueden compartir la misma impresora"
            >
              <Field label="Agrupación de comandas">
                <Select<Station['grouping']>
                  value={selected.grouping}
                  options={[
                    { value: 'inherit', label: 'Usar configuración del establecimiento' },
                    { value: 'independent', label: 'Ticket independiente' },
                    { value: 'unified', label: 'Unificar con estaciones de la misma impresora' },
                  ]}
                  onChange={(grouping) => setSelected({ ...selected, grouping })}
                />
              </Field>
              <Field label="Seguimiento de la estación">
                <Select<Station['tracking']>
                  value={selected.tracking}
                  options={[
                    { value: 'inherit', label: 'Usar configuración del establecimiento' },
                    { value: 'item', label: 'Por producto' },
                    { value: 'order', label: 'Por pedido' },
                  ]}
                  onChange={(tracking) => setSelected({ ...selected, tracking })}
                />
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
              {/* Etapa 9F (decisión D7): colores de las tarjetas de Preparación. */}
              <p className="muted text-sm">
                En Preparación la tarjeta se pone ámbar desde la advertencia (o al 75 % del objetivo
                si la advertencia no es menor que el objetivo) y roja al pasar el objetivo.
              </p>
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
