import { useRef } from 'react';
import { IconLabel } from '../../components/Icon';
import { SegmentedControl } from '../../components/SegmentedControl';
import type { InventoryUnit } from '../../services/inventory';
import { inventoryQuantityShape } from '../../services/numericInput';
import { countLabel } from '../../services/plural';
import { emptyControlLine } from './controlFormat';
import type { ControlLineDraft, LineDirection } from './controlFormat';
import { DecimalTextField, ItemPicker } from './InventoryFields';
import { measureName, MeasureSelect } from './PurchaseFields';

// Sentido de los renglones de un documento: en un ajuste lo elige quien captura («Faltante» o
// «Sobrante») cuando el motivo admite ambos; en una merma y en una transferencia no se elige.
export interface DirectionChoice {
  // Sentido que impone el motivo; null si admite los dos.
  forced: LineDirection | null;
  labels: Record<LineDirection, string>;
}

function ControlLineEditor({
  line,
  number,
  units,
  removable,
  direction,
  fixedHint,
  onChange,
  onRemove,
}: {
  line: ControlLineDraft;
  number: number;
  units: InventoryUnit[];
  removable: boolean;
  direction: DirectionChoice | null;
  fixedHint: string;
  onChange: (line: ControlLineDraft) => void;
  onRemove: () => void;
}) {
  const captured = measureName(line.item, line.measure, units);
  const current = direction?.forced ?? line.direction;
  return (
    <fieldset className="repeat-card inventory-line control-line" aria-label={`Renglón ${number}`}>
      <div className="section-heading inventory-line-heading">
        <strong>Renglón {number}</strong>
        {removable && (
          <button type="button" className="secondary-button" onClick={onRemove}>
            Quitar renglón
          </button>
        )}
      </div>
      <ItemPicker
        label="Insumo"
        value={line.item}
        activeOnly={false}
        required
        onChange={(item) => onChange({ ...line, item, measure: '' })}
      />
      <div className="form-grid control-line-grid">
        <MeasureSelect
          label="Unidad o presentación"
          item={line.item}
          units={units}
          value={line.measure}
          onChange={(measure) => onChange({ ...line, measure })}
        />
        <DecimalTextField
          label="Cantidad"
          value={line.quantity}
          onChange={(quantity) => onChange({ ...line, quantity })}
          shape={inventoryQuantityShape}
          required
          hint={captured ? `En ${captured}, sin signo.` : 'Sin signo.'}
        />
        {direction ? (
          <div className="field control-direction-field">
            <span>Sentido del ajuste</span>
            <SegmentedControl
              label={`Sentido del renglón ${number}`}
              value={current}
              options={(['out', 'in'] as LineDirection[]).map((value) => ({
                value,
                label: direction.labels[value],
                disabled: direction.forced !== null && direction.forced !== value,
              }))}
              onChange={(value) => onChange({ ...line, direction: value })}
            />
            {direction.forced && (
              <small className="muted">El motivo elegido solo admite este sentido.</small>
            )}
          </div>
        ) : (
          <p className="muted text-xs control-line-fixed">{fixedHint}</p>
        )}
      </div>
    </fieldset>
  );
}

// Renglones de un ajuste, una merma o una transferencia (de 1 a 200, un insumo una sola vez: Go
// lo valida). Cada renglón conserva su clave aunque se quiten otros. Se admiten insumos
// desactivados para ajustar, dar de baja o mover lo que les quede.
export function ControlLinesEditor({
  lines,
  units,
  direction,
  fixedHint,
  onChange,
}: {
  lines: ControlLineDraft[];
  units: InventoryUnit[];
  direction: DirectionChoice | null;
  fixedHint: string;
  onChange: (lines: ControlLineDraft[]) => void;
}) {
  const nextKey = useRef(lines.reduce((highest, line) => Math.max(highest, line.key), 0) + 1);
  return (
    <div className="form-stack">
      {lines.map((line, index) => (
        <ControlLineEditor
          key={line.key}
          line={line}
          number={index + 1}
          units={units}
          removable={lines.length > 1}
          direction={direction}
          fixedHint={fixedHint}
          onChange={(next) =>
            onChange(lines.map((entry) => (entry.key === next.key ? next : entry)))
          }
          onRemove={() => onChange(lines.filter((entry) => entry.key !== line.key))}
        />
      ))}
      <div className="button-row">
        <button
          type="button"
          className="secondary-button"
          disabled={lines.length >= 200}
          onClick={() => onChange([...lines, emptyControlLine(nextKey.current++)])}
        >
          <IconLabel icon="plus">Agregar renglón</IconLabel>
        </button>
        <span className="muted text-xs">{countLabel(lines.length, 'renglón', 'renglones')}</span>
      </div>
    </div>
  );
}
