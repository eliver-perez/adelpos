// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { PendingOperationsProvider } from '../hooks/pendingOperations';
import { realtimeLabels } from '../hooks/useRealtime';
import { useCashShortcut } from '../pages/cash/CashShortcut';
import { closingShift, currentWith, registersWith, shiftOf } from '../pages/cash/cashTestData';
import type { Shift } from '../services/payments';
import { Breadcrumb } from './Breadcrumb';
import { ServerStatus } from './ServerStatus';
import { serverState } from './serverState';
import type { ServerState } from './serverState';
import { TileScreen } from './TileScreen';
import { TopBar } from './TopBar';
import { UserMenu } from './UserMenu';
import {
  barHighlight,
  barSections,
  breadcrumbOf,
  resolveHome,
  sections,
  tilesOf,
} from './sections';
import type { ScreenID } from './sections';

// Fase A de la etapa 9C: registro de secciones, barra superior por permisos, indicador de Caja,
// estado del servidor, ruta de navegación, mosaicos de Administración e inicio por terminal. Solo
// se sustituye el transporte nativo; los permisos son los que entregaría Go.

const native = vi.hoisted(() => ({ invoke: vi.fn() }));
vi.mock('@tauri-apps/api/core', () => ({ invoke: native.invoke, isTauri: () => true }));

afterEach(() => {
  cleanup();
  native.invoke.mockReset();
});

// Conjuntos de permisos de ejemplo (los roles reales se configuran en Go).
const administrator = [
  'accounts.read',
  'restaurant.tables.read',
  'restaurant.areas.read',
  'takeaway.read',
  'kitchen.states.read',
  'cash.shifts.read',
  'cash.shifts.open',
  'cash.income.record',
  'payments.read',
  'catalog.read',
  'inventory.read',
  'reports.read',
  'documents.templates.read',
  'payments.methods.manage',
  'payments.settings.manage',
  'payments.tax.manage',
  'users.read',
  'roles.read',
  'sessions.read',
  'settings.read',
  'settings.manage',
  'devices.read',
  'devices.manage',
  'registers.read',
  'audit.read',
];
const cashier = [
  'accounts.read',
  'takeaway.read',
  'cash.shifts.read',
  'cash.shifts.open',
  'cash.income.record',
  'payments.read',
  'payments.collect',
];
const waiter = [
  'accounts.read',
  'restaurant.tables.read',
  'restaurant.areas.read',
  'takeaway.read',
  'orders.capture',
];

function serveShift(shift: Shift | null) {
  native.invoke.mockImplementation(async (command: string, args: { operation: string }) => {
    if (command === 'window_fullscreen_state') {
      return true;
    }
    if (command !== 'api_request') {
      return undefined;
    }
    if (args.operation === 'payments_current_shift') {
      return { status: 200, body: currentWith(shift) };
    }
    if (args.operation === 'payments_cash_registers') {
      return { status: 200, body: registersWith(shift) };
    }
    return { status: 200, body: {} };
  });
}

function BarHarness({
  permissions,
  highlight,
  server = 'online',
  onNavigate,
}: {
  permissions: string[];
  highlight: ScreenID | null;
  server?: ServerState;
  onNavigate: (next: ScreenID) => void;
}) {
  const can = (permission: string) => permissions.includes(permission);
  const shortcut = useCashShortcut({ active: true, can, onOpenCash: () => onNavigate('cash') });
  return (
    <TopBar
      permissions={permissions}
      highlight={highlight}
      home={resolveHome('tables', permissions)}
      onNavigate={onNavigate}
      cashShortcut={shortcut}
      can={can}
      server={server}
      userMenu={
        <UserMenu
          name="Ana"
          role="Cajera"
          onPreferences={() => onNavigate('preferences')}
          onPassword={() => onNavigate('password')}
          onLogout={() => undefined}
        />
      }
    />
  );
}

function renderBar(
  permissions: string[],
  options: { highlight?: ScreenID | null; server?: ServerState; shift?: Shift | null } = {},
) {
  serveShift(options.shift === undefined ? shiftOf() : options.shift);
  const onNavigate = vi.fn();
  render(
    <PendingOperationsProvider>
      <BarHarness
        permissions={permissions}
        highlight={options.highlight ?? null}
        server={options.server}
        onNavigate={onNavigate}
      />
    </PendingOperationsProvider>,
  );
  return onNavigate;
}

function barButtons() {
  const navigation = screen.getByRole('navigation', { name: 'Navegación principal' });
  return within(navigation)
    .getAllByRole('button')
    .map((button) => button.getAttribute('aria-label') ?? button.textContent);
}

describe('registro de secciones', () => {
  it('cada sección tiene un identificador único y su mosaico padre existe', () => {
    const ids = sections.map((entry) => entry.id);
    expect(new Set(ids).size).toBe(ids.length);
    for (const entry of sections) {
      if (entry.parent) {
        expect(ids).toContain(entry.parent);
      }
    }
    // Las cuatro secciones de la barra corresponden a la lista cerrada de Go.
    expect(sections.filter((entry) => entry.home).map((entry) => entry.home)).toEqual([
      'tables',
      'takeaway',
      'kitchen',
      'cash',
    ]);
  });

  it('«Auditoría» se llama «Registro de movimientos» y conserva su permiso', () => {
    const audit = sections.find((entry) => entry.id === 'audit');
    expect(audit?.label).toBe('Registro de movimientos');
    expect(audit?.permission).toBe('audit.read');
    expect(sections.some((entry) => entry.label === 'Auditoría')).toBe(false);
    expect(breadcrumbOf('audit').map((entry) => entry.label)).toEqual([
      'Administración',
      'Sistema',
      'Registro de movimientos',
    ]);
  });

  it('«Estado del sistema» no es sección de la barra ni mosaico: se abre desde Configuración', () => {
    expect(barSections(administrator).map((entry) => entry.id)).not.toContain('status');
    for (const parent of ['administration', 'staff', 'system', 'settings'] as ScreenID[]) {
      expect(tilesOf(parent, administrator).map((entry) => entry.id)).not.toContain('status');
    }
    expect(breadcrumbOf('status').map((entry) => entry.label)).toEqual([
      'Administración',
      'Sistema',
      'Configuración',
      'Estado del sistema',
    ]);
  });

  it('resalta Administración en sus pantallas y ninguna sección en las del usuario', () => {
    expect(barHighlight('devices')).toBe('administration');
    expect(barHighlight('status')).toBe('administration');
    expect(barHighlight('kitchen')).toBe('kitchen');
    expect(barHighlight('preferences')).toBeNull();
    expect(barHighlight('password')).toBeNull();
  });
});

describe('inicio por terminal', () => {
  it('abre en la pantalla de la terminal si el usuario puede verla; Mesas por omisión', () => {
    expect(resolveHome('tables', administrator)).toBe('restaurant');
    expect(resolveHome('takeaway', administrator)).toBe('takeaway');
    expect(resolveHome('kitchen', administrator)).toBe('kitchen');
    expect(resolveHome('cash', administrator)).toBe('cash');
    expect(resolveHome(undefined, administrator)).toBe('restaurant');
    expect(resolveHome('', administrator)).toBe('restaurant');
  });

  it('sin permiso para esa pantalla abre la primera de la barra que sí puede ver', () => {
    expect(resolveHome('kitchen', waiter)).toBe('restaurant');
    expect(resolveHome('cash', waiter)).toBe('restaurant');
    expect(resolveHome('kitchen', ['takeaway.read', 'accounts.read'])).toBe('restaurant');
    expect(resolveHome('kitchen', ['kitchen.states.read', 'cash.shifts.read'])).toBe('kitchen');
    expect(resolveHome('tables', ['cash.shifts.read'])).toBe('cash');
    // Sin secciones operativas: Administración; sin nada más: Mis preferencias.
    expect(resolveHome('tables', ['users.read'])).toBe('administration');
    expect(resolveHome('tables', [])).toBe('preferences');
  });

  it('nunca abre «Estado del sistema» ni una pantalla de Administración interna', () => {
    const homes = ['tables', 'takeaway', 'kitchen', 'cash', 'status', 'users', undefined];
    const roles = [administrator, cashier, waiter, ['users.read'], ['settings.read'], []];
    for (const home of homes) {
      for (const permissions of roles) {
        expect([
          'restaurant',
          'takeaway',
          'kitchen',
          'cash',
          'administration',
          'preferences',
        ]).toContain(resolveHome(home, permissions));
      }
    }
  });
});

describe('barra superior por permisos', () => {
  it('el administrador ve todas las secciones, Administración, el servidor, su menú y la pantalla completa', async () => {
    renderBar(administrator, { highlight: 'restaurant' });
    await screen.findByRole('button', { name: 'Caja · Turno abierto' });
    expect(barButtons()).toEqual([
      'Mesas',
      'Para llevar',
      'Cocina',
      'Caja · Turno abierto',
      'Administración',
    ]);
    expect(screen.getByRole('status', { name: 'Servidor: En línea' })).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Menú del usuario · Ana · Cajera' })).toBeTruthy();
    expect(await screen.findByRole('button', { name: 'Salir de pantalla completa' })).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Mesas' }).getAttribute('aria-current')).toBe('page');
    expect(screen.getByRole('button', { name: 'Cocina' }).getAttribute('aria-current')).toBeNull();
  });

  it('el cajero ve Mesas, Para llevar y Caja, sin Cocina ni Administración', async () => {
    renderBar(cashier);
    await screen.findByRole('button', { name: 'Caja · Turno abierto' });
    expect(barButtons()).toEqual(['Mesas', 'Para llevar', 'Caja · Turno abierto']);
    expect(screen.queryByRole('button', { name: 'Administración' })).toBeNull();
  });

  it('el mesero ve únicamente Mesas y Para llevar', () => {
    renderBar(waiter);
    expect(barButtons()).toEqual(['Mesas', 'Para llevar']);
    expect(screen.queryByRole('button', { name: /^Caja/ })).toBeNull();
    expect(screen.queryByRole('button', { name: 'Administración' })).toBeNull();
  });

  it('una sola opción de administración basta para ver el botón Administración', () => {
    renderBar([...waiter, 'registers.read']);
    expect(barButtons()).toEqual(['Mesas', 'Para llevar', 'Administración']);
  });

  it('cada sección navega por el requestScreen y el logotipo vuelve al inicio de la terminal', () => {
    const onNavigate = renderBar(administrator, { highlight: 'administration' });
    fireEvent.click(screen.getByRole('button', { name: 'Para llevar' }));
    fireEvent.click(screen.getByRole('button', { name: 'Cocina' }));
    fireEvent.click(screen.getByRole('button', { name: 'Administración' }));
    fireEvent.click(screen.getByRole('button', { name: 'AD-EL POS · Ir al inicio (Mesas)' }));
    expect(onNavigate.mock.calls.map(([next]) => next)).toEqual([
      'takeaway',
      'kitchen',
      'administration',
      'restaurant',
    ]);
  });

  it('con Caja sin el turno visible, el botón lleva directamente a la pantalla Caja', () => {
    const onNavigate = renderBar(['cash.shifts.read']);
    fireEvent.click(screen.getByRole('button', { name: 'Caja' }));
    expect(onNavigate).toHaveBeenCalledWith('cash');
  });

  it('el menú del usuario ofrece sus preferencias, la contraseña y cerrar sesión', () => {
    const onNavigate = renderBar(waiter);
    const button = screen.getByRole('button', { name: 'Menú del usuario · Ana · Cajera' });
    fireEvent.click(button);
    const menu = screen.getByRole('group', { name: 'Menú del usuario' });
    expect(
      within(menu)
        .getAllByRole('button')
        .map((entry) => entry.textContent),
    ).toEqual(['Mis preferencias', 'Cambiar contraseña', 'Cerrar sesión']);
    fireEvent.keyDown(document, { key: 'Escape' });
    expect(screen.queryByRole('group', { name: 'Menú del usuario' })).toBeNull();
    fireEvent.click(button);
    fireEvent.click(screen.getByRole('button', { name: 'Mis preferencias' }));
    expect(onNavigate).toHaveBeenCalledWith('preferences');
  });
});

describe('indicador de Caja en la barra', () => {
  async function stateOf(shift: Shift | null, label: string) {
    renderBar(cashier, { shift });
    const button = await screen.findByRole('button', { name: `Caja · ${label}` });
    const value = button.getAttribute('data-shift-state');
    cleanup();
    return value;
  }

  it('verde con el turno abierto, ámbar en arqueo y gris sin turno', async () => {
    expect(await stateOf(shiftOf(), 'Turno abierto')).toBe('open');
    expect(await stateOf(closingShift(), 'En arqueo')).toBe('closing');
    expect(await stateOf(null, 'Sin turno')).toBe('none');
  });

  it('conserva el menú rápido de la 7.1 con «Abrir turno» e «Ir a Caja»', async () => {
    const onNavigate = renderBar(cashier, { shift: null });
    const button = await screen.findByRole('button', { name: 'Caja · Sin turno' });
    expect(button.className).toContain('cash-shortcut-none');
    fireEvent.click(button);
    const menu = screen.getByRole('group', { name: 'Menú rápido de caja' });
    expect(within(menu).getByRole('button', { name: 'Abrir turno' })).toBeTruthy();
    fireEvent.click(within(menu).getByRole('button', { name: 'Ir a Caja' }));
    expect(onNavigate).toHaveBeenCalledWith('cash');
  });
});

describe('estado del servidor', () => {
  it('se deriva de la consulta de la sesión y del canal de eventos', () => {
    expect(serverState('', false, realtimeLabels.periodic)).toBe('online');
    expect(serverState('', true, realtimeLabels.connected)).toBe('online');
    expect(serverState('', true, realtimeLabels.periodic)).toBe('online');
    expect(serverState('', true, realtimeLabels.reconnecting)).toBe('reconnecting');
    expect(serverState('', true, realtimeLabels.interrupted)).toBe('reconnecting');
    expect(serverState('', true, realtimeLabels.unavailable)).toBe('reconnecting');
    expect(serverState('No se pudo conectar.', true, realtimeLabels.connected)).toBe('offline');
    expect(serverState('No se pudo conectar.', false, realtimeLabels.periodic)).toBe('offline');
  });

  it('en línea es un punto con descripción; si no, el aviso queda visible', () => {
    render(<ServerStatus state="online" />);
    const online = screen.getByRole('status', { name: 'Servidor: En línea' });
    expect(online.getAttribute('title')).toContain('El servidor responde');
    expect(online.className).toContain('server-status-online');
    cleanup();
    render(<ServerStatus state="reconnecting" />);
    expect(screen.getByRole('status', { name: 'Servidor: Reconectando' }).textContent).toBe(
      'Reconectando',
    );
    cleanup();
    render(<ServerStatus state="offline" />);
    const offline = screen.getByRole('status', { name: 'Servidor: Fuera de línea' });
    expect(offline.textContent).toBe('Fuera de línea');
    expect(offline.getAttribute('title')).toContain('Se reintenta automáticamente');
  });
});

describe('ruta de navegación', () => {
  it('«‹ Administración › Sistema › Terminales» con cada pantalla anterior tocable', () => {
    const onNavigate = vi.fn();
    render(<Breadcrumb screen="devices" tail={null} onNavigate={onNavigate} />);
    const route = screen.getByRole('navigation', { name: 'Ruta de navegación' });
    expect(route.textContent).toBe('Administración›Sistema›Terminales');
    expect(within(route).getByText('Terminales').getAttribute('aria-current')).toBe('page');
    fireEvent.click(within(route).getByRole('button', { name: 'Sistema' }));
    fireEvent.click(within(route).getByRole('button', { name: 'Administración' }));
    expect(onNavigate.mock.calls.map(([next]) => next)).toEqual(['system', 'administration']);
  });

  it('el último tramo es la pestaña que informa el módulo', () => {
    render(<Breadcrumb screen="catalog" tail="Productos" onNavigate={() => undefined} />);
    const route = screen.getByRole('navigation', { name: 'Ruta de navegación' });
    expect(route.textContent).toBe('Administración›Catálogo›Productos');
    expect(within(route).getByText('Productos').getAttribute('aria-current')).toBe('page');
    expect(within(route).getByText('Catálogo').getAttribute('aria-current')).toBeNull();
  });

  it('no aparece fuera de Administración ni en su pantalla principal', () => {
    for (const id of ['restaurant', 'kitchen', 'preferences', 'administration'] as ScreenID[]) {
      render(<Breadcrumb screen={id} tail={null} onNavigate={() => undefined} />);
      expect(screen.queryByRole('navigation', { name: 'Ruta de navegación' })).toBeNull();
      cleanup();
    }
  });
});

describe('mosaicos de Administración por permisos', () => {
  function tileCount() {
    return screen.getAllByRole('button').filter((button) => button.className === 'admin-tile')
      .length;
  }

  it('el administrador ve los siete mosaicos en su orden, con su descripción', () => {
    const onNavigate = vi.fn();
    render(
      <TileScreen parent="administration" permissions={administrator} onNavigate={onNavigate} />,
    );
    const names = tilesOf('administration', administrator).map((entry) => entry.label);
    expect(names).toEqual([
      'Catálogo',
      'Inventarios',
      'Reportes',
      'Documentos',
      'Cobros',
      'Personal',
      'Sistema',
    ]);
    for (const name of names) {
      expect(screen.getByRole('button', { name })).toBeTruthy();
    }
    const catalog = screen.getByRole('button', { name: 'Catálogo' });
    expect(catalog.getAttribute('aria-describedby')).toBeTruthy();
    expect(catalog.textContent).toContain('listas de precios');
    fireEvent.click(screen.getByRole('button', { name: 'Sistema' }));
    expect(onNavigate).toHaveBeenCalledWith('system');
    expect(tileCount()).toBe(7);
  });

  it('Personal y Sistema abren una segunda pantalla de mosaicos', () => {
    render(<TileScreen parent="system" permissions={administrator} onNavigate={() => undefined} />);
    expect(screen.getByRole('heading', { name: 'Sistema' })).toBeTruthy();
    expect(tilesOf('system', administrator).map((entry) => entry.label)).toEqual([
      'Configuración',
      'Terminales',
      'Cajas físicas',
      'Registro de movimientos',
    ]);
    expect(screen.getByRole('button', { name: 'Registro de movimientos' })).toBeTruthy();
    cleanup();
    render(<TileScreen parent="staff" permissions={['users.read']} onNavigate={() => undefined} />);
    expect(screen.getByRole('heading', { name: 'Personal' })).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Usuarios' })).toBeTruthy();
    expect(screen.queryByRole('button', { name: 'Roles' })).toBeNull();
    expect(screen.queryByRole('button', { name: 'Sesiones' })).toBeNull();
  });

  it('solo aparecen los mosaicos permitidos; Cobros exige permisos de administración de cobros', () => {
    expect(tilesOf('administration', ['cash.shifts.read']).map((entry) => entry.id)).toEqual([]);
    expect(tilesOf('administration', ['payments.tax.manage']).map((entry) => entry.id)).toEqual([
      'payments',
    ]);
    expect(
      tilesOf('administration', ['users.read', 'registers.read']).map((entry) => entry.id),
    ).toEqual(['staff', 'system']);
    expect(tilesOf('system', ['registers.read']).map((entry) => entry.id)).toEqual(['registers']);
    expect(tilesOf('administration', waiter)).toEqual([]);
  });
});

describe('pantalla completa en la barra', () => {
  it('un botón de icono que alterna entre reducir y maximizar con su descripción', async () => {
    renderBar(waiter);
    const exit = await screen.findByRole('button', { name: 'Salir de pantalla completa' });
    expect(exit.getAttribute('title')).toContain('Control + ⌘ + F');
    expect(exit.getAttribute('data-fullscreen')).toBe('true');
    expect(exit.textContent).toBe('');
    fireEvent.click(exit);
    await waitFor(() =>
      expect(native.invoke).toHaveBeenCalledWith('window_set_fullscreen', { fullscreen: false }),
    );
    const enter = await screen.findByRole('button', { name: 'Pantalla completa' });
    expect(enter.getAttribute('data-fullscreen')).toBe('false');
  });
});
