import type {
  CatalogBlock,
  CatalogKind,
  FieldItem,
  FieldShortcut,
  OptionSpec,
  TemplateBlock,
  TemplateLayout,
} from '../../../services/documents';

// Modelo del borrador del editor de plantillas (etapa 9B, fase C). Son funciones puras sobre la
// lista ordenada de bloques: agregar, quitar, mover y cambiar propiedades. No validan reglas de
// impresión: Go valida la plantilla en cada vista previa y al guardar, y su respuesta señala el
// bloque con problemas. Aquí solo se respetan los límites que el catálogo de Go declara para no
// ofrecer acciones que Go rechazaría con certeza (máximo de apariciones, bloque obligatorio).

const idPattern = /^[a-z0-9-]{1,40}$/;

// Texto inicial de un «Texto fijo» recién agregado: Go exige texto, y un bloque vacío haría fallar
// la vista previa antes de que la persona escriba.
export const newFixedText = 'Texto nuevo';

// Identificador nuevo y único con la forma que exige Go (^[a-z0-9-]{1,40}$), a partir del tipo.
export function newBlockID(type: string, blocks: TemplateBlock[]) {
  const used = new Set(blocks.map((block) => block.id));
  const base =
    type
      .replace(/_/g, '-')
      .replace(/[^a-z0-9-]/g, '')
      .slice(0, 32) || 'bloque';
  let counter = 1;
  let candidate = `${base}-${counter}`;
  while (used.has(candidate) || !idPattern.test(candidate)) {
    counter += 1;
    candidate = `${base}-${counter}`;
  }
  return candidate;
}

function cloneValue<T>(value: T): T {
  return JSON.parse(JSON.stringify(value)) as T;
}

// Opciones iniciales de un bloque: los valores predeterminados que declara el catálogo de Go.
export function defaultOptions(specs: OptionSpec[], kind: CatalogKind) {
  const options: Record<string, unknown> = {};
  for (const spec of specs) {
    options[spec.key] = cloneValue(spec.default);
  }
  const items = specs.find((spec) => spec.key === 'items' && spec.type === 'field_items');
  if (items && Array.isArray(options.items) && options.items.length === 0) {
    // Un campo de datos necesita al menos un dato: se propone el primero del tipo de documento.
    const first = kind.field_keys[0];
    if (first) {
      options.items = [{ key: first.key, label: first.default_label }];
    }
  }
  const text = specs.find((spec) => spec.key === 'text' && spec.type === 'text' && spec.required);
  if (text && options.text === '') {
    options.text = newFixedText;
  }
  return options;
}

// Bloque nuevo con las propiedades predeterminadas de Go (visible, izquierda, normal, sin
// espaciado) y sus opciones; un atajo de «Campo de datos» trae sus datos ya elegidos.
export function createBlock(
  definition: CatalogBlock,
  kind: CatalogKind,
  blocks: TemplateBlock[],
  shortcut?: FieldShortcut,
): TemplateBlock {
  const options = shortcut
    ? (cloneValue(shortcut.options) as unknown as Record<string, unknown>)
    : defaultOptions(definition.options, kind);
  return {
    id: newBlockID(definition.type, blocks),
    type: definition.type,
    visible: true,
    align: 'left',
    size: 'normal',
    bold: false,
    uppercase: false,
    spacing_before: 0,
    spacing_after: 0,
    options,
  };
}

// Inserta después del bloque seleccionado o, sin selección, al final.
export function insertAfter(
  blocks: TemplateBlock[],
  block: TemplateBlock,
  selectedID: string | null,
): TemplateBlock[] {
  const index = selectedID ? blocks.findIndex((item) => item.id === selectedID) : -1;
  if (index < 0) {
    return [...blocks, block];
  }
  return [...blocks.slice(0, index + 1), block, ...blocks.slice(index + 1)];
}

// Inserta en una posición concreta (arrastrar desde el panel de elementos).
export function insertAt(blocks: TemplateBlock[], block: TemplateBlock, index: number) {
  const position = Math.max(0, Math.min(index, blocks.length));
  return [...blocks.slice(0, position), block, ...blocks.slice(position)];
}

// Mueve un bloque de una posición a otra; fuera de rango devuelve la misma lista.
export function moveBlock(blocks: TemplateBlock[], from: number, to: number): TemplateBlock[] {
  if (from < 0 || from >= blocks.length || to < 0 || to >= blocks.length || from === to) {
    return blocks;
  }
  const next = [...blocks];
  const [moved] = next.splice(from, 1);
  if (!moved) {
    return blocks;
  }
  next.splice(to, 0, moved);
  return next;
}

export function removeBlock(blocks: TemplateBlock[], id: string) {
  return blocks.filter((block) => block.id !== id);
}

export function updateBlock(
  blocks: TemplateBlock[],
  id: string,
  change: Partial<Omit<TemplateBlock, 'id' | 'type'>>,
): TemplateBlock[] {
  return blocks.map((block) => (block.id === id ? { ...block, ...change } : block));
}

export function updateBlockOption(
  blocks: TemplateBlock[],
  id: string,
  key: string,
  value: unknown,
): TemplateBlock[] {
  return blocks.map((block) =>
    block.id === id ? { ...block, options: { ...block.options, [key]: value } } : block,
  );
}

// Cuántas veces aparece un tipo de bloque en la plantilla.
export function countOf(blocks: TemplateBlock[], type: string) {
  return blocks.filter((block) => block.type === type).length;
}

// Un tipo de bloque ya no se puede agregar si alcanzó su máximo o si es obligatorio (exactamente
// uno, como el aviso fiscal) y ya existe. El catálogo de Go declara ambos datos.
export function canAdd(definition: CatalogBlock, blocks: TemplateBlock[], limit: number) {
  if (blocks.length >= limit) {
    return false;
  }
  const count = countOf(blocks, definition.type);
  if (definition.required && count >= 1) {
    return false;
  }
  if (definition.max && definition.max > 0 && count >= definition.max) {
    return false;
  }
  return true;
}

// Un bloque obligatorio (aviso fiscal) no se quita ni se oculta.
export function isRemovable(definition: CatalogBlock | undefined) {
  return !definition?.required;
}

export function sameLayout(left: TemplateLayout | null, right: TemplateLayout | null) {
  return JSON.stringify(left) === JSON.stringify(right);
}

export function cloneLayout(layout: TemplateLayout): TemplateLayout {
  return cloneValue(layout);
}

// Datos del «Campo de datos» con al menos un elemento, para el editor de datos.
export function fieldItems(block: TemplateBlock): FieldItem[] {
  const items = block.options.items;
  if (!Array.isArray(items)) {
    return [];
  }
  return items.filter(
    (item): item is FieldItem =>
      typeof item === 'object' && item !== null && 'key' in item && 'label' in item,
  );
}
