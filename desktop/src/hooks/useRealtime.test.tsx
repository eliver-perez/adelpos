// @vitest-environment jsdom

import { act, cleanup, renderHook, waitFor } from '@testing-library/react';
import { afterEach, expect, it, vi } from 'vitest';
import { invoke } from '@tauri-apps/api/core';
import { useRealtime } from './useRealtime';

vi.mock('@tauri-apps/api/core', () => ({
  invoke: vi.fn().mockResolvedValue(7),
  Channel: class {
    onmessage: (message: unknown) => void = () => undefined;
  },
}));

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

it('deduplica avisos, recupera estado completo y descarta mensajes de una identidad anterior', async () => {
  const changed = vi.fn();
  window.addEventListener('adel-server-change', changed);
  const { result, rerender, unmount } = renderHook(({ identity }) => useRealtime(identity, true), {
    initialProps: { identity: 'usuario-uno' },
  });
  await waitFor(() => expect(invoke).toHaveBeenCalledWith('events_start', expect.any(Object)));
  const options = vi.mocked(invoke).mock.calls[0]?.[1] as {
    channel: { onmessage: (message: unknown) => void };
  };
  const event = {
    id: 'evento-1',
    sequence: 1,
    kind: 'kitchen.changed',
    scope: 'kitchen',
    version: 1,
  };
  act(() => options.channel.onmessage({ connection: 'connected' }));
  expect(result.current).toBe('En tiempo real');
  act(() => options.channel.onmessage({ events: [event], cursor: 1 }));
  act(() => options.channel.onmessage({ events: [event], cursor: 1 }));
  expect(changed).toHaveBeenCalledTimes(2);
  act(() => options.channel.onmessage({ reset: true, cursor: 5001 }));
  expect(changed).toHaveBeenCalledTimes(3);
  act(() => options.channel.onmessage({ connection: 'reconnecting' }));
  expect(result.current).toContain('sondeo activo');
  rerender({ identity: 'usuario-dos' });
  await waitFor(() => expect(invoke).toHaveBeenCalledWith('events_stop', { subscriptionId: 7 }));
  expect(invoke).toHaveBeenLastCalledWith('events_start', expect.objectContaining({ after: 0 }));
  act(() => options.channel.onmessage({ reset: true }));
  expect(changed).toHaveBeenCalledTimes(3);
  unmount();
  window.removeEventListener('adel-server-change', changed);
});

it('publica los eventos de caja con su tipo y turno para refrescar Caja y el cobro', async () => {
  const cash = vi.fn();
  const listener = (event: Event) => cash((event as CustomEvent).detail);
  window.addEventListener('adel-cash-change', listener);
  const { unmount } = renderHook(() => useRealtime('usuario-caja', true));
  await waitFor(() => expect(invoke).toHaveBeenCalledWith('events_start', expect.any(Object)));
  const options = vi.mocked(invoke).mock.calls[0]?.[1] as {
    channel: { onmessage: (message: unknown) => void };
  };
  act(() =>
    options.channel.onmessage({
      events: [
        { id: 'cocina-1', sequence: 1, kind: 'kitchen.changed', scope: 'kitchen', version: 1 },
      ],
      cursor: 1,
    }),
  );
  expect(cash).not.toHaveBeenCalled();
  act(() =>
    options.channel.onmessage({
      events: [
        {
          id: 'caja-1',
          sequence: 2,
          kind: 'cash.shift.closing_started',
          scope: 'cash',
          version: 1,
          aggregate_id: 'turno-1',
        },
      ],
      cursor: 2,
    }),
  );
  expect(cash).toHaveBeenCalledWith({
    kinds: ['cash.shift.closing_started'],
    aggregates: ['turno-1'],
  });
  // Un evento repetido no vuelve a publicarse.
  act(() =>
    options.channel.onmessage({
      events: [
        {
          id: 'caja-1',
          sequence: 2,
          kind: 'cash.shift.closing_started',
          scope: 'cash',
          version: 1,
          aggregate_id: 'turno-1',
        },
      ],
      cursor: 2,
    }),
  );
  expect(cash).toHaveBeenCalledTimes(1);
  unmount();
  window.removeEventListener('adel-cash-change', listener);
});

it('publica los eventos de inventario solo con su tipo para recargar la pestaña visible', async () => {
  const inventory = vi.fn();
  const listener = (event: Event) => inventory((event as CustomEvent).detail);
  window.addEventListener('adel-inventory-change', listener);
  const { unmount } = renderHook(() => useRealtime('usuario-inventario', true));
  await waitFor(() => expect(invoke).toHaveBeenCalledWith('events_start', expect.any(Object)));
  const options = vi.mocked(invoke).mock.calls[0]?.[1] as {
    channel: { onmessage: (message: unknown) => void };
  };
  act(() =>
    options.channel.onmessage({
      events: [
        { id: 'cocina-2', sequence: 1, kind: 'kitchen.changed', scope: 'kitchen', version: 1 },
      ],
      cursor: 1,
    }),
  );
  // Los cambios de otras áreas no recargan Inventarios.
  expect(inventory).not.toHaveBeenCalled();
  act(() =>
    options.channel.onmessage({
      events: [
        {
          id: 'inventario-1',
          sequence: 2,
          kind: 'inventory.stock.changed',
          scope: 'inventory',
          version: 1,
          aggregate_id: 'almacen-1',
        },
      ],
      cursor: 2,
    }),
  );
  expect(inventory).toHaveBeenCalledWith({ kinds: ['inventory.stock.changed'] });
  // Un reinicio del cursor también obliga a consultar de nuevo.
  act(() => options.channel.onmessage({ reset: true, cursor: 9000 }));
  expect(inventory).toHaveBeenLastCalledWith({ kinds: [] });
  unmount();
  window.removeEventListener('adel-inventory-change', listener);
});

it('publica el cambio de la identidad del establecimiento solo con su propio evento', async () => {
  const identity = vi.fn();
  window.addEventListener('adel-documents-identity-change', identity);
  const { unmount } = renderHook(() => useRealtime('usuario-identidad', true));
  await waitFor(() => expect(invoke).toHaveBeenCalledWith('events_start', expect.any(Object)));
  const options = vi.mocked(invoke).mock.calls[0]?.[1] as {
    channel: { onmessage: (message: unknown) => void };
  };
  // Un cambio de plantillas en el mismo ámbito no vuelve a consultar la identidad (lleva imágenes).
  act(() =>
    options.channel.onmessage({
      events: [
        {
          id: 'plantilla-1',
          sequence: 1,
          kind: 'documents.templates.changed',
          scope: 'documents',
          version: 1,
        },
      ],
      cursor: 1,
    }),
  );
  expect(identity).not.toHaveBeenCalled();
  act(() =>
    options.channel.onmessage({
      events: [
        {
          id: 'identidad-1',
          sequence: 2,
          kind: 'documents.identity.changed',
          scope: 'documents',
          version: 1,
        },
      ],
      cursor: 2,
    }),
  );
  expect(identity).toHaveBeenCalledTimes(1);
  // Un reinicio del cursor también obliga a consultar de nuevo.
  act(() => options.channel.onmessage({ reset: true, cursor: 9000 }));
  expect(identity).toHaveBeenCalledTimes(2);
  unmount();
  window.removeEventListener('adel-documents-identity-change', identity);
});
