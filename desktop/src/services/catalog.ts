import { orderSiblings } from './positions';

// Contratos del catálogo: importes como cadenas; Go conserva la autoridad financiera.

export interface Metadata {
  id: string;
  name: string;
  description: string;
  active: boolean;
  revision: number;
}

export interface TimeWindow {
  start: string;
  end: string;
}

export interface Schedule {
  starts_on?: string;
  ends_on?: string;
  days: number[];
  windows: TimeWindow[];
}

// Posición elegida para el orden visual: «» al inicio o el identificador del hermano anterior.
// Solo se envía al crear o guardar; Go recalcula sort_order y no la devuelve en las lecturas.
export interface Position {
  after: string;
}

export interface PaletteColor {
  hex: string;
  name: string;
}

export interface Category extends Metadata {
  sort_order: number;
  color: string;
  image_id: string | null;
  availability: Schedule;
  position?: Position;
  // Solo en los lookups: activa y dentro de su horario ahora, según el reloj de Go.
  available_now?: boolean;
}

export interface Ingredient extends Metadata {
  unit: string;
}

export interface Complement extends Metadata {
  product_id: string | null;
  ingredient_id: string | null;
}

export interface Portion extends Metadata {
  factor: string;
}

export interface QuantityTier {
  from: number;
  unit_price: string;
}

export interface GroupOption {
  id: string;
  complement_id: string;
  portion_id: string | null;
  reference_value: string;
  extra_price: string;
  max_quantity: string | null;
  active: boolean;
  tiers: QuantityTier[];
  name?: string;
  portion_name?: string;
  portion_factor?: string;
}

export interface OptionGroup extends Metadata {
  pricing_mode: string;
  min_quantity: string;
  max_quantity: string | null;
  options: GroupOption[];
}

export interface Variant {
  id: string;
  name: string;
  code: string;
  barcode: string | null;
  price: string;
  active: boolean;
  availability: Schedule;
}

export interface ProductIngredient {
  ingredient_id: string;
  included_quantity: string;
  removable: boolean;
  addable: boolean;
  max_quantity: string;
  extra_price: string;
  recipe_quantity: string;
  name?: string;
}

export interface IncludedOption {
  option_id: string;
  quantity: string;
}

export interface ProductGroup {
  group_id: string;
  min_quantity: string | null;
  max_quantity: string | null;
  included: IncludedOption[];
}

export interface ComboOption {
  product_id: string;
  substitution_price: string;
  tiers: QuantityTier[];
}

export interface ComboSlot {
  id: string;
  name: string;
  min_quantity: number;
  max_quantity: number;
  included_quantity: number;
  default_quantity: number;
  default_product_id: string;
  options: ComboOption[];
}

// Tasa de IVA propia de un producto o combo: lista cerrada validada en Go y PostgreSQL. null (o
// ausente) hereda la tasa general vigente del establecimiento.
export type ProductTaxRate = 16 | 8 | 0;

export interface Product extends Metadata {
  category_id: string;
  code: string;
  barcode: string | null;
  kind: string;
  base_price: string;
  tax_rate?: ProductTaxRate | null;
  image_id: string | null;
  sort_order: number;
  archived: boolean;
  temporary_unavailable: boolean;
  availability: Schedule;
  variants: Variant[];
  ingredients: ProductIngredient[];
  groups: ProductGroup[];
  slots: ComboSlot[];
  position?: Position;
}

export interface PriceEntry {
  product_id: string;
  variant_id: string | null;
  price: string;
}

export interface PriceList extends Metadata {
  is_default: boolean;
  fallback: string;
  allow_promotions: boolean;
  entries: PriceEntry[];
}

export interface Promotion extends Metadata {
  kind: string;
  percent: string;
  special_price: string;
  buy_quantity: number;
  pay_quantity: number;
  minimum_quantity: number;
  priority: number;
  stackable: boolean;
  include_extras: boolean;
  product_ids: string[];
  category_ids: string[];
  price_list_ids: string[];
  availability: Schedule;
}

export type Resource =
  | 'categories'
  | 'products'
  | 'ingredients'
  | 'complements'
  | 'portions'
  | 'groups'
  | 'price_lists'
  | 'promotions';
export type CatalogDocument =
  Category | Product | Ingredient | Complement | Portion | OptionGroup | PriceList | Promotion;
export interface ProductReference extends Metadata {
  code: string;
  kind: string;
  archived: boolean;
  // Categoría y orden dentro de ella: permiten mostrar las posiciones sin otra consulta.
  category_id: string;
  sort_order: number;
  variants: Pick<Variant, 'id' | 'name' | 'active'>[];
}
// Indicación rápida de una categoría (etapa 7.1): texto sin comas que la captura agrega a las
// indicaciones para cocina de la partida.
export interface CategoryQuickNote {
  id: string;
  category_id: string;
  text: string;
  active: boolean;
  sort_order: number;
  revision: number;
  updated_at: string;
}

export interface Lookups {
  categories: Category[];
  ingredients: Ingredient[];
  complements: Complement[];
  portions: Portion[];
  groups: OptionGroup[];
  'price-lists': PriceList[];
  products: ProductReference[];
  // Paleta cerrada de colores de categoría definida en Go.
  palette: PaletteColor[];
  // Indicaciones rápidas de todas las categorías, activas e inactivas, en su orden visual.
  category_quick_notes?: CategoryQuickNote[];
}

// Indicaciones rápidas activas de la categoría de un producto, en su orden visual (Go ya las
// entrega ordenadas; se reordenan por sort_order para no depender de ello).
export function quickNotesFor(lookups: Lookups, productID: string): string[] {
  const product = lookups.products.find((entry) => entry.id === productID);
  if (!product) {
    return [];
  }
  const notes = (lookups.category_quick_notes ?? []).filter(
    (note) => note.active && note.category_id === product.category_id,
  );
  return orderSiblings(notes).map((note) => note.text);
}
export interface CatalogPage {
  items: CatalogDocument[];
  total: number;
  page: number;
  page_size: number;
}
export interface Configuration {
  product: Product;
  category: Category;
  groups: OptionGroup[];
  timezone: string;
  availability_reason: string;
}

export interface Customization {
  ingredients: { ingredient_id: string; quantity: string }[];
  groups: { group_id: string; options: IncludedOption[] }[];
  components: ComponentSelection[];
  notes: string;
}

export interface ComponentSelection {
  slot_id: string;
  product_id: string;
  variant_id: string | null;
  quantity: number;
  customization: Customization;
}

export interface PreviewLine {
  product_id: string;
  variant_id: string | null;
  quantity: number;
  customization: Customization;
}

export interface ProductBreakdown {
  product_id: string;
  revision: number;
  name: string;
  base_price: string;
  price_origin: string;
  unit_price: string;
  ingredient_extras: string;
  group_extras: string;
  component_extras: string;
  notes: string;
  ingredients: {
    ingredient_id: string;
    name: string;
    included: string;
    requested: string;
    extra: string;
  }[];
  groups: {
    group_id: string;
    name: string;
    mode: string;
    original_value: string;
    requested_value: string;
    difference: string;
    extras: string;
    options: { option_id: string; name: string; included: string; requested: string }[];
  }[];
  components: {
    slot_id: string;
    name: string;
    quantity: number;
    charge: string;
    product: ProductBreakdown | null;
  }[];
}

export interface Quote {
  calculated_at: string;
  timezone: string;
  price_list_name: string;
  price_list_revision: number;
  subtotal: string;
  discount: string;
  total: string;
  lines: (ProductBreakdown & {
    quantity: number;
    subtotal: string;
    discount: string;
    total: string;
  })[];
  promotions: { id: string; name: string; applied: boolean; reason: string; discount: string }[];
}
