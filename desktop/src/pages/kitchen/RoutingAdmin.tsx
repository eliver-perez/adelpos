import { useState } from 'react';
import { Field, Form, Notice } from '../../components/Forms';
import { Check, ReferenceField } from '../../components/catalog/EditorFields';
import { useLiveResource } from '../../hooks/useLiveResource';
import type { ExecuteKitchen, KitchenRoute, Station } from '../../services/kitchen';
import type { CoverageWarning, KitchenCoverage } from '../../services/kitchenCoverage';
import { kitchenCoverageCapability, serverHas } from '../../services/serverCapabilities';
import { Select } from '../../components/Select';
import { UnroutedProducts } from './UnroutedProducts';

export function RoutingAdmin({
  can,
  execute,
  busy,
}: {
  can: (key: string) => boolean;
  execute: ExecuteKitchen;
  busy: boolean;
}) {
  // Etapa K1.1 (§4.3): «Productos sin ruta» y la lista «Sin ruta» leen la cobertura de Go, que pide ver las
  // estaciones (kitchen.stations.read) y un servidor con kitchen-coverage-v1.
  const showCoverage = serverHas(kitchenCoverageCapability) && can('kitchen.stations.read');
  const coverage = useLiveResource<KitchenCoverage>('kitchen_coverage', { enabled: showCoverage });
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
  // El aviso de una ruta a una estación desactivada abre esa ruta en el editor.
  function editWarningRoute(warning: CoverageWarning) {
    const route = routes.data?.find((entry) =>
      warning.product_id
        ? entry.product_id === warning.product_id
        : entry.category_id === warning.category_id,
    );
    if (route) {
      setSelected(route);
      setKind(route.product_id ? 'product' : 'category');
    }
  }
  return (
    <div className="form-stack">
      {showCoverage && <Notice>{coverage.error}</Notice>}
      {showCoverage && coverage.data && (
        <UnroutedProducts
          coverage={coverage.data}
          execute={execute}
          busy={busy}
          onReload={coverage.reload}
          onEditRoute={editWarningRoute}
        />
      )}
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
                  <Select<typeof kind>
                    value={kind}
                    options={[
                      { value: 'category', label: 'Categoría' },
                      { value: 'product', label: 'Producto específico' },
                    ]}
                    onChange={(next) => {
                      setKind(next);
                      setSelected({ ...selected, product_id: null, category_id: null });
                    }}
                  />
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
                <Select<KitchenRoute['mode']>
                  value={selected.mode}
                  options={[
                    { value: 'prepare', label: 'Enviar a estaciones seleccionadas' },
                    { value: 'none', label: 'No requiere preparación' },
                  ]}
                  onChange={(mode) => setSelected({ ...selected, mode, station_ids: [] })}
                />
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
    </div>
  );
}
