// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { isWindows } from '../../commands/platform';
import { drawerConnectionLabel } from '../../services/cash';
import { DrawerAdmin } from '../cash/CashHardwareSettings';
import type { Printer } from '../../services/kitchen';
import { PrintersAdmin } from './PrintersAdmin';
import { PrintQueue } from './PrintQueue';

// Etapa 9E (§3.5): en Windows la cola de impresión del equipo es la de las impresoras instaladas en
// Windows, que reciben siempre ESC/POS sin convertir. Los textos cambian solo en Windows: sin
// simular la plataforma (jsdom no se presenta como Windows), todo queda como en macOS.

const native = vi.hoisted(() => ({ invoke: vi.fn() }));
vi.mock('@tauri-apps/api/core', () => ({ invoke: native.invoke, isTauri: () => true }));

const discovered = [
  {
    queue: 'POS-80C',
    status:
      'Puerto USB001 (USB) · Controlador POS-80C · Windows no informa problemas. Recibe los comandos ESC/POS sin alterar.',
    options: '',
  },
];

const saved: Printer = {
  id: 'p1',
  name: 'Tickets de caja',
  transport: 'cups',
  destination: 'POS-80C',
  port: 9100,
  paper_width: 80,
  copies: 1,
  supports_cut: true,
  cut: true,
  cut_mode: 'full',
  cut_feed_lines: 4,
  cutter_option: '',
  accepts_escpos: false,
  columns: null,
  active: true,
  fallback_id: null,
  revision: 1,
};

// simulatePlatform presenta jsdom como el WebView de un sistema (navigator.platform y userAgent).
function simulatePlatform(platform: string, userAgent: string) {
  vi.spyOn(window.navigator, 'platform', 'get').mockReturnValue(platform);
  vi.spyOn(window.navigator, 'userAgent', 'get').mockReturnValue(userAgent);
}

const windowsAgent =
  'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36 Edg/130.0.0.0';
const macAgent =
  'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko)';

beforeEach(() => {
  native.invoke.mockImplementation(async (_command: string, args: { operation: string }) => ({
    status: 200,
    body:
      args.operation === 'kitchen_printers'
        ? [saved]
        : args.operation === 'kitchen_discovery'
          ? discovered
          : [],
  }));
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.clearAllMocks();
});

describe('detección de Windows', () => {
  it('reconoce el WebView de Windows y no el de macOS ni el agente de jsdom', () => {
    expect(isWindows()).toBe(false);
    simulatePlatform('MacIntel', macAgent);
    expect(isWindows()).toBe(false);
    vi.restoreAllMocks();
    simulatePlatform('Win32', windowsAgent);
    expect(isWindows()).toBe(true);
  });
});

describe('impresoras en Windows', () => {
  it('lista las impresoras instaladas en Windows sin la marca ESC/POS ni la opción de corte de CUPS', async () => {
    simulatePlatform('Win32', windowsAgent);
    render(<PrintersAdmin can={() => true} execute={vi.fn() as never} busy={false} />);
    expect(screen.getByText(/Se usan las impresoras instaladas en Windows/)).toBeTruthy();
    expect(screen.queryByText(/CUPS/)).toBeNull();
    expect(await screen.findByText(/Impresora instalada en Windows · 80 mm/)).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: /Impresoras instaladas en Windows/ }));
    expect(await screen.findByText(/Puerto USB001 \(USB\)/)).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Usar esta impresora' })).toBeTruthy();
    expect(screen.queryByText(/Opciones del controlador/)).toBeNull();
    fireEvent.click(screen.getByRole('button', { name: /Tickets de caja/ }));
    expect(screen.getByLabelText('Nombre de la impresora en Windows')).toBeTruthy();
    expect(
      screen.queryByLabelText('Esta impresora entiende ESC/POS (formato y logotipo)'),
    ).toBeNull();
    expect(screen.queryByLabelText('Opción de corte anunciada por CUPS')).toBeNull();
    expect(screen.getByText(/En Windows la impresora recibe los comandos ESC\/POS/)).toBeTruthy();
    expect(screen.queryByText(/CUPS/)).toBeNull();
  });

  it('sin Windows los textos de CUPS no cambian', async () => {
    simulatePlatform('MacIntel', macAgent);
    render(<PrintersAdmin can={() => true} execute={vi.fn() as never} busy={false} />);
    expect(
      screen.getByText(
        'Se usan las impresoras instaladas en el equipo servidor (colas CUPS, el sistema de impresión de macOS) y las impresoras de tickets conectadas a la red. Una impresora conectada a otra terminal todavía no se puede usar: conéctala a la red o al equipo servidor.',
      ),
    ).toBeTruthy();
    expect(await screen.findByText(/CUPS del equipo servidor · 80 mm/)).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: /Colas anunciadas por CUPS/ }));
    expect(await screen.findByText(/Puerto USB001 \(USB\)/)).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Usar esta cola' })).toBeTruthy();
    expect(screen.getByText('Opciones del controlador POS-80C')).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: /Tickets de caja/ }));
    expect(screen.getByLabelText('Nombre de cola CUPS')).toBeTruthy();
    expect(
      screen.getByLabelText('Esta impresora entiende ESC/POS (formato y logotipo)'),
    ).toBeTruthy();
    expect(screen.getByLabelText('Opción de corte anunciada por CUPS')).toBeTruthy();
  });

  it('la cola de impresión y el cajón nombran la cola de Windows', () => {
    simulatePlatform('Win32', windowsAgent);
    render(
      <PrintQueue
        can={() => true}
        execute={vi.fn() as never}
        busy={false}
        timezone="America/Mexico_City"
      />,
    );
    expect(
      screen.getByText(
        '«Enviado a la impresora» significa que la cola de impresión de Windows o la impresora de red lo aceptó; no confirma que salió papel. Si no se sabe si salió, no se reenvía solo.',
      ),
    ).toBeTruthy();
    const usb = { id: 'p1', name: 'Caja', transport: 'cups', active: true, compatible: true };
    expect(drawerConnectionLabel({ ...usb, connection: 'usb' })).toBe('Impresora de Windows · USB');
    expect(drawerConnectionLabel({ ...usb, connection: 'socket' })).toBe(
      'Impresora de Windows · TCP/IP estándar',
    );
    expect(drawerConnectionLabel({ ...usb, compatible: false })).toBe('Impresora de Windows');
    vi.restoreAllMocks();
    expect(drawerConnectionLabel({ ...usb, connection: 'usb' })).toBe('CUPS · USB');
    expect(drawerConnectionLabel({ ...usb, connection: 'socket' })).toBe('CUPS · AppSocket');
  });

  it('el cajón explica la impresora instalada en Windows y, sin Windows, la cola CUPS', async () => {
    native.invoke.mockImplementation(async () => ({
      status: 200,
      body: { drawers: [], printers: [] },
    }));
    simulatePlatform('Win32', windowsAgent);
    render(<DrawerAdmin execute={vi.fn() as never} />);
    expect(
      await screen.findByText(/una impresora instalada en Windows en el equipo servidor/),
    ).toBeTruthy();
    expect(
      screen.getByText(
        'No hay impresoras compatibles con el cajón: registra una impresora ESC/POS por red o una impresora instalada en Windows que reciba los datos sin convertir («raw») en Cocina y preparación → Impresoras.',
      ),
    ).toBeTruthy();
    expect(screen.queryByText(/CUPS/)).toBeNull();
    cleanup();
    vi.restoreAllMocks();
    render(<DrawerAdmin execute={vi.fn() as never} />);
    expect(
      await screen.findByText(
        'El cajón se abre con un comando ESC/POS (el lenguaje de las impresoras de tickets) que envía la impresora a la que está conectado: una impresora ESC/POS por red o una cola CUPS (una impresora instalada en el equipo servidor, por ejemplo por USB) que acepte datos sin convertir («raw»). AD-EL POS revisa cada cola sin imprimir nada y explica por qué una no sirve. Con el cajón activado se abre tras cobros y devoluciones en efectivo e ingresos, retiros y gastos; nunca con pagos electrónicos. La pantalla indica que el comando se envió, no que el cajón se abrió.',
      ),
    ).toBeTruthy();
    expect(
      screen.getByText(
        'No hay impresoras compatibles con el cajón: registra una impresora ESC/POS por red o una cola CUPS que acepte datos sin convertir («raw») en Cocina y preparación → Impresoras.',
      ),
    ).toBeTruthy();
  });
});
