// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { PendingOperationsProvider } from '../../hooks/pendingOperations';
import type { CountPreview, CountPreviewInput } from '../../services/cash';
import { ShiftOpening } from '../restaurant/checkout/ShiftOpening';
import { catalog, currentWith, shiftOf } from './cashTestData';

const native = vi.hoisted(() => ({ invoke: vi.fn() }));
vi.mock('@tauri-apps/api/core', () => ({ invoke: native.invoke, isTauri: () => true }));

// Vista previa simulada de Go: los importes son los que calcularía el servidor para el conteo.
function preview(input: CountPreviewInput): CountPreview {
  const lines = input.lines.map((line) => ({
    denomination_id: line.denomination_id,
    currency: 'MXN' as const,
    kind: 'bill' as const,
    value: line.denomination_id === 'den-mxn-200' ? '200.00' : '100.00',
    quantity: line.quantity,
    amount: line.denomination_id === 'den-mxn-200' ? '400.00' : '100.00',
  }));
  return {
    lines,
    extra_mxn: '0.00',
    extra_usd: '0.00',
    extra_note: '',
    counted_mxn: lines.length === 2 ? '500.00' : '400.00',
    counted_usd: '0.00',
  };
}

function calls(operation: string) {
  return native.invoke.mock.calls
    .filter(([command, args]) => command === 'api_request' && args?.operation === operation)
    .map(([, args]) => args as { body: Record<string, unknown> });
}

beforeEach(() => {
  native.invoke.mockImplementation(
    async (command: string, args: { operation: string; body?: unknown }) => {
      if (command !== 'api_request') {
        throw new Error('Comando no esperado');
      }
      switch (args.operation) {
        case 'payments_cash_catalog':
          return { status: 200, body: catalog };
        case 'payments_cash_count_preview':
          return { status: 200, body: preview(args.body as CountPreviewInput) };
        case 'payments_open_shift':
          return { status: 200, body: shiftOf() };
        default:
          throw new Error(`Operación no esperada: ${args.operation}`);
      }
    },
  );
});

afterEach(() => {
  cleanup();
  native.invoke.mockReset();
});

function Opening() {
  return (
    <PendingOperationsProvider>
      <ShiftOpening
        current={currentWith(null)}
        can={() => true}
        active
        onChanged={() => undefined}
      />
    </PendingOperationsProvider>
  );
}

function keypad() {
  return screen.getByRole('group', { name: /^Teclado numérico/ });
}

describe('apertura ampliada del turno', () => {
  it('abre por total con fondo de $0 sin pedir desglose', async () => {
    render(<Opening />);
    fireEvent.click(within(keypad()).getByRole('button', { name: '0' }));
    fireEvent.click(screen.getByRole('button', { name: 'Abrir turno con fondo de $0' }));
    await waitFor(() => expect(calls('payments_open_shift')).toHaveLength(1));
    const body = calls('payments_open_shift')[0]!.body;
    expect(body.opening_float).toBe('0');
    expect(body).not.toHaveProperty('count');
    expect(calls('payments_cash_catalog')).toHaveLength(0);
    expect(calls('payments_cash_count_preview')).toHaveLength(0);
  });

  it('abre por denominaciones con el total que calcula Go', async () => {
    render(<Opening />);
    fireEvent.click(screen.getByRole('button', { name: 'Por denominaciones' }));
    fireEvent.click(await screen.findByLabelText('Billetes de $200'));
    fireEvent.click(within(keypad()).getByRole('button', { name: '2' }));
    fireEvent.click(screen.getByLabelText('Billetes de $100'));
    fireEvent.click(within(keypad()).getByRole('button', { name: '1' }));
    expect((screen.getByLabelText('Billetes de $200') as HTMLInputElement).value).toBe('2');
    expect((screen.getByLabelText('Billetes de $100') as HTMLInputElement).value).toBe('1');
    // El punto decimal no aplica a cantidades contadas.
    expect(
      (within(keypad()).getByRole('button', { name: 'Punto decimal' }) as HTMLButtonElement)
        .disabled,
    ).toBe(true);
    const open = await screen.findByRole('button', { name: 'Abrir turno con fondo de $500.00' });
    expect(screen.getByLabelText('Importe de Billetes de $200').textContent).toBe('$400.00');
    expect(screen.getByLabelText('Fondo contado').textContent).toContain('$500.00');
    const previews = calls('payments_cash_count_preview');
    expect(previews[previews.length - 1]!.body).toEqual({
      lines: [
        { denomination_id: 'den-mxn-200', quantity: 2 },
        { denomination_id: 'den-mxn-100', quantity: 1 },
      ],
    });
    fireEvent.change(screen.getByLabelText('Observaciones de la apertura (opcional)'), {
      target: { value: 'Fondo entregado por gerencia' },
    });
    fireEvent.click(open);
    await waitFor(() => expect(calls('payments_open_shift')).toHaveLength(1));
    expect(calls('payments_open_shift')[0]!.body).toMatchObject({
      opening_float: '500.00',
      opening_notes: 'Fondo entregado por gerencia',
      count: {
        lines: [
          { denomination_id: 'den-mxn-200', quantity: 2 },
          { denomination_id: 'den-mxn-100', quantity: 1 },
        ],
      },
    });
    expect(calls('payments_open_shift')[0]!.body).not.toHaveProperty('opening_float_usd');
  });

  it('sin dólares activos no muestra campos en dólares', () => {
    render(<Opening />);
    expect(screen.queryByLabelText('Fondo inicial en dólares')).toBeNull();
    expect(screen.queryByLabelText('Tipo de cambio del dólar')).toBeNull();
  });
});
