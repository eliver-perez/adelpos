import { useId, useState } from 'react';
import {
  authorizationCodes,
  useSupervisorAuthorization,
} from '../../hooks/useSupervisorAuthorization';
import type { CashCorrectionResult, CashOperation } from '../../services/cash';
import { validReason } from '../../services/payments';
import type { Shift } from '../../services/payments';
import { OutflowAuthorization } from './OutflowAuthorization';
import { PendingCashOperation } from './PendingCashOperation';
import { RecordedOperation } from './RecordedOperation';
import { useCashMutation } from './useCashMutation';

// Corrección (mismo turno abierto) o regularización (turno cerrado de la misma caja) de una
// operación de efectivo, con motivo y autorización (§16 y §0-B). Se registra en el turno vigente
// de la caja de la terminal; la original nunca se modifica.
export function CorrectionForm({
  operation,
  target,
  can,
  onDone,
}: {
  operation: CashOperation;
  target: Shift;
  can: (permission: string) => boolean;
  onDone: () => void;
}) {
  const reasonID = useId();
  const [reason, setReason] = useState('');
  const [result, setResult] = useState<CashCorrectionResult | null>(null);
  const mutation = useCashMutation(target.id);
  const selfAuthorized = can('cash.corrections.authorize');
  const auth = useSupervisorAuthorization(!selfAuthorized);
  const regularization = operation.shift_id !== target.id;
  const ready = validReason(reason) && auth.ready && !mutation.locked;

  async function register() {
    if (!ready || mutation.isPending()) {
      return;
    }
    const authorization = auth.take();
    const body: Record<string, unknown> = {
      corrects_operation_id: operation.id,
      reason: reason.trim(),
    };
    if (authorization) {
      body.authorization = authorization;
    }
    try {
      const saved = await mutation.execute<CashCorrectionResult>('payments_shift_correction', {
        id: target.id,
        body,
      });
      setResult(saved);
      setReason('');
      auth.reset();
    } catch (problem) {
      auth.handleError(problem);
    }
  }

  function submitLabel() {
    if (mutation.busy) {
      return 'Registrando…';
    }
    return regularization ? 'Registrar regularización' : 'Registrar corrección';
  }

  if (result) {
    return (
      <div className="form-stack">
        <RecordedOperation operation={result.correction} />
        <button type="button" className="secondary-button" onClick={onDone}>
          Volver a los movimientos
        </button>
      </div>
    );
  }
  return (
    <section
      className="cash-panel"
      aria-label={regularization ? 'Regularización del movimiento' : 'Corrección del movimiento'}
    >
      <h4>{regularization ? 'Regularizar en el turno vigente' : 'Corregir el movimiento'}</h4>
      <p className="muted text-sm">
        {regularization
          ? `Se registra en el turno vigente de ${target.register_name}, enlazada al movimiento del turno cerrado. No cambia la diferencia registrada de ese turno ni el efectivo esperado del turno vigente.`
          : 'Se registra un movimiento enlazado que invierte exactamente el original (misma moneda e importe). El original se conserva; para cambiar un importe, corrígelo y registra la operación correcta.'}
      </p>
      <div className="field">
        <label htmlFor={reasonID}>Motivo</label>
        <textarea
          id={reasonID}
          rows={2}
          maxLength={500}
          aria-describedby={`${reasonID}-hint`}
          value={reason}
          onChange={(event) => {
            setReason(event.target.value);
            mutation.clearError();
          }}
        />
        <small id={`${reasonID}-hint`} className="muted">
          Entre 5 y 500 caracteres.
        </small>
      </div>
      <OutflowAuthorization
        selfAuthorized={selfAuthorized}
        auth={auth}
        disabled={mutation.locked}
        permission="cash.corrections.authorize"
      />
      <PendingCashOperation mutation={mutation} />
      {mutation.error && !authorizationCodes.includes(mutation.errorCode) && (
        <p className="checkout-inline-note" role="alert">
          {mutation.error}
        </p>
      )}
      <div className="button-row">
        <button
          type="button"
          className="primary-button"
          disabled={!ready}
          onClick={() => void register()}
        >
          {submitLabel()}
        </button>
        <button type="button" className="secondary-button" onClick={onDone}>
          Cancelar
        </button>
      </div>
    </section>
  );
}
