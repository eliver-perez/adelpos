import { useState } from 'react';
import { Field, Notice } from '../../components/Forms';
import { OptionalDateField } from '../../components/catalog/DateField';
import { useLiveResource } from '../../hooks/useLiveResource';
import { cashDifferenceLabels, shiftStateLabels } from '../../services/cash';
import type {
  ClosureSummary,
  RegisterShift,
  ShiftHistory,
  ShiftHistoryFilter,
} from '../../services/cash';
import { formatDollars, formatMoney } from '../../services/payments';
import type { Shift } from '../../services/payments';
import { serverDateTime } from '../../services/restaurant';
import { businessDate } from './movementRows';
import { ShiftHistoryDetail } from './ShiftHistoryDetail';

// Diferencia del cierre tal como la entregó Go; sin permiso no viaja y no se muestra nada.
function differenceText(closure: ClosureSummary | null) {
  if (!closure || closure.difference_mxn === undefined || !closure.result_mxn) {
    return '';
  }
  let text = `${formatMoney(closure.difference_mxn)} · ${cashDifferenceLabels[closure.result_mxn]}`;
  if (closure.difference_usd && closure.result_usd && closure.result_usd !== 'even') {
    text += ` · ${formatDollars(closure.difference_usd)}`;
  }
  return text;
}

function HistoryFilters({
  filter,
  registers,
  onChange,
}: {
  filter: ShiftHistoryFilter;
  registers: RegisterShift[];
  onChange: (filter: ShiftHistoryFilter) => void;
}) {
  return (
    <div className="catalog-filters">
      {registers.length > 1 && (
        <Field label="Caja">
          <select
            value={filter.register_id ?? ''}
            onChange={(event) => onChange({ ...filter, register_id: event.target.value, page: '' })}
          >
            <option value="">Todas</option>
            {registers.map((entry) => (
              <option key={entry.register.id} value={entry.register.id}>
                {entry.register.name}
              </option>
            ))}
          </select>
        </Field>
      )}
      <OptionalDateField
        label="Fecha comercial desde"
        value={filter.from ?? ''}
        onChange={(from) => onChange({ ...filter, from, page: '' })}
      />
      <OptionalDateField
        label="Fecha comercial hasta"
        value={filter.until ?? ''}
        onChange={(until) => onChange({ ...filter, until, page: '' })}
      />
      <Field label="Estado">
        <select
          value={filter.state ?? ''}
          onChange={(event) =>
            onChange({
              ...filter,
              state: (event.target.value || undefined) as ShiftHistoryFilter['state'],
              page: '',
            })
          }
        >
          <option value="">Todos</option>
          <option value="open">Abierto</option>
          <option value="closing">Cierre en curso</option>
          <option value="closed">Cerrado</option>
        </select>
      </Field>
    </div>
  );
}

function HistoryTable({
  history,
  timezone,
  onSelect,
}: {
  history: ShiftHistory;
  timezone: string;
  onSelect: (shift: Shift) => void;
}) {
  return (
    <div className="table-container cash-table">
      <table>
        <thead>
          <tr>
            <th scope="col">Fecha comercial</th>
            <th scope="col">Caja</th>
            <th scope="col">Apertura</th>
            <th scope="col">Cierre</th>
            <th scope="col">Estado</th>
            <th scope="col">Declarado</th>
            <th scope="col">Diferencia</th>
            <th scope="col">Detalle</th>
          </tr>
        </thead>
        <tbody>
          {history.items.map(({ shift, closure }) => (
            <tr key={shift.id}>
              <td>{businessDate(shift.business_date)}</td>
              <td>{shift.register_name}</td>
              <td>
                {serverDateTime(shift.opened_at, timezone)}
                <span className="muted text-xs"> · {shift.opened_by_name}</span>
              </td>
              <td>
                {shift.closed_at ? serverDateTime(shift.closed_at, timezone) : '—'}
                {shift.closed_by_name && (
                  <span className="muted text-xs"> · {shift.closed_by_name}</span>
                )}
              </td>
              <td>{shiftStateLabels[shift.state] ?? shift.state}</td>
              <td>{closure ? formatMoney(closure.counted_mxn) : '—'}</td>
              <td>{differenceText(closure) || '—'}</td>
              <td>
                <button
                  type="button"
                  className="secondary-button"
                  aria-label={`Detalle del turno de ${shift.register_name} del ${businessDate(shift.business_date)}`}
                  onClick={() => onSelect(shift)}
                >
                  Ver
                </button>
              </td>
            </tr>
          ))}
          {history.items.length === 0 && (
            <tr>
              <td colSpan={8} className="muted">
                No hay turnos con estos filtros.
              </td>
            </tr>
          )}
        </tbody>
      </table>
    </div>
  );
}

// Historial de turnos por caja, rango de fechas comerciales, estado y página (cash.history.read).
export function HistoryView({
  registers,
  ownShift,
  can,
  timezone,
}: {
  registers: RegisterShift[];
  ownShift: Shift | null;
  can: (permission: string) => boolean;
  timezone: string;
}) {
  const [filter, setFilter] = useState<ShiftHistoryFilter>({});
  const [selected, setSelected] = useState<Shift | null>(null);
  const query: Record<string, string> = {};
  for (const [key, value] of Object.entries(filter)) {
    if (value) {
      query[key] = value;
    }
  }
  const history = useLiveResource<ShiftHistory>('payments_shifts', { query, interval: 30000 });
  if (selected) {
    return (
      <ShiftHistoryDetail
        shift={selected}
        ownShift={ownShift}
        can={can}
        timezone={timezone}
        onClose={() => setSelected(null)}
      />
    );
  }
  const page = history.data?.page ?? 1;
  const pages = history.data
    ? Math.max(1, Math.ceil(history.data.total / history.data.page_size))
    : 1;
  return (
    <div className="form-stack">
      <HistoryFilters filter={filter} registers={registers} onChange={setFilter} />
      <Notice>{history.error}</Notice>
      {!history.data && !history.error && <p className="muted">Consultando historial…</p>}
      {history.data && (
        <HistoryTable history={history.data} timezone={timezone} onSelect={setSelected} />
      )}
      {history.data && pages > 1 && (
        <div className="button-row">
          <button
            type="button"
            className="secondary-button"
            disabled={page <= 1}
            onClick={() => setFilter({ ...filter, page: String(page - 1) })}
          >
            Página anterior
          </button>
          <span className="muted text-sm">
            Página {page} de {pages}
          </span>
          <button
            type="button"
            className="secondary-button"
            disabled={page >= pages}
            onClick={() => setFilter({ ...filter, page: String(page + 1) })}
          >
            Página siguiente
          </button>
        </div>
      )}
    </div>
  );
}
