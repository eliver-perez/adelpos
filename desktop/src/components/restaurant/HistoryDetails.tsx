import { ProductDetails } from '../catalog/QuoteBreakdown';
import type { ProductBreakdown } from '../../services/catalog';
import { formatMoney } from '../../services/money';
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

// Etapa 9D (§5): «Cancelar cuenta» registra el motivo del catálogo, la descripción, quién autorizó
// (si hizo falta un supervisor), las unidades y el importe que calculó Go. Del supervisor se muestra
// su nombre visible (`authorized_by_name`), como en el detalle de la cuenta y en «Cuentas
// canceladas»; solo un evento anterior que no lo guardó muestra su usuario.
function supervisorName(after: RecordValue) {
  if (typeof after.authorized_by_name === 'string' && after.authorized_by_name) {
    return after.authorized_by_name;
  }
  return typeof after.authorized_by_username === 'string' ? after.authorized_by_username : '';
}

function AccountCancelled({ event, after }: { event: HistoryEvent; after: RecordValue }) {
  const description = typeof after.description === 'string' ? after.description : '';
  const authorized = supervisorName(after);
  const discarded =
    typeof after.discarded_draft_items === 'number' ? after.discarded_draft_items : 0;
  return (
    <div className="form-stack">
      {description && <p>Descripción: {description}</p>}
      {typeof after.cancelled_units === 'number' && (
        <p>Unidades canceladas: {after.cancelled_units}</p>
      )}
      {typeof after.cancelled_amount === 'string' && (
        <p>Importe cancelado: {formatMoney(after.cancelled_amount)}</p>
      )}
      <p>Canceló: {event.author_name}</p>
      <p>Autorizó: {authorized || 'con su propio permiso'}</p>
      {discarded > 0 && <p>Productos sin enviar descartados: {discarded}</p>}
      {after.prepared_is_waste === true && <p>Lo preparado cuenta como merma.</p>}
      <p>La cancelación no cuenta como venta ni registra una devolución de dinero.</p>
    </div>
  );
}

// Etapa 9F (§7.2): «Cancelar unidades» y la reducción desde «Modificar» guardan quién autorizó (null si
// bastó el permiso propio). Los eventos anteriores no tienen el dato y no muestran nada.
function UnitsAuthorization({ after }: { after: RecordValue }) {
  if (!('authorized_by' in after)) {
    return null;
  }
  return <p>Autorizó: {supervisorName(after) || 'con su propio permiso'}</p>;
}

export function HistoryDetails({ event }: { event: HistoryEvent }) {
  const before = record(event.payload.before);
  const after = record(event.payload.after);
  const account = record(after.account ?? event.payload.after);
  const totals = record(after.totals ?? after.preserved_totals);
  // En «Cancelar cuenta» los identificadores son las unidades canceladas (se detallan aparte).
  const units =
    Array.isArray(after.unit_ids) && event.kind !== 'account.cancelled'
      ? after.unit_ids.length
      : null;
  const discardedDraft = Array.isArray(before.draft) ? before.draft.length : 0;
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
      {typeof totals.total === 'string' && <p>Importe histórico: {formatMoney(totals.total)}</p>}
      {event.kind === 'item.modified' && (
        <>
          <UnitsAuthorization after={after} />
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
      {event.kind === 'account.cancelled' && <AccountCancelled event={event} after={after} />}
      {event.kind === 'account.discarded' && (
        <div className="form-stack">
          {discardedDraft > 0 && <p>Productos sin enviar descartados: {discardedDraft}</p>}
          <p>
            Cuenta vacía descartada sin motivo: no es una cancelación ni aparece en los reportes.
          </p>
        </div>
      )}
      {event.kind === 'item.cancelled' && (
        <div className="form-stack">
          <UnitsAuthorization after={after} />
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
