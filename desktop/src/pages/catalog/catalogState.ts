import type { CatalogDocument, Product, Resource } from '../../services/catalog';
import type { Operation } from '../../services/api';
import { emptySchedule } from '../../components/catalog/EditorFields';

export const catalogTabs: { id: Resource | 'combos'; label: string; permission: string }[] = [
  { id: 'categories', label: 'Categorías', permission: 'catalog.categories.manage' },
  { id: 'products', label: 'Productos', permission: 'catalog.products.manage' },
  { id: 'combos', label: 'Combos', permission: 'catalog.combos.manage' },
  { id: 'ingredients', label: 'Ingredientes', permission: 'catalog.ingredients.manage' },
  { id: 'complements', label: 'Complementos', permission: 'catalog.complements.manage' },
  { id: 'portions', label: 'Porciones', permission: 'catalog.complements.manage' },
  { id: 'groups', label: 'Grupos', permission: 'catalog.groups.manage' },
  { id: 'price_lists', label: 'Listas de precios', permission: 'catalog.price_lists.manage' },
  { id: 'promotions', label: 'Promociones', permission: 'catalog.promotions.manage' },
];

export function catalogOperation(
  resource: Resource,
  action: '' | 'get_' | 'create_' | 'save_' = '',
): Operation {
  // Resource procede exclusivamente del registro local de pestañas, nunca de una URL libre.
  return `catalog_${action}${resource}` as Operation;
}

export function newCatalogDocument(
  resource: Resource,
  combo: boolean,
  categoryID: string,
): CatalogDocument {
  const metadata = { id: '', revision: 0, name: '', description: '', active: true };
  switch (resource) {
    case 'categories':
      return {
        ...metadata,
        sort_order: 0,
        color: '',
        image_id: null,
        availability: emptySchedule(),
      };
    case 'ingredients':
      return { ...metadata, unit: 'porción' };
    case 'complements':
      return { ...metadata, product_id: null, ingredient_id: null };
    case 'portions':
      return { ...metadata, factor: '1' };
    case 'groups':
      return {
        ...metadata,
        pricing_mode: 'value',
        min_quantity: '0',
        max_quantity: null,
        options: [],
      };
    case 'price_lists':
      return {
        ...metadata,
        is_default: false,
        fallback: 'base',
        allow_promotions: true,
        entries: [],
      };
    case 'promotions':
      return {
        ...metadata,
        kind: 'two_for_one',
        percent: '0',
        special_price: '0',
        buy_quantity: 2,
        pay_quantity: 1,
        minimum_quantity: 2,
        priority: 0,
        stackable: false,
        include_extras: false,
        product_ids: [],
        category_ids: [],
        price_list_ids: [],
        availability: emptySchedule(),
      };
    case 'products':
      return {
        ...metadata,
        category_id: categoryID,
        code: '',
        barcode: null,
        kind: combo ? 'combo' : 'product',
        base_price: '0.00',
        tax_rate: null,
        price_can_decrease: false,
        image_id: null,
        sort_order: 0,
        archived: false,
        temporary_unavailable: false,
        availability: emptySchedule(),
        variants: [],
        ingredients: [],
        groups: [],
        slots: [],
      } satisfies Product;
  }
}
