import { useState } from 'react';
import { Field, Notice } from '../../components/Forms';
import { useLiveResource } from '../../hooks/useLiveResource';
import { formatMoney } from '../../services/money';
import { shiftStateLabels } from '../../services/cash';
import type {
  CashOperation,
  RejectedOutflow,
  RejectedOutflows,
  ShiftHistory,
} from '../../services/cash';
import { permissionName } from '../../services/payments';
import type { Shift, ShiftMovements } from '../../services/payments';
import { serverDateTime } from '../../services/restaurant';
import { MovementDetail } from './MovementDetail';
import {
  businessDate,
  filterRows,
  movementRows,
  movementTypeLabels,
  rejectedFilter,
  rowUsers,
  serverDate,
  serverTime,
} from './movementRows';
import type { MovementFilters, MovementRow, MovementType } from './movementRows';
import { Select } from '../../components/Select';

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
      {/* Sin turno elegido (la caja no tiene uno activo) se pide elegir; así cualquier turno del
          historial, incluso el único, puede seleccionarse. */}
      <Select
        value={value}
        placeholder={shifts.length === 0 ? 'Sin turnos' : 'Elige un turno'}
        options={shifts.map((shift) => ({
          value: shift.id,
          label: `${businessDate(shift.business_date)} · ${serverDateTime(shift.opened_at, timezone)} · ${
            shift.opened_by_name
          } · ${shiftStateLabels[shift.state] ?? shift.state}`,
        }))}
        onChange={onChange}
      />
    </Field>
  );
}

function Filters({
  filters,
  users,
  rejections,
  onChange,
}: {
  filters: MovementFilters;
  users: string[];
  // El filtro «Intentos rechazados» solo existe con cash.expected.read.
  rejections: boolean;
  onChange: (filters: MovementFilters) => void;
}) {
  return (
    <>
      <Field label="Tipo de movimiento">
        <Select
          value={filters.type}
          options={[
            { value: '', label: 'Todos' },
            ...(Object.keys(movementTypeLabels) as MovementType[]).map((type) => ({
              value: type,
              label: movementTypeLabels[type],
            })),
            ...(rejections ? [{ value: rejectedFilter, label: 'Intentos rechazados' }] : []),
          ]}
          onChange={(type) => onChange({ ...filters, type: type as MovementFilters['type'] })}
        />
      </Field>
      <Field label="Usuario">
        <Select
          value={filters.user}
          options={[
            { value: '', label: 'Todos' },
            ...users.map((user) => ({ value: user, label: user })),
          ]}
          onChange={(user) => onChange({ ...filters, user })}
        />
      </Field>
    </>
  );
}

const rejectedKindLabels: Record<RejectedOutflow['kind'], string> = {
  withdrawal: 'Retiro',
  expense: 'Gasto',
};

function rejectedAuthorization(rejection: RejectedOutflow) {
  if (rejection.authorization_mode === 'supervisor') {
    return `Autorizó ${rejection.authorized_by_name ?? 'un supervisor'}`;
  }
  if (rejection.authorization_mode === 'self') {
    return 'Permiso propio';
  }
  return '—';
}

// Retiros y gastos rechazados por efectivo insuficiente (etapa 7.1, §3.5): Go los audita aunque
// la operación no se registre y solo los entrega con cash.expected.read, fuera del arqueo. Sirven
// para detectar a quien prueba importes para deducir el esperado; nunca muestran el esperado.
export function RejectedOutflowsTable({
  rejected,
  error,
  user,
  timezone,
}: {
  rejected: RejectedOutflows | null;
  error: string;
  user: string;
  timezone: string;
}) {
  if (error) {
    return <Notice>{error}</Notice>;
  }
  if (!rejected) {
    return <p className="muted">Consultando los intentos rechazados…</p>;
  }
  if (!rejected.visible) {
    return (
      <p className="muted text-sm">
        {rejected.message ?? 'Los intentos rechazados no están disponibles en este momento.'}
      </p>
    );
  }
  const rows = rejected.rejections.filter((item) => !user || item.actor_name === user);
  return (
    <div className="form-stack">
      <p className="muted text-xs cash-rejected-note">
        Retiros y gastos rechazados por exceder el efectivo de la caja. Al cajero no se le mostró
        ninguna cifra; esta lista tampoco muestra el efectivo esperado.
      </p>
      <div className="table-container cash-table">
        <table aria-label="Intentos rechazados">
          <thead>
            <tr>
              <th scope="col">Fecha</th>
              <th scope="col">Hora</th>
              <th scope="col">Tipo</th>
              <th scope="col">Importe intentado</th>
              <th scope="col">Usuario</th>
              <th scope="col">Terminal</th>
              <th scope="col">Caja</th>
              <th scope="col">Autorización</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((item) => (
              <tr key={item.id}>
                <td>{serverDate(item.occurred_at, timezone)}</td>
                <td>{serverTime(item.occurred_at, timezone)}</td>
                <td>{rejectedKindLabels[item.kind] ?? item.kind}</td>
                <td>
                  <strong>{formatMoney(item.attempted_amount, item.currency)}</strong>
                </td>
                <td>{item.actor_name}</td>
                <td>{item.device_name}</td>
                <td>{item.register_name}</td>
                <td>{rejectedAuthorization(item)}</td>
              </tr>
            ))}
            {rows.length === 0 && (
              <tr>
                <td colSpan={8} className="muted">
                  Sin intentos rechazados en este turno.
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}

// Movimientos de un turno: cobros, devoluciones y correcciones de pago (movements) junto con sus
// operaciones de efectivo (operations, con cash.movements.read). Filtros por turno, tipo y usuario;
// con cash.expected.read, también los intentos rechazados por efectivo insuficiente.
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
  const rejections = can('cash.expected.read');
  const showRejected = rejections && filters.type === rejectedFilter;
  const rejected = useLiveResource<RejectedOutflows>('payments_shift_rejected_outflows', {
    id: shiftID,
    enabled: !!shiftID && showRejected,
    interval: 15000,
  });
  const operationList = operations.data?.operations ?? [];
  const rows = movementRows(movements.data?.movements ?? [], operationList);
  const visible = filterRows(rows, filters);
  if (showRejected) {
    const names = (rejected.data?.rejections ?? []).map((item) => item.actor_name);
    return (
      <div className="form-stack">
        <div className="catalog-filters">
          <Filters
            filters={filters}
            users={[...new Set(names)].sort((first, second) => first.localeCompare(second, 'es'))}
            rejections={rejections}
            onChange={setFilters}
          />
        </div>
        <RejectedOutflowsTable
          rejected={rejected.data}
          error={rejected.error}
          user={filters.user}
          timezone={timezone}
        />
      </div>
    );
  }
  return (
    <div className="form-stack">
      <div className="catalog-filters">
        <Filters
          filters={filters}
          users={rowUsers(rows)}
          rejections={rejections}
          onChange={setFilters}
        />
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
