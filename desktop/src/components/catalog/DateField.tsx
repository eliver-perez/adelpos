import { useEffect, useRef, useState } from 'react';
import { Field } from '../Forms';

// Fecha opcional capturada como texto dd/mm/aaaa.
//
// El control nativo <input type="date"> de WebKit (el WebView de Tauri en macOS) muestra la fecha
// de hoy aunque el campo esté vacío, de modo que un registro sin restricciones parecía limitado a
// la fecha de captura. Este campo queda vacío (con el ejemplo dd/mm/aaaa) cuando no hay valor,
// acepta solo dígitos y entrega a Go la fecha ISO (AAAA-MM-DD) únicamente cuando está completa y
// existe en el calendario.

export function isoToDisplay(iso: string): string {
  const match = /^([0-9]{4})-([0-9]{2})-([0-9]{2})$/.exec(iso);
  if (!match) {
    return '';
  }
  return `${match[3]}/${match[2]}/${match[1]}`;
}

// Reconstruye dd/mm/aaaa a partir de los dígitos tecleados, insertando las diagonales.
export function formatDateDigits(text: string): string | null {
  if (!/^[0-9/]*$/.test(text)) {
    return null;
  }
  const digits = text.replaceAll('/', '').slice(0, 8);
  const day = digits.slice(0, 2);
  const month = digits.slice(2, 4);
  const year = digits.slice(4, 8);
  if (digits.length <= 2) {
    return day;
  }
  if (digits.length <= 4) {
    return `${day}/${month}`;
  }
  return `${day}/${month}/${year}`;
}

// Convierte dd/mm/aaaa completo a AAAA-MM-DD; devuelve null si la fecha no existe.
export function displayToIso(display: string): string | null {
  const match = /^([0-9]{2})\/([0-9]{2})\/([0-9]{4})$/.exec(display);
  if (!match) {
    return null;
  }
  const day = Number(match[1]);
  const month = Number(match[2]);
  const year = Number(match[3]);
  const candidate = new Date(Date.UTC(year, month - 1, day));
  const exists =
    candidate.getUTCFullYear() === year &&
    candidate.getUTCMonth() === month - 1 &&
    candidate.getUTCDate() === day;
  if (!exists || year < 2000) {
    return null;
  }
  return `${match[3]}-${match[2]}-${match[1]}`;
}

export function OptionalDateField({
  label,
  value,
  onChange,
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
}) {
  const [text, setText] = useState(isoToDisplay(value));
  const input = useRef<HTMLInputElement>(null);

  // Un valor recibido desde fuera (recarga o «Quitar fecha») reemplaza el texto local, salvo que
  // coincida con lo que se está capturando.
  useEffect(() => {
    setText((current) => {
      if (displayToIso(current) === value) {
        return current;
      }
      if (value === '' && displayToIso(current) === null) {
        return current;
      }
      return isoToDisplay(value);
    });
  }, [value]);

  const complete = text === '' || displayToIso(text) !== null;

  useEffect(() => {
    input.current?.setCustomValidity(complete ? '' : 'Completa la fecha como dd/mm/aaaa.');
  }, [complete]);

  return (
    <Field label={label} hint={complete ? undefined : 'Formato dd/mm/aaaa'}>
      <div className="inline-field">
        <input
          ref={input}
          type="text"
          inputMode="numeric"
          autoComplete="off"
          placeholder="dd/mm/aaaa"
          maxLength={10}
          value={text}
          aria-invalid={!complete}
          onChange={(event) => {
            const formatted = formatDateDigits(event.target.value);
            if (formatted === null) {
              return;
            }
            setText(formatted);
            onChange(displayToIso(formatted) ?? '');
          }}
        />
        {text !== '' && (
          <button
            type="button"
            className="secondary-button"
            onClick={() => {
              setText('');
              onChange('');
            }}
          >
            Quitar fecha
          </button>
        )}
      </div>
    </Field>
  );
}
