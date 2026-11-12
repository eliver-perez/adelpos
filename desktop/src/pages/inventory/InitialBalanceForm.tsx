import { useRef, useState } from 'react';
import type { FormEvent } from 'react';
import { useConfirmDialog } from '../../components/ConfirmDialog';
import { Field } from '../../components/Forms';
import { Icon, IconLabel } from '../../components/Icon';
import { ToggleButton } from '../../components/ToggleButton';
import { inventoryChangeEvent } from '../../hooks/useInventoryEvents';
import { useResource } from '../../hooks/useResource';
import { recordInitialBalances } from '../../services/inventory';
import type {
  InitialBalanceInput,
  InitialBalanceLineInput,
  InitialBalanceResult,
  InventoryExecute,
  InventoryItem,
  InventoryUnit,
  Warehouse,
} from '../../services/inventory';
import { formatMoney } from '../../services/money';
import { notifyError, notifySuccess, notifyWarning } from '../../services/notify';
import { inventoryCostShape, inventoryQuantityShape } from '../../services/numericInput';
import { countLabel, pluralWord } from '../../services/plural';
import { serverDateTime } from '../../services/restaurant';
import { DecimalTextField, ItemPicker } from './InventoryFields';
import { formatQuantity } from './inventoryFormat';
import { Select } from '../../components/Select';

// Renglón capturado de la existencia inicial. measure vacío es la unidad base del insumo;
// «unit:<id>» otra unidad de la misma dimensión y «presentation:<id>» una presentación.
interface LineDraft {
  key: number;
  item: InventoryItem | null;
  quantity: string;
  measure: string;
  unitCost: string;
  useReference: boolean;
}

function emptyLine(key: number): LineDraft {
  return { key, item: null, quantity: '', measure: '', unitCost: '', useReference: false };
}

// Cuerpo para Go con las cadenas tal como se capturaron: Go convierte la cantidad a la unidad base,
// valida el costo y valora cada renglón.
export function initialBalanceInput(
  warehouseID: string,
  reason: string,
  lines: LineDraft[],
): InitialBalanceInput {
  const input: InitialBalanceInput = {
    warehouse_id: warehouseID,
    lines: lines.map((line) => {
      const body: InitialBalanceLineInput = {
        item_id: line.item?.id ?? '',
        quantity: line.quantity,
      };
      if (line.measure.startsWith('unit:')) {
        body.unit_id = line.measure.slice('unit:'.length);
      }
      if (line.measure.startsWith('presentation:')) {
        body.presentation_id = line.measure.slice('presentation:'.length);
      }
      if (line.useReference) {
        body.use_reference_cost = true;
      } else {
        body.unit_cost = line.unitCost;
      }
      return body;
    }),
  };
  if (reason.trim() !== '') {
    input.reason = reason;
  }
  return input;
}

// Nombre de lo capturado para el costo: «kilogramo», «Caja con 24»…
function measureName(line: LineDraft, units: InventoryUnit[]) {
  const item = line.item;
  if (!item) {
    return '';
  }
  if (line.measure.startsWith('presentation:')) {
    const id = line.measure.slice('presentation:'.length);
    return item.presentations.find((presentation) => presentation.id === id)?.name ?? '';
  }
  if (line.measure.startsWith('unit:')) {
    const id = line.measure.slice('unit:'.length);
    return units.find((unit) => unit.id === id)?.name.toLowerCase() ?? '';
  }
  return item.base_unit.name.toLowerCase();
}

function referenceText(item: InventoryItem) {
  if (!item.costs_visible || !item.reference_cost_display) {
    return '';
  }
  return `${formatMoney(item.reference_cost_display)} por ${item.base_unit.name.toLowerCase()}`;
}

function LineEditor({
  line,
  number,
  units,
  removable,
  onChange,
  onRemove,
}: {
  line: LineDraft;
  number: number;
  units: InventoryUnit[];
  removable: boolean;
  onChange: (line: LineDraft) => void;
  onRemove: () => void;
}) {
  const item = line.item;
  const otherUnits = item
    ? units.filter(
        (unit) =>
          unit.active &&
          unit.dimension === item.base_unit.dimension &&
          unit.id !== item.base_unit.id,
      )
    : [];
  const presentations = item?.presentations.filter((presentation) => presentation.active) ?? [];
  // Sin costos visibles no se sabe si hay costo de referencia: Go lo confirma al registrar.
  const referenceMissing = !!item && item.costs_visible && item.reference_cost === null;
  const captured = measureName(line, units);
  let referenceHint = 'Valora la cantidad al costo de referencia configurado del insumo.';
  if (referenceMissing) {
    referenceHint = 'El insumo no tiene costo de referencia configurado.';
  } else if (item && referenceText(item)) {
    referenceHint = `Costo de referencia: ${referenceText(item)}.`;
  }
  return (
    <fieldset className="repeat-card inventory-line" aria-label={`Renglón ${number}`}>
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
        value={item}
        required
        onChange={(selected) =>
          onChange({ ...line, item: selected, measure: '', useReference: false })
        }
      />
      <div className="form-grid">
        <DecimalTextField
          label="Cantidad"
          value={line.quantity}
          onChange={(quantity) => onChange({ ...line, quantity })}
          shape={inventoryQuantityShape}
          required
        />
        <Field label="Unidad o presentación">
          <Select
            value={line.measure}
            disabled={!item}
            options={[
              {
                value: '',
                label: item
                  ? `${item.base_unit.name} (${item.base_unit.code}) · unidad base`
                  : 'Elige primero el insumo',
              },
              ...otherUnits.map((unit) => ({
                value: `unit:${unit.id}`,
                label: `${unit.name} (${unit.code})`,
              })),
              ...presentations.map((presentation) => ({
                value: `presentation:${presentation.id}`,
                label: `${presentation.name} (${formatQuantity(
                  presentation.base_quantity,
                  item?.base_unit,
                )})`,
              })),
            ]}
            onChange={(measure) => onChange({ ...line, measure })}
          />
        </Field>
        <DecimalTextField
          label="Costo unitario"
          value={line.useReference ? '' : line.unitCost}
          onChange={(unitCost) => onChange({ ...line, unitCost })}
          shape={inventoryCostShape}
          required={!line.useReference}
          disabled={line.useReference}
          hint={
            line.useReference
              ? 'Se usará el costo de referencia.'
              : `Por ${captured || 'unidad capturada'}, sin signo.`
          }
        />
        <div className="inventory-line-reference">
          <ToggleButton
            label="Usar el costo de referencia"
            pressed={line.useReference}
            disabled={!item || referenceMissing}
            onChange={(useReference) => onChange({ ...line, useReference })}
          />
          <small className="muted">{referenceHint}</small>
        </div>
      </div>
    </fieldset>
  );
}

function InitialBalanceSummary({
  result,
  timezone,
  onAnother,
  onClose,
}: {
  result: InitialBalanceResult;
  timezone: string;
  onAnother: () => void;
  onClose: () => void;
}) {
  return (
    <section className="panel form-stack" aria-label="Existencia inicial registrada">
      <div className="section-heading">
        <div>
          <h2>Existencia inicial registrada</h2>
          <p className="muted text-xs">
            {result.warehouse.name} · {serverDateTime(result.occurred_at, timezone)}
            {result.reason ? ` · ${result.reason}` : ''}
          </p>
        </div>
        <div className="button-row">
          <button type="button" className="secondary-button" onClick={onAnother}>
            Registrar otra
          </button>
          <button type="button" className="primary-button" onClick={onClose}>
            Volver a existencias
          </button>
        </div>
      </div>
      {result.warnings.length > 0 && (
        <div className="inventory-negative-notice" role="status">
          <Icon name="alert" />
          <ul>
            {result.warnings.map((warning, index) => (
              <li key={`${warning.code}:${index}`}>{warning.message}</li>
            ))}
          </ul>
        </div>
      )}
      <div className="table-container cash-table">
        <table className="inventory-table">
          <thead>
            <tr>
              <th scope="col">Insumo</th>
              <th scope="col">Capturado</th>
              <th scope="col">En unidad base</th>
              <th scope="col">Existencia después</th>
              {result.costs_visible && <th scope="col">Costo</th>}
              {result.costs_visible && <th scope="col">Valor</th>}
            </tr>
          </thead>
          <tbody>
            {result.lines.map((line) => (
              <tr key={line.line}>
                <td className="inventory-cell-name">
                  <strong>{line.item_name}</strong>
                  <span className="muted text-xs inventory-cell-detail">{line.item_code}</span>
                </td>
                <td>
                  {formatQuantity(line.captured_quantity)} · {line.captured_unit_label}
                </td>
                <td className="inventory-number">{formatQuantity(line.quantity, line.unit)}</td>
                <td className="inventory-number">{formatQuantity(line.stock_after, line.unit)}</td>
                {result.costs_visible && (
                  <td className="inventory-number">
                    {line.use_reference_cost
                      ? 'Costo de referencia'
                      : formatMoney(line.unit_cost_display)}
                  </td>
                )}
                {result.costs_visible && (
                  <td className="inventory-number">{formatMoney(line.value_display)}</td>
                )}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </section>
  );
}

// «Existencia inicial» (inventory.adjustments.authorize + inventory.costs.manage): uno o más
// renglones con insumo, cantidad en la unidad base, en otra unidad de su dimensión o en una
// presentación, y su costo unitario o el costo de referencia confirmado. Solo procede si el insumo no
// tiene movimientos en ese almacén. Se confirma antes de enviar porque el libro no se edita ni se
// borra; usar el costo de referencia se nombra explícitamente en la confirmación.
export function InitialBalanceForm({
  warehouses,
  execute,
  busy,
  timezone,
  onClose,
}: {
  warehouses: Warehouse[];
  execute: InventoryExecute;
  busy: boolean;
  timezone: string;
  onClose: () => void;
}) {
  const available = warehouses.filter((warehouse) => warehouse.active);
  const fallback = available.find((warehouse) => warehouse.is_default) ?? available[0];
  const [warehouseID, setWarehouseID] = useState(fallback?.id ?? '');
  const [reason, setReason] = useState('');
  const nextKey = useRef(2);
  const [lines, setLines] = useState<LineDraft[]>([emptyLine(1)]);
  const [sending, setSending] = useState(false);
  const [result, setResult] = useState<InitialBalanceResult | null>(null);
  const { confirm, dialog } = useConfirmDialog();
  const units = useResource<InventoryUnit[]>('inventory_units', {
    reloadEvent: inventoryChangeEvent,
  });
  const locked = busy || sending;

  function reset() {
    setLines([emptyLine(nextKey.current++)]);
    setReason('');
    setResult(null);
  }

  async function submit(event: FormEvent) {
    event.preventDefault();
    const warehouse = available.find((entry) => entry.id === warehouseID);
    const withReference = lines.filter((line) => line.useReference && line.item);
    const confirmed = await confirm({
      title: 'Registrar existencia inicial',
      message: (
        <>
          <p>
            Se {pluralWord(lines.length, 'registrará', 'registrarán')}{' '}
            {countLabel(lines.length, 'renglón', 'renglones')} en «{warehouse?.name ?? ''}». Los
            movimientos de inventario no se editan ni se borran.
          </p>
          {withReference.length > 0 && (
            <p>
              Se usará el costo de referencia configurado en:{' '}
              {withReference
                .map((line) => {
                  const reference = line.item ? referenceText(line.item) : '';
                  return reference ? `${line.item?.name} (${reference})` : (line.item?.name ?? '');
                })
                .join(', ')}
              .
            </p>
          )}
        </>
      ),
      confirmLabel: 'Registrar existencia inicial',
      cancelLabel: 'Revisar',
    });
    if (!confirmed) {
      return;
    }
    setSending(true);
    try {
      const recorded = await recordInitialBalances(
        execute,
        initialBalanceInput(warehouseID, reason, lines),
      );
      setResult(recorded);
      notifySuccess(
        `Existencia inicial registrada: ${countLabel(recorded.lines.length, 'renglón', 'renglones')} en «${recorded.warehouse.name}».`,
      );
      for (const warning of recorded.warnings) {
        notifyWarning(warning.message);
      }
    } catch (problem) {
      notifyError(problem, 'No se pudo registrar la existencia inicial.');
    } finally {
      setSending(false);
    }
  }

  if (result) {
    return (
      <InitialBalanceSummary
        result={result}
        timezone={timezone}
        onAnother={reset}
        onClose={onClose}
      />
    );
  }

  return (
    <section className="panel form-stack" aria-labelledby="inventory-initial-title">
      <div className="section-heading">
        <div>
          <h2 id="inventory-initial-title">Existencia inicial</h2>
          <p className="muted text-xs">
            Solo para insumos sin movimientos en el almacén elegido. Las diferencias posteriores se
            registran con recepciones o ajustes.
          </p>
        </div>
        <button type="button" className="secondary-button" onClick={onClose}>
          Volver a existencias
        </button>
      </div>
      <form className="form-stack" onSubmit={(event) => void submit(event)}>
        <fieldset className="form-stack" disabled={locked}>
          <div className="form-grid">
            <Field label="Almacén de la existencia inicial">
              <Select
                required
                value={warehouseID}
                options={available.map((warehouse) => ({
                  value: warehouse.id,
                  label: warehouse.name,
                }))}
                onChange={setWarehouseID}
              />
            </Field>
            <Field
              label="Motivo u observación"
              hint="Opcional; por ejemplo, «Inventario de apertura»."
            >
              <input
                maxLength={500}
                value={reason}
                onChange={(event) => setReason(event.target.value)}
              />
            </Field>
          </div>
          {lines.map((line, index) => (
            <LineEditor
              key={line.key}
              line={line}
              number={index + 1}
              units={units.data ?? []}
              removable={lines.length > 1}
              onChange={(next) =>
                setLines((current) =>
                  current.map((entry) => (entry.key === next.key ? next : entry)),
                )
              }
              onRemove={() =>
                setLines((current) => current.filter((entry) => entry.key !== line.key))
              }
            />
          ))}
          <div className="button-row">
            <button
              type="button"
              className="secondary-button"
              disabled={lines.length >= 500}
              onClick={() => setLines((current) => [...current, emptyLine(nextKey.current++)])}
            >
              <IconLabel icon="plus">Agregar renglón</IconLabel>
            </button>
          </div>
        </fieldset>
        <div className="form-actions">
          <button type="submit" className="primary-button" disabled={locked || warehouseID === ''}>
            {sending ? 'Registrando…' : 'Registrar existencia inicial'}
          </button>
          <span className="muted text-xs">{countLabel(lines.length, 'renglón', 'renglones')}</span>
        </div>
      </form>
      {dialog}
    </section>
  );
}
