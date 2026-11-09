import { orderSiblings, positionPayload } from './positions';
import type { PositionOption } from './positions';
import type { CatalogDocument, Category, Lookups, Product, Resource } from './catalog';

// Las categorías forman una sola lista.
export function categorySiblings(lookups: Lookups): PositionOption[] {
  return orderSiblings(lookups.categories).map((category) => ({
    id: category.id,
    name: category.name,
    note: category.active ? undefined : 'inactiva',
  }));
}

// Los productos (combos incluidos) se ordenan dentro de su categoría.
export function productSiblings(lookups: Lookups, categoryID: string): PositionOption[] {
  if (!categoryID) {
    return [];
  }
  const members = lookups.products.filter((product) => product.category_id === categoryID);
  return orderSiblings(members).map((product) => {
    let note: string | undefined;
    if (product.archived) {
      note = 'archivado';
    } else if (!product.active) {
      note = 'inactivo';
    } else if (product.kind === 'combo') {
      note = 'combo';
    }
    return { id: product.id, name: product.name, note };
  });
}

// Cuerpo que se envía a Go: la posición elegida (o la predeterminada de un registro nuevo o que
// cambia de categoría) reemplaza la captura del número de orden.
export function catalogRequestBody(
  resource: Resource,
  document: CatalogDocument,
  lookups: Lookups,
): CatalogDocument {
  if (resource !== 'categories' && resource !== 'products') {
    return document;
  }
  const positioned = { ...(document as Category | Product) };
  const siblings =
    resource === 'categories'
      ? categorySiblings(lookups)
      : productSiblings(lookups, (document as Product).category_id);
  const position = positionPayload(siblings, document.id, positioned.position?.after);
  delete positioned.position;
  if (position) {
    positioned.position = position;
  }
  return positioned;
}
