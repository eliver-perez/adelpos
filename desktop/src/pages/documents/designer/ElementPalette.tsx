import { useRef } from 'react';
import type { PointerEvent } from 'react';
import { Icon } from '../../../components/Icon';
import type {
  CatalogBlock,
  CatalogKind,
  FieldShortcut,
  TemplateBlock,
} from '../../../services/documents';
import { paletteGroupOf, paletteGroups } from './blockLabels';
import { canAdd } from './draft';

// Panel de elementos: solo los bloques que el catálogo de Go declara compatibles con el tipo de
// documento, agrupados, con su nombre y una descripción breve. Tocar un elemento lo agrega después
// del bloque elegido; también se puede arrastrar hasta la estructura. Los atajos de «Campo de
// datos» (Fecha, Hora, Folio, Mesa…) aparecen como elementos propios ya configurados.

export interface PaletteItem {
  definition: CatalogBlock;
  shortcut?: FieldShortcut;
  name: string;
  description: string;
}

// Distancia mínima para distinguir un arrastre de un toque.
const dragThreshold = 8;

function itemsOf(kind: CatalogKind): PaletteItem[] {
  const items: PaletteItem[] = [];
  for (const definition of kind.blocks) {
    items.push({ definition, name: definition.name, description: definition.description });
    if (definition.type === 'field') {
      for (const shortcut of kind.shortcuts) {
        items.push({
          definition,
          shortcut,
          name: shortcut.name,
          description: 'Campo de datos ya configurado; puedes cambiar su etiqueta.',
        });
      }
    }
  }
  return items;
}

export function ElementPalette({
  kind,
  blocks,
  limit,
  onAdd,
  onDragStart,
}: {
  kind: CatalogKind;
  blocks: TemplateBlock[];
  limit: number;
  onAdd: (item: PaletteItem) => void;
  // Comienza un arrastre hacia la estructura (el editor sigue el puntero).
  onDragStart: (item: PaletteItem, x: number, y: number) => void;
}) {
  const press = useRef<{ x: number; y: number; item: PaletteItem; dragged: boolean } | null>(null);
  const items = itemsOf(kind);
  const full = blocks.length >= limit;

  function pointerDown(event: PointerEvent<HTMLButtonElement>, item: PaletteItem) {
    press.current = { x: event.clientX, y: event.clientY, item, dragged: false };
  }

  function pointerMove(event: PointerEvent<HTMLButtonElement>) {
    const current = press.current;
    if (!current || current.dragged || event.buttons === 0) {
      return;
    }
    const distance = Math.hypot(event.clientX - current.x, event.clientY - current.y);
    if (distance >= dragThreshold) {
      current.dragged = true;
      onDragStart(current.item, event.clientX, event.clientY);
    }
  }

  function click(item: PaletteItem) {
    // Un arrastre que terminó sobre el mismo botón no cuenta como toque.
    const dragged = press.current?.dragged;
    press.current = null;
    if (!dragged) {
      onAdd(item);
    }
  }

  return (
    <section className="designer-palette" aria-label="Elementos disponibles">
      {full && (
        <p className="muted text-xs">
          La plantilla llegó al máximo de {limit} bloques: quita alguno para agregar otro.
        </p>
      )}
      {paletteGroups.map((group) => {
        const entries = items.filter((item) => paletteGroupOf(item.definition) === group.id);
        if (entries.length === 0) {
          return null;
        }
        return (
          <div className="designer-palette-group" key={group.id}>
            <h3>{group.label}</h3>
            <ul>
              {entries.map((item) => {
                const available = canAdd(item.definition, blocks, limit);
                const key = `${item.definition.type}-${item.shortcut?.name ?? ''}`
                  .toLowerCase()
                  .replace(/[^a-z0-9-]+/g, '-');
                return (
                  <li key={key}>
                    <button
                      type="button"
                      className="designer-palette-item"
                      disabled={!available}
                      aria-label={`Agregar ${item.name}`}
                      aria-describedby={`palette-${key}`}
                      onPointerDown={(event) => pointerDown(event, item)}
                      onPointerMove={pointerMove}
                      onClick={() => click(item)}
                    >
                      <Icon name="plus" size={18} />
                      <span className="designer-palette-text">
                        <strong>{item.name}</strong>
                        <span id={`palette-${key}`} className="muted">
                          {available ? item.description : 'Ya está en la plantilla.'}
                        </span>
                      </span>
                    </button>
                  </li>
                );
              })}
            </ul>
          </div>
        );
      })}
    </section>
  );
}
