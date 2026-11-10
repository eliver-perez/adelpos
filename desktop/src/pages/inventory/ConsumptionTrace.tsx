import { Notice } from '../../components/Forms';
import { inventoryChangeEvent } from '../../hooks/useInventoryEvents';
import { useResource } from '../../hooks/useResource';
import type {
  ConsumptionLine,
  ConsumptionRequest,
  UnitConsumptionTrace,
} from '../../services/inventoryRecipes';
import { formatMoney, formatSignedMoney } from '../../services/money';
import { serverDateTime } from '../../services/restaurant';
import { formatQuantity, formatSignedQuantity } from './inventoryFormat';
import { diagnosticTexts, requestAccount, serviceModeLabels } from './recipeFormat';

// Consumo por insumo y almacén de una partida: lo consumido por la liberación y sus
// modificaciones y lo neto vigente tras clasificar una cancelación. Valores solo con costos.
export function ConsumptionLinesTable({
  lines,
  costsVisible,
  caption,
}: {
  lines: ConsumptionLine[];
  costsVisible: boolean;
  caption: string;
}) {
  return (
    <div className="table-container cash-table">
      <table className="inventory-table">
        <caption className="sr-only">{caption}</caption>
        <thead>
          <tr>
            <th scope="col">Insumo</th>
            <th scope="col">Almacén</th>
            <th scope="col" className="inventory-number">
              Consumido
            </th>
            <th scope="col" className="inventory-number">
              Neto vigente
            </th>
            {costsVisible && (
              <th scope="col" className="inventory-number">
                Valor neto
              </th>
            )}
          </tr>
        </thead>
        <tbody>
          {lines.map((line) => (
            <tr key={`${line.item.id}:${line.warehouse.id}`}>
              <td className="inventory-cell-name">
                <strong>{line.item.name}</strong>
                <span className="muted text-xs inventory-cell-detail">{line.item.code}</span>
              </td>
              <td>{line.warehouse.name}</td>
              <td className="inventory-number">
                {formatQuantity(line.consumed_quantity, line.item.unit)}
              </td>
              <td className="inventory-number">
                {formatQuantity(line.net_quantity, line.item.unit)}
              </td>
              {costsVisible && (
                <td className="inventory-number">{formatMoney(line.value_display)}</td>
              )}
            </tr>
          ))}
          {lines.length === 0 && (
            <tr>
              <td colSpan={costsVisible ? 5 : 4} className="muted">
                La partida no consumió inventario.
              </td>
            </tr>
          )}
        </tbody>
      </table>
    </div>
  );
}

function requestState(request: ConsumptionRequest, timezone: string) {
  if (request.state === 'processed' && request.processed_at) {
    return `${request.state_label} el ${serverDateTime(request.processed_at, timezone)}`;
  }
  return request.state_label;
}

function TraceRequests({
  requests,
  timezone,
}: {
  requests: ConsumptionRequest[];
  timezone: string;
}) {
  return (
    <ol className="inventory-list" aria-label="Solicitudes de consumo de la partida">
      {requests.map((request) => {
        const diagnostics = diagnosticTexts(request.diagnostics);
        return (
          <li key={request.id} className="inventory-list-row">
            <div className="inventory-list-text">
              <strong>{request.kind_label}</strong>
              <span className="muted text-xs">
                Registrada el {serverDateTime(request.created_at, timezone)} ·{' '}
                {requestState(request, timezone)}
              </span>
              {request.last_error && <span className="text-xs">{request.last_error}</span>}
              {diagnostics.map((text) => (
                <span key={text} className="muted text-xs">
                  {text}
                </span>
              ))}
            </div>
            <span className={request.state === 'failed' ? 'tag tag-warning' : 'tag'}>
              {request.state_label}
            </span>
          </li>
        );
      })}
    </ol>
  );
}

// Traza del consumo de una partida (unidad comercial): solicitudes, consumo por insumo y almacén,
// movimientos del libro y la cancelación si la hubo. Sin identificadores técnicos; los valores
// solo con inventory.costs.read.
export function ConsumptionTraceView({
  unitID,
  timezone,
  onClose,
  closeLabel,
}: {
  unitID: string;
  timezone: string;
  onClose: () => void;
  closeLabel: string;
}) {
  const resource = useResource<UnitConsumptionTrace>('inventory_unit_consumption', {
    id: unitID,
    reloadEvent: inventoryChangeEvent,
  });
  const trace = resource.data;
  const first = trace?.requests[0] ?? null;
  const product = first?.product_name ?? trace?.cancellation?.product_name ?? 'Partida';
  let state = 'Sin consumo registrado';
  if (trace?.state === 'consumed') {
    state = 'Consumo registrado';
  } else if (trace?.state === 'reversed') {
    state = 'Consumo revertido';
  }
  const details: string[] = [];
  if (first) {
    details.push(requestAccount(first));
  }
  if (trace?.released_at) {
    details.push(`liberada el ${serverDateTime(trace.released_at, timezone)}`);
  }
  if (trace?.service_mode) {
    details.push(serviceModeLabels[trace.service_mode]);
  }
  return (
    <section className="panel form-stack" aria-label="Consumo de la partida">
      <div className="section-heading">
        <div>
          <h2>Consumo de la partida · {product}</h2>
          <p className="muted text-xs">{details.join(' · ') || 'Consultando la partida…'}</p>
        </div>
        <div className="button-row">
          {trace && (
            <span className={trace.state === 'consumed' ? 'tag tag-good' : 'tag'}>{state}</span>
          )}
          <button type="button" className="secondary-button" onClick={onClose}>
            {closeLabel}
          </button>
        </div>
      </div>
      {!trace && <Notice>{resource.error || 'Consultando el consumo de la partida…'}</Notice>}
      {trace && (
        <>
          <section className="form-stack" aria-labelledby="trace-lines-title">
            <h3 id="trace-lines-title">Insumos por almacén</h3>
            <ConsumptionLinesTable
              lines={trace.lines}
              costsVisible={trace.costs_visible}
              caption="Consumo de la partida por insumo y almacén"
            />
          </section>
          {trace.cancellation && (
            <p className="text-sm">
              Cancelada después de liberarse: <strong>{trace.cancellation.state_label}</strong>
              {trace.cancellation.resolution_label
                ? ` · ${trace.cancellation.resolution_label}`
                : ''}
              .
            </p>
          )}
          <section className="form-stack" aria-labelledby="trace-movements-title">
            <h3 id="trace-movements-title">Movimientos del libro</h3>
            <div className="table-container cash-table">
              <table className="inventory-table">
                <thead>
                  <tr>
                    <th scope="col">Fecha de negocio</th>
                    <th scope="col">Insumo</th>
                    <th scope="col">Almacén</th>
                    <th scope="col">Tipo</th>
                    <th scope="col" className="inventory-number">
                      Cantidad
                    </th>
                    <th scope="col">Origen</th>
                    {trace.costs_visible && (
                      <th scope="col" className="inventory-number">
                        Valor
                      </th>
                    )}
                  </tr>
                </thead>
                <tbody>
                  {trace.movements.map((movement) => (
                    <tr key={movement.id}>
                      <td>{serverDateTime(movement.occurred_at, timezone)}</td>
                      <td className="inventory-cell-name">
                        <strong>{movement.item_name}</strong>
                      </td>
                      <td>{movement.warehouse_name ?? 'Valuación del insumo'}</td>
                      <td>{movement.kind_label}</td>
                      <td className="inventory-number">
                        {formatSignedQuantity(movement.quantity, movement.unit)}
                      </td>
                      <td>
                        {movement.source_kind_label}
                        {movement.reason && (
                          <span className="muted text-xs inventory-cell-detail">
                            {movement.reason}
                          </span>
                        )}
                      </td>
                      {trace.costs_visible && (
                        <td className="inventory-number">
                          {formatSignedMoney(movement.value_display)}
                        </td>
                      )}
                    </tr>
                  ))}
                  {trace.movements.length === 0 && (
                    <tr>
                      <td colSpan={trace.costs_visible ? 7 : 6} className="muted">
                        Sin movimientos de inventario.
                      </td>
                    </tr>
                  )}
                </tbody>
              </table>
            </div>
          </section>
          <section className="form-stack" aria-labelledby="trace-requests-title">
            <h3 id="trace-requests-title">Solicitudes de consumo</h3>
            {trace.requests.length === 0 ? (
              <p className="muted text-sm">
                Sin solicitudes: la partida se liberó con el control de inventario desactivado o
                antes de activarlo.
              </p>
            ) : (
              <TraceRequests requests={trace.requests} timezone={timezone} />
            )}
          </section>
        </>
      )}
    </section>
  );
}
