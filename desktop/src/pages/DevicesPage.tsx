import { useCallback, useEffect, useState } from 'react';
import type { ReactNode } from 'react';
import { useConfirmDialog } from '../components/ConfirmDialog';
import { dateTime, Field, Form, Notice, PageTitle } from '../components/Forms';
import { SegmentedControl } from '../components/SegmentedControl';
import { ToggleButton } from '../components/ToggleButton';
import { useResource } from '../hooks/useResource';
import { terminalHomes } from '../navigation/sections';
import type { TerminalHome } from '../navigation/sections';
import { api } from '../services/api';
import { printerLabel } from '../services/kitchen';
import { notifyError, notifySuccess } from '../services/notify';
import { countLabel } from '../services/plural';
import type {
  Device,
  DeviceConfiguration,
  DeviceKind,
  KitchenDisplaySettings,
  PairingRequest,
  PairingRequests,
  PrecheckPrinterOption,
  Register,
  SessionsSummary,
} from '../services/models';
import { MultiSelect, Select } from '../components/Select';
import type { SelectOption } from '../components/Select';
import { Check } from '../components/catalog/EditorFields';
import {
  devicePrecheckPrinterCapability,
  kitchenDisplayCapability,
  readyAlertsCapability,
  serverHas,
  waiterPINCapability,
} from '../services/serverCapabilities';
import {
  licenseCapability,
  licenseSeats,
  releasableSeats,
  seatLabels,
  seatNames,
  seatsHeldText,
} from '../services/license';
import { releaseDeviceSeat } from '../services/licenseOperations';

// Etapa 9G (fase B, §3.2–§3.4): Sistema › Terminales también recibe las terminales nuevas que piden su
// código (se aprueban escribiendo el código que muestra la terminal, solo en la computadora principal),
// muestra el tipo, el sistema, el usuario actual y si cada terminal está en línea, su configuración y las
// sesiones activas por tipo de dispositivo (solo se cuentan).

// Cada cuánto se vuelven a consultar las solicitudes pendientes y la lista (estado en línea).
export const pairingRefreshMilliseconds = 3000;
export const devicesRefreshMilliseconds = 5000;

// Etapa A1 (fase B): el tipo android se dice en lenguaje sencillo, «Teléfono o tableta» (antes «Comandero Android»).
// Etapa K1 (K1-D3): kitchen es la tableta con la aplicación de cocina, que solo funciona como pantalla de cocina.
export const deviceKindLabels: Record<DeviceKind, string> = {
  desktop: 'Escritorio',
  android: 'Teléfono o tableta',
  kitchen: 'Pantalla de cocina',
};

// Versión de la aplicación de la terminal. Desde la etapa K1 (K1-D9) Go anota la que trae cada petición, así que es la
// instalada; con un servidor anterior es la que informó al emparejarse.
export function installedVersionText(device: Device): string {
  if (!device.app_version) {
    return '';
  }
  return serverHas(kitchenDisplayCapability)
    ? `versión instalada ${device.app_version}`
    : `versión ${device.app_version}`;
}

// Etapa K1 (Fase E, §6 y K1-D15): textos de los ajustes de una pantalla de cocina.
export const kitchenTextSizeLabels: Record<KitchenDisplaySettings['text_size'], string> = {
  normal: 'Normal',
  large: 'Grande',
};
export const kitchenThemeLabels: Record<KitchenDisplaySettings['theme'], string> = {
  dark: 'Oscuro',
  light: 'Claro',
};
export const kitchenVolumeLabels: Record<KitchenDisplaySettings['sound_volume'], string> = {
  low: 'Bajo',
  medium: 'Medio',
  high: 'Alto',
};

export function kitchenSoundText(settings: KitchenDisplaySettings): string {
  if (!settings.sound_enabled) {
    return 'Desactivado';
  }
  return `Activado · volumen ${kitchenVolumeLabels[settings.sound_volume].toLowerCase()}`;
}

// El cupo de cocina que ocupa la pantalla (K1-D6 y K1-D30): «Liberar cupo» no es definitivo, la tableta lo puede pedir
// otra vez con «Usar esta pantalla».
export function kitchenSeatText(device: Device): string {
  if (device.revoked_at) {
    return 'Sin cupo: la pantalla está revocada';
  }
  if (device.seats?.includes('kitchen_displays')) {
    return 'Ocupa un cupo de pantalla de cocina';
  }
  if (device.kitchen_display?.seat_released_at) {
    return `Dejó su cupo el ${dateTime(device.kitchen_display.seat_released_at)}. En la tableta se puede pedir otra vez con «Usar esta pantalla».`;
  }
  return 'Sin cupo: lo ocupa al conectarse';
}

// Las estaciones que muestra una pantalla de cocina: todas las que reciben comandas en pantalla o su selección (solo
// las de recepción en pantalla cuentan, K1-D11).
export function kitchenStationsText(configuration: DeviceConfiguration): string {
  const options = configuration.kitchen_display_station_options ?? [];
  if (options.length === 0) {
    return 'Ninguna: no hay estaciones que reciban comandas en pantalla';
  }
  if (configuration.kitchen_all_stations || configuration.kitchen_stations.length === 0) {
    return `Todas las que reciben comandas en pantalla (${options.map((station) => station.name).join(', ')})`;
  }
  const shown = configuration.kitchen_stations.filter((station) =>
    options.some((option) => option.id === station.id),
  );
  return shown.length > 0
    ? shown.map((station) => station.name).join(', ')
    : 'Ninguna de las elegidas recibe comandas en pantalla';
}

// Bloqueo por inactividad de un teléfono o tableta de meseros (etapa A1, fase B): tras ese tiempo sin uso vuelve a
// la pantalla de entrada con PIN (salvo con una captura en curso o pedidos sin enviar; lo decide el dispositivo). Go
// acepta de 30 s a 1 h; aquí se elige en minutos y 2 es el valor por omisión.
export const defaultIdleLockSeconds = 120;
const idleLockMinutes = [1, 2, 3, 5, 10, 15, 30, 60];

export function idleLockLabel(seconds: number) {
  if (seconds % 60 === 0) {
    return countLabel(seconds / 60, 'minuto', 'minutos');
  }
  if (seconds < 60) {
    return countLabel(seconds, 'segundo', 'segundos');
  }
  return `${Math.floor(seconds / 60)} min ${seconds % 60} s`;
}

function idleLockOptions(current: number) {
  const values = idleLockMinutes.map((minutes) => minutes * 60);
  // Un valor guardado que no está en la lista (puesto por otro medio) se conserva como opción.
  if (!values.includes(current)) {
    values.push(current);
    values.sort((left, right) => left - right);
  }
  return values.map((seconds) => ({
    value: String(seconds),
    label:
      seconds === defaultIdleLockSeconds
        ? `${idleLockLabel(seconds)} (recomendado)`
        : idleLockLabel(seconds),
  }));
}

// Impresora de precuentas de un teléfono o tableta (etapa A2, parte C, A2-D3): el mesero imprime solo en ella. Si una
// impresora no sirve lo decide Go (problem); aquí solo se dice en lenguaje sencillo.
const precheckPrinterProblems: Record<string, string> = {
  inactive: 'desactivada',
  unsupported: 'no se puede usar para imprimir',
};

// precheckPrinterOptions son las opciones del selector: «Sin impresora» y todas las impresoras (de red y USB de una
// terminal, con el nombre de su terminal). Las que no sirven se muestran deshabilitadas con el motivo, salvo la ya
// elegida, que se conserva visible y elegible aunque haya dejado de servir (como en Cobros › Políticas).
export function precheckPrinterOptions(
  printers: PrecheckPrinterOption[],
  chosen: string,
  device: Device,
): SelectOption[] {
  const options: SelectOption[] = [{ value: '', label: 'Sin impresora' }];
  for (const printer of printers) {
    const reason = printer.problem ? (precheckPrinterProblems[printer.problem] ?? 'no sirve') : '';
    options.push({
      value: printer.id,
      label: printerLabel(printer),
      description: reason || undefined,
      disabled: reason !== '' && printer.id !== chosen,
    });
  }
  // Mientras llega la configuración, la impresora guardada se muestra con el nombre que trae la terminal.
  if (chosen && !printers.some((printer) => printer.id === chosen)) {
    options.push({
      value: chosen,
      label: printerLabel({
        name: device.precheck_printer_name ?? 'Impresora guardada',
        terminal_name: device.precheck_printer_terminal_name,
      }),
    });
  }
  return options;
}

// precheckPrinterText es lo que «Ver configuración» dice de la impresora de precuentas.
export function precheckPrinterText(printer: PrecheckPrinterOption | null | undefined): string {
  if (!printer) {
    return 'Sin impresora: este dispositivo no imprime precuentas';
  }
  const label = printerLabel(printer);
  const reason = printer.problem ? precheckPrinterProblems[printer.problem] : '';
  return reason ? `${label} (${reason})` : label;
}

// readyAlertsText es lo que «Ver configuración» dice de los avisos de platillos listos (etapa A2, parte D).
export function readyAlertsText(device: Device): string {
  if (device.ready_alerts_enabled === false) {
    return 'Desactivado';
  }
  const sound = device.ready_alerts_sound !== false;
  const vibration = device.ready_alerts_vibration !== false;
  if (sound && vibration) {
    return 'Activado · con sonido y vibración';
  }
  if (sound) {
    return 'Activado · con sonido, sin vibración';
  }
  if (vibration) {
    return 'Activado · sin sonido, con vibración';
  }
  return 'Activado · sin sonido ni vibración';
}

// Etapa A3 (§5.1, A3-D7): cada tipo de terminal solo ve lo que le aplica. Un teléfono o tableta de meseros (android)
// no cobra ni tiene modo cocina: no tiene caja física, teclado virtual, estaciones de Cocina, aviso sonoro ni
// impresoras de la terminal, y su pantalla inicial es Mesas o Para llevar (Go rechaza las demás). Las terminales de
// escritorio y de cocina conservan todo.
function isWaiterDevice(device: Device) {
  return device.kind === 'android';
}

// Etapa K1: una pantalla de cocina Android (solo muestra y marca pedidos; se configura aquí).
function isKitchenDisplay(device: Device) {
  return device.kind === 'kitchen';
}

// terminalHomeOptions son las pantallas iniciales que se ofrecen para la terminal según su tipo.
export function terminalHomeOptions(device: Device) {
  if (!isWaiterDevice(device)) {
    return terminalHomes;
  }
  return terminalHomes.filter((entry) => entry.value === 'tables' || entry.value === 'takeaway');
}

// editableHome es la pantalla inicial con la que abre el editor: la guardada si el tipo la admite; si no (un teléfono
// con «Cocina» o «Caja» guardada en un servidor anterior a la 0027), Mesas, que es donde la aplicación ya abría.
function editableHome(device: Device): TerminalHome {
  const saved = device.home_screen ?? 'tables';
  return terminalHomeOptions(device).some((entry) => entry.value === saved) ? saved : 'tables';
}

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
        <p className="muted pairing-help">Esta computadora no recibe terminales de la red.</p>
      )}
      {data?.limit_reached && (
        <p className="pairing-alert">
          Se pidieron demasiados códigos de terminal en los últimos {data.window_minutes ?? 10}{' '}
          minutos. Espera un poco antes de pedir otro.
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

// Sesiones activas por tipo de dispositivo (§3.4, D9). Etapa L1 (Fase E, §5): con un servidor que aplica los cupos
// de la licencia, en su lugar se ven los cupos en uso y el límite de cada uno (lo que hace cada terminal, no su tipo).
function SessionsSummaryPanel({ reloadKey }: { reloadKey: number }) {
  const summary = useResource<SessionsSummary>('devices_sessions_summary');
  const reloadSummary = summary.reload;
  useEffect(() => {
    if (reloadKey > 0) void reloadSummary();
  }, [reloadKey, reloadSummary]);
  const counts = summary.data?.by_kind;
  const seats = summary.data?.seats;
  if (seats && serverHas(licenseCapability)) {
    return (
      <section className="surface panel sessions-summary" aria-labelledby="license-seats-title">
        <h2 id="license-seats-title">Cupos de la licencia</h2>
        <dl className="sessions-summary-counts">
          {licenseSeats.map((seat) => (
            <div key={seat}>
              <dt>{seatLabels[seat]}</dt>
              <dd>
                {seats[seat]?.in_use ?? 0} de {seats[seat]?.limit ?? 0}
              </dd>
            </div>
          ))}
        </dl>
        <p className="muted text-sm">
          {serverHas(kitchenDisplayCapability)
            ? 'Cada terminal ocupa un cupo según lo que hace: una caja con su turno abierto, un comandero al capturar pedidos sin turno o una pantalla de cocina: una computadora en Cocina › Preparación, o una tableta de cocina mientras esté emparejada (se libera con «Liberar cupo» o «Revocar»). Las cajas se liberan al cerrar el turno.'
            : 'Cada terminal ocupa un cupo según lo que hace: una caja con su turno abierto, un comandero al capturar pedidos sin turno o una pantalla de cocina en Preparación. Las cajas se liberan al cerrar el turno.'}
        </p>
      </section>
    );
  }
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
    </section>
  );
}

function DeviceConfigurationPanel({ device, onClose }: { device: Device; onClose: () => void }) {
  const configuration = useResource<DeviceConfiguration>('device_configuration', {
    id: device.id,
  });
  const value = configuration.data;
  const shown = value?.device ?? device;
  const waiterDevice = isWaiterDevice(shown);
  if (isKitchenDisplay(shown)) {
    return (
      <KitchenDisplayConfiguration shown={shown} configuration={value} onClose={onClose}>
        <Notice>{configuration.error}</Notice>
      </KitchenDisplayConfiguration>
    );
  }
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
              {[shown.os || 'Esta computadora', installedVersionText(shown)]
                .filter(Boolean)
                .join(' · ')}
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
          {!waiterDevice && (
            <div>
              <dt>Caja física</dt>
              <dd>{shown.register_name ?? 'Sin caja asociada'}</dd>
            </div>
          )}
          {!waiterDevice && (
            <div>
              <dt>Teclado virtual</dt>
              <dd>{keyboardLabels[shown.keyboard_mode]}</dd>
            </div>
          )}
          {shown.kind === 'android' && serverHas(waiterPINCapability) && (
            <div>
              <dt>Bloqueo por inactividad</dt>
              <dd>
                {idleLockLabel(shown.idle_lock_seconds ?? defaultIdleLockSeconds)} sin uso vuelve a
                la pantalla de entrada con PIN
              </dd>
            </div>
          )}
          {shown.kind === 'android' && serverHas(devicePrecheckPrinterCapability) && (
            <div>
              <dt>Impresora de precuentas</dt>
              <dd>{precheckPrinterText(value.precheck_printer)}</dd>
            </div>
          )}
          {shown.kind === 'android' && serverHas(readyAlertsCapability) && (
            <div>
              <dt>Avisar platillos listos</dt>
              <dd>{readyAlertsText(shown)}</dd>
            </div>
          )}
          {!waiterDevice && (
            <div>
              <dt>Estaciones que muestra en Cocina</dt>
              <dd>
                {value.kitchen_all_stations || value.kitchen_stations.length === 0
                  ? 'Todas'
                  : value.kitchen_stations.map((station) => station.name).join(', ')}
              </dd>
            </div>
          )}
          {!waiterDevice && (
            <div>
              <dt>Aviso sonoro de comandas nuevas</dt>
              <dd>{value.kitchen_sound_alert ? 'Activado' : 'Desactivado'}</dd>
            </div>
          )}
          {!waiterDevice && (
            <div>
              <dt>Impresoras de esta terminal</dt>
              {/* Etapa 9G (fase E): impresoras USB conectadas a la terminal, que imprime su agente. */}
              <dd>
                {value.printers.length > 0
                  ? value.printers
                      .map((printer) =>
                        printer.active === false ? `${printer.name} (desactivada)` : printer.name,
                      )
                      .join(', ')
                  : 'Sin impresoras propias'}
              </dd>
            </div>
          )}
          {/* Etapa A3 (§5.3): el identificador ya no está en la tabla; aquí sirve para soporte y para filtrar el
              Registro de movimientos por terminal. */}
          <div>
            <dt>Identificador</dt>
            <dd className="font-mono">{shown.id}</dd>
          </div>
        </dl>
      )}
      {value && !waiterDevice && (
        <LocalPrinters name={shown.name} local={value.local_printers ?? null} />
      )}
      {!waiterDevice && (
        <p className="muted pairing-help">
          Las estaciones y el aviso sonoro se eligen en Cocina › Estaciones › Terminales de cocina.
        </p>
      )}
      <button className="secondary-button mt-4" onClick={onClose}>
        Cerrar configuración
      </button>
    </section>
  );
}

// Etapa K1 (Fase E, §6): lo que se ve de una pantalla de cocina Android: sus estaciones, letra, tema y sonido, su
// conexión, la versión instalada y el cupo de cocina que ocupa. No tiene caja, teclado, pantalla inicial ni impresoras.
function KitchenDisplayConfiguration({
  shown,
  configuration,
  onClose,
  children,
}: {
  shown: Device;
  configuration: DeviceConfiguration | null;
  onClose: () => void;
  children: ReactNode;
}) {
  const settings = shown.kitchen_display;
  return (
    <section
      className="surface panel device-configuration"
      aria-labelledby="device-configuration-title"
    >
      <h2 id="device-configuration-title">Configuración de {shown.name}</h2>
      {children}
      {configuration && (
        <dl className="device-configuration-list">
          <div>
            <dt>Tipo</dt>
            <dd>{deviceKindLabels.kitchen}</dd>
          </div>
          <div>
            <dt>Sistema</dt>
            <dd>{[shown.os, installedVersionText(shown)].filter(Boolean).join(' · ') || '—'}</dd>
          </div>
          <div>
            <dt>Conexión</dt>
            <dd>
              {connectionState(shown).label}
              {shown.last_seen_at ? ` · última conexión ${dateTime(shown.last_seen_at)}` : ''}
            </dd>
          </div>
          <div>
            <dt>Cupo de cocina</dt>
            <dd>{kitchenSeatText(shown)}</dd>
          </div>
          <div>
            <dt>Estaciones que muestra</dt>
            <dd>{kitchenStationsText(configuration)}</dd>
          </div>
          {settings && (
            <>
              <div>
                <dt>Tamaño de letra</dt>
                <dd>{kitchenTextSizeLabels[settings.text_size]}</dd>
              </div>
              <div>
                <dt>Tema</dt>
                <dd>{kitchenThemeLabels[settings.theme]}</dd>
              </div>
              <div>
                <dt>Sonido</dt>
                <dd>{kitchenSoundText(settings)}</dd>
              </div>
            </>
          )}
          <div>
            <dt>Emparejada</dt>
            <dd>{shown.paired_at ? dateTime(shown.paired_at) : '—'}</dd>
          </div>
          <div>
            <dt>Identificador</dt>
            <dd className="font-mono">{shown.id}</dd>
          </div>
        </dl>
      )}
      <p className="muted pairing-help">
        Los colores por tiempo de espera de todas las pantallas se eligen en Cocina › Comandas y
        tiempos.
      </p>
      <button className="secondary-button mt-4" onClick={onClose}>
        Cerrar configuración
      </button>
    </section>
  );
}

// Etapa K1 (Fase E, §6): editar una pantalla de cocina: nombre, estaciones (solo las que reciben comandas en pantalla,
// con PUT kitchen/access y su permiso), letra, tema, sonido y volumen (PATCH devices/{id}). Los cambios le llegan a la
// tableta al momento: Go le avisa con su evento de configuración.
function KitchenDisplayEditor({
  device,
  can,
  reload,
}: {
  device: Device;
  can: (key: string) => boolean;
  reload: () => Promise<void>;
}) {
  const configuration = useResource<DeviceConfiguration>('device_configuration', {
    id: device.id,
  });
  const settings: KitchenDisplaySettings = device.kitchen_display ?? {
    text_size: 'normal',
    theme: 'dark',
    sound_enabled: true,
    sound_volume: 'medium',
    seat_released_at: null,
  };
  const [name, setName] = useState(device.name);
  const [textSize, setTextSize] = useState(settings.text_size);
  const [theme, setTheme] = useState(settings.theme);
  const [sound, setSound] = useState(settings.sound_enabled);
  const [volume, setVolume] = useState(settings.sound_volume);
  const loaded = configuration.data;
  const options = loaded?.kitchen_display_station_options ?? [];
  const loadedAll = !loaded || loaded.kitchen_all_stations || loaded.kitchen_stations.length === 0;
  const loadedIDs = loaded?.kitchen_stations.map((station) => station.id) ?? [];
  const [allStations, setAllStations] = useState<boolean | null>(null);
  const [stationIDs, setStationIDs] = useState<string[] | null>(null);
  const chosenAll = allStations ?? loadedAll;
  // Solo cuentan las elegidas que reciben comandas en pantalla (una que pasó a «Solo impresora» ya no se muestra).
  const shownIDs = loadedIDs.filter((id) => options.some((option) => option.id === id));
  const chosenIDs = stationIDs ?? shownIDs;
  const manageStations = can('kitchen.stations.manage');
  const stationsChanged =
    loaded !== null &&
    (chosenAll !== loadedAll ||
      (!chosenAll && [...chosenIDs].sort().join(',') !== [...shownIDs].sort().join(',')));
  return (
    <section className="surface panel">
      <h2 className="mb-5">Editar {device.name}</h2>
      <Notice>{configuration.error}</Notice>
      <Form
        successMessage="Pantalla de cocina actualizada."
        disabled={!loaded}
        onSave={async () => {
          if (manageStations && stationsChanged) {
            await api('kitchen_save_access', {
              body: {
                operation_id: crypto.randomUUID(),
                device_id: device.id,
                all_stations: chosenAll,
                station_ids: chosenAll ? [] : chosenIDs,
                expected_all_stations: loaded?.kitchen_all_stations ?? true,
                expected_station_ids: loadedIDs,
              },
            });
          }
          await api('update_device', {
            id: device.id,
            body: {
              name,
              register_id: device.register_id,
              keyboard_mode: device.keyboard_mode,
              kitchen_display_text_size: textSize,
              kitchen_display_theme: theme,
              kitchen_display_sound: sound,
              kitchen_display_volume: volume,
            },
          });
          setAllStations(null);
          setStationIDs(null);
          await configuration.reload();
          await reload();
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
        </div>
        <fieldset disabled={!manageStations} className="form-stack">
          <Check
            label="Mostrar todas las estaciones que reciben comandas en pantalla"
            value={chosenAll}
            onChange={setAllStations}
          />
          {!chosenAll && (
            <Field
              label="Estaciones que muestra esta pantalla"
              hint="Solo las estaciones con recepción «Solo pantalla de cocina» o «Impresora y pantalla de cocina»."
            >
              <MultiSelect
                required
                value={chosenIDs}
                placeholder="Elige las estaciones"
                options={options.map((station) => ({
                  value: station.id,
                  label: station.active ? station.name : `${station.name} (inactiva)`,
                }))}
                onChange={setStationIDs}
              />
            </Field>
          )}
          {!manageStations && (
            <small className="muted">
              Para cambiar las estaciones hace falta el permiso de administrar estaciones de cocina.
            </small>
          )}
        </fieldset>
        <div className="field">
          <span>Tamaño de letra</span>
          <SegmentedControl<KitchenDisplaySettings['text_size']>
            label="Tamaño de letra de la pantalla"
            value={textSize}
            options={[
              { value: 'normal', label: kitchenTextSizeLabels.normal },
              { value: 'large', label: kitchenTextSizeLabels.large },
            ]}
            onChange={setTextSize}
          />
        </div>
        <div className="field">
          <span>Tema</span>
          <SegmentedControl<KitchenDisplaySettings['theme']>
            label="Tema de la pantalla"
            value={theme}
            options={[
              { value: 'dark', label: kitchenThemeLabels.dark },
              { value: 'light', label: kitchenThemeLabels.light },
            ]}
            onChange={setTheme}
          />
        </div>
        {/* El mismo control de dos opciones que la letra y el tema: se ve cuál está elegida. */}
        <div className="field">
          <span>Sonido</span>
          <SegmentedControl<'on' | 'off'>
            label="Sonido de la pantalla"
            value={sound ? 'on' : 'off'}
            options={[
              { value: 'on', label: 'Activado' },
              { value: 'off', label: 'Desactivado' },
            ]}
            onChange={(value) => setSound(value === 'on')}
          />
          <small className="muted">
            Suena al llegar un pedido nuevo y, con otro sonido, con cancelaciones y cambios.
          </small>
        </div>
        <div className="form-grid">
          <Field label="Volumen">
            <Select<KitchenDisplaySettings['sound_volume']>
              value={volume}
              disabled={!sound}
              options={[
                { value: 'low', label: kitchenVolumeLabels.low },
                { value: 'medium', label: kitchenVolumeLabels.medium },
                { value: 'high', label: kitchenVolumeLabels.high },
              ]}
              onChange={setVolume}
            />
          </Field>
        </div>
      </Form>
    </section>
  );
}

// Impresoras que informó la computadora de la terminal (etapa 9G, fase E): cuáles sirven para tickets, comandas y
// el cajón (las decide Go) y si su agente de impresión está conectado. Se asignan en Cocina › Impresoras con
// «Impresora USB de una terminal».
function LocalPrinters({
  name,
  local,
}: {
  name: string;
  local: NonNullable<DeviceConfiguration['local_printers']> | null;
}) {
  if (!local) {
    return null;
  }
  return (
    <section
      className="device-local-printers"
      aria-label={`Impresoras de la computadora de ${name}`}
    >
      <h3>Impresoras de la computadora</h3>
      <p className="muted text-sm">
        {local.agent_online
          ? 'Conectada: imprime los trabajos de sus impresoras.'
          : 'Desconectada: sus trabajos esperan hasta que AD-EL POS esté abierto en ella.'}{' '}
        {local.reported_at
          ? `Informó su lista: ${dateTime(local.reported_at)}`
          : 'No ha informado sus impresoras.'}
      </p>
      {local.queues.length > 0 ? (
        <ul className="device-local-printer-list">
          {local.queues.map((queue) => (
            <li key={queue.queue}>
              <strong>{queue.queue}</strong>
              {queue.model && <span className="muted"> · {queue.model}</span>}
              <span className={queue.usable ? 'device-printer-usable' : 'muted'}>
                {' '}
                · {queue.usable ? 'Se puede usar' : `No se puede usar: ${queue.reason ?? ''}`}
              </span>
            </li>
          ))}
        </ul>
      ) : (
        local.reported_at && <p className="muted text-sm">Su computadora no tiene impresoras.</p>
      )}
      <p className="muted pairing-help">
        Para imprimir en una de ellas, regístrala en Cocina › Impresoras como «Impresora USB de una
        terminal».
      </p>
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
  const [home, setHome] = useState<TerminalHome>(editableHome(device));
  // Etapa A3: un teléfono o tableta de meseros no muestra la caja ni el teclado; se envían los guardados sin cambio
  // (registerID y keyboard empiezan con ellos), así que editarlo nunca le cambia la caja aunque tenga un turno abierto.
  const android = isWaiterDevice(device);
  const [idleLock, setIdleLock] = useState(
    String(device.idle_lock_seconds ?? defaultIdleLockSeconds),
  );
  // Solo los teléfonos y tabletas de meseros tienen bloqueo por inactividad, y solo si el servidor lo acepta (un
  // servidor 0.11.0 rechazaría el campo).
  const waiterDevice = device.kind === 'android' && serverHas(waiterPINCapability);
  // Etapa A2 (parte C): su impresora de precuentas, solo si el servidor la admite (uno anterior rechazaría el campo).
  // Las impresoras que se pueden elegir llegan con la configuración de la terminal (devices.read).
  const precheckDevice = device.kind === 'android' && serverHas(devicePrecheckPrinterCapability);
  const configuration = useResource<DeviceConfiguration>('device_configuration', {
    id: device.id,
    enabled: precheckDevice,
  });
  const [precheckPrinter, setPrecheckPrinter] = useState(device.precheck_printer_id ?? '');
  // Etapa A2 (parte D): sus avisos de platillos listos, solo si el servidor los admite. Activados por omisión.
  const readyDevice = device.kind === 'android' && serverHas(readyAlertsCapability);
  const [readyEnabled, setReadyEnabled] = useState(device.ready_alerts_enabled ?? true);
  const [readySound, setReadySound] = useState(device.ready_alerts_sound ?? true);
  const [readyVibration, setReadyVibration] = useState(device.ready_alerts_vibration ?? true);
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
              ...(waiterDevice ? { idle_lock_seconds: Number(idleLock) } : {}),
              ...(precheckDevice ? { precheck_printer_id: precheckPrinter || null } : {}),
              ...(readyDevice
                ? {
                    ready_alerts_enabled: readyEnabled,
                    ready_alerts_sound: readySound,
                    ready_alerts_vibration: readyVibration,
                  }
                : {}),
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
          {!android && (
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
          )}
          {!android && (
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
          )}
          {waiterDevice && (
            <Field
              label="Bloqueo por inactividad"
              hint="Tiempo sin uso tras el que el teléfono o la tableta vuelve a la pantalla de entrada con PIN. No se bloquea mientras un mesero tiene una captura en curso o pedidos sin enviar."
            >
              <Select
                value={idleLock}
                options={idleLockOptions(Number(idleLock))}
                onChange={setIdleLock}
              />
            </Field>
          )}
          {precheckDevice && (
            <Field
              label="Impresora de precuentas"
              hint="El mesero imprime la precuenta de sus mesas y pedidos para llevar solo en esta impresora. Sin impresora, este dispositivo no imprime precuentas."
            >
              <Select
                value={precheckPrinter}
                options={precheckPrinterOptions(
                  configuration.data?.precheck_printer_options ?? [],
                  precheckPrinter,
                  device,
                )}
                onChange={setPrecheckPrinter}
              />
            </Field>
          )}
        </div>
        {readyDevice && (
          <div className="field">
            <span>Avisos de platillos listos</span>
            <div className="button-row">
              <ToggleButton
                label="Avisar platillos listos"
                pressed={readyEnabled}
                onChange={setReadyEnabled}
              />
              <ToggleButton
                label="Sonido"
                pressed={readySound}
                onChange={setReadySound}
                disabled={!readyEnabled}
              />
              <ToggleButton
                label="Vibración"
                pressed={readyVibration}
                onChange={setReadyVibration}
                disabled={!readyEnabled}
              />
            </div>
            <small className="muted">
              Cuando cocina marca «Listo» en la pantalla de cocina, el mesero de la cuenta recibe el
              aviso en este dispositivo, aunque tenga la pantalla apagada.
            </small>
          </div>
        )}
        {/* Etapa 9C (§1.5): pantalla en la que abre la aplicación en esta terminal y a la que vuelve
            el logotipo. Lista cerrada validada en Go; quien no pueda ver esa sección abre en la
            primera de la barra que sí puede ver. */}
        <div className="field">
          <span>Pantalla inicial</span>
          <SegmentedControl<TerminalHome>
            label="Pantalla inicial de la terminal"
            value={home}
            options={terminalHomeOptions(device)}
            onChange={setHome}
          />
          <small className="muted">
            Si un usuario no puede ver esta sección, la aplicación abre en la primera que sí puede
            ver.
          </small>
          {/* Etapa 9F (§5.1): una terminal de cocina abre en Cocina › Preparación; sus estaciones y
              su aviso sonoro se eligen junto a las estaciones. Un teléfono o tableta no tiene Cocina. */}
          {!android && (
            <small className="muted">
              Con «Cocina» abre en Preparación. Las estaciones que muestra y el aviso sonoro se
              eligen en Cocina › Estaciones › Terminales de cocina.
            </small>
          )}
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
  // Etapa L1: los cupos se vuelven a consultar después de «Liberar cupo».
  const [seatsVersion, setSeatsVersion] = useState(0);
  const { confirm, dialog } = useConfirmDialog();
  const reloadDevices = devices.reload;
  const showSeats = serverHas(licenseCapability);
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
      message: isKitchenDisplay(device)
        ? 'La pantalla deja de mostrar pedidos al momento y libera su cupo de cocina. Para volver a usarla habrá que emparejarla de nuevo.'
        : 'La terminal perderá acceso inmediatamente, incluidas sus sesiones. Para volver a usarla habrá que emparejarla de nuevo.',
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
  // Etapa L1 (Fase E, §5): «Liberar cupo» de comandero o de pantalla de cocina. Cierra las sesiones de esa terminal; se
  // vuelve a usar iniciando sesión otra vez. Las cajas se liberan cerrando el turno.
  async function releaseSeat(device: Device) {
    const seats = releasableSeats(device.seats);
    const what = seats.map((seat) => seatNames[seat].toLowerCase()).join(' y de ');
    // Etapa K1 (K1-D30): la pantalla de cocina no tiene sesiones; deja de mostrar pedidos y puede pedir el cupo otra vez.
    const message = isKitchenDisplay(device) ? (
      <>
        <p>
          La pantalla deja de mostrar pedidos y su cupo de pantalla de cocina queda libre para otra
          pantalla.
        </p>
        <p>
          No es definitivo: en la tableta se puede pedir el cupo otra vez con «Usar esta pantalla».
          Para que deje de usarse, revócala.
        </p>
      </>
    ) : (
      <>
        <p>
          Se cierra la sesión de quien esté usando esta terminal y su cupo de {what} queda libre
          para otra terminal.
        </p>
        <p>Para volver a usarla, alguien inicia sesión otra vez.</p>
      </>
    );
    const confirmed = await confirm({
      title: `Liberar el cupo de ${device.name}`,
      message,
      confirmLabel: 'Liberar cupo',
      cancelLabel: 'Cancelar',
    });
    if (!confirmed) {
      return;
    }
    setBusy(true);
    try {
      await releaseDeviceSeat(device.id);
      notifySuccess(`Cupo de ${device.name} liberado.`);
      await devices.reload();
      setSeatsVersion((current) => current + 1);
    } catch (error) {
      notifyError(error, 'No se pudo liberar el cupo.');
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
      <SessionsSummaryPanel reloadKey={seatsVersion} />
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
          {isKitchenDisplay(editing) ? (
            <KitchenDisplayEditor
              key={editing.id}
              device={editing}
              can={can}
              reload={devices.reload}
            />
          ) : (
            <DeviceEditor
              key={editing.id}
              device={editing}
              registers={registers.data ?? []}
              reload={devices.reload}
              onSaved={onSaved}
            />
          )}
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
              {showSeats && <th>Cupo</th>}
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
                  </td>
                  <td>
                    {deviceKindLabels[device.kind ?? 'desktop']}
                    <small className="muted block">
                      {[device.os || 'Esta computadora', installedVersionText(device)]
                        .filter(Boolean)
                        .join(' · ')}
                    </small>
                  </td>
                  {/* Etapa A3 (§5.2): un teléfono o tableta no cobra; un guion en lugar de «Sin asociación». Etapa K1:
                      una pantalla de cocina tampoco, ni tiene pantalla inicial. */}
                  <td>
                    {isWaiterDevice(device) || isKitchenDisplay(device)
                      ? '—'
                      : (device.register_name ?? 'Sin asociación')}
                  </td>
                  <td>{isKitchenDisplay(device) ? '—' : homeLabel(device)}</td>
                  <td>{device.last_seen_at ? dateTime(device.last_seen_at) : '—'}</td>
                  <td>{device.revoked_at ? '—' : (device.current_user ?? 'Nadie')}</td>
                  {showSeats && <td>{seatsHeldText(device.seats)}</td>}
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
                      {showSeats &&
                        can('devices.manage') &&
                        !device.revoked_at &&
                        releasableSeats(device.seats).length > 0 && (
                          <button
                            className="secondary-button"
                            disabled={busy}
                            onClick={() => void releaseSeat(device)}
                          >
                            Liberar cupo
                          </button>
                        )}
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
