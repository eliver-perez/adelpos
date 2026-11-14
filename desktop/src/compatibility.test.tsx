// @vitest-environment jsdom

import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { App } from './App';
import product from './generated/product.json';
import { serverTerminalInstallation } from './testing/installation';

const native = vi.hoisted(() => ({ invoke: vi.fn() }));
vi.mock('@tauri-apps/api/core', () => ({ invoke: native.invoke, isTauri: () => true }));

// Etapa 9G (§5.1): en lugar de errores raros, la terminal dice «Actualiza el servidor» si el servidor es más viejo
// de lo que necesita y «Actualiza esta terminal» si Go rechaza su versión (426 CLIENT_UPDATE_REQUIRED). Solo se
// sustituye el transporte nativo.
let serverVersion = product.version;
let clientRejected = false;
const operations: string[] = [];

beforeEach(() => {
  serverVersion = product.version;
  clientRejected = false;
  operations.length = 0;
  native.invoke.mockImplementation(async (command: string, args?: { operation?: string }) => {
    if (command === 'authentication_state') {
      return {
        enrolled: true,
        has_session: false,
        local_setup_available: false,
        local_enrollment_available: false,
      };
    }
    if (command === 'connection_config') {
      return { installation_mode: 'server-terminal', server_url: 'http://127.0.0.1:8181' };
    }
    if (command === 'installation_state') {
      return serverTerminalInstallation;
    }
    if (command === 'server_snapshot') {
      return {
        live: { status: 200, body: { status: 'alive' } },
        ready: {
          status: 200,
          body: { status: 'ready', checks: { postgres: 'available', schema: 'compatible' } },
        },
        version: { status: 200, body: { server_version: serverVersion } },
      };
    }
    if (command !== 'api_request') {
      return null;
    }
    operations.push(args?.operation ?? '');
    if (args?.operation === 'version') {
      return {
        status: 200,
        body: { server_version: serverVersion, min_client_version: product.min_client_version },
      };
    }
    if (clientRejected) {
      return {
        status: 426,
        body: {
          error: {
            code: 'CLIENT_UPDATE_REQUIRED',
            message:
              'Actualiza esta terminal: el servidor del restaurante necesita una versión más reciente.',
          },
        },
      };
    }
    if (args?.operation === 'setup_status') {
      return { status: 200, body: { configured: true } };
    }
    return { status: 200, body: {} };
  });
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe('versión incompatible', () => {
  it('con un servidor compatible pide iniciar sesión', async () => {
    render(<App />);
    await screen.findByRole('button', { name: 'Entrar' });
    expect(operations.slice(0, 2)).toEqual(['version', 'setup_status']);
  });

  it('avisa «Actualiza el servidor» si el servidor es más viejo de lo que necesita la aplicación', async () => {
    serverVersion = '0.0.1';
    render(<App />);
    await screen.findByRole('heading', { name: 'Actualiza el servidor' });
    expect(
      screen.getByText(/computadora principal del restaurante tiene una versión anterior/),
    ).toBeTruthy();
    expect(operations).not.toContain('setup_status');
    expect(screen.getByRole('button', { name: 'Volver a comprobar' })).toBeTruthy();
  });

  it('avisa «Actualiza esta terminal» si Go rechaza la versión de la aplicación', async () => {
    clientRejected = true;
    render(<App />);
    await screen.findByRole('heading', { name: 'Actualiza esta terminal' });
    expect(
      screen.getByText(/necesita una versión más reciente de AD-EL POS en este equipo/),
    ).toBeTruthy();
    expect(screen.queryByRole('button', { name: 'Entrar' })).toBeNull();
  });
});
