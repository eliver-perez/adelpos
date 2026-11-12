import { Icon } from '../../components/Icon';
import type { IconName } from '../../components/Icon';
import { permissionName } from '../../services/payments';
import type { Shift } from '../../services/payments';

export type CashAction = 'income' | 'withdrawal' | 'expense' | 'partial' | 'closing';

// Acento de cada tarjeta (etapa 7.1, decisión 11; colores de la etapa 9D, decisión 8): entradas en
// azul, salidas y cierre en ámbar e informes neutros. Los colores son tokens del tema (styles.css);
// el texto conserva el color de lectura para mantener el contraste en ambos temas.
export type CashActionTone = 'entry' | 'exit' | 'report' | 'closing';

interface ActionDefinition {
  action: CashAction;
  label: string;
  icon: IconName;
  tone: CashActionTone;
  permission: string;
  // Las operaciones de efectivo se registran desde una terminal de la caja del turno.
  ownRegister: boolean;
}

const definitions: ActionDefinition[] = [
  {
    action: 'income',
    label: 'Ingreso',
    icon: 'income',
    tone: 'entry',
    permission: 'cash.income.record',
    ownRegister: true,
  },
  {
    action: 'withdrawal',
    label: 'Retiro de efectivo',
    icon: 'withdrawal',
    tone: 'exit',
    permission: 'cash.withdrawals.record',
    ownRegister: true,
  },
  {
    action: 'expense',
    label: 'Gasto',
    icon: 'expense',
    tone: 'exit',
    permission: 'cash.expenses.record',
    ownRegister: true,
  },
  {
    action: 'partial',
    label: 'Corte parcial',
    icon: 'report',
    tone: 'report',
    permission: 'cash.shifts.read',
    ownRegister: false,
  },
  {
    action: 'closing',
    label: 'Arqueo y cierre',
    icon: 'count',
    tone: 'closing',
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

// Tarjeta táctil de una acción: icono grande arriba del texto y acento por tipo. La seleccionada
// se resalta con borde y fondo (aria-pressed), sin casillas.
export function CashActionCard({
  label,
  icon,
  tone,
  selected,
  onSelect,
}: {
  label: string;
  icon: IconName;
  tone: CashActionTone;
  // Sin valor, la tarjeta no es seleccionable (por ejemplo, la que lleva a Movimientos).
  selected?: boolean;
  onSelect: () => void;
}) {
  return (
    <button
      type="button"
      className={`cash-action-card cash-action-${tone} ${selected ? 'cash-action-selected' : ''}`}
      data-tone={tone}
      aria-pressed={selected}
      onClick={onSelect}
    >
      <span className="cash-action-icon">
        <Icon name={icon} size={30} />
      </span>
      <span className="cash-action-label">{label}</span>
    </button>
  );
}

// Tarjetas de acción del turno. Las correcciones se hacen desde el detalle de cada movimiento.
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
          <CashActionCard
            key={definition.action}
            label={
              definition.action === 'closing' && shift.state === 'closing'
                ? 'Continuar arqueo'
                : definition.label
            }
            icon={definition.icon}
            tone={definition.tone}
            selected={selected === definition.action}
            onSelect={() => onSelect(definition.action)}
          />
        ))}
        {correctable && (
          <CashActionCard
            label="Corrección o regularización"
            icon="edit"
            tone="report"
            onSelect={onCorrections}
          />
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
