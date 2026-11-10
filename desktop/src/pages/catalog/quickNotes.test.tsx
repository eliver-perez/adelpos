// @vitest-environment jsdom

import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { Category, CategoryQuickNote } from '../../services/catalog';
import { CategoryQuickNotes, quickNoteProblem } from './CategoryQuickNotes';

const native = vi.hoisted(() => ({ invoke: vi.fn() }));
vi.mock('@tauri-apps/api/core', () => ({ invoke: native.invoke, isTauri: () => true }));

const category = {
  id: '00000000-0000-4000-8000-0000000000d1',
  name: 'Hamburguesas',
  description: '',
  active: true,
  revision: 3,
  sort_order: 10,
  color: '',
  image_id: null,
  availability: { days: [], windows: [] },
} as Category;

function note(id: string, text: string, order: number, active = true): CategoryQuickNote {
  return {
    id: `00000000-0000-4000-8000-0000000002${id}`,
    category_id: category.id,
    text,
    active,
    sort_order: order,
    revision: 2,
    updated_at: '2026-11-09T12:00:00Z',
  };
}

const notes = [
  note('02', 'Bien cocida', 20),
  note('01', 'Sin cebolla', 10),
  note('03', 'Sin sal', 30, false),
  // De otra categoría: no se muestra.
  { ...note('04', 'Sin hielo', 10), category_id: '00000000-0000-4000-8000-0000000000d2' },
];

function requests(operation: string) {
  return native.invoke.mock.calls.filter(
    ([command, args]) => command === 'api_request' && args?.operation === operation,
  );
}

beforeEach(() => {
  native.invoke.mockImplementation(async (_command: string, args: { body?: unknown }) => ({
    status: 200,
    body: args.body ?? {},
  }));
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe('indicaciones rápidas de una categoría', () => {
  it('rechaza comas, vacías y más de 80 caracteres con la misma regla que Go', () => {
    expect(quickNoteProblem('Sin cebolla')).toBe('');
    expect(quickNoteProblem('Sin cebolla, sin sal')).toMatch(/Sin comas/);
    expect(quickNoteProblem('   ')).toMatch(/Escribe/);
    expect(quickNoteProblem('a'.repeat(81))).toMatch(/80/);
  });

  it('lista las de su categoría en orden y agrega una nueva al final con su posición', async () => {
    const changed = vi.fn(async () => undefined);
    render(<CategoryQuickNotes category={category} notes={notes} editable onChanged={changed} />);
    const group = screen.getByRole('group', { name: 'Indicaciones de Hamburguesas' });
    expect(
      within(group)
        .getAllByRole('button')
        .map((button) => button.textContent),
    ).toEqual(['Sin cebolla', 'Bien cocida', 'Sin sal · inactiva']);
    expect(screen.getByText('2 activas de 3')).toBeTruthy();
    const field = screen.getByRole('textbox', { name: /Nueva indicación/ });
    const add = screen.getByRole('button', { name: 'Agregar indicación' }) as HTMLButtonElement;
    fireEvent.change(field, { target: { value: 'Sin pepinillos, sin mostaza' } });
    expect(add.disabled).toBe(true);
    expect(screen.getByText(/Sin comas/)).toBeTruthy();
    fireEvent.change(field, { target: { value: ' Término medio ' } });
    expect(add.disabled).toBe(false);
    await act(async () => {
      fireEvent.click(add);
    });
    await waitFor(() => expect(requests('catalog_create_quick_note')).toHaveLength(1));
    expect(requests('catalog_create_quick_note')[0]![1].body).toEqual({
      category_id: category.id,
      text: 'Término medio',
      active: true,
      position: { after: notes[2]!.id },
      revision: 0,
    });
    expect(changed).toHaveBeenCalled();
    const notice = await screen.findByRole('status', { name: 'Operación completada' });
    expect(notice.textContent).toContain('Indicación «Término medio» agregada a Hamburguesas.');
  });

  it('edita una indicación con su revisión y sin permiso solo la muestra', async () => {
    const changed = vi.fn(async () => undefined);
    const { unmount } = render(
      <CategoryQuickNotes category={category} notes={notes} editable onChanged={changed} />,
    );
    fireEvent.click(screen.getByRole('button', { name: 'Bien cocida' }));
    const editor = screen.getByRole('form', { name: 'Editar indicación Bien cocida' });
    fireEvent.click(within(editor).getByRole('button', { name: 'Indicación activa' }));
    await act(async () => {
      fireEvent.click(within(editor).getByRole('button', { name: 'Guardar indicación' }));
    });
    await waitFor(() => expect(requests('catalog_save_quick_note')).toHaveLength(1));
    expect(requests('catalog_save_quick_note')[0]![1]).toMatchObject({
      resourceId: notes[0]!.id,
      body: { text: 'Bien cocida', active: false, revision: 2 },
    });
    unmount();
    render(
      <CategoryQuickNotes category={category} notes={notes} editable={false} onChanged={changed} />,
    );
    expect(screen.queryByRole('button', { name: 'Agregar indicación' })).toBeNull();
    expect(
      (screen.getByRole('button', { name: 'Sin cebolla' }) as HTMLButtonElement).disabled,
    ).toBe(true);
  });
});
