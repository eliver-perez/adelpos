// @vitest-environment jsdom

import { readdirSync, readFileSync, statSync } from 'node:fs';
import { join, relative, sep } from 'node:path';
import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { HistoryDetails } from '../components/restaurant/HistoryDetails';
import { PendingOperationsProvider } from '../hooks/pendingOperations';
import type { CancelReason, HistoryEvent } from '../services/restaurant';
import { eventLabels } from '../services/restaurant';
import { KitchenPage } from './kitchen/KitchenPage';
import { chooseOption } from '../testing/select';
import { CancelReasonsAdmin } from './payments/CancelReasonsAdmin';
import { AccountList } from './restaurant/AccountList';
import { AccountCancellationRecord, kitchenEffect } from './restaurant/CancelAccount';
import { FeaturesSection } from './SettingsPage';

const native = vi.hoisted(() => ({ invoke: vi.fn() }));
vi.mock('@tauri-apps/api/core', () => ({ invoke: native.invoke, isTauri: () => true }));

// Etapa 9D, fase E: catálogo de motivos de cancelación en Cobros, umbral de cuenta abierta en
// «Funciones del establecimiento», registro de la cuenta cancelada, historial, Cocina y los nombres
// «Cancelar cuenta» / «Cuentas canceladas» en lugar de «Anular».

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

function reason(id: string, name: string, order: number, extra: Partial<CancelReason> = {}) {
  return {
    id: `00000000-0000-4000-8000-0000000004${id}`,
    name,
    requires_description: false,
    prepared_is_waste: false,
    active: true,
    sort_order: order,
    revision: 1,
    updated_at: '2026-11-12T10:00:00Z',
    ...extra,
  };
}

const reasons: CancelReason[] = [
  reason('04', 'Otro', 40, { requires_description: true }),
  reason('01', 'Cliente se retiró sin pagar', 10, { prepared_is_waste: true }),
  reason('03', 'Error de captura', 30, { active: false }),
  reason('02', 'Cliente canceló el pedido', 20),
];

describe('catálogo de motivos de cancelación (Cobros)', () => {
  it('lista los motivos en su orden con sus marcas y crea uno con su posición', async () => {
    const execute = vi.fn(async () => ({}) as never);
    render(
      <CancelReasonsAdmin
        reasons={reasons}
        can={(permission) => permission === 'payments.settings.manage'}
        execute={execute}
        busy={false}
      />,
    );
    const group = screen.getByRole('group', { name: 'Motivos de cancelación' });
    expect(
      within(group)
        .getAllByRole('button')
        .map((button) => button.textContent),
    ).toEqual([
      'Cliente se retiró sin pagar · cuenta como merma',
      'Cliente canceló el pedido',
      'Error de captura · inactivo',
      'Otro · exige descripción',
      '+ Nuevo motivo',
    ]);
    fireEvent.click(within(group).getByRole('button', { name: '+ Nuevo motivo' }));
    fireEvent.change(screen.getByLabelText(/Nombre del motivo/), {
      target: { value: 'Cuenta duplicada' },
    });
    fireEvent.click(screen.getByRole('button', { name: 'Lo enviado a cocina cuenta como merma' }));
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'Crear motivo' }));
    });
    // Sin operation_id aquí: lo agrega la mutación de la página (useRestaurantMutation).
    expect(execute).toHaveBeenCalledWith('restaurant_create_cancel_reason', {
      body: {
        name: 'Cuenta duplicada',
        requires_description: false,
        prepared_is_waste: true,
        active: true,
        position: { after: reasons[0]!.id },
      },
    });
  });

  it('renombra, desactiva, cambia las marcas y reordena con la revisión del motivo', async () => {
    const execute = vi.fn(async () => ({}) as never);
    render(
      <CancelReasonsAdmin reasons={reasons} can={() => true} execute={execute} busy={false} />,
    );
    fireEvent.click(screen.getByRole('button', { name: 'Cliente canceló el pedido' }));
    const name = screen.getByLabelText(/Nombre del motivo/) as HTMLInputElement;
    expect(name.value).toBe('Cliente canceló el pedido');
    fireEvent.change(name, { target: { value: 'El cliente ya no lo quiso' } });
    fireEvent.click(screen.getByRole('button', { name: 'Exige descripción' }));
    fireEvent.click(screen.getByRole('button', { name: 'Motivo activo' }));
    expect(
      screen.getByRole('button', { name: 'Exige descripción' }).getAttribute('aria-pressed'),
    ).toBe('true');
    expect(screen.getByText(/las cuentas ya canceladas lo conservan/)).toBeTruthy();
    // Orden por posiciones (etapa 6.1): «Al inicio» envía after vacío.
    chooseOption(screen.getByRole('combobox', { name: /Posición del motivo/ }), 'Al inicio');
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'Guardar motivo' }));
    });
    expect(execute).toHaveBeenCalledWith('restaurant_save_cancel_reason', {
      id: reasons[3]!.id,
      body: {
        name: 'El cliente ya no lo quiso',
        requires_description: true,
        prepared_is_waste: false,
        active: false,
        position: { after: '' },
        revision: 1,
      },
    });
  });

  it('sin «Configurar políticas de cobro» solo se consultan', () => {
    render(
      <CancelReasonsAdmin reasons={reasons} can={() => false} execute={vi.fn()} busy={false} />,
    );
    expect(screen.getByText(/Modificar los motivos requiere el permiso/)).toBeTruthy();
    expect(screen.queryByRole('button', { name: '+ Nuevo motivo' })).toBeNull();
    expect((screen.getByRole('button', { name: /^Otro/ }) as HTMLButtonElement).disabled).toBe(
      true,
    );
  });
});

describe('umbral de cuenta abierta (Funciones del establecimiento)', () => {
  let threshold: {
    long_open_hours: number;
    minimum_hours: number;
    maximum_hours: number;
    revision: number;
  };
  let saveReply: { status: number; body: unknown } | null;
  let saves: Record<string, unknown>[];

  beforeEach(() => {
    threshold = { long_open_hours: 12, minimum_hours: 1, maximum_hours: 72, revision: 3 };
    saveReply = null;
    saves = [];
    native.invoke.mockImplementation(
      async (_command: string, args: { operation: string; body?: Record<string, unknown> }) => {
        if (args.operation === 'settings_features') {
          return { status: 200, body: [] };
        }
        if (args.operation === 'settings_long_open_hours') {
          return { status: 200, body: threshold };
        }
        if (args.operation === 'save_settings_long_open_hours') {
          saves.push(args.body!);
          if (saveReply) {
            return saveReply;
          }
          threshold = {
            ...threshold,
            long_open_hours: args.body!.long_open_hours as number,
            revision: threshold.revision + 1,
          };
          return { status: 200, body: threshold };
        }
        return { status: 200, body: {} };
      },
    );
  });

  it('campo de horas con la entrada numérica de la 7.1, su explicación y guardado con revisión', async () => {
    const changes = vi.fn();
    window.addEventListener('adel-server-change', changes);
    render(<FeaturesSection can={() => true} />);
    const section = await screen.findByRole('region', { name: 'Funciones del establecimiento' });
    const field = (await within(section).findByRole('textbox', {
      name: /Marcar una cuenta abierta después de \(horas\)/,
    })) as HTMLInputElement;
    // Sin type="number": texto con teclado numérico.
    expect(field.type).toBe('text');
    expect(field.getAttribute('inputmode')).toBe('numeric');
    expect(field.value).toBe('12');
    expect(within(section).getByText(/más de 12 horas se marcan en ámbar en Mesas/)).toBeTruthy();
    fireEvent.change(field, { target: { value: '80' } });
    expect(field.getAttribute('aria-invalid')).toBe('true');
    expect(within(section).getByText('Valor permitido: 1 a 72.')).toBeTruthy();
    fireEvent.change(field, { target: { value: '6' } });
    expect(within(section).getByText(/más de 6 horas se marcan en ámbar/)).toBeTruthy();
    fireEvent.click(within(section).getByRole('button', { name: 'Guardar umbral' }));
    await waitFor(() => expect(saves).toHaveLength(1));
    // Sin operation_id: configuración con revisión optimista.
    expect(saves[0]).toEqual({ long_open_hours: 6, revision: 3 });
    const saved = await screen.findByRole('status', { name: 'Operación completada' });
    expect(saved.textContent).toContain('Umbral de cuenta abierta guardado.');
    // Las pantallas abiertas vuelven a consultar el mapa y las cuentas.
    expect(changes).toHaveBeenCalled();
    window.removeEventListener('adel-server-change', changes);
    await waitFor(() =>
      expect(
        (
          within(section).getByRole('textbox', {
            name: /Marcar una cuenta abierta/,
          }) as HTMLInputElement
        ).value,
      ).toBe('6'),
    );
  });

  it('sin permiso solo se consulta y un conflicto de revisión carga el valor vigente', async () => {
    const { unmount } = render(<FeaturesSection can={() => false} />);
    expect(
      (await screen.findByRole('button', { name: 'Guardar umbral' })) as HTMLButtonElement,
    ).toHaveProperty('disabled', true);
    unmount();
    saveReply = {
      status: 409,
      body: {
        error: { code: 'REVISION_CONFLICT', message: 'El ajuste cambió en otra terminal.' },
      },
    };
    render(<FeaturesSection can={() => true} />);
    const field = (await screen.findByRole('textbox', {
      name: /Marcar una cuenta abierta/,
    })) as HTMLInputElement;
    threshold = { ...threshold, long_open_hours: 24, revision: 4 };
    fireEvent.change(field, { target: { value: '8' } });
    fireEvent.click(screen.getByRole('button', { name: 'Guardar umbral' }));
    const conflict = await screen.findByRole('alert', { name: 'Error' });
    expect(conflict.textContent).toContain('El ajuste cambió en otra terminal.');
    await waitFor(() =>
      expect(
        (screen.getByRole('textbox', { name: /Marcar una cuenta abierta/ }) as HTMLInputElement)
          .value,
      ).toBe('24'),
    );
  });
});

describe('cuenta cancelada, historial y Cocina', () => {
  it('el detalle de una cuenta cancelada muestra su registro con «Autorizó»', () => {
    render(
      <AccountCancellationRecord
        timezone="America/Monterrey"
        cancellation={{
          reason_id: 'motivo',
          reason_name: 'Cliente se retiró sin pagar',
          description: 'Mesa de la ventana',
          prepared_is_waste: true,
          cancelled_by_name: 'Laura',
          authorized_by_name: 'Gerente',
          cancelled_units: 3,
          cancelled_amount: '1345.60',
          created_at: '2026-11-12T16:00:00Z',
        }}
      />,
    );
    const record = screen.getByRole('region', { name: 'Cuenta cancelada' });
    const value = (label: string) => within(record).getByText(label).nextSibling?.textContent;
    expect(value('Motivo')).toBe('Cliente se retiró sin pagar');
    expect(value('Descripción')).toBe('Mesa de la ventana');
    expect(value('Canceló')).toBe('Laura');
    expect(value('Autorizó')).toBe('Gerente');
    expect(value('Unidades canceladas')).toBe('3');
    expect(value('Importe cancelado')).toBe('$1,345.60');
    expect(record.textContent).toContain('Lo preparado cuenta como merma.');
    expect(record.textContent).toContain('No cuenta como venta.');
  });

  it('sin supervisor «Autorizó» dice que bastó el permiso propio; sin registro, que salió vacía', () => {
    const { unmount } = render(
      <AccountCancellationRecord
        timezone="America/Monterrey"
        cancellation={{
          reason_id: 'motivo',
          reason_name: 'Error de captura',
          description: null,
          prepared_is_waste: false,
          cancelled_by_name: 'Administrador',
          authorized_by_name: null,
          cancelled_units: 0,
          cancelled_amount: '0.00',
          created_at: '2026-11-12T16:00:00Z',
        }}
      />,
    );
    expect(screen.getByText('Autorizó').nextSibling?.textContent).toBe('Con su propio permiso');
    expect(screen.queryByText('Descripción')).toBeNull();
    unmount();
    render(<AccountCancellationRecord timezone="America/Monterrey" cancellation={null} />);
    expect(
      screen.getByText(/Cuenta cancelada sin productos activos o descartada vacía/),
    ).toBeTruthy();
  });

  it('historial: «Cuenta cancelada» con motivo, descripción, quién canceló y quién autorizó', () => {
    expect(eventLabels['account.cancelled']).toBe('Cuenta cancelada');
    expect(eventLabels['account.discarded']).toBe('Cuenta vacía descartada');
    const event: HistoryEvent = {
      id: 'evento',
      sequence: 9,
      account_id: 'cuenta',
      related_account_id: null,
      kind: 'account.cancelled',
      payload: {
        before: { account: { name: 'Mesa 1' }, lines: [], draft: [] },
        after: {
          account: { name: 'Mesa 1', table_name: 'Mesa 1' },
          reason_id: 'motivo',
          reason: 'Otro',
          description: 'Se equivocaron de mesa',
          prepared_is_waste: false,
          unit_ids: ['u1', 'u2'],
          cancelled_units: 2,
          cancelled_amount: '230.00',
          authorized_by: 'supervisor',
          authorized_by_username: 'gerente',
          authorized_by_name: 'Gerente General',
          discarded_draft_items: 1,
        },
      },
      author_name: 'Laura',
      device_name: 'Caja 1',
      created_at: '2026-11-12T16:00:00Z',
      integration_state: 'pending_integration',
    };
    const { container, unmount } = render(<HistoryDetails event={event} />);
    const text = container.textContent ?? '';
    expect(text).toContain('Motivo: Otro');
    expect(text).toContain('Descripción: Se equivocaron de mesa');
    expect(text).toContain('Unidades canceladas: 2');
    expect(text).toContain('Importe cancelado: $230.00');
    expect(text).toContain('Canceló: Laura');
    // El nombre visible del supervisor, como el detalle y el reporte; nunca su usuario.
    expect(text).toContain('Autorizó: Gerente General');
    expect(text).not.toContain('gerente');
    expect(text).toContain('Productos sin enviar descartados: 1');
    expect(text).toContain('no cuenta como venta');
    // Los identificadores de las unidades canceladas no se presentan como «conservadas».
    expect(text).not.toContain('Unidades conservadas');
    unmount();
    const own = render(
      <HistoryDetails
        event={{
          ...event,
          payload: {
            before: event.payload.before,
            after: {
              ...(event.payload.after as Record<string, unknown>),
              authorized_by: null,
              authorized_by_username: null,
              authorized_by_name: null,
            },
          },
        }}
      />,
    );
    expect(own.container.textContent).toContain('Autorizó: con su propio permiso');
    own.unmount();
    // Un evento anterior a la corrección (sin el nombre) muestra el usuario.
    const previous = render(
      <HistoryDetails
        event={{
          ...event,
          payload: {
            before: event.payload.before,
            after: {
              ...(event.payload.after as Record<string, unknown>),
              authorized_by_name: undefined,
            },
          },
        }}
      />,
    );
    expect(previous.container.textContent).toContain('Autorizó: gerente');
    previous.unmount();
    const discarded = render(
      <HistoryDetails
        event={{
          ...event,
          kind: 'account.discarded',
          payload: {
            before: { account: { name: 'Mesa 1' }, draft: [{ id: 'd1' }, { id: 'd2' }] },
            after: { account: { name: 'Mesa 1' } },
          },
        }}
      />,
    );
    expect(discarded.container.textContent).toContain('Productos sin enviar descartados: 2');
    expect(discarded.container.textContent).toContain('no es una cancelación');
  });

  it('el efecto en cocina depende de la marca de merma del motivo y del control de inventario', () => {
    expect(kitchenEffect(0, null, true)).toEqual([
      'La cuenta no tiene productos enviados a cocina.',
    ]);
    expect(kitchenEffect(2, { prepared_is_waste: true }, true)[1]).toMatch(
      /lo preparado cuenta como merma: en Inventarios queda como «Preparado \/ merma»/,
    );
    expect(kitchenEffect(2, { prepared_is_waste: true }, false)[1]).toBe(
      'Con este motivo, lo preparado cuenta como merma.',
    );
    expect(kitchenEffect(2, { prepared_is_waste: false }, true)[1]).toMatch(/queda por clasificar/);
  });

  it('Cocina llama «cancelación» al aviso pendiente de una cuenta cancelada', async () => {
    native.invoke.mockImplementation(async (command: string, args: { operation: string }) => {
      if (command !== 'api_request') {
        return null;
      }
      if (args.operation === 'kitchen_board') {
        return {
          status: 200,
          body: {
            tasks: [],
            releases: [],
            pending_changes: [
              {
                event_id: 'e1',
                kind: 'account.cancelled',
                warnings: [],
                revision: 1,
                created_at: '2026-11-12T16:00:00Z',
              },
              {
                event_id: 'e2',
                kind: 'item.modified',
                warnings: [],
                revision: 1,
                created_at: '2026-11-12T16:00:00Z',
              },
            ],
            stations: [],
            server_time: '2026-11-12T16:00:00Z',
            timezone: 'America/Monterrey',
            page: 1,
          },
        };
      }
      return { status: 200, body: [] };
    });
    render(
      <PendingOperationsProvider>
        <KitchenPage can={() => true} timezone="America/Monterrey" />
      </PendingOperationsProvider>,
    );
    expect(await screen.findByText('Aviso de cancelación pendiente')).toBeTruthy();
    expect(screen.getByText('Aviso de modificación pendiente')).toBeTruthy();
  });
});

describe('«Cancelar cuenta» y «Cuentas canceladas» en lugar de «Anular»', () => {
  it('el filtro de la lista de cuentas dice «Canceladas o descartadas»', async () => {
    render(
      <AccountList
        value={null}
        selectedID=""
        onSelect={() => undefined}
        search=""
        onSearch={() => undefined}
        state="voided"
        onStateChange={() => undefined}
        page={1}
        onPageChange={() => undefined}
        error=""
      />,
    );
    expect(screen.getByRole('combobox', { name: 'Estado de cuenta' }).textContent).toContain(
      'Canceladas o descartadas',
    );
  });

  it('ningún texto visible de la interfaz habla de anular cuentas', () => {
    const root = join(__dirname, '..');
    const found: string[] = [];
    const walk = (directory: string): string[] =>
      readdirSync(directory).flatMap((entry) => {
        const path = join(directory, entry);
        if (statSync(path).isDirectory()) {
          return walk(path);
        }
        return /\.(ts|tsx)$/.test(entry) && !entry.includes('.test.') ? [path] : [];
      });
    for (const path of walk(root)) {
      const file = relative(root, path).split(sep).join('/');
      readFileSync(path, 'utf8')
        .split('\n')
        .forEach((line, index) => {
          // Los comentarios pueden recordar el nombre anterior; los textos visibles no.
          if (line.trim().startsWith('//') || line.trim().startsWith('*')) {
            return;
          }
          if (/Anular cuenta|[Aa]nulad[ao]s?\b|Cuentas anuladas|Se anulará/.test(line)) {
            found.push(`${file}:${index + 1} ${line.trim()}`);
          }
        });
    }
    expect(found).toEqual([]);
  });
});
