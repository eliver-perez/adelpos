// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { useState } from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { originalCustomization } from '../../../components/catalog/customizationRules';
import type {
  CategoryQuickNote,
  Customization,
  Lookups,
  ProductBreakdown,
} from '../../../services/catalog';
import { describeBreakdown } from './captureRules';
import { quickNoteQuantity } from './quickNoteRules';
import { TouchCustomizer } from './TouchCustomizer';

const native = vi.hoisted(() => ({ invoke: vi.fn() }));
vi.mock('@tauri-apps/api/core', () => ({ invoke: native.invoke, isTauri: () => true }));

// Etapa 8, fase B: indicaciones rápidas con efecto y tarjetas de complementos en la captura. Solo
// se sustituye el transporte nativo; la configuración y el desglose son los que «devuelve Go».

const ids = {
  dishes: '00000000-0000-4000-8000-0000000004d1',
  burger: '00000000-0000-4000-8000-0000000004e1',
  salad: '00000000-0000-4000-8000-0000000004e2',
  onion: '00000000-0000-4000-8000-0000000004a1',
  cheese: '00000000-0000-4000-8000-0000000004a2',
  sides: '00000000-0000-4000-8000-0000000004f1',
  rice: '00000000-0000-4000-8000-0000000004f2',
  greens: '00000000-0000-4000-8000-0000000004f3',
  fries: '00000000-0000-4000-8000-0000000004f4',
  pasta: '00000000-0000-4000-8000-0000000004f5',
};

const schedule = { days: [], windows: [] };

function note(
  suffix: string,
  text: string,
  order: number,
  effect: CategoryQuickNote['effect'],
  ingredient: string | null,
): CategoryQuickNote {
  return {
    id: `00000000-0000-4000-8000-0000000005${suffix}`,
    category_id: ids.dishes,
    text,
    effect,
    ingredient_id: ingredient,
    ingredient_name: ingredient === ids.onion ? 'Cebolla' : ingredient ? 'Queso' : null,
    active: true,
    sort_order: order,
    revision: 1,
    updated_at: '2026-11-10T12:00:00Z',
  };
}

const notes = {
  without: note('01', 'Sin cebolla', 10, 'omit', ids.onion),
  with: note('02', 'Con cebolla', 20, 'normal', ids.onion),
  extra: note('03', 'Cebolla extra', 30, 'extra', ids.onion),
  cooked: note('04', 'Bien cocida', 40, 'none', null),
  cheese: note('05', 'Queso extra', 50, 'extra', ids.cheese),
};

function product(id: string, name: string, withIngredients: boolean) {
  return {
    id,
    name,
    description: '',
    active: true,
    revision: 1,
    category_id: ids.dishes,
    code: name.toUpperCase(),
    barcode: null,
    kind: 'product',
    base_price: '140.00',
    tax_rate: null,
    image_id: null,
    sort_order: 10,
    archived: false,
    temporary_unavailable: false,
    availability: schedule,
    variants: [],
    // Cebolla incluida (removible y agregable hasta 3) y queso opcional (agregable hasta 2).
    ingredients: withIngredients
      ? [
          {
            ingredient_id: ids.onion,
            included_quantity: '1',
            removable: true,
            addable: true,
            max_quantity: '3',
            extra_price: '5.00',
            recipe_quantity: '0',
            name: 'Cebolla',
          },
          {
            ingredient_id: ids.cheese,
            included_quantity: '0',
            removable: false,
            addable: true,
            max_quantity: '2',
            extra_price: '12.50',
            recipe_quantity: '0',
            name: 'Queso',
          },
        ]
      : [],
    // Ejemplo del propietario: arroz y ensalada incluidos; hasta 3 porciones.
    groups: withIngredients
      ? [
          {
            group_id: ids.sides,
            min_quantity: null,
            max_quantity: '3',
            included: [
              { option_id: ids.rice, quantity: '1' },
              { option_id: ids.greens, quantity: '1' },
            ],
          },
        ]
      : [],
    slots: [],
  };
}

function option(id: string, name: string, value: string) {
  return {
    id,
    complement_id: `c-${id}`,
    portion_id: null,
    reference_value: value,
    extra_price: '0.00',
    max_quantity: null,
    active: true,
    tiers: [],
    name,
  };
}

const sides = {
  id: ids.sides,
  name: 'Guarniciones',
  description: '',
  active: true,
  revision: 1,
  pricing_mode: 'value',
  min_quantity: '0',
  max_quantity: null,
  options: [
    option(ids.rice, 'Arroz', '30.00'),
    option(ids.greens, 'Ensalada', '25.00'),
    option(ids.fries, 'Papas', '35.00'),
    option(ids.pasta, 'Pasta', '20.00'),
  ],
};

const lookups = {
  categories: [],
  ingredients: [],
  complements: [],
  portions: [],
  groups: [sides],
  'price-lists': [],
  palette: [],
  products: [
    {
      id: ids.burger,
      name: 'Hamburguesa',
      code: 'H',
      kind: 'product',
      active: true,
      archived: false,
      category_id: ids.dishes,
      sort_order: 10,
      variants: [],
      description: '',
      revision: 1,
    },
    {
      id: ids.salad,
      name: 'Ensalada verde',
      code: 'E',
      kind: 'product',
      active: true,
      archived: false,
      category_id: ids.dishes,
      sort_order: 20,
      variants: [],
      description: '',
      revision: 1,
    },
  ],
  category_quick_notes: Object.values(notes),
} as unknown as Lookups;

function groupBreakdown(difference: string, extras: string): ProductBreakdown {
  return {
    product_id: ids.burger,
    revision: 1,
    name: 'Hamburguesa',
    base_price: '140.00',
    price_origin: 'base',
    unit_price: '140.00',
    ingredient_extras: '0.00',
    group_extras: '0.00',
    component_extras: '0.00',
    notes: '',
    ingredients: [],
    groups: [
      {
        group_id: ids.sides,
        name: 'Guarniciones',
        mode: 'value',
        original_value: '55.00',
        requested_value: '55.00',
        difference,
        extras,
        options: [],
      },
    ],
    components: [],
  };
}

let latest: Customization = originalCustomization();

function Harness({
  productID,
  breakdown = null,
}: {
  productID: string;
  breakdown?: ProductBreakdown | null;
}) {
  const [value, setValue] = useState<Customization>(originalCustomization());
  latest = value;
  return (
    <TouchCustomizer
      productID={productID}
      variantID={null}
      onVariantChange={() => undefined}
      value={value}
      onChange={setValue}
      lookups={lookups}
      breakdown={breakdown}
      noteShortcuts={['Sin cebolla', 'Salsa aparte']}
    />
  );
}

beforeEach(() => {
  latest = originalCustomization();
  native.invoke.mockImplementation(
    async (command: string, args: { operation: string; resourceId: string | null }) => {
      if (command !== 'api_request' || args.operation !== 'catalog_configuration') {
        return { status: 200, body: {} };
      }
      const burger = args.resourceId === ids.burger;
      return {
        status: 200,
        body: {
          product: product(
            args.resourceId ?? '',
            burger ? 'Hamburguesa' : 'Ensalada verde',
            burger,
          ),
          category: { id: ids.dishes },
          groups: burger ? [sides] : [],
          timezone: 'America/Monterrey',
          availability_reason: '',
        },
      };
    },
  );
});

afterEach(cleanup);

function pressed(element: HTMLElement) {
  return element.getAttribute('aria-pressed');
}

async function notesBlock() {
  return screen.findByRole('region', { name: 'Indicaciones para cocina' });
}

function names(region: HTMLElement) {
  return within(region)
    .getAllByRole('button')
    .map((button) => button.textContent);
}

describe('reglas de las indicaciones con efecto', () => {
  it('fija las mismas cantidades que los botones del personalizador y que Go', () => {
    const onion = product(ids.burger, 'Hamburguesa', true).ingredients[0]!;
    const cheese = product(ids.burger, 'Hamburguesa', true).ingredients[1]!;
    expect(quickNoteQuantity('omit', onion)).toEqual({ quantity: '0', original: false });
    expect(quickNoteQuantity('normal', onion)).toEqual({ quantity: '1', original: true });
    expect(quickNoteQuantity('extra', onion)).toEqual({ quantity: '2', original: false });
    expect(quickNoteQuantity('omit', cheese)).toBeNull();
    expect(quickNoteQuantity('normal', cheese)).toEqual({ quantity: '1', original: false });
    expect(quickNoteQuantity('extra', cheese)).toEqual({ quantity: '1', original: false });
    expect(quickNoteQuantity('extra', { ...onion, max_quantity: '1.5' })).toEqual({
      quantity: '1.5',
      original: false,
    });
    expect(quickNoteQuantity('extra', { ...onion, addable: false })).toBeNull();
    expect(quickNoteQuantity('omit', { ...onion, removable: false })).toBeNull();
  });
});

describe('resumen de la partida con indicaciones con efecto', () => {
  it('muestra la indicación una sola vez, sin repetir el cambio de su ingrediente', () => {
    const breakdown: ProductBreakdown = {
      ...groupBreakdown('0.00', '0.00'),
      groups: [],
      notes: 'Sin cebolla, Bien cocida',
      ingredients: [
        { ingredient_id: ids.onion, name: 'Cebolla', included: '1', requested: '0', extra: '0.00' },
        { ingredient_id: ids.cheese, name: 'Queso', included: '0', requested: '1', extra: '12.50' },
      ],
      quick_notes: [
        {
          note_id: notes.without.id,
          text: 'Sin cebolla',
          effect: 'omit',
          ingredient_id: ids.onion,
        },
        { note_id: notes.cooked.id, text: 'Bien cocida', effect: 'none', ingredient_id: null },
      ],
    };
    expect(describeBreakdown(breakdown)).toEqual(['Extra Queso', 'Sin cebolla, Bien cocida']);
    // Una partida anterior (solo texto) se resume como hoy.
    expect(describeBreakdown({ ...breakdown, quick_notes: undefined })).toEqual([
      'Sin Cebolla',
      'Extra Queso',
      'Sin cebolla, Bien cocida',
    ]);
  });
});

describe('indicaciones rápidas con efecto en la captura', () => {
  it('aplica la selección estructurada y guarda el identificador; dos efectos son excluyentes', async () => {
    render(<Harness productID={ids.burger} />);
    const block = await notesBlock();
    // Todas aplican a la hamburguesa, en su orden; el atajo repetido no se duplica.
    expect(names(block)).toEqual([
      'Sin cebolla',
      'Con cebolla',
      'Cebolla extra',
      'Bien cocida',
      'Queso extra',
      'Salsa aparte',
      'Escribir otra',
    ]);
    const ingredients = screen.getByRole('region', { name: 'Ingredientes' });
    fireEvent.click(within(block).getByRole('button', { name: 'Sin cebolla' }));
    expect(latest.ingredients).toEqual([{ ingredient_id: ids.onion, quantity: '0' }]);
    expect(latest.quick_notes).toEqual([{ note_id: notes.without.id, text: 'Sin cebolla' }]);
    expect(latest.notes).toBe('Sin cebolla');
    // El personalizador muestra la misma selección: «Sin cebolla» queda marcado allí también.
    expect(pressed(within(ingredients).getByRole('button', { name: 'Sin cebolla' }))).toBe('true');
    // «Cebolla extra» quita «Sin cebolla»: un solo efecto por ingrediente, sin acumular.
    fireEvent.click(within(block).getByRole('button', { name: 'Cebolla extra' }));
    expect(latest.ingredients).toEqual([{ ingredient_id: ids.onion, quantity: '2' }]);
    expect(latest.quick_notes).toEqual([{ note_id: notes.extra.id, text: 'Cebolla extra' }]);
    expect(latest.notes).toBe('Cebolla extra');
    expect(pressed(within(block).getByRole('button', { name: 'Sin cebolla' }))).toBe('false');
    expect(pressed(within(ingredients).getByRole('button', { name: /^Extra cebolla/ }))).toBe(
      'true',
    );
    // «Con cebolla» en un producto que ya la incluye no cambia ingredientes.
    fireEvent.click(within(block).getByRole('button', { name: 'Con cebolla' }));
    expect(latest.ingredients).toEqual([]);
    expect(latest.quick_notes).toEqual([{ note_id: notes.with.id, text: 'Con cebolla' }]);
    // Las de otro ingrediente y las de solo instrucción se combinan.
    fireEvent.click(within(block).getByRole('button', { name: 'Queso extra' }));
    fireEvent.click(within(block).getByRole('button', { name: 'Bien cocida' }));
    expect(latest.ingredients).toEqual([{ ingredient_id: ids.cheese, quantity: '1' }]);
    expect(latest.quick_notes).toEqual([
      { note_id: notes.with.id, text: 'Con cebolla' },
      { note_id: notes.cheese.id, text: 'Queso extra' },
      { note_id: notes.cooked.id, text: 'Bien cocida' },
    ]);
    expect(latest.notes).toBe('Con cebolla, Queso extra, Bien cocida');
    // Desmarcar una con efecto quita su texto, su identificador y su selección.
    fireEvent.click(within(block).getByRole('button', { name: 'Quitar indicación Queso extra' }));
    expect(latest.ingredients).toEqual([]);
    expect(latest.notes).toBe('Con cebolla, Bien cocida');
    expect(latest.quick_notes?.map((entry) => entry.text)).toEqual(['Con cebolla', 'Bien cocida']);
  });

  it('cambiar ese ingrediente en el personalizador quita la indicación con efecto', async () => {
    render(<Harness productID={ids.burger} />);
    const block = await notesBlock();
    const ingredients = screen.getByRole('region', { name: 'Ingredientes' });
    fireEvent.click(within(block).getByRole('button', { name: 'Cebolla extra' }));
    fireEvent.click(within(block).getByRole('button', { name: 'Bien cocida' }));
    expect(latest.notes).toBe('Cebolla extra, Bien cocida');
    // Quitar el extra desde el personalizador deja la cebolla incluida: la indicación ya no
    // describe la selección y se retira con su texto; la de solo instrucción se conserva.
    fireEvent.click(within(ingredients).getByRole('button', { name: /^Extra cebolla/ }));
    expect(latest.ingredients).toEqual([]);
    expect(latest.quick_notes).toEqual([{ note_id: notes.cooked.id, text: 'Bien cocida' }]);
    expect(latest.notes).toBe('Bien cocida');
    expect(pressed(within(block).getByRole('button', { name: 'Cebolla extra' }))).toBe('false');
    // Elegir la indicación de nuevo y cambiar a «Sin cebolla» en el personalizador también la quita.
    fireEvent.click(within(block).getByRole('button', { name: 'Cebolla extra' }));
    fireEvent.click(within(ingredients).getByRole('button', { name: 'Sin cebolla' }));
    expect(latest.ingredients).toEqual([{ ingredient_id: ids.onion, quantity: '0' }]);
    expect(latest.quick_notes).toEqual([{ note_id: notes.cooked.id, text: 'Bien cocida' }]);
  });

  it('oculta las indicaciones con efecto en un producto sin ese ingrediente', async () => {
    render(<Harness productID={ids.salad} />);
    const block = await notesBlock();
    // Solo la de instrucción; «Sin cebolla» tampoco aparece como atajo de texto.
    expect(names(block)).toEqual(['Bien cocida', 'Salsa aparte', 'Escribir otra']);
    fireEvent.click(within(block).getByRole('button', { name: 'Bien cocida' }));
    expect(latest.quick_notes).toEqual([{ note_id: notes.cooked.id, text: 'Bien cocida' }]);
    expect(latest.ingredients).toEqual([]);
  });
});

describe('complementos de varias porciones en la captura', () => {
  function card(name: string) {
    return screen.getByRole('group', { name: new RegExp(`^${name}: `) });
  }

  it('un toque suma, «−» resta en la tarjeta y sustituir es de dos toques', async () => {
    render(<Harness productID={ids.burger} breakdown={groupBreakdown('0.00', '0.00')} />);
    const group = await screen.findByRole('region', { name: 'Guarniciones' });
    // Los incluidos llegan en 1, con su precio y la cantidad grande; el encabezado dice «2 de 2».
    expect(within(group).getByText('2 de 2')).toBeTruthy();
    expect(within(group).getByText('Sin cambio')).toBeTruthy();
    const rice = card('Arroz');
    expect(rice.getAttribute('aria-label')).toBe('Arroz: 1 porción');
    expect(within(rice).getByText('$30.00')).toBeTruthy();
    expect(rice.querySelector('.capture-portion-quantity')?.textContent).toBe('1');
    expect(within(card('Pasta')).queryByRole('button', { name: /Quitar una porción/ })).toBeNull();
    // Un toque en la tarjeta suma una porción (no la quita).
    fireEvent.click(within(rice).getByRole('button', { name: 'Agregar una porción de Arroz' }));
    expect(card('Arroz').getAttribute('aria-label')).toBe('Arroz: 2 porciones');
    expect(within(group).getByText('3 de 2')).toBeTruthy();
    // Pasar del máximo del grupo avisa en el propio grupo y no cambia la selección.
    fireEvent.click(
      within(card('Papas')).getByRole('button', { name: 'Agregar una porción de Papas' }),
    );
    expect(within(group).getByRole('status').textContent).toBe(
      'Guarniciones admite hasta 3 porciones: toca «−» en otra opción para cambiarla.',
    );
    expect(card('Papas').getAttribute('aria-label')).toBe('Papas: sin elegir');
    // «−» en la tarjeta resta una porción; el aviso desaparece con el cambio.
    fireEvent.click(
      within(card('Arroz')).getByRole('button', { name: 'Quitar una porción de Arroz' }),
    );
    fireEvent.click(
      within(card('Arroz')).getByRole('button', { name: 'Quitar una porción de Arroz' }),
    );
    expect(within(group).queryByRole('status')).toBeNull();
    expect(card('Arroz').getAttribute('aria-label')).toBe('Arroz: sin elegir');
    expect(within(card('Arroz')).queryByRole('button', { name: /Quitar una porción/ })).toBeNull();
    // Segundo toque de la sustitución: pasta en lugar de arroz.
    fireEvent.click(
      within(card('Pasta')).getByRole('button', { name: 'Agregar una porción de Pasta' }),
    );
    expect(latest.groups).toEqual([
      {
        group_id: ids.sides,
        options: [
          { option_id: ids.greens, quantity: '1' },
          { option_id: ids.pasta, quantity: '1' },
        ],
      },
    ]);
    expect(within(group).getByText('2 de 2')).toBeTruthy();
  });

  it('muestra en vivo el cambio de precio que calcula Go', async () => {
    const { rerender } = render(<Harness productID={ids.burger} breakdown={null} />);
    const group = await screen.findByRole('region', { name: 'Guarniciones' });
    expect(within(group).getByText('Calculando…')).toBeTruthy();
    rerender(<Harness productID={ids.burger} breakdown={groupBreakdown('10.00', '0.00')} />);
    await waitFor(() => expect(within(group).getByText('+$10.00')).toBeTruthy());
    rerender(<Harness productID={ids.burger} breakdown={groupBreakdown('0.00', '0.00')} />);
    await waitFor(() => expect(within(group).getByText('Sin cambio')).toBeTruthy());
  });

  // Etapa 9A, §3.1: con la opción «El precio puede bajar al cambiar complementos» Go entrega la
  // diferencia del grupo con signo y la captura la muestra tal cual, sin calcularla.
  it('muestra con signo la diferencia negativa que calcula Go', async () => {
    const { rerender } = render(
      <Harness productID={ids.burger} breakdown={groupBreakdown('-10.00', '0.00')} />,
    );
    const group = await screen.findByRole('region', { name: 'Guarniciones' });
    await waitFor(() => expect(within(group).getByText('−$10.00')).toBeTruthy());
    rerender(<Harness productID={ids.burger} breakdown={groupBreakdown('-10.00', '3.00')} />);
    await waitFor(() =>
      expect(within(group).getByText('Equivalencia −$10.00 · Extras +$3.00')).toBeTruthy(),
    );
    rerender(<Harness productID={ids.burger} breakdown={groupBreakdown('5.00', '0.00')} />);
    await waitFor(() => expect(within(group).getByText('+$5.00')).toBeTruthy());
    rerender(<Harness productID={ids.burger} breakdown={groupBreakdown('0.00', '0.00')} />);
    await waitFor(() => expect(within(group).getByText('Sin cambio')).toBeTruthy());
  });
});
