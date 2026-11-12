// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, expect, it, vi } from 'vitest';
import { invoke } from '@tauri-apps/api/core';
import { WindowControls } from './WindowControls';

vi.mock('@tauri-apps/api/core', () => ({
  invoke: vi.fn(),
  isTauri: () => true,
}));

afterEach(cleanup);

// Etapa 9C: un solo botón de icono (reducir para salir, maximizar para entrar) con descripción
// accesible; el comportamiento nativo no cambia.
it('consulta el modo nativo y permite salir y volver desde el botón de icono', async () => {
  vi.mocked(invoke).mockResolvedValue(true);
  render(<WindowControls placement="bar" />);
  await waitFor(() => expect(invoke).toHaveBeenCalledWith('window_fullscreen_state'));
  const exit = screen.getByRole('button', { name: 'Salir de pantalla completa' });
  expect(exit.textContent).toBe('');
  expect(exit.getAttribute('title')).toBe('Salir de pantalla completa · Control + ⌘ + F');
  fireEvent.click(exit);
  await waitFor(() =>
    expect(invoke).toHaveBeenCalledWith('window_set_fullscreen', { fullscreen: false }),
  );
  const enter = await screen.findByRole('button', { name: 'Pantalla completa' });
  expect(enter.getAttribute('data-fullscreen')).toBe('false');
  fireEvent.click(enter);
  await waitFor(() =>
    expect(invoke).toHaveBeenCalledWith('window_set_fullscreen', { fullscreen: true }),
  );
  expect(await screen.findByRole('button', { name: 'Salir de pantalla completa' })).toBeTruthy();
});

it('fuera de la sesión queda fijo arriba a la derecha y avisa si el cambio falla', async () => {
  vi.mocked(invoke).mockImplementation(async (command: string) => {
    if (command === 'window_set_fullscreen') {
      throw new Error('rechazado');
    }
    return true;
  });
  const { container } = render(<WindowControls />);
  expect(container.querySelector('.window-controls')).not.toBeNull();
  fireEvent.click(await screen.findByRole('button', { name: 'Salir de pantalla completa' }));
  expect((await screen.findByRole('status')).textContent).toBe(
    'No se pudo cambiar el modo de ventana.',
  );
  expect(screen.getByRole('button', { name: 'Salir de pantalla completa' })).toBeTruthy();
});
