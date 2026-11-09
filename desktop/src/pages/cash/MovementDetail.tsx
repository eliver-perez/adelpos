import { useState } from 'react';
import type { CashOperation } from '../../services/cash';
import { formatDollars, formatMoney } from '../../services/payments';
import type { Shift } from '../../services/payments';
import { serverDateTime } from '../../services/restaurant';
import { CorrectionForm } from './CorrectionForm';
import { currencyAmount } from './DenominationCount';
import type { MovementRow } from './movementRows';
import { authorizationText } from './RecordedOperation';

function DetailItem({ label, value }: { label: string; value: string }) {
  if (!value) {
    return null;
  }
  return (
    <div className="detail-row">
      <dt>{label}</dt>
      <dd>{value}</dd>
    </div>
  );
}

// Describe la operación enlazada (la que corrige o la que la corrigió) sin mostrar su
// identificador: con su tipo, concepto y hora si está en el mismo turno.
function linkedText(id: string | null, operations: CashOperation[], timezone: string) {
  if (!id) {
    return '';
  }
  const linked = operations.find((operation) => operation.id === id);
  if (!linked) {
    return 'Un movimiento de otro turno de la misma caja';
  }
  return `${linked.concept} · ${currencyAmount(linked.currency, linked.amount)} · ${serverDateTime(linked.created_at, timezone)}`;
}

// La corrección se registra en el turno abierto de la caja de esta terminal: corrección si la
// original es de ese turno; regularización si pertenece a un turno cerrado de la misma caja.
export function correctionTarget(
  operation: CashOperation,
  ownShift: Shift | null,
  can: (permission: string) => boolean,
) {
  if (!ownShift || ownShift.state !== 'open' || !can('cash.corrections.record')) {
    return null;
  }
  if (operation.status !== 'recorded' || operation.register_id !== ownShift.register_id) {
    return null;
  }
  return ownShift;
}

function OperationFacts({
  operation,
  operations,
  timezone,
}: {
  operation: CashOperation;
  operations: CashOperation[];
  timezone: string;
}) {
  const equivalent =
    operation.currency === 'USD' && operation.usd_rate
      ? `${formatMoney(operation.equivalent_mxn)} a ${formatMoney(operation.usd_rate)} por dólar`
      : '';
  return (
    <dl className="cash-detail-list">
      <DetailItem
        label="Tipo o categoría"
        value={operation.type_label ?? operation.category_name ?? ''}
      />
      <DetailItem label="Concepto" value={operation.concept} />
      <DetailItem label="Importe" value={currencyAmount(operation.currency, operation.amount)} />
      <DetailItem label="Equivalencia en pesos" value={equivalent} />
      <DetailItem
        label="Usuario y terminal"
        value={`${operation.actor_name} · ${operation.device_name}`}
      />
      <DetailItem label="Autorización" value={authorizationText(operation)} />
      <DetailItem label="Salida excepcional" value={operation.exceptional ? 'Sí' : ''} />
      <DetailItem label="Motivo" value={operation.reason} />
      <DetailItem label="Referencia" value={operation.reference} />
      <DetailItem label="Observaciones" value={operation.notes} />
      <DetailItem
        label="Corrige a"
        value={linkedText(operation.corrects_operation_id, operations, timezone)}
      />
      <DetailItem
        label="Corregido por"
        value={linkedText(operation.corrected_by_operation_id, operations, timezone)}
      />
    </dl>
  );
}

function MovementFacts({ row }: { row: MovementRow }) {
  const movement = row.movement;
  if (!movement) {
    return null;
  }
  const dollars = movement.usd_in || movement.usd_out;
  return (
    <dl className="cash-detail-list">
      <DetailItem label="Cuenta" value={row.reference} />
      <DetailItem label="Método" value={movement.method_name} />
      <DetailItem label="Venta" value={formatMoney(movement.sale_amount)} />
      <DetailItem label="Propina" value={formatMoney(movement.tip_amount)} />
      <DetailItem label="Neto" value={formatMoney(movement.net_amount)} />
      {movement.is_cash && (
        <>
          <DetailItem label="Efectivo recibido" value={formatMoney(movement.cash_received)} />
          <DetailItem label="Cambio entregado" value={formatMoney(movement.change_given)} />
        </>
      )}
      {dollars && (
        <DetailItem
          label="Dólares recibidos / entregados"
          value={`${formatDollars(movement.usd_in)} / ${formatDollars(movement.usd_out)}`}
        />
      )}
      <DetailItem label="Usuario" value={movement.actor_name} />
    </dl>
  );
}

// Detalle de una fila de movimientos y, si procede, su corrección o regularización.
export function MovementDetail({
  row,
  operations,
  ownShift,
  can,
  timezone,
  onClose,
}: {
  row: MovementRow;
  operations: CashOperation[];
  ownShift: Shift | null;
  can: (permission: string) => boolean;
  timezone: string;
  onClose: () => void;
}) {
  const [correcting, setCorrecting] = useState(false);
  const operation = row.operation;
  const target = operation ? correctionTarget(operation, ownShift, can) : null;
  return (
    <section className="cash-panel" aria-label="Detalle del movimiento">
      <header className="cash-panel-header">
        <h4>
          {row.typeLabel} · {serverDateTime(row.at, timezone)}
        </h4>
        <button type="button" className="secondary-button" onClick={onClose}>
          Cerrar detalle
        </button>
      </header>
      {operation ? (
        <OperationFacts operation={operation} operations={operations} timezone={timezone} />
      ) : (
        <MovementFacts row={row} />
      )}
      {operation && target && !correcting && (
        <button type="button" className="secondary-button" onClick={() => setCorrecting(true)}>
          {operation.shift_id === target.id
            ? 'Corregir este movimiento'
            : 'Regularizar en el turno vigente'}
        </button>
      )}
      {operation && target && correcting && (
        <CorrectionForm
          operation={operation}
          target={target}
          can={can}
          onDone={() => {
            setCorrecting(false);
            onClose();
          }}
        />
      )}
      {row.movement && (
        <p className="muted text-xs">
          Los cobros y devoluciones se corrigen desde el cobro de su cuenta.
        </p>
      )}
    </section>
  );
}
