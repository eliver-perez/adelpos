import { useEffect, useRef, useState } from 'react';
import type { KeyboardEvent, PointerEvent } from 'react';
import { Icon, IconLabel } from '../../../components/Icon';
import type { CatalogKind, TemplateBlock } from '../../../services/documents';
import { blockLabel, blockSummary, blockTitle, catalogBlock } from './blockLabels';
import { isRemovable } from './draft';
import type { RemovedBlock } from './useDesignerDraft';

// Estructura de la plantilla: la lista ordenada de bloques. Cada fila elige su bloque; el orden se
// cambia con el asa (arrastrar con el dedo o el ratón), con los botones «Subir» y «Bajar» de la fila
// elegida o con el teclado (flechas para moverse entre filas; Alt + flecha para mover el bloque).
// Quitar un bloque se puede deshacer. El aviso fiscal es obligatorio: se marca y no se quita.

// Posición de inserción (0…n) para un punto vertical dentro de la lista: antes de la primera fila
// cuyo centro queda por debajo del punto. También la usa el arrastre desde el panel de elementos.
export function dropIndexAt(list: HTMLElement, clientY: number) {
  const rows = Array.from(list.querySelectorAll<HTMLElement>('[data-structure-row]'));
  for (let index = 0; index < rows.length; index++) {
    const row = rows[index];
    if (!row) {
      continue;
    }
    const box = row.getBoundingClientRect();
    if (clientY < box.top + box.height / 2) {
      return index;
    }
  }
  return rows.length;
}

// Índice final de un bloque movido de «from» a la posición de inserción «insertion».
export function targetIndex(from: number, insertion: number) {
  return insertion > from ? insertion - 1 : insertion;
}

export interface BlockMarks {
  // Bloques visibles que no imprimieron nada en la vista previa, con el motivo de Go.
  empty: Map<string, string>;
  // Bloque señalado por Go al validar la plantilla.
  invalidID: string;
}

export function StructureList({
  kind,
  blocks,
  selectedID,
  readOnly,
  marks,
  removed,
  externalInsertion,
  listRef,
  onSelect,
  onMove,
  onRemove,
  onUndoRemove,
}: {
  kind: CatalogKind;
  blocks: TemplateBlock[];
  selectedID: string | null;
  readOnly: boolean;
  marks: BlockMarks;
  removed: RemovedBlock | null;
  // Posición donde caería un elemento arrastrado desde el panel de elementos.
  externalInsertion: number | null;
  listRef: React.RefObject<HTMLOListElement | null>;
  onSelect: (id: string) => void;
  onMove: (from: number, to: number) => void;
  onRemove: (id: string) => void;
  onUndoRemove: () => void;
}) {
  const [drag, setDrag] = useState<{ from: number; insertion: number } | null>(null);
  const [focusID, setFocusID] = useState<string | null>(null);
  const rowButtons = useRef(new Map<string, HTMLButtonElement>());

  // Después de mover con el teclado o los botones, el foco sigue al bloque movido.
  useEffect(() => {
    if (!focusID) {
      return;
    }
    rowButtons.current.get(focusID)?.focus();
    setFocusID(null);
  }, [focusID, blocks]);

  function move(index: number, to: number, id: string) {
    if (to < 0 || to >= blocks.length) {
      return;
    }
    onMove(index, to);
    setFocusID(id);
  }

  function handleKeyDown(event: KeyboardEvent<HTMLButtonElement>, index: number, id: string) {
    const step = event.key === 'ArrowUp' ? -1 : event.key === 'ArrowDown' ? 1 : 0;
    if (step === 0) {
      return;
    }
    event.preventDefault();
    if ((event.altKey || event.ctrlKey || event.metaKey) && !readOnly) {
      move(index, index + step, id);
      return;
    }
    const next = blocks[index + step];
    if (next) {
      rowButtons.current.get(next.id)?.focus();
    }
  }

  function startDrag(event: PointerEvent<HTMLSpanElement>, index: number) {
    if (readOnly) {
      return;
    }
    event.preventDefault();
    event.currentTarget.setPointerCapture(event.pointerId);
    setDrag({ from: index, insertion: index });
  }

  function updateDrag(event: PointerEvent<HTMLSpanElement>) {
    if (!drag || !listRef.current) {
      return;
    }
    const insertion = dropIndexAt(listRef.current, event.clientY);
    if (insertion !== drag.insertion) {
      setDrag({ ...drag, insertion });
    }
  }

  function finishDrag() {
    if (!drag) {
      return;
    }
    const to = targetIndex(drag.from, drag.insertion);
    const moved = blocks[drag.from];
    setDrag(null);
    if (moved && to !== drag.from) {
      onMove(drag.from, to);
      onSelect(moved.id);
    }
  }

  const insertion = drag ? drag.insertion : externalInsertion;
  const removedName = removed ? blockLabel(kind, removed.block) : '';

  return (
    <div className="designer-structure">
      {removed && !readOnly && (
        <div className="designer-undo" role="status">
          <span>Se quitó «{removedName}».</span>
          <button type="button" className="secondary-button" onClick={onUndoRemove}>
            Deshacer
          </button>
        </div>
      )}
      <ol
        ref={listRef}
        className={`designer-structure-list ${externalInsertion !== null ? 'designer-drop-target' : ''}`}
        aria-label="Estructura de la plantilla"
        data-structure-list
      >
        {blocks.map((block, index) => {
          const definition = catalogBlock(kind, block.type);
          const selected = block.id === selectedID;
          const empty = marks.empty.get(block.id);
          const invalid = marks.invalidID === block.id;
          const removable = isRemovable(definition) && !readOnly;
          const summary = blockSummary(kind, block);
          const label = blockLabel(kind, block);
          return (
            <li
              key={block.id}
              data-structure-row
              className={[
                'designer-row',
                selected ? 'designer-row-selected' : '',
                invalid ? 'designer-row-invalid' : '',
                drag?.from === index ? 'designer-row-dragging' : '',
                insertion === index ? 'designer-row-drop-before' : '',
                insertion === blocks.length && index === blocks.length - 1
                  ? 'designer-row-drop-after'
                  : '',
              ]
                .filter(Boolean)
                .join(' ')}
            >
              {!readOnly && (
                <span
                  className="designer-handle"
                  role="presentation"
                  title="Arrastrar para cambiar el orden"
                  onPointerDown={(event) => startDrag(event, index)}
                  onPointerMove={updateDrag}
                  onPointerUp={finishDrag}
                  onPointerCancel={() => setDrag(null)}
                >
                  <Icon name="move" />
                </span>
              )}
              <button
                type="button"
                ref={(element) => {
                  if (element) {
                    rowButtons.current.set(block.id, element);
                  } else {
                    rowButtons.current.delete(block.id);
                  }
                }}
                className="designer-row-main"
                aria-pressed={selected}
                aria-label={`${index + 1}. ${label}`}
                aria-keyshortcuts={readOnly ? undefined : 'Alt+ArrowUp Alt+ArrowDown'}
                onClick={() => onSelect(block.id)}
                onKeyDown={(event) => handleKeyDown(event, index, block.id)}
              >
                <span className="designer-row-title">
                  {blockTitle(kind, block)}
                  {definition?.required && <span className="designer-badge">Obligatorio</span>}
                  {definition?.protected && <span className="designer-badge">Protegido</span>}
                  {!block.visible && <span className="designer-badge">Oculto</span>}
                </span>
                {summary && <span className="designer-row-summary">{summary}</span>}
                {invalid && (
                  <span className="designer-row-flag designer-row-flag-error">
                    Revisa este bloque
                  </span>
                )}
                {empty && block.visible && (
                  <span className="designer-row-flag">No imprime nada con estos datos</span>
                )}
              </button>
              {!readOnly && selected && (
                <span
                  className="designer-row-actions"
                  role="group"
                  aria-label={`Acciones de ${label}`}
                >
                  <button
                    type="button"
                    className="secondary-button"
                    aria-label={`Subir ${label}`}
                    disabled={index === 0}
                    onClick={() => move(index, index - 1, block.id)}
                  >
                    <IconLabel icon="arrow">Subir</IconLabel>
                  </button>
                  <button
                    type="button"
                    className="secondary-button designer-down"
                    aria-label={`Bajar ${label}`}
                    disabled={index === blocks.length - 1}
                    onClick={() => move(index, index + 1, block.id)}
                  >
                    <IconLabel icon="arrow">Bajar</IconLabel>
                  </button>
                  <button
                    type="button"
                    className="danger-button"
                    aria-label={
                      removable ? `Quitar ${label}` : `${label}: obligatorio, no se puede quitar`
                    }
                    disabled={!removable}
                    onClick={() => onRemove(block.id)}
                  >
                    <IconLabel icon="trash">Quitar</IconLabel>
                  </button>
                </span>
              )}
            </li>
          );
        })}
      </ol>
      {!readOnly && (
        <p className="muted text-xs">
          Arrastra el asa o usa Subir y Bajar para cambiar el orden. Con el teclado: flechas para
          moverte y Alt + flecha para mover el bloque.
        </p>
      )}
    </div>
  );
}
