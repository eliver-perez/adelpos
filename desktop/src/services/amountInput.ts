// Edición textual de importes capturados en el teclado numérico de cobro.
//
// Estas funciones solo validan la forma de la cadena que escribe el cajero: nunca convierten a
// número ni calculan dinero. Go vuelve a validar cada importe con aritmética decimal exacta.

export type AmountDigit = '0' | '1' | '2' | '3' | '4' | '5' | '6' | '7' | '8' | '9';
export type AmountKey = AmountDigit | '.' | '00' | 'backspace' | 'clear';

// Límites alineados con la validación de Go: hasta diez dígitos enteros y dos decimales.
export const maxIntegerDigits = 10;
export const maxDecimalDigits = 2;

const digits: string[] = ['0', '1', '2', '3', '4', '5', '6', '7', '8', '9'];

// Forma parcial aceptable mientras se captura: vacío, "0", "12", "12.", "12.5" o "12.50".
const partialAmount = /^(?:0|[1-9][0-9]{0,9})?(?:\.[0-9]{0,2})?$/;

function isDigit(value: string): value is AmountDigit {
  return digits.includes(value);
}

function appendDigit(current: string, digit: AmountDigit): string {
  const separator = current.indexOf('.');
  if (separator >= 0) {
    // Con punto decimal solo se admiten dos decimales; un tercero se ignora.
    const decimals = current.length - separator - 1;
    if (decimals >= maxDecimalDigits) {
      return current;
    }
    return current + digit;
  }
  // Sin ceros a la izquierda: "0" seguido de un dígito se sustituye por ese dígito.
  if (current === '0') {
    return digit;
  }
  if (current.length >= maxIntegerDigits) {
    return current;
  }
  return current + digit;
}

function appendSeparator(current: string): string {
  if (current.includes('.')) {
    return current;
  }
  if (current === '') {
    return '0.';
  }
  return `${current}.`;
}

// Aplica una tecla al importe y devuelve la nueva cadena. Una tecla inválida para el estado
// actual (segundo punto, tercer decimal, dígito once) deja el importe sin cambios.
export function applyAmountKey(current: string, key: AmountKey): string {
  if (key === 'clear') {
    return '';
  }
  if (key === 'backspace') {
    return current.slice(0, -1);
  }
  if (key === '.') {
    return appendSeparator(current);
  }
  if (key === '00') {
    return appendDigit(appendDigit(current, '0'), '0');
  }
  return appendDigit(current, key);
}

// Traduce una tecla física a una tecla del teclado de cobro. La coma se acepta como punto
// decimal; signos, letras y otros caracteres no tienen equivalente y se ignoran.
export function physicalAmountKey(key: string): AmountKey | null {
  if (isDigit(key)) {
    return key;
  }
  if (key === '.' || key === ',' || key === 'Decimal') {
    return '.';
  }
  if (key === 'Backspace') {
    return 'backspace';
  }
  if (key === 'Escape') {
    return 'clear';
  }
  return null;
}

// Acepta texto pegado o escrito directamente en el campo solo si conserva una forma válida.
// En caso contrario conserva el importe anterior en lugar de corromperlo.
export function acceptAmountText(next: string, previous: string): string {
  let normalized = next.replaceAll(',', '.').trim();
  if (normalized.startsWith('.')) {
    normalized = `0${normalized}`;
  }
  if (partialAmount.test(normalized)) {
    return normalized;
  }
  return previous;
}

// Prepara el importe para Go: "12." se envía como "12". Una cadena vacía sigue vacía.
export function toWireAmount(text: string): string {
  if (text.endsWith('.')) {
    return text.slice(0, -1);
  }
  return text;
}

// Distingue un importe capturado explícitamente como cero ("0", "0.", "0.00") por su forma.
export function isZeroAmountText(text: string): boolean {
  return /^0(\.0{0,2})?$/.test(text);
}
