import { useState } from 'react';
import { useConfirmDialog } from '../../components/ConfirmDialog';
import { IconLabel } from '../../components/Icon';
import type { InventoryExecute } from '../../services/inventory';
import {
  cancelPurchaseOrder,
  closePurchaseOrder,
  issuePurchaseOrder,
} from '../../services/inventoryPurchasing';
import type { PurchaseOrder } from '../../services/inventoryPurchasing';
import { formatDeduction, formatMoney } from '../../services/money';
import { notifyError, notifySuccess } from '../../services/notify';
import { countLabel } from '../../services/plural';
import { serverDateTime } from '../../services/restaurant';
import { formatQuantity } from './inventoryFormat';
import { ReasonAction } from './PurchaseFields';
import {
  isZeroText,
  orderDateText,
  orderFolio,
  purchasingPermissionName,
  receiptFolio,
} from './purchasingFormat';

// Detalle de una orden de compra (fase C): encabezado, renglones con lo solicitado, recibido y
// pendiente que calcula Go, sus recepciones y las acciones que admite su estado.

// Encabezado de una orden: proveedor, fecha, almacén, responsables y, si aplica, la cancelación o
// el cierre con su motivo.
export function OrderFacts({ order, timezone }: { order: PurchaseOrder; timezone: string }) {
  return (
    <dl className="inventory-facts">
      <div>
        <dt>Proveedor</dt>
        <dd>
          {order.supplier.trade_name}
          {order.supplier.active ? '' : ' (inactivo)'}
        </dd>
      </div>
      <div>
        <dt>Fecha de la orden</dt>
        <dd>{orderDateText(order.order_date)}</dd>
      </div>
      <div>
        <dt>Almacén de destino previsto</dt>
        <dd>{order.expected_warehouse.name}</dd>
      </div>
      <div>
        <dt>Elaboró</dt>
        <dd>{order.created_by_name}</dd>
      </div>
      {order.issued_at && (
        <div>
          <dt>Emitida</dt>
          <dd>
            {serverDateTime(order.issued_at, timezone)}
            {order.issued_by_name ? ` · ${order.issued_by_name}` : ''}
          </dd>
        </div>
      )}
      {order.cancelled_at && (
        <div>
          <dt>Cancelada</dt>
          <dd>
            {serverDateTime(order.cancelled_at, timezone)}
            {order.cancelled_by_name ? ` · ${order.cancelled_by_name}` : ''}
            {order.cancel_reason ? ` · ${order.cancel_reason}` : ''}
          </dd>
        </div>
      )}
      {order.closed_at && (
        <div>
          <dt>Cerrada administrativamente</dt>
          <dd>
            {serverDateTime(order.closed_at, timezone)}
            {order.closed_by_name ? ` · ${order.closed_by_name}` : ''}
            {order.close_reason ? ` · ${order.close_reason}` : ''}
          </dd>
        </div>
      )}
    </dl>
  );
}

// Renglones de una orden con lo solicitado, recibido y pendiente que calcula Go.
export function OrderLinesTable({ order }: { order: PurchaseOrder }) {
  const anyExcess = order.lines.some((line) => !isZeroText(line.excess_quantity));
  return (
    <div className="table-container cash-table">
      <table className="inventory-table">
        <caption className="sr-only">Renglones de la orden</caption>
        <thead>
          <tr>
            <th scope="col">Insumo</th>
            <th scope="col">Presentación o unidad</th>
            <th scope="col" className="inventory-number">
              Solicitado
            </th>
            <th scope="col" className="inventory-number">
              Recibido
            </th>
            <th scope="col" className="inventory-number">
              Pendiente
            </th>
            {anyExcess && (
              <th scope="col" className="inventory-number">
                Excedente
              </th>
            )}
            <th scope="col" className="inventory-number">
              Precio sin IVA
            </th>
            <th scope="col" className="inventory-number">
              Descuento
            </th>
            <th scope="col" className="inventory-number">
              IVA
            </th>
            <th scope="col" className="inventory-number">
              Total
            </th>
          </tr>
        </thead>
        <tbody>
          {order.lines.map((line) => (
            <tr key={line.line_no}>
              <td className="inventory-cell-name">
                <strong>{line.item.name}</strong>
                <span className="muted text-xs inventory-cell-detail">
                  {line.item.code} · {formatQuantity(line.base_quantity, line.item.unit)}
                </span>
              </td>
              <td>{line.capture_label}</td>
              <td className="inventory-number">{formatQuantity(line.quantity)}</td>
              <td className="inventory-number">{formatQuantity(line.received_quantity)}</td>
              <td className="inventory-number">
                <strong>{formatQuantity(line.pending_quantity)}</strong>
                {line.fully_received && (
                  <span className="muted text-xs inventory-cell-detail">Completo</span>
                )}
              </td>
              {anyExcess && (
                <td className="inventory-number">
                  {isZeroText(line.excess_quantity) ? '—' : formatQuantity(line.excess_quantity)}
                </td>
              )}
              <td className="inventory-number">{formatMoney(line.unit_price)}</td>
              <td className="inventory-number">{formatDeduction(line.discount)}</td>
              <td className="inventory-number">
                {formatMoney(line.tax)}
                <span className="muted text-xs inventory-cell-detail">{line.tax_rate} %</span>
              </td>
              <td className="inventory-number">{formatMoney(line.total)}</td>
            </tr>
          ))}
          {order.lines.length === 0 && (
            <tr>
              <td colSpan={anyExcess ? 10 : 9} className="muted">
                La orden no tiene renglones.
              </td>
            </tr>
          )}
        </tbody>
      </table>
    </div>
  );
}

export function OrderReceiptsTable({
  order,
  timezone,
  onOpen,
}: {
  order: PurchaseOrder;
  timezone: string;
  onOpen: (id: string) => void;
}) {
  if (order.receipts.length === 0) {
    return <p className="muted text-sm">Aún no hay recepciones de esta orden.</p>;
  }
  return (
    <div className="table-container cash-table">
      <table className="inventory-table">
        <caption className="sr-only">Recepciones de la orden</caption>
        <thead>
          <tr>
            <th scope="col">Recepción</th>
            <th scope="col">Fecha</th>
            <th scope="col">Almacén</th>
            <th scope="col">Recibió</th>
            <th scope="col">Referencia</th>
            <th scope="col" className="inventory-number">
              Total
            </th>
            <th scope="col">
              <span className="sr-only">Acciones</span>
            </th>
          </tr>
        </thead>
        <tbody>
          {order.receipts.map((receipt) => (
            <tr key={receipt.id}>
              <td>
                {receiptFolio(receipt.folio)}
                {receipt.excess && (
                  <span className="tag tag-warning inventory-inline-tag">Con excedente</span>
                )}
              </td>
              <td>{serverDateTime(receipt.occurred_at, timezone)}</td>
              <td>{receipt.warehouse.name}</td>
              <td>{receipt.received_by_name}</td>
              <td>{receipt.supplier_reference || '—'}</td>
              <td className="inventory-number">{formatMoney(receipt.total)}</td>
              <td>
                <button
                  type="button"
                  className="secondary-button"
                  onClick={() => onOpen(receipt.id)}
                >
                  Ver recepción
                </button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

type OrderAction = 'cancel' | 'close' | null;

// Emitir, cancelar o cerrar administrativamente (inventory.purchases.authorize), según lo que
// admite el estado de la orden (Go entrega issuable, cancellable y closable).
export function OrderActions({
  order,
  can,
  execute,
  busy,
  dirty,
  onChanged,
  onReceive,
}: {
  order: PurchaseOrder;
  can: (permission: string) => boolean;
  execute: InventoryExecute;
  busy: boolean;
  // Un borrador con cambios sin guardar no se emite hasta guardarlos.
  dirty: boolean;
  onChanged: (order: PurchaseOrder) => void;
  onReceive?: () => void;
}) {
  const [action, setAction] = useState<OrderAction>(null);
  const [issuing, setIssuing] = useState(false);
  const { confirm, dialog } = useConfirmDialog();
  const authorize = can('inventory.purchases.authorize');
  const receive = can('inventory.purchases.receive');
  const folio = orderFolio(order.folio);

  async function issue() {
    const confirmed = await confirm({
      title: 'Emitir orden de compra',
      message: `Se emitirá la ${folio.toLowerCase()} a «${order.supplier.trade_name}» por ${formatMoney(order.total)} (${countLabel(order.line_count, 'renglón', 'renglones')}). Emitirla no modifica existencias ni registra gastos de caja; después ya no se edita.`,
      confirmLabel: 'Emitir orden',
      cancelLabel: 'Revisar',
    });
    if (!confirmed) {
      return;
    }
    setIssuing(true);
    try {
      const saved = await issuePurchaseOrder(execute, order.id, { revision: order.revision });
      notifySuccess(`${orderFolio(saved.folio)} emitida a «${saved.supplier.trade_name}».`);
      onChanged(saved);
    } catch (problem) {
      notifyError(problem, 'No se pudo emitir la orden.');
    } finally {
      setIssuing(false);
    }
  }

  async function transition(kind: 'cancel' | 'close', reason: string) {
    try {
      const saved =
        kind === 'cancel'
          ? await cancelPurchaseOrder(execute, order.id, { revision: order.revision, reason })
          : await closePurchaseOrder(execute, order.id, { revision: order.revision, reason });
      notifySuccess(`${orderFolio(saved.folio)}: ${saved.state_label.toLowerCase()}.`);
      setAction(null);
      onChanged(saved);
      return true;
    } catch (problem) {
      notifyError(
        problem,
        kind === 'cancel' ? 'No se pudo cancelar la orden.' : 'No se pudo cerrar la orden.',
      );
      return false;
    }
  }

  const buttons = [];
  if (order.receivable && receive && onReceive) {
    buttons.push(
      <button
        key="receive"
        type="button"
        className="primary-button"
        disabled={busy}
        onClick={onReceive}
      >
        <IconLabel icon="inventory">Registrar recepción</IconLabel>
      </button>,
    );
  }
  if (order.issuable && authorize) {
    buttons.push(
      <button
        key="issue"
        type="button"
        className="primary-button"
        disabled={busy || issuing || dirty}
        onClick={() => void issue()}
      >
        <IconLabel icon="send">{issuing ? 'Emitiendo…' : 'Emitir orden'}</IconLabel>
      </button>,
    );
  }
  if (order.closable && authorize) {
    buttons.push(
      <button
        key="close"
        type="button"
        className="secondary-button"
        disabled={busy}
        onClick={() => setAction('close')}
      >
        <IconLabel icon="lock">Cerrar administrativamente</IconLabel>
      </button>,
    );
  }
  if (order.cancellable && authorize) {
    buttons.push(
      <button
        key="cancel"
        type="button"
        className="danger-button"
        disabled={busy}
        onClick={() => setAction('cancel')}
      >
        <IconLabel icon="ban">Cancelar orden</IconLabel>
      </button>,
    );
  }

  const authorizeHint =
    !authorize && (order.issuable || order.cancellable || order.closable)
      ? `Emitir, cancelar o cerrar requiere el permiso «${purchasingPermissionName('inventory.purchases.authorize')}».`
      : '';
  const receiveHint =
    order.receivable && !receive
      ? `Recibir mercancía requiere el permiso «${purchasingPermissionName('inventory.purchases.receive')}».`
      : '';

  return (
    <div className="form-stack">
      {buttons.length > 0 && <div className="button-row purchase-actions">{buttons}</div>}
      {dirty && order.issuable && authorize && (
        <p className="muted text-xs">Guarda los cambios del borrador antes de emitir la orden.</p>
      )}
      {(authorizeHint || receiveHint) && (
        <p className="muted text-xs">{[authorizeHint, receiveHint].filter(Boolean).join(' ')}</p>
      )}
      {action === 'cancel' && (
        <ReasonAction
          title="Cancelar la orden"
          description={
            order.receipt_count > 0
              ? `Lo ya recibido (${countLabel(order.receipt_count, 'recepción', 'recepciones')}) se conserva con sus entradas; la orden deja de admitir recepciones.`
              : 'La orden deja de admitir recepciones. No modifica existencias ni caja.'
          }
          reasonLabel="Motivo de la cancelación"
          actionLabel="Cancelar orden"
          confirmTitle="Cancelar orden de compra"
          confirmMessage={(reason) =>
            `Se cancelará la ${folio.toLowerCase()} de «${order.supplier.trade_name}» con el motivo «${reason}». Esta acción no se revierte.`
          }
          busy={busy}
          onConfirm={(reason) => transition('cancel', reason)}
          onCancel={() => setAction(null)}
        />
      )}
      {action === 'close' && (
        <ReasonAction
          title="Cerrar administrativamente"
          description="Lo pendiente deja de esperarse: la orden ya no admite recepciones y lo recibido se conserva."
          reasonLabel="Motivo del cierre"
          actionLabel="Cerrar orden"
          confirmTitle="Cerrar orden de compra"
          confirmMessage={(reason) =>
            `Se cerrará administrativamente la ${folio.toLowerCase()} de «${order.supplier.trade_name}» con el motivo «${reason}». Esta acción no se revierte.`
          }
          busy={busy}
          onConfirm={(reason) => transition('close', reason)}
          onCancel={() => setAction(null)}
        />
      )}
      {dialog}
    </div>
  );
}
