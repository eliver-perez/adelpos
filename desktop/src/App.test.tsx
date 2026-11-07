// @vitest-environment jsdom

import { act, cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { App } from './App';
import product from './generated/product.json';
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

beforeEach(() => {
  configured = true;
  enrolled = true;
  hasSession = false;
  offline = false;
  replaced = false;
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
        return { status: 200, body: identity };
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
    await screen.findByRole('heading', { name: 'Estado del sistema' });
    expect(screen.getByText('Recomendamos cambiar tu contraseña inicial.')).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: 'Mis preferencias' }));
    expect(screen.getByRole('heading', { name: 'Apariencia personal' })).toBeTruthy();
    expect(document.documentElement.dataset.theme).toBe('dark');
  });

  it('muestra y recupera la pérdida de conexión sin desmontar la sesión', async () => {
    hasSession = true;
    vi.useFakeTimers();
    render(<App />);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(screen.getByRole('heading', { name: 'Estado del sistema' })).toBeTruthy();
    offline = true;
    await act(async () => {
      await vi.advanceTimersByTimeAsync(5000);
    });
    expect(screen.getByText('Sin conexión')).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Usuarios' })).toBeTruthy();
    offline = false;
    await act(async () => {
      await vi.advanceTimersByTimeAsync(5000);
    });
    expect(screen.getByText('Sesión verificada')).toBeTruthy();
    expect(screen.queryByText('Sin conexión')).toBeNull();
  });

  it('notifica el reemplazo de sesión y vuelve al acceso', async () => {
    hasSession = true;
    vi.useFakeTimers();
    render(<App />);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(screen.getByRole('heading', { name: 'Estado del sistema' })).toBeTruthy();
    replaced = true;
    await act(async () => {
      await vi.advanceTimersByTimeAsync(5000);
    });
    expect(screen.getByRole('heading', { name: 'Inicia sesión' })).toBeTruthy();
    expect(
      screen.getByText('Tu sesión se cerró por un nuevo acceso en otra terminal.'),
    ).toBeTruthy();
    expect(screen.queryByRole('button', { name: 'Usuarios' })).toBeNull();
  });
});
