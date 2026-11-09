// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { PendingOperationsProvider } from '../../hooks/pendingOperations';
import type { ShiftHistory } from '../../services/cash';
import { HistoryView } from './HistoryView';
import { registersWith, shiftOf } from './cashTestData';

const native = vi.hoisted(() => ({ invoke: vi.fn() }));
vi.mock('@tauri-apps/api/core', () => ({ invoke: native.invoke, isTauri: () => true }));

const visibleShift = shiftOf({
  id: '00000000-0000-4000-8000-0000000000d1',
  state: 'closed',
  business_date: '2026-11-08',
  closed_at: '2026-11-09T05:00:00Z',
  closed_by_name: 'Supervisora Ana',
});
const hiddenShift = shiftOf({
  id: '00000000-0000-4000-8000-0000000000d2',
  state: 'closed',
  business_date: '2026-11-07',
  closed_at: '2026-11-08T05:00:00Z',
  closed_by_name: 'Cajero Luis',
});

// Historial simulado: el primer cierre trae diferencias (permiso de consultar el esperado) y el
// segundo solo lo declarado, como lo entrega Go a quien no tiene ese permiso.
const history: ShiftHistory = {
  items: [
    {
      shift: visibleShift,
      closure: {
        counted_mxn: '380.00',
        counted_usd: '0.00',
        expected_mxn: '450.00',
        expected_usd: '0.00',
        difference_mxn: '-70.00',
        difference_usd: '0.00',
        result_mxn: 'short',
        result_usd: 'even',
        difference_equivalent_mxn: null,
      },
    },
    { shift: hiddenShift, closure: { counted_mxn: '1200.00', counted_usd: '0.00' } },
  ],
  page: 1,
  page_size: 50,
  total: 2,
};

beforeEach(() => {
  native.invoke.mockImplementation(async (command: string, args: { operation: string }) => {
    if (command !== 'api_request') {
      throw new Error('Comando no esperado');
    }
    if (args.operation === 'payments_shifts') {
      return { status: 200, body: history };
    }
    if (args.operation === 'payments_shift_closure') {
      return {
        status: 200,
        body: {
          shift: hiddenShift,
          count: null,
          cashier_notes: '',
          closed_by_name: 'Cajero Luis',
          device_name: 'Terminal 2',
          closed_at: '2026-11-08T05:00:00Z',
          final_report_id: 'report',
          regularizations: [],
          expected_visible: false,
        },
      };
    }
    return { status: 200, body: { reports: [], movements: [], operations: [] } };
  });
});

afterEach(() => {
  cleanup();
  native.invoke.mockReset();
});

describe('historial de turnos', () => {
  it('muestra diferencias solo cuando Go las entrega', async () => {
    render(
      <PendingOperationsProvider>
        <HistoryView
          registers={registersWith(null)}
          ownShift={null}
          can={() => true}
          timezone="America/Monterrey"
        />
      </PendingOperationsProvider>,
    );
    const visibleRow = (await screen.findByText('08/11/2026')).closest('tr')!;
    expect(visibleRow.textContent).toContain('$380.00');
    expect(visibleRow.textContent).toContain('−$70.00 · Faltante');
    const hiddenRow = screen.getByText('07/11/2026').closest('tr')!;
    expect(hiddenRow.textContent).toContain('$1200.00');
    expect(hiddenRow.textContent).not.toMatch(/Faltante|Sobrante|Sin diferencia/);
    fireEvent.click(
      within(hiddenRow).getByRole('button', { name: 'Detalle del turno de Caja 1 del 07/11/2026' }),
    );
    await screen.findByText(/quedó cerrado/);
    expect(screen.queryByLabelText('Diferencias del arqueo')).toBeNull();
    expect(screen.getByText(/se consultan con el permiso/)).toBeTruthy();
  });
});
