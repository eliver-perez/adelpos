import { useState } from 'react';
import { Accordion } from '../../components/Accordion';
import { useConfirmDialog } from '../../components/ConfirmDialog';
import { Field } from '../../components/Forms';
import { IconLabel } from '../../components/Icon';
import { ToggleButton } from '../../components/ToggleButton';
import { inventoryChangeEvent } from '../../hooks/useInventoryEvents';
import { useResource } from '../../hooks/useResource';
import type { InventoryExecute, InventoryItem } from '../../services/inventory';
import { removeSupplierItem, saveSupplierItem } from '../../services/inventoryPurchasing';
import type { SupplierDetail, SupplierItem } from '../../services/inventoryPurchasing';
import { formatMoney } from '../../services/money';
import { notifyError, notifySuccess } from '../../services/notify';
import { countLabel } from '../../services/plural';
import { serverDateTime } from '../../services/restaurant';
import { ItemPicker } from './InventoryFields';
import { formatQuantity } from './inventoryFormat';
import { lastPriceText } from './PurchaseFields';
import {
  orderDateText,
  orderFolio,
  orderStateTone,
  receiptFolio,
  returnFolio,
} from './purchasingFormat';
import type { PurchasingNavigation } from './purchasingNavigation';

// Secciones del panel de un proveedor (fase C): los insumos que surte, con su presentación
// habitual, código, preferido y último precio que entrega Go, y el historial de compras.

function supplierItemDetail(entry: SupplierItem, timezone: string) {
  const parts: string[] = [];
  parts.push(
    entry.presentation
      ? `Presentación habitual: ${entry.presentation.name} (${formatQuantity(entry.presentation.base_quantity, entry.item.unit)})`
      : 'Sin presentación habitual',
  );
  if (entry.supplier_code) {
    parts.push(`Código del proveedor: ${entry.supplier_code}`);
  }
  const price = lastPriceText(entry, timezone);
  if (price) {
    parts.push(price);
  } else {
    parts.push('Sin precio recibido todavía.');
  }
  return parts;
}

// Alta o edición de un insumo surtido: presentación habitual, código del proveedor y preferido.
function SupplierItemForm({
  supplier,
  entry,
  execute,
  busy,
  onSaved,
  onCancel,
}: {
  supplier: SupplierDetail;
  entry: SupplierItem | null;
  execute: InventoryExecute;
  busy: boolean;
  onSaved: (supplier: SupplierDetail) => void;
  onCancel: () => void;
}) {
  const [item, setItem] = useState<InventoryItem | null>(null);
  const loaded = useResource<InventoryItem>('inventory_item', {
    id: entry?.item.id ?? '',
    enabled: entry !== null,
    reloadEvent: inventoryChangeEvent,
  });
  const current = entry ? (loaded.data?.id === entry.item.id ? loaded.data : null) : item;
  const [presentationID, setPresentationID] = useState(entry?.presentation?.id ?? '');
  const [code, setCode] = useState(entry?.supplier_code ?? '');
  const [preferred, setPreferred] = useState(entry?.preferred ?? false);
  const [sending, setSending] = useState(false);
  const presentations =
    current?.presentations.filter(
      (presentation) => presentation.active || presentation.id === presentationID,
    ) ?? [];
  const itemID = entry?.item.id ?? item?.id ?? '';

  async function submit() {
    setSending(true);
    try {
      const saved = await saveSupplierItem(execute, supplier.id, {
        revision: entry?.revision ?? 0,
        item_id: itemID,
        presentation_id: presentationID === '' ? null : presentationID,
        supplier_code: code,
        preferred,
      });
      notifySuccess(
        entry
          ? `Insumo de «${saved.trade_name}» actualizado.`
          : `Insumo agregado a «${saved.trade_name}».`,
      );
      onSaved(saved);
    } catch (problem) {
      notifyError(problem, 'No se pudo guardar el insumo del proveedor.');
    } finally {
      setSending(false);
    }
  }

  return (
    <form
      className="repeat-card form-stack"
      aria-label={entry ? `Insumo surtido ${entry.item.name}` : 'Nuevo insumo surtido'}
      onSubmit={(event) => {
        event.preventDefault();
        void submit();
      }}
    >
      <fieldset className="form-stack" disabled={busy || sending}>
        {entry ? (
          <strong>
            {entry.item.name} · {entry.item.code}
          </strong>
        ) : (
          <ItemPicker
            label="Insumo surtido"
            value={item}
            required
            onChange={(selected) => {
              setItem(selected);
              setPresentationID('');
            }}
          />
        )}
        <div className="form-grid">
          <Field
            label="Presentación habitual"
            hint="Se propone al comprar este insumo al proveedor."
          >
            <select
              value={presentationID}
              disabled={!current}
              onChange={(event) => setPresentationID(event.target.value)}
            >
              <option value="">Sin presentación habitual</option>
              {presentations.map((presentation) => (
                <option key={presentation.id} value={presentation.id}>
                  {presentation.name} (
                  {formatQuantity(presentation.base_quantity, current?.base_unit)})
                </option>
              ))}
            </select>
          </Field>
          <Field label="Código del proveedor" hint="Opcional; como aparece en su factura o lista.">
            <input
              maxLength={60}
              autoComplete="off"
              value={code}
              onChange={(event) => setCode(event.target.value)}
            />
          </Field>
        </div>
        <div className="button-row">
          <ToggleButton
            label="Proveedor preferido de este insumo"
            pressed={preferred}
            onChange={setPreferred}
          />
          <span className="muted text-xs">
            Un solo preferido por insumo: marcarlo aquí quita la marca del anterior.
          </span>
        </div>
      </fieldset>
      <div className="button-row">
        <button
          type="submit"
          className="primary-button"
          disabled={busy || sending || itemID === ''}
        >
          {sending ? 'Guardando…' : entry ? 'Guardar insumo surtido' : 'Agregar insumo surtido'}
        </button>
        <button type="button" className="secondary-button" onClick={onCancel}>
          Cancelar
        </button>
      </div>
    </form>
  );
}

export function SupplierItemsSection({
  supplier,
  editable,
  execute,
  busy,
  timezone,
  onChanged,
}: {
  supplier: SupplierDetail;
  editable: boolean;
  execute: InventoryExecute;
  busy: boolean;
  timezone: string;
  onChanged: (supplier: SupplierDetail) => void;
}) {
  // null: ninguno; '': alta; id del insumo: edición.
  const [editing, setEditing] = useState<string | null>(null);
  const { confirm, dialog } = useConfirmDialog();
  const entry = supplier.items.find((candidate) => candidate.item.id === editing) ?? null;

  async function remove(target: SupplierItem) {
    const confirmed = await confirm({
      title: 'Quitar insumo surtido',
      message: `«${target.item.name}» dejará de figurar entre los insumos de «${supplier.trade_name}» (presentación habitual, código, preferido y último precio). Las compras registradas no cambian; una recepción posterior vuelve a agregarlo.`,
      confirmLabel: 'Quitar insumo',
      cancelLabel: 'Conservar',
      tone: 'danger',
    });
    if (!confirmed) {
      return;
    }
    try {
      const saved = await removeSupplierItem(execute, supplier.id, {
        revision: target.revision,
        item_id: target.item.id,
      });
      notifySuccess(
        `«${target.item.name}» ya no figura entre los insumos de «${saved.trade_name}».`,
      );
      onChanged(saved);
    } catch (problem) {
      notifyError(problem, 'No se pudo quitar el insumo del proveedor.');
    }
  }

  return (
    <section className="inventory-subsection form-stack" aria-label="Insumos que surte">
      <div className="section-heading">
        <div>
          <h3>Insumos que surte</h3>
          <p className="muted text-xs">
            Presentación habitual, código del proveedor y último precio recibido (sin IVA, antes del
            descuento).
          </p>
        </div>
        {editable && editing === null && (
          <button type="button" className="secondary-button" onClick={() => setEditing('')}>
            <IconLabel icon="plus">Agregar insumo surtido</IconLabel>
          </button>
        )}
      </div>
      {editing !== null && (
        <SupplierItemForm
          key={editing}
          supplier={supplier}
          entry={entry}
          execute={execute}
          busy={busy}
          onSaved={(saved) => {
            setEditing(null);
            onChanged(saved);
          }}
          onCancel={() => setEditing(null)}
        />
      )}
      <ul className="inventory-list">
        {supplier.items.map((item) => (
          <li key={item.item.id} className="inventory-list-row">
            <div className="inventory-list-text">
              <strong>
                {item.item.name}
                <span className="muted text-xs"> · {item.item.code}</span>
              </strong>
              {supplierItemDetail(item, timezone).map((text) => (
                <span key={text} className="muted text-xs">
                  {text}
                </span>
              ))}
            </div>
            <div className="inventory-tags">
              {item.preferred && <span className="tag tag-good">Preferido</span>}
              {!item.item.active && <span className="tag tag-muted">Insumo inactivo</span>}
            </div>
            {editable && (
              <div className="button-row">
                <button
                  type="button"
                  className="secondary-button"
                  aria-label={`Editar el insumo surtido ${item.item.name}`}
                  disabled={busy}
                  onClick={() => setEditing(item.item.id)}
                >
                  Editar
                </button>
                <button
                  type="button"
                  className="danger-button"
                  aria-label={`Quitar el insumo surtido ${item.item.name}`}
                  disabled={busy}
                  onClick={() => void remove(item)}
                >
                  Quitar
                </button>
              </div>
            )}
          </li>
        ))}
        {supplier.items.length === 0 && (
          <li className="muted text-sm">Aún no hay insumos asociados a este proveedor.</li>
        )}
      </ul>
      {dialog}
    </section>
  );
}

// Historial del proveedor: conteos y totales de Go y las 20 órdenes, recepciones y devoluciones
// más recientes, cada una con acceso a su detalle.
export function SupplierHistorySection({
  supplier,
  timezone,
  navigation,
}: {
  supplier: SupplierDetail;
  timezone: string;
  navigation: PurchasingNavigation;
}) {
  const history = supplier.history;
  return (
    <Accordion
      title="Historial de compras"
      summary={`${countLabel(history.order_count, 'orden', 'órdenes')} · ${countLabel(history.receipt_count, 'recepción', 'recepciones')} · ${countLabel(history.return_count, 'devolución', 'devoluciones')}`}
      defaultOpen
    >
      <div className="form-stack">
        <div className="inventory-figures">
          <div className="inventory-figure">
            <span className="inventory-figure-label">Total recibido</span>
            <strong>{formatMoney(history.receipts_total)}</strong>
          </div>
          <div className="inventory-figure">
            <span className="inventory-figure-label">Valor devuelto</span>
            <strong>{formatMoney(history.returns_value)}</strong>
          </div>
        </div>
        <div className="table-container cash-table">
          <table className="inventory-table">
            <caption className="sr-only">Órdenes de compra del proveedor</caption>
            <thead>
              <tr>
                <th scope="col">Orden</th>
                <th scope="col">Fecha</th>
                <th scope="col">Estado</th>
                <th scope="col" className="inventory-number">
                  Total
                </th>
                <th scope="col">
                  <span className="sr-only">Acciones</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {history.orders.map((order) => (
                <tr key={order.id}>
                  <td>{orderFolio(order.folio)}</td>
                  <td>{orderDateText(order.order_date)}</td>
                  <td>
                    <span className={orderStateTone(order.state)}>{order.state_label}</span>
                  </td>
                  <td className="inventory-number">{formatMoney(order.total)}</td>
                  <td>
                    <button
                      type="button"
                      className="secondary-button"
                      onClick={() => navigation.openOrder(order.id)}
                    >
                      Ver orden
                    </button>
                  </td>
                </tr>
              ))}
              {history.orders.length === 0 && (
                <tr>
                  <td colSpan={5} className="muted">
                    Sin órdenes de compra.
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
        <div className="table-container cash-table">
          <table className="inventory-table">
            <caption className="sr-only">Recepciones del proveedor</caption>
            <thead>
              <tr>
                <th scope="col">Recepción</th>
                <th scope="col">Fecha</th>
                <th scope="col">Origen</th>
                <th scope="col" className="inventory-number">
                  Total
                </th>
                <th scope="col">
                  <span className="sr-only">Acciones</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {history.receipts.map((receipt) => (
                <tr key={receipt.id}>
                  <td>{receiptFolio(receipt.folio)}</td>
                  <td>{serverDateTime(receipt.occurred_at, timezone)}</td>
                  <td>{receipt.direct ? 'Directa' : orderFolio(receipt.purchase_order_folio)}</td>
                  <td className="inventory-number">{formatMoney(receipt.total)}</td>
                  <td>
                    <button
                      type="button"
                      className="secondary-button"
                      onClick={() => navigation.openReceipt(receipt.id)}
                    >
                      Ver recepción
                    </button>
                  </td>
                </tr>
              ))}
              {history.receipts.length === 0 && (
                <tr>
                  <td colSpan={5} className="muted">
                    Sin recepciones.
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
        {history.returns.length > 0 && (
          <div className="table-container cash-table">
            <table className="inventory-table">
              <caption className="sr-only">Devoluciones al proveedor</caption>
              <thead>
                <tr>
                  <th scope="col">Devolución</th>
                  <th scope="col">Fecha</th>
                  <th scope="col">Motivo</th>
                  <th scope="col" className="inventory-number">
                    Valor
                  </th>
                  <th scope="col">
                    <span className="sr-only">Acciones</span>
                  </th>
                </tr>
              </thead>
              <tbody>
                {history.returns.map((entry) => (
                  <tr key={entry.id}>
                    <td>
                      {returnFolio(entry.folio)}
                      <span className="muted text-xs inventory-cell-detail">
                        {receiptFolio(entry.receipt_folio)}
                      </span>
                    </td>
                    <td>{serverDateTime(entry.occurred_at, timezone)}</td>
                    <td>{entry.reason}</td>
                    <td className="inventory-number">{formatMoney(entry.value)}</td>
                    <td>
                      <button
                        type="button"
                        className="secondary-button"
                        onClick={() => navigation.openReceipt(entry.receipt_id)}
                      >
                        Ver recepción
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </Accordion>
  );
}
