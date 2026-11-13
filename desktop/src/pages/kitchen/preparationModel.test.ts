import { describe, expect, it } from 'vitest';
import type { StationScope, Task } from '../../services/kitchen';
import {
  accountContext,
  joinNames,
  newArrivals,
  noCommonStationsMessage,
  sameName,
  taskUrgency,
  visibleParts,
} from './preparationModel';

// Etapa 9F, fase C (§5.4): reglas puras del modo cocina de Preparación.

function task(overrides: Partial<Task> = {}): Task {
  return {
    id: 't1',
    unit_id: 'u1',
    release_id: 'r1',
    station_id: 's1',
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
    elapsed_seconds: 0,
    release_revision: 2,
    tracking: 'item',
    ...overrides,
  };
}

describe('color por tiempo contra el objetivo (decisión D7)', () => {
  it('normal antes de la advertencia, ámbar desde la advertencia y rojo al pasar el objetivo', () => {
    expect(taskUrgency(0, 10, 15)).toBe('normal');
    expect(taskUrgency(10 * 60 - 1, 10, 15)).toBe('normal');
    expect(taskUrgency(10 * 60, 10, 15)).toBe('warning');
    expect(taskUrgency(15 * 60, 10, 15)).toBe('warning');
    expect(taskUrgency(15 * 60 + 1, 10, 15)).toBe('late');
    expect(taskUrgency(3 * 3600, 10, 15)).toBe('late');
  });

  it('con la advertencia igual o mayor que el objetivo, ámbar desde el 75 % del objetivo', () => {
    // Valores de una estación nueva: objetivo 15 y advertencia 15 → ámbar desde 11 min 15 s.
    expect(taskUrgency(675 - 1, 15, 15)).toBe('normal');
    expect(taskUrgency(675, 15, 15)).toBe('warning');
    expect(taskUrgency(15 * 60 + 1, 15, 15)).toBe('late');
    expect(taskUrgency(30 * 60, 20, 40)).toBe('warning');
    expect(taskUrgency(29 * 60, 60, 40)).toBe('normal');
    expect(taskUrgency(30 * 60, 60, 40)).toBe('warning');
  });
});

describe('cuenta y mesa sin repetir (§4.6)', () => {
  it('no repite el nombre de la mesa cuando la cuenta se llama igual, sin distinguir mayúsculas ni acentos', () => {
    expect(accountContext('Mesa 2', 'Mesa 2')).toBe('Mesa 2');
    expect(accountContext('MESA 2', 'mesa  2')).toBe('MESA 2');
    expect(accountContext('Terraza', 'terraza')).toBe('Terraza');
    expect(accountContext('Jardín 3', 'Jardin 3')).toBe('Jardín 3');
    expect(accountContext('Cumpleaños de Ana', 'Mesa 2')).toBe('Cumpleaños de Ana · Mesa 2');
    expect(accountContext('Pedido para llevar', null)).toBe('Pedido para llevar · Sin mesa');
    expect(sameName(' Mesa 2 ', 'mesa 2')).toBe(true);
    expect(sameName('Mesa 2', 'Mesa 12')).toBe(false);
  });
});

describe('aviso sin estaciones en común', () => {
  const scope: StationScope = {
    common: false,
    all: false,
    terminal_all: false,
    terminal_stations: ['Barra', 'Postres'],
    user_all: false,
    user_all_by_permission: false,
    user_stations: ['Cocina caliente'],
  };

  it('dice qué muestra la terminal, qué atiende el usuario y qué hacer', () => {
    expect(noCommonStationsMessage(scope)).toBe(
      'Esta terminal muestra Barra y Postres, y tu usuario atiende Cocina caliente: no hay ' +
        'estaciones en común. Pide que ajusten la terminal o tu usuario.',
    );
  });

  it('explica también una terminal o un usuario sin estaciones asignadas', () => {
    expect(noCommonStationsMessage({ ...scope, terminal_stations: [] })).toContain(
      'Esta terminal no tiene estaciones asignadas, y tu usuario atiende Cocina caliente',
    );
    expect(noCommonStationsMessage({ ...scope, terminal_all: true, user_stations: [] })).toContain(
      'Esta terminal muestra todas las estaciones, y tu usuario no tiene estaciones asignadas',
    );
  });

  it('une los nombres como en español', () => {
    expect(joinNames([])).toBe('');
    expect(joinNames(['Barra'])).toBe('Barra');
    expect(joinNames(['Barra', 'Postres', 'Freidoras'])).toBe('Barra, Postres y Freidoras');
  });
});

describe('parte de cada estación a la vista', () => {
  it('muestra el componente que llegó a la estación y no repite el producto del título', () => {
    expect(visibleParts(task())).toEqual([]);
    const fryer = task({
      instructions: {
        ...task().instructions,
        parts: [{ path: 'root/0', name: 'Papas', variant: '', quantity: 2, notes: '' }],
      },
    });
    expect(visibleParts(fryer).map((part) => part.name)).toEqual(['Papas']);
    const variant = task({
      instructions: {
        ...task().instructions,
        parts: [
          { path: 'root', name: 'Hamburguesa y papas', variant: 'Doble', quantity: 1, notes: '' },
        ],
      },
    });
    expect(visibleParts(variant)).toHaveLength(1);
  });
});

describe('aviso sonoro de comanda nueva', () => {
  it('solo cuenta liberaciones nuevas y recientes que no se habían visto', () => {
    const seen = new Set(['r1']);
    const tasks = [
      task(),
      task({ id: 't2', release_id: 'r2', elapsed_seconds: 20 }),
      task({ id: 't3', release_id: 'r3', elapsed_seconds: 20, state: 'preparing' }),
      task({ id: 't4', release_id: 'r4', elapsed_seconds: 600 }),
    ];
    expect([...newArrivals(seen, tasks)]).toEqual(['r2']);
    expect(newArrivals(new Set(['r1', 'r2', 'r3', 'r4']), tasks).size).toBe(0);
  });
});
