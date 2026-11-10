// @vitest-environment jsdom

import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { PendingOperationsProvider } from '../../hooks/pendingOperations';
import { InventoryPage } from './InventoryPage';
import { allInventoryPermissions, inventoryTransport, movementPage } from './inventoryTestData';
import type { RecordedRequest, Responder } from './inventoryTestData';
import {
  automaticCancellation,
  consumptionMovement,
  pendingCancellation,
  pendingPage,
  recipeIDs,
  requestPage,
  unitTrace,
} from './inventoryRecipesTestData';

const native = vi.hoisted(() => ({ invoke: vi.fn() }));
vi.mock('@tauri-apps/api/core', () => ({ invoke: native.invoke, isTauri: () => true }));

afterEach(() => {
  cleanup();
  native.invoke.mockReset();
});

const uuidPattern = /[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/;

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

// Las pendientes para la consulta state=pending y el historial completo para las demás.
function cancellationResponder(
  pending: ReturnType<typeof pendingCancellation>[],
  history = pending,
) {
  return (request: RecordedRequest) => {
    if (request.operation !== 'inventory_cancellations') {
      return undefined;
    }
    if (request.query?.state === 'pending') {
      return { status: 200, body: pendingPage(pending) };
    }
    return { status: 200, body: pendingPage(history) };
  };
}

describe('cancelaciones por clasificar', () => {
  it('muestra la partida, la cuenta, el día operativo y el límite, y clasifica «No preparado»', async () => {
    const classified = pendingCancellation({
      state: 'not_prepared',
      state_label: 'No preparado',
      resolution: 'manual',
      resolution_label: 'Clasificada por el encargado',
      revision: 2,
    });
    const requests = useTransport((request) => {
      if (request.operation === 'inventory_classify_cancellation') {
        return { status: 200, body: classified };
      }
      return cancellationResponder([pendingCancellation()])(request);
    });
    renderInventory(allInventoryPermissions);
    fireEvent.click(await screen.findByRole('tab', { name: 'Ajustes y mermas' }));
    const card = await screen.findByRole('article', {
      name: 'Cancelación de Hamburguesa de prueba',
    });
    expect(within(card).getByText('Cuenta #12 · Mesa 4')).toBeTruthy();
    expect(within(card).getByText('10/11/2026')).toBeTruthy();
    expect(within(card).getByText('Límite para clasificar')).toBeTruthy();
    // Consumido y neto vigente, tal como los entrega Go.
    expect(within(card).getAllByText('0.18 kg', { selector: 'td' })).toHaveLength(2);
    expect(within(card).getByText('$32.40')).toBeTruthy();
    expect(screen.getByText('1 pendiente')).toBeTruthy();
    expect(card.textContent ?? '').not.toMatch(uuidPattern);
    await act(async () => {
      fireEvent.click(within(card).getByRole('button', { name: 'No preparado' }));
    });
    const [classify] = writes(requests, 'inventory_classify_cancellation');
    expect(classify?.resourceId).toBe(recipeIDs.pendingCancellation);
    expect(classify?.body).toEqual({
      operation_id: expect.any(String),
      revision: 1,
      state: 'not_prepared',
      notes: null,
    });
    await screen.findByText('Hamburguesa de prueba: No preparado.');
    expect(screen.getByText('Su consumo se revirtió con movimientos compensatorios.')).toBeTruthy();
  });

  it('los botones dependen del permiso que Go exige', async () => {
    useTransport(cancellationResponder([pendingCancellation({ can_classify: false })]));
    renderInventory(['inventory.read']);
    fireEvent.click(await screen.findByRole('tab', { name: 'Ajustes y mermas' }));
    const card = await screen.findByRole('article', {
      name: 'Cancelación de Hamburguesa de prueba',
    });
    expect(within(card).queryByRole('button', { name: 'No preparado' })).toBeNull();
    expect(within(card).queryByRole('button', { name: 'Preparado (merma)' })).toBeNull();
    expect(
      within(card).getByText(
        /Clasificarla requiere el permiso «Registrar mermas y clasificar cancelaciones»/,
      ),
    ).toBeTruthy();
    // Sin consulta de movimientos no se ofrece la traza.
    expect(within(card).queryByRole('button', { name: 'Ver consumo de la partida' })).toBeNull();
  });

  it('después del corte solo se corrige con confirmación de peligro', async () => {
    const expired = pendingCancellation({
      expired: true,
      required_permission: 'inventory.adjustments.authorize',
    });
    const requests = useTransport((request) => {
      if (request.operation === 'inventory_classify_cancellation') {
        return {
          status: 200,
          body: { ...expired, state: 'prepared_waste', state_label: 'Preparado / merma' },
        };
      }
      return cancellationResponder([expired])(request);
    });
    renderInventory(allInventoryPermissions);
    fireEvent.click(await screen.findByRole('tab', { name: 'Ajustes y mermas' }));
    const card = await screen.findByRole('article', {
      name: 'Cancelación de Hamburguesa de prueba',
    });
    expect(within(card).getByText(/Pasó el corte del día operativo/)).toBeTruthy();
    fireEvent.click(within(card).getByRole('button', { name: 'Corregir: Preparado (merma)' }));
    const dialog = await screen.findByRole('alertdialog');
    expect(dialog.className).toContain('danger');
    fireEvent.click(within(dialog).getByRole('button', { name: 'Revisar' }));
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull());
    expect(writes(requests, 'inventory_classify_cancellation')).toHaveLength(0);
    fireEvent.click(within(card).getByRole('button', { name: 'Corregir: Preparado (merma)' }));
    const again = await screen.findByRole('alertdialog');
    await act(async () => {
      fireEvent.click(within(again).getByRole('button', { name: 'Registrar corrección' }));
    });
    expect(writes(requests, 'inventory_classify_cancellation')[0]?.body).toMatchObject({
      revision: 1,
      state: 'prepared_waste',
    });
  });

  it('el historial muestra las automáticas «Por cierre del día operativo» y corrige con motivo', async () => {
    const automatic = automaticCancellation();
    const requests = useTransport((request) => {
      if (request.operation === 'inventory_classify_cancellation') {
        return {
          status: 200,
          body: automaticCancellation({
            state: 'prepared_waste',
            state_label: 'Preparado / merma',
            resolution: 'correction',
            resolution_label: 'Corrección autorizada',
            revision: 3,
          }),
        };
      }
      return cancellationResponder([], [automatic])(request);
    });
    renderInventory(allInventoryPermissions);
    fireEvent.click(await screen.findByRole('tab', { name: 'Ajustes y mermas' }));
    await screen.findByText('No hay cancelaciones por clasificar.');
    const history = screen.getByRole('region', { name: 'Historial de clasificaciones' });
    const row = (await within(history).findByText('Filete de prueba')).closest('tr')!;
    expect(within(row).getByText('Pedido #7')).toBeTruthy();
    expect(within(row).getByText('Por cierre del día operativo')).toBeTruthy();
    expect(within(row).getByText('Automática al cierre del día operativo')).toBeTruthy();
    fireEvent.click(
      within(row).getByRole('button', { name: 'Corregir la clasificación de Filete de prueba' }),
    );
    const panel = screen.getByRole('region', {
      name: 'Corrección de la clasificación de Filete de prueba',
    });
    fireEvent.change(within(panel).getByLabelText(/^Motivo del cambio/), {
      target: { value: 'Sí se preparó' },
    });
    fireEvent.click(within(panel).getByRole('button', { name: 'Aplicar corrección' }));
    const dialog = await screen.findByRole('alertdialog');
    expect(dialog.className).toContain('danger');
    expect(within(dialog).getByText(/de «No preparado» a «Preparado \/ merma»/)).toBeTruthy();
    await act(async () => {
      fireEvent.click(within(dialog).getByRole('button', { name: 'Registrar corrección' }));
    });
    expect(writes(requests, 'inventory_classify_cancellation')[0]?.body).toEqual({
      operation_id: expect.any(String),
      revision: 2,
      state: 'prepared_waste',
      notes: 'Sí se preparó',
    });
    // Filtros del historial por clasificación.
    fireEvent.change(within(history).getByLabelText('Clasificación'), {
      target: { value: 'not_prepared' },
    });
    await waitFor(() =>
      expect(
        requests.filter((request) => request.operation === 'inventory_cancellations').at(-1)?.query,
      ).toMatchObject({ state: 'not_prepared' }),
    );
  });
});

describe('diagnóstico del consumo en Movimientos', () => {
  it('muestra las solicitudes con error sin datos técnicos y abre la traza de la partida', async () => {
    const requests = useTransport((request) => {
      if (request.operation === 'inventory_consumption_requests') {
        return { status: 200, body: requestPage() };
      }
      if (request.operation === 'inventory_unit_consumption') {
        return { status: 200, body: unitTrace(false) };
      }
      return undefined;
    });
    renderInventory(['inventory.read', 'inventory.movements.read']);
    fireEvent.click(await screen.findByRole('tab', { name: 'Movimientos' }));
    // Con solicitudes fallidas el acordeón se abre solo.
    const trigger = await screen.findByRole('button', { name: /Solicitudes de consumo/ });
    await waitFor(() => expect(trigger.getAttribute('aria-expanded')).toBe('true'));
    expect(trigger.textContent).toContain('2 pendientes · 1 con error');
    const row = (await screen.findByText('Con error (se reintenta)')).closest('tr')!;
    expect(within(row).getByText('Pedido #12 · Para llevar')).toBeTruthy();
    expect(within(row).getByText('3 intentos', { exact: false })).toBeTruthy();
    expect(
      within(row).getByText('No se pudo calcular el consumo; se reintentará automáticamente.'),
    ).toBeTruthy();
    expect(
      within(row).getByText('Un ingrediente del catálogo no tiene porción habitual: no consumió.'),
    ).toBeTruthy();
    expect(
      within(row).getByText(
        'Componente 2 del combo: Sin receta ni porciones configuradas: no consumió inventario.',
      ),
    ).toBeTruthy();
    expect(row.textContent ?? '').not.toMatch(uuidPattern);
    expect(row.textContent ?? '').not.toMatch(/ingredient_without_portion|root/);

    fireEvent.click(
      within(row).getByRole('button', { name: 'Ver consumo de la partida Hamburguesa de prueba' }),
    );
    const trace = await screen.findByRole('region', { name: 'Consumo de la partida' });
    await within(trace).findByText('Consumo registrado');
    expect(writes(requests, 'inventory_unit_consumption')[0]?.resourceId).toBe(recipeIDs.unit);
    expect(within(trace).getByText(/Pedido #12 · liberada el .* · Para llevar/)).toBeTruthy();
    expect(within(trace).getAllByText('Carne molida').length).toBeGreaterThan(0);
    expect(within(trace).getByText('−0.18 kg')).toBeTruthy();
    expect(within(trace).getByText('Liberación a cocina · Estación: Cocina caliente')).toBeTruthy();
    // Sin costos, la traza no muestra valores.
    expect(within(trace).queryByRole('columnheader', { name: /Valor/ })).toBeNull();
    expect(trace.textContent ?? '').not.toMatch(uuidPattern);
    fireEvent.click(within(trace).getByRole('button', { name: 'Volver a movimientos' }));
    await screen.findByRole('region', { name: 'Filtros de movimientos' });
  });

  it('desde un movimiento de consumo abre la traza con valores si hay costos', async () => {
    const requests = useTransport((request) => {
      if (request.operation === 'inventory_movements') {
        const page = movementPage(true);
        return {
          status: 200,
          body: { ...page, items: [consumptionMovement(true), ...page.items], total: 2 },
        };
      }
      if (request.operation === 'inventory_unit_consumption') {
        return { status: 200, body: unitTrace(true) };
      }
      return undefined;
    });
    renderInventory(allInventoryPermissions);
    fireEvent.click(await screen.findByRole('tab', { name: 'Movimientos' }));
    const button = await screen.findByRole('button', {
      name: 'Ver consumo de la partida del movimiento de Carne molida',
    });
    // Solo los movimientos ligados a una partida ofrecen la traza.
    expect(
      screen.getAllByRole('button', { name: /^Ver consumo de la partida del movimiento/ }),
    ).toHaveLength(1);
    // El filtro de origen incluye los de la fase B.
    const origin = screen.getByLabelText('Origen') as HTMLSelectElement;
    expect(Array.from(origin.options).map((option) => option.textContent)).toContain(
      'Liberación a cocina',
    );
    fireEvent.click(button);
    const trace = await screen.findByRole('region', { name: 'Consumo de la partida' });
    await within(trace).findByRole('columnheader', { name: 'Valor neto' });
    expect(within(trace).getByText('$32.40')).toBeTruthy();
    expect(within(trace).getByText('−$32.40')).toBeTruthy();
    expect(writes(requests, 'inventory_unit_consumption')).toHaveLength(1);
  });
});

describe('resumen de la fase B', () => {
  it('muestra las cancelaciones por clasificar y las solicitudes con error que entrega Go', async () => {
    useTransport((request) => {
      if (request.operation === 'inventory_consumption_requests') {
        return { status: 200, body: requestPage() };
      }
      return cancellationResponder([pendingCancellation()])(request);
    });
    renderInventory(allInventoryPermissions);
    const figures = await screen.findByRole('region', { name: 'Cifras de inventario' });
    const cancellations = await within(figures).findByRole('button', {
      name: /Cancelaciones por clasificar/,
    });
    expect(cancellations.textContent).toContain('1');
    const failed = within(figures).getByRole('button', {
      name: /Solicitudes de consumo con error/,
    });
    expect(failed.textContent).toContain('2 pendientes');
    fireEvent.click(cancellations);
    await screen.findByRole('article', { name: 'Cancelación de Hamburguesa de prueba' });
  });

  it('sin consulta de movimientos no muestra la cifra de solicitudes', async () => {
    const requests = useTransport();
    renderInventory(['inventory.read']);
    const figures = await screen.findByRole('region', { name: 'Cifras de inventario' });
    await within(figures).findByText('Cancelaciones por clasificar');
    expect(within(figures).queryByText('Solicitudes de consumo con error')).toBeNull();
    expect(writes(requests, 'inventory_consumption_requests')).toHaveLength(0);
  });
});

describe('controles numéricos', () => {
  it('Ajustes y mermas no usa type="number"', async () => {
    useTransport(cancellationResponder([pendingCancellation()]));
    renderInventory(allInventoryPermissions);
    fireEvent.click(await screen.findByRole('tab', { name: 'Ajustes y mermas' }));
    await screen.findByRole('article', { name: 'Cancelación de Hamburguesa de prueba' });
    expect(document.querySelectorAll('input[type="number"]')).toHaveLength(0);
  });
});
