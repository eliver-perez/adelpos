// @vitest-environment jsdom

import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { PendingOperationsProvider } from '../../hooks/pendingOperations';
import type { CashCount, ClosureView } from '../../services/cash';
import type { Shift } from '../../services/payments';
import { CashPage } from './CashPage';
import { ClosureResult } from './ClosureResult';
import {
  catalog,
  closingShift,
  currentWith,
  failure,
  registersWith,
  shiftID,
  shiftOf,
} from './cashTestData';

const native = vi.hoisted(() => ({ invoke: vi.fn() }));
vi.mock('@tauri-apps/api/core', () => ({ invoke: native.invoke, isTauri: () => true }));

type Reply = { status: number; body: unknown };

function draft(revision: number, overrides: Partial<CashCount> = {}): CashCount {
  return {
    id: 'count-closing',
    shift_id: shiftID,
    purpose: 'closing',
    state: 'draft',
    lines: [],
    extra_mxn: '0.00',
    extra_usd: '0.00',
    extra_note: '',
    notes: '',
    counted_mxn: '0.00',
    counted_usd: '0.00',
    revision,
    updated_by_name: 'Supervisora Ana',
    updated_at: '2026-11-09T22:00:00Z',
    ...overrides,
  };
}

// Respuestas que «filtran» el esperado y diferencias: la pantalla de arqueo no debe mostrarlos
// aunque llegaran (Go nunca los envía durante el arqueo).
const leaked = {
  expected_visible: true,
  expected: {
    mxn: '987.65',
    usd: '0.00',
    usd_rate: null,
    usd_equivalent_mxn: null,
    total_equivalent_mxn: null,
  },
  differences: {
    mxn: '-12.34',
    usd: '0.00',
    result_mxn: 'short',
    result_usd: 'even',
    equivalent_mxn: null,
  },
};

let shift: Shift;
// Borrador vigente en el «servidor» simulado: guardar lo sustituye con una revisión nueva.
let serverDraft: CashCount;
let handlers: Record<string, (body: Record<string, unknown>) => Reply>;

function sent(operation: string) {
  return native.invoke.mock.calls
    .filter(([command, args]) => command === 'api_request' && args?.operation === operation)
    .map(([, args]) => args as { body: Record<string, unknown> });
}

function closureView(visible: boolean): ClosureView {
  const view: ClosureView = {
    shift: shiftOf({ state: 'closed', closed_at: '2026-11-09T23:00:00Z' }),
    count: draft(5, {
      state: 'final',
      counted_mxn: '380.00',
      lines: [
        {
          denomination_id: 'den-mxn-100',
          currency: 'MXN',
          kind: 'bill',
          value: '100.00',
          quantity: 3,
          amount: '300.00',
        },
      ],
    }),
    cashier_notes: 'Sin novedades',
    closed_by_name: 'Supervisora Ana',
    device_name: 'Terminal de barra',
    closed_at: '2026-11-09T23:00:00Z',
    final_report_id: 'report-final',
    regularizations: [],
    expected_visible: visible,
  };
  if (visible) {
    view.expected = { ...leaked.expected, mxn: '450.00' };
    view.differences = { ...leaked.differences, mxn: '-70.00' } as ClosureView['differences'];
    view.comments = [];
  }
  return view;
}

beforeEach(() => {
  shift = closingShift();
  serverDraft = draft(3);
  handlers = {
    payments_cash_registers: () => ({ status: 200, body: registersWith(shift) }),
    payments_current_shift: () => ({ status: 200, body: currentWith(shift) }),
    payments_cash_catalog: () => ({ status: 200, body: catalog }),
    payments_shift: () => ({
      status: 200,
      body: {
        shift,
        opening_count: null,
        reports: [],
        summary: { payments: 4, refunds: 0, cash_operations: 2, partial_cuts: 1 },
        ...leaked,
      },
    }),
    payments_shift_closing: () => ({ status: 200, body: { shift, count: serverDraft, ...leaked } }),
    payments_cash_count_preview: (body) => ({
      status: 200,
      body: {
        lines: (body.lines as { denomination_id: string; quantity: number }[]).map((line) => ({
          ...line,
          currency: 'MXN',
          kind: 'bill',
          value: '500.00',
          amount: '500.00',
        })),
        extra_mxn: '0.00',
        extra_usd: '0.00',
        extra_note: '',
        counted_mxn: (body.lines as unknown[]).length > 0 ? '500.00' : '0.00',
        counted_usd: '0.00',
        ...leaked,
      },
    }),
    payments_shift_save_count: (body) => {
      serverDraft = draft(serverDraft.revision + 1, {
        counted_mxn: '500.00',
        lines: (body.lines as { denomination_id: string; quantity: number }[]).map((line) => ({
          ...line,
          currency: 'MXN',
          kind: 'bill',
          value: '500.00',
          amount: '500.00',
        })),
      });
      return { status: 200, body: { shift, count: serverDraft } };
    },
    payments_shift_confirm_closing: () => ({
      status: 200,
      body: {
        shift: { ...shift, state: 'closed' },
        count: draft(4),
        final_report_id: 'r',
        closed: true,
      },
    }),
    payments_shift_closure: () => ({ status: 200, body: closureView(false) }),
  };
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
});

afterEach(() => {
  cleanup();
  native.invoke.mockReset();
});

function Page() {
  return (
    <PendingOperationsProvider>
      <CashPage can={() => true} userID="user-admin" timezone="America/Monterrey" />
    </PendingOperationsProvider>
  );
}

async function countFiveHundred() {
  const field = await screen.findByLabelText('Billetes de $500');
  fireEvent.click(field);
  const keypad = screen.getByRole('group', { name: 'Teclado numérico del arqueo' });
  // El campo aparece en el mismo render que trae el arqueo, pero el borrador del servidor se adopta en
  // un efecto posterior que sustituye la captura. En un equipo lento (se observó en Windows, etapa 9E)
  // el toque podía llegar antes y quedar reemplazado: primero se vacían los efectos pendientes.
  await act(async () => {});
  fireEvent.click(within(keypad).getByRole('button', { name: '1' }));
  await act(async () => {});
  expect((field as HTMLInputElement).value).toBe('1');
}

describe('arqueo y cierre ciego', () => {
  it('muestra el aviso «Cierre en curso» con quién, cuándo y desde qué terminal', async () => {
    render(<Page />);
    const banner = await screen.findByRole('status', { name: 'Cierre en curso' });
    expect(banner.textContent).toContain('Cierre en curso · Caja 1');
    expect(banner.textContent).toContain('Supervisora Ana');
    expect(banner.textContent).toContain('Terminal de barra');
    expect(banner.textContent).toMatch(/09\/11\/26/);
  });

  it('la pantalla de arqueo no muestra esperado ni diferencias aunque la respuesta los traiga', async () => {
    render(<Page />);
    await screen.findByRole('region', { name: 'Arqueo de cierre en curso' });
    await countFiveHundred();
    await waitFor(() =>
      expect(screen.getByLabelText('Total declarado').textContent).toContain('$500.00'),
    );
    const text = document.body.textContent ?? '';
    expect(text).not.toContain('987.65');
    expect(text).not.toContain('12.34');
    expect(text).not.toMatch(/Efectivo esperado/);
    expect(text).not.toMatch(/Diferencia/);
    expect(screen.queryByRole('group', { name: /Diferencias/ })).toBeNull();
  });

  it('guarda el borrador con su revisión y confirma con count_revision', async () => {
    render(<Page />);
    await countFiveHundred();
    await waitFor(() => expect(sent('payments_shift_save_count')).toHaveLength(1), {
      timeout: 3000,
    });
    expect(sent('payments_shift_save_count')[0]!.body).toMatchObject({
      revision: 3,
      lines: [{ denomination_id: 'den-mxn-500', quantity: 1 }],
      notes: '',
    });
    await screen.findByText('Borrador guardado.');
    fireEvent.click(screen.getByRole('button', { name: 'Confirmar cierre del turno' }));
    const dialog = await screen.findByRole('alertdialog');
    expect(dialog.textContent).toContain('$500.00 declarados');
    // Cerrar el turno es irreversible: el diálogo estándar usa la variante de peligro.
    expect(dialog.className).toContain('dialog-panel-danger');
    fireEvent.click(within(dialog).getByRole('button', { name: 'Cerrar el turno' }));
    await waitFor(() => expect(sent('payments_shift_confirm_closing')).toHaveLength(1));
    expect(sent('payments_shift_confirm_closing')[0]!.body).toMatchObject({ count_revision: 4 });
    // El aviso confirma el cierre sin ninguna cifra; el resultado sigue en pantalla.
    const confirmation = await screen.findByRole('status', { name: 'Operación completada' });
    expect(confirmation.textContent).toContain('Turno de Caja 1 cerrado.');
    expect(confirmation.textContent).not.toMatch(/\$|500|987|12\.34/);
    await screen.findByText(/quedó cerrado/);
    expect(screen.getByText(/se consultan con el permiso/)).toBeTruthy();
  });

  it('ante COUNT_CHANGED recarga, conserva lo capturado y avisa', async () => {
    handlers.payments_shift_save_count = () =>
      failure(
        409,
        'COUNT_CHANGED',
        'El conteo cambió en otra terminal. Actualiza la pantalla antes de guardar.',
      );
    render(<Page />);
    await countFiveHundred();
    await screen.findByText('El conteo cambió en otra terminal.', {}, { timeout: 3000 });
    expect((screen.getByLabelText('Billetes de $500') as HTMLInputElement).value).toBe('1');
    expect(screen.getByRole('button', { name: 'Guardar mi conteo' })).toBeTruthy();
  });

  it('explica CLOSING_MOVEMENTS_CHANGED y ofrece cancelar el arqueo', async () => {
    handlers.payments_shift_confirm_closing = () =>
      failure(
        409,
        'CLOSING_MOVEMENTS_CHANGED',
        'Se registraron movimientos en el turno después de iniciar el arqueo.',
      );
    render(<Page />);
    await screen.findByRole('region', { name: 'Arqueo de cierre en curso' });
    await waitFor(() =>
      expect(
        (screen.getByRole('button', { name: 'Confirmar cierre del turno' }) as HTMLButtonElement)
          .disabled,
      ).toBe(false),
    );
    fireEvent.click(screen.getByRole('button', { name: 'Confirmar cierre del turno' }));
    fireEvent.click(
      within(await screen.findByRole('alertdialog')).getByRole('button', {
        name: 'Cerrar el turno',
      }),
    );
    await screen.findByText('Se registraron movimientos después de iniciar el arqueo.');
    expect(
      screen.getByRole('button', { name: 'Cancelar el arqueo y volver a contar' }),
    ).toBeTruthy();
  });
});

describe('resultado del cierre', () => {
  it('con el permiso muestra esperado y diferencias de Go', async () => {
    handlers.payments_shift_closure = () => ({ status: 200, body: closureView(true) });
    render(
      <PendingOperationsProvider>
        <ClosureResult shiftID={shiftID} can={() => true} timezone="America/Monterrey" />
      </PendingOperationsProvider>,
    );
    await screen.findByText(/quedó cerrado/);
    expect(screen.getByLabelText('Efectivo esperado en caja').textContent).toContain('$450.00');
    const differences = screen.getByLabelText('Diferencias del arqueo');
    expect(differences.textContent).toContain('Diferencia en pesos · Faltante');
    expect(differences.textContent).toContain('−$70.00');
    expect(screen.getByText('3 × Billetes de $100 = $300.00')).toBeTruthy();
  });

  it('sin el permiso muestra solo lo declarado y que el turno quedó cerrado', async () => {
    handlers.payments_shift_closure = () => ({ status: 200, body: closureView(false) });
    render(
      <PendingOperationsProvider>
        <ClosureResult shiftID={shiftID} can={() => false} timezone="America/Monterrey" />
      </PendingOperationsProvider>,
    );
    await screen.findByText(/quedó cerrado/);
    expect(screen.getByLabelText('Efectivo declarado').textContent).toContain('$380.00');
    expect(screen.queryByLabelText('Efectivo esperado en caja')).toBeNull();
    expect(screen.queryByLabelText('Diferencias del arqueo')).toBeNull();
    expect(screen.queryByText('Comentarios del supervisor')).toBeNull();
  });
});
