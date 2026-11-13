// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { actionByID } from '../../commands/actions';
import { precheckName, setPrecheckName } from '../../services/documentNames';
import type { Printer, PrintJob } from '../../services/kitchen';
import { printProblemMessage } from '../../services/printProblems';
import { tableField, tableNameIncludesWord } from '../../services/restaurant';
import { chooseValue } from '../../testing/select';
import { PrintersAdmin } from './PrintersAdmin';
import { groupJobs, PrintQueue } from './PrintQueue';

// Etapa 9F, fase B (§4.3–§4.6): formulario de corte de papel, «Cola de impresión» (agrupada, en
// palabras del restaurante, con reintento de un toque y el detalle a la vista), nombre visible de la
// precuenta en las acciones y «Mesa» sin repetir.

const native = vi.hoisted(() => ({ invoke: vi.fn() }));
vi.mock('@tauri-apps/api/core', () => ({ invoke: native.invoke, isTauri: () => true }));

let jobs: PrintJob[] = [];

beforeEach(() => {
  jobs = [];
  native.invoke.mockImplementation(async (_command: string, args: { operation: string }) => {
    if (args?.operation === 'kitchen_jobs') {
      return { status: 200, body: jobs };
    }
    if (args?.operation === 'kitchen_job') {
      return { status: 200, body: { job: jobs[0], attempts: [] } };
    }
    return { status: 200, body: [] };
  });
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  setPrecheckName('precheck');
});

function job(overrides: Partial<PrintJob> = {}): PrintJob {
  return {
    id: 'job-1',
    ticket_id: 'ticket-1',
    printer_id: 'printer-1',
    printer_name: 'Caja térmica',
    printer_snapshot: {} as Printer,
    original_job_id: null,
    state: 'error',
    attempts: 3,
    last_error: 'No se abrió la conexión con la impresora; no se enviaron bytes.',
    transport_reference: 'Caja-15150',
    revision: 4,
    created_at: '2026-11-13T18:00:00Z',
    ticket_kind: 'precheck',
    folio: 12,
    document_title: null,
    problem: 'unreachable',
    ...overrides,
  };
}

describe('corte de papel por impresora (§4.4)', () => {
  it('una impresora nueva de 80 mm corta con 4 renglones; en 58 mm empieza sin corte', async () => {
    const execute = vi.fn(
      async (_operation: string, options: { body?: Record<string, unknown> }) => ({
        ...(options.body as unknown as Printer),
        id: 'p1',
        revision: 1,
      }),
    );
    render(<PrintersAdmin can={() => true} execute={execute as never} busy={false} />);
    fireEvent.click(screen.getByRole('button', { name: 'Registrar impresora' }));
    const cut = screen.getByLabelText('Cortar el papel al terminar') as HTMLInputElement;
    expect(cut.checked).toBe(true);
    // Ya no hay una segunda casilla para «el modelo admite corte».
    expect(screen.queryByLabelText('El modelo y su controlador admiten corte')).toBeNull();
    expect(screen.getByText(/La mayoría de las impresoras sin cortador ignoran/)).toBeTruthy();
    expect(
      screen.getByRole('combobox', { name: 'Renglones de avance antes del corte' }).textContent,
    ).toContain('4 (predeterminado)');
    chooseValue(screen.getByLabelText('Ancho de papel'), '58');
    expect((screen.getByLabelText('Cortar el papel al terminar') as HTMLInputElement).checked).toBe(
      false,
    );
    chooseValue(screen.getByLabelText('Ancho de papel'), '80');
    fireEvent.click(screen.getByRole('button', { name: 'Parcial' }));
    chooseValue(screen.getByRole('combobox', { name: 'Renglones de avance antes del corte' }), '8');
    fireEvent.change(screen.getByLabelText('Nombre descriptivo de impresora'), {
      target: { value: 'Tickets' },
    });
    fireEvent.change(screen.getByLabelText('Nombre de cola CUPS'), { target: { value: 'POS_80' } });
    fireEvent.click(screen.getByRole('button', { name: 'Guardar impresora' }));
    await waitFor(() => expect(execute).toHaveBeenCalledTimes(1));
    expect(execute.mock.calls[0]![1].body).toMatchObject({
      paper_width: 80,
      cut: true,
      supports_cut: true,
      cut_mode: 'partial',
      cut_feed_lines: 8,
    });
  });

  it('quitar el corte lo quita también de «el modelo admite corte»', async () => {
    const execute = vi.fn(
      async (_operation: string, options: { body?: Record<string, unknown> }) => ({
        ...(options.body as unknown as Printer),
        id: 'p1',
        revision: 1,
      }),
    );
    render(<PrintersAdmin can={() => true} execute={execute as never} busy={false} />);
    fireEvent.click(screen.getByRole('button', { name: 'Registrar impresora' }));
    fireEvent.click(screen.getByLabelText('Cortar el papel al terminar'));
    expect(
      screen.queryByRole('combobox', { name: 'Renglones de avance antes del corte' }),
    ).toBeNull();
    fireEvent.change(screen.getByLabelText('Nombre descriptivo de impresora'), {
      target: { value: 'Tickets' },
    });
    fireEvent.change(screen.getByLabelText('Nombre de cola CUPS'), { target: { value: 'POS_80' } });
    fireEvent.click(screen.getByRole('button', { name: 'Guardar impresora' }));
    await waitFor(() => expect(execute).toHaveBeenCalledTimes(1));
    expect(execute.mock.calls[0]![1].body).toMatchObject({ cut: false, supports_cut: false });
  });
});

describe('cola de impresión (§4.3)', () => {
  it('agrupa los repetidos con «×N», sin datos internos y en palabras del restaurante', async () => {
    jobs = [
      job({ id: 'job-3', created_at: '2026-11-13T18:02:00Z' }),
      job({ id: 'job-2', ticket_id: 'ticket-2', created_at: '2026-11-13T18:01:00Z' }),
      job({ id: 'job-1', ticket_id: 'ticket-3' }),
      job({
        id: 'job-0',
        ticket_id: 'ticket-4',
        ticket_kind: 'normal',
        folio: 7,
        printer_name: 'Cocina',
        state: 'intervention',
        problem: 'inactive',
      }),
    ];
    expect(groupJobs(jobs).map((group) => group.jobs.length)).toEqual([3, 1]);
    render(<PrintQueue can={() => true} execute={vi.fn() as never} busy={false} timezone="UTC" />);
    const list = await screen.findByRole('list', { name: 'Trabajos de impresión' });
    const cards = await within(list).findAllByRole('listitem');
    expect(cards).toHaveLength(2);
    expect(cards[0]!.textContent).toContain('Precuenta · #12');
    expect(cards[0]!.textContent).toContain('×3');
    expect(cards[0]!.textContent).toContain(
      'La impresora Caja térmica está apagada o sin conexión.',
    );
    expect(cards[1]!.textContent).toContain('La impresora Cocina está desactivada.');
    // Ni la referencia técnica del transporte ni los mensajes del controlador ni estados en inglés.
    expect(list.textContent).not.toContain('15150');
    expect(list.textContent).not.toContain('bytes');
    expect(list.textContent).not.toMatch(/submitted|intervention|agente/);
    fireEvent.click(within(cards[0]!).getByRole('button', { name: 'Ver los 3 trabajos' }));
    expect(
      within(cards[0]!).getByRole('list', { name: 'Trabajos repetidos' }).children,
    ).toHaveLength(3);
  });

  it('reintenta con un toque, sin motivo', async () => {
    jobs = [job()];
    const execute = vi.fn(async () => ({}));
    render(<PrintQueue can={() => true} execute={execute as never} busy={false} timezone="UTC" />);
    const card = (await screen.findAllByRole('listitem'))[0]!;
    fireEvent.click(within(card).getByRole('button', { name: 'Reintentar' }));
    await waitFor(() => expect(execute).toHaveBeenCalledTimes(1));
    expect(execute.mock.calls[0]).toEqual([
      'kitchen_retry_job',
      { id: 'job-1', body: { revision: 4, reason: '', printer_id: null } },
    ]);
    expect(screen.queryByLabelText('Motivo de la decisión de impresión')).toBeNull();
  });

  it('el detalle se lleva a la vista y la referencia técnica queda en «Información para soporte»', async () => {
    jobs = [job({ state: 'submitted', problem: null })];
    const scroll = vi.fn();
    Element.prototype.scrollIntoView = scroll;
    render(<PrintQueue can={() => true} execute={vi.fn() as never} busy={false} timezone="UTC" />);
    const card = (await screen.findAllByRole('listitem'))[0]!;
    fireEvent.click(within(card).getByRole('button', { name: 'Consultar trabajo e intentos' }));
    const detail = await screen.findByRole('region', { name: 'Detalle del trabajo de impresión' });
    expect(scroll).toHaveBeenCalled();
    expect(detail.textContent).toContain('Información para soporte');
    expect(card.textContent).not.toContain('Caja-15150');
  });
});

describe('nombre visible de la precuenta (§4.5)', () => {
  it('cambia la acción de la paleta y del menú', () => {
    expect(actionByID('file.print_precheck')?.label).toBe('Imprimir precuenta');
    setPrecheckName('sales_ticket');
    expect(actionByID('file.print_precheck')?.label).toBe('Imprimir ticket de venta');
    expect(precheckName().printed).toBe('TICKET DE VENTA');
    setPrecheckName('bill');
    expect(actionByID('file.print_precheck')?.label).toBe('Imprimir cuenta');
    setPrecheckName('desconocido');
    expect(precheckName().label).toBe('Precuenta');
  });
});

describe('textos de impresión y «Mesa» sin repetir (§4.2 y §4.6)', () => {
  it('explica cada problema sin términos técnicos', () => {
    expect(printProblemMessage('unreachable', 'Tickets', 'cash')).toBe(
      'La impresora de caja no responde.',
    );
    expect(printProblemMessage('unreachable', 'Freidoras')).toBe(
      'La impresora Freidoras está apagada o sin conexión.',
    );
    expect(printProblemMessage('inactive', 'Tickets')).toBe(
      'La impresora Tickets está desactivada.',
    );
    expect(printProblemMessage('unsupported', 'Otra')).toBe(
      'Esta impresora todavía no puede usarse para imprimir.',
    );
    expect(printProblemMessage(null, 'Tickets')).toBe('');
  });

  it('no repite «Mesa» en Preparación ni en los detalles', () => {
    // La tarjeta de Preparación usa accountContext (pages/kitchen/preparationModel.ts, fase C), con su
    // propia prueba en preparationModel.test.ts.
    expect(tableNameIncludesWord('MÉSA 1')).toBe(true);
    expect(tableNameIncludesWord('Mesanine')).toBe(false);
    expect(tableField('Mesa 1')).toBe('Mesa 1');
    expect(tableField('Terraza')).toBe('Mesa: Terraza');
  });
});
