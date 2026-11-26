import type { Station } from '../services/kitchen';
import type { CoverageStation, KitchenCoverage } from '../services/kitchenCoverage';

// Etapa K1.1: coberturas de Go simuladas para Vitest, con la configuración de la prueba del propietario (§1 de la
// especificación): la estación «Cocina» con «Solo pantalla de cocina», Desayunos y Hamburguesas a Cocina, Refrescos y
// Extras sin ruta y una pantalla de cocina emparejada que muestra todas las estaciones.

export const coverageIDs = {
  cocina: '00000000-0000-4000-8000-00000000c001',
  barra: '00000000-0000-4000-8000-00000000c002',
  plancha: '00000000-0000-4000-8000-00000000c003',
  desayunos: '00000000-0000-4000-8000-00000000d001',
  hamburguesas: '00000000-0000-4000-8000-00000000d002',
  refrescos: '00000000-0000-4000-8000-00000000d003',
  extras: '00000000-0000-4000-8000-00000000d004',
  bebidas: '00000000-0000-4000-8000-00000000d005',
  postres: '00000000-0000-4000-8000-00000000d006',
  display: '00000000-0000-4000-8000-0000000000e1',
  printer: '00000000-0000-4000-8000-0000000000f1',
};

export function coverageStation(overrides: Partial<CoverageStation>): CoverageStation {
  return {
    id: coverageIDs.cocina,
    name: 'Cocina',
    active: true,
    reception: 'kds',
    printer: null,
    categories: [],
    products: [],
    receives_unrouted: false,
    display_ids: [],
    ...overrides,
  };
}

// La configuración del propietario: lo que tiene ruta llega a Cocina; Refrescos y Extras no tienen ruta.
export function ownerCoverage(): KitchenCoverage {
  return {
    unrouted: { station_id: null, station_name: null, station_active: null, effective: 'none' },
    stations: [
      coverageStation({
        categories: [
          { id: coverageIDs.desayunos, name: 'Desayunos', products: 12 },
          { id: coverageIDs.hamburguesas, name: 'Hamburguesas', products: 5 },
        ],
        display_ids: [coverageIDs.display],
      }),
    ],
    no_preparation: { categories: [], products: [] },
    unrouted_items: [
      {
        category_id: coverageIDs.refrescos,
        category_name: 'Refrescos',
        whole_category: true,
        products: [{ id: 'p-coca', name: 'Coca Cola 600ml', reason: 'no_route' }],
      },
      {
        category_id: coverageIDs.extras,
        category_name: 'Extras',
        whole_category: true,
        products: [{ id: 'p-papas', name: 'Papas a la francesa', reason: 'no_route' }],
      },
    ],
    categories: [
      {
        id: coverageIDs.desayunos,
        name: 'Desayunos',
        route_id: 'r-desayunos',
        route_revision: 2,
        route_mode: 'prepare',
        station_ids: [coverageIDs.cocina],
      },
      {
        id: coverageIDs.hamburguesas,
        name: 'Hamburguesas',
        route_id: 'r-hamburguesas',
        route_revision: 1,
        route_mode: 'prepare',
        station_ids: [coverageIDs.cocina],
      },
      {
        id: coverageIDs.refrescos,
        name: 'Refrescos',
        route_id: null,
        route_revision: null,
        route_mode: null,
        station_ids: [],
      },
      {
        id: coverageIDs.extras,
        name: 'Extras',
        route_id: null,
        route_revision: null,
        route_mode: null,
        station_ids: [],
      },
    ],
    displays: [
      {
        device_id: coverageIDs.display,
        name: 'Tableta de la cocina',
        all_stations: true,
        station_ids: [coverageIDs.cocina],
      },
    ],
    warnings: [],
  };
}

// Un restaurante sin estaciones que reciban en pantalla: solo «Barra», que imprime, y cuatro categorías sin ruta.
export function emptyCoverage(): KitchenCoverage {
  return {
    unrouted: { station_id: null, station_name: null, station_active: null, effective: 'none' },
    stations: [
      coverageStation({
        id: coverageIDs.barra,
        name: 'Barra',
        reception: 'printer',
        printer: { id: coverageIDs.printer, name: 'Impresora de barra', active: true },
      }),
    ],
    no_preparation: { categories: [], products: [] },
    unrouted_items: [],
    categories: ['desayunos', 'hamburguesas', 'refrescos', 'extras'].map((key) => ({
      id: coverageIDs[key as keyof typeof coverageIDs],
      name: key.charAt(0).toUpperCase() + key.slice(1),
      route_id: null,
      route_revision: null,
      route_mode: null,
      station_ids: [],
    })),
    displays: [],
    warnings: [],
  };
}

// La estación completa de GET kitchen/stations que corresponde a una de la cobertura (para PATCH con su revisión).
export function fullStation(station: CoverageStation, revision = 3): Station {
  return {
    id: station.id,
    name: station.name,
    description: '',
    active: station.active,
    sort_order: 0,
    reception: station.reception,
    printer_id: station.printer?.id ?? null,
    target_minutes: 15,
    warning_minutes: 15,
    overdue_minutes: 25,
    grouping: 'inherit',
    tracking: 'inherit',
    revision,
  };
}
