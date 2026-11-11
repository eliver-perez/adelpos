// @vitest-environment jsdom

import { describe, expect, it } from 'vitest';
import { block, kitchenKind, receiptKind, receiptLayout } from './designerTestData';
import { paletteGroupOf } from './blockLabels';
import {
  canAdd,
  createBlock,
  insertAfter,
  insertAt,
  moveBlock,
  newBlockID,
  newFixedText,
} from './draft';
import { dropIndexAt, targetIndex } from './StructureList';

// Etapa 9B, fase C: modelo puro del borrador del editor (agregar, mover, límites del catálogo).

function definitionOf(type: string) {
  const found = receiptKind.blocks.find((item) => item.type === type);
  if (!found) {
    throw new Error(`sin ${type}`);
  }
  return found;
}

describe('borrador de la plantilla', () => {
  it('genera identificadores únicos con la forma que exige Go', () => {
    const blocks = [block('fixed-text-1', 'fixed_text')];
    const id = newBlockID('fixed_text', blocks);
    expect(id).toBe('fixed-text-2');
    expect(id).toMatch(/^[a-z0-9-]{1,40}$/);
  });

  it('crea bloques con los valores predeterminados del catálogo', () => {
    const text = createBlock(definitionOf('fixed_text'), receiptKind, []);
    expect(text.options).toEqual({ text: newFixedText });
    const field = createBlock(definitionOf('field'), receiptKind, []);
    // Un campo de datos necesita al menos un dato: se propone el primero del tipo.
    expect(field.options.items).toEqual([{ key: 'date', label: 'Fecha:' }]);
    expect(field.options.layout).toBe('inline');
    const shortcut = createBlock(definitionOf('field'), receiptKind, [], receiptKind.shortcuts[1]);
    expect(shortcut.options.items).toEqual([{ key: 'cashier', label: 'Atendió:' }]);
  });

  it('agrega después del bloque elegido o al final', () => {
    const blocks = receiptLayout().blocks;
    const added = block('new', 'divider');
    expect(insertAfter(blocks, added, 'greeting').map((item) => item.id)).toEqual([
      'name',
      'greeting',
      'new',
      'products',
      'total',
      'fiscal',
    ]);
    expect(insertAfter(blocks, added, null).at(-1)?.id).toBe('new');
    expect(insertAt(blocks, added, 0)[0]?.id).toBe('new');
  });

  it('mueve bloques y calcula la posición al soltar', () => {
    const blocks = receiptLayout().blocks;
    expect(moveBlock(blocks, 0, 2).map((item) => item.id)).toEqual([
      'greeting',
      'products',
      'name',
      'total',
      'fiscal',
    ]);
    expect(moveBlock(blocks, 0, 9)).toBe(blocks);
    // Soltar en la posición 3 un bloque que estaba en la 0 lo deja en la 2.
    expect(targetIndex(0, 3)).toBe(2);
    expect(targetIndex(4, 1)).toBe(1);
  });

  it('respeta el máximo y el bloque obligatorio que declara el catálogo', () => {
    const blocks = receiptLayout().blocks;
    expect(canAdd(definitionOf('products'), blocks, 60)).toBe(false);
    expect(canAdd(definitionOf('fiscal_notice'), blocks, 60)).toBe(false);
    expect(canAdd(definitionOf('divider'), blocks, 60)).toBe(true);
    expect(canAdd(definitionOf('divider'), blocks, blocks.length)).toBe(false);
  });

  it('agrupa los elementos del panel', () => {
    const groups = kitchenKind.blocks.map((item) => [item.type, paletteGroupOf(item)]);
    expect(Object.fromEntries(groups)).toMatchObject({
      logo: 'header',
      field: 'data',
      kitchen_items: 'content',
      fixed_text: 'format',
    });
  });

  it('calcula la posición de inserción por la mitad de cada fila', () => {
    const list = document.createElement('ol');
    for (let index = 0; index < 3; index++) {
      const row = document.createElement('li');
      row.dataset.structureRow = '';
      row.getBoundingClientRect = () =>
        ({ top: index * 50, height: 50, bottom: index * 50 + 50 }) as DOMRect;
      list.appendChild(row);
    }
    expect(dropIndexAt(list, 10)).toBe(0);
    expect(dropIndexAt(list, 60)).toBe(1);
    expect(dropIndexAt(list, 140)).toBe(3);
  });
});
