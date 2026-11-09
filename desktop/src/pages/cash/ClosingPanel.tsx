import { useState } from 'react';
import { ConfirmDialog } from '../../components/ConfirmDialog';
import { APIError } from '../../services/api';
import type { CashCatalog } from '../../services/cash';
import { formatDollars, formatMoney } from '../../services/payments';
import type { Shift } from '../../services/payments';
import { ClosingCount } from './ClosingCount';
import { capturableDenominations } from './countCapture';
import { useClosingDraft } from './useClosingDraft';
import type { CashMutation } from './useCashMutation';

// Iniciar el arqueo de cierre pausa la caja: se confirma con un diálogo que lo explica.
function StartClosing({
  shift,
  mutation,
  onClose,
}: {
  shift: Shift;
  mutation: CashMutation;
  onClose: () => void;
}) {
  const [asking, setAsking] = useState(false);

  async function start() {
    setAsking(false);
    if (mutation.isPending()) {
      return;
    }
    try {
      await mutation.execute('payments_shift_start_closing', { id: shift.id, body: {} });
    } catch {
      // El mensaje de Go queda visible junto al botón.
    }
  }

  return (
    <section className="cash-panel" aria-label="Arqueo de cierre">
      <header className="cash-panel-header">
        <h3>Arqueo y cierre de {shift.register_name}</h3>
        <button type="button" className="secondary-button" onClick={onClose}>
          Volver a la caja
        </button>
      </header>
      <p className="muted text-sm">
        El arqueo es ciego: cuentas el efectivo físico por denominación sin ver el efectivo
        esperado. Mientras dura, la caja queda pausada («Cierre en curso») y no acepta cobros,
        devoluciones, ingresos, retiros, gastos ni correcciones. Las cuentas abiertas siguen
        abiertas y pueden cobrarse en otra caja con turno abierto.
      </p>
      {mutation.error && (
        <p className="checkout-inline-note" role="alert">
          {mutation.error}
        </p>
      )}
      <div className="button-row">
        <button
          type="button"
          className="primary-button"
          disabled={mutation.locked}
          onClick={() => setAsking(true)}
        >
          {mutation.busy ? 'Iniciando arqueo…' : 'Iniciar arqueo de cierre'}
        </button>
      </div>
      {asking && (
        <ConfirmDialog
          title="Iniciar arqueo de cierre"
          message={`La caja ${shift.register_name} quedará pausada: no aceptará cobros, devoluciones, ingresos, retiros, gastos ni correcciones hasta que confirmes el cierre o canceles el arqueo.`}
          confirmLabel="Iniciar arqueo y pausar la caja"
          cancelLabel="Seguir operando"
          onConfirm={() => void start()}
          onCancel={() => setAsking(false)}
        />
      )}
    </section>
  );
}

// Totales declarados para el mensaje de confirmación, tal como los calculó Go.
function declaredText(mxn: string | undefined, usd: string | undefined, dollars: boolean) {
  const pesos = formatMoney(mxn);
  if (!dollars) {
    return pesos;
  }
  return `${pesos} y ${formatDollars(usd)}`;
}

// Arqueo en curso: conteo ciego, borrador en el servidor, confirmación y cancelación.
function ClosingInProgress({
  shift,
  catalog,
  can,
  userID,
  usdCash,
  mutation,
  timezone,
  onClosed,
  onClose,
}: {
  shift: Shift;
  catalog: CashCatalog;
  can: (permission: string) => boolean;
  userID: string;
  usdCash: boolean;
  mutation: CashMutation;
  timezone: string;
  onClosed: (shiftID: string) => void;
  onClose: () => void;
}) {
  // Un turno que manejó dólares los cuenta aunque la función se haya desactivado después.
  const dollars = usdCash || !!shift.usd_rate || !!shift.opening_float_usd;
  const denominations = capturableDenominations(catalog.denominations, dollars);
  const draft = useClosingDraft({ shift, denominations, mutation });
  const [dialog, setDialog] = useState<'confirm' | 'cancel' | null>(null);
  const [late, setLate] = useState(false);
  const ownClosing = shift.closing_started_by === userID;
  const canCancel = can('cash.counts.perform') && (ownClosing || can('cash.closing.cancel_any'));

  async function confirm() {
    setDialog(null);
    let revision: number | null = draft.baseRevision;
    if (draft.conflict) {
      revision = await draft.saveAfterConflict();
    } else if (draft.dirty) {
      revision = await draft.save();
    }
    if (revision === null || mutation.isPending()) {
      return;
    }
    try {
      await mutation.execute('payments_shift_confirm_closing', {
        id: shift.id,
        body: { count_revision: revision },
      });
      onClosed(shift.id);
    } catch (problem) {
      if (problem instanceof APIError && problem.code === 'CLOSING_MOVEMENTS_CHANGED') {
        setLate(true);
      }
      if (problem instanceof APIError && problem.code === 'COUNT_CHANGED') {
        void draft.closing.reload();
      }
    }
  }

  async function cancel() {
    setDialog(null);
    if (mutation.isPending()) {
      return;
    }
    try {
      await mutation.execute('payments_shift_cancel_closing', {
        id: shift.id,
        body: late ? { reason: 'Movimientos posteriores al inicio del arqueo' } : {},
      });
      setLate(false);
    } catch {
      // El mensaje de Go queda visible en el arqueo.
    }
  }

  const plan = draft.preview.plan;
  return (
    <>
      <ClosingCount
        shift={shift}
        draft={draft}
        denominations={denominations}
        dollars={dollars}
        mutation={mutation}
        timezone={timezone}
        late={late}
        canConfirm={can('cash.shifts.close')}
        canCancel={canCancel}
        onConfirm={() => setDialog('confirm')}
        onCancel={() => setDialog('cancel')}
        onClose={onClose}
      />
      {dialog === 'confirm' && (
        <ConfirmDialog
          title="Confirmar el cierre del turno"
          message={`Se cerrará el turno de ${shift.register_name} con ${declaredText(plan?.counted_mxn, plan?.counted_usd, dollars)} declarados. Después del cierre no se pueden agregar movimientos al turno.`}
          confirmLabel="Cerrar el turno"
          cancelLabel="Seguir contando"
          onConfirm={() => void confirm()}
          onCancel={() => setDialog(null)}
        />
      )}
      {dialog === 'cancel' && (
        <ConfirmDialog
          title="Cancelar el arqueo"
          message={`El turno de ${shift.register_name} vuelve a «Abierto» y la caja acepta de nuevo operaciones. Lo contado no se conserva: el siguiente arqueo empieza desde cero.`}
          confirmLabel="Cancelar el arqueo"
          cancelLabel="Seguir contando"
          onConfirm={() => void cancel()}
          onCancel={() => setDialog(null)}
        />
      )}
    </>
  );
}

// Arqueo y cierre ciego del turno (§11–§14). Durante el arqueo la pantalla nunca muestra ni pide
// nada del efectivo esperado, ni un indicador de cercanía: solo lo que se está declarando.
export function ClosingPanel({
  shift,
  catalog,
  can,
  userID,
  usdCash,
  mutation,
  timezone,
  onClosed,
  onClose,
}: {
  shift: Shift;
  catalog: CashCatalog;
  can: (permission: string) => boolean;
  userID: string;
  usdCash: boolean;
  mutation: CashMutation;
  timezone: string;
  onClosed: (shiftID: string) => void;
  onClose: () => void;
}) {
  if (shift.state === 'open') {
    return <StartClosing shift={shift} mutation={mutation} onClose={onClose} />;
  }
  return (
    <ClosingInProgress
      shift={shift}
      catalog={catalog}
      can={can}
      userID={userID}
      usdCash={usdCash}
      mutation={mutation}
      timezone={timezone}
      onClosed={onClosed}
      onClose={onClose}
    />
  );
}
