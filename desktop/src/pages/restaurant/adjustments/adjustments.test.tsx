// @vitest-environment jsdom

import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { PendingOperationsProvider, usePendingEntry } from '../../../hooks/pendingOperations';
import { paymentPendingKey } from '../../../services/payments';
import type {
  DiscountInput,
  DiscountPlan,
  DiscountReason,
  Finance,
} from '../../../services/payments';
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
      reason_id: '00000000-0000-0000-0000-0000000000e2',
      reason_name: 'Promoción',
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
      reason_id: null,
      reason_name: null,
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

// Catálogo sembrado por la 0009 más un motivo desactivado, que no debe ofrecerse.
function reason(id: string, name: string, sortOrder: number, extra: Partial<DiscountReason> = {}) {
  return {
    id: `00000000-0000-0000-0000-0000000000${id}`,
    name,
    requires_description: false,
    active: true,
    sort_order: sortOrder,
    revision: 1,
    updated_at: '2026-11-09T12:00:00Z',
    ...extra,
  };
}

const reasons: DiscountReason[] = [
  reason('e1', 'Atención', 10),
  reason('e2', 'Promoción', 20),
  reason('e3', 'Demora', 30),
  reason('e4', 'Cortesía de la casa', 40),
  reason('e6', 'Cumpleaños', 45, { active: false }),
  reason('e5', 'Otro', 50, { requires_description: true }),
];

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
        case 'payments_discount_reasons':
          return { status: 200, body: reasons };
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

function reasonButton(name: string) {
  const group = screen.getByRole('group', { name: /^Motivo de/ });
  return within(group).getByRole('button', { name }) as HTMLButtonElement;
}

const otherDescription = 'Descripción del motivo «Otro»';

async function captureDiscount() {
  renderAdjustments();
  await screen.findByText('Porcentaje de descuento (%)');
  fireEvent.change(input('Porcentaje de descuento (%)'), { target: { value: '30' } });
  fireEvent.click(await screen.findByRole('button', { name: 'Otro' }));
  fireEvent.change(input(otherDescription), { target: { value: 'Cliente frecuente' } });
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
      reason_id: '00000000-0000-0000-0000-0000000000e5',
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
    expect(input(otherDescription).value).toBe('Cliente frecuente');
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
    const charged = {
      ...finance.adjustments[1]!,
      id: 'cargo-nuevo',
      value: '1250.00',
      amount: '1250.00',
    };
    writeHandler = async () => ({
      status: 200,
      body: {
        adjustment_id: 'cargo-nuevo',
        finance: { ...finance, adjustments: [...finance.adjustments, charged] },
        closed: false,
      },
    });
    renderAdjustments();
    fireEvent.click(await screen.findByRole('button', { name: 'Cargo extra' }));
    fireEvent.change(input('Importe del cargo ($)'), { target: { value: '1250' } });
    expect(submitButton(/Registrar cargo/).disabled).toBe(true);
    fireEvent.change(input('Concepto del cargo'), {
      target: { value: 'Servicio a domicilio' },
    });
    expect(submitButton(/Registrar cargo de \$1,250\.00/).disabled).toBe(false);
    fireEvent.click(submitButton(/Registrar cargo/));
    await waitFor(() => expect(requests('payments_charge')).toHaveLength(1));
    expect(lastBody('payments_charge')).toMatchObject({
      concept: 'Servicio a domicilio',
      amount: '1250',
      expected_balance: '270.00',
    });
    // La confirmación es un aviso con el importe que calculó Go, con miles.
    const notice = await screen.findByRole('status', { name: 'Operación completada' });
    expect(notice.textContent).toContain('Cargo registrado: $1,250.00.');
  });

  it('muestra cargos con signo «+» y descuentos con «−» antes del símbolo', async () => {
    renderAdjustments();
    const list = await screen.findByRole('list', { name: 'Ajustes de la cuenta' });
    expect(within(list).getByText('\u2212$27.00')).toBeTruthy();
    expect(within(list).getByText('+$40.00')).toBeTruthy();
  });

  it('un rechazo del descuento se avisa como error y no queda como banner', async () => {
    writeHandler = async () => ({
      status: 409,
      body: {
        error: {
          code: 'DISCOUNT_LIMIT_EXCEEDED',
          message: 'El descuento supera el límite permitido.',
        },
      },
    });
    await captureDiscount();
    await waitFor(() => expect(submitButton(/Aplicar descuento \$/).disabled).toBe(false));
    fireEvent.click(submitButton(/Aplicar descuento/));
    const notice = await screen.findByRole('alert', { name: 'Error' });
    expect(notice.textContent).toContain('El descuento supera el límite permitido.');
    expect(notice.textContent).not.toContain('DISCOUNT_LIMIT_EXCEEDED');
    expect(document.querySelector('.adjustments-panel .notice')).toBeNull();
  });

  it('ofrece los motivos activos como botones y envía el elegido sin descripción', async () => {
    renderAdjustments();
    fireEvent.change(await waitFor(() => input('Porcentaje de descuento (%)')), {
      target: { value: '10' },
    });
    const group = await screen.findByRole('group', { name: 'Motivo del descuento' });
    const names = within(group)
      .getAllByRole('button')
      .map((button) => button.textContent);
    // En su orden visual y sin el motivo desactivado.
    expect(names).toEqual(['Atención', 'Promoción', 'Demora', 'Cortesía de la casa', 'Otro']);
    await waitFor(() => expect(requests('payments_discount_preview').length).toBeGreaterThan(0));
    // Sin motivo elegido no se puede aplicar.
    await waitFor(() =>
      expect(screen.getByRole('button', { name: /Aplicar descuento \$/ })).toBeTruthy(),
    );
    expect(submitButton(/Aplicar descuento/).disabled).toBe(true);
    fireEvent.click(reasonButton('Atención'));
    expect(reasonButton('Atención').getAttribute('aria-pressed')).toBe('true');
    expect(reasonButton('Otro').getAttribute('aria-pressed')).toBe('false');
    // La descripción de Atención es opcional.
    expect(input('Descripción (opcional)').value).toBe('');
    await waitFor(() => expect(submitButton(/Aplicar descuento \$/).disabled).toBe(false));
    await act(async () => {
      fireEvent.click(submitButton(/Aplicar descuento/));
    });
    expect(lastBody('payments_discount')).toMatchObject({
      reason_id: '00000000-0000-0000-0000-0000000000e1',
      reason: '',
      courtesy: false,
    });
  });

  it('«Otro» pide una descripción de 5 a 500 caracteres antes de aplicar', async () => {
    renderAdjustments();
    fireEvent.change(await waitFor(() => input('Porcentaje de descuento (%)')), {
      target: { value: '10' },
    });
    fireEvent.click(await screen.findByRole('button', { name: 'Otro' }));
    await waitFor(() => expect(requests('payments_discount_preview').length).toBeGreaterThan(0));
    expect(screen.getByText('Obligatoria, de 5 a 500 caracteres.')).toBeTruthy();
    fireEvent.change(input(otherDescription), { target: { value: 'Mal' } });
    await waitFor(() =>
      expect(screen.getByRole('button', { name: /Aplicar descuento \$/ })).toBeTruthy(),
    );
    expect(submitButton(/Aplicar descuento/).disabled).toBe(true);
    fireEvent.change(input(otherDescription), { target: { value: 'Platillo frío' } });
    await waitFor(() => expect(submitButton(/Aplicar descuento \$/).disabled).toBe(false));
    await act(async () => {
      fireEvent.click(submitButton(/Aplicar descuento/));
    });
    expect(lastBody('payments_discount')).toMatchObject({
      reason_id: '00000000-0000-0000-0000-0000000000e5',
      reason: 'Platillo frío',
    });
  });

  it('la cortesía también elige su motivo con botones', async () => {
    renderAdjustments();
    fireEvent.click(await screen.findByRole('button', { name: 'Cortesía' }));
    const group = await screen.findByRole('group', { name: 'Motivo de la cortesía' });
    fireEvent.click(within(group).getByRole('button', { name: 'Cortesía de la casa' }));
    await waitFor(() => expect(submitButton(/Otorgar cortesía \$/).disabled).toBe(false));
    await act(async () => {
      fireEvent.click(submitButton(/Otorgar cortesía/));
    });
    expect(lastBody('payments_discount')).toMatchObject({
      courtesy: true,
      reason_id: '00000000-0000-0000-0000-0000000000e4',
    });
  });

  it('muestra el motivo del catálogo en el historial de ajustes', async () => {
    renderAdjustments();
    const list = await screen.findByRole('list', { name: 'Ajustes de la cuenta' });
    expect(
      within(list).getByText(/Descuento manual · Motivo: Promoción · Promoción del día/),
    ).toBeTruthy();
    expect(within(list).queryByText(/Servicio a domicilio · Motivo/)).toBeNull();
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
