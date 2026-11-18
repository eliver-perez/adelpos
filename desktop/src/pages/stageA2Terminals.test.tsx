// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { Device, PrecheckPrinterOption } from '../services/models';
import {
  devicePrecheckPrinterCapability,
  rememberServerCapabilities,
  waiterPINCapability,
} from '../services/serverCapabilities';
import { chooseOption, findOption, optionLabels, selectText } from '../testing/select';
import { DevicesPage, precheckPrinterText } from './DevicesPage';

// Etapa A2, parte C: Terminales › Editar de un teléfono o tableta de meseros elige su «Impresora de precuentas» (las
// impresoras de red y las USB de una terminal, con el nombre de su terminal; «Sin impresora»), y «Ver configuración» la
// informa. Las impresoras que no sirven las dice Go (problem) y se ven deshabilitadas. Todo aparece solo si el servidor
// anuncia device-precheck-printer-v1: con uno anterior no se ofrece ni se envía el campo.

const native = vi.hoisted(() => ({ invoke: vi.fn() }));
vi.mock('@tauri-apps/api/core', () => ({ invoke: native.invoke, isTauri: () => true }));

type Request = { operation: string; resourceId: string | null; body?: Record<string, unknown> };

const usb: PrecheckPrinterOption = {
  id: '00000000-0000-4000-8000-0000000000c1',
  name: 'Tickets USB',
  transport: 'agent',
  terminal_name: 'Caja 1',
  active: true,
  problem: '',
};
const network: PrecheckPrinterOption = {
  id: '00000000-0000-4000-8000-0000000000c2',
  name: 'Barra',
  transport: 'escpos_tcp',
  terminal_name: '',
  active: true,
  problem: '',
};
const off: PrecheckPrinterOption = {
  id: '00000000-0000-4000-8000-0000000000c3',
  name: 'Apagada',
  transport: 'escpos_tcp',
  terminal_name: '',
  active: false,
  problem: 'inactive',
};

const tablet: Device = {
  id: '00000000-0000-4000-8000-0000000000b1',
  name: 'Tableta 1',
  register_id: null,
  keyboard_mode: 'auto',
  home_screen: 'tables',
  kind: 'android',
  os: 'Android 15',
  app_version: '0.2.0',
  paired_at: '2026-11-18T15:00:00Z',
  online: true,
  current_user: null,
  idle_lock_seconds: 120,
  precheck_printer_id: usb.id,
  precheck_printer_name: usb.name,
  precheck_printer_terminal_name: usb.terminal_name,
};
const desk: Device = {
  ...tablet,
  id: '00000000-0000-4000-8000-0000000000b2',
  name: 'Caja de la barra',
  kind: 'desktop',
  os: 'Windows',
  precheck_printer_id: null,
  precheck_printer_name: null,
  precheck_printer_terminal_name: null,
};

let requests: Request[];
let assigned: PrecheckPrinterOption | null;

function sent(operation: string) {
  return requests.filter((request) => request.operation === operation);
}

beforeEach(() => {
  rememberServerCapabilities([
    'restaurant-operations',
    waiterPINCapability,
    devicePrecheckPrinterCapability,
  ]);
  requests = [];
  assigned = usb;
  native.invoke.mockImplementation(async (command: string, args: Request) => {
    if (command !== 'api_request') {
      return null;
    }
    requests.push(args);
    switch (args.operation) {
      case 'devices':
        return { status: 200, body: [tablet, desk] };
      case 'devices_pairing_requests':
        return { status: 200, body: { requests: [], approve_here: true, pairing_available: true } };
      case 'devices_sessions_summary':
        return {
          status: 200,
          body: { by_kind: { desktop: 1, android: 1, kitchen: 0 }, total: 2, limited: false },
        };
      case 'device_configuration': {
        const android = args.resourceId === tablet.id;
        return {
          status: 200,
          body: {
            device: android ? tablet : desk,
            kitchen_all_stations: true,
            kitchen_sound_alert: false,
            kitchen_stations: [],
            printers: [],
            local_printers: null,
            precheck_printer: android ? assigned : null,
            precheck_printer_options: android ? [network, off, usb] : [],
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

async function edit(name: string) {
  const row = (await screen.findByText(name)).closest('tr') as HTMLElement;
  fireEvent.click(within(row).getByRole('button', { name: 'Editar' }));
}

describe('Terminales: impresora de precuentas de un teléfono o tableta', () => {
  it('se elige entre las impresoras con su terminal; las que no sirven se ven deshabilitadas', async () => {
    render(<DevicesPage can={() => true} />);
    await edit('Tableta 1');
    const select = screen.getByRole('combobox', { name: 'Impresora de precuentas' });
    expect(selectText(select)).toBe('Tickets USB · en Caja 1');
    await findOption(select, 'Barra');
    expect(optionLabels(select)).toEqual([
      'Sin impresora',
      'Barra',
      'Apagada',
      'Tickets USB · en Caja 1',
    ]);
    const disabled = await findOption(select, /Apagada/);
    expect(disabled.getAttribute('aria-disabled')).toBe('true');
    expect(disabled.textContent).toContain('desactivada');
    chooseOption(select, 'Barra');
    fireEvent.click(screen.getByRole('button', { name: 'Guardar cambios' }));
    await screen.findByText('Terminal actualizada.');
    expect(sent('update_device')[0]).toMatchObject({
      resourceId: tablet.id,
      body: { name: 'Tableta 1', precheck_printer_id: network.id },
    });
  });

  it('«Sin impresora» la quita (null)', async () => {
    render(<DevicesPage can={() => true} />);
    await edit('Tableta 1');
    const select = screen.getByRole('combobox', { name: 'Impresora de precuentas' });
    await findOption(select, 'Barra');
    chooseOption(select, 'Sin impresora');
    fireEvent.click(screen.getByRole('button', { name: 'Guardar cambios' }));
    await screen.findByText('Terminal actualizada.');
    expect(sent('update_device')[0]?.body).toHaveProperty('precheck_printer_id', null);
  });

  it('una terminal de escritorio no la tiene y no se envía', async () => {
    render(<DevicesPage can={() => true} />);
    await edit('Caja de la barra');
    expect(screen.queryByRole('combobox', { name: 'Impresora de precuentas' })).toBeNull();
    fireEvent.click(screen.getByRole('button', { name: 'Guardar cambios' }));
    await screen.findByText('Terminal actualizada.');
    expect(sent('update_device')[0]?.body).not.toHaveProperty('precheck_printer_id');
    expect(sent('device_configuration')).toHaveLength(0);
  });

  it('con un servidor anterior no se ofrece ni se envía', async () => {
    rememberServerCapabilities(['restaurant-operations', waiterPINCapability]);
    render(<DevicesPage can={() => true} />);
    await edit('Tableta 1');
    expect(screen.queryByRole('combobox', { name: 'Impresora de precuentas' })).toBeNull();
    fireEvent.click(screen.getByRole('button', { name: 'Guardar cambios' }));
    await screen.findByText('Terminal actualizada.');
    expect(sent('update_device')[0]?.body).not.toHaveProperty('precheck_printer_id');
    expect(sent('device_configuration')).toHaveLength(0);
  });

  it('«Ver configuración» dice cuál es, con su terminal, o que no tiene', async () => {
    render(<DevicesPage can={() => true} />);
    const row = (await screen.findByText('Tableta 1')).closest('tr') as HTMLElement;
    fireEvent.click(within(row).getByRole('button', { name: 'Ver configuración' }));
    const configuration = await screen.findByRole('region', { name: 'Configuración de Tableta 1' });
    await waitFor(() =>
      expect(configuration.textContent).toContain('Impresora de precuentasTickets USB · en Caja 1'),
    );
    expect(precheckPrinterText(null)).toBe('Sin impresora: este dispositivo no imprime precuentas');
    expect(precheckPrinterText(off)).toBe('Apagada (desactivada)');
  });
});
