// @vitest-environment jsdom

import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { PendingOperationsProvider } from '../../hooks/pendingOperations';
import type { CurrentShift, Shift, ShiftMovements } from '../../services/payments';
import { ShiftOpening } from '../restaurant/checkout/ShiftOpening';
import { ShiftOverview } from './ShiftOverview';

const native = vi.hoisted(() => ({ invoke: vi.fn() }));
vi.mock('@tauri-apps/api/core', () => ({ invoke: native.invoke, isTauri: () => true }));

// Solo se sustituye el transporte nativo: el efectivo esperado aparece únicamente si Go lo entrega.
const shift: Shift = {
  id: '00000000-0000-0000-0000-0000000000a1',
  register_id: 'register',
  register_name: 'Caja 1',
  state: 'open',
  opening_float: '500.00',
  opened_by: 'user',
  opened_by_name: 'Cajera',
  opened_device_name: 'Terminal 1',
  opened_at: '2026-11-09T15:00:00Z',
  closed_at: null,
  revision: 1,
  shared_allowed: false,
};

function movements(current: Shift, expected?: string): ShiftMovements {
  const result: ShiftMovements = {
    shift: current,
    movements: [],
    by_method: [
      { method_name: 'Efectivo', is_cash: true, sales: '100.00', tips: '0.00', net: '100.00' },
    ],
    cash_net: '100.00',
    expected_visible: expected !== undefined,
  };
  if (expected !== undefined) {
    result.expected_cash = expected;
  }
  return result;
}

function serve(current: Shift, expected?: string) {
  const state: CurrentShift = {
    register: { id: 'register', name: 'Caja 1', active: true },
    shift: current,
    shared_shifts: false,
    usable: current.state === 'open',
    reason:
      current.state === 'closing'
        ? 'La caja Caja 1 está en arqueo de cierre (lo inició Supervisor).'
        : '',
  };
  native.invoke.mockImplementation(async (command: string, args: { operation: string }) => {
    if (command !== 'api_request') {
      throw new Error('Comando no esperado');
    }
    if (args.operation === 'payments_current_shift') {
      return { status: 200, body: state };
    }
    if (args.operation === 'payments_shift_movements') {
      return { status: 200, body: movements(current, expected) };
    }
    throw new Error(`Operación no esperada: ${args.operation}`);
  });
}

afterEach(() => {
  cleanup();
  native.invoke.mockReset();
});

describe('ShiftOverview', () => {
  it('muestra el efectivo esperado solo cuando Go lo entrega', async () => {
    serve(shift, '600.00');
    render(<ShiftOverview can={() => true} timezone="America/Monterrey" />);
    expect(await screen.findByText('$600.00')).toBeTruthy();
    expect(screen.getByText(/Pesos esperados en caja/)).toBeTruthy();
  });

  it('sin el permiso explica que el esperado se omite y no lo deduce', async () => {
    serve(shift);
    render(<ShiftOverview can={() => true} timezone="America/Monterrey" />);
    expect(await screen.findByText(/se muestra con el permiso/)).toBeTruthy();
    expect(screen.queryByText(/Pesos esperados en caja/)).toBeNull();
    expect(screen.queryByText('$600.00')).toBeNull();
  });

  it('durante el arqueo muestra quién lo inició y que la captura es ciega', async () => {
    serve({
      ...shift,
      state: 'closing',
      closing_started_by_name: 'Supervisor',
      closing_started_at: '2026-11-09T22:00:00Z',
    });
    render(<ShiftOverview can={() => true} timezone="America/Monterrey" />);
    expect(await screen.findByText('Cierre en curso')).toBeTruthy();
    expect(screen.getByText('Supervisor')).toBeTruthy();
    expect(screen.getByText(/está en arqueo de cierre/)).toBeTruthy();
    expect(await screen.findByText(/no se muestra durante el conteo/)).toBeTruthy();
  });

  it('el cobro con la caja en arqueo explica la pausa y no ofrece abrir otro turno', () => {
    const current: CurrentShift = {
      register: { id: 'register', name: 'Caja 1', active: true },
      shift: { ...shift, state: 'closing', closing_started_by_name: 'Supervisor' },
      shared_shifts: false,
      usable: false,
      reason: 'La caja Caja 1 está en arqueo de cierre (lo inició Supervisor).',
    };
    render(
      <PendingOperationsProvider>
        <ShiftOpening current={current} can={() => true} active onChanged={() => undefined} />
      </PendingOperationsProvider>,
    );
    expect(screen.getByText(/está en arqueo de cierre/)).toBeTruthy();
    expect(screen.queryByLabelText('Fondo inicial')).toBeNull();
  });
});
