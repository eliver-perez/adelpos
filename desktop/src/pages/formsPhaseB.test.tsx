// @vitest-environment jsdom

// Etapa 6.1 · fase B: alta continua, posiciones, código sugerido, vista previa dentro del
// catálogo y configuración general.

import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { useState } from 'react';
import { OperationsAdmin, nextTableName } from './restaurant/OperationsAdmin';
import type { ExecuteMutation } from './restaurant/OperationsAdmin';
import { CatalogPage } from './catalog/CatalogPage';
import { CatalogEditor } from './catalog/CatalogEditor';
import { newCatalogDocument } from './catalog/catalogState';
import { SettingsFields, initialSettings } from './SettingsPage';
import { sections } from '../navigation/sections';
import { codeFromName, suggestProductCode } from '../services/productCode';
import { mexicoTimezones, timezoneOptions } from '../services/timezones';
import type { Category, CatalogDocument, Lookups, Product } from '../services/catalog';
import type { Area, Overview, RestaurantTable } from '../services/restaurant';
import type { Settings } from '../services/models';
import { chooseOption, openSelect, optionLabels, selectText } from '../testing/select';

const native = vi.hoisted(() => ({ invoke: vi.fn() }));
vi.mock('@tauri-apps/api/core', () => ({ invoke: native.invoke, isTauri: () => true }));

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

function area(id: string, name: string, sort_order: number): Area {
  return {
    id,
    name,
    description: '',
    sort_order,
    active: true,
    revision: 1,
    table_count: 0,
  };
}

function table(id: string, area_id: string, name: string): RestaurantTable {
  return {
    id,
    area_id,
    name,
    shape: 'rectangle',
    x: 20,
    y: 20,
    width: 100,
    height: 72,
    seats: 4,
    active: true,
    out_of_service: false,
    revision: 1,
    account_count: 0,
    state: 'free',
    occupied_since: null,
    occupied_seconds: 0,
    balance: '0.00',
  };
}

const overview: Overview = {
  areas: [area('salon', 'Salón', 10), area('terraza', 'Terraza', 20)],
  tables: [table('mesa-5', 'terraza', 'Mesa 5')],
  settings: { warning_minutes: 15, overdue_minutes: 30, revision: 1 },
  timezone: 'America/Monterrey',
};

describe('alta continua de mesas', () => {
  it('propone el siguiente nombre sin repetir los del área', () => {
    expect(nextTableName('Mesa 4', ['Mesa 4'])).toBe('Mesa 5');
    expect(nextTableName('Mesa 4', ['Mesa 4', 'mesa 5'])).toBe('Mesa 6');
    expect(nextTableName('M09', ['M09'])).toBe('M10');
    expect(nextTableName('Barra', ['Barra'])).toBe('');
  });

  it('«Guardar y crear otra» guarda y deja la siguiente mesa lista en la misma área', async () => {
    let created = 0;
    const execute = vi.fn(
      async (_operation: string, options: { body?: Record<string, unknown> }) => {
        created++;
        return { ...options.body, id: `nueva-${created}`, revision: 1 };
      },
    );
    render(
      <OperationsAdmin
        overview={overview}
        can={() => true}
        execute={execute as unknown as ExecuteMutation}
        busy={false}
      />,
    );
    fireEvent.click(screen.getByRole('button', { name: 'Crear mesa' }));
    chooseOption(screen.getByLabelText('Área de la mesa'), 'Terraza');
    fireEvent.change(screen.getByLabelText('Nombre o número de mesa'), {
      target: { value: 'Mesa 4' },
    });
    fireEvent.click(screen.getByRole('button', { name: 'Guardar y crear otra' }));
    await waitFor(() => expect(execute).toHaveBeenCalledTimes(1));
    expect(execute).toHaveBeenCalledWith(
      'restaurant_create_table',
      expect.objectContaining({
        body: expect.objectContaining({ name: 'Mesa 4', area_id: 'terraza' }),
      }),
    );
    const name = screen.getByLabelText('Nombre o número de mesa') as HTMLInputElement;
    // «Mesa 5» ya existe en Terraza: se propone la siguiente libre.
    await waitFor(() => expect(name.value).toBe('Mesa 6'));
    expect((screen.getByLabelText('Área de la mesa') as HTMLButtonElement).value).toBe('terraza');
    expect(screen.getByRole('button', { name: /Nueva mesa/ })).toBeTruthy();
    await waitFor(() => expect(document.activeElement).toBe(name));
    // La acción principal sigue visible en la barra fija del formulario.
    expect(screen.getByRole('button', { name: 'Guardar mesa' }).parentElement?.className).toBe(
      'form-actions',
    );
    fireEvent.click(screen.getByRole('button', { name: 'Guardar y crear otra' }));
    await waitFor(() => expect(execute).toHaveBeenCalledTimes(2));
    expect(execute).toHaveBeenLastCalledWith(
      'restaurant_create_table',
      expect.objectContaining({
        body: expect.objectContaining({ name: 'Mesa 6', area_id: 'terraza', id: '' }),
      }),
    );
    await waitFor(() => expect(name.value).toBe('Mesa 7'));
  });

  it('el área nueva se coloca por posición y no muestra un número de orden', async () => {
    const execute = vi.fn(
      async (_operation: string, options: { body?: Record<string, unknown> }) => ({
        ...options.body,
        id: 'patio',
        revision: 1,
      }),
    );
    render(
      <OperationsAdmin
        overview={overview}
        can={() => true}
        execute={execute as unknown as ExecuteMutation}
        busy={false}
      />,
    );
    fireEvent.click(screen.getByRole('button', { name: 'Crear área' }));
    expect(screen.queryByLabelText('Orden del área')).toBeNull();
    const position = screen.getByLabelText('Posición del área') as HTMLButtonElement;
    expect(position.value).toBe('terraza');
    chooseOption(position, 'Al inicio');
    fireEvent.change(screen.getByLabelText('Nombre del área'), { target: { value: 'Patio' } });
    fireEvent.click(screen.getByRole('button', { name: 'Guardar área' }));
    await waitFor(() =>
      expect(execute).toHaveBeenCalledWith(
        'restaurant_create_area',
        expect.objectContaining({
          body: expect.objectContaining({ name: 'Patio', position: { after: '' } }),
        }),
      ),
    );
  });
});

const palette = [
  { hex: '#d64545', name: 'Rojo' },
  { hex: '#3f9a4a', name: 'Verde' },
];

function category(id: string, name: string, sort_order: number): Category {
  return {
    ...(newCatalogDocument('categories', false, '') as Category),
    id,
    name,
    sort_order,
    revision: 1,
  };
}

const lookups: Lookups = {
  categories: [category('bebidas', 'Bebidas', 20), category('desayunos', 'Desayunos', 10)],
  ingredients: [],
  complements: [],
  portions: [],
  groups: [],
  'price-lists': [],
  products: [
    {
      id: 'machacado',
      name: 'Machacado natural',
      code: 'machacado-natural',
      kind: 'product',
      description: '',
      active: true,
      archived: false,
      revision: 1,
      category_id: 'desayunos',
      sort_order: 10,
      variants: [],
    },
  ],
  palette,
};

describe('código interno sugerido', () => {
  it('deriva el código del nombre con las reglas de formato', () => {
    expect(codeFromName('Machacado Natural')).toBe('machacado-natural');
    expect(codeFromName('  Café de olla (grande)  ')).toBe('cafe-de-olla-grande');
    expect(codeFromName('Piña--colada ¡¡fría!!')).toBe('pina-colada-fria');
    expect(codeFromName('Ñandú & jalapeño')).toBe('nandu-jalapeno');
    expect(codeFromName('***')).toBe('');
    const long = codeFromName('a'.repeat(59) + ' bcd');
    expect(long.length).toBeLessThanOrEqual(60);
    expect(long.endsWith('-')).toBe(false);
  });

  it('propone un código libre sin distinguir mayúsculas', () => {
    expect(suggestProductCode('Machacado Natural', ['MACHACADO-NATURAL'])).toBe(
      'machacado-natural-2',
    );
    expect(
      suggestProductCode('Machacado Natural', ['machacado-natural', 'machacado-natural-2']),
    ).toBe('machacado-natural-3');
    const suffixed = suggestProductCode('x'.repeat(70), ['x'.repeat(60)]);
    expect(suffixed).toBe(`${'x'.repeat(58)}-2`);
  });

  function Editor({ initial }: { initial: Product }) {
    const [value, setValue] = useState<CatalogDocument>(initial);
    return (
      <CatalogEditor
        resource="products"
        value={value}
        onChange={setValue}
        lookups={lookups}
        can={() => true}
        onSave={async () => {}}
        editable
      />
    );
  }

  it('llena el código mientras no se edite a mano', () => {
    render(<Editor initial={newCatalogDocument('products', false, 'desayunos') as Product} />);
    const name = screen.getByLabelText('Nombre') as HTMLInputElement;
    const code = screen.getByLabelText(/^Código interno único/) as HTMLInputElement;
    fireEvent.change(name, { target: { value: 'Machacado Natural' } });
    expect(code.value).toBe('machacado-natural-2');
    fireEvent.change(name, { target: { value: 'Café de olla' } });
    expect(code.value).toBe('cafe-de-olla');
    fireEvent.change(code, { target: { value: 'CAFE-ESPECIAL' } });
    fireEvent.change(name, { target: { value: 'Café de olla grande' } });
    expect(code.value).toBe('CAFE-ESPECIAL');
  });

  it('al editar un producto existente nunca cambia su código', () => {
    const existing = {
      ...(newCatalogDocument('products', false, 'desayunos') as Product),
      id: 'machacado',
      name: 'Machacado natural',
      code: 'MACH-01',
      revision: 3,
    };
    render(<Editor initial={existing} />);
    fireEvent.change(screen.getByLabelText('Nombre'), { target: { value: 'Machacado con huevo' } });
    expect((screen.getByLabelText(/^Código interno único/) as HTMLInputElement).value).toBe(
      'MACH-01',
    );
  });
});

describe('catálogo: posiciones, alta continua y vista previa', () => {
  beforeEach(() => {
    let created = 0;
    native.invoke.mockImplementation(
      async (_command: string, args: { operation: string; body?: Record<string, unknown> }) => {
        if (args.operation === 'catalog_lookups') {
          return { status: 200, body: lookups };
        }
        if (args.operation.startsWith('catalog_create_')) {
          created++;
          return { status: 200, body: { ...args.body, id: `nuevo-${created}`, revision: 1 } };
        }
        return { status: 200, body: { items: [], total: 0, page: 1, page_size: 20 } };
      },
    );
  });

  function requests(operation: string) {
    return native.invoke.mock.calls
      .map((call) => call[1] as { operation: string; body?: Record<string, unknown> })
      .filter((request) => request.operation === operation);
  }

  it('la categoría elige posición y color de la paleta, y «Guardar y crear otra» abre otra', async () => {
    render(<CatalogPage can={() => true} />);
    await screen.findByText('Aún no hay registros');
    fireEvent.click(screen.getByRole('tab', { name: 'Categorías' }));
    await waitFor(() =>
      expect(screen.getByRole('tab', { name: 'Categorías' }).getAttribute('aria-selected')).toBe(
        'true',
      ),
    );
    fireEvent.click(screen.getByRole('button', { name: 'Crear registro' }));
    const name = (await screen.findByLabelText('Nombre')) as HTMLInputElement;
    expect(screen.queryByLabelText('Orden visual')).toBeNull();
    const position = screen.getByLabelText('Posición de la categoría') as HTMLButtonElement;
    // Orden de Go por sort_order: Desayunos (10) y luego Bebidas (20).
    expect(optionLabels(position)).toEqual([
      'Al inicio',
      'Después de «Desayunos»',
      'Después de «Bebidas»',
    ]);
    expect(position.value).toBe('bebidas');
    chooseOption(position, 'Después de «Desayunos»');
    fireEvent.click(screen.getByRole('button', { name: 'Color de la categoría: Sin color' }));
    fireEvent.click(
      within(screen.getByRole('group', { name: 'Paleta de colores' })).getByRole('button', {
        name: 'Rojo',
      }),
    );
    fireEvent.change(name, { target: { value: 'Comidas' } });
    fireEvent.click(screen.getByRole('button', { name: 'Guardar y crear otra' }));
    await waitFor(() => expect(requests('catalog_create_categories')).toHaveLength(1));
    expect(requests('catalog_create_categories')[0]?.body).toEqual(
      expect.objectContaining({
        name: 'Comidas',
        color: '#d64545',
        position: { after: 'desayunos' },
      }),
    );
    // El formulario confirma el alta continua con un aviso del sistema único de mensajes.
    await screen.findByText('«Comidas» quedó guardado. Captura el siguiente registro.');
    const next = screen.getByLabelText('Nombre') as HTMLInputElement;
    expect(next.value).toBe('');
    await waitFor(() => expect(document.activeElement).toBe(next));
    expect(screen.getByRole('button', { name: 'Color de la categoría: Sin color' })).toBeTruthy();
    // Sin cambios en el registro nuevo, «Volver al listado» no pregunta.
    fireEvent.click(screen.getByRole('button', { name: 'Volver al listado' }));
    await waitFor(() => expect(screen.queryByLabelText('Nombre')).toBeNull());
  });

  it('el producto conserva la categoría al crear otro y se coloca después del último', async () => {
    render(<CatalogPage can={() => true} />);
    await screen.findByText('Aún no hay registros');
    fireEvent.click(screen.getByRole('button', { name: 'Crear registro' }));
    await screen.findByLabelText('Nombre');
    chooseOption(screen.getByLabelText('Categoría'), 'Desayunos');
    const position = screen.getByLabelText('Posición en su categoría') as HTMLButtonElement;
    expect(position.value).toBe('machacado');
    fireEvent.change(screen.getByLabelText('Nombre'), { target: { value: 'Chilaquiles' } });
    fireEvent.click(screen.getByRole('button', { name: 'Guardar y crear otro' }));
    await waitFor(() => expect(requests('catalog_create_products')).toHaveLength(1));
    expect(requests('catalog_create_products')[0]?.body).toEqual(
      expect.objectContaining({
        name: 'Chilaquiles',
        code: 'chilaquiles',
        category_id: 'desayunos',
        position: { after: 'machacado' },
      }),
    );
    await screen.findByText('«Chilaquiles» quedó guardado. Captura el siguiente registro.');
    expect((screen.getByLabelText('Categoría') as HTMLButtonElement).value).toBe('desayunos');
    const name = screen.getByLabelText('Nombre') as HTMLInputElement;
    expect(name.value).toBe('');
    await waitFor(() => expect(document.activeElement).toBe(name));
    // El editor nuevo vuelve a sugerir el código a partir del nombre.
    fireEvent.change(name, { target: { value: 'Huevos rancheros' } });
    expect((screen.getByLabelText(/^Código interno único/) as HTMLInputElement).value).toBe(
      'huevos-rancheros',
    );
  });

  it('«Vista previa» vive en Catálogo solo con permisos de catálogo y precios', async () => {
    const { unmount } = render(<CatalogPage can={() => true} />);
    await screen.findByText('Aún no hay registros');
    fireEvent.click(screen.getByRole('tab', { name: 'Vista previa' }));
    expect(await screen.findByLabelText('Producto a comprobar')).toBeTruthy();
    expect(screen.getByRole('tab', { name: 'Vista previa' }).getAttribute('aria-selected')).toBe(
      'true',
    );
    expect(screen.queryByRole('button', { name: 'Crear registro' })).toBeNull();
    fireEvent.click(screen.getByRole('tab', { name: 'Productos' }));
    await screen.findByText('Aún no hay registros');
    unmount();
    render(<CatalogPage can={(permission) => permission !== 'catalog.prices.read'} />);
    await screen.findByText('Aún no hay registros');
    expect(screen.queryByRole('tab', { name: 'Vista previa' })).toBeNull();
  });
});

describe('«Probar personalización» fuera del menú principal', () => {
  // Desde la etapa 9C la pantalla de inicio es de la terminal; Go sigue interpretando la
  // preferencia heredada «preview» como Catálogo (security_test.go), pero ya no decide el inicio.
  it('no es una sección del registro de navegación', () => {
    expect(sections.some((entry) => (entry.id as string) === 'preview')).toBe(false);
    expect(sections.some((entry) => entry.label === 'Probar personalización')).toBe(false);
  });
});

describe('configuración general', () => {
  function Fields() {
    const [value, setValue] = useState<Settings>({
      ...initialSettings,
      name: 'ADEL POS',
      timezone: 'America/Monterrey',
    });
    return (
      <>
        <SettingsFields value={value} onChange={setValue} />
        <output>{value.timezone}</output>
      </>
    );
  }

  it('ofrece primero las zonas de México, sin repetidos y conservando la guardada', () => {
    const zones = timezoneOptions('America/Monterrey');
    expect(zones.slice(0, mexicoTimezones.length)).toEqual(mexicoTimezones);
    expect(new Set(zones).size).toBe(zones.length);
    expect(zones).toContain('Europe/Madrid');
    expect(timezoneOptions('Etc/Zona_Guardada')[0]).toBe('Etc/Zona_Guardada');
  });

  it('elige la zona con búsqueda, sin escritura libre, y muestra la moneda como dato', () => {
    render(<Fields />);
    const zone = screen.getByLabelText(/^Zona horaria/) as HTMLButtonElement;
    expect(zone.getAttribute('role')).toBe('combobox');
    expect(zone.value).toBe('America/Monterrey');
    expect(optionLabels(zone).slice(0, 2)).toEqual(['America/Monterrey', 'America/Mexico City']);
    // Búsqueda integrada (sin el campo aparte de antes): México primero, en su propio grupo.
    const listbox = openSelect(zone);
    expect(within(listbox).getByRole('group', { name: 'México' })).toBeTruthy();
    const search = screen.getByRole('searchbox', { name: 'Buscar zona horaria del restaurante' });
    fireEvent.change(search, { target: { value: 'tijuana' } });
    expect(
      within(listbox)
        .getAllByRole('option')
        .map((option) => option.textContent),
    ).toEqual(['America/Tijuana']);
    // La zona guardada sigue a la vista en el selector aunque la búsqueda no la incluya.
    expect(selectText(zone)).toBe('America/Monterrey');
    fireEvent.click(within(listbox).getByRole('option', { name: 'America/Tijuana' }));
    expect(screen.getByText('America/Tijuana', { selector: 'output' })).toBeTruthy();
    expect(screen.queryByRole('textbox', { name: /Moneda/ })).toBeNull();
    expect(screen.getByText('MXN · Peso mexicano')).toBeTruthy();
    const address = screen.getByLabelText('Dirección (opcional)') as HTMLTextAreaElement;
    expect(address.className).toContain('auto-grow');
  });
});
