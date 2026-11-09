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

export function TextField({
  label,
  value,
  onChange,
  required = false,
  type = 'text',
  hint,
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
  required?: boolean;
  type?: string;
  hint?: string;
}) {
  return (
    <Field label={label} hint={hint}>
      <input
        type={type}
        value={value}
        required={required}
        maxLength={2000}
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
  const days = ['Dom', 'Lun', 'Mar', 'Mié', 'Jue', 'Vie', 'Sáb'];
  return (
    <Accordion title="Fechas, días y horarios">
      <p className="muted text-sm">
        Sin restricciones: disponible siempre. Se usa el reloj del servidor y la zona del
        establecimiento. Divide los horarios que cruzan medianoche en dos intervalos; usa 24:00 como
        fin del día.
      </p>
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
      <div className="button-row">
        {days.map((name, day) => (
          <Check
            key={day}
            label={name}
            value={value.days?.includes(day) ?? false}
            onChange={(selected) =>
              onChange({
                ...value,
                days: selected
                  ? [...(value.days ?? []), day]
                  : (value.days ?? []).filter((entry) => entry !== day),
              })
            }
          />
        ))}
      </div>
      <RepeatSection
        title="Intervalos (fin exclusivo)"
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
              label="Hora final (HH:mm; 24:00 cierra el día)"
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
              label="Quitar intervalo"
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

export function CatalogImage({ id }: { id: string | null }) {
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
  return source ? (
    <img src={source} alt="Fotografía del catálogo" className="catalog-image" />
  ) : (
    <span className="catalog-image catalog-placeholder" aria-label="Sin fotografía">
      ◇
    </span>
  );
}
