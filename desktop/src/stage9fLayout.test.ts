import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';

// Etapa 9F, fase D (§6): barra superior compacta y leyenda del mapa de mesas. Reglas de estilo que
// jsdom no aplica; Playwright las comprueba en el navegador a 1200, 1280, 1440, 1500 y 1920 px
// (workflow-stage9f-topbar-map.spec.ts).

const styles = readFileSync(join(__dirname, 'styles.css'), 'utf8');

// Cuerpo de la primera regla cuyo único selector es el indicado.
function rule(selector: string) {
  let start = styles.indexOf(`\n${selector} {\n`);
  while (start > 0 && styles.slice(0, start).trimEnd().endsWith(',')) {
    start = styles.indexOf(`\n${selector} {\n`, start + 1);
  }
  expect(start, `falta la regla ${selector}`).toBeGreaterThan(0);
  return styles.slice(start, styles.indexOf('\n}', start + 1));
}

// Cuerpo de la única consulta de medios con esa condición (termina en la llave de la columna 0).
function media(condition: string) {
  const header = `\n@media ${condition} {\n`;
  const start = styles.indexOf(header);
  expect(start, `falta @media ${condition}`).toBeGreaterThan(0);
  expect(styles.indexOf(header, start + 1), `@media ${condition} repetida`).toBe(-1);
  return styles.slice(start, styles.indexOf('\n}', start + header.length));
}

describe('barra compacta (§6.1)', () => {
  it('debajo de 1280 px: isotipo, estado corto de Caja, «En línea» con texto y el usuario', () => {
    const compact = media('(max-width: 1279px)');
    expect(compact).toContain(
      '.topbar-logo .brand-logo .brand-logo-compact {\n    display: block;',
    );
    expect(compact).toContain('.cash-shortcut-bar .cash-shortcut-state-full {\n    display: none;');
    expect(compact).toContain(
      '.cash-shortcut-bar .cash-shortcut-state-short {\n    display: inline;',
    );
    // «En línea», el estado del turno y el nombre del usuario ya no se ocultan en la compacta.
    expect(compact).not.toContain('server-status-text');
    expect(compact).not.toContain('user-menu-identity');
    expect(compact).not.toContain('.cash-shortcut-bar .cash-shortcut-state {');
    // El estado corto no se ve fuera de la compacta.
    expect(rule('.cash-shortcut-state-short')).toContain('display: none;');
  });

  it('debajo de 1150 px: puntos con ayuda emergente y el usuario como icono', () => {
    const narrow = media('(max-width: 1149px)');
    expect(narrow).toContain('.app-topbar .user-menu-identity,');
    expect(narrow).toContain('.app-topbar .server-status-online .server-status-text,');
    expect(narrow).toContain('.cash-shortcut-bar .cash-shortcut-state {\n    display: none;');
  });

  it('la «A·» provisional y sus colores ya no existen', () => {
    expect(styles).not.toContain('.brand-mark');
    expect(styles).not.toContain('--brand-mark');
  });

  it('el isotipo es un PNG cuadrado de 128 px reducido del ícono del instalador', () => {
    const image = readFileSync(join(__dirname, 'assets', 'adel-pos-isotipo.png'));
    // Firma PNG y medidas del encabezado IHDR (ancho y alto en los bytes 16–23).
    expect(image.subarray(0, 8).toString('hex')).toBe('89504e470d0a1a0a');
    expect(image.subarray(12, 16).toString('ascii')).toBe('IHDR');
    expect(image.readUInt32BE(16)).toBe(128);
    expect(image.readUInt32BE(20)).toBe(128);
    expect(image.length).toBeLessThan(32 * 1024);
  });
});

describe('leyenda del mapa (§6.2)', () => {
  it('se acomoda en dos renglones sin barra horizontal y cada estado queda junto', () => {
    const legend = rule('.map-legend');
    expect(legend).toContain('flex-wrap: wrap;');
    expect(legend).not.toContain('overflow-x');
    expect(legend).not.toContain('white-space: nowrap;');
    const states = rule('.map-legend-states');
    expect(states).toContain('flex-wrap: wrap;');
    expect(states).toContain('min-width: 0;');
    // Muestra, icono y texto de cada estado en un mismo renglón.
    const state = rule('.map-legend-state');
    expect(state).toContain('display: inline-flex;');
    expect(state).toContain('white-space: nowrap;');
  });
});
