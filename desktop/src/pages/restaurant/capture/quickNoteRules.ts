import {
  ingredientQuantity,
  withIngredient,
  withoutIngredientChange,
} from '../../../components/catalog/customizationRules';
import { quickNoteEffect } from '../../../services/catalog';
import type {
  CategoryQuickNote,
  Customization,
  ProductIngredient,
  QuickNoteEffect,
  QuickNoteSelection,
} from '../../../services/catalog';
import { joinNotes, splitNotes } from './captureRules';
import { fromThousandths, toThousandths, unit } from './quantities';

// Indicaciones rápidas en la captura (etapa 8). Una indicación con efecto no es un segundo
// mecanismo: al tocarla se aplica la misma selección estructurada que el personalizador
// (customization.ingredients) y se guarda su identificador en customization.quick_notes; su texto
// sigue en notes como en la etapa 7.1. Go valida que ambas cosas coincidan y calcula el precio
// solo con los ingredientes. Aquí solo se decide qué botón mostrar y qué selección enviar: las
// cantidades son porciones (no dinero) en milésimas enteras, igual que el botón «Extra».

// Indicación ofrecida al personalizar un producto.
export interface QuickNoteOption {
  id: string;
  text: string;
  effect: QuickNoteEffect;
  ingredientID: string | null;
  // Cantidad del ingrediente que fija el efecto (cadena decimal); nula sin efecto.
  quantity: string | null;
  // El efecto deja el ingrediente en lo incluido («Con cebolla» en un producto que ya la lleva):
  // se aplica como la configuración original, sin una cantidad explícita.
  original: boolean;
}

// Cantidad que fija un efecto en la regla del producto, o null si no aplica. Misma regla que Go
// (catalog.QuickNoteTarget) y que los botones «Sin» y «Extra» de los ingredientes:
// - omit: el producto incluye el ingrediente y es removible → 0;
// - normal: lo incluye → lo incluido; si no, agregable con máximo de al menos 1 → 1;
// - extra: agregable con máximo mayor que lo incluido → min(incluido + 1, máximo).
export function quickNoteQuantity(
  effect: QuickNoteEffect,
  rule: ProductIngredient,
): { quantity: string; original: boolean } | null {
  const included = toThousandths(rule.included_quantity);
  const maximum = toThousandths(rule.max_quantity);
  if (effect === 'omit') {
    return included > 0 && rule.removable ? { quantity: '0', original: false } : null;
  }
  if (effect === 'normal') {
    if (included > 0) {
      return { quantity: fromThousandths(included), original: true };
    }
    return rule.addable && maximum >= unit ? { quantity: '1', original: false } : null;
  }
  if (effect === 'extra') {
    if (!rule.addable || maximum <= included) {
      return null;
    }
    return { quantity: fromThousandths(Math.min(included + unit, maximum)), original: false };
  }
  return null;
}

// Indicaciones de la categoría que se muestran para un producto: todas las solo de instrucción
// (como hoy) y las con efecto cuyo ingrediente está en las reglas del producto y es aplicable.
// Una indicación con efecto sobre la cebolla no aparece en un producto sin cebolla.
export function applicableQuickNotes(
  notes: CategoryQuickNote[],
  rules: ProductIngredient[],
): QuickNoteOption[] {
  const options: QuickNoteOption[] = [];
  for (const note of notes) {
    const effect = quickNoteEffect(note);
    if (effect === 'none') {
      options.push({
        id: note.id,
        text: note.text,
        effect,
        ingredientID: null,
        quantity: null,
        original: false,
      });
      continue;
    }
    const rule = rules.find((entry) => entry.ingredient_id === note.ingredient_id);
    const target = rule ? quickNoteQuantity(effect, rule) : null;
    if (!rule || !target) {
      continue;
    }
    options.push({
      id: note.id,
      text: note.text,
      effect,
      ingredientID: rule.ingredient_id,
      quantity: target.quantity,
      original: target.original,
    });
  }
  return options;
}

function selections(value: Customization): QuickNoteSelection[] {
  return value.quick_notes ?? [];
}

// Sustituye las indicaciones elegidas; sin ninguna, se omite el campo para que la partida quede
// idéntica a una de la etapa 7.1.
function withSelections(value: Customization, entries: QuickNoteSelection[]): Customization {
  const next: Customization = { ...value };
  if (entries.length > 0) {
    next.quick_notes = entries;
  } else {
    delete next.quick_notes;
  }
  return next;
}

// Una indicación solo de instrucción está elegida si su texto está en las indicaciones de la
// partida (como en la etapa 7.1, también en partidas anteriores sin identificador); una con
// efecto, solo si su identificador está elegido.
export function quickNoteSelected(value: Customization, option: QuickNoteOption): boolean {
  if (option.effect === 'none') {
    return splitNotes(value.notes).includes(option.text);
  }
  return selections(value).some((entry) => entry.note_id === option.id);
}

// Quita un texto de las indicaciones de la partida y cualquier indicación elegida con ese texto o
// identificador, sin tocar ingredientes.
function withoutEntry(value: Customization, option: Pick<QuickNoteOption, 'id' | 'text'>) {
  const notes = joinNotes(splitNotes(value.notes).filter((entry) => entry !== option.text));
  return withSelections(
    { ...value, notes },
    selections(value).filter((entry) => entry.note_id !== option.id && entry.text !== option.text),
  );
}

// Quitar un texto escrito o un atajo: también deja de estar elegida la indicación con ese texto.
export function withoutNoteText(value: Customization, text: string): Customization {
  return withoutEntry(value, { id: '', text });
}

// Desmarcar una indicación: se quitan su identificador y su texto y, si tiene efecto, el
// ingrediente vuelve a su configuración original.
export function withoutQuickNote(value: Customization, option: QuickNoteOption): Customization {
  const next = withoutEntry(value, option);
  if (option.effect !== 'none' && option.ingredientID) {
    return withoutIngredientChange(next, option.ingredientID);
  }
  return next;
}

// Marcar una indicación: con efecto, quita la otra indicación con efecto sobre el mismo
// ingrediente (son excluyentes: un solo efecto por ingrediente) y aplica la misma selección del
// personalizador; después agrega su texto (sin repetirlo) y su identificador.
export function withQuickNote(
  value: Customization,
  option: QuickNoteOption,
  options: QuickNoteOption[],
): Customization {
  let next = value;
  if (option.effect !== 'none' && option.ingredientID) {
    for (const other of options) {
      if (
        other.id !== option.id &&
        other.effect !== 'none' &&
        other.ingredientID === option.ingredientID &&
        quickNoteSelected(next, other)
      ) {
        next = withoutEntry(next, other);
      }
    }
    next = option.original
      ? withoutIngredientChange(next, option.ingredientID)
      : withIngredient(next, option.ingredientID, option.quantity ?? '0');
  }
  const current = splitNotes(next.notes);
  if (!current.includes(option.text)) {
    next = { ...next, notes: joinNotes([...current, option.text]) };
  }
  const chosen = selections(next).filter((entry) => entry.note_id !== option.id);
  return withSelections(next, [...chosen, { note_id: option.id, text: option.text }]);
}

export function toggleQuickNote(
  value: Customization,
  option: QuickNoteOption,
  options: QuickNoteOption[],
): Customization {
  return quickNoteSelected(value, option)
    ? withoutQuickNote(value, option)
    : withQuickNote(value, option, options);
}

// Cambiar en el personalizador el ingrediente de una indicación con efecto la quita (con su
// texto): la indicación ya no describiría la selección. Las demás indicaciones no cambian.
export function reconcileQuickNotes(
  value: Customization,
  options: QuickNoteOption[],
  rules: ProductIngredient[],
): Customization {
  let next = value;
  for (const option of options) {
    if (option.effect === 'none' || !option.ingredientID || !quickNoteSelected(next, option)) {
      continue;
    }
    const rule = rules.find((entry) => entry.ingredient_id === option.ingredientID);
    if (!rule) {
      continue;
    }
    const current = toThousandths(ingredientQuantity(next, rule));
    if (current !== toThousandths(option.quantity)) {
      next = withoutEntry(next, option);
    }
  }
  return next;
}
