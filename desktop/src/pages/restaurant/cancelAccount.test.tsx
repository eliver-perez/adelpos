// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { PendingOperationsProvider } from '../../hooks/pendingOperations';
import { repeatAuthorizationMessage } from '../../hooks/useSupervisorAuthorization';
import { OperationalPage } from './OperationalPage';

const native = vi.hoisted(() => ({ invoke: vi.fn() }));
vi.mock('@tauri-apps/api/core', () => ({ invoke: native.invoke, isTauri: () => true }));

// Etapa 9D, fase E (§5): «Cancelar cuenta» desde el menú «Más…» del panel de la mesa y de la cuenta
// para llevar. Solo se sustituye el transporte nativo por un servidor simulado: el importe, las
// unidades, el bloqueo y si hace falta un supervisor son los que «decide Go» en estas respuestas.

const ids = {
  area: '00000000-0000-4000-8000-0000000000a1',
  table: '00000000-0000-4000-8000-0000000000b1',
  other: '00000000-0000-4000-8000-0000000000b2',
  dine: '00000000-0000-4000-8000-0000000000c1',
  takeaway: '00000000-0000-4000-8000-0000000000c2',
  steak: '00000000-0000-4000-8000-0000000000e1',
  list: '00000000-0000-4000-8000-000000000101',
  user: '00000000-0000-4000-8000-000000000201',
  left: '00000000-0000-4000-8000-000000000301',
  changed: '00000000-0000-4000-8000-000000000302',
  error: '00000000-0000-4000-8000-000000000303',
  other_reason: '00000000-0000-4000-8000-000000000304',
};

interface ServerAccount {
  id: string;
  folio: number;
  revision: number;
  mode: 'dine_in' | 'takeaway';
  tableID: string | null;
  state: 'open' | 'voided';
  units: number;
  total: string;
  paid: string;
  blocker: '' | 'payments' | 'adjustments';
  requiresAuthorization: boolean;
}

const reasons = [
  {
    id: ids.left,
    name: 'Cliente se retiró sin pagar',
    requires_description: false,
    prepared_is_waste: true,
    active: true,
    sort_order: 10,
    revision: 1,
    updated_at: '2026-11-12T10:00:00Z',
  },
  {
    id: ids.changed,
    name: 'Cliente canceló el pedido',
    requires_description: false,
    prepared_is_waste: false,
    active: true,
    sort_order: 20,
    revision: 1,
    updated_at: '2026-11-12T10:00:00Z',
  },
  {
    id: ids.error,
    name: 'Error de captura',
    requires_description: false,
    prepared_is_waste: false,
    active: false,
    sort_order: 30,
    revision: 1,
    updated_at: '2026-11-12T10:00:00Z',
  },
  {
    id: ids.other_reason,
    name: 'Otro',
    requires_description: true,
    prepared_is_waste: false,
    active: true,
    sort_order: 40,
    revision: 1,
    updated_at: '2026-11-12T10:00:00Z',
  },
];

let accounts: Record<string, ServerAccount>;
let cancels: { id: string; body: Record<string, unknown> }[];
// Respuestas preparadas para las próximas cancelaciones (un texto simula la conexión perdida).
let cancelReplies: ({ status: number; body: unknown } | string)[];
let inventoryControl: boolean;

function account(state: ServerAccount) {
  const payment = state.paid === '0.00' ? 'not_recorded' : 'partial';
  return {
    id: state.id,
    folio: state.folio,
    name: state.mode === 'takeaway' ? 'Pedido para llevar' : 'Mesa 1',
    mode: state.mode,
    state: state.state,
    table_id: state.tableID,
    table_name: state.tableID === ids.table ? 'Mesa 1' : null,
    price_list_id: ids.list,
    customer_name: '',
    phone: '',
    origin: state.mode === 'takeaway' ? 'Para llevar' : '',
    notes: '',
    scheduled_at: null,
    scheduled_local: '',
    schedule_kind: null,
    assigned_user_id: ids.user,
    assigned_name: 'Laura',
    created_at: '2026-11-12T15:00:00Z',
    updated_at: '2026-11-12T15:00:00Z',
    revision: state.revision,
    merged_into_id: null,
    elapsed_seconds: 900,
    urgency: 'normal',
    preparation_state: state.units > 0 ? 'released' : 'without_items',
    payment_state: payment,
    delivery_state: 'not_delivered',
    finance: {
      total: state.total,
      paid: state.paid,
      balance: state.total,
      tips: '0.00',
      payment_state: payment,
      delivery_state: 'not_delivered',
    },
    confirmed_quantity: state.state === 'open' ? state.units : 0,
    draft_quantity: 0,
    totals: { subtotal: state.total, discount: '0.00', total: state.total },
    releasable: false,
    discardable: false,
    cancel_blocker: state.state === 'open' ? state.blocker : '',
    cancel_requires_authorization: state.requiresAuthorization,
    long_open: false,
  };
}

function line(quantity: number, total: string) {
  return {
    preparation_state: 'released',
    version_id: 'version-1',
    item_id: 'item-1',
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
      name: 'Filete de prueba',
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
    created_at: '2026-11-12T15:00:00Z',
  };
}

function bill(state: ServerAccount) {
  return {
    account: account(state),
    draft: [],
    lines: state.units > 0 ? [line(state.units, state.total)] : [],
    server_time: '2026-11-12T15:15:00Z',
    timezone: 'America/Monterrey',
    cancellation: null,
  };
}

function overview() {
  const tables = [ids.table, ids.other].map((id, index) => {
    const count = Object.values(accounts).filter(
      (entry) => entry.tableID === id && entry.state === 'open',
    ).length;
    return {
      id,
      area_id: ids.area,
      name: index === 0 ? 'Mesa 1' : 'Mesa 2',
      shape: 'rectangle',
      x: 20 + index * 200,
      y: 20,
      width: 120,
      height: 100,
      seats: 4,
      active: true,
      out_of_service: false,
      revision: 1,
      account_count: count,
      state: count > 0 ? 'occupied' : 'free',
      occupied_since: count > 0 ? '2026-11-12T15:00:00Z' : null,
      occupied_seconds: count > 0 ? 900 : 0,
      balance: '0.00',
      long_open: false,
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
    settings: { warning_minutes: 15, overdue_minutes: 30, long_open_hours: 12, revision: 1 },
    timezone: 'America/Monterrey',
  };
}

function cancelAccount(id: string, body: Record<string, unknown>) {
  cancels.push({ id, body: JSON.parse(JSON.stringify(body)) as Record<string, unknown> });
  const reply = cancelReplies.shift();
  if (reply !== undefined) {
    return reply;
  }
  const state = accounts[id]!;
  state.state = 'voided';
  state.revision += 1;
  return { status: 200, body: bill(state) };
}

function respond(
  operation: string,
  id: string | null,
  body: Record<string, unknown> | null,
  query: Record<string, string> | null,
): { status: number; body: unknown } | string {
  switch (operation) {
    case 'restaurant_overview':
      return { status: 200, body: overview() };
    case 'restaurant_accounts': {
      const items = Object.values(accounts)
        .filter((entry) => !query?.mode || entry.mode === query.mode)
        .filter((entry) => !query?.table_id || entry.tableID === query.table_id)
        .filter((entry) => query?.state !== 'open' || entry.state === 'open')
        .map(account);
      return {
        status: 200,
        body: {
          items,
          total: items.length,
          page: 1,
          page_size: 50,
          server_time: '2026-11-12T15:15:00Z',
          timezone: 'America/Monterrey',
          settings: overview().settings,
        },
      };
    }
    case 'restaurant_account':
      return { status: 200, body: bill(accounts[id!]!) };
    case 'restaurant_cancel_reasons':
      return { status: 200, body: reasons };
    case 'restaurant_cancel_account':
      return cancelAccount(id!, body!);
    case 'settings_features':
      return {
        status: 200,
        body: [
          {
            code: 'inventory_control',
            name: 'Control de inventario',
            description: '',
            enabled: inventoryControl,
            revision: 1,
            updated_at: null,
            updated_by_name: null,
          },
        ],
      };
    case 'catalog_lookups':
      return {
        status: 200,
        body: {
          categories: [],
          ingredients: [],
          complements: [],
          portions: [],
          groups: [],
          'price-lists': [{ id: ids.list, name: 'Restaurante', is_default: true, active: true }],
          palette: [],
          products: [],
        },
      };
    case 'restaurant_preview':
      return {
        status: 200,
        body: {
          account_id: id,
          revision: accounts[id!]!.revision,
          fingerprint: 'huella',
          quote: null,
          combined: { subtotal: '0.00', discount: '0.00', total: '0.00' },
          pricing_policy: '',
        },
      };
    default:
      return {
        status: 404,
        body: { error: { code: 'NOT_FOUND', message: 'Sin datos en la prueba.' } },
      };
  }
}

beforeEach(() => {
  accounts = {
    [ids.dine]: {
      id: ids.dine,
      folio: 7,
      revision: 4,
      mode: 'dine_in',
      tableID: ids.table,
      state: 'open',
      units: 3,
      total: '345.60',
      paid: '0.00',
      blocker: '',
      requiresAuthorization: false,
    },
    [ids.takeaway]: {
      id: ids.takeaway,
      folio: 8,
      revision: 2,
      mode: 'takeaway',
      tableID: null,
      state: 'open',
      units: 1,
      total: '115.00',
      paid: '0.00',
      blocker: '',
      requiresAuthorization: false,
    },
  };
  cancels = [];
  cancelReplies = [];
  inventoryControl = true;
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
      const result = respond(args.operation, args.resourceId, args.body, args.query);
      if (typeof result === 'string') {
        throw result;
      }
      return result;
    },
  );
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

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

function disabled(element: HTMLElement) {
  return (element as HTMLButtonElement).disabled;
}

// Abre el panel de la Mesa 1 y elige «Cancelar cuenta» en «Más…».
async function openTableCancel() {
  renderPage();
  fireEvent.click(await screen.findByRole('button', { name: /^Mesa 1,/ }));
  const panel = await screen.findByRole('complementary', { name: 'Panel de Mesa 1' });
  const more = await within(panel).findByRole('button', { name: 'Más…' });
  fireEvent.click(more);
  const menu = within(panel).getByRole('group', { name: 'Más acciones de la cuenta' });
  const option = within(menu).getByRole('button', { name: 'Cancelar cuenta' });
  expect(option.className).toContain('danger-button');
  fireEvent.click(option);
  const region = await screen.findByRole('region', { name: 'Cancelar cuenta' });
  await reasonsLoaded(region);
  return region;
}

// Los motivos llegan de Go después de abrir el panel.
async function reasonsLoaded(region: HTMLElement) {
  await within(region).findByRole('group', { name: 'Motivo de la cancelación' });
}

function reasonButton(region: HTMLElement, name: string) {
  const group = within(region).getByRole('group', { name: 'Motivo de la cancelación' });
  return within(group).getByRole('button', { name: new RegExp(`^${name}`) });
}

describe('Cancelar cuenta (etapa 9D, §5)', () => {
  it('con permiso propio: motivo del catálogo, importe de Go, confirmación roja y la mesa queda libre', async () => {
    const region = await openTableCancel();
    expect(within(region).getByRole('heading', { name: 'Cancelar cuenta · #7' })).toBeTruthy();
    // Unidades e importe que entrega Go (no se suman en la interfaz).
    const summary = within(region).getByLabelText('Lo que se cancela');
    expect(within(summary).getByText('3 unidades')).toBeTruthy();
    expect(within(summary).getByText('$345.60')).toBeTruthy();
    // Solo los motivos activos, en su orden; sin supervisor porque basta el permiso propio.
    const group = within(region).getByRole('group', { name: 'Motivo de la cancelación' });
    expect(
      within(group)
        .getAllByRole('button')
        .map((entry) => entry.textContent),
    ).toEqual([
      'Cliente se retiró sin pagarLo preparado cuenta como merma',
      'Cliente canceló el pedido',
      'OtroExige descripción',
    ]);
    expect(within(region).queryByText('Autorización de supervisor')).toBeNull();
    const submit = within(region).getByRole('button', { name: 'Cancelar cuenta' });
    expect(submit.className).toContain('danger-solid-button');
    expect(disabled(submit)).toBe(true);
    fireEvent.click(reasonButton(region, 'Cliente se retiró sin pagar'));
    // Qué pasa con lo enviado a cocina, con la marca de merma del motivo.
    expect(
      within(region).getByText(/lo preparado cuenta como merma: en Inventarios queda como/),
    ).toBeTruthy();
    expect(within(region).getByText(/Mesa 1 queda libre si no tiene otras cuentas/)).toBeTruthy();
    expect(disabled(submit)).toBe(false);
    fireEvent.click(submit);
    const dialog = await screen.findByRole('alertdialog', { name: '¿Cancelar la cuenta #7?' });
    expect(dialog.className).toContain('dialog-panel-danger');
    expect(dialog.textContent).toContain('Motivo: Cliente se retiró sin pagar');
    expect(dialog.textContent).toContain('Se cancelan 3 unidades enviadas a cocina por $345.60.');
    // «Conservar la cuenta» no envía nada.
    fireEvent.click(within(dialog).getByRole('button', { name: 'Conservar la cuenta' }));
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull());
    expect(cancels).toHaveLength(0);
    fireEvent.click(submit);
    const again = await screen.findByRole('alertdialog');
    const confirm = within(again).getByRole('button', { name: 'Cancelar cuenta' });
    expect(confirm.className).toContain('danger-solid-button');
    fireEvent.click(confirm);
    await waitFor(() => expect(cancels).toHaveLength(1));
    expect(cancels[0]!.id).toBe(ids.dine);
    expect(cancels[0]!.body).toEqual({
      revision: 4,
      reason_id: ids.left,
      description: '',
      operation_id: expect.any(String),
    });
    const notice = await screen.findByRole('status', { name: 'Operación completada' });
    expect(notice.textContent).toContain('Cuenta #7 cancelada.');
    // Vuelve al mapa con la mesa libre.
    await screen.findByRole('region', { name: 'Mapa del restaurante' });
    const panel = screen.getByRole('complementary', { name: 'Panel de Mesa 1' });
    await waitFor(() => expect(within(panel).getByText('Libre')).toBeTruthy());
    expect(screen.queryByRole('region', { name: 'Cancelar cuenta' })).toBeNull();
  });

  it('sin el permiso: pide el supervisor desde el principio y no conserva la contraseña', async () => {
    accounts[ids.dine]!.requiresAuthorization = true;
    cancelReplies = [
      {
        status: 401,
        body: {
          error: {
            code: 'AUTHORIZATION_INVALID',
            message: 'Usuario o contraseña del supervisor incorrectos.',
          },
        },
      },
    ];
    const region = await openTableCancel();
    const fieldset = within(region).getByRole('group', { name: 'Autorización de supervisor' });
    expect(fieldset.textContent).toContain('«Cancelar cuentas»');
    fireEvent.click(reasonButton(region, 'Cliente canceló el pedido'));
    const submit = within(region).getByRole('button', { name: 'Cancelar cuenta' });
    // Sin usuario y contraseña del supervisor no se puede enviar.
    expect(disabled(submit)).toBe(true);
    const user = within(fieldset).getByLabelText('Usuario del supervisor');
    const password = within(fieldset).getByLabelText(
      'Contraseña del supervisor',
    ) as HTMLInputElement;
    fireEvent.change(user, { target: { value: 'gerente' } });
    fireEvent.change(password, { target: { value: 'equivocada' } });
    expect(disabled(submit)).toBe(false);
    fireEvent.click(submit);
    fireEvent.click(
      within(await screen.findByRole('alertdialog')).getByRole('button', {
        name: 'Cancelar cuenta',
      }),
    );
    await waitFor(() => expect(cancels).toHaveLength(1));
    expect(cancels[0]!.body.authorization).toEqual({ username: 'gerente', password: 'equivocada' });
    // El rechazo de Go se explica en la autorización y la contraseña ya no está en el formulario.
    expect(await within(fieldset).findByText('Usuario o contraseña del supervisor incorrectos.'));
    expect(password.value).toBe('');
    expect(disabled(submit)).toBe(true);
    fireEvent.change(password, { target: { value: 'correcta' } });
    fireEvent.click(submit);
    fireEvent.click(
      within(await screen.findByRole('alertdialog')).getByRole('button', {
        name: 'Cancelar cuenta',
      }),
    );
    await waitFor(() => expect(cancels).toHaveLength(2));
    expect(cancels[1]!.body).toEqual(
      expect.objectContaining({
        reason_id: ids.changed,
        authorization: { username: 'gerente', password: 'correcta' },
      }),
    );
    // Cada intento definitivo es otra operación.
    expect(cancels[1]!.body.operation_id).not.toBe(cancels[0]!.body.operation_id);
    await screen.findByRole('status', { name: 'Operación completada' });
  });

  it('«Otro» exige descripción de 5 a 500 caracteres antes de enviar', async () => {
    const region = await openTableCancel();
    fireEvent.click(reasonButton(region, 'Otro'));
    const field = within(region).getByRole('textbox', {
      name: /^Descripción del motivo «Otro»/,
    }) as HTMLInputElement;
    expect(within(region).getByText('Obligatoria, de 5 a 500 caracteres.')).toBeTruthy();
    const submit = within(region).getByRole('button', { name: 'Cancelar cuenta' });
    expect(disabled(submit)).toBe(true);
    fireEvent.change(field, { target: { value: 'abc' } });
    expect(field.getAttribute('aria-invalid')).toBe('true');
    expect(disabled(submit)).toBe(true);
    fireEvent.change(field, { target: { value: '   ab   ' } });
    expect(disabled(submit)).toBe(true);
    fireEvent.change(field, { target: { value: 'Mesa de la ventana' } });
    expect(disabled(submit)).toBe(false);
    // Si Go la rechaza de todos modos (por ejemplo, el motivo cambió a exigirla), se explica junto
    // al campo sin aviso de error genérico.
    cancelReplies = [
      {
        status: 422,
        body: {
          error: {
            code: 'CANCEL_DESCRIPTION_REQUIRED',
            message: 'El motivo «Otro» requiere una descripción de 5 a 500 caracteres.',
          },
        },
      },
    ];
    fireEvent.click(submit);
    fireEvent.click(
      within(await screen.findByRole('alertdialog')).getByRole('button', {
        name: 'Cancelar cuenta',
      }),
    );
    await waitFor(() => expect(cancels).toHaveLength(1));
    expect(cancels[0]!.body.description).toBe('Mesa de la ventana');
    expect(
      await within(region).findByText(
        'El motivo «Otro» requiere una descripción de 5 a 500 caracteres.',
      ),
    ).toBeTruthy();
    expect(screen.queryByRole('alert', { name: 'Error' })).toBeNull();
  });

  it('con pagos: el botón sigue a la vista y explica que primero va la devolución', async () => {
    accounts[ids.dine]!.blocker = 'payments';
    accounts[ids.dine]!.paid = '100.00';
    renderPage();
    fireEvent.click(await screen.findByRole('button', { name: /^Mesa 1,/ }));
    const panel = await screen.findByRole('complementary', { name: 'Panel de Mesa 1' });
    fireEvent.click(await within(panel).findByRole('button', { name: 'Más…' }));
    const option = within(panel).getByRole('button', { name: 'Cancelar cuenta' });
    // La aclaración se anuncia como descripción del botón, que no se deshabilita.
    expect(disabled(option)).toBe(false);
    const hint = document.getElementById(option.getAttribute('aria-describedby')!);
    expect(hint?.textContent).toBe('Primero registra la devolución');
    fireEvent.click(option);
    const region = await screen.findByRole('region', { name: 'Cancelar cuenta' });
    const blocked = within(region).getByRole('alert');
    expect(blocked.textContent).toContain('Primero registra la devolución');
    expect(blocked.textContent).toContain('(pagado $100.00)');
    expect(blocked.textContent).toContain('«Devoluciones»');
    // Sin formulario ni envío: solo el acceso a la devolución.
    expect(within(region).queryByRole('group', { name: 'Motivo de la cancelación' })).toBeNull();
    expect(within(region).queryByRole('button', { name: 'Cancelar cuenta' })).toBeNull();
    fireEvent.click(within(region).getByRole('button', { name: 'Ir a devoluciones' }));
    await waitFor(() =>
      expect(screen.queryByRole('region', { name: 'Cancelar cuenta' })).toBeNull(),
    );
    expect(cancels).toHaveLength(0);
  });

  it('sin permiso de devoluciones solo explica a quién pedirla', async () => {
    accounts[ids.dine]!.blocker = 'payments';
    accounts[ids.dine]!.paid = '100.00';
    renderPage(
      'tables',
      (permission) => !['payments.refunds.execute', 'payments.collect'].includes(permission),
    );
    fireEvent.click(await screen.findByRole('button', { name: /^Mesa 1,/ }));
    const panel = await screen.findByRole('complementary', { name: 'Panel de Mesa 1' });
    fireEvent.click(await within(panel).findByRole('button', { name: 'Más…' }));
    fireEvent.click(within(panel).getByRole('button', { name: 'Cancelar cuenta' }));
    const region = await screen.findByRole('region', { name: 'Cancelar cuenta' });
    expect(within(region).queryByRole('button', { name: 'Ir a devoluciones' })).toBeNull();
    expect(within(region).getByText(/Registrar la devolución requiere el permiso/)).toBeTruthy();
  });

  it('con un descuento o cargo vigente explica que primero hay que quitarlo', async () => {
    accounts[ids.takeaway]!.blocker = 'adjustments';
    renderPage('takeaway');
    const bill = await screen.findByRole('complementary', { name: 'Cuenta actual' });
    await within(bill).findByRole('heading', { name: 'Cuenta #8' });
    fireEvent.click(within(bill).getByRole('button', { name: 'Más…' }));
    const option = within(bill).getByRole('button', { name: 'Cancelar cuenta' });
    expect(document.getElementById(option.getAttribute('aria-describedby')!)?.textContent).toBe(
      'Primero quita el descuento o cargo',
    );
    fireEvent.click(option);
    const region = await screen.findByRole('region', { name: 'Cancelar cuenta' });
    const blocked = within(region).getByRole('alert');
    expect(blocked.textContent).toContain('Primero quita el descuento o cargo');
    expect(blocked.textContent).toContain('«Cancelar ajuste»');
    fireEvent.click(within(region).getByRole('button', { name: 'Ver descuentos y cargos' }));
    await screen.findByRole('region', { name: 'Descuentos y cargos' });
    expect(cancels).toHaveLength(0);
  });

  it('para llevar sin productos enviados: también se cancela y el texto no habla de mesa', async () => {
    accounts[ids.takeaway]!.units = 0;
    accounts[ids.takeaway]!.total = '0.00';
    inventoryControl = false;
    renderPage('takeaway');
    const bill = await screen.findByRole('complementary', { name: 'Cuenta actual' });
    await within(bill).findByRole('heading', { name: 'Cuenta #8' });
    fireEvent.click(within(bill).getByRole('button', { name: 'Más…' }));
    fireEvent.click(within(bill).getByRole('button', { name: 'Cancelar cuenta' }));
    const region = await screen.findByRole('region', { name: 'Cancelar cuenta' });
    expect(
      within(region).getAllByText('La cuenta no tiene productos enviados a cocina.'),
    ).toHaveLength(1);
    expect(within(region).queryByText(/queda libre/)).toBeNull();
    await reasonsLoaded(region);
    fireEvent.click(reasonButton(region, 'Cliente canceló el pedido'));
    fireEvent.click(within(region).getByRole('button', { name: 'Cancelar cuenta' }));
    const dialog = await screen.findByRole('alertdialog', { name: '¿Cancelar el pedido #8?' });
    // Una sola vez: sin productos enviados no hay nada más que decir de la cocina.
    expect(
      dialog.textContent?.split('La cuenta no tiene productos enviados a cocina.'),
    ).toHaveLength(2);
    fireEvent.click(within(dialog).getByRole('button', { name: 'Cancelar cuenta' }));
    await waitFor(() => expect(cancels).toHaveLength(1));
    await screen.findByRole('status', { name: 'Operación completada' });
  });

  it('resultado incierto: «Reintentar sin duplicar» repite el mismo operation_id', async () => {
    cancelReplies = ['No se pudo comunicar con el servidor.'];
    const region = await openTableCancel();
    fireEvent.click(reasonButton(region, 'Cliente se retiró sin pagar'));
    fireEvent.click(within(region).getByRole('button', { name: 'Cancelar cuenta' }));
    fireEvent.click(
      within(await screen.findByRole('alertdialog')).getByRole('button', {
        name: 'Cancelar cuenta',
      }),
    );
    const retry = await within(region).findByRole('button', { name: 'Reintentar sin duplicar' });
    expect(
      within(region).getByText('No se pudo confirmar el resultado de la cancelación.'),
    ).toBeTruthy();
    // Mientras tanto no se puede enviar otra cancelación ni aparece un error.
    expect(disabled(within(region).getByRole('button', { name: 'Cancelar cuenta' }))).toBe(true);
    expect(screen.queryByRole('alert', { name: 'Error' })).toBeNull();
    fireEvent.click(retry);
    await waitFor(() => expect(cancels).toHaveLength(2));
    expect(cancels[1]!.body).toEqual(cancels[0]!.body);
    const notice = await screen.findByRole('status', { name: 'Operación completada' });
    expect(notice.textContent).toContain('Cuenta #7 cancelada.');
    await screen.findByRole('region', { name: 'Mapa del restaurante' });
  });

  it('resultado incierto con supervisor: el reintento va sin contraseña y, si no se registró, la pide otra vez', async () => {
    accounts[ids.dine]!.requiresAuthorization = true;
    cancelReplies = [
      'No se pudo comunicar con el servidor.',
      {
        status: 403,
        body: {
          error: {
            code: 'AUTHORIZATION_REQUIRED',
            message: 'Cancelar una cuenta requiere la autorización de un supervisor.',
          },
        },
      },
    ];
    const region = await openTableCancel();
    fireEvent.click(reasonButton(region, 'Cliente canceló el pedido'));
    const fieldset = within(region).getByRole('group', { name: 'Autorización de supervisor' });
    fireEvent.change(within(fieldset).getByLabelText('Usuario del supervisor'), {
      target: { value: 'gerente' },
    });
    fireEvent.change(within(fieldset).getByLabelText('Contraseña del supervisor'), {
      target: { value: 'secreta' },
    });
    fireEvent.click(within(region).getByRole('button', { name: 'Cancelar cuenta' }));
    fireEvent.click(
      within(await screen.findByRole('alertdialog')).getByRole('button', {
        name: 'Cancelar cuenta',
      }),
    );
    const retry = await within(region).findByRole('button', { name: 'Reintentar sin duplicar' });
    expect(within(region).getByText(/La contraseña del supervisor no se conserva/)).toBeTruthy();
    fireEvent.click(retry);
    await waitFor(() => expect(cancels).toHaveLength(2));
    // Mismo operation_id y la autorización sin contraseña.
    expect(cancels[1]!.body.operation_id).toBe(cancels[0]!.body.operation_id);
    expect(cancels[0]!.body.authorization).toEqual({ username: 'gerente', password: 'secreta' });
    expect(cancels[1]!.body.authorization).toEqual({ username: 'gerente' });
    expect(await within(fieldset).findByText(repeatAuthorizationMessage)).toBeTruthy();
    expect(
      (within(fieldset).getByLabelText('Contraseña del supervisor') as HTMLInputElement).value,
    ).toBe('');
  });

  it('una cuenta que cambió en otra terminal pide revisarla antes de cancelar', async () => {
    const region = await openTableCancel();
    fireEvent.click(reasonButton(region, 'Cliente canceló el pedido'));
    accounts[ids.dine]!.revision = 5;
    accounts[ids.dine]!.units = 4;
    accounts[ids.dine]!.total = '460.80';
    window.dispatchEvent(new CustomEvent('adel-server-change'));
    expect(
      await within(region).findByText(/La cuenta cambió en otra terminal/, {}, { timeout: 3000 }),
    ).toBeTruthy();
    expect(disabled(within(region).getByRole('button', { name: 'Cancelar cuenta' }))).toBe(true);
    expect(within(region).getByText('4 unidades')).toBeTruthy();
    fireEvent.click(
      within(region).getByRole('button', { name: 'Revisé los cambios de esta cuenta' }),
    );
    expect(disabled(within(region).getByRole('button', { name: 'Cancelar cuenta' }))).toBe(false);
  });
});
