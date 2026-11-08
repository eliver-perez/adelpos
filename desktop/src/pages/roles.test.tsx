// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { RolesPage } from './RolesPage';

const native = vi.hoisted(() => ({ invoke: vi.fn() }));
vi.mock('@tauri-apps/api/core', () => ({ invoke: native.invoke, isTauri: () => true }));

const roleID = '00000000-0000-0000-0000-0000000000f1';
let updateResponse: { status: number; body: unknown };

beforeEach(() => {
  updateResponse = { status: 200, body: {} };
  native.invoke.mockImplementation(async (_command: string, args: { operation: string }) => {
    switch (args.operation) {
      case 'roles':
        return {
          status: 200,
          body: [
            {
              id: roleID,
              code: null,
              name: 'Cajero',
              permissions: ['payments.collect'],
              discount_limit: '15.00',
            },
          ],
        };
      case 'permissions':
        return {
          status: 200,
          body: [{ key: 'payments.collect', description: 'Registrar cobros' }],
        };
      case 'update_role':
        return updateResponse;
      default:
        return { status: 200, body: {} };
    }
  });
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

function limitField() {
  const span = screen.getByText('Límite de descuento (%) · opcional', {
    selector: '.field > span',
  });
  return span.parentElement?.querySelector('input') as HTMLInputElement;
}

function updates() {
  return native.invoke.mock.calls.filter(([, args]) => args?.operation === 'update_role');
}

describe('límite de descuento por rol', () => {
  it('muestra, edita y envía el límite como texto decimal', async () => {
    render(<RolesPage can={() => true} onChanged={async () => undefined} />);
    expect(await screen.findByText('15.00 %')).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: 'Editar permisos' }));
    const field = limitField();
    expect(field.type).toBe('text');
    expect(field.inputMode).toBe('decimal');
    expect(field.value).toBe('15.00');
    expect(
      screen.getByText(
        /Descuentos por encima de este límite requieren autorización de un supervisor/,
      ),
    ).toBeTruthy();
    fireEvent.change(field, { target: { value: '20.5' } });
    fireEvent.change(field, { target: { value: '-3' } });
    expect(field.value).toBe('20.5');
    fireEvent.click(screen.getByRole('button', { name: 'Guardar rol' }));
    await waitFor(() => expect(updates()).toHaveLength(1));
    expect(updates()[0]?.[1]).toMatchObject({
      resourceId: roleID,
      body: { name: 'Cajero', permissions: ['payments.collect'], discount_limit: '20.5' },
    });
  });

  it('elimina el límite con la cadena vacía y muestra el rechazo de delegación de Go', async () => {
    updateResponse = {
      status: 403,
      body: {
        error: {
          code: 'DELEGATION_DENIED',
          message: 'No puedes conceder un límite de descuento mayor que el tuyo.',
        },
      },
    };
    render(<RolesPage can={() => true} onChanged={async () => undefined} />);
    fireEvent.click(await screen.findByRole('button', { name: 'Editar permisos' }));
    fireEvent.change(limitField(), { target: { value: '' } });
    fireEvent.click(screen.getByRole('button', { name: 'Guardar rol' }));
    await screen.findByText('No puedes conceder un límite de descuento mayor que el tuyo.');
    expect(updates()[0]?.[1]).toMatchObject({ body: { discount_limit: '' } });
  });
});
