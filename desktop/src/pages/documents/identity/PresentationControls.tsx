import { useId } from 'react';
import { SegmentedControl } from '../../../components/SegmentedControl';
import { alignLabels, methodLabels } from '../../../services/documentIdentity';
import type {
  IdentityLimits,
  IdentityPresentation,
  LogoAlign,
  LogoMethod,
} from '../../../services/documentIdentity';

const widthStep = 5;
const widthPresets = [40, 60, 80, 100];
const thresholdStep = 8;
const thresholdPresets = [
  { value: 96, label: 'Más claro' },
  { value: 128, label: 'Normal' },
  { value: 160, label: 'Más oscuro' },
];

function clamp(value: number, minimum: number, maximum: number) {
  return Math.min(maximum, Math.max(minimum, value));
}

// Paso táctil: menos, valor y más, con botones de 44 px (nunca un campo numérico).
function StepControl({
  label,
  value,
  unit,
  minimum,
  maximum,
  step,
  disabled,
  onChange,
}: {
  label: string;
  value: number;
  unit: string;
  minimum: number;
  maximum: number;
  step: number;
  disabled: boolean;
  onChange: (value: number) => void;
}) {
  const labelID = useId();
  return (
    <div className="flex flex-wrap items-center gap-2" role="group" aria-labelledby={labelID}>
      <span id={labelID} className="visually-hidden">
        {label}
      </span>
      <button
        type="button"
        className="secondary-button min-w-[44px]"
        aria-label={`Reducir ${label.toLowerCase()}`}
        disabled={disabled || value <= minimum}
        onClick={() => onChange(clamp(value - step, minimum, maximum))}
      >
        −
      </button>
      <output
        className="min-w-[64px] text-center text-base font-semibold"
        aria-live="polite"
        aria-label={label}
      >
        {value}
        {unit}
      </output>
      <button
        type="button"
        className="secondary-button min-w-[44px]"
        aria-label={`Aumentar ${label.toLowerCase()}`}
        disabled={disabled || value >= maximum}
        onClick={() => onChange(clamp(value + step, minimum, maximum))}
      >
        +
      </button>
    </div>
  );
}

// Presentación del logotipo en papel térmico: ancho relativo, alineación, método y umbral. Go
// convierte el logotipo con estas opciones; aquí solo se eligen.
export function PresentationControls({
  presentation,
  limits,
  readOnly,
  onChange,
}: {
  presentation: IdentityPresentation;
  limits: IdentityLimits;
  readOnly: boolean;
  onChange: (patch: Partial<IdentityPresentation>) => void;
}) {
  const alignOptions = (Object.keys(alignLabels) as LogoAlign[]).map((value) => ({
    value,
    label: alignLabels[value],
  }));
  const methodOptions = (Object.keys(methodLabels) as LogoMethod[]).map((value) => ({
    value,
    label: methodLabels[value],
  }));
  return (
    <div className="form-stack">
      <div className="field">
        <span>Ancho del logotipo (del ancho imprimible)</span>
        <div className="flex flex-wrap items-center gap-3">
          <StepControl
            label="Ancho del logotipo"
            value={presentation.width_percent}
            unit=" %"
            minimum={limits.width_percent_min}
            maximum={limits.width_percent_max}
            step={widthStep}
            disabled={readOnly}
            onChange={(width_percent) => onChange({ width_percent })}
          />
          <div className="button-row" role="group" aria-label="Anchos frecuentes">
            {widthPresets.map((preset) => (
              <button
                key={preset}
                type="button"
                className="secondary-button"
                aria-pressed={presentation.width_percent === preset}
                disabled={readOnly}
                onClick={() => onChange({ width_percent: preset })}
              >
                {preset} %
              </button>
            ))}
          </div>
        </div>
        <small className="muted">
          La altura es proporcional y nunca pasa de {limits.max_height_dots} puntos (4 cm de papel).
        </small>
      </div>
      <div className="field">
        <span>Alineación</span>
        <SegmentedControl
          label="Alineación del logotipo"
          value={presentation.align}
          options={alignOptions}
          disabled={readOnly}
          onChange={(align) => onChange({ align })}
        />
      </div>
      <div className="field">
        <span>Método de conversión a blanco y negro</span>
        <SegmentedControl
          label="Método de conversión"
          value={presentation.method}
          options={methodOptions}
          disabled={readOnly}
          onChange={(method) => onChange({ method })}
        />
        <small className="muted">
          «Umbral» conviene para logotipos de colores planos; «Difuminado» simula grises con puntos
          y conviene para fotografías o degradados.
        </small>
      </div>
      {presentation.method === 'threshold' ? (
        <div className="field">
          <span>Umbral</span>
          <div className="flex flex-wrap items-center gap-3">
            <StepControl
              label="Umbral"
              value={presentation.threshold}
              unit=""
              minimum={limits.threshold_min}
              maximum={limits.threshold_max}
              step={thresholdStep}
              disabled={readOnly}
              onChange={(threshold) => onChange({ threshold })}
            />
            <div className="button-row" role="group" aria-label="Umbrales frecuentes">
              {thresholdPresets.map((preset) => (
                <button
                  key={preset.value}
                  type="button"
                  className="secondary-button"
                  aria-pressed={presentation.threshold === preset.value}
                  disabled={readOnly}
                  onClick={() => onChange({ threshold: preset.value })}
                >
                  {preset.label}
                </button>
              ))}
            </div>
          </div>
          <small className="muted">
            Subir el umbral oscurece el logotipo y bajarlo lo aclara ({limits.threshold_min} a{' '}
            {limits.threshold_max}).
          </small>
        </div>
      ) : (
        <p className="muted text-sm">El difuminado no usa el umbral.</p>
      )}
    </div>
  );
}
