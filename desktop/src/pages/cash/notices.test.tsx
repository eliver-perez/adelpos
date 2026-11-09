// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { PendingOperationsProvider } from '../../hooks/pendingOperations';
import { denominationLabel } from '../../services/cash';
import type { ShiftDetail } from '../../services/cash';
import type { Shift } from '../../services/payments';
import { CashPage } from './CashPage';
import { catalog, currentWith, registersWith, shiftOf } from './cashTestData';
import { PartialCutPanel } from './PartialCutPanel';
import { useCashMutation } from './useCashMutation';

const native = vi.hoisted(() => ({ invoke: vi.fn() }));
vi.mock('@tauri-apps/api/core', () => ({ invoke: native.invoke, isTauri: () => true }));

type Reply = { status: number; body: unknown };

// Simula a Go por operación; las cifras son las que entregaría Go y la prueba no calcula importes.
function serve(handlers: Record<string, () => Reply>) {
  native.invoke.mockImplementation(async (command: string, args: { operation: string }) => {
    if (command !== 'api_request') {
      throw new Error('Comando no esperado');
    }
    const handler = handlers[args.operation];
    if (!handler) {
      return { status: 200, body: {} };
    }
    return handler();
  });
}

afterEach(() => {
  cleanup();
  native.invoke.mockReset();
});

function PartialHarness({ shift }: { shift: Shift }) {
  const mutation = useCashMutation(shift.id);
  return (
    <PartialCutPanel
      shift={shift}
      reports={[]}
      can={() => true}
      mutation={mutation}
      timezone="America/Monterrey"
      onWithdraw={() => undefined}
      onClose={() => undefined}
    />
  );
}

describe('avisos de caja (etapa 7.1)', () => {
  it('el corte parcial se confirma con un aviso sin cifras y otro con su impresión', async () => {
    const shift = shiftOf();
    serve({
      payments_shift_partial_cut: () => ({
        status: 200,
        body: {
          id: 'corte-2',
          shift_id: shift.id,
          kind: 'partial',
          sequence: 2,
          expected_visible: true,
          printing: { requested: true, status: 'pending', includes_expected: false },
        },
      }),
      payments_cash_report: () => ({
        status: 404,
        body: { error: { code: 'REPORT_NOT_FOUND', message: 'El corte no está disponible.' } },
      }),
    });
    render(
      <PendingOperationsProvider>
        <PartialHarness shift={shift} />
      </PendingOperationsProvider>,
    );
    fireEvent.click(screen.getByRole('button', { name: 'Generar corte parcial' }));
    const created = await screen.findByText('Corte parcial #2 generado.');
    expect(created.closest('[role="status"]')?.getAttribute('aria-label')).toBe(
      'Operación completada',
    );
    expect(await screen.findByText('Documento enviado a la cola de impresión.')).toBeTruthy();
    // Ningún aviso lleva importes: el informe (y el esperado, con permiso) se consulta en el panel.
    const notices = Array.from(document.querySelectorAll('.app-toast'));
    expect(notices.length).toBe(2);
    for (const notice of notices) {
      expect(notice.textContent).not.toMatch(/\$/);
    }
  });

  it('un fallo al generar el corte es un error persistente con el texto de Go', async () => {
    const shift = shiftOf();
    serve({
      payments_shift_partial_cut: () => ({
        status: 409,
        body: { error: { code: 'SHIFT_CLOSING', message: 'La caja Caja 1 está en arqueo.' } },
      }),
    });
    render(
      <PendingOperationsProvider>
        <PartialHarness shift={shift} />
      </PendingOperationsProvider>,
    );
    fireEvent.click(screen.getByRole('button', { name: 'Generar corte parcial' }));
    const alert = await screen.findByRole('alert', { name: 'Error' });
    expect(alert.textContent).toContain('La caja Caja 1 está en arqueo.');
    expect(alert.textContent).not.toContain('SHIFT_CLOSING');
  });

  it('los registros del turno concuerdan en número', async () => {
    const shift = shiftOf();
    const detail: ShiftDetail = {
      shift,
      opening_count: null,
      reports: [],
      summary: { payments: 1, refunds: 0, cash_operations: 1, partial_cuts: 1 },
      expected_visible: false,
    };
    serve({
      payments_cash_registers: () => ({ status: 200, body: registersWith(shift) }),
      payments_current_shift: () => ({ status: 200, body: currentWith(shift) }),
      payments_cash_catalog: () => ({ status: 200, body: catalog }),
      payments_shift: () => ({ status: 200, body: detail }),
    });
    render(
      <PendingOperationsProvider>
        <CashPage can={() => true} userID="user-admin" timezone="America/Monterrey" />
      </PendingOperationsProvider>,
    );
    const records = await screen.findByText(/Registros del turno/);
    expect(records.textContent).toBe(
      'Registros del turno: 1 cobro · 0 devoluciones · 1 operación de efectivo · 1 corte parcial.',
    );
  });

  it('las denominaciones usan el formato único con miles y sin decimales si son enteras', () => {
    expect(denominationLabel({ currency: 'MXN', kind: 'bill', value: '1000.00' })).toBe(
      'Billetes de $1,000',
    );
    expect(denominationLabel({ currency: 'MXN', kind: 'coin', value: '0.50' })).toBe(
      'Monedas de $0.50',
    );
    expect(denominationLabel({ currency: 'USD', kind: 'bill', value: '100.00' })).toBe(
      'Billetes de US$100',
    );
  });
});
