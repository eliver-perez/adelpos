// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { canOpen, sectionByID } from '../navigation/sections';
import type { InstallationState } from '../services/installation';
import type { ServiceStatus } from '../services/serverService';
import { serverTerminalInstallation } from '../testing/installation';
import { ModalitySection, ServiceSection } from './maintenance/ServerSections';
import { FindServerPage } from './setup/FindServerPage';
import { ModalityChoicePage, suggestedMode } from './setup/ModalityChoicePage';
import { ServerOnlyPage } from './server/ServerOnlyPage';

// Etapa 9G, fase A (§2.1–§2.3): elección de la modalidad en el primer arranque, «Buscar el servidor» de
// una terminal sin servidor, pantalla de «Solo servidor» y, en Mantenimiento, «Servicio del servidor» y
// «Modalidad de esta computadora». Rust guarda la modalidad (lista cerrada) y Go decide el estado y el
// reinicio del servicio; la interfaz solo elige, confirma y explica, sin borrar nunca la base.

const native = vi.hoisted(() => ({ invoke: vi.fn() }));
vi.mock('@tauri-apps/api/core', () => ({ invoke: native.invoke, isTauri: () => true }));

let installation: InstallationState;
let service: ServiceStatus;
let modeFailure: string | null;

const managedService: ServiceStatus = {
  managed: true,
  restart_available: true,
  installation_mode: 'server-terminal',
  service_enabled: true,
  started_at: '2026-11-13T18:00:00Z',
  uptime_seconds: 2 * 3600 + 5 * 60,
  server_version: '0.9.5',
  terminals: { connected: 2, registered: 3 },
  last_backup: null,
};

function calls(command: string) {
  return native.invoke.mock.calls.filter(([name]) => name === command);
}

function requests(operation: string) {
  return native.invoke.mock.calls.filter(
    ([command, args]) => command === 'api_request' && args?.operation === operation,
  );
}

beforeEach(() => {
  installation = { ...serverTerminalInstallation };
  service = { ...managedService };
  modeFailure = null;
  native.invoke.mockImplementation(
    async (command: string, args: { operation?: string; mode?: string }) => {
      if (command === 'installation_state') {
        return installation;
      }
      if (command === 'set_installation_mode') {
        if (modeFailure) {
          throw modeFailure;
        }
        return null;
      }
      if (command === 'restart_application') {
        return null;
      }
      // Etapa 9G, fase C: «Buscar el servidor» busca en la red al abrirse.
      if (command === 'discovery_search') {
        return [];
      }
      if (command !== 'api_request') {
        throw new Error(`Comando no esperado: ${command}`);
      }
      if (args.operation === 'maintenance_service') {
        return { status: 200, body: service };
      }
      if (args.operation === 'maintenance_service_restart') {
        return { status: 200, body: { restarting: true } };
      }
      return { status: 404, body: { error: { code: 'NOT_FOUND', message: 'No existe.' } } };
    },
  );
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  vi.useRealTimers();
});

describe('Modalidad en el primer arranque', () => {
  it('sugiere la modalidad que dejó el instalador', () => {
    expect(suggestedMode({ ...installation, chosen: false })).toBe('server-terminal');
    expect(
      suggestedMode({
        ...installation,
        chosen: false,
        service_installed: true,
        service_enabled: false,
      }),
    ).toBe('terminal-only');
    expect(
      suggestedMode({
        ...installation,
        chosen: false,
        service_installed: false,
        service_enabled: false,
      }),
    ).toBe('server-terminal');
  });

  it('ofrece las tres modalidades, guarda la elegida y reinicia la aplicación', async () => {
    render(<ModalityChoicePage state={{ ...installation, chosen: false }} />);
    const options = screen.getAllByRole('radio');
    expect(options.map((option) => option.textContent)).toEqual([
      expect.stringContaining('Servidor y terminal'),
      expect.stringContaining('Solo servidor'),
      expect.stringContaining('Solo terminal'),
    ]);
    expect(options[0]!.getAttribute('aria-checked')).toBe('true');
    // Las flechas mueven la elección, como en un grupo de radio.
    fireEvent.keyDown(screen.getByRole('radiogroup'), { key: 'ArrowDown' });
    expect(options[1]!.getAttribute('aria-checked')).toBe('true');
    fireEvent.click(screen.getByRole('radio', { name: /Solo terminal/ }));
    fireEvent.click(screen.getByRole('button', { name: 'Continuar' }));
    await waitFor(() => expect(calls('restart_application')).toHaveLength(1));
    expect(calls('set_installation_mode')[0]![1]).toEqual({ mode: 'terminal-only' });
  });

  it('avisa si el servidor no está instalado como servicio y muestra el error de Rust', async () => {
    modeFailure =
      'Solo la cuenta de la computadora que instaló AD-EL POS puede cambiar su modalidad.';
    render(
      <ModalityChoicePage
        state={{ ...installation, chosen: false, service_installed: false, service_enabled: false }}
      />,
    );
    expect(screen.getByRole('note').textContent).toContain('no arrancará solo al encenderla');
    fireEvent.click(screen.getByRole('radio', { name: /Solo terminal/ }));
    expect(screen.queryByRole('note')).toBeNull();
    fireEvent.click(screen.getByRole('button', { name: 'Continuar' }));
    expect(await screen.findByText(modeFailure)).toBeTruthy();
    expect(calls('restart_application')).toHaveLength(0);
  });
});

describe('Buscar el servidor', () => {
  it('dice que la terminal aún no está conectada y que los datos de servidor se conservan', () => {
    render(
      <FindServerPage
        state={{
          ...installation,
          mode: 'terminal-only',
          server_configured: false,
          service_enabled: false,
          server_data_kept: true,
        }}
      />,
    );
    expect(screen.getByRole('status').textContent).toContain(
      'todavía no está conectada al servidor del restaurante',
    );
    expect(screen.getByRole('note').textContent).toContain('sus datos se conservan');
  });

  it('vuelve a ser servidor solo después de confirmar', async () => {
    render(
      <FindServerPage
        state={{ ...installation, mode: 'terminal-only', server_configured: false }}
      />,
    );
    fireEvent.click(screen.getByRole('button', { name: 'Usar esta computadora como servidor' }));
    const dialog = screen.getByRole('alertdialog', { name: 'Usar esta computadora como servidor' });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Seguir como terminal' }));
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull());
    expect(calls('set_installation_mode')).toHaveLength(0);
    fireEvent.click(screen.getByRole('button', { name: 'Usar esta computadora como servidor' }));
    fireEvent.click(screen.getByRole('button', { name: 'Usar como servidor' }));
    await waitFor(() => expect(calls('restart_application')).toHaveLength(1));
    expect(calls('set_installation_mode')[0]![1]).toEqual({ mode: 'server-terminal' });
  });

  it('sin poder cambiar la modalidad no ofrece el cambio', () => {
    render(
      <FindServerPage
        state={{
          ...installation,
          mode: 'terminal-only',
          server_configured: false,
          can_change: false,
        }}
      />,
    );
    // Desde la fase B la pantalla pide el código del emparejamiento («Pedir código»); lo que no se
    // ofrece es volver a ser servidor.
    expect(
      screen.queryByRole('button', { name: 'Usar esta computadora como servidor' }),
    ).toBeNull();
  });
});

describe('Solo servidor', () => {
  it('la pantalla de estado es propia de «Solo servidor» y no exige permisos', () => {
    const entry = sectionByID('server-status');
    expect(entry.serverOnly).toBe(true);
    expect(canOpen('server-status', [])).toBe(true);
  });

  it('muestra el servidor activo, las terminales conectadas, el hueco del respaldo y Administración', async () => {
    const onAdministration = vi.fn();
    render(<ServerOnlyPage canManage canAdminister onAdministration={onAdministration} />);
    expect(await screen.findByText('Activo desde hace 2 horas y 5 minutos.')).toBeTruthy();
    expect(screen.getByText('2 terminales conectadas de 3.')).toBeTruthy();
    expect(screen.getByText('Sin registro')).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: 'Abrir Administración' }));
    expect(onAdministration).toHaveBeenCalledTimes(1);
  });

  it('sin settings.manage no consulta el servicio ni ofrece Administración si no la puede abrir', () => {
    render(<ServerOnlyPage canManage={false} canAdminister={false} onAdministration={vi.fn()} />);
    expect(requests('maintenance_service')).toHaveLength(0);
    expect(screen.getByText(/Solo quien administra la configuración/)).toBeTruthy();
    expect(screen.queryByRole('button', { name: 'Abrir Administración' })).toBeNull();
  });
});

describe('Mantenimiento › Servicio del servidor', () => {
  it('explica el servicio y reinicia solo después de confirmar', async () => {
    render(<ServiceSection />);
    const state = await screen.findByText(/arranca solo al encenderla, aunque nadie inicie sesión/);
    expect(state).toBeTruthy();
    expect(screen.getByText(/Activo desde hace 2 horas y 5 minutos\. 2 terminales/)).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: 'Reiniciar' }));
    const dialog = screen.getByRole('alertdialog', { name: 'Reiniciar el servidor' });
    expect(dialog.className).not.toContain('dialog-panel-danger');
    expect(dialog.textContent).toContain('se reconectan solas');
    fireEvent.click(within(dialog).getByRole('button', { name: 'Reiniciar el servidor' }));
    expect(await screen.findByText('El servidor se está reiniciando.')).toBeTruthy();
    expect(requests('maintenance_service_restart')).toHaveLength(1);
  });

  it('un servidor que no es servicio de la computadora no se reinicia desde aquí', async () => {
    service = { ...managedService, managed: false, restart_available: false };
    render(<ServiceSection />);
    expect(
      await screen.findByText(/no está instalado como servicio de la computadora/),
    ).toBeTruthy();
    expect(screen.queryByRole('button', { name: 'Reiniciar' })).toBeNull();
  });
});

describe('Mantenimiento › Modalidad de esta computadora', () => {
  it('cambia de modalidad con confirmación sin borrar la base y reinicia la aplicación', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    render(<ModalitySection />);
    const current = await screen.findByRole('radio', { name: /Servidor y terminal/ });
    expect(current.textContent).toContain('Actual');
    const change = screen.getByRole('button', {
      name: 'Cambiar la modalidad',
    }) as HTMLButtonElement;
    expect(change.disabled).toBe(true);
    fireEvent.click(screen.getByRole('radio', { name: /Solo terminal/ }));
    fireEvent.click(change);
    const dialog = screen.getByRole('alertdialog', { name: 'Dejar de ser el servidor' });
    expect(dialog.className).not.toContain('dialog-panel-danger');
    expect(dialog.textContent).toContain('La base de datos se conserva en esta computadora');
    expect(dialog.textContent).toContain('no se borra nada');
    fireEvent.click(within(dialog).getByRole('button', { name: 'Conservar la modalidad' }));
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull());
    expect(calls('set_installation_mode')).toHaveLength(0);
    fireEvent.click(change);
    fireEvent.click(screen.getByRole('button', { name: 'Cambiar a «Solo terminal»' }));
    expect(await screen.findByText('Modalidad cambiada a «Solo terminal».')).toBeTruthy();
    expect(calls('set_installation_mode')[0]![1]).toEqual({ mode: 'terminal-only' });
    vi.advanceTimersByTime(2000);
    await waitFor(() => expect(calls('restart_application')).toHaveLength(1));
  });

  it('a «Solo servidor» explica que no se venderá en esta computadora', async () => {
    render(<ModalitySection />);
    fireEvent.click(await screen.findByRole('radio', { name: /Solo servidor/ }));
    fireEvent.click(screen.getByRole('button', { name: 'Cambiar la modalidad' }));
    const dialog = screen.getByRole('alertdialog', { name: 'Cambiar la modalidad' });
    expect(dialog.textContent).toContain('ya no se venderá');
    expect(dialog.textContent).toContain('El servidor sigue atendiendo a las terminales');
  });

  it('sin poder cambiarla (Windows) solo informa la modalidad', async () => {
    installation = { ...installation, can_change: false };
    render(<ModalitySection />);
    expect(await screen.findByText(/se elige al instalar AD-EL POS/)).toBeTruthy();
    expect(screen.queryByRole('radio')).toBeNull();
  });
});
