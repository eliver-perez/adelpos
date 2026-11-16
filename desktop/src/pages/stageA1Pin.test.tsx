// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { UserMenu } from '../navigation/UserMenu';
import { screenAuthorized, sectionByID } from '../navigation/sections';
import type { Device, User } from '../services/models';
import {
  rememberServerCapabilities,
  serverHas,
  waiterPINCapability,
} from '../services/serverCapabilities';
import { chooseOption, optionLabels, selectText } from '../testing/select';
import { DevicesPage, idleLockLabel } from './DevicesPage';
import { MyPinPage } from './UserPin';
import { UsersPage } from './UsersPage';

// Etapa A1, fase B: el PIN de los meseros en el escritorio. Personal › Usuarios asigna, cambia, quita y desbloquea el
// PIN con el nombre corto y el color del distintivo (paleta de Go); «Mi PIN» en el menú del usuario; el bloqueo por
// inactividad de los teléfonos y tabletas en Terminales. Todo aparece solo si el servidor anuncia waiter-pin-v1 (el
// escritorio 0.12.0 sigue funcionando con un servidor 0.11.0). Las reglas del PIN son de Go: la pantalla muestra su
// mensaje.

const native = vi.hoisted(() => ({ invoke: vi.fn() }));
vi.mock('@tauri-apps/api/core', () => ({ invoke: native.invoke, isTauri: () => true }));

type Reply = { status: number; body: unknown };
type Request = { operation: string; resourceId: string | null; body?: Record<string, unknown> };

const palette = [
  { hex: '#d64545', name: 'Rojo' },
  { hex: '#3591d4', name: 'Celeste' },
];

const base: User = {
  id: '00000000-0000-4000-8000-0000000000a1',
  username: 'ana',
  display_name: 'Ana López',
  role_id: '00000000-0000-4000-8000-0000000000f1',
  role_name: 'Mesero',
  active: true,
  initial_password: false,
  has_password: true,
  has_pin: false,
  short_name: null,
  badge_color: null,
  pin_locked_until: null,
};

let users: User[];
let replies: Record<string, Reply>;
let requests: Request[];

function sent(operation: string) {
  return requests.filter((request) => request.operation === operation);
}

beforeEach(() => {
  rememberServerCapabilities(['restaurant-operations', waiterPINCapability]);
  users = [
    base,
    {
      ...base,
      id: '00000000-0000-4000-8000-0000000000a2',
      username: 'beto',
      display_name: 'Beto Ruiz',
      has_pin: true,
      short_name: 'Beto',
      badge_color: '#3591d4',
    },
    {
      ...base,
      id: '00000000-0000-4000-8000-0000000000a3',
      username: 'carla',
      display_name: 'Carla Díaz',
      has_pin: true,
      pin_locked_until: '2026-11-16T20:05:00Z',
    },
  ];
  replies = {};
  requests = [];
  native.invoke.mockImplementation(async (command: string, args: Request) => {
    if (command !== 'api_request') {
      return null;
    }
    requests.push(args);
    if (replies[args.operation]) {
      return replies[args.operation];
    }
    if (args.operation === 'users') {
      return { status: 200, body: users };
    }
    if (args.operation === 'user_badge_colors') {
      return { status: 200, body: palette };
    }
    if (['set_user_pin', 'remove_user_pin', 'unlock_user_pin'].includes(args.operation)) {
      return { status: 200, body: users.find((user) => user.id === args.resourceId) };
    }
    if (args.operation === 'set_own_pin') {
      return { status: 200, body: { has_pin: true, pin_version: 2 } };
    }
    return { status: 200, body: [] };
  });
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  rememberServerCapabilities([]);
});

function manager(key: string) {
  return ['users.read', 'users.manage', 'roles.read'].includes(key);
}

async function rowOf(username: string) {
  return (await screen.findByText(username)).closest('tr') as HTMLElement;
}

describe('capacidades del servidor', () => {
  it('solo cuenta lo que anunció el servidor; una respuesta inválida no anuncia nada', () => {
    expect(serverHas(waiterPINCapability)).toBe(true);
    rememberServerCapabilities('waiter-pin-v1');
    expect(serverHas(waiterPINCapability)).toBe(false);
    rememberServerCapabilities([waiterPINCapability, 7, null]);
    expect(serverHas(waiterPINCapability)).toBe(true);
  });
});

describe('Personal › Usuarios: PIN de los meseros', () => {
  it('muestra el estado del PIN y ofrece asignar, cambiar y desbloquear', async () => {
    render(<UsersPage can={manager} onChanged={async () => undefined} />);
    const ana = await rowOf('ana');
    expect(ana.textContent).toContain('Sin PIN');
    expect(within(ana).getByRole('button', { name: 'Asignar PIN' })).toBeTruthy();
    const beto = await rowOf('beto');
    expect(beto.textContent).toContain('Con PIN · Beto');
    expect(within(beto).getByRole('button', { name: 'Cambiar PIN' })).toBeTruthy();
    expect(within(beto).queryByRole('button', { name: 'Desbloquear' })).toBeNull();
    const carla = await rowOf('carla');
    expect(carla.textContent).toMatch(/PIN bloqueado hasta las \d{2}:\d{2}/);
    fireEvent.click(within(carla).getByRole('button', { name: 'Desbloquear' }));
    await screen.findByText('PIN de Carla Díaz desbloqueado.');
    expect(sent('unlock_user_pin')[0]?.resourceId).toBe(users[2]!.id);
  });

  it('sin users.manage solo se ve el estado; con un servidor anterior no aparece nada del PIN', async () => {
    const { unmount } = render(
      <UsersPage can={(key) => key === 'users.read'} onChanged={async () => undefined} />,
    );
    const beto = await rowOf('beto');
    expect(beto.textContent).toContain('Con PIN');
    expect(within(beto).queryByRole('button', { name: 'Cambiar PIN' })).toBeNull();
    unmount();
    rememberServerCapabilities(['restaurant-operations']);
    render(<UsersPage can={manager} onChanged={async () => undefined} />);
    const ana = await rowOf('ana');
    expect(ana.textContent).not.toContain('Sin PIN');
    expect(screen.queryByRole('columnheader', { name: 'PIN' })).toBeNull();
    expect(within(ana).queryByRole('button', { name: 'Asignar PIN' })).toBeNull();
  });

  it('asigna el PIN escrito dos veces con el nombre corto y un color de la paleta de Go', async () => {
    render(<UsersPage can={manager} onChanged={async () => undefined} />);
    fireEvent.click(within(await rowOf('ana')).getByRole('button', { name: 'Asignar PIN' }));
    const panel = screen.getByRole('region', { name: 'Asignar PIN a Ana López' });
    const pin = within(panel).getByLabelText('PIN') as HTMLInputElement;
    expect(pin.type).toBe('password');
    expect(pin.inputMode).toBe('numeric');
    // Solo dígitos, como máximo 6.
    fireEvent.change(pin, { target: { value: '48a29' } });
    expect(pin.value).toBe('4829');
    fireEvent.change(within(panel).getByLabelText('Escríbelo otra vez'), {
      target: { value: '4828' },
    });
    fireEvent.change(within(panel).getByLabelText(/^Nombre corto/), { target: { value: 'Ana' } });
    fireEvent.click(within(panel).getByRole('button', { name: 'Color del distintivo: Sin color' }));
    await waitFor(() =>
      expect(within(panel).getByRole('group', { name: 'Paleta de colores' })).toBeTruthy(),
    );
    fireEvent.click(await within(panel).findByRole('button', { name: 'Celeste' }));
    fireEvent.click(within(panel).getByRole('button', { name: 'Asignar PIN' }));
    expect((await screen.findByRole('alert', { name: 'Error' })).textContent).toContain(
      'Los dos PIN no coinciden.',
    );
    expect(sent('set_user_pin')).toHaveLength(0);
    fireEvent.change(within(panel).getByLabelText('PIN'), { target: { value: '4829' } });
    fireEvent.change(within(panel).getByLabelText('Escríbelo otra vez'), {
      target: { value: '4829' },
    });
    fireEvent.click(within(panel).getByRole('button', { name: 'Asignar PIN' }));
    await screen.findByText('PIN guardado.');
    expect(sent('set_user_pin')[0]).toMatchObject({
      resourceId: base.id,
      body: { pin: '4829', short_name: 'Ana', badge_color: '#3591d4' },
    });
  });

  it('el mensaje de Go para un PIN trivial se muestra tal cual', async () => {
    replies.set_user_pin = {
      status: 400,
      body: {
        error: {
          code: 'PIN_TOO_SIMPLE',
          message:
            'Ese PIN es muy fácil de adivinar (números iguales, seguidos o repetidos, o de los más usados). Elige otro.',
        },
      },
    };
    render(<UsersPage can={manager} onChanged={async () => undefined} />);
    fireEvent.click(within(await rowOf('ana')).getByRole('button', { name: 'Asignar PIN' }));
    const panel = screen.getByRole('region', { name: 'Asignar PIN a Ana López' });
    for (const label of ['PIN', 'Escríbelo otra vez']) {
      fireEvent.change(within(panel).getByLabelText(label), { target: { value: '1234' } });
    }
    fireEvent.click(within(panel).getByRole('button', { name: 'Asignar PIN' }));
    const alert = await screen.findByRole('alert', { name: 'Error' });
    expect(alert.textContent).toContain('muy fácil de adivinar');
    expect(alert.textContent).not.toContain('PIN_TOO_SIMPLE');
  });

  it('con PIN, guardar sin escribir uno solo cambia el distintivo; quitarlo pide confirmación', async () => {
    render(<UsersPage can={manager} onChanged={async () => undefined} />);
    fireEvent.click(within(await rowOf('beto')).getByRole('button', { name: 'Cambiar PIN' }));
    const panel = screen.getByRole('region', { name: 'Cambiar el PIN de Beto Ruiz' });
    expect((within(panel).getByLabelText(/^Nombre corto/) as HTMLInputElement).value).toBe('Beto');
    fireEvent.change(within(panel).getByLabelText(/^Nombre corto/), {
      target: { value: 'Betito' },
    });
    fireEvent.click(within(panel).getByRole('button', { name: 'Guardar' }));
    await screen.findByText('Nombre corto y color guardados.');
    expect(sent('set_user_pin')[0]?.body).toEqual({ short_name: 'Betito', badge_color: '#3591d4' });

    fireEvent.click(within(await rowOf('beto')).getByRole('button', { name: 'Cambiar PIN' }));
    const again = screen.getByRole('region', { name: 'Cambiar el PIN de Beto Ruiz' });
    const remove = within(again).getByRole('button', { name: 'Quitar PIN' });
    // Se puede volver a asignar: botón secundario, no rojo.
    expect(remove.className).toContain('secondary-button');
    expect(remove.className).not.toContain('danger');
    fireEvent.click(remove);
    const dialog = screen.getByRole('alertdialog', { name: 'Quitar el PIN de Beto Ruiz' });
    expect(dialog.textContent).toContain('se cerrarán sus sesiones');
    fireEvent.click(within(dialog).getByRole('button', { name: 'Sí, quitar el PIN' }));
    await screen.findByText('Se quitó el PIN de Beto Ruiz.');
    expect(sent('remove_user_pin')[0]?.resourceId).toBe(users[1]!.id);
  });
});

describe('«Mi PIN»', () => {
  it('pide la contraseña actual y el PIN dos veces', async () => {
    const refresh = vi.fn(async () => undefined);
    render(<MyPinPage hasPIN={false} refresh={refresh} />);
    expect(screen.getByText(/Todavía no tienes PIN/)).toBeTruthy();
    fireEvent.change(screen.getByLabelText('Contraseña actual'), {
      target: { value: 'secreta-1' },
    });
    fireEvent.change(screen.getByLabelText('PIN'), { target: { value: '5071' } });
    fireEvent.change(screen.getByLabelText('Escríbelo otra vez'), { target: { value: '5071' } });
    fireEvent.click(screen.getByRole('button', { name: 'Guardar mi PIN' }));
    await screen.findByText('PIN guardado.');
    expect(sent('set_own_pin')[0]?.body).toEqual({ current_password: 'secreta-1', pin: '5071' });
    expect(refresh).toHaveBeenCalled();
    expect((screen.getByLabelText('Contraseña actual') as HTMLInputElement).value).toBe('');
  });

  it('el menú del usuario solo lo ofrece si se le pasa; la sección exige waiter_devices.use', () => {
    const onPin = vi.fn();
    const props = {
      name: 'Ana López',
      role: 'Mesero',
      onPreferences: () => undefined,
      onPassword: () => undefined,
      onLogout: () => undefined,
    };
    const { unmount } = render(<UserMenu {...props} />);
    fireEvent.click(screen.getByRole('button', { name: /Menú del usuario/ }));
    expect(screen.queryByRole('button', { name: 'Mi PIN' })).toBeNull();
    unmount();
    render(<UserMenu {...props} onPin={onPin} />);
    fireEvent.click(screen.getByRole('button', { name: /Menú del usuario/ }));
    fireEvent.click(screen.getByRole('button', { name: 'Mi PIN' }));
    expect(onPin).toHaveBeenCalled();
    expect(screenAuthorized(sectionByID('pin'), ['waiter_devices.use'])).toBe(true);
    expect(screenAuthorized(sectionByID('pin'), ['users.read'])).toBe(false);
  });
});

describe('Terminales: bloqueo por inactividad de los teléfonos y tabletas', () => {
  const tablet: Device = {
    id: '00000000-0000-4000-8000-0000000000b1',
    name: 'Tableta 1',
    register_id: null,
    keyboard_mode: 'auto',
    home_screen: 'tables',
    kind: 'android',
    os: 'Android 15',
    app_version: '0.1.0',
    paired_at: '2026-11-16T15:00:00Z',
    online: true,
    current_user: null,
    idle_lock_seconds: 120,
  };
  const desk: Device = {
    ...tablet,
    id: '00000000-0000-4000-8000-0000000000b2',
    name: 'Caja de la barra',
    kind: 'desktop',
    os: 'Windows',
  };

  beforeEach(() => {
    replies.devices = { status: 200, body: [tablet, desk] };
    replies.devices_pairing_requests = {
      status: 200,
      body: { requests: [], approve_here: true, pairing_available: true },
    };
    replies.devices_sessions_summary = {
      status: 200,
      body: { by_kind: { desktop: 1, android: 2, kitchen: 0 }, total: 3, limited: false },
    };
    replies.device_configuration = {
      status: 200,
      body: {
        device: tablet,
        kitchen_all_stations: true,
        kitchen_sound_alert: false,
        kitchen_stations: [],
        printers: [],
        local_printers: null,
      },
    };
    replies.update_device = { status: 200, body: tablet };
  });

  it('el tipo se dice en lenguaje sencillo y la configuración muestra el bloqueo', async () => {
    render(<DevicesPage can={() => true} />);
    const row = (await screen.findByText('Tableta 1')).closest('tr') as HTMLElement;
    expect(row.textContent).toContain('Teléfono o tableta');
    expect(row.textContent).not.toContain('Android 15 Android');
    const summary = screen.getByRole('region', { name: 'Sesiones activas por tipo de terminal' });
    expect(summary.textContent).toContain('Teléfono o tableta2');
    fireEvent.click(within(row).getByRole('button', { name: 'Ver configuración' }));
    const configuration = await screen.findByRole('region', { name: 'Configuración de Tableta 1' });
    await waitFor(() => expect(configuration.textContent).toContain('Bloqueo por inactividad'));
    expect(configuration.textContent).toContain('2 minutos sin uso');
  });

  it('se elige en minutos (2 por omisión) y solo se envía para un teléfono o tableta', async () => {
    render(<DevicesPage can={() => true} />);
    const row = (await screen.findByText('Tableta 1')).closest('tr') as HTMLElement;
    fireEvent.click(within(row).getByRole('button', { name: 'Editar' }));
    const idle = screen.getByRole('combobox', { name: 'Bloqueo por inactividad' });
    expect(selectText(idle)).toBe('2 minutos (recomendado)');
    expect(optionLabels(idle)).toEqual([
      '1 minuto',
      '2 minutos (recomendado)',
      '3 minutos',
      '5 minutos',
      '10 minutos',
      '15 minutos',
      '30 minutos',
      '60 minutos',
    ]);
    chooseOption(idle, '10 minutos');
    fireEvent.click(screen.getByRole('button', { name: 'Guardar cambios' }));
    await screen.findByText('Terminal actualizada.');
    expect(sent('update_device')[0]).toMatchObject({
      resourceId: tablet.id,
      body: { name: 'Tableta 1', keyboard_mode: 'auto', idle_lock_seconds: 600 },
    });
    cleanup();
    requests = [];
    render(<DevicesPage can={() => true} />);
    const deskRow = (await screen.findByText('Caja de la barra')).closest('tr') as HTMLElement;
    fireEvent.click(within(deskRow).getByRole('button', { name: 'Editar' }));
    expect(screen.queryByRole('combobox', { name: 'Bloqueo por inactividad' })).toBeNull();
    fireEvent.click(screen.getByRole('button', { name: 'Guardar cambios' }));
    await screen.findByText('Terminal actualizada.');
    expect(sent('update_device')[0]?.body).not.toHaveProperty('idle_lock_seconds');
  });

  it('con un servidor anterior no se ofrece ni se envía', async () => {
    rememberServerCapabilities([]);
    render(<DevicesPage can={() => true} />);
    const row = (await screen.findByText('Tableta 1')).closest('tr') as HTMLElement;
    fireEvent.click(within(row).getByRole('button', { name: 'Editar' }));
    expect(screen.queryByRole('combobox', { name: 'Bloqueo por inactividad' })).toBeNull();
    fireEvent.click(screen.getByRole('button', { name: 'Guardar cambios' }));
    await screen.findByText('Terminal actualizada.');
    expect(sent('update_device')[0]?.body).not.toHaveProperty('idle_lock_seconds');
  });

  it('un valor guardado fuera de la lista se conserva y se nombra en lenguaje sencillo', () => {
    expect(idleLockLabel(60)).toBe('1 minuto');
    expect(idleLockLabel(45)).toBe('45 segundos');
    expect(idleLockLabel(90)).toBe('1 min 30 s');
    expect(idleLockLabel(3600)).toBe('60 minutos');
  });
});
