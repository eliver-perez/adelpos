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

it('consulta el modo nativo y permite salir y volver desde el control visible', async () => {
  vi.mocked(invoke).mockResolvedValue(true);
  render(<WindowControls />);
  await waitFor(() => expect(invoke).toHaveBeenCalledWith('window_fullscreen_state'));
  fireEvent.click(screen.getByRole('button', { name: 'Salir de pantalla completa' }));
  await waitFor(() =>
    expect(invoke).toHaveBeenCalledWith('window_set_fullscreen', { fullscreen: false }),
  );
  fireEvent.click(await screen.findByRole('button', { name: 'Pantalla completa' }));
  await waitFor(() =>
    expect(invoke).toHaveBeenCalledWith('window_set_fullscreen', { fullscreen: true }),
  );
  expect(await screen.findByRole('button', { name: 'Salir de pantalla completa' })).toBeTruthy();
});
