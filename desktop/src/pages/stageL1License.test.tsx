// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { licenseCapability } from '../services/license';
import type { LicenseStatus, LicenseSummary } from '../services/license';
import type { Device } from '../services/models';
import { rememberServerCapabilities } from '../services/serverCapabilities';
import { DevicesPage } from './DevicesPage';
import { PreparationSeat } from './kitchen/PreparationSeat';
import { ClockBlockPage } from './license/ClockBlockPage';
import { LicensePage } from './license/LicensePage';

// Etapa L1 (Fase E, §7 de la especificación): Configuración › Licencia, la activación (en línea, la transferencia y sin
// internet), la pantalla de la fecha y hora, los cupos en Terminales y el aviso de Preparación. Solo se sustituye el
// transporte nativo; los textos salen de services/license.ts.

const native = vi.hoisted(() => ({ invoke: vi.fn() }));
vi.mock('@tauri-apps/api/core', () => ({ invoke: native.invoke, isTauri: () => true }));

// Sin jest-dom: el texto visible y el estado deshabilitado se leen del DOM.
function textOf(element: Element) {
  return (element.textContent ?? '').replace(/\s+/g, ' ');
}

function disabled(element: Element) {
  return (element as HTMLButtonElement).disabled;
}

type Reply = { status: number; body: unknown };
type Call = { command: string; args: Record<string, unknown> };

let replies: Record<string, Reply | ((args: Record<string, unknown>) => Reply)>;
let calls: Call[];

function sent(operation: string) {
  return calls.filter(
    (call) =>
      (call.command === 'api_request' && call.args.operation === operation) ||
      call.command === operation,
  );
}

const perpetual: LicenseStatus = {
  state: 'active',
  environment: 'test',
  copied: false,
  machine_unverified: false,
  identity_unavailable: false,
  revocation_reason: null,
  license: {
    license_id: '7a0c5a4e-0000-4000-8000-000000000001',
    activation_id: '7a0c5a4e-0000-4000-8000-000000000002',
    licensee_name: 'Restaurante de prueba',
    license_type: 'perpetual',
    billing_period: null,
    license_revision: 1,
    plan: { plan_id: 'elemental', name: 'Plan elemental', custom: false },
    limits: { cash_registers: 3, order_takers: 2, kitchen_displays: 1 },
    features: {},
    issued_at: '2026-11-23T16:00:00Z',
    period_starts_at: '2026-11-23T16:00:00Z',
    updates_until: '2027-11-23T16:00:00Z',
    perpetual: true,
    expires_at: null,
    grace_days: 0,
    grace_ends_at: null,
  },
  server_id: '00000000-0000-4000-8000-0000000000aa',
  effective_now: '2026-11-23T16:15:00Z',
  can_manage: true,
  identity_regeneration_available: false,
};

const notActivated: LicenseStatus = {
  ...perpetual,
  state: 'not_activated',
  license: null,
};

function error(code: string, details: unknown = {}, status = 409): Reply {
  return { status, body: { error: { code, message: 'mensaje de Go que no se muestra', details } } };
}

beforeEach(() => {
  rememberServerCapabilities(['restaurant-operations', licenseCapability]);
  calls = [];
  replies = {
    license_status: { status: 200, body: perpetual },
    devices_sessions_summary: {
      status: 200,
      body: {
        by_kind: { desktop: 2, android: 1, kitchen: 0 },
        total: 3,
        limited: true,
        seats: {
          cash_registers: { in_use: 1, limit: 3 },
          order_takers: { in_use: 2, limit: 2 },
          kitchen_displays: { in_use: 0, limit: 1 },
        },
      },
    },
  };
  native.invoke.mockImplementation(async (command: string, args: Record<string, unknown> = {}) => {
    calls.push({ command, args });
    const key = command === 'api_request' ? String(args.operation) : command;
    const reply = replies[key];
    if (typeof reply === 'function') return reply(args);
    if (reply) return reply;
    return { status: 200, body: [] };
  });
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  rememberServerCapabilities([]);
});

function renderLicense(onChanged = vi.fn(async () => undefined)) {
  render(
    <LicensePage timezone="America/Monterrey" mainComputer canReadDevices onChanged={onChanged} />,
  );
  return onChanged;
}

describe('Configuración › Licencia', () => {
  it('muestra estado, plan, cliente, vencimiento, tolerancia, entorno y cupos en uso', async () => {
    renderLicense();
    const details = await screen.findByRole('region', { name: 'Estado de la licencia' });
    expect(textOf(details)).toContain('Vigente');
    expect(textOf(details)).toContain('Plan elemental');
    expect(textOf(details)).toContain('Restaurante de prueba');
    expect(textOf(details)).toContain('Perpetua');
    expect(textOf(details)).toContain('No aplica');
    // El entorno solo en las versiones de desarrollo.
    expect(textOf(details)).toContain('Servidor de licencias de pruebas');
    const seats = within(details).getByRole('table', { name: 'Cupos de la licencia' });
    await waitFor(() => expect(textOf(seats)).toContain('Comanderos22'));
    expect(textOf(seats)).toContain('Cajas13');
    expect(textOf(seats)).toContain('Pantallas de cocina01');
    // Ningún identificador a la vista.
    expect(document.body.textContent).not.toContain(perpetual.license!.license_id);
    expect(document.body.textContent).not.toContain(perpetual.server_id!);
  });

  it('sin «Administrar la licencia» no hay botones, solo el estado', async () => {
    replies.license_status = { status: 200, body: { ...perpetual, can_manage: false } };
    renderLicense();
    await screen.findByRole('region', { name: 'Estado de la licencia' });
    expect(screen.queryByRole('button', { name: /Buscar actualización/ })).toBeNull();
    expect(screen.queryByRole('button', { name: /Liberar esta licencia/ })).toBeNull();
    expect(screen.getByText(/requiere el permiso «Administrar la licencia»/)).toBeTruthy();
  });

  it('«Buscar actualización» y «Liberar esta licencia» con su confirmación', async () => {
    replies.license_refresh = { status: 200, body: perpetual };
    replies.license_deactivate = {
      status: 200,
      body: { ...perpetual, state: 'revoked', revocation_reason: 'deactivated' },
    };
    const onChanged = renderLicense();
    fireEvent.click(await screen.findByRole('button', { name: /Buscar actualización/ }));
    await waitFor(() => expect(sent('license_refresh')).toHaveLength(1));
    await waitFor(() => expect(onChanged).toHaveBeenCalled());

    const release = screen.getByRole('button', { name: 'Liberar esta licencia' });
    expect(release.className).toContain('danger-button');
    fireEvent.click(release);
    const dialog = await screen.findByRole('alertdialog', { name: 'Liberar esta licencia' });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Conservar la licencia' }));
    expect(sent('license_deactivate')).toHaveLength(0);
    fireEvent.click(release);
    fireEvent.click(
      within(await screen.findByRole('alertdialog')).getByRole('button', {
        name: 'Liberar esta licencia',
      }),
    );
    await waitFor(() => expect(sent('license_deactivate')).toHaveLength(1));
    expect(sent('license_deactivate')[0]!.args.body).toEqual({ confirmed: true });
  });

  it('«Solicitud sin internet» e «Importar archivo de licencia» van por Rust, sin rutas ni contenido', async () => {
    replies.license_save_request = {
      status: 200,
      body: { result: 'saved', file_name: 'solicitud-renovar.licreq' },
    };
    replies.license_import_file = { status: 200, body: { result: 'imported', license: perpetual } };
    renderLicense();
    const offline = await screen.findByRole('group', { name: 'Solicitud sin internet' });
    // Con una activación no se pide otra activación.
    expect(disabled(within(offline).getByRole('button', { name: 'Activación' }))).toBe(true);
    fireEvent.click(within(offline).getByRole('button', { name: 'Liberación' }));
    fireEvent.click(within(offline).getByRole('button', { name: /Guardar la solicitud/ }));
    await waitFor(() => expect(sent('license_save_request')).toHaveLength(1));
    expect(sent('license_save_request')[0]!.args).toEqual({ action: 'deactivate' });
    await screen.findByText('Solicitud guardada: solicitud-renovar.licreq.');

    fireEvent.click(screen.getByRole('button', { name: /Importar archivo de licencia/ }));
    await waitFor(() => expect(sent('license_import_file')).toHaveLength(1));
    expect(sent('license_import_file')[0]!.args).toEqual({});
    await screen.findByText('Archivo de licencia importado.');
    // React nunca pide estas operaciones como rutas.
    expect(
      calls.some(
        (call) =>
          call.command === 'api_request' && /offline|import/.test(String(call.args.operation)),
      ),
    ).toBe(false);
  });

  it('un archivo de otra computadora se explica por su motivo', async () => {
    replies.license_import_file = error(
      'LICENSE_FILE_INVALID',
      { reason: 'other_installation' },
      400,
    );
    renderLicense();
    fireEvent.click(await screen.findByRole('button', { name: /Importar archivo de licencia/ }));
    await screen.findByText(
      'El archivo de licencia es de otra computadora y no se instaló. Tu licencia actual no cambió.',
    );
    expect(screen.queryByText('mensaje de Go que no se muestra')).toBeNull();
  });
});

describe('activación', () => {
  it('valida el formato al escribir, activa y avisa', async () => {
    replies.license_status = { status: 200, body: notActivated };
    replies.license_activate = { status: 200, body: perpetual };
    const onChanged = renderLicense();
    const field = await screen.findByLabelText('Código de la licencia');
    const activate = screen.getByRole('button', { name: 'Activar' });
    expect(disabled(activate)).toBe(true);
    fireEvent.change(field, { target: { value: 'prp2222' } });
    expect((field as HTMLInputElement).value).toBe('PRP-2222');
    expect(screen.getByText('Faltan 8 caracteres.')).toBeTruthy();
    fireEvent.change(field, { target: { value: 'PRP-2222-2222-22O2' } });
    expect(screen.getByText(/no usa O/)).toBeTruthy();
    fireEvent.change(field, { target: { value: 'prp222222222222' } });
    expect((field as HTMLInputElement).value).toBe('PRP-2222-2222-2222');
    expect(screen.getByText('El código tiene el formato correcto.')).toBeTruthy();
    fireEvent.click(activate);
    await waitFor(() => expect(sent('license_activate')).toHaveLength(1));
    expect(sent('license_activate')[0]!.args.body).toEqual({ license_key: 'PRP-2222-2222-2222' });
    await screen.findByText('Licencia activada.');
    expect(onChanged).toHaveBeenCalled();
  });

  it('«Transferir a esta computadora» solo aparece con transfer_available y pide confirmación', async () => {
    replies.license_status = { status: 200, body: notActivated };
    replies.license_activate = error('LICENSE_ACTIVATION_LIMIT', { transfer_available: true });
    replies.license_transfer = { status: 200, body: perpetual };
    renderLicense();
    expect(screen.queryByRole('button', { name: /Transferir a esta computadora/ })).toBeNull();
    fireEvent.change(await screen.findByLabelText('Código de la licencia'), {
      target: { value: 'ACT-2222-2222-2222' },
    });
    fireEvent.click(screen.getByRole('button', { name: 'Activar' }));
    const offer = await screen.findByRole('region', { name: 'Transferir la licencia' });
    fireEvent.click(within(offer).getByRole('button', { name: /Transferir a esta computadora/ }));
    const dialog = await screen.findByRole('alertdialog', {
      name: 'Transferir la licencia a esta computadora',
    });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Transferir a esta computadora' }));
    await waitFor(() => expect(sent('license_transfer')).toHaveLength(1));
    expect(sent('license_transfer')[0]!.args.body).toEqual({
      license_key: 'ACT-2222-2222-2222',
      confirmed: true,
    });
    await screen.findByText('Licencia transferida a esta computadora.');
  });

  it('sin transferencia disponible o con TRANSFER_LIMIT: «Comunícate con soporte»', async () => {
    replies.license_status = { status: 200, body: notActivated };
    replies.license_activate = error('LICENSE_ACTIVATION_LIMIT', { transfer_available: false });
    renderLicense();
    fireEvent.change(await screen.findByLabelText('Código de la licencia'), {
      target: { value: 'ACN-2222-2222-2222' },
    });
    fireEvent.click(screen.getByRole('button', { name: 'Activar' }));
    await screen.findByText(/tiene una transferencia pendiente. Comunícate con soporte./);
    expect(screen.queryByRole('region', { name: 'Transferir la licencia' })).toBeNull();
  });

  it('«Activar sin internet» guarda la solicitud con el código si es válido', async () => {
    replies.license_status = { status: 200, body: notActivated };
    replies.license_save_request = {
      status: 200,
      body: { result: 'saved', file_name: 'solicitud-activar.licreq' },
    };
    renderLicense();
    fireEvent.change(await screen.findByLabelText('Código de la licencia'), {
      target: { value: 'PRP-2222-2222-2222' },
    });
    fireEvent.click(screen.getByRole('button', { name: /Activar sin internet/ }));
    const activation = screen.getByRole('region', { name: 'Activar la licencia' });
    fireEvent.click(within(activation).getByRole('button', { name: /Guardar la solicitud/ }));
    await waitFor(() => expect(sent('license_save_request')).toHaveLength(1));
    expect(sent('license_save_request')[0]!.args).toEqual({
      action: 'activate',
      licenseKey: 'PRP-2222-2222-2222',
    });
  });

  it('una instalación copiada explica la identidad nueva y la pide con confirmación', async () => {
    replies.license_status = {
      status: 200,
      body: {
        ...notActivated,
        state: 'not_activated_with_data',
        copied: true,
        identity_regeneration_available: true,
      },
    };
    replies.license_identity_regenerate = {
      status: 200,
      body: { server_id: 'x', revoked_terminals: 2, restart_required: true, status: notActivated },
    };
    replies.maintenance_service = { status: 200, body: { restart_available: true } };
    renderLicense();
    const activation = await screen.findByRole('region', { name: 'Activar la licencia' });
    expect(textOf(activation)).toContain(
      'Esta instalación se copió de otra computadora. Actívala o transfiere la licencia.',
    );
    fireEvent.click(
      within(activation).getByRole('button', {
        name: 'Crear una identidad nueva para esta computadora',
      }),
    );
    const dialog = await screen.findByRole('alertdialog', {
      name: 'Crear una identidad nueva para esta computadora',
    });
    expect(textOf(dialog)).toContain('No se borra ni cambia ningún dato del restaurante.');
    expect(textOf(dialog)).toContain('hay que emparejarlos de nuevo');
    fireEvent.click(within(dialog).getByRole('button', { name: 'Crear la identidad nueva' }));
    await waitFor(() => expect(sent('license_identity_regenerate')).toHaveLength(1));
    expect(sent('license_identity_regenerate')[0]!.args.body).toEqual({ confirmed: true });
    // Después se pide reiniciar el servidor.
    await screen.findByRole('button', { name: 'Reiniciar el servidor' });
  });

  it('sin el permiso o fuera de la computadora principal no se ofrece la identidad nueva', async () => {
    replies.license_status = {
      status: 200,
      body: { ...notActivated, copied: true, identity_regeneration_available: false },
    };
    renderLicense();
    const activation = await screen.findByRole('region', { name: 'Activar la licencia' });
    expect(within(activation).queryByRole('button', { name: /identidad nueva/ })).toBeNull();
  });
});

const blockedSummary: LicenseSummary = {
  state: 'active',
  environment: 'test',
  copied: false,
  machine_unverified: false,
  revocation_reason: null,
  license_type: 'perpetual',
  perpetual: true,
  expires_at: null,
  grace_days: 0,
  grace_ends_at: null,
  days_until_expiry: null,
  grace_days_remaining: null,
  clock: {
    blocked: true,
    local_time: '2026-11-23T16:15:00Z',
    last_activity_time: '2026-11-23T18:40:00Z',
    margin_minutes: 10,
    timezone: 'America/Monterrey',
    local_time_text: 'lunes 23 nov 2026, 10:15',
    last_activity_time_text: 'lunes 23 nov 2026, 12:40',
    license_state: 'active',
    license_ok: true,
  },
};

function renderClock(license: LicenseSummary, onChanged = vi.fn(async () => undefined)) {
  render(
    <ClockBlockPage
      license={license}
      canOpenReports
      canOpenBackups
      onOpenReports={vi.fn()}
      onOpenBackups={vi.fn()}
      onOpenLicense={vi.fn()}
      onHome={vi.fn()}
      onChanged={onChanged}
    />,
  );
  return onChanged;
}

describe('Sistema detenido por la fecha y hora de la computadora (§6.2)', () => {
  it('tiene sus cinco partes, con las dos fechas completas', () => {
    renderClock(blockedSummary);
    expect(
      screen.getByRole('heading', {
        name: 'Sistema detenido por la fecha y hora de la computadora',
      }),
    ).toBeTruthy();
    expect(textOf(screen.getByRole('group', { name: 'Qué pasó' }))).toContain(
      'La fecha y hora de esta computadora es lunes 23 nov 2026, 10:15. La última actividad registrada en el sistema fue el lunes 23 nov 2026, 12:40. La hora de la computadora no puede ser anterior a lo que ya se registró.',
    );
    expect(textOf(screen.getByRole('group', { name: 'Por qué se detiene' }))).toContain(
      'Para proteger tus cortes y reportes',
    );
    expect(textOf(screen.getByRole('group', { name: 'Qué sí puedes hacer' }))).toContain(
      'Consultar reportes e historial y hacer respaldos.',
    );
    const how = screen.getByRole('group', { name: 'Cómo resolverlo' });
    expect(textOf(how)).toContain('Corrige la fecha y hora de la computadora.');
    expect(textOf(how)).toContain('En Mac:');
    expect(textOf(how)).toContain('En Windows:');
    expect(
      within(how).getByRole('button', { name: /Comprobar la hora con internet/ }),
    ).toBeTruthy();
    expect(within(how).getByRole('button', { name: /Desbloqueo de soporte/ })).toBeTruthy();
    expect(textOf(screen.getByRole('note'))).toContain(
      'Tu licencia está bien. Este bloqueo es solo por la fecha y hora.',
    );
  });

  it('con un problema de licencia, los dos se muestran por separado', () => {
    renderClock({
      ...blockedSummary,
      state: 'expired',
      clock: { ...blockedSummary.clock, license_state: 'expired', license_ok: false },
    });
    expect(screen.queryByRole('note')).toBeNull();
    expect(textOf(screen.getByRole('group', { name: /Además, la licencia: Vencida/ }))).toContain(
      'corregir el reloj no lo resuelve',
    );
  });

  it('«Comprobar la hora con internet» muestra la hora de internet si no coincide', async () => {
    replies.license_time_check = error('LICENSE_TIME_MISMATCH', {
      server_time_text: 'lunes 23 nov 2026, 12:45',
    });
    renderClock(blockedSummary);
    fireEvent.click(screen.getByRole('button', { name: /Comprobar la hora con internet/ }));
    expect(textOf(await screen.findByRole('alert'))).toContain(
      'Corrígela a lunes 23 nov 2026, 12:45 y vuelve a comprobar.',
    );
  });

  it('el desbloqueo de soporte muestra el código, acepta el texto pegado o el archivo', async () => {
    replies.license_support_request = {
      status: 200,
      body: {
        request_code: '3RF0-HBRQ-2082-0C20-A1SE-E1SR',
        expires_at: '2026-11-24T16:15:00Z',
        expires_at_text: 'martes 24 nov 2026, 10:15',
        last_activity_time_text: 'lunes 23 nov 2026, 12:40',
        reused: false,
      },
    };
    replies.license_support_unlock = error('SUPPORT_UNLOCK_INVALID', { reason: 'altered' }, 400);
    replies.license_import_unlock_file = {
      status: 200,
      body: { result: 'unlocked', unlock: { accepted: true, clock: { blocked: false } } },
    };
    const onChanged = renderClock(blockedSummary);
    fireEvent.click(screen.getByRole('button', { name: /Desbloqueo de soporte/ }));
    const panel = await screen.findByRole('region', { name: 'Desbloqueo de soporte' });
    await waitFor(() =>
      expect(textOf(within(panel).getByLabelText('Código de solicitud'))).toContain(
        '3RF0HBRQ20820C20A1SEE1SR',
      ),
    );
    expect(textOf(panel)).toContain('Vale hasta el martes 24 nov 2026, 10:15');
    const unlock = within(panel).getByRole('button', { name: /Desbloquear/ });
    expect(disabled(unlock)).toBe(true);
    fireEvent.change(within(panel).getByLabelText('Respuesta de soporte'), {
      target: { value: 'eyJ.alterada.firma' },
    });
    fireEvent.click(unlock);
    await screen.findByText(
      'La respuesta de soporte no es válida. Revisa que se haya copiado completa.',
    );
    expect(sent('license_support_unlock')[0]!.args.body).toEqual({
      response: 'eyJ.alterada.firma',
      source: 'text',
    });
    fireEvent.click(within(panel).getByRole('button', { name: /Importar la respuesta/ }));
    await screen.findByText('Se aplicó la respuesta de soporte: el sistema se desbloqueó.');
    expect(onChanged).toHaveBeenCalled();
  });

  it('cuando el reloj se corrige lo dice y vuelve al inicio', () => {
    renderClock({ ...blockedSummary, clock: { ...blockedSummary.clock, blocked: false } });
    expect(
      screen.getByRole('heading', { name: 'La fecha y hora de la computadora es correcta' }),
    ).toBeTruthy();
  });
});

const tablet: Device = {
  id: '00000000-0000-4000-8000-0000000000b1',
  name: 'Tableta 1',
  register_id: null,
  keyboard_mode: 'disabled',
  home_screen: 'tables',
  kind: 'android',
  online: true,
  current_user: 'Mesero',
  seats: ['order_takers'],
};
const cashier: Device = {
  ...tablet,
  id: '00000000-0000-4000-8000-0000000000b2',
  name: 'Caja principal',
  kind: 'desktop',
  seats: ['cash_registers'],
};

describe('Terminales con los cupos', () => {
  beforeEach(() => {
    replies.devices = { status: 200, body: [tablet, cashier] };
    replies.devices_pairing_requests = {
      status: 200,
      body: { requests: [], approve_here: true, pairing_available: true },
    };
    replies.devices_seat_release = {
      status: 200,
      body: { released: ['order_takers'], sessions_closed: 1 },
    };
  });

  it('el resumen de cupos reemplaza al de sesiones por tipo y cada terminal dice qué cupo ocupa', async () => {
    render(<DevicesPage can={() => true} />);
    const seats = await screen.findByRole('region', { name: 'Cupos de la licencia' });
    expect(textOf(seats)).toContain('Comanderos2 de 2');
    expect(
      screen.queryByRole('region', { name: 'Sesiones activas por tipo de terminal' }),
    ).toBeNull();
    const row = (await screen.findByText('Tableta 1')).closest('tr')!;
    expect(textOf(row)).toContain('Comandero');
    const cashRow = screen.getByText('Caja principal').closest('tr')!;
    expect(textOf(cashRow)).toContain('Caja');
    // La caja se libera cerrando el turno: sin «Liberar cupo».
    expect(within(cashRow).queryByRole('button', { name: 'Liberar cupo' })).toBeNull();
    fireEvent.click(within(row).getByRole('button', { name: 'Liberar cupo' }));
    const dialog = await screen.findByRole('alertdialog', { name: 'Liberar el cupo de Tableta 1' });
    expect(textOf(dialog)).toContain('Se cierra la sesión de quien esté usando esta terminal');
    fireEvent.click(within(dialog).getByRole('button', { name: 'Liberar cupo' }));
    await waitFor(() => expect(sent('devices_seat_release')).toHaveLength(1));
    expect(sent('devices_seat_release')[0]!.args.resourceId).toBe(tablet.id);
    await screen.findByText('Cupo de Tableta 1 liberado.');
  });

  it('con un servidor anterior se ve como antes', async () => {
    rememberServerCapabilities(['restaurant-operations']);
    replies.devices_sessions_summary = {
      status: 200,
      body: { by_kind: { desktop: 2, android: 1, kitchen: 0 }, total: 3, limited: false },
    };
    render(<DevicesPage can={() => true} />);
    await screen.findByRole('region', { name: 'Sesiones activas por tipo de terminal' });
    expect(screen.queryByRole('button', { name: 'Liberar cupo' })).toBeNull();
    expect(screen.queryByRole('columnheader', { name: 'Cupo' })).toBeNull();
  });
});

describe('Preparación y el cupo de cocina', () => {
  it('avisa al entrar y al salir', async () => {
    replies.kitchen_display_mode = (args) => ({
      status: 200,
      body: { active: (args.body as { active: boolean }).active },
    });
    const view = render(
      <PreparationSeat>
        <p>Tablero de preparación</p>
      </PreparationSeat>,
    );
    await screen.findByText('Tablero de preparación');
    expect(sent('kitchen_display_mode').map((call) => call.args.body)).toEqual([{ active: true }]);
    view.unmount();
    await waitFor(() =>
      expect(sent('kitchen_display_mode').map((call) => call.args.body)).toEqual([
        { active: true },
        { active: false },
      ]),
    );
  });

  it('sin cupo de cocina lo dice con el cupo y sus cifras, sin el tablero', async () => {
    replies.kitchen_display_mode = error('LICENSE_SEAT_UNAVAILABLE', {
      seat: 'kitchen_displays',
      in_use: 1,
      limit: 1,
    });
    render(
      <PreparationSeat>
        <p>Tablero de preparación</p>
      </PreparationSeat>,
    );
    expect(textOf(await screen.findByRole('alert'))).toContain(
      'No hay cupo de pantalla de cocina libre (1 de 1 en uso). Pide que otra terminal salga de Preparación.',
    );
    expect(screen.queryByText('Tablero de preparación')).toBeNull();
    replies.kitchen_display_mode = { status: 200, body: { active: true } };
    fireEvent.click(screen.getByRole('button', { name: 'Volver a intentar' }));
    await screen.findByText('Tablero de preparación');
  });

  it('con un servidor anterior no avisa nada', async () => {
    rememberServerCapabilities(['restaurant-operations']);
    render(
      <PreparationSeat>
        <p>Tablero de preparación</p>
      </PreparationSeat>,
    );
    await screen.findByText('Tablero de preparación');
    expect(sent('kitchen_display_mode')).toHaveLength(0);
  });
});
