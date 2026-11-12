import type { IconName } from './Icon';

// Modelo del selector único (etapa 9C, fase B): tipos de opciones, normalización para buscar sin
// distinguir mayúsculas ni acentos, filtrado por grupos y lista plana para el teclado. Sin React:
// lo usan components/Select.tsx y sus pruebas.

export interface SelectOption<T extends string = string> {
  value: T;
  label: string;
  // Segunda línea en tono discreto (por ejemplo, «inactivo» o un detalle); también se busca.
  description?: string;
  disabled?: boolean;
  // Palabras adicionales para la búsqueda que no se muestran (por ejemplo, «México»).
  keywords?: string;
}

// Grupo con encabezado, como <optgroup>.
export interface SelectGroup<T extends string = string> {
  label: string;
  options: readonly SelectOption<T>[];
}

export type SelectItem<T extends string = string> = SelectOption<T> | SelectGroup<T>;

// Opción de acción al final de la lista («+ Nueva categoría», «Otra unidad o presentación…»): no
// cambia el valor, ejecuta su función y cierra la lista. Siempre aparece, aunque se esté buscando.
export interface SelectAction {
  label: string;
  onSelect: () => void;
  icon?: IconName;
  disabled?: boolean;
}

// Con más opciones que este número la búsqueda aparece sola (§2 de la especificación).
export const searchThreshold = 8;

export function isGroup<T extends string>(item: SelectItem<T>): item is SelectGroup<T> {
  return 'options' in item;
}

// Texto comparable: sin acentos ni diéresis, sin guiones bajos y en minúsculas. «Ñ» se compara
// como «n», igual que las letras acentuadas.
export function normalizeSearchText(text: string): string {
  return text.normalize('NFD').replace(/[̀-ͯ]/g, '').replaceAll('_', ' ').toLowerCase();
}

export function flattenOptions<T extends string>(
  items: readonly SelectItem<T>[],
): SelectOption<T>[] {
  const result: SelectOption<T>[] = [];
  for (const item of items) {
    if (isGroup(item)) {
      result.push(...item.options);
    } else {
      result.push(item);
    }
  }
  return result;
}

export function optionCount<T extends string>(items: readonly SelectItem<T>[]): number {
  let count = 0;
  for (const item of items) {
    count += isGroup(item) ? item.options.length : 1;
  }
  return count;
}

export function findOption<T extends string>(
  items: readonly SelectItem<T>[],
  value: string,
): SelectOption<T> | undefined {
  for (const item of items) {
    if (isGroup(item)) {
      const found = item.options.find((option) => option.value === value);
      if (found) {
        return found;
      }
    } else if (item.value === value) {
      return item;
    }
  }
  return undefined;
}

function searchableText(option: SelectOption): string {
  return normalizeSearchText(
    [option.label, option.description ?? '', option.keywords ?? ''].join(' '),
  );
}

// Cada palabra de la búsqueda debe aparecer en la opción, en cualquier orden.
export function matchesQuery(option: SelectOption, query: string): boolean {
  const terms = normalizeSearchText(query).split(/\s+/).filter(Boolean);
  if (terms.length === 0) {
    return true;
  }
  const text = searchableText(option);
  return terms.every((term) => text.includes(term));
}

// Filtra conservando los grupos: un grupo sin coincidencias desaparece.
export function filterItems<T extends string>(
  items: readonly SelectItem<T>[],
  query: string,
): SelectItem<T>[] {
  if (query.trim() === '') {
    return [...items];
  }
  const result: SelectItem<T>[] = [];
  for (const item of items) {
    if (isGroup(item)) {
      const options = item.options.filter((option) => matchesQuery(option, query));
      if (options.length > 0) {
        result.push({ label: item.label, options });
      }
    } else if (matchesQuery(item, query)) {
      result.push(item);
    }
  }
  return result;
}

// Elemento recorrible con el teclado: una opción o una acción, en el orden en que se ven.
export type NavigableEntry<T extends string> =
  | { kind: 'option'; option: SelectOption<T>; index: number }
  | { kind: 'action'; action: SelectAction; index: number };

export function navigableEntries<T extends string>(
  items: readonly SelectItem<T>[],
  actions: readonly SelectAction[],
): NavigableEntry<T>[] {
  const entries: NavigableEntry<T>[] = [];
  for (const option of flattenOptions(items)) {
    entries.push({ kind: 'option', option, index: entries.length });
  }
  for (const action of actions) {
    entries.push({ kind: 'action', action, index: entries.length });
  }
  return entries;
}

export function entryDisabled<T extends string>(entry: NavigableEntry<T>): boolean {
  if (entry.kind === 'option') {
    return entry.option.disabled === true;
  }
  return entry.action.disabled === true;
}

// Siguiente elemento habilitado desde start en la dirección indicada, sin dar la vuelta. Si no hay
// ninguno en esa dirección, conserva el actual (o el primero habilitado si no había).
export function stepIndex<T extends string>(
  entries: readonly NavigableEntry<T>[],
  current: number,
  delta: number,
): number {
  if (entries.length === 0) {
    return -1;
  }
  const direction = delta >= 0 ? 1 : -1;
  let steps = Math.abs(delta);
  let index = current;
  let candidate = current;
  while (steps > 0) {
    candidate += direction;
    if (candidate < 0 || candidate >= entries.length) {
      break;
    }
    const entry = entries[candidate];
    if (entry && !entryDisabled(entry)) {
      index = candidate;
      steps -= 1;
    }
  }
  if (index < 0 || index >= entries.length || entryDisabled(entries[index]!)) {
    return edgeIndex(entries, direction > 0 ? 'first' : 'last');
  }
  return index;
}

export function edgeIndex<T extends string>(
  entries: readonly NavigableEntry<T>[],
  edge: 'first' | 'last',
): number {
  if (edge === 'first') {
    return entries.findIndex((entry) => !entryDisabled(entry));
  }
  for (let index = entries.length - 1; index >= 0; index -= 1) {
    const entry = entries[index];
    if (entry && !entryDisabled(entry)) {
      return index;
    }
  }
  return -1;
}

// Búsqueda por escritura sin campo de búsqueda (listas cortas): la primera opción cuyo texto
// empieza con lo escrito, a partir de la siguiente a la actual; si se repite la misma letra, va
// pasando por las opciones que empiezan con ella (como una lista nativa).
export function typeaheadIndex<T extends string>(
  entries: readonly NavigableEntry<T>[],
  current: number,
  typed: string,
): number {
  const wanted = normalizeSearchText(typed);
  if (wanted === '' || entries.length === 0) {
    return -1;
  }
  const repeated = Array.from(wanted).every((letter) => letter === wanted[0]);
  const prefix = repeated ? wanted.slice(0, 1) : wanted;
  const start = repeated ? current + 1 : Math.max(current, 0);
  for (let offset = 0; offset < entries.length; offset += 1) {
    const index = (start + offset) % entries.length;
    const entry = entries[index];
    if (!entry || entry.kind !== 'option' || entryDisabled(entry)) {
      continue;
    }
    if (normalizeSearchText(entry.option.label).startsWith(prefix)) {
      return index;
    }
  }
  return -1;
}

// Clave estable de un elemento recorrible: el activo se recuerda por clave (no por posición) para
// que no salte cuando la lista se vuelve a dibujar o llegan resultados nuevos.
export function entryKey<T extends string>(entry: NavigableEntry<T>) {
  if (entry.kind === 'option') {
    return `option:${entry.option.value}`;
  }
  return `action:${entry.action.label}`;
}

// Con la lista cerrada: qué teclas la abren y en qué elemento empieza (el elegido, el primero o el
// último).
export type OpeningKey = 'selected' | 'first' | 'last';

export function openingFor(key: string): OpeningKey | null {
  switch (key) {
    case 'ArrowDown':
    case 'ArrowUp':
    case 'Enter':
    case ' ':
      return 'selected';
    case 'Home':
      return 'first';
    case 'End':
      return 'last';
    default:
      return null;
  }
}

// Con la lista abierta: elemento al que lleva una tecla de movimiento, o null si la tecla no mueve.
export function navigationIndex<T extends string>(
  key: string,
  entries: readonly NavigableEntry<T>[],
  current: number,
  page: number,
): number | null {
  switch (key) {
    case 'ArrowDown':
      return stepIndex(entries, current, 1);
    case 'ArrowUp':
      return stepIndex(entries, current, -1);
    case 'Home':
      return edgeIndex(entries, 'first');
    case 'End':
      return edgeIndex(entries, 'last');
    case 'PageDown':
      return stepIndex(entries, current, page);
    case 'PageUp':
      return stepIndex(entries, current, -page);
    default:
      return null;
  }
}

// Teclas que atiende la lista abierta: no llegan a los componentes que la contienen ni a los
// atajos de la aplicación (Escape cierra solo la lista, no el diálogo).
export const listKeys = [
  'ArrowDown',
  'ArrowUp',
  'Home',
  'End',
  'PageDown',
  'PageUp',
  'Enter',
  'Escape',
];
