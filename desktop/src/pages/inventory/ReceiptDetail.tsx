import { useState } from 'react';
import { Notice } from '../../components/Forms';
import { Icon, IconLabel } from '../../components/Icon';
import { inventoryChangeEvent } from '../../hooks/useInventoryEvents';
import { useResource } from '../../hooks/useResource';
import type { InventoryExecute, InventoryWarning } from '../../services/inventory';
import type { Receipt } from '../../services/inventoryPurchasing';
import { formatDeduction, formatMoney } from '../../services/money';
import { serverDateTime } from '../../services/restaurant';
import { CashLinksSection } from './CashLinksSection';
import { formatQuantity } from './inventoryFormat';
import { DocumentTotals } from './PurchaseFields';
import {
  isZeroText,
  orderFolio,
  purchasingPermissionName,
  receiptFolio,
  returnFolio,
} from './purchasingFormat';
import type { PurchasingNavigation } from './purchasingNavigation';
import { SupplierReturnForm } from './SupplierReturnForm';

function ReceiptFacts({
  receipt,
  timezone,
  navigation,
}: {
  receipt: Receipt;
  timezone: string;
  navigation?: PurchasingNavigation;
}) {
  const orderID = receipt.purchase_order_id;
  return (
    <dl className="inventory-facts">
      <div>
        <dt>Proveedor</dt>
        <dd>
          {receipt.supplier.trade_name}
          {receipt.supplier.kind === 'counter' && (
            <span className="tag inventory-inline-tag">Compra de mostrador</span>
          )}
        </dd>
      </div>
      {receipt.supplier.kind === 'counter' && (
        <div>
          <dt>Comercio</dt>
          <dd>{receipt.counter_store || '—'}</dd>
        </div>
      )}
      <div>
        <dt>Fecha</dt>
        <dd>{serverDateTime(receipt.occurred_at, timezone)}</dd>
      </div>
      <div>
        <dt>Almacén</dt>
        <dd>{receipt.warehouse.name}</dd>
      </div>
      <div>
        <dt>Recibió</dt>
        <dd>{receipt.received_by_name}</dd>
      </div>
      <div>
        <dt>Referencia del proveedor</dt>
        <dd>{receipt.supplier_reference || '—'}</dd>
      </div>
      <div>
        <dt>Origen</dt>
        <dd>
          {receipt.direct || !orderID ? (
            'Recepción directa'
          ) : navigation ? (
            <button
              type="button"
              className="secondary-button"
              onClick={() => navigation.openOrder(orderID)}
            >
              {orderFolio(receipt.purchase_order_folio)}
            </button>
          ) : (
            orderFolio(receipt.purchase_order_folio)
          )}
        </dd>
      </div>
    </dl>
  );
}

// Cantidades por renglón: solicitado, recibido antes, recibido en esta recepción, pendiente
// después, devuelto y existencia del almacén tras el movimiento (todo calculado por Go).
function ReceiptQuantities({ receipt }: { receipt: Receipt }) {
  const fromOrder = !receipt.direct;
  return (
    <div className="table-container cash-table">
      <table className="inventory-table">
        <caption className="sr-only">Cantidades recibidas</caption>
        <thead>
          <tr>
            <th scope="col">Insumo</th>
            <th scope="col">Presentación o unidad</th>
            {fromOrder && (
              <th scope="col" className="inventory-number">
                Solicitado
              </th>
            )}
            {fromOrder && (
              <th scope="col" className="inventory-number">
                Recibido antes
              </th>
            )}
            <th scope="col" className="inventory-number">
              Recibido
            </th>
            {fromOrder && (
              <th scope="col" className="inventory-number">
                Pendiente después
              </th>
            )}
            <th scope="col" className="inventory-number">
              Devuelto
            </th>
            <th scope="col" className="inventory-number">
              Existencia después
            </th>
          </tr>
        </thead>
        <tbody>
          {receipt.lines.map((line) => (
            <tr key={line.line_no}>
              <td className="inventory-cell-name">
                <strong>{line.item.name}</strong>
                <span className="muted text-xs inventory-cell-detail">{line.item.code}</span>
              </td>
              <td>{line.capture_label}</td>
              {fromOrder && (
                <td className="inventory-number">{formatQuantity(line.ordered_quantity)}</td>
              )}
              {fromOrder && (
                <td className="inventory-number">
                  {formatQuantity(line.received_before_quantity)}
                </td>
              )}
              <td className="inventory-number">
                <strong>{formatQuantity(line.quantity)}</strong>
                <span className="muted text-xs inventory-cell-detail">
                  {formatQuantity(line.base_quantity, line.item.unit)}
                </span>
              </td>
              {fromOrder && (
                <td className="inventory-number">{formatQuantity(line.pending_after_quantity)}</td>
              )}
              <td className="inventory-number">
                {isZeroText(line.returned_quantity) ? '—' : formatQuantity(line.returned_quantity)}
              </td>
              <td className="inventory-number">
                {formatQuantity(line.stock_after, line.item.unit)}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

// Importes de compra por renglón (los ve quien lee compras) y el costo recibido en inventario
// (solo con inventory.costs.read: Go entrega null y costs_visible falso si falta).
function ReceiptAmounts({ receipt }: { receipt: Receipt }) {
  const costsVisible = receipt.costs_visible;
  return (
    <div className="table-container cash-table">
      <table className="inventory-table">
        <caption className="sr-only">Importes de la recepción</caption>
        <thead>
          <tr>
            <th scope="col">Insumo</th>
            <th scope="col" className="inventory-number">
              Precio sin IVA
            </th>
            <th scope="col" className="inventory-number">
              Descuento
            </th>
            <th scope="col" className="inventory-number">
              Subtotal
            </th>
            <th scope="col" className="inventory-number">
              IVA
            </th>
            <th scope="col" className="inventory-number">
              Total
            </th>
            {costsVisible && (
              <th scope="col" className="inventory-number">
                Costo recibido
              </th>
            )}
          </tr>
        </thead>
        <tbody>
          {receipt.lines.map((line) => (
            <tr key={line.line_no}>
              <td className="inventory-cell-name">
                <strong>{line.item.name}</strong>
                <span className="muted text-xs inventory-cell-detail">{line.capture_label}</span>
              </td>
              <td className="inventory-number">{formatMoney(line.unit_price)}</td>
              <td className="inventory-number">{formatDeduction(line.discount)}</td>
              <td className="inventory-number">{formatMoney(line.subtotal)}</td>
              <td className="inventory-number">
                {formatMoney(line.tax)}
                <span className="muted text-xs inventory-cell-detail">{line.tax_rate} %</span>
              </td>
              <td className="inventory-number">{formatMoney(line.total)}</td>
              {costsVisible && (
                <td className="inventory-number">
                  {formatMoney(line.inventory_value_display)}
                  {line.unit_cost_display && (
                    <span className="muted text-xs inventory-cell-detail">
                      {formatMoney(line.unit_cost_display)} / {line.item.unit.code}
                    </span>
                  )}
                </td>
              )}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function ReceiptReturns({ receipt, timezone }: { receipt: Receipt; timezone: string }) {
  if (receipt.returns.length === 0) {
    return <p className="muted text-sm">Sin devoluciones al proveedor.</p>;
  }
  return (
    <div className="table-container cash-table">
      <table className="inventory-table">
        <caption className="sr-only">Devoluciones de la recepción</caption>
        <thead>
          <tr>
            <th scope="col">Devolución</th>
            <th scope="col">Fecha</th>
            <th scope="col">Almacén</th>
            <th scope="col">Motivo</th>
            <th scope="col">Responsable</th>
            <th scope="col" className="inventory-number">
              Valor asociado
            </th>
          </tr>
        </thead>
        <tbody>
          {receipt.returns.map((entry) => (
            <tr key={entry.id}>
              <td>{returnFolio(entry.folio)}</td>
              <td>{serverDateTime(entry.occurred_at, timezone)}</td>
              <td>{entry.warehouse.name}</td>
              <td>{entry.reason}</td>
              <td>{entry.actor_name}</td>
              <td className="inventory-number">
                {formatMoney(entry.value)}
                {entry.costs_visible &&
                  entry.unabsorbed_display &&
                  !isZeroText(entry.unabsorbed) && (
                    <span className="muted text-xs inventory-cell-detail">
                      No absorbido: {formatMoney(entry.unabsorbed_display)}
                    </span>
                  )}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

// Detalle de una recepción: lo recibido con solicitado y pendiente, importes y costo recibido,
// IVA incluido o no en el costo (tax_in_cost copiado al recibir), devoluciones al proveedor y
// vínculos con gastos de caja. Se recarga con los eventos de inventario.
export function ReceiptDetail({
  receiptID,
  warnings = [],
  can,
  execute,
  busy,
  timezone,
  closeLabel,
  onClose,
  navigation,
}: {
  receiptID: string;
  warnings?: InventoryWarning[];
  can: (permission: string) => boolean;
  execute: InventoryExecute;
  busy: boolean;
  timezone: string;
  closeLabel: string;
  onClose: () => void;
  navigation?: PurchasingNavigation;
}) {
  const [returning, setReturning] = useState(false);
  const resource = useResource<Receipt>('inventory_receipt', {
    id: receiptID,
    reloadEvent: inventoryChangeEvent,
  });
  const receipt = resource.data?.id === receiptID ? resource.data : null;

  if (receipt && returning) {
    return (
      <SupplierReturnForm
        receipt={receipt}
        execute={execute}
        busy={busy}
        onDone={() => setReturning(false)}
      />
    );
  }

  const title = receipt
    ? `${receiptFolio(receipt.folio)} · ${receipt.supplier.trade_name}`
    : 'Recepción';
  const returnable = receipt?.lines.some((line) => !isZeroText(line.returnable_quantity)) ?? false;
  const canReturn = can('inventory.supplier_returns.record');
  return (
    <section className="panel form-stack" aria-label={title}>
      <div className="section-heading">
        <div>
          <h2>{title}</h2>
          {receipt && (
            <div className="inventory-tags">
              <span className="tag">{receipt.direct ? 'Directa' : 'De una orden de compra'}</span>
              <span className="tag tag-info">
                {receipt.tax_in_cost ? 'IVA incluido en el costo' : 'IVA fuera del costo'}
              </span>
              {receipt.excess && <span className="tag tag-warning">Con excedente</span>}
              {receipt.cash_link_attention && (
                <span className="tag tag-warning">Gasto vinculado corregido</span>
              )}
            </div>
          )}
        </div>
        <button type="button" className="secondary-button" onClick={onClose}>
          {closeLabel}
        </button>
      </div>
      {!receipt && <Notice>{resource.error || 'Consultando la recepción…'}</Notice>}
      {receipt && (
        <>
          <ReceiptFacts receipt={receipt} timezone={timezone} navigation={navigation} />
          {warnings.length > 0 && (
            <div className="inventory-negative-notice" role="status">
              <Icon name="alert" />
              <ul>
                {warnings.map((warning, index) => (
                  <li key={`${warning.code}:${index}`}>{warning.message}</li>
                ))}
              </ul>
            </div>
          )}
          <ReceiptQuantities receipt={receipt} />
          <ReceiptAmounts receipt={receipt} />
          <DocumentTotals
            label="Totales de la recepción"
            subtotal={receipt.subtotal}
            discount={receipt.discount}
            tax={receipt.tax}
            total={receipt.total}
            note={
              receipt.tax_in_cost
                ? 'El costo de inventario de esta recepción incluye el IVA (configuración vigente al recibir).'
                : 'El costo de inventario de esta recepción es el subtotal sin IVA (configuración vigente al recibir).'
            }
          />
          <div className="inventory-figures">
            {receipt.costs_visible && (
              <div className="inventory-figure">
                <span className="inventory-figure-label">Valor de inventario</span>
                <strong>{formatMoney(receipt.inventory_value_display)}</strong>
              </div>
            )}
            <div className="inventory-figure">
              <span className="inventory-figure-label">Devuelto al proveedor</span>
              <strong>{formatMoney(receipt.returned_value)}</strong>
            </div>
            <div className="inventory-figure">
              <span className="inventory-figure-label">Vinculado con gastos</span>
              <strong>{formatMoney(receipt.linked_amount)}</strong>
            </div>
          </div>
          {receipt.notes && (
            <p className="text-sm">
              <strong>Observaciones: </strong>
              {receipt.notes}
            </p>
          )}
          <section
            className="inventory-subsection form-stack"
            aria-label="Devoluciones al proveedor"
          >
            <div className="section-heading">
              <div>
                <h3>Devoluciones al proveedor</h3>
                <p className="muted text-xs">
                  Disminuyen existencias; no generan ingresos de efectivo ni saldo a favor.
                </p>
              </div>
              {canReturn && returnable && (
                <button
                  type="button"
                  className="secondary-button"
                  disabled={busy}
                  onClick={() => setReturning(true)}
                >
                  <IconLabel icon="refund">Registrar devolución</IconLabel>
                </button>
              )}
            </div>
            {!canReturn && returnable && (
              <p className="muted text-xs">
                Registrar devoluciones requiere el permiso «
                {purchasingPermissionName('inventory.supplier_returns.record')}».
              </p>
            )}
            <ReceiptReturns receipt={receipt} timezone={timezone} />
          </section>
          <CashLinksSection
            receipt={receipt}
            can={can}
            execute={execute}
            busy={busy}
            timezone={timezone}
          />
        </>
      )}
    </section>
  );
}
