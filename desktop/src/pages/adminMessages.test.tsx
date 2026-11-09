// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { QuoteBreakdown } from '../components/catalog/QuoteBreakdown';
import type { Quote } from '../services/catalog';
import { AuditPage } from './AuditPage';
import { DevicesPage } from './DevicesPage';
import { SessionsPage } from './SessionsPage';

// Etapa 7.1, fase A, en administración y catálogo: confirmaciones con avisos, el diálogo estándar
// de peligro para revocar, importes con miles y contadores con concordancia.

const native = vi.hoisted(() => ({ invoke: vi.fn() }));
vi.mock('@tauri-apps/api/core', () => ({ invoke: native.invoke, isTauri: () => true }));

type Reply = { status: number; body: unknown };
let replies: Record<string, Reply>;

function requests(operation: string) {
  return native.invoke.mock.calls
    .filter(([command, args]) => command === 'api_request' && args?.operation === operation)
    .map(([, args]) => args as { resourceId: string | null });
}

const device = {
  id: '00000000-0000-0000-0000-0000000000d1',
  name: 'Caja de barra',
  register_id: null,
  register_name: null,
  keyboard_mode: 'auto',
  last_seen_at: '2026-11-09T18:00:00Z',
  revoked_at: null,
};

const session = {
  id: '00000000-0000-0000-0000-0000000000a1',
  username: 'mesero1',
  device_name: 'Caja de barra',
  device_id: device.id,
  created_at: '2026-11-09T18:00:00Z',
  expires_at: '2999-01-01T00:00:00Z',
  last_seen_at: '2026-11-09T18:00:00Z',
  revoked_at: null,
  revoke_reason: null,
};

beforeEach(() => {
  replies = {
    devices: { status: 200, body: [device] },
    registers: { status: 200, body: [] },
    revoke_device: { status: 200, body: {} },
    sessions: { status: 200, body: [session] },
    revoke_session: { status: 200, body: {} },
    audit: {
      status: 200,
      body: {
        items: [],
        total: 1,
        page: 1,
        page_size: 25,
      },
    },
  };
  native.invoke.mockImplementation(async (command: string, args: { operation: string }) => {
    if (command !== 'api_request') {
      throw new Error(`Comando no esperado: ${command}`);
    }
    return replies[args.operation] ?? { status: 200, body: {} };
  });
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe('terminales y sesiones', () => {
  it('revocar una terminal pide confirmación de peligro y Escape la cancela', async () => {
    render(<DevicesPage can={() => true} />);
    fireEvent.click(await screen.findByRole('button', { name: 'Revocar' }));
    const dialog = screen.getByRole('alertdialog', { name: 'Revocar Caja de barra' });
    expect(dialog.className).toContain('dialog-panel-danger');
    expect(dialog.textContent).toContain('perderá acceso inmediatamente');
    fireEvent.keyDown(dialog, { key: 'Escape' });
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull());
    expect(requests('revoke_device')).toHaveLength(0);
  });

  it('al confirmar revoca la terminal y lo confirma con un aviso', async () => {
    render(<DevicesPage can={() => true} />);
    fireEvent.click(await screen.findByRole('button', { name: 'Revocar' }));
    const dialog = screen.getByRole('alertdialog', { name: 'Revocar Caja de barra' });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Confirmar revocación' }));
    const notice = await screen.findByRole('status', { name: 'Operación completada' });
    expect(notice.textContent).toContain('Terminal Caja de barra revocada.');
    expect(requests('revoke_device')[0]?.resourceId).toBe(device.id);
  });

  it('un rechazo al revocar una sesión es un error persistente sin el código interno', async () => {
    replies.revoke_session = {
      status: 403,
      body: { error: { code: 'PERMISSION_DENIED', message: 'No tienes permiso para revocar.' } },
    };
    render(<SessionsPage can={() => true} />);
    fireEvent.click(await screen.findByRole('button', { name: 'Revocar sesión' }));
    const alert = await screen.findByRole('alert', { name: 'Error' });
    expect(alert.textContent).toContain('No tienes permiso para revocar.');
    expect(alert.textContent).not.toContain('PERMISSION_DENIED');
  });

  it('una sesión revocada se confirma con un aviso', async () => {
    render(<SessionsPage can={() => true} />);
    fireEvent.click(await screen.findByRole('button', { name: 'Revocar sesión' }));
    expect(await screen.findByText('Sesión de mesero1 revocada.')).toBeTruthy();
  });
});

describe('contadores', () => {
  it('la auditoría concuerda el número de eventos', async () => {
    render(<AuditPage />);
    expect(await screen.findByText(/1 evento · Página 1/)).toBeTruthy();
  });
});

describe('desglose de la vista previa del catálogo', () => {
  const value: Quote = {
    calculated_at: '2026-11-09T18:00:00Z',
    timezone: 'America/Monterrey',
    price_list_name: 'Restaurante',
    price_list_revision: 1,
    subtotal: '1300.00',
    discount: '50.00',
    total: '1250.00',
    lines: [],
    promotions: [
      {
        id: 'promo',
        name: 'Martes de parrilla',
        applied: true,
        reason: 'Aplica los martes.',
        discount: '50.00',
      },
    ],
  };

  it('muestra los importes con miles y los descuentos con el signo antes del símbolo', () => {
    render(<QuoteBreakdown value={value} />);
    expect(screen.getByRole('heading', { name: '$1,250.00' })).toBeTruthy();
    expect(screen.getByText('$1,300.00')).toBeTruthy();
    expect(screen.getByText('−$50.00')).toBeTruthy();
    expect(screen.getByText('Martes de parrilla · −$50.00')).toBeTruthy();
  });
});
