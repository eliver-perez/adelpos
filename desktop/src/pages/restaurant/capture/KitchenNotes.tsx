import { useState } from 'react';
import { joinNotes, splitNotes } from './captureRules';

// Límite de Go para las observaciones de preparación de una partida.
const maximumNotesLength = 500;

// Indicaciones para cocina como botones. El catálogo no tiene todavía indicaciones rápidas
// configuradas: los atajos son las indicaciones ya usadas en esta cuenta y «Escribir otra» abre
// un campo de texto (con el teclado virtual de la terminal). Se guardan en el mismo campo de
// observaciones de la partida, separadas por comas, sin cargo automático.
export function KitchenNotes({
  value,
  onChange,
  shortcuts,
}: {
  value: string;
  onChange: (value: string) => void;
  shortcuts: string[];
}) {
  const [writing, setWriting] = useState(false);
  const [draft, setDraft] = useState('');
  const current = splitNotes(value);
  const suggestions = shortcuts.filter((entry) => !current.includes(entry));
  const next = joinNotes([...current, draft.trim()]);
  const valid = draft.trim().length > 0 && next.length <= maximumNotesLength;

  function add() {
    if (!valid) return;
    onChange(next);
    setDraft('');
    setWriting(false);
  }

  return (
    <section className="capture-block" aria-label="Indicaciones para cocina">
      <header className="capture-block-header">
        <h3>Indicaciones para cocina</h3>
        <span className="capture-block-rule">Sin cargo</span>
      </header>
      <div className="capture-choices">
        {current.map((entry) => (
          <button
            type="button"
            key={`actual-${entry}`}
            className="capture-choice"
            aria-pressed="true"
            aria-label={`Quitar indicación ${entry}`}
            onClick={() => onChange(joinNotes(current.filter((note) => note !== entry)))}
          >
            <span>{entry}</span>
          </button>
        ))}
        {suggestions.map((entry) => (
          <button
            type="button"
            key={`atajo-${entry}`}
            className="capture-choice"
            aria-pressed="false"
            disabled={joinNotes([...current, entry]).length > maximumNotesLength}
            onClick={() => onChange(joinNotes([...current, entry]))}
          >
            <span>{entry}</span>
          </button>
        ))}
        {!writing && (
          <button
            type="button"
            className="capture-choice capture-choice-write"
            onClick={() => setWriting(true)}
          >
            <span>Escribir otra</span>
          </button>
        )}
      </div>
      {writing && (
        <div className="capture-note-writer">
          <input
            type="text"
            value={draft}
            maxLength={maximumNotesLength}
            autoFocus
            aria-label="Nueva indicación para cocina"
            placeholder="Por ejemplo: salsa aparte"
            onChange={(event) => setDraft(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === 'Enter') {
                event.preventDefault();
                add();
              }
            }}
          />
          <button type="button" className="primary-button" disabled={!valid} onClick={add}>
            Agregar indicación
          </button>
          <button
            type="button"
            className="secondary-button"
            onClick={() => {
              setDraft('');
              setWriting(false);
            }}
          >
            Cancelar
          </button>
        </div>
      )}
    </section>
  );
}

// Indicaciones ya usadas en la cuenta (borrador y partidas enviadas), como atajos locales.
export function accountNoteShortcuts(notes: string[]): string[] {
  const unique: string[] = [];
  for (const value of notes) {
    for (const entry of splitNotes(value)) {
      if (!unique.includes(entry)) {
        unique.push(entry);
      }
    }
  }
  return unique.slice(0, 12);
}
