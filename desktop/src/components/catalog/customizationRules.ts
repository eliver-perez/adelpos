import type {
  ComboSlot,
  ComponentSelection,
  Customization,
  IncludedOption,
  OptionGroup,
  ProductGroup,
  ProductIngredient,
} from '../../services/catalog';

// Reglas de selección de una personalización, compartidas por el editor de formulario
// (ProductCustomizer) y la captura táctil. Solo construyen la selección que se envía a Go; los
// importes, mínimos y máximos los vuelve a validar el motor de precios del servidor.

export const originalCustomization = (): Customization => ({
  ingredients: [],
  groups: [],
  components: [],
  notes: '',
});

// Un grupo omitido conserva su configuración original (las opciones incluidas del producto);
// una selección explícita, aunque esté vacía, sustituye esa configuración.
export function groupSelection(value: Customization, binding: ProductGroup): IncludedOption[] {
  const explicit = value.groups.find((entry) => entry.group_id === binding.group_id);
  return explicit ? explicit.options : binding.included;
}

export function withGroupSelection(
  value: Customization,
  groupID: string,
  options: IncludedOption[],
): Customization {
  return {
    ...value,
    groups: [
      ...value.groups.filter((entry) => entry.group_id !== groupID),
      { group_id: groupID, options },
    ],
  };
}

// Cambia la cantidad solicitada de una opción y conserva las demás opciones de la selección
// vigente (explícita u original).
export function withGroupOption(
  value: Customization,
  binding: ProductGroup,
  optionID: string,
  quantity: string,
): Customization {
  const selection = groupSelection(value, binding);
  return withGroupSelection(value, binding.group_id, [
    ...selection.filter((entry) => entry.option_id !== optionID),
    { option_id: optionID, quantity },
  ]);
}

// Retirar todas las opciones de un grupo es una selección explícita vacía.
export function withoutGroupOptions(value: Customization, groupID: string): Customization {
  return withGroupSelection(value, groupID, []);
}

// Límites efectivos del grupo: el producto puede sobrescribir los del grupo reutilizable.
export function groupLimits(binding: ProductGroup, group: OptionGroup) {
  return {
    minimum: binding.min_quantity ?? group.min_quantity,
    maximum: binding.max_quantity ?? group.max_quantity,
  };
}

export function ingredientQuantity(value: Customization, ingredient: ProductIngredient): string {
  return (
    value.ingredients.find((entry) => entry.ingredient_id === ingredient.ingredient_id)?.quantity ??
    ingredient.included_quantity
  );
}

export function withIngredient(
  value: Customization,
  ingredientID: string,
  quantity: string,
): Customization {
  return {
    ...value,
    ingredients: [
      ...value.ingredients.filter((entry) => entry.ingredient_id !== ingredientID),
      { ingredient_id: ingredientID, quantity },
    ],
  };
}

// Sin una cantidad explícita, el ingrediente vuelve a su cantidad incluida.
export function withoutIngredientChange(value: Customization, ingredientID: string): Customization {
  return {
    ...value,
    ingredients: value.ingredients.filter((entry) => entry.ingredient_id !== ingredientID),
  };
}

// Selección vigente de un componente de combo: la explícita o la original del componente.
export function componentSelection(value: Customization, slot: ComboSlot): ComponentSelection {
  return (
    value.components.find((entry) => entry.slot_id === slot.id) ?? {
      slot_id: slot.id,
      product_id: slot.default_product_id,
      variant_id: null,
      quantity: slot.default_quantity,
      customization: originalCustomization(),
    }
  );
}

export function withComponent(value: Customization, selection: ComponentSelection): Customization {
  return {
    ...value,
    components: [
      ...value.components.filter((entry) => entry.slot_id !== selection.slot_id),
      selection,
    ],
  };
}

// Productos que puede ocupar un componente: el original siempre está permitido de forma
// implícita (etapa 6.1 §1.4) y después las alternativas configuradas.
export function componentProductIDs(slot: ComboSlot): string[] {
  const identifiers = [slot.default_product_id];
  for (const option of slot.options) {
    if (!identifiers.includes(option.product_id)) {
      identifiers.push(option.product_id);
    }
  }
  return identifiers;
}
