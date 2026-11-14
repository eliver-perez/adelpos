import { installationModes, modeDescriptions, modeLabels } from '../../services/installation';
import type { InstallationMode } from '../../services/installation';

// Las tres modalidades como opciones grandes para pantalla táctil (etapa 9G, §2.3), con el patrón de
// radio de ARIA: una sola elegida, flechas para moverse y la descripción en palabras del restaurante.
// La usan la elección del primer arranque y «Modalidad de esta computadora» en Mantenimiento.
export function ModalityOptions({
  value,
  onChange,
  disabled = false,
  current,
}: {
  value: InstallationMode;
  onChange: (mode: InstallationMode) => void;
  disabled?: boolean;
  // Modalidad vigente, marcada como «Actual» (en Mantenimiento).
  current?: InstallationMode;
}) {
  function move(step: number) {
    const index = installationModes.indexOf(value);
    const next =
      installationModes[(index + step + installationModes.length) % installationModes.length];
    if (next) {
      onChange(next);
    }
  }
  return (
    <div
      className="modality-options"
      role="radiogroup"
      aria-label="Modalidad de esta computadora"
      onKeyDown={(event) => {
        if (event.key === 'ArrowDown' || event.key === 'ArrowRight') {
          event.preventDefault();
          move(1);
        }
        if (event.key === 'ArrowUp' || event.key === 'ArrowLeft') {
          event.preventDefault();
          move(-1);
        }
      }}
    >
      {installationModes.map((mode) => {
        const selected = mode === value;
        return (
          <button
            key={mode}
            type="button"
            role="radio"
            aria-checked={selected}
            tabIndex={selected ? 0 : -1}
            disabled={disabled}
            className={`modality-option ${selected ? 'modality-option-selected' : ''}`}
            onClick={() => onChange(mode)}
          >
            <span className="modality-option-title">
              {modeLabels[mode]}
              {current === mode && <span className="modality-option-current">Actual</span>}
            </span>
            <span className="modality-option-description">{modeDescriptions[mode]}</span>
          </button>
        );
      })}
    </div>
  );
}
