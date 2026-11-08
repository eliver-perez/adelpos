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
