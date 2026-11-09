import { PrinterIcon, ToggleButton } from '../../components/ToggleButton';

// Interruptor «Imprimir» de una operación o un corte de caja. Solo aparece con el permiso de
// imprimir documentos de caja; la impresión física es opcional y por omisión no se imprime.
export function PrintToggle({
  label,
  pressed,
  onChange,
  can,
  disabled = false,
}: {
  label: string;
  pressed: boolean;
  onChange: (pressed: boolean) => void;
  can: (permission: string) => boolean;
  disabled?: boolean;
}) {
  if (!can('cash.reports.print')) {
    return null;
  }
  return (
    <div className="cash-print-toggle">
      <ToggleButton
        label={label}
        pressed={pressed}
        onChange={onChange}
        disabled={disabled}
        icon={<PrinterIcon />}
      />
      <small className="muted">
        Se imprime en la impresora de tickets de Cobros. Un fallo de impresión no revierte la
        operación.
      </small>
    </div>
  );
}
