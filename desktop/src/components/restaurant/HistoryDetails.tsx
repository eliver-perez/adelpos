import { ProductDetails } from '../catalog/QuoteBreakdown';
import type { ProductBreakdown } from '../../services/catalog';
import type { HistoryEvent } from '../../services/restaurant';

type RecordValue = Record<string, unknown>;

function record(value: unknown): RecordValue {
  return value && typeof value === 'object' && !Array.isArray(value) ? (value as RecordValue) : {};
}

function PreparationChange({ value }: { value: unknown }) {
  const item = record(value);
  const request = record(item.request);
  const selection = record(request.customization);
  const quote = record(item.quote_line);
  return (
    <div className="form-stack">
      {typeof quote.name === 'string' && <strong>{quote.name}</strong>}
      {typeof request.quantity === 'number' && <p>Cantidad: {request.quantity}</p>}
      {typeof selection.notes === 'string' && selection.notes && (
        <p>Preparación: {selection.notes}</p>
      )}
      {typeof quote.unit_price === 'string' && Array.isArray(quote.groups) && (
        <ProductDetails value={quote as unknown as ProductBreakdown} />
      )}
    </div>
  );
}

export function HistoryDetails({ event }: { event: HistoryEvent }) {
  const before = record(event.payload.before);
  const after = record(event.payload.after);
  const account = record(after.account ?? event.payload.after);
  const totals = record(after.totals ?? after.preserved_totals);
  const units = Array.isArray(after.unit_ids) ? after.unit_ids.length : null;
  return (
    <div className="form-stack">
      {typeof after.reason === 'string' && after.reason && <p>Motivo: {after.reason}</p>}
      {typeof account.name === 'string' && <p>Cuenta: {account.name}</p>}
      {typeof account.customer_name === 'string' && account.customer_name && (
        <p>Cliente: {account.customer_name}</p>
      )}
      {typeof account.table_name === 'string' && <p>Mesa: {account.table_name}</p>}
      {typeof account.assigned_name === 'string' && <p>Responsable: {account.assigned_name}</p>}
      {typeof account.notes === 'string' && account.notes && <p>Observaciones: {account.notes}</p>}
      {units !== null && <p>Unidades conservadas en la operación: {units}</p>}
      {typeof after.quantity === 'number' && <p>Unidades afectadas: {after.quantity}</p>}
      {typeof totals.total === 'string' && <p>Importe histórico: ${totals.total}</p>}
      {event.kind === 'item.modified' && (
        <>
          <h4>Antes</h4>
          <PreparationChange value={before} />
          <h4>Después</h4>
          <PreparationChange value={after} />
        </>
      )}
      {Array.isArray(after.items) &&
        after.items.map((item, index) => <PreparationChange key={index} value={item} />)}
      {Array.isArray(event.payload.after) && (
        <p>Partidas en borrador: {event.payload.after.length}</p>
      )}
      {event.kind === 'item.cancelled' && (
        <div className="form-stack">
          <PreparationChange value={before} />
          <p>La cancelación conserva el historial. No registra una devolución de dinero.</p>
        </div>
      )}
      <p className="muted text-xs">
        Cambio registrado por {event.author_name} en {event.device_name}.
      </p>
    </div>
  );
}
