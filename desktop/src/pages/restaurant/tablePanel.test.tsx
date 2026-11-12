// @vitest-environment jsdom

import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { TableMap } from '../../components/restaurant/TableMap';
import { PendingOperationsProvider } from '../../hooks/pendingOperations';
import { OperationalPage } from './OperationalPage';

const native = vi.hoisted(() => ({ invoke: vi.fn() }));
vi.mock('@tauri-apps/api/core', () => ({ invoke: native.invoke, isTauri: () => true }));

// Etapa 6.2: panel lateral de la mesa, apertura sin formulario y datos de la cuenta durante la
// captura. Solo se sustituye el transporte nativo por un servidor simulado; los importes que se
// comprueban son los que «devuelve Go» y no coinciden con una suma hecha en el cliente.

const ids = {
  area: '00000000-0000-4000-8000-0000000000a1',
  occupied: '00000000-0000-4000-8000-0000000000b1',
  free: '00000000-0000-4000-8000-0000000000b2',
  first: '00000000-0000-4000-8000-0000000000c1',
  second: '00000000-0000-4000-8000-0000000000c2',
  steak: '00000000-0000-4000-8000-0000000000e1',
  list: '00000000-0000-4000-8000-000000000101',
  user: '00000000-0000-4000-8000-000000000201',
};

const tableNames: Record<string, string> = {
  [ids.occupied]: 'Mesa 1',
  [ids.free]: 'Mesa 2',
};

interface ServerAccount {
  id: string;
  folio: number;
  revision: number;
  mode: 'dine_in' | 'takeaway';
  tableID: string | null;
  name: string;
  customerName: string;
  phone: string;
  origin: string;
  notes: string;
  elapsed: number;
  lines: unknown[];
  draft: unknown[];
  total: string;
  paid: string;
  balance: string;
  // Etapa 7.1: Go marca liberable una cuenta abierta y vacía en sentido estricto.
  releasable: boolean;
}

let accounts: Record<string, ServerAccount>;
// Etapa 7.1: liberaciones, cambios de servicio y mesas fuera de servicio del servidor simulado.
let releases: { id: string; body: Record<string, unknown> }[];
let services: { id: string; body: Record<string, unknown> }[];
let outOfService: Set<string>;
let opens: Record<string, unknown>[];
let openRequests: number;
let saves: { id: string; body: Record<string, unknown> }[];
let replays: Map<string, unknown>;
let holdOpens: boolean;
let heldOpens: (() => void)[];
// Rechazo de Go para la próxima apertura (código interno y mensaje en español).
let openFailure: { code: string; message: string } | null;

function serverAccount(values: Partial<ServerAccount> & { id: string; folio: number }) {
  return {
    revision: 3,
    mode: 'dine_in' as const,
    tableID: ids.occupied,
    name: 'Mesa 1',
    customerName: '',
    phone: '',
    origin: '',
    notes: '',
    elapsed: 120,
    lines: [],
    draft: [],
    total: '0.00',
    paid: '0.00',
    balance: '0.00',
    releasable: false,
    ...values,
  };
}

function account(state: ServerAccount) {
  return {
    id: state.id,
    folio: state.folio,
    name: state.name,
    mode: state.mode,
    state: 'open',
    table_id: state.tableID,
    table_name: state.tableID ? tableNames[state.tableID] : null,
    price_list_id: ids.list,
    customer_name: state.customerName,
    phone: state.phone,
    origin: state.origin,
    notes: state.notes,
    scheduled_at: null,
    scheduled_local: '',
    schedule_kind: null,
    assigned_user_id: ids.user,
    assigned_name: 'Laura',
    created_at: '2026-11-08T18:30:00Z',
    updated_at: '2026-11-08T18:30:00Z',
    revision: state.revision,
    merged_into_id: null,
    elapsed_seconds: state.elapsed,
    urgency: 'normal',
    preparation_state: state.lines.length ? 'released' : 'without_items',
    payment_state: state.paid === '0.00' ? 'not_recorded' : 'partial',
    delivery_state: 'not_delivered',
    finance: {
      total: state.total,
      paid: state.paid,
      balance: state.balance,
      tips: '0.00',
      payment_state: state.paid === '0.00' ? 'not_recorded' : 'partial',
      delivery_state: 'not_delivered',
    },
    confirmed_quantity: state.lines.length,
    draft_quantity: state.draft.length,
    totals: { subtotal: state.total, discount: '0.00', total: state.total },
    releasable: state.releasable,
  };
}

function bill(state: ServerAccount) {
  return {
    account: account(state),
    draft: state.draft,
    lines: state.lines,
    server_time: '2026-11-08T19:00:00Z',
    timezone: 'America/Monterrey',
  };
}

function confirmedLine(name: string, quantity: number, total: string) {
  return {
    preparation_state: 'released',
    version_id: `version-${name}`,
    item_id: `item-${name}`,
    original_order_id: 'order',
    version: 1,
    state: 'active',
    quantity,
    request: {
      product_id: ids.steak,
      variant_id: null,
      quantity,
      customization: { ingredients: [], groups: [], components: [], notes: '' },
    },
    quote_line: {
      product_id: ids.steak,
      revision: 1,
      name,
      base_price: '0.00',
      price_origin: 'base',
      unit_price: '0.00',
      ingredient_extras: '0.00',
      group_extras: '0.00',
      component_extras: '0.00',
      notes: '',
      ingredients: [],
      groups: [],
      components: [],
      quantity,
      subtotal: total,
      discount: '0.00',
      total,
    },
    totals: { subtotal: total, discount: '0.00', total },
    created_at: '2026-11-08T19:00:00Z',
  };
}

// Como en Go: sin nombre, la cuenta toma el nombre de su mesa o «Pedido para llevar».
function neutralName(mode: string, table: string | null) {
  if (mode === 'takeaway') {
    return 'Pedido para llevar';
  }
  return table ? tableNames[table]! : 'Cuenta';
}

function overview() {
  const tables = [ids.occupied, ids.free].map((id, index) => {
    const count = Object.values(accounts).filter((entry) => entry.tableID === id).length;
    return {
      id,
      area_id: ids.area,
      name: tableNames[id],
      shape: 'rectangle',
      x: 20 + index * 200,
      y: 20,
      width: 120,
      height: 100,
      seats: index === 0 ? 4 : 6,
      active: true,
      out_of_service: outOfService.has(id),
      revision: outOfService.has(id) ? 2 : 1,
      account_count: count,
      state: outOfService.has(id) ? 'out_of_service' : count > 0 ? 'occupied' : 'free',
      occupied_since: count > 0 ? '2026-11-08T18:30:00Z' : null,
      occupied_seconds: count > 0 ? 3900 : 0,
      balance: '0.00',
    };
  });
  return {
    areas: [
      {
        id: ids.area,
        name: 'Salón',
        description: '',
        sort_order: 10,
        active: true,
        revision: 1,
        table_count: 2,
      },
    ],
    tables,
    settings: { warning_minutes: 15, overdue_minutes: 30, revision: 1 },
    timezone: 'America/Monterrey',
  };
}

function openAccount(body: Record<string, unknown>) {
  const replay = replays.get(String(body.operation_id));
  if (replay) {
    return replay;
  }
  opens.push(body);
  const folio = Object.keys(accounts).length + 1;
  const id = `00000000-0000-4000-8000-0000000009${String(folio).padStart(2, '0')}`;
  const mode = body.mode as 'dine_in' | 'takeaway';
  const table = (body.table_id as string | null) ?? null;
  accounts[id] = serverAccount({
    id,
    folio,
    revision: 1,
    mode,
    tableID: table,
    name: String(body.name ?? '').trim() || neutralName(mode, table),
    origin: String(body.origin ?? ''),
    elapsed: 0,
  });
  const result = bill(accounts[id]!);
  replays.set(String(body.operation_id), result);
  return result;
}

function respond(
  operation: string,
  id: string | null,
  body: Record<string, unknown> | null,
  query: Record<string, string> | null,
) {
  switch (operation) {
    case 'restaurant_overview':
      return overview();
    case 'restaurant_accounts': {
      const items = Object.values(accounts)
        .filter((entry) => !query?.mode || entry.mode === query.mode)
        .filter((entry) => !query?.table_id || entry.tableID === query.table_id)
        .sort((left, right) => left.folio - right.folio)
        .map(account);
      return {
        items,
        total: items.length,
        page: 1,
        page_size: 50,
        server_time: '2026-11-08T19:00:00Z',
        timezone: 'America/Monterrey',
        settings: { warning_minutes: 15, overdue_minutes: 30, revision: 1 },
      };
    }
    case 'restaurant_account':
      return bill(accounts[id!]!);
    case 'restaurant_open_account':
      if (openFailure) {
        return { status: 409, body: { error: openFailure } };
      }
      return openAccount(body!);
    case 'restaurant_release_account': {
      releases.push({ id: id!, body: body! });
      const released = { ...bill(accounts[id!]!) };
      released.account = { ...released.account, state: 'voided', releasable: false };
      delete accounts[id!];
      return released;
    }
    case 'restaurant_table_service': {
      services.push({ id: id!, body: body! });
      if (body!.out_of_service) {
        outOfService.add(id!);
      } else {
        outOfService.delete(id!);
      }
      return overview().tables.find((table) => table.id === id);
    }
    case 'restaurant_save_account': {
      saves.push({ id: id!, body: body! });
      const state = accounts[id!]!;
      if (body!.revision !== state.revision) {
        return {
          status: 409,
          body: { error: { code: 'ACCOUNT_CONFLICT', message: 'La cuenta cambió.' } },
        };
      }
      state.name = String(body!.name).trim() || neutralName(state.mode, state.tableID);
      state.customerName = String(body!.customer_name);
      state.phone = String(body!.phone);
      state.origin = String(body!.origin);
      state.notes = String(body!.notes);
      state.revision += 1;
      return bill(state);
    }
    case 'catalog_lookups':
      return {
        categories: [],
        ingredients: [],
        complements: [],
        portions: [],
        groups: [],
        'price-lists': [{ id: ids.list, name: 'Restaurante' }],
        palette: [],
        products: [
          {
            id: ids.steak,
            name: 'Filete de prueba',
            code: 'FILETE',
            kind: 'product',
            active: true,
            archived: false,
            category_id: 'categoria',
            sort_order: 10,
            variants: [],
          },
        ],
      };
    case 'catalog_products':
      return { items: [], total: 0 };
    case 'restaurant_preview':
      return {
        account_id: id,
        revision: accounts[id!]!.revision,
        fingerprint: 'huella',
        quote: null,
        combined: { subtotal: '0.00', discount: '0.00', total: '0.00' },
        pricing_policy: '',
      };
    default:
      return {};
  }
}

beforeEach(() => {
  accounts = {
    [ids.first]: serverAccount({
      id: ids.first,
      folio: 1,
      elapsed: 3900,
      lines: [confirmedLine('Filete de prueba', 2, '281.10')],
      draft: [
        {
          id: 'draft-1',
          product_id: ids.steak,
          variant_id: null,
          quantity: 1,
          customization: { ingredients: [], groups: [], components: [], notes: '' },
        },
      ],
      total: '263.40',
      paid: '100.00',
      balance: '163.40',
    }),
    [ids.second]: serverAccount({
      id: ids.second,
      folio: 2,
      revision: 7,
      name: 'Familia Ruiz',
      elapsed: 600,
    }),
  };
  opens = [];
  openRequests = 0;
  saves = [];
  replays = new Map();
  holdOpens = false;
  heldOpens = [];
  openFailure = null;
  releases = [];
  services = [];
  outOfService = new Set();
  native.invoke.mockImplementation(
    async (
      command: string,
      args: {
        operation: string;
        resourceId: string | null;
        body: Record<string, unknown> | null;
        query: Record<string, string> | null;
      },
    ) => {
      if (command !== 'api_request') return null;
      if (args.operation === 'restaurant_open_account') {
        openRequests += 1;
        if (holdOpens) {
          await new Promise<void>((resolve) => heldOpens.push(resolve));
        }
      }
      const result = respond(args.operation, args.resourceId, args.body, args.query) as {
        status?: number;
        body?: unknown;
      };
      if (result && typeof result.status === 'number' && 'body' in result) {
        return result;
      }
      return { status: 200, body: result };
    },
  );
});

afterEach(cleanup);

function renderPage(
  requestedView: 'tables' | 'takeaway' = 'tables',
  can: (permission: string) => boolean = () => true,
) {
  render(
    <PendingOperationsProvider>
      <OperationalPage
        active
        timezone="America/Monterrey"
        requestedView={requestedView}
        can={can}
      />
    </PendingOperationsProvider>,
  );
}

function pressed(element: HTMLElement) {
  return element.getAttribute('aria-pressed');
}

async function tablePanel(name: RegExp, title: string) {
  fireEvent.click(await screen.findByRole('button', { name }));
  return screen.findByRole('complementary', { name: `Panel de ${title}` });
}

describe('panel lateral de la mesa', () => {
  it('mesa libre: misma estructura que una ocupada y «Capturar» entra sin crear la cuenta', async () => {
    renderPage();
    const panel = await tablePanel(/^Mesa 2, Libre/, 'Mesa 2');
    expect(within(panel).getByRole('heading', { name: 'Mesa 2' })).toBeTruthy();
    expect(within(panel).getByText('Libre')).toBeTruthy();
    expect(within(panel).getByText('Mesa para 6')).toBeTruthy();
    // Área de cuenta vacía, totales en cero y las acciones que no aplican desactivadas.
    expect(within(panel).getByRole('heading', { name: 'Sin cuenta abierta' })).toBeTruthy();
    expect(within(panel).getByText('Total').nextSibling?.textContent).toBe('$0.00');
    expect(within(panel).getByText('Pagado').nextSibling?.textContent).toBe('$0.00');
    expect(within(panel).getByText('Saldo').nextSibling?.textContent).toBe('$0.00');
    const capture = within(panel).getByRole('button', { name: 'Capturar' });
    expect((capture as HTMLButtonElement).disabled).toBe(false);
    for (const label of ['Cobrar', 'Precuenta', 'Dividir']) {
      expect(
        (within(panel).getByRole('button', { name: label }) as HTMLButtonElement).disabled,
      ).toBe(true);
    }
    // «Capturar» está en el mismo lugar que en una mesa ocupada (primera acción principal).
    expect(capture.closest('.capture-primary-actions')?.firstElementChild).toBe(capture);
    // El mapa sigue visible y sin controles de edición.
    const map = screen.getByRole('region', { name: 'Mapa del restaurante' });
    expect(within(map).queryByText(/Arrastra o usa flechas/)).toBeNull();
    expect(screen.queryByRole('dialog')).toBeNull();
    fireEvent.click(capture);
    await screen.findByRole('heading', { name: 'Mesa 2 · Cuenta nueva' });
    // Sin productos todavía no hay cuenta: nada se envía a Go.
    expect(openRequests).toBe(0);
    expect(screen.getByText(/La cuenta se abre al guardar el primer producto/)).toBeTruthy();
    expect(screen.queryByLabelText(/Nombre o referencia de cuenta/)).toBeNull();
    expect(screen.queryByRole('button', { name: 'Datos de la cuenta' })).toBeNull();
    expect(screen.getByRole('complementary', { name: 'Cuenta actual' })).toBeTruthy();
    // Salir sin productos: la mesa sigue libre y no queda registro.
    fireEvent.click(screen.getByRole('button', { name: /Mesas/ }));
    const again = await screen.findByRole('complementary', { name: 'Panel de Mesa 2' });
    expect(within(again).getByText('Libre')).toBeTruthy();
    expect(openRequests).toBe(0);
  });

  it('mesa ocupada con varias cuentas: botones, productos, totales de Go y acciones', async () => {
    renderPage();
    const panel = await tablePanel(/^Mesa 1, Ocupada/, 'Mesa 1');
    expect(within(panel).getByText('Ocupada')).toBeTruthy();
    // Etapa 9C (§4): capacidad y tiempo abierta, cada dato con su icono.
    const facts = within(panel).getByRole('list', { name: 'Datos de la mesa' });
    expect(within(facts).getByText('Mesa para 4').querySelector('svg')).not.toBeNull();
    expect(within(facts).getByText('Abierta hace 1 h 5 min').querySelector('svg')).not.toBeNull();
    const group = within(panel).getByRole('group', { name: 'Cuentas abiertas de la mesa' });
    const first = within(group).getByRole('button', { name: 'Cuenta #1' });
    await waitFor(() => expect(pressed(first)).toBe('true'));
    expect(within(group).getByRole('button', { name: 'Cuenta #2 · Familia Ruiz' })).toBeTruthy();
    const lines = within(panel).getByRole('list', { name: 'Productos de la cuenta' });
    const sent = within(lines).getByText('2 × Filete de prueba').closest('li')!;
    expect(within(sent).getByText('En cocina')).toBeTruthy();
    expect(within(sent).getByText('$281.10')).toBeTruthy();
    const added = within(lines).getByText('1 × Filete de prueba').closest('li')!;
    expect(within(added).getByText('Nuevo')).toBeTruthy();
    // Total, pagado y saldo son los de Go (no la suma de las partidas).
    expect(within(panel).getByText('Total').nextSibling?.textContent).toBe('$263.40');
    expect(within(panel).getByText('Pagado').nextSibling?.textContent).toBe('$100.00');
    expect(within(panel).getByText('Saldo').nextSibling?.textContent).toBe('$163.40');
    expect(within(panel).getByRole('button', { name: 'Capturar' })).toBeTruthy();
    expect(within(panel).getByRole('button', { name: 'Cobrar · $163.40' })).toBeTruthy();
    expect(within(panel).getByRole('button', { name: 'Precuenta' })).toBeTruthy();
    expect(within(panel).getByRole('button', { name: 'Dividir' })).toBeTruthy();
    expect(within(panel).getByRole('button', { name: 'Nueva cuenta' })).toBeTruthy();
    // Una cuenta con productos no se libera ni se pone la mesa fuera de servicio.
    expect(within(panel).queryByRole('button', { name: 'Liberar mesa' })).toBeNull();
    expect(within(panel).queryByRole('button', { name: 'Poner fuera de servicio' })).toBeNull();
    fireEvent.click(within(panel).getByRole('button', { name: 'Más…' }));
    const menu = within(panel).getByRole('group', { name: 'Más acciones de la cuenta' });
    for (const label of [
      'Datos de cuenta',
      'Cambiar mesa',
      'Transferir productos',
      'Combinar cuentas',
    ]) {
      expect(within(menu).getByRole('button', { name: label })).toBeTruthy();
    }
    fireEvent.click(within(menu).getByRole('button', { name: 'Datos de cuenta' }));
    // La acción ocupa el lugar del mapa; el panel de la mesa permanece.
    await screen.findByRole('region', { name: 'Datos de la cuenta #1' });
    expect(screen.queryByRole('region', { name: 'Mapa del restaurante' })).toBeNull();
    expect(screen.getByRole('complementary', { name: 'Panel de Mesa 1' })).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: 'Volver' }));
    await screen.findByRole('region', { name: 'Mapa del restaurante' });
    // Otra cuenta de la mesa cambia el panel; «Capturar» entra a ella.
    fireEvent.click(within(group).getByRole('button', { name: 'Cuenta #2 · Familia Ruiz' }));
    await waitFor(() =>
      expect(within(panel).getByRole('heading', { name: 'Cuenta #2 · Familia Ruiz' })).toBeTruthy(),
    );
    expect(within(panel).getByText('La cuenta aún no tiene productos.')).toBeTruthy();
    fireEvent.click(within(panel).getByRole('button', { name: 'Capturar' }));
    await screen.findByRole('heading', { name: 'Mesa 1 · Cuenta #2' });
    expect(opens).toHaveLength(0);
    // «Mesas» vuelve al mapa con el panel de la misma mesa.
    fireEvent.click(screen.getByRole('button', { name: /Mesas/ }));
    expect(await screen.findByRole('complementary', { name: 'Panel de Mesa 1' })).toBeTruthy();
  });

  it('tocar otra mesa cambia el panel sin cerrar el mapa', async () => {
    renderPage();
    await tablePanel(/^Mesa 1, Ocupada/, 'Mesa 1');
    const panel = await tablePanel(/^Mesa 2, Libre/, 'Mesa 2');
    expect(within(panel).getByRole('button', { name: 'Capturar' })).toBeTruthy();
    expect(screen.queryByRole('complementary', { name: 'Panel de Mesa 1' })).toBeNull();
    expect(screen.getByRole('region', { name: 'Mapa del restaurante' })).toBeTruthy();
  });

  it('«Nueva cuenta» entra a capturar en la mesa ocupada sin crear la cuenta', async () => {
    renderPage();
    const panel = await tablePanel(/^Mesa 1, Ocupada/, 'Mesa 1');
    await within(panel).findByRole('button', { name: 'Capturar' });
    fireEvent.click(within(panel).getByRole('button', { name: 'Nueva cuenta' }));
    await screen.findByRole('heading', { name: 'Mesa 1 · Cuenta nueva' });
    expect(openRequests).toBe(0);
    // Las cuentas abiertas de la mesa siguen a un toque; «Nueva cuenta» no se repite.
    const group = screen.getByRole('group', { name: 'Cuentas de la mesa' });
    expect(within(group).queryByRole('button', { name: 'Nueva cuenta' })).toBeNull();
    fireEvent.click(within(group).getByRole('button', { name: 'Cuenta #1' }));
    await screen.findByRole('heading', { name: 'Mesa 1 · Cuenta #1' });
    expect(openRequests).toBe(0);
    // Desde la captura de una cuenta existente, «Nueva cuenta» vuelve a una cuenta pendiente.
    fireEvent.click(
      within(screen.getByRole('group', { name: 'Cuentas de la mesa' })).getByRole('button', {
        name: 'Nueva cuenta',
      }),
    );
    await screen.findByRole('heading', { name: 'Mesa 1 · Cuenta nueva' });
    expect(openRequests).toBe(0);
  });

  it('un doble toque en «Capturar» no abre cuentas', async () => {
    renderPage();
    const panel = await tablePanel(/^Mesa 2, Libre/, 'Mesa 2');
    const capture = within(panel).getByRole('button', { name: 'Capturar' });
    fireEvent.click(capture);
    fireEvent.click(capture);
    await screen.findByRole('heading', { name: 'Mesa 2 · Cuenta nueva' });
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 50));
    });
    expect(openRequests).toBe(0);
    expect(Object.values(accounts).filter((entry) => entry.tableID === ids.free)).toHaveLength(0);
  });

  it('un rechazo de Go al abrir se avisa en español, sin código interno, y no se cierra solo', async () => {
    // Para llevar sigue abriendo al instante (etapa 6.2): su rechazo es un aviso persistente.
    renderPage('takeaway');
    openFailure = {
      code: 'TAKEAWAY_DISABLED',
      message: 'No se pudo abrir el pedido: revisa los datos del establecimiento.',
    };
    fireEvent.click(await screen.findByRole('button', { name: 'Nuevo pedido' }));
    const alert = await screen.findByRole('alert', { name: 'Error' });
    expect(alert.textContent).toContain(
      'No se pudo abrir el pedido: revisa los datos del establecimiento.',
    );
    expect(alert.textContent).not.toContain('TAKEAWAY_DISABLED');
    // No queda un banner superior duplicado ni un panel de recuperación: el rechazo fue definitivo.
    expect(document.querySelector('.operational-page > .notice')).toBeNull();
    expect(document.querySelector('.operation-recovery')).toBeNull();
    expect(screen.queryByRole('heading', { name: /Pedido #/ })).toBeNull();
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 4500));
    });
    expect(screen.queryByRole('alert', { name: 'Error' })).not.toBeNull();
  }, 10000);
});

describe('liberar mesa y fuera de servicio (etapa 7.1)', () => {
  it('«Liberar mesa» solo aparece si Go marcó la cuenta liberable y pide confirmación', async () => {
    accounts[ids.second]!.releasable = true;
    renderPage();
    const panel = await tablePanel(/^Mesa 1, Ocupada/, 'Mesa 1');
    const group = within(panel).getByRole('group', { name: 'Cuentas abiertas de la mesa' });
    await waitFor(() =>
      expect(pressed(within(group).getByRole('button', { name: 'Cuenta #1' }))).toBe('true'),
    );
    expect(within(panel).queryByRole('button', { name: 'Liberar mesa' })).toBeNull();
    fireEvent.click(within(group).getByRole('button', { name: 'Cuenta #2 · Familia Ruiz' }));
    const release = await within(panel).findByRole('button', { name: 'Liberar mesa' });
    // Etapa 9D: la anulación con motivo libre salió de la interfaz; «Cancelar cuenta» (motivo del
    // catálogo) sigue en «Más…» aunque la cuenta también se pueda liberar sin motivo.
    fireEvent.click(within(panel).getByRole('button', { name: 'Más…' }));
    const menu = within(panel).getByRole('group', { name: 'Más acciones de la cuenta' });
    expect(within(menu).queryByRole('button', { name: /^Anular/ })).toBeNull();
    expect(within(menu).getByRole('button', { name: 'Cancelar cuenta' })).toBeTruthy();
    fireEvent.click(release);
    const dialog = await screen.findByRole('alertdialog', { name: '¿Liberar Mesa 1?' });
    expect(dialog.textContent).toContain('Cuenta #2');
    // La mesa conserva otra cuenta abierta: no se anuncia como libre.
    expect(dialog.textContent).toContain('las demás cuentas de la mesa no cambian');
    fireEvent.click(within(dialog).getByRole('button', { name: 'Conservar la cuenta' }));
    expect(releases).toHaveLength(0);
    fireEvent.click(within(panel).getByRole('button', { name: 'Liberar mesa' }));
    fireEvent.click(
      within(await screen.findByRole('alertdialog', { name: '¿Liberar Mesa 1?' })).getByRole(
        'button',
        { name: 'Liberar mesa' },
      ),
    );
    await waitFor(() => expect(releases).toHaveLength(1));
    expect(releases[0]!.id).toBe(ids.second);
    expect(releases[0]!.body).toEqual(expect.objectContaining({ revision: 7 }));
    expect(typeof releases[0]!.body.operation_id).toBe('string');
    expect(
      (await screen.findByRole('status', { name: 'Operación completada' })).textContent,
    ).toContain('Cuenta #2 · Familia Ruiz liberada.');
  });

  it('una cuenta liberable sin permiso de abrir cuentas no muestra «Liberar mesa»', async () => {
    accounts[ids.second]!.releasable = true;
    renderPage('tables', (permission) => permission !== 'accounts.open');
    const panel = await tablePanel(/^Mesa 1, Ocupada/, 'Mesa 1');
    const group = within(panel).getByRole('group', { name: 'Cuentas abiertas de la mesa' });
    await waitFor(() =>
      expect(pressed(within(group).getByRole('button', { name: 'Cuenta #1' }))).toBe('true'),
    );
    fireEvent.click(within(group).getByRole('button', { name: 'Cuenta #2 · Familia Ruiz' }));
    await within(panel).findByRole('heading', { name: 'Cuenta #2 · Familia Ruiz' });
    expect(within(panel).queryByRole('button', { name: 'Liberar mesa' })).toBeNull();
  });

  it('«Poner fuera de servicio» en una mesa libre y «Habilitar» después, con su permiso', async () => {
    renderPage();
    const panel = await tablePanel(/^Mesa 2, Libre/, 'Mesa 2');
    fireEvent.click(within(panel).getByRole('button', { name: 'Poner fuera de servicio' }));
    await waitFor(() => expect(services).toHaveLength(1));
    expect(services[0]).toEqual({
      id: ids.free,
      body: expect.objectContaining({ revision: 1, out_of_service: true }),
    });
    expect(
      (await screen.findByRole('status', { name: 'Operación completada' })).textContent,
    ).toContain('Mesa 2 quedó fuera de servicio.');
    const disabledPanel = await screen.findByRole('complementary', { name: 'Panel de Mesa 2' });
    await within(disabledPanel).findByText(
      'La mesa está fuera de servicio y no admite cuentas nuevas.',
    );
    expect(within(disabledPanel).getByText('Fuera de servicio')).toBeTruthy();
    expect(
      (within(disabledPanel).getByRole('button', { name: 'Capturar' }) as HTMLButtonElement)
        .disabled,
    ).toBe(true);
    fireEvent.click(within(disabledPanel).getByRole('button', { name: 'Habilitar' }));
    await waitFor(() => expect(services).toHaveLength(2));
    expect(services[1]!.body).toEqual(
      expect.objectContaining({ revision: 2, out_of_service: false }),
    );
  });

  it('sin administrar mesas no se ofrece el cambio de servicio', async () => {
    renderPage('tables', (permission) => permission !== 'restaurant.tables.manage');
    const panel = await tablePanel(/^Mesa 2, Libre/, 'Mesa 2');
    expect(within(panel).queryByRole('button', { name: 'Poner fuera de servicio' })).toBeNull();
    expect(within(panel).getByRole('button', { name: 'Capturar' })).toBeTruthy();
  });
});

describe('mapa de mesas y leyenda (etapa 7.1)', () => {
  const area = {
    id: ids.area,
    name: 'Salón',
    description: '',
    sort_order: 10,
    active: true,
    revision: 1,
    table_count: 5,
  };
  const base = {
    area_id: ids.area,
    shape: 'rectangle' as const,
    y: 20,
    width: 120,
    height: 100,
    seats: 4,
    active: true,
    out_of_service: false,
    revision: 1,
    account_count: 1,
    occupied_since: '2026-11-08T18:30:00Z',
    occupied_seconds: 2700,
    balance: '163.40',
  };

  function renderMap() {
    render(
      <TableMap
        areas={[area]}
        tables={[
          {
            ...base,
            id: 'libre',
            name: 'Mesa 1',
            x: 20,
            state: 'free',
            account_count: 0,
            occupied_since: null,
            occupied_seconds: 0,
            balance: '0.00',
          },
          { ...base, id: 'ocupada', name: 'Mesa 2', x: 160, state: 'occupied', balance: '263.40' },
          { ...base, id: 'precuenta', name: 'Mesa 3', x: 300, state: 'awaiting_payment' },
          {
            ...base,
            id: 'parcial',
            name: 'Mesa 4',
            x: 440,
            state: 'partial_payment',
            account_count: 2,
          },
          {
            ...base,
            id: 'servicio',
            name: 'Mesa 5',
            x: 580,
            state: 'out_of_service',
            out_of_service: true,
            account_count: 0,
            occupied_since: null,
            occupied_seconds: 0,
            balance: '0.00',
          },
        ]}
        areaID={ids.area}
        onAreaChange={() => undefined}
        onSelect={() => undefined}
      />,
    );
  }

  it('cada estado se distingue por texto o icono, no solo por color', () => {
    renderMap();
    const free = screen.getByRole('button', { name: 'Mesa 1, Libre, 0 cuentas, mesa para 4' });
    expect(free.className).toContain('table-free');
    expect(within(free).getByText('Libre')).toBeTruthy();
    // Ocupada: tiempo calculado por Go («45 min»).
    const occupied = screen.getByRole('button', {
      name: 'Mesa 2, Ocupada, 1 cuenta, 45 min, mesa para 4',
    });
    expect(occupied.className).toContain('table-occupied');
    expect(within(occupied).getByText('Ocupada').querySelector('svg')).not.toBeNull();
    expect(within(occupied).getByText('45 min')).toBeTruthy();
    // Esperando pago: icono de recibo y su estado en texto.
    const awaiting = screen.getByRole('button', {
      name: 'Mesa 3, Esperando pago, 1 cuenta, 45 min, precuenta impresa, mesa para 4',
    });
    expect(awaiting.className).toContain('table-awaiting_payment');
    expect(within(awaiting).getByText('Esperando pago').querySelector('svg')).not.toBeNull();
    // Pago parcial: el saldo de Go y, con varias cuentas, cuántas son.
    const partial = screen.getByRole('button', {
      name: 'Mesa 4, Pago parcial, 2 cuentas, 45 min, saldo $163.40, mesa para 4',
    });
    expect(partial.className).toContain('table-partial_payment');
    expect(within(partial).getByText('Saldo $163.40')).toBeTruthy();
    expect(within(partial).getByText('2 cuentas')).toBeTruthy();
    // Fuera de servicio: candado y sin datos de ocupación.
    const blocked = screen.getByRole('button', { name: 'Mesa 5, Fuera de servicio, 0 cuentas' });
    expect(blocked.className).toContain('table-out_of_service');
    expect(within(blocked).getByText('Fuera de servicio').querySelector('svg')).not.toBeNull();
    expect(blocked.querySelector('.table-tile-details')).toBeNull();
  });

  it('mosaicos más legibles (etapa 9C): icono de mesa, nombre grande, tiempo y capacidad con icono', () => {
    renderMap();
    const occupied = screen.getByRole('button', { name: /^Mesa 2, Ocupada/ });
    const name = within(occupied).getByText('Mesa 2');
    expect(name.tagName).toBe('STRONG');
    expect(name.parentElement?.querySelector('svg.table-tile-icon')).not.toBeNull();
    const time = occupied.querySelector('.table-tile-time')!;
    expect(time.textContent).toBe('45 min');
    expect(time.querySelector('svg')).not.toBeNull();
    const seats = occupied.querySelector('.table-tile-seats')!;
    expect(seats.textContent).toBe('4');
    expect(seats.querySelector('svg')).not.toBeNull();
    // Una mesa libre muestra su capacidad, pero no tiempo.
    const free = screen.getByRole('button', { name: /^Mesa 1, Libre/ });
    expect(free.querySelector('.table-tile-time')).toBeNull();
    expect(free.querySelector('.table-tile-seats')?.textContent).toBe('4');
  });

  it('la leyenda muestra los cinco estados y qué indican los iconos', () => {
    renderMap();
    const legend = screen.getByRole('list', { name: 'Estados de las mesas' });
    const items = within(legend).getAllByRole('listitem');
    expect(items.map((item) => item.textContent)).toEqual([
      'Libre',
      'Ocupada',
      'Esperando pago',
      'Pago parcial',
      'Fuera de servicio',
    ]);
    for (const state of [
      'free',
      'occupied',
      'awaiting_payment',
      'partial_payment',
      'out_of_service',
    ]) {
      expect(legend.querySelector(`.legend-${state}`)).not.toBeNull();
    }
    const data = screen.getByRole('list', { name: 'Datos de cada mesa' });
    expect(
      within(data)
        .getAllByRole('listitem')
        .map((item) => item.textContent),
    ).toEqual(['Tiempo ocupada', 'Capacidad (lugares)']);
  });
});

describe('buscador de mesas y selector de área (etapa 9C, §4)', () => {
  const salon = {
    id: 'salon',
    name: 'Salón',
    description: '',
    sort_order: 10,
    active: true,
    revision: 1,
    table_count: 3,
  };
  const terrace = { ...salon, id: 'terraza', name: 'Terraza', sort_order: 20, table_count: 2 };
  const closed = { ...salon, id: 'cerrada', name: 'Privado', sort_order: 30, active: false };
  const table = {
    shape: 'rectangle' as const,
    x: 20,
    y: 20,
    width: 120,
    height: 100,
    seats: 4,
    active: true,
    out_of_service: false,
    revision: 1,
    account_count: 0,
    state: 'free' as const,
    occupied_since: null,
    occupied_seconds: 0,
    balance: '0.00',
  };
  const tables = [
    { ...table, id: 't1', area_id: 'salon', name: 'Mesa 1' },
    { ...table, id: 't5', area_id: 'salon', name: 'Mesa 5', x: 200 },
    { ...table, id: 't15', area_id: 'salon', name: 'Mesa 15', x: 380 },
    { ...table, id: 't25', area_id: 'terraza', name: 'Mesa 25' },
    { ...table, id: 'tb', area_id: 'terraza', name: 'Barra', x: 200 },
    { ...table, id: 'off', area_id: 'salon', name: 'Mesa 50', active: false },
    { ...table, id: 'hidden', area_id: 'cerrada', name: 'Mesa 51' },
  ];

  function renderSearch(onSelect = vi.fn(), onAreaChange = vi.fn()) {
    render(
      <TableMap
        areas={[salon, terrace, closed]}
        tables={tables}
        areaID="salon"
        onAreaChange={onAreaChange}
        onSelect={onSelect}
      />,
    );
    return { onSelect, onAreaChange };
  }

  function search() {
    return screen.getByRole('searchbox', { name: 'Buscar mesa por nombre, número o área' });
  }

  function found() {
    return within(screen.getByRole('group', { name: 'Mesas encontradas' }))
      .getAllByRole('button')
      .map((button) => button.getAttribute('aria-label'));
  }

  it('el encabezado tiene el selector de área y el buscador marcado para ⌘F / Ctrl+F', () => {
    renderSearch();
    const areasGroup = screen.getByRole('group', { name: 'Áreas del restaurante' });
    // Solo las áreas activas; la elegida, marcada.
    expect(
      within(areasGroup)
        .getAllByRole('button')
        .map((button) => button.textContent),
    ).toEqual(['Salón', 'Terraza']);
    expect(
      within(areasGroup).getByRole('button', { name: 'Salón' }).getAttribute('aria-pressed'),
    ).toBe('true');
    expect(search().hasAttribute('data-screen-search')).toBe(true);
    expect(screen.getByRole('search', { name: 'Mesas' })).toBeTruthy();
    // Sin escribir no hay resultados.
    expect(screen.queryByRole('group', { name: 'Mesas encontradas' })).toBeNull();
  });

  it('«5» encuentra la Mesa 5 y no la 15; busca en todas las áreas activas, también por área', () => {
    const { onSelect } = renderSearch();
    fireEvent.change(search(), { target: { value: '5' } });
    expect(found()).toEqual(['Mesa 5, Salón, Libre']);
    expect(screen.getByRole('status').textContent).toBe('1 mesa encontrada');
    fireEvent.change(search(), { target: { value: 'mesa 2' } });
    expect(found()).toEqual(['Mesa 25, Terraza, Libre']);
    fireEvent.change(search(), { target: { value: 'terraza' } });
    expect(found()).toEqual(['Mesa 25, Terraza, Libre', 'Barra, Terraza, Libre']);
    // Sin acentos ni mayúsculas; las mesas desactivadas y las de áreas inactivas no aparecen.
    fireEvent.change(search(), { target: { value: 'SALON 5' } });
    expect(found()).toEqual(['Mesa 5, Salón, Libre']);
    fireEvent.change(search(), { target: { value: 'zz' } });
    expect(screen.queryByRole('group', { name: 'Mesas encontradas' })).toBeNull();
    expect(screen.getByRole('status').textContent).toBe('Ninguna mesa coincide con «zz».');
    // Elegir un resultado de otra área lo informa con la mesa (la página cambia de área) y limpia
    // la búsqueda.
    fireEvent.change(search(), { target: { value: 'barra' } });
    fireEvent.click(screen.getByRole('button', { name: 'Barra, Terraza, Libre' }));
    expect(onSelect).toHaveBeenCalledWith(
      expect.objectContaining({ id: 'tb', area_id: 'terraza' }),
    );
    expect((search() as HTMLInputElement).value).toBe('');
  });

  it('Intro elige la primera coincidencia y Escape borra lo escrito', () => {
    const { onSelect } = renderSearch();
    fireEvent.change(search(), { target: { value: '1' } });
    // «1» empieza «1» y «15»: primero el nombre que coincide completo.
    expect(found()).toEqual(['Mesa 1, Salón, Libre', 'Mesa 15, Salón, Libre']);
    fireEvent.keyDown(search(), { key: 'Escape' });
    expect((search() as HTMLInputElement).value).toBe('');
    fireEvent.change(search(), { target: { value: '15' } });
    fireEvent.keyDown(search(), { key: 'Enter' });
    expect(onSelect).toHaveBeenCalledWith(expect.objectContaining({ id: 't15' }));
    // El foco pasa al mosaico elegido: Escape y los atajos ya no están dentro de un campo.
    expect(document.activeElement).toBe(screen.getByRole('button', { name: /^Mesa 15, Libre/ }));
    // El botón de borrar aparece solo con texto.
    expect(screen.queryByRole('button', { name: 'Borrar la búsqueda de mesas' })).toBeNull();
    fireEvent.change(search(), { target: { value: 'me' } });
    fireEvent.click(screen.getByRole('button', { name: 'Borrar la búsqueda de mesas' }));
    expect((search() as HTMLInputElement).value).toBe('');
  });

  it('en la pantalla, elegir una mesa encontrada abre su panel como tocarla en el mapa', async () => {
    renderPage();
    const box = await screen.findByRole('searchbox', {
      name: 'Buscar mesa por nombre, número o área',
    });
    fireEvent.change(box, { target: { value: 'mesa 2' } });
    fireEvent.click(screen.getByRole('button', { name: 'Mesa 2, Salón, Libre' }));
    const panel = await screen.findByRole('complementary', { name: 'Panel de Mesa 2' });
    expect(within(panel).getByRole('button', { name: 'Capturar' })).toBeTruthy();
    expect(
      screen.getByRole('button', { name: /^Mesa 2, Libre/ }).getAttribute('aria-pressed'),
    ).toBe('true');
  });
});

describe('datos de la cuenta durante la captura', () => {
  it('«Datos de la cuenta» edita nombre opcional, observaciones y lista con la edición existente', async () => {
    renderPage();
    const panel = await tablePanel(/^Mesa 1, Ocupada/, 'Mesa 1');
    const group = within(panel).getByRole('group', { name: 'Cuentas abiertas de la mesa' });
    await waitFor(() =>
      expect(pressed(within(group).getByRole('button', { name: 'Cuenta #1' }))).toBe('true'),
    );
    fireEvent.click(within(group).getByRole('button', { name: 'Cuenta #2 · Familia Ruiz' }));
    await within(panel).findByRole('heading', { name: 'Cuenta #2 · Familia Ruiz' });
    fireEvent.click(within(panel).getByRole('button', { name: 'Capturar' }));
    await screen.findByRole('heading', { name: 'Mesa 1 · Cuenta #2' });
    fireEvent.click(screen.getByRole('button', { name: 'Datos de la cuenta' }));
    const details = await screen.findByRole('region', { name: 'Datos de la cuenta #2' });
    // Sin comensales (pendiente de migración) y sin pedir datos obligatorios.
    expect(within(details).queryByText(/comensales/i)).toBeNull();
    const name = within(details).getByLabelText(
      'Nombre de la cuenta (opcional)',
    ) as HTMLInputElement;
    expect(name.value).toBe('Familia Ruiz');
    expect(name.required).toBe(false);
    expect(within(details).getByLabelText('Lista de precios para nuevas capturas')).toBeTruthy();
    // La cuenta sigue a la vista mientras se editan sus datos.
    expect(screen.getByRole('complementary', { name: 'Cuenta actual' })).toBeTruthy();
    fireEvent.change(name, { target: { value: '' } });
    fireEvent.change(within(details).getByLabelText('Observaciones'), {
      target: { value: 'Junto a la ventana' },
    });
    fireEvent.click(within(details).getByRole('button', { name: 'Guardar datos de la cuenta' }));
    await waitFor(() => expect(saves).toHaveLength(1));
    expect(saves[0]!.body).toEqual(
      expect.objectContaining({
        name: '',
        notes: 'Junto a la ventana',
        revision: 7,
        table_id: ids.occupied,
        assigned_user_id: ids.user,
        price_list_id: ids.list,
        scheduled_local: '',
        schedule_kind: '',
      }),
    );
    expect(typeof saves[0]!.body.operation_id).toBe('string');
    // La confirmación es un aviso del sistema único de mensajes.
    expect(
      (await screen.findByRole('status', { name: 'Operación completada' })).textContent,
    ).toContain('Datos de la cuenta guardados');
    // Go restauró la referencia neutra: la interfaz la trata como «sin nombre».
    await screen.findByRole('region', { name: 'Productos' });
    await waitFor(() => expect(accounts[ids.second]!.name).toBe('Mesa 1'));
    const bar = document.querySelector('.capture-topbar')!;
    await waitFor(() => expect(bar.textContent).not.toContain('Familia Ruiz'));
    // Tocar el nombre de la cuenta también abre sus datos, con el nombre vacío.
    fireEvent.click(screen.getByRole('button', { name: 'Mesa 1 · Cuenta #2' }));
    const reopened = await screen.findByRole('region', { name: 'Datos de la cuenta #2' });
    expect(
      (within(reopened).getByLabelText('Nombre de la cuenta (opcional)') as HTMLInputElement).value,
    ).toBe('');
  });

  it('sin permiso de listas de precios no se muestra el selector de lista', async () => {
    renderPage('tables', (permission) => permission !== 'orders.price_list.manage');
    const panel = await tablePanel(/^Mesa 1, Ocupada/, 'Mesa 1');
    fireEvent.click(await within(panel).findByRole('button', { name: 'Capturar' }));
    fireEvent.click(await screen.findByRole('button', { name: 'Datos de la cuenta' }));
    const details = await screen.findByRole('region', { name: 'Datos de la cuenta #1' });
    expect(within(details).queryByLabelText('Lista de precios para nuevas capturas')).toBeNull();
  });
});

describe('para llevar sin formulario', () => {
  it('«Nuevo pedido» entra a la captura; «Datos del pedido» y aviso «Pedido sin nombre»', async () => {
    renderPage('takeaway');
    fireEvent.click(await screen.findByRole('button', { name: 'Nuevo pedido' }));
    await screen.findByRole('heading', { name: 'Para llevar · Pedido #3' });
    expect(opens).toEqual([
      expect.objectContaining({
        name: '',
        mode: 'takeaway',
        table_id: null,
        origin: 'Para llevar',
      }),
    ]);
    expect(screen.queryByLabelText('Nombre del cliente (opcional)')).toBeNull();
    // Antes de enviar a cocina no hay aviso; el nombre se sugiere sin bloquear.
    expect(screen.queryByText(/Pedido sin nombre/)).toBeNull();
    expect(screen.getByText(/Sin nombre del cliente/)).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: 'Datos del pedido' }));
    const details = await screen.findByRole('region', { name: 'Datos del pedido #3' });
    expect(within(details).getByLabelText('Teléfono (opcional)')).toBeTruthy();
    const origins = within(details).getByRole('group', { name: 'Origen del pedido' });
    expect(pressed(within(origins).getByRole('button', { name: 'Para llevar' }))).toBe('true');
    fireEvent.click(within(origins).getByRole('button', { name: 'Ventanilla' }));
    expect(within(details).getByRole('button', { name: /Pedido programado/ })).toBeTruthy();
    fireEvent.click(within(details).getByRole('button', { name: 'Guardar datos del pedido' }));
    await waitFor(() => expect(saves).toHaveLength(1));
    expect(saves[0]!.body).toEqual(
      expect.objectContaining({ customer_name: '', origin: 'Ventanilla', table_id: null }),
    );
    await screen.findByRole('region', { name: 'Productos' });
    // El pedido se envía a cocina sin nombre: aviso discreto, sin bloquear, con su número.
    const order = Object.values(accounts).find((entry) => entry.mode === 'takeaway')!;
    order.lines = [confirmedLine('Filete de prueba', 1, '150.00')];
    order.total = '150.00';
    order.balance = '150.00';
    order.revision += 1;
    fireEvent.click(screen.getByRole('button', { name: 'Actualizar' }));
    await screen.findByText('Pedido sin nombre · se identifica como Pedido #3');
    fireEvent.click(screen.getByRole('button', { name: 'Datos del pedido' }));
    const again = await screen.findByRole('region', { name: 'Datos del pedido #3' });
    fireEvent.change(within(again).getByLabelText('Nombre del cliente (opcional)'), {
      target: { value: 'Ana' },
    });
    fireEvent.click(within(again).getByRole('button', { name: 'Guardar datos del pedido' }));
    await waitFor(() => expect(saves).toHaveLength(2));
    await waitFor(() =>
      expect(screen.queryByText('Pedido sin nombre · se identifica como Pedido #3')).toBeNull(),
    );
    expect(document.querySelector('.capture-topbar')!.textContent).toContain('Ana');
  });
});
