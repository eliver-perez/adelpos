import { useId, useRef, useState } from 'react';
import type { PaletteColor } from '../../services/catalog';

const hexPattern = /^#[0-9a-fA-F]{6}$/;

// Nombre visible del color guardado: el de la paleta, «Sin color» o, para un valor capturado
// antes de la paleta (que Go conserva), «Color personalizado».
export function colorName(value: string, palette: PaletteColor[]): string {
  if (!value) {
    return 'Sin color';
  }
  const match = palette.find((color) => color.hex.toLowerCase() === value.toLowerCase());
  return match ? match.name : 'Color personalizado';
}

function Swatch({ color }: { color: string }) {
  if (!hexPattern.test(color)) {
    return <span className="color-swatch color-swatch-empty" aria-hidden="true" />;
  }
  return <span className="color-swatch" style={{ background: color }} aria-hidden="true" />;
}

// Color de categoría sin escritura libre: al tocar el campo se abre la paleta definida en Go
// (cuadrícula de botones con muestra y nombre). «Sin color» es el valor inicial.
export function ColorField({
  label = 'Color de la categoría',
  value,
  palette,
  onChange,
}: {
  label?: string;
  value: string;
  palette: PaletteColor[];
  onChange: (value: string) => void;
}) {
  const [open, setOpen] = useState(false);
  const identifier = useId();
  const trigger = useRef<HTMLButtonElement>(null);
  const panelID = `${identifier}-palette`;
  const name = colorName(value, palette);

  function choose(next: string) {
    onChange(next);
    setOpen(false);
    trigger.current?.focus();
  }

  return (
    <div className="color-field">
      <span>{label}</span>
      <div className="color-field-current">
        <button
          ref={trigger}
          type="button"
          className="color-trigger"
          aria-label={`${label}: ${name}`}
          aria-expanded={open}
          aria-controls={panelID}
          onClick={() => setOpen(!open)}
        >
          <Swatch color={value} />
          <span>{name}</span>
        </button>
        {value && (
          <button type="button" className="danger-button" onClick={() => choose('')}>
            Quitar color
          </button>
        )}
      </div>
      {open && (
        <div
          id={panelID}
          className="color-palette"
          role="group"
          aria-label="Paleta de colores"
          onKeyDown={(event) => {
            if (event.key === 'Escape') {
              event.preventDefault();
              setOpen(false);
              trigger.current?.focus();
            }
          }}
        >
          {palette.map((color) => (
            <button
              key={color.hex}
              type="button"
              className="color-option"
              aria-pressed={value.toLowerCase() === color.hex.toLowerCase()}
              onClick={() => choose(color.hex)}
            >
              <Swatch color={color.hex} />
              <span>{color.name}</span>
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
