import { useId, useState } from 'react';
import type { FormEvent } from 'react';
import { useConfirmDialog } from '../../components/ConfirmDialog';
import { Field } from '../../components/Forms';
import { inventoryChangeEvent } from '../../hooks/useInventoryEvents';
import { useResource } from '../../hooks/useResource';
import type { InventoryExecute, Warehouse } from '../../services/inventory';
import { recordSupplierReturn } from '../../services/inventoryPurchasing';
import type { Receipt, SupplierReturnInput } from '../../services/inventoryPurchasing';
import { formatMoney } from '../../services/money';
import { notifyError, notifySuccess, notifyWarning } from '../../services/notify';
import { inventoryQuantityShape } from '../../services/numericInput';
import { validReason } from '../../services/payments';
import { countLabel, pluralWord } from '../../services/plural';
import { DecimalTextField } from './InventoryFields';
import { formatQuantity } from './inventoryFormat';
import { isZeroText, receiptFolio, returnFolio, uncertainResult } from './purchasingFormat';
import { Select } from '../../components/Select';

// Cantidades a devolver por renglón de la recepción (texto tal como se capturó).
type ReturnQuantities = Record<number, string>;

// Cuerpo de la devolución: solo los renglones con cantidad; Go comprueba que no exceda lo
// devolvible (recibido − devuelto) y calcula el valor asociado al costo original.
export function supplierReturnInput(
  receipt: Receipt,
  quantities: ReturnQuantities,
  warehouseID: string,
  reason: string,
): SupplierReturnInput {
  return {
    receipt_id: receipt.id,
    warehouse_id: warehouseID === '' ? null : warehouseID,
    reason: reason.trim(),
    lines: receipt.lines
      .filter((line) => !isZeroText(quantities[line.line_no]))
      .map((line) => ({
        receipt_line_no: line.line_no,
        quantity: quantities[line.line_no] ?? '',
      })),
  };
}

// Devolución a proveedor (inventory.supplier_returns.record) desde una recepción: por renglón, en
// su misma presentación o unidad, nunca más de lo devolvible. Disminuye existencias con un
// movimiento de salida; no crea ingresos de efectivo ni saldo a favor. Se confirma con tono de
// peligro porque no se edita ni se borra.
export function SupplierReturnForm({
  receipt,
  execute,
  busy,
  onDone,
}: {
  receipt: Receipt;
  execute: InventoryExecute;
  busy: boolean;
  onDone: () => void;
}) {
  const reasonID = useId();
  const [quantities, setQuantities] = useState<ReturnQuantities>({});
  const [warehouseID, setWarehouseID] = useState(receipt.warehouse.id);
  const [reason, setReason] = useState('');
  const [sending, setSending] = useState(false);
  const [uncertain, setUncertain] = useState(false);
  const { confirm, dialog } = useConfirmDialog();
  const warehouses = useResource<Warehouse[]>('inventory_warehouses', {
    reloadEvent: inventoryChangeEvent,
  });
  const offered = (warehouses.data ?? []).filter(
    (warehouse) => warehouse.active || warehouse.id === warehouseID,
  );
  const returnable = receipt.lines.filter((line) => !isZeroText(line.returnable_quantity));
  const selected = receipt.lines.filter((line) => !isZeroText(quantities[line.line_no]));
  const locked = busy || sending || uncertain;
  const ready = !locked && selected.length > 0 && validReason(reason);
  const folio = receiptFolio(receipt.folio);

  async function submit(event: FormEvent) {
    event.preventDefault();
    if (!ready) {
      return;
    }
    const warehouseName = offered.find((warehouse) => warehouse.id === warehouseID)?.name ?? '';
    const confirmed = await confirm({
      title: 'Registrar devolución a proveedor',
      message: `Se ${pluralWord(selected.length, 'devolverá', 'devolverán')} ${countLabel(selected.length, 'renglón', 'renglones')} de la ${folio.toLowerCase()} a «${receipt.supplier.trade_name}», con salida del almacén «${warehouseName}». No crea ingresos de efectivo ni saldo a favor. Una devolución no se edita ni se borra.`,
      confirmLabel: 'Registrar devolución',
      cancelLabel: 'Revisar',
      tone: 'danger',
    });
    if (!confirmed) {
      return;
    }
    setSending(true);
    try {
      const recorded = await recordSupplierReturn(
        execute,
        supplierReturnInput(receipt, quantities, warehouseID, reason),
      );
      notifySuccess(
        `${returnFolio(recorded.folio)} registrada: ${countLabel(recorded.line_count, 'renglón', 'renglones')} por ${formatMoney(recorded.value)}.`,
      );
      for (const warning of recorded.warnings) {
        notifyWarning(warning.message);
      }
      onDone();
    } catch (problem) {
      if (uncertainResult(problem)) {
        setUncertain(true);
      }
      notifyError(problem, 'No se pudo registrar la devolución.');
    } finally {
      setSending(false);
    }
  }

  return (
    <section className="panel form-stack" aria-label={`Devolución de la ${folio.toLowerCase()}`}>
      <div className="section-heading">
        <div>
          <h2>Devolución a proveedor · {folio}</h2>
          <p className="muted text-xs">
            {receipt.supplier.trade_name} · el valor se calcula al costo original de la recepción
          </p>
        </div>
        <button type="button" className="secondary-button" onClick={onDone}>
          Volver a la recepción
        </button>
      </div>
      {uncertain && (
        <div className="inventory-negative-notice" role="status">
          <p>
            No se pudo confirmar si la devolución quedó registrada. Recupérala con «Reintentar sin
            duplicar» (arriba) antes de capturarla de nuevo.
          </p>
        </div>
      )}
      <form className="form-stack" onSubmit={(event) => void submit(event)}>
        <fieldset className="form-stack" disabled={locked}>
          <Field label="Almacén de la salida" hint="Por omisión, el de la recepción.">
            <Select
              required
              value={warehouseID}
              options={offered.map((warehouse) => ({
                value: warehouse.id,
                label: warehouse.name,
              }))}
              onChange={setWarehouseID}
            />
          </Field>
          {returnable.map((line) => (
            <fieldset
              key={line.line_no}
              className="repeat-card inventory-line"
              aria-label={`Devolución de ${line.item.name}`}
            >
              <div className="section-heading inventory-line-heading">
                <strong>
                  {line.item.name}
                  <span className="muted text-xs"> · {line.capture_label}</span>
                </strong>
              </div>
              <dl className="inventory-facts">
                <div>
                  <dt>Recibido</dt>
                  <dd>{formatQuantity(line.quantity)}</dd>
                </div>
                <div>
                  <dt>Devuelto antes</dt>
                  <dd>{formatQuantity(line.returned_quantity)}</dd>
                </div>
                <div>
                  <dt>Devolvible</dt>
                  <dd>
                    <strong>{formatQuantity(line.returnable_quantity)}</strong>
                  </dd>
                </div>
              </dl>
              <DecimalTextField
                label="Cantidad a devolver"
                value={quantities[line.line_no] ?? ''}
                onChange={(value) =>
                  setQuantities((current) => ({ ...current, [line.line_no]: value }))
                }
                shape={inventoryQuantityShape}
                hint={`En ${line.capture_label}; hasta ${formatQuantity(line.returnable_quantity)}. Vacío: no se devuelve.`}
              />
            </fieldset>
          ))}
          {returnable.length === 0 && (
            <p className="muted text-sm">Ya se devolvió todo lo recibido en esta recepción.</p>
          )}
          <div className="field">
            <label htmlFor={reasonID}>Motivo de la devolución</label>
            <textarea
              id={reasonID}
              rows={2}
              maxLength={500}
              required
              aria-describedby={`${reasonID}-hint`}
              value={reason}
              onChange={(event) => setReason(event.target.value)}
            />
            <small id={`${reasonID}-hint`} className="muted">
              Entre 5 y 500 caracteres; por ejemplo, «Producto dañado en el transporte».
            </small>
          </div>
        </fieldset>
        <div className="form-actions">
          <button type="submit" className="danger-solid-button" disabled={!ready}>
            {sending ? 'Registrando…' : 'Registrar devolución'}
          </button>
          <span className="muted text-xs">
            {countLabel(selected.length, 'renglón por devolver', 'renglones por devolver')}
          </span>
        </div>
      </form>
      {dialog}
    </section>
  );
}
