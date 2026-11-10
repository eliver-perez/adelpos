// @vitest-environment jsdom

import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { App } from '../../App';
import { PendingOperationsProvider } from '../../hooks/pendingOperations';
import type { Identity } from '../../services/models';
import { screenAuthorized, screens } from '../PreferencesPage';
import { InventoryPage } from './InventoryPage';
import {
  allInventoryPermissions,
  ids,
  inventoryTransport,
  item,
  itemPage,
  reference,
  stations,
  summary,
  tomato,
} from './inventoryTestData';
import type { RecordedRequest, Responder } from './inventoryTestData';

const native = vi.hoisted(() => ({ invoke: vi.fn() }));
vi.mock('@tauri-apps/api/core', () => ({ invoke: native.invoke, isTauri: () => true }));

afterEach(() => {
  cleanup();
  native.invoke.mockReset();
});

function useTransport(override?: Responder) {
  const transport = inventoryTransport(override);
  native.invoke.mockImplementation(transport.handler);
  return transport.requests;
}

function renderInventory(permissions: string[], onOpenSettings = vi.fn()) {
  return render(
    <PendingOperationsProvider>
      <InventoryPage
        can={(permission) => permissions.includes(permission)}
        timezone="America/Monterrey"
        onOpenSettings={onOpenSettings}
      />
    </PendingOperationsProvider>,
  );
}

function writes(requests: RecordedRequest[], operation: string) {
  return requests.filter((request) => request.operation === operation);
}

function tabNames() {
  return within(screen.getByRole('tablist', { name: 'Secciones de inventario' }))
    .getAllByRole('tab')
    .map((tab) => tab.textContent);
}

describe('navegación de Inventarios', () => {
  it('la pantalla solo aparece con inventory.read', () => {
    const visible = (permissions: string[]) =>
      screens
        .filter((entry) => screenAuthorized(entry, permissions))
        .map((entry) => entry.id)
        .includes('inventory');
    expect(visible(['inventory.read'])).toBe(true);
    expect(visible(['inventory.stock.read', 'inventory.items.manage'])).toBe(false);
    expect(visible([])).toBe(false);
  });

  it('el menú principal muestra Inventarios con el permiso y abre su resumen', async () => {
    const identity: Identity = {
      user: {
        id: '00000000-0000-0000-0000-000000000001',
        username: 'inventarios',
        display_name: 'Encargada de inventario',
        role_id: '00000000-0000-0000-0000-000000000002',
        role_name: 'Inventarios',
        active: true,
        initial_password: false,
        support: false,
        session_id: '00000000-0000-0000-0000-000000000003',
        device_id: '00000000-0000-0000-0000-000000000004',
        permissions: ['inventory.read'],
      },
      preferences: { theme: 'light', home_screen: 'status' },
      terminal: {
        id: '00000000-0000-0000-0000-000000000004',
        name: 'Terminal de prueba',
        register_id: null,
        keyboard_mode: 'disabled',
      },
      establishment: { name: 'ADEL POS', timezone: 'America/Monterrey', currency: 'MXN' },
    };
    let permissions = ['inventory.read'];
    const transport = inventoryTransport();
    native.invoke.mockImplementation(async (command: string, args: RecordedRequest) => {
      if (command === 'authentication_state') {
        return {
          enrolled: true,
          has_session: true,
          local_setup_available: false,
          local_enrollment_available: false,
        };
      }
      if (command === 'connection_config') {
        return { installation_mode: 'server-terminal', server_url: 'http://127.0.0.1:8181' };
      }
      if (command === 'server_snapshot') {
        return {
          live: { status: 200, body: { status: 'alive' } },
          ready: { status: 200, body: { status: 'ready', checks: {} } },
          version: { status: 200, body: {} },
        };
      }
      if (args?.operation === 'setup_status') {
        return { status: 200, body: { configured: true } };
      }
      if (args?.operation === 'me') {
        return { status: 200, body: { ...identity, user: { ...identity.user, permissions } } };
      }
      return transport.handler(command, args);
    });
    const { unmount } = render(<App />);
    const navigation = await screen.findByRole('navigation', { name: 'Navegación principal' });
    fireEvent.click(within(navigation).getByRole('button', { name: 'Inventarios' }));
    await screen.findByRole('heading', { name: 'Inventarios' });
    await screen.findByText('Control de inventario');
    expect(tabNames()).toEqual(['Resumen', 'Insumos', 'Recetas', 'Almacenes', 'Ajustes y mermas']);
    unmount();
    permissions = ['inventory.stock.read'];
    render(<App />);
    const other = await screen.findByRole('navigation', { name: 'Navegación principal' });
    expect(within(other).queryByRole('button', { name: 'Inventarios' })).toBeNull();
  });
});

describe('pestañas y resumen', () => {
  it('muestra solo las pestañas que permiten los permisos', async () => {
    useTransport();
    const { unmount } = renderInventory(['inventory.read']);
    await screen.findByText('Control de inventario');
    expect(tabNames()).toEqual(['Resumen', 'Insumos', 'Recetas', 'Almacenes', 'Ajustes y mermas']);
    unmount();
    renderInventory(allInventoryPermissions);
    await screen.findByText('Control de inventario');
    expect(tabNames()).toEqual([
      'Resumen',
      'Insumos',
      'Recetas',
      'Almacenes',
      'Existencias',
      'Movimientos',
      'Ajustes y mermas',
      'Configuración',
    ]);
  });

  it('el resumen explica la activación y lleva a la administración de funciones', async () => {
    useTransport((request) =>
      request.operation === 'inventory_summary'
        ? {
            status: 200,
            body: summary({
              feature: {
                code: 'inventory_control',
                name: 'Control de inventario',
                enabled: true,
                enabled_at: '2026-11-10T12:00:00Z',
                revision: 2,
              },
              costs_visible: false,
              pending_cost_items: null,
              valuation_visible: false,
              total_value: null,
              total_value_display: null,
            }),
          }
        : undefined,
    );
    const openSettings = vi.fn();
    renderInventory(
      ['inventory.read', 'inventory.stock.read', 'settings.read', 'settings.features.manage'],
      openSettings,
    );
    await screen.findByText(/^Activado desde el /);
    expect(screen.getByText(/solo lo que se libera a cocina después de activarlo/)).toBeTruthy();
    // Sin un segundo interruptor: se lleva a la pantalla existente.
    expect(screen.queryByRole('button', { name: 'Control de inventario' })).toBeNull();
    fireEvent.click(screen.getByRole('button', { name: 'Administrar funciones' }));
    expect(openSettings).toHaveBeenCalledTimes(1);
    const figures = screen.getByRole('region', { name: 'Cifras de inventario' });
    expect(within(figures).getByText('Existencias negativas')).toBeTruthy();
    // Sin permisos de costos ni de valuación Go no entrega esas cifras.
    expect(within(figures).queryByText('Valor del inventario')).toBeNull();
    expect(within(figures).queryByText('Con costo pendiente')).toBeNull();
    // La cifra de negativas abre Existencias filtrada.
    fireEvent.click(within(figures).getByRole('button', { name: /Existencias negativas/ }));
    await screen.findByRole('region', { name: 'Filtros de existencias' });
    expect((screen.getByLabelText('Existencia') as HTMLSelectElement).value).toBe('negative');
  });
});

describe('insumos', () => {
  it('crea un insumo y le agrega una presentación capturada en otra unidad', async () => {
    const created = item({
      id: ids.created,
      code: 'QUE-01',
      name: 'Queso',
      base_unit: reference(ids.gram),
      category_id: ids.groceries,
      reference_cost: '0.0185',
      reference_cost_display: '0.0185',
      min_stock: '2500',
      presentations: [],
      revision: 1,
    });
    const withPresentation = {
      ...created,
      presentations: [
        {
          id: ids.sack,
          item_id: ids.created,
          name: 'Bloque 2.5 kg',
          base_quantity: '2500',
          barcode: null,
          active: true,
          purchase_default: true,
          revision: 1,
          updated_at: created.updated_at,
        },
      ],
    };
    let presentationSaved = false;
    const requests = useTransport((request) => {
      if (request.operation === 'inventory_create_item') {
        return { status: 200, body: created };
      }
      if (request.operation === 'inventory_create_presentation') {
        presentationSaved = true;
        return { status: 200, body: withPresentation };
      }
      if (request.operation === 'inventory_item' && request.resourceId === ids.created) {
        return { status: 200, body: presentationSaved ? withPresentation : created };
      }
      return undefined;
    });
    renderInventory(allInventoryPermissions);
    fireEvent.click(await screen.findByRole('tab', { name: 'Insumos' }));
    await screen.findByText('Harina');
    expect(screen.getByRole('columnheader', { name: 'Costo de referencia' })).toBeTruthy();
    expect(screen.getByText('$18.00 / kg')).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: 'Nuevo insumo' }));
    fireEvent.change(screen.getByLabelText('Nombre del insumo'), { target: { value: 'Queso' } });
    fireEvent.change(screen.getByLabelText('Código interno'), { target: { value: 'QUE-01' } });
    fireEvent.change(screen.getByLabelText(/^Categoría/), { target: { value: ids.groceries } });
    fireEvent.change(screen.getByLabelText(/^Unidad base/), { target: { value: ids.gram } });
    fireEvent.change(screen.getByLabelText(/^Costo de referencia/), {
      target: { value: '0.0185' },
    });
    // Un carácter no válido se descarta al teclear: nunca se convierte a número.
    fireEvent.change(screen.getByLabelText(/^Existencia mínima/), { target: { value: '2500x' } });
    fireEvent.change(screen.getByLabelText(/^Existencia mínima/), { target: { value: '2500' } });
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'Crear insumo' }));
    });
    const [create] = writes(requests, 'inventory_create_item');
    expect(create?.body).toEqual({
      code: 'QUE-01',
      barcode: null,
      name: 'Queso',
      description: '',
      category_id: ids.groceries,
      base_unit_id: ids.gram,
      kind: 'purchased',
      active: true,
      min_stock: '2500',
      reorder_point: null,
      reference_cost: '0.0185',
      operation_id: expect.any(String),
    });
    // El panel continúa con el insumo creado para agregar sus presentaciones.
    await screen.findByRole('heading', { name: 'Queso' });
    expect(
      screen.getByText('Sin presentaciones: se compra y se cuenta en la unidad base.'),
    ).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: 'Nueva presentación' }));
    fireEvent.change(screen.getByLabelText(/^Nombre de la presentación/), {
      target: { value: 'Bloque 2.5 kg' },
    });
    fireEvent.change(screen.getByLabelText(/^Contenido/), { target: { value: '2.5' } });
    fireEvent.change(screen.getByLabelText(/^Unidad del contenido/), {
      target: { value: ids.kilogram },
    });
    fireEvent.click(screen.getByRole('button', { name: 'Predeterminada para compra' }));
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'Agregar presentación' }));
    });
    const [presentation] = writes(requests, 'inventory_create_presentation');
    expect(presentation?.resourceId).toBe(ids.created);
    expect(presentation?.body).toEqual({
      name: 'Bloque 2.5 kg',
      quantity: '2.5',
      unit_id: ids.kilogram,
      barcode: null,
      active: true,
      purchase_default: true,
      operation_id: expect.any(String),
    });
    // La equivalencia la convierte Go: la pantalla muestra lo que devuelve en la unidad base.
    await screen.findByText('Bloque 2.5 kg');
    expect(screen.getByText('2,500 g')).toBeTruthy();
    expect(screen.getByText('Predeterminada para compra', { selector: '.tag' })).toBeTruthy();
  });

  it('oculta el costo de referencia sin costs_visible y no lo envía', async () => {
    const requests = useTransport((request) => {
      if (request.operation === 'inventory_items') {
        return { status: 200, body: itemPage([item(), tomato], false) };
      }
      if (request.operation === 'inventory_item') {
        return {
          status: 200,
          body: item({ costs_visible: false, reference_cost: null, reference_cost_display: null }),
        };
      }
      if (request.operation === 'inventory_save_item') {
        return { status: 200, body: item({ costs_visible: false, reference_cost: null }) };
      }
      return undefined;
    });
    // Puede administrar costos pero no consultarlos: Go no entrega el costo y la pantalla no lo
    // muestra ni lo reemplaza.
    renderInventory(['inventory.read', 'inventory.items.manage', 'inventory.costs.manage']);
    fireEvent.click(await screen.findByRole('tab', { name: 'Insumos' }));
    await screen.findByText('Harina');
    expect(screen.queryByRole('columnheader', { name: 'Costo de referencia' })).toBeNull();
    fireEvent.click(screen.getByRole('button', { name: 'Editar el insumo Harina' }));
    await screen.findByLabelText('Nombre del insumo');
    expect(screen.queryByLabelText(/^Costo de referencia/)).toBeNull();
    fireEvent.change(screen.getByLabelText('Nombre del insumo'), {
      target: { value: 'Harina de trigo' },
    });
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'Guardar insumo' }));
    });
    const [save] = writes(requests, 'inventory_save_item');
    expect(save?.resourceId).toBe(ids.flour);
    expect(save?.body).toMatchObject({ name: 'Harina de trigo', revision: 3, kind: 'purchased' });
    expect(save?.body).not.toHaveProperty('reference_cost');
  });

  it('confirma antes de desactivar un insumo y pregunta al salir con cambios', async () => {
    useTransport();
    renderInventory(allInventoryPermissions);
    fireEvent.click(await screen.findByRole('tab', { name: 'Insumos' }));
    fireEvent.click(await screen.findByRole('button', { name: 'Editar el insumo Harina' }));
    const toggle = await screen.findByRole('button', { name: 'Insumo activo' });
    fireEvent.click(toggle);
    const dialog = await screen.findByRole('alertdialog');
    expect(within(dialog).getByText(/dejará de ofrecerse/)).toBeTruthy();
    fireEvent.click(within(dialog).getByRole('button', { name: 'Mantener activo' }));
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull());
    expect(toggle.getAttribute('aria-pressed')).toBe('true');
    fireEvent.change(screen.getByLabelText('Nombre del insumo'), { target: { value: 'Otra' } });
    fireEvent.click(screen.getByRole('button', { name: 'Volver al listado' }));
    const discard = await screen.findByRole('alertdialog');
    fireEvent.click(within(discard).getByRole('button', { name: 'Descartar cambios' }));
    await screen.findByRole('region', { name: 'Filtros de insumos' });
  });
});

describe('almacenes', () => {
  it('marca el predeterminado y asigna el almacén de una estación', async () => {
    const requests = useTransport((request) => {
      if (request.operation === 'inventory_assign_station_warehouse') {
        return {
          status: 200,
          body: {
            ...stations[0],
            warehouse_id: ids.bar,
            warehouse_name: 'Barra',
            uses_default: false,
            revision: 1,
          },
        };
      }
      return undefined;
    });
    renderInventory(['inventory.read', 'inventory.warehouses.manage']);
    fireEvent.click(await screen.findByRole('tab', { name: 'Almacenes' }));
    await screen.findByText('Predeterminado');
    fireEvent.click(screen.getByRole('button', { name: 'Editar el almacén Almacén general' }));
    expect(
      (screen.getByRole('button', { name: 'Almacén activo' }) as HTMLButtonElement).disabled,
    ).toBe(true);
    fireEvent.click(screen.getByRole('button', { name: 'Cancelar' }));
    // La estación sin asignación usa el predeterminado; la otra tiene historial plegable.
    expect(screen.getByText('Almacén general (predeterminado)')).toBeTruthy();
    expect(screen.getByRole('button', { name: /Historial de asignaciones/ })).toBeTruthy();
    fireEvent.change(screen.getByLabelText('Almacén de Cocina caliente'), {
      target: { value: ids.bar },
    });
    await act(async () => {
      fireEvent.click(screen.getAllByRole('button', { name: 'Asignar' })[0]!);
    });
    const [assign] = writes(requests, 'inventory_assign_station_warehouse');
    expect(assign?.resourceId).toBe(ids.hotStation);
    expect(assign?.body).toEqual({
      revision: 0,
      warehouse_id: ids.bar,
      operation_id: expect.any(String),
    });
    await screen.findByText('Cocina caliente consume ahora de «Barra».');
  });

  it('desactivar un almacén pide confirmación de peligro', async () => {
    const requests = useTransport();
    renderInventory(['inventory.read', 'inventory.warehouses.manage']);
    fireEvent.click(await screen.findByRole('tab', { name: 'Almacenes' }));
    fireEvent.click(await screen.findByRole('button', { name: 'Editar el almacén Barra' }));
    fireEvent.click(screen.getByRole('button', { name: 'Almacén activo' }));
    const dialog = await screen.findByRole('alertdialog');
    expect(dialog.className).toContain('danger');
    fireEvent.click(within(dialog).getByRole('button', { name: 'Desactivar' }));
    await waitFor(() =>
      expect(
        screen.getByRole('button', { name: 'Almacén activo' }).getAttribute('aria-pressed'),
      ).toBe('false'),
    );
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'Guardar almacén' }));
    });
    const [save] = writes(requests, 'inventory_save_warehouse');
    expect(save?.body).toEqual({
      name: 'Barra',
      description: '',
      active: false,
      revision: 2,
      operation_id: expect.any(String),
    });
  });
});
