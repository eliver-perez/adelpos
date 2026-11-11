import { useState } from 'react';
import { IconLabel } from '../Icon';
import { notifyError, notifyInfo, notifySuccess } from '../../services/notify';
import { exportReport } from '../../services/reportExport';
import type { ExportFormat } from '../../services/reportExport';
import type { ReportQuery } from '../../services/reports';

// Exportación de un reporte a PDF o Excel (§17–§18). Go genera el archivo con la misma consulta y
// los mismos permisos que la pantalla, y Rust lo guarda con el diálogo nativo (services/
// reportExport.ts): React solo recibe el resultado (guardado o cancelado, nombre y tamaño). Cada
// botón aparece solo con su permiso (reports.export.pdf / reports.export.excel).

const formatNames: Record<ExportFormat, string> = { pdf: 'PDF', xlsx: 'Excel' };

const sizeFormat = new Intl.NumberFormat('es-MX', { maximumFractionDigits: 1 });

// Tamaño legible del archivo guardado: «850 bytes», «24.5 KB», «1.2 MB».
export function fileSize(bytes: number | undefined) {
  if (bytes === undefined) {
    return '';
  }
  if (bytes < 1024) {
    return `${bytes} bytes`;
  }
  if (bytes < 1024 * 1024) {
    return `${sizeFormat.format(bytes / 1024)} KB`;
  }
  return `${sizeFormat.format(bytes / (1024 * 1024))} MB`;
}

export function ExportButtons({
  reportID,
  title,
  query,
  allowPDF,
  allowExcel,
  disabled = false,
}: {
  reportID: string;
  title: string;
  query: ReportQuery;
  allowPDF: boolean;
  allowExcel: boolean;
  disabled?: boolean;
}) {
  const [running, setRunning] = useState<ExportFormat | null>(null);
  if (!allowPDF && !allowExcel) {
    return null;
  }

  async function run(format: ExportFormat) {
    setRunning(format);
    try {
      const outcome = await exportReport(reportID, format, query);
      if (outcome.result === 'saved') {
        const size = fileSize(outcome.size);
        notifySuccess(`${title}: ${formatNames[format]} guardado.`, {
          description: [outcome.file_name, size].filter(Boolean).join(' · '),
        });
      } else {
        notifyInfo(`Exportación a ${formatNames[format]} cancelada: no se guardó ningún archivo.`);
      }
    } catch (error) {
      notifyError(error, `No se pudo exportar el reporte a ${formatNames[format]}.`);
    } finally {
      setRunning(null);
    }
  }

  return (
    <div className="button-row report-export" role="group" aria-label="Exportar">
      {allowPDF && (
        <button
          type="button"
          className="secondary-button"
          disabled={disabled || running !== null}
          onClick={() => void run('pdf')}
        >
          <IconLabel icon="report">
            {running === 'pdf' ? 'Generando PDF…' : 'Exportar PDF'}
          </IconLabel>
        </button>
      )}
      {allowExcel && (
        <button
          type="button"
          className="secondary-button"
          disabled={disabled || running !== null}
          onClick={() => void run('xlsx')}
        >
          <IconLabel icon="count">
            {running === 'xlsx' ? 'Generando Excel…' : 'Exportar Excel'}
          </IconLabel>
        </button>
      )}
    </div>
  );
}
