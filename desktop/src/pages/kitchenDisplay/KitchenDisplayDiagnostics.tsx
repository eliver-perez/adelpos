import { Notice } from '../../components/Forms';
import { useLiveResource } from '../../hooks/useLiveResource';
import { useNavigation } from '../../navigation/NavigationContext';
import type { CoverageWarning, KitchenCoverage } from '../../services/kitchenCoverage';
import {
  displayStationIDs,
  displayWarnings,
  receptionLabels,
  stationsByID,
} from '../../services/kitchenCoverage';
import { countLabel } from '../../services/plural';
import { requestKitchenView } from '../kitchen/kitchenViewRequest';
import type { KitchenView } from '../kitchen/kitchenViewRequest';
import type { AssistantStep } from './KitchenDisplayAssistant';

// Etapa K1.1 (§4.2): en «Ver configuración» de una pantalla de cocina, qué le llega por estación (categorías, con
// cuántos productos, y los productos con ruta propia) y los avisos de la cobertura de Go para esa pantalla y sus
// estaciones, cada uno con un botón que lleva a corregirlo. Solo con kitchen-coverage-v1 y kitchen.stations.read.

// Adónde lleva el botón de cada aviso: al asistente (en el paso que lo corrige) o a la pestaña de Cocina donde se
// corrige, con el permiso que pide esa pestaña.
type Fix =
  | { kind: 'assistant'; step: AssistantStep; label: string }
  | { kind: 'kitchen'; view: KitchenView; permission: string; label: string };

export function warningFix(code: string): Fix | null {
  switch (code) {
    case 'display_without_stations':
    case 'display_no_screen_stations':
      return { kind: 'assistant', step: 1, label: 'Configurar qué recibe' };
    case 'station_without_routes':
      return { kind: 'assistant', step: 2, label: 'Elegir sus categorías' };
    case 'route_station_inactive':
    case 'unrouted_station_inactive':
      return {
        kind: 'kitchen',
        view: 'routing',
        permission: 'kitchen.routing.manage',
        label: 'Ir a Cocina › Enrutamiento',
      };
    case 'station_printer_inactive':
      return {
        kind: 'kitchen',
        view: 'printers',
        permission: 'kitchen.printers.manage',
        label: 'Ir a Cocina › Impresoras',
      };
  }
  return null;
}

export function KitchenDisplayDiagnostics({
  deviceId,
  can,
  onConfigure,
}: {
  deviceId: string;
  can: (key: string) => boolean;
  onConfigure: (step: AssistantStep) => void;
}) {
  const coverage = useLiveResource<KitchenCoverage>('kitchen_coverage');
  const navigation = useNavigation();
  const data = coverage.data;

  function open(warning: CoverageWarning) {
    const fix = warningFix(warning.code);
    if (!fix) {
      return;
    }
    if (fix.kind === 'assistant') {
      onConfigure(fix.step);
      return;
    }
    // La pestaña de Cocina se pide antes de navegar con el único requestScreen.
    requestKitchenView(fix.view);
    void navigation?.requestScreen('kitchen');
  }

  // Un botón solo se ofrece si lleva a algún lado que la persona puede abrir.
  function available(warning: CoverageWarning) {
    const fix = warningFix(warning.code);
    if (!fix) {
      return false;
    }
    if (fix.kind === 'assistant') {
      return true;
    }
    return navigation !== null && can('kitchen.states.read') && can(fix.permission);
  }

  if (!data) {
    return <Notice>{coverage.error}</Notice>;
  }
  const stationIDs = displayStationIDs(data, deviceId);
  const stations = stationsByID(data, stationIDs);
  const warnings = displayWarnings(data, deviceId, stationIDs);
  return (
    <section className="kitchen-diagnostics" aria-labelledby="kitchen-diagnostics-title">
      <h3 id="kitchen-diagnostics-title">Qué recibe esta pantalla</h3>
      <Notice>{coverage.error}</Notice>
      {warnings.length > 0 && (
        <ul className="kitchen-coverage-warnings" aria-label="Avisos de la pantalla">
          {warnings.map((warning, index) => {
            const fix = warningFix(warning.code);
            return (
              <li key={`${warning.code}-${warning.station_id ?? ''}-${index}`}>
                <span>{warning.message}</span>
                {fix && available(warning) && (
                  <button className="secondary-button" onClick={() => open(warning)}>
                    {fix.label}
                  </button>
                )}
              </li>
            );
          })}
        </ul>
      )}
      {stations.length > 0 && (
        <div className="kitchen-diagnostics-stations">
          {stations.map((station) => (
            <article key={station.id} aria-label={`Estación ${station.name}`}>
              <h4>
                {station.name}
                <small className="muted"> · {receptionLabels[station.reception]}</small>
              </h4>
              {station.categories.length > 0 && (
                <p>
                  Categorías:{' '}
                  {station.categories
                    .map(
                      (category) =>
                        `${category.name} (${countLabel(category.products, 'producto', 'productos')})`,
                    )
                    .join(', ')}
                </p>
              )}
              {station.products.length > 0 && (
                <p>
                  Productos:{' '}
                  {station.products
                    .map((product) => `${product.name} (${product.category_name})`)
                    .join(', ')}
                </p>
              )}
              {station.receives_unrouted && <p>También le llegan los productos sin ruta.</p>}
              {station.categories.length === 0 &&
                station.products.length === 0 &&
                !station.receives_unrouted && <p className="muted">No le llega ningún producto.</p>}
            </article>
          ))}
        </div>
      )}
    </section>
  );
}
