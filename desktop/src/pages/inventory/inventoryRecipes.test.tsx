// @vitest-environment jsdom

import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { PendingOperationsProvider } from '../../hooks/pendingOperations';
import { InventoryPage } from './InventoryPage';
import { allInventoryPermissions, ids, inventoryTransport } from './inventoryTestData';
import type { RecordedRequest, Responder } from './inventoryTestData';
import {
  burgerRecipe,
  burgerVersionOne,
  onionPortion,
  portionPage,
  preparationPage,
  production,
  recipeIDs,
  recipeItemPage,
  recipeUnits,
  sauceRecipe,
  storedRiceRecipe,
  targetPage,
} from './inventoryRecipesTestData';

const native = vi.hoisted(() => ({ invoke: vi.fn() }));
vi.mock('@tauri-apps/api/core', () => ({ invoke: native.invoke, isTauri: () => true }));

afterEach(() => {
  cleanup();
  native.invoke.mockReset();
});

// Respuestas de la fase B comunes a estas pruebas; cada prueba antepone las suyas.
function recipeResponder(override: Responder = () => undefined): Responder {
  return (request) => {
    const custom = override(request);
    if (custom) {
      return custom;
    }
    switch (request.operation) {
      case 'inventory_units':
        return { status: 200, body: recipeUnits };
      case 'inventory_items':
        return { status: 200, body: recipeItemPage() };
      case 'inventory_recipe_targets':
        return { status: 200, body: targetPage() };
      case 'inventory_recipes':
        return { status: 200, body: preparationPage() };
      case 'inventory_ingredient_portions':
        return { status: 200, body: portionPage() };
      case 'inventory_recipe':
        if (request.resourceId === recipeIDs.sauceRecipe) {
          return { status: 200, body: sauceRecipe() };
        }
        if (request.resourceId === recipeIDs.storedRiceRecipe) {
          return { status: 200, body: storedRiceRecipe() };
        }
        return { status: 200, body: burgerRecipe() };
      case 'inventory_recipe_version':
        return { status: 200, body: burgerVersionOne() };
      default:
        return undefined;
    }
  };
}

function useTransport(override?: Responder) {
  const transport = inventoryTransport(recipeResponder(override));
  native.invoke.mockImplementation(transport.handler);
  return transport.requests;
}

function renderInventory(permissions: string[]) {
  return render(
    <PendingOperationsProvider>
      <InventoryPage
        can={(permission) => permissions.includes(permission)}
        timezone="America/Monterrey"
        onOpenSettings={() => undefined}
      />
    </PendingOperationsProvider>,
  );
}

function writes(requests: RecordedRequest[], operation: string) {
  return requests.filter((request) => request.operation === operation);
}

function lastQuery(requests: RecordedRequest[], operation: string) {
  return requests.filter((request) => request.operation === operation).at(-1)?.query ?? null;
}

async function openRecipes(permissions = allInventoryPermissions) {
  renderInventory(permissions);
  fireEvent.click(await screen.findByRole('tab', { name: 'Recetas' }));
}

function optionLabels(select: HTMLElement) {
  return Array.from((select as HTMLSelectElement).options).map((option) => option.textContent);
}

describe('destinos de receta', () => {
  it('lista productos, variantes y complementos y filtra los que no tienen receta', async () => {
    const requests = useTransport();
    await openRecipes();
    const table = await screen.findByRole('table');
    expect(within(table).getAllByText('Filete de prueba')).toHaveLength(1);
    expect(within(table).getByText('Hamburguesa de prueba · Doble')).toBeTruthy();
    expect(within(table).getByText('Usa la receta del producto')).toBeTruthy();
    expect(within(table).getAllByText('1 de 2 ingredientes con porción')).toHaveLength(2);
    expect(within(table).getByText('Vinculado al ingrediente «Arroz»')).toBeTruthy();
    expect(within(table).getAllByText('Sin receta').length).toBeGreaterThan(0);
    expect(
      screen.getByRole('button', { name: 'Configurar la receta de Filete de prueba' }),
    ).toBeTruthy();
    expect(
      screen.getByRole('button', { name: 'Editar la receta de Hamburguesa de prueba' }),
    ).toBeTruthy();
    // Por omisión solo los activos del catálogo.
    expect(lastQuery(requests, 'inventory_recipe_targets')).toMatchObject({ active: 'true' });
    fireEvent.change(screen.getByLabelText('Estado de la receta'), {
      target: { value: 'without_recipe' },
    });
    await waitFor(() =>
      expect(lastQuery(requests, 'inventory_recipe_targets')).toMatchObject({
        state: 'without_recipe',
        page: '1',
      }),
    );
  });

  it('sin permiso de recetas solo permite consultarlas', async () => {
    useTransport();
    await openRecipes(['inventory.read']);
    await screen.findByRole('table');
    expect(screen.queryByRole('button', { name: /^Configurar la receta/ })).toBeNull();
    fireEvent.click(screen.getByRole('button', { name: 'Ver la receta de Hamburguesa de prueba' }));
    await screen.findByText(/Editar esta receta requiere el permiso «Administrar recetas/);
    const save = await screen.findByRole('button', { name: 'Guardar como versión 3' });
    expect((save as HTMLButtonElement).disabled).toBe(true);
  });
});

describe('editor de recetas', () => {
  it('crea la versión 1 con renglones de los tres tipos, unidades filtradas y «solo para llevar»', async () => {
    const created = burgerRecipe(true, {
      id: recipeIDs.burgerRecipe,
      name: 'Filete de prueba',
      revision: 1,
    });
    const requests = useTransport((request) => {
      if (request.operation === 'inventory_create_recipe') {
        return { status: 200, body: created };
      }
      if (
        request.operation === 'inventory_recipe' &&
        writes(requests, 'inventory_create_recipe').length > 0
      ) {
        return { status: 200, body: created };
      }
      return undefined;
    });
    await openRecipes();
    fireEvent.click(
      await screen.findByRole('button', { name: 'Configurar la receta de Filete de prueba' }),
    );
    await screen.findByRole('heading', { name: 'Filete de prueba' });
    expect(screen.getByText(/Al guardarla se crea la versión 1/)).toBeTruthy();

    // Renglón 1: insumo en kilogramos capturado en gramos; solo se ofrecen unidades de masa.
    fireEvent.click(screen.getByRole('button', { name: 'Agregar insumo' }));
    const first = screen.getByRole('group', { name: 'Renglón 1' });
    await waitFor(() =>
      expect(optionLabels(within(first).getByLabelText('Insumo'))).toContain(
        'Carne molida · CAR-01',
      ),
    );
    fireEvent.change(within(first).getByLabelText('Insumo'), { target: { value: recipeIDs.meat } });
    const firstUnit = within(first).getByLabelText(/^Unidad/);
    expect(optionLabels(firstUnit)).toEqual(['Gramo (g)', 'Kilogramo (kg)']);
    expect((firstUnit as HTMLSelectElement).value).toBe(ids.kilogram);
    fireEvent.change(firstUnit, { target: { value: ids.gram } });
    fireEvent.change(within(first).getByLabelText(/^Cantidad/), { target: { value: '150x' } });
    fireEvent.change(within(first).getByLabelText(/^Cantidad/), { target: { value: '150' } });

    // Renglón 2: preparación medida en volumen.
    fireEvent.click(screen.getByRole('button', { name: 'Agregar preparación' }));
    const second = screen.getByRole('group', { name: 'Renglón 2' });
    await waitFor(() =>
      expect(optionLabels(within(second).getByLabelText('Preparación'))).toContain(
        'Salsa de la casa · Elaboración inmediata',
      ),
    );
    fireEvent.change(within(second).getByLabelText('Preparación'), {
      target: { value: recipeIDs.sauceRecipe },
    });
    expect(optionLabels(within(second).getByLabelText(/^Unidad/))).toEqual([
      'Litro (l)',
      'Mililitro (ml)',
    ]);
    fireEvent.change(within(second).getByLabelText(/^Cantidad/), { target: { value: '20' } });

    // Renglón 3: ingrediente del catálogo con su porción para este producto (sin «solo para llevar»).
    fireEvent.click(screen.getByRole('button', { name: 'Agregar ingrediente del catálogo' }));
    const third = screen.getByRole('group', { name: 'Renglón 3' });
    await waitFor(() =>
      expect(optionLabels(within(third).getByLabelText('Ingrediente del catálogo'))).toContain(
        'Cebolla',
      ),
    );
    fireEvent.change(within(third).getByLabelText('Ingrediente del catálogo'), {
      target: { value: recipeIDs.onionIngredient },
    });
    expect(within(third).queryByRole('button', { name: 'Solo para llevar' })).toBeNull();
    await waitFor(() =>
      expect(optionLabels(within(third).getByLabelText('Insumo'))).toContain(
        'Cebolla blanca · CEB-01',
      ),
    );
    fireEvent.change(within(third).getByLabelText('Insumo'), {
      target: { value: recipeIDs.onion },
    });
    fireEvent.change(within(third).getByLabelText(/^Cantidad/), { target: { value: '20' } });

    // Renglón 4: bolsa solo para llevar (piezas).
    fireEvent.click(screen.getByRole('button', { name: 'Agregar insumo' }));
    const fourth = screen.getByRole('group', { name: 'Renglón 4' });
    await waitFor(() =>
      expect(optionLabels(within(fourth).getByLabelText('Insumo'))).toContain(
        'Bolsa para llevar · BOL-01',
      ),
    );
    fireEvent.change(within(fourth).getByLabelText('Insumo'), { target: { value: recipeIDs.bag } });
    expect(optionLabels(within(fourth).getByLabelText(/^Unidad/))).toEqual(['Pieza (pza)']);
    fireEvent.change(within(fourth).getByLabelText(/^Cantidad/), { target: { value: '1' } });
    fireEvent.click(within(fourth).getByRole('button', { name: 'Solo para llevar' }));

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'Crear receta (versión 1)' }));
    });
    const [create] = writes(requests, 'inventory_create_recipe');
    expect(create?.body).toEqual({
      operation_id: expect.any(String),
      owner_kind: 'product',
      product_id: recipeIDs.steak,
      variant_id: null,
      name: '',
      active: true,
      yield_quantity: '1',
      notes: '',
      lines: [
        {
          component_kind: 'item',
          item_id: recipeIDs.meat,
          quantity: '150',
          unit_id: ids.gram,
          takeaway_only: false,
        },
        {
          component_kind: 'preparation',
          preparation_recipe_id: recipeIDs.sauceRecipe,
          quantity: '20',
          unit_id: recipeIDs.milliliter,
          takeaway_only: false,
        },
        {
          component_kind: 'ingredient',
          ingredient_id: recipeIDs.onionIngredient,
          item_id: recipeIDs.onion,
          quantity: '20',
          unit_id: ids.gram,
        },
        {
          component_kind: 'item',
          item_id: recipeIDs.bag,
          quantity: '1',
          unit_id: ids.piece,
          takeaway_only: true,
        },
      ],
    });
    await screen.findByText('Receta creada como versión 1.');
    // El panel continúa con la receta guardada: el siguiente guardado es otra versión.
    expect(await screen.findByRole('button', { name: 'Guardar como versión 2' })).toBeTruthy();
  });

  it('guardar crea una versión nueva, con costo teórico, ingredientes e historial', async () => {
    const saved = burgerRecipe(true, { revision: 3 });
    const requests = useTransport((request) =>
      request.operation === 'inventory_save_recipe' ? { status: 200, body: saved } : undefined,
    );
    await openRecipes();
    fireEvent.click(
      await screen.findByRole('button', { name: 'Editar la receta de Hamburguesa de prueba' }),
    );
    await screen.findByRole('heading', { name: 'Hamburguesa de prueba' });
    expect(screen.getByText(/Guardar crea la versión 3, vigente desde ese momento/)).toBeTruthy();
    // Costo teórico tal como lo entrega Go.
    const cost = screen.getByRole('region', { name: 'Costo teórico de la versión vigente' });
    expect(within(cost).getByText('$35.68')).toBeTruthy();
    expect(within(cost).getByText('Adicional para llevar')).toBeTruthy();
    // Ingredientes del catálogo con su porción.
    const ingredients = screen.getByRole('region', { name: 'Ingredientes del catálogo' });
    expect(within(ingredients).getByText('Renglón de esta receta')).toBeTruthy();
    expect(within(ingredients).getByText('Sin porción: no consume')).toBeTruthy();
    // La conversión guardada por Go se muestra mientras el renglón no cambie.
    const first = screen.getByRole('group', { name: 'Renglón 1' });
    expect(
      within(first).getByText(/En la unidad base: 0\.18 kg · costo del renglón \$32\.40/),
    ).toBeTruthy();
    expect(
      within(screen.getByRole('group', { name: 'Renglón 4' }))
        .getByRole('button', { name: 'Solo para llevar' })
        .getAttribute('aria-pressed'),
    ).toBe('true');
    fireEvent.change(within(first).getByLabelText(/^Cantidad/), { target: { value: '200' } });
    expect(within(first).queryByText(/En la unidad base/)).toBeNull();
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'Guardar como versión 3' }));
    });
    const [save] = writes(requests, 'inventory_save_recipe');
    expect(save?.resourceId).toBe(recipeIDs.burgerRecipe);
    expect(save?.body).toMatchObject({ revision: 2, active: true, yield_quantity: '1' });
    expect((save?.body?.lines as unknown[])[0]).toEqual({
      component_kind: 'item',
      item_id: recipeIDs.meat,
      quantity: '200',
      unit_id: ids.gram,
      takeaway_only: false,
    });
    expect(save?.body).not.toHaveProperty('product_id');
    await screen.findByText('Receta guardada como versión 3.');

    // Historial con la versión 1 consultada tal como se guardó (150 g).
    fireEvent.click(screen.getByRole('button', { name: /Historial de versiones/ }));
    fireEvent.click(screen.getByRole('button', { name: 'Ver la versión 1' }));
    const old = await screen.findByRole('region', { name: 'Versión 1 de la receta' });
    expect(within(old).getByText('150 g')).toBeTruthy();
    expect(within(old).getByText('0.15 kg')).toBeTruthy();
    expect(writes(requests, 'inventory_recipe_version')[0]?.resourceId).toBe(recipeIDs.versionOne);
  });

  it('oculta el costo teórico sin inventory.costs.read', async () => {
    useTransport((request) =>
      request.operation === 'inventory_recipe'
        ? { status: 200, body: burgerRecipe(false) }
        : undefined,
    );
    await openRecipes(['inventory.read', 'inventory.recipes.manage']);
    fireEvent.click(
      await screen.findByRole('button', { name: 'Editar la receta de Hamburguesa de prueba' }),
    );
    await screen.findByRole('heading', { name: 'Hamburguesa de prueba' });
    expect(
      screen.queryByRole('region', { name: 'Costo teórico de la versión vigente' }),
    ).toBeNull();
    expect(screen.queryByText(/costo del renglón/)).toBeNull();
    const ingredients = screen.getByRole('region', { name: 'Ingredientes del catálogo' });
    expect(within(ingredients).queryByRole('columnheader', { name: 'Costo' })).toBeNull();
    expect(screen.queryByText('$35.68')).toBeNull();
  });

  it('desactivar la receta se confirma con tono de peligro y crea una versión inactiva', async () => {
    const requests = useTransport((request) =>
      request.operation === 'inventory_save_recipe'
        ? { status: 200, body: burgerRecipe(true, { revision: 3, active: false }) }
        : undefined,
    );
    await openRecipes();
    fireEvent.click(
      await screen.findByRole('button', { name: 'Editar la receta de Hamburguesa de prueba' }),
    );
    const toggle = await screen.findByRole('button', { name: 'Receta activa' });
    fireEvent.click(toggle);
    const dialog = await screen.findByRole('alertdialog');
    expect(dialog.className).toContain('danger');
    expect(
      within(dialog).getByText(/la versión 3 deja «Hamburguesa de prueba» inactiva/),
    ).toBeTruthy();
    fireEvent.click(within(dialog).getByRole('button', { name: 'Desactivar' }));
    await waitFor(() => expect(toggle.getAttribute('aria-pressed')).toBe('false'));
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'Guardar como versión 3' }));
    });
    expect(writes(requests, 'inventory_save_recipe')[0]?.body).toMatchObject({
      revision: 2,
      active: false,
    });
  });

  it('muestra el mensaje de Go cuando la preparación formaría un ciclo', async () => {
    useTransport((request) =>
      request.operation === 'inventory_save_recipe'
        ? {
            status: 409,
            body: {
              error: {
                code: 'RECIPE_CYCLE',
                message:
                  'La receta formaría un ciclo: una de sus preparaciones (o el insumo que produce) la usa a ella.',
              },
            },
          }
        : undefined,
    );
    await openRecipes();
    fireEvent.click(screen.getByRole('button', { name: 'Preparaciones' }));
    const immediate = await screen.findByRole('region', { name: 'Elaboración inmediata' });
    expect(
      within(
        screen.getByRole('region', { name: 'Preparadas previamente (almacenables)' }),
      ).getByText('Arroz rojo'),
    ).toBeTruthy();
    fireEvent.click(
      within(immediate).getByRole('button', { name: 'Editar la preparación Salsa de la casa' }),
    );
    await screen.findByRole('heading', { name: 'Salsa de la casa' });
    // Las preparaciones no tienen «solo para llevar» y su rendimiento conserva la dimensión.
    expect(screen.queryByRole('button', { name: 'Solo para llevar' })).toBeNull();
    expect(optionLabels(screen.getByLabelText(/^Unidad del rendimiento/))).toEqual([
      'Litro (l)',
      'Mililitro (ml)',
    ]);
    expect(screen.getByText(/Rendimiento efectivo por lote/)).toBeTruthy();
    const uses = screen.getByRole('region', { name: 'Recetas que la usan' });
    expect(within(uses).getByText('Hamburguesa de prueba')).toBeTruthy();
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'Guardar como versión 2' }));
    });
    expect(
      await screen.findByText(
        'La receta formaría un ciclo: una de sus preparaciones (o el insumo que produce) la usa a ella.',
      ),
    ).toBeTruthy();
    expect(screen.queryByText('RECIPE_CYCLE')).toBeNull();
  });
});

describe('porciones habituales', () => {
  it('edita la porción de un ingrediente, la retira con confirmación y muestra el historial', async () => {
    const requests = useTransport((request) => {
      if (request.operation !== 'inventory_save_ingredient_portion') {
        return undefined;
      }
      if (request.body?.item_id === null) {
        return {
          status: 200,
          body: onionPortion({
            configured: false,
            item: null,
            quantity: null,
            unit: null,
            base_quantity: null,
            revision: 4,
          }),
        };
      }
      return {
        status: 200,
        body: onionPortion({ quantity: '25', base_quantity: '25', revision: 3 }),
      };
    });
    await openRecipes();
    fireEvent.click(screen.getByRole('button', { name: 'Porciones habituales' }));
    expect(await screen.findByText('20 g de Cebolla blanca')).toBeTruthy();
    expect(screen.getByText('Sin porción', { selector: '.tag' })).toBeTruthy();
    fireEvent.change(screen.getByLabelText('Porción'), { target: { value: 'false' } });
    await waitFor(() =>
      expect(lastQuery(requests, 'inventory_ingredient_portions')).toMatchObject({
        configured: 'false',
      }),
    );
    fireEvent.click(screen.getByRole('button', { name: 'Editar la porción de Cebolla' }));
    const panel = screen.getByRole('region', { name: 'Porción de Cebolla' });
    fireEvent.change(within(panel).getByLabelText(/^Cantidad por porción/), {
      target: { value: '25' },
    });
    await act(async () => {
      fireEvent.click(within(panel).getByRole('button', { name: 'Guardar porción' }));
    });
    const [save] = writes(requests, 'inventory_save_ingredient_portion');
    expect(save?.resourceId).toBe(recipeIDs.onionIngredient);
    expect(save?.body).toEqual({
      operation_id: expect.any(String),
      revision: 2,
      item_id: recipeIDs.onion,
      quantity: '25',
      unit_id: ids.gram,
    });
    await screen.findByText('Porción habitual guardada.');
    fireEvent.click(within(panel).getByRole('button', { name: /Historial de la porción/ }));
    expect(within(panel).getByText('15 g de Cebolla blanca')).toBeTruthy();
    fireEvent.click(within(panel).getByRole('button', { name: 'Quitar porción' }));
    const dialog = await screen.findByRole('alertdialog');
    expect(dialog.className).toContain('danger');
    await act(async () => {
      fireEvent.click(within(dialog).getByRole('button', { name: 'Quitar porción' }));
    });
    const removal = writes(requests, 'inventory_save_ingredient_portion')[1];
    expect(removal?.body).toEqual({ operation_id: expect.any(String), revision: 3, item_id: null });
    await screen.findByText('Porción de «Cebolla» retirada.');
  });
});

describe('elaboraciones', () => {
  it('registra una elaboración con confirmación y muestra el resultado de Go', async () => {
    const requests = useTransport((request) =>
      request.operation === 'inventory_record_production'
        ? { status: 200, body: production() }
        : undefined,
    );
    await openRecipes();
    fireEvent.click(screen.getByRole('button', { name: 'Elaboraciones' }));
    await screen.findByText('Aún no hay elaboraciones registradas.');
    fireEvent.click(screen.getByRole('button', { name: 'Registrar elaboración' }));
    const select = await screen.findByLabelText('Preparación elaborada');
    // Solo las almacenables activas se elaboran.
    await waitFor(() =>
      expect(optionLabels(select)).toEqual(['Elige la preparación', 'Arroz rojo']),
    );
    fireEvent.change(select, { target: { value: recipeIDs.storedRiceRecipe } });
    await screen.findByText(/cada lote rinde 2,400 g descontada la merma/);
    expect((screen.getByLabelText(/^Almacén de la elaboración/) as HTMLSelectElement).value).toBe(
      ids.general,
    );
    fireEvent.change(screen.getByLabelText(/^Lotes elaborados/), { target: { value: '2' } });
    fireEvent.change(screen.getByLabelText(/^Cantidad producida real/), {
      target: { value: '4750' },
    });
    fireEvent.click(screen.getByRole('button', { name: 'Registrar elaboración' }));
    const dialog = await screen.findByRole('alertdialog');
    expect(within(dialog).getByText(/«Arroz rojo» en «Almacén general» \(lotes: 2\)/)).toBeTruthy();
    await act(async () => {
      fireEvent.click(within(dialog).getByRole('button', { name: 'Registrar elaboración' }));
    });
    const [record] = writes(requests, 'inventory_record_production');
    expect(record?.body).toEqual({
      operation_id: expect.any(String),
      recipe_id: recipeIDs.storedRiceRecipe,
      warehouse_id: ids.general,
      batches: '2',
      produced_quantity: '4750',
    });
    const result = await screen.findByRole('region', { name: 'Elaboración registrada' });
    expect(within(result).getByText('4,800 g')).toBeTruthy();
    expect(within(result).getByText('4,750 g')).toBeTruthy();
    expect(within(result).getByText('250 g')).toBeTruthy();
    expect(within(result).getByText('−40 g')).toBeTruthy();
    expect(within(result).getByText('$56.80')).toBeTruthy();
    await screen.findByText('Elaboración registrada: 4,750 g de Arroz rojo preparado.');
    expect(
      screen.getByText('La existencia de «Cebolla blanca» en «Almacén general» quedó negativa.'),
    ).toBeTruthy();
  });

  it('sin permiso de elaborar solo se consultan (con movimientos) o no aparece el apartado', async () => {
    useTransport();
    const { unmount } = renderInventory(['inventory.read', 'inventory.movements.read']);
    fireEvent.click(await screen.findByRole('tab', { name: 'Recetas' }));
    fireEvent.click(screen.getByRole('button', { name: 'Elaboraciones' }));
    await screen.findByText(/Registrar elaboraciones requiere el permiso/);
    expect(screen.queryByRole('button', { name: 'Registrar elaboración' })).toBeNull();
    unmount();
    renderInventory(['inventory.read']);
    fireEvent.click(await screen.findByRole('tab', { name: 'Recetas' }));
    expect(screen.queryByRole('button', { name: 'Elaboraciones' })).toBeNull();
  });
});

describe('controles numéricos', () => {
  it('ningún campo de recetas, porciones ni elaboraciones usa type="number"', async () => {
    useTransport();
    await openRecipes();
    await screen.findByRole('table');
    expect(document.querySelectorAll('input[type="number"]')).toHaveLength(0);
    fireEvent.click(
      screen.getByRole('button', { name: 'Configurar la receta de Filete de prueba' }),
    );
    fireEvent.click(await screen.findByRole('button', { name: 'Agregar insumo' }));
    fireEvent.click(screen.getByRole('button', { name: 'Agregar ingrediente del catálogo' }));
    expect(document.querySelectorAll('input[type="number"]')).toHaveLength(0);
    expect(screen.getAllByLabelText(/^Cantidad/)[0]?.getAttribute('inputmode')).toBe('decimal');
  });
});
