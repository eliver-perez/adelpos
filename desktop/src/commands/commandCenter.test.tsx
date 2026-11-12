// @vitest-environment jsdom

import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { useRef, useState } from 'react';
import type { ReactNode } from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { useConfirmDialog } from '../components/ConfirmDialog';
import { NumericKeypad } from '../components/NumericKeypad';
import type { OperationCommandHandlers, OperationCommandState } from './operationBridge';
import { useCommandCenter } from './useCommandCenter';
import type { CommandCenterOptions } from './useCommandCenter';

// Etapa 9C, fase C: la capa de teclado, el menú nativo simulado y la paleta montados con el centro
// de comandos real. jsdom no se identifica como macOS: los atajos usan Ctrl.

const native = vi.hoisted(() => ({
  invoke: vi.fn(),
  channels: [] as { onmessage?: (message: unknown) => void }[],
}));

vi.mock('@tauri-apps/api/core', () => ({
  invoke: native.invoke,
  isTauri: () => true,
  Channel: class {
    onmessage?: (message: unknown) => void;
    constructor() {
      native.channels.push(this);
    }
  },
}));

const operationState: OperationCommandState = {
  active: true,
  view: 'tables',
  captureMode: false,
  checkoutOpen: false,
  blocked: false,
  tableForNewAccount: null,
  precheckReady: false,
};

function handlers(overrides: Partial<OperationCommandHandlers> = {}): OperationCommandHandlers {
  return {
    newTableAccount: vi.fn(),
    accountWithoutTable: vi.fn(),
    newTakeaway: vi.fn(),
    openPrecheck: vi.fn(),
    leaveCapture: vi.fn(),
    chooseTable: vi.fn(),
    tables: () => [
      { id: 'm5', name: 'Mesa 5', areaName: 'Terraza' },
      { id: 'm15', name: 'Mesa 15', areaName: 'Salón' },
    ],
    back: vi.fn(() => true),
    ...overrides,
  };
}

const permissions = [
  'accounts.read',
  'accounts.open',
  'restaurant.tables.read',
  'restaurant.areas.read',
  'takeaway.read',
  'takeaway.manage',
  'kitchen.states.read',
  'cash.shifts.read',
  'payments.read',
  'catalog.read',
];

type HarnessProps = Partial<Omit<CommandCenterOptions, 'operationHandlers'>> & {
  operationHandlers?: OperationCommandHandlers;
  children?: ReactNode;
};

let requestScreen: ReturnType<typeof vi.fn>;
let openCashSheet: ReturnType<typeof vi.fn>;

function Harness({ operationHandlers, children, ...props }: HarnessProps) {
  const ref = useRef<OperationCommandHandlers | null>(operationHandlers ?? handlers());
  const { overlay } = useCommandCenter({
    authenticated: true,
    permissions,
    screen: 'restaurant',
    operation: operationState,
    operationHandlers: ref,
    cashOptions: [
      { id: 'income', disabled: false },
      { id: 'withdrawal', disabled: true },
      { id: 'cash', disabled: false },
    ],
    requestScreen: requestScreen as unknown as CommandCenterOptions['requestScreen'],
    openCashSheet: openCashSheet as unknown as CommandCenterOptions['openCashSheet'],
    logout: vi.fn(async () => undefined),
    toggleTheme: vi.fn(async () => undefined),
    ...props,
  });
  return (
    <main className="admin-main">
      {children}
      {overlay}
    </main>
  );
}

function press(target: Window | Document | Element, init: KeyboardEventInit) {
  return fireEvent.keyDown(target, { bubbles: true, cancelable: true, ...init });
}

async function flushTimers() {
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 5));
  });
}

beforeEach(() => {
  requestScreen = vi.fn(async () => true);
  openCashSheet = vi.fn();
  native.invoke.mockImplementation(async (command: string) => {
    if (command === 'window_fullscreen_state') {
      return true;
    }
    return undefined;
  });
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe('atajos de teclado', () => {
  it('Ctrl+1–5 navegan con requestScreen y no actúan mientras se escribe en un campo', async () => {
    render(
      <Harness>
        <input aria-label="Nombre" />
      </Harness>,
    );
    press(window, { key: '3', ctrlKey: true });
    expect(requestScreen).toHaveBeenLastCalledWith('kitchen');
    press(document.body, { key: '4', ctrlKey: true });
    expect(requestScreen).toHaveBeenLastCalledWith('cash');
    requestScreen.mockClear();
    const field = screen.getByLabelText('Nombre');
    field.focus();
    const event = press(field, { key: '3', ctrlKey: true });
    // La pulsación se queda en el campo: sin navegar y sin que el navegador haga lo suyo.
    expect(event).toBe(false);
    expect(requestScreen).not.toHaveBeenCalled();
    // Sin modificador no hay atajo.
    press(document.body, { key: '3' });
    expect(requestScreen).not.toHaveBeenCalled();
  });

  it('las acciones de caja no tienen atajo y la precuenta sin cuenta solo lo explica', async () => {
    const operation = handlers();
    render(<Harness operationHandlers={operation} />);
    press(document.body, { key: 'p', ctrlKey: true });
    expect(operation.openPrecheck).not.toHaveBeenCalled();
    expect(
      await screen.findByText('Elige una cuenta con productos enviados a cocina.'),
    ).toBeTruthy();
    expect(openCashSheet).not.toHaveBeenCalled();
  });

  it('con un diálogo de confirmación abierto los atajos no actúan detrás de él', async () => {
    function WithDialog() {
      const { confirm, dialog } = useConfirmDialog();
      return (
        <Harness>
          <button
            type="button"
            onClick={() =>
              void confirm({ title: 'Confirmar', message: 'Mensaje', confirmLabel: 'Sí' })
            }
          >
            Abrir confirmación
          </button>
          {dialog}
        </Harness>
      );
    }
    render(<WithDialog />);
    fireEvent.click(screen.getByRole('button', { name: 'Abrir confirmación' }));
    const dialog = screen.getByRole('alertdialog');
    press(within(dialog).getByRole('button', { name: 'Cancelar' }), { key: '1', ctrlKey: true });
    expect(requestScreen).not.toHaveBeenCalled();
  });
});

describe('Escape', () => {
  it('vuelve un nivel fuera de los campos y en un campo solo sale de él', async () => {
    const operation = handlers();
    render(
      <Harness operationHandlers={operation}>
        <input aria-label="Buscar" />
      </Harness>,
    );
    const field = screen.getByLabelText('Buscar');
    field.focus();
    press(field, { key: 'Escape' });
    await flushTimers();
    expect(document.activeElement).not.toBe(field);
    expect(operation.back).not.toHaveBeenCalled();
    press(document.body, { key: 'Escape' });
    await flushTimers();
    expect(operation.back).toHaveBeenCalledTimes(1);
  });

  it('no rompe el Escape del teclado del cobro ni el de los diálogos', async () => {
    const operation = handlers();
    function Checkout() {
      const [amount, setAmount] = useState('120');
      const { confirm, dialog } = useConfirmDialog();
      return (
        <Harness operationHandlers={operation}>
          <NumericKeypad value={amount} onChange={setAmount} captureKeyboard />
          <output aria-label="Importe">{amount}</output>
          <button
            type="button"
            onClick={() =>
              void confirm({ title: 'Confirmar', message: 'Mensaje', confirmLabel: 'Sí' })
            }
          >
            Abrir confirmación
          </button>
          {dialog}
        </Harness>
      );
    }
    render(<Checkout />);
    // El teclado físico del cobro limpia el importe con Escape; la capa global no vuelve de nivel.
    press(document.body, { key: 'Escape' });
    await flushTimers();
    expect(screen.getByLabelText('Importe').textContent).toBe('');
    expect(operation.back).not.toHaveBeenCalled();
    // Escape en un diálogo lo cierra a él y nada más.
    fireEvent.click(screen.getByRole('button', { name: 'Abrir confirmación' }));
    press(screen.getByRole('alertdialog'), { key: 'Escape' });
    await flushTimers();
    expect(screen.queryByRole('alertdialog')).toBeNull();
    expect(operation.back).not.toHaveBeenCalled();
  });

  it('en Administración vuelve a la pantalla anterior salvo con un formulario a la vista', async () => {
    const { rerender } = render(<Harness screen="users" />);
    press(document.body, { key: 'Escape' });
    await flushTimers();
    expect(requestScreen).toHaveBeenLastCalledWith('staff');
    requestScreen.mockClear();
    rerender(
      <Harness screen="catalog">
        <form>
          <input aria-label="Nombre del producto" />
        </form>
      </Harness>,
    );
    press(document.body, { key: 'Escape' });
    await flushTimers();
    expect(requestScreen).not.toHaveBeenCalled();
  });
});

describe('menú nativo simulado', () => {
  it('se suscribe por el canal, ejecuta la opción e informa el estado habilitado', async () => {
    render(<Harness />);
    await waitFor(() =>
      expect(native.invoke).toHaveBeenCalledWith('menu_set_state', {
        enabled: expect.arrayContaining(['view.tables', 'view.kitchen', 'cash.income']),
      }),
    );
    const state = native.invoke.mock.calls.find(([command]) => command === 'menu_set_state');
    expect(state?.[1].enabled).not.toContain('cash.withdrawal');
    expect(state?.[1].enabled).not.toContain('file.print_precheck');
    const channel = native.channels.at(-1);
    expect(channel).toBeTruthy();
    act(() => channel!.onmessage?.({ action: 'cash.income' }));
    expect(openCashSheet).toHaveBeenCalledWith('income');
    // Una opción desconocida o deshabilitada no hace nada.
    act(() => channel!.onmessage?.({ action: 'api_request' }));
    act(() => channel!.onmessage?.({ action: 'cash.withdrawal' }));
    expect(openCashSheet).toHaveBeenCalledTimes(1);
  });

  it('el acelerador del menú y la pulsación de la misma acción se ejecutan una sola vez', async () => {
    render(<Harness />);
    const channel = native.channels.at(-1)!;
    press(document.body, { key: '3', ctrlKey: true });
    act(() => channel.onmessage?.({ action: 'view.kitchen' }));
    expect(requestScreen).toHaveBeenCalledTimes(1);
    // Mientras se escribe, la pulsación no actúa y su gemela del menú tampoco.
    render(<input aria-label="Nota" />);
    const field = screen.getByLabelText('Nota');
    field.focus();
    press(field, { key: '2', ctrlKey: true });
    act(() => channel.onmessage?.({ action: 'view.takeaway' }));
    expect(requestScreen).toHaveBeenCalledTimes(1);
  });
});

describe('paleta de comandos', () => {
  it('Ctrl+K la abre con teclado completo, filtrada por permisos y contexto', async () => {
    const operation = handlers();
    render(<Harness operationHandlers={operation} />);
    press(document.body, { key: 'k', ctrlKey: true });
    const dialog = await screen.findByRole('dialog', { name: 'Paleta de comandos' });
    const input = within(dialog).getByRole('combobox', { name: 'Buscar pantallas y acciones' });
    expect(document.activeElement).toBe(input);
    const list = within(dialog).getByRole('listbox', { name: 'Resultados' });
    expect(input.getAttribute('aria-controls')).toBe(list.id);
    // Sin permisos de administración no aparece Usuarios; sin turno abierto, Retiro se ve
    // deshabilitado con su motivo.
    fireEvent.change(input, { target: { value: 'usuarios' } });
    expect(within(list).queryAllByRole('option')).toHaveLength(0);
    expect(within(dialog).getByRole('status').textContent).toBe('Sin resultados para «usuarios».');
    fireEvent.change(input, { target: { value: 'retiro' } });
    const withdrawal = within(list).getByRole('option', { name: /Retiro/ });
    expect(withdrawal.getAttribute('aria-disabled')).toBe('true');
    press(input, { key: 'Enter' });
    expect(within(dialog).getByRole('status').textContent).toBe(
      'Requiere un turno abierto en la caja de esta terminal.',
    );
    expect(openCashSheet).not.toHaveBeenCalled();
    // «mesa 5» con flechas e Intro: elige la mesa con la misma función del mapa.
    fireEvent.change(input, { target: { value: 'mesa 5' } });
    const options = within(list).getAllByRole('option');
    expect(options.some((option) => option.textContent?.includes('Mesa 15'))).toBe(false);
    const target = options.findIndex((option) => option.textContent?.startsWith('Mesa 5'));
    for (let index = 0; index < target; index += 1) {
      press(input, { key: 'ArrowDown' });
    }
    expect(input.getAttribute('aria-activedescendant')).toBe(options[target]!.id);
    expect(options[target]!.getAttribute('aria-selected')).toBe('true');
    press(input, { key: 'Enter' });
    await waitFor(() => expect(operation.chooseTable).toHaveBeenCalledWith('m5'));
    expect(requestScreen).toHaveBeenCalledWith('restaurant');
    expect(screen.queryByRole('dialog', { name: 'Paleta de comandos' })).toBeNull();
  });

  it('Escape la cierra y «Atajos de teclado» se abre desde ella', async () => {
    render(<Harness />);
    press(document.body, { key: 'k', ctrlKey: true });
    let dialog = await screen.findByRole('dialog', { name: 'Paleta de comandos' });
    press(within(dialog).getByRole('combobox'), { key: 'Escape' });
    expect(screen.queryByRole('dialog', { name: 'Paleta de comandos' })).toBeNull();
    press(document.body, { key: 'k', ctrlKey: true });
    dialog = await screen.findByRole('dialog', { name: 'Paleta de comandos' });
    const input = within(dialog).getByRole('combobox');
    fireEvent.change(input, { target: { value: 'atajos' } });
    press(input, { key: 'Enter' });
    const shortcuts = await screen.findByRole('dialog', { name: 'Atajos de teclado' });
    expect(within(shortcuts).getByRole('table', { name: 'Ir a una sección' })).toBeTruthy();
    expect(within(shortcuts).getByText('Ctrl+1')).toBeTruthy();
    expect(within(shortcuts).getByText('F11')).toBeTruthy();
    expect(within(shortcuts).getByText('Esc')).toBeTruthy();
    fireEvent.click(within(shortcuts).getByRole('button', { name: 'Cerrar' }));
    expect(screen.queryByRole('dialog', { name: 'Atajos de teclado' })).toBeNull();
  });

  it('Ctrl+F lleva al buscador de la pantalla y en el mapa abre la búsqueda de mesas', async () => {
    const { rerender } = render(
      <Harness>
        <input type="search" aria-label="Buscar producto" />
      </Harness>,
    );
    press(document.body, { key: 'f', ctrlKey: true });
    expect(document.activeElement).toBe(screen.getByLabelText('Buscar producto'));
    (document.activeElement as HTMLElement).blur();
    rerender(<Harness />);
    press(document.body, { key: 'f', ctrlKey: true });
    const dialog = await screen.findByRole('dialog', { name: 'Buscar mesa' });
    expect(
      within(dialog)
        .getAllByRole('option')
        .map((option) => option.textContent),
    ).toEqual([expect.stringContaining('Mesa 5'), expect.stringContaining('Mesa 15')]);
  });

  it('«Acerca de AD-EL POS» muestra la versión desde el menú nativo', async () => {
    render(<Harness authenticated={false} permissions={[]} screen={null} operation={null} />);
    const channel = native.channels.at(-1)!;
    act(() => channel.onmessage?.({ action: 'help.about' }));
    const about = await screen.findByRole('dialog', { name: 'Acerca de AD-EL POS' });
    expect(within(about).getByText(/^Versión \d+\.\d+\.\d+$/)).toBeTruthy();
    // Antes de iniciar sesión la paleta no se abre.
    fireEvent.click(within(about).getByRole('button', { name: 'Cerrar' }));
    press(document.body, { key: 'k', ctrlKey: true });
    expect(screen.queryByRole('dialog', { name: 'Paleta de comandos' })).toBeNull();
  });
});
