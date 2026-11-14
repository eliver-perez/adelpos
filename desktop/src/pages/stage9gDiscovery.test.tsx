// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { InstallationState } from '../services/installation';
import type { DiscoveredServer } from '../services/discovery';
import type { PairingView } from '../services/pairing';
import { serverTerminalInstallation } from '../testing/installation';
import { FindServerPage } from './setup/FindServerPage';

// Etapa 9G, fase C (§4.1, D2): «Buscar el servidor» muestra las computadoras principales que Rust encontró en la
// red (nombre del establecimiento y de la computadora, sin direcciones) y se elige una para pedir el código;
// React solo manda el identificador. Sin resultados o si la red no permite buscar, se explica y la dirección
// escrita a mano sigue disponible.

const native = vi.hoisted(() => ({ invoke: vi.fn() }));
vi.mock('@tauri-apps/api/core', () => ({ invoke: native.invoke, isTauri: () => true }));

const terminalOnly: InstallationState = {
  ...serverTerminalInstallation,
  mode: 'terminal-only',
  server_configured: false,
  service_installed: false,
  service_enabled: false,
};

const fonda: DiscoveredServer = {
  id: '11111111-2222-4333-8444-555555555555',
  name: 'Fonda Doña Ñeli — Mac de caja',
  paired: false,
};
const other: DiscoveredServer = {
  id: '99999999-2222-4333-8444-555555555555',
  name: 'Taquería del Centro — Caja principal',
  paired: false,
};

const code: PairingView = {
  state: 'pending',
  code: '305112',
  name: 'Caja de la terraza',
  server_name: 'Fonda Doña Ñeli',
  expires_at: new Date(Date.now() + 5 * 60 * 1000).toISOString(),
};

let found: DiscoveredServer[] | string;

function calls(command: string) {
  return native.invoke.mock.calls.filter(([name]) => name === command);
}

beforeEach(() => {
  found = [fonda, other];
  native.invoke.mockImplementation(async (command: string, args: Record<string, unknown>) => {
    if (command === 'discovery_search') {
      if (typeof found === 'string') {
        throw found;
      }
      return found;
    }
    if (command === 'pairing_start_found') {
      return { ...code, name: String(args.name) };
    }
    if (command === 'pairing_status') {
      return code;
    }
    if (command === 'pairing_cancel') {
      return null;
    }
    throw new Error(`Comando no esperado: ${command}`);
  });
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe('«Buscar el servidor»: computadoras encontradas en la red (etapa 9G, fase C)', () => {
  it('lista los servidores encontrados y pide el código al elegido solo con su identificador', async () => {
    render(<FindServerPage state={terminalOnly} />);
    expect(screen.getByText('Buscando la computadora principal en la red…')).toBeTruthy();
    const list = await screen.findByRole('list', { name: 'Computadoras encontradas' });
    expect(within(list).getAllByRole('button')).toHaveLength(2);
    expect(within(list).getByText('Fonda Doña Ñeli — Mac de caja')).toBeTruthy();
    fireEvent.change(screen.getByRole('textbox', { name: 'Nombre de esta terminal' }), {
      target: { value: 'Caja de la terraza' },
    });
    fireEvent.click(
      screen.getByRole('button', { name: 'Pedir código a Fonda Doña Ñeli — Mac de caja' }),
    );
    expect(await screen.findByLabelText('Código de emparejamiento 3 0 5 1 1 2')).toBeTruthy();
    expect(calls('pairing_start_found')[0]?.[1]).toEqual({
      serverId: fonda.id,
      name: 'Caja de la terraza',
    });
    // React nunca manda una dirección por esta vía ni muestra datos técnicos.
    expect(calls('pairing_start')).toHaveLength(0);
    expect(document.body.textContent).not.toMatch(/\d+\.\d+\.\d+\.\d+|8443|11111111/);
  });

  it('sin nombre no se puede elegir y «Buscar otra vez» repite la búsqueda', async () => {
    render(<FindServerPage state={terminalOnly} />);
    const choice = (await screen.findByRole('button', {
      name: 'Pedir código a Taquería del Centro — Caja principal',
    })) as HTMLButtonElement;
    fireEvent.change(screen.getByRole('textbox', { name: 'Nombre de esta terminal' }), {
      target: { value: '  ' },
    });
    expect(choice.disabled).toBe(true);
    found = [];
    fireEvent.click(screen.getByRole('button', { name: 'Buscar otra vez' }));
    expect(
      await screen.findByText(/No se encontró ninguna computadora principal en la red/),
    ).toBeTruthy();
    expect(calls('discovery_search')).toHaveLength(2);
    expect(screen.queryByRole('list', { name: 'Computadoras encontradas' })).toBeNull();
    // La dirección escrita a mano sigue disponible.
    expect(
      screen.getByRole('textbox', { name: 'Dirección de la computadora principal' }),
    ).toBeTruthy();
  });

  it('si la red no permite buscar lo explica y conserva la dirección a mano', async () => {
    found =
      'No se pudo buscar en la red local. Revisa que esta computadora esté conectada a la red del restaurante y que AD-EL POS tenga permiso de usar la red local; también puedes escribir la dirección.';
    render(<FindServerPage state={terminalOnly} />);
    expect(await screen.findByText(/permiso de usar la red local/)).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Pedir código' })).toBeTruthy();
  });

  it('al cancelar el código vuelve a la lista y busca de nuevo', async () => {
    render(<FindServerPage state={terminalOnly} />);
    fireEvent.click(
      await screen.findByRole('button', { name: 'Pedir código a Fonda Doña Ñeli — Mac de caja' }),
    );
    fireEvent.click(await screen.findByRole('button', { name: 'Cancelar' }));
    await waitFor(() => expect(calls('discovery_search')).toHaveLength(2));
    expect(await screen.findByRole('list', { name: 'Computadoras encontradas' })).toBeTruthy();
  });
});
