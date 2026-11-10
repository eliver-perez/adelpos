// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { PendingOperationsProvider } from '../../hooks/pendingOperations';
import type { DrawerPrinter, RejectedOutflows, ShiftDetail } from '../../services/cash';
import type { CurrentShift, Shift } from '../../services/payments';
import { CashActions } from './CashActions';
import { DrawerAdmin } from './CashHardwareSettings';
import { CashPage } from './CashPage';
import { CashShortcut, CashShortcutSheet, NoShiftNotice, useCashShortcut } from './CashShortcut';
import {
  catalog,
  closingShift,
  currentWith,
  operationOf,
  registersWith,
  shiftID,
  shiftOf,
} from './cashTestData';
import { MovementsTimeline } from './MovementsView';

// Fase C de la etapa 7.1 en la interfaz: tarjetas de acción de Caja, cierre del panel tras
// registrar con su aviso e «Imprimir comprobante», atajo «Caja» de las pantallas operativas, aviso
// sin turno y filtro de intentos rechazados. Solo se sustituye el transporte nativo: las cifras son
// las que entregaría Go y la interfaz no calcula importes.

const native = vi.hoisted(() => ({ invoke: vi.fn() }));
vi.mock('@tauri-apps/api/core', () => ({ invoke: native.invoke, isTauri: () => true }));

type Reply = { status: number; body: unknown };
type Handler = (body: Record<string, unknown>, id: string | null) => Reply;

function serve(handlers: Record<string, Handler>) {
  native.invoke.mockImplementation(
    async (
      command: string,
      args: { operation: string; body?: Record<string, unknown>; resourceId?: string | null },
    ) => {
      if (command !== 'api_request') {
        throw new Error('Comando no esperado');
      }
      const handler = handlers[args.operation];
      if (!handler) {
        return { status: 200, body: {} };
      }
      return handler(args.body ?? {}, args.resourceId ?? null);
    },
  );
}

function sent(operation: string) {
  return native.invoke.mock.calls
    .filter(([command, args]) => command === 'api_request' && args?.operation === operation)
    .map(([, args]) => args as { resourceId: string | null; body: Record<string, unknown> });
}

function detailOf(shift: Shift): ShiftDetail {
  return {
    shift,
    opening_count: null,
    reports: [],
    summary: { payments: 0, refunds: 0, cash_operations: 0, partial_cuts: 0 },
    expected_visible: false,
  };
}

function caja(shift: Shift | null, drawer = false): Record<string, Handler> {
  return {
    payments_cash_registers: () => ({
      status: 200,
      body: registersWith(shift).map((entry) => ({ ...entry, drawer_enabled: drawer })),
    }),
    payments_current_shift: () => ({ status: 200, body: currentWith(shift) }),
    payments_cash_catalog: () => ({ status: 200, body: catalog }),
    payments_shift: () => ({ status: 200, body: shift ? detailOf(shift) : {} }),
  };
}

const income = operationOf({
  id: '00000000-0000-4000-8000-0000000000c9',
  kind: 'income',
  direction: 'in',
  amount: '150.00',
  equivalent_mxn: '150.00',
  concept: 'Aportación adicional al fondo',
  type: 'float_contribution',
  type_label: 'Aportación adicional al fondo',
  authorization_mode: null,
  authorized_by_name: null,
});

function type(scope: HTMLElement, digits: string) {
  const keypad = within(scope).getByRole('group', { name: 'Teclado numérico del importe' });
  for (const digit of digits) {
    fireEvent.click(within(keypad).getByRole('button', { name: digit }));
  }
}

function toast() {
  return screen.findByRole('status', { name: 'Operación completada' });
}

afterEach(() => {
  cleanup();
  native.invoke.mockReset();
});

describe('tarjetas de acción de Caja', () => {
  it('cada acción es una tarjeta con icono grande, acento por tipo y selección sin casillas', () => {
    const select = vi.fn();
    render(
      <CashActions
        can={() => true}
        shift={shiftOf()}
        ownRegister
        selected="withdrawal"
        onSelect={select}
        onCorrections={() => undefined}
      />,
    );
    const group = screen.getByRole('group', { name: 'Acciones del turno' });
    const tones: Record<string, string> = {
      Ingreso: 'entry',
      'Retiro de efectivo': 'exit',
      Gasto: 'exit',
      'Corte parcial': 'report',
      'Arqueo y cierre': 'closing',
      'Corrección o regularización': 'report',
    };
    for (const [label, tone] of Object.entries(tones)) {
      const card = within(group).getByRole('button', { name: label });
      expect(card.classList.contains('cash-action-card')).toBe(true);
      expect(card.getAttribute('data-tone')).toBe(tone);
      // Icono grande arriba del texto (decorativo: el nombre accesible es el texto).
      const icon = card.querySelector('.cash-action-icon svg');
      expect(icon?.getAttribute('width')).toBe('30');
      expect(icon?.getAttribute('aria-hidden')).toBe('true');
      expect(card.firstElementChild?.classList.contains('cash-action-icon')).toBe(true);
    }
    const selected = within(group).getByRole('button', { name: 'Retiro de efectivo' });
    expect(selected.getAttribute('aria-pressed')).toBe('true');
    expect(selected.classList.contains('cash-action-selected')).toBe(true);
    expect(
      within(group).getByRole('button', { name: 'Ingreso' }).getAttribute('aria-pressed'),
    ).toBe('false');
    // La que lleva a Movimientos no es seleccionable.
    expect(
      within(group)
        .getByRole('button', { name: 'Corrección o regularización' })
        .hasAttribute('aria-pressed'),
    ).toBe(false);
    expect(screen.queryAllByRole('checkbox')).toHaveLength(0);
    fireEvent.click(within(group).getByRole('button', { name: 'Gasto' }));
    expect(select).toHaveBeenCalledWith('expense');
  });
});

describe('flujo tras registrar en Caja', () => {
  function Page({ can = () => true }: { can?: (permission: string) => boolean }) {
    return (
      <PendingOperationsProvider>
        <CashPage can={can} userID="user-admin" timezone="America/Monterrey" />
      </PendingOperationsProvider>
    );
  }

  it('el ingreso cierra el panel, vuelve a Operación y ofrece «Imprimir comprobante»', async () => {
    const shift = shiftOf();
    serve({
      ...caja(shift),
      payments_shift_income: () => ({
        status: 200,
        body: { ...income, drawer: { requested: true, status: 'pending', job_id: 'cajon' } },
      }),
      payments_print_cash_operation: () => ({
        status: 200,
        body: {
          requested: true,
          status: 'pending',
          document_id: 'documento-9',
          includes_expected: false,
        },
      }),
    });
    render(<Page />);
    const actions = await screen.findByRole('group', { name: 'Acciones del turno' });
    fireEvent.click(within(actions).getByRole('button', { name: 'Ingreso' }));
    const panel = screen.getByRole('region', { name: 'Ingreso de efectivo' });
    fireEvent.click(within(panel).getByRole('button', { name: 'Aportación adicional al fondo' }));
    type(panel, '150');
    const reloads = sent('payments_shift').length;
    fireEvent.click(within(panel).getByRole('button', { name: 'Registrar ingreso $150.00' }));
    const confirmation = await toast();
    expect(confirmation.textContent).toContain(
      'Ingreso registrado: $150.00 · Aportación adicional al fondo',
    );
    // El comando del cajón conserva su texto: «enviado», nunca «cajón abierto».
    expect(confirmation.textContent).toContain('Comando de apertura enviado.');
    expect(confirmation.textContent).not.toMatch(/cajón abierto|se abrió|impreso/i);
    // El panel se cerró, las tarjetas siguen a la vista y los datos del turno se recargan.
    await waitFor(() =>
      expect(screen.queryByRole('region', { name: 'Ingreso de efectivo' })).toBeNull(),
    );
    expect(screen.getByRole('group', { name: 'Acciones del turno' })).toBeTruthy();
    await waitFor(() => expect(sent('payments_shift').length).toBeGreaterThan(reloads));
    // Sin documento, el aviso ofrece la impresión posterior existente.
    fireEvent.click(within(confirmation).getByRole('button', { name: 'Imprimir comprobante' }));
    await screen.findByText('Documento enviado a la cola de impresión.');
    const print = sent('payments_print_cash_operation');
    expect(print).toHaveLength(1);
    expect(print[0]?.resourceId).toBe(income.id);
    expect(print[0]?.body.operation_id).toMatch(/^[0-9a-f-]{36}$/);
  });

  it('sin permiso de imprimir el aviso no ofrece el comprobante', async () => {
    const shift = shiftOf();
    serve({
      ...caja(shift),
      payments_shift_income: () => ({ status: 200, body: income }),
    });
    render(<Page can={(permission) => permission !== 'cash.reports.print'} />);
    const actions = await screen.findByRole('group', { name: 'Acciones del turno' });
    fireEvent.click(within(actions).getByRole('button', { name: 'Ingreso' }));
    const panel = screen.getByRole('region', { name: 'Ingreso de efectivo' });
    fireEvent.click(within(panel).getByRole('button', { name: 'Aportación adicional al fondo' }));
    type(panel, '150');
    fireEvent.click(within(panel).getByRole('button', { name: 'Registrar ingreso $150.00' }));
    const confirmation = await toast();
    expect(within(confirmation).queryByRole('button', { name: 'Imprimir comprobante' })).toBeNull();
  });

  it('el corte parcial cierra su panel y se confirma sin cifras con «Imprimir corte»', async () => {
    const shift = shiftOf();
    serve({
      ...caja(shift),
      payments_shift_partial_cut: () => ({
        status: 200,
        body: { id: 'corte-1', shift_id: shift.id, kind: 'partial', sequence: 1 },
      }),
    });
    render(<Page />);
    const actions = await screen.findByRole('group', { name: 'Acciones del turno' });
    fireEvent.click(within(actions).getByRole('button', { name: 'Corte parcial' }));
    const panel = screen.getByRole('region', { name: 'Corte parcial' });
    fireEvent.click(within(panel).getByRole('button', { name: 'Generar corte parcial' }));
    const confirmation = await toast();
    expect(confirmation.textContent).toContain('Corte parcial #1 generado.');
    expect(confirmation.textContent).not.toMatch(/\$/);
    expect(within(confirmation).getByRole('button', { name: 'Imprimir corte' })).toBeTruthy();
    await waitFor(() => expect(screen.queryByRole('region', { name: 'Corte parcial' })).toBeNull());
  });
});

function ShortcutHarness({
  can,
  onOpenCash = () => undefined,
}: {
  can: (permission: string) => boolean;
  onOpenCash?: () => void;
}) {
  const shortcut = useCashShortcut({ active: true, can, onOpenCash });
  return (
    <>
      <CashShortcut shortcut={shortcut} can={can} />
      <NoShiftNotice shortcut={shortcut} can={can} />
      <CashShortcutSheet shortcut={shortcut} can={can} timezone="America/Monterrey" />
    </>
  );
}

function renderShortcut(
  can: (permission: string) => boolean = () => true,
  onOpenCash?: () => void,
) {
  return render(
    <PendingOperationsProvider>
      <ShortcutHarness can={can} onOpenCash={onOpenCash} />
    </PendingOperationsProvider>,
  );
}

function menuOptions() {
  const menu = screen.getByRole('group', { name: 'Menú rápido de caja' });
  return within(menu)
    .getAllByRole('button')
    .map((button) => button.textContent);
}

describe('atajo «Caja» de las pantallas operativas', () => {
  it('muestra el estado del turno de la caja de la terminal', async () => {
    serve(caja(shiftOf()));
    renderShortcut();
    expect(await screen.findByRole('button', { name: 'Caja · Turno abierto' })).toBeTruthy();
    cleanup();
    serve(caja(closingShift()));
    renderShortcut();
    expect(await screen.findByRole('button', { name: 'Caja · En arqueo' })).toBeTruthy();
    cleanup();
    serve(caja(null));
    renderShortcut();
    expect(await screen.findByRole('button', { name: 'Caja · Sin turno' })).toBeTruthy();
  });

  it('cada opción del menú rápido aparece solo con su permiso', async () => {
    serve(caja(shiftOf(), true));
    renderShortcut();
    const button = await screen.findByRole('button', { name: 'Caja · Turno abierto' });
    await waitFor(() => expect(sent('payments_cash_registers').length).toBeGreaterThan(0));
    fireEvent.click(button);
    expect(button.getAttribute('aria-expanded')).toBe('true');
    await waitFor(() =>
      expect(menuOptions()).toEqual([
        'Ingreso',
        'Retiro',
        'Gasto',
        'Abrir cajón',
        'Corte parcial',
        'Ir a Caja',
      ]),
    );
    cleanup();

    const limited = new Set(['cash.shifts.read', 'payments.read', 'cash.income.record']);
    serve(caja(shiftOf(), true));
    renderShortcut((permission) => limited.has(permission));
    fireEvent.click(await screen.findByRole('button', { name: 'Caja · Turno abierto' }));
    expect(menuOptions()).toEqual(['Ingreso', 'Ir a Caja']);
    cleanup();

    // Sin consultar turnos de caja no hay atajo.
    serve(caja(shiftOf(), true));
    renderShortcut((permission) => permission !== 'cash.shifts.read');
    await waitFor(() => expect(sent('payments_current_shift').length).toBeGreaterThan(0));
    expect(screen.queryByRole('button', { name: /^Caja/ })).toBeNull();
  });

  it('durante el arqueo las operaciones de efectivo esperan y sin turno ofrece abrirlo', async () => {
    serve(caja(closingShift()));
    renderShortcut();
    fireEvent.click(await screen.findByRole('button', { name: 'Caja · En arqueo' }));
    const menu = screen.getByRole('group', { name: 'Menú rápido de caja' });
    expect(menu.textContent).toContain('arqueo de cierre');
    expect(
      (within(menu).getByRole('button', { name: 'Ingreso' }) as HTMLButtonElement).disabled,
    ).toBe(true);
    expect(within(menu).queryByRole('button', { name: 'Abrir turno' })).toBeNull();
    cleanup();

    serve(caja(null));
    renderShortcut();
    fireEvent.click(await screen.findByRole('button', { name: 'Caja · Sin turno' }));
    expect(menuOptions()).toContain('Abrir turno');
  });

  it('«Ir a Caja» navega a la pantalla Caja', async () => {
    const open = vi.fn();
    serve(caja(shiftOf()));
    renderShortcut(() => true, open);
    fireEvent.click(await screen.findByRole('button', { name: 'Caja · Turno abierto' }));
    fireEvent.click(screen.getByRole('button', { name: 'Ir a Caja' }));
    expect(open).toHaveBeenCalledTimes(1);
    expect(screen.queryByRole('group', { name: 'Menú rápido de caja' })).toBeNull();
  });

  it('el ingreso desde el atajo usa el formulario de Caja y cierra la hoja con su aviso', async () => {
    serve({
      ...caja(shiftOf()),
      payments_shift_income: () => ({ status: 200, body: income }),
    });
    renderShortcut();
    fireEvent.click(await screen.findByRole('button', { name: 'Caja · Turno abierto' }));
    fireEvent.click(screen.getByRole('button', { name: 'Ingreso' }));
    const sheet = await screen.findByRole('dialog', { name: 'Ingreso de efectivo' });
    expect(sheet.getAttribute('aria-modal')).toBe('true');
    // Es el mismo formulario de la etapa 7, dentro de la hoja.
    const form = await within(sheet).findByRole('region', { name: 'Ingreso de efectivo' });
    fireEvent.click(within(form).getByRole('button', { name: 'Aportación adicional al fondo' }));
    type(form, '150');
    fireEvent.click(within(form).getByRole('button', { name: 'Registrar ingreso $150.00' }));
    const confirmation = await toast();
    expect(confirmation.textContent).toContain(
      'Ingreso registrado: $150.00 · Aportación adicional al fondo',
    );
    expect(within(confirmation).getByRole('button', { name: 'Imprimir comprobante' })).toBeTruthy();
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(sent('payments_shift_income')[0]?.resourceId).toBe(shiftID);
    expect(sent('payments_shift_income')[0]?.body).toMatchObject({
      type: 'float_contribution',
      amount: '150',
    });
  });

  it('la hoja se cierra con Escape sin registrar nada', async () => {
    serve(caja(shiftOf()));
    renderShortcut();
    fireEvent.click(await screen.findByRole('button', { name: 'Caja · Turno abierto' }));
    fireEvent.click(screen.getByRole('button', { name: 'Retiro' }));
    const sheet = await screen.findByRole('dialog', { name: 'Retiro de efectivo' });
    fireEvent.keyDown(sheet, { key: 'Escape' });
    expect(screen.queryByRole('dialog')).toBeNull();
    expect(sent('payments_shift_withdrawal')).toHaveLength(0);
  });
});

describe('aviso sin turno', () => {
  it('permite capturar, avisa que no se puede cobrar y ofrece «Abrir turno» con permiso', async () => {
    serve(caja(null));
    renderShortcut();
    const notice = await screen.findByRole('status', { name: 'Turno de caja' });
    expect(notice.textContent).toContain('No hay turno abierto: puedes capturar, pero no cobrar.');
    fireEvent.click(within(notice).getByRole('button', { name: 'Abrir turno' }));
    const sheet = await screen.findByRole('dialog', { name: 'Abrir turno' });
    expect(within(sheet).getByRole('region', { name: 'Turno de caja' })).toBeTruthy();
    cleanup();

    serve(caja(null));
    renderShortcut((permission) => permission !== 'cash.shifts.open');
    const plain = await screen.findByRole('status', { name: 'Turno de caja' });
    expect(within(plain).queryByRole('button', { name: 'Abrir turno' })).toBeNull();
    cleanup();

    const current: CurrentShift = currentWith(shiftOf());
    serve({ ...caja(shiftOf()), payments_current_shift: () => ({ status: 200, body: current }) });
    renderShortcut();
    await screen.findByRole('button', { name: 'Caja · Turno abierto' });
    expect(screen.queryByRole('status', { name: 'Turno de caja' })).toBeNull();
  });
});

describe('intentos rechazados en Movimientos', () => {
  const rejected: RejectedOutflows = {
    shift_id: shiftID,
    visible: true,
    rejections: [
      {
        id: 'rechazo-1',
        occurred_at: '2026-11-09T18:05:00Z',
        kind: 'withdrawal',
        currency: 'MXN',
        attempted_amount: '12345.50',
        actor_name: 'Cajera Rosa',
        device_name: 'Terminal 1',
        register_name: 'Caja 1',
        authorization_mode: 'supervisor',
        authorized_by_name: 'Supervisora Ana',
      },
    ],
  };

  function renderTimeline(can: (permission: string) => boolean) {
    serve({
      payments_shift_movements: () => ({
        status: 200,
        body: { shift: shiftOf(), movements: [] },
      }),
      payments_shift_operations: () => ({
        status: 200,
        body: { shift: shiftOf(), operations: [] },
      }),
      payments_shift_rejected_outflows: () => ({ status: 200, body: rejected }),
    });
    render(
      <PendingOperationsProvider>
        <MovementsTimeline shiftID={shiftID} ownShift={shiftOf()} can={can} timezone="UTC" />
      </PendingOperationsProvider>,
    );
  }

  it('el filtro solo existe con permiso de consultar el esperado y nunca muestra el esperado', async () => {
    renderTimeline(() => true);
    const filter = await screen.findByRole('combobox', { name: /Tipo de movimiento/ });
    expect(within(filter).getByRole('option', { name: 'Intentos rechazados' })).toBeTruthy();
    expect(sent('payments_shift_rejected_outflows')).toHaveLength(0);
    fireEvent.change(filter, { target: { value: 'rejected' } });
    const table = await screen.findByRole('table', { name: 'Intentos rechazados' });
    const row = within(table).getByText('Cajera Rosa').closest('tr')!;
    expect(row.textContent).toContain('Retiro');
    expect(row.textContent).toContain('$12,345.50');
    expect(row.textContent).toContain('Terminal 1');
    expect(row.textContent).toContain('Autorizó Supervisora Ana');
    expect(sent('payments_shift_rejected_outflows')[0]?.resourceId).toBe(shiftID);
    expect(document.body.textContent).not.toMatch(/esperado de \$|Efectivo esperado en caja/);
    cleanup();
    native.invoke.mockReset();

    renderTimeline((permission) => permission !== 'cash.expected.read');
    const plain = await screen.findByRole('combobox', { name: /Tipo de movimiento/ });
    await waitFor(() => expect(sent('payments_shift_movements').length).toBeGreaterThan(0));
    expect(within(plain).queryByRole('option', { name: 'Intentos rechazados' })).toBeNull();
    expect(sent('payments_shift_rejected_outflows')).toHaveLength(0);
  });
});

describe('cajón por CUPS en la configuración', () => {
  const printers: DrawerPrinter[] = [
    {
      id: 'impresora-usb',
      name: 'Tickets USB',
      transport: 'cups',
      active: true,
      compatible: true,
      connection: 'usb',
    },
    {
      id: 'impresora-airprint',
      name: 'Oficina',
      transport: 'cups',
      active: true,
      compatible: false,
      connection: 'dnssd',
      reason:
        'La cola Oficina es una impresora sin controlador (AirPrint o IPP Everywhere): CUPS convierte cada trabajo al formato de la impresora y no garantiza entregar el comando ESC/POS sin alterar.',
    },
    { id: 'impresora-red', name: 'Barra', transport: 'escpos_tcp', active: true, compatible: true },
  ];

  it('explica las colas sin datos raw y «Probar apertura» se confirma y solo informa el envío', async () => {
    serve({
      payments_cash_drawers: () => ({
        status: 200,
        body: {
          drawers: [
            {
              register_id: 'caja-1',
              register_name: 'Caja 1',
              register_active: true,
              printer_id: 'impresora-usb',
              printer_name: 'Tickets USB',
              pulse_pin: 0,
              enabled: true,
              revision: 2,
              updated_at: '2026-11-09T18:00:00Z',
            },
          ],
          printers,
        },
      }),
      payments_test_cash_drawer: () => ({
        status: 200,
        body: { requested: true, status: 'pending', job_id: 'prueba', event_id: 'evento' },
      }),
    });
    render(
      <PendingOperationsProvider>
        <DrawerAdmin execute={vi.fn()} />
      </PendingOperationsProvider>,
    );
    const select = await screen.findByLabelText('Impresora del cajón');
    const usb = within(select).getByRole('option', {
      name: 'Tickets USB · CUPS · USB',
    }) as HTMLOptionElement;
    const airprint = within(select).getByRole('option', {
      name: 'Oficina · CUPS · Bonjour · no compatible',
    }) as HTMLOptionElement;
    expect(usb.disabled).toBe(false);
    expect(airprint.disabled).toBe(true);
    const explained = screen.getByLabelText('Impresoras que no sirven para el cajón');
    expect(explained.textContent).toContain('AirPrint o IPP Everywhere');
    fireEvent.click(screen.getByRole('button', { name: 'Probar apertura' }));
    const dialog = await screen.findByRole('alertdialog');
    expect(dialog.textContent).toContain('Tickets USB');
    expect(sent('payments_test_cash_drawer')).toHaveLength(0);
    fireEvent.click(within(dialog).getByRole('button', { name: 'Enviar prueba de apertura' }));
    const confirmation = await toast();
    expect(confirmation.textContent).toContain('Comando de apertura enviado.');
    expect(confirmation.textContent).not.toMatch(/cajón abierto|se abrió/i);
    expect(sent('payments_test_cash_drawer')[0]?.resourceId).toBe('caja-1');
  });

  it('con cambios sin guardar no se prueba la apertura', async () => {
    serve({
      payments_cash_drawers: () => ({
        status: 200,
        body: {
          drawers: [
            {
              register_id: 'caja-1',
              register_name: 'Caja 1',
              register_active: true,
              printer_id: 'impresora-usb',
              printer_name: 'Tickets USB',
              pulse_pin: 0,
              enabled: true,
              revision: 2,
              updated_at: null,
            },
          ],
          printers,
        },
      }),
    });
    render(
      <PendingOperationsProvider>
        <DrawerAdmin execute={vi.fn()} />
      </PendingOperationsProvider>,
    );
    const select = await screen.findByLabelText('Impresora del cajón');
    fireEvent.change(select, { target: { value: 'impresora-red' } });
    const test = screen.getByRole('button', { name: 'Probar apertura' }) as HTMLButtonElement;
    expect(test.disabled).toBe(true);
    expect(screen.getByText('Guarda los cambios para probar la apertura con ellos.')).toBeTruthy();
  });
});
