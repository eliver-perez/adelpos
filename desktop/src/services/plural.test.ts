import { describe, expect, it } from 'vitest';
import cases from '../../../backend/internal/plural/testdata/cases.json';
import { countLabel, pluralWord } from './plural';

// Misma tabla de casos que las pruebas de Go (backend/internal/plural).
describe('concordancia de contadores', () => {
  it.each(cases.count)('countLabel($count, "$singular")', (entry) => {
    expect(countLabel(entry.count, entry.singular, entry.plural)).toBe(entry.text);
  });

  it('pluralWord devuelve solo la palabra que concuerda', () => {
    expect(pluralWord(1, 'abierta', 'abiertas')).toBe('abierta');
    expect(pluralWord(0, 'abierta', 'abiertas')).toBe('abiertas');
    expect(pluralWord(5, 'abierta', 'abiertas')).toBe('abiertas');
  });
});
