// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { actionByID, commandEntries } from '../commands/actions';
import type { CommandContext, CommandRuntime } from '../commands/actions';
import { breadcrumbOf, canOpen } from '../navigation/sections';
import {
  failingChecks,
  monthLabel,
  passphraseProblem,
  preparationError,
  summarySentence,
} from '../services/legacyMigration';
import type {
  LegacyBatch,
  LegacyPreview,
  LegacyReport,
  LegacyRun,
  LegacyStatus,
  LegacyValidation,
} from '../services/legacyMigration';
import type { User } from '../services/models';
import { LegacyMigrationPage } from './maintenance/LegacyMigrationPage';
import { reducePreparation } from './maintenance/LegacyPrepareStep';
import { AssignPasswordForm, UsersPage } from './UsersPage';

// Etapa 9H, fase D (§5): asistente de migración del sistema anterior. Los archivos los elige Rust con sus diálogos
// (la pantalla solo ve el nombre), la subida la hace Rust por partes y Go analiza e importa en segundo plano; la
// pantalla muestra el resumen previo, confirma (en rojo si reemplaza un lote), sigue el avance y nunca oculta una
// diferencia de dinero. Además, «Asignar contraseña» en Usuarios y la acción de Archivo.

const native = vi.hoisted(() => ({ invoke: vi.fn() }));
vi.mock('@tauri-apps/api/core', () => ({
  invoke: native.invoke,
  isTauri: () => true,
  // Canal simulado: la prueba llama a onmessage como lo haría Rust.
  Channel: class {
    onmessage: (message: unknown) => void = () => undefined;
  },
}));

const batchID = '00000000-0000-4000-8000-0000000009d1';
const uploadID = '00000000-0000-4000-8000-0000000009d2';
const runID = '00000000-0000-4000-8000-0000000009d3';

function batch(overrides: Partial<LegacyBatch> = {}): LegacyBatch {
  return {
    id: batchID,
    status: 'completed',
    imported_at: '2026-11-15T18:00:00Z',
    completed_at: '2026-11-15T18:01:00Z',
    imported_by: 'admin',
    backup_taken_at: '2026-11-12T22:05:33Z',
    fingerprint: 'a'.repeat(64),
    sales: 48230,
    sales_first_at: '2018-03-02T15:00:00Z',
    sales_last_at: '2026-11-12T21:00:00Z',
    has_differences: false,
    ...overrides,
  };
}

function preview(overrides: Partial<LegacyPreview> = {}): LegacyPreview {
  return {
    package: { format_version: 1, created_at: null, backup_taken_at: null, size_bytes: 32885 },
    counts: { products: 312, tables: 18, users: 6, sales: 48230, payments: 48300, categories: 20 },
    sales_period: { first: '2018-03', last: '2026-11', label: '03/2018 a 11/2026' },
    tax_rates: { '0': 311, '16': 1 },
    totals: {
      sales_by_year: [{ year: 2026, amount: '1234.5000' }],
      payments_by_method_year: [],
      cancelled_legacy_style: '10.0000',
      withdrawals: '20.0000',
      expenses: '0.0000',
    },
    flags: [
      {
        entity: 'sale',
        code: 'total_mismatch',
        label: 'Venta cuyo total no cuadra',
        count: 2,
        listed: true,
      },
    ],
    notes: [],
    warnings: ['El sistema anterior no guardaba dólares ni tipo de cambio.'],
    current_batch: null,
    will_replace: false,
    clean: true,
    blockers: [],
    ...overrides,
  };
}

const okValidation: LegacyValidation = {
  ok: true,
  has_money_differences: false,
  checks: [
    {
      group: 'counts',
      key: 'sales',
      label: 'Ventas (todas)',
      kind: 'count',
      expected: '48230',
      actual: '48230',
      difference: '0',
      ok: true,
    },
  ],
  notes: [],
};

function run(overrides: Partial<LegacyRun> = {}): LegacyRun {
  return {
    id: runID,
    kind: 'analyze',
    status: 'succeeded',
    phase: 'done',
    progress: { done: 1, total: 1, percent: 100 },
    upload_id: uploadID,
    package_bytes: 32885,
    analysis_run_id: null,
    replace_batch_id: null,
    batch_id: null,
    backup_file: null,
    error_code: null,
    error_message: null,
    with_differences: false,
    summary: preview(),
    requested_by: '00000000-0000-4000-8000-000000000001',
    created_at: '2026-11-16T10:00:00Z',
    started_at: '2026-11-16T10:00:00Z',
    finished_at: '2026-11-16T10:00:01Z',
    ...overrides,
  };
}

let status: LegacyStatus;
let runs: LegacyRun[];
let importReply: LegacyRun;
// El paquete que «eligió» el diálogo nativo simulado (Rust lo recuerda; la pantalla lo vuelve a leer).
let chosenPackage: { file_name: string; size: number } | null;

function apiCalls(operation: string) {
  return native.invoke.mock.calls
    .filter(([command, args]) => command === 'api_request' && args?.operation === operation)
    .map(([, args]) => args as { resourceId: string | null; body: Record<string, unknown> | null });
}

function bridgeCalls(command: string) {
  return native.invoke.mock.calls.filter(([name]) => name === command);
}

beforeEach(() => {
  status = {
    max_upload_bytes: 1 << 30,
    part_size: 1 << 20,
    current_batch: null,
    active_run: null,
    recent_runs: [],
    uploads: [],
  };
  runs = [run()];
  importReply = run({ kind: 'import', status: 'queued', phase: 'queued', summary: null });
  chosenPackage = null;
  native.invoke.mockImplementation(
    async (
      command: string,
      args?: {
        operation?: string;
        resourceId?: string;
        channel?: { onmessage: (message: unknown) => void };
      },
    ) => {
      switch (command) {
        case 'legacy_prepare_cleanup':
          return { result: 'cleaned' };
        case 'legacy_prepare_watch':
          return { running: false, events: [] };
        case 'legacy_selection':
          return { backup: null, output: null, package: chosenPackage };
        case 'legacy_choose_package':
          chosenPackage = { file_name: 'paquete.adelmig', size: 32885 };
          return { result: 'chosen', file: chosenPackage };
        case 'legacy_upload_package':
          args?.channel?.onmessage({ sent: 32885, total: 32885, percent: 100 });
          return {
            status: 200,
            body: { result: 'uploaded', upload: { id: uploadID, status: 'complete' } },
          };
        case 'legacy_save_report':
          return { status: 200, body: { result: 'saved', file_name: 'informe.pdf' } };
        case 'api_request':
          break;
        default:
          throw new Error(`Comando no esperado: ${command}`);
      }
      switch (args?.operation) {
        case 'maintenance_legacy':
          return { status: 200, body: status };
        case 'maintenance_legacy_analyze':
          return { status: 200, body: run({ status: 'queued', phase: 'queued', summary: null }) };
        case 'maintenance_legacy_run':
          return { status: 200, body: runs.length > 1 ? runs.shift() : runs[0] };
        case 'maintenance_legacy_import':
          return { status: 200, body: importReply };
        case 'maintenance_legacy_cancel_upload':
          return { status: 200, body: { cancelled: true } };
        case 'users':
          return { status: 200, body: [] };
        default:
          throw new Error(`Operación no esperada: ${args?.operation}`);
      }
    },
  );
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe('textos y reglas del asistente', () => {
  it('el resumen se dice como en la especificación', () => {
    expect(summarySentence(preview())).toBe(
      '312 productos, 18 mesas, 6 usuarios y 48,230 ventas del 03/2018 al 11/2026.',
    );
    expect(monthLabel('2019-10')).toBe('10/2019');
  });

  it('la frase de paso pide 8 caracteres y que coincida', () => {
    expect(passphraseProblem('corta')).toContain('al menos 8');
    expect(passphraseProblem('ñandúñañ')).toBe('');
    expect(passphraseProblem('frase de paso', 'otra frase')).toBe(
      'Las dos frases de paso no coinciden.',
    );
  });

  it('Docker sin instalar dice qué instalar y dónde, y lo demás muestra el mensaje del lector', () => {
    expect(preparationError('DOCKER_MISSING', '').message).toContain(
      'https://www.docker.com/products/docker-desktop/',
    );
    expect(preparationError('DOCKER_NOT_RUNNING', '').message).toContain('Abre Docker Desktop');
    expect(preparationError('RESTORE_FAILED', 'No se pudo abrir el respaldo.').message).toBe(
      'No se pudo abrir el respaldo.',
    );
  });

  it('las diferencias de dinero van primero', () => {
    const checks = failingChecks({
      ok: false,
      has_money_differences: true,
      notes: null,
      checks: [
        { ...okValidation.checks[0]!, ok: false, actual: '48229', difference: '-1' },
        {
          group: 'sales_by_year',
          key: '2026',
          label: 'Total vendido 2026',
          kind: 'money',
          expected: '10.0000',
          actual: '11.0000',
          difference: '1.0000',
          ok: false,
        },
      ],
    });
    expect(checks.map((check) => check.kind)).toEqual(['money', 'count']);
  });

  it('el avance de la preparación se arma con los eventos del lector', () => {
    let view = reducePreparation(
      { running: false, steps: {}, progress: null, table: null, result: null, error: null },
      { type: 'start' },
    );
    view = reducePreparation(view, {
      type: 'event',
      event: { event: 'step', step: 'read', status: 'started' },
    });
    view = reducePreparation(view, {
      type: 'event',
      event: { event: 'table', status: 'done', done: 26, total: 52, rows: 10 },
    });
    expect(view.running).toBe(true);
    expect(view.steps.read).toBe('started');
    expect(view.table).toEqual({ done: 26, total: 52 });
    view = reducePreparation(view, {
      type: 'event',
      event: { event: 'error', code: 'CANCELLED', message: 'Cancelada' },
    });
    expect(view.running).toBe(false);
    expect(view.error?.code).toBe('CANCELLED');
  });
});

describe('pantalla del asistente', () => {
  it('en una terminal explica que se usa en la computadora principal y no consulta nada', () => {
    render(<LegacyMigrationPage mainComputer={false} />);
    expect(screen.getByText(/se hace en la computadora principal del restaurante/)).toBeTruthy();
    expect(native.invoke).not.toHaveBeenCalled();
  });

  it('se declara bajo Mantenimiento con el permiso de Mantenimiento', () => {
    expect(canOpen('legacy-migration', ['settings.read'])).toBe(false);
    expect(canOpen('legacy-migration', ['settings.manage'])).toBe(true);
    expect(breadcrumbOf('legacy-migration').map((entry) => entry.label)).toEqual([
      'Administración',
      'Sistema',
      'Mantenimiento',
      'Migrar del sistema anterior',
    ]);
  });

  it('al abrir limpia restos de una preparación y dice si ya hay un lote importado', async () => {
    status.current_batch = batch({ has_differences: true });
    render(<LegacyMigrationPage mainComputer />);
    const current = await screen.findByRole('region', {
      name: 'Ya hay datos importados del sistema anterior',
    });
    expect(current.textContent).toContain('por admin');
    expect(current.textContent).toContain('48,230');
    expect(within(current).getByText('Con diferencias')).toBeTruthy();
    expect(bridgeCalls('legacy_prepare_cleanup')).toHaveLength(1);
  });

  it('la pantalla nunca envía rutas: elegir, subir por Rust y analizar con la frase de paso', async () => {
    runs = [run({ status: 'running', phase: 'reading', summary: null }), run()];
    render(<LegacyMigrationPage mainComputer />);
    const step = await screen.findByRole('region', { name: '2. Importar el paquete' });
    fireEvent.click(within(step).getByRole('button', { name: 'Elegir el paquete' }));
    await within(step).findByText('paquete.adelmig');
    fireEvent.change(within(step).getByLabelText('Frase de paso del paquete'), {
      target: { value: 'corta' },
    });
    fireEvent.click(within(step).getByRole('button', { name: 'Revisar el paquete' }));
    expect(within(step).getByRole('alert').textContent).toContain('al menos 8');
    expect(bridgeCalls('legacy_upload_package')).toHaveLength(0);
    fireEvent.change(within(step).getByLabelText('Frase de paso del paquete'), {
      target: { value: 'Frase de migración' },
    });
    fireEvent.click(within(step).getByRole('button', { name: 'Revisar el paquete' }));
    await within(step).findByRole('heading', { name: 'Resumen previo' }, { timeout: 4000 });
    // Rust recibe solo el canal del avance; Go, el identificador de la subida y la frase.
    expect(Object.keys(bridgeCalls('legacy_upload_package')[0]![1] as object)).toEqual(['channel']);
    expect(apiCalls('maintenance_legacy_analyze')[0]!.body).toEqual({
      upload_id: uploadID,
      passphrase: 'Frase de migración',
    });
    expect(step.textContent).toContain('312 productos, 18 mesas, 6 usuarios y 48,230 ventas');
    expect(within(step).getByRole('heading', { name: 'Datos incoherentes' })).toBeTruthy();
    expect(within(step).getByRole('button', { name: 'Importar' })).toBeTruthy();
  });

  it('una instalación con operación propia explica por qué no se importa y no ofrece importar', async () => {
    runs = [
      run({
        summary: preview({
          clean: false,
          blockers: [
            { code: 'own_accounts', message: 'Hay 3 cuentas registradas en AD-EL POS.', count: 3 },
          ],
        }),
      }),
    ];
    render(<LegacyMigrationPage mainComputer />);
    const step = await screen.findByRole('region', { name: '2. Importar el paquete' });
    fireEvent.click(within(step).getByRole('button', { name: 'Elegir el paquete' }));
    await within(step).findByText('paquete.adelmig');
    fireEvent.change(within(step).getByLabelText('Frase de paso del paquete'), {
      target: { value: 'Frase de migración' },
    });
    fireEvent.click(within(step).getByRole('button', { name: 'Revisar el paquete' }));
    const refusal = await within(step).findByRole('alert', {}, { timeout: 4000 });
    expect(refusal.textContent).toContain('No se puede importar en esta instalación.');
    expect(refusal.textContent).toContain('Hay 3 cuentas registradas en AD-EL POS.');
    expect(within(step).queryByRole('button', { name: 'Importar' })).toBeNull();
  });

  it('reemplazar un lote pide confirmación en rojo y envía el lote que se reemplaza', async () => {
    status.current_batch = batch();
    runs = [run({ summary: preview({ will_replace: true, current_batch: batch() }) })];
    const report: LegacyReport = {
      ...preview(),
      batch_id: '00000000-0000-4000-8000-0000000009d4',
      backup_file: 'adel_pos-manual.dump',
      validation: {
        ok: false,
        has_money_differences: true,
        notes: [],
        checks: [
          {
            group: 'sales_by_year',
            key: '2026',
            label: 'Total vendido 2026',
            kind: 'money',
            expected: '1234.5000',
            actual: '1235.5000',
            difference: '1.0000',
            ok: false,
          },
        ],
      },
    };
    importReply = run({
      kind: 'import',
      status: 'succeeded',
      with_differences: true,
      batch_id: report.batch_id,
      summary: report,
    });
    render(<LegacyMigrationPage mainComputer />);
    const step = await screen.findByRole('region', { name: '2. Importar el paquete' });
    fireEvent.click(within(step).getByRole('button', { name: 'Elegir el paquete' }));
    await within(step).findByText('paquete.adelmig');
    fireEvent.change(within(step).getByLabelText('Frase de paso del paquete'), {
      target: { value: 'Frase de migración' },
    });
    fireEvent.click(within(step).getByRole('button', { name: 'Revisar el paquete' }));
    const replace = await within(step).findByRole(
      'button',
      { name: 'Reemplazar lo importado' },
      { timeout: 4000 },
    );
    expect(replace.className).toContain('danger-button');
    expect(step.textContent).toContain('Ya hay datos importados del sistema anterior.');
    fireEvent.click(replace);
    const dialog = await screen.findByRole('alertdialog', {
      name: 'Reemplazar lo importado del sistema anterior',
    });
    expect(dialog.textContent).toContain('no se puede deshacer salvo restaurando ese respaldo');
    const confirm = within(dialog).getByRole('button', { name: 'Reemplazar lo importado' });
    expect(confirm.className).toContain('danger-solid-button');
    fireEvent.click(confirm);
    await waitFor(() => expect(apiCalls('maintenance_legacy_import')).toHaveLength(1));
    expect(apiCalls('maintenance_legacy_import')[0]!.body).toEqual({
      upload_id: uploadID,
      passphrase: 'Frase de migración',
      analysis_run_id: runID,
      confirm: true,
      replace_batch_id: batchID,
    });
    // La importación ya terminó con una diferencia de dinero: se muestra con su importe, nunca oculta.
    const alert = await within(step).findByRole('alert', {}, { timeout: 4000 });
    expect(alert.textContent).toContain('Hay diferencias de dinero');
    expect(alert.textContent).toContain('diferencia $1.00');
    fireEvent.click(within(step).getByRole('button', { name: 'Descargar informe (PDF)' }));
    await waitFor(() => expect(bridgeCalls('legacy_save_report')).toHaveLength(1));
    expect(bridgeCalls('legacy_save_report')[0]![1]).toEqual({ batchId: report.batch_id });
  });
});

describe('«Asignar contraseña» en Usuarios', () => {
  const imported: User = {
    id: '00000000-0000-4000-8000-0000000009d5',
    username: 'cajera',
    display_name: 'Cajera Sintética',
    role_id: '00000000-0000-4000-8000-0000000009d6',
    role_name: 'Cajero',
    active: true,
    initial_password: false,
    has_password: false,
  };

  it('solo los usuarios sin contraseña aparecen «Sin acceso» y con el botón', async () => {
    native.invoke.mockImplementation(async (_command: string, args?: { operation?: string }) => {
      if (args?.operation === 'users') {
        return {
          status: 200,
          body: [
            imported,
            { ...imported, id: imported.role_id, username: 'admin', has_password: true },
          ],
        };
      }
      return { status: 200, body: [] };
    });
    render(
      <UsersPage
        can={(key) => key === 'users.read' || key === 'users.manage'}
        onChanged={async () => undefined}
      />,
    );
    const row = (await screen.findByText('cajera')).closest('tr')!;
    expect(within(row).getByText('Sin acceso')).toBeTruthy();
    expect(within(row).getByRole('button', { name: 'Asignar contraseña' })).toBeTruthy();
    const admin = screen.getByText('admin').closest('tr')!;
    expect(within(admin).queryByText('Sin acceso')).toBeNull();
    expect(within(admin).queryByRole('button', { name: 'Asignar contraseña' })).toBeNull();
  });

  it('sin users.manage no se ofrece', async () => {
    native.invoke.mockImplementation(async () => ({ status: 200, body: [imported] }));
    render(<UsersPage can={(key) => key === 'users.read'} onChanged={async () => undefined} />);
    const row = (await screen.findByText('cajera')).closest('tr')!;
    expect(within(row).getByText('Sin acceso')).toBeTruthy();
    expect(within(row).queryByRole('button', { name: 'Asignar contraseña' })).toBeNull();
  });

  it('envía solo la contraseña inicial cuando las dos coinciden', async () => {
    native.invoke.mockImplementation(async () => ({ status: 200, body: imported }));
    const assigned = vi.fn(async () => undefined);
    render(<AssignPasswordForm user={imported} onAssigned={assigned} />);
    fireEvent.change(screen.getByLabelText('Contraseña inicial'), {
      target: { value: 'Inicial 9H' },
    });
    fireEvent.change(screen.getByLabelText('Escríbela otra vez'), {
      target: { value: 'Otra 9H 1' },
    });
    fireEvent.click(screen.getByRole('button', { name: 'Asignar contraseña' }));
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Asignar contraseña' })).toHaveProperty(
        'disabled',
        false,
      ),
    );
    expect(apiCalls('assign_user_password')).toHaveLength(0);
    fireEvent.change(screen.getByLabelText('Contraseña inicial'), {
      target: { value: 'Inicial 9H' },
    });
    fireEvent.change(screen.getByLabelText('Escríbela otra vez'), {
      target: { value: 'Inicial 9H' },
    });
    fireEvent.click(screen.getByRole('button', { name: 'Asignar contraseña' }));
    await waitFor(() => expect(assigned).toHaveBeenCalled());
    expect(apiCalls('assign_user_password')[0]).toMatchObject({
      resourceId: imported.id,
      body: { password: 'Inicial 9H' },
    });
  });
});

describe('Archivo › «Migrar del sistema anterior…»', () => {
  function context(overrides: Partial<CommandContext> = {}): CommandContext {
    return {
      authenticated: true,
      permissions: ['settings.manage', 'settings.read'],
      screen: 'administration',
      desktop: true,
      mainComputer: true,
      operation: null,
      cashOptions: [],
      ...overrides,
    };
  }

  function runtime(): CommandRuntime {
    return {
      requestScreen: vi.fn(async () => true),
      operation: () => null,
      openCashSheet: vi.fn(),
      openPalette: vi.fn(),
      openShortcuts: vi.fn(),
      openAbout: vi.fn(),
      toggleFullscreen: vi.fn(async () => undefined),
      toggleTheme: vi.fn(async () => undefined),
      logout: vi.fn(async () => undefined),
      focusSearch: vi.fn(),
      notify: vi.fn(),
    };
  }

  it('solo abre el asistente, con el permiso y en la computadora principal', async () => {
    const action = actionByID('file.legacy_migration')!;
    expect(action.group).toBe('file');
    expect(action.shortcut).toBeUndefined();
    expect(action.visible(context())).toBe(true);
    expect(action.visible(context({ mainComputer: false }))).toBe(false);
    expect(action.visible(context({ permissions: ['settings.read'] }))).toBe(false);
    const tools = runtime();
    await action.run(tools, context());
    expect(tools.requestScreen).toHaveBeenCalledWith('legacy-migration');
  });

  it('la paleta la ofrece una vez (la acción), no además como pantalla', () => {
    const entries = commandEntries(context(), runtime(), []).filter((entry) =>
      entry.label.startsWith('Migrar del sistema anterior'),
    );
    expect(entries.map((entry) => entry.id)).toEqual(['file.legacy_migration']);
  });
});
