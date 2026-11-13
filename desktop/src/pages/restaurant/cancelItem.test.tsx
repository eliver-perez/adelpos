// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { APIError } from '../../services/api';
import type { Bill, ConfirmedLine } from '../../services/restaurant';
import { CancelItem } from './CancelItem';

// Etapa 9F, fase E (§7.2): «Cancelar unidades» con el permiso propio o con la autorización de un
// supervisor. Go decide si hace falta (`cancel_units_requires_authorization`); la interfaz muestra el
// formulario, envía la autorización solo entonces y nunca conserva la contraseña.

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
      name: 'Tacos 9F',
      quantity: 3,
      subtotal: '345.00',
      discount: '0.00',
      total: '345.00',
    },
    totals: { subtotal: '345.00', discount: '0.00', total: '345.00' },
    created_at: '2026-11-13T18:00:00Z',
  } as unknown as ConfirmedLine;
}

function bill(requiresAuthorization: boolean): Bill {
  return {
    account: {
      id: accountID,
      folio: 7,
      revision: 5,
      state: 'open',
      mode: 'dine_in',
      cancel_units_requires_authorization: requiresAuthorization,
    },
    draft: [],
    lines: [line()],
    server_time: '2026-11-13T18:00:00Z',
    timezone: 'America/Monterrey',
  } as unknown as Bill;
}

function renderCancel(requiresAuthorization: boolean, execute = vi.fn().mockResolvedValue({})) {
  const onClose = vi.fn();
  render(
    <CancelItem
      bill={bill(requiresAuthorization)}
      line={line()}
      execute={execute}
      busy={false}
      onClose={onClose}
    />,
  );
  return { execute, onClose };
}

function submitButton() {
  return screen.getByRole('button', { name: 'Confirmar cancelación de unidades' });
}

function disabled(element: HTMLElement) {
  return (element as HTMLButtonElement).disabled;
}

async function confirmDialog(expected?: string) {
  fireEvent.click(submitButton());
  const dialog = await screen.findByRole('alertdialog', {
    name: /^¿Cancelar 3 unidades de Tacos 9F\?$/,
  });
  if (expected) {
    expect(dialog.textContent).toContain(expected);
  }
  fireEvent.click(within(dialog).getByRole('button', { name: 'Cancelar unidades' }));
}

function fillReason() {
  fireEvent.change(screen.getByLabelText('Motivo de cancelación'), {
    target: { value: 'Se enfrió en la mesa' },
  });
}

function supervisorFields() {
  const group = screen.getByRole('group', { name: 'Autorización de supervisor' });
  return {
    group,
    user: within(group).getByLabelText('Usuario del supervisor') as HTMLInputElement,
    password: within(group).getByLabelText('Contraseña del supervisor') as HTMLInputElement,
  };
}

afterEach(cleanup);

describe('cancelar unidades con autorización (etapa 9F)', () => {
  it('con el permiso propio no pide supervisor ni envía autorización', async () => {
    const { execute, onClose } = renderCancel(false);
    expect(screen.queryByRole('group', { name: 'Autorización de supervisor' })).toBeNull();
    fillReason();
    await confirmDialog();
    await waitFor(() => expect(execute).toHaveBeenCalledTimes(1));
    expect(execute).toHaveBeenCalledWith('restaurant_cancel', {
      id: accountID,
      body: { revision: 5, version_id: 'version-1', quantity: 3, reason: 'Se enfrió en la mesa' },
    });
    await waitFor(() => expect(onClose).toHaveBeenCalled());
  });

  it('cuando Go lo indica pide al supervisor desde el principio y no conserva la contraseña', async () => {
    const { execute, onClose } = renderCancel(true);
    const { group, user, password } = supervisorFields();
    expect(group.textContent).toContain('«Cancelar partidas o cantidades confirmadas»');
    fillReason();
    // Sin usuario y contraseña del supervisor no se puede enviar.
    expect(disabled(submitButton())).toBe(true);
    fireEvent.change(user, { target: { value: 'admin' } });
    expect(disabled(submitButton())).toBe(true);
    fireEvent.change(password, { target: { value: 'clave-del-supervisor' } });
    expect(disabled(submitButton())).toBe(false);
    await confirmDialog('Autoriza: admin');
    await waitFor(() => expect(execute).toHaveBeenCalledTimes(1));
    expect(execute).toHaveBeenCalledWith('restaurant_cancel', {
      id: accountID,
      body: {
        revision: 5,
        version_id: 'version-1',
        quantity: 3,
        reason: 'Se enfrió en la mesa',
        authorization: { username: 'admin', password: 'clave-del-supervisor' },
      },
    });
    await waitFor(() => expect(onClose).toHaveBeenCalled());
    // La contraseña salió del formulario al enviar.
    expect(supervisorFields().password.value).toBe('');
  });

  it('un rechazo de Go por la autorización muestra el formulario con su mensaje y permite repetirla', async () => {
    const execute = vi
      .fn()
      .mockRejectedValueOnce(
        new APIError(
          'AUTHORIZATION_REQUIRED',
          'Cancelar o reducir unidades enviadas a cocina requiere la autorización de un supervisor con permiso para cancelar partidas.',
          403,
        ),
      )
      .mockRejectedValueOnce(
        new APIError(
          'AUTHORIZATION_INVALID',
          'Usuario o contraseña del supervisor inválidos.',
          401,
        ),
      )
      .mockResolvedValueOnce({});
    const { onClose } = renderCancel(false, execute);
    fillReason();
    await confirmDialog();
    // El formulario aparece aunque la cuenta no lo pedía, con el mensaje de Go y sin cerrar el panel.
    const alert = await screen.findByRole('alert');
    expect(alert.textContent).toContain('requiere la autorización de un supervisor');
    expect(onClose).not.toHaveBeenCalled();
    expect(disabled(submitButton())).toBe(true);
    let fields = supervisorFields();
    fireEvent.change(fields.user, { target: { value: 'supervisor' } });
    fireEvent.change(fields.password, { target: { value: 'incorrecta' } });
    await confirmDialog('Autoriza: supervisor');
    await waitFor(() =>
      expect(screen.getByRole('alert').textContent).toContain(
        'Usuario o contraseña del supervisor inválidos.',
      ),
    );
    fields = supervisorFields();
    expect(fields.password.value).toBe('');
    expect(fields.user.value).toBe('supervisor');
    fireEvent.change(fields.password, { target: { value: 'correcta' } });
    await confirmDialog();
    await waitFor(() => expect(execute).toHaveBeenCalledTimes(3));
    expect(execute.mock.calls[2]![1]).toEqual({
      id: accountID,
      body: {
        revision: 5,
        version_id: 'version-1',
        quantity: 3,
        reason: 'Se enfrió en la mesa',
        authorization: { username: 'supervisor', password: 'correcta' },
      },
    });
    await waitFor(() => expect(onClose).toHaveBeenCalled());
  });
});
