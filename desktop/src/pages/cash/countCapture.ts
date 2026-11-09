import { acceptIntegerText } from '../../services/numericInput';
import type { CashCount, CountLineInput, Denomination } from '../../services/cash';

// Captura local de un conteo por denominaciones: cantidades enteras de billetes y monedas tal como
// se teclean. Ningún importe se calcula aquí: Go multiplica, suma y valida cada conteo.

// Cantidad máxima por denominación que admite Go (100000): seis dígitos.
export const quantityDigits = 6;

export type CountQuantities = Record<string, string>;

// Acepta solo dígitos (cantidades físicas, no dinero) y descarta ceros a la izquierda.
export function acceptQuantity(next: string, previous: string) {
  return acceptIntegerText(next, previous, quantityDigits);
}

// Denominaciones que se capturan: las activas de pesos y, si el turno maneja dólares, las de
// dólares. Conserva el orden de Go (moneda, billetes antes que monedas y valor descendente).
export function capturableDenominations(denominations: Denomination[], dollars: boolean) {
  return denominations.filter((denomination) => {
    if (!denomination.active) {
      return false;
    }
    return denomination.currency === 'MXN' || dollars;
  });
}

// Líneas que se envían a Go: solo las denominaciones con cantidad capturada mayor que cero.
export function countLines(
  quantities: CountQuantities,
  denominations: Denomination[],
): CountLineInput[] {
  const lines: CountLineInput[] = [];
  for (const denomination of denominations) {
    const text = quantities[denomination.id] ?? '';
    if (text === '' || /^0+$/.test(text)) {
      continue;
    }
    lines.push({ denomination_id: denomination.id, quantity: Number.parseInt(text, 10) });
  }
  return lines;
}

// Cantidades de un conteo guardado en el servidor, para retomar un borrador del arqueo.
export function quantitiesOf(count: CashCount | null): CountQuantities {
  const quantities: CountQuantities = {};
  for (const line of count?.lines ?? []) {
    if (line.quantity > 0) {
      quantities[line.denomination_id] = String(line.quantity);
    }
  }
  return quantities;
}

// Importe adicional guardado: un cero se muestra como campo vacío.
export function extraText(value: string | undefined) {
  if (!value || /^0+(\.0+)?$/.test(value)) {
    return '';
  }
  return value;
}
