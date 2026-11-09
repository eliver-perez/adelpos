// Concordancia de contadores en español (etapa 7.1): «1 cuenta», «2 cuentas», «0 cuentas». Las
// mismas reglas y la misma tabla de casos que backend/internal/plural, que usa Go en los textos
// impresos. Solo el 1 lleva singular; el cero y los demás valores llevan plural.

// Palabra que concuerda con el contador, sin el número: útil para verbos o adjetivos
// («1 cuenta abierta», «3 cuentas abiertas»).
export function pluralWord(count: number, singular: string, plural: string) {
  if (count === 1) {
    return singular;
  }
  return plural;
}

// Contador con su sustantivo: countLabel(1, 'operación de efectivo', 'operaciones de efectivo').
export function countLabel(count: number, singular: string, plural: string) {
  return `${count} ${pluralWord(count, singular, plural)}`;
}
