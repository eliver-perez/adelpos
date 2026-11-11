// @vitest-environment jsdom

import { cleanup, render, screen, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { originalCustomization } from '../../../components/catalog/customizationRules';
import { ProductDetails } from '../../../components/catalog/QuoteBreakdown';
import type { Lookups, ProductBreakdown } from '../../../services/catalog';
import { TouchCustomizer } from './TouchCustomizer';

const native = vi.hoisted(() => ({ invoke: vi.fn() }));
vi.mock('@tauri-apps/api/core', () => ({ invoke: native.invoke, isTauri: () => true }));

// Revisión independiente de la etapa 9A, §3.1: con la opción «El precio puede bajar al cambiar
// complementos», lo que se ve al personalizar es el efecto real que calculó Go
// (`applied_difference` por grupo y `complement_adjustment` en el resumen), no una diferencia de
// valor que el límite del precio ya no aplicó completa. Solo se sustituye el transporte nativo.

const ids = {
  dishes: '00000000-0000-4000-8000-0000000009d1',
  plate: '00000000-0000-4000-8000-0000000009e1',
  sides: '00000000-0000-4000-8000-0000000009f1',
  rice: '00000000-0000-4000-8000-0000000009f2',
  salad: '00000000-0000-4000-8000-0000000009f3',
};

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
  max_quantity: '2',
  options: [option(ids.rice, 'Arroz', '20.00'), option(ids.salad, 'Ensalada', '15.00')],
};

const plate = {
  id: ids.plate,
  name: 'Filete',
  description: '',
  active: true,
  revision: 1,
  category_id: ids.dishes,
  code: 'FILETE',
  barcode: null,
  kind: 'product',
  base_price: '20.00',
  tax_rate: null,
  image_id: null,
  sort_order: 10,
  archived: false,
  temporary_unavailable: false,
  availability: { days: [], windows: [] },
  variants: [],
  ingredients: [],
  groups: [
    {
      group_id: ids.sides,
      min_quantity: null,
      max_quantity: null,
      included: [
        { option_id: ids.rice, quantity: '1' },
        { option_id: ids.salad, quantity: '1' },
      ],
    },
  ],
  slots: [],
  price_can_decrease: true,
};

const lookups = {
  categories: [],
  ingredients: [],
  complements: [],
  portions: [],
  groups: [sides],
  'price-lists': [],
  palette: [],
  products: [],
  category_quick_notes: [],
} as unknown as Lookups;

// Desglose del filete de $20 con $35 incluidos al quitar todo: la diferencia de valor es −$35,
// pero el precio solo baja $20 (no queda negativo).
function breakdown(overrides: Partial<ProductBreakdown>, group: Record<string, string>) {
  return {
    product_id: ids.plate,
    revision: 1,
    name: 'Filete',
    base_price: '20.00',
    price_origin: 'Precio base del producto',
    unit_price: '0.00',
    ingredient_extras: '0.00',
    group_extras: '-20.00',
    component_extras: '0.00',
    notes: '',
    ingredients: [],
    groups: [
      {
        group_id: ids.sides,
        name: 'Guarniciones',
        mode: 'value',
        original_value: '35.00',
        requested_value: '0.00',
        difference: '-35.00',
        extras: '0.00',
        options: [],
        ...group,
      },
    ],
    components: [],
    ...overrides,
  } as ProductBreakdown;
}

const clamped = breakdown(
  { price_can_decrease: true, complement_adjustment: '-20.00' },
  { applied_difference: '-20.00' },
);

function Customizer({ value, depth = 0 }: { value: ProductBreakdown | null; depth?: number }) {
  return (
    <TouchCustomizer
      productID={ids.plate}
      variantID={null}
      onVariantChange={() => undefined}
      value={originalCustomization()}
      onChange={() => undefined}
      lookups={lookups}
      breakdown={value}
      noteShortcuts={[]}
      depth={depth}
    />
  );
}

beforeEach(() => {
  native.invoke.mockImplementation(async (command: string, args: { operation: string }) => {
    if (command !== 'api_request' || args.operation !== 'catalog_configuration') {
      return { status: 200, body: {} };
    }
    return {
      status: 200,
      body: {
        product: plate,
        category: { id: ids.dishes },
        groups: [sides],
        timezone: 'America/Monterrey',
        availability_reason: '',
      },
    };
  });
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe('efecto real de los complementos al personalizar', () => {
  it('el grupo y el resumen muestran lo que bajó el precio, no la diferencia que el límite no aplicó', async () => {
    render(<Customizer value={clamped} />);
    const group = await screen.findByRole('region', { name: 'Guarniciones' });
    expect(group.querySelector('.capture-block-charge')?.textContent).toBe('−$20.00');
    expect(within(group).queryByText('−$35.00')).toBeNull();
    expect(screen.getByText('Ajuste por complementos: −$20.00')).toBeTruthy();
  });

  it('sin límite el efecto coincide con la diferencia y sin reducción lo indica', async () => {
    const { rerender } = render(
      <Customizer
        value={breakdown(
          { price_can_decrease: true, complement_adjustment: '-10.00' },
          { difference: '-10.00', applied_difference: '-10.00' },
        )}
      />,
    );
    const group = await screen.findByRole('region', { name: 'Guarniciones' });
    expect(group.querySelector('.capture-block-charge')?.textContent).toBe('−$10.00');
    expect(screen.getByText('Ajuste por complementos: −$10.00')).toBeTruthy();
    rerender(
      <Customizer
        value={breakdown(
          { price_can_decrease: true, complement_adjustment: '0.00' },
          { difference: '5.00', applied_difference: '5.00' },
        )}
      />,
    );
    expect(group.querySelector('.capture-block-charge')?.textContent).toBe('+$5.00');
    expect(screen.getByText('Ajuste por complementos: sin cambio')).toBeTruthy();
  });

  it('sin la opción conserva la diferencia de siempre y no muestra el resumen', async () => {
    render(<Customizer value={breakdown({}, { difference: '5.00' })} />);
    const group = await screen.findByRole('region', { name: 'Guarniciones' });
    expect(group.querySelector('.capture-block-charge')?.textContent).toBe('+$5.00');
    expect(screen.queryByText(/Ajuste por complementos/)).toBeNull();
  });

  it('en un componente de combo indica que la reducción se aplica en el precio del combo', async () => {
    render(
      <Customizer
        depth={1}
        value={breakdown(
          { price_can_decrease: true, complement_adjustment: '-15.00', unit_price: '20.00' },
          { difference: '-15.00', applied_difference: '-15.00' },
        )}
      />,
    );
    await screen.findByRole('region', { name: 'Guarniciones' });
    expect(
      screen.getByText('Ajuste por complementos en el precio del combo: −$15.00'),
    ).toBeTruthy();
  });
});

describe('desglose calculado por Go con el límite del precio', () => {
  it('muestra la diferencia de valor y, aparte, el efecto real en el precio', () => {
    render(<ProductDetails value={clamped} />);
    const group = screen.getByRole('region', { name: 'Guarniciones' });
    expect(within(group).getByText('Diferencia')).toBeTruthy();
    expect(within(group).getByText('−$35.00')).toBeTruthy();
    expect(within(group).getByText('Efecto en el precio')).toBeTruthy();
    expect(within(group).getByText('−$20.00')).toBeTruthy();
    expect(screen.getByText(/el precio bajó −\$20\.00 por los complementos elegidos/)).toBeTruthy();
  });

  it('en un componente de combo la reducción se atribuye al precio del combo', () => {
    const child = breakdown(
      { price_can_decrease: true, complement_adjustment: '-15.00', unit_price: '20.00' },
      { difference: '-15.00', applied_difference: '-15.00' },
    );
    render(
      <ProductDetails
        value={{
          ...breakdown({ price_can_decrease: true, complement_adjustment: '-15.00' }, {}),
          groups: [],
          components: [
            { slot_id: 'main', name: 'Plato', quantity: 1, charge: '2.50', product: child },
          ],
        }}
      />,
    );
    expect(screen.getByText('Efecto en el precio del combo')).toBeTruthy();
    expect(
      screen.getByText(/los complementos elegidos bajan −\$15\.00 el precio del combo/),
    ).toBeTruthy();
    expect(screen.getByText(/el precio bajó −\$15\.00 por los complementos elegidos/)).toBeTruthy();
  });
});
