// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen, within } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { PendingOperationsProvider } from '../../hooks/pendingOperations';
import { CashOperationForm } from './CashOperationForm';
import { catalog, failure, operationOf, shiftID, shiftOf } from './cashTestData';
import type { CashEntryKind } from './operationForm';
import { PendingCashOperation } from './PendingCashOperation';
import { useCashMutation } from './useCashMutation';

const native = vi.hoisted(() => ({ invoke: vi.fn() }));
vi.mock('@tauri-apps/api/core', () => ({ invoke: native.invoke, isTauri: () => true }));

type Reply = { status: number; body: unknown } | Error | string;

// Respuestas simuladas de Go en orden; una cadena simula la pérdida de la respuesta.
function serve(operation: string, replies: Reply[]) {
  native.invoke.mockImplementation(async (command: string, args: { operation: string }) => {
    if (command !== 'api_request' || args.operation !== operation) {
      throw new Error(`Operación no esperada: ${args.operation}`);
    }
    const reply = replies.shift();
    if (reply === undefined) {
      throw new Error('Sin respuesta preparada');
    }
    if (typeof reply === 'string') {
      throw reply;
    }
    if (reply instanceof Error) {
      throw reply;
    }
    return reply;
  });
}

function sent(operation: string) {
  return native.invoke.mock.calls
    .filter(([command, args]) => command === 'api_request' && args?.operation === operation)
    .map(([, args]) => args as { resourceId: string; body: Record<string, unknown> });
}

function Harness({ kind, can }: { kind: CashEntryKind; can: (permission: string) => boolean }) {
  const mutation = useCashMutation(shiftID);
  return (
    <>
      <PendingCashOperation mutation={mutation} />
      <CashOperationForm
        kind={kind}
        shift={shiftOf()}
        catalog={catalog}
        can={can}
        dollars={false}
        active
        mutation={mutation}
        onClose={() => undefined}
      />
    </>
  );
}

function renderForm(kind: CashEntryKind, can: (permission: string) => boolean = () => true) {
  return render(
    <PendingOperationsProvider>
      <Harness kind={kind} can={can} />
    </PendingOperationsProvider>,
  );
}

function type(digits: string) {
  const keypad = screen.getByRole('group', { name: 'Teclado numérico del importe' });
  for (const digit of digits) {
    fireEvent.click(within(keypad).getByRole('button', { name: digit }));
  }
}

function submit(name: RegExp) {
  const button = screen.getByRole('button', { name }) as HTMLButtonElement;
  fireEvent.click(button);
  return button;
}

afterEach(() => {
  cleanup();
  native.invoke.mockReset();
});

describe('operaciones de efectivo', () => {
  it('un retiro con permiso propio queda autoautorizado sin pedir contraseña', async () => {
    serve('payments_shift_withdrawal', [{ status: 200, body: operationOf() }]);
    renderForm('withdrawal');
    fireEvent.click(screen.getByRole('button', { name: 'Resguardo' }));
    type('200');
    expect(screen.getByText(/Autorización propia/)).toBeTruthy();
    expect(screen.queryByLabelText('Contraseña del supervisor')).toBeNull();
    submit(/^Registrar retiro \$200$/);
    await screen.findByText('Retiro registrado: $200.00 · Resguardo');
    expect(screen.getByText('Autorizado con el permiso propio de Administrador')).toBeTruthy();
    const call = sent('payments_shift_withdrawal')[0]!;
    expect(call.resourceId).toBe(shiftID);
    expect(call.body).toMatchObject({ type: 'safekeeping', amount: '200' });
    expect(call.body).not.toHaveProperty('authorization');
    expect(call.body.operation_id).toMatch(/^[0-9a-f-]{36}$/);
  });

  it('sin el permiso de autorizar, el retiro pide la autorización de otra persona', async () => {
    serve('payments_shift_withdrawal', [
      {
        status: 200,
        body: operationOf({
          authorization_mode: 'supervisor',
          authorized_by_name: 'Supervisora Ana',
        }),
      },
    ]);
    renderForm('withdrawal', (permission) => permission !== 'cash.outflows.authorize');
    fireEvent.click(screen.getByRole('button', { name: 'Resguardo' }));
    type('200');
    expect(screen.queryByText(/Autorización propia/)).toBeNull();
    expect(screen.getByText('Autorización de supervisor')).toBeTruthy();
    const button = screen.getByRole('button', { name: /^Registrar retiro/ }) as HTMLButtonElement;
    expect(button.disabled).toBe(true);
    fireEvent.change(screen.getByLabelText('Usuario del supervisor'), { target: { value: 'ana' } });
    fireEvent.change(screen.getByLabelText('Contraseña del supervisor'), {
      target: { value: 'secreta' },
    });
    expect(button.disabled).toBe(false);
    fireEvent.click(button);
    await screen.findByText('Autorizado por Supervisora Ana');
    expect(sent('payments_shift_withdrawal')[0]!.body.authorization).toEqual({
      username: 'ana',
      password: 'secreta',
    });
  });

  it('CASH_INSUFFICIENT se muestra sin cifras y ofrece una salida excepcional con motivo', async () => {
    const message =
      'El importe no puede autorizarse con el efectivo de la caja; pide a un supervisor que lo revise.';
    serve('payments_shift_withdrawal', [
      failure(409, 'CASH_INSUFFICIENT', message),
      { status: 200, body: operationOf({ amount: '900.00', exceptional: true }) },
    ]);
    renderForm('withdrawal');
    fireEvent.click(screen.getByRole('button', { name: 'Resguardo' }));
    type('900');
    submit(/^Registrar retiro \$900$/);
    const alert = await screen.findByText(message);
    expect(alert.textContent).not.toMatch(/\$|[0-9]/);
    fireEvent.click(screen.getByRole('button', { name: 'Solicitar salida excepcional' }));
    expect(screen.getByText(message)).toBeTruthy();
    const register = screen.getByRole('button', { name: /^Registrar retiro/ }) as HTMLButtonElement;
    expect(register.disabled).toBe(true);
    fireEvent.change(screen.getByLabelText('Motivo de la salida excepcional'), {
      target: { value: 'Pago urgente al proveedor de gas' },
    });
    fireEvent.click(register);
    await screen.findByText(/Salida excepcional/);
    const second = sent('payments_shift_withdrawal')[1]!.body;
    expect(second).toMatchObject({
      amount: '900',
      exceptional: true,
      reason: 'Pago urgente al proveedor de gas',
    });
    expect(second.operation_id).not.toBe(sent('payments_shift_withdrawal')[0]!.body.operation_id);
  });

  it('un reintento rechazado con SHIFT_CLOSED se explica y descarta el pendiente', async () => {
    serve('payments_shift_income', [
      'No se pudo conectar con el servidor.',
      failure(
        409,
        'SHIFT_CLOSED',
        'El turno de Caja 1 ya está cerrado. Si la operación sigue haciendo falta, regístrala como nueva en un turno abierto.',
      ),
    ]);
    renderForm('income');
    fireEvent.click(screen.getByRole('button', { name: 'Aportación adicional al fondo' }));
    type('100');
    submit(/^Registrar ingreso \$100$/);
    await screen.findByText('No se pudo confirmar el resultado del ingreso de efectivo por $100.');
    fireEvent.click(screen.getByRole('button', { name: 'Reintentar con el mismo identificador' }));
    const notice = await screen.findByText(/no se registró ni se duplicó/);
    expect(notice.textContent).toContain('El turno de Caja 1 ya está cerrado.');
    expect(notice.textContent).toContain('regístrala como una operación nueva en un turno abierto');
    expect(
      screen.queryByRole('button', { name: 'Reintentar con el mismo identificador' }),
    ).toBeNull();
    const [first, retried] = sent('payments_shift_income');
    expect(retried!.body.operation_id).toBe(first!.body.operation_id);
    // El pendiente se descartó: la captura se conserva y admite registrarse como operación nueva.
    expect(
      (screen.getByRole('button', { name: /^Registrar ingreso/ }) as HTMLButtonElement).disabled,
    ).toBe(false);
  });
});
