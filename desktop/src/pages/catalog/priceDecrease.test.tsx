// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen, within } from '@testing-library/react';
import { useState } from 'react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { ProductEditor } from '../../components/catalog/ProductEditor';
import { ProductDetails } from '../../components/catalog/QuoteBreakdown';
import type { Lookups, Product, ProductBreakdown } from '../../services/catalog';
import { newCatalogDocument } from './catalogState';

// Etapa 9A, §3.1: «El precio puede bajar al cambiar complementos». La interfaz solo presenta la
// opción y los importes que calcula Go.

const native = vi.hoisted(() => ({ invoke: vi.fn() }));
vi.mock('@tauri-apps/api/core', () => ({ invoke: native.invoke, isTauri: () => true }));

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

const lookups: Lookups = {
  categories: [],
  ingredients: [],
  complements: [],
  portions: [],
  groups: [],
  'price-lists': [],
  products: [],
  palette: [],
};

function Editor({ initial, can }: { initial: Product; can: (permission: string) => boolean }) {
  const [value, setValue] = useState(initial);
  return (
    <>
      <ProductEditor value={value} onChange={setValue} lookups={lookups} can={can} />
      <output data-testid="price-can-decrease">{JSON.stringify(value.price_can_decrease)}</output>
    </>
  );
}

function toggle() {
  return screen.getByRole('button', { name: 'El precio puede bajar al cambiar complementos' });
}

describe('opción «El precio puede bajar al cambiar complementos» en el editor de producto', () => {
  it('inicia desactivada en un producto nuevo y se activa con el interruptor', () => {
    const product = newCatalogDocument('products', false, '') as Product;
    expect(product.price_can_decrease).toBe(false);
    render(<Editor initial={product} can={() => true} />);
    expect(toggle().getAttribute('aria-pressed')).toBe('false');
    expect(screen.getByText(/Desactivada: sustituir por un complemento más barato/)).toBeTruthy();
    fireEvent.click(toggle());
    expect(toggle().getAttribute('aria-pressed')).toBe('true');
    expect(screen.getByTestId('price-can-decrease').textContent).toBe('true');
    expect(screen.getByText(/Activada: sustituir por un complemento más barato/)).toBeTruthy();
    // La explicación queda ligada al interruptor para los lectores de pantalla.
    const hint = document.getElementById(toggle().getAttribute('aria-describedby') ?? '');
    expect(hint?.textContent).toMatch(/Solo afecta a los pedidos que se confirmen después/);
    fireEvent.click(toggle());
    expect(screen.getByTestId('price-can-decrease').textContent).toBe('false');
  });

  it('cambiarla exige el permiso de precios, como el precio y la tasa de IVA', () => {
    const product = {
      ...(newCatalogDocument('products', false, '') as Product),
      price_can_decrease: true,
    };
    render(
      <Editor initial={product} can={(permission) => permission !== 'catalog.prices.manage'} />,
    );
    expect(toggle().getAttribute('aria-pressed')).toBe('true');
    expect((toggle() as HTMLButtonElement).disabled).toBe(true);
    fireEvent.click(toggle());
    expect(screen.getByTestId('price-can-decrease').textContent).toBe('true');
    expect(screen.getByText(/se necesita el permiso de precios/)).toBeTruthy();
  });

  it('un producto anterior sin el campo se muestra desactivado', () => {
    const product = { ...(newCatalogDocument('products', false, '') as Product) };
    delete product.price_can_decrease;
    render(<Editor initial={product} can={() => true} />);
    expect(toggle().getAttribute('aria-pressed')).toBe('false');
  });
});

function breakdown(overrides: Partial<ProductBreakdown>, difference: string): ProductBreakdown {
  return {
    product_id: 'plate',
    revision: 1,
    name: 'Filete',
    base_price: '150.00',
    price_origin: 'Precio base del producto',
    unit_price: '140.00',
    ingredient_extras: '0.00',
    group_extras: '-10.00',
    component_extras: '0.00',
    notes: '',
    ingredients: [],
    groups: [
      {
        group_id: 'sides',
        name: 'Guarniciones',
        mode: 'value',
        original_value: '35.00',
        requested_value: '25.00',
        difference,
        extras: '0.00',
        options: [],
      },
    ],
    components: [],
    ...overrides,
  };
}

describe('desglose de la cotización con complementos', () => {
  it('muestra la diferencia con signo y el efecto de la opción que calculó Go', () => {
    render(
      <ProductDetails
        value={breakdown({ price_can_decrease: true, complement_adjustment: '-10.00' }, '-10.00')}
      />,
    );
    expect(screen.queryByText('Diferencia positiva')).toBeNull();
    const group = screen.getByRole('region', { name: 'Guarniciones' });
    expect(within(group).getByText('Diferencia')).toBeTruthy();
    expect(within(group).getByText('−$10.00')).toBeTruthy();
    // «Complementos» del producto lleva el mismo importe con signo, sin aritmética en React.
    expect(screen.getAllByText('−$10.00').length).toBeGreaterThanOrEqual(2);
    expect(screen.getByText(/el precio bajó −\$10\.00 por los complementos elegidos/)).toBeTruthy();
  });

  it('sin la opción no muestra la nota y una diferencia positiva lleva «+»', () => {
    render(
      <ProductDetails value={breakdown({ group_extras: '5.00', unit_price: '155.00' }, '5.00')} />,
    );
    expect(screen.queryByText(/El precio puede bajar/)).toBeNull();
    const group = screen.getByRole('region', { name: 'Guarniciones' });
    expect(within(group).getByText('Diferencia')).toBeTruthy();
    expect(within(group).getByText('+$5.00')).toBeTruthy();
  });

  it('con la opción y sin reducción lo indica', () => {
    render(
      <ProductDetails
        value={breakdown(
          { price_can_decrease: true, complement_adjustment: '0.00', group_extras: '0.00' },
          '0.00',
        )}
      />,
    );
    expect(screen.getByText(/sin reducción con los complementos elegidos/)).toBeTruthy();
  });
});
