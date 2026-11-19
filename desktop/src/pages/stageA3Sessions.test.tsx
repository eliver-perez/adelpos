// @vitest-environment jsdom

import { cleanup, render, screen, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { Session } from '../services/models';
import { SessionsPage, sessionState } from './SessionsPage';

// Etapa A3 (§1, hallazgo 2 de la regresión): Personal › Sesiones dice por qué se cerró una sesión en
// palabras del restaurante, nunca con el código que guarda Go (logout, admin_revoked…).

const native = vi.hoisted(() => ({ invoke: vi.fn() }));
vi.mock('@tauri-apps/api/core', () => ({ invoke: native.invoke, isTauri: () => true }));

const future = '2999-01-01T00:00:00Z';
const past = '2020-01-01T00:00:00Z';

let counter = 0;

function session(
  username: string,
  reason: string | null,
  revoked = true,
  expires = future,
): Session {
  return {
    id: `00000000-0000-4000-8000-${String((counter += 1)).padStart(12, '0')}`,
    username,
    device_name: 'Caja principal',
    device_id: '00000000-0000-4000-8000-0000000000d1',
    created_at: '2026-11-19T15:00:00Z',
    expires_at: expires,
    last_seen_at: '2026-11-19T15:00:00Z',
    revoked_at: revoked ? '2026-11-19T16:00:00Z' : null,
    revoke_reason: reason,
  };
}

// Cada motivo que escribe Go (backend/internal/administration y legacyimport) con su frase.
const reasons: [string, string][] = [
  ['logout', 'Cerró su sesión'],
  ['admin_revoked', 'Cerrada desde Sesiones'],
  ['device_revoked', 'Terminal revocada'],
  ['expired', 'Expirada'],
  ['user_disabled', 'Usuario desactivado'],
  ['support_reset', 'Cerrada al restablecer el acceso de soporte'],
  ['replaced', 'Reemplazada por un acceso nuevo en esta terminal'],
  ['password_changed', 'Cerrada al cambiar la contraseña'],
  ['pin_changed', 'Cerrada al cambiar el PIN'],
  ['pin_removed', 'Cerrada al quitar el PIN'],
];

beforeEach(() => {
  const listed = [
    session('activa', null, false),
    session('vencida', null, false, past),
    ...reasons.map(([reason]) => session(`usuario-${reason}`, reason)),
    session('desconocida', 'otro_motivo_futuro'),
  ];
  native.invoke.mockImplementation(async (command: string, args: { operation: string }) => {
    if (command !== 'api_request') {
      return null;
    }
    if (args.operation === 'sessions') {
      return { status: 200, body: listed };
    }
    return { status: 200, body: {} };
  });
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe('Estado de las sesiones en palabras del restaurante (etapa A3, §1)', () => {
  it('cada motivo de Go tiene su frase y uno desconocido se dice en general', () => {
    for (const [reason, label] of reasons) {
      expect(sessionState(session('x', reason))).toBe(label);
    }
    expect(sessionState(session('x', 'otro_motivo_futuro'))).toBe('Cerrada');
    expect(sessionState(session('x', null))).toBe('Cerrada');
    expect(sessionState(session('x', null, false))).toBe('Activa');
    expect(sessionState(session('x', null, false, past))).toBe('Expirada');
  });

  it('la tabla nunca muestra el código del motivo', async () => {
    render(<SessionsPage can={() => true} />);
    const table = (await screen.findByText('usuario-logout')).closest('table') as HTMLElement;
    for (const [reason, label] of reasons) {
      const row = within(table).getByText(`usuario-${reason}`).closest('tr') as HTMLElement;
      expect(within(row).getAllByRole('cell')[3]?.textContent).toBe(label);
    }
    const unknown = within(table).getByText('desconocida').closest('tr') as HTMLElement;
    expect(within(unknown).getAllByRole('cell')[3]?.textContent).toBe('Cerrada');
    const states = within(table)
      .getAllByRole('row')
      .slice(1)
      .map((row) => within(row).getAllByRole('cell')[3]?.textContent ?? '');
    for (const [reason] of [...reasons, ['otro_motivo_futuro', '']]) {
      expect(states).not.toContain(reason);
    }
    expect(states.join(' ')).not.toMatch(/[a-z]+_[a-z]+|\blogout\b/);
    // Solo la activa ofrece «Revocar sesión».
    expect(within(table).getAllByRole('button', { name: 'Revocar sesión' })).toHaveLength(1);
  });
});
