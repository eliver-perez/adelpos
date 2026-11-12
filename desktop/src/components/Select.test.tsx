// @vitest-environment jsdom

import { useState } from 'react';
import { act, cleanup, fireEvent, render, screen, within } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { Accordion } from './Accordion';
import { Field } from './Forms';
import { MultiSelect, Select } from './Select';
import type { SelectAction, SelectItem } from './Select';
import { VirtualKeyboard } from './VirtualKeyboard';
import { normalizeSearchText } from './selectModel';
import { chooseOption, openSelect, optionLabels, selectText, selectValue } from '../testing/select';

afterEach(cleanup);

const fruits: SelectItem[] = [
  { value: 'apple', label: 'Manzana' },
  { value: 'pear', label: 'Pera' },
  { value: 'lemon', label: 'Limón', description: 'Ácido' },
  { value: 'mango', label: 'Mango', disabled: true },
  { value: 'melon', label: 'Melón' },
];

const categories: SelectItem[] = [
  'Bebidas',
  'Postres',
  'Café de olla',
  'Cócteles',
  'Desayunos',
  'Entradas',
  'Ensaladas',
  'Sopas',
  'Tacos',
  'Tortas',
].map((name, index) => ({ value: `c${index}`, label: name }));

function Single({
  options = fruits,
  initial = '',
  required = false,
  actions,
  hint,
  onChange,
}: {
  options?: SelectItem[];
  initial?: string;
  required?: boolean;
  actions?: SelectAction[];
  hint?: string;
  onChange?: (value: string) => void;
}) {
  const [value, setValue] = useState(initial);
  return (
    <>
      <Field label="Fruta" hint={hint}>
        <Select
          value={value}
          options={options}
          required={required}
          actions={actions}
          placeholder="Elige una fruta"
          onChange={(next) => {
            setValue(next);
            onChange?.(next);
          }}
        />
      </Field>
      <output aria-label="Elegida">{value}</output>
      <button type="button">Después</button>
    </>
  );
}

function combobox(name = 'Fruta') {
  return screen.getByRole('combobox', { name });
}

describe('selector único (etapa 9C)', () => {
  it('toma su nombre de la etiqueta, describe la indicación y anuncia su estado', () => {
    render(<Single hint="Solo frutas de temporada" />);
    const trigger = combobox();
    expect(trigger.tagName).toBe('BUTTON');
    expect(trigger.getAttribute('aria-expanded')).toBe('false');
    expect(trigger.getAttribute('aria-haspopup')).toBe('listbox');
    expect(trigger.getAttribute('aria-describedby')).toBeTruthy();
    expect(document.getElementById(trigger.getAttribute('aria-describedby')!)?.textContent).toBe(
      'Solo frutas de temporada',
    );
    expect(screen.getByLabelText('Fruta')).toBe(trigger);
    expect(selectText(trigger)).toBe('Elige una fruta');

    fireEvent.click(trigger);
    expect(trigger.getAttribute('aria-expanded')).toBe('true');
    const listbox = screen.getByRole('listbox', { name: 'Fruta' });
    expect(trigger.getAttribute('aria-controls')).toBe(listbox.id);
    expect(within(listbox).getAllByRole('option')).toHaveLength(5);
    // Cinco opciones: sin búsqueda.
    expect(screen.queryByRole('searchbox')).toBeNull();
    const active = trigger.getAttribute('aria-activedescendant');
    expect(document.getElementById(active!)?.textContent).toBe('Manzana');
    expect(
      within(listbox).getByRole('option', { name: 'Mango' }).getAttribute('aria-disabled'),
    ).toBe('true');
  });

  it('la opción con valor vacío («Todos») se muestra como elegida; sin ella, el texto vacío', () => {
    const filter: SelectItem[] = [
      { value: '', label: 'Todos' },
      { value: 'open', label: 'Abiertos' },
    ];
    render(<Single options={filter} />);
    expect(selectText(combobox())).toBe('Todos');
    expect(combobox().querySelector('.select-placeholder')).toBeNull();
    const listbox = openSelect(combobox());
    expect(
      within(listbox).getByRole('option', { name: 'Todos' }).getAttribute('aria-selected'),
    ).toBe('true');
    cleanup();
    render(<Single />);
    expect(selectText(combobox())).toBe('Elige una fruta');
    expect(combobox().querySelector('.select-placeholder')).toBeTruthy();
  });

  it('elige con el ratón, marca la opción elegida y no avisa si se repite la misma', () => {
    const changed = vi.fn();
    render(<Single onChange={changed} />);
    chooseOption(combobox(), 'Pera');
    expect(combobox().getAttribute('aria-expanded')).toBe('false');
    expect(selectValue(combobox())).toBe('pear');
    expect(selectText(combobox())).toBe('Pera');
    expect(document.activeElement).toBe(combobox());
    const listbox = openSelect(combobox());
    expect(
      within(listbox).getByRole('option', { name: 'Pera' }).getAttribute('aria-selected'),
    ).toBe('true');
    fireEvent.click(within(listbox).getByRole('option', { name: 'Pera' }));
    expect(changed).toHaveBeenCalledTimes(1);
    // Una opción deshabilitada no se elige.
    chooseOption(combobox(), 'Mango');
    expect(selectValue(combobox())).toBe('pear');
  });

  it('se usa por completo con el teclado', () => {
    render(<Single initial="pear" />);
    const trigger = combobox();
    trigger.focus();
    fireEvent.keyDown(trigger, { key: 'ArrowDown' });
    expect(trigger.getAttribute('aria-expanded')).toBe('true');
    const activeText = () =>
      document.getElementById(trigger.getAttribute('aria-activedescendant')!)?.textContent;
    // Abre sobre la opción elegida.
    expect(activeText()).toBe('Pera');
    fireEvent.keyDown(trigger, { key: 'ArrowDown' });
    expect(activeText()).toBe('LimónÁcido');
    // Salta la deshabilitada.
    fireEvent.keyDown(trigger, { key: 'ArrowDown' });
    expect(activeText()).toBe('Melón');
    fireEvent.keyDown(trigger, { key: 'Home' });
    expect(activeText()).toBe('Manzana');
    fireEvent.keyDown(trigger, { key: 'End' });
    expect(activeText()).toBe('Melón');
    fireEvent.keyDown(trigger, { key: 'PageUp' });
    expect(activeText()).toBe('Manzana');
    fireEvent.keyDown(trigger, { key: 'PageDown' });
    expect(activeText()).toBe('Melón');
    // Escribir busca la opción que empieza con lo escrito.
    fireEvent.keyDown(trigger, { key: 'p' });
    expect(activeText()).toBe('Pera');
    fireEvent.keyDown(trigger, { key: 'Enter' });
    expect(trigger.getAttribute('aria-expanded')).toBe('false');
    expect(selectValue(trigger)).toBe('pear');
    // Esc cierra sin cambiar y no llega a quien contiene al selector.
    fireEvent.keyDown(trigger, { key: 'End' });
    expect(activeText()).toBe('Melón');
    const escape = new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true });
    act(() => {
      trigger.dispatchEvent(escape);
    });
    expect(escape.defaultPrevented).toBe(true);
    expect(trigger.getAttribute('aria-expanded')).toBe('false');
    expect(selectValue(trigger)).toBe('pear');
    // Espacio abre y elige; Tab cierra sin cambiar.
    fireEvent.keyDown(trigger, { key: ' ' });
    fireEvent.keyDown(trigger, { key: 'ArrowUp' });
    fireEvent.keyDown(trigger, { key: ' ' });
    expect(selectValue(trigger)).toBe('apple');
    fireEvent.keyDown(trigger, { key: 'ArrowDown' });
    fireEvent.keyDown(trigger, { key: 'ArrowDown' });
    fireEvent.keyDown(trigger, { key: 'Tab' });
    expect(trigger.getAttribute('aria-expanded')).toBe('false');
    expect(selectValue(trigger)).toBe('apple');
  });

  it('con más de 8 opciones busca sin distinguir mayúsculas ni acentos y anuncia los resultados', () => {
    render(<Single options={categories} />);
    const trigger = combobox();
    trigger.focus();
    // Escribir sobre el disparador abre la lista y escribe en la búsqueda.
    fireEvent.keyDown(trigger, { key: 'c' });
    const search = screen.getByRole('searchbox', { name: 'Buscar fruta' });
    expect(document.activeElement).toBe(search);
    expect((search as HTMLInputElement).value).toBe('c');
    fireEvent.change(search, { target: { value: 'COCTELES' } });
    const listbox = screen.getByRole('listbox');
    expect(
      within(listbox)
        .getAllByRole('option')
        .map((option) => option.textContent),
    ).toEqual(['Cócteles']);
    expect(screen.getByRole('status', { name: '' }).textContent).toBe('1 resultado');
    expect(search.getAttribute('aria-controls')).toBe(listbox.id);
    expect(
      document.getElementById(search.getAttribute('aria-activedescendant')!)?.textContent,
    ).toBe('Cócteles');
    fireEvent.change(search, { target: { value: 'cafe olla' } });
    expect(
      within(listbox)
        .getAllByRole('option')
        .map((option) => option.textContent),
    ).toEqual(['Café de olla']);
    fireEvent.change(search, { target: { value: 'pizza' } });
    expect(within(listbox).queryAllByRole('option')).toHaveLength(0);
    expect(screen.getByText('Sin resultados', { selector: 'p' })).toBeTruthy();
    fireEvent.change(search, { target: { value: 'ta' } });
    expect(
      within(listbox)
        .getAllByRole('option')
        .map((option) => option.textContent),
    ).toEqual(['Tacos', 'Tortas']);
    fireEvent.keyDown(search, { key: 'ArrowDown' });
    fireEvent.keyDown(search, { key: 'Enter' });
    expect(selectText(trigger)).toBe('Tortas');
    expect(document.activeElement).toBe(trigger);
    // Al volver a abrir, la búsqueda empieza vacía.
    fireEvent.keyDown(trigger, { key: 'Enter' });
    expect((screen.getByRole('searchbox') as HTMLInputElement).value).toBe('');
    fireEvent.keyDown(screen.getByRole('searchbox'), { key: 'Escape' });
    expect(trigger.getAttribute('aria-expanded')).toBe('false');
    expect(document.activeElement).toBe(trigger);
    expect(normalizeSearchText('Ñandú Ávila_Sur')).toBe('nandu avila sur');
  });

  it('muestra grupos con encabezado y los filtra', () => {
    const grouped: SelectItem[] = [
      {
        label: 'Peso',
        options: [
          { value: 'kg', label: 'Kilogramo (kg)' },
          { value: 'g', label: 'Gramo (g)' },
        ],
      },
      { label: 'Volumen', options: [{ value: 'l', label: 'Litro (l)' }] },
    ];
    render(<Single options={grouped} />);
    const listbox = openSelect(combobox());
    const groups = within(listbox).getAllByRole('group');
    expect(groups.map((group) => group.getAttribute('aria-labelledby'))).toHaveLength(2);
    expect(within(listbox).getByRole('group', { name: 'Volumen' })).toBeTruthy();
    expect(
      within(within(listbox).getByRole('group', { name: 'Peso' })).getAllByRole('option'),
    ).toHaveLength(2);
    fireEvent.click(within(listbox).getByRole('option', { name: 'Litro (l)' }));
    expect(selectValue(combobox())).toBe('l');
  });

  it('ejecuta las opciones de acción sin cambiar el valor, también al buscar', () => {
    const create = vi.fn();
    render(
      <Single
        options={categories}
        initial="c1"
        actions={[{ label: '+ Nueva categoría', onSelect: create }]}
      />,
    );
    const trigger = combobox();
    const listbox = openSelect(trigger);
    fireEvent.change(screen.getByRole('searchbox'), { target: { value: 'zzz' } });
    const action = within(listbox).getByRole('option', { name: '+ Nueva categoría' });
    expect(action.getAttribute('aria-selected')).toBe('false');
    fireEvent.keyDown(screen.getByRole('searchbox'), { key: 'Enter' });
    expect(create).toHaveBeenCalledTimes(1);
    expect(selectValue(trigger)).toBe('c1');
    expect(trigger.getAttribute('aria-expanded')).toBe('false');
    expect(optionLabels(trigger)).toContain('+ Nueva categoría');
    expect(optionLabels(trigger, { actions: false })).not.toContain('+ Nueva categoría');
  });

  it('participa en la validación del formulario, revela el acordeón y enfoca el disparador', () => {
    const submitted = vi.fn();
    render(
      <form
        onSubmit={(event) => {
          event.preventDefault();
          submitted();
        }}
      >
        <Accordion title="Datos">
          <Field label="Proveedor">
            <Select
              required
              name="supplier"
              value=""
              options={fruits}
              placeholder="Elige un proveedor"
              onChange={() => undefined}
            />
          </Field>
        </Accordion>
        <button type="submit">Guardar</button>
      </form>,
    );
    const hidden = document.querySelector<HTMLInputElement>('input[name="supplier"]')!;
    expect(hidden.required).toBe(true);
    expect(hidden.getAttribute('aria-hidden')).toBe('true');
    expect(hidden.getAttribute('data-virtual-keyboard')).toBe('off');
    expect(hidden.type).toBe('text');
    expect(screen.getByRole('button', { name: 'Datos' }).getAttribute('aria-expanded')).toBe(
      'false',
    );
    // La validación nativa (la misma que corre al enviar) encuentra el campo vacío.
    let valid = true;
    act(() => {
      valid = hidden.checkValidity();
    });
    expect(valid).toBe(false);
    expect(screen.getByRole('button', { name: 'Datos' }).getAttribute('aria-expanded')).toBe(
      'true',
    );
    expect(document.activeElement).toBe(screen.getByRole('combobox', { name: 'Proveedor' }));
    expect(submitted).not.toHaveBeenCalled();
  });

  it('no se abre deshabilitado (también dentro de un fieldset deshabilitado)', () => {
    render(
      <fieldset disabled>
        <Field label="Fruta">
          <Select value="apple" options={fruits} onChange={() => undefined} />
        </Field>
      </fieldset>,
    );
    fireEvent.click(combobox());
    expect(combobox().getAttribute('aria-expanded')).toBe('false');
    expect((combobox() as HTMLButtonElement).disabled).toBe(false);
    expect(combobox().matches(':disabled')).toBe(true);
  });

  it('selección múltiple: alterna opciones, queda abierta y muestra etiquetas para quitar', () => {
    function Multiple() {
      const [value, setValue] = useState<string[]>(['c1']);
      return (
        <Field label="Categorías participantes">
          <MultiSelect value={value} options={categories} onChange={setValue} />
        </Field>
      );
    }
    render(<Multiple />);
    const trigger = screen.getByRole('combobox', { name: 'Categorías participantes' });
    expect(selectText(trigger)).toBe('Postres');
    const listbox = openSelect(trigger);
    expect(listbox.getAttribute('aria-multiselectable')).toBe('true');
    fireEvent.click(within(listbox).getByRole('option', { name: 'Tacos' }));
    expect(trigger.getAttribute('aria-expanded')).toBe('true');
    const search = screen.getByRole('searchbox');
    fireEvent.change(search, { target: { value: 'sopa' } });
    fireEvent.keyDown(search, { key: 'Enter' });
    expect(selectValue(trigger)).toBe('c1,c8,c7');
    expect(selectText(trigger)).toBe('3 seleccionados');
    expect(
      within(listbox).getByRole('option', { name: 'Sopas' }).getAttribute('aria-selected'),
    ).toBe('true');
    fireEvent.click(screen.getByRole('button', { name: 'Listo' }));
    expect(trigger.getAttribute('aria-expanded')).toBe('false');
    const chips = screen.getByRole('list', { name: 'Elegidos' });
    expect(
      within(chips)
        .getAllByRole('listitem')
        .map((chip) => chip.textContent),
    ).toEqual(['Postres', 'Tacos', 'Sopas']);
    fireEvent.click(screen.getByRole('button', { name: 'Quitar «Tacos»' }));
    expect(selectValue(trigger)).toBe('c1,c7');
    // Volver a elegir una opción la quita.
    chooseOption(trigger, 'Postres');
    expect(selectValue(trigger)).toBe('c7');
  });

  it('búsqueda en el servidor: avisa el texto, no filtra y no elige con Intro mientras busca', () => {
    function Remote() {
      const [query, setQuery] = useState('');
      const [value, setValue] = useState('');
      const options = [
        { value: 'h', label: 'Harina · HAR-01' },
        { value: 't', label: 'Tomate · TOM-01' },
      ];
      return (
        <Field label="Insumo">
          <Select
            value={value}
            options={options}
            loading={query === 'BAR-123'}
            onSearchChange={setQuery}
            onChange={setValue}
          />
        </Field>
      );
    }
    render(<Remote />);
    const trigger = screen.getByRole('combobox', { name: 'Insumo' });
    const listbox = openSelect(trigger);
    const search = screen.getByRole('searchbox', { name: 'Buscar insumo' });
    fireEvent.change(search, { target: { value: 'BAR-123' } });
    // El código de barras no está en el texto de las opciones: las filtra Go, no React.
    expect(within(listbox).getAllByRole('option')).toHaveLength(2);
    expect(screen.getByText('Buscando…', { selector: 'p' })).toBeTruthy();
    fireEvent.keyDown(search, { key: 'Enter' });
    expect(selectValue(trigger)).toBe('');
    fireEvent.change(search, { target: { value: 'tom' } });
    fireEvent.keyDown(search, { key: 'ArrowDown' });
    fireEvent.keyDown(search, { key: 'Enter' });
    expect(selectValue(trigger)).toBe('t');
  });

  it('funciona con el teclado virtual: escribe en la búsqueda y no se cierra al tocarlo', () => {
    render(
      <>
        <Single options={categories} />
        <VirtualKeyboard mode="always" />
      </>,
    );
    const trigger = combobox();
    fireEvent.keyDown(trigger, { key: 'Enter' });
    const search = screen.getByRole('searchbox', { name: 'Buscar fruta' }) as HTMLInputElement;
    expect(document.activeElement).toBe(search);
    fireEvent.focusIn(search);
    const keyboard = screen.getByRole('group', { name: 'Teclado virtual alfanumérico' });
    expect(within(keyboard).getByText('Buscar fruta')).toBeTruthy();
    for (const letter of ['s', 'o', 'p']) {
      const key = within(keyboard).getByRole('button', { name: `Escribir ${letter}` });
      fireEvent.pointerDown(key);
      fireEvent.click(key);
    }
    expect(search.value).toBe('sop');
    expect(trigger.getAttribute('aria-expanded')).toBe('true');
    expect(
      within(screen.getByRole('listbox'))
        .getAllByRole('option')
        .map((o) => o.textContent),
    ).toEqual(['Sopas']);
    fireEvent.pointerDown(within(keyboard).getByRole('button', { name: 'Listo' }));
    expect(trigger.getAttribute('aria-expanded')).toBe('true');
    // Tocar fuera sí la cierra.
    fireEvent.pointerDown(screen.getByRole('button', { name: 'Después' }));
    expect(trigger.getAttribute('aria-expanded')).toBe('false');
  });

  it('al abrirse con el dedo no enfoca la búsqueda (no aparece el teclado virtual)', () => {
    render(<Single options={categories} />);
    const trigger = combobox();
    fireEvent.pointerDown(trigger, { pointerType: 'touch' });
    fireEvent.click(trigger, { detail: 1 });
    expect(trigger.getAttribute('aria-expanded')).toBe('true');
    expect(document.activeElement).not.toBe(screen.getByRole('searchbox'));
    // Una tecla física sí lleva a la búsqueda.
    trigger.focus();
    fireEvent.keyDown(trigger, { key: 't' });
    expect(document.activeElement).toBe(screen.getByRole('searchbox'));
    expect((screen.getByRole('searchbox') as HTMLInputElement).value).toBe('t');
  });

  it('nunca usa campos numéricos ni <select> nativos', () => {
    render(<Single options={categories} required />);
    openSelect(combobox());
    expect(document.querySelector('input[type="number"]')).toBeNull();
    expect(document.querySelector('select')).toBeNull();
  });

  it('se coloca dentro de la ventana y sobre el teclado virtual', () => {
    render(<Single options={categories} />);
    const trigger = combobox();
    trigger.getBoundingClientRect = () =>
      ({
        top: 600,
        bottom: 644,
        left: 1200,
        right: 1400,
        width: 200,
        height: 44,
        x: 1200,
        y: 600,
      }) as DOMRect;
    Object.defineProperty(window, 'innerHeight', { configurable: true, value: 720 });
    Object.defineProperty(document.documentElement, 'clientWidth', {
      configurable: true,
      value: 1280,
    });
    openSelect(trigger);
    const popup = document.querySelector<HTMLElement>('.select-popup')!;
    // Abajo no caben 260 px: se abre hacia arriba, sin salirse por la derecha.
    expect(popup.style.bottom).toBe('124px');
    expect(popup.style.top).toBe('');
    expect(
      Number.parseFloat(popup.style.left) + Number.parseFloat(popup.style.width),
    ).toBeLessThanOrEqual(1268);
  });
});
