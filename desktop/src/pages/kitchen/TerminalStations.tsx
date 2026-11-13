import { useState } from 'react';
import { unlockKitchenSound } from '../../services/kitchenSound';
import { Accordion } from '../../components/Accordion';
import { Field, Form, Notice } from '../../components/Forms';
import { MultiSelect } from '../../components/Select';
import { Check, ReferenceField } from '../../components/catalog/EditorFields';
import { useLiveResource } from '../../hooks/useLiveResource';
import { terminalHomes } from '../../navigation/sections';
import type { DeviceScope, ExecuteKitchen, Station } from '../../services/kitchen';

// «Terminales de cocina» (etapa 9F, §5.1 y decisión D6): por terminal, las estaciones que muestra
// (todas, la opción predeterminada, o una selección) y el aviso sonoro al llegar una comanda nueva. Lo
// que se ve en Cocina es la intersección con las estaciones que atiende cada usuario (Personal ›
// Usuarios). Vive en Cocina › Estaciones, junto a las estaciones, con kitchen.stations.manage; la
// pantalla de inicio de la terminal sigue en Sistema › Terminales.
export function TerminalStations({
  stations,
  execute,
  busy,
}: {
  stations: Station[];
  execute: ExecuteKitchen;
  busy: boolean;
}) {
  const resource = useLiveResource<DeviceScope[]>('kitchen_access');
  // Borrador de la terminal elegida y la configuración que se leyó (la esperada por Go al guardar).
  const [draft, setDraft] = useState<DeviceScope | null>(null);
  const [loaded, setLoaded] = useState<DeviceScope | null>(null);
  const current = resource.data?.find((device) => device.device_id === draft?.device_id);
  const stale = !!current && !!loaded && !sameConfiguration(current, loaded);
  const options = stations.map((station) => ({
    value: station.id,
    label: station.active ? station.name : `${station.name} (inactiva)`,
  }));

  function choose(id: string) {
    const device = resource.data?.find((entry) => entry.device_id === id) ?? null;
    setDraft(device);
    setLoaded(device);
  }

  return (
    <Accordion
      title="Terminales de cocina"
      summary="Estaciones que muestra cada terminal y aviso sonoro"
    >
      <Notice>{resource.error}</Notice>
      <ReferenceField
        optional
        label="Terminal"
        value={draft?.device_id ?? ''}
        options={(resource.data ?? []).map((device) => ({
          id: device.device_id,
          name: device.device_name,
        }))}
        onChange={choose}
      />
      {draft && (
        <>
          <p className="muted text-sm">{homeHint(draft.home_screen)}</p>
          <Notice>
            {stale &&
              'La terminal cambió en otra pantalla. Tus cambios se conservan; revisa la configuración vigente antes de guardar.'}
          </Notice>
          {stale && current && (
            <button className="secondary-button" onClick={() => choose(current.device_id)}>
              Usar la configuración vigente
            </button>
          )}
          <Form
            disabled={busy || stale}
            label="Guardar terminal de cocina"
            successMessage={`Terminal ${draft.device_name} guardada.`}
            onSave={async () => {
              const saved = await execute<Omit<DeviceScope, 'device_name' | 'home_screen'>>(
                'kitchen_save_access',
                {
                  body: {
                    device_id: draft.device_id,
                    all_stations: draft.all_stations,
                    station_ids: draft.all_stations ? [] : draft.station_ids,
                    sound_alert: draft.sound_alert,
                    expected_all_stations: loaded?.all_stations ?? true,
                    expected_station_ids: loaded?.station_ids ?? [],
                  },
                },
              );
              // La lista se vuelve a leer antes de tomar lo guardado como la configuración vigente, para
              // no confundir la propia escritura con un cambio de otra pantalla.
              await resource.reload();
              const next = { ...draft, ...saved };
              setDraft(next);
              setLoaded(next);
            }}
          >
            <Check
              label="Mostrar todas las estaciones"
              value={draft.all_stations}
              onChange={(all_stations) => setDraft({ ...draft, all_stations })}
            />
            {!draft.all_stations && (
              <Field
                label="Estaciones que muestra esta terminal"
                hint="Elige al menos una. Cada usuario ve solo las que también puede atender."
              >
                <MultiSelect
                  required
                  value={draft.station_ids}
                  placeholder="Elige las estaciones"
                  options={options}
                  onChange={(station_ids) => setDraft({ ...draft, station_ids })}
                />
              </Field>
            )}
            <Check
              label="Aviso sonoro al llegar una comanda nueva"
              value={draft.sound_alert}
              onChange={(sound_alert) => {
                // Activarlo es un toque: el equipo permite preparar el sonido (revisión 9F, parte 3, B2).
                if (sound_alert) {
                  unlockKitchenSound();
                }
                setDraft({ ...draft, sound_alert });
              }}
            />
          </Form>
        </>
      )}
    </Accordion>
  );
}

// Recuerda dónde abre la terminal: una terminal de cocina abre directo en Cocina › Preparación con la
// pantalla inicial «Cocina» (Sistema › Terminales).
function homeHint(home: string) {
  if (home === 'kitchen') {
    return 'Esta terminal abre en Cocina › Preparación.';
  }
  const label = terminalHomes.find((entry) => entry.value === home)?.label ?? 'Mesas';
  return `Esta terminal abre en ${label}. Para que abra directo en Cocina › Preparación, elige «Cocina» como pantalla inicial en Sistema › Terminales.`;
}

function sameConfiguration(first: DeviceScope, second: DeviceScope) {
  return (
    first.all_stations === second.all_stations &&
    first.sound_alert === second.sound_alert &&
    [...first.station_ids].sort().join(',') === [...second.station_ids].sort().join(',')
  );
}
