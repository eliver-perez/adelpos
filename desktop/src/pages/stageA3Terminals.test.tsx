// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { Device } from '../services/models';
import {
  devicePrecheckPrinterCapability,
  readyAlertsCapability,
  rememberServerCapabilities,
  waiterPINCapability,
} from '../services/serverCapabilities';
import { DevicesPage, terminalHomeOptions } from './DevicesPage';

// Etapa A3 (§5 de la especificación, A3-D7): en Sistema › Terminales cada tipo de terminal solo ve lo que le aplica.
// Un teléfono o tableta de meseros (android) elige su pantalla inicial entre Mesas y Para llevar, sin caja física,
// teclado virtual, estaciones de Cocina, aviso sonoro ni impresoras de la terminal; en la tabla su caja es un guion. El
// identificador salió de la tabla y queda en «Ver configuración» como «Identificador». Las terminales de escritorio y de
// cocina conservan todo.

const native = vi.hoisted(() => ({ invoke: vi.fn() }));
vi.mock('@tauri-apps/api/core', () => ({ invoke: native.invoke, isTauri: () => true }));

type Request = { operation: string; resourceId: string | null; body?: Record<string, unknown> };

const tablet: Device = {
  id: '00000000-0000-4000-8000-0000000000b1',
  name: 'Tableta 1',
  register_id: '00000000-0000-4000-8000-0000000000a1',
  register_name: 'Caja 1',
  keyboard_mode: 'disabled',
  home_screen: 'takeaway',
  kind: 'android',
  os: 'Android 15',
  app_version: '0.3.0',
  paired_at: '2026-11-19T15:00:00Z',
  online: true,
  current_user: null,
  idle_lock_seconds: 120,
  precheck_printer_id: null,
};
const desk: Device = {
  ...tablet,
  id: '00000000-0000-4000-8000-0000000000b2',
  name: 'Caja de la barra',
  kind: 'desktop',
  os: 'Windows',
  register_id: null,
  register_name: null,
  keyboard_mode: 'auto',
  home_screen: 'cash',
};
// Desde la etapa K1 el tipo kitchen es la tableta con la aplicación de cocina: tiene su propio editor (stageK1Terminals).
const kitchenScreen: Device = {
  ...desk,
  id: '00000000-0000-4000-8000-0000000000b3',
  name: 'Tableta de cocina',
  kind: 'kitchen',
  home_screen: 'kitchen',
};

let requests: Request[];
let listed: Device[];

function sent(operation: string) {
  return requests.filter((request) => request.operation === operation);
}

beforeEach(() => {
  rememberServerCapabilities([
    'restaurant-operations',
    waiterPINCapability,
    devicePrecheckPrinterCapability,
    readyAlertsCapability,
  ]);
  requests = [];
  listed = [tablet, desk, kitchenScreen];
  native.invoke.mockImplementation(async (command: string, args: Request) => {
    if (command !== 'api_request') {
      return null;
    }
    requests.push(args);
    switch (args.operation) {
      case 'devices':
        return { status: 200, body: listed };
      case 'registers':
        return {
          status: 200,
          body: [{ id: tablet.register_id, name: 'Caja 1', description: '', active: true }],
        };
      case 'devices_pairing_requests':
        return { status: 200, body: { requests: [], approve_here: true, pairing_available: true } };
      case 'devices_sessions_summary':
        return {
          status: 200,
          body: { by_kind: { desktop: 1, android: 1, kitchen: 0 }, total: 2, limited: false },
        };
      case 'device_configuration': {
        const device = listed.find((candidate) => candidate.id === args.resourceId) ?? desk;
        return {
          status: 200,
          body: {
            device,
            kitchen_all_stations: true,
            kitchen_sound_alert: true,
            kitchen_stations: [],
            printers: [],
            local_printers: {
              agent_online: false,
              agent_seen_at: null,
              reported_at: null,
              queues: [],
            },
            precheck_printer: null,
            precheck_printer_options: [],
          },
        };
      }
      case 'update_device':
        return { status: 200, body: tablet };
    }
    return { status: 200, body: [] };
  });
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  rememberServerCapabilities([]);
});

async function row(name: string) {
  return (await screen.findByText(name)).closest('tr') as HTMLElement;
}

async function edit(name: string) {
  fireEvent.click(within(await row(name)).getByRole('button', { name: 'Editar' }));
  return screen.getByRole('group', { name: 'Pantalla inicial de la terminal' });
}

function homeOptions(group: HTMLElement) {
  return within(group)
    .getAllByRole('button')
    .map((button) => button.textContent);
}

function pressedHome(group: HTMLElement) {
  return within(group)
    .getAllByRole('button')
    .find((button) => button.getAttribute('aria-pressed') === 'true')?.textContent;
}

async function viewConfiguration(name: string) {
  fireEvent.click(within(await row(name)).getByRole('button', { name: 'Ver configuración' }));
  const panel = await screen.findByRole('region', { name: `Configuración de ${name}` });
  await waitFor(() => expect(panel.textContent).toContain('Identificador'));
  return panel;
}

describe('Terminales: cada tipo solo ve lo que le aplica (etapa A3, §5)', () => {
  it('un teléfono o tableta elige solo Mesas o Para llevar, sin caja, teclado ni nota de Cocina', async () => {
    render(<DevicesPage can={() => true} />);
    const homes = await edit('Tableta 1');
    expect(homeOptions(homes)).toEqual(['Mesas', 'Para llevar']);
    expect(pressedHome(homes)).toBe('Para llevar');
    expect(screen.queryByRole('combobox', { name: 'Caja física' })).toBeNull();
    expect(screen.queryByRole('combobox', { name: 'Teclado virtual' })).toBeNull();
    expect(screen.queryByText(/Con «Cocina» abre en Preparación/)).toBeNull();
    fireEvent.click(within(homes).getByRole('button', { name: 'Mesas' }));
    fireEvent.click(screen.getByRole('button', { name: 'Guardar cambios' }));
    await screen.findByText('Terminal actualizada.');
    // La caja y el teclado guardados viajan sin cambio: editar el teléfono nunca le cambia la caja.
    expect(sent('update_device')[0]).toMatchObject({
      resourceId: tablet.id,
      body: {
        name: 'Tableta 1',
        home_screen: 'tables',
        register_id: tablet.register_id,
        keyboard_mode: 'disabled',
      },
    });
  });

  it('un teléfono con «Cocina» o «Caja» guardada (servidor anterior) abre el editor en Mesas', async () => {
    listed = [{ ...tablet, home_screen: 'kitchen' }, desk];
    render(<DevicesPage can={() => true} />);
    const homes = await edit('Tableta 1');
    expect(pressedHome(homes)).toBe('Mesas');
    fireEvent.click(screen.getByRole('button', { name: 'Guardar cambios' }));
    await screen.findByText('Terminal actualizada.');
    expect(sent('update_device')[0]?.body).toHaveProperty('home_screen', 'tables');
  });

  it('las terminales de escritorio conservan las cuatro pantallas, la caja y el teclado; la pantalla de cocina no tiene ninguna', async () => {
    render(<DevicesPage can={() => true} />);
    const homes = await edit('Caja de la barra');
    expect(homeOptions(homes)).toEqual(['Mesas', 'Para llevar', 'Cocina', 'Caja']);
    expect(pressedHome(homes)).toBe('Caja');
    expect(screen.getByRole('combobox', { name: 'Caja física' })).toBeTruthy();
    expect(screen.getByRole('combobox', { name: 'Teclado virtual' })).toBeTruthy();
    expect(screen.getByText(/Con «Cocina» abre en Preparación/)).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: 'Cerrar editor' }));
    // Etapa K1: la tableta de cocina solo muestra y marca pedidos: sin pantalla inicial, caja ni teclado.
    fireEvent.click(within(await row('Tableta de cocina')).getByRole('button', { name: 'Editar' }));
    expect(
      await screen.findByRole('group', { name: 'Tamaño de letra de la pantalla' }),
    ).toBeTruthy();
    expect(screen.queryByRole('group', { name: 'Pantalla inicial de la terminal' })).toBeNull();
    expect(screen.queryByRole('combobox', { name: 'Caja física' })).toBeNull();
    expect(screen.queryByRole('combobox', { name: 'Teclado virtual' })).toBeNull();
    expect(terminalHomeOptions(kitchenScreen)).toHaveLength(4);
    expect(terminalHomeOptions(tablet).map((option) => option.value)).toEqual([
      'tables',
      'takeaway',
    ]);
  });

  it('en la tabla la caja de un teléfono es un guion y ninguna fila muestra el identificador', async () => {
    render(<DevicesPage can={() => true} />);
    const phone = await row('Tableta 1');
    const cells = within(phone).getAllByRole('cell');
    expect(cells[2]?.textContent).toBe('—');
    expect(within(await row('Caja de la barra')).getAllByRole('cell')[2]?.textContent).toBe(
      'Sin asociación',
    );
    const table = phone.closest('table') as HTMLElement;
    expect(table.textContent).not.toContain('Código');
    for (const device of listed) {
      expect(table.textContent).not.toContain(device.id);
    }
  });

  it('«Ver configuración» de un teléfono dice su identificador y solo lo que le aplica', async () => {
    render(<DevicesPage can={() => true} />);
    const panel = await viewConfiguration('Tableta 1');
    expect(panel.textContent).toContain(`Identificador${tablet.id}`);
    expect(panel.textContent).toContain('Pantalla inicialPara llevar');
    expect(panel.textContent).toContain('Bloqueo por inactividad');
    for (const missing of [
      'Caja física',
      'Teclado virtual',
      'Estaciones que muestra en Cocina',
      'Aviso sonoro de comandas nuevas',
      'Impresoras de esta terminal',
      'Impresoras de la computadora',
      'Terminales de cocina',
    ]) {
      expect(panel.textContent).not.toContain(missing);
    }
  });

  it('«Ver configuración» de una terminal de escritorio conserva todo y agrega el identificador', async () => {
    render(<DevicesPage can={() => true} />);
    const panel = await viewConfiguration('Caja de la barra');
    expect(panel.textContent).toContain(`Identificador${desk.id}`);
    for (const present of [
      'Caja física',
      'Teclado virtual',
      'Estaciones que muestra en Cocina',
      'Aviso sonoro de comandas nuevas',
      'Impresoras de esta terminal',
      'Impresoras de la computadora',
      'Terminales de cocina',
    ]) {
      expect(panel.textContent).toContain(present);
    }
    expect(panel.textContent).not.toContain('Bloqueo por inactividad');
  });
});
