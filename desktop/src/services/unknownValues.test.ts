import { readdirSync, readFileSync, statSync } from 'node:fs';
import { join, relative, sep } from 'node:path';
import { describe, expect, it } from 'vitest';
import { auditActionLabel } from './auditActions';
import { cashJobLabel, shiftStateLabel } from './cash';
import { preparationLabel } from './kitchen';
import {
  deliveryStateBadge,
  deliveryStateDescription,
  paymentStateBadge,
  paymentStateDescription,
} from './payments';
import { eventLabel } from './restaurant';

// Etapa A3 (§1, decisión A3-D22): un valor que llega de Go y que esta versión del escritorio no conoce
// (una acción, un tipo de movimiento o un estado nuevos) se dice en general, en palabras del
// restaurante, y nunca con su código. En el Registro de movimientos el código sigue en «Ver cambios».

const unknown = 'estado_nuevo.de_go';

const helpers: [string, (value: string) => string, string, string][] = [
  ['Registro de movimientos', auditActionLabel, 'session.login', 'Inicio de sesión'],
  ['historial de la cuenta', eventLabel, 'account.split', 'División de cuenta'],
  ['preparación de la cuenta', preparationLabel, 'ready', 'Listo'],
  ['pago en Cuentas y Para llevar', paymentStateBadge, 'partial', 'Abono'],
  ['pago en la cuenta abierta', paymentStateDescription, 'paid', 'pagado por completo'],
  ['entrega en Para llevar', deliveryStateBadge, 'delivered', 'Entregado'],
  ['entrega en la cuenta abierta', deliveryStateDescription, 'not_delivered', 'no entregado'],
  ['turno de caja', shiftStateLabel, 'closing', 'Cierre en curso'],
  ['impresión de un documento de caja', cashJobLabel, 'uncertain', 'Resultado incierto'],
];

describe('valores que esta versión no conoce (etapa A3, A3-D22)', () => {
  it.each(helpers)('%s: el nombre conocido o un texto general', (_, label, known, knownText) => {
    expect(label(known)).toBe(knownText);
    const text = label(unknown);
    expect(text).not.toContain(unknown);
    expect(text).not.toMatch(/[a-z]+[._][a-z]+/);
    expect(text.length).toBeGreaterThan(3);
  });

  // Las pantallas del Registro de movimientos, de la cuenta (historial, lista, captura y documentos) y de
  // Caja no vuelven a usar el código como respaldo de un mapa de nombres («nombres[x] ?? x»).
  it('las pantallas no muestran el código como respaldo de un nombre', () => {
    const pages = join(__dirname, '../pages');
    const files: string[] = [join(pages, 'AuditPage.tsx')];
    const walk = (directory: string) => {
      for (const entry of readdirSync(directory)) {
        const path = join(directory, entry);
        if (statSync(path).isDirectory()) {
          walk(path);
        } else if (/\.tsx?$/.test(entry) && !/\.test\.tsx?$/.test(entry)) {
          files.push(path);
        }
      }
    };
    walk(join(pages, 'restaurant'));
    walk(join(pages, 'cash'));
    expect(files.length).toBeGreaterThan(20);
    const found: string[] = [];
    for (const path of files) {
      const text = readFileSync(path, 'utf8');
      for (const match of text.matchAll(/\w+\[([\w.!?]+)\]\s*\?\?\s*([\w.!?]+)/g)) {
        if (match[1] === match[2]) {
          const line = text.slice(0, match.index).split('\n').length;
          found.push(`${relative(pages, path).split(sep).join('/')}:${line} ${match[0]}`);
        }
      }
    }
    expect(found).toEqual([]);
  });
});
