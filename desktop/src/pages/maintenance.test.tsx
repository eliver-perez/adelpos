// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { canOpen, tilesOf } from '../navigation/sections';
import type { TestDataPreview, TestDataResult } from '../services/maintenance';
import { backupState } from '../testing/backups';
import { serverTerminalInstallation } from '../testing/installation';
import { MaintenancePage } from './MaintenancePage';

// Etapa 9F, fase A (§3): «Quitar datos de prueba» en Administración › Sistema › Mantenimiento. La lista
// la arma Go; la pantalla la muestra, confirma en rojo diciendo que primero se crea un respaldo y avisa el
// resultado con el nombre del respaldo, sin rutas técnicas.

const native = vi.hoisted(() => ({ invoke: vi.fn() }));
vi.mock('@tauri-apps/api/core', () => ({ invoke: native.invoke, isTauri: () => true }));

type Reply = { status: number; body: unknown };
let previews: TestDataPreview[];
let removal: Reply;

const planID = 'a'.repeat(64);

const withTestData: TestDataPreview = {
  plan_id: planID,
  remove: [
    {
      kind: 'product',
      id: '00000000-0000-4000-8000-0000000009f5',
      name: 'Filete de prueba',
      reason: 'Producto de prueba (código FILETE).',
    },
  ],
  deactivate: [
    {
      kind: 'printer',
      id: '00000000-0000-4000-8000-0000000009f2',
      name: 'Destino controlado E2E',
      reason: 'Ya tiene registros: trabajos de impresión.',
    },
    {
      kind: 'station',
      id: '00000000-0000-4000-8000-0000000009f3',
      name: 'Cocina caliente',
      reason: 'Ya tiene registros: tareas de cocina.',
    },
  ],
  unlink: [
    {
      // Revisión de la 9F (B1): la ruta de una categoría del restaurante que solo enviaba a estaciones de
      // prueba no es un dato de prueba; Go la presenta aquí con su explicación.
      kind: 'route',
      id: '00000000-0000-4000-8000-0000000009f1',
      name: 'Ruta de la categoría «Platillos»',
      reason:
        'Solo enviaba a estaciones de prueba («Cocina caliente»); se quita: sus productos sin ruta propia quedarán sin ruta de preparación hasta que le asignes otra.',
    },
    {
      kind: 'setting',
      id: 'receipt:00000000-0000-4000-8000-0000000009f2',
      name: 'Impresora de tickets de Cobros',
      reason: 'Era «Destino controlado E2E»; queda sin impresora de tickets hasta que elijas una.',
    },
  ],
  keep: [
    {
      kind: 'category',
      id: '00000000-0000-4000-8000-0000000009f4',
      name: 'Platillos',
      reason: 'Tiene otros productos del restaurante, como «Tacos».',
    },
  ],
};

const empty: TestDataPreview = {
  plan_id: 'b'.repeat(64),
  remove: [],
  deactivate: [],
  unlink: [],
  keep: [],
};

const result: TestDataResult = {
  removed: withTestData.remove,
  deactivated: withTestData.deactivate,
  unlinked: withTestData.unlink,
  kept: withTestData.keep,
  backup: 'adel_pos-20261113T183005Z-0a1b2c.dump',
};

// Etapa 9G: la página también muestra «Servicio del servidor» y «Modalidad de esta computadora»
// (stage9gModality.test.tsx); aquí responden sin intervenir en «Quitar datos de prueba».
function stage9gReply(command: string, operation?: string) {
  if (command === 'installation_state') {
    return serverTerminalInstallation;
  }
  // Etapa 9G (fase F): la sección «Respaldos» (stage9gBackups.test.tsx).
  if (command === 'api_request' && operation === 'maintenance_backups') {
    return { status: 200, body: backupState() };
  }
  if (command === 'api_request' && operation === 'maintenance_service') {
    return {
      status: 200,
      body: {
        managed: false,
        restart_available: false,
        installation_mode: '',
        service_enabled: false,
        started_at: '2026-11-13T18:00:00Z',
        uptime_seconds: 60,
        server_version: '0.9.5',
        terminals: { connected: 0, registered: 0 },
        last_backup: null,
      },
    };
  }
  return undefined;
}

function requests(operation: string) {
  return native.invoke.mock.calls
    .filter(([command, args]) => command === 'api_request' && args?.operation === operation)
    .map(([, args]) => args as { body: Record<string, unknown> | null });
}

beforeEach(() => {
  previews = [withTestData, empty];
  removal = { status: 200, body: result };
  native.invoke.mockImplementation(async (command: string, args: { operation: string }) => {
    const stage9g = stage9gReply(command, args?.operation);
    if (stage9g) {
      return stage9g;
    }
    if (command !== 'api_request') {
      throw new Error(`Comando no esperado: ${command}`);
    }
    if (args.operation === 'maintenance_test_data') {
      const next = previews.length > 1 ? previews.shift()! : previews[0]!;
      return { status: 200, body: next };
    }
    if (args.operation === 'maintenance_remove_test_data') {
      return removal;
    }
    return { status: 404, body: { error: { code: 'NOT_FOUND', message: 'No existe.' } } };
  });
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe('Mantenimiento › Quitar datos de prueba', () => {
  it('se declara en Sistema solo con settings.manage', () => {
    expect(canOpen('maintenance', ['settings.read'])).toBe(false);
    expect(canOpen('maintenance', ['settings.manage'])).toBe(true);
    expect(tilesOf('system', ['settings.manage']).map((entry) => entry.label)).toContain(
      'Mantenimiento',
    );
  });

  it('no consulta nada hasta «Revisar datos de prueba» y muestra la lista agrupada', async () => {
    render(<MaintenancePage />);
    expect(requests('maintenance_test_data')).toHaveLength(0);
    expect(screen.queryByRole('button', { name: 'Quitar datos de prueba' })).toBeNull();
    fireEvent.click(screen.getByRole('button', { name: 'Revisar datos de prueba' }));
    const removeList = await screen.findByRole('list', { name: 'Se eliminará' });
    expect(removeList.textContent).toContain('Filete de prueba');
    const deactivate = screen.getByRole('list', { name: 'Se desactivará' });
    expect(within(deactivate).getAllByRole('listitem')).toHaveLength(2);
    expect(deactivate.textContent).toContain('Ya tiene registros: trabajos de impresión.');
    const unlink = screen.getByRole('list', { name: 'Se desvinculará' });
    expect(unlink.textContent).toContain('Impresora de tickets de Cobros');
    expect(unlink.textContent).toContain('Ruta de preparación');
    expect(unlink.textContent).toContain('Ruta de la categoría «Platillos»');
    expect(unlink.textContent).toContain('Solo enviaba a estaciones de prueba');
    expect(screen.getByRole('list', { name: 'Se conserva' }).textContent).toContain(
      'Tiene otros productos del restaurante',
    );
    const button = screen.getByRole('button', { name: 'Quitar datos de prueba' });
    expect(button.className).toContain('danger-button');
  });

  it('sin datos de prueba lo dice y no ofrece quitar', async () => {
    previews = [empty];
    render(<MaintenancePage />);
    fireEvent.click(screen.getByRole('button', { name: 'Revisar datos de prueba' }));
    expect(await screen.findByText('No hay datos de prueba.')).toBeTruthy();
    expect(screen.queryByRole('button', { name: 'Quitar datos de prueba' })).toBeNull();
  });

  it('la confirmación es roja, dice que primero se respalda y Conservar no quita nada', async () => {
    render(<MaintenancePage />);
    fireEvent.click(screen.getByRole('button', { name: 'Revisar datos de prueba' }));
    fireEvent.click(await screen.findByRole('button', { name: 'Quitar datos de prueba' }));
    const dialog = screen.getByRole('alertdialog', { name: 'Quitar datos de prueba' });
    expect(dialog.className).toContain('dialog-panel-danger');
    expect(dialog.textContent).toContain('Primero se crea un respaldo de la base de datos');
    expect(dialog.textContent).toContain(
      '1 dato se elimina, 2 datos se desactivan, 2 ajustes se desvinculan',
    );
    expect(dialog.textContent).toContain('no se tocan');
    fireEvent.click(within(dialog).getByRole('button', { name: 'Conservar' }));
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull());
    expect(requests('maintenance_remove_test_data')).toHaveLength(0);
  });

  it('al confirmar envía el plan revisado y avisa lo quitado con el nombre del respaldo', async () => {
    render(<MaintenancePage />);
    fireEvent.click(screen.getByRole('button', { name: 'Revisar datos de prueba' }));
    fireEvent.click(await screen.findByRole('button', { name: 'Quitar datos de prueba' }));
    const dialog = screen.getByRole('alertdialog', { name: 'Quitar datos de prueba' });
    const confirm = within(dialog).getByRole('button', { name: 'Quitar datos de prueba' });
    expect(confirm.className).toContain('danger-solid-button');
    fireEvent.click(confirm);
    const notice = await screen.findByRole('status', { name: 'Operación completada' });
    expect(notice.textContent).toContain('Datos de prueba quitados.');
    expect(notice.textContent).toContain(
      '1 dato eliminado, 2 datos desactivados, 2 ajustes desvinculados.',
    );
    expect(notice.textContent).toContain('Respaldo previo: adel_pos-20261113T183005Z-0a1b2c.dump');
    expect(notice.textContent).not.toMatch(/[/\\]/);
    const body = requests('maintenance_remove_test_data')[0]?.body;
    expect(body?.plan_id).toBe(planID);
    expect(String(body?.operation_id)).toMatch(/^[0-9a-f-]{36}$/);
    // Después vuelve a revisar: ya no hay nada que quitar y queda a la vista el respaldo.
    expect(await screen.findByText('No hay datos de prueba.')).toBeTruthy();
    expect(
      screen.getByText(/Respaldo creado antes de quitar los datos de prueba/).textContent,
    ).toContain('adel_pos-20261113T183005Z-0a1b2c.dump');
  });

  it('si el respaldo falla, el error lo dice y la lista sigue a la vista', async () => {
    removal = {
      status: 503,
      body: {
        error: {
          code: 'BACKUP_FAILED',
          message:
            'No se pudo crear el respaldo de la base de datos; no se quitó ningún dato de prueba.',
        },
      },
    };
    render(<MaintenancePage />);
    fireEvent.click(screen.getByRole('button', { name: 'Revisar datos de prueba' }));
    fireEvent.click(await screen.findByRole('button', { name: 'Quitar datos de prueba' }));
    const dialog = screen.getByRole('alertdialog', { name: 'Quitar datos de prueba' });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Quitar datos de prueba' }));
    const alert = await screen.findByRole('alert', { name: 'Error' });
    expect(alert.textContent).toContain('no se quitó ningún dato de prueba');
    expect(alert.textContent).not.toContain('BACKUP_FAILED');
    expect(screen.getByRole('list', { name: 'Se eliminará' })).toBeTruthy();
  });

  it('una respuesta sin conexión conserva el operation_id al reintentar', async () => {
    let attempt = 0;
    native.invoke.mockImplementation(async (command: string, args: { operation: string }) => {
      const stage9g = stage9gReply(command, args?.operation);
      if (stage9g) {
        return stage9g;
      }
      if (args.operation === 'maintenance_test_data') {
        return { status: 200, body: withTestData };
      }
      attempt += 1;
      if (attempt === 1) {
        throw 'No se pudo conectar con el servidor.';
      }
      return { status: 200, body: result };
    });
    render(<MaintenancePage />);
    fireEvent.click(screen.getByRole('button', { name: 'Revisar datos de prueba' }));
    for (let round = 0; round < 2; round += 1) {
      const button = await screen.findByRole('button', { name: 'Quitar datos de prueba' });
      await waitFor(() => expect((button as HTMLButtonElement).disabled).toBe(false));
      fireEvent.click(button);
      const dialog = screen.getByRole('alertdialog', { name: 'Quitar datos de prueba' });
      fireEvent.click(within(dialog).getByRole('button', { name: 'Quitar datos de prueba' }));
      await waitFor(() => expect(requests('maintenance_remove_test_data')).toHaveLength(round + 1));
      await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull());
    }
    const [first, second] = requests('maintenance_remove_test_data');
    expect(second?.body?.operation_id).toBe(first?.body?.operation_id);
  });
});
