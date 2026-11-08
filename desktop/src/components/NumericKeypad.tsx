import { useEffect, useId, useRef } from 'react';
import type { ReactNode } from 'react';
import { acceptAmountText, applyAmountKey, physicalAmountKey } from '../services/amountInput';
import type { AmountKey } from '../services/amountInput';

interface KeyDefinition {
  key: AmountKey;
  label: string;
  name: string;
  className?: string;
}

// Distribución fija de 4 × 4: borrar y limpiar ocupan dos filas cada uno en la última columna.
const layout: KeyDefinition[] = [
  { key: '7', label: '7', name: '7' },
  { key: '8', label: '8', name: '8' },
  { key: '9', label: '9', name: '9' },
  {
    key: 'backspace',
    label: '⌫',
    name: 'Borrar último carácter',
    className: 'numeric-key-action numeric-key-tall',
  },
  { key: '4', label: '4', name: '4' },
  { key: '5', label: '5', name: '5' },
  { key: '6', label: '6', name: '6' },
  { key: '1', label: '1', name: '1' },
  { key: '2', label: '2', name: '2' },
  { key: '3', label: '3', name: '3' },
  {
    key: 'clear',
    label: 'Limpiar',
    name: 'Limpiar importe',
    className: 'numeric-key-action numeric-key-tall',
  },
  { key: '00', label: '00', name: 'Doble cero' },
  { key: '0', label: '0', name: '0' },
  { key: '.', label: '.', name: 'Punto decimal' },
];

// Un campo editable distinto de los importes del teclado (por ejemplo, la referencia) conserva
// su escritura física normal.
function foreignEditable(target: EventTarget | null) {
  if (!(target instanceof HTMLElement)) {
    return false;
  }
  if (target.closest('[data-numeric-keypad-target="true"]')) {
    return false;
  }
  return (
    target instanceof HTMLInputElement ||
    target instanceof HTMLTextAreaElement ||
    target instanceof HTMLSelectElement ||
    target.isContentEditable
  );
}

// Teclado numérico fijo de cobro. Escribe sobre el estado de cadena del importe activo mediante
// applyAmountKey; nunca simula eventos sobre un <input>.
export function NumericKeypad({
  value,
  onChange,
  label = 'Teclado numérico',
  captureKeyboard = false,
  disabled = false,
}: {
  value: string;
  onChange: (value: string) => void;
  label?: string;
  captureKeyboard?: boolean;
  disabled?: boolean;
}) {
  // La referencia evita reinstalar el oyente del teclado físico en cada pulsación.
  const latest = useRef({ value, onChange });
  latest.current = { value, onChange };

  useEffect(() => {
    if (!captureKeyboard || disabled) {
      return;
    }
    function typed(event: KeyboardEvent) {
      if (event.defaultPrevented || event.ctrlKey || event.metaKey || event.altKey) {
        return;
      }
      if (foreignEditable(event.target)) {
        return;
      }
      const key = physicalAmountKey(event.key);
      if (!key) {
        return;
      }
      event.preventDefault();
      const next = applyAmountKey(latest.current.value, key);
      if (next !== latest.current.value) {
        latest.current.onChange(next);
      }
    }
    window.addEventListener('keydown', typed);
    return () => window.removeEventListener('keydown', typed);
  }, [captureKeyboard, disabled]);

  function press(key: AmountKey) {
    const next = applyAmountKey(value, key);
    if (next !== value) {
      onChange(next);
    }
  }

  return (
    <div
      className="numeric-keypad"
      role="group"
      aria-label={label}
      // Conservar el foco en el campo activo permite seguir escribiendo con el teclado físico.
      onPointerDown={(event) => event.preventDefault()}
    >
      {layout.map((definition) => (
        <button
          type="button"
          key={definition.key}
          className={`numeric-key ${definition.className ?? ''}`}
          aria-label={definition.name}
          disabled={disabled}
          onClick={() => press(definition.key)}
        >
          {definition.label}
        </button>
      ))}
    </div>
  );
}

// Importes sugeridos junto al campo que completan (por ejemplo, billetes para el efectivo
// recibido). Copian una cadena fija o entregada por Go; no calculan nada.
export function QuickAmounts({
  amounts,
  onSelect,
  disabled = false,
}: {
  amounts: { label: string; value: string }[];
  onSelect: (value: string) => void;
  disabled?: boolean;
}) {
  if (amounts.length === 0) {
    return null;
  }
  return (
    <div className="numeric-quick-keys" role="group" aria-label="Importes rápidos">
      {amounts.map((quick) => (
        <button
          type="button"
          key={quick.label}
          disabled={disabled}
          onPointerDown={(event) => event.preventDefault()}
          onClick={() => onSelect(quick.value)}
        >
          {quick.label}
        </button>
      ))}
    </div>
  );
}

// Campo de importe enlazado al teclado fijo: texto con inputMode decimal, excluido del teclado
// flotante y validado con la misma regla que el teclado de cobro.
export function AmountField({
  label,
  value,
  onChange,
  active = false,
  onActivate,
  placeholder,
  hint,
  disabled = false,
  emphasis = false,
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
  active?: boolean;
  onActivate?: () => void;
  placeholder?: string;
  hint?: ReactNode;
  disabled?: boolean;
  emphasis?: boolean;
}) {
  const identifier = useId();
  const inputID = `${identifier}-amount`;
  const hintID = `${identifier}-hint`;
  const container = useRef<HTMLDivElement>(null);

  // El campo que recibe el teclado se mantiene visible aunque el área se desplace o cambie de
  // tamaño (por ejemplo, cuando Go entrega el cambio y el pie del cobro crece).
  useEffect(() => {
    const element = container.current;
    if (!active || !element) {
      return;
    }
    element.scrollIntoView?.({ block: 'nearest' });
    const area = element.closest('.checkout-area-body');
    if (!area || typeof ResizeObserver === 'undefined') {
      return;
    }
    const observer = new ResizeObserver(() => element.scrollIntoView?.({ block: 'nearest' }));
    observer.observe(area);
    return () => observer.disconnect();
  }, [active]);

  return (
    <div
      ref={container}
      className={`amount-field ${active ? 'amount-field-active' : ''} ${emphasis ? 'amount-field-emphasis' : ''}`}
    >
      <label className="amount-field-label" htmlFor={inputID}>
        {label}
      </label>
      <span className="amount-field-control">
        <span className="amount-field-currency" aria-hidden="true">
          $
        </span>
        <input
          id={inputID}
          type="text"
          inputMode="decimal"
          autoComplete="off"
          spellCheck={false}
          data-virtual-keyboard="off"
          data-numeric-keypad-target="true"
          aria-describedby={hint ? hintID : undefined}
          value={value}
          placeholder={placeholder}
          disabled={disabled}
          onFocus={onActivate}
          onClick={onActivate}
          onChange={(event) => {
            const next = acceptAmountText(event.target.value, value);
            if (next !== value) {
              onChange(next);
            }
          }}
        />
      </span>
      {hint && (
        <small id={hintID} className="amount-field-hint">
          {hint}
        </small>
      )}
    </div>
  );
}
