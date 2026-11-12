// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { PendingOperationsProvider } from '../../../hooks/pendingOperations';
import { emptyCustomization } from '../../../services/restaurant';
import type { DraftItem } from '../../../services/restaurant';
import { OperationalPage } from '../OperationalPage';

const native = vi.hoisted(() => ({ invoke: vi.fn() }));
vi.mock('@tauri-apps/api/core', () => ({ invoke: native.invoke, isTauri: () => true }));

// Solo se sustituye el transporte nativo por un servidor simulado. Todas las cifras que se
// comprueban son las que «devuelve Go» en estas respuestas; la interfaz solo las muestra. Los
// importes se eligen de forma que no coincidan con una suma hecha en el cliente.

const ids = {
  area: '00000000-0000-4000-8000-0000000000a1',
  table: '00000000-0000-4000-8000-0000000000b1',
  // Etapa 7.1: mesa libre para la captura sin cuenta.
  freeTable: '00000000-0000-4000-8000-0000000000b2',
  first: '00000000-0000-4000-8000-0000000000c1',
  second: '00000000-0000-4000-8000-0000000000c2',
  dishes: '00000000-0000-4000-8000-0000000000d1',
  drinks: '00000000-0000-4000-8000-0000000000d2',
  steak: '00000000-0000-4000-8000-0000000000e1',
  burger: '00000000-0000-4000-8000-0000000000e2',
  garnish: '00000000-0000-4000-8000-0000000000f1',
  fries: '00000000-0000-4000-8000-0000000000f2',
  rings: '00000000-0000-4000-8000-0000000000f3',
  list: '00000000-0000-4000-8000-000000000101',
};

const schedule = { days: [], windows: [] };

function product(id: string, name: string, code: string, groups: unknown[] = []) {
  return {
    id,
    name,
    description: '',
    active: true,
    revision: 1,
    category_id: ids.dishes,
    code,
    barcode: null,
    kind: 'product',
    base_price: id === ids.steak ? '150.00' : '120.00',
    tax_rate: null,
    image_id: null,
    sort_order: id === ids.steak ? 10 : 20,
    archived: false,
    temporary_unavailable: false,
    availability: schedule,
    variants: [],
    ingredients: [],
    groups,
    slots: [],
  };
}

// Hamburguesa con un grupo obligatorio (mínimo y máximo 1) que incluye papas: requiere elegir y
// llega con lo incluido marcado.
const burgerGroups = [
  {
    group_id: ids.garnish,
    min_quantity: null,
    max_quantity: null,
    included: [{ option_id: ids.fries, quantity: '1' }],
  },
];
const garnish = {
  id: ids.garnish,
  name: 'Guarnición',
  description: '',
  active: true,
  revision: 1,
  pricing_mode: 'value',
  min_quantity: '1',
  max_quantity: '1',
  options: [
    {
      id: ids.fries,
      complement_id: 'c1',
      portion_id: null,
      reference_value: '0.00',
      extra_price: '0.00',
      max_quantity: null,
      active: true,
      tiers: [],
      name: 'Papas a la francesa',
    },
    {
      id: ids.rings,
      complement_id: 'c2',
      portion_id: null,
      reference_value: '0.00',
      extra_price: '10.00',
      max_quantity: null,
      active: true,
      tiers: [],
      name: 'Aros de cebolla',
    },
  ],
};

function category(id: string, name: string, color: string, available: boolean, order: number) {
  return {
    id,
    name,
    description: '',
    active: true,
    revision: 1,
    sort_order: order,
    color,
    image_id: null,
    availability: schedule,
    available_now: available,
  };
}

const lookups = {
  categories: [
    category(ids.drinks, 'Bebidas', '#3591d4', false, 5),
    category(ids.dishes, 'Platillos', '#d64545', true, 10),
  ],
  ingredients: [],
  complements: [],
  portions: [],
  groups: [garnish],
  'price-lists': [],
  palette: [],
  products: [
    {
      id: ids.steak,
      name: 'Filete de prueba',
      code: 'FILETE',
      kind: 'product',
      active: true,
      archived: false,
      category_id: ids.dishes,
      sort_order: 10,
      variants: [],
    },
    {
      id: ids.burger,
      name: 'Hamburguesa de prueba',
      code: 'HAMBURGUESA',
      kind: 'product',
      active: true,
      archived: false,
      category_id: ids.dishes,
      sort_order: 20,
      variants: [],
    },
    {
      id: 'agua',
      name: 'Agua',
      code: 'AGUA',
      kind: 'product',
      active: true,
      archived: false,
      category_id: ids.drinks,
      sort_order: 10,
      variants: [],
    },
  ],
  // Indicaciones rápidas por categoría (etapa 7.1): llegan en los mismos lookups, con su orden y
  // estado. La inactiva y la de otra categoría no se ofrecen al personalizar un platillo.
  category_quick_notes: [
    quickNote('q1', ids.dishes, 'Bien cocida', 20),
    quickNote('q2', ids.dishes, 'Sin cebolla', 10),
    quickNote('q3', ids.dishes, 'Sin sal', 30, false),
    quickNote('q4', ids.drinks, 'Sin hielo', 10),
  ],
};

function quickNote(id: string, categoryID: string, text: string, order: number, active = true) {
  return {
    id: `00000000-0000-4000-8000-0000000002${id.slice(1).padStart(2, '0')}`,
    category_id: categoryID,
    text,
    active,
    sort_order: order,
    revision: 1,
    updated_at: '2026-11-09T12:00:00Z',
  };
}

interface ServerAccount {
  id: string;
  folio: number;
  revision: number;
  draft: DraftItem[];
  lines: unknown[];
  confirmedTotal: string;
  // Mesa de la cuenta; sin indicarla, Mesa 1.
  table?: string;
}

let accounts: Record<string, ServerAccount>;
let replays: Map<string, unknown>;
let saves: { id: string; body: { revision: number; items: DraftItem[] } }[];
let confirms: { id: string; body: Record<string, unknown> }[];
let cancels: { id: string; body: Record<string, unknown> }[];
let holdSaves: boolean;
let heldSaves: (() => void)[];
let quoteChanged: boolean;
let failNextSave: boolean;
let saveRequests: Record<string, unknown>[];
// Etapa 7.1: aperturas con el primer producto y la cuenta que «otra terminal» abre en la mesa.
let opens: Record<string, unknown>[];
let otherTerminal: ServerAccount | null;
let catalogPreviews: Record<string, unknown>[];
// Turno de la caja de la terminal que entrega Go al atajo «Caja» (etapa 7.1); sin valor, un
// cuerpo vacío como cualquier consulta no preparada.
let terminalShift: unknown;

function account(state: ServerAccount) {
  return {
    id: state.id,
    folio: state.folio,
    name: 'Cuenta',
    mode: 'dine_in',
    state: 'open',
    table_id: state.table ?? ids.table,
    table_name: state.table === ids.freeTable ? 'Mesa 2' : 'Mesa 1',
    price_list_id: ids.list,
    customer_name: '',
    phone: '',
    origin: '',
    notes: '',
    scheduled_at: null,
    scheduled_local: '',
    schedule_kind: null,
    assigned_user_id: 'user',
    assigned_name: 'Laura',
    created_at: '2026-11-08T18:30:00Z',
    updated_at: '2026-11-08T18:30:00Z',
    revision: state.revision,
    merged_into_id: null,
    elapsed_seconds: 60,
    urgency: 'normal',
    preparation_state: state.lines.length ? 'released' : 'without_items',
    payment_state: 'not_recorded',
    delivery_state: 'not_delivered',
    finance: {
      total: state.confirmedTotal,
      paid: '0.00',
      balance: state.confirmedTotal,
      tips: '0.00',
      payment_state: 'not_recorded',
      delivery_state: 'not_delivered',
    },
    confirmed_quantity: state.lines.length,
    draft_quantity: state.draft.reduce((total, item) => total + item.quantity, 0),
    totals: { subtotal: state.confirmedTotal, discount: '0.00', total: state.confirmedTotal },
    releasable: false,
  };
}

function tableAccounts(table: string) {
  return Object.values(accounts).filter((entry) => (entry.table ?? ids.table) === table);
}

function mapTable(id: string, name: string, x: number) {
  const count = tableAccounts(id).length;
  return {
    id,
    area_id: ids.area,
    name,
    shape: 'rectangle',
    x,
    y: 20,
    width: 120,
    height: 100,
    seats: 4,
    active: true,
    out_of_service: false,
    revision: 1,
    account_count: count,
    state: count ? 'occupied' : 'free',
    occupied_since: count ? '2026-11-08T18:30:00Z' : null,
    occupied_seconds: count ? 60 : 0,
    balance: '0.00',
  };
}

// Apertura con el primer producto como en Go: idempotente por operation_id y con TABLE_HAS_ACCOUNT
// si la mesa tiene una cuenta abierta que la terminal no veía (sin crear nada).
function openWithItems(body: Record<string, unknown>) {
  const replay = replays.get(String(body.operation_id));
  if (replay) {
    return replay;
  }
  opens.push(body);
  const table = String(body.table_id);
  if (otherTerminal && !accounts[otherTerminal.id]) {
    accounts[otherTerminal.id] = otherTerminal;
  }
  const seen = (body.seen_account_ids as string[] | undefined) ?? [];
  const unseen = tableAccounts(table).filter((entry) => !seen.includes(entry.id));
  if (body.seen_account_ids && unseen.length > 0) {
    return {
      status: 409,
      body: {
        error: {
          code: 'TABLE_HAS_ACCOUNT',
          message: 'Esta mesa ya tiene una cuenta abierta en otra terminal.',
          details: {
            accounts: unseen.map((entry) => ({ id: entry.id, folio: entry.folio, name: 'Mesa 2' })),
          },
        },
      },
    };
  }
  const folio = Math.max(...Object.values(accounts).map((entry) => entry.folio)) + 1;
  const id = `00000000-0000-4000-8000-0000000009${String(folio).padStart(2, '0')}`;
  accounts[id] = {
    id,
    folio,
    revision: 1,
    draft: (body.items as DraftItem[]) ?? [],
    lines: [],
    confirmedTotal: '0.00',
    table,
  };
  const result = bill(accounts[id]!);
  replays.set(String(body.operation_id), result);
  return result;
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

// Importes «de Go» por partida: no son precio × cantidad para comprobar que no se calculan aquí.
function lineTotal(item: DraftItem) {
  if (item.product_id === ids.burger) {
    return item.customization.groups[0]?.options[0]?.option_id === ids.rings ? '131.25' : '121.25';
  }
  return `${item.quantity * 140}.55`;
}

function quoteLine(item: DraftItem) {
  const options = item.customization.groups[0]?.options ?? [];
  return {
    product_id: item.product_id,
    revision: 1,
    name: item.product_id === ids.burger ? 'Hamburguesa de prueba' : 'Filete de prueba',
    base_price: '0.00',
    price_origin: 'base',
    unit_price: '0.00',
    ingredient_extras: '0.00',
    group_extras: '0.00',
    component_extras: '0.00',
    notes: item.customization.notes,
    ingredients: [],
    groups:
      item.product_id === ids.burger
        ? [
            {
              group_id: ids.garnish,
              name: 'Guarnición',
              mode: 'value',
              original_value: '0.00',
              requested_value: '0.00',
              difference: '0.00',
              extras: options[0]?.option_id === ids.rings ? '10.00' : '0.00',
              options: garnish.options.map((option) => ({
                option_id: option.id,
                name: option.name,
                included: '0',
                requested: options.some((entry) => entry.option_id === option.id) ? '1' : '0',
              })),
            },
          ]
        : [],
    components: [],
    quantity: item.quantity,
    subtotal: lineTotal(item),
    discount: '0.00',
    total: lineTotal(item),
  };
}

function quote(state: ServerAccount, items: DraftItem[], revision: number) {
  return {
    account_id: state.id,
    revision,
    fingerprint: `huella-${state.id}-${revision}${quoteChanged ? '-nueva' : ''}`,
    quote: {
      calculated_at: '2026-11-08T19:00:00Z',
      timezone: 'America/Monterrey',
      price_list_name: 'Restaurante',
      price_list_revision: 1,
      subtotal: '0.00',
      discount: '0.00',
      // Total del lote con una promoción simulada: distinto de la suma de las partidas.
      total: items.length ? `${items.length}07.30` : '0.00',
      lines: items.map(quoteLine),
      promotions: [],
    },
    combined: {
      subtotal: '0.00',
      discount: '0.00',
      total: items.length ? `9${items.length}9.10` : state.confirmedTotal,
    },
    pricing_policy: 'Los precios se fijan al confirmar.',
  };
}

function invalidSelection() {
  return {
    status: 400,
    body: {
      error: {
        code: 'INVALID',
        message: 'La selección no respeta los límites del grupo Guarnición.',
      },
    },
  };
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
            name: 'Salón',
            description: '',
            sort_order: 10,
            active: true,
            revision: 1,
            table_count: 1,
          },
        ],
        tables: [mapTable(ids.table, 'Mesa 1', 20), mapTable(ids.freeTable, 'Mesa 2', 300)],
        settings: { warning_minutes: 15, overdue_minutes: 30, revision: 1 },
        timezone: 'America/Monterrey',
      };
    case 'restaurant_accounts': {
      const items = (query?.table_id ? tableAccounts(query.table_id) : Object.values(accounts)).map(
        (entry) => account(entry),
      );
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
      return bill(state!);
    case 'restaurant_open_account':
      return openWithItems(body!);
    case 'catalog_preview': {
      // Cotización del catálogo para una cuenta pendiente: los mismos importes «de Go».
      catalogPreviews.push(body!);
      const lines = ((body!.lines as DraftItem[]) ?? []).map((line, index) => ({
        ...line,
        id: `linea-${index}`,
      }));
      return {
        ...quote(
          { id: '', folio: 0, revision: 0, draft: [], lines: [], confirmedTotal: '0.00' },
          lines,
          0,
        ).quote,
      };
    }
    case 'restaurant_preview':
      return quote(state!, state!.draft, state!.revision);
    case 'restaurant_draft_preview': {
      const items = (body!.items as DraftItem[]) ?? [];
      // Un grupo obligatorio vacío (selección explícita sin opciones) se rechaza, como en Go.
      if (
        items.some(
          (item) =>
            item.product_id === ids.burger &&
            item.customization.groups.some((group) => group.options.length === 0),
        )
      ) {
        return invalidSelection();
      }
      return quote(state!, items, body!.revision as number);
    }
    case 'restaurant_save_draft': {
      // Idempotencia como en Go: el mismo operation_id devuelve el resultado original.
      const replay = replays.get(String(body!.operation_id));
      if (replay) {
        return replay;
      }
      saves.push({ id: id!, body: body as { revision: number; items: DraftItem[] } });
      if (body!.revision !== state!.revision) {
        return {
          status: 409,
          body: { error: { code: 'ACCOUNT_CONFLICT', message: 'La cuenta cambió.' } },
        };
      }
      state!.draft = body!.items as DraftItem[];
      state!.revision += 1;
      const result = bill(state!);
      replays.set(String(body!.operation_id), result);
      return result;
    }
    case 'restaurant_confirm': {
      confirms.push({ id: id!, body: body! });
      if (
        body!.expected_fingerprint !== quote(state!, state!.draft, state!.revision).fingerprint ||
        quoteChanged
      ) {
        return {
          status: 409,
          body: { error: { code: 'QUOTE_CHANGED', message: 'El cálculo cambió.' } },
        };
      }
      state!.lines = state!.draft.map((item, index) => ({
        preparation_state: 'released',
        version_id: `version-${index}`,
        item_id: `item-${index}`,
        original_order_id: 'order',
        version: 1,
        state: 'active',
        quantity: item.quantity,
        request: item,
        quote_line: quoteLine(item),
        totals: { subtotal: lineTotal(item), discount: '0.00', total: lineTotal(item) },
        created_at: '2026-11-08T19:00:00Z',
      }));
      state!.confirmedTotal = '281.10';
      state!.draft = [];
      state!.revision += 1;
      return bill(state!);
    }
    case 'restaurant_cancel': {
      cancels.push({ id: id!, body: body! });
      state!.revision += 1;
      return bill(state!);
    }
    case 'restaurant_history':
      return { items: [], page: 1, total: 0 };
    case 'catalog_lookups':
      return lookups;
    case 'catalog_products': {
      const items = [
        product(ids.steak, 'Filete de prueba', 'FILETE'),
        product(ids.burger, 'Hamburguesa de prueba', 'HAMBURGUESA', burgerGroups),
      ]
        .filter((entry) => !query?.category_id || entry.category_id === query.category_id)
        .map((entry) => ({
          ...entry,
          available_now: true,
          availability_reason: '',
          effective_price: entry.id === ids.steak ? '150.00' : '120.00',
          price_origin: 'base',
        }));
      return { items, total: items.length };
    }
    case 'catalog_configuration':
      return {
        product:
          id === ids.burger
            ? product(ids.burger, 'Hamburguesa de prueba', 'HAMBURGUESA', burgerGroups)
            : product(ids.steak, 'Filete de prueba', 'FILETE'),
        category: lookups.categories[1],
        groups: id === ids.burger ? [garnish] : [],
        timezone: 'America/Monterrey',
        availability_reason: '',
      };
    case 'payments_current_shift':
      return terminalShift;
    default:
      return {};
  }
}

beforeEach(() => {
  accounts = {
    [ids.first]: {
      id: ids.first,
      folio: 1,
      revision: 4,
      draft: [],
      lines: [],
      confirmedTotal: '0.00',
    },
    [ids.second]: {
      id: ids.second,
      folio: 2,
      revision: 9,
      draft: [],
      lines: [],
      confirmedTotal: '0.00',
    },
  };
  saves = [];
  replays = new Map();
  confirms = [];
  cancels = [];
  holdSaves = false;
  heldSaves = [];
  quoteChanged = false;
  failNextSave = false;
  saveRequests = [];
  opens = [];
  otherTerminal = null;
  catalogPreviews = [];
  terminalShift = {};
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
      if (args.operation === 'restaurant_save_draft') {
        saveRequests.push(args.body ?? {});
        if (holdSaves) {
          await new Promise<void>((resolve) => heldSaves.push(resolve));
        }
        if (failNextSave) {
          // La petición llega y se aplica, pero la respuesta se pierde en el camino.
          failNextSave = false;
          respond(args.operation, args.resourceId, args.body, args.query);
          throw 'Conexión interrumpida';
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

function renderCapture() {
  render(
    <PendingOperationsProvider>
      <OperationalPage
        active
        timezone="America/Monterrey"
        requestedView="tables"
        can={() => true}
      />
    </PendingOperationsProvider>,
  );
}

async function openFirstAccount() {
  renderCapture();
  // Etapa 6.2: tocar la mesa abre su panel lateral; «Capturar» entra a la cuenta elegida.
  fireEvent.click(await screen.findByRole('button', { name: /^Mesa 1, Ocupada/ }));
  const panel = await screen.findByRole('complementary', { name: 'Panel de Mesa 1' });
  fireEvent.click(
    await within(panel).findByRole('button', { name: 'Capturar pedidos en esta mesa' }),
  );
  await screen.findByRole('heading', { name: 'Mesa 1 · Cuenta #1' });
  return screen.getByRole('complementary', { name: 'Cuenta actual' });
}

function disabled(element: HTMLElement) {
  return (element as HTMLButtonElement).disabled;
}

function pressed(element: HTMLElement) {
  return element.getAttribute('aria-pressed');
}

function productButton(name: string) {
  return screen.getByRole('button', { name: new RegExp(`^${name}`) });
}

describe('captura táctil de pedidos', () => {
  it('muestra las cuentas de la mesa como botones y cambia de cuenta sin listas desplegables', async () => {
    await openFirstAccount();
    const group = screen.getByRole('group', { name: 'Cuentas de la mesa' });
    expect(pressed(within(group).getByRole('button', { name: 'Cuenta #1' }))).toBe('true');
    expect(within(group).getByRole('button', { name: 'Nueva cuenta' })).toBeTruthy();
    expect(screen.queryByRole('combobox', { name: 'Cuenta de la mesa' })).toBeNull();
    expect(screen.queryByRole('combobox', { name: 'Acciones de cuenta' })).toBeNull();
    expect(screen.getByText(/Mesero: Laura · Mesa para 4 · Abierta/)).toBeTruthy();
    fireEvent.click(within(group).getByRole('button', { name: 'Cuenta #2' }));
    await screen.findByRole('heading', { name: 'Mesa 1 · Cuenta #2' });
    // Categorías: solo las disponibles según Go, con su número de productos.
    const categories = screen.getByRole('navigation', { name: 'Categorías' });
    expect(
      pressed(within(categories).getByRole('button', { name: /Platillos.*2 productos/ })),
    ).toBe('true');
    expect(within(categories).queryByRole('button', { name: /Bebidas/ })).toBeNull();
  });

  it('un toque agrega el producto, suma su cantidad y guarda con antirrebote una sola vez', async () => {
    const bill = await openFirstAccount();
    await waitFor(() => expect(disabled(productButton('Filete de prueba'))).toBe(false));
    fireEvent.click(productButton('Filete de prueba'));
    fireEvent.click(productButton('Filete de prueba'));
    fireEvent.click(productButton('Filete de prueba'));
    // El estado local cambia de inmediato: una sola partida nueva con cantidad 3.
    const line = within(bill).getByRole('article', { name: '3 × Filete de prueba' });
    expect(within(line).getByText('Nuevo')).toBeTruthy();
    expect(within(line).getByLabelText('Cantidad de Filete de prueba').textContent).toBe('3');
    expect(saves).toHaveLength(0);
    await waitFor(() => expect(saves).toHaveLength(1), { timeout: 2500 });
    expect(saves[0]!.body.revision).toBe(4);
    expect(saves[0]!.body.items).toEqual([
      expect.objectContaining({ product_id: ids.steak, quantity: 3 }),
    ]);
    // Las cifras son las de Go: partida, nuevo sin enviar y total.
    await waitFor(() => expect(within(line).getByText('$420.55')).toBeTruthy());
    expect(within(bill).getByText('Nuevo sin enviar').nextSibling?.textContent).toBe('$107.30');
    expect(within(bill).getByText('Total').nextSibling?.textContent).toBe('$919.10');
    expect(within(bill).getByText('Precios con IVA incluido')).toBeTruthy();
    // «−» quita una unidad de la partida nueva.
    fireEvent.click(
      within(line).getByRole('button', { name: 'Quitar una unidad de Filete de prueba' }),
    );
    expect(within(bill).getByRole('article', { name: '2 × Filete de prueba' })).toBeTruthy();
    await waitFor(() => expect(saves).toHaveLength(2), { timeout: 2500 });
    expect(saves[1]!.body).toEqual(
      expect.objectContaining({
        revision: 5,
        items: [expect.objectContaining({ quantity: 2 })],
      }),
    );
  });

  it('no pierde toques ni abre otra petición mientras un guardado sigue en vuelo', async () => {
    const bill = await openFirstAccount();
    await waitFor(() => expect(disabled(productButton('Filete de prueba'))).toBe(false));
    holdSaves = true;
    fireEvent.click(productButton('Filete de prueba'));
    await waitFor(() => expect(heldSaves).toHaveLength(1), { timeout: 2500 });
    fireEvent.click(productButton('Filete de prueba'));
    fireEvent.click(productButton('Filete de prueba'));
    await new Promise((resolve) => setTimeout(resolve, 900));
    expect(heldSaves).toHaveLength(1);
    expect(within(bill).getByRole('article', { name: '3 × Filete de prueba' })).toBeTruthy();
    holdSaves = false;
    heldSaves[0]!();
    await waitFor(() => expect(saves).toHaveLength(2), { timeout: 2500 });
    expect(saves[1]!.body.revision).toBe(5);
    expect(saves[1]!.body.items[0]?.quantity).toBe(3);
  });

  it('un producto con opciones abre la personalización en el panel central', async () => {
    const bill = await openFirstAccount();
    const burger = await waitFor(() => {
      const button = productButton('Hamburguesa de prueba');
      expect(disabled(button)).toBe(false);
      return button;
    });
    expect(within(burger).getByText('Opciones')).toBeTruthy();
    fireEvent.click(burger);
    const panel = screen.getByRole('region', { name: 'Personalizar Hamburguesa de prueba' });
    // Sin ventanas emergentes: el panel sustituye a la cuadrícula y las categorías siguen visibles.
    expect(screen.queryByRole('dialog')).toBeNull();
    expect(screen.queryByRole('region', { name: 'Productos' })).toBeNull();
    expect(screen.getByRole('navigation', { name: 'Categorías' })).toBeTruthy();
    expect(panel.closest('.capture-center')).toBeTruthy();
    const add = within(panel).getByRole('button', { name: /^Agregar a la cuenta/ });
    const fries = await within(panel).findByRole('button', { name: 'Papas a la francesa' });
    const rings = within(panel).getByRole('button', { name: 'Aros de cebolla +$10.00' });
    // Lo incluido llega marcado y el total del botón es el de la cotización de Go.
    expect(pressed(fries)).toBe('true');
    expect(pressed(rings)).toBe('false');
    expect(within(panel).getByText('Obligatorio · elige 1')).toBeTruthy();
    await waitFor(() => expect(add.textContent).toBe('Agregar a la cuenta · $121.25'));
    fireEvent.click(rings);
    expect(pressed(rings)).toBe('true');
    expect(pressed(fries)).toBe('false');
    // Una elección obligatoria única no se deja vacía con un segundo toque.
    fireEvent.click(rings);
    expect(pressed(rings)).toBe('true');
    await waitFor(() => expect(add.textContent).toBe('Agregar a la cuenta · $131.25'));
    expect(disabled(add)).toBe(false);
    fireEvent.click(add);
    await screen.findByRole('region', { name: 'Productos' });
    const line = within(bill).getByRole('article', { name: '1 × Hamburguesa de prueba' });
    expect(within(line).getByText(/Aros de cebolla/)).toBeTruthy();
    await waitFor(() => expect(saves).toHaveLength(1), { timeout: 2500 });
    expect(saves[0]!.body.items[0]?.customization.groups).toEqual([
      { group_id: ids.garnish, options: [{ option_id: ids.rings, quantity: '1' }] },
    ]);
  });

  it('ofrece las indicaciones rápidas de la categoría antes de «Escribir otra» sin duplicar atajos', async () => {
    // La cuenta ya usó «Sin cebolla» y «Salsa aparte» en otra partida.
    accounts[ids.first]!.draft = [
      {
        id: '00000000-0000-4000-8000-000000000301',
        product_id: ids.steak,
        variant_id: null,
        quantity: 1,
        customization: { ...emptyCustomization(), notes: 'Sin cebolla, Salsa aparte' },
      },
    ];
    await openFirstAccount();
    const burger = await waitFor(() => {
      const button = productButton('Hamburguesa de prueba');
      expect(disabled(button)).toBe(false);
      return button;
    });
    fireEvent.click(burger);
    const panel = screen.getByRole('region', { name: 'Personalizar Hamburguesa de prueba' });
    const notes = await within(panel).findByRole('region', { name: 'Indicaciones para cocina' });
    const names = () =>
      within(notes)
        .getAllByRole('button')
        .map((button) => button.textContent);
    // En el orden del catálogo, sin la inactiva ni la de Bebidas, y «Sin cebolla» una sola vez.
    expect(names()).toEqual(['Sin cebolla', 'Bien cocida', 'Salsa aparte', 'Escribir otra']);
    const onion = within(notes).getByRole('button', { name: 'Sin cebolla' });
    expect(pressed(onion)).toBe('false');
    fireEvent.click(onion);
    // Se marca en su lugar y se puede quitar con otro toque.
    const marked = within(notes).getByRole('button', { name: 'Quitar indicación Sin cebolla' });
    expect(pressed(marked)).toBe('true');
    expect(names()).toEqual(['Sin cebolla', 'Bien cocida', 'Salsa aparte', 'Escribir otra']);
    fireEvent.click(within(notes).getByRole('button', { name: 'Bien cocida' }));
    const add = within(panel).getByRole('button', { name: /^Agregar a la cuenta/ });
    await waitFor(() => expect(disabled(add)).toBe(false));
    fireEvent.click(add);
    await screen.findByRole('region', { name: 'Productos' });
    await waitFor(() => expect(saves.length).toBeGreaterThan(0), { timeout: 2500 });
    const saved = saves[saves.length - 1]!.body.items.find(
      (item) => item.product_id === ids.burger,
    );
    // Se guardan como texto en las observaciones de la partida, igual que las escritas, y desde la
    // etapa 8 también con su identificador (sin efecto: no cambian ingredientes).
    expect(saved?.customization.notes).toBe('Sin cebolla, Bien cocida');
    expect(saved?.customization.quick_notes).toEqual([
      { note_id: lookups.category_quick_notes[1]!.id, text: 'Sin cebolla' },
      { note_id: lookups.category_quick_notes[0]!.id, text: 'Bien cocida' },
    ]);
    expect(saved?.customization.ingredients).toEqual([]);
  });

  it('«Enviar a cocina» confirma con la huella vigente y queda desactivado sin productos nuevos', async () => {
    const bill = await openFirstAccount();
    const send = () => within(bill).getByRole('button', { name: /^Enviar a cocina/ });
    expect(send().textContent).toBe('Enviar a cocina · 0');
    expect(disabled(send())).toBe(true);
    await waitFor(() => expect(disabled(productButton('Filete de prueba'))).toBe(false));
    fireEvent.click(productButton('Filete de prueba'));
    fireEvent.click(productButton('Filete de prueba'));
    expect(send().textContent).toBe('Enviar a cocina · 2');
    fireEvent.click(send());
    // Vacía la cola sin esperar el antirrebote, cotiza y confirma con la huella de Go.
    await waitFor(() => expect(confirms).toHaveLength(1), { timeout: 2500 });
    expect(saves).toHaveLength(1);
    expect(confirms[0]!.body).toEqual(
      expect.objectContaining({ revision: 5, expected_fingerprint: `huella-${ids.first}-5` }),
    );
    expect(typeof confirms[0]!.body.operation_id).toBe('string');
    const sent = await within(bill).findByRole('article', { name: '2 × Filete de prueba' });
    expect(within(sent).getByText('En cocina')).toBeTruthy();
    await waitFor(() => expect(send().textContent).toBe('Enviar a cocina · 0'));
    expect(disabled(send())).toBe(true);
    expect(within(bill).getByText('Enviado a cocina').nextSibling?.textContent).toBe('$281.10');
    // El envío confirmado se avisa con el número de productos en concordancia.
    const notice = await screen.findByRole('status', { name: 'Operación completada' });
    expect(notice.textContent).toContain('Pedido enviado a cocina · 2 productos');
  });

  it('ante QUOTE_CHANGED avisa y conserva los productos nuevos para revisarlos', async () => {
    const bill = await openFirstAccount();
    await waitFor(() => expect(disabled(productButton('Filete de prueba'))).toBe(false));
    fireEvent.click(productButton('Filete de prueba'));
    quoteChanged = true;
    fireEvent.click(within(bill).getByRole('button', { name: 'Enviar a cocina · 1' }));
    // El rechazo es un aviso persistente; ningún aviso de éxito afirma el envío.
    const alert = await screen.findByRole('alert', { name: 'Error' });
    expect(alert.textContent).toContain('El cálculo cambió antes de enviar');
    expect(screen.queryByText(/Pedido enviado a cocina/)).toBeNull();
    expect(within(bill).getByRole('article', { name: '1 × Filete de prueba' })).toBeTruthy();
    expect(accounts[ids.first]!.draft).toHaveLength(1);
  });

  it('recupera un guardado incierto con el mismo operation_id sin perder toques', async () => {
    const bill = await openFirstAccount();
    await waitFor(() => expect(disabled(productButton('Filete de prueba'))).toBe(false));
    failNextSave = true;
    fireEvent.click(productButton('Filete de prueba'));
    const recover = await screen.findByRole(
      'button',
      { name: 'Recuperar resultado del borrador' },
      { timeout: 2500 },
    );
    // Mientras el resultado es incierto la captura se pausa (como cualquier otra escritura): el
    // toque ya hecho se conserva y no se envía otra petición.
    expect(disabled(productButton('Filete de prueba'))).toBe(true);
    expect(within(bill).getByRole('article', { name: '1 × Filete de prueba' })).toBeTruthy();
    expect(disabled(within(bill).getByRole('button', { name: 'Enviar a cocina · 1' }))).toBe(true);
    await new Promise((resolve) => setTimeout(resolve, 900));
    expect(saveRequests).toHaveLength(1);
    fireEvent.click(recover);
    await waitFor(() => expect(saveRequests).toHaveLength(2), { timeout: 2500 });
    // Mismo operation_id y mismo cuerpo: Go devuelve el resultado original sin aplicarlo dos veces.
    expect(saveRequests[1]!.operation_id).toBe(saveRequests[0]!.operation_id);
    expect(saveRequests[1]).toEqual(saveRequests[0]);
    expect(saves).toHaveLength(1);
    await waitFor(() =>
      expect(screen.queryByRole('button', { name: 'Recuperar resultado del borrador' })).toBeNull(),
    );
    await waitFor(() => expect(disabled(productButton('Filete de prueba'))).toBe(false));
    fireEvent.click(productButton('Filete de prueba'));
    await waitFor(() => expect(saves).toHaveLength(2), { timeout: 2500 });
    expect(saves[1]!.body).toEqual(
      expect.objectContaining({ revision: 5, items: [expect.objectContaining({ quantity: 2 })] }),
    );
  });

  it('conserva la personalización sin guardar al cambiar de cuenta y permite retomarla', async () => {
    await openFirstAccount();
    const burger = await waitFor(() => {
      const button = productButton('Hamburguesa de prueba');
      expect(disabled(button)).toBe(false);
      return button;
    });
    fireEvent.click(burger);
    const panel = screen.getByRole('region', { name: 'Personalizar Hamburguesa de prueba' });
    fireEvent.click(await within(panel).findByRole('button', { name: 'Aros de cebolla +$10.00' }));
    const group = screen.getByRole('group', { name: 'Cuentas de la mesa' });
    fireEvent.click(within(group).getByRole('button', { name: 'Cuenta #2' }));
    await screen.findByRole('heading', { name: 'Mesa 1 · Cuenta #2' });
    expect(screen.getByText('Selección sin guardar conservada.')).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: 'Retomar selección' }));
    await screen.findByRole('heading', { name: 'Mesa 1 · Cuenta #1' });
    const resumed = screen.getByRole('region', { name: 'Personalizar Hamburguesa de prueba' });
    expect(
      pressed(await within(resumed).findByRole('button', { name: 'Aros de cebolla +$10.00' })),
    ).toBe('true');
  });

  it('descartar la selección conservada pide confirmación de peligro', async () => {
    await openFirstAccount();
    const burger = await waitFor(() => {
      const button = productButton('Hamburguesa de prueba');
      expect(disabled(button)).toBe(false);
      return button;
    });
    fireEvent.click(burger);
    const panel = screen.getByRole('region', { name: 'Personalizar Hamburguesa de prueba' });
    fireEvent.click(await within(panel).findByRole('button', { name: 'Aros de cebolla +$10.00' }));
    const group = screen.getByRole('group', { name: 'Cuentas de la mesa' });
    fireEvent.click(within(group).getByRole('button', { name: 'Cuenta #2' }));
    await screen.findByRole('heading', { name: 'Mesa 1 · Cuenta #2' });
    fireEvent.click(screen.getByRole('button', { name: 'Descartar selección' }));
    let dialog = await screen.findByRole('alertdialog', {
      name: '¿Descartar la selección sin guardar?',
    });
    expect(dialog.className).toContain('dialog-panel-danger');
    fireEvent.click(within(dialog).getByRole('button', { name: 'Conservarla' }));
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull());
    expect(screen.getByText('Selección sin guardar conservada.')).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: 'Descartar selección' }));
    dialog = await screen.findByRole('alertdialog');
    fireEvent.click(within(dialog).getByRole('button', { name: 'Descartar selección' }));
    await waitFor(() => expect(screen.queryByText('Selección sin guardar conservada.')).toBeNull());
  });

  it('cancelar unidades pide confirmación de peligro antes de escribir', async () => {
    const item: DraftItem = {
      id: 'draft-enviado',
      product_id: ids.steak,
      variant_id: null,
      quantity: 2,
      customization: emptyCustomization(),
    };
    accounts[ids.first]!.lines = [
      {
        preparation_state: 'released',
        version_id: 'version-0',
        item_id: 'item-0',
        original_order_id: 'order',
        version: 1,
        state: 'active',
        quantity: 2,
        request: item,
        quote_line: quoteLine(item),
        totals: { subtotal: lineTotal(item), discount: '0.00', total: lineTotal(item) },
        created_at: '2026-11-08T19:00:00Z',
      },
    ];
    accounts[ids.first]!.confirmedTotal = '281.10';
    const bill = await openFirstAccount();
    const line = await within(bill).findByRole('article', { name: '2 × Filete de prueba' });
    // Etapa 9D (§4.5): «Cancelar unidades» está en el menú del producto («⋯»).
    fireEvent.click(within(line).getByRole('button', { name: 'Acciones de 2 × Filete de prueba' }));
    fireEvent.click(within(line).getByRole('button', { name: 'Cancelar unidades' }));
    await screen.findByRole('heading', { name: 'Cancelar unidades · Filete de prueba' });
    fireEvent.change(screen.getByLabelText('Motivo de cancelación'), {
      target: { value: 'El cliente cambió de opinión' },
    });
    const submit = screen.getByRole('button', { name: 'Confirmar cancelación de unidades' });
    fireEvent.click(submit);
    let dialog = await screen.findByRole('alertdialog', {
      name: '¿Cancelar 2 unidades de Filete de prueba?',
    });
    expect(dialog.className).toContain('dialog-panel-danger');
    // «Conservar unidades» no envía nada a Go.
    fireEvent.click(within(dialog).getByRole('button', { name: 'Conservar unidades' }));
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull());
    expect(cancels).toHaveLength(0);
    fireEvent.click(submit);
    dialog = await screen.findByRole('alertdialog');
    fireEvent.click(within(dialog).getByRole('button', { name: 'Cancelar unidades' }));
    await waitFor(() => expect(cancels).toHaveLength(1));
    expect(cancels[0]!.body).toEqual(
      expect.objectContaining({
        revision: 4,
        version_id: 'version-0',
        quantity: 2,
        reason: 'El cliente cambió de opinión',
      }),
    );
    const notice = await screen.findByRole('status', { name: 'Operación completada' });
    expect(notice.textContent).toContain('Cancelación registrada: 2 unidades de Filete de prueba');
  });

  it('las acciones secundarias son botones y «Más…» reúne las demás', async () => {
    const bill = await openFirstAccount();
    expect(within(bill).getByRole('button', { name: 'Descuento' })).toBeTruthy();
    expect(within(bill).queryByRole('combobox')).toBeNull();
    fireEvent.click(within(bill).getByRole('button', { name: 'Más…' }));
    const menu = within(bill).getByRole('group', { name: 'Más acciones de la cuenta' });
    for (const label of [
      'Datos de cuenta',
      'Historial de cambios',
      'Preparación y comandas',
      'Cambiar mesa',
      'Transferir productos',
      'Combinar cuentas',
      'Cambiar responsable',
      'Cancelar cuenta',
    ]) {
      expect(within(menu).getByRole('button', { name: label })).toBeTruthy();
    }
    fireEvent.click(within(menu).getByRole('button', { name: 'Historial de cambios' }));
    await screen.findByRole('heading', { name: 'Historial · #1' });
    expect(within(bill).queryByRole('group', { name: 'Más acciones de la cuenta' })).toBeNull();
  });
});

// Etapa 7.1 (fase B): la captura de una mesa libre trabaja con una cuenta pendiente; Go la crea
// con el primer producto guardado y avisa si otra terminal abrió una cuenta en la mesa.
async function openFreeTable() {
  renderCapture();
  fireEvent.click(await screen.findByRole('button', { name: /^Mesa 2, Libre/ }));
  const panel = await screen.findByRole('complementary', { name: 'Panel de Mesa 2' });
  fireEvent.click(
    await within(panel).findByRole('button', { name: 'Capturar pedidos en esta mesa' }),
  );
  await screen.findByRole('heading', { name: 'Mesa 2 · Cuenta nueva' });
  await waitFor(() => expect(disabled(productButton('Filete de prueba'))).toBe(false));
  return screen.getByRole('complementary', { name: 'Cuenta actual' });
}

describe('captura de una mesa sin cuenta (etapa 7.1)', () => {
  it('el primer guardado crea la cuenta con sus productos y los siguientes guardan el borrador', async () => {
    const bill = await openFreeTable();
    expect(within(bill).getByRole('heading', { name: 'Cuenta nueva' })).toBeTruthy();
    expect(opens).toHaveLength(0);
    fireEvent.click(productButton('Filete de prueba'));
    fireEvent.click(productButton('Filete de prueba'));
    expect(within(bill).getByRole('article', { name: '2 × Filete de prueba' })).toBeTruthy();
    // Una sola apertura con antirrebote, con la mesa, las cuentas vistas y el borrador completo.
    await waitFor(() => expect(opens).toHaveLength(1), { timeout: 2500 });
    expect(opens[0]).toEqual(
      expect.objectContaining({
        mode: 'dine_in',
        table_id: ids.freeTable,
        seen_account_ids: [],
        items: [expect.objectContaining({ product_id: ids.steak, quantity: 2 })],
      }),
    );
    expect(typeof opens[0]!.operation_id).toBe('string');
    expect(saves).toHaveLength(0);
    // La captura continúa en la cuenta creada, sin perder los productos nuevos.
    await screen.findByRole('heading', { name: 'Mesa 2 · Cuenta #3' });
    expect(within(bill).getByRole('article', { name: '2 × Filete de prueba' })).toBeTruthy();
    fireEvent.click(productButton('Filete de prueba'));
    await waitFor(() => expect(saves).toHaveLength(1), { timeout: 2500 });
    const created = Object.values(accounts).find((entry) => entry.table === ids.freeTable)!;
    expect(saves[0]!.id).toBe(created.id);
    expect(saves[0]!.body.revision).toBe(1);
    expect(saves[0]!.body.items[0]?.quantity).toBe(3);
    expect(opens).toHaveLength(1);
  });

  it('salir sin productos no llama al servidor y la mesa sigue libre', async () => {
    await openFreeTable();
    fireEvent.click(screen.getByRole('button', { name: /Mesas/ }));
    const panel = await screen.findByRole('complementary', { name: 'Panel de Mesa 2' });
    expect(within(panel).getByText('Libre')).toBeTruthy();
    expect(within(panel).getByText('Sin cuenta abierta')).toBeTruthy();
    await new Promise((resolve) => setTimeout(resolve, 900));
    expect(opens).toHaveLength(0);
    expect(saves).toHaveLength(0);
  });

  it('personalizar en la cuenta pendiente cotiza con el catálogo y la lista predeterminada', async () => {
    const bill = await openFreeTable();
    fireEvent.click(productButton('Hamburguesa de prueba'));
    const options = await screen.findByRole('region', {
      name: 'Personalizar Hamburguesa de prueba',
    });
    const add = await within(options).findByRole('button', { name: /^Agregar a la cuenta/ });
    await waitFor(() => expect(add.textContent).toContain('$121.25'));
    expect(catalogPreviews.at(-1)).toEqual(
      expect.objectContaining({
        lines: [expect.objectContaining({ product_id: ids.burger, quantity: 1 })],
      }),
    );
    fireEvent.click(add);
    expect(within(bill).getByRole('article', { name: '1 × Hamburguesa de prueba' })).toBeTruthy();
    await waitFor(() => expect(opens).toHaveLength(1), { timeout: 2500 });
  });

  it('«Esta mesa ya tiene una cuenta»: agregar los productos a esa cuenta conserva su borrador', async () => {
    const theirs: DraftItem = {
      id: 'borrador-ajeno',
      product_id: ids.steak,
      variant_id: null,
      quantity: 1,
      customization: emptyCustomization(),
    };
    otherTerminal = {
      id: '00000000-0000-4000-8000-000000000955',
      folio: 5,
      revision: 2,
      draft: [theirs],
      lines: [],
      confirmedTotal: '0.00',
      table: ids.freeTable,
    };
    await openFreeTable();
    fireEvent.click(productButton('Hamburguesa de prueba'));
    const options = await screen.findByRole('region', {
      name: 'Personalizar Hamburguesa de prueba',
    });
    const add = await within(options).findByRole('button', { name: /^Agregar a la cuenta/ });
    await waitFor(() => expect(disabled(add)).toBe(false));
    fireEvent.click(add);
    const dialog = await screen.findByRole('alertdialog', {
      name: 'Esta mesa ya tiene una cuenta',
    });
    expect(dialog.textContent).toContain('Cuenta #5');
    // Go no creó nada: la cuenta pendiente conserva sus productos.
    expect(Object.values(accounts).filter((entry) => entry.table === ids.freeTable)).toHaveLength(
      1,
    );
    expect(within(dialog).getByRole('button', { name: 'Abrir una cuenta nueva' })).toBeTruthy();
    fireEvent.click(within(dialog).getByRole('button', { name: 'Agregar a la Cuenta #5' }));
    await waitFor(() => expect(saves).toHaveLength(1), { timeout: 2500 });
    expect(saves[0]!.id).toBe(otherTerminal.id);
    expect(saves[0]!.body.revision).toBe(2);
    expect(saves[0]!.body.items.map((item) => item.product_id)).toEqual([ids.steak, ids.burger]);
    expect(saves[0]!.body.items[0]!.id).toBe('borrador-ajeno');
    await screen.findByRole('heading', { name: 'Mesa 2 · Cuenta #5' });
    expect(screen.queryByRole('alertdialog')).toBeNull();
    expect(opens).toHaveLength(1);
  });

  it('«Esta mesa ya tiene una cuenta»: abrir una cuenta nueva reenvía sabiendo de la otra', async () => {
    otherTerminal = {
      id: '00000000-0000-4000-8000-000000000955',
      folio: 5,
      revision: 2,
      draft: [],
      lines: [],
      confirmedTotal: '0.00',
      table: ids.freeTable,
    };
    await openFreeTable();
    fireEvent.click(productButton('Filete de prueba'));
    const dialog = await screen.findByRole(
      'alertdialog',
      { name: 'Esta mesa ya tiene una cuenta' },
      { timeout: 2500 },
    );
    // Escape no decide por el operador.
    fireEvent.keyDown(dialog, { key: 'Escape' });
    expect(screen.getByRole('alertdialog', { name: 'Esta mesa ya tiene una cuenta' })).toBeTruthy();
    fireEvent.click(within(dialog).getByRole('button', { name: 'Abrir una cuenta nueva' }));
    await waitFor(() => expect(opens).toHaveLength(2));
    expect(opens[1]).toEqual(
      expect.objectContaining({
        seen_account_ids: [otherTerminal.id],
        items: [expect.objectContaining({ product_id: ids.steak, quantity: 1 })],
      }),
    );
    expect(opens[1]!.operation_id).not.toBe(opens[0]!.operation_id);
    await screen.findByRole('heading', { name: 'Mesa 2 · Cuenta #6' });
    expect(Object.values(accounts).filter((entry) => entry.table === ids.freeTable)).toHaveLength(
      2,
    );
  });
});

describe('aviso sin turno en la captura (etapa 7.1)', () => {
  const register = { id: '00000000-0000-4000-8000-000000000201', name: 'Caja 1', active: true };

  it('sin turno se puede capturar con un aviso discreto y «Abrir turno» con permiso', async () => {
    terminalShift = {
      register,
      shift: null,
      shared_shifts: false,
      usable: false,
      reason: 'No hay un turno abierto en Caja 1. Ábrelo con su fondo inicial antes de cobrar.',
    };
    const bill = await openFirstAccount();
    // El aviso está en la cuenta, junto a «Cobrar»; el atajo de la barra indica «Sin turno».
    const notice = await within(bill).findByRole('status', { name: 'Turno de caja' });
    expect(notice.textContent).toContain('No hay turno abierto: puedes capturar, pero no cobrar.');
    expect(within(notice).getByRole('button', { name: 'Abrir turno' })).toBeTruthy();
    expect(
      screen.getByRole('button', { name: 'Caja · Sin turno' }).closest('.capture-topbar'),
    ).not.toBeNull();
    fireEvent.click(productButton('Filete de prueba'));
    await waitFor(() =>
      expect(within(bill).getAllByText(/Filete de prueba/).length).toBeGreaterThan(0),
    );
    fireEvent.click(within(notice).getByRole('button', { name: 'Abrir turno' }));
    expect(await screen.findByRole('dialog', { name: 'Abrir turno' })).toBeTruthy();
  });

  it('con el turno abierto no hay aviso y el atajo lo indica en la barra', async () => {
    terminalShift = {
      register,
      shift: {
        id: '00000000-0000-4000-8000-000000000202',
        register_id: register.id,
        register_name: 'Caja 1',
        state: 'open',
        opening_float: '500.00',
        opening_float_usd: null,
        opening_notes: '',
        business_date: '2026-11-09',
        opened_by: 'usuario',
        opened_by_name: 'Administrador',
        opened_device_name: 'Terminal 1',
        opened_at: '2026-11-09T15:00:00Z',
        closed_at: null,
        revision: 1,
        shared_allowed: false,
      },
      shared_shifts: false,
      usable: true,
      reason: '',
    };
    await openFirstAccount();
    expect(await screen.findByRole('button', { name: 'Caja · Turno abierto' })).toBeTruthy();
    expect(screen.queryByRole('status', { name: 'Turno de caja' })).toBeNull();
  });
});
