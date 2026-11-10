import { useId, useState } from 'react';
import { IconLabel } from '../../components/Icon';
import { AmountField, NumericKeypad } from '../../components/NumericKeypad';
import { SegmentedControl } from '../../components/SegmentedControl';
import { useSupervisorAuthorization } from '../../hooks/useSupervisorAuthorization';
import { APIError } from '../../services/api';
import { toWireAmount } from '../../services/amountInput';
import type { CashCatalog, CashCurrency, RecordedCashOperation } from '../../services/cash';
import { dismissNotice, notifyError } from '../../services/notify';
import { permissionName, validReason } from '../../services/payments';
import type { Shift } from '../../services/payments';
import { currencyAmount } from './DenominationCount';
import { cashEntries, entryBody, initialEntry, outflowPermissions } from './operationForm';
import type { CashEntryCapture, CashEntryKind } from './operationForm';
import { OutflowAuthorization } from './OutflowAuthorization';
import { PrintToggle } from './PrintToggle';
import type { CashMutation } from './useCashMutation';

// Opciones del tipo de ingreso, del motivo del retiro o de la categoría del gasto.
function entryChoices(kind: CashEntryKind, catalog: CashCatalog) {
  if (kind === 'income') {
    return catalog.income_types.map((option) => ({ id: option.code, label: option.label }));
  }
  if (kind === 'withdrawal') {
    return catalog.withdrawal_types.map((option) => ({ id: option.code, label: option.label }));
  }
  return catalog.expense_categories
    .filter((category) => category.active)
    .map((category) => ({ id: category.id, label: category.name }));
}

// Aviso del rechazo por efectivo insuficiente: se retira en cuanto cambia la captura.
const insufficientNoticeID = 'cash-insufficient';

const choiceLabels: Record<CashEntryKind, string> = {
  income: 'Tipo de ingreso',
  withdrawal: 'Motivo del retiro',
  expense: 'Categoría del gasto',
};

// Botones táctiles para elegir el tipo, el motivo o la categoría.
function ChoiceGrid({
  label,
  choices,
  value,
  disabled,
  onChange,
}: {
  label: string;
  choices: { id: string; label: string }[];
  value: string;
  disabled: boolean;
  onChange: (value: string) => void;
}) {
  return (
    <div className="field">
      <span>{label}</span>
      <div className="cash-choice-grid" role="group" aria-label={label}>
        {choices.map((choice) => (
          <button
            type="button"
            key={choice.id}
            className={value === choice.id ? 'cash-choice-selected' : ''}
            aria-pressed={value === choice.id}
            disabled={disabled}
            onClick={() => onChange(choice.id)}
          >
            {choice.label}
          </button>
        ))}
      </div>
    </div>
  );
}

// Aviso de CASH_INSUFFICIENT: el mensaje de Go tal cual (sin cifras) y la solicitud de una salida
// excepcional con motivo, que autoriza alguien que pueda consultar el efectivo esperado.
function InsufficientCash({
  capture,
  update,
  locked,
}: {
  capture: CashEntryCapture;
  update: (patch: Partial<CashEntryCapture>) => void;
  locked: boolean;
}) {
  const reasonID = useId();
  const hintID = `${reasonID}-hint`;
  if (!capture.exceptional) {
    return (
      <button
        type="button"
        className="secondary-button"
        disabled={locked}
        onClick={() => update({ exceptional: true })}
      >
        Solicitar salida excepcional
      </button>
    );
  }
  return (
    <div className="cash-exceptional">
      <p className="muted text-sm">
        Una salida excepcional se registra aunque exceda el efectivo disponible. La autoriza alguien
        con los permisos «{permissionName('cash.outflows.authorize')}» y «
        {permissionName('cash.expected.read')}».
      </p>
      <div className="field">
        <label htmlFor={reasonID}>Motivo de la salida excepcional</label>
        <textarea
          id={reasonID}
          rows={2}
          maxLength={500}
          aria-describedby={hintID}
          value={capture.reason}
          onChange={(event) => update({ reason: event.target.value })}
        />
        <small id={hintID} className="muted">
          Entre 5 y 500 caracteres.
        </small>
      </div>
      <button
        type="button"
        className="secondary-button"
        disabled={locked}
        onClick={() => update({ exceptional: false, reason: '' })}
      >
        Volver a una salida ordinaria
      </button>
    </div>
  );
}

// Campos de texto de la operación: concepto, referencia (gastos) y observaciones.
function EntryDetails({
  kind,
  capture,
  update,
  placeholder,
}: {
  kind: CashEntryKind;
  capture: CashEntryCapture;
  update: (patch: Partial<CashEntryCapture>) => void;
  placeholder: string;
}) {
  const identifier = useId();
  return (
    <div className="form-grid">
      <label className="field" htmlFor={`${identifier}-concept`}>
        <span>Concepto (opcional)</span>
        <input
          id={`${identifier}-concept`}
          maxLength={120}
          placeholder={placeholder}
          value={capture.concept}
          onChange={(event) => update({ concept: event.target.value })}
        />
      </label>
      {kind === 'expense' && (
        <label className="field" htmlFor={`${identifier}-reference`}>
          <span>Referencia o comprobante (opcional)</span>
          <input
            id={`${identifier}-reference`}
            maxLength={120}
            value={capture.reference}
            onChange={(event) => update({ reference: event.target.value })}
          />
        </label>
      )}
      <label className="field" htmlFor={`${identifier}-notes`}>
        <span>Observaciones (opcional)</span>
        <input
          id={`${identifier}-notes`}
          maxLength={2000}
          value={capture.notes}
          onChange={(event) => update({ notes: event.target.value })}
        />
      </label>
    </div>
  );
}

// Ingreso, retiro o gasto del turno. Go valida el importe, la autorización y el efectivo
// disponible; la interfaz envía la cadena capturada. Desde la etapa 7.1 el resultado confirmado
// no se queda en el panel: la escritura (useCashRecording) lo anuncia con un aviso y quien abrió
// el formulario lo cierra (vista de Operación de Caja u hoja del atajo «Caja»).
export function CashOperationForm({
  kind,
  shift,
  catalog,
  can,
  dollars,
  active,
  mutation,
  onClose,
  backLabel = 'Volver a la caja',
}: {
  kind: CashEntryKind;
  shift: Shift;
  catalog: CashCatalog;
  can: (permission: string) => boolean;
  // El turno admite dólares: función activa y tipo de cambio capturado.
  dollars: boolean;
  active: boolean;
  mutation: CashMutation;
  onClose: () => void;
  // Texto del botón que cierra el formulario sin registrar.
  backLabel?: string;
}) {
  const definition = cashEntries[kind];
  const [capture, setCapture] = useState<CashEntryCapture>(initialEntry);
  // Imprimir el comprobante es opcional y se elige por operación (cash.reports.print).
  const [print, setPrint] = useState(false);
  const [insufficient, setInsufficient] = useState(false);
  const selfAuthorized =
    definition.outflow && outflowPermissions(capture.exceptional).every((key) => can(key));
  const auth = useSupervisorAuthorization(definition.outflow && !selfAuthorized);
  const choices = entryChoices(kind, catalog);
  const chosen = choices.find((choice) => choice.id === capture.choice);
  const wire = toWireAmount(capture.amount);
  const reasonReady = !capture.exceptional || validReason(capture.reason);
  const ready =
    !!chosen &&
    wire !== '' &&
    reasonReady &&
    auth.ready &&
    !mutation.locked &&
    can(definition.permission);

  function update(patch: Partial<CashEntryCapture>) {
    if (mutation.locked) {
      return;
    }
    setCapture((current) => ({ ...current, ...patch }));
    // Cambiar la captura invalida el rechazo anterior; pedir o redactar la salida excepcional lo
    // conserva a la vista.
    const exceptionalOnly = Object.keys(patch).every(
      (key) => key === 'exceptional' || key === 'reason',
    );
    if (!exceptionalOnly) {
      setInsufficient(false);
      mutation.clearError();
      dismissNotice(insufficientNoticeID);
    }
  }

  async function register() {
    if (!ready || mutation.isPending()) {
      return;
    }
    // La contraseña se retira del formulario antes de enviar, sea cual sea el resultado.
    const authorization = definition.outflow ? auth.take() : undefined;
    const body: Record<string, unknown> = { ...entryBody(kind, capture) };
    if (authorization) {
      body.authorization = authorization;
    }
    if (print && can('cash.reports.print')) {
      body.print = true;
    }
    try {
      await mutation.execute<RecordedCashOperation>(definition.operation, {
        id: shift.id,
        body,
      });
      // El aviso de confirmación y el cierre del panel los resuelve la escritura del turno.
      setCapture(initialEntry());
      setInsufficient(false);
      auth.reset();
      dismissNotice(insufficientNoticeID);
    } catch (problem) {
      // Un problema de autorización se explica en el formulario de autorización.
      if (auth.handleError(problem)) {
        return;
      }
      if (problem instanceof APIError && problem.code === 'CASH_INSUFFICIENT') {
        // El texto de Go no lleva cifras; la salida excepcional queda a la vista.
        setInsufficient(true);
        notifyError(problem, undefined, { id: insufficientNoticeID });
        return;
      }
      mutation.reportFailure(problem);
    }
  }

  const label =
    wire === '' ? definition.verb : `${definition.verb} ${currencyAmount(capture.currency, wire)}`;
  return (
    <section className="cash-panel" aria-label={definition.title}>
      <header className="cash-panel-header">
        <h3>{definition.title}</h3>
        <button type="button" className="secondary-button" onClick={onClose}>
          <IconLabel icon="back">{backLabel}</IconLabel>
        </button>
      </header>
      <div className="cash-capture">
        <div className="cash-capture-fields">
          <ChoiceGrid
            label={choiceLabels[kind]}
            choices={choices}
            value={capture.choice}
            disabled={mutation.locked}
            onChange={(choice) => update({ choice })}
          />
          {dollars && (
            <SegmentedControl<CashCurrency>
              label="Moneda"
              value={capture.currency}
              onChange={(currency) => update({ currency })}
              disabled={mutation.locked}
              options={[
                { value: 'MXN', label: 'Pesos' },
                { value: 'USD', label: 'Dólares' },
              ]}
            />
          )}
          <AmountField
            label="Importe"
            value={capture.amount}
            onChange={(amount) => update({ amount })}
            active
            emphasis
            currency={capture.currency === 'USD' ? 'US$' : '$'}
            placeholder="0.00"
          />
          <EntryDetails
            kind={kind}
            capture={capture}
            update={update}
            placeholder={chosen?.label ?? ''}
          />
          <PrintToggle
            label="Imprimir comprobante"
            pressed={print}
            onChange={setPrint}
            can={can}
            disabled={mutation.locked}
          />
          {definition.outflow && (
            <OutflowAuthorization
              selfAuthorized={selfAuthorized}
              exceptional={capture.exceptional}
              auth={auth}
              disabled={mutation.locked}
            />
          )}
          {definition.outflow && (insufficient || capture.exceptional) && (
            <InsufficientCash capture={capture} update={update} locked={mutation.locked} />
          )}
        </div>
        <div className="cash-capture-keypad">
          <NumericKeypad
            value={capture.amount}
            onChange={(amount) => update({ amount })}
            label="Teclado numérico del importe"
            captureKeyboard={active}
            disabled={mutation.locked}
          />
          <button
            type="button"
            className="primary-button checkout-register"
            disabled={!ready}
            onClick={() => void register()}
          >
            <IconLabel icon="check">{mutation.busy ? 'Registrando…' : label}</IconLabel>
          </button>
        </div>
      </div>
    </section>
  );
}
