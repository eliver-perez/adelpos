// @vitest-environment jsdom

// Etapa 6.1 · fase C: funciones del establecimiento en Configuración y tasa de IVA del producto.

import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { useState } from 'react';
import { FeaturesSection } from './SettingsPage';
import { ProductEditor } from '../components/catalog/ProductEditor';
import { newCatalogDocument } from './catalog/catalogState';
import type { Lookups, Product } from '../services/catalog';
import type { EstablishmentFeature } from '../services/features';

const native = vi.hoisted(() => ({ invoke: vi.fn() }));
vi.mock('@tauri-apps/api/core', () => ({ invoke: native.invoke, isTauri: () => true }));

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

function usdFeature(enabled: boolean, revision: number): EstablishmentFeature {
  return {
    code: 'usd_cash',
    name: 'Pago en dólares en efectivo',
    description:
      'Permite recibir dólares estadounidenses en efectivo con un tipo de cambio fijo por turno.',
    enabled,
    revision,
    updated_at: null,
    updated_by_name: enabled ? 'Administrador' : null,
  };
}

function requests(operation: string) {
  return native.invoke.mock.calls
    .filter(([command, args]) => command === 'api_request' && args?.operation === operation)
    .map(([, args]) => args as { body: Record<string, unknown> });
}

describe('funciones del establecimiento', () => {
  let state: EstablishmentFeature;
  let saveReply: { status: number; body: unknown } | null;

  beforeEach(() => {
    state = usdFeature(false, 1);
    saveReply = null;
    native.invoke.mockImplementation(
      async (_command: string, args: { operation: string; body?: unknown }) => {
        if (args.operation === 'settings_features') {
          return { status: 200, body: [state] };
        }
        if (args.operation === 'save_settings_feature') {
          if (saveReply) {
            return saveReply;
          }
          const input = args.body as { enabled: boolean; revision: number };
          state = usdFeature(input.enabled, input.revision + 1);
          return { status: 200, body: state };
        }
        return { status: 200, body: {} };
      },
    );
  });

  it('muestra un interruptor por función con su explicación y guarda con la revisión vigente', async () => {
    render(<FeaturesSection can={() => true} />);
    const section = await screen.findByRole('region', { name: 'Funciones del establecimiento' });
    const toggle = await within(section).findByRole('button', {
      name: 'Pago en dólares en efectivo',
    });
    expect(toggle.getAttribute('aria-pressed')).toBe('false');
    expect(toggle.getAttribute('aria-describedby')).toBe('feature-usd_cash-description');
    expect(within(section).getByText(/tipo de cambio fijo por turno/)).toBeTruthy();
    expect(section.querySelectorAll('input[type="checkbox"]')).toHaveLength(0);
    fireEvent.click(toggle);
    await waitFor(() => expect(requests('save_settings_feature')).toHaveLength(1));
    // Es configuración con revisión optimista, no una operación con operation_id.
    expect(requests('save_settings_feature')[0]?.body).toEqual({
      code: 'usd_cash',
      enabled: true,
      revision: 1,
    });
    // La confirmación es un aviso del sistema único de mensajes, fuera de la sección.
    const saved = await screen.findByRole('status', { name: 'Operación completada' });
    expect(saved.textContent).toContain('Pago en dólares en efectivo: activada.');
    await waitFor(() =>
      expect(
        within(section)
          .getByRole('button', { name: 'Pago en dólares en efectivo' })
          .getAttribute('aria-pressed'),
      ).toBe('true'),
    );
  });

  it('sin permiso solo consulta y un conflicto de revisión recarga el estado', async () => {
    const { unmount } = render(<FeaturesSection can={() => false} />);
    const toggle = await screen.findByRole('button', { name: 'Pago en dólares en efectivo' });
    expect((toggle as HTMLButtonElement).disabled).toBe(true);
    expect(screen.getByText(/requiere el permiso «Activar o desactivar funciones/)).toBeTruthy();
    unmount();
    saveReply = {
      status: 409,
      body: {
        error: { code: 'REVISION_CONFLICT', message: 'La función cambió en otra terminal.' },
      },
    };
    render(<FeaturesSection can={() => true} />);
    fireEvent.click(await screen.findByRole('button', { name: 'Pago en dólares en efectivo' }));
    // El conflicto es un error que permanece hasta que se cierra, sin el código interno.
    const conflict = await screen.findByRole('alert', { name: 'Error' });
    expect(conflict.textContent).toMatch(/cambió en otra terminal/);
    expect(conflict.textContent).not.toContain('REVISION_CONFLICT');
    expect(requests('settings_features').length).toBeGreaterThanOrEqual(3);
  });
});

const lookups: Lookups = {
  categories: [],
  ingredients: [],
  complements: [],
  portions: [],
  groups: [],
  'price-lists': [],
  products: [],
  palette: [],
};

function Editor({ initial, can }: { initial: Product; can: (permission: string) => boolean }) {
  const [value, setValue] = useState(initial);
  return (
    <>
      <ProductEditor value={value} onChange={setValue} lookups={lookups} can={can} />
      <output data-testid="tax-rate">{JSON.stringify(value.tax_rate ?? null)}</output>
    </>
  );
}

describe('tasa de IVA del producto', () => {
  it('ofrece la tasa general y 16, 8 y 0 % como botones, sin captura libre', () => {
    const product = newCatalogDocument('products', false, '') as Product;
    render(<Editor initial={product} can={() => true} />);
    const group = screen.getByRole('group', { name: 'Tasa de IVA del producto' });
    const options = within(group)
      .getAllByRole('button')
      .map((button) => button.textContent);
    expect(options).toEqual(['Tasa general del establecimiento', '16 %', '8 %', '0 %']);
    expect(
      within(group)
        .getByRole('button', { name: 'Tasa general del establecimiento' })
        .getAttribute('aria-pressed'),
    ).toBe('true');
    expect(screen.getByTestId('tax-rate').textContent).toBe('null');
    fireEvent.click(within(group).getByRole('button', { name: '8 %' }));
    expect(screen.getByTestId('tax-rate').textContent).toBe('8');
    fireEvent.click(within(group).getByRole('button', { name: '0 %' }));
    expect(screen.getByTestId('tax-rate').textContent).toBe('0');
    fireEvent.click(
      within(group).getByRole('button', { name: 'Tasa general del establecimiento' }),
    );
    expect(screen.getByTestId('tax-rate').textContent).toBe('null');
    expect(group.querySelectorAll('input')).toHaveLength(0);
  });

  it('cambiar la tasa exige el permiso de precios, como el precio', () => {
    const product = {
      ...(newCatalogDocument('products', false, '') as Product),
      tax_rate: 16 as const,
    };
    render(
      <Editor initial={product} can={(permission) => permission !== 'catalog.prices.manage'} />,
    );
    const group = screen.getByRole('group', { name: 'Tasa de IVA del producto' });
    expect(within(group).getByRole('button', { name: '16 %' }).getAttribute('aria-pressed')).toBe(
      'true',
    );
    for (const button of within(group).getAllByRole('button')) {
      expect((button as HTMLButtonElement).disabled).toBe(true);
    }
    expect(
      screen.getByText(/requiere el permiso «Modificar precios y valores comerciales»/),
    ).toBeTruthy();
  });
});
