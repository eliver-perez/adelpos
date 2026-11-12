// @vitest-environment jsdom

import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import type { RefObject } from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { OperationCommandHandlers } from '../../commands/operationBridge';
import { PendingOperationsProvider } from '../../hooks/pendingOperations';
import { NavigationContext } from '../../navigation/NavigationContext';
import type { NavigationValue } from '../../navigation/NavigationContext';
import type { ProductBreakdown } from '../../services/catalog';
import type { DraftItem } from '../../services/restaurant';
import { distinctWarnings } from './AccountKitchen';
import { choiceLabel, chosenComplements, describeBreakdown } from './capture/captureRules';
import { OperationalPage } from './OperationalPage';

const native = vi.hoisted(() => ({ invoke: vi.fn() }));
vi.mock('@tauri-apps/api/core', () => ({ invoke: native.invoke, isTauri: () => true }));

// Etapa 9D, fase D (§4): creación diferida de Para llevar y «Cuenta sin mesa», «Descartar cuenta
// vacía» sin motivo, tarjetas compactas, textos del detalle y menú por producto. Solo se sustituye el
// transporte nativo por un servidor simulado; los importes son los que «devuelve Go».

const ids = {
  area: '00000000-0000-4000-8000-0000000000a1',
  freeTable: '00000000-0000-4000-8000-0000000000b1',
  draftTable: '00000000-0000-4000-8000-0000000000b2',
  sentTable: '00000000-0000-4000-8000-0000000000b3',
  category: '00000000-0000-4000-8000-0000000000d1',
  steak: '00000000-0000-4000-8000-0000000000e1',
  combo: '00000000-0000-4000-8000-0000000000e2',
  list: '00000000-0000-4000-8000-000000000101',
};

const tableNames: Record<string, string> = {
  [ids.freeTable]: 'Mesa A',
  [ids.draftTable]: 'Mesa B',
  [ids.sentTable]: 'Mesa C',
};

interface ServerAccount {
  id: string;
  folio: number;
  revision: number;
  mode: 'dine_in' | 'takeaway';
  tableID: string | null;
  state: 'open' | 'voided';
  draft: DraftItem[];
  lines: unknown[];
  total: string;
  preparation: string;
  customerName: string;
}

let accounts: Record<string, ServerAccount>;
let opens: Record<string, unknown>[];
let saves: { id: string; body: Record<string, unknown> }[];
let discards: { id: string; body: Record<string, unknown> }[];
let replays: Map<string, unknown>;
let openFailure: { code: string; message: string } | null;

function accountID(folio: number) {
  return `00000000-0000-4000-8000-0000000009${String(folio).padStart(2, '0')}`;
}

function account(state: ServerAccount) {
  // Como en Go: descartable sin productos enviados (aunque tenga borrador); liberable además sin
  // borrador.
  const discardable = state.state === 'open' && state.lines.length === 0;
  const units = state.lines.reduce<number>(
    (total, line) => total + (line as { quantity: number }).quantity,
    0,
  );
  return {
    id: state.id,
    folio: state.folio,
    name:
      state.mode === 'takeaway' ? 'Pedido para llevar' : (tableNames[state.tableID!] ?? 'Cuenta'),
    mode: state.mode,
    state: state.state,
    table_id: state.tableID,
    table_name: state.tableID ? tableNames[state.tableID] : null,
    price_list_id: ids.list,
    customer_name: state.customerName,
    phone: '',
    origin: state.mode === 'takeaway' ? 'Para llevar' : '',
    notes: '',
    scheduled_at: null,
    scheduled_local: '',
    schedule_kind: null,
    assigned_user_id: 'usuario',
    assigned_name: 'Laura',
    created_at: '2026-11-12T15:00:00Z',
    updated_at: '2026-11-12T15:00:00Z',
    revision: state.revision,
    merged_into_id: null,
    elapsed_seconds: 900,
    urgency: 'normal',
    preparation_state: state.preparation,
    payment_state: 'not_recorded',
    delivery_state: 'not_delivered',
    finance: {
      total: state.total,
      paid: '0.00',
      balance: state.total,
      tips: '0.00',
      payment_state: 'not_recorded',
      delivery_state: 'not_delivered',
    },
    confirmed_quantity: state.state === 'open' ? units : 0,
    draft_quantity: state.draft.reduce((total, item) => total + item.quantity, 0),
    totals: { subtotal: state.total, discount: '0.00', total: state.total },
    releasable: discardable && state.draft.length === 0,
    discardable,
    cancel_blocker: '',
    cancel_requires_authorization: false,
    long_open: false,
  };
}

function bill(state: ServerAccount) {
  return {
    account: account(state),
    draft: state.draft,
    lines: state.lines,
    server_time: '2026-11-12T16:00:00Z',
    timezone: 'America/Monterrey',
    cancellation: null,
  };
}

function breakdown(name: string, values: Partial<ProductBreakdown> = {}): ProductBreakdown {
  return {
    product_id: ids.steak,
    revision: 3,
    name,
    base_price: '150.00',
    price_origin: 'base',
    unit_price: '150.00',
    ingredient_extras: '0.00',
    group_extras: '0.00',
    component_extras: '0.00',
    notes: '',
    ingredients: [],
    groups: [],
    components: [],
    ...values,
  };
}

function line(
  name: string,
  quantity: number,
  total: string,
  values: { preparation?: string; discount?: string; quote?: Partial<ProductBreakdown> } = {},
) {
  return {
    preparation_state: values.preparation ?? 'released',
    version_id: `version-${name}`,
    item_id: `item-${name}`,
    original_order_id: 'pedido',
    version: 4,
    state: 'active',
    quantity,
    request: {
      product_id: ids.steak,
      variant_id: null,
      quantity,
      customization: { ingredients: [], groups: [], components: [], notes: '' },
    },
    quote_line: {
      ...breakdown(name, values.quote),
      quantity,
      subtotal: total,
      discount: values.discount ?? '0.00',
      total,
    },
    totals: { subtotal: total, discount: values.discount ?? '0.00', total },
    created_at: '2026-11-12T15:05:00Z',
  };
}

// Combo con componentes que repiten el nombre de su grupo («Papas a la francesa»), que lo
// contienen («Hamburguesa sencilla» en «Hamburguesa») y uno distinto («Refresco: Coca Cola 600ml»).
const comboQuote: Partial<ProductBreakdown> = {
  component_extras: '15.00',
  components: [
    {
      slot_id: 's1',
      name: 'Papas a la francesa',
      quantity: 1,
      charge: '0.00',
      product: breakdown('Papas a la francesa'),
    },
    {
      slot_id: 's2',
      name: 'Hamburguesa',
      quantity: 1,
      charge: '15.00',
      product: breakdown('Hamburguesa sencilla'),
    },
    {
      slot_id: 's3',
      name: 'Refresco',
      quantity: 1,
      charge: '0.00',
      product: breakdown('Coca Cola 600ml'),
    },
  ],
};

function server(): Record<string, ServerAccount> {
  const base = {
    revision: 3,
    state: 'open' as const,
    draft: [],
    lines: [],
    total: '0.00',
    preparation: 'without_items',
    customerName: '',
  };
  return {
    [accountID(1)]: {
      ...base,
      id: accountID(1),
      folio: 1,
      mode: 'dine_in',
      tableID: ids.draftTable,
      revision: 5,
      // Como las cuentas #1 y #3 de la instalación real: un borrador sin enviar, $0.
      draft: [
        {
          id: 'borrador-1',
          product_id: ids.steak,
          variant_id: null,
          quantity: 2,
          customization: { ingredients: [], groups: [], components: [], notes: '' },
        },
      ],
      preparation: 'draft',
    },
    [accountID(2)]: {
      ...base,
      id: accountID(2),
      folio: 2,
      mode: 'dine_in',
      tableID: ids.sentTable,
      lines: [
        line('Filete de prueba', 3, '450.00', { preparation: 'pending_release' }),
        line('Combo de prueba', 1, '185.00', { discount: '10.00', quote: comboQuote }),
      ],
      total: '635.00',
      preparation: 'pending_release',
    },
    [accountID(3)]: { ...base, id: accountID(3), folio: 3, mode: 'takeaway', tableID: null },
    [accountID(4)]: {
      ...base,
      id: accountID(4),
      folio: 4,
      mode: 'takeaway',
      tableID: null,
      customerName: 'Cliente 9D',
      lines: [line('Filete de prueba', 1, '150.00', { preparation: 'pending_release' })],
      total: '150.00',
      preparation: 'pending_release',
    },
  };
}

function mapTable(id: string, x: number) {
  const open = Object.values(accounts).filter(
    (entry) => entry.state === 'open' && entry.tableID === id,
  );
  return {
    id,
    area_id: ids.area,
    name: tableNames[id],
    shape: 'rectangle',
    x,
    y: 40,
    width: 120,
    height: 100,
    seats: 4,
    active: true,
    out_of_service: false,
    revision: 1,
    account_count: open.length,
    state: open.length ? 'occupied' : 'free',
    occupied_since: open.length ? '2026-11-12T15:00:00Z' : null,
    occupied_seconds: open.length ? 900 : 0,
    balance: '0.00',
    long_open: false,
  };
}

const settings = { warning_minutes: 15, overdue_minutes: 30, long_open_hours: 12, revision: 1 };

function quote(state: ServerAccount) {
  return {
    account_id: state.id,
    revision: state.revision,
    fingerprint: `huella-${state.revision}`,
    quote: {
      calculated_at: '2026-11-12T16:00:00Z',
      timezone: 'America/Monterrey',
      price_list_name: 'Restaurante',
      price_list_revision: 1,
      subtotal: '0.00',
      discount: '0.00',
      total: state.draft.length ? '150.00' : '0.00',
      lines: state.draft.map((item) => ({
        ...breakdown('Filete de prueba'),
        quantity: item.quantity,
        subtotal: '150.00',
        discount: '0.00',
        total: '150.00',
      })),
      promotions: [],
    },
    combined: { subtotal: '0.00', discount: '0.00', total: state.draft.length ? '150.00' : '0.00' },
    pricing_policy: '',
  };
}

function openAccount(body: Record<string, unknown>) {
  const replay = replays.get(String(body.operation_id));
  if (replay) {
    return replay;
  }
  opens.push(body);
  if (openFailure) {
    return { status: 409, body: { error: openFailure } };
  }
  const folio = Object.keys(accounts).length + 1;
  const id = accountID(folio);
  accounts[id] = {
    id,
    folio,
    revision: 1,
    mode: body.mode as 'dine_in' | 'takeaway',
    tableID: (body.table_id as string | null) ?? null,
    state: 'open',
    draft: (body.items as DraftItem[]) ?? [],
    lines: [],
    total: '0.00',
    preparation: 'draft',
    customerName: '',
  };
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
  const state = id ? accounts[id] : undefined;
  switch (operation) {
    case 'restaurant_overview':
      return {
        areas: [
          {
            id: ids.area,
            name: 'Salón D',
            description: '',
            sort_order: 10,
            active: true,
            revision: 1,
            table_count: 3,
          },
        ],
        tables: [
          mapTable(ids.freeTable, 40),
          mapTable(ids.draftTable, 220),
          mapTable(ids.sentTable, 400),
        ],
        settings,
        timezone: 'America/Monterrey',
      };
    case 'restaurant_accounts': {
      const wanted = query?.state ?? 'open';
      const items = Object.values(accounts)
        .filter((entry) => !query?.mode || entry.mode === query.mode)
        .filter((entry) => !query?.table_id || entry.tableID === query.table_id)
        .filter((entry) => wanted === 'all' || entry.state === wanted)
        .sort((left, right) => left.folio - right.folio)
        .map(account);
      return {
        items,
        total: items.length,
        page: 1,
        page_size: 50,
        server_time: '2026-11-12T16:00:00Z',
        timezone: 'America/Monterrey',
        settings,
      };
    }
    case 'restaurant_account':
      return bill(state!);
    case 'restaurant_open_account':
      return openAccount(body!);
    case 'restaurant_save_draft': {
      saves.push({ id: id!, body: body! });
      state!.draft = body!.items as DraftItem[];
      state!.revision += 1;
      return bill(state!);
    }
    case 'restaurant_preview':
      return quote(state!);
    case 'restaurant_discard_account': {
      discards.push({ id: id!, body: body! });
      if (body!.revision !== state!.revision) {
        return {
          status: 409,
          body: {
            error: { code: 'ACCOUNT_CONFLICT', message: 'La cuenta cambió en otra terminal.' },
          },
        };
      }
      state!.state = 'voided';
      state!.draft = [];
      state!.revision += 1;
      return bill(state!);
    }
    case 'restaurant_history':
      return { items: [], page: 1, total: 0 };
    case 'restaurant_cancel_reasons':
      return [];
    case 'catalog_lookups':
      return {
        categories: [
          {
            id: ids.category,
            name: 'Platillos',
            description: '',
            active: true,
            revision: 1,
            sort_order: 10,
            color: '#d64545',
            image_id: null,
            availability: { days: [], windows: [] },
            available_now: true,
          },
        ],
        ingredients: [],
        complements: [],
        portions: [],
        groups: [],
        'price-lists': [{ id: ids.list, name: 'Restaurante', is_default: true, active: true }],
        palette: [],
        products: [
          {
            id: ids.steak,
            name: 'Filete de prueba',
            code: 'FILETE',
            kind: 'product',
            active: true,
            archived: false,
            category_id: ids.category,
            sort_order: 10,
            variants: [],
          },
        ],
      };
    case 'catalog_products':
      return {
        items: [
          {
            id: ids.steak,
            name: 'Filete de prueba',
            description: '',
            active: true,
            revision: 1,
            category_id: ids.category,
            code: 'FILETE',
            barcode: null,
            kind: 'product',
            base_price: '150.00',
            tax_rate: null,
            image_id: null,
            sort_order: 10,
            archived: false,
            temporary_unavailable: false,
            availability: { days: [], windows: [] },
            variants: [],
            ingredients: [],
            groups: [],
            slots: [],
            available_now: true,
            availability_reason: '',
            effective_price: '150.00',
            price_origin: 'base',
          },
        ],
        total: 1,
      };
    default:
      return {};
  }
}

beforeEach(() => {
  accounts = server();
  opens = [];
  saves = [];
  discards = [];
  replays = new Map();
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

function page(
  requestedView: 'tables' | 'takeaway',
  options: { requestKey?: number; handlers?: RefObject<OperationCommandHandlers | null> } = {},
) {
  return (
    <PendingOperationsProvider>
      <OperationalPage
        active
        timezone="America/Monterrey"
        requestedView={requestedView}
        requestKey={options.requestKey ?? 0}
        can={() => true}
        commandHandlers={options.handlers}
      />
    </PendingOperationsProvider>
  );
}

function renderPage(requestedView: 'tables' | 'takeaway' = 'tables') {
  return render(page(requestedView));
}

async function addSteak() {
  const product = await screen.findByRole('button', { name: /^Filete de prueba/ });
  await waitFor(() => expect((product as HTMLButtonElement).disabled).toBe(false));
  fireEvent.click(product);
}

async function settle(milliseconds = 50) {
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, milliseconds));
  });
}

describe('creación diferida (§4.1 y decisión 2)', () => {
  it('«Nuevo pedido» entra a la captura sin crear nada y salir sin productos no deja nada', async () => {
    renderPage('takeaway');
    fireEvent.click(await screen.findByRole('button', { name: 'Nuevo pedido' }));
    await screen.findByRole('heading', { name: 'Para llevar · Pedido nuevo' });
    const bill = screen.getByRole('complementary', { name: 'Cuenta actual' });
    expect(within(bill).getByRole('heading', { name: 'Pedido nuevo' })).toBeTruthy();
    expect(within(bill).getByText('Se abre al guardar el primer producto.')).toBeTruthy();
    // Sin pedido no hay «Datos del pedido» ni acciones de «Más…» todavía.
    expect(screen.queryByRole('button', { name: 'Datos del pedido' })).toBeNull();
    expect(within(bill).queryByRole('button', { name: 'Más…' })).toBeNull();
    fireEvent.click(screen.getByRole('button', { name: /Para llevar/ }));
    await screen.findByRole('group', { name: 'Pedidos para llevar' });
    await settle(900);
    expect(opens).toHaveLength(0);
    expect(Object.values(accounts).filter((entry) => entry.mode === 'takeaway')).toHaveLength(2);
  });

  it('el pedido nace con el primer producto, para llevar y sin mesa', async () => {
    renderPage('takeaway');
    fireEvent.click(await screen.findByRole('button', { name: 'Nuevo pedido' }));
    await screen.findByRole('heading', { name: 'Para llevar · Pedido nuevo' });
    await addSteak();
    await waitFor(() => expect(opens).toHaveLength(1), { timeout: 2500 });
    expect(opens[0]).toEqual(
      expect.objectContaining({
        mode: 'takeaway',
        table_id: null,
        origin: 'Para llevar',
        seen_account_ids: [],
        items: [expect.objectContaining({ product_id: ids.steak, quantity: 1 })],
      }),
    );
    // La captura sigue en el pedido creado y ya ofrece sus datos.
    await screen.findByRole('heading', { name: 'Para llevar · Pedido #5' });
    expect(screen.getByRole('button', { name: 'Datos del pedido' })).toBeTruthy();
    // El siguiente toque guarda el borrador del pedido real.
    await addSteak();
    await waitFor(() => expect(saves).toHaveLength(1), { timeout: 2500 });
    expect(saves[0]!.id).toBe(accountID(5));
    expect(opens).toHaveLength(1);
  });

  it('«Cuenta sin mesa» de Cuentas y del resumen de Mesas nace igual, sin mesa', async () => {
    renderPage();
    const summary = await screen.findByRole('complementary', { name: 'Resumen del área' });
    fireEvent.click(within(summary).getByRole('button', { name: 'Cuenta sin mesa' }));
    await screen.findByRole('heading', { name: 'Cuenta nueva sin mesa' });
    // «‹ Cuentas» vuelve al listado sin dejar nada.
    fireEvent.click(screen.getByRole('button', { name: /Cuentas/ }));
    await screen.findByRole('group', { name: 'Cuentas disponibles' });
    await settle();
    expect(opens).toHaveLength(0);
    // Desde la barra de Cuentas, con el primer producto la cuenta nace en el salón y sin mesa.
    fireEvent.click(screen.getByRole('button', { name: 'Cuenta sin mesa' }));
    await screen.findByRole('heading', { name: 'Cuenta nueva sin mesa' });
    await addSteak();
    await waitFor(() => expect(opens).toHaveLength(1), { timeout: 2500 });
    expect(opens[0]).toEqual(
      expect.objectContaining({
        mode: 'dine_in',
        table_id: null,
        origin: '',
        seen_account_ids: [],
      }),
    );
    const bill = screen.getByRole('complementary', { name: 'Cuenta actual' });
    await within(bill).findByRole('heading', { name: 'Cuenta #5' });
    expect(screen.getByRole('heading', { level: 1, name: 'Cuenta #5' })).toBeTruthy();
  });

  it('un rechazo de Go al crear el pedido se explica en la cuenta, sin código interno', async () => {
    openFailure = {
      code: 'TAKEAWAY_DISABLED',
      message: 'No se pudo abrir el pedido: revisa los datos del establecimiento.',
    };
    renderPage('takeaway');
    fireEvent.click(await screen.findByRole('button', { name: 'Nuevo pedido' }));
    await screen.findByRole('heading', { name: 'Para llevar · Pedido nuevo' });
    await addSteak();
    const bill = screen.getByRole('complementary', { name: 'Cuenta actual' });
    await within(bill).findByText(
      'No se pudo abrir el pedido: revisa los datos del establecimiento.',
      {},
      { timeout: 2500 },
    );
    expect(bill.textContent).not.toContain('TAKEAWAY_DISABLED');
    // Los productos se conservan en la terminal; no se creó nada.
    expect(within(bill).getByRole('article', { name: '1 × Filete de prueba' })).toBeTruthy();
    expect(Object.values(accounts)).toHaveLength(4);
  });

  it('«Nuevo pedido para llevar» del resumen respeta la navegación pedida a la aplicación', async () => {
    let allowed = false;
    const requestScreen = vi.fn(async () => allowed);
    const navigation: NavigationValue = {
      screen: 'restaurant',
      home: 'restaurant',
      requestScreen,
      setTail: () => undefined,
    };
    const shell = (requestedView: 'tables' | 'takeaway', requestKey: number) => (
      <NavigationContext.Provider value={navigation}>
        {page(requestedView, { requestKey })}
      </NavigationContext.Provider>
    );
    const view = render(shell('tables', 0));
    let summary = await screen.findByRole('complementary', { name: 'Resumen del área' });
    // La aplicación no navegó (por ejemplo, cambios sin guardar que la persona conservó): nada se
    // abre.
    fireEvent.click(within(summary).getByRole('button', { name: 'Nuevo pedido para llevar' }));
    await waitFor(() => expect(requestScreen).toHaveBeenCalledWith('takeaway'));
    await settle();
    expect(screen.queryByRole('heading', { name: 'Para llevar · Pedido nuevo' })).toBeNull();
    // Con la navegación hecha (la sección llega a la pantalla), entra a la captura del pedido.
    allowed = true;
    summary = screen.getByRole('complementary', { name: 'Resumen del área' });
    fireEvent.click(within(summary).getByRole('button', { name: 'Nuevo pedido para llevar' }));
    await waitFor(() => expect(requestScreen).toHaveBeenCalledTimes(2));
    view.rerender(shell('takeaway', 1));
    await screen.findByRole('heading', { name: 'Para llevar · Pedido nuevo' });
    expect(opens).toHaveLength(0);
  });

  it('la paleta y el menú nativo abren la captura después de pedir la sección', async () => {
    const handlers: RefObject<OperationCommandHandlers | null> = { current: null };
    const view = render(page('tables', { handlers }));
    await screen.findByRole('complementary', { name: 'Resumen del área' });
    // La sección y la apertura llegan juntas (el menú nativo): la vuelta a la vista principal de la
    // sección no deja la captura detrás.
    act(() => {
      view.rerender(page('takeaway', { handlers, requestKey: 1 }));
      handlers.current!.newTakeaway();
    });
    await screen.findByRole('heading', { name: 'Para llevar · Pedido nuevo' });
    // Primero la sección y después la apertura (la paleta).
    view.rerender(page('tables', { handlers, requestKey: 2 }));
    await screen.findByRole('complementary', { name: 'Resumen del área' });
    act(() => handlers.current!.accountWithoutTable());
    await screen.findByRole('heading', { name: 'Cuenta nueva sin mesa' });
    await settle();
    expect(screen.getByRole('heading', { name: 'Cuenta nueva sin mesa' })).toBeTruthy();
    expect(opens).toHaveLength(0);
  });
});

describe('descartar cuentas vacías (§4.2 y decisión 1)', () => {
  it('en Mesas: botón a la vista y en «Más…», sin motivo y con confirmación roja', async () => {
    renderPage();
    fireEvent.click(await screen.findByRole('button', { name: /^Mesa B, Ocupada/ }));
    const panel = await screen.findByRole('complementary', { name: 'Panel de Mesa B' });
    const discard = await within(panel).findByRole('button', { name: 'Descartar cuenta vacía' });
    expect(discard.className).toContain('danger-button');
    // Con borrador no es liberable: «Liberar mesa» no aparece.
    expect(within(panel).queryByRole('button', { name: 'Liberar mesa' })).toBeNull();
    // En «Más…» sustituye a «Cancelar cuenta».
    fireEvent.click(within(panel).getByRole('button', { name: 'Más…' }));
    const menu = within(panel).getByRole('group', { name: 'Más acciones de la cuenta' });
    expect(
      within(menu).getByRole('button', { name: 'Descartar cuenta vacía' }).className,
    ).toContain('danger-button');
    expect(within(menu).queryByRole('button', { name: 'Cancelar cuenta' })).toBeNull();
    fireEvent.click(within(menu).getByRole('button', { name: 'Descartar cuenta vacía' }));
    let dialog = await screen.findByRole('alertdialog', { name: '¿Descartar la cuenta #1?' });
    expect(dialog.className).toContain('dialog-panel-danger');
    expect(dialog.textContent).toContain('Se borrarán 2 productos sin enviar.');
    expect(dialog.textContent).toContain('no es una cancelación');
    expect(dialog.textContent).toContain('Mesa B quedará libre.');
    fireEvent.click(within(dialog).getByRole('button', { name: 'Conservar la cuenta' }));
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull());
    expect(discards).toHaveLength(0);
    fireEvent.click(discard);
    dialog = await screen.findByRole('alertdialog', { name: '¿Descartar la cuenta #1?' });
    const confirm = within(dialog).getByRole('button', { name: 'Descartar cuenta vacía' });
    expect(confirm.className).toContain('danger-solid-button');
    fireEvent.click(confirm);
    await waitFor(() => expect(discards).toHaveLength(1));
    // Sin motivo ni supervisor: solo la revisión y el operation_id.
    expect(Object.keys(discards[0]!.body).sort()).toEqual(['operation_id', 'revision']);
    expect(discards[0]).toEqual({
      id: accountID(1),
      body: expect.objectContaining({ revision: 5 }),
    });
    await screen.findByText('Cuenta #1 descartada.');
    await within(panel).findByText('Sin cuenta abierta');
  });

  it('una cuenta con productos enviados conserva «Cancelar cuenta» y no se descarta', async () => {
    renderPage();
    fireEvent.click(await screen.findByRole('button', { name: /^Mesa C, Ocupada/ }));
    const panel = await screen.findByRole('complementary', { name: 'Panel de Mesa C' });
    await within(panel).findByRole('heading', { name: 'Cuenta #2' });
    expect(within(panel).queryByRole('button', { name: 'Descartar cuenta vacía' })).toBeNull();
    fireEvent.click(within(panel).getByRole('button', { name: 'Más…' }));
    const menu = within(panel).getByRole('group', { name: 'Más acciones de la cuenta' });
    expect(within(menu).getByRole('button', { name: 'Cancelar cuenta' })).toBeTruthy();
    expect(within(menu).queryByRole('button', { name: 'Descartar cuenta vacía' })).toBeNull();
  });

  it('en Para llevar: a la vista en el detalle; descartado, el listado pasa al siguiente pedido', async () => {
    renderPage('takeaway');
    const bill = await screen.findByRole('complementary', { name: 'Cuenta actual' });
    await within(bill).findByRole('heading', { name: 'Cuenta #3' });
    const box = bill.querySelector('.bill-discard')!;
    expect(box.textContent).toContain('Sin productos enviados a cocina ni pagos.');
    const discard = within(bill).getByRole('button', { name: 'Descartar cuenta vacía' });
    expect(discard.className).toContain('danger-button');
    // Sigue habiendo un solo botón principal en el detalle.
    expect(bill.querySelectorAll('.primary-button')).toHaveLength(1);
    fireEvent.click(discard);
    const dialog = await screen.findByRole('alertdialog', { name: '¿Descartar el pedido #3?' });
    expect(dialog.textContent).not.toContain('quedará libre');
    expect(dialog.textContent).not.toContain('sin enviar');
    fireEvent.click(within(dialog).getByRole('button', { name: 'Descartar cuenta vacía' }));
    await waitFor(() => expect(discards).toHaveLength(1));
    await screen.findByText('Pedido #3 descartado.');
    await within(bill).findByRole('heading', { name: 'Cuenta #4' });
    // El pedido con productos enviados no ofrece descartarlo.
    expect(within(bill).queryByRole('button', { name: 'Descartar cuenta vacía' })).toBeNull();
    const cards = screen.getByRole('group', { name: 'Pedidos para llevar' });
    await waitFor(() => expect(within(cards).getAllByRole('button')).toHaveLength(1));
  });

  it('dentro de la captura se descarta desde «Más…» y se vuelve al listado', async () => {
    renderPage('takeaway');
    const bill = await screen.findByRole('complementary', { name: 'Cuenta actual' });
    fireEvent.click(
      await within(bill).findByRole('button', { name: 'Capturar productos en esta cuenta' }),
    );
    await screen.findByRole('heading', { name: 'Para llevar · Pedido #3' });
    const captureBill = screen.getByRole('complementary', { name: 'Cuenta actual' });
    // En la captura el descarte está solo en «Más…».
    expect(captureBill.querySelector('.bill-discard')).toBeNull();
    fireEvent.click(within(captureBill).getByRole('button', { name: 'Más…' }));
    fireEvent.click(within(captureBill).getByRole('button', { name: 'Descartar cuenta vacía' }));
    const dialog = await screen.findByRole('alertdialog', { name: '¿Descartar el pedido #3?' });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Descartar cuenta vacía' }));
    await waitFor(() => expect(discards).toHaveLength(1));
    await screen.findByRole('group', { name: 'Pedidos para llevar' });
    expect(screen.queryByRole('heading', { name: /^Para llevar · Pedido #/ })).toBeNull();
    expect(screen.getByRole('button', { name: 'Nuevo pedido' })).toBeTruthy();
  });

  it('con productos nuevos sin guardar, primero los guarda y descarta con la revisión vigente', async () => {
    renderPage('takeaway');
    const bill = await screen.findByRole('complementary', { name: 'Cuenta actual' });
    fireEvent.click(
      await within(bill).findByRole('button', { name: 'Capturar productos en esta cuenta' }),
    );
    await screen.findByRole('heading', { name: 'Para llevar · Pedido #3' });
    await addSteak();
    const captureBill = screen.getByRole('complementary', { name: 'Cuenta actual' });
    fireEvent.click(within(captureBill).getByRole('button', { name: 'Más…' }));
    fireEvent.click(within(captureBill).getByRole('button', { name: 'Descartar cuenta vacía' }));
    const dialog = await screen.findByRole('alertdialog', { name: '¿Descartar el pedido #3?' });
    expect(dialog.textContent).toContain('Se borrará 1 producto sin enviar.');
    fireEvent.click(within(dialog).getByRole('button', { name: 'Descartar cuenta vacía' }));
    await waitFor(() => expect(discards).toHaveLength(1));
    expect(saves).toHaveLength(1);
    expect(discards[0]!.body.revision).toBe(4);
    await screen.findByText('Pedido #3 descartado.');
  });
});

const styles = readFileSync(join(__dirname, '..', '..', 'styles.css'), 'utf8');

function rule(selector: string) {
  const start = styles.indexOf(`\n${selector} {\n`);
  expect(start, `falta la regla ${selector}`).toBeGreaterThan(0);
  return styles.slice(start, styles.indexOf('\n}', start + 1));
}

describe('tarjetas compactas (§4.3)', () => {
  it('dos renglones: folio, lugar, tiempo e importe; debajo los estados y las unidades', async () => {
    renderPage('takeaway');
    const cards = await screen.findByRole('group', { name: 'Pedidos para llevar' });
    const card = (await within(cards).findByText('Pedido #4 · Cliente 9D')).closest('button')!;
    const rows = card.querySelectorAll(':scope > span');
    expect(rows).toHaveLength(2);
    expect(rows[0]!.className).toBe('account-card-line');
    expect(rows[0]!.textContent).toBe('Pedido #4 · Cliente 9D15 min$150.00');
    const tags = Array.from(rows[1]!.querySelectorAll('.finance-badge')).map(
      (tag) => tag.textContent,
    );
    expect(tags).toEqual([
      'Sin confirmar en cocina',
      'Pago: Sin pago',
      'Entrega: Sin entregar',
      '1 unidad',
    ]);
    // Sin datos repetidos: ni la fecha de apertura, ni «Confirmado:», ni el origen de siempre.
    expect(card.textContent).not.toMatch(/12\/11\/26|Confirmado|unidades confirmadas|Para llevar/);
    // La selección usa el estilo azul de §1.
    fireEvent.click(card);
    await waitFor(() => expect(card.getAttribute('aria-pressed')).toBe('true'));
    expect(card.className).toContain('account-list-selected');
  });

  it('en Mesas › Cuentas la tarjeta lleva la mesa; los estilos compactan la tarjeta', async () => {
    renderPage();
    fireEvent.click(await screen.findByRole('button', { name: 'Cuentas' }));
    const cards = await screen.findByRole('group', { name: 'Cuentas disponibles' });
    const card = (await within(cards).findByText('Mesa B')).closest('button')!;
    expect(card.querySelector('.account-card-title')!.textContent).toBe('Cuenta #1');
    expect(card.querySelector('.account-card-preparation')!.textContent).toBe('Borrador');
    expect(card.querySelector('.account-card-preparation')!.getAttribute('title')).toBe(
      'Preparación: Borrador por confirmar',
    );
    // Sin unidades enviadas, las del borrador; nunca «0 unidades».
    expect(card.querySelector('.account-card-tags')!.textContent).toContain('2 sin enviar');
    expect(card.textContent).not.toContain('0 unidades');
    const compact = rule('.account-list-card');
    expect(compact).toContain('padding: 7px 12px;');
    expect(compact).toContain('min-height: 44px;');
    expect(rule('.account-list-items')).toContain('gap: 6px;');
    expect(rule('.account-card-title')).toContain('text-overflow: ellipsis;');
    // Las pestañas «Mesas / Cuentas / Áreas y distribución» con el objetivo táctil de 44 px.
    expect(rule('.operation-view-tabs button')).toContain('min-height: 44px;');
    // La tarjeta seleccionada conserva el azul de la fase A.
    expect(rule('.account-list-card.account-list-selected')).toContain(
      'border-color: var(--accent);',
    );
  });
});

describe('textos del detalle (§4.4)', () => {
  it('«Enviado · sin confirmar en cocina» una sola vez por producto y no en el encabezado', async () => {
    renderPage();
    fireEvent.click(await screen.findByRole('button', { name: 'Cuentas' }));
    const cards = await screen.findByRole('group', { name: 'Cuentas disponibles' });
    fireEvent.click((await within(cards).findByText('Mesa C')).closest('button')!);
    const bill = await screen.findByRole('complementary', { name: 'Cuenta actual' });
    await within(bill).findByRole('heading', { name: 'Cuenta #2' });
    const header = bill.querySelector('.capture-bill-header')!;
    expect(header.textContent).toContain('Pago: sin registro de cobro');
    expect(header.textContent).not.toContain('cocina');
    const steak = within(bill).getByRole('article', { name: '3 × Filete de prueba' });
    expect(steak.textContent!.split('Enviado · sin confirmar en cocina')).toHaveLength(2);
    expect(steak.querySelector('.capture-tag-unconfirmed')).not.toBeNull();
    expect(document.body.textContent).not.toContain('revisar cocina');
    expect(document.body.textContent).not.toContain('Pendiente de liberación');
  });

  it('desglose sin versión ni descuento en cero; combos sin nombres repetidos', async () => {
    renderPage();
    fireEvent.click(await screen.findByRole('button', { name: 'Cuentas' }));
    const cards = await screen.findByRole('group', { name: 'Cuentas disponibles' });
    fireEvent.click((await within(cards).findByText('Mesa C')).closest('button')!);
    const bill = await screen.findByRole('complementary', { name: 'Cuenta actual' });
    const steak = await within(bill).findByRole('article', { name: '3 × Filete de prueba' });
    // Sin complementos ni descuento no hay desglose.
    expect(within(steak).queryByRole('button', { name: /Desglose/ })).toBeNull();
    const combo = within(bill).getByRole('article', { name: '1 × Combo de prueba' });
    expect(combo.querySelector('.capture-line-detail')!.textContent).toBe(
      'Papas a la francesa · Hamburguesa sencilla · Refresco: Coca Cola 600ml',
    );
    const breakdown = within(combo).getByRole('button', { name: /Desglose/ });
    expect(breakdown.textContent).toContain('Descuento −$10.00');
    fireEvent.click(breakdown);
    const content = within(combo).getByRole('region', { name: /Desglose/ });
    expect(content.textContent).toContain('Cargo por componentes+$15.00');
    expect(content.textContent).toContain('Descuento−$10.00');
    expect(bill.textContent).not.toMatch(/Versión|versión|Descuento \$0\.00/);
  });

  it('nombres de grupo y elección, complementos elegidos y advertencias sin repetir', () => {
    expect(choiceLabel('Papas a la francesa', 'Papas a la francesa')).toBe('Papas a la francesa');
    expect(choiceLabel('Hamburguesa', 'Hamburguesa Sencilla')).toBe('Hamburguesa Sencilla');
    expect(choiceLabel('Bebida', 'Agua de horchata')).toBe('Bebida: Agua de horchata');
    expect(choiceLabel('Refresco', 'Coca Cola 600ml')).toBe('Refresco: Coca Cola 600ml');
    // Sin distinguir mayúsculas ni acentos y solo con palabras completas.
    expect(choiceLabel('Guarnición', 'guarnicion de papas')).toBe('guarnicion de papas');
    expect(choiceLabel('Pan', 'Panela')).toBe('Pan: Panela');
    const combo = breakdown('Combo', comboQuote);
    expect(describeBreakdown(combo)).toEqual([
      'Papas a la francesa',
      'Hamburguesa sencilla',
      'Refresco: Coca Cola 600ml',
    ]);
    const steak = breakdown('Filete', {
      ingredients: [
        { ingredient_id: 'i1', name: 'cebolla', included: '1', requested: '0', extra: '0.00' },
        { ingredient_id: 'i2', name: 'queso', included: '0', requested: '1', extra: '5.00' },
      ],
      groups: [
        {
          group_id: 'g1',
          name: 'Guarnición',
          mode: 'value',
          original_value: '30.00',
          requested_value: '30.00',
          difference: '0.00',
          extras: '0.00',
          options: [
            { option_id: 'o1', name: 'Arroz', included: '1', requested: '1' },
            { option_id: 'o2', name: 'Papas', included: '0', requested: '2' },
            { option_id: 'o3', name: 'Ensalada', included: '0', requested: '0' },
          ],
        },
        {
          group_id: 'g2',
          name: 'Salsa',
          mode: 'value',
          original_value: '0.00',
          requested_value: '0.00',
          difference: '0.00',
          extras: '0.00',
          options: [{ option_id: 'o4', name: 'Verde', included: '1', requested: '0' }],
        },
      ],
    });
    // Lo elegido aunque sea lo incluido de origen (el resumen del renglón solo dice lo que cambió).
    expect(chosenComplements(steak)).toEqual([
      'Sin cebolla',
      'Extra queso',
      'Guarnición: Arroz, 2 × Papas',
      'Sin Salsa',
    ]);
    expect(distinctWarnings(['Sin ruta: Filete', 'Sin ruta: Filete', 'Sin ruta: Agua'])).toEqual([
      { text: 'Sin ruta: Filete', count: 2 },
      { text: 'Sin ruta: Agua', count: 1 },
    ]);
  });
});

describe('menú por producto (§4.5)', () => {
  async function sentBill() {
    renderPage('takeaway');
    const cards = await screen.findByRole('group', { name: 'Pedidos para llevar' });
    fireEvent.click((await within(cards).findByText('Pedido #4 · Cliente 9D')).closest('button')!);
    const bill = await screen.findByRole('complementary', { name: 'Cuenta actual' });
    await within(bill).findByRole('heading', { name: 'Cuenta #4' });
    return bill;
  }

  it('tocar el renglón o «⋯» abre «Modificar» y «Cancelar unidades» en rojo', async () => {
    const bill = await sentBill();
    const steak = within(bill).getByRole('article', { name: '1 × Filete de prueba' });
    // Sin abrir el menú el producto no tiene botones de acción a la vista.
    expect(within(steak).queryByRole('button', { name: 'Modificar' })).toBeNull();
    expect(within(steak).queryByRole('button', { name: 'Cancelar unidades' })).toBeNull();
    const more = within(steak).getByRole('button', { name: 'Acciones de 1 × Filete de prueba' });
    expect(more.getAttribute('aria-expanded')).toBe('false');
    // Tocar el renglón.
    fireEvent.click(steak.querySelector('.capture-line-row')!);
    const menu = within(steak).getByRole('group', { name: 'Acciones de 1 × Filete de prueba' });
    expect(more.getAttribute('aria-expanded')).toBe('true');
    expect(within(menu).getByRole('button', { name: 'Modificar' }).className).toContain(
      'secondary-button',
    );
    expect(within(menu).getByRole('button', { name: 'Cancelar unidades' }).className).toContain(
      'danger-button',
    );
    // Escape lo cierra; «⋯» lo vuelve a abrir; tocar fuera lo cierra.
    fireEvent.keyDown(menu, { key: 'Escape' });
    expect(within(steak).queryByRole('group')).toBeNull();
    fireEvent.click(more);
    expect(within(steak).getByRole('group')).toBeTruthy();
    fireEvent.pointerDown(document.body);
    expect(within(steak).queryByRole('group')).toBeNull();
    // «Cancelar unidades» abre su panel de siempre.
    fireEvent.click(more);
    fireEvent.click(within(steak).getByRole('button', { name: 'Cancelar unidades' }));
    await screen.findByRole('heading', { name: 'Cancelar unidades · Filete de prueba' });
  });

  it('solo un menú abierto a la vez; los productos nuevos conservan − 1 + y Personalizar', async () => {
    renderPage();
    fireEvent.click(await screen.findByRole('button', { name: 'Cuentas' }));
    const cards = await screen.findByRole('group', { name: 'Cuentas disponibles' });
    fireEvent.click((await within(cards).findByText('Mesa C')).closest('button')!);
    const bill = await screen.findByRole('complementary', { name: 'Cuenta actual' });
    const steak = await within(bill).findByRole('article', { name: '3 × Filete de prueba' });
    const combo = within(bill).getByRole('article', { name: '1 × Combo de prueba' });
    fireEvent.click(within(steak).getByRole('button', { name: /^Acciones de/ }));
    expect(within(steak).getByRole('group')).toBeTruthy();
    fireEvent.click(within(combo).getByRole('button', { name: /^Acciones de/ }));
    expect(within(combo).getByRole('group')).toBeTruthy();
    expect(within(steak).queryByRole('group')).toBeNull();
    // Cuenta con productos nuevos sin enviar (Mesa B): sus controles no cambian.
    fireEvent.click(within(cards).getByText('Mesa B').closest('button')!);
    const draft = await within(bill).findByRole('article', { name: '2 × Filete de prueba' });
    expect(within(draft).getByRole('button', { name: 'Quitar una unidad de Filete de prueba' }));
    expect(within(draft).getByRole('button', { name: 'Agregar una unidad de Filete de prueba' }));
    expect(within(draft).getByRole('button', { name: 'Personalizar' })).toBeTruthy();
    expect(within(draft).queryByRole('button', { name: /^Acciones de/ })).toBeNull();
  });
});
