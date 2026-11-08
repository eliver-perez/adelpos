import { useState } from 'react';
import { Field } from '../../../components/Forms';
import type { FinancialMutation } from '../../../hooks/useFinancialMutation';
import { APIError } from '../../../services/api';
import { acceptAmountText, isZeroAmountText, toWireAmount } from '../../../services/amountInput';
import { formatMoney, refreshCodes, textLength } from '../../../services/payments';
import type { ChargeInput, Finance } from '../../../services/payments';

// Cargo extra general de la cuenta (por ejemplo, servicio a domicilio). Es distinto de los extras
// de un producto, nunca es negativo y lleva la tasa de IVA vigente al registrarse.
export function ChargeForm({
  accountID,
  finance,
  mutation,
  reloadFinance,
}: {
  accountID: string;
  finance: Finance;
  mutation: FinancialMutation;
  reloadFinance: () => Promise<void>;
}) {
  const [concept, setConcept] = useState('');
  const [amount, setAmount] = useState('');
  const [reason, setReason] = useState('');
  const [conflict, setConflict] = useState('');
  const locked = mutation.busy || !!mutation.pending;
  const wireAmount = toWireAmount(amount);
  const conceptLength = textLength(concept);
  const canSubmit =
    finance.state === 'open' &&
    conceptLength >= 1 &&
    conceptLength <= 120 &&
    wireAmount !== '' &&
    !isZeroAmountText(wireAmount) &&
    textLength(reason) <= 500 &&
    !locked;

  async function submit() {
    if (!canSubmit || mutation.isPending()) {
      return;
    }
    const body: ChargeInput = {
      concept: concept.trim(),
      amount: wireAmount,
      reason: reason.trim(),
      expected_balance: finance.balance,
    };
    setConflict('');
    try {
      await mutation.execute('payments_charge', { id: accountID, body: { ...body } });
    } catch (problem) {
      if (problem instanceof APIError && refreshCodes.includes(problem.code)) {
        setConflict(problem.message);
        void reloadFinance();
      }
    }
  }

  return (
    <div className="form-stack adjustment-form">
      <p className="muted text-xs">
        El cargo forma parte de la venta y lleva la tasa de IVA vigente. No sustituye a los extras
        de un producto, que se capturan en su personalización.
      </p>
      <div className="form-grid">
        <Field label="Concepto del cargo" hint="Obligatorio, hasta 120 caracteres.">
          <input
            type="text"
            maxLength={120}
            autoComplete="off"
            value={concept}
            disabled={locked}
            onChange={(event) => setConcept(event.target.value)}
          />
        </Field>
        <Field label="Importe del cargo ($)" hint="Mayor que cero, hasta dos decimales.">
          <input
            type="text"
            inputMode="decimal"
            autoComplete="off"
            value={amount}
            disabled={locked}
            onChange={(event) => setAmount(acceptAmountText(event.target.value, amount))}
          />
        </Field>
      </div>
      <Field label="Motivo (opcional)" hint="Hasta 500 caracteres.">
        <input
          type="text"
          maxLength={500}
          autoComplete="off"
          value={reason}
          disabled={locked}
          onChange={(event) => setReason(event.target.value)}
        />
      </Field>
      {conflict && (
        <p className="checkout-conflict" role="alert">
          {conflict} Saldo actual: {formatMoney(finance.balance)}.
        </p>
      )}
      <div>
        <button
          type="button"
          className="primary-button"
          disabled={!canSubmit}
          onClick={() => void submit()}
        >
          {wireAmount !== '' ? `Registrar cargo de ${formatMoney(wireAmount)}` : 'Registrar cargo'}
        </button>
      </div>
    </div>
  );
}
