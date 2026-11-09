// Captura textual de importes, cantidades y medidas en los formularios.
//
// Igual que en el cobro, estas funciones solo deciden si el texto tecleado conserva una forma
// válida; nunca convierten a número ni calculan dinero. Un carácter no válido se descarta al
// teclear (se conserva el texto anterior) y Go vuelve a validar cada valor al guardar.

export interface DecimalShape {
  // Dígitos decimales permitidos: 2 en importes y precios, 3 en cantidades.
  decimals: number;
  // Dígitos enteros permitidos: 10 en importes (límite de Go), 4 en cantidades (0 a 1000).
  integerDigits: number;
}

export const amountShape: DecimalShape = { decimals: 2, integerDigits: 10 };
export const quantityShape: DecimalShape = { decimals: 3, integerDigits: 4 };

function partialDecimalPattern(shape: DecimalShape): RegExp {
  // Forma parcial aceptable mientras se captura: vacío, "0", "12", "12.", "12.5" o "12.50".
  const integerPart = `(?:0|[1-9][0-9]{0,${shape.integerDigits - 1}})?`;
  const decimalPart = `(?:\\.[0-9]{0,${shape.decimals}})?`;
  return new RegExp(`^${integerPart}${decimalPart}$`);
}

// Acepta el texto nuevo solo si es un número decimal parcial válido para la forma indicada.
// La coma se interpreta como punto decimal y un punto inicial se completa con cero.
export function acceptDecimalText(next: string, previous: string, shape: DecimalShape): string {
  let normalized = next.replaceAll(',', '.').trim();
  if (normalized.startsWith('.')) {
    normalized = `0${normalized}`;
  }
  // Un cero inicial seguido de otro dígito ("05") se interpreta como el dígito tecleado ("5").
  if (/^0[0-9]/.test(normalized)) {
    normalized = normalized.replace(/^0+(?=[0-9])/, '');
  }
  if (partialDecimalPattern(shape).test(normalized)) {
    return normalized;
  }
  return previous;
}

// Acepta el texto nuevo solo si contiene únicamente dígitos y no excede el máximo de dígitos.
// Los ceros a la izquierda se descartan para que "05" se lea como "5".
export function acceptIntegerText(next: string, previous: string, maxDigits: number): string {
  const trimmed = next.trim();
  if (trimmed === '') {
    return '';
  }
  if (!/^[0-9]+$/.test(trimmed)) {
    return previous;
  }
  const normalized = trimmed.replace(/^0+(?=[0-9])/, '');
  if (normalized.length > maxDigits) {
    return previous;
  }
  return normalized;
}

// Número de dígitos necesarios para representar el máximo de un campo entero.
export function digitsFor(maximum: number): number {
  return String(Math.max(0, Math.trunc(maximum))).length;
}
