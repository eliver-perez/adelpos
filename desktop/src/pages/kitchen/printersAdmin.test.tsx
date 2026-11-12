// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { Printer } from '../../services/kitchen';
import { PrintersAdmin } from './PrintersAdmin';
import { chooseValue } from '../../testing/select';

// Revisión de la etapa 9B (hallazgo medio 4): una impresora CUPS solo recibe formato y logotipo si
// se marca «Esta impresora entiende ESC/POS (formato y logotipo)», desactivada por omisión. La
// marca solo existe en CUPS (ESC/POS por red ya recibe formato) y viaja a Go en el cuerpo.

const native = vi.hoisted(() => ({ invoke: vi.fn() }));
vi.mock('@tauri-apps/api/core', () => ({ invoke: native.invoke, isTauri: () => true }));
const notices = vi.hoisted(() => ({ success: vi.fn() }));
vi.mock('../../services/notify', async (original) => ({
  ...(await original<typeof import('../../services/notify')>()),
  notifySuccess: notices.success,
}));

const label = 'Esta impresora entiende ESC/POS (formato y logotipo)';

beforeEach(() => {
  native.invoke.mockImplementation(async () => ({ status: 200, body: [] }));
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

function renderAdmin(execute = vi.fn()) {
  render(<PrintersAdmin can={() => true} execute={execute as never} busy={false} />);
  fireEvent.click(screen.getByRole('button', { name: 'Registrar impresora' }));
  return execute;
}

describe('impresoras del servidor', () => {
  it('una impresora CUPS nueva no entiende ESC/POS hasta marcarlo, y la marca viaja a Go', async () => {
    const execute = vi.fn(
      async (_operation: string, options: { body?: Record<string, unknown> }) => {
        return { ...(options.body as unknown as Printer), id: 'p1', revision: 1 };
      },
    );
    renderAdmin(execute);
    const check = screen.getByLabelText(label) as HTMLInputElement;
    expect(check.checked).toBe(false);
    expect(
      screen.getByText(/Actívala solo si la impresora es térmica y entiende comandos ESC\/POS/),
    ).toBeTruthy();
    fireEvent.change(screen.getByLabelText('Nombre descriptivo de impresora'), {
      target: { value: 'Tickets USB' },
    });
    fireEvent.change(screen.getByLabelText('Nombre de cola CUPS'), { target: { value: 'POS_80' } });
    fireEvent.click(check);
    fireEvent.click(screen.getByRole('button', { name: 'Guardar impresora' }));
    await waitFor(() => expect(execute).toHaveBeenCalledTimes(1));
    const [operation, options] = execute.mock.calls[0]!;
    expect(operation).toBe('kitchen_create_printer');
    expect(options.body).toMatchObject({
      transport: 'cups',
      destination: 'POS_80',
      accepts_escpos: true,
    });
  });

  it('ESC/POS por red no ofrece la marca y cambiar de transporte la desactiva', async () => {
    const execute = vi.fn(
      async (_operation: string, options: { body?: Record<string, unknown> }) => {
        return { ...(options.body as unknown as Printer), id: 'p2', revision: 1 };
      },
    );
    renderAdmin(execute);
    fireEvent.click(screen.getByLabelText(label));
    chooseValue(screen.getByLabelText('Transporte de impresión'), 'escpos_tcp');
    expect(screen.queryByLabelText(label)).toBeNull();
    expect(screen.getByText(/Por red la impresora recibe comandos ESC\/POS/)).toBeTruthy();
    fireEvent.change(screen.getByLabelText('Nombre descriptivo de impresora'), {
      target: { value: 'Tickets red' },
    });
    fireEvent.change(screen.getByLabelText('IP privada de la impresora'), {
      target: { value: '192.168.1.40' },
    });
    fireEvent.click(screen.getByRole('button', { name: 'Guardar impresora' }));
    await waitFor(() => expect(execute).toHaveBeenCalledTimes(1));
    expect(execute.mock.calls[0]![1].body).toMatchObject({
      transport: 'escpos_tcp',
      accepts_escpos: false,
    });
  });
});

// Etapa 9C (§5.3): columnas de texto por impresora (lista cerrada por papel, sin escritura libre) y
// «Imprimir prueba de ancho» junto a la opción, con las columnas elegidas aún sin guardar.
const savedPrinter = (overrides: Partial<Printer> = {}): Printer => ({
  id: 'p80',
  name: 'Cocina caliente',
  transport: 'cups',
  destination: 'COCINA',
  port: 9100,
  paper_width: 80,
  copies: 1,
  supports_cut: false,
  cut: false,
  cutter_option: '',
  accepts_escpos: false,
  columns: null,
  active: true,
  fallback_id: null,
  revision: 3,
  ...overrides,
});

function renderSaved(
  printer: Printer,
  execute = vi.fn(),
  can: (key: string) => boolean = () => true,
): ReturnType<typeof vi.fn> {
  native.invoke.mockImplementation(async (_command: string, args: { operation: string }) => ({
    status: 200,
    body: args.operation === 'kitchen_printers' ? [printer] : [],
  }));
  render(<PrintersAdmin can={can} execute={execute as never} busy={false} />);
  return execute;
}

async function openSaved(name: string) {
  fireEvent.click(await screen.findByRole('button', { name: new RegExp(name) }));
}

function columnsGroup() {
  return screen.getByRole('group', { name: 'Columnas de texto' });
}

describe('columnas de texto por impresora', () => {
  it('ofrece solo la lista cerrada de su papel y al cambiar el papel vuelve al predeterminado', async () => {
    const execute = vi.fn(
      async (_operation: string, options: { body?: Record<string, unknown> }) => {
        return { ...(options.body as unknown as Printer), revision: 4 };
      },
    );
    renderSaved(savedPrinter(), execute);
    await openSaved('Cocina caliente');
    expect(screen.getByText(/80 mm · 42 columnas/)).toBeTruthy();
    const group = columnsGroup();
    const buttons = Array.from(group.querySelectorAll('button')).map(
      (button) => button.textContent,
    );
    expect(buttons).toEqual(['42 (predeterminado)', '48 columnas']);
    expect(
      screen.getByRole('button', { name: '42 (predeterminado)' }).getAttribute('aria-pressed'),
    ).toBe('true');
    expect(document.querySelector('input[type="number"]')).toBeNull();
    fireEvent.click(screen.getByRole('button', { name: '48 columnas' }));
    expect(screen.getByRole('button', { name: '48 columnas' }).getAttribute('aria-pressed')).toBe(
      'true',
    );
    chooseValue(screen.getByLabelText('Ancho de papel'), '58');
    const narrow = Array.from(columnsGroup().querySelectorAll('button')).map(
      (button) => button.textContent,
    );
    expect(narrow).toEqual(['30 (predeterminado)', '32 columnas']);
    expect(
      screen.getByRole('button', { name: '30 (predeterminado)' }).getAttribute('aria-pressed'),
    ).toBe('true');
    fireEvent.click(screen.getByRole('button', { name: '32 columnas' }));
    fireEvent.click(screen.getByRole('button', { name: 'Guardar impresora' }));
    await waitFor(() => expect(execute).toHaveBeenCalledTimes(1));
    const [operation, options] = execute.mock.calls[0]!;
    expect(operation).toBe('kitchen_save_printer');
    expect(options.body).toMatchObject({ paper_width: 58, columns: 32 });
  });

  it('el predeterminado viaja como null y una impresora con 48 columnas lo muestra elegido', async () => {
    const execute = vi.fn(
      async (_operation: string, options: { body?: Record<string, unknown> }) => {
        return { ...(options.body as unknown as Printer), revision: 4 };
      },
    );
    renderSaved(savedPrinter({ columns: 48 }), execute);
    await openSaved('Cocina caliente');
    expect(screen.getByRole('button', { name: '48 columnas' }).getAttribute('aria-pressed')).toBe(
      'true',
    );
    fireEvent.click(screen.getByRole('button', { name: '42 (predeterminado)' }));
    fireEvent.click(screen.getByRole('button', { name: 'Guardar impresora' }));
    await waitFor(() => expect(execute).toHaveBeenCalledTimes(1));
    expect(execute.mock.calls[0]![1].body).toMatchObject({ paper_width: 80, columns: null });
  });

  it('imprime la prueba de ancho con las columnas elegidas y avisa que va a la cola', async () => {
    const execute = vi.fn(
      async (_operation: string, options: { id?: string; body?: Record<string, unknown> }) => ({
        job: { id: 'j1', state: 'pending', last_error: '' },
        ticket_id: 't1',
        paper_width: 80,
        columns: (options.body?.columns as number) ?? 42,
      }),
    );
    renderSaved(savedPrinter(), execute);
    await openSaved('Cocina caliente');
    fireEvent.click(screen.getByRole('button', { name: '48 columnas' }));
    fireEvent.click(screen.getByRole('button', { name: 'Imprimir prueba de ancho (48 columnas)' }));
    await waitFor(() => expect(execute).toHaveBeenCalledTimes(1));
    expect(execute.mock.calls[0]![0]).toBe('kitchen_printer_width_test');
    expect(execute.mock.calls[0]![1]).toEqual({ id: 'p80', body: { columns: 48 } });
    await waitFor(() =>
      expect(notices.success).toHaveBeenCalledWith(
        'Prueba de ancho de 48 columnas enviada a la cola de Cocina caliente.',
      ),
    );
  });

  it('con el papel cambiado sin guardar pide guardarlo antes de la prueba', async () => {
    renderSaved(savedPrinter());
    await openSaved('Cocina caliente');
    chooseValue(screen.getByLabelText('Ancho de papel'), '58');
    const button = screen.getByRole('button', {
      name: 'Imprimir prueba de ancho (30 columnas)',
    }) as HTMLButtonElement;
    expect(button.disabled).toBe(true);
    expect(screen.getByText(/Guarda primero el nuevo ancho de papel/)).toBeTruthy();
  });

  it('sin administrar impresoras no ofrece la prueba; una impresora nueva tampoco', async () => {
    renderSaved(savedPrinter(), vi.fn(), (key) => key !== 'kitchen.printers.manage');
    await openSaved('Cocina caliente');
    expect(screen.queryByRole('button', { name: /Imprimir prueba de ancho/ })).toBeNull();
    cleanup();
    renderAdmin();
    expect(columnsGroup()).toBeTruthy();
    expect(screen.queryByRole('button', { name: /Imprimir prueba de ancho/ })).toBeNull();
  });
});
