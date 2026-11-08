// @vitest-environment jsdom

import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { PendingOperationsProvider } from '../../../hooks/pendingOperations';
import type {
  CurrentShift,
  Finance,
  PaymentInput,
  PaymentMethod,
  PaymentPlan,
} from '../../../services/payments';
import type { Bill } from '../../../services/restaurant';
import { CheckoutPanel } from './CheckoutPanel';

const native = vi.hoisted(() => ({ invoke: vi.fn() }));
vi.mock('@tauri-apps/api/core', () => ({ invoke: native.invoke, isTauri: () => true }));

// Solo se sustituye el transporte nativo: las cifras simuladas representan lo que calcularía Go.
const accountID = '00000000-0000-0000-0000-0000000000a1';
const versionID = '00000000-0000-0000-0000-0000000000b1';

const methods: PaymentMethod[] = [
  {
    id: 'method-cash',
    code: 'cash',
    name: 'Efectivo',
    icon: 'cash',
    active: true,
    sort_order: 10,
    is_cash: true,
    reference_mode: 'none',
    allows_tip: false,
    allows_refund: true,
    revision: 1,
    updated_at: '2026-11-08T12:00:00Z',
  },
  {
    id: 'method-debit',
    code: 'debit',
    name: 'Tarjeta de débito',
    icon: 'card',
    active: true,
    sort_order: 20,
    is_cash: false,
    reference_mode: 'optional',
    allows_tip: true,
    allows_refund: true,
    revision: 1,
    updated_at: '2026-11-08T12:00:00Z',
  },
];

function finance(balance: string, serverTime = '2026-11-08T12:00:00Z'): Finance {
  return {
    account_id: accountID,
    folio: 41,
    name: 'Mesa 4',
    mode: 'dine_in',
    state: 'open',
    revision: 3,
    table_name: 'Mesa 4',
    items_total: '270.00',
    item_discounts: '0.00',
    account_discounts: '0.00',
    charges: '0.00',
    total: '270.00',
    paid: balance === '270.00' ? '0.00' : '150.00',
    refunded: '0.00',
    tips: '0.00',
    balance,
    payment_state: balance === '270.00' ? 'not_recorded' : 'partial',
    delivery_state: 'not_delivered',
    settled: false,
    tax: [{ rate: 16, base: '232.76', tax: '37.24', total: '270.00' }],
    tax_base: '232.76',
    tax_total: '37.24',
    lines: [
      {
        version_id: versionID,
        item_id: 'item',
        name: 'Hamburguesa',
        notes: '',
        quantity: 2,
        settled_quantity: 0,
        partial_quantity: 0,
        due: '270.00',
        remaining: '270.00',
        units: [
          {
            id: 'unit-1',
            ordinal: 1,
            total: '135.00',
            discount: '0.00',
            due: '135.00',
            paid: '0.00',
            remaining: '135.00',
            status: 'unpaid',
            tax_rate: 16,
          },
          {
            id: 'unit-2',
            ordinal: 2,
            total: '135.00',
            discount: '0.00',
            due: '135.00',
            paid: '0.00',
            remaining: '135.00',
            status: 'unpaid',
            tax_rate: 16,
          },
        ],
      },
    ],
    payments: [],
    refunds: [],
    adjustments: [],
    delivery: null,
    documents: [],
    server_time: serverTime,
  };
}

const bill = {
  account: {
    id: accountID,
    folio: 41,
    name: 'Mesa 4',
    mode: 'dine_in',
    state: 'open',
    table_id: 'table',
    table_name: 'Mesa 4',
    customer_name: '',
    revision: 3,
    confirmed_quantity: 2,
    draft_quantity: 0,
    payment_state: 'not_recorded',
    delivery_state: 'not_delivered',
    finance: {
      total: '270.00',
      paid: '0.00',
      balance: '270.00',
      tips: '0.00',
      payment_state: 'not_recorded',
      delivery_state: 'not_delivered',
    },
  },
  draft: [],
  lines: [
    {
      version_id: versionID,
      state: 'active',
      quantity: 2,
      request: { customization: { notes: 'Sin cebolla' } },
      quote_line: { discount: '0.00', ingredients: [], groups: [], components: [] },
    },
  ],
  server_time: '2026-11-08T12:00:00Z',
  timezone: 'America/Monterrey',
} as unknown as Bill;

const usableShift: CurrentShift = {
  register: { id: 'register', name: 'Caja 1', active: true },
  shift: {
    id: 'shift',
    register_id: 'register',
    register_name: 'Caja 1',
    state: 'open',
    opening_float: '500.00',
    opened_by: 'user',
    opened_by_name: 'Administrador',
    opened_device_name: 'Terminal',
    opened_at: '2026-11-08T11:00:00Z',
    closed_at: null,
    revision: 1,
    shared_allowed: false,
  },
  shared_shifts: false,
  usable: true,
  reason: '',
};

// Simula la previsualización de Go: el cambio solo existe en efectivo y depende de lo recibido.
function plan(input: PaymentInput, balance: string): PaymentPlan {
  const cash = input.method_id === 'method-cash';
  const received = cash ? input.received || '270.00' : null;
  return {
    mode: input.mode,
    target: balance,
    amount: input.mode === 'partial' ? input.amount : balance,
    tip: input.tip || '0.00',
    received,
    change: cash && input.received === '500' ? '230.00' : '0.00',
    balance_before: balance,
    balance_after: '0.00',
    settles: true,
    selection_total: input.selections.length > 0 ? '135.00' : null,
    units: [],
  };
}

type Handler = (body: unknown) => Promise<{ status: number; body: unknown }>;

let balance = '270.00';
let shiftState: CurrentShift = usableShift;
let payHandler: Handler;
let openShiftHandler: Handler;

function requests(operation: string) {
  return native.invoke.mock.calls.filter(
    ([command, args]) => command === 'api_request' && args?.operation === operation,
  );
}

beforeEach(() => {
  balance = '270.00';
  shiftState = usableShift;
  payHandler = async () => ({
    status: 200,
    body: {
      payment: null,
      finance: { ...finance('0.00', '2026-11-08T12:10:00Z'), settled: true },
      closed: true,
      delivered: false,
      printing: { requested: false },
    },
  });
  openShiftHandler = async () => ({ status: 200, body: usableShift.shift });
  native.invoke.mockImplementation(
    async (command: string, args: { operation: string; body?: unknown }) => {
      if (command !== 'api_request') {
        throw new Error('Comando de prueba desconocido');
      }
      switch (args.operation) {
        case 'payments_account':
          return { status: 200, body: finance(balance) };
        case 'payments_methods':
          return { status: 200, body: methods };
        case 'payments_current_shift':
          return { status: 200, body: shiftState };
        case 'payments_preview':
          return { status: 200, body: plan(args.body as PaymentInput, balance) };
        case 'payments_pay':
          return payHandler(args.body);
        case 'payments_open_shift':
          return openShiftHandler(args.body);
        default:
          return { status: 200, body: {} };
      }
    },
  );
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

function Checkout({ visible = true }: { visible?: boolean }) {
  return (
    <PendingOperationsProvider>
      {visible && (
        <CheckoutPanel
          bill={bill}
          active
          can={() => true}
          mode="collect"
          onModeChange={() => undefined}
          onClose={() => undefined}
        />
      )}
    </PendingOperationsProvider>
  );
}

async function chooseCash() {
  const group = await screen.findByRole('group', { name: 'Métodos de pago disponibles' });
  fireEvent.click(within(group).getByRole('button', { name: 'Efectivo' }));
}

function registerButton() {
  const button = document.querySelector<HTMLButtonElement>('.checkout-register');
  if (!button) {
    throw new Error('No se encontró el botón principal de registro.');
  }
  return button;
}

describe('pantalla de cobro', () => {
  it('muestra los métodos como botones sin casillas y propone el saldo pendiente', async () => {
    render(<Checkout />);
    const group = await screen.findByRole('group', { name: 'Métodos de pago disponibles' });
    const cash = within(group).getByRole('button', { name: 'Efectivo' });
    const debit = within(group).getByRole('button', { name: 'Tarjeta de débito' });
    expect(cash.getAttribute('aria-pressed')).toBe('false');
    expect(debit.getAttribute('aria-pressed')).toBe('false');
    expect(within(group).queryAllByRole('checkbox')).toHaveLength(0);
    expect(document.querySelectorAll('input[type="checkbox"]')).toHaveLength(0);
    expect(cash.querySelector('svg')).toBeTruthy();
    const proposal = screen.getByText('Saldo a cobrar').closest('.amount-field');
    expect(proposal?.textContent).toContain('$270.00');
    expect(screen.getAllByText('$270.00').length).toBeGreaterThan(0);
    expect(screen.getByText('Preparación: Sin cebolla')).toBeTruthy();
    expect(registerButton().disabled).toBe(true);
    fireEvent.click(cash);
    expect(cash.getAttribute('aria-pressed')).toBe('true');
    await waitFor(() => expect(registerButton().disabled).toBe(false));
    expect(registerButton().textContent).toBe('Registrar $270.00 · Efectivo');
  });

  it('muestra el cambio que calcula la previsualización de Go', async () => {
    render(<Checkout />);
    await chooseCash();
    for (const key of ['5', '0', '0']) {
      fireEvent.click(screen.getByRole('button', { name: key }));
    }
    expect((screen.getByLabelText('Efectivo recibido') as HTMLInputElement).value).toBe('500');
    await screen.findByText('$230.00');
    const change = screen.getByText('Cambio').closest('div');
    expect(change?.textContent).toContain('$230.00');
    const previews = requests('payments_preview');
    const last = previews[previews.length - 1]?.[1] as { body: PaymentInput };
    expect(last.body).toMatchObject({
      method_id: 'method-cash',
      mode: 'total',
      amount: '',
      received: '500',
    });
  });

  it('calcula en Go el total de la selección de productos', async () => {
    render(<Checkout />);
    fireEvent.click(
      await screen.findByRole('button', { name: 'Agregar una unidad de Hamburguesa' }),
    );
    await screen.findByText('$135.00');
    const previews = requests('payments_preview');
    const last = previews[previews.length - 1]?.[1] as { body: PaymentInput };
    expect(last.body.selections).toEqual([{ version_id: versionID, quantity: 1 }]);
  });

  it('impide el doble envío y registra con el saldo esperado mostrado', async () => {
    let finish!: (value: { status: number; body: unknown }) => void;
    payHandler = () =>
      new Promise((resolve) => {
        finish = resolve;
      });
    render(<Checkout />);
    await chooseCash();
    await waitFor(() => expect(registerButton().disabled).toBe(false));
    const button = registerButton();
    fireEvent.click(button);
    fireEvent.click(button);
    fireEvent.click(button);
    expect(requests('payments_pay')).toHaveLength(1);
    await screen.findByText('Registrando pago…', { selector: '.checkout-status' });
    expect(registerButton().disabled).toBe(true);
    const sent = requests('payments_pay')[0]?.[1] as { resourceId: string; body: PaymentInput };
    expect(sent.resourceId).toBe(accountID);
    expect(sent.body).toMatchObject({
      method_id: 'method-cash',
      mode: 'total',
      amount: '270.00',
      expected_balance: '270.00',
    });
    expect((sent.body as unknown as { operation_id: string }).operation_id).toMatch(
      /^[0-9a-f-]{36}$/,
    );
    await act(async () => {
      finish({
        status: 200,
        body: {
          payment: {
            id: 'payment',
            kind: 'general',
            method_id: 'method-cash',
            method_name: 'Efectivo',
            is_cash: true,
            amount: '270.00',
            tip: '0.00',
            received: '270.00',
            change: '0.00',
          },
          finance: { ...finance('0.00', '2026-11-08T12:10:00Z'), settled: true },
          closed: true,
          delivered: false,
          printing: { requested: false },
        },
      });
    });
    await screen.findByText('Pago registrado: $270.00 con Efectivo');
    expect(requests('payments_pay')).toHaveLength(1);
  });

  it('muestra el conflicto de cobro y el saldo real tras BALANCE_CHANGED', async () => {
    payHandler = async () => {
      balance = '120.00';
      return {
        status: 409,
        body: {
          error: {
            code: 'BALANCE_CHANGED',
            message: 'El saldo de la cuenta cambió en otra terminal.',
          },
        },
      };
    };
    render(<Checkout />);
    await chooseCash();
    await waitFor(() => expect(registerButton().disabled).toBe(false));
    fireEvent.click(registerButton());
    await screen.findByText('Conflicto de cobro', { selector: '.checkout-status' });
    await screen.findByText(/Saldo real: \$120\.00/);
    expect(screen.getByText('El saldo de la cuenta cambió en otra terminal.')).toBeTruthy();
  });

  it('bloquea nuevos cobros con un pago incierto y lo reintenta con el mismo identificador', async () => {
    payHandler = async () => {
      throw 'No se pudo conectar con el servidor.';
    };
    const view = render(<Checkout />);
    await chooseCash();
    await waitFor(() => expect(registerButton().disabled).toBe(false));
    fireEvent.click(registerButton());
    await screen.findByText(/No se pudo confirmar el resultado del pago \$270\.00 con Efectivo/);
    expect(registerButton().disabled).toBe(true);
    const first = requests('payments_pay')[0]?.[1] as { body: { operation_id: string } };

    view.rerender(<Checkout visible={false} />);
    view.rerender(<Checkout />);
    await screen.findByText(/No se pudo confirmar el resultado del pago/);
    expect(registerButton().disabled).toBe(true);

    payHandler = async () => ({
      status: 200,
      body: {
        payment: null,
        finance: { ...finance('0.00', '2026-11-08T12:10:00Z'), settled: true },
        closed: true,
        delivered: false,
        printing: { requested: false },
      },
    });
    fireEvent.click(screen.getByRole('button', { name: 'Reintentar con el mismo identificador' }));
    await screen.findByText('Pago registrado');
    const retried = requests('payments_pay')[1]?.[1] as { body: { operation_id: string } };
    expect(retried.body.operation_id).toBe(first.body.operation_id);
  });

  it('ofrece abrir el turno con un fondo inicial capturado explícitamente', async () => {
    shiftState = {
      register: { id: 'register', name: 'Caja 1', active: true },
      shift: null,
      shared_shifts: false,
      usable: false,
      reason: 'No hay un turno abierto en Caja 1. Ábrelo con su fondo inicial antes de cobrar.',
    };
    render(<Checkout />);
    await screen.findByText(/No hay un turno abierto en Caja 1/);
    const open = screen.getByRole('button', {
      name: 'Captura el fondo inicial',
    }) as HTMLButtonElement;
    expect(open.disabled).toBe(true);
    expect(requests('payments_open_shift')).toHaveLength(0);
    fireEvent.click(screen.getByRole('button', { name: '0' }));
    fireEvent.click(screen.getByRole('button', { name: 'Abrir turno con fondo de $0' }));
    await waitFor(() => expect(requests('payments_open_shift')).toHaveLength(1));
    const sent = requests('payments_open_shift')[0]?.[1] as { body: Record<string, unknown> };
    expect(sent.body.opening_float).toBe('0');
    expect(sent.body.operation_id).toMatch(/^[0-9a-f-]{36}$/);
  });
});
