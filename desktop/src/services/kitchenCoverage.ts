import type { Station } from './kitchen';

// Etapa K1.1 (§4, K11-D5): lo que recibe cada estación según Go (GET kitchen/coverage, capacidad
// kitchen-coverage-v1). Go resuelve cada producto activo con la misma función que usa el envío; aquí solo se
// agrupa y se dice en lenguaje sencillo lo que Go ya resolvió. React no decide rutas.

// El ajuste «Productos sin ruta» (PUT kitchen/unrouted y la cobertura): sin estación, «No necesitan preparación»;
// con una estación activa, van a ella. Con la estación del ajuste desactivada, Go dice effective: 'none'.
export interface UnroutedSetting {
  station_id: string | null;
  station_name: string | null;
  station_active: boolean | null;
  effective: 'none' | 'station';
}

export interface CoverageProduct {
  id: string;
  name: string;
  category_id: string;
  category_name: string;
}

export interface CoverageStation {
  id: string;
  name: string;
  active: boolean;
  reception: Station['reception'];
  printer: { id: string; name: string; active: boolean } | null;
  // Categorías cuya ruta llega a esta estación, con cuántos de sus productos activos llegan de verdad.
  categories: { id: string; name: string; products: number }[];
  // Excepciones por producto que llegan a esta estación.
  products: CoverageProduct[];
  // Es la estación del ajuste «Productos sin ruta» y está activa.
  receives_unrouted: boolean;
  // Pantallas de cocina que la muestran.
  display_ids: string[];
}

export interface UnroutedGroup {
  category_id: string;
  category_name: string;
  // Toda la categoría está sin ruta.
  whole_category: boolean;
  products: { id: string; name: string; reason: 'no_route' | 'inactive_station' }[];
}

// Una categoría activa del catálogo con su ruta de categoría, si tiene (para crearla o cambiarla en el asistente).
export interface CoverageCategory {
  id: string;
  name: string;
  route_id: string | null;
  route_revision: number | null;
  route_mode: 'prepare' | 'none' | null;
  station_ids: string[];
}

export interface CoverageDisplay {
  device_id: string;
  name: string;
  all_stations: boolean;
  // Estaciones que muestra (ya filtradas por Go a las activas que reciben en pantalla).
  station_ids: string[];
}

// Códigos de los avisos (lista cerrada del contrato). El texto llega de Go listo para la pantalla.
export type CoverageWarningCode =
  | 'display_without_stations'
  | 'display_no_screen_stations'
  | 'station_without_routes'
  | 'route_station_inactive'
  | 'unrouted_station_inactive'
  | 'station_printer_inactive';

export interface CoverageWarning {
  code: CoverageWarningCode | string;
  message: string;
  device_id: string | null;
  station_id: string | null;
  category_id: string | null;
  product_id: string | null;
}

export interface KitchenCoverage {
  unrouted: UnroutedSetting;
  stations: CoverageStation[];
  no_preparation: {
    categories: { id: string; name: string }[];
    products: CoverageProduct[];
  };
  unrouted_items: UnroutedGroup[];
  categories: CoverageCategory[];
  displays: CoverageDisplay[];
  warnings: CoverageWarning[];
}

// Una estación recibe comandas en la pantalla de cocina con «Solo pantalla» o «Impresora y pantalla».
export function receivesOnScreen(station: { reception: Station['reception'] }) {
  return station.reception === 'kds' || station.reception === 'both';
}

export const receptionLabels: Record<Station['reception'], string> = {
  printer: 'Solo impresora',
  kds: 'Solo pantalla de cocina',
  both: 'Impresora y pantalla de cocina',
};

// Las estaciones que muestra una pantalla. Una pantalla emparejada dice las suyas en la cobertura (Go ya las
// filtró); una que no aparece (revocada) no muestra ninguna. Una pantalla recién aprobada todavía no tiene
// dispositivo (se crea cuando la tableta reclama su credencial) y empieza mostrando todas las estaciones activas que
// reciben en pantalla, como hace Go (kitchen_all_stations es verdadero por omisión).
export function displayStationIDs(coverage: KitchenCoverage, deviceID: string | null): string[] {
  if (deviceID) {
    return coverage.displays.find((entry) => entry.device_id === deviceID)?.station_ids ?? [];
  }
  return coverage.stations
    .filter((station) => station.active && receivesOnScreen(station))
    .map((station) => station.id);
}

export function stationsByID(coverage: KitchenCoverage, ids: string[]) {
  return ids
    .map((id) => coverage.stations.find((station) => station.id === id))
    .filter((station): station is CoverageStation => station !== undefined);
}

// Nombres de lo que llega a unas estaciones: las categorías y, después, los productos con su ruta propia, sin repetir.
export function receivedNames(coverage: KitchenCoverage, stationIDs: string[]) {
  const names: string[] = [];
  for (const station of stationsByID(coverage, stationIDs)) {
    for (const category of station.categories) {
      if (!names.includes(category.name)) {
        names.push(category.name);
      }
    }
  }
  for (const station of stationsByID(coverage, stationIDs)) {
    for (const product of station.products) {
      if (!names.includes(product.name)) {
        names.push(product.name);
      }
    }
  }
  return names;
}

// Lo que no va a ninguna estación por una ruta, en palabras: una categoría completa por su nombre y, si solo son
// algunos de sus productos, «Bebidas: Agua, Jugo». Lo que tenía ruta a una estación desactivada lo dice.
export function unroutedNames(coverage: KitchenCoverage) {
  return coverage.unrouted_items.map((group) => {
    const inactive = group.products.filter((product) => product.reason === 'inactive_station');
    if (group.whole_category) {
      return inactive.length > 0 && inactive.length === group.products.length
        ? `${group.category_name} (su estación está desactivada)`
        : group.category_name;
    }
    const products = group.products.map((product) =>
      product.reason === 'inactive_station'
        ? `${product.name} (su estación está desactivada)`
        : product.name,
    );
    return `${group.category_name}: ${products.join(', ')}`;
  });
}

// Lo que dice el ajuste «Productos sin ruta» de lo que no tiene ruta.
export function unroutedOutcome(unrouted: UnroutedSetting) {
  if (unrouted.effective === 'station' && unrouted.station_name) {
    return `van a ${unrouted.station_name}`;
  }
  return 'no necesitan preparación';
}

// «Sin ruta: Refrescos, Extras (no necesitan preparación).» o «(van a Cocina).»; vacío si todo tiene ruta.
export function unroutedSummary(coverage: KitchenCoverage) {
  const names = unroutedNames(coverage);
  if (names.length === 0) {
    return '';
  }
  return `Sin ruta: ${names.join(', ')} (${unroutedOutcome(coverage.unrouted)}).`;
}

// Lo que no tiene ruta, por nombre: la categoría completa o, si solo son algunos de sus productos, esos productos.
export function unroutedLabels(coverage: KitchenCoverage) {
  const names: string[] = [];
  for (const group of coverage.unrouted_items) {
    if (group.whole_category) {
      names.push(group.category_name);
    } else {
      names.push(...group.products.map((product) => product.name));
    }
  }
  return unique(names);
}

function unique(names: string[]) {
  return names.filter((name, index) => names.indexOf(name) === index);
}

// Resumen del asistente (§4.1, paso 3): «Esta pantalla recibirá: Desayunos, Hamburguesas.» y «No recibirá:
// Refrescos, Extras (no necesitan preparación).». Solo junta lo que Go resolvió por estación: lo que llega a las
// estaciones de la pantalla, lo que no necesita preparación y lo que va a otra estación.
export function displaySummary(coverage: KitchenCoverage, stationIDs: string[]) {
  const shown = stationsByID(coverage, stationIDs);
  const unrouted = unroutedLabels(coverage);
  const unroutedHere = shown.some((station) => station.receives_unrouted);
  const received = unique([
    ...receivedNames(coverage, stationIDs),
    ...(unroutedHere ? unrouted : []),
  ]);
  // Lo marcado «Sin preparación» y, si el ajuste no los manda a una estación, los productos sin ruta.
  const withoutPreparation = unique([
    ...(coverage.unrouted.effective === 'none' ? unrouted : []),
    ...coverage.no_preparation.categories.map((category) => category.name),
    ...coverage.no_preparation.products.map((product) => product.name),
  ]).filter((name) => !received.includes(name));
  // Lo que llega a otra estación activa que esta pantalla no muestra, con el nombre de esa estación.
  const elsewhere = new Map<string, string[]>();
  for (const station of coverage.stations) {
    if (!station.active || stationIDs.includes(station.id)) {
      continue;
    }
    const names = [
      ...station.categories.map((category) => category.name),
      ...station.products.map((product) => product.name),
      ...(station.receives_unrouted ? unrouted : []),
    ];
    for (const name of unique(names)) {
      if (received.includes(name) || withoutPreparation.includes(name)) {
        continue;
      }
      elsewhere.set(name, [...(elsewhere.get(name) ?? []), station.name]);
    }
  }
  const missing: string[] = [];
  if (withoutPreparation.length > 0) {
    missing.push(`${withoutPreparation.join(', ')} (no necesitan preparación)`);
  }
  for (const [name, stations] of elsewhere) {
    missing.push(`${name} (va a ${stations.join(' y ')})`);
  }
  return {
    receives:
      received.length > 0
        ? `Esta pantalla recibirá: ${received.join(', ')}.`
        : 'Esta pantalla no recibirá ningún producto.',
    misses: missing.length > 0 ? `No recibirá: ${missing.join(', ')}.` : '',
  };
}

// Avisos de una pantalla para su diagnóstico (§4.2): los suyos (device_id) y los de las estaciones que muestra.
export function displayWarnings(
  coverage: KitchenCoverage,
  deviceID: string | null,
  stationIDs: string[],
) {
  return coverage.warnings.filter((warning) => {
    if (warning.device_id) {
      return warning.device_id === deviceID;
    }
    return warning.station_id !== null && stationIDs.includes(warning.station_id);
  });
}

// Avisos de Enrutamiento (§4.3): rutas a estaciones desactivadas y el ajuste con su estación desactivada.
export function routingWarnings(coverage: KitchenCoverage) {
  return coverage.warnings.filter(
    (warning) =>
      warning.code === 'route_station_inactive' || warning.code === 'unrouted_station_inactive',
  );
}

// Qué pasa hoy con una categoría, para las casillas del asistente: «ya llega a Cocina», «va a Barra», «no necesita
// preparación» o «sin ruta». La lectura es la ruta que Go informó; no se resuelve nada aquí.
export function categoryRouteText(
  coverage: KitchenCoverage,
  category: CoverageCategory,
  stationIDs: string[],
) {
  if (category.route_mode === 'none') {
    return 'no necesita preparación';
  }
  if (category.route_mode !== 'prepare' || category.station_ids.length === 0) {
    return 'sin ruta';
  }
  const names = category.station_ids.map((id) => {
    const station = coverage.stations.find((entry) => entry.id === id);
    if (!station) {
      return 'otra estación';
    }
    return station.active ? station.name : `${station.name} (desactivada)`;
  });
  const arrives = category.station_ids.some((id) => stationIDs.includes(id));
  return `${arrives ? 'ya llega a' : 'va a'} ${names.join(' y ')}`;
}

// La categoría ya llega a alguna de las estaciones de la pantalla (sale marcada y no se desmarca en el asistente:
// borrar una ruta no existe; se cambia en Enrutamiento).
export function categoryArrives(category: CoverageCategory, stationIDs: string[]) {
  return (
    category.route_mode === 'prepare' && category.station_ids.some((id) => stationIDs.includes(id))
  );
}

// El aviso al marcar una categoría que hoy va a otro lado (K11-D6): su ruta cambia a la estación de la pantalla.
export function categoryChangeWarning(coverage: KitchenCoverage, category: CoverageCategory) {
  if (category.route_mode === 'none') {
    return `${category.name} dejará de marcarse como «No requiere preparación».`;
  }
  if (category.route_mode !== 'prepare' || category.station_ids.length === 0) {
    return '';
  }
  const names = category.station_ids.map(
    (id) => coverage.stations.find((station) => station.id === id)?.name ?? 'otra estación',
  );
  return `${category.name} dejará de ir a ${names.join(' y ')}.`;
}
