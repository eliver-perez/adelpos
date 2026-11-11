// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { Printer } from '../../services/kitchen';
import { PrintersAdmin } from './PrintersAdmin';

// Revisión de la etapa 9B (hallazgo medio 4): una impresora CUPS solo recibe formato y logotipo si
// se marca «Esta impresora entiende ESC/POS (formato y logotipo)», desactivada por omisión. La
// marca solo existe en CUPS (ESC/POS por red ya recibe formato) y viaja a Go en el cuerpo.

const native = vi.hoisted(() => ({ invoke: vi.fn() }));
vi.mock('@tauri-apps/api/core', () => ({ invoke: native.invoke, isTauri: () => true }));

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
    fireEvent.change(screen.getByLabelText('Transporte de impresión'), {
      target: { value: 'escpos_tcp' },
    });
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
