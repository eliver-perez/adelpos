import { useState } from 'react';
import { Field, Form, Notice } from '../components/Forms';
import { MultiSelect } from '../components/Select';
import { Check } from '../components/catalog/EditorFields';
import { useResource } from '../hooks/useResource';
import { api } from '../services/api';
import type { UserStations } from '../services/kitchen';
import type { User } from '../services/models';

// «Estaciones que puede atender» (etapa 9F, §5.2 y decisión D6): todas por omisión o una selección. En
// Cocina cada terminal muestra solo sus estaciones y, dentro de ellas, las que el usuario atiende. Go
// valida, audita y rechaza una edición concurrente; quien tiene «Operar todas las estaciones del
// establecimiento» siempre atiende todas y la pantalla lo dice.
export function UserKitchenStations({ user, editable }: { user: User; editable: boolean }) {
  const resource = useResource<UserStations>('user_kitchen_stations', { id: user.id });
  // Borrador local; sin cambios se muestra lo guardado.
  const [draft, setDraft] = useState<{ all: boolean; stationIDs: string[] } | null>(null);
  const saved = resource.data;
  if (!saved) {
    return <Notice>{resource.error}</Notice>;
  }
  const value = draft ?? { all: saved.all_stations, stationIDs: saved.station_ids };
  const options = saved.stations.map((station) => ({
    value: station.id,
    label: station.active ? station.name : `${station.name} (inactiva)`,
  }));
  return (
    <>
      <Notice>{resource.error}</Notice>
      <Form
        disabled={!editable}
        label="Guardar estaciones"
        successMessage="Estaciones del usuario guardadas."
        onSave={async () => {
          await api('save_user_kitchen_stations', {
            id: user.id,
            body: {
              all_stations: value.all,
              station_ids: value.all ? [] : value.stationIDs,
              expected_all_stations: saved.all_stations,
              expected_station_ids: saved.station_ids,
            },
          });
          setDraft(null);
          await resource.reload();
        }}
      >
        {saved.all_by_permission && (
          <p className="user-stations-permission" role="status">
            Tiene el permiso «Operar todas las estaciones del establecimiento»: siempre atiende
            todas las estaciones, aunque aquí se elija una selección.
          </p>
        )}
        <Check
          label="Puede atender todas las estaciones"
          value={value.all}
          onChange={(all) => setDraft({ ...value, all })}
        />
        {!value.all && (
          <Field
            label="Estaciones que puede atender"
            hint="Elige al menos una. Si no tiene ninguna en común con la terminal, Cocina se lo avisa."
          >
            <MultiSelect
              required
              value={value.stationIDs}
              placeholder="Elige las estaciones"
              options={options}
              onChange={(stationIDs) => setDraft({ ...value, stationIDs })}
            />
          </Field>
        )}
        <p className="muted text-sm">
          En Cocina cada terminal muestra solo sus estaciones y, dentro de ellas, las que este
          usuario puede atender.
        </p>
      </Form>
    </>
  );
}
