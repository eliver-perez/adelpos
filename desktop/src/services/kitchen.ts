import type { Operation } from './api';
import { precheckName } from './documentNames';

export interface Printer {
  id: string;
  name: string;
  transport: 'cups' | 'escpos_tcp' | 'agent';
  destination: string;
  port: number;
  paper_width: number;
  copies: number;
  // Desde la etapa 9F Go guarda supports_cut igual a cut: «Cortar el papel al terminar» es una sola
  // casilla. Se conserva por compatibilidad de la API.
  supports_cut: boolean;
  cut: boolean;
  cutter_option: string;
  // Tipo de corte y renglones de avance antes del corte (etapa 9F, §4.4; lista cerrada 0–8). Una
  // impresora nueva sin ellos recibe corte total y 4 renglones.
  cut_mode: 'full' | 'partial';
  cut_feed_lines: number;
  // Solo en CUPS: la impresora entiende ESC/POS y recibe formato y logotipo si su cola admite datos
  // raw (etapa 9B). Desactivada por omisión; ESC/POS por red ya los recibe.
  accepts_escpos: boolean;
  // Columnas de texto (etapa 9C): null es el predeterminado del papel (42 en 80 mm y 30 en 58 mm);
  // la otra opción de la lista cerrada es 48 o 32. Go valida la lista y la guarda.
  columns: number | null;
  active: boolean;
  fallback_id: string | null;
  revision: number;
}

// Columnas de texto que admite cada ancho de papel: el predeterminado primero (lista cerrada de Go,
// documents.ColumnChoices). Solo sirve para ofrecer las opciones; Go decide y valida.
// Renglones de avance antes del corte (etapa 9F, §4.4): lista cerrada de Go y del CHECK de la 0022.
export const cutFeedLineChoices = [0, 1, 2, 3, 4, 5, 6, 7, 8];
export const defaultCutFeedLines = 4;

// Corte predeterminado de una impresora nueva: activado en 80 mm y desactivado en 58 mm (muchas no
// tienen cortador; el propietario lo activa si la suya lo tiene).
export function defaultCutFor(paperWidth: number) {
  return paperWidth === 80;
}

export const printerColumnChoices: Record<number, [number, number]> = {
  80: [42, 48],
  58: [30, 32],
};

// Columnas efectivas de una impresora: las elegidas o las predeterminadas de su papel.
export function printerColumns(printer: Pick<Printer, 'paper_width' | 'columns'>): number {
  const choices = printerColumnChoices[printer.paper_width] ?? printerColumnChoices[80]!;
  return printer.columns !== null && choices.includes(printer.columns)
    ? printer.columns
    : choices[0];
}

// Respuesta de la prueba de ancho (POST kitchen/printers/{id}/width-test).
export interface WidthTestResult {
  job: { id: string; state: string; last_error: string };
  ticket_id: string;
  paper_width: number;
  columns: number;
}

export interface Station {
  id: string;
  name: string;
  description: string;
  active: boolean;
  sort_order: number;
  reception: 'printer' | 'kds' | 'both';
  printer_id: string | null;
  target_minutes: number;
  warning_minutes: number;
  overdue_minutes: number;
  grouping: 'inherit' | 'independent' | 'unified';
  tracking: 'inherit' | 'item' | 'order';
  revision: number;
}

export interface KitchenSettings {
  grouping: 'independent' | 'unified';
  tracking: 'item' | 'order';
  automatic_release: boolean;
  lead_minutes: number;
  show_prices: boolean;
  show_notes: boolean;
  show_establishment: boolean;
  text_size: 'normal' | 'large';
  line_spacing: number;
  quantity_style: 'prefix' | 'multiplier';
  revision: number;
}

export interface KitchenRoute {
  id: string;
  product_id: string | null;
  category_id: string | null;
  mode: 'prepare' | 'none';
  station_ids: string[];
  revision: number;
}

export interface Release {
  id: string;
  account_id: string;
  order_id: string;
  folio?: number;
  account_name?: string;
  state: 'pending' | 'scheduled' | 'blocked' | 'released' | 'cancelled';
  scheduled_at: string | null;
  due_at: string | null;
  automatic: boolean;
  lead_minutes: number;
  warnings: string[];
  revision: number;
  created_at: string;
}

export interface PreparationPart {
  path: string;
  name: string;
  variant: string;
  quantity: number;
  notes: string;
}

export interface Instructions {
  item_id: string;
  version_id: string;
  name: string;
  quantity: number;
  notes?: string;
  parts: PreparationPart[];
  reason?: string;
}

export interface Task {
  id: string;
  unit_id: string;
  release_id: string;
  station_id: string;
  state: 'released' | 'preparing' | 'ready' | 'cancelled';
  instructions: Instructions;
  revision: number;
  released_at: string;
  folio: number;
  account_name: string;
  table_name: string | null;
  station_name: string;
  warning_minutes: number;
  overdue_minutes: number;
  target_minutes: number;
  elapsed_seconds: number;
  release_revision: number;
  tracking: 'item' | 'order';
}

export interface Board {
  tasks: Task[];
  releases: Release[];
  pending_changes: {
    event_id: string;
    kind: string;
    warnings: string[];
    revision: number;
    created_at: string;
  }[];
  stations: Station[];
  server_time: string;
  timezone: string;
  page: number;
  // Etapa 9F (decisión D6): las estaciones visibles son las de la terminal intersectadas con las del
  // usuario; Go informa la intersección para el aviso sin estaciones en común.
  station_scope: StationScope;
  // Aviso sonoro de esta terminal al llegar una comanda nueva de sus estaciones (desactivado por omisión).
  sound_alert: boolean;
}

// Intersección de estaciones que calcula Go (stationscope.Summary): si hay estaciones en común y, para
// el aviso, qué estaciones muestra la terminal y cuáles atiende el usuario.
export interface StationScope {
  common: boolean;
  all: boolean;
  terminal_all: boolean;
  terminal_stations: string[];
  user_all: boolean;
  user_all_by_permission: boolean;
  user_stations: string[];
}

export interface PrintJob {
  id: string;
  ticket_id: string;
  printer_id: string;
  printer_name: string;
  printer_snapshot: Printer;
  original_job_id: string | null;
  state:
    'pending' | 'processing' | 'submitted' | 'error' | 'uncertain' | 'cancelled' | 'intervention';
  attempts: number;
  last_error: string;
  transport_reference: string;
  revision: number;
  created_at: string;
  // Etapa 9F (§4.3): tipo de documento, su folio o título y la clave del problema si exige atención,
  // para explicar y agrupar los trabajos en la cola sin datos internos.
  ticket_kind?: string;
  folio?: number | null;
  document_title?: string | null;
  problem?: string | null;
}

// Intento de envío de un trabajo, con el resultado guardado por el trabajador (code desde la 9F).
export interface PrintAttempt {
  id: string;
  attempt: number;
  outcome: string | null;
  detail: string | null;
  started_at: string;
  result?: { code?: string; reference?: string } | null;
}

// Nombre del documento de un trabajo de la cola, para el restaurante.
export const jobDocumentLabels: Record<string, string> = {
  normal: 'Comanda',
  modification: 'Aviso de modificación',
  cancellation: 'Aviso de cancelación',
  test: 'Prueba de impresión',
  receipt: 'Ticket de pago',
  get precheck() {
    return precheckName().label;
  },
  cash_report: 'Corte de caja',
  cash_voucher: 'Comprobante de caja',
  cash_drawer: 'Apertura del cajón',
  document_sample: 'Muestra de documento',
  document_rerender: 'Documento con la plantilla vigente',
};

export interface TicketSummary {
  id: string;
  kind: 'normal' | 'modification' | 'cancellation' | 'test' | 'receipt' | 'precheck';
  created_at: string;
  release_id: string;
  folio: number;
  pending_kds: boolean;
}

export interface TicketDocument {
  id: string;
  kind: TicketSummary['kind'];
  created_at: string;
  context: { folio: number; account_name: string; timezone: string };
  sections: { station: Station; items: Instructions[] }[];
  preview: string;
}

// Configuración de cocina de una terminal (GET kitchen/access): desde la etapa 9F, «Estaciones que muestra
// esta terminal» (todas o una selección) y el aviso sonoro, con su pantalla de inicio.
export interface DeviceScope {
  device_id: string;
  device_name: string;
  home_screen: string;
  all_stations: boolean;
  station_ids: string[];
  sound_alert: boolean;
}

// «Estaciones que puede atender» de un usuario (GET users/{id}/kitchen-stations, etapa 9F).
export interface UserStations {
  user_id: string;
  all_stations: boolean;
  station_ids: string[];
  // Con «Operar todas las estaciones del establecimiento» atiende todas aunque tenga una selección.
  all_by_permission: boolean;
  stations: { id: string; name: string; active: boolean }[];
}

export type ExecuteKitchen = <T>(
  operation: Operation,
  options: { id?: string; body?: Record<string, unknown> },
) => Promise<T>;

export const preparationLabels: Record<string, string> = {
  without_items: 'Sin productos confirmados',
  draft: 'Borrador por confirmar',
  confirmed_pending_dispatch: 'Pendiente de liberación',
  // Etapa 9D (§4.4, decisión 10): la liberación quedó bloqueada (por ejemplo, sin ruta de
  // preparación) y se libera a mano en Cocina; el texto ya no suena a error.
  pending_release: 'Enviado · sin confirmar en cocina',
  change_pending: 'Cambio de cocina pendiente de revisión',
  scheduled: 'Programado para preparación',
  released: 'Liberado a preparación',
  preparing: 'En preparación',
  ready: 'Listo',
  cancelled: 'Cancelado',
};

// Tipo de documento en comandas y cola de impresión, incluidos los documentos comerciales.
export const ticketKindLabels: Record<TicketSummary['kind'], string> = {
  normal: 'Comanda',
  modification: 'MODIFICACIÓN',
  cancellation: 'CANCELACIÓN',
  test: 'Prueba autorizada',
  receipt: 'Ticket de pago',
  get precheck() {
    return precheckName().label;
  },
};

// Estados de un trabajo en palabras del restaurante (etapa 9F, §4.3): también los de cada intento,
// que antes se mostraban en inglés («submitted», «intervention»).
export const jobLabels: Record<PrintJob['state'], string> = {
  pending: 'Pendiente',
  processing: 'En proceso',
  submitted: 'Enviado a la impresora',
  error: 'No se pudo imprimir',
  uncertain: 'Resultado incierto · requiere revisión',
  cancelled: 'Cancelado administrativamente',
  intervention: 'Requiere intervención',
};

export function zonedInput(value: string | null, timezone: string) {
  if (!value) return '';
  const parts = Object.fromEntries(
    new Intl.DateTimeFormat('en-CA', {
      timeZone: timezone,
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      hourCycle: 'h23',
    })
      .formatToParts(new Date(value))
      .map((part) => [part.type, part.value]),
  );
  return `${parts.year}-${parts.month}-${parts.day}T${parts.hour}:${parts.minute}`;
}
