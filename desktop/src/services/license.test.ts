// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from 'vitest';
import { api } from './api';
import {
  formatLicenseKey,
  licenseChangeEvent,
  licenseErrorText,
  licenseKeyProblem,
  licenseNotices,
  licenseStateExplanation,
  seatsHeldText,
  seatUnavailableText,
} from './license';
import type { LicenseSummary } from './license';
import { kitchenDisplayCapability, rememberServerCapabilities } from './serverCapabilities';

// Etapa L1 (Fase E): los textos de la licencia salen de los códigos de error y de sus details (nunca del message del
// servidor de licencias), en un solo lugar (services/license.ts).

const native = vi.hoisted(() => ({ invoke: vi.fn() }));
vi.mock('@tauri-apps/api/core', () => ({ invoke: native.invoke, isTauri: () => true }));

afterEach(() => {
  vi.restoreAllMocks();
  vi.clearAllMocks();
});

const clockOK = {
  blocked: false,
  local_time: '2026-11-23T16:15:00Z',
  last_activity_time: '2026-11-23T16:10:00Z',
  margin_minutes: 10,
  timezone: 'America/Monterrey',
  local_time_text: 'lunes 23 nov 2026, 10:15',
  last_activity_time_text: 'lunes 23 nov 2026, 10:10',
  license_state: 'active' as const,
  license_ok: true,
};

function summary(partial: Partial<LicenseSummary>): LicenseSummary {
  return {
    state: 'active',
    environment: 'test',
    copied: false,
    machine_unverified: false,
    revocation_reason: null,
    license_type: 'subscription',
    perpetual: false,
    expires_at: '2026-12-01T06:00:00Z',
    grace_days: 5,
    grace_ends_at: '2026-12-06T06:00:00Z',
    days_until_expiry: 30,
    grace_days_remaining: null,
    clock: clockOK,
    ...partial,
  };
}

describe('código de la licencia (L1-D4)', () => {
  it('se ordena al escribirlo: mayúsculas, sin espacios y con sus guiones', () => {
    expect(formatLicenseKey('prp2222')).toBe('PRP-2222');
    expect(formatLicenseKey('prp 2222 2222 2222')).toBe('PRP-2222-2222-2222');
    expect(formatLicenseKey('PRP-2222-2222-2222')).toBe('PRP-2222-2222-2222');
    expect(formatLicenseKey('')).toBe('');
  });

  it('dice qué le falta mientras se escribe', () => {
    expect(licenseKeyProblem('')).toContain('15 letras y números');
    expect(licenseKeyProblem('PRP-2222')).toBe('Faltan 8 caracteres.');
    expect(licenseKeyProblem('PRP-2222-2222-222')).toBe('Falta 1 carácter.');
    expect(licenseKeyProblem('PR0-2222-2222-2222')).toContain('no usa 0');
    expect(licenseKeyProblem('PRO-2222-2222-2222')).toContain('no usa O');
    expect(licenseKeyProblem('prp-2222-2222-2222')).toBeNull();
    expect(licenseKeyProblem('PRP222222222222')).toBeNull();
  });
});

describe('mensajes sin cupo (§5)', () => {
  it('dicen qué cupo falta y cuántos usa la licencia', () => {
    expect(seatUnavailableText({ seat: 'order_takers', in_use: 3, limit: 3 })).toBe(
      'No hay cupo de comandero libre (3 de 3 en uso). Pide que se cierre la sesión de otra terminal.',
    );
    expect(seatUnavailableText({ seat: 'cash_registers', in_use: 1, limit: 1 })).toBe(
      'No hay cupo de caja libre (1 de 1 en uso). Se libera al cerrar un turno de caja.',
    );
    expect(seatUnavailableText({ seat: 'kitchen_displays', in_use: 1, limit: 1 })).toBe(
      'No hay cupo de pantalla de cocina libre (1 de 1 en uso). Pide que otra terminal salga de Preparación.',
    );
    // Etapa K1: con un servidor que tiene tabletas de cocina, también la tableta emparejada ocupa el cupo.
    rememberServerCapabilities([kitchenDisplayCapability]);
    try {
      expect(seatUnavailableText({ seat: 'kitchen_displays', in_use: 1, limit: 1 })).toBe(
        'No hay cupo de pantalla de cocina libre (1 de 1 en uso). Pide que otra terminal salga de Preparación o que se libere el cupo de una tableta de cocina en Terminales.',
      );
    } finally {
      rememberServerCapabilities([]);
    }
  });

  it('con límite 0 (plan básico) dicen que la licencia no los incluye', () => {
    expect(seatUnavailableText({ seat: 'order_takers', in_use: 0, limit: 0 })).toContain(
      'La licencia no incluye comanderos (0 de 0)',
    );
    expect(seatUnavailableText({ seat: 'kitchen_displays', in_use: 0, limit: 0 })).toContain(
      'Preparación no está disponible',
    );
  });

  it('la columna de Terminales nombra los cupos de cada terminal', () => {
    expect(seatsHeldText([])).toBe('—');
    expect(seatsHeldText(undefined)).toBe('—');
    expect(seatsHeldText(['order_takers'])).toBe('Comandero');
    expect(seatsHeldText(['kitchen_displays', 'cash_registers'])).toBe('Caja y pantalla de cocina');
  });
});

describe('textos por código de error', () => {
  it('la hora nunca es un error genérico ni se confunde con la licencia', () => {
    expect(licenseErrorText('SYSTEM_CLOCK_INVALID', {})).toBe(
      'No se puede registrar: la computadora principal tiene la fecha y hora incorrectas. Avisa al encargado.',
    );
  });

  it('la puerta dice qué operación no se permite y qué sí se puede hacer', () => {
    expect(licenseErrorText('LICENSE_EXPIRED', { operation: 'capture' })).toBe(
      'La licencia del sistema venció y ya no permite capturar pedidos. Se puede seguir cobrando lo que está abierto, hacer cortes y consultar reportes. Avisa al encargado.',
    );
    expect(
      licenseErrorText('LICENSE_EXPIRED', { operation: 'open_shift', pending_accounts: false }),
    ).toContain('solo se puede abrir turno para cobrar cuentas pendientes');
    expect(
      licenseErrorText('LICENSE_REVOKED', { operation: 'capture', reason: 'transferred' }),
    ).toContain('La licencia se transfirió a otra computadora');
    expect(licenseErrorText('LICENSE_NOT_ACTIVATED', { operation: 'capture', copied: true })).toBe(
      'Esta instalación se copió de otra computadora. Actívala o transfiere la licencia.',
    );
    expect(licenseErrorText('LICENSE_NOT_ACTIVATED', { operation: 'administration' })).toContain(
      'no permite cambiar la administración',
    );
  });

  it('la gestión de la licencia tiene su texto por código y por motivo', () => {
    expect(licenseErrorText('LICENSE_ACTIVATION_LIMIT', { transfer_available: true })).toContain(
      'Puedes transferirla',
    );
    expect(licenseErrorText('LICENSE_ACTIVATION_LIMIT', { transfer_available: false })).toContain(
      'Comunícate con soporte',
    );
    expect(licenseErrorText('LICENSE_TRANSFER_LIMIT', {})).toContain('Comunícate con soporte');
    expect(licenseErrorText('LICENSE_FILE_INVALID', { reason: 'other_installation' })).toContain(
      'de otra computadora',
    );
    expect(licenseErrorText('LICENSE_SERVER_BUSY', { retry_after_seconds: 90 })).toContain(
      '2 minutos',
    );
    expect(licenseErrorText('SUPPORT_UNLOCK_INVALID', { reason: 'used' })).toContain('ya se usó');
    expect(
      licenseErrorText('LICENSE_TIME_MISMATCH', { server_time_text: 'lunes 23 nov 2026, 12:40' }),
    ).toContain('Corrígela a lunes 23 nov 2026, 12:40');
    expect(licenseErrorText('ACCOUNT_CONFLICT', {})).toBeNull();
  });

  it('api() usa estos textos y no el message que llegó', async () => {
    native.invoke.mockResolvedValue({
      status: 409,
      body: {
        error: {
          code: 'LICENSE_SEAT_UNAVAILABLE',
          message: 'texto que no se muestra',
          details: { seat: 'order_takers', in_use: 2, limit: 2 },
        },
      },
    });
    await expect(api('restaurant_open_account')).rejects.toMatchObject({
      code: 'LICENSE_SEAT_UNAVAILABLE',
      message:
        'No hay cupo de comandero libre (2 de 2 en uso). Pide que se cierre la sesión de otra terminal.',
    });
    // Un código ajeno a la licencia conserva el mensaje de Go.
    native.invoke.mockResolvedValue({
      status: 409,
      body: { error: { code: 'ACCOUNT_CONFLICT', message: 'Otra terminal cambió la cuenta.' } },
    });
    await expect(api('restaurant_open_account')).rejects.toMatchObject({
      message: 'Otra terminal cambió la cuenta.',
    });
  });

  it('un rechazo por la hora pide volver a leer el estado de la licencia', async () => {
    native.invoke.mockResolvedValue({
      status: 409,
      body: { error: { code: 'SYSTEM_CLOCK_INVALID', message: 'x', details: clockOK } },
    });
    const listener = vi.fn();
    window.addEventListener(licenseChangeEvent, listener);
    await expect(api('restaurant_open_account')).rejects.toMatchObject({
      code: 'SYSTEM_CLOCK_INVALID',
    });
    window.removeEventListener(licenseChangeEvent, listener);
    expect(listener).toHaveBeenCalledOnce();
  });
});

describe('avisos (§7)', () => {
  it('7 días antes: aviso diario para quien administra la licencia e indicador para los demás', () => {
    const expiring = summary({ days_until_expiry: 5 });
    const manager = licenseNotices(expiring, { canManage: true, mainComputer: true });
    expect(manager).toEqual([
      expect.objectContaining({
        kind: 'expiring',
        presentation: 'daily',
        title: 'La licencia vence en 5 días',
      }),
    ]);
    const others = licenseNotices(expiring, { canManage: false, mainComputer: true });
    expect(others).toEqual([
      expect.objectContaining({ kind: 'expiring_indicator', presentation: 'indicator' }),
    ]);
    // Más de 7 días, una perpetua o fuera de las suscripciones y pruebas: nada.
    expect(
      licenseNotices(summary({ days_until_expiry: 8 }), { canManage: true, mainComputer: true }),
    ).toEqual([]);
    expect(
      licenseNotices(
        summary({ perpetual: true, license_type: 'perpetual', days_until_expiry: null }),
        {
          canManage: true,
          mainComputer: true,
        },
      ),
    ).toEqual([]);
  });

  it('en tolerancia, uno diario para todos con los días que quedan', () => {
    const [notice] = licenseNotices(summary({ state: 'grace', grace_days_remaining: 3 }), {
      canManage: false,
      mainComputer: false,
    });
    expect(notice).toMatchObject({ kind: 'grace', presentation: 'daily' });
    expect(notice!.description).toContain('Quedan 3 días antes de que se bloquee la captura');
  });

  it('vencida, revocada y sin activar: fijos, con lo que sí se puede hacer', () => {
    const [expired] = licenseNotices(summary({ state: 'expired' }), {
      canManage: true,
      mainComputer: true,
    });
    expect(expired).toMatchObject({ kind: 'expired', presentation: 'fixed' });
    expect(expired!.description).toContain('Se puede seguir cobrando lo que está abierto');
    const [released] = licenseNotices(
      summary({ state: 'revoked', revocation_reason: 'deactivated' }),
      { canManage: true, mainComputer: true },
    );
    expect(released!.title).toBe('La licencia se liberó en esta computadora');
    const [copied] = licenseNotices(summary({ state: 'not_activated_with_data', copied: true }), {
      canManage: true,
      mainComputer: true,
    });
    expect(copied!.title).toBe(
      'Esta instalación se copió de otra computadora. Actívala o transfiere la licencia.',
    );
  });

  it('la hora incorrecta va aparte de la licencia, con su propio texto en las terminales', () => {
    const blocked = summary({ clock: { ...clockOK, blocked: true } });
    const terminal = licenseNotices(blocked, { canManage: false, mainComputer: false });
    expect(terminal[0]).toMatchObject({
      kind: 'clock',
      presentation: 'fixed',
      title:
        'No se puede registrar: la computadora principal tiene la fecha y hora incorrectas. Avisa al encargado.',
    });
    const main = licenseNotices(blocked, { canManage: false, mainComputer: true });
    expect(main[0]!.title).toBe('Sistema detenido por la fecha y hora de la computadora');
  });

  it('el estado se explica sin códigos', () => {
    expect(licenseStateExplanation({ state: 'not_activated_with_data' })).toContain(
      'Lo registrado se conserva',
    );
    expect(licenseStateExplanation({ state: 'revoked', revocation_reason: 'revoked' })).toContain(
      'fue revocada',
    );
  });
});
