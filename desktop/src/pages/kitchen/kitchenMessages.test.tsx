// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { PendingOperationsProvider } from '../../hooks/pendingOperations';
import { KitchenPage } from './KitchenPage';

// Etapa 7.1, fase A: las escrituras de cocina confirman su resultado con avisos. Un rechazo de Go
// es un error persistente; un resultado incierto queda en el panel de recuperación sin aviso.

const native = vi.hoisted(() => ({ invoke: vi.fn() }));
vi.mock('@tauri-apps/api/core', () => ({ invoke: native.invoke, isTauri: () => true }));

type Reply = { status: number; body: unknown } | string;
let releaseReplies: Reply[];

const board = {
  tasks: [],
  releases: [
    {
      id: '00000000-0000-0000-0000-0000000000r1',
      account_id: 'a1',
      order_id: 'o1',
      folio: 12,
      account_name: 'Mesa 4',
      state: 'pending',
      scheduled_at: null,
      due_at: null,
      automatic: false,
      lead_minutes: 0,
      warnings: [],
      revision: 3,
      created_at: '2026-11-09T18:00:00Z',
    },
  ],
  pending_changes: [],
  stations: [],
  server_time: '2026-11-09T18:00:00Z',
  timezone: 'America/Monterrey',
  // Etapa 9F: intersección de estaciones y aviso sonoro de la terminal.
  station_scope: {
    common: true,
    all: true,
    terminal_all: true,
    terminal_stations: [],
    user_all: true,
    user_all_by_permission: true,
    user_stations: [],
  },
  sound_alert: false,
};

beforeEach(() => {
  releaseReplies = [];
  native.invoke.mockImplementation(async (command: string, args: { operation: string }) => {
    if (command !== 'api_request') {
      return null;
    }
    if (args.operation === 'kitchen_release') {
      const reply = releaseReplies.shift();
      if (reply === undefined) {
        throw new Error('Sin respuesta preparada');
      }
      if (typeof reply === 'string') {
        throw reply;
      }
      return reply;
    }
    if (args.operation === 'kitchen_board') {
      return { status: 200, body: board };
    }
    return { status: 200, body: [] };
  });
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

function renderKitchen() {
  render(
    <PendingOperationsProvider>
      <KitchenPage can={() => true} timezone="America/Monterrey" />
    </PendingOperationsProvider>,
  );
}

describe('avisos de cocina', () => {
  it('confirma la liberación con un aviso de éxito', async () => {
    releaseReplies = [{ status: 200, body: {} }];
    renderKitchen();
    fireEvent.click(await screen.findByRole('button', { name: 'Liberar a preparación' }));
    const notice = await screen.findByRole('status', { name: 'Operación completada' });
    expect(notice.textContent).toContain('Pedido #12 liberado a preparación');
  });

  it('un rechazo de Go es un error persistente sin el código interno', async () => {
    releaseReplies = [
      {
        status: 409,
        body: { error: { code: 'RELEASE_CHANGED', message: 'El pedido cambió en otra terminal.' } },
      },
    ];
    renderKitchen();
    fireEvent.click(await screen.findByRole('button', { name: 'Liberar a preparación' }));
    const alert = await screen.findByRole('alert', { name: 'Error' });
    expect(alert.textContent).toContain('El pedido cambió en otra terminal.');
    expect(alert.textContent).not.toContain('RELEASE_CHANGED');
  });

  it('un resultado incierto queda en el panel de recuperación y se confirma al recuperarlo', async () => {
    releaseReplies = ['No se pudo comunicar con el servidor.', { status: 200, body: {} }];
    renderKitchen();
    fireEvent.click(await screen.findByRole('button', { name: 'Liberar a preparación' }));
    const recover = await screen.findByRole('button', { name: 'Recuperar operación de cocina' });
    expect(screen.getByText(/El resultado no pudo confirmarse/)).toBeTruthy();
    expect(screen.queryByRole('alert', { name: 'Error' })).toBeNull();
    fireEvent.click(recover);
    expect(await screen.findByText('Operación de cocina recuperada.')).toBeTruthy();
    await waitFor(() =>
      expect(screen.queryByRole('button', { name: 'Recuperar operación de cocina' })).toBeNull(),
    );
  });
});
