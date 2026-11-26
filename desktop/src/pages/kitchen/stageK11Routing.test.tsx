// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { PendingOperationsProvider } from '../../hooks/pendingOperations';
import type { KitchenRoute, Station } from '../../services/kitchen';
import type { KitchenCoverage } from '../../services/kitchenCoverage';
import {
  kitchenCoverageCapability,
  rememberServerCapabilities,
} from '../../services/serverCapabilities';
import { coverageIDs, coverageStation, ownerCoverage } from '../../testing/kitchenCoverage';
import { chooseOption } from '../../testing/select';
import { KitchenPage } from './KitchenPage';
import { requestKitchenView } from './kitchenViewRequest';

// Etapa K1.1 (§2 y §4.3, K11-D3): Cocina › Enrutamiento con el ajuste «Productos sin ruta» (PUT kitchen/unrouted con
// la estación que se leyó; 409 → volver a leer y avisar), la lista «Sin ruta: Refrescos, Extras» con lo que dice el
// ajuste y los avisos de rutas a estaciones desactivadas. También el historial de cocina con los nombres nuevos y la
// pestaña que pide el diagnóstico de Terminales. Go se simula con el puente nativo.

const native = vi.hoisted(() => ({ invoke: vi.fn() }));
vi.mock('@tauri-apps/api/core', () => ({ invoke: native.invoke, isTauri: () => true }));

type Request = { operation: string; resourceId: string | null; body?: Record<string, unknown> };
type Reply = { status: number; body: unknown };

let coverage: KitchenCoverage;
let requests: Request[];
let unroutedReplies: Reply[];
let history: unknown[];

const routes: KitchenRoute[] = [
  {
    id: 'r-desayunos',
    product_id: null,
    category_id: coverageIDs.desayunos,
    mode: 'prepare',
    station_ids: [coverageIDs.cocina],
    revision: 2,
  },
  {
    id: 'r-extras',
    product_id: null,
    category_id: coverageIDs.extras,
    mode: 'prepare',
    station_ids: [coverageIDs.barra],
    revision: 5,
  },
];

const emptyBoard = {
  tasks: [],
  releases: [],
  pending_changes: [],
  stations: [],
  server_time: '2026-11-26T15:00:00Z',
  timezone: 'America/Monterrey',
  page: 1,
  station_scope: {
    common: true,
    all: true,
    terminal_all: true,
    terminal_stations: [],
    user_all: true,
    user_all_by_permission: true,
    user_stations: [],
  },
  sound_alert: false,
};

function station(id: string, name: string, active = true): Station {
  return {
    id,
    name,
    description: '',
    active,
    sort_order: 0,
    reception: 'kds',
    printer_id: null,
    target_minutes: 15,
    warning_minutes: 15,
    overdue_minutes: 25,
    grouping: 'inherit',
    tracking: 'inherit',
    revision: 1,
  };
}

beforeEach(() => {
  rememberServerCapabilities([kitchenCoverageCapability]);
  coverage = ownerCoverage();
  coverage.stations.push(
    coverageStation({ id: coverageIDs.barra, name: 'Barra', active: false, reception: 'printer' }),
  );
  coverage.warnings = [
    {
      code: 'route_station_inactive',
      message: 'Extras va a Barra, que está desactivada: se trata como producto sin ruta.',
      device_id: null,
      station_id: coverageIDs.barra,
      category_id: coverageIDs.extras,
      product_id: null,
    },
  ];
  requests = [];
  unroutedReplies = [];
  history = [];
  native.invoke.mockImplementation(async (command: string, args: Request) => {
    if (command !== 'api_request') {
      return null;
    }
    requests.push(args);
    switch (args.operation) {
      case 'kitchen_coverage':
        return { status: 200, body: coverage };
      case 'kitchen_routes':
        return { status: 200, body: routes };
      case 'kitchen_stations':
        return {
          status: 200,
          body: [station(coverageIDs.cocina, 'Cocina'), station(coverageIDs.barra, 'Barra', false)],
        };
      case 'kitchen_targets':
        return {
          status: 200,
          body: {
            products: [],
            categories: coverage.categories.map((category) => ({
              id: category.id,
              name: category.name,
            })),
          },
        };
      case 'kitchen_save_unrouted': {
        const reply = unroutedReplies.shift();
        if (!reply) {
          throw new Error('Sin respuesta preparada');
        }
        return reply;
      }
      case 'kitchen_history':
        return { status: 200, body: history };
      case 'kitchen_board':
        return { status: 200, body: emptyBoard };
    }
    return { status: 200, body: [] };
  });
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  rememberServerCapabilities([]);
});

function renderKitchen(can: (key: string) => boolean = () => true) {
  render(
    <PendingOperationsProvider>
      <KitchenPage can={can} timezone="America/Monterrey" />
    </PendingOperationsProvider>,
  );
}

async function openRouting() {
  fireEvent.click(await screen.findByRole('button', { name: 'Enrutamiento' }));
  return screen.findByRole('region', { name: 'Productos sin ruta' });
}

function coverageReads() {
  return requests.filter((request) => request.operation === 'kitchen_coverage').length;
}

describe('Productos sin ruta en Enrutamiento', () => {
  it('lista lo que no va a ninguna estación con lo que dice el ajuste y avisa de la ruta a una estación desactivada', async () => {
    renderKitchen();
    const panel = await openRouting();
    expect(panel.textContent).toContain('Sin ruta: Refrescos, Extras (no necesitan preparación).');
    const choice = within(panel).getByRole('group', {
      name: 'Qué pasa con los productos sin ruta',
    });
    expect(
      within(choice)
        .getByRole('button', { name: 'No necesitan preparación' })
        .getAttribute('aria-pressed'),
    ).toBe('true');
    const warnings = within(panel).getByRole('list', { name: 'Avisos de enrutamiento' });
    expect(warnings.textContent).toContain('Extras va a Barra, que está desactivada');
    // «Cambiar la ruta» abre esa ruta en el editor.
    fireEvent.click(within(warnings).getByRole('button', { name: 'Cambiar la ruta' }));
    expect(await screen.findByRole('heading', { name: 'Ruta de Extras' })).toBeTruthy();
  });

  it('«Enviar a una estación» guarda la estación con la que se leyó y la lista lo dice', async () => {
    unroutedReplies = [
      {
        status: 200,
        body: {
          station_id: coverageIDs.cocina,
          station_name: 'Cocina',
          station_active: true,
          effective: 'station',
        },
      },
    ];
    renderKitchen();
    const panel = await openRouting();
    fireEvent.click(within(panel).getByRole('button', { name: 'Enviar a una estación' }));
    const select = within(panel).getByRole('combobox', {
      name: 'Estación para los productos sin ruta',
    });
    // Solo las estaciones activas se pueden elegir.
    chooseOption(select, 'Cocina');
    coverage = {
      ...coverage,
      unrouted: {
        station_id: coverageIDs.cocina,
        station_name: 'Cocina',
        station_active: true,
        effective: 'station',
      },
    };
    fireEvent.click(within(panel).getByRole('button', { name: 'Guardar productos sin ruta' }));
    await waitFor(() =>
      expect(requests.some((request) => request.operation === 'kitchen_save_unrouted')).toBe(true),
    );
    const save = requests.find((request) => request.operation === 'kitchen_save_unrouted')!;
    expect(save.body).toMatchObject({ station_id: coverageIDs.cocina, expected_station_id: null });
    expect(typeof save.body?.operation_id).toBe('string');
    expect(await screen.findByText('Productos sin ruta guardado.')).toBeTruthy();
    await waitFor(() =>
      expect(panel.textContent).toContain('Sin ruta: Refrescos, Extras (van a Cocina).'),
    );
  });

  it('si otra persona lo cambió (409), se vuelve a leer y se avisa en su lugar', async () => {
    unroutedReplies = [
      {
        status: 409,
        body: {
          error: {
            code: 'KITCHEN_UNROUTED_CHANGED',
            message: 'Otra persona cambió los productos sin ruta. Revisa el valor vigente.',
          },
        },
      },
    ];
    renderKitchen();
    const panel = await openRouting();
    fireEvent.click(within(panel).getByRole('button', { name: 'Enviar a una estación' }));
    chooseOption(
      within(panel).getByRole('combobox', { name: 'Estación para los productos sin ruta' }),
      'Cocina',
    );
    const before = coverageReads();
    fireEvent.click(within(panel).getByRole('button', { name: 'Guardar productos sin ruta' }));
    const alert = await screen.findByRole('alert', { name: 'Error' });
    expect(alert.textContent).toContain('Otra persona cambió los productos sin ruta.');
    expect(alert.textContent).not.toContain('KITCHEN_UNROUTED_CHANGED');
    expect(
      await within(panel).findByText(/Alguien más cambió este ajuste mientras lo editabas/),
    ).toBeTruthy();
    await waitFor(() => expect(coverageReads()).toBeGreaterThan(before));
    // Se vuelve a mostrar lo guardado.
    expect(
      within(panel)
        .getByRole('button', { name: 'No necesitan preparación' })
        .getAttribute('aria-pressed'),
    ).toBe('true');
  });

  it('sin kitchen-coverage-v1 ni permiso para ver las estaciones no se pide la cobertura', async () => {
    rememberServerCapabilities([]);
    renderKitchen();
    fireEvent.click(await screen.findByRole('button', { name: 'Enrutamiento' }));
    await screen.findByRole('heading', { name: 'Enrutamiento' });
    expect(screen.queryByRole('region', { name: 'Productos sin ruta' })).toBeNull();
    cleanup();
    rememberServerCapabilities([kitchenCoverageCapability]);
    renderKitchen((key) => key !== 'kitchen.stations.read');
    fireEvent.click(await screen.findByRole('button', { name: 'Enrutamiento' }));
    await screen.findByRole('heading', { name: 'Enrutamiento' });
    expect(requests.some((request) => request.operation === 'kitchen_coverage')).toBe(false);
  });
});

describe('historial y pestaña pedida', () => {
  it('el historial dice en español lo que se dio por terminado y el ajuste guardado', async () => {
    history = [
      {
        id: 'h1',
        kind: 'release.dismissed',
        actor_name: 'Sistema',
        device_name: 'Caja principal',
        created_at: '2026-11-26T15:00:00Z',
        after_value: { state: 'cancelled', reason: 'account_closed' },
      },
      {
        id: 'h2',
        kind: 'change.dismissed',
        actor_name: 'Sistema',
        device_name: 'Caja principal',
        created_at: '2026-11-26T15:01:00Z',
        after_value: { reason: 'account_not_open' },
      },
      {
        id: 'h3',
        kind: 'unrouted.saved',
        actor_name: 'Administrador',
        device_name: 'Caja principal',
        created_at: '2026-11-26T15:02:00Z',
        after_value: {},
      },
    ];
    renderKitchen();
    fireEvent.click(await screen.findByRole('button', { name: 'Historial' }));
    expect(await screen.findByText('Pedido sin enviar dado por terminado')).toBeTruthy();
    expect(screen.getByText('Aviso sin entregar dado por terminado')).toBeTruthy();
    expect(screen.getByText('Productos sin ruta actualizado')).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: /Pedido sin enviar dado por terminado/ }));
    expect(await screen.findByText('Motivo: La cuenta se cobró y se cerró.')).toBeTruthy();
    expect(document.body.textContent).not.toContain('account_closed');
  });

  it('Cocina abre en la pestaña que pidió el diagnóstico de Terminales, una sola vez', async () => {
    requestKitchenView('routing');
    renderKitchen();
    expect(await screen.findByRole('region', { name: 'Productos sin ruta' })).toBeTruthy();
    cleanup();
    renderKitchen();
    expect(
      (await screen.findByRole('button', { name: 'Preparación' })).getAttribute('aria-pressed'),
    ).toBe('true');
  });
});
