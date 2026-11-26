// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { PendingOperationsProvider } from '../../hooks/pendingOperations';
import type { Board, Release } from '../../services/kitchen';
import {
  kitchenCoverageCapability,
  rememberServerCapabilities,
} from '../../services/serverCapabilities';
import { KitchenPage } from './KitchenPage';

// Etapa 7.1, fase A: las escrituras de cocina confirman su resultado con avisos. Un rechazo de Go
// es un error persistente; un resultado incierto queda en el panel de recuperación sin aviso.
// Etapa K1.1 (§3, K11-D8): en «Por enviar a cocina», «Enviar a cocina ahora» solo dice que lo envió si Go responde
// que quedó enviado; los 409 del contrato (ya no está pendiente) se dicen con el mensaje de Go y el panel se vuelve a
// leer. Con un servidor sin kitchen-coverage-v1 el panel es el de siempre.

const native = vi.hoisted(() => ({ invoke: vi.fn() }));
vi.mock('@tauri-apps/api/core', () => ({ invoke: native.invoke, isTauri: () => true }));

type Reply = { status: number; body: unknown } | string;
let releaseReplies: Reply[];
let retryReplies: Reply[];
let boardReads: number;
let board: Board;

const pending: Release = {
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
};

function baseBoard(): Board {
  return {
    tasks: [],
    releases: [pending],
    pending_changes: [],
    stations: [],
    server_time: '2026-11-09T18:00:00Z',
    timezone: 'America/Monterrey',
    page: 1,
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
}

function answer(replies: Reply[]) {
  const reply = replies.shift();
  if (reply === undefined) {
    throw new Error('Sin respuesta preparada');
  }
  if (typeof reply === 'string') {
    throw reply;
  }
  return reply;
}

beforeEach(() => {
  rememberServerCapabilities([kitchenCoverageCapability]);
  releaseReplies = [];
  retryReplies = [];
  boardReads = 0;
  board = baseBoard();
  native.invoke.mockImplementation(async (command: string, args: { operation: string }) => {
    if (command !== 'api_request') {
      return null;
    }
    if (args.operation === 'kitchen_release') {
      return answer(releaseReplies);
    }
    if (args.operation === 'kitchen_retry_change') {
      return answer(retryReplies);
    }
    if (args.operation === 'kitchen_board') {
      boardReads++;
      return { status: 200, body: board };
    }
    return { status: 200, body: [] };
  });
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  rememberServerCapabilities([]);
});

function renderKitchen() {
  render(
    <PendingOperationsProvider>
      <KitchenPage can={() => true} timezone="America/Monterrey" />
    </PendingOperationsProvider>,
  );
}

describe('avisos de cocina', () => {
  it('confirma el envío con un aviso de éxito solo cuando Go dice que quedó enviado', async () => {
    releaseReplies = [{ status: 200, body: { ...pending, state: 'released', revision: 4 } }];
    renderKitchen();
    fireEvent.click(await screen.findByRole('button', { name: 'Enviar a cocina ahora' }));
    const notice = await screen.findByRole('status', { name: 'Operación completada' });
    expect(notice.textContent).toContain('Pedido #12 enviado a cocina.');
  });

  it('si Go responde que sigue pendiente, no dice que lo envió', async () => {
    releaseReplies = [{ status: 200, body: { ...pending, revision: 4 } }];
    renderKitchen();
    fireEvent.click(await screen.findByRole('button', { name: 'Enviar a cocina ahora' }));
    const notice = await screen.findByRole('status', { name: 'Atención' });
    expect(notice.textContent).toContain('La cocina no recibió el pedido #12: sigue pendiente.');
    expect(screen.queryByRole('status', { name: 'Operación completada' })).toBeNull();
  });

  it('«ya no está pendiente» (409) se dice con el mensaje de Go y el panel se vuelve a leer', async () => {
    releaseReplies = [
      {
        status: 409,
        body: {
          error: {
            code: 'KITCHEN_RELEASE_NOT_PENDING',
            message: 'Este pedido ya no está pendiente.',
          },
        },
      },
    ];
    renderKitchen();
    const button = await screen.findByRole('button', { name: 'Enviar a cocina ahora' });
    const before = boardReads;
    board = { ...board, releases: [] };
    fireEvent.click(button);
    const alert = await screen.findByRole('alert', { name: 'Error' });
    expect(alert.textContent).toContain('Este pedido ya no está pendiente.');
    expect(alert.textContent).not.toContain('KITCHEN_RELEASE_NOT_PENDING');
    await waitFor(() => expect(boardReads).toBeGreaterThan(before));
    await waitFor(() =>
      expect(screen.queryByRole('button', { name: 'Enviar a cocina ahora' })).toBeNull(),
    );
    expect(screen.queryByRole('status', { name: 'Operación completada' })).toBeNull();
  });

  it('un rechazo de Go es un error persistente sin el código interno', async () => {
    releaseReplies = [
      {
        status: 409,
        body: { error: { code: 'RELEASE_CHANGED', message: 'El pedido cambió en otra terminal.' } },
      },
    ];
    renderKitchen();
    fireEvent.click(await screen.findByRole('button', { name: 'Enviar a cocina ahora' }));
    const alert = await screen.findByRole('alert', { name: 'Error' });
    expect(alert.textContent).toContain('El pedido cambió en otra terminal.');
    expect(alert.textContent).not.toContain('RELEASE_CHANGED');
  });

  it('un resultado incierto queda en el panel de recuperación y se confirma al recuperarlo', async () => {
    releaseReplies = [
      'No se pudo comunicar con el servidor.',
      { status: 200, body: { ...pending, state: 'released', revision: 4 } },
    ];
    renderKitchen();
    fireEvent.click(await screen.findByRole('button', { name: 'Enviar a cocina ahora' }));
    const recover = await screen.findByRole('button', { name: 'Recuperar operación de cocina' });
    expect(screen.getByText(/El resultado no pudo confirmarse/)).toBeTruthy();
    expect(screen.queryByRole('alert', { name: 'Error' })).toBeNull();
    fireEvent.click(recover);
    expect(await screen.findByText('Operación de cocina recuperada.')).toBeTruthy();
    await waitFor(() =>
      expect(screen.queryByRole('button', { name: 'Recuperar operación de cocina' })).toBeNull(),
    );
  });

  it('«Volver a avisar» dice si el aviso salió o si la estación sigue desactivada', async () => {
    const change = {
      event_id: '00000000-0000-0000-0000-0000000000e1',
      kind: 'item.updated',
      warnings: ['Aviso pendiente para estación inactiva: Barra'],
      revision: 1,
      created_at: '2026-11-09T18:00:00Z',
    };
    board = { ...board, releases: [], pending_changes: [change] };
    retryReplies = [
      { status: 200, body: { reviewed: true } },
      {
        status: 409,
        body: {
          error: {
            code: 'KITCHEN_CHANGE_NOT_PENDING',
            message: 'Este aviso ya no está pendiente.',
          },
        },
      },
    ];
    renderKitchen();
    const card = await screen.findByRole('article', {
      name: 'La cocina no recibió una modificación',
    });
    expect(card.textContent).toContain('No se pudo avisar a Barra: está desactivada.');
    expect(card.textContent).toContain('Actívala en Cocina › Estaciones y vuelve a avisar.');
    expect(card.textContent).toContain('Manda el cambio a la estación si ya está activa.');
    fireEvent.click(within(card).getByRole('button', { name: 'Volver a avisar' }));
    const notice = await screen.findByRole('status', { name: 'Atención' });
    expect(notice.textContent).toContain('La estación sigue desactivada: el aviso no llegó.');
    // Con el aviso ya terminado, Go responde 409 con su mensaje.
    fireEvent.click(within(card).getByRole('button', { name: 'Volver a avisar' }));
    const alert = await screen.findByRole('alert', { name: 'Error' });
    expect(alert.textContent).toContain('Este aviso ya no está pendiente.');
  });

  it('sin kitchen-coverage-v1 el panel es el de siempre', async () => {
    rememberServerCapabilities([]);
    releaseReplies = [{ status: 200, body: {} }];
    renderKitchen();
    fireEvent.click(await screen.findByRole('button', { name: 'Liberar a preparación' }));
    const notice = await screen.findByRole('status', { name: 'Operación completada' });
    expect(notice.textContent).toContain('Pedido #12 liberado a preparación');
    expect(screen.getByText('Pendientes de liberación y avisos')).toBeTruthy();
    expect(screen.queryByText('Por enviar a cocina')).toBeNull();
  });
});
