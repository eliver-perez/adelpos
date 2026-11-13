// @vitest-environment jsdom

import { act, cleanup, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { Board, Task } from '../../services/kitchen';
import { PreparationBoard } from './PreparationBoard';

// Etapa 9F, fase C (§5.4): modo cocina de Preparación con Go simulado por el puente: colores por tiempo
// con texto, sin «Actualizar» («Reintentar» solo sin conexión), aviso sin estaciones en común en lugar
// de una pantalla vacía, botones grandes, la parte de cada estación a la vista, «Mesa 2» sin repetir y
// aviso sonoro solo al llegar una comanda nueva.

const native = vi.hoisted(() => ({ invoke: vi.fn() }));
vi.mock('@tauri-apps/api/core', () => ({ invoke: native.invoke, isTauri: () => true }));
const sound = vi.hoisted(() => ({ play: vi.fn() }));
vi.mock('../../services/kitchenSound', () => ({ playArrivalChime: sound.play }));

let board: Board;

function task(overrides: Partial<Task>): Task {
  return {
    id: 't1',
    unit_id: 'u1',
    release_id: 'r1',
    station_id: 's-plancha',
    state: 'released',
    instructions: {
      item_id: 'i1',
      version_id: 'v1',
      name: 'Hamburguesa y papas',
      quantity: 1,
      parts: [{ path: 'root', name: 'Hamburguesa y papas', variant: '', quantity: 1, notes: '' }],
    },
    revision: 1,
    released_at: '2026-11-13T18:00:00Z',
    folio: 7,
    account_name: 'Mesa 2',
    table_name: 'Mesa 2',
    station_name: 'Plancha',
    warning_minutes: 10,
    overdue_minutes: 25,
    target_minutes: 15,
    elapsed_seconds: 60,
    release_revision: 2,
    tracking: 'item',
    ...overrides,
  };
}

function baseBoard(tasks: Task[]): Board {
  return {
    tasks,
    releases: [],
    pending_changes: [],
    stations: [],
    server_time: '2026-11-13T18:30:00Z',
    timezone: 'America/Monterrey',
    page: 1,
    station_scope: {
      common: true,
      all: false,
      terminal_all: false,
      terminal_stations: ['Freidoras', 'Plancha'],
      user_all: true,
      user_all_by_permission: false,
      user_stations: [],
    },
    sound_alert: false,
  };
}

beforeEach(() => {
  board = baseBoard([
    task({ id: 'normal', release_id: 'r1', elapsed_seconds: 60 }),
    task({
      id: 'warning',
      release_id: 'r2',
      unit_id: 'u2',
      elapsed_seconds: 11 * 60,
      account_name: 'Cumpleaños de Ana',
      table_name: 'Mesa 5',
      instructions: {
        item_id: 'i2',
        version_id: 'v2',
        name: 'Hamburguesa y papas',
        quantity: 1,
        parts: [{ path: 'root/0', name: 'Papas', variant: '', quantity: 1, notes: 'Bien doradas' }],
      },
      station_id: 's-freidoras',
      station_name: 'Freidoras',
    }),
    task({ id: 'late', release_id: 'r3', unit_id: 'u3', elapsed_seconds: 16 * 60 }),
  ]);
  native.invoke.mockImplementation(async (command: string, args: { operation: string }) => {
    if (command !== 'api_request') {
      return null;
    }
    if (args.operation === 'kitchen_board') {
      return { status: 200, body: board };
    }
    return { status: 200, body: {} };
  });
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  vi.useRealTimers();
});

function renderBoard(connectionLost = false, onRetry = vi.fn()) {
  const execute = vi.fn(async () => ({}) as never);
  render(
    <PreparationBoard
      can={() => true}
      execute={execute}
      busy={false}
      connectionLost={connectionLost}
      onRetry={onRetry}
    />,
  );
  return { execute, onRetry };
}

async function cards() {
  await screen.findAllByRole('heading', { name: /Hamburguesa y papas/ });
  return Array.from(document.querySelectorAll<HTMLElement>('.kitchen-task-card'));
}

describe('Preparación en modo cocina', () => {
  it('pinta cada tarjeta según el tiempo contra el objetivo, con texto además del color', async () => {
    renderBoard();
    const [normal, warning, late] = await cards();
    expect(normal!.dataset.urgency).toBe('normal');
    expect(normal!.className).toContain('kitchen-card-normal');
    expect(within(normal!).queryByText('Atrasada')).toBeNull();
    expect(warning!.dataset.urgency).toBe('warning');
    expect(warning!.className).toContain('kitchen-card-warning');
    expect(within(warning!).getByText('Por vencer')).toBeTruthy();
    expect(late!.dataset.urgency).toBe('late');
    expect(late!.className).toContain('kitchen-card-late');
    expect(within(late!).getByText('Atrasada')).toBeTruthy();
    // Más antiguas primero: el orden es el de Go (released_at), sin reordenar en la pantalla.
    expect(document.body.textContent).toContain('Más antiguos primero');
  });

  it('no tiene «Actualizar»; «Reintentar» solo aparece sin conexión', async () => {
    renderBoard();
    await cards();
    expect(screen.queryByRole('button', { name: /Actualizar/ })).toBeNull();
    expect(screen.queryByRole('button', { name: 'Reintentar' })).toBeNull();
    cleanup();
    const { onRetry } = renderBoard(true);
    await cards();
    const retry = screen.getByRole('button', { name: 'Reintentar' });
    act(() => retry.click());
    expect(onRetry).toHaveBeenCalledTimes(1);
  });

  it('usa botones grandes y muestra a la vista la parte de cada estación', async () => {
    renderBoard();
    const [normal, warning] = await cards();
    const ready = within(normal!).getByRole('button', { name: 'Marcar producto listo' });
    expect(ready.className).toContain('kitchen-action-button');
    expect(within(normal!).getByRole('button', { name: 'En preparación' }).className).toContain(
      'kitchen-action-button',
    );
    // La parte de las freidoras se ve sin abrir nada: el componente del combo y su indicación.
    const parts = within(warning!).getByRole('list', { name: 'Parte de Freidoras' });
    expect(parts.textContent).toContain('1 × Papas');
    expect(parts.textContent).toContain('Bien doradas');
    expect(within(normal!).queryByRole('list')).toBeNull();
  });

  it('no repite el nombre de la mesa cuando la cuenta se llama igual', async () => {
    renderBoard();
    const [normal, warning] = await cards();
    expect(normal!.textContent).toContain('#7 · Mesa 2');
    expect(normal!.textContent).not.toContain('Mesa 2 · Mesa 2');
    expect(warning!.textContent).toContain('Cumpleaños de Ana · Mesa 5');
  });

  it('sin estaciones en común muestra un aviso claro en lugar de una pantalla vacía', async () => {
    board = {
      ...baseBoard([]),
      station_scope: {
        common: false,
        all: false,
        terminal_all: false,
        terminal_stations: ['Barra', 'Postres'],
        user_all: false,
        user_all_by_permission: false,
        user_stations: ['Cocina caliente'],
      },
    };
    renderBoard();
    const notice = await screen.findByRole('status', { name: 'Sin estaciones en común' });
    expect(notice.textContent).toContain(
      'Esta terminal muestra Barra y Postres, y tu usuario atiende Cocina caliente: no hay ' +
        'estaciones en común. Pide que ajusten la terminal o tu usuario.',
    );
    expect(screen.queryByText('No hay unidades de preparación en este filtro.')).toBeNull();
  });

  it('el aviso sonoro suena solo al llegar una comanda nueva y si la terminal lo tiene activado', async () => {
    board.sound_alert = true;
    renderBoard();
    await cards();
    // Al abrir la pantalla no suena.
    expect(sound.play).not.toHaveBeenCalled();
    // Un cambio que no es una comanda nueva (otra tarea pasa a preparación) tampoco.
    board = { ...board, tasks: board.tasks.map((entry) => ({ ...entry, revision: 2 })) };
    act(() => {
      window.dispatchEvent(new CustomEvent('adel-server-change'));
    });
    await waitFor(() => expect(native.invoke.mock.calls.length).toBeGreaterThan(1));
    await new Promise((resolve) => setTimeout(resolve, 150));
    expect(sound.play).not.toHaveBeenCalled();
    // Llega una comanda nueva de una estación visible: suena una vez.
    board = {
      ...board,
      tasks: [
        ...board.tasks,
        task({ id: 'nueva', release_id: 'r9', unit_id: 'u9', elapsed_seconds: 3 }),
      ],
    };
    act(() => {
      window.dispatchEvent(new CustomEvent('adel-server-change'));
    });
    await waitFor(() => expect(sound.play).toHaveBeenCalledTimes(1));
  });

  it('sin el aviso activado en la terminal no suena', async () => {
    renderBoard();
    await cards();
    board = {
      ...board,
      tasks: [
        ...board.tasks,
        task({ id: 'nueva', release_id: 'r9', unit_id: 'u9', elapsed_seconds: 3 }),
      ],
    };
    act(() => {
      window.dispatchEvent(new CustomEvent('adel-server-change'));
    });
    await waitFor(() => expect(document.querySelectorAll('.kitchen-task-card')).toHaveLength(4));
    expect(sound.play).not.toHaveBeenCalled();
  });
});
