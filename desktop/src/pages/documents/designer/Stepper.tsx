// Selector táctil de un entero pequeño (espaciado, líneas en blanco): menos, valor y más. Los
// límites los declara el catálogo de Go; nunca se usa un campo numérico nativo del navegador.
export function Stepper({
  label,
  value,
  min,
  max,
  unit,
  disabled = false,
  onChange,
}: {
  label: string;
  value: number;
  min: number;
  max: number;
  // Texto del valor: «1 línea», «2 líneas».
  unit: (value: number) => string;
  disabled?: boolean;
  onChange: (value: number) => void;
}) {
  return (
    <div className="designer-stepper" role="group" aria-label={label}>
      <span className="designer-stepper-label">{label}</span>
      <div className="designer-stepper-controls">
        <button
          type="button"
          className="secondary-button"
          aria-label={`${label}: menos`}
          disabled={disabled || value <= min}
          onClick={() => onChange(value - 1)}
        >
          −
        </button>
        <output aria-live="polite">{unit(value)}</output>
        <button
          type="button"
          className="secondary-button"
          aria-label={`${label}: más`}
          disabled={disabled || value >= max}
          onClick={() => onChange(value + 1)}
        >
          +
        </button>
      </div>
    </div>
  );
}

export function linesUnit(value: number) {
  if (value === 0) {
    return 'Ninguna';
  }
  return value === 1 ? '1 línea' : `${value} líneas`;
}
