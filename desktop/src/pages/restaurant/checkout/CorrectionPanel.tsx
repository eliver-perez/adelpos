import type { ReactNode } from 'react';
import { Field } from '../../../components/Forms';
import { IconLabel } from '../../../components/Icon';
import { SupervisorAuthorization } from '../../../components/SupervisorAuthorization';
import { formatMoney, isZeroMoney, permissionName } from '../../../services/payments';
import type { CheckoutStatus } from './PaymentCapture';
import type { CorrectionFlow } from './useCorrectionFlow';

// Corrección del método de un pago capturado por error. El método correcto se elige en el área de
// métodos; Go registra un reverso y un pago nuevo con el mismo importe y propina.
export function CorrectionPanel({
  flow,
  status,
  banner,
  canCorrect,
  registering,
  onRefundInstead,
}: {
  flow: CorrectionFlow;
  status: CheckoutStatus;
  banner: ReactNode;
  canCorrect: boolean;
  registering: boolean;
  onRefundInstead: () => void;
}) {
  const original = flow.original;
  let registerLabel = 'Elige el método correcto';
  if (registering) {
    registerLabel = 'Registrando corrección…';
  } else if (flow.method) {
    registerLabel = `Corregir a ${flow.method.name}`;
  }
  return (
    <section className="checkout-area checkout-capture" aria-label="Corrección de método de pago">
      <header className="checkout-area-header">
        <h3>Corregir método de pago</h3>
        <span className={`checkout-status checkout-status-${status.state}`} role="status">
          {status.label}
        </span>
      </header>
      <div className="checkout-area-body checkout-capture-fields">
        {banner}
        {original && (
          <div className="refund-origin">
            <span>
              Registrado con {original.method_name}: {formatMoney(original.amount)}
              {!isZeroMoney(original.tip) && ` · propina ${formatMoney(original.tip)}`}
            </span>
            <p className="muted text-xs">
              Se registrará un reverso del pago original y un pago nuevo por el mismo importe con el
              método correcto. El pago original no se edita. Solo procede mientras su turno siga
              abierto.
            </p>
          </div>
        )}
        {!canCorrect && (
          <p className="muted text-xs">
            Sin el permiso «{permissionName('payments.corrections')}», la corrección requiere la
            autorización de un supervisor con ese permiso.
          </p>
        )}
        <p className="checkout-inline-note">
          {flow.method
            ? `Método correcto: ${flow.method.name}.`
            : 'Elige en «Método correcto» el método con el que realmente se pagó.'}
        </p>
        {flow.method && flow.method.reference_mode !== 'none' && (
          <Field
            label={
              flow.method.reference_mode === 'required'
                ? 'Referencia (obligatoria)'
                : 'Referencia (opcional)'
            }
          >
            <input
              type="text"
              maxLength={120}
              autoComplete="off"
              value={flow.reference}
              disabled={flow.locked}
              onChange={(event) => flow.setReference(event.target.value)}
            />
          </Field>
        )}
        <Field label="Motivo de la corrección" hint="Obligatorio, de 5 a 500 caracteres.">
          <input
            type="text"
            maxLength={500}
            autoComplete="off"
            value={flow.reason}
            disabled={flow.locked}
            onChange={(event) => flow.setReason(event.target.value)}
          />
        </Field>
        <SupervisorAuthorization auth={flow.auth} disabled={flow.locked} />
        {flow.notice && (
          <div className="checkout-conflict" role="alert">
            <p>{flow.notice}</p>
            {flow.suggestRefund && (
              <button type="button" className="secondary-button" onClick={onRefundInstead}>
                <IconLabel icon="refund">Ir a devoluciones</IconLabel>
              </button>
            )}
          </div>
        )}
      </div>
      <footer className="checkout-area-footer checkout-capture-footer">
        <div className="button-row">
          <button
            type="button"
            className="secondary-button"
            disabled={registering}
            onClick={flow.cancel}
          >
            <IconLabel icon="back">Cancelar corrección</IconLabel>
          </button>
        </div>
        <button
          type="button"
          className="primary-button checkout-register"
          disabled={!flow.canRegister}
          onClick={() => void flow.register()}
        >
          <IconLabel icon="edit">{registerLabel}</IconLabel>
        </button>
      </footer>
    </section>
  );
}
