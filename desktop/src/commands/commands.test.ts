import { readFileSync } from 'node:fs';
import { describe, expect, it, vi } from 'vitest';
import { actionByID, actions, commandEntries, enabledMenuActions } from './actions';
import type { CommandContext, CommandRuntime } from './actions';
import type { ScreenID } from '../navigation/sections';
import type { OperationCommandHandlers } from './operationBridge';
import { createDispatcher, twinWindowMs } from './dispatcher';
import { ariaShortcut, matchesShortcut, shortcutLabel } from './platform';
import type { KeyCombo, Shortcut } from './platform';
import { searchEntries } from './search';

// Etapa 9C, fase C: registro de acciones, despachador, atajos y paleta (sin DOM).

const adminPermissions = [
  'accounts.read',
  'accounts.open',
  'restaurant.tables.read',
  'restaurant.areas.read',
  'takeaway.read',
  'takeaway.manage',
  'kitchen.states.read',
  'cash.shifts.read',
  'payments.read',
  'catalog.read',
  'inventory.read',
  'reports.read',
  'users.read',
  'settings.read',
  'devices.read',
  'audit.read',
];

// Mesero: mesas y cuentas, sin caja ni administración.
const waiterPermissions = [
  'accounts.read',
  'accounts.open',
  'restaurant.tables.read',
  'restaurant.areas.read',
  'takeaway.read',
];

function context(overrides: Partial<CommandContext> = {}): CommandContext {
  return {
    authenticated: true,
    permissions: adminPermissions,
    screen: 'restaurant',
    desktop: true,
    operation: {
      active: true,
      view: 'tables',
      captureMode: false,
      checkoutOpen: false,
      blocked: false,
      tableForNewAccount: null,
      precheckReady: false,
    },
    cashOptions: [
      { id: 'income', disabled: false },
      { id: 'withdrawal', disabled: false },
      { id: 'expense', disabled: false },
      { id: 'partial', disabled: false },
      { id: 'cash', disabled: false },
    ],
    ...overrides,
  };
}

function runtime() {
  const tools = {
    requestScreen: vi.fn(async (_next: ScreenID) => true),
    operation: vi.fn((): OperationCommandHandlers | null => null),
    openCashSheet: vi.fn(),
    openPalette: vi.fn(),
    openShortcuts: vi.fn(),
    openAbout: vi.fn(),
    toggleFullscreen: vi.fn(async () => undefined),
    toggleTheme: vi.fn(async () => undefined),
    logout: vi.fn(async () => undefined),
    focusSearch: vi.fn(),
    notify: vi.fn(),
  };
  const typed: CommandRuntime = tools;
  return Object.assign(typed, tools);
}

function key(event: Partial<KeyboardEvent>) {
  return {
    key: '',
    code: '',
    metaKey: false,
    ctrlKey: false,
    altKey: false,
    shiftKey: false,
    ...event,
  } as KeyboardEvent;
}

describe('despachador sin doble ejecución', () => {
  it('descarta una sola vez la gemela que llega por la otra vía (menú + teclado)', () => {
    let time = 1000;
    const dispatcher = createDispatcher(() => time);
    const run = vi.fn();
    expect(dispatcher.dispatch('view.tables', 'keyboard', run)).toBe(true);
    // El evento del menú nativo del mismo acelerador llega después: se descarta.
    time += 30;
    expect(dispatcher.dispatch('view.tables', 'menu', run)).toBe(false);
    expect(run).toHaveBeenCalledTimes(1);
    // Una segunda pulsación es otra ejecución (y su gemela del menú también se descarta).
    time += 200;
    expect(dispatcher.dispatch('view.tables', 'keyboard', run)).toBe(true);
    time += 10;
    expect(dispatcher.dispatch('view.tables', 'menu', run)).toBe(false);
    expect(run).toHaveBeenCalledTimes(2);
    // Fuera de la ventana de gemelas, cada vía ejecuta.
    time += twinWindowMs + 1;
    expect(dispatcher.dispatch('view.tables', 'menu', run)).toBe(true);
    expect(run).toHaveBeenCalledTimes(3);
    // Otra acción no se ve afectada.
    expect(dispatcher.dispatch('view.kitchen', 'keyboard', run)).toBe(true);
  });

  it('el menú primero también descarta la pulsación gemela', () => {
    let time = 0;
    const dispatcher = createDispatcher(() => time);
    const run = vi.fn();
    expect(dispatcher.dispatch('view.palette', 'menu', run)).toBe(true);
    time += 5;
    expect(dispatcher.dispatch('view.palette', 'keyboard', run)).toBe(false);
    expect(run).toHaveBeenCalledTimes(1);
  });

  it('una pulsación que no se ejecutó (mientras se escribe) consume su gemela del menú', () => {
    let time = 0;
    const dispatcher = createDispatcher(() => time);
    const run = vi.fn();
    dispatcher.suppress('view.tables', 'keyboard');
    time += 20;
    expect(dispatcher.dispatch('view.tables', 'menu', run)).toBe(false);
    expect(run).not.toHaveBeenCalled();
  });

  it('una acción asíncrona en curso no vuelve a ejecutarse', async () => {
    let release: () => void = () => undefined;
    const dispatcher = createDispatcher();
    const run = vi.fn(
      () =>
        new Promise<void>((resolve) => {
          release = resolve;
        }),
    );
    expect(dispatcher.dispatch('view.theme', 'palette', run)).toBe(true);
    expect(dispatcher.running('view.theme')).toBe(true);
    expect(dispatcher.dispatch('view.theme', 'palette', run)).toBe(false);
    release();
    await vi.waitFor(() => expect(dispatcher.running('view.theme')).toBe(false));
    expect(dispatcher.dispatch('view.theme', 'palette', run)).toBe(true);
    expect(run).toHaveBeenCalledTimes(2);
  });
});

describe('atajos por plataforma', () => {
  const tables = actionByID('view.tables')!.shortcut!;
  const fullscreen = actionByID('view.fullscreen')!.shortcut!;

  it('usan ⌘ en macOS y Ctrl en Windows, con el atajo estándar de pantalla completa', () => {
    expect(shortcutLabel(tables, 'mac')).toBe('⌘1');
    expect(shortcutLabel(tables, 'other')).toBe('Ctrl+1');
    expect(ariaShortcut(tables, 'mac')).toBe('Meta+1');
    expect(ariaShortcut(tables, 'other')).toBe('Control+1');
    expect(shortcutLabel(fullscreen, 'mac')).toBe('⌃⌘F');
    expect(shortcutLabel(fullscreen, 'other')).toBe('F11');
    expect(ariaShortcut(fullscreen, 'mac')).toBe('Control+Meta+F');
    expect(matchesShortcut(key({ key: '1', metaKey: true }), tables, 'mac')).toBe(true);
    expect(matchesShortcut(key({ key: '1', ctrlKey: true }), tables, 'mac')).toBe(false);
    expect(matchesShortcut(key({ key: '1', ctrlKey: true }), tables, 'other')).toBe(true);
    expect(matchesShortcut(key({ key: '1', metaKey: true }), tables, 'other')).toBe(false);
    // Con otra distribución, la tecla física del dígito.
    expect(matchesShortcut(key({ key: '&', code: 'Digit1', ctrlKey: true }), tables, 'other')).toBe(
      true,
    );
    // Mayús o Alt cambian el atajo.
    expect(matchesShortcut(key({ key: '1', ctrlKey: true, shiftKey: true }), tables, 'other')).toBe(
      false,
    );
    expect(
      matchesShortcut(key({ key: 'f', ctrlKey: true, metaKey: true }), fullscreen, 'mac'),
    ).toBe(true);
    expect(matchesShortcut(key({ key: 'F11' }), fullscreen, 'other')).toBe(true);
    expect(
      matchesShortcut(
        key({ key: 'F', ctrlKey: true }),
        actionByID('edit.find')!.shortcut!,
        'other',
      ),
    ).toBe(true);
  });

  it('cada atajo es único y las acciones financieras no tienen atajo', () => {
    for (const platform of ['mac', 'other'] as const) {
      const labels = actions
        .filter((action) => action.shortcut)
        .map((action) => shortcutLabel(action.shortcut!, platform));
      expect(new Set(labels).size).toBe(labels.length);
    }
    for (const id of [
      'cash.income',
      'cash.withdrawal',
      'cash.expense',
      'cash.partial',
      'cash.drawer',
      'cash.opening',
      'file.account_without_table',
      'file.takeaway_order',
      'file.logout',
    ]) {
      expect(actionByID(id)?.shortcut, id).toBeUndefined();
    }
  });
});

describe('registro de acciones', () => {
  it('ninguna acción de caja registra nada: solo abre su hoja, como el menú rápido', () => {
    const tools = runtime();
    for (const id of ['income', 'withdrawal', 'expense', 'partial'] as const) {
      void actionByID(`cash.${id}`)!.run(tools, context());
      expect(tools.openCashSheet).toHaveBeenLastCalledWith(id);
    }
    expect(tools.requestScreen).not.toHaveBeenCalled();
    expect(tools.operation).not.toHaveBeenCalled();
  });

  it('el menú habilita según permisos y contexto', () => {
    const admin = enabledMenuActions(context());
    expect(admin).toEqual(
      expect.arrayContaining([
        'view.tables',
        'view.takeaway',
        'view.kitchen',
        'view.cash',
        'view.administration',
        'cash.income',
        'cash.withdrawal',
        'cash.open',
        'file.table_account',
        'file.account_without_table',
        'file.takeaway_order',
        'view.theme',
        'help.about',
      ]),
    );
    // Sin cuenta con productos enviados, «Imprimir precuenta» queda deshabilitada.
    expect(admin).not.toContain('file.print_precheck');
    expect(
      enabledMenuActions(
        context({
          operation: { ...context().operation!, precheckReady: true },
        }),
      ),
    ).toContain('file.print_precheck');
    // Sin turno abierto, ingreso y retiro se deshabilitan; «Abrir cajón» sin permiso no existe.
    const noShift = enabledMenuActions(
      context({
        cashOptions: [
          { id: 'income', disabled: true },
          { id: 'withdrawal', disabled: true },
          { id: 'opening', disabled: false },
          { id: 'cash', disabled: false },
        ],
      }),
    );
    expect(noShift).not.toContain('cash.income');
    expect(noShift).not.toContain('cash.drawer');
    expect(noShift).toContain('cash.opening');
    // Con una operación en curso, las acciones que abren cuentas se deshabilitan.
    const busy = enabledMenuActions(
      context({ operation: { ...context().operation!, blocked: true } }),
    );
    expect(busy).not.toContain('file.account_without_table');
    expect(busy).not.toContain('file.table_account');
  });

  it('el mesero no ve Caja ni Administración; antes de iniciar sesión solo Ayuda y la ventana', () => {
    const waiter = enabledMenuActions(context({ permissions: waiterPermissions, cashOptions: [] }));
    expect(waiter).toEqual(
      expect.arrayContaining(['view.tables', 'view.takeaway', 'file.table_account']),
    );
    for (const id of [
      'view.kitchen',
      'view.cash',
      'view.administration',
      'cash.open',
      'file.takeaway_order',
    ]) {
      expect(waiter).not.toContain(id);
    }
    expect(
      enabledMenuActions(
        context({ authenticated: false, permissions: [], screen: null, operation: null }),
      ).sort(),
    ).toEqual(['help.about', 'help.shortcuts', 'view.fullscreen']);
  });

  it('Mesas dentro de la captura de una mesa equivale a «‹ Mesas»', () => {
    const tools = runtime();
    const leaveCapture = vi.fn();
    tools.operation.mockReturnValue({ leaveCapture } as never);
    void actionByID('view.tables')!.run(
      tools,
      context({ operation: { ...context().operation!, captureMode: true } }),
    );
    expect(leaveCapture).toHaveBeenCalled();
    expect(tools.requestScreen).not.toHaveBeenCalled();
    // En el cobro solo navega (no lo cierra).
    void actionByID('view.tables')!.run(
      tools,
      context({ operation: { ...context().operation!, captureMode: true, checkoutOpen: true } }),
    );
    expect(tools.requestScreen).toHaveBeenCalledWith('restaurant');
    expect(leaveCapture).toHaveBeenCalledTimes(1);
  });

  it('«Nueva cuenta en mesa» sin mesa elegida abre el mapa y lo explica', async () => {
    const tools = runtime();
    await actionByID('file.table_account')!.run(tools, context({ screen: 'kitchen' }));
    expect(tools.requestScreen).toHaveBeenCalledWith('restaurant');
    expect(tools.notify).toHaveBeenCalledWith('Elige la mesa en el mapa para abrir la cuenta.');
    const newTableAccount = vi.fn();
    tools.operation.mockReturnValue({ newTableAccount } as never);
    await actionByID('file.table_account')!.run(
      tools,
      context({ operation: { ...context().operation!, tableForNewAccount: 'mesa-1' } }),
    );
    expect(newTableAccount).toHaveBeenCalled();
  });
});

describe('paleta de comandos', () => {
  const tables = [
    { id: '5', name: 'Mesa 5', areaName: 'Terraza' },
    { id: '15', name: 'Mesa 15', areaName: 'Salón' },
  ];

  function labels(query: string, value = context()) {
    return searchEntries(commandEntries(value, runtime(), tables), query).map(
      (entry) => entry.label,
    );
  }

  it('busca por nombre y sinónimos sin distinguir acentos', () => {
    expect(labels('productos')[0]).toBe('Catálogo');
    expect(labels('retiro')[0]).toBe('Retiro');
    expect(labels('corte')[0]).toBe('Corte parcial');
    expect(labels('corte')).toContain('Caja');
    expect(labels('mesa 5')).toContain('Mesa 5');
    expect(labels('mesa 5')).not.toContain('Mesa 15');
    expect(labels('terraza')).toContain('Mesa 5');
    expect(labels('configuracion')).toContain('Configuración');
    expect(labels('auditoria')).toContain('Registro de movimientos');
    expect(labels('atajos')[0]).toBe('Atajos de teclado');
    expect(labels('zzz')).toEqual([]);
    // Sin texto no se listan las mesas.
    expect(labels('')).not.toContain('Mesa 5');
  });

  it('se filtra por permisos y contexto', () => {
    const waiter = context({ permissions: waiterPermissions, cashOptions: [] });
    expect(labels('', waiter)).not.toContain('Catálogo');
    expect(labels('retiro', waiter)).toEqual([]);
    expect(labels('caja', waiter)).not.toContain('Ir a Caja');
    const entries = commandEntries(context(), runtime(), tables);
    const precheck = entries.find((entry) => entry.id === 'file.print_precheck');
    expect(precheck?.enabled).toBe(false);
    expect(precheck?.hint).toBe('Elige una cuenta con productos enviados a cocina.');
    // Sin permiso de leer mesas, la paleta no ofrece mesas.
    const blind = context({
      permissions: adminPermissions.filter((permission) => permission !== 'restaurant.tables.read'),
    });
    expect(labels('mesa 5', blind)).not.toContain('Mesa 5');
  });

  it('elegir una mesa navega con requestScreen y la elige como en el mapa', async () => {
    const tools = runtime();
    const chooseTable = vi.fn();
    tools.operation.mockReturnValue({ chooseTable } as never);
    const entry = commandEntries(context({ screen: 'kitchen' }), tools, tables).find(
      (candidate) => candidate.id === 'table.5',
    );
    await entry!.run();
    expect(tools.requestScreen).toHaveBeenCalledWith('restaurant');
    expect(chooseTable).toHaveBeenCalledWith('5');
  });
});

// Contrato con el menú nativo de Rust: la lista cerrada MENU_ACTIONS de menu.rs tiene las mismas
// acciones, textos y aceleradores que este registro.
describe('contrato con src-tauri/src/menu.rs', () => {
  const source = readFileSync(new URL('../../src-tauri/src/menu.rs', import.meta.url), 'utf8');
  const block = source.slice(
    source.indexOf('pub const MENU_ACTIONS'),
    source.indexOf('];', source.indexOf('pub const MENU_ACTIONS')),
  );
  const rust = [
    ...block.matchAll(
      /MenuAction\s*\{\s*id:\s*"([a-z_.]+)",\s*label:\s*"([^"]+)",\s*mac:\s*(None|Some\("([^"]+)"\)),\s*other:\s*(None|Some\("([^"]+)"\)),?\s*\}/g,
    ),
  ].map((match) => ({ id: match[1]!, label: match[2]!, mac: match[4], other: match[6] }));

  function combo(accelerator: string | undefined): KeyCombo | undefined {
    if (!accelerator) {
      return undefined;
    }
    const parts = accelerator.split('+');
    const keyName = parts.pop()!;
    return {
      key: keyName.length === 1 ? keyName.toLowerCase() : keyName,
      ...(parts.includes('CmdOrCtrl') || parts.includes('Super') ? { mod: true } : {}),
      ...(parts.includes('Ctrl') ? { ctrl: true } : {}),
    };
  }

  it('tiene las mismas opciones, textos y atajos', () => {
    expect(rust.length).toBeGreaterThan(20);
    expect(rust.map((entry) => entry.id)).toEqual(actions.map((action) => action.id));
    for (const entry of rust) {
      const action = actionByID(entry.id)!;
      expect(entry.label, entry.id).toBe(action.label);
      const shortcut: Shortcut | undefined = action.shortcut;
      expect(combo(entry.mac), entry.id).toEqual(shortcut?.mac);
      expect(combo(entry.other), entry.id).toEqual(shortcut?.other);
    }
  });
});
