// Sugerencia del código interno a partir de un nombre (etapa 6.1, generalizada en la 9A). Es la única
// regla de códigos sugeridos del sistema: productos y combos (60 caracteres), presentaciones y
// copias de productos (60) e insumos (40). Go solo exige un código único, no vacío y dentro del
// largo de cada registro (catalog/product_identifiers.go, inventory/items.go); esta forma
// («Machacado Natural» → «machacado-natural») lo cumple y es fácil de leer en tickets y reportes.
// Para insumos, Go comprueba además la disponibilidad con la misma regla de sufijos
// (inventory/item_codes.go), porque la pantalla no conoce todos los códigos.

export const productCodeLength = 60;
export const inventoryItemCodeLength = 40;

function trimTo(value: string, length: number): string {
  return value.slice(0, length).replace(/-+$/, '');
}

// Minúsculas, sin acentos, cualquier otro carácter convertido en guion, sin guiones repetidos ni
// en los extremos y con un máximo de maxLength caracteres (60 por omisión).
export function codeFromName(name: string, maxLength: number = productCodeLength): string {
  const base = name
    .normalize('NFD')
    .replace(/[̀-ͯ]/g, '')
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '');
  return trimTo(base, maxLength);
}

// Propone un código libre a partir de una base ya formada: si existe (sin distinguir mayúsculas),
// agrega -2, -3… recortando la base para no superar el máximo.
export function freeCode(
  base: string,
  existingCodes: string[],
  maxLength: number = productCodeLength,
): string {
  if (!base) {
    return '';
  }
  const taken = new Set(existingCodes.map((code) => code.toLowerCase()));
  if (!taken.has(base.toLowerCase())) {
    return base;
  }
  for (let attempt = 2; attempt < 10000; attempt++) {
    const suffix = `-${attempt}`;
    const candidate = `${trimTo(base, maxLength - suffix.length)}${suffix}`;
    if (!taken.has(candidate.toLowerCase())) {
      return candidate;
    }
  }
  return base;
}

// Propone un código libre a partir del nombre (codeFromName + freeCode).
export function suggestProductCode(
  name: string,
  existingCodes: string[],
  maxLength: number = productCodeLength,
): string {
  return freeCode(codeFromName(name, maxLength), existingCodes, maxLength);
}

// Código de una presentación nueva de un producto: el código del producto y el nombre de la
// presentación («cafe-de-olla» + «Grande» → «cafe-de-olla-grande»), libre frente a los códigos
// conocidos (Go valida además los de presentaciones de otros productos).
export function suggestVariantCode(
  productCode: string,
  variantName: string,
  existingCodes: string[],
): string {
  const variant = codeFromName(variantName);
  if (!variant) {
    return '';
  }
  const product = codeFromName(productCode);
  const base = product ? codeFromName(`${product} ${variant}`) : variant;
  return freeCode(base, existingCodes);
}

// Sigue el nombre mientras el código sea vacío o el que se habría sugerido para el nombre anterior:
// así un código escrito a mano nunca se sobrescribe, sin necesitar otra marca de estado.
export function followsSuggestion(code: string, previousSuggestion: string): boolean {
  return code === '' || code === previousSuggestion;
}
