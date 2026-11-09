import { shiftStateLabels } from '../../services/cash';
import type { RegisterShift } from '../../services/cash';

// Selector de caja. Por omisión, la de esta terminal; las demás solo aparecen si Go las entrega
// (cash.shifts.read_all). Botones táctiles, sin identificadores internos.
export function RegisterPicker({
  registers,
  selectedID,
  onSelect,
}: {
  registers: RegisterShift[];
  selectedID: string;
  onSelect: (id: string) => void;
}) {
  if (registers.length < 2) {
    return null;
  }
  return (
    <div className="cash-choice-grid cash-register-picker" role="group" aria-label="Caja">
      {registers.map((entry) => {
        const state = entry.shift ? shiftStateLabels[entry.shift.state] : 'Sin turno';
        return (
          <button
            type="button"
            key={entry.register.id}
            className={selectedID === entry.register.id ? 'cash-choice-selected' : ''}
            aria-pressed={selectedID === entry.register.id}
            onClick={() => onSelect(entry.register.id)}
          >
            <strong>{entry.register.name}</strong>
            <span className="muted text-xs">
              {entry.own ? 'Esta terminal · ' : ''}
              {state}
            </span>
          </button>
        );
      })}
    </div>
  );
}
