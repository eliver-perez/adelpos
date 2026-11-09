import { permissionName } from '../../services/payments';
import type { Shift } from '../../services/payments';

export type CashAction = 'income' | 'withdrawal' | 'expense' | 'partial' | 'closing';

interface ActionDefinition {
  action: CashAction;
  label: string;
  permission: string;
  // Las operaciones de efectivo se registran desde una terminal de la caja del turno.
  ownRegister: boolean;
}

const definitions: ActionDefinition[] = [
  { action: 'income', label: 'Ingreso', permission: 'cash.income.record', ownRegister: true },
  {
    action: 'withdrawal',
    label: 'Retiro de efectivo',
    permission: 'cash.withdrawals.record',
    ownRegister: true,
  },
  { action: 'expense', label: 'Gasto', permission: 'cash.expenses.record', ownRegister: true },
  { action: 'partial', label: 'Corte parcial', permission: 'cash.shifts.read', ownRegister: false },
  {
    action: 'closing',
    label: 'Arqueo y cierre',
    permission: 'cash.counts.perform',
    ownRegister: false,
  },
];

// Acciones del turno según permisos, estado y caja. Con la caja en arqueo solo continúa el
// arqueo; Go vuelve a validar cada permiso y estado al registrar.
export function availableActions(
  can: (permission: string) => boolean,
  shift: Shift,
  ownRegister: boolean,
) {
  return definitions.filter((definition) => {
    if (!can(definition.permission)) {
      return false;
    }
    if (shift.state === 'closing') {
      return definition.action === 'closing';
    }
    return !definition.ownRegister || ownRegister;
  });
}

// Botones de acción del turno. Las correcciones se hacen desde el detalle de cada movimiento.
export function CashActions({
  can,
  shift,
  ownRegister,
  selected,
  onSelect,
  onCorrections,
}: {
  can: (permission: string) => boolean;
  shift: Shift;
  ownRegister: boolean;
  selected: CashAction | null;
  onSelect: (action: CashAction) => void;
  onCorrections: () => void;
}) {
  const actions = availableActions(can, shift, ownRegister);
  const correctable =
    shift.state === 'open' &&
    ownRegister &&
    can('cash.corrections.record') &&
    can('cash.movements.read');
  return (
    <div className="form-stack">
      <div className="cash-actions" role="group" aria-label="Acciones del turno">
        {actions.map((definition) => (
          <button
            type="button"
            key={definition.action}
            className={selected === definition.action ? 'cash-choice-selected' : ''}
            aria-pressed={selected === definition.action}
            onClick={() => onSelect(definition.action)}
          >
            {definition.action === 'closing' && shift.state === 'closing'
              ? 'Continuar arqueo'
              : definition.label}
          </button>
        ))}
        {correctable && (
          <button type="button" onClick={onCorrections}>
            Corrección o regularización
          </button>
        )}
      </div>
      {!ownRegister && shift.state === 'open' && (
        <p className="muted text-xs">
          Los ingresos, retiros, gastos y correcciones se registran desde una terminal de{' '}
          {shift.register_name}.
        </p>
      )}
      {actions.length === 0 && (
        <p className="muted text-xs">
          Registrar operaciones de caja requiere permisos como «
          {permissionName('cash.income.record')}» o «{permissionName('cash.counts.perform')}».
        </p>
      )}
    </div>
  );
}
