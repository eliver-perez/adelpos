import { useId } from 'react';
import { denominationLabel, currencyLabels } from '../../services/cash';
import type { CashCurrency, CountLine, CountPreview, Denomination } from '../../services/cash';
import { formatDollars, formatMoney } from '../../services/money';
import { acceptQuantity } from './countCapture';
import type { CountQuantities } from './countCapture';

// Importe de una cifra que entregó Go, con el símbolo de su moneda (formato único de money.ts).
export function currencyAmount(currency: CashCurrency, value: string | null | undefined) {
  return formatMoney(value, currency);
}

// Campo táctil de una denominación: la cantidad contada (texto con dígitos) y el importe que Go
// calculó para ella. El teclado numérico fijo escribe en el campo activo.
function DenominationField({
  denomination,
  quantity,
  line,
  active,
  disabled,
  onActivate,
  onChange,
}: {
  denomination: Denomination;
  quantity: string;
  line: CountLine | undefined;
  active: boolean;
  disabled: boolean;
  onActivate: () => void;
  onChange: (value: string) => void;
}) {
  const identifier = useId();
  const label = denominationLabel(denomination);
  return (
    <div className={`denomination-field ${active ? 'denomination-field-active' : ''}`}>
      <label htmlFor={identifier}>{label}</label>
      <input
        id={identifier}
        type="text"
        inputMode="numeric"
        autoComplete="off"
        spellCheck={false}
        data-virtual-keyboard="off"
        data-numeric-keypad-target="true"
        placeholder="0"
        value={quantity}
        disabled={disabled}
        onFocus={onActivate}
        onClick={onActivate}
        onChange={(event) => onChange(acceptQuantity(event.target.value, quantity))}
      />
      <span className="denomination-amount" aria-label={`Importe de ${label}`}>
        {line ? currencyAmount(denomination.currency, line.amount) : '—'}
      </span>
    </div>
  );
}

// Cuadrícula de denominaciones por moneda. Solo captura cantidades; los importes por línea
// llegan de la vista previa de Go.
export function DenominationCount({
  denominations,
  quantities,
  activeID,
  preview,
  disabled = false,
  onActivate,
  onChange,
}: {
  denominations: Denomination[];
  quantities: CountQuantities;
  activeID: string;
  preview: CountPreview | null;
  disabled?: boolean;
  onActivate: (id: string) => void;
  onChange: (id: string, value: string) => void;
}) {
  const currencies: CashCurrency[] = ['MXN', 'USD'];
  return (
    <div className="denomination-count">
      {currencies.map((currency) => {
        const group = denominations.filter((denomination) => denomination.currency === currency);
        if (group.length === 0) {
          return null;
        }
        return (
          <fieldset key={currency} className="denomination-group" disabled={disabled}>
            <legend>{currencyLabels[currency]}</legend>
            <div className="denomination-grid">
              {group.map((denomination) => (
                <DenominationField
                  key={denomination.id}
                  denomination={denomination}
                  quantity={quantities[denomination.id] ?? ''}
                  line={preview?.lines.find((line) => line.denomination_id === denomination.id)}
                  active={activeID === denomination.id}
                  disabled={disabled}
                  onActivate={() => onActivate(denomination.id)}
                  onChange={(value) => onChange(denomination.id, value)}
                />
              ))}
            </div>
          </fieldset>
        );
      })}
    </div>
  );
}

// Totales declarados por moneda, tal como los calculó Go. Mientras Go recalcula se indica sin
// mostrar cifras viejas. Nunca se compara con el efectivo esperado.
export function CountTotals({
  preview,
  validating,
  error,
  dollars,
  label = 'Total contado',
}: {
  preview: CountPreview | null;
  validating: boolean;
  error: string;
  dollars: boolean;
  label?: string;
}) {
  return (
    <div className="cash-figures" aria-label={label} aria-live="polite">
      <div>
        {label} en pesos
        <strong>{validating ? 'Calculando…' : formatMoney(preview?.counted_mxn)}</strong>
      </div>
      {dollars && (
        <div>
          {label} en dólares
          <strong>{validating ? 'Calculando…' : formatDollars(preview?.counted_usd)}</strong>
        </div>
      )}
      {error && (
        <p className="checkout-inline-note" role="alert">
          {error}
        </p>
      )}
    </div>
  );
}
