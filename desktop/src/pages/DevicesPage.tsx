import { useCallback, useEffect, useState } from 'react';
import { useConfirmDialog } from '../components/ConfirmDialog';
import { dateTime, Field, Form, Notice, PageTitle } from '../components/Forms';
import { SegmentedControl } from '../components/SegmentedControl';
import { useResource } from '../hooks/useResource';
import { terminalHomes } from '../navigation/sections';
import type { TerminalHome } from '../navigation/sections';
import { api } from '../services/api';
import { notifyError, notifySuccess } from '../services/notify';
import { countLabel } from '../services/plural';
import type {
  Device,
  DeviceConfiguration,
  DeviceKind,
  PairingRequest,
  PairingRequests,
  Register,
  SessionsSummary,
} from '../services/models';
import { Select } from '../components/Select';

// Etapa 9G (fase B, §3.2–§3.4): Sistema › Terminales también recibe las terminales nuevas que piden su
// código (se aprueban escribiendo el código que muestra la terminal, solo en la computadora principal),
// muestra el tipo, el sistema, el usuario actual y si cada terminal está en línea, su configuración y las
// sesiones activas por tipo de dispositivo (solo se cuentan).

// Cada cuánto se vuelven a consultar las solicitudes pendientes y la lista (estado en línea).
export const pairingRefreshMilliseconds = 3000;
export const devicesRefreshMilliseconds = 5000;

export const deviceKindLabels: Record<DeviceKind, string> = {
  desktop: 'Escritorio',
  android: 'Comandero Android',
  kitchen: 'Cocina',
};

const keyboardLabels: Record<Device['keyboard_mode'], string> = {
  auto: 'Automático',
  always: 'Siempre habilitado',
  disabled: 'Desactivado',
};

function clock(value: string) {
  return new Date(value).toLocaleTimeString('es-MX', { hour: '2-digit', minute: '2-digit' });
}

// Solo los dígitos del código (se puede escribir «123 456»).
function codeDigits(value: string) {
  return value.replace(/\D/g, '').slice(0, 6);
}

function PairingRequestItem({
  request,
  approveHere,
  canManage,
  onChanged,
}: {
  request: PairingRequest;
  approveHere: boolean;
  canManage: boolean;
  onChanged: () => Promise<void>;
}) {
  const [code, setCode] = useState('');
  const [busy, setBusy] = useState(false);
  const { confirm, dialog } = useConfirmDialog();
  const digits = codeDigits(code);

  async function approve() {
    setBusy(true);
    try {
      await api('approve_device_pairing', { id: request.id, body: { code: digits } });
      notifySuccess(`Terminal ${request.name} aprobada. Se conectará en unos segundos.`);
      setCode('');
    } catch (error) {
      notifyError(error, 'No se pudo aprobar la terminal.');
      setCode('');
    } finally {
      setBusy(false);
      await onChanged();
    }
  }

  async function reject() {
    const confirmed = await confirm({
      title: `Rechazar ${request.name}`,
      message: 'Esta terminal no se conectará. Si fue un error, puede pedir otro código.',
      confirmLabel: 'Rechazar terminal',
      cancelLabel: 'Volver',
    });
    if (!confirmed) {
      return;
    }
    setBusy(true);
    try {
      await api('reject_device_pairing', { id: request.id, body: {} });
      notifySuccess(`Se rechazó la terminal ${request.name}.`);
    } catch (error) {
      notifyError(error, 'No se pudo rechazar la terminal.');
    } finally {
      setBusy(false);
      await onChanged();
    }
  }

  return (
    <li className="pairing-request" aria-busy={busy}>
      <div className="pairing-request-summary">
        <strong>{request.name}</strong>
        <small className="muted">
          {deviceKindLabels[request.device_kind]} · {request.os} · versión {request.app_version} ·
          pidió su código a las {clock(request.created_at)}
        </small>
      </div>
      {approveHere && canManage && (
        <form
          className="pairing-request-approval"
          onSubmit={(event) => {
            event.preventDefault();
            void approve();
          }}
        >
          <Field
            label={`Código que muestra ${request.name}`}
            hint={
              request.attempts_left < 5
                ? `Quedan ${request.attempts_left} intentos antes de rechazarla.`
                : 'Escribe los 6 números que aparecen en la pantalla de la terminal.'
            }
          >
            <input
              inputMode="numeric"
              autoComplete="off"
              maxLength={7}
              value={code}
              disabled={busy || !request.ready}
              onChange={(event) => setCode(event.target.value)}
            />
          </Field>
          <div className="button-row">
            <button
              type="submit"
              className="primary-button"
              disabled={busy || !request.ready || digits.length !== 6}
            >
              Aprobar
            </button>
            <button
              type="button"
              className="secondary-button"
              disabled={busy}
              onClick={() => void reject()}
            >
              Rechazar
            </button>
          </div>
        </form>
      )}
      {dialog}
    </li>
  );
}

// Dirección de esta computadora para escribirla en una terminal nueva (sin el puerto si es el de
// siempre).
function pairingAddresses(network: NonNullable<PairingRequests['network']>) {
  return network.addresses.map((address) =>
    network.port === 8443 || address.endsWith('.local') ? address : `${address}:${network.port}`,
  );
}

function PairingRequestsPanel({
  can,
  onChanged,
}: {
  can: (key: string) => boolean;
  onChanged: () => Promise<void>;
}) {
  const [data, setData] = useState<PairingRequests | null>(null);
  const [error, setError] = useState('');
  const load = useCallback(async () => {
    try {
      setData(await api<PairingRequests>('devices_pairing_requests'));
      setError('');
    } catch (problem) {
      setError(
        problem instanceof Error ? problem.message : 'No se pudieron consultar las solicitudes.',
      );
    }
  }, []);
  useEffect(() => {
    void load();
    const timer = window.setInterval(() => void load(), pairingRefreshMilliseconds);
    return () => window.clearInterval(timer);
  }, [load]);
  const requests = data?.requests ?? [];
  const network = data?.network;
  const addresses = network ? pairingAddresses(network) : [];
  return (
    <section className="surface panel pairing-requests" aria-labelledby="pairing-requests-title">
      <h2 id="pairing-requests-title">Terminales nuevas</h2>
      <Notice>{error}</Notice>
      {data && !data.approve_here && (
        <p className="muted pairing-help">
          Las terminales nuevas se aprueban en la computadora principal del restaurante.
        </p>
      )}
      {network && network.available && addresses.length > 0 && (
        <p className="muted pairing-help">
          Para emparejar una terminal, abre AD-EL POS en ella, elige «Solo terminal» y escribe la
          dirección de esta computadora: <strong>{addresses.join(' o ')}</strong>.
        </p>
      )}
      {data?.pairing_available === false && (
        <p className="pairing-alert" role="alert">
          Esta computadora no puede emparejar terminales ahora: su identidad necesita revisión de
          soporte. Las terminales ya emparejadas tampoco pueden conectarse hasta resolverlo; aquí se
          sigue vendiendo con normalidad.
        </p>
      )}
      {data?.pairing_available !== false && network && !network.available && (
        <p className="muted pairing-help">
          Esta computadora todavía no recibe terminales de la red.
        </p>
      )}
      {data?.limit_reached && (
        <p className="pairing-alert">
          Se pidieron demasiados códigos de terminal en los últimos {data.window_minutes ?? 10}{' '}
          minutos: por ahora no se aceptan más solicitudes.
        </p>
      )}
      {(data?.recent_abandoned ?? 0) > 0 && (
        <p className="pairing-alert">
          En los últimos {data?.window_minutes ?? 10} minutos hubo{' '}
          {countLabel(
            data?.recent_abandoned ?? 0,
            'solicitud cancelada, rechazada o vencida',
            'solicitudes canceladas, rechazadas o vencidas',
          )}{' '}
          sin aprobarse. Si nadie del restaurante las pidió, alguien podría estar intentando hacerse
          pasar por una terminal: aprueba solo el código que veas en la pantalla de la terminal.
        </p>
      )}
      {data && requests.length === 0 && (
        <p className="muted pairing-help">No hay terminales esperando aprobación.</p>
      )}
      {requests.length > 0 && (
        <ul className="pairing-request-list">
          {requests.map((request) => (
            <PairingRequestItem
              key={request.id}
              request={request}
              approveHere={data?.approve_here ?? false}
              canManage={can('devices.manage')}
              onChanged={async () => {
                await load();
                await onChanged();
              }}
            />
          ))}
        </ul>
      )}
    </section>
  );
}

// Sesiones activas por tipo de dispositivo (§3.4, D9): solo se cuentan para las licencias futuras.
function SessionsSummaryPanel() {
  const summary = useResource<SessionsSummary>('devices_sessions_summary');
  const counts = summary.data?.by_kind;
  if (!counts) {
    return null;
  }
  return (
    <section className="surface panel sessions-summary" aria-labelledby="sessions-summary-title">
      <h2 id="sessions-summary-title">Sesiones activas por tipo de terminal</h2>
      <dl className="sessions-summary-counts">
        {(Object.keys(deviceKindLabels) as DeviceKind[]).map((kind) => (
          <div key={kind}>
            <dt>{deviceKindLabels[kind]}</dt>
            <dd>{counts[kind] ?? 0}</dd>
          </div>
        ))}
      </dl>
      <p className="muted pairing-help">Solo se cuentan; todavía no se limita ninguna.</p>
    </section>
  );
}

function DeviceConfigurationPanel({ device, onClose }: { device: Device; onClose: () => void }) {
  const configuration = useResource<DeviceConfiguration>('device_configuration', {
    id: device.id,
  });
  const value = configuration.data;
  const shown = value?.device ?? device;
  return (
    <section
      className="surface panel device-configuration"
      aria-labelledby="device-configuration-title"
    >
      <h2 id="device-configuration-title">Configuración de {shown.name}</h2>
      <Notice>{configuration.error}</Notice>
      {value && (
        <dl className="device-configuration-list">
          <div>
            <dt>Tipo</dt>
            <dd>{deviceKindLabels[shown.kind ?? 'desktop']}</dd>
          </div>
          <div>
            <dt>Sistema</dt>
            <dd>
              {shown.os
                ? `${shown.os}${shown.app_version ? ` · versión ${shown.app_version}` : ''}`
                : 'Esta computadora'}
            </dd>
          </div>
          <div>
            <dt>Emparejada</dt>
            <dd>
              {shown.paired_at ? dateTime(shown.paired_at) : 'No (es la computadora principal)'}
            </dd>
          </div>
          <div>
            <dt>Pantalla inicial</dt>
            <dd>{homeLabel(shown)}</dd>
          </div>
          <div>
            <dt>Caja física</dt>
            <dd>{shown.register_name ?? 'Sin caja asociada'}</dd>
          </div>
          <div>
            <dt>Teclado virtual</dt>
            <dd>{keyboardLabels[shown.keyboard_mode]}</dd>
          </div>
          <div>
            <dt>Estaciones que muestra en Cocina</dt>
            <dd>
              {value.kitchen_all_stations || value.kitchen_stations.length === 0
                ? 'Todas'
                : value.kitchen_stations.map((station) => station.name).join(', ')}
            </dd>
          </div>
          <div>
            <dt>Aviso sonoro de comandas nuevas</dt>
            <dd>{value.kitchen_sound_alert ? 'Activado' : 'Desactivado'}</dd>
          </div>
          <div>
            <dt>Impresoras de esta terminal</dt>
            {/* Fase E: impresoras USB conectadas a la terminal (agente de impresión). */}
            <dd>
              {value.printers.length > 0
                ? value.printers.map((printer) => printer.name).join(', ')
                : 'Sin impresoras propias'}
            </dd>
          </div>
        </dl>
      )}
      <p className="muted pairing-help">
        Las estaciones y el aviso sonoro se eligen en Cocina › Estaciones › Terminales de cocina.
      </p>
      <button className="secondary-button mt-4" onClick={onClose}>
        Cerrar configuración
      </button>
    </section>
  );
}

function DeviceEditor({
  device,
  registers,
  reload,
  onSaved,
}: {
  device: Device;
  registers: Register[];
  reload: () => Promise<void>;
  onSaved?: () => Promise<void>;
}) {
  const [name, setName] = useState(device.name);
  const [registerID, setRegisterID] = useState(device.register_id ?? '');
  const [keyboard, setKeyboard] = useState(device.keyboard_mode);
  const [home, setHome] = useState<TerminalHome>(device.home_screen ?? 'tables');
  return (
    <section className="surface panel">
      <h2 className="mb-5">Editar {device.name}</h2>
      <Form
        successMessage="Terminal actualizada."
        onSave={async () => {
          await api('update_device', {
            id: device.id,
            body: {
              name,
              register_id: registerID || null,
              keyboard_mode: keyboard,
              home_screen: home,
            },
          });
          await reload();
          // La terminal actual vuelve a leer su identidad: el logotipo usa el inicio nuevo.
          await onSaved?.();
        }}
      >
        <div className="form-grid">
          <Field label="Nombre">
            <input
              required
              maxLength={80}
              value={name}
              onChange={(event) => setName(event.target.value)}
            />
          </Field>
          <Field label="Caja física">
            <Select
              value={registerID}
              options={[
                { value: '', label: 'Sin caja asociada' },
                ...registers
                  .filter((register) => register.active)
                  .map((register) => ({ value: register.id, label: register.name })),
              ]}
              onChange={setRegisterID}
            />
          </Field>
          <Field label="Teclado virtual">
            <Select<Device['keyboard_mode']>
              value={keyboard}
              options={[
                { value: 'auto', label: 'Automático' },
                { value: 'always', label: 'Siempre habilitado' },
                { value: 'disabled', label: 'Desactivado' },
              ]}
              onChange={setKeyboard}
            />
          </Field>
        </div>
        {/* Etapa 9C (§1.5): pantalla en la que abre la aplicación en esta terminal y a la que vuelve
            el logotipo. Lista cerrada validada en Go; quien no pueda ver esa sección abre en la
            primera de la barra que sí puede ver. */}
        <div className="field">
          <span>Pantalla inicial</span>
          <SegmentedControl<TerminalHome>
            label="Pantalla inicial de la terminal"
            value={home}
            options={terminalHomes}
            onChange={setHome}
          />
          <small className="muted">
            Si un usuario no puede ver esta sección, la aplicación abre en la primera que sí puede
            ver.
          </small>
          {/* Etapa 9F (§5.1): una terminal de cocina abre en Cocina › Preparación; sus estaciones y
              su aviso sonoro se eligen junto a las estaciones. */}
          <small className="muted">
            Con «Cocina» abre en Preparación. Las estaciones que muestra y el aviso sonoro se eligen
            en Cocina › Estaciones › Terminales de cocina.
          </small>
        </div>
      </Form>
    </section>
  );
}

function homeLabel(device: Device) {
  return terminalHomes.find((entry) => entry.value === (device.home_screen ?? 'tables'))?.label;
}

function connectionState(device: Device) {
  if (device.revoked_at) {
    return { label: 'Revocada', tone: 'neutral' };
  }
  return device.online
    ? { label: 'En línea', tone: 'good' }
    : { label: 'Desconectada', tone: 'neutral' };
}

export function DevicesPage({
  can,
  onSaved,
}: {
  can: (key: string) => boolean;
  // Vuelve a leer la identidad de esta terminal tras guardar (pantalla inicial nueva).
  onSaved?: () => Promise<void>;
}) {
  const devices = useResource<Device[]>('devices');
  const registers = useResource<Register[]>('registers', { enabled: can('registers.read') });
  const [editing, setEditing] = useState<Device | null>(null);
  const [viewing, setViewing] = useState<Device | null>(null);
  const [invite, setInvite] = useState<{ code: string; expires_at: string } | null>(null);
  const [busy, setBusy] = useState(false);
  const { confirm, dialog } = useConfirmDialog();
  const reloadDevices = devices.reload;
  // La lista se vuelve a consultar sola: estado en línea, usuario actual y terminales recién aprobadas.
  useEffect(() => {
    const timer = window.setInterval(() => void reloadDevices(), devicesRefreshMilliseconds);
    return () => window.clearInterval(timer);
  }, [reloadDevices]);
  async function invitation() {
    setBusy(true);
    try {
      setInvite(await api('invite_device', { body: {} }));
      notifySuccess('Código de autorización creado.');
    } catch (error) {
      notifyError(error, 'No se pudo crear la autorización.');
    } finally {
      setBusy(false);
    }
  }
  // Revocar es irreversible: se confirma con el diálogo estándar en su variante de peligro.
  async function revoke(device: Device) {
    const confirmed = await confirm({
      title: `Revocar ${device.name}`,
      message:
        'La terminal perderá acceso inmediatamente, incluidas sus sesiones. Para volver a usarla habrá que emparejarla de nuevo.',
      confirmLabel: 'Confirmar revocación',
      cancelLabel: 'Cancelar',
      tone: 'danger',
    });
    if (!confirmed) {
      return;
    }
    setBusy(true);
    try {
      await api('revoke_device', { id: device.id, body: {} });
      notifySuccess(`Terminal ${device.name} revocada.`);
      await devices.reload();
    } catch (error) {
      notifyError(error, 'No se pudo revocar la terminal.');
    } finally {
      setBusy(false);
    }
  }
  return (
    <>
      <PageTitle
        title="Terminales"
        description="Las computadoras y dispositivos que usan AD-EL POS en el restaurante."
      >
        {can('devices.manage') && (
          <button className="secondary-button" disabled={busy} onClick={() => void invitation()}>
            Código para esta computadora
          </button>
        )}
      </PageTitle>
      <PairingRequestsPanel can={can} onChanged={reloadDevices} />
      <SessionsSummaryPanel />
      <Notice>{devices.error || registers.error}</Notice>
      {invite && (
        <section className="surface panel">
          <h2>Código de un solo uso</h2>
          <p className="muted my-3 text-sm">
            Sirve para volver a autorizar AD-EL POS en esta misma computadora antes de{' '}
            {dateTime(invite.expires_at)}. Las demás terminales se emparejan con el código que
            muestran. No concede una sesión de usuario.
          </p>
          <code className="enrollment-code">{invite.code}</code>
          <button className="secondary-button mt-4" onClick={() => setInvite(null)}>
            Ocultar código
          </button>
        </section>
      )}
      {editing && (
        <>
          <DeviceEditor
            key={editing.id}
            device={editing}
            registers={registers.data ?? []}
            reload={devices.reload}
            onSaved={onSaved}
          />
          <button className="secondary-button mb-5" onClick={() => setEditing(null)}>
            Cerrar editor
          </button>
        </>
      )}
      {viewing && (
        <DeviceConfigurationPanel
          key={viewing.id}
          device={viewing}
          onClose={() => setViewing(null)}
        />
      )}
      <div className="surface table-container">
        <table>
          <thead>
            <tr>
              <th>Terminal</th>
              <th>Tipo y sistema</th>
              <th>Caja</th>
              <th>Pantalla inicial</th>
              <th>Última conexión</th>
              <th>Usuario actual</th>
              <th>Estado</th>
              <th>Acciones</th>
            </tr>
          </thead>
          <tbody>
            {devices.data?.map((device) => {
              const state = connectionState(device);
              return (
                <tr key={device.id}>
                  <td>
                    <strong>{device.name}</strong>
                    <small className="muted block">
                      Código: <span className="font-mono">{device.id}</span>
                    </small>
                  </td>
                  <td>
                    {deviceKindLabels[device.kind ?? 'desktop']}
                    <small className="muted block">{device.os || 'Esta computadora'}</small>
                  </td>
                  <td>{device.register_name ?? 'Sin asociación'}</td>
                  <td>{homeLabel(device)}</td>
                  <td>{device.last_seen_at ? dateTime(device.last_seen_at) : '—'}</td>
                  <td>{device.revoked_at ? '—' : (device.current_user ?? 'Nadie')}</td>
                  <td>
                    <span className="device-state">
                      <span className={`status-dot ${state.tone}`} aria-hidden="true" />
                      {state.label}
                    </span>
                  </td>
                  <td>
                    <div className="button-row">
                      {can('devices.manage') && !device.revoked_at && (
                        <button className="secondary-button" onClick={() => setEditing(device)}>
                          Editar
                        </button>
                      )}
                      <button className="secondary-button" onClick={() => setViewing(device)}>
                        Ver configuración
                      </button>
                      {can('devices.manage') && !device.revoked_at && (
                        <button
                          className="danger-button"
                          disabled={busy}
                          onClick={() => void revoke(device)}
                        >
                          Revocar
                        </button>
                      )}
                    </div>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
      {dialog}
    </>
  );
}
