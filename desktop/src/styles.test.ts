import { readdirSync, readFileSync, statSync } from 'node:fs';
import { join, relative, sep } from 'node:path';
import { describe, expect, it } from 'vitest';

// Etapa 9D (§1.1 y §7): el color de la interfaz se define en un solo lugar, los bloques de tokens
// `:root` y `:root[data-theme='dark']` de styles.css. Esta prueba recorre src/**/*.{ts,tsx,css} y
// falla si aparece un color escrito directamente fuera de esos bloques (hexadecimal, rgb()/hsl()…,
// nombre de color de CSS o clase de color fijo de Tailwind). También comprueba los valores de la
// marca, el contraste de los pares principales en ambos temas y que el verde salió de la interfaz.

const sourceRoot = __dirname;
const styles = readFileSync(join(sourceRoot, 'styles.css'), 'utf8');

// Archivos que no son interfaz: pruebas, ayudantes de pruebas y datos de prueba. Contienen colores
// como datos (la paleta cerrada de categorías que define Go, p. ej. «#d64545») o textos que parecen
// colores («Calle 5 #120»); ninguno se pinta en la aplicación.
function excluded(path: string) {
  const name = path.split(sep).pop() ?? '';
  return (
    name.includes('.test.') ||
    name.endsWith('TestData.ts') ||
    path.split(sep).includes('testing') ||
    path.split(sep).includes('generated')
  );
}

function sources(directory: string): string[] {
  return readdirSync(directory).flatMap((entry) => {
    const path = join(directory, entry);
    if (statSync(path).isDirectory()) {
      return sources(path);
    }
    return /\.(tsx?|css)$/.test(entry) && !excluded(path) ? [path] : [];
  });
}

// Excepciones justificadas. Los demás colores que no dependen del tema (paleta de los gráficos con
// su serie verde, papel simulado del diseñador, fondo blanco de las vistas previas del logotipo y
// marca compacta) son tokens del bloque `:root`; la paleta de colores de categorías no está escrita
// en la interfaz: es un dato que entrega Go (`backend/internal/catalog/palette.go`) y los
// componentes la aplican desde el dato (CategoryColumn, ProductGrid, ColorField).
const exceptions = [
  {
    file: 'styles.css',
    pattern: /conic-gradient\(from -45deg at bottom, #0000, #000 1deg 89deg, #0000 90deg\)/,
    reason:
      'máscara del borde dentado del papel simulado (.paper-sheet): solo define opacidad, no un color',
    uses: 2,
  },
];

const hexColor = /(?<![\w&-])#(?:[0-9a-fA-F]{8}|[0-9a-fA-F]{6}|[0-9a-fA-F]{3,4})\b/;
const functionalColor = /\b(?:rgba?|hsla?|hwb|lab|lch|oklab|oklch)\(/;
const namedColors =
  'white|black|red|green|blue|gray|grey|silver|maroon|navy|teal|olive|lime|aqua|cyan|magenta|fuchsia|purple|orange|yellow|pink|brown|gold|indigo|violet';
const cssNamedColor = new RegExp(`(?:^|[\\s,(])(?:${namedColors})(?:$|[\\s,;)])`, 'i');
const tailwindColor = new RegExp(
  `\\b(?:bg|text|border|fill|stroke|ring|outline|from|to|via|shadow|accent|caret|decoration|divide|placeholder)-(?:${namedColors}|slate|zinc|neutral|stone|amber|emerald|sky|rose)(?:-\\d{2,3})?\\b`,
);
const tailwindArbitraryColor = /-\[(?:#|rgb|hsl)/;
const jsxNamedColor = new RegExp(
  `\\b(?:fill|stroke|color|stopColor|background|backgroundColor|borderColor)\\s*[=:]\\s*\\{?\\s*["'](?:${namedColors})["']`,
  'i',
);

// Quita los comentarios (/* … */ y los de línea completa //) sin mover los números de línea: los
// comentarios pueden citar colores sin pintarlos.
function withoutComments(source: string) {
  const blocks = source.replace(/\/\*[\s\S]*?\*\//g, (comment) => comment.replace(/[^\n]/g, ' '));
  return blocks.replace(/^\s*\/\/.*$/gm, '');
}

// Intervalo (en caracteres) de un bloque de tokens de styles.css.
function blockRange(css: string, selector: string): [number, number] {
  const start = css.indexOf(`\n${selector} {\n`);
  expect(start, `falta el bloque ${selector}`).toBeGreaterThan(0);
  const end = css.indexOf('\n}', start + 1);
  return [start, end + 2];
}

const tokenBlocks = [blockRange(styles, ':root'), blockRange(styles, ":root[data-theme='dark']")];

function insideTokens(offset: number) {
  return tokenBlocks.some(([start, end]) => offset >= start && offset < end);
}

interface Finding {
  file: string;
  line: number;
  text: string;
}

function cssFindings(file: string, source: string): Finding[] {
  const clean = withoutComments(source);
  const findings: Finding[] = [];
  let offset = 0;
  clean.split('\n').forEach((line, index) => {
    const lineStart = offset;
    offset += line.length + 1;
    if (file === 'styles.css' && insideTokens(lineStart)) {
      return;
    }
    const value = line.includes(':') ? line.slice(line.indexOf(':') + 1) : line;
    if (hexColor.test(line) || functionalColor.test(line) || cssNamedColor.test(value)) {
      findings.push({ file, line: index + 1, text: line.trim() });
    }
  });
  return findings;
}

function tsxFindings(file: string, source: string): Finding[] {
  const findings: Finding[] = [];
  withoutComments(source)
    .split('\n')
    .forEach((line, index) => {
      if (
        hexColor.test(line) ||
        functionalColor.test(line) ||
        tailwindColor.test(line) ||
        tailwindArbitraryColor.test(line) ||
        jsxNamedColor.test(line)
      ) {
        findings.push({ file, line: index + 1, text: line.trim() });
      }
    });
  return findings;
}

describe('colores escritos directamente (etapa 9D, §1.1)', () => {
  it('ningún archivo de la interfaz escribe un color fuera de los tokens', () => {
    const files = sources(sourceRoot);
    expect(files.length).toBeGreaterThan(200);
    const findings = files.flatMap((path) => {
      const file = relative(sourceRoot, path).split(sep).join('/');
      const source = readFileSync(path, 'utf8');
      return file.endsWith('.css') ? cssFindings(file, source) : tsxFindings(file, source);
    });
    const used = new Map<string, number>();
    const unexpected = findings.filter((finding) => {
      const exception = exceptions.find(
        (entry) => entry.file === finding.file && entry.pattern.test(finding.text),
      );
      if (exception) {
        used.set(exception.reason, (used.get(exception.reason) ?? 0) + 1);
        return false;
      }
      return true;
    });
    expect(unexpected.map((finding) => `${finding.file}:${finding.line} ${finding.text}`)).toEqual(
      [],
    );
    // Cada excepción sigue siendo necesaria (no quedan excepciones obsoletas).
    for (const exception of exceptions) {
      expect(used.get(exception.reason), exception.reason).toBe(exception.uses);
    }
  });

  it('el detector reconoce los colores escritos a mano que retiró la etapa 9D', () => {
    expect(cssFindings('otro.css', '.brand-mark {\n  background: #1e3d37;\n}')).toHaveLength(1);
    expect(
      cssFindings('otro.css', '.x {\n  box-shadow: 0 0 0 4px rgb(0 0 0 / 12%);\n}'),
    ).toHaveLength(1);
    expect(cssFindings('otro.css', '.x {\n  color: white;\n}')).toHaveLength(1);
    expect(cssFindings('otro.css', '.x {\n  white-space: nowrap;\n}')).toHaveLength(0);
    expect(tsxFindings('x.tsx', '<span className="text-sm text-[#5b6675]">')).toHaveLength(1);
    expect(tsxFindings('x.tsx', '<div className="rounded bg-white p-3">')).toHaveLength(1);
    expect(tsxFindings('x.tsx', '<div className="bg-[var(--logo-paper)] p-3">')).toHaveLength(0);
    expect(tsxFindings('x.tsx', '<path fill="none" stroke="currentColor" />')).toHaveLength(0);
    expect(tsxFindings('x.tsx', "<circle fill='white' />")).toHaveLength(1);
  });
});

// Tokens de cada tema con las referencias var() resueltas. El tema oscuro hereda del claro lo que
// no redefine (ambos bloques se aplican al mismo elemento <html>).
function declarations(selector: string) {
  const [start, end] = blockRange(styles, selector);
  const block = withoutComments(styles.slice(start, end));
  const values = new Map<string, string>();
  for (const match of block.matchAll(/(--[\w-]+)\s*:\s*([^;]+);/g)) {
    values.set(match[1]!, match[2]!.trim());
  }
  return values;
}

const light = declarations(':root');
const dark = new Map([...light, ...declarations(":root[data-theme='dark']")]);
const themes = { claro: light, oscuro: dark };

function resolve(tokens: Map<string, string>, name: string, depth = 0): string {
  expect(depth, `referencia circular en ${name}`).toBeLessThan(10);
  const value = tokens.get(name);
  expect(value, `falta el token ${name}`).toBeDefined();
  const reference = /^var\((--[\w-]+)\)$/.exec(value!);
  return reference ? resolve(tokens, reference[1]!, depth + 1) : value!.toLowerCase();
}

function channels(hex: string): [number, number, number] {
  const digits = hex.replace('#', '');
  const full =
    digits.length === 3
      ? digits
          .split('')
          .map((digit) => digit + digit)
          .join('')
      : digits;
  expect(full, `color no hexadecimal: ${hex}`).toMatch(/^[0-9a-f]{6}$/);
  return [0, 2, 4].map((position) => parseInt(full.slice(position, position + 2), 16)) as [
    number,
    number,
    number,
  ];
}

// Luminancia relativa y razón de contraste de WCAG 2.x.
function luminance(hex: string) {
  const [red, green, blue] = channels(hex).map((channel) => {
    const value = channel / 255;
    return value <= 0.04045 ? value / 12.92 : ((value + 0.055) / 1.055) ** 2.4;
  }) as [number, number, number];
  return 0.2126 * red + 0.7152 * green + 0.0722 * blue;
}

function contrast(first: string, second: string) {
  const [lighter, darker] = [luminance(first), luminance(second)].sort((a, b) => b - a) as [
    number,
    number,
  ];
  return (lighter + 0.05) / (darker + 0.05);
}

// Tono (0–360) y saturación (0–1) en HSL.
function hueAndSaturation(hex: string): [number, number] {
  const [red, green, blue] = channels(hex).map((channel) => channel / 255) as [
    number,
    number,
    number,
  ];
  const max = Math.max(red, green, blue);
  const min = Math.min(red, green, blue);
  const delta = max - min;
  const lightness = (max + min) / 2;
  if (delta === 0) {
    return [0, 0];
  }
  const saturation = delta / (1 - Math.abs(2 * lightness - 1));
  let hue = 0;
  if (max === red) {
    hue = ((green - blue) / delta) % 6;
  } else if (max === green) {
    hue = (blue - red) / delta + 2;
  } else {
    hue = (red - green) / delta + 4;
  }
  return [(hue * 60 + 360) % 360, saturation];
}

// Pares principales: texto (4.5:1, WCAG AA) y elementos gráficos o bordes que identifican un
// control o un estado (3:1, WCAG 1.4.11).
const textPairs: [string, string][] = [
  ['--text', '--panel'],
  ['--text', '--bg'],
  ['--muted', '--panel'],
  ['--muted', '--bg'],
  ['--muted', '--accent-soft'],
  ['--accent', '--panel'],
  ['--accent', '--bg'],
  ['--accent', '--accent-soft'],
  ['--on-accent', '--accent'],
  ['--on-accent', '--accent-strong'],
  ['--danger', '--panel'],
  ['--danger', '--danger-soft'],
  ['--danger-contrast', '--danger'],
  ['--warning', '--panel'],
  ['--warning', '--warning-soft'],
  ['--info', '--panel'],
  ['--info', '--info-soft'],
  ['--grape', '--panel'],
  ['--grape', '--grape-soft'],
  ['--gray', '--panel'],
  ['--gray', '--gray-soft'],
  ['--cash-entry', '--cash-entry-soft'],
  ['--cash-exit', '--cash-exit-soft'],
  ['--cash-closing', '--cash-closing-soft'],
  ['--table-free-text', '--table-free-bg'],
  ['--table-occupied-text', '--table-occupied-bg'],
  ['--table-awaiting-text', '--table-awaiting-bg'],
  ['--table-partial-text', '--table-partial-bg'],
  ['--table-out-text', '--table-out-bg'],
];

const graphicPairs: [string, string][] = [
  ['--accent', '--subtle'],
  ['--status-online', '--panel'],
  ['--warning-mark', '--panel'],
  ['--table-free-border', '--subtle'],
  ['--table-occupied-border', '--subtle'],
  ['--table-occupied-border', '--table-occupied-bg'],
  ['--table-awaiting-border', '--subtle'],
  ['--table-partial-border', '--subtle'],
  ['--table-partial-border', '--table-partial-bg'],
];

describe('tokens de color (etapa 9D, §1)', () => {
  it('el azul de la marca y sus variantes del tema claro son los de la especificación', () => {
    expect(resolve(light, '--accent')).toBe('#1f5fa8');
    expect(resolve(light, '--accent-strong')).toBe('#173d6e');
    expect(resolve(light, '--accent-soft')).toBe('#e7eef7');
    expect(resolve(light, '--accent-border')).toBe('#a9c2e2');
    expect(resolve(dark, '--brand-blue-light')).toBe('#4090f0');
    // El tema oscuro aclara el azul del logotipo sin cambiar de familia.
    const [hue] = hueAndSaturation(resolve(dark, '--accent'));
    expect(Math.abs(hue - hueAndSaturation('#4090f0')[0])).toBeLessThan(4);
  });

  for (const [theme, tokens] of Object.entries(themes)) {
    it(`contraste AA de los pares principales en el tema ${theme}`, () => {
      const failures: string[] = [];
      for (const [foreground, background] of textPairs) {
        const ratio = contrast(resolve(tokens, foreground), resolve(tokens, background));
        if (ratio < 4.5) {
          failures.push(`${foreground} sobre ${background}: ${ratio.toFixed(2)}`);
        }
      }
      for (const [foreground, background] of graphicPairs) {
        const ratio = contrast(resolve(tokens, foreground), resolve(tokens, background));
        if (ratio < 3) {
          failures.push(`${foreground} junto a ${background}: ${ratio.toFixed(2)}`);
        }
      }
      expect(failures).toEqual([]);
    });

    it(`el verde salió de la interfaz en el tema ${theme} (salvo la serie de los gráficos)`, () => {
      const green: string[] = [];
      for (const name of tokens.keys()) {
        const value = resolve(tokens, name);
        if (!/^#[0-9a-f]{3,6}$/.test(value) || name === '--chart-3') {
          continue;
        }
        const [hue, saturation] = hueAndSaturation(value);
        if (hue >= 75 && hue <= 170 && saturation > 0.2) {
          green.push(`${name} ${value}`);
        }
      }
      expect(green).toEqual([]);
    });

    it(`uva (pago parcial) se distingue del azul (ocupada) en el tema ${theme}`, () => {
      const [blue] = hueAndSaturation(resolve(tokens, '--accent'));
      const [grape] = hueAndSaturation(resolve(tokens, '--grape'));
      expect(Math.abs(grape - blue)).toBeGreaterThan(45);
      const [occupied] = hueAndSaturation(resolve(tokens, '--table-occupied-bg'));
      const [partial] = hueAndSaturation(resolve(tokens, '--table-partial-bg'));
      expect(Math.abs(partial - occupied)).toBeGreaterThan(45);
    });
  }

  it('la paleta de estado de §0-B se asigna a sus usos', () => {
    const uses: [string, string][] = [
      ['--accent', '--blue'],
      ['--status-online', '--blue'],
      ['--cash-entry', '--blue'],
      ['--warning', '--amber'],
      ['--cash-exit', '--amber'],
      ['--cash-closing', '--amber'],
      ['--table-partial-border', '--grape'],
      ['--cash-report', '--gray'],
      ['--info', '--teal'],
      ['--danger', '--red'],
    ];
    for (const tokens of Object.values(themes)) {
      for (const [use, palette] of uses) {
        expect(resolve(tokens, use), use).toBe(resolve(tokens, palette));
      }
    }
  });
});

// Cuerpo de la primera regla cuyo único selector es el indicado (no una lista de selectores).
function rule(selector: string) {
  let start = styles.indexOf(`\n${selector} {\n`);
  while (start > 0 && styles.slice(0, start).trimEnd().endsWith(',')) {
    start = styles.indexOf(`\n${selector} {\n`, start + 1);
  }
  expect(start, `falta la regla ${selector}`).toBeGreaterThan(0);
  return styles.slice(start, styles.indexOf('\n}', start + 1));
}

describe('reglas que usan los tokens (etapa 9D)', () => {
  it('avisos: éxito azul, información turquesa, advertencia ámbar y error rojo', () => {
    expect(rule('.app-toast')).toContain('--toast-accent: var(--accent);');
    expect(rule('.app-toast-info')).toContain('--toast-accent: var(--info);');
    expect(rule('.app-toast-warning')).toContain('--toast-accent: var(--warning);');
    expect(rule('.app-toast-error')).toContain('--toast-accent: var(--danger);');
    expect(rule('.app-toast-icon')).toContain('color: var(--toast-accent);');
  });

  it('total y saldo en el color del texto y en negritas (§1.4)', () => {
    for (const selector of [
      '.quote-panel h2.quote-total',
      '.checkout-summary-balance dd',
      '.checkout-figure-emphasis dd',
      '.amount-field-value',
      '.capture-totals .capture-grand-total dd',
      '.table-panel-totals .table-panel-balance dd',
    ]) {
      const body = rule(selector);
      expect(body, selector).toContain('color: var(--text);');
      expect(body, selector).toMatch(/font-weight: (700|800);/);
      expect(body, selector).not.toContain('var(--accent)');
    }
  });

  it('sección activa con el fondo suave y su borde; tarjeta seleccionada con borde azul', () => {
    const active = rule('.topbar-section-active');
    expect(active).toContain('background: var(--accent-soft);');
    expect(active).toContain('border-color: var(--accent-border);');
    expect(active).toContain('color: var(--accent);');
    const card = rule('.account-list-card.account-list-selected');
    expect(card).toContain('background: var(--accent-soft);');
    expect(card).toContain('border-color: var(--accent);');
  });

  it('acciones destructivas en rojo y botón principal azul con su presionado', () => {
    expect(rule('.danger-button')).toContain('color: var(--danger);');
    expect(rule('.danger-solid-button')).toContain('background: var(--danger);');
    expect(rule('.primary-button')).toContain('background: var(--accent);');
    expect(rule('.primary-button:active:not(:disabled)')).toContain(
      'background: var(--accent-strong);',
    );
  });

  it('controles deshabilitados con «no disponible» y progreso solo con aria-busy (§1.6)', () => {
    expect(styles).not.toMatch(/cursor: (wait|default);/);
    const disabled = styles.slice(styles.indexOf('\nbutton:disabled,\n'));
    expect(disabled.slice(0, disabled.indexOf('}'))).toContain('cursor: not-allowed;');
    const busy = styles.slice(styles.lastIndexOf("\n[aria-busy='true'],\n"));
    expect(busy).toContain('cursor: progress;');
    // La regla de progreso es la última del archivo: prevalece sobre las variantes de «not-allowed».
    expect(styles.indexOf('cursor: not-allowed;', styles.lastIndexOf("[aria-busy='true']"))).toBe(
      -1,
    );
  });

  it('estados de mesa con sus tokens y selección con borde azul más grueso en ocupada', () => {
    for (const state of ['free', 'occupied', 'awaiting', 'partial']) {
      expect(styles).toContain(`var(--table-${state}-bg)`);
    }
    expect(rule('.restaurant-table.selected-table')).toContain('outline: 3px solid var(--accent);');
    const occupied = rule('.restaurant-table.table-occupied.selected-table');
    expect(occupied).toContain('border-color: var(--accent);');
    expect(occupied).toContain('outline-width: 4px;');
  });
});
