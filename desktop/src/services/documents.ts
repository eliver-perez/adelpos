import { api } from './api';

// Contratos del diseñador de documentos (etapa 9B, fase A; backend/internal/designer e
// internal/documents). Go es la única autoridad de las líneas impresas: valida las plantillas,
// renderiza la representación intermedia (IR) con el ancho real del papel y decide qué bloques
// protegidos se incluyen. React solo muestra las líneas que devuelve Go, nunca corta, alinea ni
// calcula texto impreso. Toda escritura lleva un operation_id que se conserva al reintentar.

export type DocumentKind =
  | 'receipt'
  | 'precheck'
  | 'kitchen_order'
  | 'kitchen_modification'
  | 'kitchen_cancellation'
  | 'cash_income'
  | 'cash_withdrawal'
  | 'cash_expense'
  | 'cash_partial_cut'
  | 'cash_final_cut';

export type DocumentFamily = 'commercial' | 'kitchen' | 'cash';

export type BlockAlign = 'left' | 'center' | 'right';

export type BlockSize = 'normal' | 'double_height' | 'double_width' | 'double';

export type TextSize = 'inherit' | 'normal' | 'large';

export type PaperWidth = 58 | 80;

// Un bloque de la plantilla. options sigue el esquema cerrado de su tipo (CatalogBlock.options);
// Go rechaza claves desconocidas y completa las ausentes con sus valores predeterminados.
export interface TemplateBlock {
  id: string;
  type: string;
  visible: boolean;
  align: BlockAlign;
  size: BlockSize;
  bold: boolean;
  uppercase: boolean;
  spacing_before: number;
  spacing_after: number;
  options: Record<string, unknown>;
}

export interface TemplateLayout {
  options: { text_size: TextSize };
  blocks: TemplateBlock[];
}

export interface FieldItem {
  key: string;
  label: string;
}

export interface OptionValue {
  value: string;
  label: string;
}

export interface OptionSpec {
  key: string;
  label: string;
  type: 'enum' | 'boolean' | 'text' | 'integer' | 'field_items';
  values?: OptionValue[];
  default: unknown;
  max_length?: number;
  max_lines?: number;
  min?: number;
  max?: number;
  required?: boolean;
  // Explicación breve de Go que el editor muestra junto al control.
  help?: string;
}

export type BlockProperty =
  'visible' | 'align' | 'size' | 'bold' | 'uppercase' | 'spacing_before' | 'spacing_after';

export interface CatalogBlock {
  type: string;
  name: string;
  group: 'common' | 'commercial' | 'kitchen' | 'cash';
  description: string;
  properties: BlockProperty[];
  options: OptionSpec[];
  max?: number;
  required: boolean;
  protected: boolean;
}

export interface FieldKeyInfo {
  key: string;
  name: string;
  default_label: string;
}

export interface FieldShortcut {
  name: string;
  options: { items: FieldItem[]; layout: 'inline' | 'pair'; omit_empty: boolean };
}

export interface CatalogKind {
  kind: DocumentKind;
  name: string;
  family: DocumentFamily;
  text_sizes: TextSize[];
  blocks: CatalogBlock[];
  field_keys: FieldKeyInfo[];
  shortcuts: FieldShortcut[];
}

export interface DocumentCatalog {
  kinds: CatalogKind[];
  paper_widths: { paper_width: PaperWidth; columns: number }[];
  properties: {
    align: OptionValue[];
    size: OptionValue[];
    spacing: { min: number; max: number };
  };
  limits: {
    blocks: number;
    bytes: number;
    fixed_text_chars: number;
    fixed_text_lines: number;
    label_chars: number;
    field_items: number;
    spacing: number;
    spacer_lines: number;
  };
}

export interface VersionSummary {
  id: string;
  number: number;
  note: string;
  created_at: string;
  created_by_name: string | null;
  block_count: number;
}

export interface TemplateSummary {
  id: string;
  kind: DocumentKind;
  kind_name: string;
  name: string;
  system: boolean;
  active: boolean;
  revision: number;
  current_version: VersionSummary;
  created_at: string;
  created_by_name: string | null;
  updated_at: string;
  updated_by_name: string | null;
}

export interface TemplateDetail extends TemplateSummary {
  layout: TemplateLayout;
}

export interface VersionDetail extends VersionSummary {
  template_id: string;
  template_name: string;
  kind: DocumentKind;
  layout: TemplateLayout;
}

export interface PreviewLine {
  text: string;
  bold: boolean;
  size: BlockSize;
  // Bloque que produjo la línea; vacío en las líneas que no salen de un bloque.
  block_id: string;
  image: {
    raster_id: string;
    width_dots: number;
    height_dots: number;
    // Vista previa PNG del logotipo convertido; vacía si no existe (fase B).
    data_uri: string;
  } | null;
}

export interface DocumentPreview {
  kind: DocumentKind;
  paper_width: PaperWidth;
  columns: number;
  text_size: 'normal' | 'large';
  sample: boolean;
  includes_expected: boolean;
  lines: PreviewLine[];
  text: string;
  warnings: string[];
  // Bloques visibles que no imprimieron nada en este documento, con el motivo.
  empty_blocks: { block_id: string; reason: string }[];
}

export type PreviewSource = { sample: true } | { ticket_id: string };

export interface RecentDocument {
  ticket_id: string;
  kind: DocumentKind;
  created_at: string;
  label: string;
}

// Compatibilidad de una impresora (documents/printers). code identifica el motivo: escpos_network,
// escpos_disabled (marca ESC/POS desactivada en una impresora CUPS), queue_raw, queue_not_raw (cola
// sin raw), queue_unavailable, queue_timeout o agent; reason lo explica en español.
export interface Compatibility {
  status: 'supported' | 'unsupported' | 'unknown';
  code: string;
  reason: string;
}

export interface PrinterCompatibility {
  id: string;
  name: string;
  transport: 'cups' | 'escpos_tcp' | 'agent';
  paper_width: PaperWidth;
  columns: number;
  format: Compatibility;
  logo: Compatibility;
}

// Resultado de encolar una muestra: el estado del trabajo en la cola, nunca «impreso».
export interface SampleResult {
  status: string;
  job_id: string;
  ticket_id: string;
  printer_id: string;
  message: string;
}

export interface RerenderResult {
  status?: string;
  job_id?: string;
  ticket_id: string;
  template_version_id?: string;
}

export function documentCatalog() {
  return api<DocumentCatalog>('documents_catalog');
}

export function documentTemplates(kind?: DocumentKind) {
  return api<TemplateSummary[]>('documents_templates', kind ? { query: { kind } } : {});
}

export function documentTemplate(id: string) {
  return api<TemplateDetail>('documents_template', { id });
}

// Crea una plantilla del restaurante (inactiva); con sourceVersionID la duplica.
export function createDocumentTemplate(
  operationID: string,
  kind: DocumentKind,
  name: string,
  sourceVersionID?: string,
) {
  const body: Record<string, unknown> = { operation_id: operationID, kind, name };
  if (sourceVersionID) {
    body.source_version_id = sourceVersionID;
  }
  return api<TemplateDetail>('documents_create_template', { body });
}

// Guarda el layout como versión nueva. Un 409 REVISION_CONFLICT indica que otro administrador
// guardó antes: el borrador local se conserva.
export function saveDocumentTemplate(
  operationID: string,
  id: string,
  revision: number,
  layout: TemplateLayout,
  note = '',
) {
  return api<TemplateDetail>('documents_save_template', {
    id,
    body: { operation_id: operationID, revision, layout, note },
  });
}

export function renameDocumentTemplate(
  operationID: string,
  id: string,
  revision: number,
  name: string,
) {
  return api<TemplateDetail>('documents_rename_template', {
    id,
    body: { operation_id: operationID, revision, name },
  });
}

export function activateDocumentTemplate(operationID: string, id: string, revision: number) {
  return api<TemplateDetail>('documents_activate_template', {
    id,
    body: { operation_id: operationID, revision },
  });
}

export function documentTemplateVersions(id: string) {
  return api<VersionSummary[]>('documents_template_versions', { id });
}

export function documentVersion(id: string) {
  return api<VersionDetail>('documents_version', { id });
}

export function restoreDefaultTemplate(operationID: string, kind: DocumentKind) {
  return api<TemplateDetail>('documents_restore_default', {
    body: { operation_id: operationID, kind },
  });
}

// Vista previa sin escrituras: la IR exacta de Go para el layout en edición.
export function previewDocument(
  kind: DocumentKind,
  layout: TemplateLayout,
  paperWidth: PaperWidth,
  source: PreviewSource,
) {
  return api<DocumentPreview>('documents_preview', {
    body: { kind, layout, paper_width: paperWidth, source },
  });
}

export function recentDocuments(kind: DocumentKind) {
  return api<RecentDocument[]>('documents_recent', { query: { kind } });
}

export function printDocumentSample(
  operationID: string,
  kind: DocumentKind,
  layout: TemplateLayout,
  printerID: string,
) {
  return api<SampleResult>('documents_print_sample', {
    body: { operation_id: operationID, kind, layout, printer_id: printerID },
  });
}

export function documentPrinters() {
  return api<PrinterCompatibility[]>('documents_printers');
}

// Impresión de un histórico con la plantilla vigente (acción distinta de la reimpresión exacta).
export function rerenderKitchenJob(
  operationID: string,
  jobID: string,
  reason: string,
  printerID?: string,
) {
  const body: Record<string, unknown> = { operation_id: operationID, job_id: jobID, reason };
  if (printerID) {
    body.printer_id = printerID;
  }
  return api<{
    job: { id: string; state: string };
    ticket_id: string;
    template_version_id: string;
  }>('kitchen_rerender', { body });
}

export function rerenderAccountDocument(operationID: string, accountID: string, ticketID: string) {
  return api<RerenderResult>('payments_rerender', {
    id: accountID,
    body: { operation_id: operationID, ticket_id: ticketID },
  });
}

export function rerenderCashDocument(operationID: string, documentID: string) {
  return api<{
    requested: boolean;
    status: string;
    document_id: string;
    ticket_id: string;
    job_id: string;
    includes_expected: boolean;
    message?: string;
  }>('payments_rerender_cash_document', { id: documentID, body: { operation_id: operationID } });
}

// Permisos del diseñador (registrados en Go; la interfaz solo decide qué botones mostrar).
export const documentPermissions = {
  read: 'documents.templates.read',
  manage: 'documents.templates.manage',
  previewReal: 'documents.preview.real',
  samples: 'documents.samples.print',
  identity: 'documents.identity.manage',
  renderCurrent: 'documents.history.render_current',
} as const;

// Evento de la aplicación que publica useRealtime al recibir documents.templates.changed; detail
// lleva los identificadores de las plantillas que cambiaron.
export const templatesChangeEvent = 'adel-document-templates-change';

export interface TemplatesChange {
  templates: string[];
}
