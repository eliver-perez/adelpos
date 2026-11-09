// Selector táctil de unidades enteras de una partida (no de dinero): menos, cantidad y más.
// `minimum` (cero por omisión) y `showMax` permiten usarlo también como cantidad del producto en
// la captura, donde no se muestra el máximo; `large` agranda los botones (captura táctil).
export function QuantityStepper({
  verb,
  name,
  quantity,
  max,
  onChange,
  disabled = false,
  minimum = 0,
  showMax = true,
  large = false,
}: {
  verb: string;
  name: string;
  quantity: number;
  max: number;
  onChange: (quantity: number) => void;
  disabled?: boolean;
  minimum?: number;
  showMax?: boolean;
  large?: boolean;
}) {
  if (max === 0) {
    return null;
  }
  return (
    <div
      className={`checkout-line-selector ${large ? 'quantity-stepper-large' : ''}`}
      role="group"
      aria-label={`${verb} ${name}`}
    >
      <span className="muted text-xs">{verb}</span>
      <button
        type="button"
        className="secondary-button"
        aria-label={`Quitar una unidad de ${name}`}
        disabled={disabled || quantity <= minimum}
        onClick={() => onChange(quantity - 1)}
      >
        −
      </button>
      <output aria-label={`Unidades seleccionadas de ${name}`}>{quantity}</output>
      <button
        type="button"
        className="secondary-button"
        aria-label={`Agregar una unidad de ${name}`}
        disabled={disabled || quantity >= max}
        onClick={() => onChange(quantity + 1)}
      >
        +
      </button>
      {showMax && <span className="muted text-xs">de {max}</span>}
    </div>
  );
}
