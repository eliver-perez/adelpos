// @vitest-environment jsdom

import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { PendingOperationsProvider } from '../../hooks/pendingOperations';
import { cashPrintMessage, drawerMessage } from '../../services/cash';
import type { CashDocumentRecord, ShiftDetail } from '../../services/cash';
import type { Shift } from '../../services/payments';
import { CashDocumentActions } from './CashDocumentActions';
import { CashOperationForm } from './CashOperationForm';
import { CashPage } from './CashPage';
import {
  catalog,
  closingShift,
  currentWith,
  operationOf,
  registersWith,
  shiftID,
  shiftOf,
} from './cashTestData';
import { useCashMutation } from './useCashMutation';

const native = vi.hoisted(() => ({ invoke: vi.fn() }));
vi.mock('@tauri-apps/api/core', () => ({ invoke: native.invoke, isTauri: () => true }));

type Reply = { status: number; body: unknown };
type Handler = (body: Record<string, unknown>) => Reply;

// Simula a Go por operación del puente nativo; una operación sin respuesta preparada devuelve un
// cuerpo vacío. Las cifras son las que entregaría Go: la prueba nunca calcula importes.
function serve(handlers: Record<string, Handler>) {
  native.invoke.mockImplementation(
    async (command: string, args: { operation: string; body?: Record<string, unknown> }) => {
      if (command !== 'api_request') {
        throw new Error('Comando no esperado');
      }
      const handler = handlers[args.operation];
      if (!handler) {
        return { status: 200, body: {} };
      }
      return handler(args.body ?? {});
    },
  );
}

function sent(operation: string) {
  return native.invoke.mock.calls
    .filter(([command, args]) => command === 'api_request' && args?.operation === operation)
    .map(([, args]) => args as { resourceId: string | null; body: Record<string, unknown> });
}

function detailOf(shift: Shift, overrides: Partial<ShiftDetail> = {}): ShiftDetail {
  return {
    shift,
    opening_count: null,
    reports: [],
    summary: { payments: 0, refunds: 0, cash_operations: 0, partial_cuts: 0 },
    expected_visible: false,
    ...overrides,
  };
}

function caja(shift: Shift, detail: ShiftDetail, drawer = false): Record<string, Handler> {
  return {
    payments_cash_registers: () => ({
      status: 200,
      body: registersWith(shift).map((entry) => ({ ...entry, drawer_enabled: drawer })),
    }),
    payments_current_shift: () => ({ status: 200, body: currentWith(shift) }),
    payments_cash_catalog: () => ({ status: 200, body: catalog }),
    payments_shift: () => ({ status: 200, body: detail }),
  };
}

function Page({ can = () => true }: { can?: (permission: string) => boolean }) {
  return (
    <PendingOperationsProvider>
      <CashPage can={can} userID="user-admin" timezone="America/Monterrey" />
    </PendingOperationsProvider>
  );
}

const warning = {
  currencies: ['MXN' as const],
  message: 'El efectivo esperado supera el tope de caja; considera un retiro.',
};

afterEach(() => {
  cleanup();
  native.invoke.mockReset();
});

describe('tope de caja', () => {
  it('muestra el aviso solo cuando Go lo entrega', async () => {
    const shift = shiftOf();
    serve(caja(shift, detailOf(shift)));
    render(<Page />);
    await screen.findByLabelText('Turno de Caja 1');
    await waitFor(() => expect(sent('payments_shift').length).toBeGreaterThan(0));
    expect(screen.queryByRole('status', { name: 'Tope de caja superado' })).toBeNull();
    cleanup();

    serve(caja(shift, detailOf(shift, { cash_limit_warning: warning })));
    render(<Page />);
    const notice = await screen.findByRole('status', { name: 'Tope de caja superado' });
    expect(notice.textContent).toContain('considera un retiro');
    expect(notice.textContent).toContain('pesos');
    expect(notice.textContent).toContain('no impide cobrar');
  });

  it('nunca muestra el aviso durante el arqueo aunque una respuesta lo traiga', async () => {
    const shift = closingShift();
    serve(caja(shift, detailOf(shift, { cash_limit_warning: warning })));
    render(<Page />);
    await screen.findByRole('status', { name: 'Cierre en curso' });
    await waitFor(() => expect(sent('payments_shift').length).toBeGreaterThan(0));
    expect(screen.queryByRole('status', { name: 'Tope de caja superado' })).toBeNull();
  });
});

function FormHarness({ can }: { can: (permission: string) => boolean }) {
  const mutation = useCashMutation(shiftID);
  return (
    <CashOperationForm
      kind="income"
      shift={shiftOf()}
      catalog={catalog}
      can={can}
      dollars={false}
      active
      mutation={mutation}
      onClose={() => undefined}
    />
  );
}

function documentOf(overrides: Partial<CashDocumentRecord> = {}): CashDocumentRecord {
  return {
    id: 'documento-1',
    kind: 'partial_cut',
    shift_id: shiftID,
    report_id: 'corte-1',
    cash_operation_id: null,
    ticket_id: 'ticket-1',
    includes_expected: false,
    created_by_name: 'Administrador',
    created_at: '2026-11-09T18:00:00Z',
    job_id: 'trabajo-1',
    job_state: 'submitted',
    printer_name: 'Tickets',
    copies: 1,
    ...overrides,
  };
}

function renderActions(can: (permission: string) => boolean, documents: CashDocumentRecord[]) {
  serve({
    payments_shift_documents: () => ({ status: 200, body: { shift_id: shiftID, documents } }),
    payments_cash_report_preview: () => ({
      status: 200,
      body: {
        kind: 'partial_cut',
        title: 'CORTE PARCIAL #1',
        content: 'ADEL POS\nCORTE PARCIAL #1\nCaja: Caja 1',
        includes_expected: false,
        paper_width: 80,
        document: documents[0] ?? null,
      },
    }),
    payments_reprint_cash_document: () => ({
      status: 200,
      body: { requested: true, status: 'pending', job_id: 'copia', includes_expected: false },
    }),
  });
  return render(
    <PendingOperationsProvider>
      <CashDocumentActions source={{ kind: 'report', id: 'corte-1' }} shiftID={shiftID} can={can} />
    </PendingOperationsProvider>,
  );
}

describe('impresión y reimpresión según permisos', () => {
  it('ofrece imprimir el comprobante solo con su permiso y lo envía en la operación', async () => {
    serve({
      payments_shift_income: () => ({
        status: 200,
        body: {
          ...operationOf({ kind: 'income', concept: 'Aportación adicional al fondo' }),
          printing: { requested: true, status: 'pending', includes_expected: false },
          drawer: { requested: true, status: 'pending', job_id: 'cajon' },
        },
      }),
    });
    const { unmount } = render(
      <PendingOperationsProvider>
        <FormHarness can={(permission) => permission !== 'cash.reports.print'} />
      </PendingOperationsProvider>,
    );
    expect(screen.queryByRole('button', { name: 'Imprimir comprobante' })).toBeNull();
    unmount();
    render(
      <PendingOperationsProvider>
        <FormHarness can={() => true} />
      </PendingOperationsProvider>,
    );
    const toggle = screen.getByRole('button', { name: 'Imprimir comprobante' });
    expect(toggle.getAttribute('aria-pressed')).toBe('false');
    fireEvent.click(toggle);
    fireEvent.click(screen.getByRole('button', { name: 'Aportación adicional al fondo' }));
    const keypad = screen.getByRole('group', { name: 'Teclado numérico del importe' });
    fireEvent.click(within(keypad).getByRole('button', { name: '5' }));
    fireEvent.click(screen.getByRole('button', { name: /^Registrar ingreso/ }));
    await screen.findByText('Documento enviado a la cola de impresión.');
    expect(screen.getByText('Comando de apertura enviado.')).toBeTruthy();
    expect(sent('payments_shift_income')[0]?.body.print).toBe(true);
  });

  it('imprime una vez y después solo reimprime, con permiso', async () => {
    renderActions(() => true, []);
    expect(await screen.findByRole('button', { name: 'Imprimir' })).toBeTruthy();
    expect(screen.queryByRole('button', { name: 'Reimprimir' })).toBeNull();
    cleanup();

    renderActions((permission) => permission !== 'cash.reports.print', []);
    await waitFor(() => expect(sent('payments_shift_documents').length).toBeGreaterThan(0));
    expect(screen.queryByRole('button', { name: 'Imprimir' })).toBeNull();
    cleanup();

    renderActions(() => true, [documentOf()]);
    const reprint = await screen.findByRole('button', { name: 'Reimprimir' });
    expect(screen.queryByRole('button', { name: 'Imprimir' })).toBeNull();
    expect(screen.getByText(/Enviado a la impresora/)).toBeTruthy();
    fireEvent.click(reprint);
    // El resultado de reimprimir es un aviso de confirmación, fuera del panel.
    const reprinted = await screen.findByRole('status', { name: 'Operación completada' });
    expect(reprinted.textContent).toContain('Documento enviado a la cola de impresión.');
    expect(sent('payments_reprint_cash_document')[0]?.resourceId).toBe('documento-1');
    cleanup();

    renderActions((permission) => permission !== 'cash.reports.reprint', [documentOf()]);
    await screen.findByText(/Enviado a la impresora/);
    expect(screen.queryByRole('button', { name: 'Reimprimir' })).toBeNull();
  });

  it('un documento con el esperado solo se reimprime con el permiso de consultarlo', async () => {
    const withExpected = documentOf({ includes_expected: true });
    renderActions((permission) => permission !== 'cash.expected.read', [withExpected]);
    await screen.findByText(/reimprimirlo requiere el permiso/);
    expect(screen.getByText(/Corte parcial · Enviado a la impresora/).textContent).toContain(
      'incluye el efectivo esperado',
    );
    expect(screen.queryByRole('button', { name: 'Reimprimir' })).toBeNull();
    cleanup();
    renderActions(() => true, [withExpected]);
    expect(await screen.findByRole('button', { name: 'Reimprimir' })).toBeTruthy();
  });

  it('muestra la vista previa renderizada por Go sin escribir', async () => {
    renderActions(() => true, []);
    fireEvent.click(await screen.findByRole('button', { name: 'Vista previa' }));
    const preview = await screen.findByLabelText('Vista previa de CORTE PARCIAL #1');
    expect(preview.textContent).toContain('Caja: Caja 1');
    expect(sent('payments_print_cash_report')).toHaveLength(0);
  });
});

describe('mensajes de envío', () => {
  it('nunca afirman que el papel salió ni que el cajón se abrió', () => {
    const printed = [
      'pending',
      'processing',
      'submitted',
      'uncertain',
      'error',
      'not_configured',
    ].map((status) =>
      cashPrintMessage({ requested: true, status, includes_expected: false }, 'el corte'),
    );
    expect(printed[2]).toBe('Documento enviado a la impresora.');
    for (const message of printed) {
      expect(message).not.toMatch(/se imprimió|salió|papel impreso|impreso correctamente/i);
    }
    expect(printed[5]).toContain('quedó registrado');
    const drawer = ['pending', 'submitted', 'uncertain', 'error'].map((status) =>
      drawerMessage({ requested: true, status }),
    );
    expect(drawer[0]).toBe('Comando de apertura enviado.');
    expect(drawer[1]).toBe('Comando de apertura enviado.');
    expect(drawer[2]).toContain('No se reintenta');
    for (const message of drawer) {
      expect(message).not.toMatch(/cajón abierto|se abrió|abrió el cajón/i);
    }
    expect(drawerMessage({ requested: false, status: 'disabled' })).toBe('');
  });

  it('la apertura manual del cajón pide motivo e informa solo el envío del comando', async () => {
    const shift = shiftOf();
    serve({
      ...caja(shift, detailOf(shift), true),
      payments_open_drawer: () => ({
        status: 200,
        body: { requested: true, status: 'pending', job_id: 'cajon', event_id: 'evento' },
      }),
    });
    render(<Page />);
    fireEvent.click(await screen.findByRole('button', { name: 'Abrir cajón' }));
    const send = screen.getByRole('button', { name: 'Enviar comando de apertura' });
    expect((send as HTMLButtonElement).disabled).toBe(true);
    fireEvent.change(screen.getByLabelText(/Motivo de la apertura del cajón/), {
      target: { value: 'Cambio para el fondo' },
    });
    fireEvent.click(send);
    const confirmation = await screen.findByRole('status', { name: 'Operación completada' });
    expect(confirmation.textContent).toContain('Comando de apertura enviado.');
    expect(sent('payments_open_drawer')[0]?.body.reason).toBe('Cambio para el fondo');
    expect(document.body.textContent).not.toMatch(/cajón abierto|se abrió/i);
  });

  it('un comando del cajón con resultado incierto queda como aviso persistente', async () => {
    const shift = shiftOf();
    serve({
      ...caja(shift, detailOf(shift), true),
      payments_open_drawer: () => ({
        status: 200,
        body: { requested: true, status: 'uncertain', job_id: 'cajon', event_id: 'evento' },
      }),
    });
    render(<Page />);
    fireEvent.click(await screen.findByRole('button', { name: 'Abrir cajón' }));
    fireEvent.change(screen.getByLabelText(/Motivo de la apertura del cajón/), {
      target: { value: 'Cambio para el fondo' },
    });
    fireEvent.click(screen.getByRole('button', { name: 'Enviar comando de apertura' }));
    const warning = await screen.findByRole('status', { name: 'Atención' });
    expect(warning.textContent).toContain('No se reintenta automáticamente');
    expect(warning.textContent).not.toMatch(/cajón abierto|se abrió/i);
  });

  it('sin el permiso no ofrece abrir el cajón', async () => {
    const shift = shiftOf();
    serve(caja(shift, detailOf(shift), true));
    render(<Page can={(permission) => permission !== 'cash.drawer.open'} />);
    await screen.findByLabelText('Turno de Caja 1');
    expect(screen.queryByRole('button', { name: 'Abrir cajón' })).toBeNull();
  });
});

describe('tiempo real', () => {
  it('un evento de caja vuelve a consultar de inmediato y refleja la pausa de otra terminal', async () => {
    let shift: Shift = shiftOf();
    serve({
      payments_cash_registers: () => ({ status: 200, body: registersWith(shift) }),
      payments_current_shift: () => ({ status: 200, body: currentWith(shift) }),
      payments_cash_catalog: () => ({ status: 200, body: catalog }),
      payments_shift: () => ({ status: 200, body: detailOf(shift) }),
    });
    render(<Page can={(permission) => permission !== 'cash.counts.perform'} />);
    await screen.findByLabelText('Turno de Caja 1');
    const before = sent('payments_cash_registers').length;
    // Otra terminal inicia el arqueo: solo llega el evento de caja (sin esperar el sondeo).
    shift = closingShift();
    act(() => {
      window.dispatchEvent(
        new CustomEvent('adel-cash-change', {
          detail: { kinds: ['cash.shift.closing_started'], aggregates: [shiftID] },
        }),
      );
    });
    await waitFor(() => expect(sent('payments_cash_registers').length).toBeGreaterThan(before));
    const banner = await screen.findByRole('status', { name: 'Cierre en curso' });
    expect(banner.textContent).toContain('Supervisora Ana');
  });
});
