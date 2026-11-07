import type { ReactNode } from 'react';
import { useEffect, useState } from 'react';
import { Field, Notice } from '../Forms';
import { api } from '../../services/api';
import type { QuantityTier, Schedule } from '../../services/catalog';

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
  return (
    <Field label={label} hint={optional ? 'Vacío: heredar o sin límite.' : undefined}>
      <input
        inputMode="decimal"
        value={value ?? ''}
        required={!optional}
        pattern={quantity ? '[0-9]+([.][0-9]{1,3})?' : '[0-9]+([.][0-9]{1,2})?'}
        onChange={(event) => onChange(event.target.value)}
      />
    </Field>
  );
}

export function IntegerField({
  label,
  value,
  onChange,
  min = 0,
  max = 10000,
}: {
  label: string;
  value: number;
  onChange: (value: number) => void;
  min?: number;
  max?: number;
}) {
  return (
    <Field label={label}>
      <input
        type="number"
        step="1"
        min={min}
        max={max}
        value={value}
        required
        onChange={(event) => onChange(Number(event.target.value))}
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
  return (
    <Field label={label}>
      <select
        value={value ?? ''}
        required={!optional}
        onChange={(event) => onChange(event.target.value)}
      >
        <option value="">{optional ? 'Sin asignar' : 'Selecciona…'}</option>
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

export function RemoveButton({ onClick }: { onClick: () => void }) {
  return (
    <button type="button" className="secondary-button" onClick={onClick}>
      Quitar fila
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
    <details className="catalog-section">
      <summary>Fechas, días y horarios</summary>
      <p className="muted text-sm">
        Sin restricciones: disponible siempre. Se usa el reloj del servidor y la zona del
        establecimiento. Divide los horarios que cruzan medianoche en dos intervalos; usa 24:00 como
        fin del día.
      </p>
      <div className="form-grid">
        <TextField
          label="Desde la fecha"
          type="date"
          value={value.starts_on ?? ''}
          onChange={(starts_on) => onChange({ ...value, starts_on })}
        />
        <TextField
          label="Hasta la fecha (incluida)"
          type="date"
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
    </details>
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

export function ImageEditor({
  id,
  onChange,
}: {
  id: string | null;
  onChange: (id: string | null) => void;
}) {
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  return (
    <section className="catalog-section">
      <h3>Imagen opcional</h3>
      <div className="button-row">
        <CatalogImage id={id} />
        <Field label={busy ? 'Cargando…' : 'PNG o JPEG · hasta 1 MiB y 2048 × 2048'}>
          <input
            type="file"
            accept="image/png,image/jpeg"
            disabled={busy}
            onChange={(event) => {
              const file = event.target.files?.[0];
              if (!file) return;
              setError('');
              if (file.size > 1024 * 1024) {
                setError('La imagen excede 1 MiB.');
                return;
              }
              setBusy(true);
              const reader = new FileReader();
              reader.onload = () => {
                const content = String(reader.result).split(',')[1];
                void api<{ id: string }>('catalog_upload_image', {
                  body: { content_base64: content },
                })
                  .then((result) => onChange(result.id))
                  .catch((problem: unknown) =>
                    setError(problem instanceof Error ? problem.message : 'No se pudo cargar.'),
                  )
                  .finally(() => setBusy(false));
              };
              reader.onerror = () => {
                setError('No se pudo leer la imagen.');
                setBusy(false);
              };
              reader.readAsDataURL(file);
            }}
          />
        </Field>
        {id && (
          <button type="button" className="secondary-button" onClick={() => onChange(null)}>
            Retirar imagen
          </button>
        )}
      </div>
      <Notice>{error}</Notice>
    </section>
  );
}
