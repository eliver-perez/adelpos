// @vitest-environment jsdom

import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { PendingOperationsProvider } from '../../hooks/pendingOperations';
import { InventoryPage } from './InventoryPage';
import {
  cashExpense,
  cashLink,
  draftOrder,
  orderLine,
  orderPage,
  purchaseOrder,
  purchasingIDs,
  purchasingPermissions,
  receipt,
  receiptPage,
  receiptWithoutCosts,
  soda,
  supplierDetail,
  supplierPage,
  supplierSummary,
} from './inventoryPurchasingTestData';
import { allInventoryPermissions, ids, inventoryTransport, itemPage } from './inventoryTestData';
import type { RecordedRequest, Responder } from './inventoryTestData';
import { navigationMap, openInventorySection, sectionTab } from './inventoryNavigationTestUtils';

const native = vi.hoisted(() => ({ invoke: vi.fn() }));
vi.mock('@tauri-apps/api/core', () => ({ invoke: native.invoke, isTauri: () => true }));

afterEach(() => {
  cleanup();
  native.invoke.mockReset();
});

// Respuestas de la fase C comunes a estas pruebas; cada prueba antepone las suyas.
function purchasingResponder(override: Responder = () => undefined): Responder {
  return (request) => {
    const custom = override(request);
    if (custom) {
      return custom;
    }
    switch (request.operation) {
      case 'inventory_items':
        return { status: 200, body: itemPage([soda]) };
      case 'inventory_item':
        return { status: 200, body: soda };
      case 'inventory_suppliers':
        return { status: 200, body: supplierPage([supplierSummary()]) };
      case 'inventory_supplier':
        return { status: 200, body: supplierDetail() };
      case 'inventory_purchase_orders':
        return { status: 200, body: orderPage([purchaseOrder()]) };
      case 'inventory_purchase_order':
        return { status: 200, body: purchaseOrder() };
      case 'inventory_receipts':
        return { status: 200, body: receiptPage([receipt()]) };
      case 'inventory_receipt':
        return { status: 200, body: receipt() };
      case 'inventory_cash_expenses':
        return {
          status: 200,
          body: {
            items: [
              cashExpense(),
              cashExpense({
                id: purchasingIDs.dollarExpense,
                concept: 'Pago en dólares',
                category_name: null,
                currency: 'USD',
                amount: '50.00',
                available_amount: '50.00',
                linkable: false,
                unavailable_reason: 'currency',
                unavailable_label:
                  'Gasto en dólares: las compras se vinculan solo con gastos en pesos',
              }),
            ],
            total: 2,
            page: 1,
            page_size: 20,
          },
        };
      default:
        return undefined;
    }
  };
}

function useTransport(override?: Responder) {
  const transport = inventoryTransport(purchasingResponder(override));
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

async function openTab(name: string, permissions = purchasingPermissions) {
  renderInventory(permissions);
  await openInventorySection(name);
}

// Envía con el botón principal cuando ya está habilitado (los almacenes llegan después del
// primer render).
async function submitWith(name: string) {
  const button = screen
    .getAllByRole('button', { name })
    .find((candidate) => candidate.getAttribute('type') === 'submit') as HTMLButtonElement;
  await waitFor(() => expect(button.disabled).toBe(false));
  await act(async () => {
    fireEvent.click(button);
  });
  return button;
}

async function confirmDialog(button: string) {
  const dialog = await screen.findByRole('alertdialog');
  await act(async () => {
    fireEvent.click(within(dialog).getByRole('button', { name: button }));
  });
  return dialog;
}

const overlinkedResponse = {
  status: 409,
  body: {
    error: {
      code: 'CASH_EXPENSE_OVERLINKED',
      message: 'El gasto de $850.00 ya tiene $500.00 vinculados: solo quedan $350.00 disponibles.',
    },
  },
};

describe('secciones de compras', () => {
  it('se muestran con cualquiera de los permisos de compras', async () => {
    useTransport();
    const { unmount } = renderInventory(allInventoryPermissions);
    await screen.findByText('Control de inventario');
    const withoutPurchasing = navigationMap();
    expect(Object.keys(withoutPurchasing)).not.toContain('Compras');
    expect(withoutPurchasing['Catálogo']).not.toContain('Proveedores');
    unmount();
    renderInventory(['inventory.read', 'inventory.purchases.receive']);
    await screen.findByText('Control de inventario');
    expect(navigationMap()).toEqual({
      Resumen: [],
      Catálogo: ['Insumos', 'Recetas', 'Almacenes', 'Proveedores'],
      Existencias: ['Ajustes y mermas'],
      Compras: [
        'Órdenes de compra',
        'Sugerencias de compra',
        'Recepciones',
        'Devoluciones a proveedor',
      ],
      Configuración: ['Unidades', 'Categorías'],
    });
  });
});

describe('proveedores', () => {
  it('da de alta un proveedor sin RFC y agrega el insumo que surte', async () => {
    const created = supplierDetail({
      id: purchasingIDs.otherSupplier,
      trade_name: 'Abarrotes Rivera',
      legal_name: '',
      rfc: null,
      item_count: 0,
      has_purchases: false,
      revision: 1,
      items: [],
    });
    let current = created;
    const requests = useTransport((request) => {
      if (request.operation === 'inventory_create_supplier') {
        return { status: 200, body: created };
      }
      if (request.operation === 'inventory_supplier') {
        return { status: 200, body: current };
      }
      if (request.operation === 'inventory_save_supplier_item') {
        current = { ...created, item_count: 1, revision: 2 };
        current.items = [supplierDetail().items[0]!];
        return { status: 200, body: current };
      }
      return undefined;
    });
    await openTab('Proveedores');
    expect(lastQuery(requests, 'inventory_suppliers')).toMatchObject({ active: 'true' });
    fireEvent.click(await screen.findByRole('button', { name: 'Nuevo proveedor' }));
    fireEvent.change(screen.getByLabelText(/^Nombre comercial/), {
      target: { value: 'Abarrotes Rivera' },
    });
    fireEvent.change(screen.getByLabelText('Teléfono'), { target: { value: '868 555 0199' } });
    // Etapa 9A: celular y página web opcionales; Go valida su formato.
    fireEvent.change(screen.getByLabelText(/^Teléfono celular/), {
      target: { value: '868 555 0100' },
    });
    fireEvent.change(screen.getByLabelText(/^Página web/), {
      target: { value: 'abarrotesrivera.mx' },
    });
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'Crear proveedor' }));
    });
    const [create] = writes(requests, 'inventory_create_supplier');
    expect(create?.body).toEqual({
      trade_name: 'Abarrotes Rivera',
      legal_name: '',
      rfc: null,
      phone: '868 555 0199',
      phone_secondary: '',
      mobile_phone: '868 555 0100',
      website: 'abarrotesrivera.mx',
      email: '',
      address: '',
      contact: '',
      notes: '',
      operation_id: expect.any(String),
    });
    await screen.findByText('Proveedor creado. Ya puedes agregar los insumos que surte.');
    const supplied = await screen.findByRole('region', { name: 'Insumos que surte' });
    fireEvent.click(within(supplied).getByRole('button', { name: 'Agregar insumo surtido' }));
    const form = screen.getByRole('form', { name: 'Nuevo insumo surtido' });
    await within(form).findByRole('option', { name: 'Refresco · REF-01' });
    fireEvent.change(within(form).getByRole('combobox', { name: /^Insumo surtido/ }), {
      target: { value: purchasingIDs.soda },
    });
    fireEvent.change(within(form).getByRole('combobox', { name: /^Presentación habitual/ }), {
      target: { value: purchasingIDs.box },
    });
    fireEvent.change(within(form).getByLabelText(/^Código del proveedor/), {
      target: { value: 'R-24' },
    });
    fireEvent.click(
      within(form).getByRole('button', { name: 'Proveedor preferido de este insumo' }),
    );
    await act(async () => {
      fireEvent.click(within(form).getByRole('button', { name: 'Agregar insumo surtido' }));
    });
    const [saveItem] = writes(requests, 'inventory_save_supplier_item');
    expect(saveItem?.resourceId).toBe(purchasingIDs.otherSupplier);
    expect(saveItem?.body).toEqual({
      revision: 0,
      item_id: purchasingIDs.soda,
      presentation_id: purchasingIDs.box,
      supplier_code: 'R-24',
      preferred: true,
      operation_id: expect.any(String),
    });
    await screen.findByText('Insumo agregado a «Abarrotes Rivera».');
    const row = within(supplied).getByText('Refresco').closest('li')!;
    expect(within(row).getByText('Preferido')).toBeTruthy();
    expect(
      within(row).getByText(/Último precio de este proveedor: \$250\.00 sin IVA por Caja con 24/),
    ).toBeTruthy();
  });

  it('desactiva un proveedor con compras solo después de confirmarlo', async () => {
    const requests = useTransport((request) =>
      request.operation === 'inventory_save_supplier'
        ? { status: 200, body: supplierDetail({ active: false, revision: 5 }) }
        : undefined,
    );
    await openTab('Proveedores');
    fireEvent.click(
      await screen.findByRole('button', { name: 'Editar el proveedor Distribuidora del Norte' }),
    );
    fireEvent.click(await screen.findByRole('button', { name: 'Proveedor activo' }));
    const dialog = await screen.findByRole('alertdialog');
    expect(dialog.textContent).toContain('Sus compras, recepciones y devoluciones se conservan');
    fireEvent.click(within(dialog).getByRole('button', { name: 'Desactivar' }));
    await waitFor(() =>
      expect(
        screen.getByRole('button', { name: 'Proveedor activo' }).getAttribute('aria-pressed'),
      ).toBe('false'),
    );
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'Guardar proveedor' }));
    });
    const [save] = writes(requests, 'inventory_save_supplier');
    expect(save?.resourceId).toBe(purchasingIDs.supplier);
    expect(save?.body).toMatchObject({ revision: 4, active: false, rfc: 'DNO010203AB1' });
    expect(screen.queryByRole('button', { name: /Eliminar|Borrar/ })).toBeNull();
  });

  it('sin permiso de administrar proveedores solo permite consultarlos', async () => {
    useTransport();
    await openTab('Proveedores', ['inventory.read', 'inventory.purchases.receive']);
    await screen.findByRole('table');
    expect(screen.queryByRole('button', { name: 'Nuevo proveedor' })).toBeNull();
    fireEvent.click(
      screen.getByRole('button', { name: 'Ver el proveedor Distribuidora del Norte' }),
    );
    await screen.findByText(/Editar proveedores requiere el permiso «Administrar proveedores»/);
    const save = await screen.findByRole('button', { name: 'Guardar proveedor' });
    expect((save as HTMLButtonElement).disabled).toBe(true);
    expect(screen.queryByRole('button', { name: 'Agregar insumo surtido' })).toBeNull();
  });

  it('el historial del proveedor abre sus órdenes en la pestaña Compras', async () => {
    const history = supplierDetail();
    history.history = {
      ...history.history,
      orders: [purchaseOrder({ state: 'received', state_label: 'Recibida', receipt_count: 2 })],
    };
    const requests = useTransport((request) =>
      request.operation === 'inventory_supplier' ? { status: 200, body: history } : undefined,
    );
    await openTab('Proveedores');
    fireEvent.click(
      await screen.findByRole('button', { name: 'Editar el proveedor Distribuidora del Norte' }),
    );
    const summary = await screen.findByRole('button', { name: /^Historial de compras/ });
    expect(summary.textContent).toContain('1 orden · 2 recepciones · 0 devoluciones');
    expect(screen.getByText('$6,846.00')).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: 'Ver orden' }));
    await waitFor(() =>
      expect(sectionTab('Órdenes de compra').getAttribute('aria-selected')).toBe('true'),
    );
    await screen.findByRole('region', { name: 'Orden 12 · Distribuidora del Norte' });
    expect(writes(requests, 'inventory_purchase_order').at(-1)?.resourceId).toBe(
      purchasingIDs.order,
    );
  });
});

describe('órdenes de compra', () => {
  it('crea el borrador con su renglón, IVA por botones y muestra los totales de Go', async () => {
    let created = false;
    const requests = useTransport((request) => {
      if (request.operation === 'inventory_purchase_orders') {
        return { status: 200, body: orderPage([]) };
      }
      if (request.operation === 'inventory_create_purchase_order') {
        created = true;
        return { status: 200, body: draftOrder() };
      }
      if (request.operation === 'inventory_purchase_order' && created) {
        return { status: 200, body: draftOrder() };
      }
      if (request.operation === 'inventory_issue_purchase_order') {
        return { status: 200, body: purchaseOrder({ id: purchasingIDs.draft, folio: 13 }) };
      }
      return undefined;
    });
    await openTab('Órdenes de compra');
    fireEvent.click(await screen.findByRole('button', { name: 'Nueva orden de compra' }));
    await screen.findByRole('option', { name: 'Distribuidora del Norte' });
    fireEvent.change(screen.getByRole('combobox', { name: /^Proveedor/ }), {
      target: { value: purchasingIDs.supplier },
    });
    const line = screen.getByRole('group', { name: 'Renglón 1' });
    await within(line).findByRole('option', { name: 'Refresco · REF-01' });
    // Esperar a que lleguen los insumos del proveedor para proponer su presentación habitual.
    await waitFor(() => expect(writes(requests, 'inventory_supplier').length).toBeGreaterThan(0));
    fireEvent.change(within(line).getByRole('combobox', { name: /^Insumo/ }), {
      target: { value: purchasingIDs.soda },
    });
    const measure = within(line).getByRole('combobox', {
      name: /^Unidad o presentación/,
    }) as HTMLSelectElement;
    expect(measure.value).toBe(`presentation:${purchasingIDs.box}`);
    expect(
      within(line).getByText(/Último precio de este proveedor: \$250\.00 sin IVA por Caja con 24/),
    ).toBeTruthy();
    fireEvent.change(within(line).getByLabelText(/^Cantidad/), { target: { value: '3' } });
    fireEvent.change(within(line).getByLabelText(/^Precio unitario sin IVA/), {
      target: { value: '410' },
    });
    fireEvent.click(within(line).getByRole('button', { name: 'IVA 8 %' }));
    expect(within(line).getByRole('button', { name: 'IVA 8 %' }).getAttribute('aria-pressed')).toBe(
      'true',
    );
    // Antes de guardar no hay importes: los calcula Go.
    expect(
      screen.getByText(/Los importes los calcula el servidor al guardar el borrador/),
    ).toBeTruthy();
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'Crear borrador' }));
    });
    const [create] = writes(requests, 'inventory_create_purchase_order');
    expect(create?.body).toEqual({
      supplier_id: purchasingIDs.supplier,
      expected_warehouse_id: ids.general,
      notes: '',
      lines: [
        {
          item_id: purchasingIDs.soda,
          presentation_id: purchasingIDs.box,
          quantity: '3',
          unit_price: '410',
          tax_rate: 8,
        },
      ],
      operation_id: expect.any(String),
    });
    // Totales y renglón tal como los devolvió Go (no son la suma de lo capturado).
    const totals = await screen.findByRole('region', { name: 'Totales de la orden' });
    expect(within(totals).getByText('$1,333.32')).toBeTruthy();
    expect(within(totals).getByText('$1,234.56')).toBeTruthy();
    expect(within(totals).getByText('−$7.89')).toBeTruthy();
    expect(within(totals).getByText('$98.76')).toBeTruthy();
    const amounts = within(screen.getByRole('group', { name: 'Renglón 1' })).getByLabelText(
      'Importes del renglón',
    );
    expect(within(amounts).getByText('$1,328.40')).toBeTruthy();
    // Emitir con permiso de autorizar, previa confirmación.
    fireEvent.click(screen.getByRole('button', { name: 'Emitir orden' }));
    const dialog = await confirmDialog('Emitir orden');
    expect(dialog.textContent).toContain('$1,333.32');
    const [issue] = writes(requests, 'inventory_issue_purchase_order');
    expect(issue?.resourceId).toBe(purchasingIDs.draft);
    expect(issue?.body).toEqual({ revision: 1, operation_id: expect.any(String) });
    await screen.findByText('Orden 13 emitida a «Distribuidora del Norte».');
  });

  it('sin permiso de autorizar no emite, cancela ni cierra', async () => {
    useTransport();
    await openTab(
      'Órdenes de compra',
      purchasingPermissions.filter((permission) => permission !== 'inventory.purchases.authorize'),
    );
    fireEvent.click(
      await screen.findByRole('button', { name: 'Abrir la orden 12 de Distribuidora del Norte' }),
    );
    await screen.findByRole('button', { name: 'Registrar recepción' });
    expect(screen.queryByRole('button', { name: 'Cancelar orden' })).toBeNull();
    expect(screen.queryByRole('button', { name: 'Cerrar administrativamente' })).toBeNull();
    expect(
      screen.getByText(/Emitir, cancelar o cerrar requiere el permiso «Emitir \(autorizar\)/),
    ).toBeTruthy();
    // Solicitado, recibido y pendiente por renglón.
    const row = screen.getByText('Refresco').closest('tr')!;
    expect(within(row).getByText('Caja con 24')).toBeTruthy();
    expect(within(row).getByText('$5,742.00')).toBeTruthy();
  });

  it('cierra administrativamente con motivo y confirmación', async () => {
    const requests = useTransport((request) => {
      if (request.operation === 'inventory_close_purchase_order') {
        return {
          status: 200,
          body: purchaseOrder({
            state: 'closed',
            state_label: 'Cerrada administrativamente',
            cancellable: false,
            closable: false,
            receivable: false,
          }),
        };
      }
      return undefined;
    });
    await openTab('Órdenes de compra');
    fireEvent.click(
      await screen.findByRole('button', { name: 'Abrir la orden 12 de Distribuidora del Norte' }),
    );
    fireEvent.click(await screen.findByRole('button', { name: 'Cerrar administrativamente' }));
    const closing = screen.getByRole('region', { name: 'Cerrar administrativamente' });
    const closeButton = within(closing).getByRole('button', { name: 'Cerrar orden' });
    expect((closeButton as HTMLButtonElement).disabled).toBe(true);
    fireEvent.change(within(closing).getByLabelText('Motivo del cierre'), {
      target: { value: 'El proveedor ya no surtirá lo pendiente' },
    });
    fireEvent.click(closeButton);
    await confirmDialog('Cerrar orden');
    const [close] = writes(requests, 'inventory_close_purchase_order');
    expect(close?.body).toEqual({
      revision: 2,
      reason: 'El proveedor ya no surtirá lo pendiente',
      operation_id: expect.any(String),
    });
    await screen.findByText('Orden 12: cerrada administrativamente.');
    expect(screen.queryByRole('button', { name: 'Cancelar orden' })).toBeNull();
  });

  it('cancela una orden emitida con motivo y confirmación de peligro', async () => {
    const requests = useTransport((request) =>
      request.operation === 'inventory_cancel_purchase_order'
        ? {
            status: 200,
            body: purchaseOrder({
              state: 'cancelled',
              state_label: 'Cancelada',
              cancellable: false,
              closable: false,
              receivable: false,
            }),
          }
        : undefined,
    );
    await openTab('Órdenes de compra');
    fireEvent.click(
      await screen.findByRole('button', { name: 'Abrir la orden 12 de Distribuidora del Norte' }),
    );
    fireEvent.click(await screen.findByRole('button', { name: 'Cancelar orden' }));
    const cancelling = screen.getByRole('region', { name: 'Cancelar la orden' });
    fireEvent.change(within(cancelling).getByLabelText('Motivo de la cancelación'), {
      target: { value: 'Pedido duplicado' },
    });
    fireEvent.click(within(cancelling).getByRole('button', { name: 'Cancelar orden' }));
    const dialog = await confirmDialog('Cancelar orden');
    expect(dialog.textContent).toContain('«Pedido duplicado»');
    expect(dialog.className).toContain('dialog-panel-danger');
    const [cancel] = writes(requests, 'inventory_cancel_purchase_order');
    expect(cancel?.body).toEqual({
      revision: 2,
      reason: 'Pedido duplicado',
      operation_id: expect.any(String),
    });
  });
});

describe('recepciones de una orden', () => {
  it('recibe 12 de 20 cajas y muestra lo pendiente que calcula Go', async () => {
    const requests = useTransport((request) =>
      request.operation === 'inventory_record_receipt'
        ? { status: 200, body: receipt() }
        : undefined,
    );
    await openTab('Órdenes de compra');
    fireEvent.click(
      await screen.findByRole('button', { name: 'Abrir la orden 12 de Distribuidora del Norte' }),
    );
    fireEvent.click(await screen.findByRole('button', { name: 'Registrar recepción' }));
    const line = screen.getByRole('group', { name: 'Renglón 1: Refresco' });
    const quantity = within(line).getByLabelText(/^Cantidad recibida/) as HTMLInputElement;
    // Se propone lo pendiente tal como lo entrega Go.
    expect(quantity.value).toBe('20');
    fireEvent.change(quantity, { target: { value: '12' } });
    fireEvent.change(screen.getByLabelText(/^Referencia del proveedor/), {
      target: { value: 'F-1001' },
    });
    await submitWith('Registrar recepción');
    const dialog = await confirmDialog('Registrar recepción');
    expect(dialog.textContent).toContain('1 renglón de la orden 12 en «Almacén general»');
    const [record] = writes(requests, 'inventory_record_receipt');
    expect(record?.body).toEqual({
      purchase_order_id: purchasingIDs.order,
      warehouse_id: ids.general,
      supplier_reference: 'F-1001',
      lines: [{ order_line_no: 1, quantity: '12' }],
      operation_id: expect.any(String),
    });
    await screen.findByText('Recepción 5 registrada: 1 renglón en «Almacén general».');
    const detail = await screen.findByRole('region', {
      name: 'Recepción 5 · Distribuidora del Norte',
    });
    const quantities = within(detail).getByRole('table', { name: 'Cantidades recibidas' });
    const row = within(quantities).getByText('Refresco').closest('tr')!;
    expect(within(row).getAllByText('20').length).toBeGreaterThan(0);
    expect(within(row).getByText('12')).toBeTruthy();
    expect(within(row).getByText('8')).toBeTruthy();
    expect(within(detail).getByText('IVA incluido en el costo')).toBeTruthy();
    expect(within(detail).getByText('Almacenista')).toBeTruthy();
    expect(within(detail).getByRole('columnheader', { name: 'Costo recibido' })).toBeTruthy();
    expect(within(detail).getByText('$11.9625 / pza')).toBeTruthy();
  });

  it('recibe las 8 cajas restantes de una orden parcialmente recibida', async () => {
    const partial = purchaseOrder({
      state: 'partially_received',
      state_label: 'Parcialmente recibida',
      receipt_count: 1,
      lines: [
        orderLine({
          received_quantity: '12',
          pending_quantity: '8',
          received_base_quantity: '288',
          pending_base_quantity: '192',
        }),
      ],
    });
    const requests = useTransport((request) => {
      if (request.operation === 'inventory_purchase_order') {
        return { status: 200, body: partial };
      }
      if (request.operation === 'inventory_record_receipt') {
        return {
          status: 200,
          body: receipt({ folio: 6, lines: [] }),
        };
      }
      return undefined;
    });
    await openTab('Órdenes de compra');
    fireEvent.click(
      await screen.findByRole('button', { name: 'Abrir la orden 12 de Distribuidora del Norte' }),
    );
    expect(await screen.findByText('Parcialmente recibida', { selector: '.tag' })).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: 'Registrar recepción' }));
    const quantity = within(
      screen.getByRole('group', { name: 'Renglón 1: Refresco' }),
    ).getByLabelText(/^Cantidad recibida/) as HTMLInputElement;
    expect(quantity.value).toBe('8');
    await submitWith('Registrar recepción');
    await confirmDialog('Registrar recepción');
    const [record] = writes(requests, 'inventory_record_receipt');
    expect(record?.body).toMatchObject({ lines: [{ order_line_no: 1, quantity: '8' }] });
    expect(record?.body).not.toHaveProperty('allow_excess');
  });

  it('recibir de más exige confirmarlo y reenvía con allow_excess', async () => {
    let attempts = 0;
    const requests = useTransport((request) => {
      if (request.operation !== 'inventory_record_receipt') {
        return undefined;
      }
      attempts += 1;
      if (attempts === 1) {
        return {
          status: 409,
          body: {
            error: {
              code: 'RECEIPT_EXCEEDS_PENDING',
              message:
                '«Refresco» tiene pendiente 20 Caja con 24 y se capturó 21. Para recibir de más confirma explícitamente el excedente.',
            },
          },
        };
      }
      return {
        status: 200,
        body: receipt({
          excess: true,
          warnings: [
            {
              code: 'RECEIPT_EXCESS',
              message: 'Se recibió más de lo pendiente de «Refresco»: excedente de 1 Caja con 24.',
              item_id: purchasingIDs.soda,
            },
          ],
        }),
      };
    });
    await openTab('Órdenes de compra');
    fireEvent.click(
      await screen.findByRole('button', { name: 'Abrir la orden 12 de Distribuidora del Norte' }),
    );
    fireEvent.click(await screen.findByRole('button', { name: 'Registrar recepción' }));
    fireEvent.change(screen.getByLabelText(/^Cantidad recibida/), { target: { value: '21' } });
    await submitWith('Registrar recepción');
    await confirmDialog('Registrar recepción');
    const excess = await screen.findByRole('alertdialog');
    expect(excess.textContent).toContain('tiene pendiente 20 Caja con 24 y se capturó 21');
    await act(async () => {
      fireEvent.click(within(excess).getByRole('button', { name: 'Registrar con excedente' }));
    });
    const [first, second] = writes(requests, 'inventory_record_receipt');
    expect(first?.body).not.toHaveProperty('allow_excess');
    expect(second?.body).toMatchObject({
      allow_excess: true,
      lines: [{ order_line_no: 1, quantity: '21' }],
    });
    await screen.findByText(
      'Se recibió más de lo pendiente de «Refresco»: excedente de 1 Caja con 24.',
      {
        selector: 'li',
      },
    );
  });

  it('un resultado incierto bloquea el formulario y se recupera con el mismo identificador', async () => {
    let attempts = 0;
    const requests = useTransport((request) => {
      if (request.operation !== 'inventory_record_receipt') {
        return undefined;
      }
      attempts += 1;
      if (attempts === 1) {
        return {
          status: 503,
          body: { error: { code: 'SERVICE_UNAVAILABLE', message: 'El servidor no respondió.' } },
        };
      }
      return { status: 200, body: receipt() };
    });
    await openTab('Órdenes de compra');
    fireEvent.click(
      await screen.findByRole('button', { name: 'Abrir la orden 12 de Distribuidora del Norte' }),
    );
    fireEvent.click(await screen.findByRole('button', { name: 'Registrar recepción' }));
    await submitWith('Registrar recepción');
    await confirmDialog('Registrar recepción');
    await screen.findByText(/No se pudo confirmar si la recepción quedó registrada/);
    const submit = screen
      .getAllByRole('button', { name: 'Registrar recepción' })
      .find((button) => button.getAttribute('type') === 'submit') as HTMLButtonElement;
    expect(submit.disabled).toBe(true);
    await act(async () => {
      fireEvent.click(
        screen.getByRole('button', { name: 'Reintentar con el mismo identificador' }),
      );
    });
    const [first, retry] = writes(requests, 'inventory_record_receipt');
    expect(retry?.body?.operation_id).toBe(first?.body?.operation_id);
    expect(writes(requests, 'inventory_record_receipt')).toHaveLength(2);
    await screen.findByText('Cambio de inventario confirmado en el servidor.');
    expect(submit.disabled).toBe(true);
  });
});

describe('recepción directa', () => {
  it('exige proveedor y envía los renglones capturados en una sola pantalla', async () => {
    const direct = receipt({
      id: purchasingIDs.direct,
      folio: 7,
      direct: true,
      purchase_order_id: null,
      purchase_order_folio: null,
    });
    const requests = useTransport((request) => {
      if (request.operation === 'inventory_record_receipt') {
        return { status: 200, body: direct };
      }
      if (
        request.operation === 'inventory_receipt' &&
        request.resourceId === purchasingIDs.direct
      ) {
        return { status: 200, body: direct };
      }
      return undefined;
    });
    await openTab('Recepciones');
    fireEvent.click(await screen.findByRole('button', { name: 'Recepción directa' }));
    const submit = screen.getByRole('button', { name: 'Registrar recepción' }) as HTMLButtonElement;
    expect(submit.disabled).toBe(true);
    await screen.findByRole('option', { name: 'Distribuidora del Norte' });
    fireEvent.change(screen.getByRole('combobox', { name: /^Proveedor/ }), {
      target: { value: purchasingIDs.supplier },
    });
    const line = screen.getByRole('group', { name: 'Renglón 1' });
    await within(line).findByRole('option', { name: 'Refresco · REF-01' });
    fireEvent.change(within(line).getByRole('combobox', { name: /^Insumo/ }), {
      target: { value: purchasingIDs.soda },
    });
    fireEvent.change(within(line).getByRole('combobox', { name: /^Unidad o presentación/ }), {
      target: { value: '' },
    });
    fireEvent.change(within(line).getByLabelText(/^Cantidad/), { target: { value: '24' } });
    fireEvent.change(within(line).getByLabelText(/^Precio unitario sin IVA/), {
      target: { value: '10.5' },
    });
    fireEvent.change(within(line).getByLabelText(/^Descuento comercial/), {
      target: { value: '2' },
    });
    fireEvent.click(within(line).getByRole('button', { name: 'IVA 0 %' }));
    await submitWith('Registrar recepción');
    const dialog = await confirmDialog('Registrar recepción');
    expect(dialog.textContent).toContain('recepción directa de «Distribuidora del Norte»');
    const [record] = writes(requests, 'inventory_record_receipt');
    expect(record?.body).toEqual({
      supplier_id: purchasingIDs.supplier,
      warehouse_id: ids.general,
      lines: [
        {
          item_id: purchasingIDs.soda,
          quantity: '24',
          unit_price: '10.5',
          discount: '2',
          tax_rate: 0,
        },
      ],
      operation_id: expect.any(String),
    });
    await screen.findByRole('region', { name: 'Recepción 7 · Distribuidora del Norte' });
  });

  it('sin permiso de crear compras no ofrece la recepción directa', async () => {
    useTransport();
    await openTab('Recepciones', ['inventory.read', 'inventory.purchases.receive']);
    await screen.findByRole('button', { name: 'Recibir una orden' });
    expect(screen.queryByRole('button', { name: 'Recepción directa' })).toBeNull();
    expect(screen.getByText(/La recepción directa \(sin orden\) requiere además/)).toBeTruthy();
  });
});

describe('devolución a proveedor', () => {
  it('devuelve parte de lo recibido con motivo y confirmación', async () => {
    const requests = useTransport((request) =>
      request.operation === 'inventory_record_supplier_return'
        ? {
            status: 200,
            body: {
              id: '00000000-0000-4000-8000-000000000981',
              folio: 3,
              receipt_id: purchasingIDs.receipt,
              receipt_folio: 5,
              supplier: supplierSummary(),
              warehouse: { id: ids.general, name: 'Almacén general' },
              occurred_at: '2026-11-10T16:00:00Z',
              reason: 'Latas golpeadas',
              actor_name: 'Administrador',
              value: '574.20',
              line_count: 1,
              costs_visible: true,
              unabsorbed: '0',
              unabsorbed_display: '0.00',
              value_exact: '574.2',
              created_at: '2026-11-10T16:00:00Z',
              receipt_supplier_reference: 'F-1001',
              lines: [],
              warnings: [],
            },
          }
        : undefined,
    );
    await openTab('Recepciones');
    fireEvent.click(
      await screen.findByRole('button', {
        name: 'Abrir la recepción 5 de Distribuidora del Norte',
      }),
    );
    fireEvent.click(await screen.findByRole('button', { name: 'Registrar devolución' }));
    const line = screen.getByRole('group', { name: 'Devolución de Refresco' });
    expect(within(line).getByText('12', { selector: 'strong' })).toBeTruthy();
    fireEvent.change(within(line).getByLabelText(/^Cantidad a devolver/), {
      target: { value: '2' },
    });
    const submit = screen.getByRole('button', {
      name: 'Registrar devolución',
    }) as HTMLButtonElement;
    expect(submit.disabled).toBe(true);
    fireEvent.change(screen.getByLabelText('Motivo de la devolución'), {
      target: { value: 'Latas golpeadas' },
    });
    await submitWith('Registrar devolución');
    const dialog = await confirmDialog('Registrar devolución');
    expect(dialog.className).toContain('dialog-panel-danger');
    expect(dialog.textContent).toContain('No crea ingresos de efectivo');
    const [record] = writes(requests, 'inventory_record_supplier_return');
    expect(record?.body).toEqual({
      receipt_id: purchasingIDs.receipt,
      warehouse_id: ids.general,
      reason: 'Latas golpeadas',
      lines: [{ receipt_line_no: 1, quantity: '2' }],
      operation_id: expect.any(String),
    });
    await screen.findByText('Devolución 3 registrada: 1 renglón por $574.20.');
  });
});

describe('vínculo con gastos de caja', () => {
  it('busca candidatos, muestra el rechazo de Go y vincula parcialmente', async () => {
    let attempts = 0;
    const requests = useTransport((request) => {
      if (request.operation !== 'inventory_link_cash_expense') {
        return undefined;
      }
      attempts += 1;
      if (attempts === 1) {
        return overlinkedResponse;
      }
      return {
        status: 200,
        body: receipt({
          linked_amount: '300.00',
          linkable_amount: '3145.20',
          cash_links: [cashLink({ amount: '300.00' })],
        }),
      };
    });
    await openTab('Recepciones');
    fireEvent.click(
      await screen.findByRole('button', {
        name: 'Abrir la recepción 5 de Distribuidora del Norte',
      }),
    );
    const section = await screen.findByRole('region', { name: 'Vínculo con gastos de caja' });
    expect(
      within(section).getByText('La recepción no está vinculada con ningún gasto.'),
    ).toBeTruthy();
    fireEvent.click(within(section).getByRole('button', { name: 'Vincular con un gasto' }));
    const candidates = await screen.findByRole('region', { name: 'Gastos de caja para vincular' });
    await within(candidates).findByText('Compra de suministros · Proveedores');
    expect(lastQuery(requests, 'inventory_cash_expenses')).toMatchObject({
      receipt_id: purchasingIDs.receipt,
    });
    // El gasto en dólares no se puede elegir y Go explica por qué.
    expect(
      within(candidates).getByText(
        'Gasto en dólares: las compras se vinculan solo con gastos en pesos',
      ),
    ).toBeTruthy();
    const dollar = within(candidates).getByRole('button', {
      name: 'Elegir el gasto Pago en dólares de US$50.00',
    }) as HTMLButtonElement;
    expect(dollar.disabled).toBe(true);
    fireEvent.change(within(candidates).getByLabelText(/^Importe mínimo/), {
      target: { value: '800' },
    });
    await waitFor(() =>
      expect(lastQuery(requests, 'inventory_cash_expenses')).toMatchObject({ min_amount: '800' }),
    );
    fireEvent.click(
      within(candidates).getByRole('button', {
        name: 'Elegir el gasto Compra de suministros · Proveedores de $850.00',
      }),
    );
    const form = screen.getByRole('form', { name: 'Vincular el gasto elegido' });
    fireEvent.click(within(form).getByRole('button', { name: 'Disponible del gasto: $850.00' }));
    expect((within(form).getByLabelText(/^Importe a vincular/) as HTMLInputElement).value).toBe(
      '850.00',
    );
    await act(async () => {
      fireEvent.click(within(form).getByRole('button', { name: 'Vincular gasto' }));
    });
    await confirmDialog('Vincular gasto');
    await screen.findByText(
      'El gasto de $850.00 ya tiene $500.00 vinculados: solo quedan $350.00 disponibles.',
    );
    fireEvent.change(within(form).getByLabelText(/^Importe a vincular/), {
      target: { value: '300' },
    });
    await act(async () => {
      fireEvent.click(within(form).getByRole('button', { name: 'Vincular gasto' }));
    });
    await confirmDialog('Vincular gasto');
    const [, link] = writes(requests, 'inventory_link_cash_expense');
    expect(link?.resourceId).toBe(purchasingIDs.receipt);
    expect(link?.body).toEqual({
      cash_operation_id: purchasingIDs.expense,
      amount: '300',
      operation_id: expect.any(String),
    });
    await screen.findByText('Gasto vinculado con la recepción 5.');
  });

  it('señala el gasto corregido después del vínculo y desvincula con motivo', async () => {
    const corrected = receipt({
      linked_amount: '500.00',
      linkable_amount: '2945.20',
      cash_link_attention: true,
      cash_links: [
        cashLink({
          status: 'expense_corrected',
          status_label: 'Gasto corregido después del vínculo',
        }),
      ],
    });
    const requests = useTransport((request) => {
      if (request.operation === 'inventory_receipt') {
        return { status: 200, body: corrected };
      }
      if (request.operation === 'inventory_unlink_cash_expense') {
        return { status: 200, body: receipt() };
      }
      return undefined;
    });
    await openTab('Recepciones');
    fireEvent.click(
      await screen.findByRole('button', {
        name: 'Abrir la recepción 5 de Distribuidora del Norte',
      }),
    );
    const section = await screen.findByRole('region', { name: 'Vínculo con gastos de caja' });
    expect(within(section).getByText('Gasto corregido después del vínculo')).toBeTruthy();
    expect(
      within(section).getByText(/Un gasto vinculado se corrigió o regularizó en caja/),
    ).toBeTruthy();
    fireEvent.click(
      within(section).getByRole('button', {
        name: 'Desvincular el gasto Compra de suministros · Proveedores · $850.00',
      }),
    );
    const panel = within(section).getByRole('region', { name: 'Desvincular el gasto' });
    fireEvent.change(within(panel).getByLabelText('Motivo de la desvinculación'), {
      target: { value: 'El gasto se corrigió en caja' },
    });
    fireEvent.click(within(panel).getByRole('button', { name: 'Desvincular' }));
    await confirmDialog('Desvincular');
    const [unlink] = writes(requests, 'inventory_unlink_cash_expense');
    expect(unlink?.resourceId).toBe(purchasingIDs.link);
    expect(unlink?.body).toEqual({
      reason: 'El gasto se corrigió en caja',
      operation_id: expect.any(String),
    });
    await screen.findByText('Gasto desvinculado de la recepción 5.');
  });

  it('sin leer movimientos de efectivo no ofrece vincular', async () => {
    useTransport();
    await openTab(
      'Recepciones',
      purchasingPermissions.filter((permission) => permission !== 'cash.movements.read'),
    );
    fireEvent.click(
      await screen.findByRole('button', {
        name: 'Abrir la recepción 5 de Distribuidora del Norte',
      }),
    );
    const section = await screen.findByRole('region', { name: 'Vínculo con gastos de caja' });
    expect(within(section).queryByRole('button', { name: 'Vincular con un gasto' })).toBeNull();
    expect(
      within(section).getByText(/requiere el permiso «Consultar movimientos de efectivo»/),
    ).toBeTruthy();
  });
});

describe('costos de las recepciones', () => {
  it('sin inventory.costs.read no muestra el costo recibido ni el valor de inventario', async () => {
    useTransport((request) => {
      if (request.operation === 'inventory_receipts') {
        return { status: 200, body: receiptPage([receiptWithoutCosts()], false) };
      }
      if (request.operation === 'inventory_receipt') {
        return { status: 200, body: receiptWithoutCosts() };
      }
      return undefined;
    });
    await openTab(
      'Recepciones',
      purchasingPermissions.filter((permission) => permission !== 'inventory.costs.read'),
    );
    await screen.findByRole('table');
    expect(screen.queryByRole('columnheader', { name: 'Valor de inventario' })).toBeNull();
    // Los importes de compra sí se muestran a quien lee compras.
    expect(screen.getByText('$3,445.20')).toBeTruthy();
    fireEvent.click(
      screen.getByRole('button', { name: 'Abrir la recepción 5 de Distribuidora del Norte' }),
    );
    const detail = await screen.findByRole('region', {
      name: 'Recepción 5 · Distribuidora del Norte',
    });
    expect(within(detail).queryByRole('columnheader', { name: 'Costo recibido' })).toBeNull();
    expect(within(detail).queryByText('Valor de inventario')).toBeNull();
    expect(within(detail).queryByText(/11\.9625/)).toBeNull();
    expect(within(detail).getAllByText('$3,445.20').length).toBeGreaterThan(0);
  });
});

describe('campos numéricos de compras', () => {
  it('ningún formulario de compras usa type="number"', async () => {
    useTransport();
    renderInventory(purchasingPermissions);
    await openInventorySection('Órdenes de compra');
    fireEvent.click(await screen.findByRole('button', { name: 'Nueva orden de compra' }));
    await screen.findByRole('group', { name: 'Renglón 1' });
    expect(document.querySelectorAll('input[type="number"]')).toHaveLength(0);
    await openInventorySection('Recepciones');
    fireEvent.click(await screen.findByRole('button', { name: 'Recepción directa' }));
    await screen.findByRole('group', { name: 'Renglón 1' });
    expect(document.querySelectorAll('input[type="number"]')).toHaveLength(0);
    await openInventorySection('Proveedores');
    fireEvent.click(await screen.findByRole('button', { name: 'Nuevo proveedor' }));
    await screen.findByLabelText(/^Nombre comercial/);
    expect(document.querySelectorAll('input[type="number"]')).toHaveLength(0);
  });
});
