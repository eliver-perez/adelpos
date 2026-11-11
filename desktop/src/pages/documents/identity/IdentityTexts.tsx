import { useId } from 'react';
import { typedTextStats } from '../../../services/documentIdentity';
import type { IdentityFields, IdentityLimits } from '../../../services/documentIdentity';

type TextKey = 'additional_info' | 'header_text' | 'footer_text';

const textFields: { key: TextKey; label: string; hint: string; placeholder: string }[] = [
  {
    key: 'additional_info',
    label: 'Información adicional',
    hint: 'Datos autorizados que quieras mostrar, por ejemplo RFC, razón social o número de permiso.',
    placeholder: 'Ej. RFC: XAXX010101000',
  },
  {
    key: 'header_text',
    label: 'Texto de encabezado',
    hint: 'Se imprime con el bloque «Texto de encabezado» de las plantillas.',
    placeholder: 'Ej. ¡Bienvenidos!',
  },
  {
    key: 'footer_text',
    label: 'Texto de pie',
    hint: 'Se imprime con el bloque «Texto de pie» de las plantillas.',
    placeholder: 'Ej. Gracias por su visita',
  },
];

function textPatch(key: TextKey, value: string): Partial<IdentityFields> {
  const patch: Partial<IdentityFields> = {};
  patch[key] = value;
  return patch;
}

// Textos de la identidad. Son campos de texto comunes (compatibles con el teclado virtual): Go
// quita los caracteres de control, corta las líneas al ancho del papel y valida los límites.
export function IdentityTexts({
  fields,
  limits,
  readOnly,
  onChange,
}: {
  fields: IdentityFields;
  limits: IdentityLimits;
  readOnly: boolean;
  onChange: (patch: Partial<IdentityFields>) => void;
}) {
  return (
    <section className="panel form-stack" aria-labelledby="identity-texts-title">
      <div>
        <h2 id="identity-texts-title">Textos de los documentos</h2>
        <p className="muted text-sm">
          Hasta {limits.text_max_characters} caracteres y {limits.text_max_lines} renglones cada
          uno. Solo aparecen en los documentos cuya plantilla incluye su bloque.
        </p>
      </div>
      <div className="grid gap-5">
        {textFields.map((field) => (
          <IdentityTextField
            key={field.key}
            label={field.label}
            hint={field.hint}
            placeholder={field.placeholder}
            value={fields[field.key]}
            limits={limits}
            readOnly={readOnly}
            onChange={(value) => onChange(textPatch(field.key, value))}
          />
        ))}
      </div>
    </section>
  );
}

function IdentityTextField({
  label,
  hint,
  placeholder,
  value,
  limits,
  readOnly,
  onChange,
}: {
  label: string;
  hint: string;
  placeholder: string;
  value: string;
  limits: IdentityLimits;
  readOnly: boolean;
  onChange: (value: string) => void;
}) {
  const fieldID = useId();
  const counterID = useId();
  const stats = typedTextStats(value);
  const over = stats.characters > limits.text_max_characters || stats.lines > limits.text_max_lines;
  return (
    <div className="field">
      <label htmlFor={fieldID}>{label}</label>
      <textarea
        id={fieldID}
        rows={4}
        value={value}
        placeholder={readOnly ? '' : placeholder}
        readOnly={readOnly}
        aria-describedby={counterID}
        aria-invalid={over}
        onChange={(event) => onChange(event.target.value)}
      />
      <small id={counterID} className={over ? 'text-[var(--danger)]' : 'muted'}>
        {stats.characters} de {limits.text_max_characters} caracteres · {stats.lines} de{' '}
        {limits.text_max_lines} renglones. {hint}
      </small>
    </div>
  );
}
