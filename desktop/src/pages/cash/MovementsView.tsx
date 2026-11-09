import { useState } from 'react';
import { Field, Notice } from '../../components/Forms';
import { useLiveResource } from '../../hooks/useLiveResource';
import { shiftStateLabels } from '../../services/cash';
import type { CashOperation, ShiftHistory } from '../../services/cash';
import { permissionName } from '../../services/payments';
import type { Shift, ShiftMovements } from '../../services/payments';
import { serverDateTime } from '../../services/restaurant';
import { MovementDetail } from './MovementDetail';
import {
  businessDate,
  filterRows,
  movementRows,
  movementTypeLabels,
  rowUsers,
  serverDate,
  serverTime,
} from './movementRows';
import type { MovementFilters, MovementRow, MovementType } from './movementRows';

function MovementsTable({
  rows,
  timezone,
  selectedKey,
  onSelect,
}: {
  rows: MovementRow[];
  timezone: string;
  selectedKey: string;
  onSelect: (row: MovementRow) => void;
}) {
  return (
    <div className="table-container cash-table">
      <table>
        <thead>
          <tr>
            <th scope="col">Fecha</th>
            <th scope="col">Hora</th>
            <th scope="col">Tipo</th>
            <th scope="col">Concepto</th>
            <th scope="col">Método o moneda</th>
            <th scope="col">Importe</th>
            <th scope="col">Usuario</th>
            <th scope="col">Referencia</th>
            <th scope="col">Estado</th>
            <th scope="col">Detalle</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((row) => (
            <tr key={row.key} className={selectedKey === row.key ? 'cash-row-selected' : ''}>
              <td>{serverDate(row.at, timezone)}</td>
              <td>{serverTime(row.at, timezone)}</td>
              <td>
                {row.typeLabel}
                {row.direction && <span className="muted text-xs"> · {row.direction}</span>}
              </td>
              <td>{row.concept}</td>
              <td>{row.method}</td>
              <td>
                <strong>{row.amount}</strong>
              </td>
              <td>{row.user}</td>
              <td>{row.reference}</td>
              <td>{row.status}</td>
              <td>
                <button
                  type="button"
                  className="secondary-button"
                  aria-label={`Detalle de ${row.typeLabel} ${row.concept} de las ${serverTime(row.at, timezone)}`}
                  onClick={() => onSelect(row)}
                >
                  Ver
                </button>
              </td>
            </tr>
          ))}
          {rows.length === 0 && (
            <tr>
              <td colSpan={10} className="muted">
                Sin movimientos con estos filtros.
              </td>
            </tr>
          )}
        </tbody>
      </table>
    </div>
  );
}

// Selector del turno: el activo de la caja y, con permiso de historial, los anteriores.
function ShiftChooser({
  registerID,
  active,
  value,
  can,
  timezone,
  onChange,
}: {
  registerID: string;
  active: Shift | null;
  value: string;
  can: (permission: string) => boolean;
  timezone: string;
  onChange: (id: string) => void;
}) {
  const history = useLiveResource<ShiftHistory>('payments_shifts', {
    query: { register_id: registerID, page: '1' },
    enabled: can('cash.history.read') && !!registerID,
    interval: 60000,
  });
  const shifts = history.data?.items.map((item) => item.shift) ?? [];
  if (active && !shifts.some((shift) => shift.id === active.id)) {
    shifts.unshift(active);
  }
  return (
    <Field label="Turno">
      <select value={value} onChange={(event) => onChange(event.target.value)}>
        {/* Sin turno elegido (la caja no tiene uno activo) se pide elegir; así cualquier turno
            del historial, incluso el único, puede seleccionarse. */}
        {value === '' && (
          <option value="">{shifts.length === 0 ? 'Sin turnos' : 'Elige un turno'}</option>
        )}
        {shifts.map((shift) => (
          <option key={shift.id} value={shift.id}>
            {businessDate(shift.business_date)} · {serverDateTime(shift.opened_at, timezone)} ·{' '}
            {shift.opened_by_name} · {shiftStateLabels[shift.state] ?? shift.state}
          </option>
        ))}
      </select>
    </Field>
  );
}

function Filters({
  filters,
  users,
  onChange,
}: {
  filters: MovementFilters;
  users: string[];
  onChange: (filters: MovementFilters) => void;
}) {
  return (
    <>
      <Field label="Tipo de movimiento">
        <select
          value={filters.type}
          onChange={(event) =>
            onChange({ ...filters, type: event.target.value as MovementType | '' })
          }
        >
          <option value="">Todos</option>
          {(Object.keys(movementTypeLabels) as MovementType[]).map((type) => (
            <option key={type} value={type}>
              {movementTypeLabels[type]}
            </option>
          ))}
        </select>
      </Field>
      <Field label="Usuario">
        <select
          value={filters.user}
          onChange={(event) => onChange({ ...filters, user: event.target.value })}
        >
          <option value="">Todos</option>
          {users.map((user) => (
            <option key={user} value={user}>
              {user}
            </option>
          ))}
        </select>
      </Field>
    </>
  );
}

// Movimientos de un turno: cobros, devoluciones y correcciones de pago (movements) junto con sus
// operaciones de efectivo (operations, con cash.movements.read). Filtros por turno, tipo y usuario.
export function MovementsTimeline({
  shiftID,
  ownShift,
  can,
  timezone,
}: {
  shiftID: string;
  ownShift: Shift | null;
  can: (permission: string) => boolean;
  timezone: string;
}) {
  const [filters, setFilters] = useState<MovementFilters>({ type: '', user: '' });
  const [selected, setSelected] = useState<MovementRow | null>(null);
  const movements = useLiveResource<ShiftMovements>('payments_shift_movements', {
    id: shiftID,
    enabled: !!shiftID,
    interval: 15000,
  });
  const operations = useLiveResource<{ shift: Shift; operations: CashOperation[] }>(
    'payments_shift_operations',
    { id: shiftID, enabled: !!shiftID && can('cash.movements.read'), interval: 15000 },
  );
  const operationList = operations.data?.operations ?? [];
  const rows = movementRows(movements.data?.movements ?? [], operationList);
  const visible = filterRows(rows, filters);
  return (
    <div className="form-stack">
      <div className="catalog-filters">
        <Filters filters={filters} users={rowUsers(rows)} onChange={setFilters} />
      </div>
      <Notice>{movements.error || operations.error}</Notice>
      {!can('cash.movements.read') && (
        <p className="muted text-xs">
          Los ingresos, retiros y gastos se muestran con el permiso «
          {permissionName('cash.movements.read')}».
        </p>
      )}
      {!movements.data && !movements.error && <p className="muted">Consultando movimientos…</p>}
      {movements.data && (
        <MovementsTable
          rows={visible}
          timezone={timezone}
          selectedKey={selected?.key ?? ''}
          onSelect={setSelected}
        />
      )}
      {selected && (
        <MovementDetail
          key={selected.key}
          row={selected}
          operations={operationList}
          ownShift={ownShift}
          can={can}
          timezone={timezone}
          onClose={() => setSelected(null)}
        />
      )}
    </div>
  );
}

// Vista de movimientos de Caja: el turno activo por omisión y, con historial, los anteriores.
export function MovementsView({
  registerID,
  active,
  ownShift,
  can,
  timezone,
}: {
  registerID: string;
  active: Shift | null;
  ownShift: Shift | null;
  can: (permission: string) => boolean;
  timezone: string;
}) {
  const [chosen, setChosen] = useState('');
  const shiftID = chosen || active?.id || '';
  return (
    <div className="form-stack">
      <div className="catalog-filters">
        <ShiftChooser
          registerID={registerID}
          active={active}
          value={shiftID}
          can={can}
          timezone={timezone}
          onChange={setChosen}
        />
      </div>
      {!shiftID && <Notice>La caja no tiene un turno activo. Elige un turno del historial.</Notice>}
      {shiftID && (
        <MovementsTimeline
          key={shiftID}
          shiftID={shiftID}
          ownShift={ownShift}
          can={can}
          timezone={timezone}
        />
      )}
    </div>
  );
}
