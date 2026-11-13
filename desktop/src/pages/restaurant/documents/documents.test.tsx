// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { PendingOperationsProvider } from '../../../hooks/pendingOperations';
import type {
  CommercialDocument,
  CurrentShift,
  Finance,
  PaymentInput,
  PaymentMethod,
  PaymentPlan,
  PaymentSettings,
} from '../../../services/payments';
import type { Account, Bill } from '../../../services/restaurant';
import { setPrecheckName } from '../../../services/documentNames';
import { registerPrinterSetup } from '../../../services/printProblems';
import { CheckoutPanel } from '../checkout/CheckoutPanel';
import { DocumentList } from './DocumentList';
import { PrecheckPanel } from './PrecheckPanel';
import { TakeawayDelivery } from './TakeawayDelivery';

const native = vi.hoisted(() => ({ invoke: vi.fn() }));
vi.mock('@tauri-apps/api/core', () => ({ invoke: native.invoke, isTauri: () => true }));

// Solo se sustituye el transporte nativo; las cifras simuladas representan lo que calcularía Go.
const accountID = '00000000-0000-0000-0000-0000000000a9';
const ticketID = '00000000-0000-0000-0000-0000000000b9';

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

let mode: 'dine_in' | 'takeaway' = 'dine_in';
let settings: Partial<PaymentSettings>;
type Response = { status: number; body: unknown };
let payHandler: (body: PaymentInput) => Response;
let writeHandler: (operation: string) => Response;

function finance(): Finance {
  return {
    account_id: accountID,
    folio: 90,
    name: 'Pedido 90',
    mode,
    state: 'open',
    revision: 2,
    table_name: null,
    items_total: '300.00',
    item_discounts: '0.00',
    account_discounts: '0.00',
    charges: '0.00',
    total: '300.00',
    paid: '0.00',
    refunded: '0.00',
    tips: '0.00',
    balance: '300.00',
    payment_state: 'not_recorded',
    delivery_state: 'not_delivered',
    settled: false,
    tax: [],
    tax_base: '0.00',
    tax_total: '0.00',
    lines: [],
    payments: [],
    refunds: [],
    adjustments: [],
    delivery: null,
    documents: [
      {
        ticket_id: ticketID,
        kind: 'receipt',
        payment_id: 'payment',
        created_at: '2026-11-08T12:05:00Z',
        job_id: 'job',
        job_state: 'submitted',
        printer_name: 'Caja térmica',
        copies: 1,
      },
    ],
    server_time: '2026-11-08T12:10:00Z',
  };
}

function plan(body: PaymentInput): PaymentPlan {
  const amount = body.mode === 'partial' ? body.amount : '300.00';
  return {
    mode: body.mode,
    target: '300.00',
    amount,
    tip: '0.00',
    received: amount,
    change: '0.00',
    balance_before: '300.00',
    balance_after: body.mode === 'partial' ? '200.00' : '0.00',
    settles: body.mode !== 'partial',
    selection_total: null,
    units: [],
  };
}

const shift: CurrentShift = {
  register: { id: 'register', name: 'Caja 1', active: true },
  shift: null,
  shared_shifts: false,
  usable: true,
  reason: '',
};

const precheckDocument: CommercialDocument = {
  kind: 'precheck',
  establishment: 'ADEL POS',
  folio: 90,
  account_name: 'Pedido 90',
  context: 'Para llevar',
  issued_at: '2026-11-08T12:10:00Z',
  local_time: '08/11/2026 06:10',
  cashier: 'Cajero',
  lines: [],
  adjustments: [],
  tax: [],
  tax_base: '258.62',
  tax_total: '41.38',
  total: '300.00',
  payments: [],
  paid: '0.00',
  tips: '0.00',
  balance: '300.00',
  payment: null,
  content: 'ADEL POS\nPRECUENTA #90\nTOTAL            $300.00',
};

function requests(operation: string) {
  return native.invoke.mock.calls.filter(
    ([command, args]) => command === 'api_request' && args?.operation === operation,
  );
}

function lastBody<T>(operation: string) {
  const calls = requests(operation);
  return (calls[calls.length - 1]?.[1] as { body: T }).body;
}

beforeEach(() => {
  mode = 'dine_in';
  settings = { print_receipt_default: true, receipt_printer_id: null, receipt_printer_name: null };
  payHandler = (body) => ({
    status: 200,
    body: {
      payment: null,
      finance: { ...finance(), balance: '0.00', settled: true },
      closed: false,
      delivered: false,
      printing: body.print ? { requested: true, status: 'not_configured' } : { requested: false },
    },
  });
  writeHandler = () => ({ status: 200, body: {} });
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
          return { status: 200, body: shift };
        case 'payments_settings':
          return { status: 200, body: settings };
        case 'payments_preview':
          return { status: 200, body: plan(args.body as PaymentInput) };
        case 'payments_pay':
          return payHandler(args.body as PaymentInput);
        case 'payments_precheck':
          return { status: 200, body: precheckDocument };
        default:
          return writeHandler(args.operation);
      }
    },
  );
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  registerPrinterSetup(null);
  setPrecheckName('precheck');
});

function bill(): Bill {
  return {
    account: {
      id: accountID,
      folio: 90,
      name: 'Pedido 90',
      mode,
      state: 'open',
      customer_name: '',
      payment_state: 'not_recorded',
    },
    draft: [],
    lines: [],
    timezone: 'America/Monterrey',
  } as unknown as Bill;
}

function Checkout() {
  return (
    <PendingOperationsProvider>
      <CheckoutPanel
        bill={bill()}
        active
        can={() => true}
        mode="collect"
        onModeChange={() => undefined}
        onClose={() => undefined}
      />
    </PendingOperationsProvider>
  );
}

function registerButton() {
  const button = document.querySelector<HTMLButtonElement>('.checkout-register');
  if (!button) {
    throw new Error('No se encontró el botón principal.');
  }
  return button;
}

async function chooseCash() {
  const group = await screen.findByRole('group', { name: 'Métodos de pago disponibles' });
  fireEvent.click(within(group).getByRole('button', { name: 'Efectivo' }));
  await waitFor(() => expect(registerButton().disabled).toBe(false));
}

describe('impresión del ticket y entrega en el cobro', () => {
  it('el interruptor de impresión toma el valor de la configuración y se envía explícito', async () => {
    render(<Checkout />);
    const toggle = (await screen.findByRole('button', {
      name: 'Imprimir ticket',
    })) as HTMLButtonElement;
    await waitFor(() => expect(toggle.getAttribute('aria-pressed')).toBe('true'));
    expect(toggle.querySelector('input')).toBeNull();
    expect(document.querySelectorAll('input[type="checkbox"]')).toHaveLength(0);
    expect(screen.getByText(/Sin impresora de tickets configurada/)).toBeTruthy();
    fireEvent.click(toggle);
    expect(toggle.getAttribute('aria-pressed')).toBe('false');
    await chooseCash();
    fireEvent.click(registerButton());
    await waitFor(() => expect(requests('payments_pay')).toHaveLength(1));
    expect(lastBody<PaymentInput>('payments_pay')).toMatchObject({ print: false, deliver: false });
  });

  it('informa la falta de impresora sin presentarla como fallo del pago', async () => {
    render(<Checkout />);
    await chooseCash();
    fireEvent.click(registerButton());
    await screen.findByText('Sin impresora configurada: el pago quedó registrado.');
    expect(lastBody<PaymentInput>('payments_pay').print).toBe(true);
    expect(screen.getAllByText(/^Pago registrado/).length).toBeGreaterThan(0);
    expect(screen.queryByText('Error recuperable')).toBeNull();
  });

  it('solo ofrece marcar como entregado en para llevar y explica un pago que no liquida', async () => {
    const view = render(<Checkout />);
    await screen.findByRole('button', { name: 'Imprimir ticket' });
    expect(screen.queryByRole('button', { name: 'Marcar como entregado' })).toBeNull();
    view.unmount();

    mode = 'takeaway';
    payHandler = () => ({
      status: 200,
      body: {
        payment: null,
        finance: { ...finance(), balance: '200.00' },
        closed: false,
        delivered: false,
        printing: { requested: false },
      },
    });
    render(<Checkout />);
    const deliver = await screen.findByRole('button', { name: 'Marcar como entregado' });
    expect(deliver.getAttribute('aria-pressed')).toBe('false');
    fireEvent.click(deliver);
    expect(deliver.getAttribute('aria-pressed')).toBe('true');
    const group = await screen.findByRole('group', { name: 'Métodos de pago disponibles' });
    fireEvent.click(within(group).getByRole('button', { name: 'Efectivo' }));
    fireEvent.click(screen.getByRole('button', { name: 'Pago parcial' }));
    for (const key of ['1', '0', '0']) {
      fireEvent.click(screen.getByRole('button', { name: key }));
    }
    await waitFor(() => expect(registerButton().disabled).toBe(false));
    fireEvent.click(registerButton());
    await screen.findByText('El pedido no se marcó entregado porque queda saldo pendiente.');
    expect(lastBody<PaymentInput>('payments_pay')).toMatchObject({
      mode: 'partial',
      amount: '100',
      deliver: true,
    });
  });
});

function takeawayAccount(overrides: Partial<Account> = {}) {
  return {
    id: accountID,
    folio: 90,
    mode: 'takeaway',
    state: 'open',
    confirmed_quantity: 2,
    payment_state: 'paid',
    delivery_state: 'not_delivered',
    ...overrides,
  } as Account;
}

function renderDelivery(account: Account, onCollect = vi.fn()) {
  render(
    <PendingOperationsProvider>
      <TakeawayDelivery
        account={account}
        timezone="America/Monterrey"
        can={() => true}
        disabled={false}
        onCollect={onCollect}
      />
    </PendingOperationsProvider>,
  );
  return onCollect;
}

describe('entrega de pedidos para llevar', () => {
  it('entrega directamente un pedido liquidado tras confirmarlo en el diálogo estándar', async () => {
    writeHandler = () => ({
      status: 200,
      body: { finance: finance(), closed: true, delivered: true },
    });
    renderDelivery(takeawayAccount());
    fireEvent.click(screen.getByRole('button', { name: 'Marcar como entregado' }));
    expect(requests('payments_deliver')).toHaveLength(0);
    const dialog = screen.getByRole('alertdialog', { name: 'Confirmar entrega' });
    expect(dialog.textContent).toContain('¿El cliente recibió el pedido #90?');
    fireEvent.click(within(dialog).getByRole('button', { name: 'Confirmar entrega' }));
    const notice = await screen.findByRole('status', { name: 'Operación completada' });
    expect(notice.textContent).toContain('Pedido #90 entregado.');
    expect(lastBody<{ operation_id: string }>('payments_deliver').operation_id).toMatch(
      /^[0-9a-f-]{36}$/,
    );
  });

  it('cancelar el diálogo de entrega no registra nada', async () => {
    renderDelivery(takeawayAccount());
    fireEvent.click(screen.getByRole('button', { name: 'Marcar como entregado' }));
    const dialog = screen.getByRole('alertdialog', { name: 'Confirmar entrega' });
    fireEvent.keyDown(dialog, { key: 'Escape' });
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull());
    expect(requests('payments_deliver')).toHaveLength(0);
  });

  it('con saldo pendiente abre el cobro en lugar de entregar', () => {
    const onCollect = renderDelivery(takeawayAccount({ payment_state: 'partial' }));
    fireEvent.click(screen.getByRole('button', { name: 'Marcar como entregado' }));
    expect(onCollect).toHaveBeenCalledOnce();
    expect(requests('payments_deliver')).toHaveLength(0);
  });

  it('informa ALREADY_DELIVERED sin registrar otra entrega', async () => {
    writeHandler = () => ({
      status: 409,
      body: { error: { code: 'ALREADY_DELIVERED', message: 'El pedido ya fue entregado.' } },
    });
    renderDelivery(takeawayAccount());
    fireEvent.click(screen.getByRole('button', { name: 'Marcar como entregado' }));
    const dialog = screen.getByRole('alertdialog', { name: 'Confirmar entrega' });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Confirmar entrega' }));
    const notice = await screen.findByRole('status', { name: 'Información' });
    expect(notice.textContent).toContain('El pedido ya estaba entregado.');
  });

  it('no ofrece entregar un pedido sin productos confirmados', () => {
    renderDelivery(takeawayAccount({ confirmed_quantity: 0, payment_state: 'not_recorded' }));
    expect(screen.queryByRole('button', { name: /Marcar.*entregado|Cobrar/ })).toBeNull();
  });
});

describe('precuenta y documentos', () => {
  function renderPrecheck() {
    render(
      <PendingOperationsProvider>
        <PrecheckPanel bill={bill()} active can={() => true} onClose={() => undefined} />
      </PendingOperationsProvider>,
    );
  }

  it('muestra el contenido de la precuenta sin registrar nada', async () => {
    renderPrecheck();
    const preview = await screen.findByLabelText('Vista previa de la precuenta');
    expect(preview.textContent).toBe(precheckDocument.content);
    expect(screen.getByText(/no es un comprobante fiscal \(CFDI\)/)).toBeTruthy();
    const operations = native.invoke.mock.calls.map(([, args]) => args?.operation);
    expect(operations.every((operation) => operation === 'payments_precheck')).toBe(true);
  });

  // Etapa 9F (§4.1, decisión D4): sin impresora válida el aviso llega en el momento, con «Configurar
  // impresora» (solo si el usuario puede elegirla) y «Reintentar» en el mismo aviso.
  it('explica PRINTER_NOT_CONFIGURED con «Configurar impresora» y «Reintentar»', async () => {
    const openSetup = vi.fn();
    registerPrinterSetup(openSetup);
    writeHandler = () => ({
      status: 409,
      body: {
        error: {
          code: 'PRINTER_NOT_CONFIGURED',
          message: 'No hay impresora configurada para precuentas.',
          details: { document: 'precheck', problem: 'missing' },
        },
      },
    });
    renderPrecheck();
    await screen.findByLabelText('Vista previa de la precuenta');
    fireEvent.click(screen.getByRole('button', { name: 'Imprimir precuenta' }));
    const notice = (
      await screen.findByText('No hay impresora configurada para precuentas.')
    ).closest('.app-toast') as HTMLElement;
    expect(notice.textContent).not.toContain('Cola de impresión');
    fireEvent.click(within(notice).getByRole('button', { name: 'Reintentar' }));
    await waitFor(() => expect(requests('payments_print_precheck')).toHaveLength(2));
    // El reintento que vuelve a fallar muestra un aviso nuevo (el anterior se retira).
    const again = (await screen.findAllByText('No hay impresora configurada para precuentas.'))
      .at(-1)!
      .closest('.app-toast') as HTMLElement;
    fireEvent.click(within(again).getByRole('button', { name: 'Configurar impresora' }));
    expect(openSetup).toHaveBeenCalledTimes(1);
  });

  it('sin permiso para elegir la impresora, el aviso dice a quién pedirla', async () => {
    writeHandler = () => ({
      status: 409,
      body: {
        error: {
          code: 'PRINTER_UNAVAILABLE',
          message: 'La impresora Tickets está desactivada.',
          details: { document: 'precheck', problem: 'inactive', printer_name: 'Tickets' },
        },
      },
    });
    renderPrecheck();
    await screen.findByLabelText('Vista previa de la precuenta');
    fireEvent.click(screen.getByRole('button', { name: 'Imprimir precuenta' }));
    const notice = (await screen.findByText('La impresora Tickets está desactivada.')).closest(
      '.app-toast',
    ) as HTMLElement;
    expect(notice.textContent).toContain('Pide a quien administra Cobros');
    expect(within(notice).queryByRole('button', { name: 'Configurar impresora' })).toBeNull();
    expect(within(notice).getByRole('button', { name: 'Reintentar' })).toBeTruthy();
  });

  // Etapa 9F (§4.5): el nombre visible de la precuenta en el panel, el botón y el aviso.
  it('usa el nombre visible que eligió el restaurante', async () => {
    setPrecheckName('sales_ticket');
    writeHandler = () => ({ status: 200, body: { requested: true, status: 'pending' } });
    renderPrecheck();
    await screen.findByLabelText('Vista previa del ticket de venta');
    expect(screen.getByRole('region', { name: 'Ticket de venta' })).toBeTruthy();
    expect(screen.getByText(/El ticket de venta solo informa el consumo/)).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: 'Imprimir ticket de venta' }));
    await screen.findByText(/Ticket de venta enviado a la cola de impresión/);
  });

  it('muestra PRINT_COPY_PENDING al pedir otra copia', async () => {
    writeHandler = () => ({
      status: 409,
      body: {
        error: {
          code: 'PRINT_COPY_PENDING',
          message: 'Ya existe otra copia sin resolver de este documento.',
        },
      },
    });
    render(
      <PendingOperationsProvider>
        <DocumentList
          accountID={accountID}
          documents={finance().documents}
          timezone="America/Monterrey"
          can={() => true}
        />
      </PendingOperationsProvider>,
    );
    expect(screen.getByText('Ticket de pago')).toBeTruthy();
    expect(screen.getByText('Enviado a la impresora')).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: 'Reimprimir ticket de pago' }));
    // Etapa 9F (decisión D3): reimprimir algo que ya salió pide motivo (5 a 500 caracteres).
    const form = screen.getByRole('form', { name: 'Reimpresión con motivo' });
    const send = within(form).getByRole('button', { name: 'Reimprimir' }) as HTMLButtonElement;
    expect(send.disabled).toBe(true);
    fireEvent.change(within(form).getByLabelText('Motivo de la reimpresión'), {
      target: { value: 'El cliente pidió otra copia' },
    });
    expect(send.disabled).toBe(false);
    fireEvent.click(send);
    await screen.findByText('Ya existe otra copia sin resolver de este documento.');
    const body = lastBody<{ ticket_id: string; reason: string }>('payments_reprint');
    expect(body.ticket_id).toBe(ticketID);
    expect(body.reason).toBe('El cliente pidió otra copia');
  });

  // Etapa 9F (§4.2): una impresión que nunca salió en papel se explica en palabras sencillas y se
  // reintenta con un toque, sin motivo.
  it('reintenta sin motivo un documento que no salió y explica qué pasó', async () => {
    writeHandler = () => ({
      status: 200,
      body: { status: 'pending', job_id: 'job', ticket_id: ticketID },
    });
    const failed = {
      ...finance().documents[0]!,
      job_state: 'error',
      job_problem: 'unreachable',
      printer_name: 'Caja térmica',
    };
    render(
      <PendingOperationsProvider>
        <DocumentList
          accountID={accountID}
          documents={[failed]}
          timezone="America/Monterrey"
          can={() => true}
        />
      </PendingOperationsProvider>,
    );
    const list = screen.getByRole('region', { name: 'Documentos de la cuenta' });
    expect(list.textContent).toContain('La impresora de caja no responde.');
    expect(list.textContent).not.toContain('Cola de impresión');
    fireEvent.click(within(list).getByRole('button', { name: 'Reintentar' }));
    await waitFor(() => expect(requests('payments_retry_document')).toHaveLength(1));
    const body = lastBody<{ ticket_id: string; job_id: string; reason?: string }>(
      'payments_retry_document',
    );
    expect(body.ticket_id).toBe(ticketID);
    expect(body.job_id).toBe('job');
    expect(body.reason).toBeUndefined();
    await screen.findByText(/Reintento enviado a la impresora/);
  });

  // Cuando un documento en camino a la impresora falla, el aviso llega en el momento con «Reintentar».
  it('avisa en el momento cuando la impresión pasa a error', async () => {
    writeHandler = () => ({
      status: 200,
      body: { status: 'pending', job_id: 'job', ticket_id: ticketID },
    });
    const pending = { ...finance().documents[0]!, job_state: 'pending' };
    const view = (documents: (typeof pending)[]) => (
      <PendingOperationsProvider>
        <DocumentList
          accountID={accountID}
          documents={documents}
          timezone="America/Monterrey"
          can={() => true}
        />
      </PendingOperationsProvider>
    );
    const { rerender } = render(view([pending]));
    rerender(view([{ ...pending, job_state: 'error', job_problem: 'unreachable' }]));
    const notice = (
      await screen.findByText('Ticket de pago: La impresora de caja no responde.')
    ).closest('.app-toast') as HTMLElement;
    fireEvent.click(within(notice).getByRole('button', { name: 'Reintentar' }));
    await waitFor(() => expect(requests('payments_retry_document')).toHaveLength(1));
  });
});
