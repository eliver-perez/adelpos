import { countLabel, pluralWord } from './plural';

// Licencia del sistema en el escritorio (etapa L1, Fase E; §7 de la especificación y decisiones L1-D4, L1-D11,
// L1-D17 y L1-D21). Go es la única autoridad de la licencia, de los cupos y de la hora: el escritorio solo muestra lo
// que Go responde.
//
// Este archivo es **el único lugar de los textos de la licencia**: los estados, los avisos, la pantalla de la fecha y
// hora, los cupos y los mensajes de cada código de error. Todos salen del código y de sus `details`, nunca del
// `message` del servidor de licencias (que Go tampoco reenvía). services/api.ts los usa para cada rechazo, así que
// toda pantalla que muestre un error de una operación dice lo correcto sin conocer la licencia.
//
// No importa services/api.ts en tiempo de ejecución (api.ts lo importa a él): las operaciones están en
// services/licenseOperations.ts.

// Capacidad del servidor 0.15.0 (L1-D22). Sin ella, contra un servidor 0.14.0, el escritorio se ve como antes.
export const licenseCapability = 'license-v1';

// Permiso «Administrar la licencia» (L1-D21). Ver el estado no pide permiso.
export const licenseManagePermission = 'license.manage';

export type LicenseState =
  'active' | 'grace' | 'expired' | 'revoked' | 'not_activated' | 'not_activated_with_data';

export type RevocationReason = 'deactivated' | 'transferred' | 'revoked';

export type LicenseSeat = 'cash_registers' | 'order_takers' | 'kitchen_displays';

export const licenseSeats: LicenseSeat[] = ['cash_registers', 'order_takers', 'kitchen_displays'];

// Hora de la computadora principal (LicenseClock de Go, Fase C): con blocked, «Hora del equipo incorrecta». Las dos
// fechas ya vienen completas, en la zona horaria del restaurante (local_time_text y last_activity_time_text).
export interface LicenseClock {
  blocked: boolean;
  local_time: string;
  last_activity_time: string | null;
  margin_minutes: number;
  timezone: string;
  local_time_text: string;
  last_activity_time_text: string;
  license_state: LicenseState;
  // Vigente o Tolerancia: solo entonces se dice «Tu licencia está bien».
  license_ok: boolean;
}

// Estado de la licencia que llega con la sesión (`license` de auth/me y restaurant/state, Fase C).
export interface LicenseSummary {
  state: LicenseState;
  environment: string;
  copied: boolean;
  machine_unverified: boolean;
  revocation_reason: RevocationReason | null;
  license_type: string | null;
  perpetual: boolean;
  expires_at: string | null;
  grace_days: number;
  grace_ends_at: string | null;
  // Los días ya vienen calculados por Go (redondeados hacia arriba).
  days_until_expiry: number | null;
  grace_days_remaining: number | null;
  clock: LicenseClock;
}

export interface LicenseLimits {
  cash_registers: number;
  order_takers: number;
  kitchen_displays: number;
}

export interface LicenseDetails {
  license_id: string;
  activation_id: string;
  licensee_name: string;
  license_type: string;
  billing_period: string | null;
  license_revision: number;
  plan: { plan_id: string; name: string; custom: boolean };
  limits: LicenseLimits;
  features: Record<string, boolean>;
  issued_at: string;
  period_starts_at: string;
  updates_until: string;
  perpetual: boolean;
  expires_at: string | null;
  grace_days: number;
  grace_ends_at: string | null;
}

// GET license/status con sesión (Fase B).
export interface LicenseStatus {
  state: LicenseState;
  environment: string;
  copied: boolean;
  machine_unverified: boolean;
  identity_unavailable: boolean;
  revocation_reason: RevocationReason | null;
  license: LicenseDetails | null;
  server_id: string | null;
  effective_now: string;
  can_manage: boolean;
  identity_regeneration_available: boolean;
}

// Cupos en uso y límite de la licencia (sessions-summary, Fase D).
export type SeatUsage = Record<LicenseSeat, { in_use: number; limit: number }>;

// --- Estados ---------------------------------------------------------------------------------------------------------

export function notActivated(state: LicenseState | undefined) {
  return state === 'not_activated' || state === 'not_activated_with_data';
}

export const licenseStateLabels: Record<LicenseState, string> = {
  active: 'Vigente',
  grace: 'Tolerancia',
  expired: 'Vencida',
  revoked: 'Revocada',
  not_activated: 'Sin activar',
  not_activated_with_data: 'Sin activar con datos',
};

// Lo que permite cada estado, en palabras del restaurante (tabla de la §6 del contrato).
const stillAllowed =
  'Se puede seguir cobrando lo que está abierto, hacer cortes y consultar reportes.';
const notAllowed = 'No se pueden capturar pedidos nuevos ni abrir un turno sin cuentas pendientes.';

// La copia de otra computadora (§1.1 de la especificación): el texto exacto.
export const copiedInstallationMessage =
  'Esta instalación se copió de otra computadora. Actívala o transfiere la licencia.';

const machineUnverifiedMessage =
  'No se pudo comprobar esta computadora. Reinicia el servidor; si sigue igual, comunícate con soporte.';

// revocationCause es el principio del texto de una licencia revocada, según su motivo.
export function revocationCause(reason: RevocationReason | null | undefined) {
  switch (reason) {
    case 'deactivated':
      return 'La licencia se liberó en esta computadora';
    case 'transferred':
      return 'La licencia se transfirió a otra computadora';
  }
  return 'La licencia del sistema fue revocada';
}

// licenseStateExplanation explica el estado en una o dos frases.
export function licenseStateExplanation(license: {
  state: LicenseState;
  copied?: boolean;
  machine_unverified?: boolean;
  revocation_reason?: RevocationReason | null;
  grace_days_remaining?: number | null;
}) {
  if (license.copied) {
    return copiedInstallationMessage;
  }
  if (license.machine_unverified && notActivated(license.state)) {
    return machineUnverifiedMessage;
  }
  switch (license.state) {
    case 'active':
      return 'La licencia está vigente: se puede usar todo el sistema dentro de sus cupos.';
    case 'grace': {
      const days = license.grace_days_remaining;
      return days === null || days === undefined
        ? 'La licencia venció y está en sus días de tolerancia: todo sigue funcionando. Al terminar ya no se podrán capturar pedidos.'
        : `La licencia venció y está en sus días de tolerancia: ${graceDaysText(days)}. Después ya no se podrán capturar pedidos.`;
    }
    case 'expired':
      return `La licencia del sistema venció. ${stillAllowed} ${notAllowed}`;
    case 'revoked':
      return `${revocationCause(license.revocation_reason)}. ${stillAllowed} ${notAllowed}`;
    case 'not_activated':
      return 'Este sistema no tiene una licencia activada: no se pueden capturar pedidos ni cambiar la administración.';
    case 'not_activated_with_data':
      return 'Este sistema no tiene una licencia activada: no se pueden capturar pedidos ni cambiar la administración. Lo registrado se conserva y se puede consultar en Reportes.';
  }
  return '';
}

export function graceDaysText(days: number) {
  if (days <= 0) {
    return 'hoy es el último día';
  }
  return `${pluralWord(days, 'queda', 'quedan')} ${countLabel(days, 'día', 'días')} antes de que se bloquee la captura`;
}

export function licenseTypeLabel(type: string | null | undefined, billing?: string | null) {
  switch (type) {
    case 'perpetual':
      return 'Perpetua';
    case 'trial':
      return 'Prueba';
    case 'subscription':
      if (billing === 'monthly') return 'Suscripción mensual';
      if (billing === 'annual' || billing === 'yearly') return 'Suscripción anual';
      return 'Suscripción';
  }
  return 'Licencia';
}

// licenseDate es una fecha completa en la zona horaria del restaurante.
export function licenseDate(value: string, timezone: string) {
  try {
    return new Intl.DateTimeFormat('es-MX', {
      dateStyle: 'long',
      timeStyle: 'short',
      timeZone: timezone || undefined,
    }).format(new Date(value));
  } catch {
    return new Intl.DateTimeFormat('es-MX', { dateStyle: 'long', timeStyle: 'short' }).format(
      new Date(value),
    );
  }
}

export function expiryText(
  license: { perpetual: boolean; expires_at: string | null },
  timezone: string,
) {
  if (license.perpetual || !license.expires_at) {
    return 'Perpetua';
  }
  return licenseDate(license.expires_at, timezone);
}

// El entorno solo se muestra en Configuración › Licencia y en las versiones de desarrollo (las que confían en el
// servidor de licencias de pruebas).
export function environmentLabel(environment: string) {
  return environment === 'test' ? 'Servidor de licencias de pruebas' : '';
}

// --- Avisos (§7) -----------------------------------------------------------------------------------------------------

// Días antes del vencimiento en que empieza el aviso de las suscripciones y las pruebas.
export const expiryWarningDays = 7;

export type LicenseNoticeKind =
  'clock' | 'not_activated' | 'expired' | 'revoked' | 'grace' | 'expiring' | 'expiring_indicator';

export interface LicenseNotice {
  kind: LicenseNoticeKind;
  title: string;
  description: string;
  // fixed: no se puede cerrar mientras dure; daily: se cierra y vuelve al día siguiente; indicator: discreto.
  presentation: 'fixed' | 'daily' | 'indicator';
  tone: 'danger' | 'warning' | 'info';
}

// clockTerminalMessage es el texto de las terminales cuando Go rechaza por la hora (§6.2): nunca un error genérico.
export const clockTerminalMessage =
  'No se puede registrar: la computadora principal tiene la fecha y hora incorrectas. Avisa al encargado.';

// licenseNotices decide qué avisos se ven con el estado que dio Go. canManage: «Administrar la licencia».
export function licenseNotices(
  license: LicenseSummary,
  options: { canManage: boolean; mainComputer: boolean },
): LicenseNotice[] {
  const notices: LicenseNotice[] = [];
  // La hora va aparte de la licencia: nadie debe confundir un bloqueo con el otro.
  if (license.clock?.blocked) {
    notices.push({
      kind: 'clock',
      title: options.mainComputer
        ? 'Sistema detenido por la fecha y hora de la computadora'
        : clockTerminalMessage,
      description: options.mainComputer
        ? 'La fecha y hora de esta computadora es anterior a la última actividad registrada. No se puede registrar nada con fecha hasta corregirla; los reportes, el historial y los respaldos siguen disponibles.'
        : 'Los reportes, el historial y los respaldos siguen disponibles.',
      presentation: 'fixed',
      tone: 'danger',
    });
  }
  switch (license.state) {
    case 'not_activated':
    case 'not_activated_with_data':
      notices.push({
        kind: 'not_activated',
        title: license.copied
          ? copiedInstallationMessage
          : 'El sistema no tiene una licencia activada',
        description: license.copied
          ? 'Mientras tanto no se pueden capturar pedidos ni cambiar la administración.'
          : licenseStateExplanation(license),
        presentation: 'fixed',
        tone: 'danger',
      });
      break;
    case 'expired':
      notices.push({
        kind: 'expired',
        title: 'La licencia del sistema venció',
        description: `${stillAllowed} ${notAllowed}`,
        presentation: 'fixed',
        tone: 'danger',
      });
      break;
    case 'revoked':
      notices.push({
        kind: 'revoked',
        title: revocationCause(license.revocation_reason),
        description:
          license.revocation_reason === 'revoked'
            ? `${stillAllowed} ${notAllowed} Comunícate con soporte.`
            : `${stillAllowed} ${notAllowed} Para seguir capturando, activa una licencia en esta computadora.`,
        presentation: 'fixed',
        tone: 'danger',
      });
      break;
    case 'grace':
      notices.push({
        kind: 'grace',
        title: 'La licencia venció y está en sus días de tolerancia',
        description: `${capitalize(graceDaysText(license.grace_days_remaining ?? 0))}. ${
          options.canManage
            ? 'Renuévala para seguir capturando pedidos.'
            : 'Avisa al encargado para que la renueve.'
        }`,
        presentation: 'daily',
        tone: 'warning',
      });
      break;
    case 'active': {
      const days = license.days_until_expiry;
      const expiring =
        !license.perpetual &&
        (license.license_type === 'subscription' || license.license_type === 'trial') &&
        days !== null &&
        days !== undefined &&
        days <= expiryWarningDays;
      if (!expiring) break;
      const when = days <= 0 ? 'hoy' : `en ${countLabel(days, 'día', 'días')}`;
      notices.push(
        options.canManage
          ? {
              kind: 'expiring',
              title: `La licencia vence ${when}`,
              description:
                'Renuévala para que el sistema siga funcionando sin interrupciones. Después de vencer hay unos días de tolerancia.',
              presentation: 'daily',
              tone: 'warning',
            }
          : {
              kind: 'expiring_indicator',
              title: `La licencia vence ${when}`,
              description: '',
              presentation: 'indicator',
              tone: 'info',
            },
      );
      break;
    }
  }
  return notices;
}

function capitalize(text: string) {
  return text.charAt(0).toUpperCase() + text.slice(1);
}

// --- Cupos (§5) ------------------------------------------------------------------------------------------------------

export const seatLabels: Record<LicenseSeat, string> = {
  cash_registers: 'Cajas',
  order_takers: 'Comanderos',
  kitchen_displays: 'Pantallas de cocina',
};

// seatName es el nombre de un cupo en singular, para la columna de Terminales.
export const seatNames: Record<LicenseSeat, string> = {
  cash_registers: 'Caja',
  order_takers: 'Comandero',
  kitchen_displays: 'Pantalla de cocina',
};

export function isLicenseSeat(value: unknown): value is LicenseSeat {
  return typeof value === 'string' && (licenseSeats as string[]).includes(value);
}

// seatUnavailableText es el mensaje sin cupo (§5) a partir de details: cuál cupo, cuántos en uso y el límite.
export function seatUnavailableText(details: {
  seat?: unknown;
  in_use?: unknown;
  limit?: unknown;
}) {
  const seat = isLicenseSeat(details.seat) ? details.seat : 'order_takers';
  const inUse = typeof details.in_use === 'number' ? details.in_use : 0;
  const limit = typeof details.limit === 'number' ? details.limit : 0;
  if (limit <= 0) {
    switch (seat) {
      case 'cash_registers':
        return 'La licencia no incluye cajas (0 de 0). Avisa al encargado.';
      case 'kitchen_displays':
        return 'La licencia no incluye pantallas de cocina (0 de 0): Preparación no está disponible.';
      default:
        return 'La licencia no incluye comanderos (0 de 0): solo se capturan pedidos en una terminal con el turno de caja abierto.';
    }
  }
  const count = `(${inUse} de ${limit} en uso)`;
  switch (seat) {
    case 'cash_registers':
      return `No hay cupo de caja libre ${count}. Se libera al cerrar un turno de caja.`;
    case 'kitchen_displays':
      return `No hay cupo de pantalla de cocina libre ${count}. Pide que otra terminal salga de Preparación.`;
    default:
      return `No hay cupo de comandero libre ${count}. Pide que se cierre la sesión de otra terminal.`;
  }
}

// seatsHeldText son los cupos que ocupa una terminal («Comandero», «Caja y pantalla de cocina» o «—»).
export function seatsHeldText(seats: unknown) {
  const held = (Array.isArray(seats) ? seats : []).filter(isLicenseSeat);
  if (held.length === 0) {
    return '—';
  }
  const names = licenseSeats
    .filter((seat) => held.includes(seat))
    .map((seat, index) => (index === 0 ? seatNames[seat] : seatNames[seat].toLowerCase()));
  return names.length === 1 ? names[0]! : `${names.slice(0, -1).join(', ')} y ${names.at(-1)}`;
}

// releasableSeat: «Liberar cupo» solo aplica al comandero y a la pantalla de cocina (la caja se libera al cerrar el
// turno, como ya existe).
export function releasableSeats(seats: unknown) {
  return (Array.isArray(seats) ? seats : []).filter(
    (seat) => seat === 'order_takers' || seat === 'kitchen_displays',
  ) as LicenseSeat[];
}

// --- Código de la licencia (L1-D4) -----------------------------------------------------------------------------------

// El formato del servidor de licencias: 15 símbolos de este alfabeto en grupos de 3, 4, 4 y 4, con guiones opcionales y
// sin distinguir mayúsculas. Go lo vuelve a validar y lo normaliza antes de enviarlo.
export const licenseKeyAlphabet = '23456789ABCDEFGHJKMNPQRSTUVWXYZ';
const licenseKeyGroups = [3, 4, 4, 4];
const licenseKeyLength = 15;

// formatLicenseKey ordena lo que se escribe: mayúsculas, sin espacios y con los guiones en su lugar.
export function formatLicenseKey(text: string) {
  const compact = text
    .toUpperCase()
    .replace(/[\s-]/g, '')
    .slice(0, licenseKeyLength + 4);
  const parts: string[] = [];
  let start = 0;
  for (const size of licenseKeyGroups) {
    if (start >= compact.length) break;
    parts.push(compact.slice(start, start + size));
    start += size;
  }
  if (start < compact.length) {
    parts[parts.length - 1] += compact.slice(start);
  }
  return parts.join('-');
}

// licenseKeyProblem dice qué le falta al código mientras se escribe; null si ya tiene el formato.
export function licenseKeyProblem(text: string): string | null {
  const compact = text.toUpperCase().replace(/[\s-]/g, '');
  if (compact === '') {
    return 'Escribe el código de la licencia: 15 letras y números en grupos de 3, 4, 4 y 4.';
  }
  const invalid = [...compact].filter((symbol) => !licenseKeyAlphabet.includes(symbol));
  if (invalid.length > 0) {
    const unique = [...new Set(invalid)].join(', ');
    return `El código no usa ${unique}: revisa ese carácter (los códigos no llevan 0, 1, I, L, O ni U).`;
  }
  if (compact.length < licenseKeyLength) {
    const missing = licenseKeyLength - compact.length;
    return `${pluralWord(missing, 'Falta', 'Faltan')} ${countLabel(missing, 'carácter', 'caracteres')}.`;
  }
  if (compact.length > licenseKeyLength) {
    return 'El código tiene caracteres de más: son 15 letras y números.';
  }
  return null;
}

// --- Mensajes por código de error (L1-D17) ---------------------------------------------------------------------------

type Details = Record<string, unknown>;

function detailsOf(value: unknown): Details {
  return value && typeof value === 'object' ? (value as Details) : {};
}

// Qué no se puede hacer, según la fila de la tabla de la §6 que rechazó Go (details.operation).
function blockedOperation(operation: unknown) {
  switch (operation) {
    case 'capture':
      return 'capturar pedidos';
    case 'open_shift':
      return 'abrir turno';
    case 'administration':
      return 'cambiar la administración';
    case 'captured':
      return 'cobrar ni cancelar';
    case 'close_shift':
      return 'cerrar el turno';
    case 'kitchen':
      return 'usar Preparación';
  }
  return 'hacer esta operación';
}

function fileProblem(reason: unknown) {
  switch (reason) {
    case 'other_installation':
      return 'El archivo de licencia es de otra computadora y no se instaló. Tu licencia actual no cambió.';
    case 'stale_revision':
      return 'Ese archivo de licencia es anterior al que ya está instalado y no se usó. Tu licencia actual no cambió.';
    case 'malformed_jws':
    case 'invalid_header':
      return 'El archivo elegido no es un archivo de licencia. Tu licencia actual no cambió.';
  }
  return 'El archivo de licencia no es válido para esta computadora y no se instaló. Tu licencia actual no cambió.';
}

const supportUnlockProblems: Record<string, string> = {
  used: 'Esta respuesta de soporte ya se usó. Pide un código nuevo y envíalo a soporte.',
  other_installation: 'Esta respuesta de soporte es de otra computadora.',
  expired: 'La solicitud venció: vale 24 horas. Pide un código nuevo y envíalo a soporte.',
  altered: 'La respuesta de soporte no es válida. Revisa que se haya copiado completa.',
  not_requested:
    'Esta respuesta no corresponde al código de esta computadora. Pide un código nuevo y envíalo a soporte.',
};

// licenseErrorText es el texto de un rechazo de la licencia, de los cupos o de la hora; null si el código no es de
// la licencia (entonces se muestra el mensaje de Go de siempre).
export function licenseErrorText(code: string, rawDetails: unknown): string | null {
  const details = detailsOf(rawDetails);
  switch (code) {
    // La puerta (Fase C).
    case 'SYSTEM_CLOCK_INVALID':
      return clockTerminalMessage;
    case 'LICENSE_NOT_ACTIVATED':
      if (details.copied === true) return copiedInstallationMessage;
      if (details.machine_unverified === true) return machineUnverifiedMessage;
      if (details.operation === undefined)
        return 'Esta computadora no tiene una licencia activada.';
      return `La licencia del sistema no permite ${blockedOperation(details.operation)}: no hay una licencia activada. Avisa al encargado.`;
    case 'LICENSE_EXPIRED':
      if (details.operation === 'open_shift' && details.pending_accounts === false)
        return 'La licencia del sistema venció: solo se puede abrir turno para cobrar cuentas pendientes, y no hay ninguna. Avisa al encargado.';
      return `La licencia del sistema venció y ya no permite ${blockedOperation(details.operation)}. ${stillAllowed} Avisa al encargado.`;
    case 'LICENSE_REVOKED': {
      const reason = details.reason as RevocationReason | undefined;
      if (details.operation === 'open_shift' && details.pending_accounts === false)
        return `${revocationCause(reason)}: solo se puede abrir turno para cobrar cuentas pendientes, y no hay ninguna. Avisa al encargado.`;
      return `${revocationCause(reason)} y el sistema ya no permite ${blockedOperation(details.operation)}. ${stillAllowed} Avisa al encargado.`;
    }
    // Los cupos (Fase D).
    case 'LICENSE_SEAT_UNAVAILABLE':
      return seatUnavailableText(details);
    case 'LICENSE_SEAT_NOT_HELD': {
      const seats = Array.isArray(details.seats) ? details.seats : [];
      return seats.includes('cash_registers')
        ? 'Esta terminal no ocupa un cupo de comandero ni de pantalla de cocina. Su cupo de caja se libera al cerrar su turno.'
        : 'Esta terminal no ocupa un cupo de comandero ni de pantalla de cocina.';
    }
    // La gestión de la licencia (Fase B).
    case 'LICENSE_KEY_INVALID_FORMAT':
      return 'El código de la licencia no tiene el formato correcto: son 15 letras y números en grupos de 3, 4, 4 y 4.';
    case 'LICENSE_KEY_NOT_FOUND':
      return 'No existe una licencia con ese código. Revisa que esté bien escrito.';
    case 'LICENSE_SERVER_REVOKED':
      return 'Esa licencia fue revocada. Comunícate con soporte.';
    case 'LICENSE_ACTIVATION_LIMIT':
      return details.transfer_available === true
        ? 'La licencia ya está activa en otra computadora. Puedes transferirla a esta computadora.'
        : 'La licencia ya está activa en otra computadora y tiene una transferencia pendiente. Comunícate con soporte.';
    case 'LICENSE_TRANSFER_LIMIT':
      return 'La licencia ya tiene una transferencia pendiente. Comunícate con soporte.';
    case 'LICENSE_INSTALLATION_COPIED':
      return `${copiedInstallationMessage} Antes crea una identidad nueva para esta computadora.`;
    case 'LICENSE_SERVER_BUSY': {
      const seconds =
        typeof details.retry_after_seconds === 'number' ? details.retry_after_seconds : 0;
      const minutes = Math.max(1, Math.ceil(seconds / 60));
      return `El servidor de licencias recibió demasiados intentos. Vuelve a intentarlo en ${countLabel(minutes, 'minuto', 'minutos')}.`;
    }
    case 'LICENSE_SERVER_UNAVAILABLE':
      return details.reason === 'temporary'
        ? 'El servidor de licencias no está disponible en este momento. Tu licencia actual no cambió; inténtalo más tarde.'
        : 'No se pudo conectar con el servidor de licencias. Revisa la conexión a internet o usa la activación sin internet. Tu licencia actual no cambió.';
    case 'LICENSE_SERVER_ERROR':
      return 'El servidor de licencias rechazó la operación. Inténtalo de nuevo; si sigue igual, comunícate con soporte.';
    case 'LICENSE_RESPONSE_INVALID':
      return 'La licencia que envió el servidor de licencias no es válida y no se instaló. Tu licencia actual no cambió.';
    case 'LICENSE_FILE_INVALID':
      return fileProblem(details.reason);
    case 'LICENSE_STATE_NOT_SAVED':
      return 'No se pudo guardar la licencia en esta computadora. No se cambió nada; inténtalo de nuevo.';
    case 'IDENTITY_UNAVAILABLE':
      return details.reason === 'machine_unverified'
        ? machineUnverifiedMessage
        : 'La identidad de esta computadora no está disponible. Comunícate con soporte.';
    case 'IDENTITY_REGENERATION_NOT_NEEDED':
      return 'Esta instalación no se copió de otra computadora: su identidad no se cambia.';
    case 'LICENSE_IDENTITY_LOCAL_ONLY':
      return 'La identidad nueva solo se crea en la computadora principal.';
    // La hora (Fase C).
    case 'LICENSE_LOCAL_ONLY':
      return 'Esto se hace en la computadora principal, en la pantalla de la fecha y hora.';
    case 'LICENSE_TIME_MISMATCH':
      return typeof details.server_time_text === 'string' && details.server_time_text !== ''
        ? `La fecha y hora de esta computadora no coinciden con la de internet. Corrígela a ${details.server_time_text} y vuelve a comprobar.`
        : 'La fecha y hora de esta computadora no coinciden con la de internet. Corrígela y vuelve a comprobar.';
    case 'SUPPORT_UNLOCK_INVALID':
      return (
        supportUnlockProblems[String(details.reason)] ??
        'La respuesta de soporte no es válida. Revisa que se haya copiado completa.'
      );
    case 'SUPPORT_UNLOCK_UNAVAILABLE':
      return 'El desbloqueo de soporte no está disponible en esta versión. Comunícate con soporte.';
  }
  return null;
}

// Rechazos que cambian el estado que se muestra: la sesión vuelve a leer la licencia en cuanto llegan.
export function changesLicenseState(code: string) {
  return (
    code === 'SYSTEM_CLOCK_INVALID' ||
    code === 'LICENSE_NOT_ACTIVATED' ||
    code === 'LICENSE_EXPIRED' ||
    code === 'LICENSE_REVOKED'
  );
}

// Evento de ventana con el que un rechazo de la licencia o de la hora pide volver a leer el estado.
export const licenseChangeEvent = 'adel-license-change';

// --- Pantalla de la fecha y hora (§6.2) ------------------------------------------------------------------------------

export const clockScreenTitle = 'Sistema detenido por la fecha y hora de la computadora';

export function clockWhatHappened(clock: LicenseClock) {
  return {
    local: clock.local_time_text,
    lastActivity: clock.last_activity_time_text,
  };
}

export const clockWhyText =
  'Para proteger tus cortes y reportes: si se registran ventas o cobros con una fecha atrasada, quedarían fuera de orden y los cortes no cuadrarían.';

export const clockLicenseOkText =
  'Tu licencia está bien. Este bloqueo es solo por la fecha y hora.';

// supportCodeGroups separa el código de solicitud en sus grupos para dictarlo por teléfono.
export function supportCodeGroups(code: string) {
  return code.split('-').filter((group) => group !== '');
}
