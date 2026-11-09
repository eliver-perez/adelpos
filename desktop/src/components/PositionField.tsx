import { Field } from './Forms';
import { currentAfter } from '../services/positions';
import type { PositionOption } from '../services/positions';

// Selector de orden visual: en lugar de un número, el usuario elige «Al inicio» o «Después de
// …» entre los registros existentes en su orden actual. El propio registro no se ofrece.
export function PositionField({
  label = 'Posición',
  siblings,
  recordID,
  value,
  onChange,
  hint,
}: {
  label?: string;
  siblings: PositionOption[];
  recordID: string;
  value: string | undefined;
  onChange: (after: string) => void;
  hint?: string;
}) {
  const options = siblings.filter((sibling) => sibling.id !== recordID);
  const selected = value ?? currentAfter(siblings, recordID);
  return (
    <Field label={label} hint={hint}>
      <select value={selected} onChange={(event) => onChange(event.target.value)}>
        <option value="">Al inicio</option>
        {options.map((option) => (
          <option key={option.id} value={option.id}>
            Después de «{option.name}»{option.note ? ` · ${option.note}` : ''}
          </option>
        ))}
      </select>
    </Field>
  );
}
