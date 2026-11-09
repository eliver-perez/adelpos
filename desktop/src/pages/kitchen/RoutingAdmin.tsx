import { useState } from 'react';
import { Field, Form, Notice } from '../../components/Forms';
import { Check, ReferenceField } from '../../components/catalog/EditorFields';
import { useLiveResource } from '../../hooks/useLiveResource';
import type { ExecuteKitchen, KitchenRoute, Station } from '../../services/kitchen';

export function RoutingAdmin({ execute, busy }: { execute: ExecuteKitchen; busy: boolean }) {
  const routes = useLiveResource<KitchenRoute[]>('kitchen_routes');
  const stations = useLiveResource<Station[]>('kitchen_stations');
  const targets = useLiveResource<{
    products: { id: string; name: string }[];
    categories: { id: string; name: string }[];
  }>('kitchen_targets');
  const [selected, setSelected] = useState<KitchenRoute | null>(null);
  const [kind, setKind] = useState<'product' | 'category'>('category');
  const name = (route: KitchenRoute) =>
    route.product_id
      ? targets.data?.products.find((item) => item.id === route.product_id)?.name
      : targets.data?.categories.find((item) => item.id === route.category_id)?.name;
  return (
    <div className="kitchen-editor-layout">
      <section className="panel form-stack">
        <div className="section-heading">
          <h2>Enrutamiento</h2>
          <button
            className="primary-button"
            onClick={() => {
              setSelected({
                id: '',
                product_id: null,
                category_id: null,
                mode: 'prepare',
                station_ids: [],
                revision: 0,
              });
              setKind('category');
            }}
          >
            Nueva ruta
          </button>
        </div>
        <p className="muted">
          La ruta específica del producto prevalece sobre su categoría. Los componentes de combo
          conservan su preparación sin duplicar la unidad comercial.
        </p>
        <Notice>{routes.error || stations.error || targets.error}</Notice>
        {routes.data?.map((route) => (
          <button
            className="kitchen-list-button"
            key={route.id}
            onClick={() => {
              setSelected(route);
              setKind(route.product_id ? 'product' : 'category');
            }}
          >
            <strong>
              {route.product_id ? 'Producto' : 'Categoría'} ·{' '}
              {name(route) ?? 'Referencia conservada'}
            </strong>
            <span>
              {route.mode === 'none'
                ? 'No requiere preparación'
                : route.station_ids
                    .map(
                      (id) =>
                        stations.data?.find((station) => station.id === id)?.name ?? 'Estación',
                    )
                    .join(' · ')}
            </span>
          </button>
        ))}
      </section>
      {selected && (
        <section className="panel form-stack">
          <h2>{selected.id ? `Ruta de ${name(selected)}` : 'Nueva ruta de preparación'}</h2>
          <Form
            disabled={busy}
            label="Guardar ruta"
            successMessage="Ruta de preparación guardada."
            onSave={async () =>
              setSelected(
                await execute<KitchenRoute>(
                  selected.id ? 'kitchen_save_route' : 'kitchen_create_route',
                  { id: selected.id || undefined, body: { ...selected } },
                ),
              )
            }
          >
            {!selected.id && (
              <Field label="Asignar ruta a">
                <select
                  value={kind}
                  onChange={(event) => {
                    setKind(event.target.value as typeof kind);
                    setSelected({ ...selected, product_id: null, category_id: null });
                  }}
                >
                  <option value="category">Categoría</option>
                  <option value="product">Producto específico</option>
                </select>
              </Field>
            )}
            {!selected.id && (
              <ReferenceField
                label={kind === 'product' ? 'Producto de la ruta' : 'Categoría de la ruta'}
                value={selected.product_id ?? selected.category_id}
                options={
                  kind === 'product'
                    ? (targets.data?.products ?? [])
                    : (targets.data?.categories ?? [])
                }
                onChange={(id) =>
                  setSelected({
                    ...selected,
                    product_id: kind === 'product' ? id : null,
                    category_id: kind === 'category' ? id : null,
                  })
                }
              />
            )}
            <Field label="Necesidad de preparación">
              <select
                value={selected.mode}
                onChange={(event) =>
                  setSelected({
                    ...selected,
                    mode: event.target.value as KitchenRoute['mode'],
                    station_ids: [],
                  })
                }
              >
                <option value="prepare">Enviar a estaciones seleccionadas</option>
                <option value="none">No requiere preparación</option>
              </select>
            </Field>
            {selected.mode === 'prepare' &&
              stations.data?.map((station) => (
                <Check
                  key={station.id}
                  label={`${station.name}${station.active ? '' : ' · inactiva'}`}
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
            <p className="muted text-sm">
              Los pedidos liberados conservan el enrutamiento original. Cambiar esta configuración
              afecta liberaciones posteriores.
            </p>
          </Form>
        </section>
      )}
    </div>
  );
}
