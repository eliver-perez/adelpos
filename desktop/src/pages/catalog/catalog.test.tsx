// @vitest-environment jsdom

import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { useState } from 'react';
import { CatalogPage } from './CatalogPage';
import { PreviewPage } from './PreviewPage';
import { newCatalogDocument } from './catalogState';
import {
  ProductCustomizer,
  originalCustomization,
} from '../../components/catalog/ProductCustomizer';
import type {
  Category,
  Configuration,
  Lookups,
  OptionGroup,
  Product,
} from '../../services/catalog';

const native = vi.hoisted(() => ({ invoke: vi.fn() }));
vi.mock('@tauri-apps/api/core', () => ({ invoke: native.invoke, isTauri: () => true }));

const category = {
  ...newCatalogDocument('categories', false, ''),
  id: 'category',
  name: 'Platillos',
  revision: 1,
} as Category;
const group: OptionGroup = {
  id: 'sides',
  name: 'Guarniciones',
  description: '',
  active: true,
  revision: 1,
  pricing_mode: 'value',
  min_quantity: '0',
  max_quantity: null,
  options: ['Arroz', 'Ensalada', 'Papas'].map((name, index) => ({
    id: name,
    name,
    complement_id: name,
    portion_id: null,
    portion_name: 'Entera',
    reference_value: ['20', '15', '25'][index] ?? '0',
    extra_price: '0',
    max_quantity: null,
    active: true,
    tiers: [],
  })),
};
const product = {
  ...newCatalogDocument('products', false, category.id),
  id: 'product',
  revision: 1,
  name: 'Filete',
  code: 'FILETE',
  base_price: '150.00',
  ingredients: [
    {
      ingredient_id: 'onion',
      name: 'Cebolla',
      included_quantity: '1',
      removable: true,
      addable: true,
      max_quantity: '3',
      extra_price: '2.50',
      recipe_quantity: '0',
    },
  ],
  groups: [
    {
      group_id: group.id,
      min_quantity: null,
      max_quantity: null,
      included: [
        { option_id: 'Arroz', quantity: '1' },
        { option_id: 'Ensalada', quantity: '1' },
      ],
    },
  ],
} as Product;
const lookups: Lookups = {
  categories: [category],
  ingredients: [],
  complements: [],
  portions: [],
  groups: [group],
  'price-lists': [],
  products: [{ ...product, variants: [] }],
  palette: [],
};
const configuration: Configuration = {
  product,
  category,
  groups: [group],
  availability_reason: '',
  timezone: 'America/Monterrey',
};
const quote = (total = '150.00') => ({
  total,
  subtotal: total,
  discount: '0.00',
  lines: [],
  promotions: [],
  timezone: 'America/Monterrey',
  price_list_name: 'Restaurante',
  price_list_revision: 1,
  calculated_at: '2026-11-07T18:00:00Z',
});

beforeEach(() => {
  native.invoke.mockImplementation(
    async (_command: string, args: { operation: string; body?: unknown }) => {
      let body: unknown;
      switch (args.operation) {
        case 'catalog_lookups':
          body = lookups;
          break;
        case 'catalog_configuration':
          body = configuration;
          break;
        case 'catalog_preview':
          body = quote();
          break;
        case 'catalog_create_categories':
          body = { ...(args.body as object), id: 'new-category', revision: 1 };
          break;
        default:
          body = { items: [], total: 0, page: 1, page_size: 20 };
      }
      return { status: 200, body };
    },
  );
});
afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  vi.useRealTimers();
});

describe('catálogo y personalización', () => {
  it('guarda una categoría por el puente nativo con su estado y disponibilidad', async () => {
    render(<CatalogPage can={() => true} />);
    await screen.findByText('Aún no hay registros');
    fireEvent.click(screen.getByRole('tab', { name: 'Categorías' }));
    await waitFor(() =>
      expect(screen.getByRole('tab', { name: 'Categorías' }).getAttribute('aria-selected')).toBe(
        'true',
      ),
    );
    fireEvent.click(screen.getByRole('button', { name: 'Crear registro' }));
    fireEvent.change(await screen.findByLabelText('Nombre'), { target: { value: 'Bebidas' } });
    fireEvent.click(screen.getByRole('button', { name: 'Crear registro' }));
    await waitFor(() =>
      expect(native.invoke).toHaveBeenCalledWith(
        'api_request',
        expect.objectContaining({
          operation: 'catalog_create_categories',
          body: expect.objectContaining({
            name: 'Bebidas',
            active: true,
            revision: 0,
            availability: { days: [], windows: [] },
          }),
        }),
      ),
    );
  });

  it('oculta creación cuando solo se permite consultar', async () => {
    render(<CatalogPage can={(permission) => permission === 'catalog.read'} />);
    await screen.findByText('Aún no hay registros');
    expect(screen.queryByRole('button', { name: /Crear/ })).toBeNull();
  });

  it('conserva otras selecciones y permite retirar y restaurar todos los complementos', async () => {
    function Editor() {
      const [value, setValue] = useState(originalCustomization());
      return (
        <ProductCustomizer
          productID={product.id}
          variantID={null}
          onVariantChange={() => {}}
          value={value}
          onChange={setValue}
          onReset={() => setValue(originalCustomization())}
          lookups={lookups}
        />
      );
    }
    render(<Editor />);
    await screen.findByRole('heading', { name: 'Guarniciones' });
    const quantities = screen.getAllByLabelText('Porciones solicitadas') as HTMLInputElement[];
    expect(quantities.map((input) => input.value)).toEqual(['1', '1', '0']);
    fireEvent.change(quantities[2]!, { target: { value: '2' } });
    fireEvent.change(quantities[0]!, { target: { value: '0' } });
    expect(quantities.map((input) => input.value)).toEqual(['0', '1', '2']);
    fireEvent.change(screen.getByLabelText(/Observaciones/), { target: { value: 'Salsa aparte' } });
    fireEvent.click(
      screen.getByRole('button', { name: 'Retirar todos los complementos de este grupo' }),
    );
    expect(quantities.map((input) => input.value)).toEqual(['0', '0', '0']);
    expect((screen.getByLabelText(/Observaciones/) as HTMLTextAreaElement).value).toBe(
      'Salsa aparte',
    );
    fireEvent.click(screen.getByRole('button', { name: 'Restablecer configuración original' }));
    expect(quantities.map((input) => input.value)).toEqual(['1', '1', '0']);
  });

  it('presenta importes del servidor y envía la selección estructurada sin calcular en JavaScript', async () => {
    render(<PreviewPage can={() => true} />);
    await screen.findByRole('combobox', { name: 'Producto a comprobar' });
    fireEvent.change(screen.getByLabelText('Producto a comprobar'), {
      target: { value: product.id },
    });
    await screen.findByRole('heading', { name: 'Guarniciones' });
    const quantities = screen.getAllByLabelText('Porciones solicitadas');
    fireEvent.change(quantities[0]!, { target: { value: '0' } });
    fireEvent.change(quantities[2]!, { target: { value: '1' } });
    await screen.findByRole('heading', { name: '$150.00' });
    expect(native.invoke).toHaveBeenCalledWith(
      'api_request',
      expect.objectContaining({
        operation: 'catalog_preview',
        body: expect.objectContaining({
          lines: [
            expect.objectContaining({
              product_id: 'product',
              customization: expect.objectContaining({
                groups: [
                  {
                    group_id: 'sides',
                    options: expect.arrayContaining([
                      { option_id: 'Arroz', quantity: '0' },
                      { option_id: 'Papas', quantity: '1' },
                    ]),
                  },
                ],
              }),
            }),
          ],
        }),
      }),
    );
  });

  it('restablece un componente sin perder las observaciones del combo', async () => {
    const combo: Product = {
      ...product,
      id: 'combo',
      name: 'Combo',
      kind: 'combo',
      ingredients: [],
      groups: [],
      slots: [
        {
          id: 'main',
          name: 'Principal',
          min_quantity: 1,
          max_quantity: 1,
          included_quantity: 1,
          default_quantity: 1,
          default_product_id: product.id,
          options: [{ product_id: product.id, substitution_price: '0', tiers: [] }],
        },
      ],
    };
    native.invoke.mockImplementation(async (_command: string, args: { resourceId: string }) => ({
      status: 200,
      body:
        args.resourceId === 'combo'
          ? { ...configuration, product: combo, groups: [] }
          : configuration,
    }));
    function Editor() {
      const [value, setValue] = useState(originalCustomization());
      return (
        <ProductCustomizer
          productID="combo"
          variantID={null}
          onVariantChange={() => {}}
          value={value}
          onChange={setValue}
          onReset={() => setValue(originalCustomization())}
          lookups={lookups}
        />
      );
    }
    render(<Editor />);
    await screen.findByText('Personalizar este componente');
    fireEvent.click(screen.getByText('Personalizar este componente'));
    await screen.findByRole('heading', { name: 'Guarniciones' });
    const notes = screen.getAllByLabelText(
      /Observaciones para preparación/,
    ) as HTMLTextAreaElement[];
    fireEvent.change(notes[0]!, { target: { value: 'Salsa aparte del hijo' } });
    fireEvent.change(notes[1]!, { target: { value: 'Observaciones del combo' } });
    const resets = screen.getAllByRole('button', { name: 'Restablecer configuración original' });
    fireEvent.click(resets[0]!);
    expect(notes[0]?.value).toBe('');
    expect(notes[1]?.value).toBe('Observaciones del combo');
  });

  it('ignora una respuesta tardía para una selección anterior', async () => {
    let completeOld: ((value: unknown) => void) | undefined;
    let previews = 0;
    const originalTransport = native.invoke.getMockImplementation();
    native.invoke.mockImplementation((_command: string, args: { operation: string }) => {
      if (args.operation === 'catalog_preview') {
        previews++;
        if (previews === 1)
          return new Promise((resolve) => {
            completeOld = resolve;
          });
        return Promise.resolve({ status: 200, body: quote('300.00') });
      }
      return originalTransport?.(_command, args);
    });
    render(<PreviewPage can={() => false} />);
    await screen.findByLabelText('Producto a comprobar');
    fireEvent.change(screen.getByLabelText('Producto a comprobar'), {
      target: { value: product.id },
    });
    await waitFor(() => expect(previews).toBe(1));
    fireEvent.change(screen.getByLabelText('Unidades para probar promociones'), {
      target: { value: '2' },
    });
    await screen.findByRole('heading', { name: '$300.00' });
    await act(async () => {
      completeOld?.({ status: 200, body: quote('150.00') });
    });
    expect(screen.getByRole('heading', { name: '$300.00' })).toBeTruthy();
    expect(screen.queryByRole('heading', { name: '$150.00' })).toBeNull();
  });
});
