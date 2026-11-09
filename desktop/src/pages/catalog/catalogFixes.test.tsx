// @vitest-environment jsdom

// Etapa 6.1 · fase A: cada prueba reproduce un error reportado al usar la aplicación nativa y
// falla con el código anterior a la corrección.

import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { useState } from 'react';
import { CatalogPage } from './CatalogPage';
import { newCatalogDocument } from './catalogState';
import { ComboEditor } from '../../components/catalog/ComboEditor';
import { ImageEditor } from '../../components/catalog/ImageEditor';
import { ProductEditor } from '../../components/catalog/ProductEditor';
import {
  DecimalField,
  IntegerField,
  ScheduleEditor,
  TierEditor,
  emptySchedule,
} from '../../components/catalog/EditorFields';
import type {
  Category,
  ComboSlot,
  Lookups,
  Product,
  ProductReference,
  QuantityTier,
  Schedule,
} from '../../services/catalog';

const native = vi.hoisted(() => ({ invoke: vi.fn() }));
vi.mock('@tauri-apps/api/core', () => ({ invoke: native.invoke, isTauri: () => true }));

const category = {
  ...newCatalogDocument('categories', false, ''),
  id: 'category',
  name: 'Platillos',
  revision: 1,
} as Category;

function reference(id: string, name: string, kind = 'product'): ProductReference {
  return {
    id,
    name,
    kind,
    code: id.toUpperCase(),
    description: '',
    active: true,
    archived: false,
    revision: 1,
    category_id: 'category',
    sort_order: 0,
    variants: [],
  };
}

const burger = reference('burger', 'Hamburguesa');
const fries = reference('fries', 'Papas');
const comboSelf = reference('combo-self', 'Combo del día', 'combo');
const lookups: Lookups = {
  categories: [category],
  ingredients: [],
  complements: [],
  portions: [],
  groups: [],
  'price-lists': [],
  products: [burger, fries, comboSelf],
  palette: [],
};

beforeEach(() => {
  native.invoke.mockImplementation(async (_command: string, args: { operation: string }) => {
    if (args.operation === 'catalog_lookups') {
      return { status: 200, body: lookups };
    }
    return { status: 200, body: { items: [], total: 0, page: 1, page_size: 20 } };
  });
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  vi.restoreAllMocks();
});

async function openNewProduct() {
  render(<CatalogPage can={() => true} />);
  await screen.findByText('Aún no hay registros');
  fireEvent.click(screen.getByRole('button', { name: 'Crear registro' }));
  return screen.findByLabelText('Nombre');
}

describe('etapa 6.1 · errores del catálogo', () => {
  it('1.2 «Volver al listado» pregunta dentro de la aplicación aunque window.confirm no exista', async () => {
    // El WebView nativo de macOS resuelve window.confirm como «no» sin mostrar nada.
    vi.spyOn(window, 'confirm').mockReturnValue(false);
    const name = await openNewProduct();
    fireEvent.change(name, { target: { value: 'Machacado' } });
    fireEvent.click(screen.getByRole('button', { name: 'Volver al listado' }));
    const dialog = await screen.findByRole('alertdialog', { name: 'Cambios sin guardar' });
    expect(window.confirm).not.toHaveBeenCalled();
    fireEvent.click(within(dialog).getByRole('button', { name: 'Seguir editando' }));
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull());
    expect(screen.getByLabelText('Nombre')).toHaveProperty('value', 'Machacado');
    fireEvent.click(screen.getByRole('button', { name: 'Volver al listado' }));
    fireEvent.click(await screen.findByRole('button', { name: 'Descartar cambios' }));
    await waitFor(() =>
      expect(screen.queryByRole('button', { name: 'Volver al listado' })).toBeNull(),
    );
    expect(screen.getByText('Aún no hay registros')).toBeTruthy();
  });

  it('1.2 sin cambios vuelve al listado de inmediato', async () => {
    await openNewProduct();
    fireEvent.click(screen.getByRole('button', { name: 'Volver al listado' }));
    await waitFor(() =>
      expect(screen.queryByRole('button', { name: 'Volver al listado' })).toBeNull(),
    );
    expect(screen.queryByRole('alertdialog')).toBeNull();
  });

  it('1.3 el precio base descarta letras al teclear y conserva el valor válido', async () => {
    await openNewProduct();
    const price = screen.getByLabelText('Precio base anunciado') as HTMLInputElement;
    expect(price.type).toBe('text');
    expect(price.inputMode).toBe('decimal');
    fireEvent.change(price, { target: { value: '95' } });
    fireEvent.change(price, { target: { value: '95a' } });
    expect(price.value).toBe('95');
    fireEvent.change(price, { target: { value: '95.5' } });
    fireEvent.change(price, { target: { value: '95.505' } });
    expect(price.value).toBe('95.5');
    fireEvent.change(price, { target: { value: '95,50' } });
    expect(price.value).toBe('95.50');
  });

  it('1.3 cantidades con tres decimales y enteros solo con dígitos', () => {
    function Fields() {
      const [quantity, setQuantity] = useState('1');
      const [order, setOrder] = useState(4);
      return (
        <>
          <DecimalField quantity label="Cantidad" value={quantity} onChange={setQuantity} />
          <IntegerField label="Capacidad" value={order} max={100} onChange={setOrder} />
          <output>{order}</output>
        </>
      );
    }
    render(<Fields />);
    const quantity = screen.getByLabelText('Cantidad') as HTMLInputElement;
    fireEvent.change(quantity, { target: { value: '1.250' } });
    fireEvent.change(quantity, { target: { value: '1.2505' } });
    fireEvent.change(quantity, { target: { value: '1e3' } });
    expect(quantity.value).toBe('1.250');
    const capacity = screen.getByLabelText('Capacidad') as HTMLInputElement;
    expect(capacity.type).toBe('text');
    expect(capacity.inputMode).toBe('numeric');
    fireEvent.change(capacity, { target: { value: '4x' } });
    fireEvent.change(capacity, { target: { value: '4.5' } });
    fireEvent.change(capacity, { target: { value: '-4' } });
    expect(capacity.value).toBe('4');
    fireEvent.change(capacity, { target: { value: '12' } });
    expect(screen.getByText('12')).toBeTruthy();
    fireEvent.change(capacity, { target: { value: '1200' } });
    expect(capacity.value).toBe('12');
  });

  it('1.4 un componente no exige «Productos permitidos» y el original lleva sus tramos', () => {
    let latest: ComboSlot[] = [];
    function Editor() {
      const [slots, setSlots] = useState<ComboSlot[]>([]);
      latest = slots;
      return <ComboEditor value={slots} onChange={setSlots} products={[burger, fries]} />;
    }
    render(<Editor />);
    fireEvent.click(screen.getByRole('button', { name: 'Agregar' }));
    const component = screen.getByRole('group', { name: 'Componente 1' });
    expect(within(component).queryByLabelText('Producto permitido')).toBeNull();
    fireEvent.change(within(component).getByLabelText('Producto original'), {
      target: { value: 'burger' },
    });
    expect(latest[0]?.default_product_id).toBe('burger');
    expect(latest[0]?.options).toEqual([
      { product_id: 'burger', substitution_price: '0.00', tiers: [] },
    ]);
    // Los tramos del original se configuran sin agregar una alternativa.
    expect(within(component).getByRole('heading', { name: 'Tramos incrementales' })).toBeTruthy();
    expect(within(component).queryByRole('group', { name: /Alternativa/ })).toBeNull();
  });

  it('1.5 los selectores de producto inician vacíos y el combo no se ofrece a sí mismo', () => {
    const combo = {
      ...newCatalogDocument('products', true, category.id),
      id: 'combo-self',
      revision: 1,
      name: 'Combo del día',
    } as Product;
    function Editor() {
      const [value, setValue] = useState(combo);
      return <ProductEditor value={value} onChange={setValue} lookups={lookups} can={() => true} />;
    }
    render(<Editor />);
    const components = screen
      .getAllByRole('heading', { name: 'Componentes del combo' })[0]!
      .closest('section')!;
    fireEvent.click(within(components).getAllByRole('button', { name: 'Agregar' })[0]!);
    const original = screen.getByLabelText('Producto original') as HTMLSelectElement;
    expect(original.value).toBe('');
    expect(original.selectedOptions[0]?.textContent).toBe('Selecciona…');
    const names = Array.from(original.options).map((option) => option.textContent);
    expect(names).not.toContain('Combo del día');
    fireEvent.click(
      within(screen.getByRole('group', { name: 'Componente 1' })).getAllByRole('button', {
        name: 'Agregar',
      })[0]!,
    );
    const allowed = screen.getByLabelText('Producto permitido') as HTMLSelectElement;
    expect(allowed.value).toBe('');
  });

  it('1.5 un producto nuevo no hereda la primera categoría sin elegirla', async () => {
    await openNewProduct();
    const select = screen.getByLabelText('Categoría') as HTMLSelectElement;
    expect(select.value).toBe('');
    expect(select.selectedOptions[0]?.textContent).toBe('Selecciona…');
  });

  it('1.6 cada tramo muestra sus campos y los botones indican qué quitan', () => {
    function Editor() {
      const [tiers, setTiers] = useState<QuantityTier[]>([]);
      return <TierEditor value={tiers} onChange={setTiers} />;
    }
    render(<Editor />);
    fireEvent.click(screen.getByRole('button', { name: 'Agregar' }));
    expect(screen.getByLabelText('Desde la unidad')).toBeTruthy();
    expect(screen.getByLabelText('Cargo por unidad')).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Quitar tramo' })).toBeTruthy();
    expect(screen.queryByRole('button', { name: 'Quitar fila' })).toBeNull();
  });

  it('1.7 la imagen se elige con controles en español', () => {
    render(<ImageEditor id={null} onChange={() => {}} />);
    expect(screen.getByText('Sin imagen')).toBeTruthy();
    const input = screen.getByLabelText('Elegir imagen') as HTMLInputElement;
    expect(input.type).toBe('file');
    expect(input.className).toContain('visually-hidden');
    expect(screen.queryByRole('button', { name: 'Quitar imagen' })).toBeNull();
  });

  it('1.8 sin restricciones las fechas quedan vacías y solo aceptan dígitos', () => {
    let latest: Schedule = emptySchedule();
    function Editor() {
      const [schedule, setSchedule] = useState<Schedule>(emptySchedule());
      latest = schedule;
      return <ScheduleEditor value={schedule} onChange={setSchedule} />;
    }
    render(<Editor />);
    fireEvent.click(screen.getByRole('button', { name: /Restricciones de horario/ }));
    const from = screen.getByLabelText('Desde la fecha') as HTMLInputElement;
    // El control nativo type="date" de WebKit mostraba la fecha de hoy con el valor vacío.
    expect(from.type).toBe('text');
    expect(from.value).toBe('');
    expect(from.placeholder).toBe('dd/mm/aaaa');
    fireEvent.change(from, { target: { value: 'hoy' } });
    expect(from.value).toBe('');
    fireEvent.change(from, { target: { value: '15112026' } });
    expect(from.value).toBe('15/11/2026');
    expect(latest.starts_on).toBe('2026-11-15');
    fireEvent.change(from, { target: { value: '31/02/2026' } });
    expect(latest.starts_on).toBe('');
    fireEvent.click(screen.getByRole('button', { name: 'Quitar fecha' }));
    expect(from.value).toBe('');
  });
});
