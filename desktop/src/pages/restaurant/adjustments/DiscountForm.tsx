import { useState } from 'react';
import { Field } from '../../../components/Forms';
import { IconLabel } from '../../../components/Icon';
import { QuantityStepper } from '../../../components/QuantityStepper';
import { SegmentedControl } from '../../../components/SegmentedControl';
import { SupervisorAuthorization } from '../../../components/SupervisorAuthorization';
import type { FinancialMutation } from '../../../hooks/useFinancialMutation';
import { useDiscountPreview } from '../../../hooks/usePaymentPreview';
import { useSupervisorAuthorization } from '../../../hooks/useSupervisorAuthorization';
import { APIError } from '../../../services/api';
import { acceptAmountText, toWireAmount } from '../../../services/amountInput';
import { formatDeduction } from '../../../services/money';
import {
  formatMoney,
  isZeroMoney,
  refreshCodes,
  textLength,
  validReason,
} from '../../../services/payments';
import type {
  DiscountInput,
  DiscountMethod,
  DiscountPlan,
  DiscountScope,
  Finance,
  FinanceLine,
  Selection,
} from '../../../services/payments';
import { financeKey } from '../checkout/captureState';
import { notifyActionError } from '../checkout/actionErrors';

// Solo se descuentan unidades sin pago asignado y sin otro descuento manual vigente.
export function discountableUnits(line: FinanceLine) {
  return line.units.filter((unit) => isZeroMoney(unit.paid) && isZeroMoney(unit.discount)).length;
}

function discountSelections(selections: Record<string, number>, finance: Finance): Selection[] {
  const result: Selection[] = [];
  for (const line of finance.lines) {
    const quantity = Math.min(selections[line.version_id] ?? 0, discountableUnits(line));
    if (quantity > 0) {
      result.push({ version_id: line.version_id, quantity });
    }
  }
  return result;
}

function DiscountPreview({ plan, courtesy }: { plan: DiscountPlan; courtesy: boolean }) {
  return (
    <div className="refund-summary" aria-live="polite">
      <dl className="checkout-figures">
        <div className="checkout-figure">
          <dt>Base del descuento</dt>
          <dd>{formatMoney(plan.base)}</dd>
        </div>
        <div className="checkout-figure checkout-figure-emphasis">
          <dt>{courtesy ? 'Cortesía' : 'Descuento'}</dt>
          <dd>{formatDeduction(plan.amount)}</dd>
        </div>
        <div className="checkout-figure">
          <dt>Porcentaje efectivo</dt>
          <dd>{plan.percent} %</dd>
        </div>
        <div className="checkout-figure">
          <dt>Saldo resultante</dt>
          <dd>{formatMoney(plan.balance_after)}</dd>
        </div>
        <div className="checkout-figure">
          <dt>Tu límite de descuento</dt>
          <dd>{plan.actor_limit} %</dd>
        </div>
      </dl>
      {plan.settles && (
        <p className="muted text-xs">
          La cuenta quedará liquidada sin registrar un pago de {formatMoney('0')} y se cerrará si no
          tiene borrador.
        </p>
      )}
      {plan.requires_authorization && (
        <p className="checkout-inline-note">
          Este {courtesy ? 'otorgamiento' : 'descuento'} supera tu límite o tu permiso: requiere la
          autorización de un supervisor.
        </p>
      )}
    </div>
  );
}

// Descuento manual o cortesía (100 %). Go calcula base, importe, porcentaje efectivo y saldo; el
// límite del usuario y la autorización también los decide Go.
export function DiscountForm({
  accountID,
  finance,
  courtesy,
  mutation,
  reloadFinance,
}: {
  accountID: string;
  finance: Finance;
  courtesy: boolean;
  mutation: FinancialMutation;
  reloadFinance: () => Promise<void>;
}) {
  const [scope, setScope] = useState<DiscountScope>('account');
  const [method, setMethod] = useState<DiscountMethod>('percent');
  const [value, setValue] = useState('');
  const [concept, setConcept] = useState('');
  const [reason, setReason] = useState('');
  const [selections, setSelections] = useState<Record<string, number>>({});
  const [conflict, setConflict] = useState('');
  const locked = mutation.busy || !!mutation.pending;
  // La cortesía es siempre un descuento del 100 %: no se captura el valor.
  const wireValue = courtesy ? '100' : toWireAmount(value);
  const chosen = scope === 'items' ? discountSelections(selections, finance) : [];
  let input: DiscountInput | null = null;
  if (finance.state === 'open' && wireValue !== '' && (scope === 'account' || chosen.length > 0)) {
    input = {
      scope,
      method: courtesy ? 'percent' : method,
      value: wireValue,
      courtesy,
    };
    if (scope === 'items') {
      input.selections = chosen;
    }
  }
  const preview = useDiscountPreview(accountID, input, financeKey(finance));
  const plan = preview.plan;
  const auth = useSupervisorAuthorization(plan ? plan.requires_authorization : null);
  const conceptValid = textLength(concept) <= 120;
  const canSubmit =
    !!input &&
    !!plan &&
    !preview.validating &&
    validReason(reason) &&
    conceptValid &&
    auth.ready &&
    !locked;

  async function submit() {
    if (!input || !canSubmit || mutation.isPending()) {
      return;
    }
    // La contraseña se retira del formulario antes de enviar, sea cual sea el resultado.
    const authorization = auth.take();
    const body: DiscountInput = {
      ...input,
      concept: concept.trim(),
      reason: reason.trim(),
      expected_balance: finance.balance,
    };
    if (authorization) {
      body.authorization = authorization;
    }
    setConflict('');
    try {
      await mutation.execute('payments_discount', { id: accountID, body: { ...body } });
    } catch (problem) {
      if (auth.handleError(problem)) {
        return;
      }
      if (problem instanceof APIError && refreshCodes.includes(problem.code)) {
        // La cuenta cambió: se consulta de nuevo y se conserva la captura para revisarla; el
        // conflicto se explica junto al formulario.
        setConflict(problem.message);
        void reloadFinance();
        return;
      }
      notifyActionError(
        problem,
        courtesy ? 'No se pudo registrar la cortesía.' : 'No se pudo registrar el descuento.',
      );
    }
  }

  const label = courtesy ? 'Otorgar cortesía' : 'Aplicar descuento';
  return (
    <div className="form-stack adjustment-form">
      <div className="form-grid">
        <div className="field">
          <span>Alcance</span>
          <SegmentedControl<DiscountScope>
            label="Alcance del descuento"
            value={scope}
            onChange={setScope}
            disabled={locked}
            options={[
              { value: 'account', label: 'Toda la cuenta' },
              { value: 'items', label: 'Por productos' },
            ]}
          />
        </div>
        {courtesy ? (
          <div className="field">
            <span>Valor</span>
            <p className="courtesy-value">100 % · Cortesía</p>
          </div>
        ) : (
          <div className="field">
            <span>Tipo</span>
            <SegmentedControl<DiscountMethod>
              label="Tipo de descuento"
              value={method}
              onChange={(next) => {
                setMethod(next);
                setValue('');
              }}
              disabled={locked}
              options={[
                { value: 'percent', label: 'Porcentaje' },
                { value: 'amount', label: 'Importe' },
              ]}
            />
          </div>
        )}
      </div>
      {!courtesy && (
        <Field
          label={method === 'percent' ? 'Porcentaje de descuento (%)' : 'Importe del descuento ($)'}
          hint="Hasta dos decimales. Go valida el límite y el importe."
        >
          <input
            type="text"
            inputMode="decimal"
            autoComplete="off"
            value={value}
            disabled={locked}
            onChange={(event) => setValue(acceptAmountText(event.target.value, value))}
          />
        </Field>
      )}
      {scope === 'items' && (
        <div className="adjustment-lines" role="group" aria-label="Partidas por descontar">
          {finance.lines.map((line) => (
            <div className="adjustment-line" key={line.version_id}>
              <span>
                {line.quantity} × {line.name} · {formatMoney(line.due)}
              </span>
              {discountableUnits(line) === 0 ? (
                <span className="muted text-xs">
                  Sin unidades descontables (con pago o descuento)
                </span>
              ) : (
                <QuantityStepper
                  verb="Descontar"
                  name={line.name}
                  quantity={selections[line.version_id] ?? 0}
                  max={discountableUnits(line)}
                  disabled={locked}
                  onChange={(quantity) =>
                    setSelections({ ...selections, [line.version_id]: quantity })
                  }
                />
              )}
            </div>
          ))}
        </div>
      )}
      <div className="form-grid">
        <Field label="Concepto (opcional)" hint="Hasta 120 caracteres.">
          <input
            type="text"
            maxLength={120}
            autoComplete="off"
            value={concept}
            disabled={locked}
            onChange={(event) => setConcept(event.target.value)}
          />
        </Field>
        <Field label="Motivo" hint="Obligatorio, de 5 a 500 caracteres.">
          <input
            type="text"
            maxLength={500}
            autoComplete="off"
            value={reason}
            disabled={locked}
            onChange={(event) => setReason(event.target.value)}
          />
        </Field>
      </div>
      {plan && <DiscountPreview plan={plan} courtesy={courtesy} />}
      {preview.error && (
        <p className="checkout-inline-note" role="alert">
          {preview.error.message}
        </p>
      )}
      {conflict && (
        <p className="checkout-conflict" role="alert">
          {conflict} Saldo actual: {formatMoney(finance.balance)}.
        </p>
      )}
      <SupervisorAuthorization
        auth={auth}
        disabled={locked}
        description={
          courtesy
            ? 'La cortesía requiere un supervisor con el permiso «Otorgar cortesías». La autorización es puntual y no inicia sesión.'
            : 'El descuento requiere un supervisor con el permiso «Autorizar descuentos superiores al límite» y un límite suficiente. La autorización es puntual y no inicia sesión.'
        }
      />
      <div>
        <button
          type="button"
          className="primary-button"
          disabled={!canSubmit}
          onClick={() => void submit()}
        >
          <IconLabel icon="discount">
            {plan ? `${label} ${formatMoney(plan.amount)}` : label}
          </IconLabel>
        </button>
      </div>
    </div>
  );
}
