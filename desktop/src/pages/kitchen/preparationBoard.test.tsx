// @vitest-environment jsdom

import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { Board, Release, Task } from '../../services/kitchen';
import {
  kitchenCoverageCapability,
  rememberServerCapabilities,
} from '../../services/serverCapabilities';
import { PreparationBoard } from './PreparationBoard';
import { releaseCase, serverDayAndTime } from './PendingKitchen';

// Etapa 9F, fase C (§5.4): modo cocina de Preparación con Go simulado por el puente: colores por tiempo
// con texto, sin «Actualizar» («Reintentar» solo sin conexión), aviso sin estaciones en común en lugar
// de una pantalla vacía, botones grandes, la parte de cada estación a la vista, «Mesa 2» sin repetir y
// aviso sonoro solo al llegar una comanda nueva.

const native = vi.hoisted(() => ({ invoke: vi.fn() }));
vi.mock('@tauri-apps/api/core', () => ({ invoke: native.invoke, isTauri: () => true }));
const sound = vi.hoisted(() => ({
  play: vi.fn(async () => true),
  unlock: vi.fn(),
  blocked: vi.fn(),
}));
vi.mock('../../services/kitchenSound', () => ({
  playArrivalChime: sound.play,
  unlockKitchenSound: sound.unlock,
  notifySoundBlocked: sound.blocked,
}));

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
    expect(screen.queryByText('No hay pedidos con este filtro.')).toBeNull();
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
    expect(sound.blocked).not.toHaveBeenCalled();
  });

  it('con el aviso activado, el primer toque o tecla prepara el sonido y, si no puede sonar, lo avisa (revisión 9F, parte 3, B2)', async () => {
    board.sound_alert = true;
    renderBoard();
    await cards();
    // Las tarjetas a la vista no garantizan que React ya corrió el efecto que escucha el primer toque: con la Mac
    // cargada (la regresión de la K1), el toque llegaba antes y no se contaba. Se espera a que lo escuche.
    await waitFor(() => {
      fireEvent.pointerDown(document.body);
      expect(sound.unlock).toHaveBeenCalled();
    });
    sound.unlock.mockClear();
    fireEvent.pointerDown(document.body);
    fireEvent.keyDown(document.body, { key: 'a' });
    expect(sound.unlock).toHaveBeenCalledTimes(2);
    // El equipo todavía no permite reproducir: se pide tocar la pantalla en lugar de callar el fallo.
    sound.play.mockResolvedValueOnce(false);
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
    await waitFor(() => expect(sound.blocked).toHaveBeenCalledTimes(1));
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
    // Sin el aviso, los toques no preparan el sonido.
    fireEvent.pointerDown(document.body);
    expect(sound.unlock).not.toHaveBeenCalled();
  });
});

// Etapa K1.1 (§3, K11-D8 y K11-D9): «Por enviar a cocina» se explica solo. Cada pedido dice qué pasa con él en
// lenguaje sencillo, cada botón lleva su texto corto, «Cambiar la hora» solo en los programados y un pedido detenido
// por la versión anterior ya no muestra los renglones «Sin ruta de preparación».
describe('Por enviar a cocina', () => {
  function release(overrides: Partial<Release>): Release {
    return {
      id: 'r-base',
      account_id: 'a1',
      order_id: 'o1',
      folio: 20,
      account_name: 'Para llevar · Ana',
      state: 'pending',
      scheduled_at: null,
      due_at: null,
      automatic: false,
      lead_minutes: 0,
      warnings: [],
      revision: 1,
      created_at: '2026-11-13T17:00:00Z',
      ...overrides,
    };
  }

  beforeEach(() => {
    rememberServerCapabilities([kitchenCoverageCapability]);
    board = {
      ...baseBoard([]),
      releases: [
        release({
          id: 'r-auto',
          folio: 21,
          state: 'scheduled',
          scheduled_at: '2026-11-13T20:00:00Z',
          due_at: '2026-11-13T19:45:00Z',
          automatic: true,
          lead_minutes: 15,
        }),
        release({
          id: 'r-manual',
          folio: 22,
          state: 'scheduled',
          scheduled_at: '2026-11-13T21:00:00Z',
          due_at: '2026-11-13T21:00:00Z',
        }),
        release({ id: 'r-pending', folio: 23 }),
        release({
          id: 'r-blocked',
          folio: 24,
          state: 'blocked',
          warnings: ['Sin ruta de preparación: Coca Cola 600ml'],
        }),
      ],
    };
  });

  afterEach(() => {
    rememberServerCapabilities([]);
  });

  it('cada caso tiene su título y su explicación, con la hora del restaurante', () => {
    const timezone = 'America/Monterrey';
    expect(serverDayAndTime('2026-11-13T19:45:00Z', timezone)).toBe('el 13/11/26 a la 1:45 p.m.');
    expect(serverDayAndTime('2026-11-13T21:00:00Z', timezone)).toBe('el 13/11/26 a las 3:00 p.m.');
    const [automatic, manual, waiting, blocked] = board.releases;
    expect(releaseCase(automatic!, timezone)).toEqual({
      title: 'Programado: se envía solo a su hora',
      detail: 'Se envía a cocina solo el 13/11/26 a la 1:45 p.m.',
    });
    expect(releaseCase(manual!, timezone)).toEqual({
      title: 'Programado: espera a que lo envíes',
      detail: 'Programado para el 13/11/26 a las 3:00 p.m. Espera a que lo envíes a cocina.',
    });
    expect(releaseCase(waiting!, timezone).title).toBe('Espera a que lo envíes a cocina');
    expect(releaseCase(blocked!, timezone).title).toBe('La cocina no recibió este pedido');
  });

  it('el panel se llama «Por enviar a cocina» y explica cada botón', async () => {
    renderBoard();
    expect(await screen.findByText('Por enviar a cocina')).toBeTruthy();
    expect(screen.getByText('4 pedidos · 0 avisos')).toBeTruthy();
    const automatic = screen.getByRole('article', { name: 'Pedido #21' });
    expect(automatic.textContent).toContain('Programado: se envía solo a su hora');
    // Antes de su hora el envío se adelanta; sin el permiso de liberar anticipadamente el botón no se puede usar.
    expect(automatic.textContent).toContain('La cocina lo recibe al momento, antes de su hora.');
    expect(within(automatic).getByRole('button', { name: 'Cambiar la hora' })).toBeTruthy();
    expect(automatic.textContent).toContain(
      'Cambia cuándo llega a cocina. No cambia el pedido ni su precio.',
    );
    const waiting = screen.getByRole('article', { name: 'Pedido #23' });
    expect(waiting.textContent).toContain('Espera a que lo envíes a cocina');
    expect(waiting.textContent).toContain('La cocina lo recibe al momento.');
    expect(within(waiting).queryByRole('button', { name: 'Cambiar la hora' })).toBeNull();
    const blocked = screen.getByRole('article', { name: 'Pedido #24' });
    expect(blocked.textContent).toContain('La cocina no recibió este pedido');
    expect(blocked.textContent).toContain('Se envió con la versión anterior');
    expect(blocked.textContent).not.toContain('Sin ruta de preparación');
    expect(within(blocked).getByRole('button', { name: 'Enviar a cocina ahora' })).toBeTruthy();
    for (const old of [
      'Bloqueado por configuración',
      'Liberar a preparación',
      'Ajustar programación',
      'Pendientes de liberación',
    ]) {
      expect(document.body.textContent).not.toContain(old);
    }
  });

  it('sin el permiso de liberar anticipadamente, un programado no se envía antes de su hora', async () => {
    render(
      <PreparationBoard
        can={(key) => key !== 'kitchen.release_early'}
        execute={vi.fn() as never}
        busy={false}
      />,
    );
    const automatic = await screen.findByRole('article', { name: 'Pedido #21' });
    const send = within(automatic).getByRole('button', { name: 'Enviar a cocina ahora' });
    expect((send as HTMLButtonElement).disabled).toBe(true);
    expect(automatic.textContent).toContain(
      'Para enviarlo antes de su hora hace falta el permiso de enviar a cocina antes de tiempo.',
    );
    const waiting = screen.getByRole('article', { name: 'Pedido #23' });
    expect(
      (within(waiting).getByRole('button', { name: 'Enviar a cocina ahora' }) as HTMLButtonElement)
        .disabled,
    ).toBe(false);
  });

  it('«Cambiar la hora» abre la programación del pedido', async () => {
    renderBoard();
    const manual = await screen.findByRole('article', { name: 'Pedido #22' });
    fireEvent.click(within(manual).getByRole('button', { name: 'Cambiar la hora' }));
    expect(await screen.findByText('Programación de preparación · #22')).toBeTruthy();
  });
});
