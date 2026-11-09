import { Accordion } from '../Accordion';
import type { ReactNode } from 'react';
import { useEffect, useState } from 'react';
import { Field } from '../Forms';
import { api } from '../../services/api';
import type { QuantityTier, Schedule } from '../../services/catalog';
import {
  acceptDecimalText,
  acceptIntegerText,
  amountShape,
  digitsFor,
  quantityShape,
} from '../../services/numericInput';
import { OptionalDateField } from './DateField';
import { ToggleButton } from '../ToggleButton';
import { dayShortNames, scheduleSummary, weekOrder } from '../../services/scheduleSummary';

export function TextField({
  label,
  value,
  onChange,
  required = false,
  type = 'text',
  hint,
  autoFocus = false,
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
  required?: boolean;
  type?: string;
  hint?: string;
  autoFocus?: boolean;
}) {
  return (
    <Field label={label} hint={hint}>
      <input
        type={type}
        value={value}
        required={required}
        maxLength={2000}
        autoFocus={autoFocus}
        onChange={(event) => onChange(event.target.value)}
      />
    </Field>
  );
}

export function DecimalField({
  label,
  value,
  onChange,
  optional = false,
  quantity = false,
}: {
  label: string;
  value: string | null;
  onChange: (value: string) => void;
  optional?: boolean;
  quantity?: boolean;
}) {
  const shape = quantity ? quantityShape : amountShape;
  const current = value ?? '';
  return (
    <Field label={label} hint={optional ? 'Vacío: heredar o sin límite.' : undefined}>
      <input
        type="text"
        inputMode="decimal"
        autoComplete="off"
        value={current}
        required={!optional}
        pattern={quantity ? '[0-9]+([.][0-9]{1,3})?' : '[0-9]+([.][0-9]{1,2})?'}
        onChange={(event) => {
          // Los caracteres no válidos se descartan al teclear; Go vuelve a validar al guardar.
          const next = acceptDecimalText(event.target.value, current, shape);
          if (next !== current) {
            onChange(next);
          }
        }}
      />
    </Field>
  );
}

// Entero capturado como texto (nunca type="number", hallazgo M2): solo dígitos y hasta el
// máximo indicado. Mientras el campo está vacío no se emite ningún valor; al salir del campo
// vacío se restaura el último valor válido.
export function IntegerField({
  label,
  value,
  onChange,
  min = 0,
  max = 10000,
  hint,
}: {
  label: string;
  value: number;
  onChange: (value: number) => void;
  min?: number;
  max?: number;
  hint?: string;
}) {
  const [text, setText] = useState(String(value));

  useEffect(() => {
    setText((current) => (current === '' || Number(current) !== value ? String(value) : current));
  }, [value]);

  const number = text === '' ? null : Number(text);
  const outOfRange = number !== null && (number < min || number > max);

  return (
    <Field label={label} hint={outOfRange ? `Valor permitido: ${min} a ${max}.` : hint}>
      <input
        type="text"
        inputMode="numeric"
        autoComplete="off"
        value={text}
        required
        aria-invalid={outOfRange}
        onChange={(event) => {
          const next = acceptIntegerText(event.target.value, text, digitsFor(max));
          setText(next);
          if (next !== '' && Number(next) >= min && Number(next) <= max) {
            onChange(Number(next));
          }
        }}
        onBlur={() => {
          if (text === '' || outOfRange) {
            setText(String(value));
          }
        }}
      />
    </Field>
  );
}

export function Check({
  label,
  value,
  onChange,
}: {
  label: string;
  value: boolean;
  onChange: (value: boolean) => void;
}) {
  return (
    <label className="checkbox-label">
      <input type="checkbox" checked={value} onChange={(event) => onChange(event.target.checked)} />
      <span>{label}</span>
    </label>
  );
}

export function ReferenceField({
  label,
  value,
  options,
  onChange,
  optional = false,
}: {
  label: string;
  value: string | null;
  options: { id: string; name: string; active?: boolean }[];
  onChange: (value: string) => void;
  optional?: boolean;
}) {
  // Si el valor guardado no está entre las opciones (registro inactivo o excluido), el navegador
  // mostraría la primera opción de la lista como si estuviera elegida. Se agrega una opción que
  // describe el valor real para no aparentar una elección que el usuario no hizo.
  const current = value ?? '';
  const missing = current !== '' && !options.some((option) => option.id === current);
  return (
    <Field label={label}>
      <select
        value={current}
        required={!optional}
        onChange={(event) => onChange(event.target.value)}
      >
        <option value="">{optional ? 'Sin asignar' : 'Selecciona…'}</option>
        {missing && (
          <option value={current} disabled>
            Registro no disponible en esta lista
          </option>
        )}
        {options.map((option) => (
          <option key={option.id} value={option.id}>
            {option.name}
            {option.active === false ? ' · inactivo' : ''}
          </option>
        ))}
      </select>
    </Field>
  );
}

export function RepeatSection({
  title,
  onAdd,
  children,
  hint,
}: {
  title: string;
  onAdd: () => void;
  children: ReactNode;
  hint?: string;
}) {
  return (
    <section className="catalog-section">
      <div className="section-heading">
        <h3>{title}</h3>
        <button type="button" className="secondary-button" onClick={onAdd}>
          Agregar
        </button>
      </div>
      {hint && <p className="muted text-sm">{hint}</p>}
      {children}
    </section>
  );
}

// Botón para retirar un elemento repetible. Siempre indica qué retira («Quitar tramo»,
// «Quitar componente»…): un «Quitar fila» genérico a todo lo ancho parecía una fila vacía del
// bloque anterior.
export function RemoveButton({ label, onClick }: { label: string; onClick: () => void }) {
  return (
    <button type="button" className="secondary-button remove-button" onClick={onClick}>
      {label}
    </button>
  );
}

export function TierEditor({
  value,
  onChange,
}: {
  value: QuantityTier[];
  onChange: (value: QuantityTier[]) => void;
}) {
  return (
    <RepeatSection
      title="Tramos incrementales"
      hint="Primera unidad desde 1; cada tramo indica el cargo de cada unidad hasta el siguiente. Ejemplo: desde 1 → 10.00; desde 2 → 40.00."
      onAdd={() =>
        onChange([
          ...value,
          { from: value.length ? (value[value.length - 1]?.from ?? 0) + 1 : 1, unit_price: '0.00' },
        ])
      }
    >
      {value.map((tier, index) => (
        <div className="repeat-row" key={index}>
          <IntegerField
            label="Desde la unidad"
            value={tier.from}
            min={1}
            max={1000}
            onChange={(from) =>
              onChange(
                value.map((item, position) => (position === index ? { ...item, from } : item)),
              )
            }
          />
          <DecimalField
            label="Cargo por unidad"
            value={tier.unit_price}
            onChange={(unit_price) =>
              onChange(
                value.map((item, position) =>
                  position === index ? { ...item, unit_price } : item,
                ),
              )
            }
          />
          <RemoveButton
            label="Quitar tramo"
            onClick={() => onChange(value.filter((_, position) => position !== index))}
          />
        </div>
      ))}
    </RepeatSection>
  );
}

export const emptySchedule = (): Schedule => ({ days: [], windows: [] });

export function ScheduleEditor({
  value,
  onChange,
}: {
  value: Schedule;
  onChange: (value: Schedule) => void;
}) {
  const days = value.days ?? [];
  return (
    <Accordion title="Restricciones de horario" summary={scheduleSummary(value)}>
      <ul className="schedule-help muted text-sm">
        <li>
          Sin restricciones, el registro está disponible siempre. Se usa el reloj del servidor con
          la zona horaria del establecimiento.
        </li>
        <li>Si no eliges días, aplica todos los días de la semana.</li>
        <li>
          La hora final no se incluye: con 12:00 a 15:00 deja de estar disponible justo a las 15:00.
          Para llegar hasta la medianoche usa 24:00 como hora final.
        </li>
        <li>
          Si el horario cruza la medianoche, captúralo como dos horarios. Ejemplo: de 20:00 a 02:00
          se captura como 20:00 a 24:00 y 00:00 a 02:00. Si además eliges días, el horario de 00:00
          a 02:00 cuenta para el día siguiente.
        </li>
      </ul>
      <div className="form-grid">
        <OptionalDateField
          label="Desde la fecha"
          value={value.starts_on ?? ''}
          onChange={(starts_on) => onChange({ ...value, starts_on })}
        />
        <OptionalDateField
          label="Hasta la fecha (incluida)"
          value={value.ends_on ?? ''}
          onChange={(ends_on) => onChange({ ...value, ends_on })}
        />
      </div>
      <div className="field">
        <span>Días de la semana</span>
        <div className="schedule-days" role="group" aria-label="Días de la semana">
          {weekOrder.map((day) => (
            <ToggleButton
              key={day}
              label={dayShortNames[day] ?? ''}
              pressed={days.includes(day)}
              onChange={(selected) =>
                onChange({
                  ...value,
                  days: selected
                    ? [...days, day].sort((first, second) => first - second)
                    : days.filter((entry) => entry !== day),
                })
              }
            />
          ))}
        </div>
      </div>
      <RepeatSection
        title="Horarios del día"
        hint="La hora final no se incluye."
        onAdd={() =>
          onChange({
            ...value,
            windows: [...(value.windows ?? []), { start: '09:00', end: '18:00' }],
          })
        }
      >
        {(value.windows ?? []).map((window, index) => (
          <div className="repeat-row" key={index}>
            <TextField
              label="Hora inicial"
              type="time"
              value={window.start}
              onChange={(start) =>
                onChange({
                  ...value,
                  windows: value.windows.map((item, position) =>
                    position === index ? { ...item, start } : item,
                  ),
                })
              }
            />
            <TextField
              label="Hora final, no incluida (HH:mm; 24:00 = medianoche)"
              type="text"
              value={window.end}
              onChange={(end) =>
                onChange({
                  ...value,
                  windows: value.windows.map((item, position) =>
                    position === index ? { ...item, end } : item,
                  ),
                })
              }
            />
            <RemoveButton
              label="Quitar horario"
              onClick={() =>
                onChange({
                  ...value,
                  windows: value.windows.filter((_, position) => position !== index),
                })
              }
            />
          </div>
        ))}
      </RepeatSection>
    </Accordion>
  );
}

// `decorative` omite el texto alternativo cuando la imagen acompaña a un nombre ya visible (por
// ejemplo, dentro del botón de un producto en la captura).
export function CatalogImage({
  id,
  decorative = false,
}: {
  id: string | null;
  decorative?: boolean;
}) {
  const [source, setSource] = useState('');
  useEffect(() => {
    let current = true;
    setSource('');
    if (id) {
      void api<{ data_uri: string }>('catalog_image', { id })
        .then((result) => {
          if (current) setSource(result.data_uri);
        })
        .catch(() => {});
    }
    return () => {
      current = false;
    };
  }, [id]);
  if (decorative) {
    return source ? (
      <img src={source} alt="" className="catalog-image" />
    ) : (
      <span className="catalog-image catalog-placeholder" aria-hidden="true">
        ◇
      </span>
    );
  }
  return source ? (
    <img src={source} alt="Fotografía del catálogo" className="catalog-image" />
  ) : (
    <span className="catalog-image catalog-placeholder" aria-label="Sin fotografía">
      ◇
    </span>
  );
}
