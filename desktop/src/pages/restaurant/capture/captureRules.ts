import { componentProductIDs, groupLimits } from '../../../components/catalog/customizationRules';
import type {
  Customization,
  Lookups,
  OptionGroup,
  PreviewLine,
  Product,
  ProductBreakdown,
} from '../../../services/catalog';
import type { DraftItem } from '../../../services/restaurant';
import { displayQuantity, toThousandths } from './quantities';

// Producto del listado de captura: documento del catálogo más la disponibilidad y el precio
// efectivo que calcula Go para la lista de la cuenta.
export interface CaptureProduct extends Product {
  available_now: boolean;
  availability_reason: string;
  effective_price?: string;
  price_origin?: string;
  pricing_error?: string;
}

// Límites del borrador que Go aplica al guardar y confirmar; la interfaz los anticipa para avisar
// antes de enviar una selección que sería rechazada.
export const maximumDraftLines = 20;
export const maximumDraftUnits = 1000;

function activeVariants(product: Product) {
  return product.variants.filter((variant) => variant.active);
}

// Un producto tiene elecciones obligatorias (etiqueta «Opciones»: el toque abre la
// personalización en lugar de agregarlo) cuando:
// - algún grupo exige al menos una porción (mínimo del producto o del grupo mayor que cero) y
//   ofrece más de una opción activa: guarnición, término, salsa…; lo incluido llega marcado;
// - un componente de combo es obligatorio (mínimo de una unidad) y admite más de un producto;
// - el precio del producto base no está disponible en la lista y hay presentaciones activas.
// Go exige que la configuración original de cada grupo respete sus límites, así que estos
// productos tienen siempre una selección válida de partida; la regla solo decide la
// presentación. Go valida de nuevo al cotizar y al confirmar.
export function requiresChoices(product: CaptureProduct, groups: OptionGroup[]): boolean {
  if (product.pricing_error && activeVariants(product).length > 0) {
    return true;
  }
  const groupChoice = product.groups.some((binding) => {
    const group = groups.find((entry) => entry.id === binding.group_id);
    if (!group) {
      return false;
    }
    const minimum = toThousandths(groupLimits(binding, group).minimum);
    return minimum > 0 && group.options.filter((option) => option.active).length > 1;
  });
  if (groupChoice) {
    return true;
  }
  return product.slots.some(
    (slot) => slot.min_quantity > 0 && componentProductIDs(slot).length > 1,
  );
}

// Disponible para un toque: activo ahora y con un precio que Go pudo determinar.
export function productSelectable(product: CaptureProduct): boolean {
  if (!product.available_now) {
    return false;
  }
  if (product.pricing_error && activeVariants(product).length === 0) {
    return false;
  }
  return true;
}

export function emptyCustomizationValue(value: Customization): boolean {
  return (
    value.ingredients.length === 0 &&
    value.groups.length === 0 &&
    value.components.length === 0 &&
    value.notes.trim() === '' &&
    (value.quick_notes ?? []).length === 0
  );
}

// Partida «sencilla»: sin personalización ni indicaciones. Solo estas se suman a una partida
// nueva idéntica; cualquier personalización crea otra partida.
export function isPlainLine(line: PreviewLine): boolean {
  return emptyCustomizationValue(line.customization);
}

export function sameProductAndVariant(first: PreviewLine, second: PreviewLine): boolean {
  return first.product_id === second.product_id && (first.variant_id ?? null) === second.variant_id;
}

// Igualdad de contenido de dos partidas (producto, presentación, cantidad y personalización).
export function sameDraftItem(first: DraftItem, second: DraftItem): boolean {
  return (
    first.id === second.id &&
    first.quantity === second.quantity &&
    sameProductAndVariant(first, second) &&
    JSON.stringify(first.customization) === JSON.stringify(second.customization)
  );
}

export function draftUnits(items: DraftItem[]): number {
  return items.reduce((total, item) => total + item.quantity, 0);
}

// Indicaciones para cocina: se guardan en el mismo campo de observaciones, separadas por comas.
export function splitNotes(value: string): string[] {
  return value
    .split(',')
    .map((entry) => entry.trim())
    .filter((entry) => entry.length > 0);
}

export function joinNotes(entries: string[]): string {
  return entries.join(', ');
}

// Ingredientes cuyo cambio ya describe una indicación con efecto que está en las indicaciones de
// la partida (etapa 8): el resumen la muestra una sola vez, como la comanda.
function coveredIngredients(
  notes: string,
  quickNotes: { text: string; effect?: string; ingredient_id?: string | null }[],
): Set<string> {
  const entries = splitNotes(notes);
  const covered = new Set<string>();
  for (const note of quickNotes) {
    if (
      note.effect &&
      note.effect !== 'none' &&
      note.ingredient_id &&
      entries.includes(note.text)
    ) {
      covered.add(note.ingredient_id);
    }
  }
  return covered;
}

function comparableName(text: string) {
  return text
    .normalize('NFD')
    .replace(/[\u0300-\u036f]/g, '')
    .toLowerCase()
    .replace(/\s+/g, ' ')
    .trim();
}

// Nombre de lo elegido en un grupo o componente (etapa 9D, §4.4): «Grupo: elección» solo si lo
// elegido no repite el nombre del grupo. Si lo repite («Papas a la francesa» en «Papas a la
// francesa», «Hamburguesa sencilla» en «Hamburguesa»), solo lo elegido.
export function choiceLabel(group: string, choice: string) {
  const name = comparableName(group);
  if (!name || ` ${comparableName(choice)} `.includes(` ${name} `)) {
    return choice;
  }
  return `${group}: ${choice}`;
}

// Resumen breve de una partida desde el desglose de Go (nombres y cantidades que devolvió el
// motor). Solo describe lo que cambió respecto de la configuración original.
export function describeBreakdown(breakdown: ProductBreakdown | null | undefined): string[] {
  if (!breakdown) {
    return [];
  }
  const parts: string[] = [];
  const covered = coveredIngredients(breakdown.notes ?? '', breakdown.quick_notes ?? []);
  for (const ingredient of breakdown.ingredients ?? []) {
    if (covered.has(ingredient.ingredient_id)) {
      continue;
    }
    const requested = toThousandths(ingredient.requested);
    const included = toThousandths(ingredient.included);
    if (requested === 0 && included > 0) {
      parts.push(`Sin ${ingredient.name}`);
    } else if (requested > included) {
      parts.push(`Extra ${ingredient.name}`);
    }
  }
  for (const group of breakdown.groups ?? []) {
    const requested = (group.options ?? []).filter((option) => toThousandths(option.requested) > 0);
    const changed = (group.options ?? []).some(
      (option) => toThousandths(option.requested) !== toThousandths(option.included),
    );
    if (!changed) {
      continue;
    }
    if (requested.length === 0) {
      parts.push(`Sin ${group.name}`);
      continue;
    }
    for (const option of requested) {
      const quantity = displayQuantity(option.requested);
      parts.push(quantity === '1' ? option.name : `${quantity} × ${option.name}`);
    }
  }
  for (const component of breakdown.components ?? []) {
    if (component.product && component.quantity > 0) {
      const nested = describeBreakdown(component.product);
      const label = choiceLabel(component.name, component.product.name);
      parts.push(nested.length > 0 ? `${label} (${nested.join(', ')})` : label);
    }
  }
  if (breakdown.notes) {
    parts.push(breakdown.notes);
  }
  return parts;
}

// Complementos elegidos de un producto enviado para el «Desglose» de la cuenta (etapa 9D, §4.4):
// los cambios de ingredientes, lo elegido en cada grupo (también lo incluido de origen) y, en un
// combo, cada componente con lo elegido en él. Sin precios por opción ni números de versión: los
// cargos se muestran con los totales de Go de la partida.
export function chosenComplements(breakdown: ProductBreakdown | null | undefined): string[] {
  if (!breakdown) {
    return [];
  }
  const parts: string[] = [];
  for (const ingredient of breakdown.ingredients ?? []) {
    const requested = toThousandths(ingredient.requested);
    const included = toThousandths(ingredient.included);
    if (requested === 0 && included > 0) {
      parts.push(`Sin ${ingredient.name}`);
    } else if (requested > included) {
      parts.push(`Extra ${ingredient.name}`);
    }
  }
  for (const group of breakdown.groups ?? []) {
    const options = group.options ?? [];
    const requested = options.filter((option) => toThousandths(option.requested) > 0);
    if (requested.length === 0) {
      if (options.some((option) => toThousandths(option.included) > 0)) {
        parts.push(`Sin ${group.name}`);
      }
      continue;
    }
    const names = requested.map((option) => {
      const quantity = displayQuantity(option.requested);
      return quantity === '1' ? option.name : `${quantity} × ${option.name}`;
    });
    parts.push(choiceLabel(group.name, names.join(', ')));
  }
  for (const component of breakdown.components ?? []) {
    if (!component.product || component.quantity <= 0) {
      continue;
    }
    const name = choiceLabel(component.name, component.product.name);
    const label = component.quantity === 1 ? name : `${component.quantity} × ${name}`;
    const nested = chosenComplements(component.product);
    parts.push(nested.length > 0 ? `${label} (${nested.join(', ')})` : label);
  }
  return parts;
}

// Resumen de respaldo mientras una partida nueva aún no tiene desglose de Go: usa los nombres
// de los lookups para las opciones elegidas y las indicaciones escritas.
export function describeSelection(line: PreviewLine, lookups: Lookups | null): string[] {
  const parts: string[] = [];
  // Un borrador guardado sin personalización completa (por ejemplo, por la API) se describe con lo
  // que tenga, sin fallar.
  const saved: Partial<PreviewLine['customization']> = line.customization ?? {};
  const customization = {
    ...saved,
    ingredients: saved.ingredients ?? [],
    groups: saved.groups ?? [],
    components: saved.components ?? [],
    notes: saved.notes ?? '',
  };
  if (line.variant_id) {
    const product = lookups?.products.find((entry) => entry.id === line.product_id);
    const variant = product?.variants.find((entry) => entry.id === line.variant_id);
    if (variant) {
      parts.push(variant.name);
    }
  }
  // Las indicaciones elegidas se describen con su texto (en las notas); su efecto no se repite.
  const chosen = (customization.quick_notes ?? []).map((entry) => {
    const note = lookups?.category_quick_notes?.find((item) => item.id === entry.note_id);
    return { text: entry.text, effect: note?.effect, ingredient_id: note?.ingredient_id };
  });
  const covered = coveredIngredients(customization.notes, chosen);
  for (const ingredient of customization.ingredients) {
    if (covered.has(ingredient.ingredient_id)) {
      continue;
    }
    const name = lookups?.ingredients.find((entry) => entry.id === ingredient.ingredient_id)?.name;
    if (name && toThousandths(ingredient.quantity) === 0) {
      parts.push(`Sin ${name}`);
    } else if (name) {
      parts.push(`Extra ${name}`);
    }
  }
  for (const selection of customization.groups) {
    const group = lookups?.groups.find((entry) => entry.id === selection.group_id);
    const chosen = selection.options.filter((option) => toThousandths(option.quantity) > 0);
    if (group && chosen.length === 0) {
      parts.push(`Sin ${group.name}`);
    }
    for (const option of chosen) {
      const name = group?.options.find((entry) => entry.id === option.option_id)?.name;
      if (name) {
        const quantity = displayQuantity(option.quantity);
        parts.push(quantity === '1' ? name : `${quantity} × ${name}`);
      }
    }
  }
  for (const component of customization.components) {
    const name = lookups?.products.find((entry) => entry.id === component.product_id)?.name;
    if (name && component.quantity > 0) {
      parts.push(name);
    }
  }
  if (customization.notes.trim()) {
    parts.push(customization.notes.trim());
  }
  return parts;
}

// Importe de Go mayor que cero (cadena decimal sin signo). Solo decide si se muestra un cargo
// en el botón; no hace aritmética con dinero.
export function positiveAmount(value: string | null | undefined): boolean {
  if (!value || value.startsWith('-')) {
    return false;
  }
  return !/^0*(\.0*)?$/.test(value.trim());
}

// Importe de Go distinto de cero, con o sin signo (por ejemplo «-10.00»). Solo decide si se
// muestra una diferencia; no hace aritmética con dinero.
export function nonZeroAmount(value: string | null | undefined): boolean {
  if (!value) {
    return false;
  }
  return positiveAmount(value.trim().replace(/^-/, ''));
}
