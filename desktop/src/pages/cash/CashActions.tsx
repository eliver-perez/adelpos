import { IconLabel } from '../../components/Icon';
import type { IconName } from '../../components/Icon';
import { permissionName } from '../../services/payments';
import type { Shift } from '../../services/payments';

export type CashAction = 'income' | 'withdrawal' | 'expense' | 'partial' | 'closing';

interface ActionDefinition {
  action: CashAction;
  label: string;
  icon: IconName;
  permission: string;
  // Las operaciones de efectivo se registran desde una terminal de la caja del turno.
  ownRegister: boolean;
}

const definitions: ActionDefinition[] = [
  {
    action: 'income',
    label: 'Ingreso',
    icon: 'income',
    permission: 'cash.income.record',
    ownRegister: true,
  },
  {
    action: 'withdrawal',
    label: 'Retiro de efectivo',
    icon: 'withdrawal',
    permission: 'cash.withdrawals.record',
    ownRegister: true,
  },
  {
    action: 'expense',
    label: 'Gasto',
    icon: 'expense',
    permission: 'cash.expenses.record',
    ownRegister: true,
  },
  {
    action: 'partial',
    label: 'Corte parcial',
    icon: 'report',
    permission: 'cash.shifts.read',
    ownRegister: false,
  },
  {
    action: 'closing',
    label: 'Arqueo y cierre',
    icon: 'count',
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
            <IconLabel icon={definition.icon}>
              {definition.action === 'closing' && shift.state === 'closing'
                ? 'Continuar arqueo'
                : definition.label}
            </IconLabel>
          </button>
        ))}
        {correctable && (
          <button type="button" onClick={onCorrections}>
            <IconLabel icon="edit">Corrección o regularización</IconLabel>
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
