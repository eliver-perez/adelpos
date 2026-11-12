import { APIError } from '../../../services/api';
import type { Bill, DraftItem, TableAccountRef } from '../../../services/restaurant';
import { applyOperations } from './draftOperations';
import type { DraftOperation } from './draftOperations';

// Sincronización del borrador persistido de la captura táctil.
//
// - Cada toque modifica primero el estado local (una operación pendiente) sin esperar la red.
// - El borrador se guarda con antirrebote: `delay` milisegundos después del último toque.
// - Hay como máximo una petición en vuelo en toda la terminal. Siempre envía el documento
//   completo más reciente (borrador confirmado por Go + operaciones pendientes) con la revisión
//   en la que se basa. Los toques que llegan durante la petición se acumulan y se envían al
//   terminar; ninguno se pierde ni genera una petición propia.
// - Un error de comunicación o del servidor deja el resultado incierto: no se envía nada más
//   hasta recuperar la misma operación (mismo operation_id) con `recover`.
// - Si otra terminal cambió la cuenta, el conflicto es explícito: se conservan los toques y el
//   operador decide si aplicarlos sobre la cuenta actual o descartarlos.
// - Etapa 7.1: una mesa sin cuenta se captura con una «cuenta pendiente» (identificador local). El
//   primer guardado crea la cuenta con su borrador en una sola operación idempotente; después la
//   entrada pasa al identificador real y los guardados siguientes usan PUT …/draft. Si otra
//   terminal abrió una cuenta en la mesa (TABLE_HAS_ACCOUNT), no se crea nada: los toques se
//   conservan hasta que el operador elige agregarlos a esa cuenta o abrir una nueva.
// - Etapa 9D (§4.1, decisión 2): el pedido para llevar y la cuenta sin mesa nacen igual, con su
//   primer producto (sin mesa no hay cuentas vistas que comparar).

export interface DraftSyncTransport {
  // Guarda el borrador completo con un operation_id nuevo.
  save(accountID: string, body: { revision: number; items: DraftItem[] }): Promise<Bill>;
  // Crea la cuenta con su primer producto (etapa 7.1; desde la 9D también para llevar y sin mesa)
  // con un operation_id nuevo.
  create(body: {
    mode: 'dine_in' | 'takeaway';
    table_id: string | null;
    seen_account_ids: string[];
    items: DraftItem[];
  }): Promise<Bill>;
  // Reintenta la última operación incierta con el mismo operation_id y cuerpo.
  retry(): Promise<Bill>;
}

export interface DraftView {
  accountID: string;
  // Borrador visible: lo confirmado por Go más los toques pendientes.
  items: DraftItem[];
  // Último borrador que Go confirmó y la revisión de la cuenta en la que se basa.
  base: DraftItem[];
  baseRevision: number;
  pending: boolean;
  sending: boolean;
  held: boolean;
  conflict: boolean;
  uncertain: boolean;
  error: string;
}

// Mesa y cuentas que la terminal conocía al empezar a capturar sin cuenta. Etapa 9D: la modalidad
// (en salón por omisión) y la mesa opcional del pedido para llevar o de la cuenta sin mesa.
export interface PendingCreation {
  mode?: 'dine_in' | 'takeaway';
  tableID: string | null;
  seenAccountIDs: string[];
}

// Cuenta pendiente detenida porque otra terminal abrió una cuenta en la mesa.
export interface TableAccountConflict {
  localID: string;
  tableID: string;
  accounts: TableAccountRef[];
}

interface Creation extends PendingCreation {
  // Cuentas que Go informó con TABLE_HAS_ACCOUNT; mientras existan no se envía nada.
  accounts: TableAccountRef[] | null;
}

export class DraftSyncError extends Error {
  constructor(
    public reason: 'conflict' | 'uncertain' | 'table' | 'error',
    message: string,
  ) {
    super(message);
    this.name = 'DraftSyncError';
  }
}

interface Entry {
  accountID: string;
  base: DraftItem[];
  baseRevision: number;
  operations: DraftOperation[];
  // Operaciones incluidas en la petición en vuelo (o incierta).
  sentCount: number;
  conflict: boolean;
  error: string;
  // Se pidió enviar mientras otra petición estaba en vuelo o la cuenta estaba retenida.
  due: boolean;
  timer: ReturnType<typeof setTimeout> | null;
  // Última cuenta vista desde el servidor: base para conservar o descartar en un conflicto.
  latest: Bill | null;
}

function tableAccountsOf(problem: unknown): TableAccountRef[] | null {
  if (!(problem instanceof APIError) || problem.code !== 'TABLE_HAS_ACCOUNT') {
    return null;
  }
  const details = problem.details as { accounts?: TableAccountRef[] } | undefined;
  return details?.accounts?.length ? details.accounts : null;
}

function conflictError(problem: unknown) {
  return (
    problem instanceof APIError &&
    problem.status === 409 &&
    (problem.code === 'ACCOUNT_CONFLICT' || problem.code === 'REVISION_CONFLICT')
  );
}

export function uncertainError(problem: unknown) {
  return (
    problem instanceof APIError && (problem.code === 'CONNECTION_ERROR' || problem.status >= 500)
  );
}

function message(problem: unknown, fallback: string) {
  return problem instanceof Error && problem.message ? problem.message : fallback;
}

export class DraftSyncEngine {
  private entries = new Map<string, Entry>();
  private listeners = new Set<() => void>();
  private version = 0;
  private inFlight: { accountID: string; promise: Promise<void> } | null = null;
  private uncertainAccount: string | null = null;
  private held = new Set<string>();
  // Cuentas pendientes (identificador local → mesa y cuentas vistas) y a qué cuenta real pasó
  // cada una al crearse o al agregarse a otra.
  private creations = new Map<string, Creation>();
  private renamed = new Map<string, string>();
  private createdListeners = new Set<(localID: string, bill: Bill) => void>();

  constructor(
    private transport: DraftSyncTransport,
    private delay = 700,
  ) {}

  subscribe = (listener: () => void) => {
    this.listeners.add(listener);
    return () => {
      this.listeners.delete(listener);
    };
  };

  getVersion = () => this.version;

  // Avisa cuando una cuenta pendiente pasa a una cuenta real (creada o existente).
  onAccountCreated = (listener: (localID: string, bill: Bill) => void) => {
    this.createdListeners.add(listener);
    return () => {
      this.createdListeners.delete(listener);
    };
  };

  // Identificador vigente de una cuenta: el real si la cuenta pendiente ya se creó.
  resolve(accountID: string): string {
    let current = accountID;
    while (this.renamed.has(current)) {
      current = this.renamed.get(current)!;
    }
    return current;
  }

  isPending(accountID: string): boolean {
    return this.creations.has(accountID);
  }

  // Registra una cuenta pendiente de la mesa. Las pendientes anteriores que no recibieron ningún
  // toque se olvidan: no hay nada que crear.
  preparePending(localID: string, creation: PendingCreation) {
    for (const id of this.creations.keys()) {
      if (id !== localID && !this.entries.has(id)) {
        this.creations.delete(id);
      }
    }
    this.creations.set(localID, { ...creation, accounts: null });
  }

  // Olvida una cuenta pendiente sin toques ni envíos: salir sin productos no deja registro.
  // Devuelve false si todavía hay algo que crear o resolver.
  discardPendingIfIdle(localID: string): boolean {
    const creation = this.creations.get(localID);
    if (!creation) {
      return false;
    }
    const entry = this.entries.get(localID);
    const idle =
      !creation.accounts &&
      (!entry ||
        (entry.operations.length === 0 && !this.sendingAccount(localID) && entry.timer === null));
    if (!idle) {
      return false;
    }
    this.creations.delete(localID);
    this.entries.delete(localID);
    this.emit();
    return true;
  }

  // Primera cuenta pendiente detenida por TABLE_HAS_ACCOUNT.
  tableConflict(): TableAccountConflict | null {
    for (const [localID, creation] of this.creations) {
      if (creation.accounts && creation.tableID) {
        return { localID, tableID: creation.tableID, accounts: creation.accounts };
      }
    }
    return null;
  }

  // «Abrir una cuenta nueva»: se reenvía la creación sabiendo de las cuentas informadas (con un
  // operation_id nuevo: el rechazo no confirmó nada).
  openNewAccount(localID: string) {
    const creation = this.creations.get(localID);
    const entry = this.entries.get(localID);
    if (!creation?.accounts || !entry) {
      return;
    }
    creation.seenAccountIDs = [
      ...creation.seenAccountIDs,
      ...creation.accounts.map((account) => account.id),
    ];
    creation.accounts = null;
    entry.error = '';
    this.emit();
    this.trySend(entry);
  }

  // «Agregar a esa cuenta»: los toques de la cuenta pendiente se aplican sobre el borrador de la
  // cuenta existente, que conserva el suyo, y se guardan con su revisión.
  addToAccount(localID: string, target: Bill) {
    const creation = this.creations.get(localID);
    const entry = this.entries.get(localID);
    if (!creation || !entry || this.sendingAccount(localID)) {
      return;
    }
    this.clearTimer(entry);
    this.creations.delete(localID);
    this.entries.delete(localID);
    this.renamed.set(localID, target.account.id);
    let destination = this.entries.get(target.account.id);
    if (destination) {
      destination.operations.push(...entry.operations);
      this.observe(target);
    } else {
      destination = entry;
      destination.accountID = target.account.id;
      destination.base = target.draft;
      destination.baseRevision = target.account.revision;
      destination.latest = target;
      destination.conflict = false;
      destination.error = '';
      destination.sentCount = 0;
      this.entries.set(target.account.id, destination);
    }
    this.notifyCreated(localID, target);
    this.emit();
    this.trySend(destination);
  }

  private notifyCreated(localID: string, bill: Bill) {
    for (const listener of this.createdListeners) {
      listener(localID, bill);
    }
  }

  private emit() {
    this.version++;
    for (const listener of this.listeners) {
      listener();
    }
  }

  private sendingAccount(accountID: string) {
    return this.inFlight?.accountID === accountID || this.uncertainAccount === accountID;
  }

  get busy() {
    return this.inFlight !== null;
  }

  get uncertain() {
    return this.uncertainAccount !== null;
  }

  // Cuenta con una operación de borrador incierta: bloquea las demás escrituras de la cuenta.
  get uncertainAccountID() {
    return this.uncertainAccount;
  }

  // Cuenta más reciente que conoce el motor (respuesta de un guardado propio o la última
  // observada): evita mostrar por un instante una revisión anterior mientras llega el sondeo.
  latestBill(accountID: string): Bill | null {
    return this.entries.get(this.resolve(accountID))?.latest ?? null;
  }

  view(requested: string): DraftView | null {
    const accountID = this.resolve(requested);
    const entry = this.entries.get(accountID);
    if (!entry) {
      return null;
    }
    return {
      accountID,
      items: applyOperations(entry.base, entry.operations),
      base: entry.base,
      baseRevision: entry.baseRevision,
      pending: entry.operations.length > 0,
      sending: this.sendingAccount(accountID),
      held: this.held.has(accountID),
      conflict: entry.conflict,
      uncertain: this.uncertainAccount === accountID,
      error: entry.error,
    };
  }

  // Registra un toque sobre la cuenta. Si no había estado local, parte de la cuenta mostrada.
  stage(bill: Bill, operation: DraftOperation) {
    const entry = this.ensure(bill);
    entry.operations.push(operation);
    entry.error = '';
    this.schedule(entry);
    this.emit();
  }

  private ensure(bill: Bill): Entry {
    // Un toque sobre la cuenta pendiente que acaba de crearse se suma a la entrada real.
    const existing = this.entries.get(this.resolve(bill.account.id));
    if (existing) {
      if (existing.accountID === bill.account.id) {
        this.observe(bill);
      }
      return existing;
    }
    const entry: Entry = {
      accountID: bill.account.id,
      base: bill.draft,
      baseRevision: bill.account.revision,
      operations: [],
      sentCount: 0,
      conflict: false,
      error: '',
      due: false,
      timer: null,
      latest: bill,
    };
    this.entries.set(bill.account.id, entry);
    return entry;
  }

  // Concilia el estado local con la cuenta que llegó por sondeo o tiempo real. Una revisión más
  // reciente sin toques pendientes se adopta; con toques pendientes es un conflicto explícito.
  // Mientras hay una petición propia en vuelo no se concilia: su respuesta es la autoridad.
  observe(bill: Bill) {
    const entry = this.entries.get(bill.account.id);
    if (!entry) {
      return;
    }
    if (!entry.latest || bill.account.revision >= entry.latest.account.revision) {
      entry.latest = bill;
    }
    if (this.sendingAccount(entry.accountID) || this.held.has(entry.accountID)) {
      return;
    }
    if (bill.account.revision <= entry.baseRevision) {
      return;
    }
    if (entry.operations.length === 0) {
      entry.base = bill.draft;
      entry.baseRevision = bill.account.revision;
      entry.conflict = false;
      entry.error = '';
      this.emit();
      return;
    }
    if (!entry.conflict) {
      entry.conflict = true;
      this.clearTimer(entry);
      this.emit();
    }
  }

  private clearTimer(entry: Entry) {
    if (entry.timer !== null) {
      clearTimeout(entry.timer);
      entry.timer = null;
    }
  }

  private schedule(entry: Entry) {
    this.clearTimer(entry);
    entry.timer = setTimeout(() => {
      entry.timer = null;
      this.trySend(entry);
    }, this.delay);
  }

  // Envía de inmediato lo pendiente de la cuenta (al cambiar de cuenta o de vista).
  flushNow(accountID: string) {
    const entry = this.entries.get(this.resolve(accountID));
    if (!entry) {
      return;
    }
    this.clearTimer(entry);
    this.trySend(entry);
  }

  private trySend(entry: Entry) {
    if (entry.operations.length === 0 || entry.conflict || this.blockedCreation(entry)) {
      return;
    }
    if (this.inFlight || this.uncertainAccount || this.held.has(entry.accountID)) {
      entry.due = true;
      this.emit();
      return;
    }
    this.send(entry);
  }

  // La cuenta pendiente espera la decisión del operador ante TABLE_HAS_ACCOUNT.
  private blockedCreation(entry: Entry) {
    return !!this.creations.get(entry.accountID)?.accounts;
  }

  private send(entry: Entry) {
    const items = applyOperations(entry.base, entry.operations);
    const creation = this.creations.get(entry.accountID);
    if (creation && items.length === 0) {
      // Los toques se anularon entre sí (agregar y quitar): no hay cuenta que crear.
      entry.operations = [];
      entry.sentCount = 0;
      entry.due = false;
      entry.error = '';
      this.emit();
      return;
    }
    entry.sentCount = entry.operations.length;
    entry.due = false;
    entry.error = '';
    const request = creation
      ? this.transport.create({
          mode: creation.mode ?? 'dine_in',
          table_id: creation.tableID,
          seen_account_ids: [...creation.seenAccountIDs],
          items,
        })
      : this.transport.save(entry.accountID, { revision: entry.baseRevision, items });
    const promise = request
      .then(
        (bill) => {
          if (creation) {
            this.created(entry, bill);
          } else {
            this.acknowledge(entry, bill);
          }
        },
        (problem: unknown) => this.reject(entry, problem),
      )
      .finally(() => this.next());
    this.inFlight = { accountID: entry.accountID, promise };
    this.emit();
  }

  // La cuenta pendiente ya existe en Go: la entrada pasa a su identificador real y los toques
  // siguientes se guardan con PUT …/draft.
  private created(entry: Entry, bill: Bill) {
    const localID = entry.accountID;
    const accountID = bill.account.id;
    this.creations.delete(localID);
    this.entries.delete(localID);
    this.renamed.set(localID, accountID);
    entry.accountID = accountID;
    this.entries.set(accountID, entry);
    if (this.inFlight?.accountID === localID) {
      this.inFlight.accountID = accountID;
    }
    if (this.uncertainAccount === localID) {
      this.uncertainAccount = accountID;
    }
    this.acknowledge(entry, bill);
    this.notifyCreated(localID, bill);
  }

  private acknowledge(entry: Entry, bill: Bill) {
    entry.base = bill.draft;
    entry.baseRevision = bill.account.revision;
    entry.latest = bill;
    entry.operations = entry.operations.slice(entry.sentCount);
    entry.sentCount = 0;
    entry.conflict = false;
    entry.error = '';
    if (entry.operations.length > 0) {
      entry.due = true;
    }
  }

  private reject(entry: Entry, problem: unknown) {
    if (uncertainError(problem)) {
      // Se conservan las operaciones enviadas: la recuperación confirmará cuáles se aplicaron.
      this.uncertainAccount = entry.accountID;
      entry.error = message(problem, 'No se pudo confirmar el guardado del borrador.');
      return;
    }
    entry.sentCount = 0;
    const creation = this.creations.get(entry.accountID);
    // Sin mesa (para llevar o cuenta sin mesa) no hay cuentas de la mesa que decidir.
    const accounts = creation?.tableID ? tableAccountsOf(problem) : null;
    if (creation && accounts) {
      // Otra terminal abrió una cuenta en la mesa: Go no creó nada y el operador decide.
      creation.accounts = accounts;
      entry.error = '';
      return;
    }
    if (conflictError(problem)) {
      entry.conflict = true;
      entry.error = '';
      return;
    }
    entry.error = message(problem, 'No se pudo guardar el borrador.');
  }

  private next() {
    this.inFlight = null;
    if (!this.uncertainAccount) {
      for (const entry of this.entries.values()) {
        if (
          entry.due &&
          entry.operations.length > 0 &&
          !entry.conflict &&
          !this.blockedCreation(entry) &&
          entry.timer === null &&
          !this.held.has(entry.accountID)
        ) {
          this.send(entry);
          if (this.inFlight) {
            return;
          }
        }
      }
    }
    this.emit();
  }

  // Recupera la operación incierta con el mismo operation_id y cuerpo.
  recover(): Promise<void> {
    const accountID = this.uncertainAccount;
    const entry = accountID ? this.entries.get(accountID) : undefined;
    if (!entry || this.inFlight) {
      return this.inFlight?.promise ?? Promise.resolve();
    }
    const promise = this.transport
      .retry()
      .then(
        (bill) => {
          this.uncertainAccount = null;
          if (this.creations.has(entry.accountID)) {
            this.created(entry, bill);
          } else {
            this.acknowledge(entry, bill);
          }
        },
        (problem: unknown) => {
          if (uncertainError(problem)) {
            entry.error = message(problem, 'No se pudo confirmar el guardado del borrador.');
            return;
          }
          this.uncertainAccount = null;
          this.reject(entry, problem);
        },
      )
      .finally(() => this.next());
    this.inFlight = { accountID: entry.accountID, promise };
    this.emit();
    return promise;
  }

  // Resuelve un conflicto aplicando los toques pendientes sobre la cuenta actual.
  keepMine(requested: string) {
    const entry = this.entries.get(this.resolve(requested));
    if (!entry?.latest || !entry.conflict) {
      return;
    }
    entry.base = entry.latest.draft;
    entry.baseRevision = entry.latest.account.revision;
    entry.conflict = false;
    entry.error = '';
    this.emit();
    this.trySend(entry);
  }

  // Descarta los toques pendientes y adopta la cuenta actual del servidor.
  discard(requested: string) {
    const accountID = this.resolve(requested);
    const entry = this.entries.get(accountID);
    if (!entry || this.sendingAccount(accountID)) {
      return;
    }
    this.clearTimer(entry);
    entry.operations = [];
    entry.conflict = false;
    entry.error = '';
    entry.due = false;
    if (entry.latest) {
      entry.base = entry.latest.draft;
      entry.baseRevision = entry.latest.account.revision;
    }
    this.emit();
  }

  // Retiene el envío de la cuenta durante una operación propia que cambia su revisión (enviar a
  // cocina). Los toques siguen acumulándose localmente.
  hold(accountID: string) {
    this.held.add(accountID);
    this.emit();
  }

  // Termina la retención. Con la cuenta que devolvió la operación propia, los toques acumulados
  // se aplican sobre ella sin tratarse como conflicto.
  release(accountID: string, bill?: Bill) {
    this.held.delete(accountID);
    const entry = this.entries.get(accountID);
    if (entry && bill && bill.account.id === accountID) {
      entry.base = bill.draft;
      entry.baseRevision = bill.account.revision;
      entry.latest = bill;
      entry.conflict = false;
    }
    this.emit();
    if (entry && entry.operations.length > 0) {
      this.schedule(entry);
    }
  }

  // Envía lo pendiente y espera a que la cuenta quede sincronizada. Falla de forma explícita si
  // hay un conflicto, una operación incierta o un rechazo que el operador debe revisar.
  async settle(requested: string): Promise<DraftView | null> {
    // Un rechazo previo se reintenta una sola vez; si el envío hecho aquí falla, se informa.
    let attempted = false;
    for (;;) {
      // Una cuenta pendiente puede crearse mientras se espera: se sigue a su identificador real.
      const accountID = this.resolve(requested);
      const entry = this.entries.get(accountID);
      if (!entry) {
        return null;
      }
      if (this.blockedCreation(entry)) {
        throw new DraftSyncError(
          'table',
          'Esta mesa ya tiene una cuenta. Elige si agregas tus productos a esa cuenta o abres una nueva.',
        );
      }
      if (entry.conflict) {
        throw new DraftSyncError(
          'conflict',
          'Otra terminal cambió la cuenta. Revisa los productos nuevos antes de continuar.',
        );
      }
      if (this.uncertainAccount) {
        throw new DraftSyncError(
          'uncertain',
          'Recupera el resultado del borrador pendiente antes de continuar.',
        );
      }
      if (this.inFlight) {
        await this.inFlight.promise;
        continue;
      }
      if (entry.operations.length === 0) {
        return this.view(accountID);
      }
      if (entry.error && attempted) {
        throw new DraftSyncError('error', entry.error);
      }
      attempted = true;
      this.clearTimer(entry);
      this.send(entry);
    }
  }
}
