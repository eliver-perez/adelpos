import { useId, useState } from 'react';
import type { ReactNode } from 'react';
import { Notice } from '../../components/Forms';
import { IconLabel } from '../../components/Icon';
import { AmountField, NumericKeypad } from '../../components/NumericKeypad';
import type { Denomination } from '../../services/cash';
import type { Shift } from '../../services/payments';
import { serverDateTime } from '../../services/restaurant';
import { CountTotals, DenominationCount } from './DenominationCount';
import { businessDate } from './movementRows';
import type { ClosingDraft } from './useClosingDraft';
import type { CashMutation } from './useCashMutation';

// Campo que recibe el teclado: una denominación («count:<id>») o un importe adicional.
type CountTarget = `count:${string}` | 'extra_mxn' | 'extra_usd';

// Importe adicional identificado (efectivo sin desglosar) y observaciones del cajero.
function ExtraFields({
  draft,
  dollars,
  target,
  onTarget,
}: {
  draft: ClosingDraft;
  dollars: boolean;
  target: CountTarget;
  onTarget: (target: CountTarget) => void;
}) {
  const identifier = useId();
  const { capture, update } = draft;
  return (
    <div className="form-stack">
      <div className="form-grid">
        <AmountField
          label="Importe adicional en pesos"
          value={capture.extraMXN}
          onChange={(extraMXN) => update({ extraMXN })}
          active={target === 'extra_mxn'}
          onActivate={() => onTarget('extra_mxn')}
          placeholder="0.00"
          hint="Efectivo sin desglosar, por ejemplo monedas sueltas."
        />
        {dollars && (
          <AmountField
            label="Importe adicional en dólares"
            value={capture.extraUSD}
            onChange={(extraUSD) => update({ extraUSD })}
            active={target === 'extra_usd'}
            onActivate={() => onTarget('extra_usd')}
            currency="US$"
            placeholder="0.00"
          />
        )}
      </div>
      <label className="field" htmlFor={`${identifier}-extra-note`}>
        <span>Identificación del importe adicional</span>
        <input
          id={`${identifier}-extra-note`}
          maxLength={200}
          placeholder="Por ejemplo: monedas sueltas sin desglosar"
          value={capture.extraNote}
          onChange={(event) => update({ extraNote: event.target.value })}
        />
      </label>
      <label className="field" htmlFor={`${identifier}-notes`}>
        <span>Observaciones del arqueo (opcional)</span>
        <textarea
          id={`${identifier}-notes`}
          rows={2}
          maxLength={2000}
          value={capture.notes}
          onChange={(event) => update({ notes: event.target.value })}
        />
      </label>
    </div>
  );
}

// Estado del borrador en el servidor y avisos de conflicto o de movimientos posteriores.
function DraftStatus({
  draft,
  mutation,
  late,
  canCancel,
  onCancel,
}: {
  draft: ClosingDraft;
  mutation: CashMutation;
  late: boolean;
  canCancel: boolean;
  onCancel: () => void;
}) {
  return (
    <>
      <p className="muted text-xs" role="status">
        {draft.dirty
          ? 'Cambios sin guardar: el borrador se guarda automáticamente.'
          : 'Borrador guardado.'}
      </p>
      {draft.conflict && (
        <div className="cash-warning" role="alert">
          <strong>El conteo cambió en otra terminal.</strong>
          <p>
            Se cargó el conteo vigente y lo que capturaste en esta terminal se conserva. Revisa las
            cantidades y guarda tu conteo para sustituir el de la otra terminal.
          </p>
          <button
            type="button"
            className="secondary-button"
            disabled={mutation.locked}
            onClick={() => void draft.saveAfterConflict()}
          >
            Guardar mi conteo
          </button>
        </div>
      )}
      {late && (
        <div className="cash-warning" role="alert">
          <strong>Se registraron movimientos después de iniciar el arqueo.</strong>
          <p>
            Un cobro u operación quedó confirmado en el turno después de pausar la caja, por lo que
            el conteo ya no corresponde. El cierre no se realizó: cancela el arqueo, revisa la caja
            y vuelve a contar.
          </p>
          {canCancel && (
            <button type="button" className="danger-button" onClick={onCancel}>
              Cancelar el arqueo y volver a contar
            </button>
          )}
        </div>
      )}
    </>
  );
}

// Captura táctil del arqueo ciego: denominaciones por moneda con teclado numérico, importe
// adicional identificado y observaciones. Los totales los calcula Go con la vista previa; la
// pantalla no contiene ni solicita ninguna cifra del efectivo esperado.
export function ClosingCount({
  shift,
  draft,
  denominations,
  dollars,
  mutation,
  timezone,
  late,
  canConfirm,
  canCancel,
  onConfirm,
  onCancel,
  onClose,
  printControl,
}: {
  shift: Shift;
  draft: ClosingDraft;
  denominations: Denomination[];
  dollars: boolean;
  mutation: CashMutation;
  timezone: string;
  late: boolean;
  canConfirm: boolean;
  canCancel: boolean;
  onConfirm: () => void;
  onCancel: () => void;
  onClose: () => void;
  // Interruptor opcional para imprimir el corte definitivo al confirmar (fase C).
  printControl?: ReactNode;
}) {
  const [target, setTarget] = useState<CountTarget>(`count:${denominations[0]?.id ?? ''}`);
  const countID = target.startsWith('count:') ? target.slice('count:'.length) : '';
  const activeID = target.startsWith('count:') ? countID || denominations[0]?.id || '' : '';
  const { capture, preview } = draft;
  const ready = !!preview.plan && !preview.validating && !mutation.locked && !late;

  function keypad() {
    if (target === 'extra_mxn') {
      return {
        value: capture.extraMXN,
        onChange: (extraMXN: string) => draft.update({ extraMXN }),
        integer: false,
      };
    }
    if (target === 'extra_usd') {
      return {
        value: capture.extraUSD,
        onChange: (extraUSD: string) => draft.update({ extraUSD }),
        integer: false,
      };
    }
    return {
      value: capture.quantities[activeID] ?? '',
      onChange: (next: string) => draft.changeQuantity(activeID, next),
      integer: true,
    };
  }

  if (draft.closing.error) {
    return <Notice>{draft.closing.error}</Notice>;
  }
  if (!draft.closing.data) {
    return <p className="muted">Consultando el arqueo…</p>;
  }
  const keys = keypad();
  return (
    <section className="cash-panel" aria-label="Arqueo de cierre en curso">
      <header className="cash-panel-header">
        <h3>Arqueo de cierre · {shift.register_name}</h3>
        <button type="button" className="secondary-button" onClick={onClose}>
          <IconLabel icon="back">Volver a la caja</IconLabel>
        </button>
      </header>
      <p className="muted text-sm">
        Turno del {businessDate(shift.business_date)} · arqueo iniciado por{' '}
        {shift.closing_started_by_name}
        {shift.closing_started_at ? ` (${serverDateTime(shift.closing_started_at, timezone)})` : ''}
        . Captura ciega: cuenta el efectivo físico; el efectivo esperado y las diferencias se
        consultan después del cierre con el permiso correspondiente.
      </p>
      <DraftStatus
        draft={draft}
        mutation={mutation}
        late={late}
        canCancel={canCancel}
        onCancel={onCancel}
      />
      <div className="cash-capture">
        <div className="cash-capture-fields">
          <DenominationCount
            denominations={denominations}
            quantities={capture.quantities}
            activeID={activeID}
            preview={preview.plan}
            disabled={mutation.locked}
            onActivate={(id) => setTarget(`count:${id}`)}
            onChange={draft.changeQuantity}
          />
          <ExtraFields draft={draft} dollars={dollars} target={target} onTarget={setTarget} />
        </div>
        <div className="cash-capture-keypad">
          <CountTotals
            label="Total declarado"
            preview={preview.plan}
            validating={preview.validating}
            error={preview.error?.message ?? ''}
            dollars={dollars}
          />
          <NumericKeypad
            value={keys.value}
            onChange={keys.onChange}
            integer={keys.integer}
            label="Teclado numérico del arqueo"
            disabled={mutation.locked}
            captureKeyboard
          />
          <div className="button-row">
            <button
              type="button"
              className="secondary-button"
              disabled={mutation.locked || !draft.dirty}
              onClick={() => void draft.save()}
            >
              Guardar borrador
            </button>
            {canCancel && (
              <button
                type="button"
                className="danger-button"
                disabled={mutation.locked}
                onClick={onCancel}
              >
                Cancelar arqueo
              </button>
            )}
          </div>
          {canConfirm && printControl}
          {canConfirm && (
            <button
              type="button"
              className="primary-button checkout-register"
              disabled={!ready}
              onClick={onConfirm}
            >
              <IconLabel icon="count">Confirmar cierre del turno</IconLabel>
            </button>
          )}
        </div>
      </div>
    </section>
  );
}
