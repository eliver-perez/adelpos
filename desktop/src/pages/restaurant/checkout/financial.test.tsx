// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { PendingOperationsProvider } from '../../../hooks/pendingOperations';
import type {
  CurrentShift,
  Finance,
  FinanceUnit,
  PaymentMethod,
  PaymentRecord,
  RefundInput,
  RefundPlan,
} from '../../../services/payments';
import type { Bill } from '../../../services/restaurant';
import { CheckoutPanel } from './CheckoutPanel';
import type { CheckoutMode } from './CheckoutPanel';

const native = vi.hoisted(() => ({ invoke: vi.fn() }));
vi.mock('@tauri-apps/api/core', () => ({ invoke: native.invoke, isTauri: () => true }));

// Solo se sustituye el transporte nativo; las cifras simuladas representan lo que calcularía Go.
const accountID = '00000000-0000-0000-0000-0000000000e1';
const versionID = '00000000-0000-0000-0000-0000000000e2';
const paymentID = '00000000-0000-0000-0000-0000000000e3';

function method(id: string, name: string, overrides: Partial<PaymentMethod> = {}): PaymentMethod {
  return {
    id,
    code: null,
    name,
    icon: 'other',
    active: true,
    sort_order: 10,
    is_cash: false,
    reference_mode: 'none',
    allows_tip: false,
    allows_refund: true,
    revision: 1,
    updated_at: '2026-11-08T12:00:00Z',
    ...overrides,
  };
}

const methods: PaymentMethod[] = [
  method('method-cash', 'Efectivo', { icon: 'cash', is_cash: true }),
  method('method-transfer', 'Transferencia', { icon: 'transfer', reference_mode: 'required' }),
  method('method-platform', 'Plataformas', { icon: 'platform', allows_refund: false }),
];

function paidUnit(id: string): FinanceUnit {
  return {
    id,
    ordinal: 1,
    total: '135.00',
    discount: '0.00',
    due: '135.00',
    paid: '135.00',
    remaining: '0.00',
    status: 'settled',
    tax_rate: 16,
  };
}

const payment: PaymentRecord = {
  id: paymentID,
  kind: 'selection',
  method_id: 'method-cash',
  method_name: 'Efectivo',
  is_cash: true,
  amount: '270.00',
  tip: '0.00',
  received: '300.00',
  change: '30.00',
  reference: '',
  corrects_payment_id: null,
  refunded_amount: '0.00',
  refunded_tip: '0.00',
  corrected: false,
  shift_id: 'shift',
  register_name: 'Caja 1',
  actor_name: 'Cajero',
  device_name: 'Terminal',
  created_at: '2026-11-08T12:00:00Z',
  unit_ids: ['unit-1', 'unit-2'],
};

// Cuenta abierta con un abono: permite cobrar el resto y devolver lo pagado.
let finance: Finance;

function baseFinance(): Finance {
  return {
    account_id: accountID,
    folio: 77,
    name: 'Mesa 9',
    mode: 'dine_in',
    state: 'open',
    revision: 5,
    table_name: 'Mesa 9',
    items_total: '405.00',
    item_discounts: '0.00',
    account_discounts: '0.00',
    charges: '0.00',
    total: '405.00',
    paid: '270.00',
    refunded: '0.00',
    tips: '0.00',
    balance: '135.00',
    payment_state: 'partial',
    delivery_state: 'not_delivered',
    settled: false,
    tax: [],
    tax_base: '0.00',
    tax_total: '0.00',
    lines: [
      {
        version_id: versionID,
        item_id: 'item',
        name: 'Hamburguesa',
        notes: '',
        quantity: 3,
        settled_quantity: 2,
        partial_quantity: 0,
        due: '405.00',
        remaining: '135.00',
        units: [
          paidUnit('unit-1'),
          paidUnit('unit-2'),
          { ...paidUnit('unit-3'), paid: '0.00', remaining: '135.00', status: 'unpaid' },
        ],
      },
    ],
    payments: [payment],
    refunds: [],
    adjustments: [],
    delivery: null,
    documents: [],
    server_time: '2026-11-08T12:10:00Z',
  };
}

const bill = {
  account: {
    id: accountID,
    folio: 77,
    name: 'Mesa 9',
    mode: 'dine_in',
    state: 'open',
    customer_name: '',
    payment_state: 'partial',
  },
  draft: [],
  lines: [],
  timezone: 'America/Monterrey',
} as unknown as Bill;

const usableShift: CurrentShift = {
  register: { id: 'register', name: 'Caja 1', active: true },
  shift: null,
  shared_shifts: false,
  usable: true,
  reason: '',
};

type Response = { status: number; body: unknown };
let refundPreview: (body: RefundInput) => Response;
let refundHandler: (body: unknown) => Promise<Response>;
let correctionHandler: (body: unknown) => Promise<Response>;

function planFor(body: RefundInput): RefundPlan {
  const amount = body.mode === 'items' ? '135.00' : '270.00';
  return {
    mode: body.mode,
    amount,
    tip: '0.00',
    parts: [{ payment_id: paymentID, amount, tip: '0.00', units: [], tax: [] }],
    requires_authorization: false,
    balance_after: body.mode === 'items' ? '270.00' : '405.00',
  };
}

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
  finance = baseFinance();
  refundPreview = (body) => ({ status: 200, body: planFor(body) });
  refundHandler = async () => ({
    status: 200,
    body: { refund_ids: ['r'], plan: planFor({ mode: 'items' }), finance, reopened: false },
  });
  correctionHandler = async () => ({
    status: 200,
    body: { reversal_id: 'r', payment_id: 'p', finance },
  });
  native.invoke.mockImplementation(
    async (command: string, args: { operation: string; body?: unknown }) => {
      if (command !== 'api_request') {
        throw new Error('Comando de prueba desconocido');
      }
      switch (args.operation) {
        case 'payments_account':
          return { status: 200, body: finance };
        case 'payments_methods':
          return { status: 200, body: methods };
        case 'payments_current_shift':
          return { status: 200, body: usableShift };
        case 'payments_refund_preview':
          return refundPreview(args.body as RefundInput);
        case 'payments_refund':
          return refundHandler(args.body);
        case 'payments_correct':
          return correctionHandler(args.body);
        case 'payments_preview':
          return { status: 200, body: null };
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

function Checkout({ mode = 'refund', visible = true }: { mode?: CheckoutMode; visible?: boolean }) {
  return (
    <PendingOperationsProvider>
      {visible && (
        <CheckoutPanel
          bill={bill}
          active
          can={() => true}
          mode={mode}
          onModeChange={() => undefined}
          onClose={() => undefined}
        />
      )}
    </PendingOperationsProvider>
  );
}

function input(label: string) {
  const span = screen.getByText(label, { selector: '.field > span' });
  const control = span.parentElement?.querySelector('input');
  if (!control) {
    throw new Error(`No se encontró el campo ${label}`);
  }
  return control;
}

function registerButton() {
  const button = document.querySelector<HTMLButtonElement>('.checkout-register');
  if (!button) {
    throw new Error('No se encontró el botón principal.');
  }
  return button;
}

async function chooseOutlet(name: string) {
  const group = await screen.findByRole('group', { name: 'Métodos de devolución disponibles' });
  fireEvent.click(within(group).getByRole('button', { name }));
}

describe('devoluciones y correcciones en la pantalla de cobro', () => {
  it('devuelve productos por un método distinto del original con referencia y motivo', async () => {
    render(<Checkout />);
    fireEvent.click(await screen.findByRole('button', { name: 'Por productos' }));
    fireEvent.click(screen.getByRole('button', { name: 'Agregar una unidad de Hamburguesa' }));
    expect(screen.getByText('de 2')).toBeTruthy();
    const group = await screen.findByRole('group', { name: 'Métodos de devolución disponibles' });
    expect(within(group).queryByRole('button', { name: 'Plataformas' })).toBeNull();
    await chooseOutlet('Transferencia');
    expect(screen.getByText(/AD-EL POS no procesa reembolsos bancarios/)).toBeTruthy();
    fireEvent.change(input('Referencia de la devolución (obligatoria)'), {
      target: { value: 'SPEI-123' },
    });
    fireEvent.change(input('Motivo de la devolución'), {
      target: { value: 'Producto equivocado' },
    });
    await waitFor(() => expect(registerButton().disabled).toBe(false));
    expect(registerButton().textContent).toBe('Registrar devolución $135.00 · Transferencia');
    fireEvent.click(registerButton());
    // La devolución no se envía hasta confirmarla en el diálogo estándar de peligro.
    const dialog = await screen.findByRole('alertdialog', {
      name: '¿Registrar la devolución de $135.00?',
    });
    expect(dialog.className).toContain('dialog-panel-danger');
    expect(dialog.textContent).toContain('Se devolverán $135.00 por Transferencia.');
    expect(dialog.textContent).toContain('Motivo: Producto equivocado');
    expect(requests('payments_refund')).toHaveLength(0);
    fireEvent.click(within(dialog).getByRole('button', { name: 'Devolver $135.00' }));
    await waitFor(() => expect(requests('payments_refund')).toHaveLength(1));
    expect(lastBody('payments_refund')).toMatchObject({
      mode: 'items',
      selections: [{ version_id: versionID, quantity: 1 }],
      method_id: 'method-transfer',
      reference: 'SPEI-123',
      reason: 'Producto equivocado',
    });
    // El resultado permanece en pantalla y la confirmación se avisa además como mensaje.
    await screen.findByText('Devolución registrada: $135.00');
    const notice = await screen.findByRole('status', { name: 'Operación completada' });
    expect(notice.textContent).toContain('Devolución de $135.00 registrada.');
  });

  it('revisar la devolución en el diálogo no envía nada', async () => {
    render(<Checkout />);
    await chooseOutlet('Efectivo');
    fireEvent.change(input('Motivo de la devolución'), { target: { value: 'Pedido cancelado' } });
    await waitFor(() => expect(registerButton().disabled).toBe(false));
    fireEvent.click(registerButton());
    const dialog = await screen.findByRole('alertdialog');
    fireEvent.click(within(dialog).getByRole('button', { name: 'Revisar la devolución' }));
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull());
    expect(requests('payments_refund')).toHaveLength(0);
    expect((input('Motivo de la devolución') as HTMLInputElement).value).toBe('Pedido cancelado');
  });

  it('muestra NOTHING_TO_REFUND cuando ya no hay importe devolvible', async () => {
    refundPreview = () => ({
      status: 409,
      body: {
        error: {
          code: 'NOTHING_TO_REFUND',
          message: 'No hay importe devolvible con esa selección: ya se devolvió o no se pagó.',
        },
      },
    });
    render(<Checkout />);
    await screen.findByText(/No hay importe devolvible con esa selección/);
    await chooseOutlet('Efectivo');
    fireEvent.change(input('Motivo de la devolución'), { target: { value: 'Cliente molesto' } });
    expect(registerButton().disabled).toBe(true);
  });

  it('informa que una corrección con el turno cerrado requiere una regularización posterior', async () => {
    correctionHandler = async () => ({
      status: 409,
      body: {
        error: {
          code: 'SHIFT_CLOSED',
          message: 'El turno del pago ya está cerrado.',
        },
      },
    });
    render(<Checkout mode="collect" />);
    const history = await screen.findByRole('list', { name: 'Pagos registrados' });
    fireEvent.click(within(history).getByRole('button', { name: 'Corregir método' }));
    const group = await screen.findByRole('group', { name: 'Métodos correctos disponibles' });
    expect(within(group).queryByRole('button', { name: 'Efectivo' })).toBeNull();
    fireEvent.click(within(group).getByRole('button', { name: 'Transferencia' }));
    fireEvent.change(input('Referencia (obligatoria)'), { target: { value: 'SPEI-9' } });
    fireEvent.change(input('Motivo de la corrección'), {
      target: { value: 'Se cobró con transferencia' },
    });
    fireEvent.click(screen.getByRole('button', { name: 'Corregir a Transferencia' }));
    await screen.findByText(/Usa una regularización posterior \(etapa 7\)/);
    expect(lastBody('payments_correct')).toMatchObject({
      payment_id: paymentID,
      method_id: 'method-transfer',
      reference: 'SPEI-9',
      reason: 'Se cobró con transferencia',
    });
  });

  it('una devolución incierta bloquea un cobro nuevo y se reintenta con el mismo identificador', async () => {
    refundHandler = async () => {
      throw 'No se pudo conectar con el servidor.';
    };
    const view = render(<Checkout />);
    await chooseOutlet('Efectivo');
    fireEvent.change(input('Motivo de la devolución'), { target: { value: 'Pedido cancelado' } });
    await waitFor(() => expect(registerButton().disabled).toBe(false));
    fireEvent.click(registerButton());
    const dialog = await screen.findByRole('alertdialog');
    fireEvent.click(within(dialog).getByRole('button', { name: /^Devolver / }));
    await screen.findByText(/No se pudo confirmar el resultado de la devolución/);
    // Un resultado incierto se queda en su panel de recuperación: no se avisa como error.
    expect(screen.queryByRole('alert', { name: 'Error' })).toBeNull();
    const first = lastBody<{ operation_id: string }>('payments_refund');

    // Al cambiar al cobro de la misma cuenta, el pendiente bloquea un pago nuevo.
    view.rerender(<Checkout mode="collect" />);
    await screen.findByText(/No se pudo confirmar el resultado de la devolución/);
    const methodsGroup = screen.getByRole('group', { name: 'Métodos de pago disponibles' });
    expect(
      (within(methodsGroup).getByRole('button', { name: 'Efectivo' }) as HTMLButtonElement)
        .disabled,
    ).toBe(true);
    expect(registerButton().disabled).toBe(true);

    refundHandler = async () => ({
      status: 200,
      body: { refund_ids: ['r'], plan: planFor({ mode: 'total' }), finance, reopened: false },
    });
    fireEvent.click(screen.getByRole('button', { name: 'Reintentar con el mismo identificador' }));
    await waitFor(() => expect(requests('payments_refund')).toHaveLength(2));
    expect(lastBody<{ operation_id: string }>('payments_refund').operation_id).toBe(
      first.operation_id,
    );
    await waitFor(() =>
      expect(screen.queryByText(/No se pudo confirmar el resultado de la devolución/)).toBeNull(),
    );
  });
});
