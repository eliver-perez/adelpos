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
    // Sin elegir efecto, la indicación es solo instrucción, como en la etapa 7.1.
    expect(requests('catalog_create_quick_note')[0]![1].body).toEqual({
      category_id: category.id,
      text: 'Término medio',
      active: true,
      effect: 'none',
      ingredient_id: null,
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

  it('configura el efecto con botones y el ingrediente vinculado, sin campos de gramos', async () => {
    const changed = vi.fn(async () => undefined);
    const ingredients = [
      {
        id: '00000000-0000-4000-8000-0000000003a1',
        name: 'Cebolla',
        description: '',
        active: true,
        revision: 1,
        unit: 'porción',
      },
      {
        id: '00000000-0000-4000-8000-0000000003a2',
        name: 'Queso',
        description: '',
        active: true,
        revision: 1,
        unit: 'porción',
      },
    ];
    const withEffect = [
      ...notes,
      {
        ...note('05', 'Queso extra', 40),
        effect: 'extra' as const,
        ingredient_id: ingredients[1]!.id,
        ingredient_name: 'Queso',
      },
    ];
    render(
      <CategoryQuickNotes
        category={category}
        notes={withEffect}
        ingredients={ingredients}
        editable
        onChanged={changed}
      />,
    );
    // El listado resume el efecto de las indicaciones que lo tienen.
    const group = screen.getByRole('group', { name: 'Indicaciones de Hamburguesas' });
    expect(within(group).getByRole('button', { name: /Queso extra/ }).textContent).toBe(
      'Queso extra · Porción extra: Queso',
    );
    const form = screen.getByRole('form', { name: 'Nueva indicación rápida' });
    const effects = within(form).getByRole('group', { name: 'Tipo de efecto' });
    expect(
      within(effects)
        .getAllByRole('button')
        .map((button) => button.textContent),
    ).toEqual(['Solo instrucción', 'Omitir', 'Porción habitual', 'Porción extra']);
    expect(
      within(effects)
        .getByRole('button', { name: 'Solo instrucción' })
        .getAttribute('aria-pressed'),
    ).toBe('true');
    // Sin efecto no hay selector de ingrediente; ningún campo pide gramos.
    expect(within(form).queryByRole('combobox', { name: /Ingrediente vinculado/ })).toBeNull();
    expect(within(form).queryByText(/gramos/i)).toBeNull();
    fireEvent.change(within(form).getByRole('textbox', { name: /Nueva indicación/ }), {
      target: { value: 'Sin cebolla morada' },
    });
    fireEvent.click(within(effects).getByRole('button', { name: 'Omitir' }));
    const add = within(form).getByRole('button', {
      name: 'Agregar indicación',
    }) as HTMLButtonElement;
    // Un efecto exige su ingrediente.
    expect(add.disabled).toBe(true);
    const select = within(form).getByRole('combobox', { name: /Ingrediente vinculado/ });
    fireEvent.change(select, { target: { value: ingredients[0]!.id } });
    expect(add.disabled).toBe(false);
    await act(async () => {
      fireEvent.click(add);
    });
    await waitFor(() => expect(requests('catalog_create_quick_note')).toHaveLength(1));
    expect(requests('catalog_create_quick_note')[0]![1].body).toMatchObject({
      text: 'Sin cebolla morada',
      effect: 'omit',
      ingredient_id: ingredients[0]!.id,
      revision: 0,
    });
    // Editar una indicación con efecto conserva su ingrediente y permite volver a solo instrucción.
    fireEvent.click(within(group).getByRole('button', { name: /Queso extra/ }));
    const editor = screen.getByRole('form', { name: 'Editar indicación Queso extra' });
    const editorEffects = within(editor).getByRole('group', { name: 'Tipo de efecto' });
    expect(
      within(editorEffects)
        .getByRole('button', { name: 'Porción extra' })
        .getAttribute('aria-pressed'),
    ).toBe('true');
    expect(
      (within(editor).getByRole('combobox', { name: /Ingrediente vinculado/ }) as HTMLSelectElement)
        .value,
    ).toBe(ingredients[1]!.id);
    fireEvent.click(within(editorEffects).getByRole('button', { name: 'Solo instrucción' }));
    await act(async () => {
      fireEvent.click(within(editor).getByRole('button', { name: 'Guardar indicación' }));
    });
    await waitFor(() => expect(requests('catalog_save_quick_note')).toHaveLength(1));
    expect(requests('catalog_save_quick_note')[0]![1].body).toMatchObject({
      text: 'Queso extra',
      effect: 'none',
      ingredient_id: null,
      revision: 2,
    });
  });
});
