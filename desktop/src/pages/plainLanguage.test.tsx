// @vitest-environment jsdom

import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { initialSettings, SettingsFields } from './SettingsPage';
import { UserPreferencesForm } from './PreferencesPage';

vi.mock('@tauri-apps/api/core', () => ({ invoke: vi.fn(), isTauri: () => true }));

afterEach(cleanup);

// Etapa 9C (§4.3 y §4.4): textos visibles sin términos técnicos y las dos apariencias explicadas.

// Frases técnicas que ya no deben aparecer en textos visibles. «Información para soporte» (Estado
// del sistema) y su diagnóstico conservan los datos técnicos a propósito (§4.1).
const forbidden: [RegExp, string][] = [
  [/PostgreSQL/, 'nombre de la base de datos'],
  [/\bIANA\b/, 'identificador de zona horaria'],
  [/persistid|persistente/, '«persistente»'],
  [/mismo identificador/, '«mismo identificador»'],
  [/\bGo (valida|calcula|la convierte|lo convierte|ejecuta|no pudo)/, 'Go como autor'],
  [/(por|de) Go\b/, 'Go como autor'],
  [/\bTauri\b/, 'nombre de la tecnología de escritorio'],
  [/en el servidor\b/, '«en el servidor»'],
  [/\bKDS\b/, 'sigla KDS'],
];
const supportOnly = ['StatusPage.tsx', 'services/diagnostics.ts'];

// Fuentes de la interfaz: sin las pruebas, sus utilidades (`testing/`) ni los datos que solo usan
// las pruebas (`*TestData.ts`).
function interfaceSources() {
  return import.meta.glob<string>(
    [
      '../**/*.tsx',
      '../**/*.ts',
      '!../**/*.test.tsx',
      '!../**/*.test.ts',
      '!../testing/**',
      '!../**/*TestData.ts',
    ],
    { query: '?raw', import: 'default', eager: true },
  );
}

// Fuentes del puente nativo (Rust): sus mensajes de error también llegan a la pantalla. Sin el
// módulo de pruebas, que por convención va al final de cada archivo con #[cfg(test)].
function nativeSources() {
  const sources = import.meta.glob<string>('../../src-tauri/src/**/*.rs', {
    query: '?raw',
    import: 'default',
    eager: true,
  });
  const withoutTests: Record<string, string> = {};
  for (const [key, source] of Object.entries(sources)) {
    withoutTests[key] = source.split(/^#\[cfg\(test\)\]/m)[0]!;
  }
  return withoutTests;
}

// Ruta de una fuente: las claves de import.meta.glob son relativas a esta carpeta (pages/); las de
// desktop/src quedan relativas a desktop/src y las del puente nativo, a desktop/.
function sourcePath(key: string) {
  if (key.startsWith('./')) {
    return `pages/${key.slice(2)}`;
  }
  return key.replace(/^\.\.\/(\.\.\/)?/, '');
}

// Recorre las líneas de código de una fuente, sin los comentarios: se quitan los de bloque
// (conservando sus saltos de línea para numerar bien) y se omiten las líneas que empiezan con «//».
function forEachCodeLine(source: string, visit: (line: string, number: number) => void) {
  const code = source.replace(/\/\*[\s\S]*?\*\//g, (comment) => comment.replace(/[^\n]/g, ''));
  code.split('\n').forEach((line, index) => {
    if (/^\s*\/\//.test(line)) {
      return;
    }
    visit(line, index + 1);
  });
}

describe('textos sin términos técnicos (etapa 9C, §4.3)', () => {
  it('ninguna pantalla muestra nombres de tecnologías ni identificadores internos', () => {
    const sources = interfaceSources();
    expect(Object.keys(sources).length).toBeGreaterThan(150);
    const found: string[] = [];
    for (const [name, source] of Object.entries(sources)) {
      if (supportOnly.some((path) => name.endsWith(path))) {
        continue;
      }
      forEachCodeLine(source, (line, number) => {
        for (const [pattern, reason] of forbidden) {
          if (pattern.test(line)) {
            found.push(`${name}:${number} (${reason}): ${line.trim()}`);
          }
        }
      });
    }
    expect(found).toEqual([]);
  });
});

// Etapa A3 (§1 de docs/etapas/ETAPA_A3_AJUSTES.md): ningún texto provisional o de desarrollo en la
// interfaz: nada de «todavía no…», notas de etapas, «próximamente», «provisional» ni identificadores
// internos a la vista. Las mismas frases las busca la prueba de Go en los mensajes que Go manda a la
// pantalla (backend/internal/apicontract/plain_language_test.go) y la prueba de textos de la
// aplicación Android.
const developmentPhrases: [RegExp, string][] = [
  [/todav[ií]a no\b/i, '«todavía no»'],
  [/\ba[uú]n no\b/i, '«aún no»'],
  [/\betapa\b/i, 'nota de etapas'],
  [/\bfase [0-9a-z]\b/i, 'nota de fases'],
  [/pr[oó]ximamente/i, '«próximamente»'],
  [/provisional/i, '«provisional»'],
  [/\bpor ahora\b/i, '«por ahora»'],
  [/\bm[aá]s adelante\b/i, '«más adelante»'],
  [/\ben desarrollo\b/i, '«en desarrollo»'],
  [/\bversi[oó]n de prueba\b/i, '«versión de prueba»'],
  [/\(muestra\)/i, '«(muestra)»'],
  [/Código: </, 'identificador interno a la vista'],
  [/\{[A-Za-z_.?]*\.(id|[a-z_]*_id)\}\s*<\//, 'identificador interno a la vista'],
];

// Las únicas excepciones, cada una con su porqué: agregar una exige escribir el motivo. Una
// excepción que deja de hacer falta hace fallar la prueba, para que la lista no crezca sin control.
const allowedDevelopmentText: { file: string; line: RegExp; reason: string }[] = [
  {
    file: 'services/inventory.ts',
    line: /^\s*\| 'provisional'$/,
    reason:
      'código de la base del costo que manda Go (el costo estimado de un faltante); la interfaz nunca lo muestra crudo',
  },
  {
    file: 'pages/DevicesPage.tsx',
    line: /^\s*<dd className="font-mono">\{shown\.id\}<\/dd>$/,
    reason:
      '«Identificador» de la terminal solo en «Ver configuración» (§5.3 de la A3: salió de la tabla); es el dato que piden soporte y el filtro por terminal del Registro de movimientos',
  },
];

describe('textos sin notas de desarrollo (etapa A3, §1)', () => {
  it('ninguna pantalla muestra textos provisionales, notas de etapas ni identificadores internos', () => {
    const native = nativeSources();
    expect(Object.keys(native).length).toBeGreaterThan(10);
    const sources = { ...interfaceSources(), ...native };
    expect(Object.keys(sources).length).toBeGreaterThan(160);
    const found: string[] = [];
    const used = new Set<number>();
    for (const [key, source] of Object.entries(sources)) {
      const name = sourcePath(key);
      forEachCodeLine(source, (line, number) => {
        for (const [pattern, reason] of developmentPhrases) {
          if (!pattern.test(line)) {
            continue;
          }
          const allowed = allowedDevelopmentText.findIndex(
            (exception) => name === exception.file && exception.line.test(line),
          );
          if (allowed >= 0) {
            used.add(allowed);
            continue;
          }
          found.push(`${name}:${number} (${reason}): ${line.trim()}`);
        }
      });
    }
    expect(found).toEqual([]);
    const unused = allowedDevelopmentText
      .filter((_, index) => !used.has(index))
      .map((exception) => `${exception.file}: ${exception.line} ya no hace falta`);
    expect(unused).toEqual([]);
  });
});

describe('apariencia (etapa 9C, §4.4)', () => {
  it('Configuración general la presenta como la predeterminada para usuarios nuevos', () => {
    render(<SettingsFields value={initialSettings} onChange={() => undefined} />);
    const appearance = screen.getByRole('combobox', {
      name: 'Apariencia predeterminada para usuarios nuevos',
    });
    expect(appearance.getAttribute('aria-describedby')).toBeTruthy();
    expect(
      screen.getByText(
        'Cada usuario la cambia en Mis preferencias; los usuarios que ya existen conservan la suya.',
      ),
    ).toBeTruthy();
    expect(screen.getByRole('combobox', { name: 'Zona horaria del restaurante' })).toBeTruthy();
  });

  it('en el asistente inicial explica que también es la del administrador', () => {
    render(<SettingsFields setup value={initialSettings} onChange={() => undefined} />);
    expect(
      screen.getByText(
        'También es la de tu usuario administrador. Cada usuario la cambia en Mis preferencias.',
      ),
    ).toBeTruthy();
  });

  it('Mis preferencias muestra la apariencia propia, la que se aplica', () => {
    render(
      <UserPreferencesForm
        value={{ theme: 'dark', home_screen: 'status' }}
        onSaved={async () => undefined}
      />,
    );
    const appearance = screen.getByRole('combobox', { name: 'Apariencia' });
    expect(appearance.getAttribute('value')).toBe('dark');
    expect(screen.getByText(/Se aplica en cuanto la guardas/)).toBeTruthy();
  });
});
