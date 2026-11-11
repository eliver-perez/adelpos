import { useLayoutEffect, useState } from 'react';
import type { RefObject } from 'react';
import type { ColumnType } from '../../services/reports';

// Escalas y medidas de los gráficos SVG propios de Reportes (sin librerías). Los números de este
// archivo son solo geometría: posición de marcas, rejilla y rótulos de los ejes. Los valores de los
// datos siempre se rotulan con el texto que entrega Go (reportValues.formatReportValue).

// Marcas «redondas» del eje: 0, 1, 2, 2.5 o 5 × 10^n, con el cero incluido.
export function niceTicks(minimum: number, maximum: number, count = 4) {
  let low = Math.min(0, minimum);
  let high = Math.max(0, maximum);
  if (low === high) {
    high = low + 1;
  }
  const rough = (high - low) / Math.max(1, count);
  const magnitude = 10 ** Math.floor(Math.log10(rough));
  const steps = [1, 2, 2.5, 5, 10];
  const step = (steps.find((candidate) => candidate * magnitude >= rough) ?? 10) * magnitude;
  low = Math.floor(low / step) * step;
  high = Math.ceil(high / step) * step;
  const ticks: number[] = [];
  for (let value = low; value <= high + step / 2; value += step) {
    ticks.push(Math.abs(value) < step / 1e6 ? 0 : value);
  }
  return { low, high, ticks };
}

const compact = new Intl.NumberFormat('es-MX', { notation: 'compact', maximumFractionDigits: 1 });
const full = new Intl.NumberFormat('es-MX', { maximumFractionDigits: 2 });

// Rótulo de una marca del eje: completo por debajo de 10 000 y compacto a partir de ahí.
export function axisLabel(value: number, type: ColumnType) {
  const text = Math.abs(value) >= 10000 ? compact.format(value) : full.format(value);
  const signless = text.replace('-', '');
  const sign = value < 0 ? '−' : '';
  if (type === 'money_mxn') {
    return `${sign}$${signless}`;
  }
  if (type === 'money_usd') {
    return `${sign}US$${signless}`;
  }
  if (type === 'percent') {
    return `${sign}${signless} %`;
  }
  return `${sign}${signless}`;
}

// Ancho real del contenedor para dibujar en píxeles (sin deformar el texto del SVG). En entornos
// sin ResizeObserver (pruebas) se usa el ancho de respaldo.
export function useElementWidth(ref: RefObject<HTMLElement | null>, fallback: number) {
  const [width, setWidth] = useState(fallback);
  // Antes de pintar, para que el primer cuadro ya tenga el ancho real.
  useLayoutEffect(() => {
    const element = ref.current;
    if (!element) {
      return;
    }
    const measure = () => {
      const measured = element.getBoundingClientRect().width;
      if (measured > 0) {
        setWidth(Math.floor(measured));
      }
    };
    measure();
    if (typeof ResizeObserver === 'undefined') {
      return;
    }
    const observer = new ResizeObserver(measure);
    observer.observe(element);
    return () => observer.disconnect();
  }, [ref]);
  return width;
}

// Trayecto de una barra horizontal con el extremo de datos redondeado (4 px) y el de la línea base
// recto, hacia la derecha o hacia la izquierda según el signo.
export function barPath(baseline: number, end: number, top: number, height: number, radius = 4) {
  const length = Math.abs(end - baseline);
  const r = Math.min(radius, length, height / 2);
  if (length === 0) {
    return '';
  }
  if (end >= baseline) {
    return [
      `M${baseline},${top}`,
      `H${end - r}`,
      `Q${end},${top} ${end},${top + r}`,
      `V${top + height - r}`,
      `Q${end},${top + height} ${end - r},${top + height}`,
      `H${baseline}`,
      'Z',
    ].join(' ');
  }
  return [
    `M${baseline},${top}`,
    `H${end + r}`,
    `Q${end},${top} ${end},${top + r}`,
    `V${top + height - r}`,
    `Q${end},${top + height} ${end + r},${top + height}`,
    `H${baseline}`,
    'Z',
  ].join(' ');
}
