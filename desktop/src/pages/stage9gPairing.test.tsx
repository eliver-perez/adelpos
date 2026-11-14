// @vitest-environment jsdom

import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { InstallationState } from '../services/installation';
import type { PairingRequest, PairingRequests } from '../services/models';
import type { PairingView } from '../services/pairing';
import { serverTerminalInstallation } from '../testing/installation';
import { DevicesPage } from './DevicesPage';
import { FindServerPage, pairingPollMilliseconds } from './setup/FindServerPage';
import { RepairTerminalPage } from './setup/RepairTerminal';

// Etapa 9G, fase B (§3.2–§3.4): la terminal nueva escribe la dirección, pide su código y lo muestra en
// grande con su nombre (editable) mientras espera; en la computadora principal se aprueba escribiendo ese
// código (solo ahí), se rechaza, se ve la configuración de cada terminal y las sesiones por tipo. Rust hace
// el emparejamiento y guarda la credencial; React solo propone la dirección y muestra el estado.

const native = vi.hoisted(() => ({ invoke: vi.fn() }));
vi.mock('@tauri-apps/api/core', () => ({ invoke: native.invoke, isTauri: () => true }));

type Reply = { status: number; body: unknown };
let replies: Record<string, Reply>;
let pairing: PairingView;
let statusQueue: PairingView[];
let startFailure: string | null;

const terminalOnly: InstallationState = {
  ...serverTerminalInstallation,
  mode: 'terminal-only',
  server_configured: false,
  service_installed: false,
  service_enabled: false,
};

const pendingView: PairingView = {
  state: 'pending',
  code: '481926',
  name: 'Caja 2',
  server_name: 'Fonda Doña Ñeli',
  expires_at: new Date(Date.now() + 5 * 60 * 1000).toISOString(),
};

const request: PairingRequest = {
  id: '00000000-0000-4000-8000-0000000000b1',
  name: 'Caja 2',
  device_kind: 'desktop',
  os: 'macOS',
  app_version: '0.10.0',
  created_at: '2026-11-13T18:00:00Z',
  expires_at: '2026-11-13T18:05:00Z',
  ready: true,
  attempts_left: 5,
};

const pending = (overrides: Partial<PairingRequests> = {}): Reply => ({
  status: 200,
  body: {
    requests: [request],
    approve_here: true,
    network: { available: true, port: 8443, addresses: ['192.168.1.20', 'caja.local'] },
    ...overrides,
  },
});

const device = {
  id: '00000000-0000-4000-8000-0000000000d2',
  name: 'Caja de la barra',
  register_id: null,
  register_name: null,
  keyboard_mode: 'auto',
  home_screen: 'tables',
  last_seen_at: '2026-11-13T18:00:00Z',
  revoked_at: null,
  kind: 'desktop',
  os: 'Windows',
  app_version: '0.10.0',
  paired_at: '2026-11-13T17:00:00Z',
  online: true,
  current_user: 'Mesero Uno',
};

function calls(command: string) {
  return native.invoke.mock.calls.filter(([name]) => name === command);
}

function requests(operation: string) {
  return native.invoke.mock.calls
    .filter(([command, args]) => command === 'api_request' && args?.operation === operation)
    .map(([, args]) => args as { resourceId: string | null; body: unknown });
}

beforeEach(() => {
  pairing = { ...pendingView };
  statusQueue = [];
  startFailure = null;
  replies = {
    devices: { status: 200, body: [device] },
    registers: { status: 200, body: [] },
    devices_pairing_requests: pending(),
    devices_sessions_summary: {
      status: 200,
      body: { by_kind: { desktop: 3, android: 1, kitchen: 0 }, total: 4, limited: false },
    },
    approve_device_pairing: { status: 200, body: { state: 'approved', name: 'Caja 2' } },
    reject_device_pairing: { status: 200, body: { state: 'rejected' } },
    device_configuration: {
      status: 200,
      body: {
        device,
        kitchen_all_stations: false,
        kitchen_sound_alert: true,
        kitchen_stations: [{ id: 's1', name: 'Plancha', active: true }],
        printers: [],
      },
    },
  };
  native.invoke.mockImplementation(async (command: string, args: Record<string, unknown>) => {
    if (command === 'pairing_start') {
      if (startFailure) {
        throw startFailure;
      }
      pairing = { ...pairing, name: String(args.name) };
      return pairing;
    }
    if (command === 'pairing_status') {
      return statusQueue.shift() ?? pairing;
    }
    if (command === 'pairing_rename') {
      pairing = { ...pairing, name: String(args.name) };
      return pairing;
    }
    if (command === 'pairing_cancel' || command === 'pairing_forget') {
      return null;
    }
    if (command === 'restart_application') {
      return null;
    }
    if (command !== 'api_request') {
      throw new Error(`Comando no esperado: ${command}`);
    }
    return replies[String(args.operation)] ?? { status: 200, body: {} };
  });
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.clearAllMocks();
});

async function requestCode(address = '192.168.1.20') {
  render(<FindServerPage state={terminalOnly} />);
  fireEvent.change(screen.getByRole('textbox', { name: 'Dirección de la computadora principal' }), {
    target: { value: address },
  });
  fireEvent.click(screen.getByRole('button', { name: 'Pedir código' }));
}

describe('«Buscar el servidor»: dirección escrita a mano y código (etapa 9G, fase B)', () => {
  it('propone «Caja 2», exige la dirección y muestra el código en grande con el restaurante', async () => {
    render(<FindServerPage state={terminalOnly} />);
    const name = screen.getByRole('textbox', { name: 'Nombre de esta terminal' });
    expect((name as HTMLInputElement).value).toBe('Caja 2');
    const ask = screen.getByRole('button', { name: 'Pedir código' }) as HTMLButtonElement;
    expect(ask.disabled).toBe(true);
    fireEvent.change(name, { target: { value: 'Caja de la terraza' } });
    fireEvent.change(
      screen.getByRole('textbox', { name: 'Dirección de la computadora principal' }),
      {
        target: { value: 'caja.local' },
      },
    );
    fireEvent.click(ask);
    const code = await screen.findByLabelText('Código de emparejamiento 4 8 1 9 2 6');
    expect(code.textContent).toBe('481 926');
    expect(calls('pairing_start')[0]?.[1]).toEqual({
      address: 'caja.local',
      name: 'Caja de la terraza',
    });
    expect(screen.getByText('Fonda Doña Ñeli')).toBeTruthy();
    expect(screen.getByText('Caja de la terraza')).toBeTruthy();
    expect(screen.getByRole('status').textContent).toContain('Esperando la aprobación');
    expect(screen.getByRole('timer').textContent).toMatch(/El código vence en [45]:\d\d/);
    // Nada técnico: ni la dirección ni el secreto se muestran en el código.
    expect(document.body.textContent).not.toContain('8443');
  });

  it('explica el rechazo de la dirección que da Rust y conserva lo escrito', async () => {
    startFailure =
      'La dirección debe ser de la red local del restaurante (por ejemplo 192.168.1.20 o caja.local).';
    await requestCode('8.8.8.8');
    expect(await screen.findByText(/red local del restaurante/)).toBeTruthy();
    expect(
      (
        screen.getByRole('textbox', {
          name: 'Dirección de la computadora principal',
        }) as HTMLInputElement
      ).value,
    ).toBe('8.8.8.8');
  });

  it('consulta la aprobación y, emparejada, reinicia la aplicación', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    await requestCode();
    await screen.findByLabelText(/Código de emparejamiento/);
    statusQueue = [{ ...pairing }, { ...pairing, state: 'paired' }];
    await act(async () => {
      await vi.advanceTimersByTimeAsync(pairingPollMilliseconds);
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(pairingPollMilliseconds);
    });
    expect(screen.getByRole('status').textContent).toContain('Terminal emparejada');
    expect(calls('restart_application')).toHaveLength(0);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1600);
    });
    expect(calls('restart_application')).toHaveLength(1);
    expect(calls('pairing_status').length).toBeGreaterThanOrEqual(2);
  });

  // Hallazgo 3 de la revisión: una consulta lenta (el diálogo del Llavero al guardar la credencial) no se
  // solapa con la siguiente, y un error no detiene la espera.
  it('encadena las consultas: nunca dos a la vez y sigue esperando tras un error', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    await requestCode();
    await screen.findByLabelText(/Código de emparejamiento/);
    let release: (value: PairingView) => void = () => undefined;
    const slow = new Promise<PairingView>((resolve) => {
      release = resolve;
    });
    let pending = 0;
    native.invoke.mockImplementation(async (command: string) => {
      if (command === 'pairing_status') {
        pending += 1;
        if (pending === 1) {
          return slow;
        }
        if (pending === 2) {
          throw 'No se pudo guardar la credencial en el almacén seguro del sistema.';
        }
        return { ...pairing, state: 'paired' };
      }
      return null;
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(pairingPollMilliseconds * 4);
    });
    expect(calls('pairing_status')).toHaveLength(1);
    await act(async () => {
      release({ ...pairing });
      await vi.advanceTimersByTimeAsync(pairingPollMilliseconds);
    });
    expect(calls('pairing_status')).toHaveLength(2);
    expect(await screen.findByText(/almacén seguro/)).toBeTruthy();
    await act(async () => {
      await vi.advanceTimersByTimeAsync(pairingPollMilliseconds);
    });
    expect(calls('pairing_status')).toHaveLength(3);
    expect(screen.getByRole('status').textContent).toContain('Terminal emparejada');
  });

  it('rechazada o vencida ofrece pedir otro código', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    await requestCode();
    await screen.findByLabelText(/Código de emparejamiento/);
    statusQueue = [{ ...pairing, state: 'rejected' }];
    await act(async () => {
      await vi.advanceTimersByTimeAsync(pairingPollMilliseconds);
    });
    expect(screen.getByRole('status').textContent).toContain('rechazó esta terminal');
    expect(screen.queryByLabelText(/Código de emparejamiento/)).toBeNull();
    fireEvent.click(screen.getByRole('button', { name: 'Pedir otro código' }));
    expect(screen.getByRole('button', { name: 'Pedir código' })).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: 'Pedir código' }));
    await screen.findByLabelText(/Código de emparejamiento/);
    statusQueue = [{ ...pairing, state: 'expired' }];
    await act(async () => {
      await vi.advanceTimersByTimeAsync(pairingPollMilliseconds);
    });
    expect(screen.getByRole('status').textContent).toContain('El código venció');
  });

  it('cambia el nombre mientras espera y cancela la solicitud', async () => {
    await requestCode();
    await screen.findByLabelText(/Código de emparejamiento/);
    fireEvent.click(screen.getByRole('button', { name: 'Cambiar nombre' }));
    fireEvent.change(screen.getByRole('textbox', { name: 'Nombre de esta terminal' }), {
      target: { value: 'Caja 3' },
    });
    fireEvent.click(screen.getByRole('button', { name: 'Guardar nombre' }));
    await screen.findByText('Caja 3');
    expect(calls('pairing_rename')[0]?.[1]).toEqual({ name: 'Caja 3' });
    fireEvent.click(screen.getByRole('button', { name: 'Cancelar' }));
    await screen.findByRole('button', { name: 'Pedir código' });
    expect(calls('pairing_cancel')).toHaveLength(1);
  });
});

describe('Sistema › Terminales con solicitudes, configuración y sesiones (etapa 9G, fase B)', () => {
  it('aprueba escribiendo el código de la terminal y muestra cómo escribir la dirección', async () => {
    render(<DevicesPage can={() => true} />);
    const item = (await screen.findByText('Caja 2')).closest('li') as HTMLElement;
    expect(item.textContent).toContain('macOS');
    expect(screen.getByText('192.168.1.20 o caja.local')).toBeTruthy();
    const input = within(item).getByRole('textbox', { name: /^Código que muestra Caja 2/ });
    const approve = within(item).getByRole('button', { name: 'Aprobar' }) as HTMLButtonElement;
    expect(approve.disabled).toBe(true);
    fireEvent.change(input, { target: { value: '481 926' } });
    expect(approve.disabled).toBe(false);
    fireEvent.click(approve);
    const notice = await screen.findByRole('status', { name: 'Operación completada' });
    expect(notice.textContent).toContain('Terminal Caja 2 aprobada');
    expect(requests('approve_device_pairing')[0]).toMatchObject({
      resourceId: request.id,
      body: { code: '481926' },
    });
  });

  it('un código incorrecto avisa sin el código interno y muestra los intentos que quedan', async () => {
    replies.approve_device_pairing = {
      status: 400,
      body: {
        error: {
          code: 'PAIRING_CODE_INVALID',
          message: 'El código no coincide con el de la terminal. Revísalo e inténtalo de nuevo.',
          details: { attempts_left: 4 },
        },
      },
    };
    render(<DevicesPage can={() => true} />);
    const item = (await screen.findByText('Caja 2')).closest('li') as HTMLElement;
    fireEvent.change(within(item).getByRole('textbox', { name: /^Código que muestra Caja 2/ }), {
      target: { value: '000000' },
    });
    replies.devices_pairing_requests = pending({ requests: [{ ...request, attempts_left: 4 }] });
    fireEvent.click(within(item).getByRole('button', { name: 'Aprobar' }));
    const alert = await screen.findByRole('alert', { name: 'Error' });
    expect(alert.textContent).toContain('no coincide');
    expect(alert.textContent).not.toContain('PAIRING_CODE_INVALID');
    expect(await screen.findByText('Quedan 4 intentos antes de rechazarla.')).toBeTruthy();
  });

  // Revisión de la fase B (hallazgos 1 y 4): la computadora principal ve las solicitudes abandonadas, el tope
  // alcanzado y si no puede emparejar por la identidad.
  it('avisa de solicitudes abandonadas, del tope y de la identidad que necesita revisión', async () => {
    replies.devices_pairing_requests = pending({
      requests: [],
      recent_abandoned: 7,
      limit_reached: true,
      window_minutes: 10,
      pairing_available: false,
    });
    render(<DevicesPage can={() => true} />);
    const panel = await screen.findByRole('region', { name: 'Terminales nuevas' });
    await waitFor(() => expect(panel.textContent).toContain('7 solicitudes canceladas'));
    expect(panel.textContent).toContain('hacerse pasar por una terminal');
    expect(panel.textContent).toContain('por ahora no se aceptan más solicitudes');
    expect(within(panel).getByRole('alert').textContent).toContain('identidad necesita revisión');
    expect(panel.textContent).not.toContain('todavía no recibe terminales de la red');
  });

  it('en otra terminal no se aprueba: lo explica y no pide el código', async () => {
    replies.devices_pairing_requests = pending({ approve_here: false, network: undefined });
    render(<DevicesPage can={() => true} />);
    expect(
      await screen.findByText(
        'Las terminales nuevas se aprueban en la computadora principal del restaurante.',
      ),
    ).toBeTruthy();
    expect(screen.queryByRole('textbox', { name: /Código que muestra/ })).toBeNull();
    expect(screen.queryByRole('button', { name: 'Aprobar' })).toBeNull();
  });

  it('rechazar pide confirmación', async () => {
    render(<DevicesPage can={() => true} />);
    const item = (await screen.findByText('Caja 2')).closest('li') as HTMLElement;
    fireEvent.click(within(item).getByRole('button', { name: 'Rechazar' }));
    const dialog = screen.getByRole('alertdialog', { name: 'Rechazar Caja 2' });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Rechazar terminal' }));
    await screen.findByText('Se rechazó la terminal Caja 2.');
    expect(requests('reject_device_pairing')[0]?.resourceId).toBe(request.id);
  });

  it('lista tipo, sistema, usuario actual y estado, la configuración y las sesiones por tipo', async () => {
    render(<DevicesPage can={() => true} />);
    const row = (await screen.findByText('Caja de la barra')).closest('tr') as HTMLElement;
    expect(row.textContent).toContain('Escritorio');
    expect(row.textContent).toContain('Windows');
    expect(row.textContent).toContain('Mesero Uno');
    expect(row.textContent).toContain('En línea');
    const summary = screen.getByRole('region', { name: 'Sesiones activas por tipo de terminal' });
    expect(summary.textContent).toContain('Escritorio3');
    expect(summary.textContent).toContain('Comandero Android1');
    expect(summary.textContent).toContain('todavía no se limita ninguna');
    fireEvent.click(within(row).getByRole('button', { name: 'Ver configuración' }));
    const configuration = await screen.findByRole('region', {
      name: 'Configuración de Caja de la barra',
    });
    await waitFor(() => expect(configuration.textContent).toContain('Plancha'));
    expect(configuration.textContent).toContain('Sin impresoras propias');
    expect(configuration.textContent).toContain('Mesas');
    expect(requests('device_configuration')[0]?.resourceId).toBe(device.id);
  });

  it('una terminal revocada se ve así y sin usuario', async () => {
    replies.devices = {
      status: 200,
      body: [{ ...device, revoked_at: '2026-11-13T19:00:00Z', online: false }],
    };
    render(<DevicesPage can={() => true} />);
    const row = (await screen.findByText('Caja de la barra')).closest('tr') as HTMLElement;
    expect(row.textContent).toContain('Revocada');
    expect(row.textContent).not.toContain('Mesero Uno');
    expect(within(row).queryByRole('button', { name: 'Revocar' })).toBeNull();
  });
});

describe('volver a emparejar (etapa 9G, fase B)', () => {
  it('una terminal revocada se empareja de nuevo solo después de confirmar', async () => {
    render(<RepairTerminalPage message="La autorización de esta terminal fue revocada." />);
    expect(screen.getByText('La autorización de esta terminal fue revocada.')).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: 'Emparejar de nuevo' }));
    const dialog = screen.getByRole('alertdialog', { name: 'Emparejar de nuevo' });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Volver' }));
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull());
    expect(calls('pairing_forget')).toHaveLength(0);
    fireEvent.click(screen.getByRole('button', { name: 'Emparejar de nuevo' }));
    fireEvent.click(
      within(screen.getByRole('alertdialog', { name: 'Emparejar de nuevo' })).getByRole('button', {
        name: 'Emparejar de nuevo',
      }),
    );
    await waitFor(() => expect(calls('restart_application')).toHaveLength(1));
    expect(calls('pairing_forget')).toHaveLength(1);
  });
});
