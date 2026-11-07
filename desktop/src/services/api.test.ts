// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from 'vitest';
import { api } from './api';

const native = vi.hoisted(() => ({ invoke: vi.fn() }));
vi.mock('@tauri-apps/api/core', () => ({ invoke: native.invoke, isTauri: () => true }));

afterEach(() => {
  vi.restoreAllMocks();
  vi.clearAllMocks();
});

describe('errores centralizados de autenticación', () => {
  it('mantiene la sesión si la contraseña actual para el cambio es incorrecta', async () => {
    native.invoke.mockResolvedValue({
      status: 401,
      body: { error: { code: 'INVALID_CREDENTIALS', message: 'Contraseña actual incorrecta.' } },
    });
    const notification = vi.spyOn(window, 'dispatchEvent');
    await expect(api('change_password')).rejects.toMatchObject({ code: 'INVALID_CREDENTIALS' });
    expect(notification).not.toHaveBeenCalled();
  });

  it('notifica una terminal revocada incluso desde el inicio de sesión', async () => {
    native.invoke.mockResolvedValue({
      status: 401,
      body: { error: { code: 'DEVICE_REVOKED', message: 'Terminal revocada.' } },
    });
    const notification = vi.spyOn(window, 'dispatchEvent');
    await expect(api('login')).rejects.toMatchObject({ code: 'DEVICE_REVOKED' });
    expect(notification).toHaveBeenCalledOnce();
  });
});
