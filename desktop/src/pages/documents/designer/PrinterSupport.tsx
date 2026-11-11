import type { Compatibility, PrinterCompatibility } from '../../../services/documents';

// Lo que puede imprimir cada impresora del diseñador, según Go (documents/printers): formato
// (negrita y tamaños) y logotipo, con su motivo. Son elementos en línea para poder ir dentro del
// botón que elige la impresora.

const statusLabels: Record<Compatibility['status'], string> = {
  supported: 'Admitido',
  unsupported: 'No admitido',
  unknown: 'Sin confirmar',
};

function SupportLine({ label, value }: { label: string; value: Compatibility }) {
  return (
    <span className="designer-support-line">
      <span className="designer-support-status" data-status={value.status}>
        {label}: {statusLabels[value.status]}
      </span>
      {value.reason && <span className="muted"> · {value.reason}</span>}
    </span>
  );
}

export function PrinterSupport({ printer }: { printer: PrinterCompatibility }) {
  return (
    <span className="designer-support">
      <SupportLine label="Negrita y tamaños" value={printer.format} />
      <SupportLine label="Logotipo" value={printer.logo} />
    </span>
  );
}

// Resumen para el editor: si la vista previa usa formato o logotipo, qué impresoras no lo
// reproducirán. El dato «usa formato» sale de la IR de Go (líneas con negrita, tamaño o imagen).
export function PrinterWarnings({
  printers,
  formatted,
  withLogo,
}: {
  printers: PrinterCompatibility[] | null;
  formatted: boolean;
  withLogo: boolean;
}) {
  if (!printers || printers.length === 0 || (!formatted && !withLogo)) {
    return null;
  }
  const limited = printers.filter(
    (printer) =>
      (formatted && printer.format.status !== 'supported') ||
      (withLogo && printer.logo.status !== 'supported'),
  );
  if (limited.length === 0) {
    return null;
  }
  return (
    <div className="designer-preview-notes">
      <h3>Impresoras que no reproducen todo el formato</h3>
      <ul>
        {limited.map((printer) => (
          <li key={printer.id}>
            <strong>{printer.name}</strong>
            <PrinterSupport printer={printer} />
          </li>
        ))}
      </ul>
    </div>
  );
}
