// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { DeviceScope, Station } from '../../services/kitchen';
import { chooseOption, optionLabels } from '../../testing/select';
import { StationsAdmin } from './StationsAdmin';
import { TerminalStations } from './TerminalStations';

// Etapa 9F, fase C (§5.1): «Terminales de cocina» en Cocina › Estaciones (desde la K1.1, K11-D7, «Computadoras en
// modo Cocina», con la línea que dice dónde se configuran las tabletas). Por terminal, «Mostrar todas
// las estaciones» (predeterminada) o una selección con el selector múltiple, y el aviso sonoro; lo
// guardado viaja a Go con la configuración que se leyó para rechazar una edición concurrente.

const native = vi.hoisted(() => ({ invoke: vi.fn() }));
vi.mock('@tauri-apps/api/core', () => ({ invoke: native.invoke, isTauri: () => true }));

function station(id: string, name: string, active = true): Station {
  return {
    id,
    name,
    description: '',
    active,
    sort_order: 0,
    reception: 'kds',
    printer_id: null,
    target_minutes: 15,
    warning_minutes: 15,
    overdue_minutes: 25,
    grouping: 'inherit',
    tracking: 'inherit',
    revision: 1,
  };
}

const stations = [station('s-barra', 'Barra'), station('s-postres', 'Postres', false)];
let devices: DeviceScope[];

beforeEach(() => {
  devices = [
    {
      device_id: 'd-caja',
      device_name: 'Caja principal',
      home_screen: 'tables',
      all_stations: true,
      station_ids: [],
      sound_alert: false,
    },
    {
      device_id: 'd-cocina',
      device_name: 'Pantalla de cocina',
      home_screen: 'kitchen',
      all_stations: false,
      station_ids: ['s-barra'],
      sound_alert: true,
    },
  ];
  native.invoke.mockImplementation(async (command: string, args: { operation: string }) => {
    if (command === 'api_request' && args.operation === 'kitchen_access') {
      return { status: 200, body: devices };
    }
    return { status: 200, body: [] };
  });
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

async function chooseTerminal(name: string) {
  fireEvent.click(screen.getByRole('button', { name: /Computadoras en modo Cocina/ }));
  const terminal = screen.getByRole('combobox', { name: 'Terminal' });
  await waitFor(() => expect(optionLabels(terminal)).toContain(name));
  chooseOption(terminal, name);
}

describe('terminales de cocina', () => {
  it('una terminal muestra todas por omisión; elegir estaciones y el aviso sonoro viaja a Go', async () => {
    const execute = vi.fn(
      async (_operation: string, options: { body?: Record<string, unknown> }) => {
        const body = options.body!;
        return {
          device_id: body.device_id,
          all_stations: body.all_stations,
          station_ids: body.station_ids,
          sound_alert: body.sound_alert,
        };
      },
    );
    render(<TerminalStations stations={stations} execute={execute as never} busy={false} />);
    await chooseTerminal('Caja principal');
    expect(
      screen.getByText('Las pantallas de cocina (tabletas) se configuran en Sistema › Terminales.'),
    ).toBeTruthy();
    expect(
      screen.getByText(
        'Esta terminal abre en Mesas. Para que abra directo en Cocina › Preparación, elige «Cocina» como pantalla inicial en Sistema › Terminales.',
      ),
    ).toBeTruthy();
    const all = screen.getByLabelText('Mostrar todas las estaciones') as HTMLInputElement;
    expect(all.checked).toBe(true);
    expect(
      screen.queryByRole('combobox', { name: 'Estaciones que muestra esta terminal' }),
    ).toBeNull();
    fireEvent.click(all);
    const chosen = screen.getByRole('combobox', { name: 'Estaciones que muestra esta terminal' });
    expect(optionLabels(chosen)).toEqual(['Barra', 'Postres (inactiva)']);
    chooseOption(chosen, 'Barra');
    fireEvent.click(screen.getByLabelText('Aviso sonoro al llegar una comanda nueva'));
    fireEvent.click(screen.getByRole('button', { name: 'Guardar terminal de cocina' }));
    await waitFor(() => expect(execute).toHaveBeenCalledTimes(1));
    const [operation, options] = execute.mock.calls[0]!;
    expect(operation).toBe('kitchen_save_access');
    expect(options.body).toEqual({
      device_id: 'd-caja',
      all_stations: false,
      station_ids: ['s-barra'],
      sound_alert: true,
      expected_all_stations: true,
      expected_station_ids: [],
    });
  });

  it('volver a «todas» envía la lista vacía y la selección esperada de la terminal', async () => {
    const execute = vi.fn(async () => ({
      device_id: 'd-cocina',
      all_stations: true,
      station_ids: [],
      sound_alert: true,
    }));
    render(<TerminalStations stations={stations} execute={execute as never} busy={false} />);
    await chooseTerminal('Pantalla de cocina');
    expect(screen.getByText('Esta terminal abre en Cocina › Preparación.')).toBeTruthy();
    expect(
      (screen.getByLabelText('Aviso sonoro al llegar una comanda nueva') as HTMLInputElement)
        .checked,
    ).toBe(true);
    fireEvent.click(screen.getByLabelText('Mostrar todas las estaciones'));
    fireEvent.click(screen.getByRole('button', { name: 'Guardar terminal de cocina' }));
    await waitFor(() => expect(execute).toHaveBeenCalledTimes(1));
    expect((execute.mock.calls[0] as unknown[])[1]).toEqual({
      body: {
        device_id: 'd-cocina',
        all_stations: true,
        station_ids: [],
        sound_alert: true,
        expected_all_stations: false,
        expected_station_ids: ['s-barra'],
      },
    });
  });
});

// Etapa K1.1 (§4.4, K11-D7): la recepción en pantalla se explica en lenguaje sencillo.
describe('recepción en la pantalla de cocina', () => {
  it('la estación que recibe en pantalla lo explica sin hablar de confirmaciones', async () => {
    native.invoke.mockImplementation(async (command: string, args: { operation: string }) => {
      if (command === 'api_request' && args.operation === 'kitchen_stations') {
        return { status: 200, body: stations };
      }
      return { status: 200, body: [] };
    });
    render(<StationsAdmin can={() => true} execute={vi.fn() as never} busy={false} />);
    fireEvent.click(await screen.findByRole('button', { name: /^Barra/ }));
    expect(
      await screen.findByText(
        'Las pantallas de cocina muestran estos pedidos al momento. En Comandas se ve si la pantalla ya los recibió.',
      ),
    ).toBeTruthy();
    expect(document.body.textContent).not.toContain('queda pendiente hasta que');
    expect(screen.getByRole('button', { name: /Computadoras en modo Cocina/ })).toBeTruthy();
  });
});
