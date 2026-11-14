// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { Printer, PrintJob, TerminalPrinterSet } from '../services/kitchen';
import { printerLabel } from '../services/kitchen';
import { printProblemHint, printProblemMessage, terminalSubject } from '../services/printProblems';
import { chooseValue } from '../testing/select';
import { DevicesPage } from './DevicesPage';
import { PrintQueue } from './kitchen/PrintQueue';
import { PrintersAdmin } from './kitchen/PrintersAdmin';
import { receiptPrinterOptions } from './payments/PaymentPoliciesForm';

// Etapa 9G, fase E (§6, D10 y D13): impresoras USB de las terminales. Sistema › Terminales muestra las impresoras
// que informó cada terminal; al registrar una impresora se elige la terminal y una de sus impresoras (Go decide
// cuáles sirven); al asignar impresoras se ve en qué terminal está cada una, y la cola dice «La Caja 2 está
// desconectada» cuando su terminal no está conectada.

const native = vi.hoisted(() => ({ invoke: vi.fn() }));
vi.mock('@tauri-apps/api/core', () => ({ invoke: native.invoke, isTauri: () => true }));

type Reply = { status: number; body: unknown };
let replies: Record<string, Reply>;

const southDevice = '00000000-0000-4000-8000-0000000000e2';

const terminals: TerminalPrinterSet[] = [
  {
    device_id: southDevice,
    device_name: 'Caja 2',
    agent_online: true,
    agent_seen_at: '2026-11-13T18:00:00Z',
    reported_at: '2026-11-13T17:59:00Z',
    queues: [
      { queue: 'POS_80', model: 'POS-80 Series', connection: 'usb', usable: true },
      {
        queue: 'Oficina',
        model: 'Generic PostScript Printer',
        connection: 'ipp',
        usable: false,
        reason: 'Está conectada por la red como impresora de documentos.',
      },
    ],
    printers: [],
  },
  {
    device_id: '00000000-0000-4000-8000-0000000000e3',
    device_name: 'Barra',
    agent_online: false,
    agent_seen_at: null,
    reported_at: null,
    queues: [],
    printers: [],
  },
];

function printer(overrides: Partial<Printer> = {}): Printer {
  return {
    id: 'usb',
    name: 'USB de caja',
    transport: 'agent',
    destination: 'POS_80',
    port: 9100,
    paper_width: 80,
    copies: 1,
    supports_cut: true,
    cut: true,
    cut_mode: 'full',
    cut_feed_lines: 4,
    cutter_option: '',
    accepts_escpos: false,
    columns: null,
    active: true,
    fallback_id: null,
    revision: 1,
    agent_device_id: southDevice,
    terminal_name: 'Caja 2',
    ...overrides,
  };
}

function job(overrides: Partial<PrintJob> = {}): PrintJob {
  return {
    id: 'job-1',
    ticket_id: 'ticket-1',
    printer_id: 'usb',
    printer_name: 'USB de caja',
    printer_snapshot: printer(),
    original_job_id: null,
    state: 'pending',
    attempts: 0,
    last_error: '',
    transport_reference: '',
    revision: 1,
    created_at: '2026-11-13T18:00:00Z',
    ticket_kind: 'kitchen',
    folio: 7,
    document_title: null,
    problem: 'agent_offline',
    terminal_name: 'Caja 2',
    ...overrides,
  };
}

const device = {
  id: southDevice,
  name: 'Caja 2',
  register_id: null,
  register_name: null,
  keyboard_mode: 'auto',
  home_screen: 'tables',
  last_seen_at: '2026-11-13T18:00:00Z',
  revoked_at: null,
  kind: 'desktop',
  os: 'macOS',
  app_version: '0.10.0',
  paired_at: '2026-11-13T17:00:00Z',
  online: true,
  current_user: null,
};

beforeEach(() => {
  replies = {
    kitchen_printers: { status: 200, body: [printer()] },
    kitchen_terminal_printers: { status: 200, body: terminals },
    kitchen_discovery: { status: 200, body: [] },
    kitchen_jobs: { status: 200, body: [job()] },
    devices: { status: 200, body: [device] },
    registers: { status: 200, body: [] },
    devices_pairing_requests: { status: 200, body: { requests: [], approve_here: true } },
    devices_sessions_summary: {
      status: 200,
      body: { by_kind: { desktop: 1, android: 0, kitchen: 0 }, total: 1, limited: false },
    },
    device_configuration: {
      status: 200,
      body: {
        device,
        kitchen_all_stations: true,
        kitchen_sound_alert: false,
        kitchen_stations: [],
        printers: [{ id: 'usb', name: 'USB de caja', destination: 'POS_80', active: true }],
        local_printers: {
          agent_online: true,
          agent_seen_at: '2026-11-13T18:00:00Z',
          reported_at: '2026-11-13T17:59:00Z',
          queues: terminals[0]!.queues,
        },
      },
    },
  };
  native.invoke.mockImplementation(async (command: string, args: { operation?: string }) => {
    if (command === 'api_request' && args.operation && replies[args.operation]) {
      return replies[args.operation];
    }
    return { status: 200, body: [] };
  });
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe('impresoras USB de las terminales', () => {
  it('Terminales muestra las impresoras de la computadora de cada terminal', async () => {
    render(<DevicesPage can={() => true} />);
    const row = (await screen.findByText('Caja 2')).closest('tr') as HTMLElement;
    fireEvent.click(within(row).getByRole('button', { name: 'Ver configuración' }));
    const configuration = await screen.findByRole('region', { name: 'Configuración de Caja 2' });
    await waitFor(() => expect(configuration.textContent).toContain('USB de caja'));
    const local = within(configuration).getByRole('region', {
      name: 'Impresoras de la computadora de Caja 2',
    });
    expect(local.textContent).toContain('Conectada: imprime los trabajos de sus impresoras.');
    expect(local.textContent).toContain('POS_80 · POS-80 Series · Se puede usar');
    expect(local.textContent).toContain(
      'Oficina · Generic PostScript Printer · No se puede usar: Está conectada por la red',
    );
    expect(local.textContent).toContain('«Impresora USB de una terminal»');
  });

  it('una terminal que aún no informó lo dice', async () => {
    replies.device_configuration = {
      status: 200,
      body: {
        ...(replies.device_configuration!.body as object),
        printers: [],
        local_printers: { agent_online: false, agent_seen_at: null, reported_at: null, queues: [] },
      },
    };
    render(<DevicesPage can={() => true} />);
    const row = (await screen.findByText('Caja 2')).closest('tr') as HTMLElement;
    fireEvent.click(within(row).getByRole('button', { name: 'Ver configuración' }));
    const configuration = await screen.findByRole('region', { name: 'Configuración de Caja 2' });
    await waitFor(() => expect(configuration.textContent).toContain('Sin impresoras propias'));
    expect(configuration.textContent).toContain('Desconectada: sus trabajos esperan');
    expect(configuration.textContent).toContain('Todavía no informó sus impresoras.');
  });

  it('se registra eligiendo la terminal y una de sus impresoras que sirven', async () => {
    const execute = vi.fn(
      async (_operation: string, options: { body?: Record<string, unknown> }) => ({
        ...(options.body as unknown as Printer),
        id: 'p-new',
        revision: 1,
      }),
    );
    render(<PrintersAdmin can={() => true} execute={execute as never} busy={false} />);
    // La lista dice en qué terminal está cada impresora.
    expect(await screen.findByText(/Impresora USB de Caja 2 · 80 mm/)).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: 'Registrar impresora' }));
    fireEvent.change(screen.getByLabelText('Nombre descriptivo de impresora'), {
      target: { value: 'Tickets de la Caja 2' },
    });
    chooseValue(screen.getByLabelText('Transporte de impresión'), 'agent');
    // Sin terminal elegida, la impresora no imprime y se dice.
    expect(screen.getByText(/Sin terminal elegida esta impresora no imprime/)).toBeTruthy();
    await waitFor(() =>
      chooseValue(screen.getByLabelText('Terminal donde está conectada'), southDevice),
    );
    const queue = screen.getByLabelText('Impresora de esa terminal');
    fireEvent.click(queue);
    const office = await screen.findByRole('option', { name: /Oficina/ });
    expect(office.getAttribute('aria-disabled')).toBe('true');
    expect(office.textContent).toContain('impresora de documentos');
    fireEvent.keyDown(queue, { key: 'Escape' });
    chooseValue(queue, 'POS_80');
    expect(
      screen.getByText(/Caja 2 imprime esta impresora mientras AD-EL POS está abierto/),
    ).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: 'Guardar impresora' }));
    await waitFor(() => expect(execute).toHaveBeenCalledTimes(1));
    expect(execute.mock.calls[0]![1].body).toMatchObject({
      transport: 'agent',
      destination: 'POS_80',
      agent_device_id: southDevice,
    });
  });

  it('una terminal sin impresoras informadas lo explica', async () => {
    render(<PrintersAdmin can={() => true} execute={vi.fn() as never} busy={false} />);
    fireEvent.click(await screen.findByRole('button', { name: 'Registrar impresora' }));
    chooseValue(screen.getByLabelText('Transporte de impresión'), 'agent');
    await waitFor(() =>
      chooseValue(
        screen.getByLabelText('Terminal donde está conectada'),
        '00000000-0000-4000-8000-0000000000e3',
      ),
    );
    expect(screen.getByText(/Barra todavía no informó sus impresoras/)).toBeTruthy();
  });

  it('al asignar impresoras se ve en qué terminal está y la de una terminal sirve para tickets', () => {
    expect(printerLabel(printer())).toBe('USB de caja · en Caja 2');
    expect(printerLabel({ name: 'Cocina red' })).toBe('Cocina red');
    const options = receiptPrinterOptions([
      printer(),
      printer({ id: 'legacy', name: 'Otra terminal', agent_device_id: null, terminal_name: '' }),
    ]);
    expect(options[1]).toMatchObject({
      value: 'usb',
      label: 'USB de caja · en Caja 2',
      disabled: false,
    });
    expect(options[2]).toMatchObject({ value: 'legacy', disabled: true });
  });

  it('la cola dice que la terminal está desconectada, sin reintentar', async () => {
    render(<PrintQueue can={() => true} execute={vi.fn() as never} busy={false} timezone="UTC" />);
    const card = (await screen.findByRole('listitem', { name: /USB de caja/ })) as HTMLElement;
    expect(card.textContent).toContain('La Caja 2 está desconectada.');
    expect(card.textContent).toContain('Se imprimirá sola cuando esa terminal esté encendida');
    expect(card.textContent).toContain('USB de caja · en Caja 2');
    expect(within(card).queryByRole('button', { name: 'Reintentar' })).toBeNull();
  });

  it('el sujeto de la frase siempre concuerda', () => {
    expect(terminalSubject('Caja 2')).toBe('La Caja 2');
    expect(terminalSubject('Computadora principal')).toBe('La Computadora principal');
    expect(terminalSubject('Mostrador')).toBe('La terminal Mostrador');
    expect(terminalSubject('')).toBe('La terminal de esta impresora');
    expect(printProblemMessage('agent_offline', 'USB', 'cash', 'Mostrador')).toBe(
      'La terminal Mostrador está desconectada.',
    );
    expect(printProblemHint('agent_offline', 'USB')).toContain('no hace falta reintentar');
  });
});
