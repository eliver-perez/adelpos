// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { PermissionCatalog } from '../services/permissionCatalog';
import {
  permissionGroupsCapability,
  rememberServerCapabilities,
} from '../services/serverCapabilities';
import { RolesPage } from './RolesPage';

// Revisión independiente de la Fase B de la etapa A3 (§3.1, A3-D6): la pantalla no debe ofrecer
// «Restablecer permisos predeterminados» donde Go lo rechaza. Go exige que quien restablece tenga
// todos los permisos actuales del rol (role_defaults.go, «No puedes editar un rol con permisos que
// tú no tienes.»), y la pantalla lo sabe con can(); hoy muestra el botón igual y el clic termina en
// un 403 de la vista previa.

const native = vi.hoisted(() => ({ invoke: vi.fn() }));
vi.mock('@tauri-apps/api/core', () => ({ invoke: native.invoke, isTauri: () => true }));

const catalog: PermissionCatalog = {
  groups: [
    {
      code: 'tables',
      name: 'Mesas y cuentas',
      permissions: ['waiter_devices.use', 'accounts.read'],
    },
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
  ],
  capabilities: [],
  factory_roles: ['waiter'],
};

const waiterRole = {
  id: '00000000-0000-4000-8000-0000000000f1',
  code: 'waiter',
  name: 'Mesero',
  permissions: ['waiter_devices.use', 'accounts.read'],
  discount_limit: null,
};

beforeEach(() => {
  rememberServerCapabilities([permissionGroupsCapability]);
  native.invoke.mockImplementation(async (command: string, args: { operation: string }) => {
    if (command !== 'api_request') {
      return null;
    }
    switch (args.operation) {
      case 'roles':
        return { status: 200, body: [waiterRole] };
      case 'permission_catalog':
        return { status: 200, body: catalog };
    }
    return { status: 200, body: [] };
  });
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  rememberServerCapabilities([]);
});

describe('«Restablecer permisos predeterminados» solo donde Go lo acepta (revisión de la Fase B)', () => {
  it('no se ofrece en un rol con permisos que quien edita no tiene', async () => {
    // Quien edita roles no tiene «Entrar con PIN…», que el Mesero sí tiene: Go rechazaría la vista
    // previa y la confirmación con 403.
    const can = (key: string) => key !== 'waiter_devices.use';
    render(<RolesPage can={can} onChanged={async () => undefined} />);
    const row = (await screen.findByText('Mesero')).closest('tr') as HTMLElement;
    fireEvent.click(within(row).getByRole('button', { name: 'Editar permisos' }));
    await screen.findByRole('tab', { name: 'Vista sencilla' });
    expect(
      screen.queryByRole('button', { name: 'Restablecer permisos predeterminados' }),
    ).toBeNull();
  });
});
