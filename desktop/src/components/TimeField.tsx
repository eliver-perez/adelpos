import { useEffect, useId, useRef, useState } from 'react';
import { displayToIso, formatDateDigits, isoToDisplay } from './catalog/DateField';
import { Select } from './Select';
import type { SelectOption } from './Select';

// Selector de hora (etapa 9C, §2.1): hora y minutos como dos selectores de la fase B, formato de
// 24 horas y sin escritura libre. Los minutos van de 5 en 5 o de 1 en 1 según el uso; un valor
// guardado que no cae en el paso (por ejemplo 05:07) se conserva como una opción más. «24:00» solo
// existe donde se pide (hora final de las restricciones de horario). La variante con fecha combina
// el campo de fecha dd/mm/aaaa con estos dos selectores. Go sigue validando el valor: aquí solo se
// arma el texto que ya enviaba el campo anterior («HH:MM» o «AAAA-MM-DDTHH:MM»).

export type MinuteStep = 1 | 5;

interface TimeParts {
  hour: string;
  minute: string;
}

const emptyParts: TimeParts = { hour: '', minute: '' };

function pad(value: number) {
  return String(value).padStart(2, '0');
}

// «HH:MM» → hora y minutos; cualquier otro texto, vacío.
export function splitTime(value: string): TimeParts {
  const match = /^([0-9]{2}):([0-9]{2})$/.exec(value);
  return match ? { hour: match[1]!, minute: match[2]! } : emptyParts;
}

// Hora y minutos → «HH:MM»; mientras falte una de las dos partes, vacío.
export function joinTime(parts: TimeParts) {
  return parts.hour && parts.minute ? `${parts.hour}:${parts.minute}` : '';
}

// Agrega el valor vigente si no está entre las opciones del paso (un valor guardado antes).
function withCurrent(values: string[], current: string) {
  if (!/^[0-9]{2}$/.test(current) || values.includes(current)) {
    return values;
  }
  return [...values, current].sort();
}

export function hourOptions(current: string, endOfDay = false): SelectOption[] {
  const values = Array.from({ length: endOfDay ? 25 : 24 }, (_, hour) => pad(hour));
  return withCurrent(values, current).map((value) => ({
    value,
    label: value,
    description: value === '24' ? 'Medianoche, fin del día' : undefined,
  }));
}

export function minuteOptions(step: MinuteStep, current: string, hour = ''): SelectOption[] {
  if (hour === '24') {
    return [{ value: '00', label: '00' }];
  }
  const values = Array.from({ length: 60 / step }, (_, index) => pad(index * step));
  return withCurrent(values, current).map((value) => ({ value, label: value }));
}

// «AAAA-MM-DDTHH:MM» → fecha ISO y hora; cualquier otro texto, vacío.
export function splitDateTime(value: string) {
  const match = /^([0-9]{4}-[0-9]{2}-[0-9]{2})T([0-9]{2}:[0-9]{2})$/.exec(value);
  return match ? { date: match[1]!, time: match[2]! } : { date: '', time: '' };
}

export function joinDateTime(date: string, time: string) {
  return date && time ? `${date}T${time}` : '';
}

// «AAAA-MM-DDTHH:MM» como se lee en pantalla: «15/12/2026 19:30» (sin conversión de zona).
export function formatLocalDateTime(value: string) {
  const { date, time } = splitDateTime(value);
  return date ? `${isoToDisplay(date)} ${time}` : value;
}

interface TimeInputsProps {
  value: string;
  onChange: (value: string) => void;
  step: MinuteStep;
  endOfDay: boolean;
  required: boolean;
  disabled: boolean;
}

// Los dos selectores sin etiqueta propia (la pone el grupo que los contiene). Una hora elegida sin
// minutos toma «00»; unos minutos sin hora quedan a la espera de la hora (el valor sigue vacío).
function TimeInputs({ value, onChange, step, endOfDay, required, disabled }: TimeInputsProps) {
  const [parts, setParts] = useState<TimeParts>(() => splitTime(value));

  // Un valor recibido desde fuera (recarga, «Quitar») reemplaza la selección local, salvo que
  // coincida con ella o que la selección esté a medias y el valor siga vacío.
  useEffect(() => {
    setParts((current) => {
      const joined = joinTime(current);
      if (joined === value || (value === '' && joined === '')) {
        return current;
      }
      return splitTime(value);
    });
  }, [value]);

  function update(next: TimeParts) {
    setParts(next);
    onChange(joinTime(next));
  }

  const midnight = parts.hour === '24';
  return (
    <span className="time-inputs">
      <Select
        aria-label="Hora"
        className="time-select"
        searchable={false}
        placeholder="--"
        required={required}
        disabled={disabled}
        value={parts.hour || null}
        options={hourOptions(parts.hour, endOfDay)}
        onChange={(hour) => update({ hour, minute: hour === '24' ? '00' : parts.minute || '00' })}
      />
      <span className="time-separator" aria-hidden="true">
        :
      </span>
      <Select
        aria-label="Minutos"
        className="time-select"
        searchable={false}
        placeholder="--"
        required={required && !midnight}
        disabled={disabled || midnight}
        value={parts.minute || null}
        options={minuteOptions(step, parts.minute, parts.hour)}
        onChange={(minute) => update({ ...parts, minute })}
      />
    </span>
  );
}

export function TimeField({
  label,
  hint,
  value,
  onChange,
  step = 5,
  endOfDay = false,
  required = false,
  disabled = false,
}: {
  label: string;
  hint?: string;
  // «HH:MM» o vacío.
  value: string;
  onChange: (value: string) => void;
  step?: MinuteStep;
  // Ofrece «24» en la hora (solo la hora final de las restricciones de horario).
  endOfDay?: boolean;
  required?: boolean;
  disabled?: boolean;
}) {
  const id = useId();
  return (
    <div
      className="field time-field"
      role="group"
      aria-labelledby={`${id}-label`}
      aria-describedby={hint ? `${id}-hint` : undefined}
    >
      <span id={`${id}-label`}>{label}</span>
      <TimeInputs
        value={value}
        onChange={onChange}
        step={step}
        endOfDay={endOfDay}
        required={required}
        disabled={disabled}
      />
      {hint && (
        <small id={`${id}-hint`} className="muted">
          {hint}
        </small>
      )}
    </div>
  );
}

// Fecha (dd/mm/aaaa, como el resto de las fechas) y hora con los dos selectores. Entrega
// «AAAA-MM-DDTHH:MM» solo cuando la fecha existe y la hora está completa. Con defaultTime, al
// completar la fecha sin hora se propone esa hora (los filtros: 00:00 «Desde», 23:59 «Hasta»).
export function DateTimeField({
  label,
  hint,
  value,
  onChange,
  step = 5,
  required = false,
  disabled = false,
  defaultTime = '',
}: {
  label: string;
  hint?: string;
  // «AAAA-MM-DDTHH:MM» o vacío.
  value: string;
  onChange: (value: string) => void;
  step?: MinuteStep;
  required?: boolean;
  disabled?: boolean;
  defaultTime?: string;
}) {
  const id = useId();
  const input = useRef<HTMLInputElement>(null);
  const [date, setDate] = useState(() => splitDateTime(value).date);
  const [time, setTime] = useState(() => splitDateTime(value).time);
  const [text, setText] = useState(() => isoToDisplay(splitDateTime(value).date));

  // Un valor recibido desde fuera (recarga o reprogramación en otra terminal) reemplaza la fecha y
  // la hora locales, salvo que coincida con ellas o que estén a medias y el valor siga vacío. Solo
  // se revisa cuando cambia el valor recibido: la fecha y la hora locales son su consecuencia.
  const received = useRef(value);
  useEffect(() => {
    if (received.current === value) {
      return;
    }
    received.current = value;
    if (joinDateTime(date, time) === value || (value === '' && (date === '' || time === ''))) {
      return;
    }
    const next = splitDateTime(value);
    setDate(next.date);
    setTime(next.time);
    setText(isoToDisplay(next.date));
  }, [value, date, time]);

  const dateComplete = text === '' || date !== '';
  const validity = !dateComplete
    ? 'Completa la fecha como dd/mm/aaaa.'
    : time !== '' && date === ''
      ? 'Escribe también la fecha.'
      : '';

  useEffect(() => {
    input.current?.setCustomValidity(validity);
  }, [validity]);

  function changeText(raw: string) {
    const formatted = formatDateDigits(raw);
    if (formatted === null) {
      return;
    }
    const iso = displayToIso(formatted) ?? '';
    const nextTime = iso && !time && defaultTime ? defaultTime : time;
    setText(formatted);
    setDate(iso);
    setTime(nextTime);
    onChange(joinDateTime(iso, nextTime));
  }

  function changeTime(next: string) {
    setTime(next);
    onChange(joinDateTime(date, next));
  }

  function clear() {
    setText('');
    setDate('');
    setTime('');
    onChange('');
  }

  return (
    <div
      className="field date-time-field"
      role="group"
      aria-labelledby={`${id}-label`}
      aria-describedby={hint || validity ? `${id}-hint` : undefined}
    >
      <span id={`${id}-label`}>{label}</span>
      <span className="date-time-inputs">
        <input
          ref={input}
          type="text"
          className="date-time-date"
          aria-label="Fecha"
          inputMode="numeric"
          autoComplete="off"
          placeholder="dd/mm/aaaa"
          maxLength={10}
          required={required}
          disabled={disabled}
          value={text}
          aria-invalid={validity !== '' || undefined}
          onChange={(event) => changeText(event.target.value)}
        />
        <TimeInputs
          value={time}
          onChange={changeTime}
          step={step}
          endOfDay={false}
          required={required || date !== ''}
          disabled={disabled}
        />
        {!required && (text !== '' || time !== '') && (
          <button type="button" className="danger-button" disabled={disabled} onClick={clear}>
            Quitar
          </button>
        )}
      </span>
      {(hint || validity) && (
        <small id={`${id}-hint`} className="muted">
          {validity || hint}
        </small>
      )}
    </div>
  );
}
