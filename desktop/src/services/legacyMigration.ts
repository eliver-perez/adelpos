import { Channel, invoke, isTauri } from '@tauri-apps/api/core';
import { api, APIError } from './api';
import { formatMoney } from './money';

// Migración del sistema anterior (etapa 9H, fase D; §5). La pantalla nunca toca archivos: los elige el
// administrador con los diálogos nativos de Rust (desktop/src-tauri/src/legacy_migration.rs), que solo
// devuelve su nombre y su tamaño. Rust prepara el paquete con el lector (la frase de paso es lo único que
// viaja desde aquí, y Rust la entrega por la entrada estándar), lo sube a Go por partes y guarda el
// informe en PDF. El análisis, la importación, el avance y el lote se consultan con las rutas de la Fase C
// (maintenance/legacy/*, solo en la computadora principal y con settings.manage).

export interface LegacyBatch {
  id: string;
  status: 'completed' | 'replaced';
  imported_at: string;
  completed_at: string | null;
  imported_by: string;
  backup_taken_at: string | null;
  fingerprint: string;
  sales: number;
  sales_first_at: string | null;
  sales_last_at: string | null;
  has_differences: boolean;
}

export interface LegacyProgress {
  done: number;
  total: number;
  percent: number;
}

export type LegacyRunStatus = 'queued' | 'running' | 'succeeded' | 'failed' | 'interrupted';

export interface LegacyRun {
  id: string;
  kind: 'analyze' | 'import';
  status: LegacyRunStatus;
  phase: string;
  progress: LegacyProgress | null;
  upload_id: string;
  package_bytes: number;
  analysis_run_id: string | null;
  replace_batch_id: string | null;
  batch_id: string | null;
  backup_file: string | null;
  error_code: string | null;
  error_message: string | null;
  with_differences: boolean;
  summary: LegacyPreview | LegacyReport | null;
  requested_by: string;
  created_at: string;
  started_at: string | null;
  finished_at: string | null;
}

export interface LegacyUpload {
  id: string;
  status: 'receiving' | 'complete';
  size: number;
  sha256: string;
  part_size: number;
  parts: number;
  received_parts: number;
  received_bytes: number;
}

export interface LegacyStatus {
  max_upload_bytes: number;
  part_size: number;
  current_batch: LegacyBatch | null;
  active_run: LegacyRun | null;
  recent_runs: LegacyRun[];
  uploads: LegacyUpload[];
}

export interface LegacyFlag {
  entity: string;
  code: string;
  label: string;
  count: number;
  listed: boolean;
}

export interface LegacySummary {
  package: {
    format_version: number;
    created_at: string | null;
    backup_taken_at: string | null;
    size_bytes: number;
  };
  counts: Record<string, number>;
  sales_period: { first: string; last: string; label: string };
  tax_rates: Record<string, number>;
  totals: {
    sales_by_year: { year: number; amount: string }[] | null;
    payments_by_method_year:
      { method: string; label: string; year: number; amount: string }[] | null;
    cancelled_legacy_style: string;
    withdrawals: string;
    expenses: string;
  };
  flags: LegacyFlag[];
  notes: { code: string; message: string }[];
  warnings: string[];
}

export interface LegacyBlocker {
  code: string;
  message: string;
  count: number;
}

export interface LegacyPreview extends LegacySummary {
  current_batch: LegacyBatch | null;
  will_replace: boolean;
  clean: boolean;
  blockers: LegacyBlocker[] | null;
}

export interface LegacyCheck {
  group: 'counts' | 'sales_by_year' | 'payments_by_method_year' | 'totals';
  key: string;
  label: string;
  kind: 'count' | 'money';
  expected: string;
  actual: string;
  difference: string;
  ok: boolean;
}

export interface LegacyValidation {
  ok: boolean;
  has_money_differences: boolean;
  checks: LegacyCheck[];
  notes: string[] | null;
}

export interface LegacyReport extends LegacySummary {
  batch_id: string;
  replaced_batch_id?: string;
  validation: LegacyValidation;
  backup_file: string;
}

export interface LegacyBatchDetail {
  batch: LegacyBatch;
  report: LegacyReport | null;
  validation: LegacyValidation | null;
}

// Archivo elegido en un diálogo nativo: Rust solo dice su nombre (y su tamaño, si lo tiene).
export interface ChosenFile {
  file_name: string;
  size?: number;
}

export type FileChoice = { result: 'cancelled' } | { result: 'chosen'; file: ChosenFile };

export interface LegacySelection {
  backup: ChosenFile | null;
  output: ChosenFile | null;
  package: ChosenFile | null;
}

// --- Rutas de Go (api_request) ---------------------------------------------------------------

export function readLegacyStatus() {
  return api<LegacyStatus>('maintenance_legacy');
}

export function readLegacyRun(id: string) {
  return api<LegacyRun>('maintenance_legacy_run', { id });
}

export function readLegacyBatch(id: string) {
  return api<LegacyBatchDetail>('maintenance_legacy_batch', { id });
}

export function analyzeLegacyPackage(uploadID: string, passphrase: string) {
  return api<LegacyRun>('maintenance_legacy_analyze', {
    body: { upload_id: uploadID, passphrase },
  });
}

export function importLegacyPackage(input: {
  uploadID: string;
  passphrase: string;
  analysisRunID: string;
  replaceBatchID: string | null;
}) {
  return api<LegacyRun>('maintenance_legacy_import', {
    body: {
      upload_id: input.uploadID,
      passphrase: input.passphrase,
      analysis_run_id: input.analysisRunID,
      confirm: true,
      replace_batch_id: input.replaceBatchID,
    },
  });
}

export function cancelLegacyUpload(id: string) {
  return api<{ cancelled: boolean }>('maintenance_legacy_cancel_upload', { id, body: {} });
}

// --- Operaciones cerradas de Rust (legacy_migration.rs) --------------------------------------

function requireDesktop() {
  if (!isTauri()) {
    throw new APIError(
      'DESKTOP_REQUIRED',
      'La migración del sistema anterior requiere la aplicación de escritorio.',
    );
  }
}

// Un rechazo del puente llega como texto (Rust) y uno de Go como {status, body}, igual que api_request.
async function bridge<T>(command: string, args: Record<string, unknown> = {}): Promise<T> {
  requireDesktop();
  try {
    return await invoke<T>(command, args);
  } catch (error) {
    const message =
      typeof error === 'string' && error.length > 0
        ? error
        : 'No se pudo completar la operación en esta computadora.';
    throw new APIError('BRIDGE_ERROR', message);
  }
}

function goRejection(response: { status: number; body: unknown }, fallback: string) {
  const body = response.body as {
    error?: { code?: string; message?: string; details?: unknown };
  };
  const error = new APIError(
    body.error?.code ?? 'API_ERROR',
    body.error?.message ?? fallback,
    response.status,
    body.error?.details,
  );
  if (
    response.status === 401 &&
    (error.code.startsWith('SESSION_') || error.code.startsWith('DEVICE_'))
  ) {
    window.dispatchEvent(new CustomEvent('session-invalid', { detail: error }));
  }
  return error;
}

export function chooseLegacyBackup() {
  return bridge<FileChoice>('legacy_choose_backup');
}

export function chooseLegacyOutput() {
  return bridge<FileChoice>('legacy_choose_package_output');
}

export function chooseLegacyPackage() {
  return bridge<FileChoice>('legacy_choose_package');
}

export function readLegacySelection() {
  return bridge<LegacySelection>('legacy_selection');
}

// Eventos de la preparación (saneados por Rust: sin rutas ni detalles técnicos).
export type PreparationStep = 'image' | 'start' | 'restore' | 'read' | 'write' | 'cleanup';

export type PreparationEvent =
  | { event: 'step'; step: PreparationStep; status: 'started' | 'finished'; seconds?: number }
  | { event: 'progress'; step: PreparationStep; current: number; total: number }
  | { event: 'table'; status: string; done: number; total: number; rows: number }
  | {
      event: 'result';
      package_bytes: number;
      verified: boolean;
      package_file_name?: string;
      backup: { started_at: string; finished_at: string; sets: number };
      sales: { first: string; last: string } | null;
      tables: { name: string; rows: number }[];
      recorded?: boolean;
    }
  | { event: 'error'; code: string; message: string; recorded?: boolean };

export function startLegacyPreparation(
  passphrase: string,
  onEvent: (event: PreparationEvent) => void,
) {
  requireDesktop();
  const channel = new Channel<PreparationEvent>();
  channel.onmessage = onEvent;
  return bridge<{ started: boolean }>('legacy_prepare_start', { passphrase, channel });
}

// Al volver a entrar al asistente: escucha otra vez la preparación en curso y recibe lo que ya pasó.
export function watchLegacyPreparation(onEvent: (event: PreparationEvent) => void) {
  requireDesktop();
  const channel = new Channel<PreparationEvent>();
  channel.onmessage = onEvent;
  return bridge<{ running: boolean; events: PreparationEvent[] }>('legacy_prepare_watch', {
    channel,
  });
}

export function cancelLegacyPreparation() {
  return bridge<{ cancelling: boolean }>('legacy_prepare_cancel');
}

// Borra lo que haya dejado una preparación terminada a la fuerza (se pide al abrir el asistente).
export function cleanLegacyLeftovers() {
  return bridge<{ result: string }>('legacy_prepare_cleanup');
}

export type UploadOutcome = { result: 'uploaded'; upload: LegacyUpload } | { result: 'cancelled' };

export async function uploadLegacyPackage(
  onProgress: (progress: { sent: number; total: number; percent: number }) => void,
): Promise<UploadOutcome> {
  requireDesktop();
  const channel = new Channel<{ sent: number; total: number; percent: number }>();
  channel.onmessage = onProgress;
  const response = await bridge<{ status: number; body: unknown }>('legacy_upload_package', {
    channel,
  });
  if (response.status >= 400) {
    throw goRejection(response, 'No se pudo subir el paquete.');
  }
  return response.body as UploadOutcome;
}

export function cancelLegacyUploadInProgress() {
  return bridge<{ cancelling: boolean }>('legacy_upload_cancel');
}

export type ReportSave = { result: 'cancelled' } | { result: 'saved'; file_name: string };

export async function saveLegacyReport(batchID: string): Promise<ReportSave> {
  const response = await bridge<{ status: number; body: unknown }>('legacy_save_report', {
    batchId: batchID,
  });
  if (response.status >= 400) {
    throw goRejection(response, 'No se pudo descargar el informe.');
  }
  return response.body as ReportSave;
}

// --- Textos ------------------------------------------------------------------------------------

export const minimumPassphrase = 8;

export function passphraseProblem(passphrase: string, repeated?: string) {
  if ([...passphrase].length < minimumPassphrase) {
    return `La frase de paso debe tener al menos ${minimumPassphrase} caracteres.`;
  }
  if (/[\r\n]/.test(passphrase)) {
    return 'La frase de paso debe escribirse en una sola línea.';
  }
  if (repeated !== undefined && repeated !== passphrase) {
    return 'Las dos frases de paso no coinciden.';
  }
  return '';
}

export const preparationSteps: { step: PreparationStep; label: string }[] = [
  { step: 'image', label: 'Preparar el lector del sistema anterior' },
  { step: 'start', label: 'Iniciar el lector temporal' },
  { step: 'restore', label: 'Abrir el respaldo' },
  { step: 'read', label: 'Leer los datos' },
  { step: 'write', label: 'Guardar el paquete cifrado' },
  { step: 'cleanup', label: 'Borrar el lector temporal' },
];

export const dockerDownloadAddress = 'https://www.docker.com/products/docker-desktop/';

// Mensajes de la preparación por código (los del lector, con lo que tiene que hacer el propietario). Un código que no
// esté aquí muestra el mensaje del lector.
export const preparationErrors: Record<string, { title: string; message: string }> = {
  DOCKER_MISSING: {
    title: 'Falta instalar Docker Desktop',
    message: `Para preparar el paquete hace falta Docker Desktop, que es gratuito. Instálalo desde ${dockerDownloadAddress}, ábrelo, espera a que diga que está listo y vuelve a intentarlo.`,
  },
  DOCKER_NOT_RUNNING: {
    title: 'Docker Desktop no está abierto',
    message: 'Abre Docker Desktop, espera a que diga que está listo y vuelve a intentarlo.',
  },
  DOCKER_UNSUPPORTED: {
    title: 'Docker Desktop necesita un ajuste',
    message:
      'Docker Desktop está usando contenedores de Windows. Cámbialo a contenedores de Linux desde su menú y vuelve a intentarlo.',
  },
  DOCKER_REMOTE: {
    title: 'Docker Desktop apunta a otra computadora',
    message:
      'El lector debe correr en esta computadora: elige el Docker de esta computadora en Docker Desktop y vuelve a intentarlo.',
  },
  READER_MEMORY: {
    title: 'Falta memoria para el lector',
    message:
      'Docker Desktop necesita al menos 2 GB de memoria para el lector. Auméntala en su configuración (Resources) y vuelve a intentarlo.',
  },
  BACKUP_INVALID: {
    title: 'El archivo no es un respaldo válido',
    message:
      'El archivo elegido no es un respaldo del sistema anterior o está dañado. Elige otro respaldo.',
  },
  BACKUP_UNREADABLE: {
    title: 'No se pudo leer el respaldo',
    message:
      'Comprueba que el respaldo exista y que se pueda abrir. Si está en una USB, que siga conectada.',
  },
  ALREADY_RUNNING: {
    title: 'Ya se está preparando un paquete',
    message: 'Espera a que termine la otra preparación en esta computadora y vuelve a intentarlo.',
  },
  IMAGE_UNAVAILABLE: {
    title: 'No se pudo descargar el lector',
    message:
      'La primera vez se descarga el lector del sistema anterior. Revisa la conexión a Internet y vuelve a intentarlo.',
  },
  OUTPUT_EXISTS: {
    title: 'Ya existe un paquete con ese nombre',
    message: 'Elige otro nombre para el paquete y vuelve a intentarlo.',
  },
  CANCELLED: {
    title: 'Preparación cancelada',
    message: 'Se canceló la preparación del paquete. No quedó nada a medias.',
  },
};

export function preparationError(code: string, message: string) {
  return (
    preparationErrors[code] ?? {
      title: 'No se pudo preparar el paquete',
      message: message || 'Ocurrió un error inesperado al preparar el paquete.',
    }
  );
}

// Fases de una corrida (legacyimport/write.go), en palabras del restaurante.
const phaseLabels: Record<string, string> = {
  queued: 'En espera',
  reading: 'Leyendo y comprobando el paquete',
  checking: 'Revisando esta instalación',
  backup: 'Haciendo un respaldo de la base de datos',
  removing_previous: 'Quitando lo importado antes',
  writing_history: 'Guardando el historial de ventas',
  validating: 'Comprobando conteos y totales',
  waiting_locks: 'Esperando a que se desocupen las pantallas de operación',
  writing_catalog: 'Guardando catálogo, mesas y usuarios',
  committing: 'Confirmando la importación',
  done: 'Terminado',
};

export function phaseLabel(phase: string) {
  return phaseLabels[phase] ?? 'Trabajando';
}

export function runFinished(run: LegacyRun) {
  return run.status === 'succeeded' || run.status === 'failed' || run.status === 'interrupted';
}

// Conteos del resumen previo y del informe (llaves de la validación de Go), en el orden del resumen.
export const countLabels: { key: string; singular: string; plural: string }[] = [
  { key: 'products', singular: 'producto', plural: 'productos' },
  { key: 'categories', singular: 'categoría', plural: 'categorías' },
  { key: 'complements', singular: 'complemento', plural: 'complementos' },
  { key: 'areas', singular: 'área', plural: 'áreas' },
  { key: 'tables', singular: 'mesa', plural: 'mesas' },
  { key: 'users', singular: 'usuario', plural: 'usuarios' },
  { key: 'sales', singular: 'venta', plural: 'ventas' },
  { key: 'payments', singular: 'pago', plural: 'pagos' },
  { key: 'cash_shifts', singular: 'corte', plural: 'cortes' },
  { key: 'withdrawals', singular: 'retiro', plural: 'retiros' },
  { key: 'expenses', singular: 'salida de efectivo', plural: 'salidas de efectivo' },
];

const integerFormat = new Intl.NumberFormat('es-MX');

export function formatCount(value: number) {
  return integerFormat.format(value);
}

function plural(count: number, singular: string, words: string) {
  return `${formatCount(count)} ${count === 1 ? singular : words}`;
}

// Frase del resumen: «312 productos, 18 mesas, 6 usuarios, 48 230 ventas del 03/2018 al 11/2026».
export function summarySentence(summary: LegacySummary) {
  const counts = summary.counts;
  const parts = ['products', 'tables', 'users'].map((key) => {
    const label = countLabels.find((entry) => entry.key === key)!;
    return plural(counts[key] ?? 0, label.singular, label.plural);
  });
  const sales = plural(counts.sales ?? 0, 'venta', 'ventas');
  const period = summary.sales_period;
  const range =
    period.first && period.last
      ? ` del ${monthLabel(period.first)} al ${monthLabel(period.last)}`
      : '';
  return `${parts.join(', ')} y ${sales}${range}.`;
}

// «2018-03» → «03/2018».
export function monthLabel(value: string) {
  const match = /^(\d{4})-(\d{2})/.exec(value);
  return match ? `${match[2]}/${match[1]}` : value;
}

// Fecha y hora con el formato de la pantalla.
export function legacyDateTime(value: string | null | undefined) {
  if (!value) {
    return '—';
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return date.toLocaleString('es-MX', { dateStyle: 'medium', timeStyle: 'short' });
}

// Fecha de un evento del lector («2026-11-12T22:05:33.000», hora local del sistema anterior) como día.
export function readerDate(value: string) {
  const match = /^(\d{4})-(\d{2})-(\d{2})/.exec(value);
  return match ? `${match[3]}/${match[2]}/${match[1]}` : value;
}

export function sizeText(bytes: number | undefined) {
  if (bytes === undefined) {
    return '';
  }
  if (bytes < 1024 * 1024) {
    return `${Math.max(1, Math.round(bytes / 1024))} KB`;
  }
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

// Valor de una comprobación (conteo o importe).
export function checkValue(check: LegacyCheck, value: string) {
  return check.kind === 'money' ? formatMoney(value) : formatCount(Number(value));
}

// Comprobaciones que no cuadran, primero las de dinero (nunca se ocultan).
export function failingChecks(validation: LegacyValidation) {
  return validation.checks
    .filter((check) => !check.ok)
    .sort((left, right) => Number(right.kind === 'money') - Number(left.kind === 'money'));
}

export const checkGroupLabels: Record<LegacyCheck['group'], string> = {
  counts: 'Conteos',
  sales_by_year: 'Total vendido por año',
  payments_by_method_year: 'Pagos por forma de pago y año',
  totals: 'Otros totales',
};
