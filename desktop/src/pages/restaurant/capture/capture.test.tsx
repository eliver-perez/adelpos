// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { PendingOperationsProvider } from '../../../hooks/pendingOperations';
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
};

interface ServerAccount {
  id: string;
  folio: number;
  revision: number;
  draft: DraftItem[];
  lines: unknown[];
  confirmedTotal: string;
}

let accounts: Record<string, ServerAccount>;
let replays: Map<string, unknown>;
let saves: { id: string; body: { revision: number; items: DraftItem[] } }[];
let confirms: { id: string; body: Record<string, unknown> }[];
let holdSaves: boolean;
let heldSaves: (() => void)[];
let quoteChanged: boolean;
let failNextSave: boolean;
let saveRequests: Record<string, unknown>[];

function account(state: ServerAccount) {
  return {
    id: state.id,
    folio: state.folio,
    name: 'Cuenta',
    mode: 'dine_in',
    state: 'open',
    table_id: ids.table,
    table_name: 'Mesa 1',
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
        tables: [
          {
            id: ids.table,
            area_id: ids.area,
            name: 'Mesa 1',
            shape: 'rectangle',
            x: 20,
            y: 20,
            width: 120,
            height: 100,
            seats: 4,
            active: true,
            out_of_service: false,
            revision: 1,
            account_count: 2,
            state: 'occupied',
          },
        ],
        settings: { warning_minutes: 15, overdue_minutes: 30, revision: 1 },
        timezone: 'America/Monterrey',
      };
    case 'restaurant_accounts':
      return {
        items: Object.values(accounts).map((entry) => account(entry)),
        total: Object.keys(accounts).length,
        page: 1,
        page_size: 50,
        server_time: '2026-11-08T19:00:00Z',
        timezone: 'America/Monterrey',
        settings: { warning_minutes: 15, overdue_minutes: 30, revision: 1 },
      };
    case 'restaurant_account':
      return bill(state!);
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
  holdSaves = false;
  heldSaves = [];
  quoteChanged = false;
  failNextSave = false;
  saveRequests = [];
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
  fireEvent.click(await screen.findByRole('button', { name: /^Mesa 1, Ocupada/ }));
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
    expect(within(group).getByRole('button', { name: '+ Nueva cuenta' })).toBeTruthy();
    expect(screen.queryByRole('combobox', { name: 'Cuenta de la mesa' })).toBeNull();
    expect(screen.queryByRole('combobox', { name: 'Acciones de cuenta' })).toBeNull();
    expect(screen.getByText(/Mesero: Laura · 4 lugares · Abierta/)).toBeTruthy();
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
  });

  it('ante QUOTE_CHANGED avisa y conserva los productos nuevos para revisarlos', async () => {
    const bill = await openFirstAccount();
    await waitFor(() => expect(disabled(productButton('Filete de prueba'))).toBe(false));
    fireEvent.click(productButton('Filete de prueba'));
    quoteChanged = true;
    fireEvent.click(within(bill).getByRole('button', { name: 'Enviar a cocina · 1' }));
    await screen.findByText(/El cálculo cambió antes de enviar/);
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
      'Anular cuenta vacía',
    ]) {
      expect(within(menu).getByRole('button', { name: label })).toBeTruthy();
    }
    fireEvent.click(within(menu).getByRole('button', { name: 'Historial de cambios' }));
    await screen.findByRole('heading', { name: 'Historial · #1' });
    expect(within(bill).queryByRole('group', { name: 'Más acciones de la cuenta' })).toBeNull();
  });
});
