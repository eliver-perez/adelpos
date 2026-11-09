import type { PreviewLine } from '../../../services/catalog';
import type { DraftItem } from '../../../services/restaurant';
import { isPlainLine, sameProductAndVariant } from './captureRules';

// Cambios locales sobre el borrador de una cuenta. Cada toque se registra como una operación y
// el borrador visible se obtiene aplicándolas, en orden, sobre el último borrador confirmado por
// Go. Así, si otra terminal cambia la cuenta, los mismos toques pueden volver a aplicarse sobre
// el borrador actual («conservar mis cambios») sin sobrescribir lo que hizo la otra terminal.
export type DraftOperation =
  | { kind: 'add'; item: DraftItem }
  // `fallback` es la partida que se vuelve a crear si, al aplicarse, la original ya no existe
  // en el borrador (por ejemplo, porque se envió a cocina mientras se tocaba «+»).
  | { kind: 'quantity'; id: string; delta: number; fallback: DraftItem }
  | { kind: 'replace'; id: string; item: DraftItem }
  | { kind: 'remove'; id: string };

function applyOperation(items: DraftItem[], operation: DraftOperation): DraftItem[] {
  switch (operation.kind) {
    case 'add': {
      if (items.some((item) => item.id === operation.item.id)) {
        return items;
      }
      return [...items, operation.item];
    }
    case 'quantity': {
      const target = items.find((item) => item.id === operation.id);
      if (!target) {
        if (operation.delta <= 0 || items.some((item) => item.id === operation.fallback.id)) {
          return items;
        }
        return [...items, { ...operation.fallback, quantity: operation.delta }];
      }
      const quantity = target.quantity + operation.delta;
      if (quantity <= 0) {
        return items.filter((item) => item.id !== operation.id);
      }
      return items.map((item) => (item.id === operation.id ? { ...item, quantity } : item));
    }
    case 'replace': {
      if (!items.some((item) => item.id === operation.id)) {
        return [...items, { ...operation.item, id: operation.id }];
      }
      return items.map((item) =>
        item.id === operation.id ? { ...operation.item, id: operation.id } : item,
      );
    }
    case 'remove':
      return items.filter((item) => item.id !== operation.id);
  }
}

export function applyOperations(base: DraftItem[], operations: DraftOperation[]): DraftItem[] {
  return operations.reduce(applyOperation, base);
}

// Agregar un producto: si ya existe una partida nueva idéntica y sencilla (mismo producto y
// presentación, sin personalización ni indicaciones) se suma a su cantidad; en otro caso se crea
// otra partida. Una partida personalizada nunca se combina con otra.
export function addLineOperation(
  current: DraftItem[],
  line: PreviewLine,
  newID: () => string,
): DraftOperation {
  if (isPlainLine(line)) {
    const existing = current.find((item) => isPlainLine(item) && sameProductAndVariant(item, line));
    if (existing) {
      return {
        kind: 'quantity',
        id: existing.id,
        delta: line.quantity,
        fallback: { ...existing, id: newID() },
      };
    }
  }
  return { kind: 'add', item: { ...line, id: newID() } };
}

export function quantityOperation(
  item: DraftItem,
  delta: number,
  newID: () => string,
): DraftOperation {
  return { kind: 'quantity', id: item.id, delta, fallback: { ...item, id: newID() } };
}
