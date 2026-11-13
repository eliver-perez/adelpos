import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';

// Etapa 9D, fases B y C (§2 y §3): reglas de estilo de la barra superior y de Mesas que no se ven
// en jsdom (las comprueba también Playwright con los colores calculados). Todos los colores salen
// de los tokens de la fase A.

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

describe('barra superior (§2)', () => {
  it('las cuatro secciones forman columnas del mismo ancho y los botones miden 52 px', () => {
    const sections = rule('.topbar-sections');
    expect(sections).toContain('display: grid;');
    expect(sections).toContain('grid-auto-flow: column;');
    expect(sections).toContain('grid-auto-columns: 1fr;');
    expect(rule('.topbar-button')).toContain('height: 52px;');
    expect(rule('.cash-shortcut-bar .cash-shortcut-button')).toContain('height: 52px;');
  });

  it('indicadores: turno abierto y en línea en azul, arqueo ámbar, sin turno gris, sin conexión rojo', () => {
    expect(rule('.cash-shortcut-open .cash-shortcut-indicator')).toContain(
      'background: var(--cash-entry);',
    );
    expect(rule('.cash-shortcut-closing .cash-shortcut-indicator')).toContain(
      'background: var(--warning-mark);',
    );
    expect(rule('.cash-shortcut-none .cash-shortcut-indicator')).toContain(
      'background: var(--muted);',
    );
    expect(rule('.cash-shortcut-open .cash-shortcut-state')).toContain('color: var(--cash-entry);');
    expect(rule('.cash-shortcut-closing .cash-shortcut-state')).toContain(
      'color: var(--cash-exit);',
    );
    expect(rule('.app-topbar .server-status-online .server-status-text')).toContain(
      'color: var(--status-online);',
    );
    expect(rule('.server-status-offline')).toContain('color: var(--danger);');
    // Caja en dos renglones y su menú alineado a la derecha (Caja está a la derecha de la barra).
    expect(rule('.cash-shortcut-lines')).toContain('display: grid;');
    expect(rule('.cash-shortcut-bar .cash-shortcut-menu')).toContain('right: 0;');
  });

  it('si falta espacio solo se acorta el nombre del usuario', () => {
    expect(rule('.topbar-end > *')).toContain('flex-shrink: 0;');
    expect(rule('.topbar-end > .user-menu')).toContain('flex-shrink: 1;');
  });
});

describe('Mesas (§3)', () => {
  it('panel lateral de ancho fijo y mapa a la misma altura', () => {
    const split = rule('.operation-map-split');
    expect(split).toContain('--table-panel-width: 360px;');
    expect(split).toContain('grid-template-columns: minmax(0, 1fr) var(--table-panel-width);');
    expect(split).toContain('align-content: stretch;');
    expect(rule('.table-map-operation .table-map-viewport')).toContain('overflow: auto;');
  });

  // Desde la etapa 9F (§6.2) la leyenda ya no se queda en una sola línea con desplazamiento
  // horizontal: se acomoda en dos renglones (ver stage9fLayout.test.ts).
  it('leyenda dentro del marco del mapa, sin desplazamiento horizontal', () => {
    const legend = rule('.map-legend');
    expect(legend).toContain('border-top: 1px solid var(--border);');
    expect(legend).not.toContain('overflow-x: auto;');
  });

  it('«Cobrar» fuera de la captura con contorno azul, sin relleno', () => {
    const outline = rule('.capture-checkout-outline:not(:disabled)');
    expect(outline).toContain('border: 2px solid var(--accent);');
    expect(outline).toContain('background: var(--panel);');
    expect(outline).toContain('color: var(--accent);');
  });

  it('advertencia ámbar discreta de la cuenta abierta por mucho tiempo', () => {
    expect(rule('.table-tile-time.table-tile-long-open')).toContain('color: var(--warning);');
    expect(rule('.restaurant-table.table-long-open::after')).toContain(
      'background: var(--warning-mark);',
    );
    expect(rule('.table-panel-long-open')).toContain('color: var(--warning);');
    expect(rule('.account-list-card.account-long-open')).toContain(
      'border-left-color: var(--warning-mark);',
    );
  });

  it('las mesas resaltadas desde el resumen no se confunden con la seleccionada', () => {
    expect(rule('.restaurant-table.table-highlighted')).toContain(
      'outline: 3px dashed var(--accent);',
    );
    expect(rule('.restaurant-table.selected-table')).toContain('outline: 3px solid var(--accent);');
  });
});
