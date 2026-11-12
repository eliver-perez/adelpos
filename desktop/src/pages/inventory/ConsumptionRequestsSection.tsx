import { useState } from 'react';
import { Accordion } from '../../components/Accordion';
import { Field, Notice } from '../../components/Forms';
import { inventoryChangeEvent } from '../../hooks/useInventoryEvents';
import { useResource } from '../../hooks/useResource';
import type {
  ConsumptionRequest,
  ConsumptionRequestPage,
  ConsumptionRequestState,
} from '../../services/inventoryRecipes';
import { countLabel } from '../../services/plural';
import { serverDateTime } from '../../services/restaurant';
import { Pagination } from './InventoryFields';
import { diagnosticTexts, requestAccount, serviceModeLabels } from './recipeFormat';
import { Select } from '../../components/Select';

const pageSize = 25;

type RequestFilter = ConsumptionRequestState | 'all' | '';

function requestQuery(state: RequestFilter, page: number) {
  const query: Record<string, string> = { page: String(page), page_size: String(pageSize) };
  if (state !== '') {
    query.state = state;
  }
  return query;
}

function RequestRow({
  request,
  timezone,
  onShowTrace,
}: {
  request: ConsumptionRequest;
  timezone: string;
  onShowTrace: (unitID: string) => void;
}) {
  const diagnostics = diagnosticTexts(request.diagnostics);
  const failed = request.state === 'failed';
  return (
    <tr className={failed ? 'inventory-row-negative' : undefined}>
      <td className="inventory-cell-name">
        <strong>{request.product_name || 'Partida'}</strong>
        <span className="muted text-xs inventory-cell-detail">
          {requestAccount(request)} · {serviceModeLabels[request.service_mode]}
        </span>
      </td>
      <td>{request.kind_label}</td>
      <td>{serverDateTime(request.released_at, timezone)}</td>
      <td>
        <span className={failed ? 'tag tag-warning' : 'tag'}>{request.state_label}</span>
        {request.attempts > 0 && (
          <span className="muted text-xs inventory-cell-detail">
            {countLabel(request.attempts, 'intento', 'intentos')}
            {failed ? ` · próximo: ${serverDateTime(request.next_attempt_at, timezone)}` : ''}
          </span>
        )}
      </td>
      <td>
        {request.last_error || diagnostics.length > 0 ? (
          <span className="inventory-cell-stack text-xs">
            {request.last_error && <span>{request.last_error}</span>}
            {diagnostics.map((text) => (
              <span key={text} className="muted">
                {text}
              </span>
            ))}
          </span>
        ) : (
          '—'
        )}
      </td>
      <td>
        <button
          type="button"
          className="secondary-button"
          aria-label={`Ver consumo de la partida ${request.product_name}`}
          onClick={() => onShowTrace(request.restaurant_unit_id)}
        >
          Ver consumo
        </button>
      </td>
    </tr>
  );
}

// Solicitudes de consumo (inventory.movements.read): Go calcula en segundo plano el consumo de cada
// partida liberada a cocina. Aquí se ven las pendientes o con error y su diagnóstico, sin datos
// técnicos crudos; una solicitud con error se reintenta sola con espera creciente.
export function ConsumptionRequestsSection({
  timezone,
  onShowTrace,
}: {
  timezone: string;
  onShowTrace: (unitID: string) => void;
}) {
  const [state, setState] = useState<RequestFilter>('');
  const [page, setPage] = useState(1);
  const list = useResource<ConsumptionRequestPage>('inventory_consumption_requests', {
    query: requestQuery(state, page),
    reloadEvent: inventoryChangeEvent,
  });
  const data = list.data;
  const summary = data
    ? `${countLabel(data.pending, 'pendiente', 'pendientes')} · ${countLabel(data.failed, 'con error', 'con error')}`
    : 'Consultando solicitudes…';
  return (
    <Accordion
      title="Solicitudes de consumo"
      summary={summary}
      hasError={(data?.failed ?? 0) > 0}
      className="inventory-requests"
    >
      <div className="form-stack">
        <p className="muted text-sm">
          Cada partida liberada a cocina con el control de inventario activado genera una solicitud
          que AD-EL POS procesa en segundo plano. Las que fallan se reintentan solas y bloquean solo
          los cambios posteriores de la misma partida.
        </p>
        <Field label="Solicitudes">
          <Select<RequestFilter>
            value={state}
            options={[
              { value: '', label: 'Sin procesar (pendientes o con error)' },
              { value: 'pending', label: 'Pendientes' },
              { value: 'failed', label: 'Con error' },
              { value: 'processed', label: 'Procesadas' },
              { value: 'all', label: 'Todas' },
            ]}
            onChange={(next) => {
              setState(next);
              setPage(1);
            }}
          />
        </Field>
        <Notice>{list.error}</Notice>
        {data && (
          <>
            <div className="table-container cash-table">
              <table className="inventory-table">
                <thead>
                  <tr>
                    <th scope="col">Partida</th>
                    <th scope="col">Tipo</th>
                    <th scope="col">Liberada</th>
                    <th scope="col">Estado</th>
                    <th scope="col">Diagnóstico</th>
                    <th scope="col">
                      <span className="sr-only">Acciones</span>
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {data.items.map((request) => (
                    <RequestRow
                      key={request.id}
                      request={request}
                      timezone={timezone}
                      onShowTrace={onShowTrace}
                    />
                  ))}
                  {data.items.length === 0 && (
                    <tr>
                      <td colSpan={6} className="muted">
                        {state === ''
                          ? 'Todas las solicitudes están procesadas.'
                          : 'No hay solicitudes con este filtro.'}
                      </td>
                    </tr>
                  )}
                </tbody>
              </table>
            </div>
            <Pagination
              page={data.page}
              pageSize={data.page_size}
              total={data.total}
              singular="solicitud"
              plural="solicitudes"
              onPage={setPage}
            />
          </>
        )}
      </div>
    </Accordion>
  );
}
