import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { APIError } from '../../../services/api';
import type { Bill, DraftItem } from '../../../services/restaurant';
import { addLineOperation, quantityOperation } from './draftOperations';
import { DraftSyncEngine } from './draftSync';
import type { DraftSyncTransport } from './draftSync';

// Motor de sincronización del borrador: antirrebote, una sola petición en vuelo, ningún toque
// perdido, conflictos explícitos y recuperación de la misma operación incierta.

let sequence = 0;
const newID = () => `00000000-0000-4000-8000-${String(++sequence).padStart(12, '0')}`;

function bill(revision: number, draft: DraftItem[] = []): Bill {
  return {
    account: { id: 'cuenta', revision, state: 'open' },
    draft,
    lines: [],
    server_time: '2026-11-08T12:00:00Z',
    timezone: 'America/Monterrey',
  } as unknown as Bill;
}

const filete = {
  product_id: 'filete',
  variant_id: null,
  quantity: 1,
  customization: { ingredients: [], groups: [], components: [], notes: '' },
};

interface Deferred {
  body: { revision: number; items: DraftItem[] };
  resolve: (value: Bill) => void;
  reject: (problem: unknown) => void;
}

interface Creation {
  body: {
    mode: 'dine_in' | 'takeaway';
    table_id: string | null;
    seen_account_ids: string[];
    items: DraftItem[];
  };
  resolve: (value: Bill) => void;
  reject: (problem: unknown) => void;
}

function controlledTransport() {
  const calls: Deferred[] = [];
  const retries: Deferred[] = [];
  const creations: Creation[] = [];
  const saveTargets: string[] = [];
  const transport: DraftSyncTransport = {
    save: (accountID, body) =>
      new Promise<Bill>((resolve, reject) => {
        saveTargets.push(accountID);
        calls.push({ body, resolve, reject });
      }),
    create: (body) =>
      new Promise<Bill>((resolve, reject) => {
        creations.push({ body, resolve, reject });
      }),
    retry: () =>
      new Promise<Bill>((resolve, reject) => {
        retries.push({ body: calls.at(-1)?.body ?? { revision: 0, items: [] }, resolve, reject });
      }),
  };
  return { transport, calls, retries, creations, saveTargets };
}

// Cuenta pendiente de una mesa sin cuenta (etapa 7.1) y la cuenta que Go crea con su borrador.
function pendingBill(localID = 'pendiente:mesa') {
  return {
    account: { id: localID, revision: 0, state: 'open', table_id: 'mesa' },
    draft: [],
    lines: [],
    server_time: '2026-11-08T12:00:00Z',
    timezone: 'America/Monterrey',
  } as unknown as Bill;
}

function createdBill(id: string, items: DraftItem[], revision = 1) {
  return {
    account: { id, revision, state: 'open', table_id: 'mesa' },
    draft: items,
    lines: [],
    server_time: '2026-11-08T12:00:00Z',
    timezone: 'America/Monterrey',
  } as unknown as Bill;
}

// Respuesta de Go a un guardado: el borrador enviado con la revisión siguiente.
function saved(call: Deferred) {
  return bill(call.body.revision + 1, call.body.items);
}

function tap(engine: DraftSyncEngine, current: Bill) {
  const items = engine.view('cuenta')?.items ?? current.draft;
  engine.stage(current, addLineOperation(items, filete, newID));
}

beforeEach(() => {
  vi.useFakeTimers();
  sequence = 0;
});

afterEach(() => {
  vi.useRealTimers();
});

describe('sincronización del borrador de la captura', () => {
  it('agrupa los toques con antirrebote y envía el documento completo con su revisión', async () => {
    const { transport, calls } = controlledTransport();
    const engine = new DraftSyncEngine(transport, 700);
    const current = bill(4);
    tap(engine, current);
    await vi.advanceTimersByTimeAsync(400);
    tap(engine, current);
    await vi.advanceTimersByTimeAsync(400);
    tap(engine, current);
    // Cada toque reinicia la espera: todavía no hay petición, pero la cuenta ya muestra 3.
    expect(calls).toHaveLength(0);
    expect(engine.view('cuenta')?.items).toEqual([expect.objectContaining({ quantity: 3 })]);
    await vi.advanceTimersByTimeAsync(699);
    expect(calls).toHaveLength(0);
    await vi.advanceTimersByTimeAsync(1);
    expect(calls).toHaveLength(1);
    expect(calls[0]!.body.revision).toBe(4);
    expect(calls[0]!.body.items).toEqual([
      expect.objectContaining({ product_id: 'filete', quantity: 3 }),
    ]);
  });

  it('mantiene una sola petición en vuelo y envía al terminar los toques acumulados', async () => {
    const { transport, calls } = controlledTransport();
    const engine = new DraftSyncEngine(transport, 700);
    const current = bill(7);
    tap(engine, current);
    await vi.advanceTimersByTimeAsync(700);
    expect(calls).toHaveLength(1);
    // Llegan más toques mientras la primera petición sigue sin respuesta.
    tap(engine, current);
    tap(engine, current);
    await vi.advanceTimersByTimeAsync(2000);
    expect(calls).toHaveLength(1);
    expect(engine.view('cuenta')?.items[0]?.quantity).toBe(3);
    calls[0]!.resolve(saved(calls[0]!));
    await vi.advanceTimersByTimeAsync(0);
    // Al terminar se envía el documento más reciente con la revisión que devolvió Go.
    expect(calls).toHaveLength(2);
    expect(calls[1]!.body.revision).toBe(8);
    expect(calls[1]!.body.items).toEqual([expect.objectContaining({ quantity: 3 })]);
    calls[1]!.resolve(saved(calls[1]!));
    await vi.advanceTimersByTimeAsync(0);
    const view = engine.view('cuenta');
    expect(view?.pending).toBe(false);
    expect(view?.baseRevision).toBe(9);
    expect(view?.items[0]?.quantity).toBe(3);
    expect(calls).toHaveLength(2);
  });

  it('convierte un cambio de otra terminal en un conflicto explícito y conserva los toques', async () => {
    const { transport, calls } = controlledTransport();
    const engine = new DraftSyncEngine(transport, 700);
    const current = bill(3);
    tap(engine, current);
    const remote: DraftItem = { ...filete, product_id: 'agua', id: newID() };
    engine.observe(bill(4, [remote]));
    expect(engine.view('cuenta')?.conflict).toBe(true);
    await vi.advanceTimersByTimeAsync(2000);
    expect(calls).toHaveLength(0);
    // «Conservar mis productos nuevos» aplica los toques sobre la cuenta actual.
    engine.keepMine('cuenta');
    expect(calls).toHaveLength(1);
    expect(calls[0]!.body.revision).toBe(4);
    expect(calls[0]!.body.items.map((item) => item.product_id)).toEqual(['agua', 'filete']);
  });

  it('un rechazo por revisión también queda como conflicto y «Descartar» adopta la cuenta actual', async () => {
    const { transport, calls } = controlledTransport();
    const engine = new DraftSyncEngine(transport, 700);
    tap(engine, bill(2));
    await vi.advanceTimersByTimeAsync(700);
    calls[0]!.reject(new APIError('ACCOUNT_CONFLICT', 'Cambió', 409));
    await vi.advanceTimersByTimeAsync(0);
    expect(engine.view('cuenta')?.conflict).toBe(true);
    expect(engine.view('cuenta')?.items).toHaveLength(1);
    engine.observe(bill(3));
    engine.discard('cuenta');
    expect(engine.view('cuenta')).toEqual(
      expect.objectContaining({ conflict: false, pending: false, baseRevision: 3, items: [] }),
    );
  });

  it('ante un resultado incierto no envía nada más y recupera la misma operación', async () => {
    const { transport, calls, retries } = controlledTransport();
    const engine = new DraftSyncEngine(transport, 700);
    const current = bill(5);
    tap(engine, current);
    await vi.advanceTimersByTimeAsync(700);
    calls[0]!.reject(new APIError('CONNECTION_ERROR', 'Sin conexión'));
    await vi.advanceTimersByTimeAsync(0);
    expect(engine.uncertain).toBe(true);
    tap(engine, current);
    await vi.advanceTimersByTimeAsync(2000);
    expect(calls).toHaveLength(1);
    await expect(engine.settle('cuenta')).rejects.toThrow('Recupera');
    const recovery = engine.recover();
    expect(retries).toHaveLength(1);
    retries[0]!.resolve(saved(calls[0]!));
    await recovery;
    await vi.advanceTimersByTimeAsync(0);
    // La operación recuperada confirmó el primer toque; el segundo se envía después.
    expect(engine.uncertain).toBe(false);
    expect(calls).toHaveLength(2);
    expect(calls[1]!.body.revision).toBe(6);
    expect(calls[1]!.body.items[0]?.quantity).toBe(2);
  });

  it('«settle» envía de inmediato lo pendiente y resuelve con la cuenta sincronizada', async () => {
    const { transport, calls } = controlledTransport();
    const engine = new DraftSyncEngine(transport, 700);
    tap(engine, bill(1));
    const settled = engine.settle('cuenta');
    expect(calls).toHaveLength(1);
    calls[0]!.resolve(saved(calls[0]!));
    await expect(settled).resolves.toEqual(
      expect.objectContaining({ pending: false, baseRevision: 2 }),
    );
  });

  it('los toques durante el envío a cocina se aplican sobre la cuenta confirmada', async () => {
    const { transport, calls } = controlledTransport();
    const engine = new DraftSyncEngine(transport, 700);
    const current = bill(1);
    tap(engine, current);
    const settled = engine.settle('cuenta');
    calls[0]!.resolve(saved(calls[0]!));
    await settled;
    const line = engine.view('cuenta')!.items[0]!;
    engine.hold('cuenta');
    // «+» sobre la partida que se está enviando y otro producto nuevo.
    engine.stage(current, quantityOperation(line, 1, newID));
    await vi.advanceTimersByTimeAsync(2000);
    expect(calls).toHaveLength(1);
    // Go confirmó: el borrador queda vacío en la revisión 3.
    engine.release('cuenta', bill(3, []));
    await vi.advanceTimersByTimeAsync(700);
    expect(calls).toHaveLength(2);
    expect(calls[1]!.body.revision).toBe(3);
    expect(calls[1]!.body.items).toEqual([
      expect.objectContaining({ product_id: 'filete', quantity: 1 }),
    ]);
  });
});

describe('cuenta creada con el primer producto (etapa 7.1)', () => {
  it('el primer guardado crea la cuenta con su borrador y los siguientes usan PUT …/draft', async () => {
    const { transport, calls, creations, saveTargets } = controlledTransport();
    const engine = new DraftSyncEngine(transport, 700);
    const created: string[] = [];
    engine.onAccountCreated((localID, result) => created.push(`${localID}>${result.account.id}`));
    engine.preparePending('pendiente:mesa', { tableID: 'mesa', seenAccountIDs: ['vista'] });
    const pending = pendingBill();
    engine.stage(pending, addLineOperation([], filete, newID));
    engine.stage(pending, addLineOperation(engine.view('pendiente:mesa')!.items, filete, newID));
    await vi.advanceTimersByTimeAsync(700);
    // Una sola creación, con antirrebote, la mesa, las cuentas vistas y el borrador completo.
    expect(creations).toHaveLength(1);
    expect(calls).toHaveLength(0);
    expect(creations[0]!.body).toEqual({
      mode: 'dine_in',
      table_id: 'mesa',
      seen_account_ids: ['vista'],
      items: [expect.objectContaining({ product_id: 'filete', quantity: 2 })],
    });
    // Un toque durante la creación se acumula sobre la cuenta pendiente.
    engine.stage(pending, addLineOperation(engine.view('pendiente:mesa')!.items, filete, newID));
    creations[0]!.resolve(createdBill('cuenta-real', creations[0]!.body.items));
    await vi.advanceTimersByTimeAsync(0);
    expect(created).toEqual(['pendiente:mesa>cuenta-real']);
    expect(engine.resolve('pendiente:mesa')).toBe('cuenta-real');
    expect(engine.isPending('pendiente:mesa')).toBe(false);
    // El toque acumulado viaja con PUT …/draft a la cuenta real y su revisión (con su antirrebote).
    await vi.advanceTimersByTimeAsync(700);
    expect(calls).toHaveLength(1);
    expect(saveTargets).toEqual(['cuenta-real']);
    expect(calls[0]!.body.revision).toBe(1);
    expect(calls[0]!.body.items[0]?.quantity).toBe(3);
    expect(engine.view('pendiente:mesa')?.accountID).toBe('cuenta-real');
  });

  it('salir sin productos no crea nada; toques que se anulan tampoco', async () => {
    const { transport, calls, creations } = controlledTransport();
    const engine = new DraftSyncEngine(transport, 700);
    engine.preparePending('pendiente:mesa', { tableID: 'mesa', seenAccountIDs: [] });
    expect(engine.discardPendingIfIdle('pendiente:mesa')).toBe(true);
    engine.preparePending('pendiente:otra', { tableID: 'mesa', seenAccountIDs: [] });
    const pending = pendingBill('pendiente:otra');
    engine.stage(pending, addLineOperation([], filete, newID));
    const line = engine.view('pendiente:otra')!.items[0]!;
    engine.stage(pending, quantityOperation(line, -1, newID));
    engine.flushNow('pendiente:otra');
    await vi.advanceTimersByTimeAsync(2000);
    expect(creations).toHaveLength(0);
    expect(calls).toHaveLength(0);
    expect(engine.discardPendingIfIdle('pendiente:otra')).toBe(true);
    expect(engine.view('pendiente:otra')).toBeNull();
  });

  it('TABLE_HAS_ACCOUNT detiene la creación y conserva los toques hasta decidir', async () => {
    const { transport, calls, creations, saveTargets } = controlledTransport();
    const engine = new DraftSyncEngine(transport, 700);
    engine.preparePending('pendiente:mesa', { tableID: 'mesa', seenAccountIDs: [] });
    engine.stage(pendingBill(), addLineOperation([], filete, newID));
    await vi.advanceTimersByTimeAsync(700);
    const other = { id: 'cuenta-ajena', folio: 12, name: 'Mesa 3' };
    creations[0]!.reject(
      new APIError('TABLE_HAS_ACCOUNT', 'Esta mesa ya tiene una cuenta.', 409, {
        accounts: [other],
      }),
    );
    await vi.advanceTimersByTimeAsync(0);
    expect(engine.tableConflict()).toEqual({
      localID: 'pendiente:mesa',
      tableID: 'mesa',
      accounts: [other],
    });
    expect(engine.view('pendiente:mesa')?.items).toHaveLength(1);
    expect(engine.discardPendingIfIdle('pendiente:mesa')).toBe(false);
    await expect(engine.settle('pendiente:mesa')).rejects.toThrow('ya tiene una cuenta');
    await vi.advanceTimersByTimeAsync(2000);
    expect(creations).toHaveLength(1);
    // «Abrir una cuenta nueva»: se reenvía sabiendo de la cuenta informada.
    engine.openNewAccount('pendiente:mesa');
    expect(engine.tableConflict()).toBeNull();
    expect(creations).toHaveLength(2);
    expect(creations[1]!.body.seen_account_ids).toEqual(['cuenta-ajena']);
    creations[1]!.reject(
      new APIError('TABLE_HAS_ACCOUNT', 'Esta mesa ya tiene una cuenta.', 409, {
        accounts: [{ id: 'cuenta-tercera', folio: 13, name: 'Mesa 3' }],
      }),
    );
    await vi.advanceTimersByTimeAsync(0);
    // «Agregar a esa cuenta»: los toques se aplican sobre su borrador vigente y su revisión.
    const theirs: DraftItem = { ...filete, product_id: 'agua', id: newID() };
    engine.addToAccount('pendiente:mesa', createdBill('cuenta-tercera', [theirs], 6));
    expect(engine.resolve('pendiente:mesa')).toBe('cuenta-tercera');
    expect(saveTargets).toEqual(['cuenta-tercera']);
    expect(calls[0]!.body.revision).toBe(6);
    expect(calls[0]!.body.items.map((item) => item.product_id)).toEqual(['agua', 'filete']);
  });

  it('el pedido para llevar y la cuenta sin mesa nacen con el primer producto (etapa 9D)', async () => {
    const { transport, calls, creations, saveTargets } = controlledTransport();
    const engine = new DraftSyncEngine(transport, 700);
    const created: string[] = [];
    engine.onAccountCreated((localID, result) => created.push(`${localID}>${result.account.id}`));
    // Sin productos no se crea nada.
    engine.preparePending('pendiente:pedido', {
      mode: 'takeaway',
      tableID: null,
      seenAccountIDs: [],
    });
    expect(engine.discardPendingIfIdle('pendiente:pedido')).toBe(true);
    await vi.advanceTimersByTimeAsync(2000);
    expect(creations).toHaveLength(0);
    // Con el primer producto nace el pedido, sin mesa y con su modalidad.
    engine.preparePending('pendiente:otro', {
      mode: 'takeaway',
      tableID: null,
      seenAccountIDs: [],
    });
    engine.stage(pendingBill('pendiente:otro'), addLineOperation([], filete, newID));
    await vi.advanceTimersByTimeAsync(700);
    expect(creations).toHaveLength(1);
    expect(creations[0]!.body).toEqual({
      mode: 'takeaway',
      table_id: null,
      seen_account_ids: [],
      items: [expect.objectContaining({ product_id: 'filete', quantity: 1 })],
    });
    creations[0]!.resolve(createdBill('pedido-real', creations[0]!.body.items));
    await vi.advanceTimersByTimeAsync(0);
    expect(created).toEqual(['pendiente:otro>pedido-real']);
    expect(engine.resolve('pendiente:otro')).toBe('pedido-real');
    // La cuenta sin mesa: en el salón, sin mesa. Un rechazo TABLE_HAS_ACCOUNT (imposible sin mesa)
    // no la detiene a la espera de una decisión: queda como error del guardado.
    engine.preparePending('pendiente:sin-mesa', {
      mode: 'dine_in',
      tableID: null,
      seenAccountIDs: [],
    });
    engine.stage(pendingBill('pendiente:sin-mesa'), addLineOperation([], filete, newID));
    await vi.advanceTimersByTimeAsync(700);
    expect(creations[1]!.body).toEqual(
      expect.objectContaining({ mode: 'dine_in', table_id: null, seen_account_ids: [] }),
    );
    creations[1]!.reject(
      new APIError('TABLE_HAS_ACCOUNT', 'Esta mesa ya tiene una cuenta.', 409, {
        accounts: [{ id: 'otra', folio: 4, name: 'Cuenta' }],
      }),
    );
    await vi.advanceTimersByTimeAsync(0);
    expect(engine.tableConflict()).toBeNull();
    expect(engine.view('pendiente:sin-mesa')?.error).toBe('Esta mesa ya tiene una cuenta.');
    expect(calls).toHaveLength(0);
    expect(saveTargets).toEqual([]);
  });

  it('una creación incierta se recupera con la misma operación y pasa a la cuenta real', async () => {
    const { transport, creations, retries } = controlledTransport();
    const engine = new DraftSyncEngine(transport, 700);
    engine.preparePending('pendiente:mesa', { tableID: 'mesa', seenAccountIDs: [] });
    engine.stage(pendingBill(), addLineOperation([], filete, newID));
    await vi.advanceTimersByTimeAsync(700);
    creations[0]!.reject(new APIError('CONNECTION_ERROR', 'Sin conexión'));
    await vi.advanceTimersByTimeAsync(0);
    expect(engine.uncertainAccountID).toBe('pendiente:mesa');
    expect(engine.discardPendingIfIdle('pendiente:mesa')).toBe(false);
    const recovery = engine.recover();
    expect(retries).toHaveLength(1);
    retries[0]!.resolve(createdBill('cuenta-real', creations[0]!.body.items));
    await recovery;
    expect(engine.uncertain).toBe(false);
    expect(engine.resolve('pendiente:mesa')).toBe('cuenta-real');
    expect(engine.view('cuenta-real')?.pending).toBe(false);
  });
});
