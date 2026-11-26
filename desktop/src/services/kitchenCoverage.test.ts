import { describe, expect, it } from 'vitest';
import {
  coverageIDs,
  coverageStation,
  emptyCoverage,
  ownerCoverage,
} from '../testing/kitchenCoverage';
import {
  categoryArrives,
  categoryChangeWarning,
  categoryRouteText,
  displayStationIDs,
  displaySummary,
  displayWarnings,
  routingWarnings,
  unroutedNames,
  unroutedSummary,
} from './kitchenCoverage';

// Etapa K1.1 (§4, K11-D5 y K11-D6): lo que el escritorio dice con la cobertura de Go. Solo agrupa lo que Go resolvió
// por estación; los casos son los de la prueba del propietario.

describe('cobertura de cocina en palabras', () => {
  it('el resumen de la pantalla del propietario: recibe Desayunos y Hamburguesas; Refrescos y Extras no', () => {
    const coverage = ownerCoverage();
    const stations = displayStationIDs(coverage, coverageIDs.display);
    expect(stations).toEqual([coverageIDs.cocina]);
    expect(displaySummary(coverage, stations)).toEqual({
      receives: 'Esta pantalla recibirá: Desayunos, Hamburguesas.',
      misses: 'No recibirá: Refrescos, Extras (no necesitan preparación).',
    });
    expect(unroutedNames(coverage)).toEqual(['Refrescos', 'Extras']);
    expect(unroutedSummary(coverage)).toBe(
      'Sin ruta: Refrescos, Extras (no necesitan preparación).',
    );
  });

  it('con «Enviar a una estación», lo sin ruta llega a esa estación y el resumen lo dice', () => {
    const coverage = ownerCoverage();
    coverage.unrouted = {
      station_id: coverageIDs.cocina,
      station_name: 'Cocina',
      station_active: true,
      effective: 'station',
    };
    coverage.stations[0]!.receives_unrouted = true;
    expect(unroutedSummary(coverage)).toBe('Sin ruta: Refrescos, Extras (van a Cocina).');
    expect(displaySummary(coverage, [coverageIDs.cocina])).toEqual({
      receives: 'Esta pantalla recibirá: Desayunos, Hamburguesas, Refrescos, Extras.',
      misses: '',
    });
  });

  it('lo que va a otra estación que la pantalla no muestra se dice con esa estación', () => {
    const coverage = ownerCoverage();
    coverage.stations.push(
      coverageStation({
        id: coverageIDs.barra,
        name: 'Barra',
        reception: 'printer',
        categories: [{ id: coverageIDs.bebidas, name: 'Bebidas', products: 4 }],
        products: [
          {
            id: 'p-malteada',
            name: 'Malteada',
            category_id: coverageIDs.postres,
            category_name: 'Postres',
          },
        ],
      }),
    );
    coverage.no_preparation.categories.push({ id: coverageIDs.postres, name: 'Postres' });
    expect(displaySummary(coverage, [coverageIDs.cocina]).misses).toBe(
      'No recibirá: Refrescos, Extras, Postres (no necesitan preparación), Bebidas (va a Barra), Malteada (va a Barra).',
    );
  });

  it('una pantalla recién aprobada muestra todas las estaciones activas que reciben en pantalla', () => {
    const coverage = ownerCoverage();
    coverage.stations.push(
      coverageStation({ id: coverageIDs.barra, name: 'Barra', reception: 'printer' }),
      coverageStation({
        id: coverageIDs.plancha,
        name: 'Plancha',
        reception: 'both',
        active: false,
      }),
    );
    expect(displayStationIDs(coverage, null)).toEqual([coverageIDs.cocina]);
    // Una pantalla que no aparece (revocada) no muestra ninguna.
    expect(displayStationIDs(coverage, 'otra')).toEqual([]);
    expect(displayStationIDs(emptyCoverage(), null)).toEqual([]);
    expect(displaySummary(emptyCoverage(), []).receives).toBe(
      'Esta pantalla no recibirá ningún producto.',
    );
  });

  it('solo algunos productos sin ruta se dicen con su categoría, y los de una estación desactivada lo explican', () => {
    const coverage = ownerCoverage();
    coverage.unrouted_items = [
      {
        category_id: coverageIDs.bebidas,
        category_name: 'Bebidas',
        whole_category: false,
        products: [
          { id: 'p-agua', name: 'Agua', reason: 'no_route' },
          { id: 'p-jugo', name: 'Jugo', reason: 'inactive_station' },
        ],
      },
      {
        category_id: coverageIDs.extras,
        category_name: 'Extras',
        whole_category: true,
        products: [{ id: 'p-papas', name: 'Papas', reason: 'inactive_station' }],
      },
    ];
    expect(unroutedNames(coverage)).toEqual([
      'Bebidas: Agua, Jugo (su estación está desactivada)',
      'Extras (su estación está desactivada)',
    ]);
    expect(displaySummary(coverage, [coverageIDs.cocina]).misses).toBe(
      'No recibirá: Agua, Jugo, Extras (no necesitan preparación).',
    );
  });

  it('las casillas del asistente dicen adónde va hoy cada categoría y qué cambia al marcarla', () => {
    const coverage = ownerCoverage();
    coverage.stations.push(
      coverageStation({ id: coverageIDs.barra, name: 'Barra', active: false }),
    );
    const [desayunos, , refrescos] = coverage.categories;
    expect(categoryRouteText(coverage, desayunos!, [coverageIDs.cocina])).toBe('ya llega a Cocina');
    expect(categoryArrives(desayunos!, [coverageIDs.cocina])).toBe(true);
    expect(categoryRouteText(coverage, refrescos!, [coverageIDs.cocina])).toBe('sin ruta');
    expect(categoryChangeWarning(coverage, refrescos!)).toBe('');
    const bebidas = {
      id: coverageIDs.bebidas,
      name: 'Bebidas',
      route_id: 'r-bebidas',
      route_revision: 4,
      route_mode: 'prepare' as const,
      station_ids: [coverageIDs.barra],
    };
    expect(categoryRouteText(coverage, bebidas, [coverageIDs.cocina])).toBe(
      'va a Barra (desactivada)',
    );
    expect(categoryArrives(bebidas, [coverageIDs.cocina])).toBe(false);
    expect(categoryChangeWarning(coverage, bebidas)).toBe('Bebidas dejará de ir a Barra.');
    const postres = { ...bebidas, name: 'Postres', route_mode: 'none' as const, station_ids: [] };
    expect(categoryRouteText(coverage, postres, [coverageIDs.cocina])).toBe(
      'no necesita preparación',
    );
    expect(categoryChangeWarning(coverage, postres)).toBe(
      'Postres dejará de marcarse como «No requiere preparación».',
    );
  });

  it('cada lugar ve sus avisos: la pantalla los suyos y los de sus estaciones; Enrutamiento los de rutas', () => {
    const coverage = ownerCoverage();
    coverage.warnings = [
      {
        code: 'display_without_stations',
        message: 'Esta pantalla no tiene estaciones.',
        device_id: 'otra-pantalla',
        station_id: null,
        category_id: null,
        product_id: null,
      },
      {
        code: 'station_without_routes',
        message: 'La estación Cocina no tiene categorías ni productos asignados.',
        device_id: null,
        station_id: coverageIDs.cocina,
        category_id: null,
        product_id: null,
      },
      {
        code: 'route_station_inactive',
        message: 'Extras va a Barra, que está desactivada.',
        device_id: null,
        station_id: coverageIDs.barra,
        category_id: coverageIDs.extras,
        product_id: null,
      },
      {
        code: 'unrouted_station_inactive',
        message: 'Los productos sin ruta iban a Barra, que está desactivada.',
        device_id: null,
        station_id: coverageIDs.barra,
        category_id: null,
        product_id: null,
      },
    ];
    expect(
      displayWarnings(coverage, coverageIDs.display, [coverageIDs.cocina]).map(
        (warning) => warning.code,
      ),
    ).toEqual(['station_without_routes']);
    expect(routingWarnings(coverage).map((warning) => warning.code)).toEqual([
      'route_station_inactive',
      'unrouted_station_inactive',
    ]);
  });
});
