import { invoke, isTauri } from '@tauri-apps/api/core';
import { APIError } from './api';
import type { ReportQuery } from './reports';

// Exportación de reportes a PDF y Excel (etapa 9A). Go genera el archivo con la misma consulta, los
// mismos permisos y los mismos totales que la pantalla; Rust lo pide con las credenciales que
// custodia, valida su tipo y su firma, abre el diálogo nativo «Guardar como» y lo escribe. El
// WebView nunca recibe los bytes, la ruta completa ni los tokens: solo el resultado.

export type ExportFormat = 'pdf' | 'xlsx';

// Tamaño de papel del PDF (carta por omisión; la orientación la decide Go según el ancho).
export type ExportPaper = 'letter' | 'a4';

export interface ExportOutcome {
  // «cancelled» si la persona cerró el diálogo sin guardar (no es un error).
  result: 'saved' | 'cancelled';
  // Solo el nombre del archivo guardado (sin carpeta) y su tamaño en bytes.
  file_name?: string;
  size?: number;
}

// Parámetros que no aplican a una exportación: el archivo incluye todos los renglones del
// conjunto filtrado, no una página.
const pageParameters = new Set(['page', 'page_size']);

// exportQuery toma la misma consulta de la pantalla (periodo, comparación, orden y filtros) sin la
// paginación y agrega el tamaño de papel del PDF si se indicó.
export function exportQuery(query: ReportQuery, format: ExportFormat, paper?: ExportPaper) {
  const result: ReportQuery = {};
  for (const [key, value] of Object.entries(query)) {
    if (!pageParameters.has(key) && value !== '') {
      result[key] = value;
    }
  }
  if (format === 'pdf' && paper) {
    result.paper = paper;
  }
  return result;
}

export async function exportReport(
  report: string,
  format: ExportFormat,
  query: ReportQuery = {},
  options: { paper?: ExportPaper } = {},
): Promise<ExportOutcome> {
  if (!isTauri()) {
    throw new APIError('DESKTOP_REQUIRED', 'La exportación requiere la aplicación de escritorio.');
  }
  let response: { status: number; body: unknown };
  try {
    response = await invoke('export_report', {
      report,
      format,
      query: exportQuery(query, format, options.paper),
    });
  } catch (error) {
    // Rechazos locales del puente: reporte o parámetros fuera de la lista cerrada, archivo con
    // tipo o firma inválidos, otra exportación en curso o un fallo al escribir el archivo.
    const message = typeof error === 'string' ? error : 'No se pudo exportar el reporte.';
    throw new APIError('EXPORT_FAILED', message);
  }
  if (response.status >= 400) {
    const body = response.body as { error?: { code?: string; message?: string } };
    const error = new APIError(
      body.error?.code ?? 'API_ERROR',
      body.error?.message ?? 'No se pudo exportar el reporte.',
      response.status,
    );
    if (
      response.status === 401 &&
      (error.code.startsWith('SESSION_') || error.code.startsWith('DEVICE_'))
    ) {
      window.dispatchEvent(new CustomEvent('session-invalid', { detail: error }));
    }
    throw error;
  }
  const outcome = response.body as ExportOutcome;
  if (outcome.result !== 'saved' && outcome.result !== 'cancelled') {
    throw new APIError('EXPORT_FAILED', 'El puente nativo no respondió con un resultado válido.');
  }
  return outcome;
}
