import { useState } from 'react';
import type { FormEvent } from 'react';
import { useConfirmDialog } from '../../components/ConfirmDialog';
import { Field, Notice } from '../../components/Forms';
import { IconLabel } from '../../components/Icon';
import { SupervisorAuthorization } from '../../components/SupervisorAuthorization';
import type { PendingRequest } from '../../hooks/pendingOperations';
import { useResource } from '../../hooks/useResource';
import {
  isAuthorizationProblem,
  repeatAuthorizationMessage,
  useSupervisorAuthorization,
} from '../../hooks/useSupervisorAuthorization';
import { APIError } from '../../services/api';
import { featureEnabled } from '../../services/features';
import type { EstablishmentFeature } from '../../services/features';
import { formatMoney } from '../../services/money';
import { notifyError, notifySuccess } from '../../services/notify';
import { permissionName, textLength, validReasonDescription } from '../../services/payments';
import { countLabel } from '../../services/plural';
import { accountLabel, serverDateTime } from '../../services/restaurant';
import type {
  Account,
  AccountCancellation,
  Bill,
  CancelAccountInput,
  CancelBlocker,
  CancelReason,
} from '../../services/restaurant';
import { adjustmentsAvailable } from './adjustments/AccountAdjustments';
import { uncertainError } from './capture/draftSync';
import type { ExecuteMutation } from './OperationsAdmin';

// «Cancelar cuenta» (etapa 9D, §5). Cualquier cuenta abierta sin pagos ni ajustes vigentes, de
// mesa o para llevar, con o sin productos enviados, se cancela con un motivo del catálogo. Go
// decide todo: si basta el permiso propio («Cancelar cuentas») o hace falta un supervisor
// (`cancel_requires_authorization`), qué la impide (`cancel_blocker`), el importe y las unidades
// que se cancelan y sus efectos en cocina e inventario. La interfaz solo explica y pide
// confirmación.

// Texto breve junto a «Cancelar cuenta» en el menú «Más…» cuando Go indica qué hacer primero.
export const cancelBlockerHints: Record<Exclude<CancelBlocker, ''>, string> = {
  payments: 'Primero registra la devolución',
  adjustments: 'Primero quita el descuento o cargo',
};

// Operación de «Cancelar cuenta» con resultado incierto (conservada sin la contraseña del
// supervisor).
export interface CancelRecovery {
  uncertain: boolean;
  busy: boolean;
  pending: PendingRequest | null;
  retry: () => Promise<unknown>;
}

// Códigos de Go que corresponden al motivo o a su descripción: se explican junto al campo.
const reasonCodes = [
  'CANCEL_DESCRIPTION_REQUIRED',
  'CANCEL_DESCRIPTION_TOO_LONG',
  'CANCEL_REASON_INACTIVE',
  'NOT_FOUND',
];

// Qué pasa con lo enviado a cocina según el motivo elegido (marca «lo enviado a cocina cuenta como
// merma» del catálogo) y si el establecimiento lleva control de inventario.
export function kitchenEffect(
  units: number,
  reason: Pick<CancelReason, 'prepared_is_waste'> | null,
  inventory: boolean,
): string[] {
  if (units === 0) {
    return ['La cuenta no tiene productos enviados a cocina.'];
  }
  const lines = [
    'Lo que aún no se libera a preparación se retira sin imprimir; las estaciones reciben el aviso de cancelación de lo ya liberado.',
  ];
  if (!reason) {
    lines.push('Lo ya preparado se trata según el motivo que elijas.');
  } else if (reason.prepared_is_waste) {
    lines.push(
      inventory
        ? 'Con este motivo, lo preparado cuenta como merma: en Inventarios queda como «Preparado / merma» y el encargado puede corregirlo hasta el corte del día.'
        : 'Con este motivo, lo preparado cuenta como merma.',
    );
  } else {
    lines.push(
      inventory
        ? 'Con este motivo, lo ya preparado queda por clasificar en Inventarios (preparado o no preparado), como cualquier cancelación.'
        : 'Con este motivo, lo ya preparado no se marca como merma.',
    );
  }
  return lines;
}

// Títulos y avisos con el nombre de siempre: «la cuenta #7» o «el pedido #8» (para llevar).
function confirmTitle(account: Account) {
  const label = accountLabel(account);
  if (account.mode === 'takeaway') {
    return `¿Cancelar el ${label.replace(/^Pedido/, 'pedido')}?`;
  }
  return `¿Cancelar la ${label.replace(/^Cuenta/, 'cuenta')}?`;
}

function cancelledMessage(account: Account) {
  return `${accountLabel(account)} ${account.mode === 'takeaway' ? 'cancelado' : 'cancelada'}.`;
}

// Dónde queda la cuenta cancelada: nunca es venta y la mesa se libera si no tiene otras cuentas.
function outcomeText(account: Account) {
  const report = 'No cuenta como venta: aparece en el reporte «Cuentas canceladas».';
  if (account.mode === 'dine_in' && account.table_name) {
    return `${report} ${account.table_name} queda libre si no tiene otras cuentas abiertas.`;
  }
  return report;
}

// Explicación cuando Go no permite cancelar todavía: qué hacer primero y un acceso directo al lugar
// donde se hace (si el usuario tiene el permiso).
function CancelBlocked({
  account,
  blocker,
  can,
  busy,
  onOpenPanel,
}: {
  account: Account;
  blocker: Exclude<CancelBlocker, ''>;
  can: (permission: string) => boolean;
  busy: boolean;
  onOpenPanel: (panel: 'refund' | 'adjustments') => void;
}) {
  if (blocker === 'payments') {
    const refundAllowed =
      can('payments.read') && (can('payments.refunds.execute') || can('payments.collect'));
    return (
      <div className="cancel-account-blocked" role="alert">
        <strong>{cancelBlockerHints.payments}</strong>
        <p>
          La cuenta tiene pagos o propinas registrados
          {account.finance ? ` (pagado ${formatMoney(account.finance.paid)})` : ''}. Una cuenta con
          pagos no se cancela: primero devuelve los pagos desde el cobro, en «Devoluciones». Cuando
          no quede nada pagado podrás cancelarla con su motivo.
        </p>
        {refundAllowed ? (
          <div>
            <button
              type="button"
              className="secondary-button"
              disabled={busy}
              onClick={() => onOpenPanel('refund')}
            >
              <IconLabel icon="refund">Ir a devoluciones</IconLabel>
            </button>
          </div>
        ) : (
          <p className="muted text-xs">
            Registrar la devolución requiere el permiso «
            {permissionName('payments.refunds.execute')}».
          </p>
        )}
      </div>
    );
  }
  return (
    <div className="cancel-account-blocked" role="alert">
      <strong>{cancelBlockerHints.adjustments}</strong>
      <p>
        La cuenta tiene un descuento, una cortesía o un cargo vigente. Quítalo primero desde
        «Descuentos y cargos» con «Cancelar ajuste»; después podrás cancelar la cuenta con su
        motivo.
      </p>
      {adjustmentsAvailable(account, can) ? (
        <div>
          <button
            type="button"
            className="secondary-button"
            disabled={busy}
            onClick={() => onOpenPanel('adjustments')}
          >
            <IconLabel icon="discount">Ver descuentos y cargos</IconLabel>
          </button>
        </div>
      ) : (
        <p className="muted text-xs">
          Para quitar el ajuste hacen falta los permisos de descuentos o de cargos de la cuenta.
        </p>
      )}
    </div>
  );
}

export function CancelAccount({
  bill,
  can,
  execute,
  recovery,
  busy,
  onOpenPanel,
  onClose,
}: {
  bill: Bill;
  can: (permission: string) => boolean;
  execute: ExecuteMutation;
  // Operación pendiente del espacio de trabajo: el reintento usa el mismo operation_id.
  recovery: CancelRecovery;
  busy: boolean;
  onOpenPanel: (panel: 'refund' | 'adjustments') => void;
  onClose: () => void;
}) {
  const account = bill.account;
  const [revision, setRevision] = useState(account.revision);
  const [reasonID, setReasonID] = useState('');
  const [description, setDescription] = useState('');
  const [reasonError, setReasonError] = useState('');
  const [saving, setSaving] = useState(false);
  const { confirm, dialog } = useConfirmDialog();
  const reasons = useResource<CancelReason[]>('restaurant_cancel_reasons');
  const features = useResource<EstablishmentFeature[]>('settings_features');
  const inventory = featureEnabled(features.data, 'inventory_control');
  // Go decide desde el principio si hace falta un supervisor (el usuario no tiene «Cancelar
  // cuentas»); un rechazo AUTHORIZATION_REQUIRED también muestra el formulario.
  const auth = useSupervisorAuthorization(account.cancel_requires_authorization);
  const active = reasons.data ? reasons.data.filter((entry) => entry.active) : null;
  const reason = active?.find((entry) => entry.id === reasonID) ?? null;
  const units = account.confirmed_quantity;
  const draftItems = Math.max(account.draft_quantity, bill.draft.length);
  const stale = revision !== account.revision;
  const recovering =
    recovery.uncertain &&
    recovery.pending?.operation === 'restaurant_cancel_account' &&
    recovery.pending.id === account.id;
  const locked = busy || saving || stale || recovering;
  const descriptionTooLong = textLength(description) > 500;
  const ready = !!reason && validReasonDescription(reason, description) && auth.ready && !locked;
  const blocker = account.cancel_blocker;

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!reason || !ready) {
      return;
    }
    const confirmed = await confirm({
      title: confirmTitle(account),
      message: (
        <>
          <p>
            Motivo: {reason.name}
            {description.trim() ? ` · ${description.trim()}` : ''}
          </p>
          <p>
            {units > 0
              ? `Se cancelan ${countLabel(units, 'unidad enviada', 'unidades enviadas')} a cocina por ${formatMoney(account.totals.total)}.`
              : 'La cuenta no tiene productos enviados a cocina.'}
            {draftItems > 0 ? ' Los productos nuevos sin enviar se descartan.' : ''}
          </p>
          {/* Sin productos enviados el párrafo anterior ya lo dice; con ellos, lo que pasa con lo
              ya preparado según el motivo. */}
          {(units > 0 ? kitchenEffect(units, reason, inventory).slice(1) : []).map((line) => (
            <p key={line}>{line}</p>
          ))}
          <p>{outcomeText(account)} No se puede deshacer.</p>
        </>
      ),
      confirmLabel: 'Cancelar cuenta',
      cancelLabel: 'Conservar la cuenta',
      tone: 'danger',
    });
    if (!confirmed) {
      return;
    }
    // La contraseña del supervisor sale del formulario antes de enviar, sea cual sea el resultado;
    // el almacén de pendientes tampoco la conserva (storableRequest).
    const authorization = auth.take();
    const body: CancelAccountInput = {
      revision,
      reason_id: reason.id,
      description: description.trim(),
    };
    if (authorization) {
      body.authorization = authorization;
    }
    setReasonError('');
    setSaving(true);
    try {
      await execute('restaurant_cancel_account', { id: account.id, body: { ...body } });
      notifySuccess(cancelledMessage(account), {
        description: 'No cuenta como venta; aparece en «Cuentas canceladas».',
      });
      onClose();
    } catch (problem) {
      if (auth.handleError(problem)) {
        return;
      }
      // Un resultado incierto queda en «Reintentar sin duplicar» (abajo) y en la página.
      if (uncertainError(problem)) {
        return;
      }
      if (problem instanceof APIError && reasonCodes.includes(problem.code)) {
        setReasonError(problem.message);
        void reasons.reload();
        return;
      }
      notifyError(problem, 'No se pudo cancelar la cuenta.');
    } finally {
      setSaving(false);
    }
  }

  // Repite la misma cancelación con su operation_id. Si ya se había registrado, Go devuelve el
  // resultado sin cancelar dos veces; si no llegó a registrarse, la autorización sin contraseña se
  // rechaza y se pide de nuevo.
  async function recover() {
    const authorized = typeof recovery.pending?.body.authorization === 'object';
    const problem = await recovery.retry();
    if (!problem) {
      notifySuccess(cancelledMessage(account), {
        description: 'Resultado recuperado: la cancelación quedó registrada una sola vez.',
      });
      onClose();
      return;
    }
    if (uncertainError(problem)) {
      return;
    }
    if (authorized && isAuthorizationProblem(problem)) {
      auth.requireRepeat();
      notifyError(repeatAuthorizationMessage);
      return;
    }
    notifyError(problem, 'No se pudo recuperar el resultado de la cancelación.');
  }

  return (
    <section className="operation-inline-panel cancel-account" aria-label="Cancelar cuenta">
      {dialog}
      <div className="section-heading">
        <h2>Cancelar cuenta · #{account.folio}</h2>
        <button type="button" className="secondary-button" onClick={onClose}>
          <IconLabel icon="back">Volver sin cancelar</IconLabel>
        </button>
      </div>
      {recovering && (
        <div className="checkout-uncertain" role="alert">
          <strong>No se pudo confirmar el resultado de la cancelación.</strong>
          <p>
            No la repitas desde el principio: «Reintentar sin duplicar» envía la misma operación y,
            si ya se había registrado, la recupera sin cancelar dos veces.
          </p>
          {typeof recovery.pending?.body.authorization === 'object' && (
            <p>
              La contraseña del supervisor no se conserva: si la cancelación no llegó a registrarse,
              se pedirá de nuevo su autorización.
            </p>
          )}
          <div>
            <button
              type="button"
              className="primary-button"
              disabled={recovery.busy}
              onClick={() => void recover()}
            >
              <IconLabel icon="refresh">
                {recovery.busy ? 'Reintentando…' : 'Reintentar sin duplicar'}
              </IconLabel>
            </button>
          </div>
        </div>
      )}
      {account.state !== 'open' ? (
        <Notice>La cuenta ya no está abierta; no se puede cancelar.</Notice>
      ) : blocker ? (
        <CancelBlocked
          account={account}
          blocker={blocker}
          can={can}
          busy={busy}
          onOpenPanel={onOpenPanel}
        />
      ) : (
        <form className="form-stack" onSubmit={(event) => void submit(event)}>
          {stale && (
            <div className="cancel-account-stale" role="alert">
              <p>
                La cuenta cambió en otra terminal. Revisa sus productos e importes antes de seguir.
              </p>
              <div>
                <button
                  type="button"
                  className="secondary-button"
                  onClick={() => setRevision(account.revision)}
                >
                  Revisé los cambios de esta cuenta
                </button>
              </div>
            </div>
          )}
          <dl className="cancel-account-summary" aria-label="Lo que se cancela">
            <div>
              <dt>Enviado a cocina</dt>
              <dd>{countLabel(units, 'unidad', 'unidades')}</dd>
            </div>
            <div>
              <dt>Importe que se cancela</dt>
              <dd>{formatMoney(account.totals.total)}</dd>
            </div>
          </dl>
          {draftItems > 0 && (
            <p className="muted text-sm">
              Los productos nuevos sin enviar ({draftItems}) se descartan; no llegaron a cocina.
            </p>
          )}
          <fieldset className="form-stack cancel-account-fields" disabled={locked}>
            <div className="field">
              <span>Motivo de la cancelación</span>
              {reasons.error ? (
                <Notice>{reasons.error}</Notice>
              ) : !active ? (
                <p className="muted text-sm">Consultando los motivos de cancelación…</p>
              ) : active.length === 0 ? (
                <Notice>
                  No hay motivos de cancelación activos. Actívalos en Administración → Cobros,
                  sección «Motivos de cancelación».
                </Notice>
              ) : (
                <div className="capture-choices" role="group" aria-label="Motivo de la cancelación">
                  {active.map((entry) => (
                    <button
                      type="button"
                      key={entry.id}
                      className="capture-choice"
                      aria-pressed={reasonID === entry.id}
                      onClick={() => {
                        setReasonID(entry.id);
                        setReasonError('');
                      }}
                    >
                      <span>{entry.name}</span>
                      {entry.requires_description && (
                        <small className="muted">Exige descripción</small>
                      )}
                      {entry.prepared_is_waste && (
                        <small className="muted">Lo preparado cuenta como merma</small>
                      )}
                    </button>
                  ))}
                </div>
              )}
            </div>
            {reason && (
              <Field
                label={
                  reason.requires_description
                    ? `Descripción del motivo «${reason.name}»`
                    : 'Descripción (opcional)'
                }
                hint={
                  descriptionTooLong
                    ? 'La descripción admite hasta 500 caracteres.'
                    : reason.requires_description
                      ? 'Obligatoria, de 5 a 500 caracteres.'
                      : 'Hasta 500 caracteres.'
                }
              >
                <input
                  type="text"
                  maxLength={500}
                  autoComplete="off"
                  required={reason.requires_description}
                  aria-invalid={
                    !!reasonError || (reason.requires_description && textLength(description) < 5)
                  }
                  value={description}
                  onChange={(event) => {
                    setDescription(event.target.value);
                    setReasonError('');
                  }}
                />
              </Field>
            )}
            {reasonError && (
              <p className="checkout-inline-note" role="alert">
                {reasonError}
              </p>
            )}
            <div className="cancel-account-kitchen">
              <strong>Lo enviado a cocina</strong>
              {kitchenEffect(units, reason, inventory).map((line) => (
                <p key={line}>{line}</p>
              ))}
            </div>
            <p className="muted text-sm">{outcomeText(account)}</p>
            <SupervisorAuthorization
              auth={auth}
              disabled={locked}
              description="Cancelar la cuenta requiere la autorización de un supervisor con el permiso «Cancelar cuentas». Es puntual: no inicia sesión ni cambia la sesión del supervisor en su terminal."
            />
          </fieldset>
          <div className="form-actions">
            <button type="submit" className="danger-solid-button" disabled={!ready}>
              <IconLabel icon="ban">{saving ? 'Cancelando…' : 'Cancelar cuenta'}</IconLabel>
            </button>
          </div>
        </form>
      )}
    </section>
  );
}

// Registro de una cuenta cancelada (`Bill.cancellation`), en el detalle de la cuenta. Sin registro
// (anulaciones anteriores a la etapa 9D, liberaciones y descartes) se indica que salió vacía.
export function AccountCancellationRecord({
  cancellation,
  timezone,
}: {
  cancellation: AccountCancellation | null | undefined;
  timezone: string;
}) {
  if (!cancellation) {
    return (
      <Notice>
        Cuenta cancelada sin productos activos o descartada vacía. Su historial permanece
        disponible.
      </Notice>
    );
  }
  return (
    <section className="cancel-account-record" aria-label="Cuenta cancelada">
      <strong>Cuenta cancelada</strong>
      <dl>
        <div>
          <dt>Motivo</dt>
          <dd>{cancellation.reason_name}</dd>
        </div>
        {cancellation.description && (
          <div>
            <dt>Descripción</dt>
            <dd>{cancellation.description}</dd>
          </div>
        )}
        <div>
          <dt>Canceló</dt>
          <dd>{cancellation.cancelled_by_name}</dd>
        </div>
        <div>
          <dt>Autorizó</dt>
          <dd>{cancellation.authorized_by_name ?? 'Con su propio permiso'}</dd>
        </div>
        <div>
          <dt>Unidades canceladas</dt>
          <dd>{cancellation.cancelled_units}</dd>
        </div>
        <div>
          <dt>Importe cancelado</dt>
          <dd>{formatMoney(cancellation.cancelled_amount)}</dd>
        </div>
        <div>
          <dt>Fecha</dt>
          <dd>{serverDateTime(cancellation.created_at, timezone)}</dd>
        </div>
      </dl>
      <p className="muted text-xs">
        {cancellation.prepared_is_waste ? 'Lo preparado cuenta como merma. ' : ''}No cuenta como
        venta. Su historial permanece disponible.
      </p>
    </section>
  );
}
