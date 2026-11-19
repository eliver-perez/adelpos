// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { PermissionCatalog } from '../services/permissionCatalog';
import {
  capabilityState,
  matchesSearch,
  toggleCapability,
  withoutPermissions,
  withRequirements,
} from '../services/permissionCatalog';
import {
  permissionGroupsCapability,
  rememberServerCapabilities,
} from '../services/serverCapabilities';
import { RolesPage } from './RolesPage';

// Etapa A3 (§3.2 y §3.1): la pantalla de permisos de un rol, agrupada por área con secciones
// plegables, «5 de 12», marcar y quitar todo el grupo, el buscador, las dependencias automáticas con
// su aviso, la vista sencilla por capacidades (también parcial) y «Restablecer permisos
// predeterminados» con su vista previa, solo en los roles de fábrica. Todo sale del catálogo que
// informa Go (aquí, uno pequeño de prueba) y solo con la capacidad permission-groups-v1.

const native = vi.hoisted(() => ({ invoke: vi.fn() }));
vi.mock('@tauri-apps/api/core', () => ({ invoke: native.invoke, isTauri: () => true }));

type Request = { operation: string; resourceId: string | null; body?: Record<string, unknown> };

const catalog: PermissionCatalog = {
  groups: [
    {
      code: 'tables',
      name: 'Mesas y cuentas',
      permissions: ['waiter_devices.use', 'accounts.read', 'accounts.open'],
    },
    { code: 'cash', name: 'Caja y cobros', permissions: ['payments.read', 'payments.collect'] },
  ],
  permissions: [
    {
      key: 'waiter_devices.use',
      name: 'Entrar con PIN en teléfonos y tabletas de meseros',
      description: 'Entrar con su PIN en los teléfonos y tabletas de los meseros.',
      group: 'tables',
      requires: [],
    },
    {
      key: 'accounts.read',
      name: 'Consultar cuentas e importes',
      description: 'Ver las cuentas abiertas con sus productos e importes.',
      group: 'tables',
      requires: [],
    },
    {
      key: 'accounts.open',
      name: 'Abrir cuentas',
      description: 'Abrir una cuenta con su primer producto y descartar una cuenta vacía.',
      group: 'tables',
      requires: ['accounts.read'],
    },
    {
      key: 'payments.read',
      name: 'Consultar información financiera y pagos de cuentas',
      description: 'Ver los pagos, el saldo y lo que falta por cobrar de cada cuenta.',
      group: 'cash',
      requires: ['accounts.read'],
    },
    {
      key: 'payments.collect',
      name: 'Registrar cobros',
      description: 'Cobrar una cuenta completa.',
      group: 'cash',
      requires: ['payments.read'],
    },
  ],
  capabilities: [
    {
      code: 'tables',
      name: 'Atender mesas',
      description: 'Entrar con PIN, abrir cuentas, capturar y enviar pedidos.',
      permissions: ['waiter_devices.use', 'accounts.read', 'accounts.open'],
    },
    {
      code: 'collect',
      name: 'Cobrar',
      description: 'Cobrar cuentas.',
      permissions: ['payments.read', 'payments.collect'],
    },
  ],
  factory_roles: ['waiter'],
};

const waiterRole = {
  id: '00000000-0000-4000-8000-0000000000f1',
  code: 'waiter',
  name: 'Mesero',
  permissions: ['waiter_devices.use', 'accounts.read'],
  discount_limit: null,
};
const adminRole = {
  id: '00000000-0000-4000-8000-0000000000f2',
  code: 'administrator',
  name: 'Administrador',
  permissions: catalog.permissions.map((permission) => permission.key),
  discount_limit: '100.00',
};
const customRole = {
  id: '00000000-0000-4000-8000-0000000000f3',
  code: null,
  name: 'Barra',
  permissions: ['accounts.read'],
  discount_limit: null,
};

let requests: Request[];
let restoreReply: { status: number; body: unknown };

function sent(operation: string) {
  return requests.filter((request) => request.operation === operation);
}

beforeEach(() => {
  rememberServerCapabilities(['restaurant-operations', permissionGroupsCapability]);
  requests = [];
  restoreReply = {
    status: 200,
    body: {
      role: {
        ...waiterRole,
        permissions: ['waiter_devices.use', 'accounts.read', 'accounts.open'],
      },
      changed: true,
      added: ['accounts.open'],
      removed: [],
    },
  };
  native.invoke.mockImplementation(async (command: string, args: Request) => {
    if (command !== 'api_request') {
      return null;
    }
    requests.push(args);
    switch (args.operation) {
      case 'roles':
        return { status: 200, body: [adminRole, customRole, waiterRole] };
      case 'permission_catalog':
        return { status: 200, body: catalog };
      case 'permissions':
        return {
          status: 200,
          body: catalog.permissions.map(({ key, name }) => ({ key, description: name })),
        };
      case 'role_defaults':
        return {
          status: 200,
          body: {
            role: { id: waiterRole.id, code: 'waiter', name: 'Mesero' },
            current: waiterRole.permissions,
            factory: ['accounts.open', 'accounts.read', 'waiter_devices.use'],
            add: ['accounts.open'],
            remove: [],
          },
        };
      case 'restore_role_defaults':
        return restoreReply;
      case 'update_role':
        return { status: 200, body: waiterRole };
    }
    return { status: 200, body: [] };
  });
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  rememberServerCapabilities([]);
});

async function editRole(name: string) {
  const row = (await screen.findByText(name)).closest('tr') as HTMLElement;
  fireEvent.click(within(row).getByRole('button', { name: 'Editar permisos' }));
  await screen.findByRole('tab', { name: 'Vista sencilla' });
}

function openDetailed() {
  fireEvent.click(screen.getByRole('tab', { name: 'Vista detallada' }));
}

function section(name: string) {
  return screen.getByRole('button', { name: new RegExp(`^${name}`) }).closest('section')!;
}

// Las secciones están plegadas: se abre la del grupo antes de tocar sus permisos.
function openSection(name: string) {
  const header = screen.getByRole('button', { name: new RegExp(`^${name}`) });
  if (header.getAttribute('aria-expanded') !== 'true') {
    fireEvent.click(header);
  }
  return section(name);
}

function searchBox() {
  return screen.getByRole('searchbox', { name: /^Buscar permiso/ });
}

describe('reglas de la pantalla de permisos (datos de Go)', () => {
  it('marcar agrega lo que se necesita y quitar avisa qué deja de funcionar', () => {
    const checked = withRequirements(catalog, [], ['payments.collect']);
    expect(checked.selected).toEqual(['payments.collect', 'payments.read', 'accounts.read']);
    expect(checked.added).toEqual(['payments.read', 'accounts.read']);
    const removed = withoutPermissions(catalog, checked.selected, ['accounts.read']);
    expect(removed.selected).toEqual(['payments.collect', 'payments.read']);
    expect(removed.broken).toEqual(['payments.collect', 'payments.read']);
  });

  it('una capacidad es completa, parcial o ninguna, y quitarla conserva otra completa', () => {
    const [tables, collect] = catalog.capabilities;
    expect(capabilityState(tables!, ['accounts.read'])).toBe('some');
    expect(capabilityState(tables!, [])).toBe('none');
    const both = withRequirements(catalog, [], [...tables!.permissions, ...collect!.permissions]);
    expect(capabilityState(collect!, both.selected)).toBe('all');
    // accounts.read es de «Atender mesas», que sigue completa: no se quita con «Cobrar».
    const without = toggleCapability(catalog, both.selected, collect!);
    expect(without.selected.sort()).toEqual([
      'accounts.open',
      'accounts.read',
      'waiter_devices.use',
    ]);
  });

  it('el buscador compara el nombre y la descripción, sin acentos ni mayúsculas', () => {
    const open = catalog.permissions[2]!;
    expect(matchesSearch(open, 'ABRIR')).toBe(true);
    expect(matchesSearch(open, 'vacia')).toBe(true);
    expect(matchesSearch(open, 'cobrar')).toBe(false);
  });
});

describe('Roles y permisos agrupados (etapa A3, §3.2)', () => {
  it('abre en la vista sencilla con las capacidades y la parcial', async () => {
    render(<RolesPage can={() => true} onChanged={async () => undefined} />);
    await editRole('Mesero');
    expect(screen.getByRole('tab', { name: 'Vista sencilla' }).getAttribute('aria-selected')).toBe(
      'true',
    );
    const tables = screen.getByRole('checkbox', { name: 'Atender mesas' });
    expect(tables.getAttribute('aria-checked')).toBe('mixed');
    expect(screen.getByText('Parcial: 2 de 3')).toBeTruthy();
    expect((screen.getByRole('checkbox', { name: 'Cobrar' }) as HTMLInputElement).checked).toBe(
      false,
    );
    expect(sent('permissions')).toHaveLength(0);
  });

  it('marcar una capacidad marca su conjunto con lo que necesita y se guarda', async () => {
    render(<RolesPage can={() => true} onChanged={async () => undefined} />);
    await editRole('Mesero');
    fireEvent.click(screen.getByRole('checkbox', { name: 'Cobrar' }));
    expect((screen.getByRole('checkbox', { name: 'Cobrar' }) as HTMLInputElement).checked).toBe(
      true,
    );
    fireEvent.click(screen.getByRole('checkbox', { name: 'Atender mesas' }));
    expect(
      screen.getByRole('checkbox', { name: 'Atender mesas' }).getAttribute('aria-checked'),
    ).toBe('true');
    fireEvent.click(screen.getByRole('button', { name: 'Guardar rol' }));
    await screen.findByText('Rol guardado.');
    expect(
      ([...(sent('update_role')[0]?.body?.permissions as string[])] as string[]).sort(),
    ).toEqual([
      'accounts.open',
      'accounts.read',
      'payments.collect',
      'payments.read',
      'waiter_devices.use',
    ]);
  });

  it('la vista detallada agrupa por área con «x de y» y marca y quita todo el grupo', async () => {
    render(<RolesPage can={() => true} onChanged={async () => undefined} />);
    await editRole('Mesero');
    openDetailed();
    expect(section('Mesas y cuentas').textContent).toContain('2 de 3');
    expect(section('Caja y cobros').textContent).toContain('0 de 2');
    fireEvent.click(
      within(openSection('Caja y cobros')).getByRole('button', { name: 'Marcar todo el grupo' }),
    );
    expect(section('Caja y cobros').textContent).toContain('2 de 2');
    fireEvent.click(
      within(openSection('Mesas y cuentas')).getByRole('button', { name: 'Quitar todo el grupo' }),
    );
    expect(section('Mesas y cuentas').textContent).toContain('0 de 3');
    // Los cobros necesitan consultar cuentas: se avisa qué dejará de funcionar.
    expect(screen.getByRole('status').textContent).toContain('dejarán de funcionar');
    expect(screen.getByRole('status').textContent).toContain('«Registrar cobros»');
  });

  it('marcar un permiso marca lo que necesita y quitarlo avisa a cuáles les hace falta', async () => {
    render(<RolesPage can={() => true} onChanged={async () => undefined} />);
    await editRole('Barra');
    openDetailed();
    openSection('Mesas y cuentas');
    openSection('Caja y cobros');
    fireEvent.click(screen.getByRole('checkbox', { name: 'Registrar cobros' }));
    expect(
      (
        screen.getByRole('checkbox', {
          name: 'Consultar información financiera y pagos de cuentas',
        }) as HTMLInputElement
      ).checked,
    ).toBe(true);
    expect(screen.getByRole('status').textContent).toContain(
      'También se marcó lo que necesita: «Consultar información financiera y pagos de cuentas»',
    );
    fireEvent.click(screen.getByRole('checkbox', { name: 'Consultar cuentas e importes' }));
    const warning = screen.getByRole('status').textContent;
    expect(warning).toContain('dejarán de funcionar:');
    expect(warning).toContain('«Consultar información financiera y pagos de cuentas»');
    expect(warning).toContain('«Registrar cobros»');
    // Se avisa, no se quita nada más.
    expect(
      (screen.getByRole('checkbox', { name: 'Registrar cobros' }) as HTMLInputElement).checked,
    ).toBe(true);
  });

  it('el buscador filtra por nombre y por descripción y abre las secciones', async () => {
    render(<RolesPage can={() => true} onChanged={async () => undefined} />);
    await editRole('Mesero');
    openDetailed();
    fireEvent.change(searchBox(), { target: { value: 'vacía' } });
    expect(screen.getByRole('checkbox', { name: 'Abrir cuentas' })).toBeTruthy();
    expect(screen.queryByRole('checkbox', { name: 'Registrar cobros' })).toBeNull();
    expect(screen.queryByRole('button', { name: /^Caja y cobros/ })).toBeNull();
    expect(section('Mesas y cuentas').getAttribute('data-expanded')).toBe('true');
    fireEvent.change(searchBox(), { target: { value: 'cobrar' } });
    expect(
      screen.getByRole('checkbox', { name: 'Consultar información financiera y pagos de cuentas' }),
    ).toBeTruthy();
    fireEvent.change(searchBox(), {
      target: { value: 'nada parecido' },
    });
    expect(screen.getByText('Ningún permiso coincide con la búsqueda.')).toBeTruthy();
  });

  it('«Restablecer permisos predeterminados» solo en los roles de fábrica, con su vista previa', async () => {
    const changed = vi.fn(async () => undefined);
    render(<RolesPage can={() => true} onChanged={changed} />);
    await editRole('Administrador');
    expect(
      screen.queryByRole('button', { name: 'Restablecer permisos predeterminados' }),
    ).toBeNull();
    fireEvent.click(screen.getByRole('button', { name: 'Cerrar editor' }));
    await editRole('Barra');
    expect(
      screen.queryByRole('button', { name: 'Restablecer permisos predeterminados' }),
    ).toBeNull();
    fireEvent.click(screen.getByRole('button', { name: 'Cerrar editor' }));
    await editRole('Mesero');
    fireEvent.click(screen.getByRole('button', { name: 'Restablecer permisos predeterminados' }));
    const dialog = await screen.findByRole('alertdialog');
    expect(dialog.textContent).toContain('Se agregarán (1):');
    expect(dialog.textContent).toContain('Abrir cuentas');
    expect(dialog.textContent).not.toContain('Se quitarán');
    expect(dialog.textContent).toContain('No cambian el límite de descuento');
    fireEvent.click(within(dialog).getByRole('button', { name: 'Restablecer permisos' }));
    await screen.findByText('Permisos predeterminados de Mesero restablecidos.');
    expect(sent('restore_role_defaults')[0]).toMatchObject({
      resourceId: waiterRole.id,
      body: { expected: waiterRole.permissions },
    });
    await waitFor(() =>
      expect(
        screen.getByRole('checkbox', { name: 'Atender mesas' }).getAttribute('aria-checked'),
      ).toBe('true'),
    );
    expect(changed).toHaveBeenCalled();
  });

  it('si el rol cambió entre la vista previa y la confirmación, lo dice y no cambia nada', async () => {
    restoreReply = {
      status: 409,
      body: {
        error: {
          code: 'ROLE_CHANGED',
          message:
            'Los permisos de este rol cambiaron mientras revisabas los cambios. Revisa otra vez qué se agregará y qué se quitará.',
        },
      },
    };
    render(<RolesPage can={() => true} onChanged={async () => undefined} />);
    await editRole('Mesero');
    fireEvent.click(screen.getByRole('button', { name: 'Restablecer permisos predeterminados' }));
    const dialog = await screen.findByRole('alertdialog');
    fireEvent.click(within(dialog).getByRole('button', { name: 'Restablecer permisos' }));
    await screen.findByText(/cambiaron mientras revisabas los cambios/);
    expect(
      screen.getByRole('checkbox', { name: 'Atender mesas' }).getAttribute('aria-checked'),
    ).toBe('mixed');
  });

  it('con un servidor anterior, la lista de siempre', async () => {
    rememberServerCapabilities(['restaurant-operations']);
    render(<RolesPage can={() => true} onChanged={async () => undefined} />);
    const row = (await screen.findByText('Mesero')).closest('tr') as HTMLElement;
    fireEvent.click(within(row).getByRole('button', { name: 'Editar permisos' }));
    await screen.findByText('Abrir cuentas');
    expect(screen.queryByRole('tab', { name: 'Vista sencilla' })).toBeNull();
    expect(
      screen.queryByRole('button', { name: 'Restablecer permisos predeterminados' }),
    ).toBeNull();
    expect(sent('permission_catalog')).toHaveLength(0);
  });
});
