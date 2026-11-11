// @vitest-environment jsdom

import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { PendingOperationsProvider } from '../../hooks/pendingOperations';
import type { InventoryCategory } from '../../services/inventory';
import type { SupplierReturnSummary } from '../../services/inventoryPurchasing';
import { InventoryPage } from './InventoryPage';
import { openInventorySection, sectionTab } from './inventoryNavigationTestUtils';
import {
  purchasingIDs,
  purchasingPermissions,
  receipt,
  soda,
  supplierPage,
  supplierSummary,
} from './inventoryPurchasingTestData';
import {
  allInventoryPermissions,
  ids,
  inventoryTransport,
  item,
  itemPage,
} from './inventoryTestData';
import type { RecordedRequest, Responder } from './inventoryTestData';

// Etapa 9A, fase A (A2): código interno sugerido, alta rápida y administración de categorías,
// recepción directa con «Compras de mostrador», devoluciones a proveedor y proveedores formales en
// órdenes de compra.

const native = vi.hoisted(() => ({ invoke: vi.fn() }));
vi.mock('@tauri-apps/api/core', () => ({ invoke: native.invoke, isTauri: () => true }));

afterEach(() => {
  cleanup();
  native.invoke.mockReset();
});

const counterID = '00000000-0000-4000-8000-000000000909';
const newCategoryID = '00000000-0000-4000-8000-0000000000e9';
const stamp = '2026-11-10T15:00:00Z';

function useTransport(override?: Responder) {
  const transport = inventoryTransport(override);
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

function codeInput() {
  return screen.getByLabelText(/^Código interno/) as HTMLInputElement;
}

async function openNewItem(permissions = allInventoryPermissions) {
  renderInventory(permissions);
  await openInventorySection('Insumos');
  await screen.findByText('Harina');
  fireEvent.click(screen.getByRole('button', { name: 'Nuevo insumo' }));
  await screen.findByLabelText('Nombre del insumo');
}

describe('código interno sugerido de insumos', () => {
  it('sugiere desde el nombre, usa el código libre de Go y nunca sobrescribe lo escrito', async () => {
    const requests = useTransport((request) => {
      if (
        request.operation === 'inventory_item_code' &&
        request.query?.code === 'harina-de-trigo'
      ) {
        return {
          status: 200,
          body: {
            code: 'harina-de-trigo',
            available: false,
            suggestion: 'harina-de-trigo-2',
            taken_by: 'Harina',
          },
        };
      }
      return undefined;
    });
    await openNewItem();
    expect(screen.getByText('Se sugiere a partir del nombre mientras no lo edites.')).toBeTruthy();
    fireEvent.change(screen.getByLabelText('Nombre del insumo'), {
      target: { value: 'Harina de Trigo' },
    });
    // La forma base aparece al instante y Go propone el libre (ya existe «harina-de-trigo»).
    expect(codeInput().value).toBe('harina-de-trigo');
    await waitFor(() => expect(codeInput().value).toBe('harina-de-trigo-2'));
    expect(writes(requests, 'inventory_item_code')[0]?.query).toEqual({ code: 'harina-de-trigo' });
    fireEvent.change(screen.getByLabelText('Nombre del insumo'), {
      target: { value: 'Harina de trigo integral' },
    });
    expect(codeInput().value).toBe('harina-de-trigo-integral');
    // Escrito por el usuario: el nombre ya no lo cambia; Go avisa si está tomado, sin reemplazarlo.
    fireEvent.change(codeInput(), { target: { value: 'harina-de-trigo' } });
    fireEvent.change(screen.getByLabelText('Nombre del insumo'), {
      target: { value: 'Harina integral' },
    });
    expect(codeInput().value).toBe('harina-de-trigo');
    await screen.findByText('Ese código lo usa «Harina». Libre: harina-de-trigo-2.');
    expect(codeInput().value).toBe('harina-de-trigo');
    expect(codeInput().getAttribute('aria-invalid')).toBe('true');
  });

  it('al editar un insumo el código no se sugiere y se comprueba sin contarse a sí mismo', async () => {
    const requests = useTransport();
    renderInventory(allInventoryPermissions);
    await openInventorySection('Insumos');
    fireEvent.click(await screen.findByRole('button', { name: 'Editar el insumo Harina' }));
    await screen.findByLabelText('Nombre del insumo');
    fireEvent.change(screen.getByLabelText('Nombre del insumo'), {
      target: { value: 'Harina de trigo' },
    });
    expect(codeInput().value).toBe('HAR-01');
    expect(writes(requests, 'inventory_item_code')).toHaveLength(0);
    fireEvent.change(codeInput(), { target: { value: 'HAR-02' } });
    await waitFor(() =>
      expect(writes(requests, 'inventory_item_code').at(-1)?.query).toEqual({
        code: 'HAR-02',
        item_id: ids.flour,
      }),
    );
  });
});

describe('alta rápida de categorías desde el insumo', () => {
  it('crea la categoría, la selecciona y conserva lo capturado sin crear el insumo', async () => {
    const created: InventoryCategory = {
      id: newCategoryID,
      name: 'Quesos',
      active: true,
      sort_order: 30,
      item_count: 0,
      revision: 1,
      updated_at: stamp,
    };
    const requests = useTransport((request) => {
      if (request.operation === 'inventory_create_category') {
        return { status: 200, body: created };
      }
      if (request.operation === 'inventory_create_item') {
        return { status: 200, body: item({ id: ids.created, name: 'Queso fresco' }) };
      }
      return undefined;
    });
    await openNewItem();
    fireEvent.change(screen.getByLabelText('Nombre del insumo'), {
      target: { value: 'Queso fresco' },
    });
    fireEvent.change(codeInput(), { target: { value: 'QUE-02' } });
    fireEvent.change(screen.getByLabelText(/^Unidad base/), { target: { value: ids.gram } });
    fireEvent.change(screen.getByLabelText('Descripción'), { target: { value: 'Del rancho' } });
    fireEvent.change(screen.getByLabelText(/^Categoría/), {
      target: { value: '__new_category__' },
    });
    const dialog = await screen.findByRole('dialog', { name: 'Nueva categoría de insumos' });
    // El diálogo vive fuera del formulario del insumo (sin formularios anidados).
    expect(dialog.parentElement?.closest('form')).toBeNull();
    const name = within(dialog).getByLabelText(/^Nombre de la categoría/);
    const create = within(dialog).getByRole('button', { name: 'Crear y seleccionar' });
    // Una categoría que ya está en la lista no se vuelve a crear.
    fireEvent.change(name, { target: { value: 'abarrotes' } });
    expect(within(dialog).getByText(/Ya existe la categoría «Abarrotes»/)).toBeTruthy();
    expect((create as HTMLButtonElement).disabled).toBe(true);
    fireEvent.change(name, { target: { value: 'Quesos' } });
    await act(async () => {
      fireEvent.click(create);
    });
    expect(writes(requests, 'inventory_create_category')[0]?.body).toEqual({
      name: 'Quesos',
      active: true,
      operation_id: expect.any(String),
    });
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    await screen.findByText('Categoría «Quesos» creada y seleccionada.');
    expect((screen.getByLabelText(/^Categoría/) as HTMLSelectElement).value).toBe(newCategoryID);
    expect((screen.getByLabelText('Nombre del insumo') as HTMLInputElement).value).toBe(
      'Queso fresco',
    );
    expect(codeInput().value).toBe('QUE-02');
    expect((screen.getByLabelText('Descripción') as HTMLTextAreaElement).value).toBe('Del rancho');
    expect(writes(requests, 'inventory_create_item')).toHaveLength(0);
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'Crear insumo' }));
    });
    expect(writes(requests, 'inventory_create_item')[0]?.body).toMatchObject({
      code: 'QUE-02',
      name: 'Queso fresco',
      category_id: newCategoryID,
      base_unit_id: ids.gram,
      description: 'Del rancho',
    });
  });

  it('cancelar el diálogo deja la categoría que había y no escribe', async () => {
    const requests = useTransport();
    await openNewItem();
    fireEvent.change(screen.getByLabelText(/^Categoría/), { target: { value: ids.dairy } });
    fireEvent.change(screen.getByLabelText(/^Categoría/), {
      target: { value: '__new_category__' },
    });
    const dialog = await screen.findByRole('dialog', { name: 'Nueva categoría de insumos' });
    fireEvent.keyDown(dialog, { key: 'Escape' });
    expect(screen.queryByRole('dialog')).toBeNull();
    expect((screen.getByLabelText(/^Categoría/) as HTMLSelectElement).value).toBe(ids.dairy);
    expect(writes(requests, 'inventory_create_category')).toHaveLength(0);
  });

  it('sin inventory.items.manage no ofrece «+ Nueva categoría»', async () => {
    useTransport();
    renderInventory(['inventory.read']);
    await openInventorySection('Insumos');
    fireEvent.click(await screen.findByRole('button', { name: 'Ver el insumo Harina' }));
    const select = await screen.findByLabelText(/^Categoría/);
    expect(within(select).queryByRole('option', { name: '+ Nueva categoría' })).toBeNull();
  });
});

describe('categorías de insumos en Configuración', () => {
  it('busca, abre los insumos de la categoría y desactiva con confirmación', async () => {
    const requests = useTransport((request) =>
      request.operation === 'inventory_save_category'
        ? { status: 200, body: { id: ids.groceries, name: 'Abarrotes', active: false } }
        : undefined,
    );
    renderInventory(allInventoryPermissions);
    await openInventorySection('Categorías');
    const table = await screen.findByRole('table', { name: 'Categorías de insumos' });
    expect(within(table).getByText('Abarrotes')).toBeTruthy();
    fireEvent.change(screen.getByLabelText('Buscar categoría'), { target: { value: 'LACT' } });
    expect(within(table).queryByText('Abarrotes')).toBeNull();
    expect(within(table).getByText('Lácteos')).toBeTruthy();
    fireEvent.change(screen.getByLabelText('Buscar categoría'), { target: { value: '' } });
    await act(async () => {
      fireEvent.click(
        within(table).getByRole('button', { name: 'Desactivar la categoría Abarrotes' }),
      );
    });
    const confirmation = await screen.findByRole('alertdialog');
    expect(confirmation.textContent).toContain('Los 2 insumos que ya la tienen la conservan.');
    await act(async () => {
      fireEvent.click(within(confirmation).getByRole('button', { name: 'Desactivar' }));
    });
    expect(writes(requests, 'inventory_save_category')[0]).toMatchObject({
      resourceId: ids.groceries,
      body: { name: 'Abarrotes', active: false, revision: 1, operation_id: expect.any(String) },
    });
    // «Ver insumos» abre Insumos (grupo Catálogo) filtrado por la categoría.
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'Ver los insumos de Lácteos' }));
    });
    await waitFor(() => expect(sectionTab('Insumos').getAttribute('aria-selected')).toBe('true'));
    await waitFor(() =>
      expect(writes(requests, 'inventory_items').at(-1)?.query).toMatchObject({
        category_id: ids.dairy,
      }),
    );
    expect((screen.getByLabelText('Categoría') as HTMLSelectElement).value).toBe(ids.dairy);
  });
});

// Proveedor especial «Compras de mostrador» y un proveedor formal.
function purchasingResponder(override?: Responder): Responder {
  const counter = supplierSummary({
    id: counterID,
    trade_name: 'Compras de mostrador',
    legal_name: '',
    rfc: null,
    kind: 'counter',
    item_count: 0,
  });
  return (request) => {
    const custom = override?.(request);
    if (custom) {
      return custom;
    }
    switch (request.operation) {
      case 'inventory_items':
        return { status: 200, body: itemPage([soda]) };
      case 'inventory_item':
        return { status: 200, body: soda };
      case 'inventory_suppliers':
        return {
          status: 200,
          body: supplierPage(
            request.query?.kind === 'counter'
              ? [counter]
              : request.query?.kind === 'formal'
                ? [supplierSummary()]
                : [counter, supplierSummary()],
          ),
        };
      default:
        return undefined;
    }
  };
}

describe('recepción directa con «Compras de mostrador»', () => {
  it('se abre preseleccionada, registra el comercio y no pide elegir proveedor', async () => {
    const recorded = receipt({
      id: purchasingIDs.direct,
      folio: 8,
      direct: true,
      purchase_order_id: null,
      purchase_order_folio: null,
      supplier: {
        id: counterID,
        trade_name: 'Compras de mostrador',
        kind: 'counter',
        active: true,
      },
      counter_store: 'Central de abastos',
    });
    const requests = useTransport(
      purchasingResponder((request) => {
        if (request.operation === 'inventory_record_receipt') {
          return { status: 200, body: recorded };
        }
        if (request.operation === 'inventory_receipt') {
          return { status: 200, body: recorded };
        }
        return undefined;
      }),
    );
    renderInventory(purchasingPermissions);
    await openInventorySection('Recepciones');
    fireEvent.click(await screen.findByRole('button', { name: 'Compra de mostrador' }));
    const modality = screen.getByRole('group', { name: 'Modalidad de la compra' });
    await waitFor(() =>
      expect(
        within(modality)
          .getByRole('button', { name: 'Compras de mostrador' })
          .getAttribute('aria-pressed'),
      ).toBe('true'),
    );
    expect(screen.queryByRole('combobox', { name: /^Proveedor/ })).toBeNull();
    fireEvent.change(screen.getByLabelText(/^Comercio/), {
      target: { value: 'Central de abastos' },
    });
    const line = screen.getByRole('group', { name: 'Renglón 1' });
    await within(line).findByRole('option', { name: 'Refresco · REF-01' });
    fireEvent.change(within(line).getByRole('combobox', { name: /^Insumo/ }), {
      target: { value: purchasingIDs.soda },
    });
    fireEvent.change(within(line).getByRole('combobox', { name: /^Unidad o presentación/ }), {
      target: { value: '' },
    });
    fireEvent.change(within(line).getByLabelText(/^Cantidad/), { target: { value: '6' } });
    fireEvent.change(within(line).getByLabelText(/^Precio unitario sin IVA/), {
      target: { value: '15' },
    });
    fireEvent.click(within(line).getByRole('button', { name: 'IVA 0 %' }));
    const submit = screen
      .getAllByRole('button', { name: 'Registrar recepción' })
      .find((button) => button.getAttribute('type') === 'submit') as HTMLButtonElement;
    await waitFor(() => expect(submit.disabled).toBe(false));
    await act(async () => {
      fireEvent.click(submit);
    });
    const dialog = await screen.findByRole('alertdialog');
    expect(dialog.textContent).toContain('«Compras de mostrador» (Central de abastos)');
    await act(async () => {
      fireEvent.click(within(dialog).getByRole('button', { name: 'Registrar recepción' }));
    });
    expect(writes(requests, 'inventory_record_receipt')[0]?.body).toEqual({
      supplier_id: counterID,
      warehouse_id: ids.general,
      counter_store: 'Central de abastos',
      lines: [{ item_id: purchasingIDs.soda, quantity: '6', unit_price: '15', tax_rate: 0 }],
      operation_id: expect.any(String),
    });
    const detail = await screen.findByRole('region', {
      name: 'Recepción 8 · Compras de mostrador',
    });
    expect(within(detail).getByText('Central de abastos')).toBeTruthy();
  });

  it('con un proveedor registrado ofrece solo formales y no envía comercio', async () => {
    const requests = useTransport(purchasingResponder());
    renderInventory(purchasingPermissions);
    await openInventorySection('Recepciones');
    fireEvent.click(await screen.findByRole('button', { name: 'Recepción directa' }));
    await screen.findByRole('option', { name: 'Distribuidora del Norte' });
    // El selector de la recepción pide solo proveedores formales activos.
    const pickerQuery = writes(requests, 'inventory_suppliers').filter(
      (request) => request.query?.active === 'true' && request.query?.page_size === '50',
    );
    expect(pickerQuery.length).toBeGreaterThan(0);
    expect(pickerQuery.every((request) => request.query?.kind === 'formal')).toBe(true);
    expect(screen.queryByLabelText(/^Comercio/)).toBeNull();
    // Cambiar a mostrador oculta el selector y muestra el comercio.
    fireEvent.click(screen.getByRole('button', { name: 'Compras de mostrador' }));
    expect(screen.queryByRole('combobox', { name: /^Proveedor/ })).toBeNull();
    expect(screen.getByLabelText(/^Comercio/)).toBeTruthy();
  });

  it('las recepciones de mostrador se distinguen y se filtran por modalidad', async () => {
    const counterReceipt = receipt({
      id: purchasingIDs.direct,
      folio: 8,
      direct: true,
      purchase_order_id: null,
      purchase_order_folio: null,
      supplier: {
        id: counterID,
        trade_name: 'Compras de mostrador',
        kind: 'counter',
        active: true,
      },
      counter_store: 'Mercado Juárez',
    });
    const requests = useTransport(
      purchasingResponder((request) =>
        request.operation === 'inventory_receipts'
          ? {
              status: 200,
              body: {
                items: [counterReceipt],
                total: 1,
                page: 1,
                page_size: 25,
                costs_visible: false,
              },
            }
          : undefined,
      ),
    );
    renderInventory(purchasingPermissions);
    await openInventorySection('Recepciones');
    await screen.findByText('Mostrador · Mercado Juárez');
    fireEvent.change(screen.getByLabelText('Tipo de recepción'), { target: { value: 'counter' } });
    await waitFor(() =>
      expect(writes(requests, 'inventory_receipts').at(-1)?.query).toMatchObject({
        supplier_kind: 'counter',
      }),
    );
    expect(writes(requests, 'inventory_receipts').at(-1)?.query?.direct).toBeUndefined();
  });
});

describe('órdenes de compra y proveedores formales', () => {
  it('el proveedor de una orden nueva solo se elige entre formales', async () => {
    const requests = useTransport(purchasingResponder());
    renderInventory(purchasingPermissions);
    await openInventorySection('Órdenes de compra');
    fireEvent.click(await screen.findByRole('button', { name: 'Nueva orden de compra' }));
    await screen.findByRole('group', { name: 'Renglón 1' });
    await waitFor(() =>
      expect(
        writes(requests, 'inventory_suppliers').some(
          (request) => request.query?.kind === 'formal' && request.query?.active === 'true',
        ),
      ).toBe(true),
    );
    expect(
      writes(requests, 'inventory_suppliers').every((request) => request.query?.kind === 'formal'),
    ).toBe(true);
  });
});

describe('devoluciones a proveedor', () => {
  it('lista las devoluciones y abre la recepción de origen', async () => {
    const entry: SupplierReturnSummary = {
      id: '00000000-0000-4000-8000-000000000981',
      folio: 3,
      receipt_id: purchasingIDs.receipt,
      receipt_folio: 5,
      supplier: {
        id: purchasingIDs.supplier,
        trade_name: 'Distribuidora del Norte',
        kind: 'formal',
        active: true,
      },
      warehouse: { id: ids.general, name: 'Almacén general' },
      occurred_at: stamp,
      reason: 'Producto dañado en la entrega',
      actor_name: 'Almacenista',
      value: '52.20',
      line_count: 1,
      costs_visible: false,
      unabsorbed: null,
      unabsorbed_display: null,
      value_exact: null,
      created_at: stamp,
      receipt_supplier_reference: 'F-1001',
    };
    const requests = useTransport(
      purchasingResponder((request) => {
        if (request.operation === 'inventory_supplier_returns') {
          return {
            status: 200,
            body: { items: [entry], total: 1, page: 1, page_size: 25, costs_visible: false },
          };
        }
        if (request.operation === 'inventory_receipt') {
          return { status: 200, body: receipt() };
        }
        return undefined;
      }),
    );
    renderInventory(purchasingPermissions);
    await openInventorySection('Devoluciones a proveedor');
    const row = (await screen.findByText('Producto dañado en la entrega')).closest('tr')!;
    expect(within(row).getByText('$52.20')).toBeTruthy();
    expect(within(row).getByText('De la recepción 5 · Ref. F-1001')).toBeTruthy();
    await act(async () => {
      fireEvent.click(within(row).getByRole('button', { name: /^Abrir la recepción 5/ }));
    });
    await waitFor(() =>
      expect(sectionTab('Recepciones').getAttribute('aria-selected')).toBe('true'),
    );
    await screen.findByRole('region', { name: 'Recepción 5 · Distribuidora del Norte' });
    expect(writes(requests, 'inventory_receipt').at(-1)?.resourceId).toBe(purchasingIDs.receipt);
  });
});
