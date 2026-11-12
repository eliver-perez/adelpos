// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  TableMap,
  fitTables,
  maximumMapScale,
  minimumTableSide,
} from '../../components/restaurant/TableMap';
import { PendingOperationsProvider } from '../../hooks/pendingOperations';
import { elapsedTime } from '../../services/restaurant';
import type { Area, RestaurantTable } from '../../services/restaurant';
import { OperationalPage } from './OperationalPage';
import { TablesSummaryPanel, areaTables, countTableStates } from './TablesSummaryPanel';

const native = vi.hoisted(() => ({ invoke: vi.fn() }));
vi.mock('@tauri-apps/api/core', () => ({ invoke: native.invoke, isTauri: () => true }));

// Etapa 9D, fase C (§3): Mesas con el panel lateral siempre visible y el resumen del área, mapa con
// escala automática (mínimo de 64 px), un solo botón principal por panel, tiempos en días y horas,
// advertencia de cuenta abierta por mucho tiempo (marca long_open de Go) y sin «Actualizar»
// («Reintentar» solo sin conexión). Solo se sustituye el transporte nativo por un servidor simulado.

const longSeconds = (3 * 24 + 14) * 3600 + 33 * 60;

const area: Area = {
  id: 'salon',
  name: 'Salón',
  description: '',
  sort_order: 10,
  active: true,
  revision: 1,
  table_count: 3,
};

function tableOf(values: Partial<RestaurantTable> & { id: string; name: string }): RestaurantTable {
  return {
    area_id: 'salon',
    shape: 'rectangle',
    x: 20,
    y: 20,
    width: 120,
    height: 100,
    seats: 4,
    active: true,
    out_of_service: false,
    revision: 1,
    account_count: 0,
    state: 'free',
    occupied_since: null,
    occupied_seconds: 0,
    balance: '0.00',
    long_open: false,
    ...values,
  };
}

const tables = [
  tableOf({ id: 'mesa-1', name: 'Mesa 1', x: 300, y: 100 }),
  tableOf({
    id: 'mesa-2',
    name: 'Mesa 2',
    x: 460,
    y: 100,
    state: 'occupied',
    account_count: 1,
    occupied_since: '2026-11-08T18:30:00Z',
    occupied_seconds: longSeconds,
    long_open: true,
  }),
  tableOf({
    id: 'mesa-3',
    name: 'Mesa 3',
    x: 300,
    y: 260,
    state: 'partial_payment',
    account_count: 1,
    occupied_since: '2026-11-08T18:30:00Z',
    occupied_seconds: 1500,
    balance: '50.00',
  }),
  tableOf({
    id: 'mesa-4',
    name: 'Mesa 4',
    x: 460,
    y: 260,
    out_of_service: true,
    state: 'out_of_service',
  }),
];

function serverAccount(id: string, folio: number, table: string, longOpen: boolean) {
  return {
    id,
    folio,
    name: '',
    mode: 'dine_in',
    state: 'open',
    table_id: table,
    table_name: tables.find((entry) => entry.id === table)!.name,
    price_list_id: 'lista',
    customer_name: '',
    phone: '',
    origin: '',
    notes: '',
    scheduled_at: null,
    scheduled_local: '',
    schedule_kind: null,
    assigned_user_id: 'usuario',
    assigned_name: 'Laura',
    created_at: '2026-11-08T18:30:00Z',
    updated_at: '2026-11-08T18:30:00Z',
    revision: 2,
    merged_into_id: null,
    elapsed_seconds: longOpen ? longSeconds : 1500,
    urgency: 'normal',
    preparation_state: 'released',
    payment_state: longOpen ? 'not_recorded' : 'partial',
    delivery_state: 'not_delivered',
    finance: {
      total: '150.00',
      paid: longOpen ? '0.00' : '100.00',
      balance: longOpen ? '150.00' : '50.00',
      tips: '0.00',
      payment_state: longOpen ? 'not_recorded' : 'partial',
      delivery_state: 'not_delivered',
    },
    confirmed_quantity: 1,
    draft_quantity: 0,
    totals: { subtotal: '150.00', discount: '0.00', total: '150.00' },
    releasable: false,
    discardable: false,
    cancel_blocker: '',
    cancel_requires_authorization: false,
    long_open: longOpen,
  };
}

const accounts = [
  serverAccount('cuenta-1', 1, 'mesa-2', true),
  serverAccount('cuenta-2', 2, 'mesa-3', false),
];

const settings = { warning_minutes: 15, overdue_minutes: 30, long_open_hours: 12, revision: 1 };
let requests: { operation: string; body: unknown }[];

function respond(operation: string, id: string | null, query: Record<string, string> | null) {
  switch (operation) {
    case 'restaurant_overview':
      return { areas: [area], tables, settings, timezone: 'America/Monterrey' };
    case 'restaurant_accounts': {
      const items = accounts.filter(
        (entry) => !query?.table_id || entry.table_id === query.table_id,
      );
      return {
        items: query?.mode === 'takeaway' ? [] : items,
        total: items.length,
        page: 1,
        page_size: 50,
        server_time: '2026-11-12T19:00:00Z',
        timezone: 'America/Monterrey',
        settings,
      };
    }
    case 'restaurant_account':
      return {
        account: accounts.find((entry) => entry.id === id),
        draft: [],
        lines: [],
        server_time: '2026-11-12T19:00:00Z',
        timezone: 'America/Monterrey',
      };
    case 'catalog_lookups':
      return {
        categories: [],
        ingredients: [],
        complements: [],
        portions: [],
        groups: [],
        'price-lists': [{ id: 'lista', name: 'Restaurante' }],
        palette: [],
        products: [],
      };
    case 'catalog_products':
      return { items: [], total: 0 };
    default:
      return {};
  }
}

beforeEach(() => {
  requests = [];
  native.invoke.mockImplementation(
    async (
      command: string,
      args: {
        operation: string;
        resourceId: string | null;
        body: unknown;
        query: Record<string, string> | null;
      },
    ) => {
      if (command !== 'api_request') return null;
      requests.push({ operation: args.operation, body: args.body });
      return { status: 200, body: respond(args.operation, args.resourceId, args.query) };
    },
  );
});

afterEach(cleanup);

function renderPage(
  options: { connectionLost?: boolean; onRetryConnection?: () => void } = {},
  can: (permission: string) => boolean = () => true,
) {
  render(
    <PendingOperationsProvider>
      <OperationalPage
        active
        timezone="America/Monterrey"
        requestedView="tables"
        can={can}
        connectionLost={options.connectionLost}
        onRetryConnection={options.onRetryConnection}
      />
    </PendingOperationsProvider>,
  );
}

describe('tiempos en días y horas (§3.4)', () => {
  it('minutos, horas y minutos y, desde 24 horas, días y horas', () => {
    expect(elapsedTime(0)).toBe('0 min');
    expect(elapsedTime(45 * 60 + 59)).toBe('45 min');
    expect(elapsedTime(3900)).toBe('1 h 5 min');
    expect(elapsedTime(24 * 3600 - 1)).toBe('23 h 59 min');
    expect(elapsedTime(24 * 3600)).toBe('1 d 0 h');
    expect(elapsedTime(longSeconds)).toBe('3 d 14 h');
    // «87 h 33 min» ya no aparece: son 3 días y 15 horas.
    expect(elapsedTime(87 * 3600 + 33 * 60)).toBe('3 d 15 h');
  });
});

describe('escala automática del mapa (§3.2)', () => {
  it('ajusta una sola escala para ver todas las mesas del área', () => {
    const boxes = [
      { x: 300, y: 100, width: 120, height: 100 },
      { x: 600, y: 300, width: 120, height: 100 },
    ];
    const fit = fitTables(boxes, { width: 800, height: 500 })!;
    // El conjunto mide 420 × 300 unidades; el espacio útil es 768 × 468.
    expect(fit.scale).toBeCloseTo(468 / 300, 5);
    expect(fit.overflow).toBe(false);
    expect(fit.width).toBe(800);
    expect(fit.height).toBe(500);
    for (const entry of boxes) {
      const right = fit.offsetX + (entry.x - fit.originX + entry.width) * fit.scale;
      const bottom = fit.offsetY + (entry.y - fit.originY + entry.height) * fit.scale;
      expect(right).toBeLessThanOrEqual(800);
      expect(bottom).toBeLessThanOrEqual(500);
    }
  });

  it('ninguna mesa queda por debajo de 64 px: si no caben, el mapa se desplaza', () => {
    const boxes = [];
    for (let row = 0; row < 5; row += 1) {
      for (let column = 0; column < 7; column += 1) {
        boxes.push({ x: 10 + column * 140, y: 10 + row * 110, width: 64, height: 56 });
      }
    }
    const fit = fitTables(boxes, { width: 600, height: 400 })!;
    expect(fit.scale).toBeCloseTo(minimumTableSide / 56, 5);
    expect(Math.min(...boxes.map((entry) => entry.height * fit.scale))).toBeCloseTo(64, 5);
    expect(fit.overflow).toBe(true);
    expect(fit.width).toBeGreaterThan(600);
    expect(fit.height).toBeGreaterThan(400);
  });

  it('pocas mesas no pasan del doble de tamaño y quedan centradas', () => {
    const fit = fitTables([{ x: 0, y: 0, width: 120, height: 100 }], { width: 1000, height: 800 })!;
    expect(fit.scale).toBe(maximumMapScale);
    expect(fit.offsetX).toBeCloseTo((1000 - 240) / 2, 5);
    expect(fit.offsetY).toBeCloseTo((800 - 200) / 2, 5);
  });

  it('sin mesas o sin espacio medido no hay escala (el lienzo proporcional de siempre)', () => {
    expect(fitTables([], { width: 800, height: 500 })).toBeNull();
    expect(fitTables([{ x: 0, y: 0, width: 64, height: 56 }], { width: 0, height: 0 })).toBeNull();
  });

  it('el mapa operativo dibuja las mesas en píxeles con la misma escala', () => {
    const width = Object.getOwnPropertyDescriptor(HTMLElement.prototype, 'clientWidth');
    const height = Object.getOwnPropertyDescriptor(HTMLElement.prototype, 'clientHeight');
    Object.defineProperty(HTMLElement.prototype, 'clientWidth', {
      configurable: true,
      get() {
        return (this as HTMLElement).classList.contains('table-map-viewport') ? 800 : 0;
      },
    });
    Object.defineProperty(HTMLElement.prototype, 'clientHeight', {
      configurable: true,
      get() {
        return (this as HTMLElement).classList.contains('table-map-viewport') ? 500 : 0;
      },
    });
    try {
      render(
        <TableMap
          areas={[area]}
          tables={tables}
          areaID="salon"
          onAreaChange={() => undefined}
          onSelect={() => undefined}
          autoFit
        />,
      );
      const canvas = screen.getByLabelText('Mapa de mesas');
      const scale = Number(canvas.getAttribute('data-scale'));
      expect(scale).toBeGreaterThan(1);
      expect(canvas.getAttribute('data-overflow')).toBe('false');
      for (const entry of tables) {
        const tile = canvas.querySelector<HTMLElement>(`[data-table-id="${entry.id}"]`)!;
        expect(parseFloat(tile.style.width)).toBeCloseTo(entry.width * scale, 3);
        expect(parseFloat(tile.style.height)).toBeCloseTo(entry.height * scale, 3);
        expect(tile.style.left).toMatch(/px$/);
      }
      // La leyenda va dentro del marco del mapa.
      const frame = canvas.closest('.table-map-frame')!;
      expect(
        within(frame as HTMLElement).getByRole('list', { name: 'Estados de las mesas' }),
      ).toBeTruthy();
    } finally {
      // jsdom define las medidas en Element: se quita la sustitución de HTMLElement.
      for (const [name, original] of [
        ['clientWidth', width],
        ['clientHeight', height],
      ] as const) {
        if (original) {
          Object.defineProperty(HTMLElement.prototype, name, original);
        } else {
          delete (HTMLElement.prototype as unknown as Record<string, unknown>)[name];
        }
      }
    }
  });

  it('el diseñador conserva el lienzo proporcional', () => {
    render(
      <TableMap
        areas={[area]}
        tables={tables}
        areaID="salon"
        onAreaChange={() => undefined}
        onSelect={() => undefined}
        designer
        onPosition={() => undefined}
      />,
    );
    const canvas = screen.getByLabelText('Diseñador de distribución de mesas');
    expect(canvas.getAttribute('data-scale')).toBeNull();
    const tile = canvas.querySelector<HTMLElement>('[data-table-id="mesa-1"]')!;
    expect(tile.style.width).toBe('12%');
  });
});

describe('advertencia de cuenta abierta por mucho tiempo (§3.4)', () => {
  it('la mesa con long_open de Go muestra su tiempo en ámbar con el triángulo', () => {
    render(
      <TableMap
        areas={[area]}
        tables={tables}
        areaID="salon"
        onAreaChange={() => undefined}
        onSelect={() => undefined}
        longOpenHours={12}
      />,
    );
    const aged = screen.getByRole('button', { name: /^Mesa 2, Ocupada/ });
    expect(aged.getAttribute('aria-label')).toBe(
      'Mesa 2, Ocupada, 1 cuenta, 3 d 14 h, abierta hace más de 12 h, mesa para 4',
    );
    expect(aged.className).toContain('table-long-open');
    const time = aged.querySelector('.table-tile-long-open')!;
    expect(time.textContent).toBe('3 d 14 h');
    expect(time.querySelector('svg')?.getAttribute('data-icon')).toBe('alert');
    // Las demás mesas no llevan la advertencia aunque estén ocupadas.
    const other = screen.getByRole('button', { name: /^Mesa 3, Pago parcial/ });
    expect(other.className).not.toContain('table-long-open');
    expect(other.querySelector('.table-tile-time svg')?.getAttribute('data-icon')).toBe('clock');
  });
});

describe('resumen del área (§3.1)', () => {
  it('cuenta las mesas por estado y cada número se toca para resaltar', () => {
    const onHighlight = vi.fn();
    const onAccountWithoutTable = vi.fn();
    const onNewTakeaway = vi.fn();
    expect(countTableStates(areaTables(tables, 'salon'))).toEqual({
      free: 1,
      occupied: 1,
      awaiting_payment: 0,
      partial_payment: 1,
      out_of_service: 1,
    });
    render(
      <TablesSummaryPanel
        area={area}
        tables={areaTables(tables, 'salon')}
        loading={false}
        highlight={null}
        onHighlight={onHighlight}
        canOpenWithoutTable
        canOpenTakeaway
        blocked={false}
        onAccountWithoutTable={onAccountWithoutTable}
        onNewTakeaway={onNewTakeaway}
      />,
    );
    const counts = screen.getByRole('group', { name: 'Mesas por estado' });
    expect(
      within(counts)
        .getAllByRole('button')
        .map((button) => button.getAttribute('aria-label')),
    ).toEqual([
      '1 libre · resaltar en el mapa',
      '1 ocupada · resaltar en el mapa',
      '0 esperando pago · resaltar en el mapa',
      '1 pago parcial · resaltar en el mapa',
    ]);
    // Sin mesas en ese estado no hay nada que resaltar.
    expect(
      (within(counts).getByRole('button', { name: /esperando pago/ }) as HTMLButtonElement)
        .disabled,
    ).toBe(true);
    fireEvent.click(within(counts).getByRole('button', { name: /ocupada/ }));
    expect(onHighlight).toHaveBeenCalledWith('occupied');
    expect(screen.getByText('1 mesa fuera de servicio')).toBeTruthy();
    const quick = screen.getByRole('group', { name: 'Acciones rápidas' });
    fireEvent.click(within(quick).getByRole('button', { name: 'Cuenta sin mesa' }));
    fireEvent.click(within(quick).getByRole('button', { name: 'Nuevo pedido para llevar' }));
    expect(onAccountWithoutTable).toHaveBeenCalledTimes(1);
    expect(onNewTakeaway).toHaveBeenCalledTimes(1);
  });

  it('sin permisos para abrir cuentas no hay acciones rápidas', () => {
    render(
      <TablesSummaryPanel
        area={area}
        tables={areaTables(tables, 'salon')}
        loading={false}
        highlight="occupied"
        onHighlight={() => undefined}
        canOpenWithoutTable={false}
        canOpenTakeaway={false}
        blocked={false}
        onAccountWithoutTable={() => undefined}
        onNewTakeaway={() => undefined}
      />,
    );
    expect(screen.queryByRole('group', { name: 'Acciones rápidas' })).toBeNull();
    expect(
      screen.getByRole('button', { name: /ocupada · resaltar/ }).getAttribute('aria-pressed'),
    ).toBe('true');
  });
});

describe('Mesas con el panel siempre visible (§3.1, §3.3 y §3.5)', () => {
  it('sin mesa elegida el panel muestra el resumen, la ayuda y resalta las mesas en el mapa', async () => {
    renderPage();
    const summary = await screen.findByRole('complementary', { name: 'Resumen del área' });
    const map = await screen.findByRole('region', { name: 'Mapa del restaurante' });
    expect(within(summary).getByText(/Toca una mesa para ver sus cuentas/)).toBeTruthy();
    expect(within(map).queryByText(/Toca una mesa/)).toBeNull();
    expect(screen.queryByRole('button', { name: 'Actualizar' })).toBeNull();
    expect(screen.queryByRole('button', { name: 'Reintentar' })).toBeNull();
    const occupied = await within(summary).findByRole('button', {
      name: '1 ocupada · resaltar en el mapa',
    });
    fireEvent.click(occupied);
    expect(occupied.getAttribute('aria-pressed')).toBe('true');
    const busy = within(map).getByRole('button', { name: /^Mesa 2, Ocupada/ });
    expect(busy.className).toContain('table-highlighted');
    expect(within(map).getByRole('button', { name: /^Mesa 1, Libre/ }).className).not.toContain(
      'table-highlighted',
    );
    fireEvent.click(occupied);
    expect(busy.className).not.toContain('table-highlighted');
    // Elegir una mesa cambia el resumen por el panel de la mesa y quita el resaltado.
    fireEvent.click(occupied);
    fireEvent.click(busy);
    await screen.findByRole('complementary', { name: 'Panel de Mesa 2' });
    expect(screen.queryByRole('complementary', { name: 'Resumen del área' })).toBeNull();
    expect(map.querySelector('.table-highlighted')).toBeNull();
  });

  it('«Cuenta sin mesa» del resumen entra a la captura sin crear la cuenta (fase D)', async () => {
    renderPage();
    const summary = await screen.findByRole('complementary', { name: 'Resumen del área' });
    fireEvent.click(within(summary).getByRole('button', { name: 'Cuenta sin mesa' }));
    // Etapa 9D (§4.1, decisión 2): la cuenta nace con el primer producto guardado.
    await screen.findByRole('heading', { name: 'Cuenta nueva sin mesa' });
    expect(requests.some((entry) => entry.operation === 'restaurant_open_account')).toBe(false);
  });

  it('un solo botón principal en el panel de la mesa: «Capturar pedidos en esta mesa»', async () => {
    renderPage();
    fireEvent.click(await screen.findByRole('button', { name: /^Mesa 2, Ocupada/ }));
    const panel = await screen.findByRole('complementary', { name: 'Panel de Mesa 2' });
    const capture = await within(panel).findByRole('button', {
      name: 'Capturar pedidos en esta mesa',
    });
    expect(capture.className).toContain('primary-button');
    const checkout = await within(panel).findByRole('button', { name: 'Cobrar · $150.00' });
    expect(checkout.className).toContain('secondary-button');
    expect(checkout.className).toContain('capture-checkout-outline');
    expect(checkout.className).not.toContain('primary-button');
    expect(panel.querySelectorAll('.primary-button')).toHaveLength(1);
    // «Abierta hace 3 d 14 h» en ámbar con la explicación para el lector de pantalla.
    const fact = panel.querySelector('.table-panel-long-open')!;
    expect(fact.textContent).toBe('Abierta hace 3 d 14 h · abierta hace más de 12 h');
    expect(fact.querySelector('svg')?.getAttribute('data-icon')).toBe('alert');
  });

  it('en Cuentas la tarjeta abierta por mucho tiempo va en ámbar y el detalle tiene un principal', async () => {
    renderPage();
    fireEvent.click(await screen.findByRole('button', { name: 'Cuentas' }));
    expect(screen.queryByRole('button', { name: 'Actualizar' })).toBeNull();
    const cards = await screen.findByRole('group', { name: 'Cuentas disponibles' });
    const aged = (await within(cards).findByText('Mesa 2')).closest('button')!;
    expect(aged.className).toContain('account-long-open');
    expect(aged.querySelector('.account-long-open-time')?.textContent).toContain('3 d 14 h');
    const other = within(cards).getByText('Mesa 3').closest('button')!;
    expect(other.className).not.toContain('account-long-open');
    fireEvent.click(aged);
    const bill = await screen.findByRole('complementary', { name: 'Cuenta actual' });
    const capture = await within(bill).findByRole('button', {
      name: 'Capturar productos en esta cuenta',
    });
    expect(capture.className).toContain('primary-button');
    expect(within(bill).getByRole('button', { name: 'Cobrar · $150.00' }).className).toContain(
      'capture-checkout-outline',
    );
    expect(within(bill).getByRole('button', { name: /^Enviar a cocina/ }).className).toContain(
      'secondary-button',
    );
    expect(bill.querySelectorAll('.primary-button')).toHaveLength(1);
    // Dentro de la captura «Enviar a cocina» y «Cobrar» vuelven a ser los principales.
    fireEvent.click(capture);
    const captureBill = await screen.findByRole('complementary', { name: 'Cuenta actual' });
    await waitFor(() =>
      expect(
        within(captureBill).getByRole('button', { name: 'Cobrar · $150.00' }).className,
      ).toContain('primary-button'),
    );
    expect(
      within(captureBill).queryByRole('button', { name: 'Capturar productos en esta cuenta' }),
    ).toBeNull();
  });

  it('«Reintentar» aparece solo sin conexión, en Mesas y en la barra de la captura', async () => {
    const onRetryConnection = vi.fn();
    renderPage({ connectionLost: true, onRetryConnection });
    await screen.findByRole('complementary', { name: 'Resumen del área' });
    const before = requests.filter((entry) => entry.operation === 'restaurant_overview').length;
    fireEvent.click(screen.getByRole('button', { name: 'Reintentar' }));
    expect(onRetryConnection).toHaveBeenCalledTimes(1);
    await waitFor(() =>
      expect(
        requests.filter((entry) => entry.operation === 'restaurant_overview').length,
      ).toBeGreaterThan(before),
    );
    fireEvent.click(await screen.findByRole('button', { name: /^Mesa 2, Ocupada/ }));
    const panel = await screen.findByRole('complementary', { name: 'Panel de Mesa 2' });
    fireEvent.click(
      await within(panel).findByRole('button', { name: 'Capturar pedidos en esta mesa' }),
    );
    const bar = await waitFor(() => document.querySelector<HTMLElement>('.capture-topbar')!);
    expect(within(bar).queryByRole('button', { name: 'Actualizar' })).toBeNull();
    fireEvent.click(within(bar).getByRole('button', { name: 'Reintentar' }));
    expect(onRetryConnection).toHaveBeenCalledTimes(2);
  });

  it('con conexión no hay «Actualizar» ni «Reintentar» en la barra de la captura', async () => {
    renderPage();
    fireEvent.click(await screen.findByRole('button', { name: /^Mesa 2, Ocupada/ }));
    const panel = await screen.findByRole('complementary', { name: 'Panel de Mesa 2' });
    fireEvent.click(
      await within(panel).findByRole('button', { name: 'Capturar pedidos en esta mesa' }),
    );
    const bar = await waitFor(() => document.querySelector<HTMLElement>('.capture-topbar')!);
    expect(bar).toBeTruthy();
    expect(within(bar).queryByRole('button', { name: 'Actualizar' })).toBeNull();
    expect(within(bar).queryByRole('button', { name: 'Reintentar' })).toBeNull();
  });
});
