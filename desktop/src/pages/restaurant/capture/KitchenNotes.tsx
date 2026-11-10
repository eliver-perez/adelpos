import { useState } from 'react';
import type { Customization } from '../../../services/catalog';
import { joinNotes, splitNotes } from './captureRules';
import { quickNoteSelected, toggleQuickNote, withoutNoteText } from './quickNoteRules';
import type { QuickNoteOption } from './quickNoteRules';

// Límite de Go para las observaciones de preparación de una partida.
const maximumNotesLength = 500;

// Indicaciones para cocina como botones. Primero las indicaciones rápidas de la categoría del
// producto (etapa 7.1), en su orden del catálogo, que se marcan y desmarcan en su lugar; después
// las demás indicaciones ya elegidas para la partida, los atajos de las ya usadas en esta cuenta
// (sin repetir ninguna) y «Escribir otra», que abre un campo de texto (con el teclado virtual de
// la terminal). Todas se guardan en el mismo campo de observaciones de la partida, separadas por
// comas, sin cargo automático, y llegan a la comanda igual.
//
// Etapa 8: quickNotes son solo las aplicables al producto. Una indicación con efecto aplica al
// tocarla la misma selección del personalizador (y quita la otra con efecto sobre el mismo
// ingrediente); hiddenTexts son las de la categoría que no aplican, que tampoco se ofrecen como
// atajo de texto.
export function KitchenNotes({
  value,
  onChange,
  shortcuts,
  quickNotes = [],
  hiddenTexts = [],
}: {
  value: Customization;
  onChange: (value: Customization) => void;
  shortcuts: string[];
  quickNotes?: QuickNoteOption[];
  hiddenTexts?: string[];
}) {
  const [writing, setWriting] = useState(false);
  const [draft, setDraft] = useState('');
  const current = splitNotes(value.notes);
  const quickTexts = quickNotes.map((note) => note.text);
  // Las indicaciones escritas o de otra partida que no son una indicación rápida elegida. El texto
  // de una indicación con efecto sin elegir (por ejemplo, escrito a mano) sigue aquí: es solo
  // texto y no cambia ingredientes.
  const others = current.filter(
    (entry) => !quickNotes.some((note) => note.text === entry && quickNoteSelected(value, note)),
  );
  const suggestions = shortcuts.filter(
    (entry) =>
      !current.includes(entry) && !quickTexts.includes(entry) && !hiddenTexts.includes(entry),
  );
  const next = joinNotes([...current, draft.trim()]);
  const valid = draft.trim().length > 0 && next.length <= maximumNotesLength;

  function add() {
    if (!valid) return;
    onChange({ ...value, notes: next });
    setDraft('');
    setWriting(false);
  }

  function fits(entry: string) {
    return current.includes(entry) || joinNotes([...current, entry]).length <= maximumNotesLength;
  }

  return (
    <section className="capture-block" aria-label="Indicaciones para cocina">
      <header className="capture-block-header">
        <h3>Indicaciones para cocina</h3>
        <span className="capture-block-rule">Sin cargo</span>
      </header>
      <div className="capture-choices">
        {quickNotes.map((note) => {
          const selected = quickNoteSelected(value, note);
          return (
            <button
              type="button"
              key={`rapida-${note.id}`}
              className="capture-choice"
              aria-pressed={selected}
              aria-label={selected ? `Quitar indicación ${note.text}` : undefined}
              data-effect={note.effect}
              disabled={!selected && !fits(note.text)}
              onClick={() => onChange(toggleQuickNote(value, note, quickNotes))}
            >
              <span>{note.text}</span>
            </button>
          );
        })}
        {others.map((entry) => (
          <button
            type="button"
            key={`actual-${entry}`}
            className="capture-choice"
            aria-pressed="true"
            aria-label={`Quitar indicación ${entry}`}
            onClick={() => onChange(withoutNoteText(value, entry))}
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
            disabled={!fits(entry)}
            onClick={() => onChange({ ...value, notes: joinNotes([...current, entry]) })}
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
