// @vitest-environment jsdom

import { act, cleanup, fireEvent, render, screen, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { App } from './App';
import product from './generated/product.json';
import { useLeaveGuard } from './hooks/useLeaveGuard';
import type { Identity } from './services/models';

const native = vi.hoisted(() => ({ invoke: vi.fn() }));
vi.mock('@tauri-apps/api/core', () => ({ invoke: native.invoke, isTauri: () => true }));

// Solo el transporte nativo se sustituye en estas pruebas de interfaz.
// La aceptación de Go usa PostgreSQL real en una suite independiente.
const identity: Identity = {
  user: {
    id: '00000000-0000-0000-0000-000000000001',
    username: 'admin',
    display_name: 'Administrador',
    role_id: '00000000-0000-0000-0000-000000000002',
    role_name: 'Administrador',
    active: true,
    initial_password: true,
    support: false,
    session_id: '00000000-0000-0000-0000-000000000003',
    device_id: '00000000-0000-0000-0000-000000000004',
    permissions: ['users.read', 'users.manage', 'roles.read', 'permissions.read'],
  },
  preferences: { theme: 'dark', home_screen: 'status' },
  terminal: {
    id: '00000000-0000-0000-0000-000000000004',
    name: 'Terminal de prueba',
    register_id: null,
    keyboard_mode: 'auto',
  },
  establishment: { name: 'ADEL POS', timezone: 'America/Monterrey', currency: 'MXN' },
};

let configured = true;
let enrolled = true;
let hasSession = false;
let offline = false;
let replaced = false;
// Identidad que entrega `me` en cada prueba (permisos y pantalla de inicio de la terminal).
let current: Identity = identity;

const settings = {
  name: 'ADEL POS',
  timezone: 'America/Monterrey',
  currency: 'MXN',
  phone: '',
  email: '',
  address: '',
  theme: 'light',
};

beforeEach(() => {
  configured = true;
  enrolled = true;
  hasSession = false;
  offline = false;
  replaced = false;
  current = identity;
  native.invoke.mockImplementation(async (command: string, args?: { operation?: string }) => {
    if (command === 'authentication_state') {
      return {
        enrolled,
        has_session: hasSession,
        local_setup_available: true,
        local_enrollment_available: false,
      };
    }
    if (command === 'connection_config') {
      return { installation_mode: 'server-terminal', server_url: 'http://127.0.0.1:8181' };
    }
    if (command === 'server_snapshot') {
      return {
        live: { status: 200, body: { status: 'alive' } },
        ready: {
          status: 200,
          body: { status: 'ready', checks: { postgres: 'available', schema: 'compatible' } },
        },
        version: {
          status: 200,
          body: {
            product: product.technical_id,
            server_version: product.version,
            api_version: product.api_version,
            schema_version: product.schema_version,
          },
        },
      };
    }
    if (command !== 'api_request') {
      throw new Error('Comando de prueba desconocido');
    }
    if (offline) {
      throw 'No se pudo conectar con el servidor.';
    }
    switch (args?.operation) {
      case 'setup_status':
        return { status: 200, body: { configured } };
      case 'setup':
        configured = true;
        enrolled = true;
        return { status: 200, body: { configured: true } };
      case 'login':
        hasSession = true;
        return { status: 200, body: {} };
      case 'logout':
        hasSession = false;
        return { status: 200, body: {} };
      case 'me':
        if (replaced) {
          hasSession = false;
          return {
            status: 401,
            body: {
              error: {
                code: 'SESSION_REPLACED',
                message: 'Tu sesión se cerró por un nuevo acceso en otra terminal.',
              },
            },
          };
        }
        return { status: 200, body: current };
      case 'settings':
        return { status: 200, body: settings };
      case 'settings_business_day':
        return {
          status: 200,
          body: { operating_day_cutoff: '05:00', timezone: 'America/Monterrey' },
        };
      default:
        return { status: 200, body: [] };
    }
  });
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.clearAllMocks();
});

describe('acceso y recuperación de la interfaz', () => {
  it('informa una autorización pendiente del Llavero y permite reintentar sin perder credenciales', async () => {
    let pending = true;
    const originalTransport = native.invoke.getMockImplementation();
    native.invoke.mockImplementation((command: string, args?: { operation?: string }) => {
      if (command === 'authentication_state' && pending) {
        return Promise.reject(
          'Esperando autorización del almacén seguro del sistema. Revisa el diálogo de macOS.',
        );
      }
      return originalTransport?.(command, args);
    });
    render(<App />);
    await screen.findByRole('heading', { name: 'Revisa el acceso seguro' });
    expect(screen.getByRole('button', { name: 'Ver estado del sistema' })).toBeTruthy();
    expect(native.invoke.mock.calls.some(([command]) => command === 'api_request')).toBe(false);
    pending = false;
    fireEvent.click(screen.getByRole('button', { name: 'Volver a comprobar' }));
    await screen.findByRole('heading', { name: 'Inicia sesión' });
  });

  it('utiliza ADEL POS y la zona del sistema como valores iniciales configurables', async () => {
    configured = false;
    enrolled = false;
    render(<App />);
    await screen.findByRole('heading', { name: 'Prepara tu establecimiento' });
    const name = screen.getByLabelText('Nombre comercial') as HTMLInputElement;
    const timezone = screen.getByRole('combobox', { name: /Zona horaria/ }) as HTMLInputElement;
    expect(name.value).toBe('ADEL POS');
    expect(timezone.value).toBe(Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC');
    fireEvent.change(timezone, { target: { value: 'America/Tijuana' } });
    fireEvent.click(screen.getByRole('button', { name: 'Configurar establecimiento' }));
    await screen.findByRole('heading', { name: 'Inicia sesión' });
    expect(native.invoke).toHaveBeenCalledWith(
      'api_request',
      expect.objectContaining({
        operation: 'setup',
        body: expect.objectContaining({ name: 'ADEL POS', timezone: 'America/Tijuana' }),
      }),
    );
  });

  it('permite usar el sistema con la advertencia de contraseña inicial', async () => {
    render(<App />);
    await screen.findByRole('heading', { name: 'Inicia sesión' });
    fireEvent.change(screen.getByLabelText('Contraseña'), { target: { value: 'admin' } });
    fireEvent.click(screen.getByRole('button', { name: 'Entrar' }));
    // Etapa 9C: sin secciones operativas abre Administración, nunca «Estado del sistema».
    await screen.findByRole('heading', { name: 'Administración' });
    expect(screen.queryByRole('heading', { name: 'Estado del sistema' })).toBeNull();
    expect(screen.getByText('Recomendamos cambiar tu contraseña inicial.')).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: /^Menú del usuario/ }));
    fireEvent.click(screen.getByRole('button', { name: 'Mis preferencias' }));
    expect(screen.getByRole('heading', { name: 'Apariencia personal' })).toBeTruthy();
    // «Pantalla inicial» salió de Mis preferencias: es de la terminal.
    expect(screen.queryByText('Pantalla inicial')).toBeNull();
    expect(document.documentElement.dataset.theme).toBe('dark');
  });

  it('muestra y recupera la pérdida de conexión sin desmontar la sesión', async () => {
    hasSession = true;
    vi.useFakeTimers();
    render(<App />);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(screen.getByRole('heading', { name: 'Administración' })).toBeTruthy();
    expect(screen.getByRole('status', { name: 'Servidor: En línea' })).toBeTruthy();
    offline = true;
    await act(async () => {
      await vi.advanceTimersByTimeAsync(5000);
    });
    const status = screen.getByRole('status', { name: 'Servidor: Fuera de línea' });
    expect(status.textContent).toBe('Fuera de línea');
    expect(screen.getByRole('button', { name: 'Personal' })).toBeTruthy();
    offline = false;
    await act(async () => {
      await vi.advanceTimersByTimeAsync(5000);
    });
    expect(screen.getByRole('status', { name: 'Servidor: En línea' })).toBeTruthy();
    expect(screen.queryByRole('status', { name: 'Servidor: Fuera de línea' })).toBeNull();
  });

  it('notifica el reemplazo de sesión y vuelve al acceso', async () => {
    hasSession = true;
    vi.useFakeTimers();
    render(<App />);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(screen.getByRole('heading', { name: 'Administración' })).toBeTruthy();
    replaced = true;
    await act(async () => {
      await vi.advanceTimersByTimeAsync(5000);
    });
    expect(screen.getByRole('heading', { name: 'Inicia sesión' })).toBeTruthy();
    expect(
      screen.getByText('Tu sesión se cerró por un nuevo acceso en otra terminal.'),
    ).toBeTruthy();
    expect(screen.queryByRole('button', { name: 'Personal' })).toBeNull();
  });
});

// Componente de prueba con cambios sin guardar, como el editor de plantillas de la 9B.
function UnsavedChanges() {
  useLeaveGuard(true, 'Hay cambios sin guardar en la plantilla.');
  return null;
}

function withPermissions(permissions: string[], home?: Identity['terminal']['home_screen']) {
  current = {
    ...identity,
    user: { ...identity.user, initial_password: false, permissions },
    terminal: { ...identity.terminal, home_screen: home },
  };
}

async function signedIn(ui = <App />) {
  hasSession = true;
  render(ui);
  return screen.findByRole('navigation', { name: 'Navegación principal' });
}

describe('navegación de la etapa 9C', () => {
  it('sin el permiso de la pantalla de la terminal abre la primera permitida, nunca el estado', async () => {
    withPermissions(['users.read', 'roles.read', 'sessions.read'], 'kitchen');
    const navigation = await signedIn();
    await screen.findByRole('heading', { name: 'Administración' });
    expect(
      within(navigation)
        .getAllByRole('button')
        .map((button) => button.textContent),
    ).toEqual(['Administración']);
    expect(
      within(navigation)
        .getByRole('button', { name: 'Administración' })
        .getAttribute('aria-current'),
    ).toBe('page');
    expect(screen.queryByRole('heading', { name: 'Estado del sistema' })).toBeNull();
  });

  it('Personal abre su segunda pantalla de mosaicos con ruta y el logotipo vuelve al inicio', async () => {
    withPermissions(['users.read', 'roles.read', 'sessions.read']);
    await signedIn();
    fireEvent.click(await screen.findByRole('button', { name: 'Personal' }));
    await screen.findByRole('heading', { name: 'Personal' });
    const route = screen.getByRole('navigation', { name: 'Ruta de navegación' });
    expect(route.textContent).toBe('Administración›Personal');
    expect(screen.getByRole('button', { name: 'Usuarios' })).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Roles' })).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Sesiones' })).toBeTruthy();
    fireEvent.click(within(route).getByRole('button', { name: 'Administración' }));
    await screen.findByRole('heading', { name: 'Administración' });
    fireEvent.click(screen.getByRole('button', { name: 'Personal' }));
    await screen.findByRole('heading', { name: 'Personal' });
    fireEvent.click(screen.getByRole('button', { name: /^AD-EL POS · Ir al inicio/ }));
    await screen.findByRole('heading', { name: 'Administración' });
  });

  it('la barra, los mosaicos y el logotipo respetan los cambios sin guardar', async () => {
    withPermissions(['users.read', 'settings.read']);
    const navigation = await signedIn(
      <>
        <App />
        <UnsavedChanges />
      </>,
    );
    await screen.findByRole('heading', { name: 'Administración' });
    fireEvent.click(screen.getByRole('button', { name: 'Sistema' }));
    let dialog = await screen.findByRole('alertdialog', { name: 'Salir sin guardar' });
    expect(dialog.textContent).toContain('Hay cambios sin guardar en la plantilla.');
    fireEvent.click(within(dialog).getByRole('button', { name: 'Seguir editando' }));
    await act(async () => undefined);
    expect(screen.getByRole('heading', { name: 'Administración' })).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: 'Sistema' }));
    dialog = await screen.findByRole('alertdialog', { name: 'Salir sin guardar' });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Salir sin guardar' }));
    await screen.findByRole('heading', { name: 'Sistema' });
    // Desde la barra superior.
    fireEvent.click(within(navigation).getByRole('button', { name: 'Administración' }));
    dialog = await screen.findByRole('alertdialog', { name: 'Salir sin guardar' });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Seguir editando' }));
    await act(async () => undefined);
    expect(screen.getByRole('heading', { name: 'Sistema' })).toBeTruthy();
    // Desde el logotipo y el menú del usuario.
    fireEvent.click(screen.getByRole('button', { name: /^AD-EL POS · Ir al inicio/ }));
    dialog = await screen.findByRole('alertdialog', { name: 'Salir sin guardar' });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Seguir editando' }));
    await act(async () => undefined);
    fireEvent.click(screen.getByRole('button', { name: /^Menú del usuario/ }));
    fireEvent.click(screen.getByRole('button', { name: 'Mis preferencias' }));
    dialog = await screen.findByRole('alertdialog', { name: 'Salir sin guardar' });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Salir sin guardar' }));
    await screen.findByRole('heading', { name: 'Mis preferencias' });
  });

  it('«Ver estado del sistema» en Sistema → Configuración, con soporte solo para el administrador', async () => {
    withPermissions(['settings.read', 'settings.manage']);
    await signedIn();
    fireEvent.click(await screen.findByRole('button', { name: 'Sistema' }));
    fireEvent.click(await screen.findByRole('button', { name: 'Configuración' }));
    await screen.findByRole('heading', { name: 'Configuración general' });
    expect(screen.getByRole('navigation', { name: 'Ruta de navegación' }).textContent).toBe(
      'Administración›Sistema›Configuración',
    );
    fireEvent.click(screen.getByRole('button', { name: 'Ver estado del sistema' }));
    await screen.findByRole('heading', { name: 'Estado del sistema' });
    expect(screen.getByRole('navigation', { name: 'Ruta de navegación' }).textContent).toBe(
      'Administración›Sistema›Configuración›Estado del sistema',
    );
    expect(
      await screen.findByText(
        'Todo funciona correctamente: el servidor responde y la base de datos está conectada.',
      ),
    ).toBeTruthy();
    const main = document.querySelector('main')!;
    const support = screen.getByRole('button', { name: /Información para soporte/ });
    expect(support.getAttribute('aria-expanded')).toBe('false');
    // Lo visible sin desplegar el soporte no usa nombres de tecnologías ni direcciones.
    const visible = Array.from(
      main.querySelectorAll('.page-title, .system-status-summary, .surface'),
    )
      .map((element) => element.textContent)
      .join(' ');
    for (const technical of ['Go', 'PostgreSQL', 'Tauri', 'React', '127.0.0.1', '8181']) {
      expect(visible).not.toContain(technical);
    }
    cleanup();
    withPermissions(['settings.read']);
    await signedIn();
    fireEvent.click(await screen.findByRole('button', { name: 'Sistema' }));
    fireEvent.click(await screen.findByRole('button', { name: 'Configuración' }));
    fireEvent.click(await screen.findByRole('button', { name: 'Ver estado del sistema' }));
    await screen.findByRole('heading', { name: 'Estado del sistema' });
    expect(screen.queryByRole('button', { name: /Información para soporte/ })).toBeNull();
  });

  it('el diagnóstico antes de iniciar sesión usa el mismo lenguaje sencillo', async () => {
    offline = true;
    render(<App />);
    await screen.findByRole('heading', { name: 'Revisa la conexión' });
    fireEvent.click(screen.getByRole('button', { name: 'Ver estado del sistema' }));
    await screen.findByRole('heading', { name: 'Estado del sistema' });
    expect(screen.getByRole('button', { name: /Información para soporte/ })).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Volver al acceso' })).toBeTruthy();
  });
});
