// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { invoke } from '@tauri-apps/api/core';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { TableMap, searchEntries } from '../../components/restaurant/TableMap';
import { APIError } from '../../services/api';
import type {
  Account,
  Area,
  Bill,
  DraftItem,
  Overview,
  RestaurantTable,
  TableGroup,
  TableUnion,
} from '../../services/restaurant';
import { accountCustomName, accountPlace, searchTables } from '../../services/restaurant';
import { addLineOperation } from './capture/draftOperations';
import { DraftSyncEngine } from './capture/draftSync';
import type { DraftSyncTransport } from './capture/draftSync';
import { TableGroupsAdmin } from './TableGroupsAdmin';
import { TablePanel } from './TablePanel';
import type { TableJoinActions } from './TablePanel';
import { JoinTablesDialog, unavailableReason } from './TableUnions';

vi.mock('@tauri-apps/api/core', () => ({ invoke: vi.fn(), isTauri: () => true }));

// Etapa M1 (§1 y §3; contrato §10): unir mesas en el escritorio. Go decide todo; aquí se prueba lo que el escritorio
// muestra (contorno, marco, panel de la unión, el diálogo con la vista previa de Go) y la unión diferida del motor del
// borrador (los campos que viajan con el primer producto y los rechazos de Go).

afterEach(cleanup);

const area: Area = {
  id: 'area',
  name: 'Terraza',
  description: '',
  sort_order: 1,
  active: true,
  revision: 1,
  table_count: 5,
};

function table(id: string, name: string, x: number, values: Partial<RestaurantTable> = {}) {
  return {
    id,
    area_id: 'area',
    name,
    shape: 'rectangle',
    x,
    y: 40,
    width: 100,
    height: 80,
    seats: 4,
    active: true,
    out_of_service: false,
    revision: 1,
    account_count: 0,
    state: 'free',
    occupied_since: null,
    occupied_seconds: 0,
    balance: '0.00',
    long_open: false,
    union_id: null,
    group_id: null,
    ...values,
  } as RestaurantTable;
}

const union: TableUnion = {
  id: 'union',
  name: 'Mesa 5 + 6',
  area_id: 'area',
  group_id: null,
  table_ids: ['m5', 'm6'],
  seats: 8,
  state: 'occupied',
  account_count: 1,
  balance: '120.00',
  occupied_since: '2026-11-27T18:00:00Z',
  occupied_seconds: 600,
  long_open: false,
  revision: 2,
};

const group: TableGroup = {
  id: 'grupo',
  area_id: 'area',
  name: 'Mesa 3',
  table_ids: ['3a', '3b'],
  seats: 8,
  revision: 1,
};

const occupied = { state: 'occupied' as const, account_count: 1, union_id: 'union' };
const tables = [
  table('m5', 'Mesa 5', 20, occupied),
  table('m6', 'Mesa 6', 140, occupied),
  table('m7', 'Mesa 7', 260, { state: 'occupied', account_count: 1 }),
  table('m8', 'Mesa 8', 380, { out_of_service: true, state: 'out_of_service' }),
  table('3a', '3A', 500, { group_id: 'grupo' }),
  table('3b', '3B', 620, { group_id: 'grupo' }),
];

const overview: Overview = {
  areas: [area],
  tables,
  settings: { warning_minutes: 15, overdue_minutes: 30, long_open_hours: 12, revision: 1 },
  timezone: 'America/Monterrey',
  unions: [union],
  groups: [group],
};

describe('el mapa con uniones y grupos (contrato §10)', () => {
  it('el contorno de la unión con su nombre y su capacidad; el marco del grupo con su atajo', () => {
    const onGroupShortcut = vi.fn();
    const onSelect = vi.fn();
    const { container } = render(
      <TableMap
        areas={[area]}
        tables={tables}
        areaID="area"
        onAreaChange={() => undefined}
        onSelect={onSelect}
        unions={[union]}
        groups={[group]}
        onGroupShortcut={onGroupShortcut}
      />,
    );
    const outline = container.querySelector('[data-union-id="union"]')!;
    expect(outline.textContent).toBe('Mesa 5 + 6 · 8 lugares');
    const frame = container.querySelector('[data-group-id="grupo"]')!;
    fireEvent.click(within(frame as HTMLElement).getByRole('button', { name: 'Unir Mesa 3' }));
    expect(onGroupShortcut).toHaveBeenCalledWith(group);
    // Tocar una mesa unida la elige (la pantalla abre la unión); su nombre accesible lo dice.
    const unidas = screen.getByRole('button', { name: /^Mesa 6, unida en Mesa 5 \+ 6, Ocupada/ });
    fireEvent.click(unidas);
    expect(onSelect).toHaveBeenCalledWith(expect.objectContaining({ id: 'm6' }));
  });

  it('elegida una mesa de la unión, toda la unión se ve elegida', () => {
    render(
      <TableMap
        areas={[area]}
        tables={tables}
        areaID="area"
        selectedTableID="m5"
        onAreaChange={() => undefined}
        onSelect={() => undefined}
        unions={[union]}
        groups={[group]}
      />,
    );
    expect(screen.getByRole('button', { name: /^Mesa 6, / }).getAttribute('aria-pressed')).toBe(
      'true',
    );
    expect(screen.getByRole('button', { name: /^Mesa 7, / }).getAttribute('aria-pressed')).toBe(
      'false',
    );
    // Sin el atajo (sin la capacidad o el permiso) el nombre del grupo no se toca.
    expect(screen.queryByRole('button', { name: 'Unir Mesa 3' })).toBeNull();
    expect(screen.getByText('Mesa 3')).toBeTruthy();
  });

  it('la unión que vive en la pantalla se dibuja aparte, con «sin guardar»', () => {
    const { container } = render(
      <TableMap
        areas={[area]}
        tables={tables}
        areaID="area"
        selectedTableID="3a"
        onAreaChange={() => undefined}
        onSelect={() => undefined}
        screenUnion={{
          startTableID: '3a',
          tableIDs: [],
          groupID: 'grupo',
          allTableIDs: ['3a', '3b'],
          name: 'Mesa 3',
          seats: 8,
        }}
      />,
    );
    expect(container.querySelector('.table-union-screen')?.textContent).toBe(
      'Mesa 3 · 8 lugares · sin guardar',
    );
  });

  it('en la lista de mesas encontradas la unión es una sola entrada', () => {
    const results = searchEntries(searchTables(tables, [area], 'mesa', 'area'), [union]);
    expect(results.map((entry) => entry.union?.name ?? entry.table.name)).toEqual([
      'Mesa 5 + 6',
      'Mesa 7',
      'Mesa 8',
    ]);
  });
});

describe('el panel de la unión y sus acciones', () => {
  const actions: TableJoinActions = {
    onJoin: vi.fn(),
    onGroup: vi.fn(),
    onAddTable: vi.fn(),
    onRemoveTable: vi.fn(),
    onDropScreenUnion: vi.fn(),
  };

  function panel(values: Partial<Parameters<typeof TablePanel>[0]>) {
    return render(
      <TablePanel
        table={tables[0]!}
        accounts={[]}
        bill={null}
        billError=""
        localDraft={[]}
        lookups={null}
        can={() => true}
        blocked={false}
        onChooseAccount={() => undefined}
        onCapture={() => undefined}
        onOpenAccount={() => undefined}
        onAction={() => undefined}
        onCheckout={() => undefined}
        onRelease={() => undefined}
        onService={() => undefined}
        {...values}
      />,
    );
  }

  it('una unión: su nombre como título, la capacidad sumada, «Agregar mesa» y «Quitar mesa»', () => {
    panel({ union, joining: actions });
    expect(screen.getByRole('complementary', { name: 'Panel de Mesa 5 + 6' })).toBeTruthy();
    expect(screen.getByText('Mesa para 8')).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: 'Agregar mesa' }));
    expect(actions.onAddTable).toHaveBeenCalledWith(union);
    fireEvent.click(screen.getByRole('button', { name: 'Quitar mesa' }));
    expect(actions.onRemoveTable).toHaveBeenCalledWith(union);
    expect(screen.queryByRole('button', { name: 'Unir con…' })).toBeNull();
    // Una mesa unida no se pone fuera de servicio desde su panel.
    expect(screen.queryByRole('button', { name: 'Poner fuera de servicio' })).toBeNull();
  });

  it('una sección libre: «Unir con…» y el atajo de su grupo', () => {
    panel({ table: tables[4]!, group, joining: actions });
    fireEvent.click(screen.getByRole('button', { name: 'Unir con…' }));
    expect(actions.onJoin).toHaveBeenCalled();
    fireEvent.click(screen.getByRole('button', { name: 'Unir Mesa 3' }));
    expect(actions.onGroup).toHaveBeenCalledWith(group);
  });

  it('la unión en pantalla: su nombre, su nota y «Deshacer la unión»', () => {
    panel({
      table: tables[4]!,
      group,
      joining: actions,
      screenUnion: {
        startTableID: '3a',
        tableIDs: [],
        groupID: 'grupo',
        allTableIDs: ['3a', '3b'],
        name: 'Mesa 3',
        seats: 8,
      },
    });
    expect(screen.getByRole('complementary', { name: 'Panel de Mesa 3' })).toBeTruthy();
    expect(screen.getByText(/La unión se guarda con el primer producto/)).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: 'Deshacer la unión' }));
    expect(actions.onDropScreenUnion).toHaveBeenCalled();
  });

  it('sin la capacidad o sin el permiso no se ve nada nuevo', () => {
    panel({ table: tables[4]!, group, joining: null });
    expect(screen.queryByRole('button', { name: 'Unir con…' })).toBeNull();
    expect(screen.queryByRole('button', { name: 'Unir Mesa 3' })).toBeNull();
    expect(screen.queryByRole('group', { name: 'Unión de mesas' })).toBeNull();
  });
});

describe('«Unir con…»: la vista previa de Go y las mesas que no se pueden unir', () => {
  beforeEach(() => {
    vi.mocked(invoke).mockReset();
  });

  it('motivos de las mesas deshabilitadas, según lo que manda Go en el mapa', () => {
    expect(unavailableReason(tables[2]!, [union])).toBe('Ocupada');
    expect(unavailableReason(tables[1]!, [union])).toBe('Unida con otras mesas');
    expect(unavailableReason(tables[3]!, [union])).toBe('Fuera de servicio o desactivada');
    expect(unavailableReason(tables[4]!, [union])).toBe('');
  });

  it('pide la vista previa con la mesa de inicio primero y muestra el nombre, la capacidad y los problemas', async () => {
    vi.mocked(invoke).mockResolvedValue({
      status: 200,
      body: {
        name: '3A + 3B',
        seats: 8,
        table_ids: ['3a', '3b'],
        problems: [{ code: 'TABLE_UNION_OCCUPIED', message: 'Texto de Go.', table_id: '3b' }],
      },
    });
    const onConfirm = vi.fn();
    render(
      <JoinTablesDialog
        request={{ kind: 'join', start: tables[4]! }}
        overview={overview}
        busy={false}
        error=""
        onCancel={() => undefined}
        onConfirm={onConfirm}
      />,
    );
    const dialog = screen.getByRole('dialog', { name: 'Unir 3A con…' });
    const occupiedOption = within(dialog).getByRole('button', { name: /^Mesa 7/ });
    expect((occupiedOption as HTMLButtonElement).disabled).toBe(true);
    expect(occupiedOption.textContent).toContain('Ocupada');
    fireEvent.click(within(dialog).getByRole('button', { name: /^3B/ }));
    await waitFor(() => expect(dialog.textContent).toContain('3A + 3B · 8 lugares'));
    expect(vi.mocked(invoke)).toHaveBeenCalledWith('api_request', {
      operation: 'restaurant_preview_union',
      resourceId: null,
      body: { table_ids: ['3a', '3b'] },
      query: null,
    });
    // Con problemas de Go no se puede unir; su texto se ve tal cual.
    expect(dialog.textContent).toContain('Texto de Go.');
    expect(
      (within(dialog).getByRole('button', { name: 'Unir' }) as HTMLButtonElement).disabled,
    ).toBe(true);
  });

  it('sin problemas se confirma con lo elegido y la vista previa', async () => {
    vi.mocked(invoke).mockResolvedValue({
      status: 200,
      body: { name: 'Mesa 3', seats: 8, table_ids: ['3a', '3b'], problems: [] },
    });
    const onConfirm = vi.fn();
    render(
      <JoinTablesDialog
        request={{ kind: 'group', group, start: tables[4]! }}
        overview={overview}
        busy={false}
        error=""
        onCancel={() => undefined}
        onConfirm={onConfirm}
      />,
    );
    const dialog = screen.getByRole('dialog', { name: 'Unir Mesa 3' });
    await waitFor(() => expect(dialog.textContent).toContain('Mesa 3 · 8 lugares'));
    fireEvent.click(within(dialog).getByRole('button', { name: 'Unir' }));
    expect(onConfirm).toHaveBeenCalledWith({
      tableIDs: [],
      preview: expect.objectContaining({ name: 'Mesa 3' }),
    });
  });

  it('el rechazo de la escritura se muestra con el texto de Go', () => {
    render(
      <JoinTablesDialog
        request={{ kind: 'remove', union }}
        overview={overview}
        busy={false}
        error="La unión cambió en otra terminal. Revisa las mesas e inténtalo otra vez."
        onCancel={() => undefined}
        onConfirm={() => undefined}
      />,
    );
    const dialog = screen.getByRole('dialog', { name: 'Quitar una mesa de Mesa 5 + 6' });
    expect(within(dialog).getByRole('alert').textContent).toBe(
      'La unión cambió en otra terminal. Revisa las mesas e inténtalo otra vez.',
    );
    // Quitar no pide vista previa: solo se elige una de las mesas de la unión.
    expect(vi.mocked(invoke)).not.toHaveBeenCalled();
    expect(within(dialog).getAllByRole('button', { name: /^Mesa [56]/ })).toHaveLength(2);
  });
});

describe('mesas divisibles en Áreas y distribución (M1-D4)', () => {
  it('lista el grupo con sus secciones y lo quita con PATCH active: false después de confirmarlo', async () => {
    const execute = vi.fn().mockResolvedValue({ group: { ...group, revision: 2 } });
    render(<TableGroupsAdmin overview={overview} areaID="area" execute={execute} busy={false} />);
    fireEvent.click(screen.getByRole('button', { name: /^Mesas divisibles/ }));
    const list = screen.getByRole('list', { name: 'Mesas divisibles del área' });
    expect(list.textContent).toContain('Mesa 3 · 3A, 3B · 8 lugares');
    fireEvent.click(within(list).getByRole('button', { name: 'Quitar Mesa 3' }));
    const confirm = screen.getByRole('alertdialog', { name: '¿Quitar Mesa 3?' });
    fireEvent.click(within(confirm).getByRole('button', { name: 'Quitar grupo' }));
    await waitFor(() =>
      expect(execute).toHaveBeenCalledWith('restaurant_save_table_group', {
        id: 'grupo',
        body: { revision: 1, active: false },
      }),
    );
  });
});

describe('el nombre de la cuenta en una unión (M1-D14 y M1-D29)', () => {
  const inUnion = {
    table_name: 'Mesa 5',
    table_union: { id: 'union', name: 'Mesa 5 + 6', table_ids: ['m5', 'm6'] },
  };

  it('la referencia neutra es el nombre de la unión, también recortado en « + …»', () => {
    expect(accountCustomName({ ...inUnion, name: 'Mesa 5 + 6' })).toBe('');
    const long = {
      table_name: 'Terraza 1',
      table_union: { id: 'union', name: 'Terraza 1 + Terraza 2 + Terraza 3', table_ids: [] },
    };
    expect(accountCustomName({ ...long, name: 'Terraza 1 + Terraza 2 + …' })).toBe('');
    expect(accountCustomName({ ...inUnion, name: 'Cumpleaños' })).toBe('Cumpleaños');
    expect(accountPlace(inUnion as unknown as Account)).toBe('Mesa 5 + 6');
  });
});

// Motor del borrador con la unión diferida (M1-D9): lo que viaja con el primer producto y los rechazos de Go.
describe('la unión diferida en el motor del borrador', () => {
  let creations: {
    body: Parameters<DraftSyncTransport['create']>[0];
    reject: (e: unknown) => void;
  }[];
  let engine: DraftSyncEngine;

  const filete = {
    product_id: 'filete',
    variant_id: null,
    quantity: 1,
    customization: { ingredients: [], groups: [], components: [], notes: '' },
  };
  let sequence = 0;
  const newID = () => `00000000-0000-4000-8000-${String(++sequence).padStart(12, '0')}`;

  function pending(localID: string) {
    return {
      account: { id: localID, revision: 0, state: 'open', table_id: 'm7' },
      draft: [] as DraftItem[],
      lines: [],
      server_time: '2026-11-27T18:00:00Z',
      timezone: 'America/Monterrey',
    } as unknown as Bill;
  }

  beforeEach(() => {
    vi.useFakeTimers();
    creations = [];
    engine = new DraftSyncEngine(
      {
        save: () => new Promise<Bill>(() => undefined),
        create: (body) =>
          new Promise<Bill>((_resolve, reject) => {
            creations.push({ body, reject });
          }),
        retry: () => new Promise<Bill>(() => undefined),
      },
      700,
    );
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('la unión en pantalla viaja con el primer producto; un rechazo de Go no guarda nada y se decide', async () => {
    engine.preparePending('pendiente:union', {
      tableID: 'm7',
      seenAccountIDs: [],
      seenUnionID: null,
      union: { tableIDs: ['m9'], groupID: null },
    });
    engine.stage(pending('pendiente:union'), addLineOperation([], filete, newID));
    await vi.advanceTimersByTimeAsync(700);
    expect(creations[0]!.body).toEqual(
      expect.objectContaining({ table_id: 'm7', union_table_ids: ['m9'], seen_union_id: null }),
    );
    creations[0]!.reject(
      new APIError(
        'TABLE_UNION_OCCUPIED',
        'Solo se pueden unir mesas libres: la Mesa 9 ya está ocupada.',
        409,
      ),
    );
    await vi.advanceTimersByTimeAsync(0);
    expect(engine.unionConflict()).toEqual({
      localID: 'pendiente:union',
      tableID: 'm7',
      code: 'TABLE_UNION_OCCUPIED',
      message: 'Solo se pueden unir mesas libres: la Mesa 9 ya está ocupada.',
      joined: true,
    });
    // Lo capturado se conserva y no se reenvía solo.
    expect(engine.view('pendiente:union')?.items).toHaveLength(1);
    await vi.advanceTimersByTimeAsync(3000);
    expect(creations).toHaveLength(1);
    // «Enviar solo a la mesa»: sin la unión y sin la unión vista.
    engine.dropUnion('pendiente:union');
    expect(engine.unionConflict()).toBeNull();
    expect(creations).toHaveLength(2);
    expect(creations[1]!.body.union_table_ids).toBeUndefined();
    expect(creations[1]!.body.seen_union_id).toBeNull();
  });

  it('el atajo del grupo viaja como union_group_id; «Esperar» deja el texto de Go en la cuenta', async () => {
    engine.preparePending('pendiente:grupo', {
      tableID: '3a',
      seenAccountIDs: [],
      seenUnionID: null,
      union: { tableIDs: [], groupID: 'grupo' },
    });
    engine.stage(pending('pendiente:grupo'), addLineOperation([], filete, newID));
    await vi.advanceTimersByTimeAsync(700);
    expect(creations[0]!.body.union_group_id).toBe('grupo');
    expect(creations[0]!.body.union_table_ids).toBeUndefined();
    creations[0]!.reject(
      new APIError('TABLE_GROUP_SECTION_OCCUPIED', 'La sección 3B está ocupada.', 409),
    );
    await vi.advanceTimersByTimeAsync(0);
    engine.waitForUnion('pendiente:grupo');
    expect(engine.unionConflict()).toBeNull();
    expect(engine.view('pendiente:grupo')?.error).toBe('La sección 3B está ocupada.');
    expect(creations).toHaveLength(1);
  });

  it('TABLE_IN_UNION sin unión en pantalla se decide como una cuenta de otra terminal, en la unión', async () => {
    engine.preparePending('pendiente:mesa', {
      tableID: 'm5',
      seenAccountIDs: [],
      seenUnionID: null,
    });
    engine.stage(pending('pendiente:mesa'), addLineOperation([], filete, newID));
    await vi.advanceTimersByTimeAsync(700);
    // El transporte ya completó las cuentas con su folio (useDraftSync).
    creations[0]!.reject(
      new APIError('TABLE_IN_UNION', 'La Mesa 5 ya está unida con otras mesas (Mesa 5 + 6).', 409, {
        union: { id: 'union', name: 'Mesa 5 + 6' },
        accounts: [{ id: 'cuenta', folio: 27, name: 'Mesa 5 + 6' }],
      }),
    );
    await vi.advanceTimersByTimeAsync(0);
    expect(engine.tableConflict()).toEqual({
      localID: 'pendiente:mesa',
      tableID: 'm5',
      accounts: [{ id: 'cuenta', folio: 27, name: 'Mesa 5 + 6' }],
      union: { id: 'union', name: 'Mesa 5 + 6' },
    });
    // «Abrir una cuenta nueva en la unión»: a sabiendas de la unión y de sus cuentas.
    engine.openNewAccount('pendiente:mesa');
    expect(creations[1]!.body.seen_union_id).toBe('union');
    expect(creations[1]!.body.seen_account_ids).toEqual(['cuenta']);
  });

  it('sin la capacidad no viaja ningún campo de uniones', async () => {
    engine.preparePending('pendiente:vieja', { tableID: 'm7', seenAccountIDs: [] });
    engine.stage(pending('pendiente:vieja'), addLineOperation([], filete, newID));
    await vi.advanceTimersByTimeAsync(700);
    expect(Object.keys(creations[0]!.body).sort()).toEqual([
      'items',
      'mode',
      'seen_account_ids',
      'table_id',
    ]);
  });
});
