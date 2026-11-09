import { useState } from 'react';
import { Field } from '../../../components/Forms';
import { IconLabel } from '../../../components/Icon';
import { SupervisorAuthorization } from '../../../components/SupervisorAuthorization';
import type { FinancialMutation } from '../../../hooks/useFinancialMutation';
import { useSupervisorAuthorization } from '../../../hooks/useSupervisorAuthorization';
import { APIError } from '../../../services/api';
import { formatDeduction, formatSignedMoney } from '../../../services/money';
import { refreshCodes, validReason } from '../../../services/payments';
import type { Adjustment, VoidAdjustmentInput } from '../../../services/payments';
import { serverDateTime } from '../../../services/restaurant';
import { notifyActionError } from '../checkout/actionErrors';

function adjustmentTitle(adjustment: Adjustment) {
  if (adjustment.kind === 'charge') {
    return 'Cargo';
  }
  if (adjustment.courtesy) {
    return 'Cortesía';
  }
  return adjustment.scope === 'items' ? 'Descuento por productos' : 'Descuento general';
}

// Los cargos suman y los descuentos y cortesías restan; Go entrega el importe siempre positivo.
function adjustmentValue(adjustment: Adjustment) {
  if (adjustment.kind === 'charge') {
    return formatSignedMoney(adjustment.amount);
  }
  return formatDeduction(adjustment.amount);
}

// Permiso con el que el propio usuario cancela el ajuste sin autorización de supervisor.
export function ownVoidPermission(adjustment: Adjustment) {
  return adjustment.kind === 'charge' ? 'payments.charges.manage' : 'payments.discounts.apply';
}

function VoidAdjustmentForm({
  accountID,
  adjustment,
  ownPermission,
  mutation,
  reloadFinance,
  onCancel,
}: {
  accountID: string;
  adjustment: Adjustment;
  ownPermission: boolean;
  mutation: FinancialMutation;
  reloadFinance: () => Promise<void>;
  onCancel: () => void;
}) {
  const [reason, setReason] = useState('');
  const auth = useSupervisorAuthorization(!ownPermission);
  const locked = mutation.busy || !!mutation.pending;
  const canSubmit = validReason(reason) && auth.ready && !locked;

  async function submit() {
    if (!canSubmit || mutation.isPending()) {
      return;
    }
    const authorization = auth.take();
    const body: VoidAdjustmentInput = { adjustment_id: adjustment.id, reason: reason.trim() };
    if (authorization) {
      body.authorization = authorization;
    }
    try {
      await mutation.execute('payments_void_adjustment', { id: accountID, body: { ...body } });
    } catch (problem) {
      if (auth.handleError(problem)) {
        return;
      }
      notifyActionError(problem, 'No se pudo cancelar el ajuste.');
      if (problem instanceof APIError && refreshCodes.includes(problem.code)) {
        void reloadFinance();
      }
    }
  }

  return (
    <div className="adjustment-void form-stack">
      <p className="muted text-xs">
        Se registra una cancelación compensatoria; el ajuste original permanece en el historial.
      </p>
      <Field label="Motivo de la cancelación" hint="Obligatorio, de 5 a 500 caracteres.">
        <input
          type="text"
          maxLength={500}
          autoComplete="off"
          value={reason}
          disabled={locked}
          onChange={(event) => setReason(event.target.value)}
        />
      </Field>
      <SupervisorAuthorization auth={auth} disabled={locked} />
      <div className="button-row">
        <button
          type="button"
          className="primary-button"
          disabled={!canSubmit}
          onClick={() => void submit()}
        >
          Confirmar cancelación
        </button>
        <button type="button" className="secondary-button" onClick={onCancel}>
          Conservar ajuste
        </button>
      </div>
    </div>
  );
}

// Ajustes de la cuenta: vigentes y cancelados (estos últimos como registro histórico).
export function AdjustmentList({
  accountID,
  adjustments,
  timezone,
  editable,
  can,
  mutation,
  reloadFinance,
}: {
  accountID: string;
  adjustments: Adjustment[];
  timezone: string;
  editable: boolean;
  can: (permission: string) => boolean;
  mutation: FinancialMutation;
  reloadFinance: () => Promise<void>;
}) {
  const [voiding, setVoiding] = useState('');
  const locked = mutation.busy || !!mutation.pending;
  // Con un permiso cercano se puede pedir la autorización de un supervisor para cancelar.
  const nearPermission = [
    'payments.discounts.apply',
    'payments.courtesy',
    'payments.collect',
    'payments.charges.create',
    'payments.charges.manage',
  ].some(can);
  if (adjustments.length === 0) {
    return <p className="muted text-sm">La cuenta no tiene descuentos, cortesías ni cargos.</p>;
  }
  return (
    <ol className="adjustment-list" aria-label="Ajustes de la cuenta">
      {adjustments.map((adjustment) => (
        <li
          key={adjustment.id}
          className={`adjustment-item ${adjustment.voided ? 'adjustment-voided' : ''}`}
        >
          <div className="payment-history-heading">
            <strong>
              {adjustmentTitle(adjustment)}
              {adjustment.kind === 'discount' &&
                !adjustment.courtesy &&
                adjustment.method === 'percent' &&
                ` · ${adjustment.value} %`}
            </strong>
            <strong>{adjustmentValue(adjustment)}</strong>
          </div>
          <p className="payment-history-detail">
            {adjustment.concept}
            {adjustment.reason && ` · ${adjustment.reason}`}
          </p>
          <p className="muted text-xs">
            {serverDateTime(adjustment.created_at, timezone)} · {adjustment.actor_name}
            {adjustment.authorized_by_name && ` · autorizó ${adjustment.authorized_by_name}`}
          </p>
          {adjustment.voided && (
            <p className="payment-history-detail payment-history-flag">
              Cancelado{adjustment.void_reason ? `: ${adjustment.void_reason}` : ''}
              {adjustment.voided_at && ` · ${serverDateTime(adjustment.voided_at, timezone)}`}
            </p>
          )}
          {!adjustment.voided && editable && nearPermission && voiding !== adjustment.id && (
            <div className="button-row">
              <button
                type="button"
                className="secondary-button"
                disabled={locked}
                onClick={() => setVoiding(adjustment.id)}
              >
                <IconLabel icon="ban">Cancelar ajuste</IconLabel>
              </button>
            </div>
          )}
          {!adjustment.voided && voiding === adjustment.id && (
            <VoidAdjustmentForm
              accountID={accountID}
              adjustment={adjustment}
              ownPermission={can(ownVoidPermission(adjustment))}
              mutation={mutation}
              reloadFinance={reloadFinance}
              onCancel={() => setVoiding('')}
            />
          )}
        </li>
      ))}
    </ol>
  );
}
