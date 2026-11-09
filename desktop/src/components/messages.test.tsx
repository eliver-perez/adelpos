// @vitest-environment jsdom

import { act, cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { APIError } from '../services/api';
import {
  dismissNotice,
  errorMessage,
  notifyError,
  notifyInfo,
  notifySuccess,
  notifyWarning,
} from '../services/notify';
import { useConfirmDialog } from './ConfirmDialog';
import type { ConfirmRequest } from './ConfirmDialog';

vi.mock('@tauri-apps/api/core', () => ({ invoke: vi.fn(), isTauri: () => true }));

// La pila de avisos la monta la preparación común de Vitest (src/testing/toasterSetup.tsx), igual
// que App.tsx en la aplicación.

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

// Sonner programa la aparición con setTimeout y el cierre con requestAnimationFrame: ambos se
// controlan con relojes simulados.
function useToastClock() {
  vi.useFakeTimers({
    toFake: [
      'setTimeout',
      'clearTimeout',
      'setInterval',
      'clearInterval',
      'Date',
      'requestAnimationFrame',
      'cancelAnimationFrame',
    ],
  });
}

// Avanza el reloj en pasos cortos, cada uno dentro de act: React aplica las actualizaciones de la
// pila al terminar cada paso, y los temporizadores que esas actualizaciones programan (la salida
// animada de un aviso) corren en el paso siguiente.
async function advance(milliseconds: number) {
  const step = 25;
  for (let elapsed = 0; elapsed < milliseconds; elapsed += step) {
    await act(async () => {
      await vi.advanceTimersByTimeAsync(Math.min(step, milliseconds - elapsed));
    });
  }
}

describe('avisos (Sonner)', () => {
  it('el éxito aparece arriba, se anuncia como estado y se cierra solo', async () => {
    useToastClock();
    notifySuccess('Ingreso registrado: $500.00');
    await advance(50);
    const notice = screen.getByRole('status', { name: 'Operación completada' });
    expect(notice.textContent).toContain('Ingreso registrado: $500.00');
    const toaster = notice.closest('[data-sonner-toaster]');
    expect(toaster?.getAttribute('data-y-position')).toBe('top');
    expect(toaster?.getAttribute('data-x-position')).toBe('center');
    // Sigue visible antes de los cuatro segundos.
    await advance(3000);
    expect(screen.queryByText('Ingreso registrado: $500.00')).not.toBeNull();
    // Se cierra solo pasado el tiempo (más la animación de salida).
    await advance(1500);
    expect(screen.queryByText('Ingreso registrado: $500.00')).toBeNull();
  });

  it('el puntero sobre el aviso (último toque en pantalla táctil) no detiene su cierre', async () => {
    useToastClock();
    notifySuccess('Pedido enviado a cocina');
    await advance(50);
    const notice = screen.getByRole('status', { name: 'Operación completada' });
    const stack = notice.closest('[data-sonner-toaster]') as HTMLElement;
    // Eventos del puntero dentro de la pila. La entrada desde un elemento de la página (el
    // «mouseout» que React convierte en onMouseEnter) solo se reproduce con un único árbol de React:
    // la cubre Playwright en Chromium y WebKit.
    fireEvent.mouseOver(notice);
    fireEvent.mouseMove(notice);
    fireEvent.pointerDown(notice);
    // La pila no se despliega ni pausa el temporizador.
    expect(notice.closest('[data-sonner-toast]')?.getAttribute('data-expanded')).toBe('false');
    expect(stack).not.toBeNull();
    await advance(4500);
    expect(screen.queryByText('Pedido enviado a cocina')).toBeNull();
  });

  it('el error permanece hasta que se cierra con un toque y no muestra el código interno', async () => {
    useToastClock();
    notifyError(
      new APIError('CASH_INSUFFICIENT', 'El efectivo disponible no alcanza para el retiro.', 409),
    );
    await advance(50);
    const alert = screen.getByRole('alert', { name: 'Error' });
    expect(alert.textContent).toContain('El efectivo disponible no alcanza para el retiro.');
    expect(alert.textContent).not.toContain('CASH_INSUFFICIENT');
    // El código queda solo como dato oculto para depuración.
    expect(alert.getAttribute('data-error-code')).toBe('CASH_INSUFFICIENT');
    await advance(20000);
    expect(screen.queryByRole('alert', { name: 'Error' })).not.toBeNull();
    const close = screen.getByRole('button', { name: 'Cerrar aviso' });
    expect(close.className).toContain('app-toast-close');
    fireEvent.click(close);
    await advance(500);
    expect(screen.queryByRole('alert', { name: 'Error' })).toBeNull();
  });

  it('ejecuta la acción opcional y cierra el aviso', async () => {
    useToastClock();
    const print = vi.fn();
    notifySuccess('Retiro registrado: $200.00 · Resguardo', {
      action: {
        label: 'Imprimir comprobante',
        onClick: print,
      },
    });
    await advance(50);
    fireEvent.click(screen.getByRole('button', { name: 'Imprimir comprobante' }));
    expect(print).toHaveBeenCalledTimes(1);
    await advance(500);
    expect(screen.queryByText('Retiro registrado: $200.00 · Resguardo')).toBeNull();
  });

  it('un aviso con el mismo texto o id sustituye al anterior en lugar de apilarse', async () => {
    useToastClock();
    notifyError('No se pudo guardar.');
    notifyError('No se pudo guardar.');
    notifyWarning('Sin conexión con el servidor.', { id: 'connection', persistent: true });
    notifyWarning('Sin conexión con el servidor. Reintentando.', {
      id: 'connection',
      persistent: true,
    });
    await advance(50);
    expect(screen.getAllByText('No se pudo guardar.')).toHaveLength(1);
    expect(screen.queryByText('Sin conexión con el servidor.')).toBeNull();
    expect(screen.getByText('Sin conexión con el servidor. Reintentando.')).not.toBeNull();
    dismissNotice('connection');
    await advance(500);
    expect(screen.queryByText('Sin conexión con el servidor. Reintentando.')).toBeNull();
  });

  it('la información se anuncia como estado con su propio tono', async () => {
    useToastClock();
    notifyInfo('Pedido enviado a cocina', { description: 'Mesa 4 · 3 productos' });
    await advance(50);
    const notice = screen.getByRole('status', { name: 'Información' });
    expect(notice.className).toContain('app-toast-info');
    expect(notice.textContent).toContain('Mesa 4 · 3 productos');
  });

  it('errorMessage nunca devuelve un código y usa el respaldo en español', () => {
    expect(errorMessage(new APIError('X', 'Mensaje de Go.'), 'Respaldo.')).toBe('Mensaje de Go.');
    expect(errorMessage(new APIError('API_ERROR', 'SHIFT_REQUIRED'), 'Respaldo.')).toBe(
      'Respaldo.',
    );
    expect(errorMessage(new Error(''), 'Respaldo.')).toBe('Respaldo.');
    expect(errorMessage('Texto directo.', 'Respaldo.')).toBe('Texto directo.');
    expect(errorMessage({ code: 'RARE' })).toBe('No se pudo completar la operación.');
  });
});

function ConfirmHarness({
  request,
  onResult,
}: {
  request: ConfirmRequest;
  onResult: (confirmed: boolean) => void;
}) {
  const { confirm, dialog } = useConfirmDialog();
  return (
    <div>
      <button
        type="button"
        onClick={() => {
          void confirm(request).then(onResult);
        }}
      >
        Abrir confirmación
      </button>
      {dialog}
    </div>
  );
}

const cancelItems: ConfirmRequest = {
  title: 'Cancelar 2 productos',
  message: (
    <>
      <p>Los productos se retiran de la cuenta.</p>
      <p>Esta acción no se puede deshacer.</p>
    </>
  ),
  confirmLabel: 'Cancelar productos',
  cancelLabel: 'Conservar',
  tone: 'danger',
};

describe('diálogo de confirmación estándar', () => {
  it('la variante de peligro diferencia la acción y acepta contenido enriquecido', async () => {
    const onResult = vi.fn();
    render(<ConfirmHarness request={cancelItems} onResult={onResult} />);
    fireEvent.click(screen.getByRole('button', { name: 'Abrir confirmación' }));
    const dialog = screen.getByRole('alertdialog', { name: 'Cancelar 2 productos' });
    expect(dialog.className).toContain('dialog-panel-danger');
    expect(dialog.getAttribute('aria-modal')).toBe('true');
    expect(dialog.textContent).toContain('Esta acción no se puede deshacer.');
    const confirm = screen.getByRole('button', { name: 'Cancelar productos' });
    expect(confirm.className).toContain('danger-solid-button');
    expect(confirm.className).toContain('dialog-button');
    fireEvent.click(confirm);
    await act(async () => {});
    expect(onResult).toHaveBeenCalledWith(true);
    expect(screen.queryByRole('alertdialog')).toBeNull();
  });

  it('enfoca la opción segura, atrapa el foco y lo devuelve al cerrar', async () => {
    const onResult = vi.fn();
    render(<ConfirmHarness request={cancelItems} onResult={onResult} />);
    const opener = screen.getByRole('button', { name: 'Abrir confirmación' });
    opener.focus();
    fireEvent.click(opener);
    const keep = screen.getByRole('button', { name: 'Conservar' });
    const confirm = screen.getByRole('button', { name: 'Cancelar productos' });
    expect(document.activeElement).toBe(keep);
    const dialog = screen.getByRole('alertdialog');
    // Tab desde la última opción vuelve a la primera; Mayús+Tab desde la primera va a la última.
    confirm.focus();
    fireEvent.keyDown(dialog, { key: 'Tab' });
    expect(document.activeElement).toBe(keep);
    fireEvent.keyDown(dialog, { key: 'Tab', shiftKey: true });
    expect(document.activeElement).toBe(confirm);
    fireEvent.click(keep);
    await act(async () => {});
    expect(onResult).toHaveBeenCalledWith(false);
    expect(document.activeElement).toBe(opener);
  });

  it('Escape cancela sin ejecutar la acción', async () => {
    const onResult = vi.fn();
    render(
      <ConfirmHarness
        request={{
          title: '¿Descartar los cambios?',
          message: 'Los cambios sin guardar se perderán.',
          confirmLabel: 'Descartar',
        }}
        onResult={onResult}
      />,
    );
    fireEvent.click(screen.getByRole('button', { name: 'Abrir confirmación' }));
    const dialog = screen.getByRole('alertdialog', { name: '¿Descartar los cambios?' });
    // Sin tono de peligro, la confirmación usa el botón principal.
    expect(screen.getByRole('button', { name: 'Descartar' }).className).toContain('primary-button');
    fireEvent.keyDown(dialog, { key: 'Escape' });
    await act(async () => {});
    expect(onResult).toHaveBeenCalledWith(false);
    expect(screen.queryByRole('alertdialog')).toBeNull();
  });
});
