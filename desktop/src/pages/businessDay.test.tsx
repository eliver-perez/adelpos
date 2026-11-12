// @vitest-environment jsdom

// Etapa 9A: «Hora de inicio del día» en Configuración general (una sola configuración, editable
// solo con settings.manage) y servicios de reportes sin aritmética en React.

import { act, cleanup, fireEvent, render, screen, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { BusinessDaySection } from './SettingsPage';
import { fetchFilterOptions, runReport } from '../services/reports';
import { chooseOption, optionLabels, selectText } from '../testing/select';
import type { BusinessDaySettings } from '../services/reports';

const native = vi.hoisted(() => ({ invoke: vi.fn() }));
vi.mock('@tauri-apps/api/core', () => ({ invoke: native.invoke, isTauri: () => true }));

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

function requests(operation: string) {
  return native.invoke.mock.calls
    .filter(([command, args]) => command === 'api_request' && args?.operation === operation)
    .map(
      ([, args]) => args as { body: Record<string, unknown>; query: unknown; resourceId: unknown },
    );
}

describe('hora de inicio del día', () => {
  let state: BusinessDaySettings;

  beforeEach(() => {
    state = {
      operating_day_cutoff: '05:00',
      timezone: 'America/Monterrey',
      revision: 3,
      updated_at: '2026-11-10T12:00:00Z',
      updated_by_name: null,
    };
    native.invoke.mockImplementation(
      async (_command: string, args: { operation: string; body?: unknown }) => {
        if (args.operation === 'settings_business_day') {
          return { status: 200, body: state };
        }
        if (args.operation === 'save_settings_business_day') {
          const input = args.body as { operating_day_cutoff: string; revision: number };
          state = { ...state, operating_day_cutoff: input.operating_day_cutoff, revision: 4 };
          return { status: 200, body: state };
        }
        return { status: 404, body: { error: { code: 'NOT_FOUND', message: 'No existe.' } } };
      },
    );
  });

  it('explica la configuración con un ejemplo y guarda la hora con su revisión', async () => {
    render(<BusinessDaySection can={() => true} />);
    // Etapa 9C (§2.1): hora y minutos con el selector, sin escritura libre.
    const field = await screen.findByRole('group', { name: 'Hora de inicio del día' });
    const hour = within(field).getByRole('combobox', { name: 'Hora' });
    const minutes = within(field).getByRole('combobox', { name: 'Minutos' });
    expect(selectText(hour)).toBe('05');
    expect(selectText(minutes)).toBe('00');
    expect(field.querySelector('input:not([aria-hidden="true"])')).toBeNull();
    // Go acepta de 00:00 a 23:59: no se ofrece «24».
    expect(optionLabels(hour)).not.toContain('24');
    expect(
      screen.getByText(/con 05:00 una venta de la 01:30 pertenece al día anterior/),
    ).toBeTruthy();
    expect(screen.getByText(/Cambiarla no modifica los registros/)).toBeTruthy();
    chooseOption(hour, '00');
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'Guardar hora de inicio' }));
    });
    expect(requests('save_settings_business_day')[0]?.body).toEqual({
      operating_day_cutoff: '00:00',
      revision: 3,
    });
  });

  it('sin settings.manage se muestra pero no se puede guardar', async () => {
    render(<BusinessDaySection can={(key) => key === 'settings.read'} />);
    await screen.findByRole('group', { name: 'Hora de inicio del día' });
    const button = screen.getByRole('button', {
      name: 'Guardar hora de inicio',
    }) as HTMLButtonElement;
    expect(button.disabled).toBe(true);
  });
});

describe('servicios de reportes', () => {
  it('envían el reporte como identificador y sus parámetros por la lista blanca', async () => {
    native.invoke.mockResolvedValue({ status: 200, body: { rows: [], options: [] } });
    await runReport('sales.by_day', { period: 'iso_week', week: '2026-W53' });
    await fetchFilterOptions('products', 'hamb');
    await fetchFilterOptions('registers');
    const [run] = requests('reports_run');
    expect(run?.resourceId).toBe('sales.by_day');
    expect(run?.query).toEqual({ period: 'iso_week', week: '2026-W53' });
    const filters = requests('reports_filters');
    expect(filters[0]?.query).toEqual({ search: 'hamb' });
    expect(filters[1]?.query).toEqual({});
    expect(filters[1]?.resourceId).toBe('registers');
  });
});
