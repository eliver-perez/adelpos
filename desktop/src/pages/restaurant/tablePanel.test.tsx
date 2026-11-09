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
}

let accounts: Record<string, ServerAccount>;
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
      out_of_service: false,
      revision: 1,
      account_count: count,
      state: count > 0 ? 'occupied' : 'free',
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
  it('mesa libre: «Abrir cuenta y capturar» abre sin formulario y entra directo a la captura', async () => {
    renderPage();
    const panel = await tablePanel(/^Mesa 2, Libre/, 'Mesa 2');
    expect(within(panel).getByRole('heading', { name: 'Mesa 2' })).toBeTruthy();
    expect(within(panel).getByText('Libre')).toBeTruthy();
    expect(within(panel).getByText('Mesa para 6')).toBeTruthy();
    // El mapa sigue visible y sin controles de edición.
    const map = screen.getByRole('region', { name: 'Mapa del restaurante' });
    expect(within(map).queryByText(/Arrastra o usa flechas/)).toBeNull();
    expect(screen.queryByRole('dialog')).toBeNull();
    fireEvent.click(within(panel).getByRole('button', { name: 'Abrir cuenta y capturar' }));
    await screen.findByRole('heading', { name: 'Mesa 2 · Cuenta #3' });
    expect(opens).toHaveLength(1);
    // Sin nombre, lista ni observaciones: Go asigna la lista predeterminada y la referencia neutra.
    expect(opens[0]).toEqual(
      expect.objectContaining({
        name: '',
        mode: 'dine_in',
        table_id: ids.free,
        price_list_id: null,
        notes: '',
        assigned_user_id: null,
      }),
    );
    expect(typeof opens[0]!.operation_id).toBe('string');
    expect(screen.queryByLabelText(/Nombre o referencia de cuenta/)).toBeNull();
    // La referencia neutra («Mesa 2») no se muestra como nombre capturado.
    expect(screen.getByText(/Mesero: Laura · Mesa para 6 · Abierta/).textContent).not.toMatch(
      /Mesa 2$/,
    );
    expect(screen.getByRole('complementary', { name: 'Cuenta actual' })).toBeTruthy();
  });

  it('mesa ocupada con varias cuentas: botones, productos, totales de Go y acciones', async () => {
    renderPage();
    const panel = await tablePanel(/^Mesa 1, Ocupada/, 'Mesa 1');
    expect(within(panel).getByText('Ocupada')).toBeTruthy();
    expect(within(panel).getByText('Mesa para 4 · Abierta hace 1 h 5 min')).toBeTruthy();
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
    expect(within(panel).getByRole('button', { name: 'Abrir cuenta y capturar' })).toBeTruthy();
    expect(screen.queryByRole('complementary', { name: 'Panel de Mesa 1' })).toBeNull();
    expect(screen.getByRole('region', { name: 'Mapa del restaurante' })).toBeTruthy();
  });

  it('«Nueva cuenta» abre otra cuenta en la mesa ocupada y entra a capturar', async () => {
    renderPage();
    const panel = await tablePanel(/^Mesa 1, Ocupada/, 'Mesa 1');
    await within(panel).findByRole('button', { name: 'Capturar' });
    fireEvent.click(within(panel).getByRole('button', { name: 'Nueva cuenta' }));
    await screen.findByRole('heading', { name: 'Mesa 1 · Cuenta #3' });
    expect(opens).toEqual([expect.objectContaining({ name: '', table_id: ids.occupied })]);
    // Desde la captura, «Nueva cuenta» abre otra en la misma mesa sin formulario.
    const group = screen.getByRole('group', { name: 'Cuentas de la mesa' });
    await waitFor(() =>
      expect(pressed(within(group).getByRole('button', { name: 'Cuenta #3' }))).toBe('true'),
    );
    fireEvent.click(within(group).getByRole('button', { name: 'Nueva cuenta' }));
    await screen.findByRole('heading', { name: 'Mesa 1 · Cuenta #4' });
    expect(opens).toHaveLength(2);
    expect(opens[1]).toEqual(expect.objectContaining({ table_id: ids.occupied }));
  });

  it('un doble toque no abre una segunda cuenta ni genera otro operation_id', async () => {
    renderPage();
    const panel = await tablePanel(/^Mesa 2, Libre/, 'Mesa 2');
    holdOpens = true;
    const open = within(panel).getByRole('button', { name: 'Abrir cuenta y capturar' });
    fireEvent.click(open);
    fireEvent.click(open);
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 50));
    });
    fireEvent.click(open);
    expect(openRequests).toBe(1);
    holdOpens = false;
    heldOpens.forEach((resolve) => resolve());
    await screen.findByRole('heading', { name: 'Mesa 2 · Cuenta #3' });
    expect(openRequests).toBe(1);
    expect(opens).toHaveLength(1);
    expect(Object.values(accounts).filter((entry) => entry.tableID === ids.free)).toHaveLength(1);
    expect(screen.queryByText(/Primero recupera|Espera a que termine/)).toBeNull();
  });

  it('un rechazo de Go al abrir se avisa en español, sin código interno, y no se cierra solo', async () => {
    renderPage();
    const panel = await tablePanel(/^Mesa 2, Libre/, 'Mesa 2');
    openFailure = {
      code: 'TABLE_OUT_OF_SERVICE',
      message: 'La mesa está fuera de servicio y no admite cuentas nuevas.',
    };
    fireEvent.click(within(panel).getByRole('button', { name: 'Abrir cuenta y capturar' }));
    const alert = await screen.findByRole('alert', { name: 'Error' });
    expect(alert.textContent).toContain(
      'La mesa está fuera de servicio y no admite cuentas nuevas.',
    );
    expect(alert.textContent).not.toContain('TABLE_OUT_OF_SERVICE');
    // No queda un banner superior duplicado ni un panel de recuperación: el rechazo fue definitivo.
    expect(document.querySelector('.operational-page > .notice')).toBeNull();
    expect(document.querySelector('.operation-recovery')).toBeNull();
    expect(screen.queryByRole('heading', { name: 'Mesa 2 · Cuenta #3' })).toBeNull();
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 4500));
    });
    expect(screen.queryByRole('alert', { name: 'Error' })).not.toBeNull();
  }, 10000);
});

describe('contadores del mapa', () => {
  it('concuerda «1 cuenta» y «2 cuentas» en el texto y en el nombre accesible', () => {
    const area = {
      id: ids.area,
      name: 'Salón',
      description: '',
      sort_order: 10,
      active: true,
      revision: 1,
      table_count: 2,
    };
    const table = {
      area_id: ids.area,
      shape: 'rectangle' as const,
      x: 20,
      y: 20,
      width: 120,
      height: 100,
      seats: 4,
      active: true,
      out_of_service: false,
      revision: 1,
      state: 'occupied' as const,
    };
    render(
      <TableMap
        areas={[area]}
        tables={[
          { ...table, id: ids.occupied, name: 'Mesa 1', account_count: 1 },
          { ...table, id: ids.free, name: 'Mesa 2', x: 300, account_count: 2 },
        ]}
        areaID={ids.area}
        onAreaChange={() => undefined}
        onSelect={() => undefined}
      />,
    );
    const single = screen.getByRole('button', { name: 'Mesa 1, Ocupada, 1 cuenta' });
    expect(within(single).getByText('1 cuenta')).toBeTruthy();
    const double = screen.getByRole('button', { name: 'Mesa 2, Ocupada, 2 cuentas' });
    expect(within(double).getByText('2 cuentas')).toBeTruthy();
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
