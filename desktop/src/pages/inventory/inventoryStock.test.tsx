// @vitest-environment jsdom

import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { PendingOperationsProvider } from '../../hooks/pendingOperations';
import { notifyInventoryChanged } from '../../hooks/useInventoryEvents';
import { APIError } from '../../services/api';
import { InventoryPage } from './InventoryPage';
import { formatQuantity, formatSignedQuantity } from './inventoryFormat';
import {
  allInventoryPermissions,
  ids,
  inventoryTransport,
  item,
  itemPage,
  movementPage,
  reference,
  settings,
  stockPage,
  tomato,
} from './inventoryTestData';
import type { RecordedRequest, Responder } from './inventoryTestData';
import { conflictMessage } from './useInventoryMutation';
import { openInventorySection, sectionTab } from './inventoryNavigationTestUtils';
import { chooseOptionAsync, chooseValue } from '../../testing/select';

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

function lastQuery(requests: RecordedRequest[], operation: string) {
  return requests.filter((request) => request.operation === operation).at(-1)?.query ?? null;
}

describe('formato de cantidades', () => {
  it('solo agrega miles, signo tipográfico y unidad sin convertir a número', () => {
    const kilogram = reference(ids.kilogram);
    // Cifra y unidad unidas por un espacio de no separación.
    expect(formatQuantity('25000', { code: 'g' })).toBe('25,000\u00a0g');
    expect(formatQuantity('-1.5', kilogram)).toBe('−1.5\u00a0kg');
    expect(formatQuantity('0.000125', kilogram)).toBe('0.000125\u00a0kg');
    expect(formatQuantity('1234567890.123456')).toBe('1,234,567,890.123456');
    expect(formatQuantity('abc', kilogram)).toBe('abc\u00a0kg');
    expect(formatQuantity(null, kilogram)).toBe('—');
    expect(formatSignedQuantity('20', kilogram)).toBe('+20\u00a0kg');
    expect(formatSignedQuantity('-3', kilogram)).toBe('−3\u00a0kg');
    expect(formatSignedQuantity('0', kilogram)).toBe('0\u00a0kg');
  });

  it('explica el conflicto de revisión con los datos ya recargados', () => {
    const problem = new APIError(
      'REVISION_CONFLICT',
      'La configuración cambió en otra terminal. Actualiza y vuelve a guardar.',
      409,
    );
    expect(conflictMessage(problem)).toBe(
      'La configuración cambió en otra terminal. Se cargaron los datos vigentes: revísalos y vuelve a guardar.',
    );
  });
});

describe('existencias', () => {
  it('resalta la existencia negativa sin bloquear y muestra costos y valuación con permiso', async () => {
    useTransport();
    renderInventory(allInventoryPermissions);
    await openInventorySection('Existencias');
    const negative = (await screen.findByText('Tomate')).closest('tr')!;
    expect(negative.className).toContain('inventory-row-negative');
    expect(within(negative).getByText('Existencia negativa')).toBeTruthy();
    expect(within(negative).getAllByText('−1.5 kg').length).toBeGreaterThan(0);
    expect(screen.getByText(/1 insumo de esta página con existencia negativa/)).toBeTruthy();
    const positive = screen.getByText('Harina').closest('tr')!;
    expect(positive.className).not.toContain('inventory-row-negative');
    expect(within(positive).getByText('$18.00 / kg')).toBeTruthy();
    expect(within(positive).getByText('$225.00')).toBeTruthy();
    expect(screen.getByText('Valor total del inventario')).toBeTruthy();
    expect(screen.getByText('$195.00')).toBeTruthy();
    // La existencia inicial está disponible con sus dos permisos.
    expect(screen.getByRole('button', { name: 'Existencia inicial' })).toBeTruthy();
  });

  it('oculta costos y valuación cuando Go no los entrega y filtra por almacén y estado', async () => {
    const requests = useTransport((request) =>
      request.operation === 'inventory_stock'
        ? { status: 200, body: stockPage(false, false) }
        : undefined,
    );
    renderInventory(['inventory.read', 'inventory.stock.read']);
    await openInventorySection('Existencias');
    await screen.findByText('Tomate');
    expect(screen.queryByRole('columnheader', { name: 'Costo promedio' })).toBeNull();
    expect(screen.queryByRole('columnheader', { name: 'Valor del insumo' })).toBeNull();
    expect(screen.queryByText('Valor total del inventario')).toBeNull();
    expect(screen.queryByText(/\$/)).toBeNull();
    expect(screen.queryByRole('button', { name: 'Existencia inicial' })).toBeNull();
    chooseValue(screen.getByLabelText('Almacén'), ids.general);
    chooseValue(screen.getByLabelText('Existencia'), 'negative');
    await waitFor(() =>
      expect(lastQuery(requests, 'inventory_stock')).toEqual({
        page: '1',
        page_size: '25',
        warehouse_id: ids.general,
        state: 'negative',
        active: 'true',
      }),
    );
  });
  it('se recarga con los eventos de inventario y no con los de otras áreas', async () => {
    const requests = useTransport();
    renderInventory(['inventory.read', 'inventory.stock.read']);
    await openInventorySection('Existencias');
    await screen.findByText('Tomate');
    const count = () =>
      requests.filter((request) => request.operation === 'inventory_stock').length;
    const before = count();
    await act(async () => {
      window.dispatchEvent(new CustomEvent('adel-server-change'));
      await new Promise((resolve) => setTimeout(resolve, 150));
    });
    expect(count()).toBe(before);
    act(() => notifyInventoryChanged(['inventory.stock.changed']));
    await waitFor(() => expect(count()).toBe(before + 1));
  });
});

describe('existencia inicial', () => {
  it('envía las cadenas tal como se capturan y confirma el uso del costo de referencia', async () => {
    const requests = useTransport((request) => {
      if (request.operation === 'inventory_items') {
        return { status: 200, body: itemPage([item(), tomato]) };
      }
      if (request.operation === 'inventory_record_initial_balances') {
        return {
          status: 200,
          body: {
            operation_id: request.body?.operation_id,
            warehouse: { id: ids.general, name: 'Almacén general' },
            occurred_at: '2026-11-10T15:00:00Z',
            reason: 'Inventario de apertura',
            costs_visible: true,
            lines: [
              {
                line: 1,
                item_id: ids.flour,
                item_code: 'HAR-01',
                item_name: 'Harina',
                unit: reference(ids.kilogram),
                captured_quantity: '2',
                captured_unit_label: 'Costal 25 kg',
                quantity: '50',
                stock_after: '50',
                movement_id: ids.movement,
                use_reference_cost: true,
                unit_cost: '18.00',
                unit_cost_display: '18.0000',
                value: '900.00',
                value_display: '900.00',
              },
            ],
            warnings: [
              { code: 'NEGATIVE_STOCK', message: 'Aviso de prueba de Go.', item_id: ids.flour },
            ],
          },
        };
      }
      return undefined;
    });
    renderInventory(allInventoryPermissions);
    await openInventorySection('Existencias');
    fireEvent.click(await screen.findByRole('button', { name: 'Existencia inicial' }));
    await screen.findByRole('heading', { name: 'Existencia inicial' });
    fireEvent.change(screen.getByLabelText(/^Motivo u observación/), {
      target: { value: 'Inventario de apertura' },
    });
    // Renglón 1: Harina en costales, al costo de referencia.
    const first = screen.getByRole('group', { name: 'Renglón 1' });
    await chooseOptionAsync(within(first).getByLabelText('Insumo'), 'Harina · HAR-01');
    fireEvent.change(within(first).getByLabelText('Cantidad'), { target: { value: '2' } });
    chooseValue(within(first).getByLabelText('Unidad o presentación'), `presentation:${ids.sack}`);
    expect(within(first).getByText('Costo de referencia: $18.00 por kilogramo.')).toBeTruthy();
    fireEvent.click(within(first).getByRole('button', { name: 'Usar el costo de referencia' }));
    // Renglón 2: Tomate en gramos con su costo capturado; sin costo de referencia no se ofrece.
    fireEvent.click(screen.getByRole('button', { name: 'Agregar renglón' }));
    const second = screen.getByRole('group', { name: 'Renglón 2' });
    await chooseOptionAsync(within(second).getByLabelText('Insumo'), 'Tomate · TOM-01');
    expect(
      (
        within(second).getByRole('button', {
          name: 'Usar el costo de referencia',
        }) as HTMLButtonElement
      ).disabled,
    ).toBe(true);
    fireEvent.change(within(second).getByLabelText('Cantidad'), { target: { value: '1500.250' } });
    chooseValue(within(second).getByLabelText('Unidad o presentación'), `unit:${ids.gram}`);
    fireEvent.change(within(second).getByLabelText(/^Costo unitario/), {
      target: { value: '0.0230' },
    });
    // Cancelar la confirmación no envía nada.
    fireEvent.click(screen.getByRole('button', { name: 'Registrar existencia inicial' }));
    let dialog = await screen.findByRole('alertdialog');
    expect(within(dialog).getByText(/Se usará el costo de referencia configurado en/)).toBeTruthy();
    expect(within(dialog).getByText(/Harina \(\$18\.00 por kilogramo\)/)).toBeTruthy();
    fireEvent.click(within(dialog).getByRole('button', { name: 'Revisar' }));
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull());
    expect(
      requests.some((request) => request.operation === 'inventory_record_initial_balances'),
    ).toBe(false);
    fireEvent.click(screen.getByRole('button', { name: 'Registrar existencia inicial' }));
    dialog = await screen.findByRole('alertdialog');
    expect(
      within(dialog).getByText(/Se registrarán 2 renglones en «Almacén general»/),
    ).toBeTruthy();
    await act(async () => {
      fireEvent.click(within(dialog).getByRole('button', { name: 'Registrar existencia inicial' }));
    });
    const recorded = requests.filter(
      (request) => request.operation === 'inventory_record_initial_balances',
    );
    expect(recorded).toHaveLength(1);
    expect(recorded[0]?.body).toEqual({
      warehouse_id: ids.general,
      reason: 'Inventario de apertura',
      lines: [
        {
          item_id: ids.flour,
          quantity: '2',
          presentation_id: ids.sack,
          use_reference_cost: true,
        },
        {
          item_id: ids.tomato,
          quantity: '1500.250',
          unit_id: ids.gram,
          unit_cost: '0.0230',
        },
      ],
      operation_id: expect.any(String),
    });
    // Resultado de Go: renglones convertidos y sus advertencias.
    await screen.findByRole('heading', { name: 'Existencia inicial registrada' });
    const summary = screen.getByRole('region', { name: 'Existencia inicial registrada' });
    expect(within(summary).getByText('2 · Costal 25 kg')).toBeTruthy();
    expect(within(summary).getAllByText('50 kg').length).toBe(2);
    expect(within(summary).getByText('$900.00')).toBeTruthy();
    expect(within(summary).getByText('Aviso de prueba de Go.')).toBeTruthy();
  });
});

describe('movimientos', () => {
  it('muestra el libro sin identificadores y aplica los filtros', async () => {
    const requests = useTransport((request) =>
      request.operation === 'inventory_movements'
        ? { status: 200, body: movementPage(false) }
        : undefined,
    );
    renderInventory(['inventory.read', 'inventory.movements.read']);
    await openInventorySection('Movimientos');
    const row = (await screen.findByText('+12.5 kg')).closest('tr')!;
    expect(within(row).getAllByText('Existencia inicial').length).toBeGreaterThan(0);
    expect(within(row).getByText('Administrador')).toBeTruthy();
    expect(within(row).getByText('Inventario de apertura')).toBeTruthy();
    expect(within(row).getByText(/^Registrado:/)).toBeTruthy();
    // Sin consulta de costos no hay columna de valor; nunca se muestran identificadores crudos.
    expect(screen.queryByRole('columnheader', { name: 'Valor' })).toBeNull();
    expect(document.body.textContent).not.toContain(ids.operation);
    expect(document.body.textContent).not.toContain(ids.movement);
    await chooseOptionAsync(screen.getByLabelText('Insumo'), 'Harina · HAR-01');
    chooseValue(screen.getByLabelText('Almacén'), ids.general);
    chooseValue(screen.getByLabelText('Tipo de movimiento'), 'initial');
    chooseValue(screen.getByLabelText('Origen'), 'initial');
    fireEvent.change(screen.getByLabelText('Fecha de negocio desde'), {
      target: { value: '01112026' },
    });
    fireEvent.change(screen.getByLabelText('Fecha de negocio hasta'), {
      target: { value: '10112026' },
    });
    await waitFor(() =>
      expect(lastQuery(requests, 'inventory_movements')).toEqual({
        page: '1',
        page_size: '50',
        item_id: ids.flour,
        warehouse_id: ids.general,
        kind: 'initial',
        source_kind: 'initial',
        from: '2026-11-01',
        until: '2026-11-10',
      }),
    );
  });

  it('muestra valor, costo y base de costo con permiso de costos', async () => {
    useTransport();
    renderInventory(['inventory.read', 'inventory.movements.read', 'inventory.costs.read']);
    await openInventorySection('Movimientos');
    const row = (await screen.findByText('+12.5 kg')).closest('tr')!;
    expect(screen.getByRole('columnheader', { name: 'Valor' })).toBeTruthy();
    expect(within(row).getByText('+$225.00')).toBeTruthy();
    expect(within(row).getByText('$18.00 / kg')).toBeTruthy();
    expect(within(row).getByText('Costo del documento')).toBeTruthy();
  });
});

describe('configuración', () => {
  it('ante un conflicto de revisión recarga la versión vigente y lo explica', async () => {
    let current = settings();
    const requests = useTransport((request) => {
      if (request.operation === 'inventory_settings') {
        return { status: 200, body: current };
      }
      if (request.operation === 'inventory_save_settings') {
        // Otra terminal guardó antes: la revisión enviada quedó obsoleta.
        current = settings({
          revision: 2,
          operating_day_cutoff: '04:30',
          count_frequency: 'weekly',
          count_frequency_label: 'Semanal',
        });
        return {
          status: 409,
          body: {
            error: {
              code: 'REVISION_CONFLICT',
              message: 'La configuración cambió en otra terminal. Actualiza y vuelve a guardar.',
            },
          },
        };
      }
      return undefined;
    });
    renderInventory(['inventory.read', 'inventory.settings.manage']);
    await openInventorySection('Parámetros');
    // Etapa 9A: la hora de inicio del día se muestra en solo lectura (se cambia en Configuración).
    const cutoff = (await screen.findByLabelText(/^Hora de inicio del día/)) as HTMLInputElement;
    expect(cutoff.value).toBe('05:00');
    expect(cutoff.readOnly).toBe(true);
    fireEvent.click(screen.getByRole('button', { name: 'Cada cierto número de días' }));
    fireEvent.change(screen.getByLabelText(/^Cada cuántos días/), { target: { value: '15' } });
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'Guardar configuración' }));
    });
    const [save] = requests.filter((request) => request.operation === 'inventory_save_settings');
    expect(save?.body).toEqual({
      revision: 1,
      purchase_tax_in_cost: true,
      warn_products_without_recipe: true,
      count_frequency: 'custom',
      count_interval_days: 15,
      operation_id: expect.any(String),
    });
    await screen.findByText(/Se cargaron los datos vigentes: revísalos y vuelve a guardar\./);
    await waitFor(() =>
      expect((screen.getByLabelText(/^Hora de inicio del día/) as HTMLInputElement).value).toBe(
        '04:30',
      ),
    );
    expect(screen.getByRole('button', { name: 'Semanal' }).getAttribute('aria-pressed')).toBe(
      'true',
    );
    expect(screen.queryByLabelText(/^Cada cuántos días/)).toBeNull();
  });
});

describe('campos numéricos', () => {
  it('ninguna sección usa type="number"', async () => {
    useTransport();
    renderInventory(allInventoryPermissions);
    for (const tab of [
      'Insumos',
      'Almacenes',
      'Existencias',
      'Movimientos',
      'Unidades',
      'Categorías',
      'Parámetros',
    ]) {
      await openInventorySection(tab);
      await waitFor(() => expect(sectionTab(tab).getAttribute('aria-selected')).toBe('true'));
      expect(document.querySelectorAll('input[type="number"]')).toHaveLength(0);
    }
    await openInventorySection('Resumen');
    expect(document.querySelectorAll('input[type="number"]')).toHaveLength(0);
    await openInventorySection('Existencias');
    fireEvent.click(await screen.findByRole('button', { name: 'Existencia inicial' }));
    await screen.findByRole('heading', { name: 'Existencia inicial' });
    expect(document.querySelectorAll('input[type="number"]')).toHaveLength(0);
    // Y ningún archivo de la sección lo declara.
    const sources = import.meta.glob<string>(['./*.tsx', '!./*.test.tsx'], {
      query: '?raw',
      import: 'default',
      eager: true,
    });
    expect(Object.keys(sources).length).toBeGreaterThan(8);
    for (const [name, source] of Object.entries(sources)) {
      expect(source, name).not.toContain('type="number"');
    }
  });
});
