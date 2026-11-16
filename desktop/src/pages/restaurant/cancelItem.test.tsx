// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { APIError } from '../../services/api';
import type { Bill, ConfirmedLine } from '../../services/restaurant';
import { CancelItem } from './CancelItem';

const notices = vi.hoisted(() => ({ error: vi.fn(), success: vi.fn() }));
vi.mock('../../services/notify', async (original) => ({
  ...(await original<typeof import('../../services/notify')>()),
  notifyError: notices.error,
  notifySuccess: notices.success,
}));

// Etapa 9H, fase A (§2.2): «Cancelar unidades» no pide autorización de supervisor (decisión del
// propietario). El panel solo se ofrece con el permiso propio; envía la cantidad y el motivo, nunca una
// autorización, y un rechazo de Go se avisa sin abrir un formulario de supervisor. Hasta la 9H (etapa
// 9F) este archivo probaba la autorización de un supervisor para quien no tenía el permiso.

const accountID = '00000000-0000-4000-8000-0000000000c1';

function line(): ConfirmedLine {
  return {
    preparation_state: 'released',
    version_id: 'version-1',
    item_id: 'item-1',
    original_order_id: 'order-1',
    version: 1,
    state: 'active',
    quantity: 3,
    request: { product_id: 'producto', variant_id: null, quantity: 3 },
    quote_line: {
      name: 'Tacos 9H',
      quantity: 3,
      subtotal: '345.00',
      discount: '0.00',
      total: '345.00',
    },
    totals: { subtotal: '345.00', discount: '0.00', total: '345.00' },
    created_at: '2026-11-15T18:00:00Z',
  } as unknown as ConfirmedLine;
}

function bill(): Bill {
  return {
    account: {
      id: accountID,
      folio: 7,
      revision: 5,
      state: 'open',
      mode: 'dine_in',
    },
    draft: [],
    lines: [line()],
    server_time: '2026-11-15T18:00:00Z',
    timezone: 'America/Monterrey',
  } as unknown as Bill;
}

function renderCancel(execute = vi.fn().mockResolvedValue({})) {
  const onClose = vi.fn();
  render(
    <CancelItem bill={bill()} line={line()} execute={execute} busy={false} onClose={onClose} />,
  );
  return { execute, onClose };
}

function submitButton() {
  return screen.getByRole('button', { name: 'Confirmar cancelación de unidades' });
}

async function confirmDialog() {
  fireEvent.click(submitButton());
  const dialog = await screen.findByRole('alertdialog', {
    name: /^¿Cancelar 3 unidades de Tacos 9H\?$/,
  });
  expect(dialog.textContent).not.toContain('Autoriza');
  fireEvent.click(within(dialog).getByRole('button', { name: 'Cancelar unidades' }));
}

function fillReason() {
  fireEvent.change(screen.getByLabelText('Motivo de cancelación'), {
    target: { value: 'Se enfrió en la mesa' },
  });
}

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe('cancelar unidades sin autorización de supervisor (etapa 9H)', () => {
  it('envía la cantidad y el motivo, sin autorización', async () => {
    const { execute, onClose } = renderCancel();
    expect(screen.queryByRole('group', { name: 'Autorización de supervisor' })).toBeNull();
    expect(screen.queryByLabelText('Contraseña del supervisor')).toBeNull();
    fillReason();
    await confirmDialog();
    await waitFor(() => expect(execute).toHaveBeenCalledTimes(1));
    expect(execute).toHaveBeenCalledWith('restaurant_cancel', {
      id: accountID,
      body: { revision: 5, version_id: 'version-1', quantity: 3, reason: 'Se enfrió en la mesa' },
    });
    await waitFor(() => expect(onClose).toHaveBeenCalled());
    expect(notices.success).toHaveBeenCalledWith('Cancelación registrada: 3 unidades de Tacos 9H');
  });

  it('un rechazo de Go por permisos se avisa sin pedir un supervisor', async () => {
    const execute = vi
      .fn()
      .mockRejectedValueOnce(
        new APIError(
          'PERMISSION_DENIED',
          'No tienes permiso para esta operación del restaurante.',
          403,
        ),
      );
    const { onClose } = renderCancel(execute);
    fillReason();
    await confirmDialog();
    await waitFor(() => expect(notices.error).toHaveBeenCalledTimes(1));
    expect(notices.error.mock.calls[0]![0]).toBeInstanceOf(APIError);
    expect(screen.queryByRole('group', { name: 'Autorización de supervisor' })).toBeNull();
    expect(execute).toHaveBeenCalledTimes(1);
    expect(onClose).not.toHaveBeenCalled();
  });
});
