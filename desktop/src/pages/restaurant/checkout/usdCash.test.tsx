// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
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

// Solo se sustituye el transporte nativo: toda cifra en pesos y dólares es la que entregaría Go.
const accountID = '00000000-0000-0000-0000-0000000000c1';
const shiftID = '00000000-0000-0000-0000-0000000000d1';

const cash: PaymentMethod = {
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
};

function finance(): Finance {
  return {
    account_id: accountID,
    folio: 52,
    name: 'Mesa 7',
    mode: 'dine_in',
    state: 'open',
    revision: 2,
    table_name: 'Mesa 7',
    items_total: '290.00',
    item_discounts: '0.00',
    account_discounts: '0.00',
    charges: '0.00',
    total: '290.00',
    paid: '0.00',
    refunded: '0.00',
    tips: '0.00',
    balance: '290.00',
    payment_state: 'not_recorded',
    delivery_state: 'not_delivered',
    settled: false,
    tax: [{ rate: 16, base: '250.00', tax: '40.00', total: '290.00' }],
    tax_base: '250.00',
    tax_total: '40.00',
    lines: [],
    payments: [],
    refunds: [],
    adjustments: [],
    delivery: null,
    documents: [],
    server_time: '2026-11-08T12:00:00Z',
  };
}

const bill = {
  account: {
    id: accountID,
    folio: 52,
    name: 'Mesa 7',
    mode: 'dine_in',
    state: 'open',
    table_id: 'table',
    table_name: 'Mesa 7',
    customer_name: '',
    revision: 2,
    confirmed_quantity: 2,
    draft_quantity: 0,
    payment_state: 'not_recorded',
    delivery_state: 'not_delivered',
    finance: {
      total: '290.00',
      paid: '0.00',
      balance: '290.00',
      tips: '0.00',
      payment_state: 'not_recorded',
      delivery_state: 'not_delivered',
    },
  },
  draft: [],
  lines: [],
  server_time: '2026-11-08T12:00:00Z',
  timezone: 'America/Monterrey',
} as unknown as Bill;

function shift(rate: string | null) {
  return {
    id: shiftID,
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
    usd_rate: rate,
    usd_rate_set_by_name: rate ? 'Administrador' : null,
    usd_rate_set_at: rate ? '2026-11-08T11:00:00Z' : null,
  };
}

function current(options: { dollars: boolean; rate: string | null; open?: boolean }): CurrentShift {
  const open = options.open ?? true;
  return {
    register: { id: 'register', name: 'Caja 1', active: true },
    shift: open ? shift(options.rate) : null,
    shared_shifts: false,
    usable: open,
    reason: open
      ? ''
      : 'No hay un turno abierto en Caja 1. Ábrelo con su fondo inicial antes de cobrar.',
    usd_cash: options.dollars,
    usd_rate_required: options.dollars && open && !options.rate,
  };
}

// Vista previa simulada de Go: con dólares, 100 pesos y 20 dólares a 18.50 dan 470 pesos.
function plan(input: PaymentInput, dollars: boolean): PaymentPlan {
  const base: PaymentPlan = {
    mode: input.mode,
    target: '290.00',
    amount: '290.00',
    tip: '0.00',
    received: '290.00',
    change: '0.00',
    balance_before: '290.00',
    balance_after: '0.00',
    settles: true,
    selection_total: null,
    units: [],
  };
  if (!dollars) {
    return base;
  }
  const withDollars = input.received_usd === '20';
  const inDollars = input.change_currency === 'USD';
  return {
    ...base,
    received: withDollars ? '470.00' : '290.00',
    change: withDollars ? '180.00' : '0.00',
    usd: {
      rate: '18.50',
      balance_usd: '15.68',
      charged_usd: '15.68',
      received_mxn: withDollars ? '100.00' : '290.00',
      received_usd: withDollars ? '20.00' : '0.00',
      received_usd_mxn: withDollars ? '370.00' : '0.00',
      received_total: withDollars ? '470.00' : '290.00',
      change_mxn: withDollars ? '180.00' : '0.00',
      change_usd: withDollars ? '9.73' : '0.00',
      change_currency: withDollars && inDollars ? 'USD' : 'MXN',
      covered: true,
      missing_mxn: '0.00',
      missing_usd: '0.00',
    },
  };
}

type Reply = { status: number; body: unknown };

let shiftState: CurrentShift;
let dollarsEnabled = false;
let openShiftReplies: Reply[] = [];
let rateReply: Reply;

function requests(operation: string) {
  return native.invoke.mock.calls
    .filter(([command, args]) => command === 'api_request' && args?.operation === operation)
    .map(([, args]) => args as { resourceId?: string; body: Record<string, unknown> });
}

function lastBody(operation: string) {
  const calls = requests(operation);
  return calls[calls.length - 1]?.body ?? null;
}

beforeEach(() => {
  dollarsEnabled = false;
  shiftState = current({ dollars: false, rate: null });
  openShiftReplies = [];
  rateReply = { status: 200, body: shift('18.50') };
  native.invoke.mockImplementation(
    async (command: string, args: { operation: string; body?: unknown }) => {
      if (command !== 'api_request') {
        throw new Error('Comando de prueba desconocido');
      }
      switch (args.operation) {
        case 'payments_account':
          return { status: 200, body: finance() };
        case 'payments_methods':
          return { status: 200, body: [cash] };
        case 'payments_current_shift':
          return { status: 200, body: shiftState };
        case 'payments_settings':
          return { status: 200, body: { print_receipt_default: false, receipt_printer_id: null } };
        case 'payments_preview':
          return { status: 200, body: plan(args.body as PaymentInput, dollarsEnabled) };
        case 'payments_pay':
          return {
            status: 200,
            body: {
              payment: null,
              finance: {
                ...finance(),
                balance: '0.00',
                settled: true,
                server_time: '2026-11-08T12:10:00Z',
              },
              closed: true,
              delivered: false,
              printing: { requested: false },
            },
          };
        case 'payments_open_shift':
          return openShiftReplies.shift() ?? { status: 200, body: shift('16.00') };
        case 'payments_shift_usd_rate':
          return rateReply;
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

function Checkout() {
  return (
    <PendingOperationsProvider>
      <CheckoutPanel
        bill={bill}
        active
        can={() => true}
        mode="collect"
        onModeChange={() => undefined}
        onClose={() => undefined}
      />
    </PendingOperationsProvider>
  );
}

async function chooseCash() {
  const group = await screen.findByRole('group', { name: 'Métodos de pago disponibles' });
  fireEvent.click(within(group).getByRole('button', { name: 'Efectivo' }));
}

function press(...keys: string[]) {
  const keypad = screen.getByRole('group', { name: /Teclado numérico/ });
  for (const key of keys) {
    const name = key === '.' ? 'Punto decimal' : key;
    fireEvent.click(within(keypad).getByRole('button', { name }));
  }
}

function registerButton() {
  const button = document.querySelector<HTMLButtonElement>('.checkout-register');
  if (!button) {
    throw new Error('No se encontró el botón principal.');
  }
  return button;
}

describe('pago en dólares en el cobro', () => {
  it('con la función desactivada el cobro queda como en la etapa 6', async () => {
    render(<Checkout />);
    await chooseCash();
    await waitFor(() => expect(registerButton().disabled).toBe(false));
    expect(screen.getByLabelText('Efectivo recibido')).toBeTruthy();
    expect(screen.queryByLabelText('Dólares recibidos')).toBeNull();
    expect(screen.queryByText('Tipo de cambio del turno')).toBeNull();
    expect(screen.queryByRole('group', { name: 'Cambio entregado en' })).toBeNull();
    const body = lastBody('payments_preview');
    expect(body).not.toHaveProperty('received_usd');
    expect(body).not.toHaveProperty('change_currency');
  });

  it('captura pesos y dólares con el teclado y muestra el cambio en ambas monedas de Go', async () => {
    dollarsEnabled = true;
    shiftState = current({ dollars: true, rate: '18.50' });
    render(<Checkout />);
    await chooseCash();
    expect(await screen.findByText('Tipo de cambio del turno')).toBeTruthy();
    expect(screen.getByText('$18.50 por dólar')).toBeTruthy();
    expect(await screen.findByText(/equivale a US\$15\.68/)).toBeTruthy();
    // El teclado escribe en el campo activo: primero pesos y luego dólares.
    expect(screen.getByLabelText('Pesos recibidos')).toBeTruthy();
    press('1', '0', '0');
    fireEvent.click(screen.getByLabelText('Dólares recibidos'));
    press('2', '0');
    expect((screen.getByLabelText('Pesos recibidos') as HTMLInputElement).value).toBe('100');
    expect((screen.getByLabelText('Dólares recibidos') as HTMLInputElement).value).toBe('20');
    // El cambio se destaca en la moneda en que se entrega y muestra debajo la otra moneda.
    await screen.findByText('En dólares: US$9.73');
    const pesosChange = screen.getByText('Cambio en pesos').closest('div');
    expect(pesosChange?.textContent).toContain('$180.00');
    expect(pesosChange?.textContent).toContain('US$9.73');
    expect(pesosChange?.className).toContain('checkout-figure-emphasis');
    const summary = screen.getByLabelText('Pago en dólares');
    expect(summary.textContent).toContain('$370.00');
    expect(summary.textContent).toContain('total $470.00');
    expect(lastBody('payments_preview')).toMatchObject({ received: '100', received_usd: '20' });
    // «Cambio entregado en» inicia en pesos; elegir dólares lo envía a Go.
    const selector = screen.getByRole('group', { name: 'Cambio entregado en' });
    expect(
      within(selector).getByRole('button', { name: 'Pesos' }).getAttribute('aria-pressed'),
    ).toBe('true');
    fireEvent.click(within(selector).getByRole('button', { name: 'Dólares' }));
    await waitFor(() =>
      expect(lastBody('payments_preview')).toMatchObject({ change_currency: 'USD' }),
    );
    await waitFor(() =>
      expect(screen.getByText('Cambio en dólares').closest('div')?.className).toContain(
        'checkout-figure-emphasis',
      ),
    );
    expect(screen.getByText('Cambio en dólares').closest('div')?.textContent).toContain(
      'En pesos: $180.00',
    );
    await waitFor(() => expect(registerButton().disabled).toBe(false));
    fireEvent.click(registerButton());
    await waitFor(() => expect(requests('payments_pay')).toHaveLength(1));
    expect(lastBody('payments_pay')).toMatchObject({
      method_id: 'method-cash',
      received: '100',
      received_usd: '20',
      change_currency: 'USD',
      expected_balance: '290.00',
    });
  });

  it('un turno sin tipo de cambio no acepta dólares y permite capturarlo con permiso', async () => {
    dollarsEnabled = true;
    shiftState = current({ dollars: true, rate: null });
    render(<Checkout />);
    await chooseCash();
    expect(await screen.findByText(/no tiene tipo de cambio del dólar/)).toBeTruthy();
    expect(screen.queryByLabelText('Dólares recibidos')).toBeNull();
    fireEvent.click(screen.getByRole('button', { name: 'Capturar tipo de cambio' }));
    expect(await screen.findByLabelText('Tipo de cambio del dólar')).toBeTruthy();
    press('1', '8', '.', '5', '0');
    shiftState = current({ dollars: true, rate: '18.50' });
    fireEvent.click(screen.getByRole('button', { name: 'Fijar el dólar a $18.50' }));
    await waitFor(() => expect(requests('payments_shift_usd_rate')).toHaveLength(1));
    const call = requests('payments_shift_usd_rate')[0];
    expect(call?.resourceId).toBe(shiftID);
    expect(call?.body).toMatchObject({ usd_rate: '18.50' });
    expect(await screen.findByLabelText('Dólares recibidos')).toBeTruthy();
  });

  it('abre el turno con tipo de cambio y pide confirmación si Go detecta una diferencia grande', async () => {
    shiftState = current({ dollars: true, rate: null, open: false });
    openShiftReplies = [
      {
        status: 409,
        body: {
          error: {
            code: 'USD_RATE_CONFIRMATION_REQUIRED',
            message:
              'El tipo de cambio $16.00 difiere 13.51 % del turno anterior ($18.50). Confirma que es correcto.',
          },
        },
      },
    ];
    render(<Checkout />);
    expect(await screen.findByLabelText('Fondo inicial')).toBeTruthy();
    press('5', '0', '0');
    fireEvent.click(screen.getByLabelText('Tipo de cambio del dólar'));
    press('1', '6');
    expect((screen.getByLabelText('Fondo inicial') as HTMLInputElement).value).toBe('500');
    expect((screen.getByLabelText('Tipo de cambio del dólar') as HTMLInputElement).value).toBe(
      '16',
    );
    fireEvent.click(
      screen.getByRole('button', { name: 'Abrir turno con fondo de $500 y dólar a $16' }),
    );
    const dialog = await screen.findByRole('alertdialog');
    expect(dialog.textContent).toContain('difiere 13.51 %');
    expect(requests('payments_open_shift')[0]?.body).toMatchObject({
      opening_float: '500',
      usd_rate: '16',
    });
    expect(requests('payments_open_shift')[0]?.body).not.toHaveProperty('confirm_rate_difference');
    fireEvent.click(
      within(dialog).getByRole('button', { name: 'Sí, el tipo de cambio es correcto' }),
    );
    await waitFor(() => expect(requests('payments_open_shift')).toHaveLength(2));
    expect(requests('payments_open_shift')[1]?.body).toMatchObject({
      opening_float: '500',
      usd_rate: '16',
      confirm_rate_difference: true,
    });
  });

  it('sin la función activa la apertura no pide tipo de cambio', async () => {
    shiftState = current({ dollars: false, rate: null, open: false });
    render(<Checkout />);
    expect(await screen.findByLabelText('Fondo inicial')).toBeTruthy();
    expect(screen.queryByLabelText('Tipo de cambio del dólar')).toBeNull();
    press('0');
    fireEvent.click(screen.getByRole('button', { name: 'Abrir turno con fondo de $0' }));
    await waitFor(() => expect(requests('payments_open_shift')).toHaveLength(1));
    expect(requests('payments_open_shift')[0]?.body).not.toHaveProperty('usd_rate');
  });
});
