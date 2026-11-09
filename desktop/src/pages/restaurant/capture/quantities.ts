// Cantidades de porciones e ingredientes (no dinero). Go las transporta como cadenas decimales
// con hasta tres decimales; la captura táctil las convierte a milésimas enteras para comparar,
// sumar unidades completas y decidir qué botón está seleccionado sin usar coma flotante. Los
// importes nunca pasan por estas funciones.

const pattern = /^(\d{1,7})(?:\.(\d{1,3}))?$/;

// «1.500» → 1500. Un valor no reconocido se trata como cero: Go rechazaría la selección.
export function toThousandths(value: string | null | undefined): number {
  const match = pattern.exec((value ?? '').trim());
  if (!match) {
    return 0;
  }
  const whole = Number(match[1]);
  const fraction = Number((match[2] ?? '').padEnd(3, '0'));
  return whole * 1000 + fraction;
}

// 1500 → «1.5»; 2000 → «2». Se usa la forma más corta que Go acepta.
export function fromThousandths(value: number): string {
  const safe = Math.max(0, Math.trunc(value));
  const whole = Math.floor(safe / 1000);
  const fraction = safe % 1000;
  if (fraction === 0) {
    return String(whole);
  }
  return `${whole}.${String(fraction).padStart(3, '0').replace(/0+$/, '')}`;
}

export const unit = 1000;

// Texto de una cantidad para mostrarla: «1», «0.5».
export function displayQuantity(value: string | null | undefined): string {
  return fromThousandths(toThousandths(value));
}
