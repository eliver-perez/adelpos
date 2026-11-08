// @vitest-environment jsdom

import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { PendingOperationsProvider, usePendingEntry } from '../../../hooks/pendingOperations';
import { paymentPendingKey } from '../../../services/payments';
import type { DiscountInput, DiscountPlan, Finance } from '../../../services/payments';
import type { Bill } from '../../../services/restaurant';
import { AccountAdjustments } from './AccountAdjustments';

const native = vi.hoisted(() => ({ invoke: vi.fn() }));
vi.mock('@tauri-apps/api/core', () => ({ invoke: native.invoke, isTauri: () => true }));

// Solo se sustituye el transporte nativo; las cifras simuladas representan lo que calcularía Go.
const accountID = '00000000-0000-0000-0000-0000000000c1';
const versionID = '00000000-0000-0000-0000-0000000000c2';

function unit(id: string) {
  return {
    id,
    ordinal: 1,
    total: '135.00',
    discount: '0.00',
    due: '135.00',
    paid: '0.00',
    remaining: '135.00',
    status: 'unpaid' as const,
    tax_rate: 16,
  };
}

const finance: Finance = {
  account_id: accountID,
  folio: 52,
  name: 'Mesa 2',
  mode: 'dine_in',
  state: 'open',
  revision: 2,
  table_name: 'Mesa 2',
  items_total: '270.00',
  item_discounts: '0.00',
  account_discounts: '0.00',
  charges: '0.00',
  total: '270.00',
  paid: '0.00',
  refunded: '0.00',
  tips: '0.00',
  balance: '270.00',
  payment_state: 'not_recorded',
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
      quantity: 2,
      settled_quantity: 0,
      partial_quantity: 0,
      due: '270.00',
      remaining: '270.00',
      units: [unit('unit-1'), unit('unit-2')],
    },
  ],
  payments: [],
  refunds: [],
  adjustments: [
    {
      id: '00000000-0000-0000-0000-0000000000d1',
      kind: 'discount',
      scope: 'account',
      method: 'percent',
      value: '10.00',
      amount: '27.00',
      courtesy: false,
      tax_rate: null,
      concept: 'Descuento manual',
      reason: 'Promoción del día',
      actor_name: 'Cajero',
      authorized_by_name: null,
      created_at: '2026-11-08T12:00:00Z',
      voided: false,
      voided_at: null,
      void_reason: null,
      unit_ids: [],
    },
    {
      id: '00000000-0000-0000-0000-0000000000d2',
      kind: 'charge',
      scope: 'account',
      method: 'amount',
      value: '40.00',
      amount: '40.00',
      courtesy: false,
      tax_rate: 16,
      concept: 'Servicio a domicilio',
      reason: '',
      actor_name: 'Cajero',
      authorized_by_name: null,
      created_at: '2026-11-08T12:01:00Z',
      voided: true,
      voided_at: '2026-11-08T12:05:00Z',
      void_reason: 'Capturado por error',
      unit_ids: [],
    },
  ],
  delivery: null,
  documents: [],
  server_time: '2026-11-08T12:10:00Z',
};

const bill = {
  account: { id: accountID, folio: 52, name: 'Mesa 2', state: 'open', mode: 'dine_in' },
  draft: [],
  lines: [],
  timezone: 'America/Monterrey',
} as unknown as Bill;

function plan(input: DiscountInput, requiresAuthorization: boolean): DiscountPlan {
  return {
    scope: input.scope,
    method: input.method,
    value: input.value,
    base: '270.00',
    amount: input.courtesy ? '270.00' : '81.00',
    percent: input.courtesy ? '100.00' : '30.00',
    courtesy: input.courtesy,
    actor_limit: '10.00',
    requires_authorization: requiresAuthorization,
    balance_after: input.courtesy ? '0.00' : '189.00',
    settles: input.courtesy,
    units: [],
  };
}

type Response = { status: number; body: unknown };
let granted: string[] = [];
let requiresAuthorization = false;
let writeHandler: (operation: string, body: unknown) => Promise<Response>;

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
  granted = [
    'payments.read',
    'payments.collect',
    'payments.discounts.apply',
    'payments.courtesy',
    'payments.charges.create',
  ];
  requiresAuthorization = false;
  writeHandler = async () => ({
    status: 200,
    body: { adjustment_id: 'nuevo', finance, closed: false },
  });
  native.invoke.mockImplementation(
    async (command: string, args: { operation: string; body?: unknown }) => {
      if (command !== 'api_request') {
        throw new Error('Comando de prueba desconocido');
      }
      switch (args.operation) {
        case 'payments_account':
          return { status: 200, body: finance };
        case 'payments_discount_preview':
          return {
            status: 200,
            body: plan(args.body as DiscountInput, requiresAuthorization),
          };
        case 'payments_discount':
        case 'payments_charge':
        case 'payments_void_adjustment':
          return writeHandler(args.operation, args.body);
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

// Muestra lo que conserva el almacén global para la cuenta (nunca debe incluir contraseñas).
function PendingProbe() {
  const entry = usePendingEntry(paymentPendingKey(accountID));
  return <pre data-testid="pendiente">{entry ? JSON.stringify(entry.request) : ''}</pre>;
}

function renderAdjustments() {
  return render(
    <PendingOperationsProvider>
      <AccountAdjustments
        bill={bill}
        active
        can={(permission) => granted.includes(permission)}
        onClose={() => undefined}
      />
      <PendingProbe />
    </PendingOperationsProvider>,
  );
}

// Field incluye la ayuda dentro de la etiqueta; se localiza el campo por su texto visible.
function input(label: string) {
  const span = screen.getByText(label, { selector: '.field > span' });
  const control = span.parentElement?.querySelector('input');
  if (!control) {
    throw new Error(`No se encontró el campo ${label}`);
  }
  return control;
}

function submitButton(name: RegExp) {
  return screen.getByRole('button', { name }) as HTMLButtonElement;
}

async function captureDiscount() {
  renderAdjustments();
  await screen.findByText('Porcentaje de descuento (%)');
  fireEvent.change(input('Porcentaje de descuento (%)'), { target: { value: '30' } });
  fireEvent.change(input('Motivo'), { target: { value: 'Cliente frecuente' } });
}

describe('descuentos, cortesías, cargos y cancelaciones', () => {
  it('pide autorización de supervisor y la envía sin conservar la contraseña', async () => {
    requiresAuthorization = true;
    writeHandler = async () => {
      throw 'No se pudo conectar con el servidor.';
    };
    await captureDiscount();
    await screen.findByText('Autorización de supervisor');
    expect(submitButton(/Aplicar descuento/).disabled).toBe(true);
    fireEvent.change(screen.getByLabelText('Usuario del supervisor'), {
      target: { value: 'supervisor' },
    });
    const password = screen.getByLabelText('Contraseña del supervisor') as HTMLInputElement;
    expect(password.type).toBe('password');
    fireEvent.change(password, { target: { value: 'secreto' } });
    await waitFor(() => expect(submitButton(/Aplicar descuento \$81\.00/).disabled).toBe(false));
    fireEvent.click(submitButton(/Aplicar descuento/));
    await waitFor(() => expect(requests('payments_discount')).toHaveLength(1));
    const sent = lastBody<DiscountInput & { operation_id: string }>('payments_discount');
    expect(sent).toMatchObject({
      scope: 'account',
      method: 'percent',
      value: '30',
      courtesy: false,
      reason: 'Cliente frecuente',
      expected_balance: '270.00',
      authorization: { username: 'supervisor', password: 'secreto' },
    });
    // Tras el envío la contraseña desaparece del formulario y del almacén de pendientes.
    expect(password.value).toBe('');
    await screen.findByText(/No se pudo confirmar el resultado del descuento/);
    const stored = screen.getByTestId('pendiente').textContent ?? '';
    expect(stored).toContain(sent.operation_id);
    expect(stored).toContain('supervisor');
    expect(stored).not.toContain('secreto');
  });

  it('pide repetir la autorización si el reintento sin contraseña no llegó a confirmarse', async () => {
    requiresAuthorization = true;
    writeHandler = async () => {
      throw 'No se pudo conectar con el servidor.';
    };
    await captureDiscount();
    fireEvent.change(await screen.findByLabelText('Usuario del supervisor'), {
      target: { value: 'supervisor' },
    });
    fireEvent.change(screen.getByLabelText('Contraseña del supervisor'), {
      target: { value: 'secreto' },
    });
    await waitFor(() => expect(submitButton(/Aplicar descuento \$81/).disabled).toBe(false));
    fireEvent.click(submitButton(/Aplicar descuento/));
    await screen.findByText(/La contraseña del supervisor no se conserva/);
    // Go recibe el reintento sin contraseña: si el original no se confirmó, exige la autorización.
    writeHandler = async (_operation, body) => {
      expect(JSON.stringify(body)).not.toContain('secreto');
      return {
        status: 400,
        body: {
          error: {
            code: 'AUTHORIZATION_REQUIRED',
            message: 'Indica el usuario y la contraseña del supervisor que autoriza.',
          },
        },
      };
    };
    fireEvent.click(screen.getByRole('button', { name: 'Reintentar con el mismo identificador' }));
    await screen.findByText(/su autorización no se conserva\. Solicita de nuevo/);
    expect(screen.getByTestId('pendiente').textContent).toBe('');
    expect(requests('payments_discount')).toHaveLength(2);
  });

  it('muestra AUTHORIZATION_INVALID junto al formulario sin cerrar la sesión', async () => {
    requiresAuthorization = true;
    writeHandler = async () => ({
      status: 401,
      body: {
        error: {
          code: 'AUTHORIZATION_INVALID',
          message: 'Usuario o contraseña del supervisor inválidos.',
        },
      },
    });
    const sessionInvalid = vi.fn();
    window.addEventListener('session-invalid', sessionInvalid);
    await captureDiscount();
    fireEvent.change(await screen.findByLabelText('Usuario del supervisor'), {
      target: { value: 'supervisor' },
    });
    fireEvent.change(screen.getByLabelText('Contraseña del supervisor'), {
      target: { value: 'incorrecta' },
    });
    await waitFor(() => expect(submitButton(/Aplicar descuento \$81/).disabled).toBe(false));
    fireEvent.click(submitButton(/Aplicar descuento/));
    await screen.findByText('Usuario o contraseña del supervisor inválidos.');
    expect(sessionInvalid).not.toHaveBeenCalled();
    window.removeEventListener('session-invalid', sessionInvalid);
    expect((screen.getByLabelText('Contraseña del supervisor') as HTMLInputElement).value).toBe('');
    expect(input('Motivo').value).toBe('Cliente frecuente');
    expect(screen.getByTestId('pendiente').textContent).toBe('');
  });

  it('presenta la cortesía como un modo que fija el 100 %', async () => {
    renderAdjustments();
    fireEvent.click(await screen.findByRole('button', { name: 'Cortesía' }));
    expect(screen.queryByText('Porcentaje de descuento (%)')).toBeNull();
    expect(screen.getByText('100 % · Cortesía')).toBeTruthy();
    await waitFor(() => expect(requests('payments_discount_preview').length).toBeGreaterThan(0));
    expect(lastBody<DiscountInput>('payments_discount_preview')).toMatchObject({
      method: 'percent',
      value: '100',
      courtesy: true,
    });
    await screen.findByText(/La cuenta quedará liquidada sin registrar un pago de \$0/);
  });

  it('exige el concepto del cargo y envía el saldo esperado', async () => {
    renderAdjustments();
    fireEvent.click(await screen.findByRole('button', { name: 'Cargo extra' }));
    fireEvent.change(input('Importe del cargo ($)'), { target: { value: '50' } });
    expect(submitButton(/Registrar cargo/).disabled).toBe(true);
    fireEvent.change(input('Concepto del cargo'), {
      target: { value: 'Servicio a domicilio' },
    });
    expect(submitButton(/Registrar cargo de \$50/).disabled).toBe(false);
    fireEvent.click(submitButton(/Registrar cargo/));
    await waitFor(() => expect(requests('payments_charge')).toHaveLength(1));
    expect(lastBody('payments_charge')).toMatchObject({
      concept: 'Servicio a domicilio',
      amount: '50',
      expected_balance: '270.00',
    });
    await screen.findByText('Cargo registrado.');
  });

  it('cancela un ajuste con motivo y conserva los cancelados como históricos', async () => {
    renderAdjustments();
    const list = await screen.findByRole('list', { name: 'Ajustes de la cuenta' });
    expect(within(list).getByText(/Cancelado: Capturado por error/)).toBeTruthy();
    expect(within(list).getAllByRole('button', { name: 'Cancelar ajuste' })).toHaveLength(1);
    fireEvent.click(within(list).getByRole('button', { name: 'Cancelar ajuste' }));
    const confirm = screen.getByRole('button', {
      name: 'Confirmar cancelación',
    }) as HTMLButtonElement;
    expect(confirm.disabled).toBe(true);
    expect(screen.queryByText('Autorización de supervisor')).toBeNull();
    fireEvent.change(input('Motivo de la cancelación'), {
      target: { value: 'Descuento aplicado por error' },
    });
    expect(confirm.disabled).toBe(false);
    await act(async () => {
      fireEvent.click(confirm);
    });
    expect(lastBody('payments_void_adjustment')).toMatchObject({
      adjustment_id: '00000000-0000-0000-0000-0000000000d1',
      reason: 'Descuento aplicado por error',
    });
  });
});
