// Sugerencia del código interno de un producto a partir de su nombre. Go solo exige un código
// único, no vacío y de hasta 60 caracteres (catalog/product_identifiers.go); esta forma
// («Machacado Natural» → «machacado-natural») lo cumple y es fácil de leer en tickets y reportes.

export const productCodeLength = 60;

function trimTo(value: string, length: number): string {
  return value.slice(0, length).replace(/-+$/, '');
}

// Minúsculas, sin acentos, cualquier otro carácter convertido en guion, sin guiones repetidos ni
// en los extremos y con un máximo de 60 caracteres.
export function codeFromName(name: string): string {
  const base = name
    .normalize('NFD')
    .replace(/[̀-ͯ]/g, '')
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '');
  return trimTo(base, productCodeLength);
}

// Propone un código libre: si la forma base ya existe (sin distinguir mayúsculas), agrega -2, -3…
// recortando la base para no superar el máximo.
export function suggestProductCode(name: string, existingCodes: string[]): string {
  const base = codeFromName(name);
  if (!base) {
    return '';
  }
  const taken = new Set(existingCodes.map((code) => code.toLowerCase()));
  if (!taken.has(base)) {
    return base;
  }
  for (let attempt = 2; attempt < 10000; attempt++) {
    const suffix = `-${attempt}`;
    const candidate = `${trimTo(base, productCodeLength - suffix.length)}${suffix}`;
    if (!taken.has(candidate)) {
      return candidate;
    }
  }
  return base;
}
