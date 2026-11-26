// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { NavigationValue } from '../navigation/NavigationContext';
import { NavigationContext } from '../navigation/NavigationContext';
import type { Station } from '../services/kitchen';
import type {
  CoverageStation,
  CoverageWarning,
  KitchenCoverage,
} from '../services/kitchenCoverage';
import type { Device, DeviceConfiguration } from '../services/models';
import {
  kitchenCoverageCapability,
  kitchenDisplayCapability,
  rememberServerCapabilities,
} from '../services/serverCapabilities';
import { coverageIDs, coverageStation, fullStation } from '../testing/kitchenCoverage';
import { chooseOption } from '../testing/select';
import { DevicesPage } from './DevicesPage';
import { takeKitchenViewRequest } from './kitchen/kitchenViewRequest';

// Etapa K1.1 (§4.1 y §4.2; K11-D5 y K11-D6): en Sistema › Terminales, al aprobar una pantalla de cocina se abre el
// asistente de qué recibe (estaciones, categorías y resumen), que también se abre con «Configurar qué recibe»; «Ver
// configuración» dice qué le llega por estación y los avisos de Go con un botón que lleva a corregirlos. El asistente
// usa las rutas de siempre en orden y la cobertura para leer; aquí un Go simulado la vuelve a resolver tras cada
// escritura. Todo se esconde sin kitchen-coverage-v1.

const native = vi.hoisted(() => ({ invoke: vi.fn() }));
vi.mock('@tauri-apps/api/core', () => ({ invoke: native.invoke, isTauri: () => true }));

type Request = { operation: string; resourceId: string | null; body?: Record<string, unknown> };

const display: Device = {
  id: coverageIDs.display,
  name: 'Tableta de la cocina',
  register_id: null,
  keyboard_mode: 'auto',
  home_screen: 'tables',
  kind: 'kitchen',
  os: 'Android 14',
  app_version: '0.1.0',
  paired_at: '2026-11-26T15:00:00Z',
  online: true,
  current_user: null,
  seats: ['kitchen_displays'],
  kitchen_display: {
    text_size: 'normal',
    theme: 'dark',
    sound_enabled: true,
    sound_volume: 'medium',
    seat_released_at: null,
  },
};

// Lo que guarda el Go simulado: estaciones, impresoras, categorías y sus rutas de categoría.
interface Model {
  stations: CoverageStation[];
  revisions: Record<string, number>;
  printers: { id: string; name: string; active: boolean }[];
  categories: { id: string; name: string }[];
  routes: Record<
    string,
    { id: string; revision: number; mode: 'prepare' | 'none'; stations: string[] }
  >;
  warnings: CoverageWarning[];
  paired: boolean;
}

let model: Model;
let requests: Request[];
let nextID: number;

function newID() {
  nextID++;
  return `00000000-0000-4000-8000-${String(nextID).padStart(12, '0')}`;
}

// La cobertura que resolvería Go con el modelo: cada categoría llega a las estaciones activas de su ruta y lo que no
// llega a ninguna queda sin ruta.
function coverage(): KitchenCoverage {
  const stations = model.stations.map((station) => ({
    ...station,
    categories: model.categories
      .filter((category) => {
        const route = model.routes[category.id];
        return station.active && route?.mode === 'prepare' && route.stations.includes(station.id);
      })
      .map((category) => ({ id: category.id, name: category.name, products: 3 })),
  }));
  const reaching = new Set(
    stations.flatMap((station) => station.categories.map((entry) => entry.id)),
  );
  const screen = stations.filter(
    (station) => station.active && (station.reception === 'kds' || station.reception === 'both'),
  );
  return {
    unrouted: { station_id: null, station_name: null, station_active: null, effective: 'none' },
    stations,
    no_preparation: { categories: [], products: [] },
    unrouted_items: model.categories
      .filter((category) => !reaching.has(category.id))
      .map((category) => ({
        category_id: category.id,
        category_name: category.name,
        whole_category: true,
        products: [
          { id: `p-${category.id}`, name: `Producto de ${category.name}`, reason: 'no_route' },
        ],
      })),
    categories: model.categories.map((category) => {
      const route = model.routes[category.id];
      return {
        id: category.id,
        name: category.name,
        route_id: route?.id ?? null,
        route_revision: route?.revision ?? null,
        route_mode: route?.mode ?? null,
        station_ids: route?.stations ?? [],
      };
    }),
    displays: model.paired
      ? [
          {
            device_id: display.id,
            name: display.name,
            all_stations: true,
            station_ids: screen.map((station) => station.id),
          },
        ]
      : [],
    warnings: model.warnings,
  };
}

function configuration(): DeviceConfiguration {
  return {
    device: display,
    kitchen_all_stations: true,
    kitchen_sound_alert: false,
    kitchen_stations: [],
    printers: [],
    kitchen_display_station_options: model.stations
      .filter((station) => station.reception !== 'printer')
      .map((station) => ({
        id: station.id,
        name: station.name,
        active: station.active,
        reception: station.reception as 'kds' | 'both',
      })),
  };
}

const categories = [
  { id: coverageIDs.desayunos, name: 'Desayunos' },
  { id: coverageIDs.hamburguesas, name: 'Hamburguesas' },
  { id: coverageIDs.refrescos, name: 'Refrescos' },
  { id: coverageIDs.extras, name: 'Extras' },
];

beforeEach(() => {
  rememberServerCapabilities([kitchenDisplayCapability, kitchenCoverageCapability]);
  nextID = 100;
  requests = [];
  model = {
    stations: [],
    revisions: {},
    printers: [],
    categories: [...categories],
    routes: {},
    warnings: [],
    paired: false,
  };
  native.invoke.mockImplementation(async (command: string, args: Request) => {
    if (command !== 'api_request') {
      return null;
    }
    requests.push(args);
    const body = args.body ?? {};
    switch (args.operation) {
      case 'devices':
        return { status: 200, body: model.paired ? [display] : [] };
      case 'devices_pairing_requests':
        return {
          status: 200,
          body: {
            requests: model.paired
              ? []
              : [
                  {
                    id: '00000000-0000-4000-8000-0000000000aa',
                    name: 'Tableta nueva',
                    device_kind: 'kitchen',
                    os: 'Android 14',
                    app_version: '0.1.0',
                    created_at: '2026-11-26T15:00:00Z',
                    expires_at: '2026-11-26T15:10:00Z',
                    ready: true,
                    attempts_left: 5,
                  },
                ],
            approve_here: true,
            pairing_available: true,
          },
        };
      case 'approve_device_pairing':
        return { status: 200, body: { state: 'approved', name: 'Tableta nueva' } };
      case 'devices_sessions_summary':
        return { status: 200, body: { by_kind: { desktop: 1, android: 0, kitchen: 0 }, total: 1 } };
      case 'kitchen_coverage':
        return { status: 200, body: coverage() };
      case 'kitchen_stations':
        return {
          status: 200,
          body: model.stations.map((station) => fullStation(station, model.revisions[station.id])),
        };
      case 'kitchen_outputs':
        return { status: 200, body: model.printers };
      case 'kitchen_create_station': {
        const id = newID();
        const printer = model.printers.find((entry) => entry.id === body.printer_id) ?? null;
        model.stations.push(
          coverageStation({
            id,
            name: String(body.name),
            active: body.active === true,
            reception: body.reception as Station['reception'],
            printer,
          }),
        );
        model.revisions[id] = 1;
        return { status: 200, body: fullStation(model.stations.at(-1)!, 1) };
      }
      case 'kitchen_save_station': {
        const station = model.stations.find((entry) => entry.id === args.resourceId)!;
        if (body.revision !== model.revisions[station.id]) {
          return { status: 409, body: { error: { code: 'CONFLICT', message: 'Cambió.' } } };
        }
        station.active = body.active === true;
        station.reception = body.reception as Station['reception'];
        model.revisions[station.id] = (model.revisions[station.id] ?? 1) + 1;
        return { status: 200, body: fullStation(station, model.revisions[station.id]) };
      }
      case 'kitchen_create_route': {
        const id = newID();
        model.routes[String(body.category_id)] = {
          id,
          revision: 1,
          mode: 'prepare',
          stations: body.station_ids as string[],
        };
        return { status: 200, body: { id, ...body, revision: 1 } };
      }
      case 'kitchen_save_route': {
        const route = model.routes[String(body.category_id)]!;
        if (route.id !== args.resourceId || route.revision !== body.revision) {
          return { status: 409, body: { error: { code: 'CONFLICT', message: 'Cambió.' } } };
        }
        route.mode = 'prepare';
        route.stations = body.station_ids as string[];
        route.revision++;
        return { status: 200, body: { ...body, revision: route.revision } };
      }
      case 'device_configuration':
        return { status: 200, body: configuration() };
      case 'kitchen_save_access':
        return { status: 200, body: { device_id: display.id, all_stations: body.all_stations } };
    }
    return { status: 200, body: [] };
  });
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  rememberServerCapabilities([]);
});

function sent(operation: string) {
  return requests.filter((request) => request.operation === operation);
}

async function approveNewDisplay() {
  const code = await screen.findByRole('textbox', { name: /^Código que muestra Tableta nueva/ });
  fireEvent.change(code, { target: { value: '123456' } });
  fireEvent.click(screen.getByRole('button', { name: 'Aprobar' }));
  return screen.findByRole('region', { name: 'Qué recibe Tableta nueva' });
}

async function row(name: string) {
  return (await screen.findByText(name)).closest('tr') as HTMLElement;
}

describe('asistente al aprobar una pantalla de cocina', () => {
  it('sin estaciones crea «Cocina» de solo pantalla, marca dos categorías y resume lo que recibirá', async () => {
    render(<DevicesPage can={() => true} />);
    const assistant = await approveNewDisplay();
    await within(assistant).findByText('No hay estaciones que reciban comandas en pantalla.');
    expect(
      await within(assistant).findByText(
        'Recibirá las comandas solo en la pantalla: no hay impresoras de cocina activas.',
      ),
    ).toBeTruthy();
    fireEvent.click(within(assistant).getByRole('button', { name: 'Crear «Cocina»' }));
    await waitFor(() => expect(sent('kitchen_create_station')).toHaveLength(1));
    expect(sent('kitchen_create_station')[0]!.body).toMatchObject({
      name: 'Cocina',
      active: true,
      reception: 'kds',
      printer_id: null,
    });
    expect(typeof sent('kitchen_create_station')[0]!.body?.operation_id).toBe('string');
    expect(await screen.findByText('Estación Cocina creada.')).toBeTruthy();
    await waitFor(() =>
      expect(assistant.textContent).toContain('Cocina · Solo pantalla de cocina'),
    );

    fireEvent.click(within(assistant).getByRole('button', { name: 'Siguiente' }));
    const desayunos = await within(assistant).findByRole('checkbox', { name: /^Desayunos/ });
    expect(desayunos.closest('label')!.textContent).toContain('sin ruta');
    fireEvent.click(desayunos);
    fireEvent.click(within(assistant).getByRole('checkbox', { name: /^Hamburguesas/ }));
    fireEvent.click(within(assistant).getByRole('button', { name: 'Guardar y seguir' }));
    await waitFor(() => expect(sent('kitchen_create_route')).toHaveLength(2));
    const cocina = model.stations[0]!.id;
    expect(sent('kitchen_create_route').map((request) => request.body)).toEqual([
      expect.objectContaining({
        category_id: coverageIDs.desayunos,
        product_id: null,
        mode: 'prepare',
        station_ids: [cocina],
      }),
      expect.objectContaining({
        category_id: coverageIDs.hamburguesas,
        product_id: null,
        mode: 'prepare',
        station_ids: [cocina],
      }),
    ]);
    const summary = await within(assistant).findByLabelText('Resumen de lo que recibe');
    await waitFor(() =>
      expect(summary.textContent).toContain('Esta pantalla recibirá: Desayunos, Hamburguesas.'),
    );
    expect(summary.textContent).toContain(
      'No recibirá: Refrescos, Extras (no necesitan preparación).',
    );
    fireEvent.click(within(assistant).getByRole('button', { name: 'Terminar' }));
    expect(screen.queryByRole('region', { name: 'Qué recibe Tableta nueva' })).toBeNull();
  });

  it('con una impresora activa «Cocina» imprime y se ve; con varias hay que elegir', async () => {
    model.printers = [{ id: coverageIDs.printer, name: 'Impresora de cocina', active: true }];
    render(<DevicesPage can={() => true} />);
    const assistant = await approveNewDisplay();
    const printer = await within(assistant).findByRole('combobox', {
      name: 'Impresora de la estación',
    });
    expect(printer.textContent).toContain('Impresora de cocina');
    fireEvent.click(within(assistant).getByRole('button', { name: 'Crear «Cocina»' }));
    await waitFor(() => expect(sent('kitchen_create_station')).toHaveLength(1));
    expect(sent('kitchen_create_station')[0]!.body).toMatchObject({
      reception: 'both',
      printer_id: coverageIDs.printer,
    });
    cleanup();
    model.stations = [];
    model.printers = [
      { id: coverageIDs.printer, name: 'Impresora de cocina', active: true },
      { id: 'otra-impresora', name: 'Impresora de barra', active: true },
    ];
    render(<DevicesPage can={() => true} />);
    const again = await approveNewDisplay();
    const create = await within(again).findByRole('button', { name: 'Crear «Cocina»' });
    await waitFor(() =>
      expect(
        within(again).getByRole('combobox', { name: 'Impresora de la estación' }),
      ).toBeTruthy(),
    );
    expect((create as HTMLButtonElement).disabled).toBe(true);
    chooseOption(
      within(again).getByRole('combobox', { name: 'Impresora de la estación' }),
      'Sin impresora: solo pantalla',
    );
    expect((create as HTMLButtonElement).disabled).toBe(false);
  });

  it('con «Cocina» que solo imprime ofrece usarla y las que solo imprimen pueden recibir en pantalla', async () => {
    model.printers = [{ id: coverageIDs.printer, name: 'Impresora de cocina', active: true }];
    model.stations = [
      coverageStation({
        id: coverageIDs.cocina,
        name: 'Cocina',
        reception: 'printer',
        printer: model.printers[0]!,
      }),
      coverageStation({
        id: coverageIDs.barra,
        name: 'Barra',
        reception: 'printer',
        printer: model.printers[0]!,
      }),
    ];
    model.revisions = { [coverageIDs.cocina]: 4, [coverageIDs.barra]: 2 };
    render(<DevicesPage can={() => true} />);
    const assistant = await approveNewDisplay();
    expect(
      await within(assistant).findByText(/Ya existe y solo imprime: pasa a «Impresora y pantalla/),
    ).toBeTruthy();
    expect(within(assistant).queryByRole('button', { name: 'Crear «Cocina»' })).toBeNull();
    const use = within(assistant).getByRole('button', { name: 'Usar «Cocina»' });
    await waitFor(() => expect((use as HTMLButtonElement).disabled).toBe(false));
    fireEvent.click(use);
    await waitFor(() => expect(sent('kitchen_save_station')).toHaveLength(1));
    expect(sent('kitchen_save_station')[0]!.resourceId).toBe(coverageIDs.cocina);
    expect(sent('kitchen_save_station')[0]!.body).toMatchObject({
      id: coverageIDs.cocina,
      revision: 4,
      active: true,
      reception: 'both',
      printer_id: coverageIDs.printer,
    });
    const barra = await within(assistant).findByText('Barra');
    fireEvent.click(
      within(barra.closest('li')!).getByRole('button', { name: 'Recibir también en pantalla' }),
    );
    await waitFor(() => expect(sent('kitchen_save_station')).toHaveLength(2));
    expect(sent('kitchen_save_station')[1]!.body).toMatchObject({
      id: coverageIDs.barra,
      revision: 2,
      reception: 'both',
    });
  });

  it('marcar una categoría que va a otra estación la cambia, con su aviso', async () => {
    model.stations = [
      coverageStation({ id: coverageIDs.cocina, name: 'Cocina', reception: 'kds' }),
      coverageStation({ id: coverageIDs.barra, name: 'Barra', reception: 'printer' }),
    ];
    model.categories = [{ id: coverageIDs.bebidas, name: 'Bebidas' }];
    model.routes[coverageIDs.bebidas] = {
      id: 'r-bebidas',
      revision: 7,
      mode: 'prepare',
      stations: [coverageIDs.barra],
    };
    model.paired = true;
    render(<DevicesPage can={() => true} />);
    fireEvent.click(
      within(await row('Tableta de la cocina')).getByRole('button', {
        name: 'Configurar qué recibe',
      }),
    );
    const assistant = await screen.findByRole('region', {
      name: 'Qué recibe Tableta de la cocina',
    });
    // La pantalla muestra Cocina (Barra solo imprime): Bebidas va a Barra.
    fireEvent.click(within(assistant).getByRole('button', { name: 'Siguiente' }));
    const bebidas = await within(assistant).findByRole('checkbox', { name: /^Bebidas/ });
    await waitFor(() => expect(bebidas.closest('label')!.textContent).toContain('va a Barra'));
    fireEvent.click(bebidas);
    expect(within(assistant).getByText('Bebidas dejará de ir a Barra.')).toBeTruthy();
    fireEvent.click(within(assistant).getByRole('button', { name: 'Guardar y seguir' }));
    await waitFor(() => expect(sent('kitchen_save_route')).toHaveLength(1));
    expect(sent('kitchen_save_route')[0]!.resourceId).toBe('r-bebidas');
    expect(sent('kitchen_save_route')[0]!.body).toMatchObject({
      id: 'r-bebidas',
      revision: 7,
      category_id: coverageIDs.bebidas,
      mode: 'prepare',
      station_ids: [coverageIDs.cocina],
    });
  });

  it('en una pantalla emparejada se eligen sus estaciones con la configuración que se leyó', async () => {
    model.stations = [
      coverageStation({ id: coverageIDs.cocina, name: 'Cocina', reception: 'kds' }),
      coverageStation({ id: coverageIDs.plancha, name: 'Plancha', reception: 'both' }),
    ];
    model.paired = true;
    render(<DevicesPage can={() => true} />);
    fireEvent.click(
      within(await row('Tableta de la cocina')).getByRole('button', {
        name: 'Configurar qué recibe',
      }),
    );
    const assistant = await screen.findByRole('region', {
      name: 'Qué recibe Tableta de la cocina',
    });
    const all = await within(assistant).findByLabelText(
      'Mostrar todas las estaciones que reciben comandas en pantalla',
    );
    await waitFor(() => expect((all as HTMLInputElement).disabled).toBe(false));
    fireEvent.click(all);
    chooseOption(
      within(assistant).getByRole('combobox', { name: 'Estaciones que muestra esta pantalla' }),
      'Plancha',
    );
    fireEvent.click(
      within(assistant).getByRole('button', { name: 'Guardar estaciones de la pantalla' }),
    );
    await waitFor(() => expect(sent('kitchen_save_access')).toHaveLength(1));
    expect(sent('kitchen_save_access')[0]!.body).toMatchObject({
      device_id: display.id,
      all_stations: false,
      station_ids: [coverageIDs.plancha],
      expected_all_stations: true,
      expected_station_ids: [],
    });
  });

  it('sin los permisos de cocina cada paso se ve en solo lectura y se puede saltar', async () => {
    const can = (key: string) =>
      key !== 'kitchen.stations.manage' && key !== 'kitchen.routing.manage';
    render(<DevicesPage can={can} />);
    const assistant = await approveNewDisplay();
    await within(assistant).findByText(/Para cambiar las estaciones hace falta el permiso/);
    expect(within(assistant).queryByRole('button', { name: 'Crear «Cocina»' })).toBeNull();
    expect(sent('kitchen_outputs')).toHaveLength(0);
    model.stations = [coverageStation({ id: coverageIDs.cocina, name: 'Cocina' })];
    window.dispatchEvent(new CustomEvent('adel-server-change'));
    fireEvent.click(within(assistant).getByRole('button', { name: 'Siguiente' }));
    const desayunos = await within(assistant).findByRole('checkbox', { name: /^Desayunos/ });
    expect((desayunos as HTMLInputElement).disabled).toBe(true);
    expect(
      within(assistant).getByText(/Para cambiar las rutas hace falta el permiso/),
    ).toBeTruthy();
    expect(within(assistant).queryByRole('button', { name: 'Guardar y seguir' })).toBeNull();
    fireEvent.click(within(assistant).getByRole('button', { name: 'Saltar el asistente' }));
    expect(screen.queryByRole('region', { name: 'Qué recibe Tableta nueva' })).toBeNull();
  });

  it('sin kitchen-coverage-v1 aprobar no abre el asistente ni hay «Configurar qué recibe»', async () => {
    rememberServerCapabilities([kitchenDisplayCapability]);
    render(<DevicesPage can={() => true} />);
    const code = await screen.findByRole('textbox', { name: /^Código que muestra Tableta nueva/ });
    fireEvent.change(code, { target: { value: '123456' } });
    fireEvent.click(screen.getByRole('button', { name: 'Aprobar' }));
    await waitFor(() => expect(sent('approve_device_pairing')).toHaveLength(1));
    expect(await screen.findByText(/Terminal Tableta nueva aprobada/)).toBeTruthy();
    expect(screen.queryByRole('region', { name: /Qué recibe/ })).toBeNull();
    expect(sent('kitchen_coverage')).toHaveLength(0);
    cleanup();
    model.paired = true;
    render(<DevicesPage can={() => true} />);
    const line = await row('Tableta de la cocina');
    expect(within(line).queryByRole('button', { name: 'Configurar qué recibe' })).toBeNull();
  });
});

describe('diagnóstico en «Ver configuración»', () => {
  function warning(overrides: Partial<CoverageWarning>): CoverageWarning {
    return {
      code: 'station_without_routes',
      message: '',
      device_id: null,
      station_id: null,
      category_id: null,
      product_id: null,
      ...overrides,
    };
  }

  beforeEach(() => {
    model.paired = true;
    model.stations = [
      coverageStation({ id: coverageIDs.cocina, name: 'Cocina', reception: 'kds' }),
      coverageStation({
        id: coverageIDs.plancha,
        name: 'Plancha',
        reception: 'both',
        printer: { id: coverageIDs.printer, name: 'Impresora de la plancha', active: false },
      }),
    ];
    model.routes[coverageIDs.desayunos] = {
      id: 'r-desayunos',
      revision: 1,
      mode: 'prepare',
      stations: [coverageIDs.cocina],
    };
    model.warnings = [
      warning({
        code: 'display_no_screen_stations',
        message: 'Ninguna de sus estaciones recibe comandas en pantalla.',
        device_id: display.id,
      }),
      warning({
        code: 'station_without_routes',
        message: 'La estación Plancha no tiene categorías ni productos asignados.',
        station_id: coverageIDs.plancha,
      }),
      warning({
        code: 'station_printer_inactive',
        message:
          'La impresora Impresora de la plancha de Plancha está desactivada: sus comandas esperan en la cola.',
        station_id: coverageIDs.plancha,
      }),
      // De otra pantalla: no se muestra aquí.
      warning({
        code: 'display_without_stations',
        message: 'Esta pantalla no tiene estaciones.',
        device_id: 'otra-pantalla',
      }),
    ];
  });

  async function openConfiguration(navigation?: NavigationValue) {
    render(
      <NavigationContext.Provider value={navigation ?? null}>
        <DevicesPage can={() => true} />
      </NavigationContext.Provider>,
    );
    fireEvent.click(
      within(await row('Tableta de la cocina')).getByRole('button', { name: 'Ver configuración' }),
    );
    return screen.findByRole('region', { name: 'Qué recibe esta pantalla' });
  }

  it('dice qué le llega por estación y solo los avisos de esta pantalla y sus estaciones', async () => {
    const diagnostics = await openConfiguration();
    const cocina = await within(diagnostics).findByRole('article', { name: 'Estación Cocina' });
    expect(cocina.textContent).toContain('Solo pantalla de cocina');
    expect(cocina.textContent).toContain('Categorías: Desayunos (3 productos)');
    const plancha = within(diagnostics).getByRole('article', { name: 'Estación Plancha' });
    expect(plancha.textContent).toContain('No le llega ningún producto.');
    const warnings = within(diagnostics).getByRole('list', { name: 'Avisos de la pantalla' });
    expect(warnings.textContent).toContain(
      'Ninguna de sus estaciones recibe comandas en pantalla.',
    );
    expect(warnings.textContent).toContain(
      'La estación Plancha no tiene categorías ni productos asignados.',
    );
    expect(warnings.textContent).toContain('sus comandas esperan en la cola');
    expect(warnings.textContent).not.toContain('Esta pantalla no tiene estaciones.');
  });

  it('cada aviso lleva a corregirlo: al asistente en su paso o a la pestaña de Cocina', async () => {
    const requestScreen = vi.fn(async () => true);
    const navigation: NavigationValue = {
      screen: 'devices',
      home: 'restaurant',
      requestScreen,
      setTail: vi.fn(),
    };
    let diagnostics = await openConfiguration(navigation);
    fireEvent.click(within(diagnostics).getByRole('button', { name: 'Ir a Cocina › Impresoras' }));
    expect(requestScreen).toHaveBeenCalledWith('kitchen');
    expect(takeKitchenViewRequest()).toBe('printers');

    fireEvent.click(within(diagnostics).getByRole('button', { name: 'Elegir sus categorías' }));
    let assistant = await screen.findByRole('region', { name: 'Qué recibe Tableta de la cocina' });
    expect(
      within(assistant).getByText('2. Qué categorías llegan').getAttribute('aria-current'),
    ).toBe('step');
    expect(screen.queryByRole('region', { name: 'Qué recibe esta pantalla' })).toBeNull();
    fireEvent.click(within(assistant).getByRole('button', { name: 'Saltar el asistente' }));

    diagnostics = await openConfiguration(navigation);
    const warnings = within(diagnostics).getByRole('list', { name: 'Avisos de la pantalla' });
    fireEvent.click(within(warnings).getByRole('button', { name: 'Configurar qué recibe' }));
    assistant = await screen.findByRole('region', { name: 'Qué recibe Tableta de la cocina' });
    expect(
      within(assistant).getByText('1. Estaciones de esta pantalla').getAttribute('aria-current'),
    ).toBe('step');
  });

  it('sin navegación ni permisos de la pestaña, el aviso no ofrece un botón que no lleva a ningún lado', async () => {
    const diagnostics = await openConfiguration();
    expect(
      within(diagnostics).queryByRole('button', { name: 'Ir a Cocina › Impresoras' }),
    ).toBeNull();
    expect(within(diagnostics).getByRole('button', { name: 'Elegir sus categorías' })).toBeTruthy();
  });
});
