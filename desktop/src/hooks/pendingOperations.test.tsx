// @vitest-environment jsdom

import { act, cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { api, APIError } from '../services/api';
import { PendingOperationsProvider, storableRequest, usePendingEntry } from './pendingOperations';
import { useRestaurantMutation } from './useRestaurantMutation';

vi.mock('../services/api', async (original) => ({
  ...(await original<typeof import('../services/api')>()),
  api: vi.fn(),
}));

beforeEach(() => vi.mocked(api).mockReset());
afterEach(cleanup);

const key = 'payments:cuenta-1';

// Página mínima de cobro que se monta y desmonta dentro del mismo proveedor de aplicación.
function PaymentPage() {
  const mutation = useRestaurantMutation(() => undefined, key);
  const entry = usePendingEntry(key);
  return (
    <section>
      <button
        onClick={() =>
          void mutation
            .execute('payments_pay', {
              id: 'cuenta-1',
              body: {
                method_id: 'efectivo',
                mode: 'total',
                amount: '270.00',
                expected_balance: '270.00',
                authorization: { username: 'supervisor', password: 'secreto' },
              },
            })
            .catch(() => undefined)
        }
      >
        Registrar
      </button>
      <button onClick={() => void mutation.retry()}>Reintentar</button>
      <p>{mutation.uncertain ? 'Resultado incierto' : 'Sin pendientes'}</p>
      <pre data-testid="pendiente">{entry ? JSON.stringify(entry.request) : ''}</pre>
    </section>
  );
}

function Application({ visible }: { visible: boolean }) {
  return <PendingOperationsProvider>{visible && <PaymentPage />}</PendingOperationsProvider>;
}

describe('almacén global de operaciones inciertas (S6, M8)', () => {
  it('conserva identificador y cuerpo al cambiar de pantalla y reintenta con el mismo UUID', async () => {
    vi.mocked(api).mockRejectedValueOnce(new APIError('CONNECTION_ERROR', 'Sin respuesta'));
    vi.mocked(api).mockResolvedValueOnce({ payment: { id: 'pago' } });
    const view = render(<Application visible />);
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'Registrar' }));
    });
    expect(screen.getByText('Resultado incierto')).toBeTruthy();
    const first = vi.mocked(api).mock.calls[0];
    const firstBody = first?.[1]?.body as Record<string, unknown>;
    expect(firstBody.operation_id).toMatch(/^[0-9a-f-]{36}$/);

    // La página se desmonta (otra pantalla) y vuelve a montarse con el pendiente intacto.
    view.rerender(<Application visible={false} />);
    expect(screen.queryByText('Resultado incierto')).toBeNull();
    view.rerender(<Application visible />);
    expect(screen.getByText('Resultado incierto')).toBeTruthy();
    const stored = JSON.parse(screen.getByTestId('pendiente').textContent ?? '{}') as {
      operation: string;
      id: string;
      body: Record<string, unknown> & { authorization: Record<string, unknown> };
    };
    expect(stored.operation).toBe('payments_pay');
    expect(stored.id).toBe('cuenta-1');
    expect(stored.body.operation_id).toBe(firstBody.operation_id);
    expect(stored.body.amount).toBe('270.00');
    expect(stored.body.authorization).toEqual({ username: 'supervisor' });

    // Mientras exista el pendiente no se acepta un cobro nuevo sobre la misma cuenta.
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'Registrar' }));
    });
    expect(vi.mocked(api)).toHaveBeenCalledTimes(1);

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'Reintentar' }));
    });
    expect(vi.mocked(api)).toHaveBeenCalledTimes(2);
    const retry = vi.mocked(api).mock.calls[1];
    const retryBody = retry?.[1]?.body as Record<string, unknown>;
    expect(retry?.[0]).toBe('payments_pay');
    expect(retry?.[1]?.id).toBe('cuenta-1');
    expect(retryBody.operation_id).toBe(firstBody.operation_id);
    expect(retryBody.amount).toBe('270.00');
    expect(retryBody.expected_balance).toBe('270.00');
    expect(retryBody.authorization).toEqual({ username: 'supervisor' });
    expect(screen.getByText('Sin pendientes')).toBeTruthy();
    expect(screen.getByTestId('pendiente').textContent).toBe('');
  });

  it('libera el identificador ante un rechazo definitivo del servidor', async () => {
    vi.mocked(api).mockRejectedValueOnce(new APIError('BALANCE_CHANGED', 'El saldo cambió', 409));
    vi.mocked(api).mockResolvedValueOnce({});
    render(<Application visible />);
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'Registrar' }));
    });
    expect(screen.getByText('Sin pendientes')).toBeTruthy();
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'Registrar' }));
    });
    const calls = vi.mocked(api).mock.calls;
    const firstID = (calls[0]?.[1]?.body as Record<string, unknown>).operation_id;
    const secondID = (calls[1]?.[1]?.body as Record<string, unknown>).operation_id;
    expect(secondID).not.toBe(firstID);
  });

  it('nunca guarda la contraseña de autorización en la copia para reintentos', () => {
    const original = {
      operation: 'payments_refund' as const,
      id: 'cuenta-1',
      body: {
        operation_id: 'op',
        authorization: { username: 'supervisor', password: 'secreto' },
      },
    };
    const stored = storableRequest(original);
    expect(JSON.stringify(stored)).not.toContain('secreto');
    expect(stored.body.operation_id).toBe('op');
    expect(original.body.authorization.password).toBe('secreto');
  });

  it('exige el proveedor de aplicación para operaciones con clave', () => {
    const silence = vi.spyOn(console, 'error').mockImplementation(() => undefined);
    expect(() => render(<PaymentPage />)).toThrow('PendingOperationsProvider');
    silence.mockRestore();
  });
});
