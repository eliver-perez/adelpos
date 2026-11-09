// Orden visual por posiciones (etapa 6.1 §2.2). Funciones puras compartidas por los editores;
// Go recalcula sort_order al guardar y es la autoridad del orden.

export interface Ordered {
  id: string;
  sort_order: number;
}

export interface PositionOption {
  id: string;
  name: string;
  // Aclaración breve junto al nombre (por ejemplo «inactivo»).
  note?: string;
}

// Reproduce el orden de Go (sort_order, nombre, identificador). Las listas del servidor ya
// llegan ordenadas por nombre e identificador, así que basta un ordenamiento estable por
// sort_order que conserve ese orden en los empates.
export function orderSiblings<T extends Ordered>(items: T[]): T[] {
  return items
    .map((item, index) => ({ item, index }))
    .sort((first, second) => {
      if (first.item.sort_order !== second.item.sort_order) {
        return first.item.sort_order - second.item.sort_order;
      }
      return first.index - second.index;
    })
    .map((entry) => entry.item);
}

// Posición actual de un registro: después de su hermano anterior, o «al inicio» si es el
// primero. Un registro nuevo, o que todavía no pertenece a la lista (por ejemplo un producto que
// cambia de categoría), queda por omisión después del último.
export function currentAfter(siblings: { id: string }[], recordID: string): string {
  const index = recordID ? siblings.findIndex((sibling) => sibling.id === recordID) : -1;
  if (index >= 0) {
    return siblings[index - 1]?.id ?? '';
  }
  const others = siblings.filter((sibling) => sibling.id !== recordID);
  return others[others.length - 1]?.id ?? '';
}

// Posición que se envía a Go: la elegida por el usuario o, si el registro es nuevo o se
// incorpora a otra lista, la predeterminada. undefined conserva el orden guardado sin tocar a
// los hermanos.
export function positionPayload(
  siblings: { id: string }[],
  recordID: string,
  chosen: string | undefined,
): { after: string } | undefined {
  if (chosen !== undefined) {
    return { after: chosen };
  }
  const member = recordID !== '' && siblings.some((sibling) => sibling.id === recordID);
  if (member) {
    return undefined;
  }
  return { after: currentAfter(siblings, recordID) };
}
