// @vitest-environment jsdom

import { act, cleanup, renderHook, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { api, APIError } from '../services/api';
import { useLiveResource } from './useLiveResource';
import { useRestaurantMutation } from './useRestaurantMutation';

vi.mock('../services/api', async (original) => ({
  ...(await original<typeof import('../services/api')>()),
  api: vi.fn(),
}));

beforeEach(() => vi.mocked(api).mockReset());
afterEach(cleanup);

describe('operación entre terminales', () => {
  it('descarta respuestas tardías de otra cuenta sin mostrar sus datos', async () => {
    let finishFirst!: (value: unknown) => void;
    vi.mocked(api).mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          finishFirst = resolve;
        }),
    );
    vi.mocked(api).mockResolvedValueOnce({ account: 'segunda' });
    const { result, rerender } = renderHook(
      ({ id }) => useLiveResource('restaurant_account', { id }),
      { initialProps: { id: 'primera' } },
    );
    rerender({ id: 'segunda' });
    expect(result.current.data).toBeNull();
    await waitFor(() => expect(result.current.data).toEqual({ account: 'segunda' }));
    await act(async () => finishFirst({ account: 'primera' }));
    expect(result.current.data).toEqual({ account: 'segunda' });
  });

  it('recupera el resultado incierto con el mismo UUID y cuerpo', async () => {
    const resolved = vi.fn();
    vi.mocked(api).mockRejectedValueOnce(new APIError('CONNECTION_ERROR', 'Interrumpida'));
    vi.mocked(api).mockResolvedValueOnce({ confirmed: true });
    const { result } = renderHook(() => useRestaurantMutation(resolved));
    await act(async () => {
      await result.current
        .execute('restaurant_confirm', { id: 'cuenta', body: { revision: 4 } })
        .catch(() => undefined);
    });
    expect(result.current.uncertain).toBe(true);
    const original = vi.mocked(api).mock.calls[0];
    await act(async () => {
      await expect(result.current.execute('restaurant_confirm', { id: 'otra' })).rejects.toThrow(
        'pendiente',
      );
    });
    await act(async () => result.current.retry());
    expect(vi.mocked(api).mock.calls[1]).toEqual(original);
    expect(result.current.uncertain).toBe(false);
    expect(resolved).toHaveBeenCalledOnce();
  });

  it('libera una revisión rechazada y permite enviar una nueva operación', async () => {
    vi.mocked(api).mockRejectedValueOnce(new APIError('REVISION_CONFLICT', 'Actualiza', 409));
    vi.mocked(api).mockResolvedValueOnce({});
    const { result } = renderHook(() => useRestaurantMutation(vi.fn()));
    await act(async () => {
      await result.current
        .execute('restaurant_save_draft', { id: 'cuenta' })
        .catch(() => undefined);
    });
    expect(result.current.uncertain).toBe(false);
    await act(async () => {
      await result.current.execute('restaurant_save_draft', { id: 'cuenta' });
    });
    const requests = vi.mocked(api).mock.calls;
    expect(requests[0]?.[1]?.body).not.toEqual(requests[1]?.[1]?.body);
  });
});
