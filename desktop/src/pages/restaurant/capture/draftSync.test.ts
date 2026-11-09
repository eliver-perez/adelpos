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

function controlledTransport() {
  const calls: Deferred[] = [];
  const retries: Deferred[] = [];
  const transport: DraftSyncTransport = {
    save: (_accountID, body) =>
      new Promise<Bill>((resolve, reject) => {
        calls.push({ body, resolve, reject });
      }),
    retry: () =>
      new Promise<Bill>((resolve, reject) => {
        retries.push({ body: calls.at(-1)!.body, resolve, reject });
      }),
  };
  return { transport, calls, retries };
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
