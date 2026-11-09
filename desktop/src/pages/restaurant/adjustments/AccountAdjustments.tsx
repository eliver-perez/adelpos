import { useState } from 'react';
import { Notice } from '../../../components/Forms';
import { IconLabel } from '../../../components/Icon';
import { SegmentedControl } from '../../../components/SegmentedControl';
import { useFinancialMutation } from '../../../hooks/useFinancialMutation';
import { useLiveResource } from '../../../hooks/useLiveResource';
import {
  isAuthorizationProblem,
  repeatAuthorizationMessage,
} from '../../../hooks/useSupervisorAuthorization';
import { notifyError, notifySuccess } from '../../../services/notify';
import { financialOutcome, formatMoney, permissionName } from '../../../services/payments';
import type { Finance } from '../../../services/payments';
import type { Account, Bill } from '../../../services/restaurant';
import { newestFinance } from '../checkout/captureState';
import { UncertainOperation } from '../checkout/UncertainOperation';
import { notifyActionError } from '../checkout/actionErrors';
import { AdjustmentList } from './AdjustmentList';
import { ChargeForm } from './ChargeForm';
import { DiscountForm } from './DiscountForm';

export type AdjustmentKind = 'discount' | 'courtesy' | 'charge';

// Permisos «cercanos»: con ellos se ofrece la acción y, si falta el permiso propio, Go pide la
// autorización de un supervisor. Go sigue siendo la autoridad sobre cada operación.
const discountPermissions = ['payments.discounts.apply', 'payments.courtesy', 'payments.collect'];

export function adjustmentKinds(can: (permission: string) => boolean) {
  const kinds: { value: AdjustmentKind; label: string }[] = [];
  if (discountPermissions.some(can)) {
    kinds.push({ value: 'discount', label: 'Descuento' });
    kinds.push({ value: 'courtesy', label: 'Cortesía' });
  }
  // Un cargo no admite autorización de supervisor: solo se ofrece con su permiso propio.
  if (can('payments.charges.create')) {
    kinds.push({ value: 'charge', label: 'Cargo extra' });
  }
  return kinds;
}

// Descuentos, cortesías y cargos se administran desde las acciones de una cuenta abierta.
export function adjustmentsAvailable(account: Account, can: (permission: string) => boolean) {
  return (
    account.state === 'open' &&
    can('payments.read') &&
    (adjustmentKinds(can).length > 0 || can('payments.charges.manage'))
  );
}

// Único componente de ajustes de la cuenta: se abre desde las acciones de la cuenta y, como
// acceso contextual, desde la pantalla de cobro. Comparte la clave de pendientes de la cuenta.
export function AccountAdjustments({
  bill,
  active,
  can,
  onClose,
}: {
  bill: Bill;
  active: boolean;
  can: (permission: string) => boolean;
  onClose: () => void;
}) {
  const accountID = bill.account.id;
  const polled = useLiveResource<Finance>('payments_account', { id: accountID, enabled: active });
  const [recorded, setRecorded] = useState<Finance | null>(null);
  const [generation, setGeneration] = useState(0);
  const kinds = adjustmentKinds(can);
  const [kind, setKind] = useState<AdjustmentKind>(kinds[0]?.value ?? 'discount');
  const mutation = useFinancialMutation(accountID, (operation, result) => {
    const outcome = financialOutcome(operation, result);
    if (outcome.finance) {
      setRecorded(outcome.finance);
    }
    notifySuccess(outcome.message);
    // Tras registrar, el formulario vuelve a su estado inicial.
    setGeneration((value) => value + 1);
  });
  const finance = newestFinance(polled.data, recorded, accountID);
  const pending = mutation.pending;

  async function retryPending() {
    const authorized = typeof pending?.body.authorization === 'object';
    const problem = await mutation.retry();
    if (!problem) {
      return;
    }
    // Un reintento autorizado que no llegó a registrarse pide repetir la autorización.
    if (authorized && isAuthorizationProblem(problem)) {
      notifyError(repeatAuthorizationMessage);
      return;
    }
    notifyActionError(problem, 'No se pudo registrar el ajuste pendiente.');
  }

  return (
    <section className="operation-inline-panel adjustments-panel" aria-label="Descuentos y cargos">
      <div className="section-heading">
        <h2>Descuentos y cargos · #{bill.account.folio}</h2>
        <button type="button" className="secondary-button" onClick={onClose}>
          <IconLabel icon="back">Volver</IconLabel>
        </button>
      </div>
      <Notice>{polled.error}</Notice>
      {pending && mutation.uncertain && (
        <UncertainOperation
          request={pending}
          methods={[]}
          busy={mutation.busy}
          onRetry={() => void retryPending()}
          onConsult={() => void polled.reload()}
        />
      )}
      {!finance && !polled.error && <p className="muted">Consultando el estado financiero…</p>}
      {finance && (
        <>
          <dl className="checkout-figures adjustments-figures">
            <div className="checkout-figure">
              <dt>Total</dt>
              <dd>{formatMoney(finance.total)}</dd>
            </div>
            <div className="checkout-figure">
              <dt>Pagado</dt>
              <dd>{formatMoney(finance.paid)}</dd>
            </div>
            <div className="checkout-figure checkout-figure-emphasis">
              <dt>Saldo pendiente</dt>
              <dd>{formatMoney(finance.balance)}</dd>
            </div>
          </dl>
          {finance.state !== 'open' && (
            <Notice>
              La cuenta no está abierta; no admite descuentos, cargos ni cancelaciones.
            </Notice>
          )}
          {finance.state === 'open' && kinds.length > 0 && (
            <>
              <SegmentedControl<AdjustmentKind>
                label="Tipo de ajuste"
                value={kind}
                onChange={setKind}
                options={kinds}
              />
              {kind === 'charge' ? (
                <ChargeForm
                  key={`charge-${generation}`}
                  accountID={accountID}
                  finance={finance}
                  mutation={mutation}
                  reloadFinance={polled.reload}
                />
              ) : (
                <DiscountForm
                  key={`${kind}-${generation}`}
                  accountID={accountID}
                  finance={finance}
                  courtesy={kind === 'courtesy'}
                  mutation={mutation}
                  reloadFinance={polled.reload}
                />
              )}
            </>
          )}
          {finance.state === 'open' && kinds.length === 0 && (
            <p className="muted text-sm">
              Registrar descuentos requiere «{permissionName('payments.discounts.apply')}» y los
              cargos «{permissionName('payments.charges.create')}».
            </p>
          )}
          <h3 className="bill-section-title">Ajustes registrados</h3>
          <AdjustmentList
            accountID={accountID}
            adjustments={finance.adjustments}
            timezone={bill.timezone}
            editable={finance.state === 'open'}
            can={can}
            mutation={mutation}
            reloadFinance={polled.reload}
          />
        </>
      )}
    </section>
  );
}
