import { api } from './api';

// Identidad del establecimiento para documentos (etapa 9B, fase B2): información adicional, texto
// de encabezado y de pie, y logotipo del restaurante con su presentación en papel térmico. Go es la
// autoridad: valida y sanea los textos, valida y convierte el logotipo a 58 y 80 mm y entrega las
// vistas previas ya convertidas (PNG con exactamente los puntos que se imprimen). Esta capa solo
// llama a las operaciones cerradas del puente.

export type LogoAlign = 'left' | 'center' | 'right';
export type LogoMethod = 'threshold' | 'dither';

export interface IdentityPresentation {
  width_percent: number;
  align: LogoAlign;
  method: LogoMethod;
  threshold: number;
}

export interface IdentityPerson {
  id: string;
  name: string;
}

export interface IdentityEstablishment {
  name: string;
  address: string;
  phone: string;
  email: string;
}

// Conversión del logotipo para un ancho de papel. data_uri solo llega en las lecturas.
export interface IdentityRaster {
  paper_width: number;
  raster_id?: string;
  width_dots: number;
  height_dots: number;
  data_uri?: string;
}

export interface IdentityLogo {
  id: string;
  media_type: string;
  width: number;
  height: number;
  size_bytes: number;
  sha256: string;
  uploaded_at: string;
  uploaded_by: IdentityPerson | null;
  // Original tal como se cargó (solo en la lectura).
  data_uri?: string;
  rasters: IdentityRaster[];
}

export interface IdentityLimits {
  text_max_characters: number;
  text_max_lines: number;
  logo_max_bytes: number;
  logo_max_dimension: number;
  width_percent_min: number;
  width_percent_max: number;
  threshold_min: number;
  threshold_max: number;
  max_height_dots: number;
  paper_widths: { paper_width: number; width_dots: number }[];
}

export interface DocumentIdentity {
  revision: number;
  establishment: IdentityEstablishment;
  additional_info: string;
  header_text: string;
  footer_text: string;
  presentation: IdentityPresentation;
  logo: IdentityLogo | null;
  updated_at: string;
  updated_by: IdentityPerson | null;
  limits: IdentityLimits;
  warnings: string[];
}

// Campos que la persona edita y se guardan juntos con PUT documents/identity.
export interface IdentityFields {
  additional_info: string;
  header_text: string;
  footer_text: string;
  presentation: IdentityPresentation;
}

export interface LogoPreview {
  presentation: IdentityPresentation;
  rasters: IdentityRaster[];
}

// Evento de la aplicación que vuelve a consultar la identidad (lo publica el panel después de una
// escritura propia y, cuando el canal en tiempo real lo reenvía, al llegar
// documents.identity.changed por SSE).
export const identityChangeEvent = 'adel-documents-identity-change';

export const identityPermissions = {
  manage: 'documents.identity.manage',
  readTemplates: 'documents.templates.read',
  readSettings: 'settings.read',
};

export const alignLabels: Record<LogoAlign, string> = {
  left: 'Izquierda',
  center: 'Centro',
  right: 'Derecha',
};

export const methodLabels: Record<LogoMethod, string> = {
  threshold: 'Umbral',
  dither: 'Difuminado',
};

export function readDocumentIdentity() {
  return api<DocumentIdentity>('documents_identity');
}

export function saveDocumentIdentity(
  fields: IdentityFields,
  revision: number,
  operationID: string,
) {
  return api<DocumentIdentity>('documents_save_identity', {
    body: {
      operation_id: operationID,
      revision,
      additional_info: fields.additional_info,
      header_text: fields.header_text,
      footer_text: fields.footer_text,
      presentation: fields.presentation,
    },
  });
}

export function uploadIdentityLogo(contentBase64: string, revision: number, operationID: string) {
  return api<DocumentIdentity>('documents_upload_identity_logo', {
    body: {
      operation_id: operationID,
      revision,
      content_base64: contentBase64,
    },
  });
}

export function removeIdentityLogo(revision: number, operationID: string) {
  return api<DocumentIdentity>('documents_remove_identity_logo', {
    body: {
      operation_id: operationID,
      revision,
    },
  });
}

// Vista previa de otra presentación sin guardarla (no escribe nada en el servidor).
export function previewIdentityLogo(presentation: IdentityPresentation) {
  return api<LogoPreview>('documents_preview_identity_logo', {
    body: { presentation },
  });
}

export function identityFields(identity: DocumentIdentity): IdentityFields {
  return {
    additional_info: identity.additional_info,
    header_text: identity.header_text,
    footer_text: identity.footer_text,
    presentation: { ...identity.presentation },
  };
}

export function samePresentation(left: IdentityPresentation, right: IdentityPresentation) {
  return (
    left.width_percent === right.width_percent &&
    left.align === right.align &&
    left.method === right.method &&
    left.threshold === right.threshold
  );
}

export function sameFields(left: IdentityFields, right: IdentityFields) {
  return (
    left.additional_info === right.additional_info &&
    left.header_text === right.header_text &&
    left.footer_text === right.footer_text &&
    samePresentation(left.presentation, right.presentation)
  );
}

// Contador informativo de lo escrito (caracteres y renglones del campo, no líneas impresas: Go
// corta las líneas al ancho del papel y vuelve a validar los límites al guardar).
export function typedTextStats(value: string) {
  return {
    characters: Array.from(value).length,
    lines: value === '' ? 0 : value.split('\n').length,
  };
}

// Lee un archivo elegido como base64 sin el prefijo data: (como las imágenes del catálogo).
export function readFileAsBase64(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => {
      const content = String(reader.result);
      const comma = content.indexOf(',');
      resolve(comma >= 0 ? content.slice(comma + 1) : '');
    };
    reader.onerror = () => reject(new Error('No se pudo leer el archivo del logotipo.'));
    reader.readAsDataURL(file);
  });
}
