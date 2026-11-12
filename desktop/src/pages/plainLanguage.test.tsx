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

describe('textos sin términos técnicos (etapa 9C, §4.3)', () => {
  it('ninguna pantalla muestra nombres de tecnologías ni identificadores internos', () => {
    const sources = import.meta.glob<string>(
      ['../**/*.tsx', '../**/*.ts', '!../**/*.test.tsx', '!../**/*.test.ts', '!../testing/**'],
      { query: '?raw', import: 'default', eager: true },
    );
    expect(Object.keys(sources).length).toBeGreaterThan(150);
    const found: string[] = [];
    for (const [name, source] of Object.entries(sources)) {
      if (supportOnly.some((path) => name.endsWith(path))) {
        continue;
      }
      // Los comentarios del código pueden nombrar las tecnologías: se quitan los de bloque
      // (conservando sus saltos de línea para numerar bien) y se omiten las líneas que empiezan
      // con «//».
      const code = source.replace(/\/\*[\s\S]*?\*\//g, (comment) => comment.replace(/[^\n]/g, ''));
      code.split('\n').forEach((line, index) => {
        if (/^\s*\/\//.test(line)) {
          return;
        }
        for (const [pattern, reason] of forbidden) {
          if (pattern.test(line)) {
            found.push(`${name}:${index + 1} (${reason}): ${line.trim()}`);
          }
        }
      });
    }
    expect(found).toEqual([]);
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
