// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { UserStations } from '../services/kitchen';
import type { User } from '../services/models';
import { chooseOption, optionLabels } from '../testing/select';
import { UserKitchenStations } from './UserKitchenStations';

// Etapa 9F, fase C (§5.2): «Estaciones que puede atender» en Personal › Usuarios. «Todas» por omisión o
// una selección; lo guardado viaja a Go con la selección leída (conflicto ante otra edición) y, con el
// permiso global de estaciones, la pantalla dice que el usuario siempre atiende todas.

const native = vi.hoisted(() => ({ invoke: vi.fn() }));
vi.mock('@tauri-apps/api/core', () => ({ invoke: native.invoke, isTauri: () => true }));

const user: User = {
  id: '00000000-0000-4000-8000-000000000011',
  username: 'cocina',
  display_name: 'Cocinera',
  role_id: 'r1',
  role_name: 'Cocina',
  active: true,
  initial_password: false,
};

let stored: UserStations;
let saves: Record<string, unknown>[];

beforeEach(() => {
  saves = [];
  stored = {
    user_id: user.id,
    all_stations: true,
    station_ids: [],
    all_by_permission: false,
    stations: [
      { id: 's-caliente', name: 'Cocina caliente', active: true },
      { id: 's-barra', name: 'Barra', active: true },
      { id: 's-vieja', name: 'Parrilla', active: false },
    ],
  };
  native.invoke.mockImplementation(
    async (command: string, args: { operation: string; body?: Record<string, unknown> }) => {
      if (command !== 'api_request') {
        return null;
      }
      if (args.operation === 'user_kitchen_stations') {
        return { status: 200, body: stored };
      }
      if (args.operation === 'save_user_kitchen_stations') {
        saves.push(args.body!);
        stored = {
          ...stored,
          all_stations: args.body!.all_stations as boolean,
          station_ids: args.body!.station_ids as string[],
        };
        return { status: 200, body: stored };
      }
      return { status: 200, body: [] };
    },
  );
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe('estaciones que puede atender', () => {
  it('«todas» por omisión; una selección viaja a Go con la selección esperada', async () => {
    render(<UserKitchenStations user={user} editable />);
    const all = (await screen.findByLabelText(
      'Puede atender todas las estaciones',
    )) as HTMLInputElement;
    expect(all.checked).toBe(true);
    expect(screen.queryByText(/siempre atiende todas las estaciones/)).toBeNull();
    fireEvent.click(all);
    const chosen = screen.getByRole('combobox', { name: 'Estaciones que puede atender' });
    expect(optionLabels(chosen)).toEqual(['Cocina caliente', 'Barra', 'Parrilla (inactiva)']);
    chooseOption(chosen, 'Cocina caliente');
    fireEvent.click(screen.getByRole('button', { name: 'Guardar estaciones' }));
    await waitFor(() => expect(saves).toHaveLength(1));
    expect(saves[0]).toEqual({
      all_stations: false,
      station_ids: ['s-caliente'],
      expected_all_stations: true,
      expected_station_ids: [],
    });
  });

  it('con el permiso global dice que siempre atiende todas', async () => {
    stored = { ...stored, all_by_permission: true };
    render(<UserKitchenStations user={user} editable />);
    const statement = await screen.findByRole('status');
    expect(statement.textContent).toContain(
      'Tiene el permiso «Operar todas las estaciones del establecimiento»: siempre atiende todas las estaciones',
    );
  });

  it('sin users.manage solo se consulta', async () => {
    render(<UserKitchenStations user={user} editable={false} />);
    const save = await screen.findByRole('button', { name: 'Guardar estaciones' });
    expect((save as HTMLButtonElement).disabled).toBe(true);
  });
});
