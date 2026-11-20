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
  factory_permissions: { waiter: ['accounts.open', 'accounts.read', 'waiter_devices.use'] },
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
// Lo que devuelve la lista de roles (otra persona puede cambiarla entre una consulta y otra).
let listedRoles: (typeof waiterRole | typeof adminRole | typeof customRole)[];

function sent(operation: string) {
  return requests.filter((request) => request.operation === operation);
}

beforeEach(() => {
  rememberServerCapabilities(['restaurant-operations', permissionGroupsCapability]);
  requests = [];
  listedRoles = [adminRole, customRole, waiterRole];
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
        return { status: 200, body: listedRoles };
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

  it('si el rol cambió entre la vista previa y la confirmación, lo dice y carga lo de ahora (revisión R5)', async () => {
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
    // Mientras tanto otra persona le dio al Mesero «Abrir cuentas» desde otra terminal.
    listedRoles = [
      adminRole,
      customRole,
      { ...waiterRole, permissions: ['waiter_devices.use', 'accounts.read', 'accounts.open'] },
    ];
    fireEvent.click(screen.getByRole('button', { name: 'Restablecer permisos predeterminados' }));
    const dialog = await screen.findByRole('alertdialog');
    fireEvent.click(within(dialog).getByRole('button', { name: 'Restablecer permisos' }));
    await screen.findByText(/Otra persona cambió los permisos de Mesero/);
    // La pantalla ya tiene lo que Go tiene ahora: un «Guardar rol» no pisa el cambio sin aviso.
    await waitFor(() =>
      expect(
        screen.getByRole('checkbox', { name: 'Atender mesas' }).getAttribute('aria-checked'),
      ).toBe('true'),
    );
    fireEvent.click(screen.getByRole('button', { name: 'Guardar rol' }));
    await screen.findByText('Rol guardado.');
    expect([...(sent('update_role')[0]?.body?.permissions as string[])].sort()).toEqual([
      'accounts.open',
      'accounts.read',
      'waiter_devices.use',
    ]);
  });

  it('no ofrece restablecer donde Go lo rechazaría y dice por qué (revisión R2)', async () => {
    // El rol propio de quien edita.
    render(
      <RolesPage
        can={() => true}
        onChanged={async () => undefined}
        currentRoleID={waiterRole.id}
      />,
    );
    await editRole('Mesero');
    expect(
      screen.queryByRole('button', { name: 'Restablecer permisos predeterminados' }),
    ).toBeNull();
    expect(screen.getByText(/Tu propio rol no se restablece desde tu usuario/)).toBeTruthy();
    cleanup();
    // Un conjunto de fábrica con un permiso que quien edita no tiene (el Mesero de hoy no lo trae).
    const withoutOpen = (key: string) => key !== 'accounts.open';
    render(
      <RolesPage
        can={withoutOpen}
        onChanged={async () => undefined}
        currentRoleID={adminRole.id}
      />,
    );
    await editRole('Mesero');
    expect(
      screen.getByText(/Los permisos predeterminados de este rol incluyen permisos/),
    ).toBeTruthy();
    expect(
      screen.queryByRole('button', { name: 'Restablecer permisos predeterminados' }),
    ).toBeNull();
    cleanup();
    // Un rol con un límite de descuento mayor que el de quien edita (el rol Barra, sin límite).
    listedRoles = [adminRole, customRole, { ...waiterRole, discount_limit: '10.00' }];
    render(
      <RolesPage
        can={() => true}
        onChanged={async () => undefined}
        currentRoleID={customRole.id}
      />,
    );
    await editRole('Mesero');
    expect(screen.getByText(/límite de descuento mayor que el tuyo/)).toBeTruthy();
    cleanup();
    // El soporte reservado tiene el 100 %: sí se ofrece, y ninguno de estos casos llama a Go.
    render(<RolesPage can={() => true} onChanged={async () => undefined} support />);
    await editRole('Mesero');
    expect(
      screen.getByRole('button', { name: 'Restablecer permisos predeterminados' }),
    ).toBeTruthy();
    expect(sent('role_defaults')).toHaveLength(0);
  });

  it('con una búsqueda, los botones del grupo actúan solo sobre lo que se ve (revisión R3)', async () => {
    render(<RolesPage can={() => true} onChanged={async () => undefined} />);
    await editRole('Barra');
    openDetailed();
    fireEvent.change(searchBox(), { target: { value: 'cobr' } });
    const cash = section('Caja y cobros');
    // «Registrar cobros» y la descripción de «Consultar información financiera…» («…por cobrar…»).
    fireEvent.click(within(cash).getByRole('button', { name: 'Marcar los 2 que se ven' }));
    expect(section('Caja y cobros').textContent).toContain('2 de 2');
    fireEvent.change(searchBox(), { target: { value: 'pin' } });
    const tables = section('Mesas y cuentas');
    fireEvent.click(within(tables).getByRole('button', { name: 'Marcar el que se ve' }));
    // Solo «Entrar con PIN…»: «Abrir cuentas», que no se ve, sigue sin marcar.
    expect(section('Mesas y cuentas').textContent).toContain('2 de 3');
    fireEvent.change(searchBox(), { target: { value: '' } });
    expect(
      (screen.getByRole('button', { name: /^Mesas y cuentas/ }) as HTMLElement).textContent,
    ).toContain('2 de 3');
    fireEvent.change(searchBox(), { target: { value: 'cobros' } });
    fireEvent.click(
      within(section('Caja y cobros')).getByRole('button', { name: 'Quitar el que se ve' }),
    );
    expect(section('Caja y cobros').textContent).toContain('1 de 2');
    // Sin búsqueda, los botones vuelven a ser de todo el grupo.
    fireEvent.change(searchBox(), { target: { value: '' } });
    openSection('Caja y cobros');
    expect(
      within(section('Caja y cobros')).getByRole('button', { name: 'Marcar todo el grupo' }),
    ).toBeTruthy();
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
