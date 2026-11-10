import { useId } from 'react';
import { Notice } from '../../../components/Forms';
import { IconLabel } from '../../../components/Icon';
import { AmountField, NumericKeypad } from '../../../components/NumericKeypad';
import { SegmentedControl } from '../../../components/SegmentedControl';
import { permissionName } from '../../../services/payments';
import type { CurrentShift } from '../../../services/payments';
import { CountTotals, DenominationCount } from '../../cash/DenominationCount';
import { RateConfirmation } from './ShiftRateCapture';
import { useShiftOpening } from './useShiftOpening';
import type { OpeningMode, ShiftOpeningState } from './useShiftOpening';

// Fondo por total: pesos y, con la función de dólares activa, fondo en dólares. Un fondo de $0
// no exige desglose.
function TotalFields({ state }: { state: ShiftOpeningState }) {
  return (
    <>
      <AmountField
        label="Fondo inicial"
        value={state.opening}
        onChange={state.setOpening}
        active={state.target === 'opening'}
        onActivate={() => state.setTarget('opening')}
        emphasis
        placeholder="0.00"
        hint="Obligatorio. Captura 0 si la caja inicia sin efectivo."
      />
      {state.dollars && (
        <AmountField
          label="Fondo inicial en dólares"
          value={state.openingUSD}
          onChange={state.setOpeningUSD}
          active={state.target === 'opening_usd'}
          onActivate={() => state.setTarget('opening_usd')}
          currency="US$"
          placeholder="0.00"
          hint="Opcional. Déjalo vacío si la caja no inicia con dólares."
        />
      )}
    </>
  );
}

// Fondo por denominaciones: el cajero captura cantidades y Go calcula importes y totales.
function DenominationFields({ state }: { state: ShiftOpeningState }) {
  const { catalog, preview } = state;
  if (catalog.error) {
    return <Notice>{catalog.error}</Notice>;
  }
  if (!catalog.data) {
    return <p className="muted text-sm">Consultando denominaciones…</p>;
  }
  return (
    <>
      <DenominationCount
        denominations={state.denominations}
        quantities={state.quantities}
        activeID={state.activeCountID}
        preview={preview.plan}
        disabled={state.mutation.busy}
        onActivate={(id) => state.setTarget(`count:${id}`)}
        onChange={state.changeQuantity}
      />
      <CountTotals
        label="Fondo contado"
        preview={preview.plan}
        validating={preview.validating}
        error={preview.error?.message ?? ''}
        dollars={state.dollars}
      />
    </>
  );
}

// Formulario de apertura con su teclado numérico fijo.
function OpeningForm({ state, active }: { state: ShiftOpeningState; active: boolean }) {
  const notesID = useId();
  const keypad = state.keypad();
  return (
    <>
      <SegmentedControl<OpeningMode>
        label="Forma de registrar el fondo"
        value={state.mode}
        onChange={state.changeMode}
        disabled={state.mutation.busy}
        options={[
          { value: 'total', label: 'Por total' },
          { value: 'denominations', label: 'Por denominaciones' },
        ]}
      />
      {state.mode === 'total' ? (
        <TotalFields state={state} />
      ) : (
        <DenominationFields state={state} />
      )}
      {state.canSetRate && (
        <AmountField
          label="Tipo de cambio del dólar"
          value={state.rate}
          onChange={state.setRate}
          active={state.target === 'rate'}
          onActivate={() => state.setTarget('rate')}
          placeholder="0.00"
          hint="Pesos por dólar. Queda fijo durante el turno; sin él, el turno no acepta dólares."
        />
      )}
      {state.dollars && !state.canSetRate && (
        <p className="muted text-sm">
          El tipo de cambio lo captura alguien con el permiso «
          {permissionName('payments.usd_rate.manage')}»; mientras falte, el turno no acepta dólares.
        </p>
      )}
      <NumericKeypad
        value={keypad.value}
        onChange={keypad.onChange}
        label={keypad.label}
        integer={keypad.integer ?? false}
        captureKeyboard={active}
        disabled={state.mutation.busy || state.mutation.uncertain}
      />
      <label className="field" htmlFor={notesID}>
        <span>Observaciones de la apertura (opcional)</span>
        <textarea
          id={notesID}
          rows={2}
          maxLength={2000}
          value={state.notes}
          onChange={(event) => state.setNotes(event.target.value)}
        />
      </label>
      <button
        type="button"
        className="primary-button checkout-register"
        disabled={!state.ready || state.mutation.busy || !!state.mutation.pending}
        onClick={() => void state.open()}
      >
        <IconLabel icon="register">{state.openLabel()}</IconLabel>
      </button>
    </>
  );
}

// Turno no utilizable: muestra el motivo de Go y, si falta el turno y el usuario puede abrirlo,
// ofrece la apertura en la misma pantalla. El fondo inicial se captura siempre de forma explícita
// (se permite 0), por total o por denominaciones; nunca se abre un turno automáticamente. Con el
// pago en dólares activo se capturan además el fondo en dólares y el tipo de cambio fijo del
// turno. Se usa desde el cobro y desde la pantalla Caja.
export function ShiftOpening({
  current,
  can,
  active,
  onChanged,
  onOpened,
}: {
  current: CurrentShift;
  can: (permission: string) => boolean;
  active: boolean;
  onChanged: () => void;
  onOpened?: () => void;
}) {
  const state = useShiftOpening({ current, can, onChanged, onOpened });
  const { mutation, register } = state;
  return (
    <section className="checkout-shift" aria-label="Turno de caja">
      <h3>{register ? `Turno de ${register.name}` : 'Caja no asignada'}</h3>
      <p className="checkout-inline-note">{current.reason}</p>
      {mutation.uncertain && (
        <div className="operation-recovery">
          <p>
            No se pudo confirmar la apertura del turno. Reintenta con el mismo identificador o
            consulta el turno actual.
          </p>
          <button
            type="button"
            className="primary-button"
            disabled={mutation.busy}
            onClick={() => void state.retry()}
          >
            <IconLabel icon="refresh">Reintentar con el mismo identificador</IconLabel>
          </button>
          <button type="button" className="secondary-button" onClick={onChanged}>
            Consultar turno
          </button>
        </div>
      )}
      {register && !current.shift && !can('cash.shifts.open') && (
        <p className="muted text-sm">
          Para abrir el turno se necesita el permiso «{permissionName('cash.shifts.open')}».
        </p>
      )}
      {state.openable && <OpeningForm state={state} active={active} />}
      {state.confirmation && (
        <RateConfirmation
          message={state.confirmation}
          onConfirm={() => {
            state.setConfirmation('');
            void state.open(true);
          }}
          onCancel={() => {
            state.setConfirmation('');
            state.setTarget('rate');
          }}
        />
      )}
    </section>
  );
}
