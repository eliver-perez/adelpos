import { useState } from 'react';
import { Field, Form, Notice } from '../../components/Forms';
import { SegmentedControl } from '../../components/SegmentedControl';
import { Select } from '../../components/Select';
import { APIError } from '../../services/api';
import type { ExecuteKitchen } from '../../services/kitchen';
import type {
  CoverageWarning,
  KitchenCoverage,
  UnroutedSetting,
} from '../../services/kitchenCoverage';
import { routingWarnings, unroutedNames, unroutedOutcome } from '../../services/kitchenCoverage';

// Etapa K1.1 (§2 y §4.3, K11-D3): en Cocina › Enrutamiento, el ajuste «Productos sin ruta» (qué pasa con lo que no
// tiene ruta propia ni de su categoría), la lista de lo que no va a ninguna estación y los avisos de rutas a
// estaciones desactivadas. Todo sale de la cobertura de Go (GET kitchen/coverage); solo con kitchen-coverage-v1.

type Choice = 'none' | 'station';

export function UnroutedProducts({
  coverage,
  execute,
  busy,
  onReload,
  onEditRoute,
}: {
  coverage: KitchenCoverage;
  execute: ExecuteKitchen;
  busy: boolean;
  onReload: () => Promise<void>;
  // Abre la ruta del aviso en el editor de Enrutamiento.
  onEditRoute: (warning: CoverageWarning) => void;
}) {
  const saved = coverage.unrouted;
  // Borrador local: null mientras no se toque (se muestra lo que dice Go).
  const [draft, setDraft] = useState<{ choice: Choice; station: string } | null>(null);
  const [conflict, setConflict] = useState(false);
  const choice: Choice = draft?.choice ?? (saved.station_id ? 'station' : 'none');
  const station = draft?.station ?? saved.station_id ?? '';
  const names = unroutedNames(coverage);
  const warnings = routingWarnings(coverage);
  // Solo las estaciones activas se pueden elegir; la guardada, si se desactivó, se ve sin poder elegirla otra vez.
  const options = coverage.stations
    .filter((entry) => entry.active || entry.id === saved.station_id)
    .map((entry) => ({
      value: entry.id,
      label: entry.active ? entry.name : `${entry.name} (desactivada)`,
      disabled: !entry.active,
    }));

  async function save() {
    setConflict(false);
    try {
      const result = await execute<UnroutedSetting>('kitchen_save_unrouted', {
        body: {
          station_id: choice === 'station' ? station : null,
          expected_station_id: saved.station_id,
        },
      });
      setDraft(null);
      await onReload();
      return result;
    } catch (problem) {
      // Otra persona lo cambió mientras se editaba: se vuelve a leer y se avisa en su lugar (el controlador común ya
      // mostró el mensaje de Go).
      if (problem instanceof APIError && problem.code === 'KITCHEN_UNROUTED_CHANGED') {
        setConflict(true);
        setDraft(null);
        await onReload();
      }
      throw problem;
    }
  }

  return (
    <section className="panel form-stack" aria-labelledby="unrouted-products-title">
      <h2 id="unrouted-products-title">Productos sin ruta</h2>
      <p className="muted">
        Un producto sin ruta propia ni de su categoría nunca detiene el pedido: lo demás llega a su
        estación igual. Aquí eliges qué pasa con él.
      </p>
      <Notice>
        {conflict &&
          'Alguien más cambió este ajuste mientras lo editabas. Ya se muestra lo guardado; revísalo y vuelve a guardar.'}
      </Notice>
      <Form
        disabled={busy}
        label="Guardar productos sin ruta"
        successMessage="Productos sin ruta guardado."
        onSave={async () => {
          await save();
        }}
      >
        <div className="field">
          <span>Qué pasa con los productos sin ruta</span>
          <SegmentedControl<Choice>
            label="Qué pasa con los productos sin ruta"
            value={choice}
            options={[
              { value: 'none', label: 'No necesitan preparación' },
              { value: 'station', label: 'Enviar a una estación' },
            ]}
            onChange={(next) => setDraft({ choice: next, station })}
          />
          <small className="muted">
            {choice === 'none'
              ? 'No van a ninguna estación. Es lo normal para refrescos y extras que se sirven sin preparar.'
              : 'Todo lo que no tenga ruta llega a esta estación.'}
          </small>
        </div>
        {choice === 'station' && (
          <Field label="Estación para los productos sin ruta">
            <Select
              required
              value={station}
              placeholder="Elige la estación"
              options={options}
              onChange={(next) => setDraft({ choice, station: next })}
            />
          </Field>
        )}
      </Form>
      <section className="kitchen-unrouted-list" aria-labelledby="unrouted-list-title">
        <h3 id="unrouted-list-title">Lo que no va a ninguna estación</h3>
        {names.length > 0 ? (
          <p>
            <strong>Sin ruta:</strong> {names.join(', ')} ({unroutedOutcome(saved)}).
          </p>
        ) : (
          <p className="muted">Todos los productos tienen una ruta.</p>
        )}
      </section>
      {warnings.length > 0 && (
        <ul className="kitchen-coverage-warnings" aria-label="Avisos de enrutamiento">
          {warnings.map((warning, index) => (
            <li key={`${warning.code}-${index}`}>
              <span>{warning.message}</span>
              {warning.code === 'route_station_inactive' &&
                (warning.category_id || warning.product_id) && (
                  <button className="secondary-button" onClick={() => onEditRoute(warning)}>
                    Cambiar la ruta
                  </button>
                )}
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
