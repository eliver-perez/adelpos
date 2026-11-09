import { useState } from 'react';
import { Accordion } from '../../components/Accordion';
import { Notice } from '../../components/Forms';
import { denominationLabel } from '../../services/cash';
import type { ShiftDetail } from '../../services/cash';
import { permissionName } from '../../services/payments';
import type { Shift } from '../../services/payments';
import { ShiftOpening } from '../restaurant/checkout/ShiftOpening';
import { CashActions } from './CashActions';
import type { CashAction } from './CashActions';
import { CashOperationForm } from './CashOperationForm';
import { ClosingPanel } from './ClosingPanel';
import { ClosureResult } from './ClosureResult';
import { currencyAmount } from './DenominationCount';
import { ExpectedFigures } from './ExpectedBlock';
import { PartialCutPanel } from './PartialCutPanel';
import { PendingCashOperation } from './PendingCashOperation';
import { ClosingNotice, ShiftStatusCard } from './ShiftStatusCard';
import { useCashMutation } from './useCashMutation';
import type { CashWorkspace } from './useCashWorkspace';

export type CashSection = 'operation' | 'movements' | 'history' | 'reports' | 'settings';

// Registros del turno sin importes y desglose del fondo inicial, si se capturó.
function ShiftRecords({ detail }: { detail: ShiftDetail }) {
  const summary = detail.summary;
  const opening = detail.opening_count;
  return (
    <>
      <p className="muted text-sm">
        Registros del turno: {summary.payments} cobros · {summary.refunds} devoluciones ·{' '}
        {summary.cash_operations} operaciones de efectivo · {summary.partial_cuts} cortes parciales.
      </p>
      {opening && (
        <Accordion title="Fondo inicial por denominaciones" summary="Desglose capturado al abrir">
          <ul className="cash-count-lines">
            {opening.lines.map((line) => (
              <li key={line.denomination_id}>
                {line.quantity} × {denominationLabel(line)} ={' '}
                {currencyAmount(line.currency, line.amount)}
              </li>
            ))}
          </ul>
        </Accordion>
      )}
    </>
  );
}

// Efectivo esperado vigente: solo con el turno abierto y si Go lo entregó. Durante el arqueo la
// pantalla no lo muestra aunque una respuesta lo trajera.
function ExpectedNow({ shift, detail }: { shift: Shift; detail: ShiftDetail | null }) {
  if (shift.state !== 'open' || !detail) {
    return null;
  }
  if (detail.expected_visible && detail.expected) {
    return <ExpectedFigures expected={detail.expected} />;
  }
  return (
    <p className="muted text-xs">
      El efectivo esperado se muestra con el permiso «{permissionName('cash.expected.read')}».
    </p>
  );
}

// Trabajo sobre un turno activo: datos, acciones y el panel de la acción elegida. Se monta por
// turno para que una acción a medias no pase a otro turno.
function ShiftWorkspace({
  workspace,
  shift,
  can,
  userID,
  timezone,
  onNavigate,
  onClosed,
}: {
  workspace: CashWorkspace;
  shift: Shift;
  can: (permission: string) => boolean;
  userID: string;
  timezone: string;
  onNavigate: (section: CashSection) => void;
  onClosed: (shiftID: string) => void;
}) {
  const [chosen, setChosen] = useState<CashAction | null>(null);
  const mutation = useCashMutation(shift.id);
  const ownRegister = !!workspace.selected?.own;
  const catalog = workspace.catalog.data;
  const detail = workspace.detail;
  // Con la caja en arqueo, quien puede contar entra directo al arqueo.
  const action =
    chosen ?? (shift.state === 'closing' && can('cash.counts.perform') ? 'closing' : null);

  function choose(next: CashAction | null) {
    mutation.clearError();
    setChosen(next);
  }

  function panel() {
    if (!action) {
      return null;
    }
    if (!can('payments.read')) {
      return (
        <Notice>
          Las operaciones de caja usan el catálogo de denominaciones y categorías, que requiere el
          permiso «{permissionName('payments.read')}».
        </Notice>
      );
    }
    if (!catalog) {
      return <Notice>{workspace.catalog.error || 'Consultando el catálogo de caja…'}</Notice>;
    }
    if (action === 'partial') {
      return (
        <PartialCutPanel
          shift={shift}
          reports={detail?.reports ?? []}
          can={can}
          mutation={mutation}
          timezone={timezone}
          onWithdraw={() => choose('withdrawal')}
          onClose={() => choose(null)}
        />
      );
    }
    if (action === 'closing') {
      return (
        <ClosingPanel
          shift={shift}
          catalog={catalog}
          can={can}
          userID={userID}
          usdCash={workspace.usdCash}
          mutation={mutation}
          timezone={timezone}
          onClosed={onClosed}
          onClose={() => choose(null)}
        />
      );
    }
    return (
      <CashOperationForm
        key={action}
        kind={action}
        shift={shift}
        catalog={catalog}
        can={can}
        dollars={workspace.usdCash && !!shift.usd_rate}
        active
        mutation={mutation}
        onClose={() => choose(null)}
      />
    );
  }

  return (
    <div className="form-stack">
      <ClosingNotice shift={shift} timezone={timezone} />
      <ShiftStatusCard shift={shift} timezone={timezone} />
      <ExpectedNow shift={shift} detail={detail} />
      {detail && shift.state === 'open' && <ShiftRecords detail={detail} />}
      <PendingCashOperation mutation={mutation} />
      <CashActions
        can={can}
        shift={shift}
        ownRegister={ownRegister}
        selected={action}
        onSelect={(next) => choose(next)}
        onCorrections={() => onNavigate('movements')}
      />
      {panel()}
    </div>
  );
}

// Vista operativa de Caja (§19): caja seleccionada, turno, aviso de «Cierre en curso», acciones
// según permisos y apertura cuando la caja de la terminal no tiene turno.
export function OperationalView({
  workspace,
  can,
  userID,
  timezone,
  onNavigate,
}: {
  workspace: CashWorkspace;
  can: (permission: string) => boolean;
  userID: string;
  timezone: string;
  onNavigate: (section: CashSection) => void;
}) {
  const [closedShiftID, setClosedShiftID] = useState('');
  const { selected, shift, current } = workspace;
  if (closedShiftID) {
    return (
      <div className="form-stack">
        <ClosureResult shiftID={closedShiftID} can={can} timezone={timezone} />
        <div className="button-row">
          <button
            type="button"
            className="primary-button"
            onClick={() => {
              setClosedShiftID('');
              workspace.reload();
            }}
          >
            Volver a la caja
          </button>
        </div>
      </div>
    );
  }
  if (!selected) {
    return (
      <Notice>
        Esta terminal no tiene una caja física asignada. Asígnala desde Terminales para operar la
        caja.
      </Notice>
    );
  }
  if (!shift) {
    return (
      <div className="cash-panel">
        <p className="muted text-sm">No hay un turno abierto en {selected.register.name}.</p>
        {selected.own && current.data && (
          <ShiftOpening
            current={current.data}
            can={can}
            active
            onChanged={() => workspace.reload()}
          />
        )}
        {!selected.own && (
          <p className="muted text-xs">El turno se abre desde una terminal de esa caja.</p>
        )}
      </div>
    );
  }
  return (
    <ShiftWorkspace
      key={shift.id}
      workspace={workspace}
      shift={shift}
      can={can}
      userID={userID}
      timezone={timezone}
      onNavigate={onNavigate}
      onClosed={setClosedShiftID}
    />
  );
}
