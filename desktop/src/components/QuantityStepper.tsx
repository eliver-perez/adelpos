// Selector táctil de unidades enteras de una partida (no de dinero): menos, cantidad y más.
export function QuantityStepper({
  verb,
  name,
  quantity,
  max,
  onChange,
  disabled = false,
}: {
  verb: string;
  name: string;
  quantity: number;
  max: number;
  onChange: (quantity: number) => void;
  disabled?: boolean;
}) {
  if (max === 0) {
    return null;
  }
  return (
    <div className="checkout-line-selector" role="group" aria-label={`${verb} ${name}`}>
      <span className="muted text-xs">{verb}</span>
      <button
        type="button"
        className="secondary-button"
        aria-label={`Quitar una unidad de ${name}`}
        disabled={disabled || quantity === 0}
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
      <span className="muted text-xs">de {max}</span>
    </div>
  );
}
