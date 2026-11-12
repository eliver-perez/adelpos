import type { Operation } from './api';

export interface Printer {
  id: string;
  name: string;
  transport: 'cups' | 'escpos_tcp' | 'agent';
  destination: string;
  port: number;
  paper_width: number;
  copies: number;
  supports_cut: boolean;
  cut: boolean;
  cutter_option: string;
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
}

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

export interface DeviceScope {
  device_id: string;
  device_name: string;
  station_ids: string[];
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
  precheck: 'Precuenta',
};

export const jobLabels: Record<PrintJob['state'], string> = {
  pending: 'Pendiente',
  processing: 'En proceso',
  submitted: 'Aceptado por el transporte',
  error: 'Error confirmado antes del envío',
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
