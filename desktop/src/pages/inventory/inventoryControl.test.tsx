// @vitest-environment jsdom

import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { PendingOperationsProvider } from '../../hooks/pendingOperations';
import { InventoryPage } from './InventoryPage';
import {
  adjustment,
  adjustmentPage,
  alertPage,
  appliedAdjustment,
  blindCount,
  controlIDs,
  controlPermissions,
  count,
  countedTomato,
  countLine,
  countPage,
  stockLevel,
  suggestion,
  suggestionPage,
  transfer,
} from './inventoryControlTestData';
import { draftOrder } from './inventoryPurchasingTestData';
import {
  ids,
  inventoryTransport,
  item,
  itemPage,
  movementPage,
  stockPage,
  tomato,
} from './inventoryTestData';
import type { RecordedRequest, Responder } from './inventoryTestData';
import {
  groupTabNames,
  navigationMap,
  openInventorySection,
  sectionTab,
} from './inventoryNavigationTestUtils';

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

function writes(requests: RecordedRequest[], operation: string) {
  return requests.filter((request) => request.operation === operation);
}

function lastQuery(requests: RecordedRequest[], operation: string) {
  return requests.filter((request) => request.operation === operation).at(-1)?.query ?? null;
}

async function openTab(name: string, permissions: string[]) {
  renderInventory(permissions);
  await openInventorySection(name);
}

// Envía con el botón principal del formulario cuando ya está habilitado.
async function submitWith(name: string) {
  const button = screen
    .getAllByRole('button', { name })
    .find((candidate) => candidate.getAttribute('type') === 'submit') as HTMLButtonElement;
  await waitFor(() => expect(button.disabled).toBe(false));
  await act(async () => {
    fireEvent.click(button);
  });
}

async function confirmDialog(button: string) {
  const dialog = await screen.findByRole('alertdialog');
  await act(async () => {
    fireEvent.click(within(dialog).getByRole('button', { name: button }));
  });
  return dialog;
}

// Elige el insumo de un renglón de captura después de que llegan las opciones de Go.
async function chooseLineItem(line: HTMLElement, itemID: string, label: string) {
  await within(line).findByRole('option', { name: label });
  fireEvent.change(within(line).getByRole('combobox', { name: 'Insumo' }), {
    target: { value: itemID },
  });
}

// Navegación completa con todos los permisos (etapa 9A): cinco grupos con sus secciones.
const fullNavigation = {
  Resumen: [],
  Catálogo: ['Insumos', 'Recetas', 'Almacenes', 'Proveedores'],
  Existencias: ['Existencias', 'Movimientos', 'Transferencias', 'Ajustes y mermas', 'Conteos'],
  Compras: [
    'Órdenes de compra',
    'Sugerencias de compra',
    'Recepciones',
    'Devoluciones a proveedor',
  ],
  Configuración: ['Unidades', 'Categorías', 'Parámetros'],
};

describe('navegación de Inventarios por grupos', () => {
  it('agrupa las secciones en el orden de la especificación y las muestra por permiso', async () => {
    useTransport();
    const { unmount } = renderInventory(controlPermissions);
    await screen.findByText('Control de inventario');
    expect(groupTabNames()).toEqual([
      'Resumen',
      'Catálogo',
      'Existencias',
      'Compras',
      'Configuración',
    ]);
    expect(navigationMap()).toEqual(fullNavigation);
    unmount();
    const second = renderInventory(['inventory.read', 'inventory.counts.approve']);
    await screen.findByText('Control de inventario');
    expect(navigationMap()).toEqual({
      Resumen: [],
      Catálogo: ['Insumos', 'Recetas', 'Almacenes'],
      Existencias: ['Ajustes y mermas', 'Conteos'],
      Configuración: ['Unidades', 'Categorías'],
    });
    second.unmount();
    renderInventory(['inventory.read', 'inventory.transfers.record']);
    await screen.findByText('Control de inventario');
    expect(navigationMap()).toEqual({
      Resumen: [],
      Catálogo: ['Insumos', 'Recetas', 'Almacenes'],
      Existencias: ['Transferencias', 'Ajustes y mermas'],
      Configuración: ['Unidades', 'Categorías'],
    });
  });

  it('cada grupo recuerda su última sección y los destinos internos cambian de grupo', async () => {
    useTransport((request) =>
      request.operation === 'inventory_receipts'
        ? {
            status: 200,
            body: { items: [], total: 0, page: 1, page_size: 25, costs_visible: false },
          }
        : undefined,
    );
    renderInventory(controlPermissions);
    await openInventorySection('Conteos');
    expect(sectionTab('Conteos').getAttribute('aria-selected')).toBe('true');
    await openInventorySection('Recepciones');
    const groups = screen.getByRole('tablist', { name: 'Secciones de inventario' });
    expect(within(groups).getByRole('tab', { name: 'Compras' }).getAttribute('aria-selected')).toBe(
      'true',
    );
    // Al volver a Existencias se abre Conteos, la última sección usada del grupo.
    await act(async () => {
      fireEvent.click(within(groups).getByRole('tab', { name: 'Existencias' }));
    });
    expect(sectionTab('Conteos').getAttribute('aria-selected')).toBe('true');
    // Resumen no tiene segunda fila.
    await act(async () => {
      fireEvent.click(within(groups).getByRole('tab', { name: 'Resumen' }));
    });
    expect(screen.queryByRole('tablist', { name: 'Apartados de Resumen' })).toBeNull();
  });
});

describe('ajustes y mermas', () => {
  it('registra un ajuste con motivo de lista cerrada y renglones de faltante y sobrante', async () => {
    const requests = useTransport((request) => {
      if (request.operation === 'inventory_record_adjustment') {
        return { status: 200, body: adjustment({ reason: 'entry_error', line_count: 2 }) };
      }
      if (request.operation === 'inventory_adjustment') {
        return { status: 200, body: adjustment({ reason: 'entry_error', line_count: 2 }) };
      }
      return undefined;
    });
    // Sin permiso de autorizar: el ajuste queda pendiente.
    await openTab('Ajustes y mermas', ['inventory.read', 'inventory.adjustments.record']);
    // Las cancelaciones por clasificar siguen en la misma pestaña.
    await screen.findByRole('heading', { name: 'Cancelaciones por clasificar' });
    fireEvent.click(await screen.findByRole('button', { name: 'Registrar ajuste' }));
    const reasons = screen.getByRole('group', { name: 'Motivo del ajuste' });
    const line = screen.getByRole('group', { name: 'Renglón 1' });
    // «Faltante físico» solo admite salidas: «Sobrante» queda deshabilitado.
    fireEvent.click(within(reasons).getByRole('button', { name: 'Faltante físico' }));
    expect(
      (within(line).getByRole('button', { name: 'Sobrante (entrada)' }) as HTMLButtonElement)
        .disabled,
    ).toBe(true);
    // «Error de registro» admite ambos sentidos.
    fireEvent.click(within(reasons).getByRole('button', { name: 'Error de registro' }));
    await chooseLineItem(line, ids.flour, 'Harina · HAR-01');
    fireEvent.change(within(line).getByLabelText(/^Cantidad/), { target: { value: '3' } });
    fireEvent.click(within(line).getByRole('button', { name: 'Sobrante (entrada)' }));
    fireEvent.click(screen.getByRole('button', { name: 'Agregar renglón' }));
    const second = screen.getByRole('group', { name: 'Renglón 2' });
    await chooseLineItem(second, ids.tomato, 'Tomate · TOM-01');
    // Un signo tecleado se descarta: el sentido se elige con los botones.
    fireEvent.change(within(second).getByLabelText(/^Cantidad/), { target: { value: '-2' } });
    fireEvent.change(within(second).getByLabelText(/^Cantidad/), { target: { value: '2' } });
    expect(
      within(second)
        .getByRole('button', { name: 'Faltante (salida)' })
        .getAttribute('aria-pressed'),
    ).toBe('true');
    await submitWith('Registrar ajuste');
    const dialog = await confirmDialog('Registrar ajuste');
    expect(dialog.textContent).toContain('Quedará pendiente de autorización');
    expect(dialog.textContent).toContain('Harina: sobrante de 3 kilogramo');
    expect(dialog.textContent).toContain('Tomate: faltante de 2 kilogramo');
    const [record] = writes(requests, 'inventory_record_adjustment');
    expect(record?.body).toEqual({
      kind: 'discrepancy',
      reason: 'entry_error',
      warehouse_id: ids.general,
      lines: [
        { item_id: ids.flour, quantity: '3' },
        { item_id: ids.tomato, quantity: '-2' },
      ],
      operation_id: expect.any(String),
    });
    await screen.findByText('Ajuste 12 registrado: queda pendiente de autorización.');
    const panel = await screen.findByRole('region', { name: 'Ajuste 12' });
    expect(within(panel).getByText('Pendiente de autorización', { selector: '.tag' })).toBeTruthy();
    // Quien lo registró no ve los botones de autorizar ni rechazar.
    expect(within(panel).queryByRole('button', { name: 'Autorizar y aplicar' })).toBeNull();
    expect(within(panel).queryByRole('button', { name: 'Rechazar' })).toBeNull();
    expect(within(panel).getByText(/Quien lo registró no puede autorizarlo/)).toBeTruthy();
  });

  it('otra persona con permiso autoriza el pendiente previa confirmación', async () => {
    const pending = adjustment({ authorizable: true });
    const requests = useTransport((request) => {
      if (request.operation === 'inventory_adjustments') {
        return { status: 200, body: adjustmentPage([pending]) };
      }
      if (request.operation === 'inventory_adjustment') {
        return {
          status: 200,
          body: writes(requests, 'inventory_authorize_adjustment').length
            ? appliedAdjustment()
            : pending,
        };
      }
      if (request.operation === 'inventory_authorize_adjustment') {
        return { status: 200, body: appliedAdjustment() };
      }
      return undefined;
    });
    await openTab('Ajustes y mermas', [
      'inventory.read',
      'inventory.adjustments.authorize',
      'inventory.costs.read',
    ]);
    // Sin permiso de registrar no se ofrece el alta.
    const row = (await screen.findByText('Ajuste 12')).closest('tr')!;
    expect(screen.queryByRole('button', { name: 'Registrar ajuste' })).toBeNull();
    expect(within(row).getByText('Pendiente de autorización')).toBeTruthy();
    fireEvent.click(within(row).getByRole('button', { name: 'Abrir ajuste 12' }));
    const panel = await screen.findByRole('region', { name: 'Ajuste 12' });
    expect(within(panel).getByText('Pendiente de autorización', { selector: 'td' })).toBeTruthy();
    fireEvent.click(within(panel).getByRole('button', { name: 'Autorizar y aplicar' }));
    expect(writes(requests, 'inventory_authorize_adjustment')).toHaveLength(0);
    const dialog = await confirmDialog('Autorizar y aplicar');
    expect(dialog.textContent).toContain('con la hora de la autorización');
    const [authorize] = writes(requests, 'inventory_authorize_adjustment');
    expect(authorize?.resourceId).toBe(controlIDs.adjustment);
    expect(authorize?.body).toEqual({ revision: 1, operation_id: expect.any(String) });
    await screen.findByText('Ajuste 12 autorizado y aplicado.');
    // Movimiento, existencia después y valor tal como los calculó Go.
    await within(panel).findByText('Ajuste negativo');
    expect(within(panel).getByText('10.5 kg')).toBeTruthy();
    expect(within(panel).getAllByText('−$36.00').length).toBeGreaterThan(0);
    expect(within(panel).getByText(/^Gerente · /)).toBeTruthy();
  });

  it('una merma solo registra salidas y avisa la existencia negativa que devuelve Go', async () => {
    const recorded = adjustment({
      id: controlIDs.waste,
      folio: 3,
      kind: 'waste',
      kind_label: 'Merma operativa',
      reason: 'breakage',
      reason_label: 'Rotura',
      state: 'applied',
      state_label: 'Aplicado',
      applied_at: '2026-11-10T15:00:00Z',
      warnings: [
        {
          code: 'NEGATIVE_STOCK',
          message: 'Harina quedó con existencia negativa en «Almacén general» (−12.5 kg).',
          item_id: ids.flour,
        },
      ],
    });
    const requests = useTransport((request) => {
      if (request.operation === 'inventory_record_adjustment') {
        return { status: 200, body: recorded };
      }
      if (request.operation === 'inventory_adjustment') {
        return { status: 200, body: { ...recorded, warnings: [] } };
      }
      return undefined;
    });
    await openTab('Ajustes y mermas', ['inventory.read', 'inventory.waste.record']);
    fireEvent.click(await screen.findByRole('button', { name: 'Registrar merma' }));
    const reasons = screen.getByRole('group', { name: 'Motivo de la merma' });
    expect(
      within(reasons)
        .getAllByRole('button')
        .map((button) => button.textContent),
    ).toEqual(['Rotura', 'Derrame', 'Caducidad', 'Consumo extraordinario', 'Otro']);
    const line = screen.getByRole('group', { name: 'Renglón 1' });
    expect(within(line).queryByRole('group', { name: 'Sentido del renglón 1' })).toBeNull();
    expect(within(line).getByText(/La merma solo registra salidas/)).toBeTruthy();
    await chooseLineItem(line, ids.flour, 'Harina · HAR-01');
    fireEvent.change(within(line).getByLabelText(/^Cantidad/), { target: { value: '1' } });
    fireEvent.change(within(line).getByRole('combobox', { name: 'Unidad o presentación' }), {
      target: { value: `presentation:${ids.sack}` },
    });
    // Sin motivo no se envía.
    await submitWith('Registrar merma');
    await screen.findByText('Elige el motivo.');
    expect(writes(requests, 'inventory_record_adjustment')).toHaveLength(0);
    fireEvent.click(within(reasons).getByRole('button', { name: 'Rotura' }));
    await submitWith('Registrar merma');
    await confirmDialog('Registrar merma');
    const [record] = writes(requests, 'inventory_record_adjustment');
    expect(record?.body).toMatchObject({
      kind: 'waste',
      reason: 'breakage',
      lines: [{ item_id: ids.flour, quantity: '-1', presentation_id: ids.sack }],
    });
    await screen.findByText('Merma 3 registrada y aplicada.');
    const panel = await screen.findByRole('region', { name: 'Merma 3' });
    expect(
      within(panel).getByText(
        'Harina quedó con existencia negativa en «Almacén general» (−12.5 kg).',
      ),
    ).toBeTruthy();
  });

  it('una entrada sin costo explica el rechazo de Go y lleva a Insumos', async () => {
    useTransport((request) => {
      if (request.operation === 'inventory_record_adjustment') {
        return {
          status: 409,
          body: {
            error: {
              code: 'COST_REQUIRED',
              message:
                'El insumo Tomate no tiene costo: configura su costo de referencia o registra una compra antes del sobrante.',
            },
          },
        };
      }
      return undefined;
    });
    await openTab('Ajustes y mermas', [
      'inventory.read',
      'inventory.adjustments.record',
      'inventory.adjustments.authorize',
    ]);
    fireEvent.click(await screen.findByRole('button', { name: 'Registrar ajuste' }));
    fireEvent.click(screen.getByRole('button', { name: 'Sobrante' }));
    const line = screen.getByRole('group', { name: 'Renglón 1' });
    await chooseLineItem(line, ids.tomato, 'Tomate · TOM-01');
    fireEvent.change(within(line).getByLabelText(/^Cantidad/), { target: { value: '1' } });
    await submitWith('Registrar ajuste');
    await confirmDialog('Registrar ajuste');
    await screen.findByText(/El insumo Tomate no tiene costo/);
    fireEvent.click(screen.getByRole('button', { name: 'Ir a Insumos' }));
    await waitFor(() => expect(sectionTab('Insumos').getAttribute('aria-selected')).toBe('true'));
  });
});

describe('transferencias', () => {
  it('exige origen y destino distintos y muestra las existencias que calcula Go', async () => {
    const requests = useTransport((request) => {
      if (request.operation === 'inventory_record_transfer') {
        return { status: 200, body: transfer() };
      }
      if (request.operation === 'inventory_transfer') {
        return { status: 200, body: { ...transfer(), warnings: [] } };
      }
      return undefined;
    });
    await openTab('Transferencias', [
      'inventory.read',
      'inventory.transfers.record',
      'inventory.costs.read',
    ]);
    fireEvent.click(await screen.findByRole('button', { name: 'Nueva transferencia' }));
    const destination = screen.getByRole('combobox', { name: /^Almacén de destino/ });
    await within(destination).findByRole('option', { name: 'Barra' });
    // El destino no ofrece el almacén de origen.
    expect(
      within(destination)
        .getAllByRole('option')
        .map((option) => option.textContent),
    ).toEqual(['Elige el destino', 'Barra']);
    fireEvent.change(destination, { target: { value: ids.bar } });
    // Elegir como origen el mismo almacén deja el destino sin elegir.
    fireEvent.change(screen.getByRole('combobox', { name: /^Almacén de origen/ }), {
      target: { value: ids.bar },
    });
    expect((destination as HTMLSelectElement).value).toBe('');
    const submit = screen
      .getAllByRole('button', { name: 'Registrar transferencia' })
      .find((button) => button.getAttribute('type') === 'submit') as HTMLButtonElement;
    expect(submit.disabled).toBe(true);
    fireEvent.change(screen.getByRole('combobox', { name: /^Almacén de origen/ }), {
      target: { value: ids.general },
    });
    fireEvent.change(destination, { target: { value: ids.bar } });
    const line = screen.getByRole('group', { name: 'Renglón 1' });
    expect(within(line).queryByRole('group', { name: 'Sentido del renglón 1' })).toBeNull();
    await chooseLineItem(line, ids.flour, 'Harina · HAR-01');
    fireEvent.change(within(line).getByLabelText(/^Cantidad/), { target: { value: '2' } });
    fireEvent.change(within(line).getByRole('combobox', { name: 'Unidad o presentación' }), {
      target: { value: `presentation:${ids.sack}` },
    });
    await submitWith('Registrar transferencia');
    const dialog = await confirmDialog('Registrar transferencia');
    expect(dialog.textContent).toContain('una existencia negativa en el origen no bloquea');
    const [record] = writes(requests, 'inventory_record_transfer');
    expect(record?.body).toEqual({
      from_warehouse_id: ids.general,
      to_warehouse_id: ids.bar,
      lines: [{ item_id: ids.flour, quantity: '2', presentation_id: ids.sack }],
      operation_id: expect.any(String),
    });
    await screen.findByText('Transferencia 4 registrada: de «Almacén general» a «Barra».');
    const panel = await screen.findByRole('region', { name: 'Transferencia 4' });
    expect(within(panel).getByText('2 · Costal 25 kg')).toBeTruthy();
    expect(within(panel).getByText('−37.5 kg')).toBeTruthy();
    expect(within(panel).getAllByText('$450.00').length).toBeGreaterThan(0);
    expect(within(panel).getByText(/quedó con existencia negativa/)).toBeTruthy();
  });
});

describe('conteos físicos', () => {
  it('quien solo cuenta captura a ciegas, guarda el avance y envía', async () => {
    let current = blindCount();
    const requests = useTransport((request) => {
      if (request.operation === 'inventory_counts') {
        return { status: 200, body: countPage([current]) };
      }
      if (request.operation === 'inventory_count') {
        return { status: 200, body: current };
      }
      if (request.operation === 'inventory_capture_count_lines') {
        current = blindCount({
          counted_lines: 2,
          warnings: [],
          lines: [
            countLine({
              counted: true,
              counted_quantity: '12.5',
              captured_quantity: '12.5',
              capture_label: '',
              captured_at: '2026-11-10T14:10:00Z',
              captured_by_name: 'Ana',
            }),
            {
              ...countedTomato(),
              theoretical_quantity: null,
              difference: null,
              difference_kind: null,
              current_quantity: null,
            },
          ],
        });
        return { status: 200, body: current };
      }
      if (request.operation === 'inventory_submit_count') {
        current = blindCount({
          state: 'submitted',
          state_label: 'Enviado',
          editable: false,
          submittable: false,
          revision: 4,
        });
        return { status: 200, body: current };
      }
      return undefined;
    });
    await openTab('Conteos', ['inventory.read', 'inventory.counts.perform']);
    fireEvent.click(
      await screen.findByRole('button', { name: 'Abrir el conteo 7 de Almacén general' }),
    );
    const panel = await screen.findByRole('region', { name: 'Conteo 7 · Almacén general' });
    expect(within(panel).getByText(/La venta sigue durante el conteo/)).toBeTruthy();
    expect(within(panel).getByText(/Conteo a ciegas/)).toBeTruthy();
    // Sin teórico ni diferencias para quien solo cuenta.
    expect(within(panel).queryByText('Teórico a esa hora')).toBeNull();
    expect(within(panel).queryByText('Diferencia')).toBeNull();
    // Búsqueda rápida sin acentos ni mayúsculas.
    fireEvent.change(within(panel).getByLabelText('Buscar en la hoja'), {
      target: { value: 'HARI' },
    });
    expect(within(panel).queryByRole('listitem', { name: 'Renglón de Tomate' })).toBeNull();
    fireEvent.change(within(panel).getByLabelText('Buscar en la hoja'), { target: { value: '' } });
    const input = within(panel).getByRole('textbox', { name: 'Cantidad contada de Harina' });
    expect(input.getAttribute('inputmode')).toBe('decimal');
    fireEvent.change(input, { target: { value: '12.5' } });
    await within(panel).findByText('Sin guardar');
    // Con cambios sin guardar no se envía.
    expect(
      (within(panel).getByRole('button', { name: 'Enviar a revisión' }) as HTMLButtonElement)
        .disabled,
    ).toBe(true);
    await act(async () => {
      fireEvent.click(within(panel).getByRole('button', { name: 'Guardar avance' }));
    });
    const [capture] = writes(requests, 'inventory_capture_count_lines');
    expect(capture?.resourceId).toBe(controlIDs.count);
    expect(capture?.body).toEqual({
      lines: [{ item_id: ids.flour, counted_quantity: '12.5' }],
      operation_id: expect.any(String),
    });
    await screen.findByText('Avance guardado: 1 renglón.');
    // Cada renglón muestra su hora y quién lo capturó.
    const flour = within(panel).getByRole('listitem', { name: 'Renglón de Harina' });
    await within(flour).findByText(/12.5 kg · .* · Ana/);
    // Enviar con confirmación; sin permiso de aprobar no aparecen Aprobar ni Aplicar.
    expect(within(panel).queryByRole('button', { name: 'Aprobar' })).toBeNull();
    expect(within(panel).queryByRole('button', { name: 'Aplicar ajustes' })).toBeNull();
    fireEvent.click(within(panel).getByRole('button', { name: 'Enviar a revisión' }));
    await confirmDialog('Enviar a revisión');
    const [submit] = writes(requests, 'inventory_submit_count');
    expect(submit?.body).toEqual({ revision: 3, operation_id: expect.any(String) });
    await screen.findByText('Conteo 7 enviado a revisión.');
    await within(panel).findByText(/aprobarlo y aplicarlo requiere el permiso/);
  });

  it('quien aprueba ve las diferencias de Go, aprueba y aplica con confirmación', async () => {
    let current = count({
      state: 'submitted',
      state_label: 'Enviado',
      editable: false,
      submittable: false,
      approvable: true,
      submitted_by_name: 'Ana',
      submitted_at: '2026-11-10T14:20:00Z',
      warnings: [],
    });
    const requests = useTransport((request) => {
      if (request.operation === 'inventory_alerts') {
        return { status: 200, body: alertPage() };
      }
      if (request.operation === 'inventory_count') {
        return { status: 200, body: current };
      }
      if (request.operation === 'inventory_approve_count') {
        current = count({
          ...current,
          state: 'approved',
          state_label: 'Aprobado',
          approvable: false,
          applicable: true,
          approved_by_name: 'Gerente',
          approved_at: '2026-11-10T14:30:00Z',
          revision: 4,
        });
        return { status: 200, body: current };
      }
      if (request.operation === 'inventory_apply_count') {
        current = count({
          ...current,
          state: 'applied',
          state_label: 'Aplicado',
          applicable: false,
          cancellable: false,
          applied_by_name: 'Gerente',
          applied_at: '2026-11-10T14:40:00Z',
          reference_at: '2026-11-10T14:05:00Z',
          difference_lines: 1,
          revision: 5,
          lines: [
            countLine(),
            countedTomato({
              movement_id: '00000000-0000-4000-8000-000000000a22',
              movement_kind: 'count_out',
              movement_label: 'Faltante de conteo',
            }),
          ],
        });
        return { status: 200, body: current };
      }
      return undefined;
    });
    renderInventory(['inventory.read', 'inventory.counts.approve']);
    // Desde la alerta del resumen se abre el conteo enviado.
    fireEvent.click(await screen.findByRole('button', { name: 'Abrir conteo: Conteo 7' }));
    const panel = await screen.findByRole('region', { name: 'Conteo 7 · Almacén general' });
    expect(sectionTab('Conteos').getAttribute('aria-selected')).toBe('true');
    // Diferencias calculadas por Go con el teórico a la hora del renglón.
    const tomatoRow = within(panel).getByRole('listitem', { name: 'Renglón de Tomate' });
    expect(within(tomatoRow).getByText('Teórico a esa hora')).toBeTruthy();
    expect(within(tomatoRow).getByText('4.75 kg')).toBeTruthy();
    expect(within(tomatoRow).getByText('−1.5 kg')).toBeTruthy();
    expect(within(tomatoRow).getByText('Faltante')).toBeTruthy();
    expect(within(tomatoRow).getByText('4.1 kg')).toBeTruthy();
    expect(within(panel).getAllByText(/^Ana · /, { selector: 'dd' }).length).toBe(2);
    // Sin permiso de realizar conteos no se captura ni se envía.
    expect(within(panel).queryByRole('textbox', { name: /^Cantidad contada/ })).toBeNull();
    expect(within(panel).queryByRole('button', { name: 'Enviar a revisión' })).toBeNull();
    fireEvent.click(within(panel).getByRole('button', { name: 'Aprobar' }));
    await confirmDialog('Aprobar conteo');
    await screen.findByText('Conteo 7 aprobado.');
    const [approve] = writes(requests, 'inventory_approve_count');
    expect(approve?.body).toEqual({ revision: 3, operation_id: expect.any(String) });
    fireEvent.click(await within(panel).findByRole('button', { name: 'Aplicar ajustes' }));
    const dialog = await screen.findByRole('alertdialog');
    expect(dialog.textContent).toContain('se registrará 1 movimiento de ajuste');
    expect(writes(requests, 'inventory_apply_count')).toHaveLength(0);
    await confirmDialog('Aplicar ajustes');
    const [apply] = writes(requests, 'inventory_apply_count');
    expect(apply?.body).toEqual({ revision: 4, operation_id: expect.any(String) });
    await screen.findByText('Conteo 7 aplicado: se registraron sus ajustes.');
    await within(panel).findByText('Faltante de conteo');
    expect(within(panel).getByText('Teórico aplicado')).toBeTruthy();
    expect(within(panel).getByText('1 renglón', { selector: 'dd' })).toBeTruthy();
  });

  it('un sobrante sin costo rechaza la aplicación con el mensaje de Go', async () => {
    const approved = count({
      state: 'approved',
      state_label: 'Aprobado',
      editable: false,
      submittable: false,
      applicable: true,
      warnings: [],
      lines: [countedTomato({ difference: '2', difference_kind: 'surplus' })],
    });
    const requests = useTransport((request) => {
      if (request.operation === 'inventory_counts') {
        return { status: 200, body: countPage([approved]) };
      }
      if (request.operation === 'inventory_count') {
        return { status: 200, body: approved };
      }
      if (request.operation === 'inventory_apply_count') {
        return {
          status: 409,
          body: {
            error: {
              code: 'COST_REQUIRED',
              message: 'El insumo Tomate no tiene costo: configura su costo de referencia.',
              details: { item_id: ids.tomato },
            },
          },
        };
      }
      return undefined;
    });
    await openTab('Conteos', [
      'inventory.read',
      'inventory.counts.approve',
      'inventory.items.manage',
    ]);
    fireEvent.click(
      await screen.findByRole('button', { name: 'Abrir el conteo 7 de Almacén general' }),
    );
    const panel = await screen.findByRole('region', { name: 'Conteo 7 · Almacén general' });
    expect(within(panel).getByText('Sobrante')).toBeTruthy();
    fireEvent.click(within(panel).getByRole('button', { name: 'Aplicar ajustes' }));
    await confirmDialog('Aplicar ajustes');
    await screen.findByText('El insumo Tomate no tiene costo: configura su costo de referencia.');
    // El conteo sigue aprobado y se puede reintentar.
    expect(within(panel).getByRole('button', { name: 'Aplicar ajustes' })).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: 'Abrir insumo' }));
    await waitFor(() =>
      expect(writes(requests, 'inventory_item').at(-1)?.resourceId).toBe(ids.tomato),
    );
    expect(sectionTab('Insumos').getAttribute('aria-selected')).toBe('true');
  });
});

describe('niveles por almacén', () => {
  it('define y quita el nivel de un almacén desde el insumo con items.manage', async () => {
    const requests = useTransport((request) => {
      if (request.operation === 'inventory_stock_levels') {
        return {
          status: 200,
          body: { items: [stockLevel()], total: 1, page: 1, page_size: 100 },
        };
      }
      if (request.operation === 'inventory_save_stock_level') {
        return { status: 200, body: stockLevel() };
      }
      return undefined;
    });
    await openTab('Insumos', ['inventory.read', 'inventory.items.manage', 'inventory.stock.read']);
    fireEvent.click(await screen.findByRole('button', { name: 'Editar el insumo Harina' }));
    const levels = await screen.findByRole('region', { name: 'Niveles por almacén' });
    expect(lastQuery(requests, 'inventory_stock_levels')).toMatchObject({ item_id: ids.flour });
    const barRow = (await within(levels).findByText('Barra')).closest('li')!;
    expect(within(barRow).getByText('Mín. 2 kg · Reabastecer en 4 kg')).toBeTruthy();
    expect(within(barRow).getByText('Bajo mínimo')).toBeTruthy();
    expect(within(barRow).getByText('Existencia: 1.5 kg')).toBeTruthy();
    const generalRow = within(levels).getByText('Almacén general').closest('li')!;
    expect(within(generalRow).getByText('Usa los niveles del insumo')).toBeTruthy();
    fireEvent.click(
      within(generalRow).getByRole('button', { name: 'Editar el nivel de Almacén general' }),
    );
    fireEvent.change(within(generalRow).getByLabelText(/^Mínimo en Almacén general/), {
      target: { value: '3' },
    });
    await act(async () => {
      fireEvent.click(within(generalRow).getByRole('button', { name: 'Guardar nivel' }));
    });
    const [saved] = writes(requests, 'inventory_save_stock_level');
    expect(saved?.body).toEqual({
      item_id: ids.flour,
      warehouse_id: ids.general,
      revision: 0,
      min_stock: '3',
      reorder_point: null,
      operation_id: expect.any(String),
    });
    await screen.findByText('Nivel de «Almacén general» guardado.');
    fireEvent.click(within(barRow).getByRole('button', { name: 'Editar el nivel de Barra' }));
    await act(async () => {
      fireEvent.click(within(barRow).getByRole('button', { name: 'Quitar nivel' }));
    });
    const removed = writes(requests, 'inventory_save_stock_level').at(-1);
    expect(removed?.body).toMatchObject({
      warehouse_id: ids.bar,
      revision: 2,
      min_stock: null,
      reorder_point: null,
    });
    await screen.findByText('Nivel de «Barra» quitado: se usan los del insumo.');
  });

  it('sin items.manage los niveles solo se consultan', async () => {
    useTransport((request) =>
      request.operation === 'inventory_stock_levels'
        ? { status: 200, body: { items: [stockLevel()], total: 1, page: 1, page_size: 100 } }
        : undefined,
    );
    await openTab('Insumos', ['inventory.read']);
    fireEvent.click(await screen.findByRole('button', { name: 'Ver el insumo Harina' }));
    const levels = await screen.findByRole('region', { name: 'Niveles por almacén' });
    await within(levels).findByText('Barra');
    expect(within(levels).queryByRole('button', { name: /^Editar el nivel/ })).toBeNull();
    expect(within(levels).getByText(/Cambiarlos requiere el permiso/)).toBeTruthy();
  });
});

describe('alertas del resumen', () => {
  it('agrupa por tipo con su gravedad y lleva al registro relacionado sin identificadores', async () => {
    const requests = useTransport((request) =>
      request.operation === 'inventory_alerts' ? { status: 200, body: alertPage() } : undefined,
    );
    const view = renderInventory(controlPermissions);
    const section = await screen.findByRole('region', { name: 'Alertas de inventario' });
    await within(section).findByText(/Tomate tiene existencia negativa/);
    expect(within(section).getByText('1 crítica')).toBeTruthy();
    expect(within(section).getByText('1 advertencia')).toBeTruthy();
    expect(within(section).getByText('2 avisos')).toBeTruthy();
    // Grupos por tipo en el orden de Go; los avisos empiezan cerrados.
    const triggers = within(section)
      .getAllByRole('button', { expanded: true })
      .concat(within(section).getAllByRole('button', { expanded: false }))
      .map((button) => button.textContent ?? '');
    expect(triggers.some((text) => text.startsWith('Existencia negativa'))).toBe(true);
    expect(
      within(section)
        .getByRole('button', { name: /^Insumo sin proveedor/ })
        .getAttribute('aria-expanded'),
    ).toBe('false');
    expect(
      within(section)
        .getByRole('button', { name: /^Existencia negativa/ })
        .getAttribute('aria-expanded'),
    ).toBe('true');
    // Ningún identificador técnico a la vista.
    expect(view.container.textContent).not.toContain(ids.tomato);
    expect(view.container.textContent).not.toContain(controlIDs.count);
    // Filtro por gravedad con los conteos de Go.
    fireEvent.click(within(section).getByRole('button', { name: 'Críticas (1)' }));
    await waitFor(() =>
      expect(lastQuery(requests, 'inventory_alerts')).toMatchObject({ severity: 'critical' }),
    );
    // La existencia negativa abre Existencias con el insumo y el almacén.
    fireEvent.click(within(section).getByRole('button', { name: 'Ver existencias: Tomate' }));
    await screen.findByRole('region', { name: 'Filtros de existencias' });
    await waitFor(() =>
      expect(lastQuery(requests, 'inventory_stock')).toMatchObject({
        search: 'TOM-01',
        warehouse_id: ids.general,
      }),
    );
    // El producto sin receta abre Recetas con la búsqueda del producto.
    await openInventorySection('Resumen');
    const again = await screen.findByRole('region', { name: 'Alertas de inventario' });
    fireEvent.click(await within(again).findByRole('button', { name: /^Producto sin receta/ }));
    fireEvent.click(within(again).getByRole('button', { name: 'Ver recetas: Agua fresca' }));
    await waitFor(() =>
      expect(lastQuery(requests, 'inventory_recipe_targets')).toMatchObject({
        search: 'Agua fresca',
      }),
    );
    // El insumo sin proveedor abre su ficha.
    await openInventorySection('Resumen');
    const third = await screen.findByRole('region', { name: 'Alertas de inventario' });
    fireEvent.click(await within(third).findByRole('button', { name: /^Insumo sin proveedor/ }));
    fireEvent.click(within(third).getByRole('button', { name: 'Abrir insumo: Harina' }));
    await waitFor(() =>
      expect(writes(requests, 'inventory_item').at(-1)?.resourceId).toBe(ids.flour),
    );
  });

  it('sin permiso del destino la alerta no ofrece acceso directo', async () => {
    useTransport((request) =>
      request.operation === 'inventory_alerts' ? { status: 200, body: alertPage() } : undefined,
    );
    renderInventory(['inventory.read']);
    const section = await screen.findByRole('region', { name: 'Alertas de inventario' });
    await within(section).findByText(/El conteo 7 de «Almacén general»/);
    expect(within(section).queryByRole('button', { name: 'Abrir conteo: Conteo 7' })).toBeNull();
    // Sin existencias, la alerta de existencia lleva al insumo.
    expect(within(section).queryByRole('button', { name: 'Ver existencias: Tomate' })).toBeNull();
    expect(within(section).getByRole('button', { name: 'Abrir insumo: Tomate' })).toBeTruthy();
  });
});

describe('sugerencias de compra', () => {
  it('muestra lo sugerido y crea el borrador solo por acción explícita con confirmación', async () => {
    const created = draftOrder({ id: controlIDs.draftOrder, folio: 31 });
    const requests = useTransport((request) => {
      if (request.operation === 'inventory_purchase_suggestions') {
        return { status: 200, body: suggestionPage([suggestion()]) };
      }
      if (request.operation === 'inventory_draft_order_from_suggestions') {
        return { status: 200, body: created };
      }
      if (request.operation === 'inventory_purchase_order') {
        return { status: 200, body: created };
      }
      if (request.operation === 'inventory_purchase_orders') {
        return { status: 200, body: { items: [], total: 0, page: 1, page_size: 25 } };
      }
      return undefined;
    });
    await openTab('Sugerencias de compra', controlPermissions);
    const group = await screen.findByRole('region', { name: 'Sugerencias: Molinos del Norte' });
    const row = within(group).getByText('Harina').closest('tr')!;
    expect(within(row).getByText('2 · Costal 25 kg')).toBeTruthy();
    expect(within(row).getByText('47.5 kg')).toBeTruthy();
    expect(within(row).getByText('$450.00 / Costal 25 kg')).toBeTruthy();
    const include = within(row).getByRole('button', { name: 'Incluir Harina en el borrador' });
    const create = within(group).getByRole('button', { name: 'Crear orden en borrador' });
    fireEvent.click(include);
    expect(include.getAttribute('aria-pressed')).toBe('false');
    expect((create as HTMLButtonElement).disabled).toBe(true);
    fireEvent.click(include);
    fireEvent.click(create);
    const dialog = await screen.findByRole('alertdialog');
    expect(dialog.textContent).toContain('Harina: 2 · Costal 25 kg');
    expect(dialog.textContent).toContain('No se emite');
    expect(writes(requests, 'inventory_draft_order_from_suggestions')).toHaveLength(0);
    await confirmDialog('Crear borrador');
    const [draft] = writes(requests, 'inventory_draft_order_from_suggestions');
    expect(draft?.body).toEqual({
      supplier_id: controlIDs.supplier,
      item_ids: [ids.flour],
      operation_id: expect.any(String),
    });
    await screen.findByText(/^Orden 31 creada como borrador/);
    // Se abre el borrador creado.
    await screen.findByRole('region', { name: /^Orden 31 · / });
    expect(writes(requests, 'inventory_issue_purchase_order')).toHaveLength(0);
  });

  it('sin permiso de crear compras ni de existencias solo consulta y muestra «—»', async () => {
    useTransport((request) =>
      request.operation === 'inventory_purchase_suggestions'
        ? {
            status: 200,
            body: {
              ...suggestionPage([
                suggestion({
                  reorder_point: null,
                  suggested_quantity: null,
                  suggested_base_quantity: null,
                  stock_visible: false,
                  stock: null,
                  pending_quantity: null,
                  draft_quantity: null,
                  need_quantity: null,
                }),
              ]),
              stock_visible: false,
            },
          }
        : undefined,
    );
    await openTab('Sugerencias de compra', ['inventory.read', 'inventory.purchases.receive']);
    const group = await screen.findByRole('region', { name: 'Sugerencias: Molinos del Norte' });
    const row = within(group).getByText('Harina').closest('tr')!;
    expect(within(row).getAllByText('—').length).toBeGreaterThanOrEqual(2);
    expect(within(group).queryByRole('columnheader', { name: 'Existencia' })).toBeNull();
    expect(within(group).queryByRole('button', { name: 'Crear orden en borrador' })).toBeNull();
    expect(screen.getByText(/Crear el borrador requiere el permiso/)).toBeTruthy();
  });
});

describe('valores sin permiso de valuación', () => {
  it('Existencias muestra el costo promedio pero no el valor del insumo sin valuation.read', async () => {
    useTransport((request) =>
      request.operation === 'inventory_stock'
        ? { status: 200, body: stockPage(true, false) }
        : undefined,
    );
    await openTab('Existencias', [
      'inventory.read',
      'inventory.stock.read',
      'inventory.costs.read',
    ]);
    const row = (await screen.findByText('Harina')).closest('tr')!;
    expect(within(row).getByText('$18.00 / kg')).toBeTruthy();
    expect(screen.getByRole('columnheader', { name: 'Costo promedio' })).toBeTruthy();
    expect(screen.queryByRole('columnheader', { name: 'Valor del insumo' })).toBeNull();
    expect(within(row).queryByText('$225.00')).toBeNull();
  });

  it('Movimientos acepta la valuación posterior en null', async () => {
    // Con costos pero sin inventory.valuation.read, Go envía la valuación posterior en null.
    const page = movementPage(true, false);
    expect(page.items[0]?.valuation_value_after).toBeNull();
    useTransport((request) =>
      request.operation === 'inventory_movements' ? { status: 200, body: page } : undefined,
    );
    await openTab('Movimientos', [
      'inventory.read',
      'inventory.movements.read',
      'inventory.costs.read',
    ]);
    const row = (await screen.findByText('Harina')).closest('tr')!;
    expect(within(row).getByText('+$225.00')).toBeTruthy();
  });
});

describe('controles numéricos de la fase D', () => {
  it('ningún formulario de ajustes, transferencias, conteos ni niveles usa type="number"', async () => {
    useTransport((request) => {
      if (request.operation === 'inventory_counts') {
        return { status: 200, body: countPage([count()]) };
      }
      if (request.operation === 'inventory_count') {
        return { status: 200, body: count() };
      }
      if (request.operation === 'inventory_items') {
        return { status: 200, body: itemPage([item(), tomato]) };
      }
      return undefined;
    });
    renderInventory(controlPermissions);
    await openInventorySection('Ajustes y mermas');
    fireEvent.click(await screen.findByRole('button', { name: 'Registrar ajuste' }));
    await screen.findByRole('group', { name: 'Renglón 1' });
    expect(document.querySelectorAll('input[type="number"]')).toHaveLength(0);
    await openInventorySection('Transferencias');
    fireEvent.click(await screen.findByRole('button', { name: 'Nueva transferencia' }));
    await screen.findByRole('group', { name: 'Renglón 1' });
    expect(document.querySelectorAll('input[type="number"]')).toHaveLength(0);
    await openInventorySection('Conteos');
    fireEvent.click(
      await screen.findByRole('button', { name: 'Abrir el conteo 7 de Almacén general' }),
    );
    await screen.findByRole('textbox', { name: 'Cantidad contada de Harina' });
    expect(document.querySelectorAll('input[type="number"]')).toHaveLength(0);
    await openInventorySection('Insumos');
    fireEvent.click(await screen.findByRole('button', { name: 'Editar el insumo Harina' }));
    const levels = await screen.findByRole('region', { name: 'Niveles por almacén' });
    fireEvent.click(
      await within(levels).findByRole('button', { name: 'Editar el nivel de Almacén general' }),
    );
    expect(document.querySelectorAll('input[type="number"]')).toHaveLength(0);
  });
});
