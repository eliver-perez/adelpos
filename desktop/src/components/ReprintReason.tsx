import { useState } from 'react';
import { Field } from './Forms';
import { IconLabel } from './Icon';

// Motivo de una reimpresión (etapa 9F, decisión D3): reimprimir un documento que ya salió en papel pide
// un motivo de 5 a 500 caracteres, que Go guarda en la auditoría y en el historial del trabajo.
// Reintentar uno que nunca salió no lo pide. Se muestra en línea, debajo del documento.
export const minimumReasonLength = 5;
export const maximumReasonLength = 500;

export function reasonValid(reason: string) {
  const trimmed = reason.trim();
  return trimmed.length >= minimumReasonLength && reason.length <= maximumReasonLength;
}

export function ReprintReason({
  subject,
  busy,
  onSubmit,
  onCancel,
}: {
  // Qué se reimprime, con «de» y su artículo («de la precuenta», «del corte parcial»).
  subject: string;
  busy: boolean;
  onSubmit: (reason: string) => void;
  onCancel: () => void;
}) {
  const [reason, setReason] = useState('');
  const valid = reasonValid(reason);
  return (
    <form
      className="reprint-reason"
      aria-label="Reimpresión con motivo"
      onSubmit={(event) => {
        event.preventDefault();
        if (valid && !busy) {
          onSubmit(reason.trim());
        }
      }}
    >
      <Field label="Motivo de la reimpresión">
        <input
          type="text"
          value={reason}
          maxLength={maximumReasonLength}
          autoFocus
          onChange={(event) => setReason(event.target.value)}
        />
      </Field>
      <p className="muted text-sm">
        {`El documento ya se envió a la impresora: escribe por qué hace falta otra copia ${subject} (${minimumReasonLength} a ${maximumReasonLength} caracteres).`}
      </p>
      <div className="button-row">
        <button type="submit" className="primary-button" disabled={!valid || busy}>
          <IconLabel icon="printer">Reimprimir</IconLabel>
        </button>
        <button type="button" className="secondary-button" onClick={onCancel}>
          Cancelar
        </button>
      </div>
    </form>
  );
}
