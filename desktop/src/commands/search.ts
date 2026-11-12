import type { CommandEntry } from './actions';

// Búsqueda de la paleta de comandos: sin distinguir mayúsculas ni acentos, cada palabra escrita
// debe ser el inicio de alguna palabra del nombre o de sus sinónimos («mesa 5» encuentra «Mesa 5»
// pero no «Mesa 15»; «retiro» encuentra «Retiro»; «productos», el Catálogo).

export function normalizeText(text: string) {
  return text
    .normalize('NFD')
    .replace(/[\u0300-\u036f]/g, '')
    .toLowerCase()
    .trim();
}

function words(text: string) {
  return normalizeText(text)
    .split(/[^a-z0-9ñ]+/)
    .filter(Boolean);
}

function tokensMatch(tokens: string[], candidates: string[]) {
  return tokens.every((token) => candidates.some((word) => word.startsWith(token)));
}

// Puntuación de una entrada (0 = no coincide). Primero el nombre que empieza con lo escrito, luego
// el nombre que contiene todas las palabras y al final los sinónimos y la descripción.
export function scoreEntry(entry: CommandEntry, query: string) {
  const normalized = normalizeText(query);
  if (!normalized) {
    return 1;
  }
  const tokens = words(query);
  if (tokens.length === 0) {
    return 0;
  }
  const label = normalizeText(entry.label);
  const labelWords = words(entry.label);
  if (label.startsWith(normalized)) {
    return 100;
  }
  if (tokensMatch(tokens, labelWords)) {
    return 60;
  }
  const extra = [...labelWords, ...entry.keywords.flatMap(words), ...words(entry.kind)];
  if (tokensMatch(tokens, extra)) {
    return 30;
  }
  if (entry.description && tokensMatch(tokens, [...extra, ...words(entry.description)])) {
    return 10;
  }
  return 0;
}

// Sin texto se muestran las acciones y pantallas (no las mesas, que pueden ser muchas); con texto,
// las coincidencias por puntuación y, a igual puntuación, en el orden del registro. Las entradas
// deshabilitadas van al final de su puntuación.
export function searchEntries(entries: CommandEntry[], query: string, limit = 60) {
  const empty = !normalizeText(query);
  return entries
    .map((entry, index) => ({ entry, index, score: scoreEntry(entry, query) }))
    .filter(({ entry, score }) => score > 0 && !(empty && entry.kind === 'Mesa'))
    .sort((left, right) => {
      if (right.score !== left.score) {
        return right.score - left.score;
      }
      if (left.entry.enabled !== right.entry.enabled) {
        return left.entry.enabled ? -1 : 1;
      }
      return left.index - right.index;
    })
    .slice(0, limit)
    .map(({ entry }) => entry);
}
