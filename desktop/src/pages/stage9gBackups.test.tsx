// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { App } from '../App';
import product from '../generated/product.json';
import { sectionByID } from '../navigation/sections';
import { alertMessage, folderLabel, lastBackupCard, sizeLabel } from '../services/backups';
import type { BackupState, BackupSummary } from '../services/backups';
import type { Identity } from '../services/models';
import { backupRun, backupState } from '../testing/backups';
import { serverTerminalInstallation } from '../testing/installation';
import { BackupAlertBanner } from './maintenance/BackupAlert';
import { BackupSection } from './maintenance/BackupSection';
import { ServerOnlyPage } from './server/ServerOnlyPage';

// Etapa 9G, fase F (§7): Sistema › Mantenimiento › «Respaldos», el aviso persistente de la pantalla de inicio para
// quien administra la configuración y el último respaldo en «Solo servidor». Go decide la programación, la carpeta,
// la retención y el aviso; la interfaz solo los muestra. La carpeta se elige con la operación cerrada de Rust
// (choose_backup_directory): la interfaz nunca envía una ruta.

const native = vi.hoisted(() => ({ invoke: vi.fn() }));
vi.mock('@tauri-apps/api/core', () => ({ invoke: native.invoke, isTauri: () => true }));

let state: BackupState;
let summary: BackupSummary;
let backupReply: () => Promise<unknown>;
let folderReply: () => Promise<unknown>;

function requests(operation: string) {
  return native.invoke.mock.calls
    .filter(([command, args]) => command === 'api_request' && args?.operation === operation)
    .map(([, args]) => args as { body: Record<string, unknown> | null });
}

function calls(command: string) {
  return native.invoke.mock.calls.filter(([name]) => name === command);
}

const failedAlert = {
  kind: 'failed' as const,
  reason: 'verify_failed',
  since: '2026-11-13T11:00:00Z',
  failures: 2,
};

beforeEach(() => {
  state = backupState();
  summary = { last_backup: null, alert: null, running: false };
  backupReply = async () => ({ status: 200, body: { run: backupRun({ kind: 'manual' }) } });
  folderReply = async () => ({ status: 200, body: { result: 'cancelled' } });
  native.invoke.mockImplementation(async (command: string, args?: { operation?: string }) => {
    if (command === 'choose_backup_directory') {
      return folderReply();
    }
    if (command !== 'api_request') {
      throw new Error(`Comando no esperado: ${command}`);
    }
    switch (args?.operation) {
      case 'maintenance_backups':
        return { status: 200, body: state };
      case 'maintenance_backups_status':
        return { status: 200, body: summary };
      case 'maintenance_save_backup_settings':
        return { status: 200, body: state };
      case 'maintenance_backup_default_directory':
        return { status: 200, body: state };
      case 'maintenance_backup_now':
        return backupReply();
      default:
        throw new Error(`Operación no esperada: ${args?.operation}`);
    }
  });
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe('textos de los respaldos', () => {
  it('dice dónde está cada respaldo sin rutas y su tamaño en palabras sencillas', () => {
    expect(folderLabel({ location: 'default', folder_name: 'backups', volume_name: '' })).toBe(
      'la carpeta predeterminada',
    );
    expect(folderLabel({ custom: true, folder_name: 'Respaldos', volume_name: 'KINGSTON' })).toBe(
      '«Respaldos» en «KINGSTON»',
    );
    expect(folderLabel({ custom: true, folder_name: '', volume_name: 'KINGSTON' })).toBe(
      'el disco «KINGSTON»',
    );
    expect(folderLabel({ location: 'custom', folder_name: 'Respaldos', volume_name: '' })).toBe(
      'la carpeta «Respaldos»',
    );
    expect(sizeLabel(5_662_310)).toBe('5.4 MB');
    expect(sizeLabel(820_000)).toBe('801 KB');
    expect(sizeLabel(null)).toBe('');
  });

  it('explica el último respaldo y el aviso en palabras del restaurante', () => {
    expect(lastBackupCard(null).value).toBe('Sin registro');
    const correct = lastBackupCard(backupRun());
    expect(correct.value).toBe('Correcto');
    expect(correct.state).toBe('good');
    expect(correct.description).toContain('5.4 MB');
    expect(correct.description).toContain('la carpeta predeterminada');
    const failed = lastBackupCard(
      backupRun({ status: 'failed', failure_reason: 'dump_failed', file_name: '' }),
    );
    expect(failed.value).toBe('Falló');
    expect(failed.state).toBe('warning');
    expect(failed.description).toContain('No se pudo copiar la base de datos.');
    const message = alertMessage(failedAlert);
    expect(message.title).toBe('No se pudo crear el respaldo de la base de datos');
    expect(message.description).toContain('Van 2 intentos fallidos');
    const fallback = alertMessage({ kind: 'fallback', reason: 'missing', since: '', failures: 0 });
    expect(fallback.title).toBe('La carpeta de los respaldos no estaba disponible');
    expect(fallback.description).toContain('¿se desconectó el disco o la USB?');
    const stale = alertMessage({
      kind: 'overdue',
      reason: 'stale',
      since: '2026-11-11T11:00:00Z',
      failures: 0,
    });
    expect(stale.title).toBe('No hay un respaldo reciente de la base de datos');
    expect(stale.description).toContain('El último respaldo correcto es del');
    const never = alertMessage({ kind: 'overdue', reason: 'never', since: '', failures: 0 });
    expect(never.description).toContain('Todavía no hay ningún respaldo correcto.');
    for (const text of [
      message.description,
      fallback.description,
      correct.description,
      stale.description,
      never.description,
    ]) {
      expect(text).not.toMatch(/\/|pg_|PostgreSQL|\.dump/);
    }
  });

  it('Mantenimiento anuncia los respaldos', () => {
    expect(sectionByID('maintenance').description).toContain('respaldos');
  });
});

describe('Mantenimiento › Respaldos', () => {
  it('muestra la programación, la carpeta, los últimos respaldos y el aviso vigente', async () => {
    state = backupState({
      alert: failedAlert,
      last_success: backupRun(),
      runs: [
        backupRun({
          id: 'r3',
          kind: 'manual',
          status: 'failed',
          failure_reason: 'verify_failed',
          file_name: '',
          size_bytes: null,
          started_at: '2026-11-14T15:00:00Z',
        }),
        backupRun({
          id: 'r2',
          location: 'custom',
          folder_name: 'Respaldos',
          volume_name: 'KINGSTON',
          used_default_directory: false,
        }),
        backupRun({ id: 'r1', removed: true, available: false }),
      ],
    });
    render(<BackupSection />);
    const section = await screen.findByRole('region', { name: 'Respaldos' });
    await within(section).findByText('No se pudo crear el respaldo de la base de datos');
    expect(within(section).getByRole('alert')).toBeTruthy();
    expect(within(section).queryByRole('button', { name: 'Ver respaldos' })).toBeNull();
    expect(
      within(section)
        .getByRole('button', { name: 'Respaldo automático diario' })
        .getAttribute('aria-pressed'),
    ).toBe('true');
    expect(
      within(section)
        .getByRole('button', { name: 'Hora de inicio del día (05:00)' })
        .getAttribute('aria-pressed'),
    ).toBe('true');
    expect(
      (within(section).getByLabelText(/Respaldos automáticos que se conservan/) as HTMLInputElement)
        .value,
    ).toBe('30');
    expect(
      within(section).getByText(
        'Se guardan en la carpeta predeterminada de la computadora principal.',
      ),
    ).toBeTruthy();
    const list = within(section).getByRole('list', { name: 'Últimos respaldos' });
    const items = within(list).getAllByRole('listitem');
    expect(items).toHaveLength(3);
    expect(items[0]?.textContent).toContain('Manual');
    expect(items[0]?.textContent).toContain('Falló');
    expect(items[0]?.textContent).toContain('al revisarlo no se pudo leer');
    expect(items[1]?.textContent).toContain('Correcto y revisado');
    expect(items[1]?.textContent).toContain('5.4 MB');
    expect(items[1]?.textContent).toContain('En «Respaldos» en «KINGSTON»');
    expect(items[2]?.textContent).toContain('Se borró al conservar solo los más recientes.');
    // Las rutas solo están en «Información para soporte».
    expect(
      within(section)
        .getByRole('button', { name: /Información para soporte/ })
        .getAttribute('aria-expanded'),
    ).toBe('false');
  });

  it('guarda la hora de inicio del día como null y otra hora con su valor', async () => {
    render(<BackupSection />);
    const section = await screen.findByRole('region', { name: 'Respaldos' });
    await within(section).findByRole('button', { name: 'Guardar respaldos automáticos' });
    fireEvent.click(within(section).getByRole('button', { name: 'Guardar respaldos automáticos' }));
    await waitFor(() => expect(requests('maintenance_save_backup_settings')).toHaveLength(1));
    expect(requests('maintenance_save_backup_settings')[0]?.body).toEqual({
      enabled: true,
      time: null,
      keep: 30,
    });
    fireEvent.click(within(section).getByRole('button', { name: 'Otra hora' }));
    expect(within(section).getByRole('group', { name: 'Hora del respaldo' })).toBeTruthy();
    fireEvent.click(within(section).getByRole('button', { name: 'Respaldo automático diario' }));
    expect(
      within(section).getByText(
        'Sin respaldo automático: solo se crean respaldos con «Respaldar ahora».',
      ),
    ).toBeTruthy();
    fireEvent.click(within(section).getByRole('button', { name: 'Guardar respaldos automáticos' }));
    await waitFor(() => expect(requests('maintenance_save_backup_settings')).toHaveLength(2));
    expect(requests('maintenance_save_backup_settings')[1]?.body).toEqual({
      enabled: false,
      time: '05:00',
      keep: 30,
    });
  });

  it('«Respaldar ahora» conserva su clave ante una respuesta sin conexión y avisa el resultado', async () => {
    let attempt = 0;
    backupReply = async () => {
      attempt += 1;
      if (attempt === 1) {
        throw 'No se pudo conectar con el servidor.';
      }
      return { status: 200, body: { run: backupRun({ kind: 'manual' }) } };
    };
    render(<BackupSection />);
    const section = await screen.findByRole('region', { name: 'Respaldos' });
    for (let round = 0; round < 2; round += 1) {
      const button = await within(section).findByRole('button', { name: 'Respaldar ahora' });
      await waitFor(() => expect((button as HTMLButtonElement).disabled).toBe(false));
      fireEvent.click(button);
      await waitFor(() => expect(requests('maintenance_backup_now')).toHaveLength(round + 1));
    }
    const [first, second] = requests('maintenance_backup_now');
    expect(first?.body?.operation_id).toMatch(/^[0-9a-f-]{36}$/);
    expect(second?.body?.operation_id).toBe(first?.body?.operation_id);
    // Después de un resultado, el siguiente respaldo lleva otra clave.
    const button = await within(section).findByRole('button', { name: 'Respaldar ahora' });
    await waitFor(() => expect((button as HTMLButtonElement).disabled).toBe(false));
    fireEvent.click(button);
    await waitFor(() => expect(requests('maintenance_backup_now')).toHaveLength(3));
    expect(requests('maintenance_backup_now')[2]?.body?.operation_id).not.toBe(
      first?.body?.operation_id,
    );
  });

  it('la carpeta se elige con el diálogo nativo, sin enviar rutas, y se puede volver a la predeterminada', async () => {
    render(<BackupSection />);
    const section = await screen.findByRole('region', { name: 'Respaldos' });
    fireEvent.click(await within(section).findByRole('button', { name: 'Elegir carpeta' }));
    await waitFor(() => expect(calls('choose_backup_directory')).toHaveLength(1));
    // La operación nativa no recibe argumentos: Rust abre el diálogo y envía la carpeta a Go.
    expect(calls('choose_backup_directory')[0]?.[1]).toBeUndefined();
    const chosen = backupState({
      settings: {
        ...backupState().settings,
        directory: {
          custom: true,
          path: '/Volumes/KINGSTON/Respaldos',
          folder_name: 'Respaldos',
          volume_name: 'KINGSTON',
          available: false,
        },
      },
    });
    folderReply = async () => ({ status: 200, body: { result: 'saved', state: chosen } });
    state = chosen;
    fireEvent.click(within(section).getByRole('button', { name: 'Elegir carpeta' }));
    await within(section).findByText('Se guardan en «Respaldos» en «KINGSTON».');
    expect(within(section).getByText(/Ahora no se encuentra/)).toBeTruthy();
    fireEvent.click(
      within(section).getByRole('button', { name: 'Usar la carpeta predeterminada' }),
    );
    await waitFor(() => expect(requests('maintenance_backup_default_directory')).toHaveLength(1));
    for (const [, args] of native.invoke.mock.calls) {
      expect(JSON.stringify(args ?? {})).not.toContain('/Volumes');
    }
  });

  it('desde otra terminal no ofrece cambiar la carpeta', async () => {
    state = backupState({ directory_change_available: false });
    render(<BackupSection />);
    const section = await screen.findByRole('region', { name: 'Respaldos' });
    await within(section).findByText('La carpeta solo se cambia desde la computadora principal.');
    expect(within(section).queryByRole('button', { name: 'Elegir carpeta' })).toBeNull();
  });
});

describe('aviso de respaldos en la pantalla de inicio', () => {
  it('el aviso de respaldo atrasado se ve como el de un fallo', async () => {
    summary = {
      last_backup: backupRun({ started_at: '2026-11-11T11:00:00Z' }),
      alert: { kind: 'overdue', reason: 'stale', since: '2026-11-11T11:00:00Z', failures: 0 },
      running: false,
    };
    render(<BackupAlertBanner onOpen={vi.fn()} />);
    const alert = await screen.findByRole('alert');
    expect(alert.textContent).toContain('No hay un respaldo reciente de la base de datos');
    expect(alert.className).toContain('backup-warning-failed');
  });

  it('sin aviso no muestra nada; con un fallo, el aviso lleva a Mantenimiento', async () => {
    const onOpen = vi.fn();
    const { unmount } = render(<BackupAlertBanner onOpen={onOpen} />);
    await waitFor(() => expect(requests('maintenance_backups_status')).toHaveLength(1));
    expect(screen.queryByRole('alert')).toBeNull();
    unmount();
    summary = { last_backup: backupRun({ status: 'failed' }), alert: failedAlert, running: false };
    render(<BackupAlertBanner onOpen={onOpen} />);
    const alert = await screen.findByRole('alert');
    expect(alert.textContent).toContain('No se pudo crear el respaldo de la base de datos');
    fireEvent.click(within(alert).getByRole('button', { name: 'Ver respaldos' }));
    expect(onOpen).toHaveBeenCalledTimes(1);
  });
});

describe('Solo servidor › último respaldo', () => {
  it('muestra el último respaldo del estado del servicio y el aviso al administrador', async () => {
    summary = { last_backup: null, alert: failedAlert, running: false };
    native.invoke.mockImplementation(async (command: string, args?: { operation?: string }) => {
      if (command === 'api_request' && args?.operation === 'maintenance_service') {
        return {
          status: 200,
          body: {
            managed: true,
            restart_available: true,
            installation_mode: 'server-only',
            service_enabled: true,
            started_at: '2026-11-13T18:00:00Z',
            uptime_seconds: 120,
            server_version: product.version,
            terminals: { connected: 1, registered: 2 },
            last_backup: backupRun(),
          },
        };
      }
      if (command === 'api_request' && args?.operation === 'maintenance_backups_status') {
        return { status: 200, body: summary };
      }
      throw new Error(`Operación no esperada: ${args?.operation}`);
    });
    const onMaintenance = vi.fn();
    render(
      <ServerOnlyPage
        canManage
        canAdminister
        onAdministration={vi.fn()}
        onMaintenance={onMaintenance}
      />,
    );
    expect(await screen.findByText('Correcto')).toBeTruthy();
    expect(screen.getByText(/Revisado y guardado en la carpeta predeterminada/)).toBeTruthy();
    const alert = await screen.findByRole('alert');
    fireEvent.click(within(alert).getByRole('button', { name: 'Ver respaldos' }));
    expect(onMaintenance).toHaveBeenCalledTimes(1);
  });
});

// La pantalla de inicio de la terminal con el aviso: solo para quien tiene settings.manage y solo en el inicio.
describe('aviso en la aplicación', () => {
  const identity: Identity = {
    user: {
      id: '00000000-0000-0000-0000-000000000001',
      username: 'admin',
      display_name: 'Administrador',
      role_id: '00000000-0000-0000-0000-000000000002',
      role_name: 'Administrador',
      active: true,
      initial_password: false,
      support: false,
      session_id: '00000000-0000-0000-0000-000000000003',
      device_id: '00000000-0000-0000-0000-000000000004',
      permissions: ['users.read', 'settings.read', 'settings.manage'],
    },
    preferences: { theme: 'light', home_screen: 'administration' },
    terminal: {
      id: '00000000-0000-0000-0000-000000000004',
      name: 'Terminal de prueba',
      register_id: null,
      keyboard_mode: 'auto',
    },
    establishment: { name: 'Restaurante', timezone: 'America/Monterrey', currency: 'MXN' },
  };

  function mockApp(current: Identity) {
    native.invoke.mockImplementation(async (command: string, args?: { operation?: string }) => {
      if (command === 'authentication_state') {
        return {
          enrolled: true,
          has_session: true,
          local_setup_available: false,
          local_enrollment_available: false,
        };
      }
      if (command === 'connection_config') {
        return { installation_mode: 'server-terminal', server_url: 'http://127.0.0.1:8181' };
      }
      if (command === 'installation_state') {
        return serverTerminalInstallation;
      }
      if (command === 'server_snapshot') {
        return {
          live: { status: 200, body: { status: 'alive' } },
          ready: {
            status: 200,
            body: { status: 'ready', checks: { postgres: 'available', schema: 'compatible' } },
          },
          version: {
            status: 200,
            body: {
              product: product.technical_id,
              server_version: product.version,
              api_version: product.api_version,
              schema_version: product.schema_version,
            },
          },
        };
      }
      if (command !== 'api_request') {
        throw new Error('Comando de prueba desconocido');
      }
      switch (args?.operation) {
        case 'setup_status':
          return { status: 200, body: { configured: true } };
        case 'me':
          return { status: 200, body: current };
        case 'maintenance_backups_status':
          return { status: 200, body: summary };
        case 'maintenance_backups':
          return { status: 200, body: backupState() };
        case 'maintenance_service':
          return {
            status: 200,
            body: {
              managed: false,
              restart_available: false,
              installation_mode: '',
              service_enabled: false,
              started_at: '2026-11-13T18:00:00Z',
              uptime_seconds: 60,
              server_version: product.version,
              terminals: { connected: 0, registered: 0 },
              last_backup: null,
            },
          };
        default:
          return { status: 200, body: [] };
      }
    });
  }

  it('el administrador lo ve en la pantalla de inicio y lo lleva a Mantenimiento', async () => {
    summary = { last_backup: backupRun({ status: 'failed' }), alert: failedAlert, running: false };
    mockApp(identity);
    render(<App />);
    const alert = await screen.findByRole('alert');
    expect(alert.textContent).toContain('No se pudo crear el respaldo de la base de datos');
    fireEvent.click(within(alert).getByRole('button', { name: 'Ver respaldos' }));
    await screen.findByRole('heading', { name: 'Mantenimiento' });
    // Fuera de la pantalla de inicio el aviso no se repite.
    await waitFor(() =>
      expect(screen.queryByText('No se pudo crear el respaldo de la base de datos')).toBeNull(),
    );
  });

  it('sin settings.manage no se consulta ni se muestra', async () => {
    summary = { last_backup: null, alert: failedAlert, running: false };
    mockApp({ ...identity, user: { ...identity.user, permissions: ['users.read'] } });
    render(<App />);
    await screen.findByRole('navigation', { name: 'Navegación principal' });
    await screen.findByRole('heading', { name: 'Administración' });
    expect(requests('maintenance_backups_status')).toHaveLength(0);
    expect(screen.queryByRole('alert')).toBeNull();
  });
});
