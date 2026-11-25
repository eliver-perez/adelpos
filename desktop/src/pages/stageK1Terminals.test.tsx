// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { Device, DeviceConfiguration } from '../services/models';
import {
  kitchenDisplayCapability,
  rememberServerCapabilities,
} from '../services/serverCapabilities';
import { licenseCapability } from '../services/license';
import {
  DevicesPage,
  installedVersionText,
  kitchenSeatText,
  kitchenSoundText,
  kitchenStationsText,
} from './DevicesPage';
import { displayLimitsProblem } from './kitchen/KitchenSettingsForm';

// Etapa K1 (Fase E, §6): una pantalla de cocina Android en Sistema › Terminales, con sus estaciones (solo las que
// reciben en pantalla), letra, tema, sonido y volumen, la conexión, la versión instalada y el cupo de cocina que ocupa;
// «Liberar cupo» con el texto de K1-D30 y «Revocar». Todo se esconde sin la capacidad kitchen-display-v1.

const native = vi.hoisted(() => ({ invoke: vi.fn() }));
vi.mock('@tauri-apps/api/core', () => ({ invoke: native.invoke, isTauri: () => true }));

type Request = { operation: string; resourceId: string | null; body?: Record<string, unknown> };

const display: Device = {
  id: '00000000-0000-4000-8000-0000000000c1',
  name: 'Tableta de la cocina',
  register_id: null,
  keyboard_mode: 'auto',
  home_screen: 'tables',
  kind: 'kitchen',
  os: 'Android 14',
  app_version: '0.1.2',
  paired_at: '2026-11-24T15:00:00Z',
  online: true,
  current_user: null,
  seats: ['kitchen_displays'],
  kitchen_display: {
    text_size: 'normal',
    theme: 'dark',
    sound_enabled: true,
    sound_volume: 'medium',
    seat_released_at: null,
  },
};
const desk: Device = {
  ...display,
  id: '00000000-0000-4000-8000-0000000000c2',
  name: 'Caja principal',
  kind: 'desktop',
  os: '',
  app_version: '0.16.0',
  seats: [],
  kitchen_display: null,
};
const configuration: DeviceConfiguration = {
  device: display,
  kitchen_all_stations: false,
  kitchen_sound_alert: false,
  kitchen_stations: [
    { id: 's1', name: 'Cocina', active: true },
    { id: 's3', name: 'Postres', active: true },
  ],
  printers: [],
  kitchen_display_station_options: [
    { id: 's1', name: 'Cocina', active: true, reception: 'kds' },
    { id: 's2', name: 'Barra', active: true, reception: 'both' },
  ],
};

let requests: Request[];

beforeEach(() => {
  rememberServerCapabilities([kitchenDisplayCapability, licenseCapability]);
  requests = [];
  native.invoke.mockImplementation(async (command: string, args: Request) => {
    if (command !== 'api_request') {
      return null;
    }
    requests.push(args);
    switch (args.operation) {
      case 'devices':
        return { status: 200, body: [display, desk] };
      case 'devices_pairing_requests':
        return { status: 200, body: { requests: [], approve_here: true, pairing_available: true } };
      case 'devices_sessions_summary':
        return {
          status: 200,
          body: {
            by_kind: { desktop: 1, android: 0, kitchen: 0 },
            total: 1,
            seats: {
              cash_registers: { in_use: 0, limit: 2 },
              order_takers: { in_use: 0, limit: 2 },
              kitchen_displays: { in_use: 1, limit: 2 },
            },
          },
        };
      case 'device_configuration':
        return { status: 200, body: { ...configuration, device: display } };
      case 'update_device':
        return { status: 200, body: display };
    }
    return { status: 200, body: [] };
  });
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  rememberServerCapabilities([]);
});

async function row(name: string) {
  return (await screen.findByText(name)).closest('tr') as HTMLElement;
}

describe('pantalla de cocina en Terminales', () => {
  it('textos de la versión instalada, el cupo, el sonido, las estaciones y los límites', () => {
    expect(installedVersionText(display)).toBe('versión instalada 0.1.2');
    rememberServerCapabilities([]);
    expect(installedVersionText(display)).toBe('versión 0.1.2');
    expect(kitchenSeatText(display)).toBe('Ocupa un cupo de pantalla de cocina');
    expect(
      kitchenSeatText({
        ...display,
        seats: [],
        kitchen_display: { ...display.kitchen_display!, seat_released_at: '2026-11-24T16:00:00Z' },
      }),
    ).toContain('«Usar esta pantalla»');
    expect(kitchenSeatText({ ...display, seats: [] })).toBe('Sin cupo: lo ocupa al conectarse');
    expect(kitchenSoundText(display.kitchen_display!)).toBe('Activado · volumen medio');
    expect(kitchenSoundText({ ...display.kitchen_display!, sound_enabled: false })).toBe(
      'Desactivado',
    );
    // Postres no recibe en pantalla: no cuenta aunque esté elegida.
    expect(kitchenStationsText(configuration)).toBe('Cocina');
    expect(kitchenStationsText({ ...configuration, kitchen_all_stations: true })).toBe(
      'Todas las que reciben comandas en pantalla (Cocina, Barra)',
    );
    expect(displayLimitsProblem(10, 20)).toBe('');
    expect(displayLimitsProblem(10, 10)).not.toBe('');
    expect(displayLimitsProblem(0, 20)).not.toBe('');
  });

  it('la fila y la configuración dicen el tipo, la versión instalada, la conexión, el cupo y los ajustes', async () => {
    render(<DevicesPage can={() => true} />);
    const line = await row('Tableta de la cocina');
    expect(line.textContent).toContain('Pantalla de cocina');
    expect(line.textContent).toContain('Android 14 · versión instalada 0.1.2');
    expect((await row('Caja principal')).textContent).toContain('versión instalada 0.16.0');
    fireEvent.click(within(line).getByRole('button', { name: 'Ver configuración' }));
    const panel = await screen.findByRole('region', {
      name: 'Configuración de Tableta de la cocina',
    });
    await waitFor(() => expect(panel.textContent).toContain('Estaciones que muestra'));
    for (const text of [
      'Ocupa un cupo de pantalla de cocina',
      'Cocina',
      'Normal',
      'Oscuro',
      'Activado · volumen medio',
      'En línea',
    ]) {
      expect(panel.textContent).toContain(text);
    }
    expect(panel.textContent).not.toContain('Pantalla inicial');
    expect(panel.textContent).not.toContain('Caja física');
  });

  it('el editor guarda letra, tema y sonido sin tocar las estaciones si no cambiaron', async () => {
    render(<DevicesPage can={() => true} />);
    fireEvent.click(
      within(await row('Tableta de la cocina')).getByRole('button', { name: 'Editar' }),
    );
    const size = await screen.findByRole('group', { name: 'Tamaño de letra de la pantalla' });
    await waitFor(() =>
      expect(requests.some((request) => request.operation === 'device_configuration')).toBe(true),
    );
    fireEvent.click(within(size).getByRole('button', { name: 'Grande' }));
    fireEvent.click(
      within(screen.getByRole('group', { name: 'Tema de la pantalla' })).getByRole('button', {
        name: 'Claro',
      }),
    );
    // El sonido es un control de dos opciones como la letra y el tema: se ve cuál está elegida, y con «Desactivado»
    // el volumen no se puede cambiar.
    const sound = screen.getByRole('group', { name: 'Sonido de la pantalla' });
    expect(
      within(sound).getByRole('button', { name: 'Activado' }).getAttribute('aria-pressed'),
    ).toBe('true');
    expect(
      within(sound).getByRole('button', { name: 'Desactivado' }).getAttribute('aria-pressed'),
    ).toBe('false');
    fireEvent.click(within(sound).getByRole('button', { name: 'Desactivado' }));
    expect(
      within(sound).getByRole('button', { name: 'Desactivado' }).getAttribute('aria-pressed'),
    ).toBe('true');
    expect(screen.getByRole('combobox', { name: 'Volumen' }).hasAttribute('disabled')).toBe(true);
    await waitFor(() =>
      expect(
        (screen.getByRole('button', { name: 'Guardar cambios' }) as HTMLButtonElement).disabled,
      ).toBe(false),
    );
    fireEvent.click(screen.getByRole('button', { name: 'Guardar cambios' }));
    await waitFor(() =>
      expect(requests.some((request) => request.operation === 'update_device')).toBe(true),
    );
    const update = requests.find((request) => request.operation === 'update_device')!;
    expect(update.body).toMatchObject({
      name: 'Tableta de la cocina',
      kitchen_display_text_size: 'large',
      kitchen_display_theme: 'light',
      kitchen_display_sound: false,
      kitchen_display_volume: 'medium',
    });
    expect(update.body).not.toHaveProperty('home_screen');
    expect(requests.some((request) => request.operation === 'kitchen_save_access')).toBe(false);
  });

  it('«Liberar cupo» dice que no es definitivo y que lo definitivo es revocar', async () => {
    render(<DevicesPage can={() => true} />);
    fireEvent.click(
      within(await row('Tableta de la cocina')).getByRole('button', { name: 'Liberar cupo' }),
    );
    const dialog = await screen.findByRole('alertdialog');
    expect(dialog.textContent).toContain('«Usar esta pantalla»');
    expect(dialog.textContent).toContain('revócala');
  });

  it('«Cupos de la licencia» dice que una tableta de cocina ocupa el cupo mientras está emparejada', async () => {
    render(<DevicesPage can={() => true} />);
    const panel = await screen.findByRole('region', { name: 'Cupos de la licencia' });
    expect(panel.textContent).toContain(
      'una pantalla de cocina: una computadora en Cocina › Preparación, o una tableta de cocina mientras esté emparejada (se libera con «Liberar cupo» o «Revocar»). Las cajas se liberan al cerrar el turno.',
    );
    expect(panel.textContent).not.toContain('pantalla de cocina en Preparación');
  });

  it('sin la capacidad no se ofrece el editor de la pantalla ni la versión instalada', async () => {
    rememberServerCapabilities([licenseCapability]);
    render(<DevicesPage can={() => true} />);
    const line = await row('Tableta de la cocina');
    expect(line.textContent).toContain('versión 0.1.2');
    expect(line.textContent).not.toContain('versión instalada');
  });
});
