// @vitest-environment jsdom

// Etapa 9B, fase C: «Imprimir con la plantilla vigente» junto a las reimpresiones existentes
// (documentos de una cuenta, documentos de caja y cola de cocina). Solo aparece con el permiso de
// reimprimir del origen más documents.history.render_current, se confirma con un texto que la
// distingue de «Reimprimir» (copia idéntica) y llama a su operación propia.

import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { PendingOperationsProvider } from '../../hooks/pendingOperations';
import type { CashDocumentRecord } from '../../services/cash';
import type { ExecuteKitchen } from '../../services/kitchen';
import { documentOptions } from '../cash/CashDocumentActions';
import { PrintQueue } from '../kitchen/PrintQueue';
import { DocumentList } from '../restaurant/documents/DocumentList';

const native = vi.hoisted(() => ({ invoke: vi.fn() }));
vi.mock('@tauri-apps/api/core', () => ({ invoke: native.invoke, isTauri: () => true }));

type Handler = (args: Record<string, unknown>) => { status: number; body: unknown };
let handlers: Record<string, Handler> = {};

beforeEach(() => {
  handlers = {};
  native.invoke.mockImplementation(async (command: string, args: Record<string, unknown>) => {
    if (command !== 'api_request') {
      return null;
    }
    const handler = handlers[String(args.operation)];
    return handler ? handler(args) : { status: 200, body: [] };
  });
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

function allowing(...permissions: string[]) {
  return (permission: string) => permissions.includes(permission);
}

function calls(operation: string) {
  return native.invoke.mock.calls
    .filter((call) => (call[1] as { operation?: string })?.operation === operation)
    .map((call) => call[1] as { body: Record<string, unknown>; resourceId: string | null });
}

const accountDocuments = [
  {
    ticket_id: 'tk-1',
    kind: 'receipt',
    payment_id: 'pay-1',
    created_at: '2026-11-11T15:00:00Z',
    job_id: 'job-1',
    job_state: 'submitted',
    printer_name: 'Tickets',
    copies: 1,
  },
];

function renderList(can: (permission: string) => boolean) {
  render(
    <PendingOperationsProvider>
      <DocumentList
        accountID="acc-1"
        documents={accountDocuments}
        timezone="America/Monterrey"
        can={can}
      />
    </PendingOperationsProvider>,
  );
}

describe('documentos de una cuenta', () => {
  it('sin documents.history.render_current solo se reimprime', () => {
    renderList(allowing('payments.tickets.reprint'));
    expect(screen.getByRole('button', { name: 'Reimprimir ticket de pago' })).toBeTruthy();
    expect(screen.queryByRole('button', { name: 'Imprimir con la plantilla vigente' })).toBeNull();
  });

  // Revisión 9F (B2): otra copia en papel de un documento ya emitido pide motivo, como reimprimir.
  function askReason(reason: string) {
    fireEvent.click(screen.getByRole('button', { name: 'Imprimir con la plantilla vigente' }));
    const form = screen.getByRole('form', {
      name: 'Impresión con la plantilla vigente con motivo',
    });
    const submit = within(form).getByRole('button', { name: 'Imprimir con la plantilla vigente' });
    expect((submit as HTMLButtonElement).disabled).toBe(true);
    fireEvent.change(
      within(form).getByLabelText('Motivo de la impresión con la plantilla vigente'),
      { target: { value: reason } },
    );
    fireEvent.click(submit);
  }

  it('con su permiso, pide motivo, confirma la diferencia con «Reimprimir» y usa su operación', async () => {
    handlers.payments_rerender = () => ({
      status: 200,
      body: { status: 'pending', job_id: 'job-2', ticket_id: 'tk-2', source_ticket_id: 'tk-1' },
    });
    renderList(allowing('payments.tickets.reprint', 'documents.history.render_current'));
    askReason('Cambió el logotipo del restaurante');
    const dialog = await screen.findByRole('alertdialog', {
      name: 'Imprimir con la plantilla vigente',
    });
    expect(within(dialog).getByText(/envía una copia idéntica al original/)).toBeTruthy();
    expect(within(dialog).getByText(/plantilla activa hoy/)).toBeTruthy();
    fireEvent.click(within(dialog).getByRole('button', { name: 'Cancelar' }));
    expect(calls('payments_rerender')).toHaveLength(0);
    // El motivo escrito se conserva para volver a intentarlo.
    fireEvent.click(
      within(
        screen.getByRole('form', { name: 'Impresión con la plantilla vigente con motivo' }),
      ).getByRole('button', { name: 'Imprimir con la plantilla vigente' }),
    );
    fireEvent.click(
      within(
        await screen.findByRole('alertdialog', { name: 'Imprimir con la plantilla vigente' }),
      ).getByRole('button', { name: 'Imprimir con la plantilla vigente' }),
    );
    await waitFor(() => expect(calls('payments_rerender')).toHaveLength(1));
    expect(calls('payments_rerender')[0]).toMatchObject({
      resourceId: 'acc-1',
      body: { ticket_id: 'tk-1', reason: 'Cambió el logotipo del restaurante' },
    });
    expect(typeof calls('payments_rerender')[0]?.body.operation_id).toBe('string');
    expect(calls('payments_reprint')).toHaveLength(0);
    await screen.findByText(/Documento con la plantilla vigente solicitado/);
  });
});

describe('documentos de caja', () => {
  const record = { id: 'cd-1', includes_expected: false } as CashDocumentRecord;

  it('ofrece la plantilla vigente solo con reimprimir y documents.history.render_current', () => {
    expect(documentOptions(allowing('cash.reports.reprint'), record).renderCurrent).toBe(false);
    expect(
      documentOptions(allowing('cash.reports.reprint', 'documents.history.render_current'), record)
        .renderCurrent,
    ).toBe(true);
    // Un documento con el esperado sin permiso de consultarlo no se reimprime de ninguna forma.
    expect(
      documentOptions(allowing('cash.reports.reprint', 'documents.history.render_current'), {
        ...record,
        includes_expected: true,
      }).renderCurrent,
    ).toBe(false);
    expect(documentOptions(allowing('documents.history.render_current'), null).renderCurrent).toBe(
      false,
    );
  });
});

describe('cola de cocina', () => {
  const job = {
    id: 'job-k',
    ticket_id: 'tk-k',
    printer_id: 'p-1',
    printer_name: 'Cocina',
    printer_snapshot: {},
    original_job_id: null,
    state: 'submitted',
    attempts: 1,
    last_error: '',
    transport_reference: '',
    revision: 2,
    created_at: '2026-11-11T15:00:00Z',
  };

  beforeEach(() => {
    handlers.kitchen_jobs = () => ({ status: 200, body: [job] });
    handlers.kitchen_job = () => ({ status: 200, body: { job, attempts: [] } });
    handlers.kitchen_ticket = () => ({
      status: 200,
      body: {
        id: 'tk-k',
        kind: 'normal',
        created_at: job.created_at,
        context: { folio: 1, account_name: 'Mesa 1', timezone: 'America/Monterrey' },
        sections: [],
        preview: 'COMANDA',
      },
    });
  });

  it('aparece solo con kitchen.reprint y documents.history.render_current', async () => {
    const execute = vi.fn() as unknown as ExecuteKitchen;
    render(
      <PrintQueue
        can={allowing('kitchen.reprint', 'kitchen.tickets.read')}
        execute={execute}
        busy={false}
        timezone="America/Monterrey"
      />,
    );
    await screen.findByRole('button', { name: 'Solicitar otra copia' });
    expect(screen.queryByRole('button', { name: 'Imprimir con la plantilla vigente' })).toBeNull();
  });

  it('pide motivo, confirma y envía kitchen_rerender', async () => {
    const execute = vi.fn(async () => ({})) as unknown as ExecuteKitchen & ReturnType<typeof vi.fn>;
    render(
      <PrintQueue
        can={allowing(
          'kitchen.reprint',
          'kitchen.tickets.read',
          'documents.history.render_current',
        )}
        execute={execute}
        busy={false}
        timezone="America/Monterrey"
      />,
    );
    fireEvent.click(
      await screen.findByRole('button', { name: 'Imprimir con la plantilla vigente' }),
    );
    await screen.findByRole('heading', { name: 'Imprimir con la plantilla vigente' });
    const send = screen
      .getAllByRole('button', { name: 'Imprimir con la plantilla vigente' })
      .at(-1) as HTMLButtonElement;
    expect(send.disabled).toBe(true);
    fireEvent.change(screen.getByLabelText(/Motivo de la impresión con la plantilla vigente/), {
      target: { value: 'Prueba del formato nuevo' },
    });
    await waitFor(() => expect(send.disabled).toBe(false));
    fireEvent.click(send);
    const dialog = await screen.findByRole('alertdialog', {
      name: 'Imprimir con la plantilla vigente',
    });
    fireEvent.click(
      within(dialog).getByRole('button', { name: 'Imprimir con la plantilla vigente' }),
    );
    await waitFor(() => expect(execute).toHaveBeenCalledTimes(1));
    expect(execute).toHaveBeenCalledWith('kitchen_rerender', {
      body: { job_id: 'job-k', reason: 'Prueba del formato nuevo' },
    });
  });
});
