// @vitest-environment jsdom

import { act, cleanup, fireEvent, render, screen, within } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { DiscountReason } from '../../services/payments';
import { DiscountReasonsAdmin } from './DiscountReasonsAdmin';

function reason(id: string, name: string, order: number, extra: Partial<DiscountReason> = {}) {
  return {
    id: `00000000-0000-4000-8000-0000000003${id}`,
    name,
    requires_description: false,
    active: true,
    sort_order: order,
    revision: 1,
    updated_at: '2026-11-09T12:00:00Z',
    ...extra,
  };
}

const reasons: DiscountReason[] = [
  reason('05', 'Otro', 50, { requires_description: true }),
  reason('01', 'Atención', 10),
  reason('02', 'Promoción', 20, { active: false }),
];

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe('administración de motivos de descuento', () => {
  it('muestra los motivos en su orden y crea uno con su posición', async () => {
    const execute = vi.fn(async () => ({}) as never);
    render(
      <DiscountReasonsAdmin
        reasons={reasons}
        can={(permission) => permission === 'payments.settings.manage'}
        execute={execute}
        busy={false}
      />,
    );
    const group = screen.getByRole('group', { name: 'Motivos de descuento' });
    expect(
      within(group)
        .getAllByRole('button')
        .map((button) => button.textContent),
    ).toEqual(['Atención', 'Promoción · inactivo', 'Otro · exige descripción', '+ Nuevo motivo']);
    fireEvent.click(within(group).getByRole('button', { name: '+ Nuevo motivo' }));
    fireEvent.change(screen.getByLabelText(/Nombre del motivo/), {
      target: { value: 'Cumpleaños' },
    });
    fireEvent.click(screen.getByRole('button', { name: 'Exige descripción' }));
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'Crear motivo' }));
    });
    expect(execute).toHaveBeenCalledWith('payments_create_discount_reason', {
      body: {
        name: 'Cumpleaños',
        requires_description: true,
        active: true,
        position: { after: reasons[0]!.id },
      },
    });
  });

  it('desactiva un motivo con su revisión y sin permiso no ofrece cambios', async () => {
    const execute = vi.fn(async () => ({}) as never);
    const { unmount } = render(
      <DiscountReasonsAdmin reasons={reasons} can={() => true} execute={execute} busy={false} />,
    );
    fireEvent.click(screen.getByRole('button', { name: 'Atención' }));
    fireEvent.click(screen.getByRole('button', { name: 'Motivo activo' }));
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'Guardar motivo' }));
    });
    expect(execute).toHaveBeenCalledWith('payments_save_discount_reason', {
      id: reasons[1]!.id,
      body: { name: 'Atención', requires_description: false, active: false, revision: 1 },
    });
    unmount();
    render(
      <DiscountReasonsAdmin reasons={reasons} can={() => false} execute={execute} busy={false} />,
    );
    expect(screen.queryByRole('button', { name: '+ Nuevo motivo' })).toBeNull();
    expect((screen.getByRole('button', { name: 'Atención' }) as HTMLButtonElement).disabled).toBe(
      true,
    );
    expect(screen.getByText(/Modificar los motivos requiere el permiso/)).toBeTruthy();
  });
});
