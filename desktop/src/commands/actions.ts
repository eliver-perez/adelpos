import { breadcrumbOf, canOpen, screenAuthorized, sections } from '../navigation/sections';
import type { ScreenID } from '../navigation/sections';
import type { CashSheet } from '../pages/cash/CashShortcut';
import type {
  OperationCommandHandlers,
  OperationCommandState,
  OperationTable,
} from './operationBridge';
import { precheckName } from '../services/documentNames';
import { fullscreenShortcut, modShortcut } from './platform';
import type { Shortcut } from './platform';

// Registro único de acciones (etapa 9C, fase C), sobre el registro de secciones de la fase A. El
// menú nativo, los atajos de teclado y la paleta de comandos leen esta misma lista, de modo que las
// tres vías siempre muestran las mismas opciones con los mismos permisos.
//
// Reglas:
// - Ejecutar una acción es llamar a la misma función que usa el botón de la interfaz (navegar con el
//   único requestScreen, abrir la hoja de Caja con su opción, «Cuenta sin mesa», «Nuevo pedido»,
//   «Precuenta», el tema de Mis preferencias, cerrar sesión…). Nunca hay un camino nuevo hacia el
//   servidor: Go sigue validando todo.
// - Ninguna acción ejecuta algo financiero o irreversible: las de Caja solo abren su hoja (que
//   registra con su confirmación de siempre) y «Imprimir precuenta» abre la precuenta de la cuenta.
// - La visibilidad repite las reglas de la interfaz (permisos y contexto); solo decide qué se
//   muestra.

export type ActionID =
  | 'file.table_account'
  | 'file.account_without_table'
  | 'file.takeaway_order'
  | 'file.print_precheck'
  | 'file.logout'
  | 'edit.find'
  | 'view.tables'
  | 'view.takeaway'
  | 'view.kitchen'
  | 'view.cash'
  | 'view.administration'
  | 'view.palette'
  | 'view.fullscreen'
  | 'view.theme'
  | 'cash.income'
  | 'cash.withdrawal'
  | 'cash.expense'
  | 'cash.partial'
  | 'cash.drawer'
  | 'cash.opening'
  | 'cash.open'
  | 'help.shortcuts'
  | 'help.about';

export type MenuGroup = 'file' | 'edit' | 'view' | 'cash' | 'help';

export const menuGroupLabels: Record<MenuGroup, string> = {
  file: 'Archivo',
  edit: 'Edición',
  view: 'Ver',
  cash: 'Caja',
  help: 'Ayuda',
};

// Opción del menú rápido de Caja con su estado (shortcutOptions de CashShortcut).
export interface CashOptionState {
  id: CashSheet | 'cash';
  disabled: boolean;
}

export interface CommandContext {
  authenticated: boolean;
  permissions: string[];
  screen: ScreenID | null;
  // Ventana de escritorio (pantalla completa nativa disponible).
  desktop: boolean;
  operation: OperationCommandState | null;
  cashOptions: CashOptionState[];
}

// Funciones de la interfaz que ejecutan las acciones (las mismas de sus botones).
export interface CommandRuntime {
  requestScreen: (next: ScreenID) => Promise<boolean>;
  operation: () => OperationCommandHandlers | null;
  openCashSheet: (sheet: CashSheet) => void;
  openPalette: (query?: string) => void;
  openShortcuts: () => void;
  openAbout: () => void;
  toggleFullscreen: () => Promise<void>;
  toggleTheme: () => Promise<void>;
  logout: () => Promise<void>;
  focusSearch: () => void;
  notify: (message: string) => void;
}

export interface ActionDefinition {
  id: ActionID;
  label: string;
  group: MenuGroup;
  // Sinónimos para la paleta («retiro», «corte», «productos»…).
  keywords: string[];
  shortcut?: Shortcut;
  // Qué hace, en la paleta y en «Atajos de teclado».
  description?: string;
  // Permisos: si no, la acción no aparece en la paleta y queda deshabilitada en el menú.
  visible: (context: CommandContext) => boolean;
  // Contexto (una cuenta abierta, un turno abierto…): si no, se muestra deshabilitada con `hint`.
  enabled?: (context: CommandContext) => boolean;
  hint?: string;
  run: (runtime: CommandRuntime, context: CommandContext) => unknown;
}

function has(context: CommandContext, permission: string) {
  return context.permissions.includes(permission);
}

function allowed(context: CommandContext, screen: ScreenID) {
  return context.authenticated && canOpen(screen, context.permissions);
}

function operationVisible(context: CommandContext) {
  return context.screen === 'restaurant' || context.screen === 'takeaway';
}

// Abrir otra cuenta o elegir otra mesa: no durante una operación en curso ni con el cobro abierto
// (la interfaz no ofrece esos botones en el cobro y cerrarlo perdería lo tecleado; revisión 9C).
function freeForAnotherAccount(context: CommandContext) {
  return !context.operation?.blocked && !context.operation?.checkoutOpen;
}

const anotherAccountHint =
  'Disponible sin un cobro abierto y cuando termine la operación en curso.';

// Opción del menú rápido de Caja: visible con su permiso (las calcula shortcutOptions) y
// habilitada como en el menú rápido (ingreso, retiro, gasto y corte parcial exigen turno abierto).
function cashAction(
  id: CashSheet,
  label: string,
  keywords: string[],
  description: string,
): ActionDefinition {
  return {
    id: `cash.${id}` as ActionID,
    label,
    group: 'cash',
    keywords,
    description,
    visible: (context) =>
      context.authenticated && context.cashOptions.some((option) => option.id === id),
    enabled: (context) =>
      context.cashOptions.some((option) => option.id === id && !option.disabled),
    hint: 'Requiere un turno abierto en la caja de esta terminal.',
    run: (runtime) => runtime.openCashSheet(id),
  };
}

// Mesas o Para llevar: dentro de la captura de esa sección equivale a «‹ Mesas» / «‹ Para llevar»
// de la barra reducida; si no, al botón de la barra superior.
function sectionRun(screen: 'restaurant' | 'takeaway') {
  return (runtime: CommandRuntime, context: CommandContext) => {
    const operation = context.operation;
    const sameSection = operation?.view === 'takeaway' ? 'takeaway' : 'restaurant';
    if (
      operationVisible(context) &&
      operation?.captureMode &&
      !operation.checkoutOpen &&
      sameSection === screen
    ) {
      runtime.operation()?.leaveCapture();
      return undefined;
    }
    return runtime.requestScreen(screen);
  };
}

export const actions: ActionDefinition[] = [
  {
    id: 'file.table_account',
    label: 'Nueva cuenta en mesa',
    group: 'file',
    keywords: ['cuenta', 'mesa', 'nueva', 'abrir', 'capturar'],
    shortcut: modShortcut('n'),
    description:
      'Captura una cuenta nueva en la mesa elegida; sin mesa elegida, abre el mapa para elegirla.',
    visible: (context) =>
      allowed(context, 'restaurant') &&
      has(context, 'accounts.open') &&
      has(context, 'restaurant.tables.read') &&
      has(context, 'restaurant.areas.read') &&
      context.operation !== null,
    enabled: freeForAnotherAccount,
    hint: anotherAccountHint,
    run: async (runtime, context) => {
      // La misma función que «Capturar» o «Nueva cuenta» de la mesa: entra a la captura sin crear
      // la cuenta (Go la crea con el primer producto).
      if (operationVisible(context) && context.operation?.tableForNewAccount) {
        runtime.operation()?.newTableAccount();
        return;
      }
      if (await runtime.requestScreen('restaurant')) {
        runtime.notify('Elige la mesa en el mapa para abrir la cuenta.');
      }
    },
  },
  {
    id: 'file.account_without_table',
    label: 'Cuenta sin mesa',
    group: 'file',
    keywords: ['cuenta', 'sin mesa', 'barra', 'nueva'],
    // Etapa 9D (§4.1): entra a la captura sin crear nada; la cuenta nace con el primer producto.
    description:
      'Captura una cuenta sin mesa, como «Cuenta sin mesa» en Mesas → Cuentas. Se abre al guardar el primer producto.',
    visible: (context) =>
      allowed(context, 'restaurant') && has(context, 'accounts.open') && context.operation !== null,
    enabled: freeForAnotherAccount,
    hint: anotherAccountHint,
    run: async (runtime) => {
      if (await runtime.requestScreen('restaurant')) {
        runtime.operation()?.accountWithoutTable();
      }
    },
  },
  {
    id: 'file.takeaway_order',
    label: 'Nuevo pedido para llevar',
    group: 'file',
    keywords: ['pedido', 'para llevar', 'nuevo', 'ventanilla'],
    description:
      'Captura un pedido para llevar, como «Nuevo pedido» en Para llevar. Se abre al guardar el primer producto.',
    visible: (context) =>
      allowed(context, 'takeaway') &&
      has(context, 'accounts.open') &&
      has(context, 'takeaway.manage') &&
      context.operation !== null,
    enabled: freeForAnotherAccount,
    hint: anotherAccountHint,
    run: async (runtime) => {
      if (await runtime.requestScreen('takeaway')) {
        runtime.operation()?.newTakeaway();
      }
    },
  },
  {
    id: 'file.print_precheck',
    // Etapa 9F (§4.5): con el nombre visible que eligió el restaurante («Imprimir ticket de venta»);
    // el menú nativo recibe el mismo nombre por menu_set_state.
    get label() {
      return `Imprimir ${precheckName().lower}`;
    },
    group: 'file',
    get keywords() {
      return ['precuenta', 'imprimir', 'cuenta', 'ticket de venta', precheckName().lower];
    },
    shortcut: modShortcut('p'),
    get description() {
      return `Abre ${precheckName().withArticle} de la cuenta a la vista para revisarla e imprimirla.`;
    },
    visible: (context) =>
      context.authenticated && has(context, 'payments.read') && context.operation !== null,
    enabled: (context) =>
      operationVisible(context) && !!context.operation?.active && !!context.operation.precheckReady,
    hint: 'Elige una cuenta con productos enviados a cocina.',
    run: (runtime) => runtime.operation()?.openPrecheck(),
  },
  {
    id: 'file.logout',
    label: 'Cerrar sesión',
    group: 'file',
    keywords: ['cerrar sesión', 'salir', 'usuario'],
    visible: (context) => context.authenticated,
    run: (runtime) => runtime.logout(),
  },
  {
    id: 'edit.find',
    label: 'Buscar en la pantalla',
    group: 'edit',
    keywords: ['buscar', 'búsqueda', 'filtrar'],
    shortcut: modShortcut('f'),
    description:
      'Lleva al buscador de la pantalla (productos en la captura, cuentas, tablas); en el mapa busca mesas.',
    visible: (context) => context.authenticated,
    run: (runtime) => runtime.focusSearch(),
  },
  {
    id: 'view.tables',
    label: 'Mesas',
    group: 'view',
    keywords: ['mesas', 'mapa', 'salón', 'restaurante'],
    shortcut: modShortcut('1'),
    visible: (context) => allowed(context, 'restaurant'),
    run: sectionRun('restaurant'),
  },
  {
    id: 'view.takeaway',
    label: 'Para llevar',
    group: 'view',
    keywords: ['para llevar', 'pedidos', 'ventanilla'],
    shortcut: modShortcut('2'),
    visible: (context) => allowed(context, 'takeaway'),
    run: sectionRun('takeaway'),
  },
  {
    id: 'view.kitchen',
    label: 'Cocina',
    group: 'view',
    keywords: ['cocina', 'comandas', 'preparación', 'cola de impresión'],
    shortcut: modShortcut('3'),
    visible: (context) => allowed(context, 'kitchen'),
    run: (runtime) => runtime.requestScreen('kitchen'),
  },
  {
    id: 'view.cash',
    label: 'Caja',
    group: 'view',
    keywords: ['caja', 'turno', 'efectivo', 'arqueo', 'corte', 'cierre'],
    shortcut: modShortcut('4'),
    description: 'Abre la pantalla de Caja (el botón Caja de la barra abre su menú rápido).',
    visible: (context) => allowed(context, 'cash'),
    run: (runtime) => runtime.requestScreen('cash'),
  },
  {
    id: 'view.administration',
    label: 'Administración',
    group: 'view',
    keywords: ['administración', 'mosaicos', 'configuración'],
    shortcut: modShortcut('5'),
    visible: (context) => allowed(context, 'administration'),
    run: (runtime) => runtime.requestScreen('administration'),
  },
  {
    id: 'view.palette',
    label: 'Paleta de comandos',
    group: 'view',
    keywords: ['paleta', 'comandos', 'buscar', 'ir a'],
    shortcut: modShortcut('k'),
    description: 'Busca pantallas, acciones y mesas por su nombre.',
    visible: (context) => context.authenticated,
    run: (runtime) => runtime.openPalette(),
  },
  {
    id: 'view.fullscreen',
    label: 'Pantalla completa',
    group: 'view',
    keywords: ['pantalla completa', 'ventana', 'maximizar'],
    shortcut: fullscreenShortcut,
    description: 'Entra o sale de la pantalla completa, como el botón de la barra.',
    visible: (context) => context.desktop,
    run: (runtime) => runtime.toggleFullscreen(),
  },
  {
    id: 'view.theme',
    label: 'Tema claro u oscuro',
    group: 'view',
    keywords: ['tema', 'oscuro', 'claro', 'apariencia', 'modo noche'],
    description: 'Cambia tu tema y lo guarda en Mis preferencias.',
    visible: (context) => context.authenticated,
    run: (runtime) => runtime.toggleTheme(),
  },
  cashAction(
    'income',
    'Ingreso',
    ['ingreso', 'entrada de efectivo', 'efectivo'],
    'Abre «Ingreso de efectivo» de Caja.',
  ),
  cashAction(
    'withdrawal',
    'Retiro',
    ['retiro', 'salida de efectivo', 'efectivo'],
    'Abre «Retiro de efectivo» de Caja.',
  ),
  cashAction(
    'expense',
    'Gasto',
    ['gasto', 'pago', 'salida de efectivo'],
    'Abre «Gasto pagado desde caja».',
  ),
  cashAction(
    'partial',
    'Corte parcial',
    ['corte parcial', 'corte x', 'corte'],
    'Abre el corte parcial del turno.',
  ),
  cashAction('drawer', 'Abrir cajón', ['abrir cajón', 'cajón'], 'Abre «Abrir cajón» de Caja.'),
  cashAction(
    'opening',
    'Abrir turno',
    ['abrir turno', 'turno', 'fondo inicial'],
    'Abre la apertura del turno de la caja de esta terminal.',
  ),
  {
    id: 'cash.open',
    label: 'Ir a Caja',
    group: 'cash',
    keywords: ['caja', 'turno', 'cierre', 'arqueo'],
    visible: (context) => allowed(context, 'cash'),
    run: (runtime) => runtime.requestScreen('cash'),
  },
  {
    id: 'help.shortcuts',
    label: 'Atajos de teclado',
    group: 'help',
    keywords: ['atajos', 'teclado', 'ayuda', 'teclas'],
    visible: () => true,
    run: (runtime) => runtime.openShortcuts(),
  },
  {
    id: 'help.about',
    label: 'Acerca de AD-EL POS',
    group: 'help',
    keywords: ['acerca de', 'versión', 'ayuda'],
    visible: () => true,
    run: (runtime) => runtime.openAbout(),
  },
];

export function actionByID(id: string): ActionDefinition | undefined {
  return actions.find((action) => action.id === id);
}

export function actionEnabled(action: ActionDefinition, context: CommandContext) {
  return action.visible(context) && (action.enabled ? action.enabled(context) : true);
}

// Opciones del menú nativo habilitadas en el contexto (las demás se deshabilitan).
export function enabledMenuActions(context: CommandContext): ActionID[] {
  return actions.filter((action) => actionEnabled(action, context)).map((action) => action.id);
}

// Pantallas que la paleta ofrece además de las acciones (las secciones de la barra ya están como
// acciones de «Ver»). Los sinónimos ayudan a encontrar la pantalla por lo que contiene.
const screenKeywords: Partial<Record<ScreenID, string[]>> = {
  catalog: [
    'productos',
    'categorías',
    'combos',
    'complementos',
    'porciones',
    'precios',
    'promociones',
    'menú',
  ],
  inventory: [
    'insumos',
    'existencias',
    'recetas',
    'almacenes',
    'compras',
    'proveedores',
    'conteos',
    'mermas',
  ],
  reports: ['ventas', 'reportes', 'resumen', 'estadísticas', 'exportar'],
  documents: ['tickets', 'plantillas', 'logotipo', 'identidad', 'diseñador'],
  payments: ['métodos de pago', 'iva', 'descuentos', 'motivos', 'políticas de cobro'],
  staff: ['personal', 'empleados'],
  users: ['usuarios', 'empleados', 'meseros', 'cajeros'],
  roles: ['roles', 'permisos'],
  sessions: ['sesiones'],
  system: ['sistema'],
  settings: ['configuración', 'establecimiento', 'hora de inicio del día', 'funciones'],
  devices: ['terminales', 'equipos', 'pantalla inicial'],
  registers: ['cajas físicas'],
  audit: ['registro de movimientos', 'auditoría', 'bitácora', 'historial'],
  status: ['estado del sistema', 'diagnóstico', 'servidor'],
  preferences: ['mis preferencias', 'tema', 'apariencia'],
  password: ['contraseña', 'clave'],
};

const coveredByActions: ScreenID[] = [
  'restaurant',
  'takeaway',
  'kitchen',
  'cash',
  'administration',
];

// Entrada de la paleta: una acción del registro, una pantalla o una mesa.
export interface CommandEntry {
  id: string;
  label: string;
  // Etiqueta visible del tipo («Ver», «Caja», «Pantalla», «Mesa»…).
  kind: string;
  description?: string;
  keywords: string[];
  shortcut?: Shortcut;
  enabled: boolean;
  hint?: string;
  run: () => unknown;
}

export function commandEntries(
  context: CommandContext,
  runtime: CommandRuntime,
  tables: OperationTable[],
): CommandEntry[] {
  const entries: CommandEntry[] = [];
  for (const action of actions) {
    if (!action.visible(context)) {
      continue;
    }
    entries.push({
      id: action.id,
      label: action.label,
      kind: menuGroupLabels[action.group],
      description: action.description,
      keywords: action.keywords,
      shortcut: action.shortcut,
      enabled: actionEnabled(action, context),
      hint: action.hint,
      run: () => action.run(runtime, context),
    });
  }
  if (!context.authenticated) {
    return entries;
  }
  for (const entry of sections) {
    if (coveredByActions.includes(entry.id) || !screenAuthorized(entry, context.permissions)) {
      continue;
    }
    const route = breadcrumbOf(entry.id)
      .slice(0, -1)
      .map((part) => part.label)
      .join(' › ');
    entries.push({
      id: `go.${entry.id}`,
      label: entry.label,
      kind: 'Pantalla',
      description: route || entry.description,
      keywords: screenKeywords[entry.id] ?? [],
      enabled: true,
      run: () => runtime.requestScreen(entry.id),
    });
  }
  // Mesas del mapa: elegir una es tocarla en el mapa (abre su panel lateral).
  if (allowed(context, 'restaurant') && has(context, 'restaurant.tables.read')) {
    for (const table of tables) {
      entries.push({
        id: `table.${table.id}`,
        label: table.name,
        kind: 'Mesa',
        description: table.areaName,
        keywords: ['mesa', table.areaName],
        enabled: !context.operation?.checkoutOpen,
        hint: 'Cierra el cobro antes de elegir otra mesa.',
        run: async () => {
          if (await runtime.requestScreen('restaurant')) {
            runtime.operation()?.chooseTable(table.id);
          }
        },
      });
    }
  }
  return entries;
}
