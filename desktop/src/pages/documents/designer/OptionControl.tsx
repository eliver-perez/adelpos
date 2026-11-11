import { SegmentedControl } from '../../../components/SegmentedControl';
import { ToggleButton } from '../../../components/ToggleButton';
import type {
  CatalogKind,
  DocumentCatalog,
  OptionSpec,
  TemplateBlock,
} from '../../../services/documents';
import { fieldItems } from './draft';
import { FieldItemsEditor } from './FieldItemsEditor';
import { Stepper, linesUnit } from './Stepper';

// Control táctil de una opción de bloque según su tipo en el catálogo de Go: opciones cerradas con
// botones, interruptores, textos con su límite (el teclado virtual aparece al tocarlos), enteros
// con pasos y los datos del «Campo de datos». Nunca se pide código, HTML, CSS, JSON ni ESC/POS.

// Las opciones cortas caben en un control segmentado; las largas («Según la configuración de
// Cocina») se muestran como una lista de botones de una opción por fila.
function compactValues(spec: OptionSpec) {
  const values = spec.values ?? [];
  return values.length <= 4 && values.every((value) => value.label.length <= 14);
}

function EnumOption({
  spec,
  value,
  disabled,
  onChange,
}: {
  spec: OptionSpec;
  value: string;
  disabled: boolean;
  onChange: (value: string) => void;
}) {
  const values = spec.values ?? [];
  if (compactValues(spec)) {
    return (
      <div className="designer-control">
        <span className="designer-control-label">{spec.label}</span>
        <SegmentedControl
          label={spec.label}
          value={value}
          options={values.map((item) => ({ value: item.value, label: item.label }))}
          disabled={disabled}
          onChange={onChange}
        />
      </div>
    );
  }
  return (
    <div className="designer-control">
      <span className="designer-control-label">{spec.label}</span>
      <div className="designer-choice-list" role="group" aria-label={spec.label}>
        {values.map((item) => (
          <button
            type="button"
            key={item.value}
            className={`designer-choice ${item.value === value ? 'designer-choice-selected' : ''}`}
            aria-pressed={item.value === value}
            disabled={disabled}
            onClick={() => onChange(item.value)}
          >
            {item.label}
          </button>
        ))}
      </div>
    </div>
  );
}

function TextOption({
  spec,
  value,
  disabled,
  onChange,
}: {
  spec: OptionSpec;
  value: string;
  disabled: boolean;
  onChange: (value: string) => void;
}) {
  const multiline = (spec.max_lines ?? 1) > 1;
  const limit = spec.max_length ?? 300;
  const lines = value === '' ? 0 : value.split('\n').length;
  const hint = multiline
    ? `${value.length} de ${limit} caracteres · ${lines} de ${spec.max_lines} líneas`
    : `${value.length} de ${limit} caracteres`;
  return (
    <label className="field">
      <span>{spec.label}</span>
      {multiline ? (
        <textarea
          aria-label={spec.label}
          value={value}
          maxLength={limit}
          rows={Math.min(spec.max_lines ?? 3, 4)}
          disabled={disabled}
          onChange={(event) => onChange(event.target.value)}
        />
      ) : (
        <input
          type="text"
          aria-label={spec.label}
          value={value}
          maxLength={limit}
          disabled={disabled}
          onChange={(event) => onChange(event.target.value)}
        />
      )}
      <small className="muted">{hint}</small>
    </label>
  );
}

export function OptionControl({
  spec,
  block,
  kind,
  catalog,
  disabled,
  onChange,
}: {
  spec: OptionSpec;
  block: TemplateBlock;
  kind: CatalogKind;
  catalog: DocumentCatalog;
  disabled: boolean;
  onChange: (key: string, value: unknown) => void;
}) {
  const value = block.options[spec.key] ?? spec.default;
  switch (spec.type) {
    case 'boolean':
      return (
        <ToggleButton
          label={spec.label}
          pressed={value === true}
          disabled={disabled}
          onChange={(pressed) => onChange(spec.key, pressed)}
        />
      );
    case 'enum':
      return (
        <EnumOption
          spec={spec}
          value={String(value)}
          disabled={disabled}
          onChange={(next) => onChange(spec.key, next)}
        />
      );
    case 'text':
      return (
        <TextOption
          spec={spec}
          value={typeof value === 'string' ? value : ''}
          disabled={disabled}
          onChange={(next) => onChange(spec.key, next)}
        />
      );
    case 'integer':
      return (
        <Stepper
          label={spec.label}
          value={typeof value === 'number' ? value : (spec.min ?? 0)}
          min={spec.min ?? 0}
          max={spec.max ?? catalog.limits.spacer_lines}
          unit={linesUnit}
          disabled={disabled}
          onChange={(next) => onChange(spec.key, next)}
        />
      );
    case 'field_items':
      return (
        <FieldItemsEditor
          items={fieldItems(block)}
          keys={kind.field_keys}
          max={spec.max ?? catalog.limits.field_items}
          labelLimit={spec.max_length ?? catalog.limits.label_chars}
          pair={block.options.layout === 'pair'}
          disabled={disabled}
          onChange={(items) => onChange(spec.key, items)}
        />
      );
  }
  return null;
}
