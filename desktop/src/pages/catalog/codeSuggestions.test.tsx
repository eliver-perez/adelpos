// @vitest-environment jsdom

// Etapa 9A (§3.4): el código interno sugerido de la etapa 6.1 también en las presentaciones nuevas
// de un producto y en la copia de un producto, sin sobrescribir lo que el usuario escribió.

import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { useState } from 'react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { VariantEditor } from '../../components/catalog/ProductDetailsEditors';
import type { Lookups, Product, ProductReference } from '../../services/catalog';
import { CatalogRecord } from './CatalogRecord';
import { newCatalogDocument } from './catalogState';

afterEach(() => {
  cleanup();
});

function reference(id: string, code: string): ProductReference {
  return {
    id,
    name: id,
    kind: 'product',
    code,
    description: '',
    active: true,
    archived: false,
    revision: 1,
    category_id: 'category',
    sort_order: 0,
    variants: [],
  };
}

const lookups: Lookups = {
  categories: [],
  ingredients: [],
  complements: [],
  portions: [],
  groups: [],
  'price-lists': [],
  products: [reference('otro', 'cafe-de-olla-chico')],
  palette: [],
};

function product(extra: Partial<Product> = {}): Product {
  return {
    ...(newCatalogDocument('products', false, 'category') as Product),
    name: 'Café de olla',
    code: 'cafe-de-olla',
    ...extra,
  };
}

function Variants({ initial }: { initial: Product }) {
  const [value, setValue] = useState(initial);
  return <VariantEditor value={value} onChange={setValue} lookups={lookups} can={() => false} />;
}

describe('código de presentaciones nuevas', () => {
  it('se forma con el del producto, evita los conocidos y no sobrescribe lo escrito', () => {
    render(<Variants initial={product()} />);
    fireEvent.click(screen.getByRole('button', { name: /Agregar/ }));
    const name = screen.getByLabelText('Nombre de presentación');
    const code = () => screen.getByLabelText(/^Código de presentación/) as HTMLInputElement;
    fireEvent.change(name, { target: { value: 'Grande' } });
    expect(code().value).toBe('cafe-de-olla-grande');
    // «cafe-de-olla-chico» ya es el código de otro producto.
    fireEvent.change(name, { target: { value: 'Chico' } });
    expect(code().value).toBe('cafe-de-olla-chico-2');
    fireEvent.change(code(), { target: { value: 'CDO-CH' } });
    fireEvent.change(name, { target: { value: 'Chico de barro' } });
    expect(code().value).toBe('CDO-CH');
  });

  it('una presentación guardada nunca cambia de código', () => {
    render(
      <Variants
        initial={product({
          variants: [
            {
              id: 'guardada',
              name: 'Grande',
              code: 'cafe-de-olla-grande',
              barcode: null,
              price: '45.00',
              active: true,
              availability: { days: [], windows: [] },
            },
          ],
        })}
      />,
    );
    fireEvent.change(screen.getByLabelText('Nombre de presentación'), {
      target: { value: 'Jarra' },
    });
    expect((screen.getByLabelText(/^Código de presentación/) as HTMLInputElement).value).toBe(
      'cafe-de-olla-grande',
    );
  });
});

describe('código de la copia de un producto', () => {
  it('se sugiere desde el nombre de la copia y deja de seguirlo al editarlo', () => {
    render(
      <CatalogRecord
        record={product({ id: 'cafe', revision: 2 })}
        resource="products"
        categoryName="Bebidas"
        editable
        can={() => true}
        onOpen={() => undefined}
        onViewProducts={() => undefined}
        runAction={vi.fn(async () => true)}
        existingCodes={['cafe-de-olla', 'cafe-de-olla-copia']}
      />,
    );
    fireEvent.click(screen.getByRole('button', { name: 'Duplicar' }));
    const code = () => screen.getByLabelText(/^Código interno de la copia/) as HTMLInputElement;
    expect(code().value).toBe('cafe-de-olla-copia-2');
    fireEvent.change(screen.getByLabelText('Nombre de la copia'), {
      target: { value: 'Café de olla grande' },
    });
    expect(code().value).toBe('cafe-de-olla-grande');
    fireEvent.change(code(), { target: { value: 'CDO-G' } });
    fireEvent.change(screen.getByLabelText('Nombre de la copia'), {
      target: { value: 'Café de olla jarra' },
    });
    expect(code().value).toBe('CDO-G');
  });
});
