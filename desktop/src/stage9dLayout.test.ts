import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';

// Etapa 9D, fase B (§2): reglas de estilo de la barra superior que no se ven
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
