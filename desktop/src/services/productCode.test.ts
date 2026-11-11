import { describe, expect, it } from 'vitest';
import {
  codeFromName,
  followsSuggestion,
  freeCode,
  inventoryItemCodeLength,
  suggestProductCode,
  suggestVariantCode,
} from './productCode';

// Etapa 9A (§3.4): la regla de códigos de la etapa 6.1 con un largo máximo por registro. Los mismos
// casos de sufijos están en backend/internal/inventory/suppliers_test.go (TestSuggestFreeCode),
// porque Go aplica la misma regla al comprobar los códigos de insumos.
describe('código interno sugerido', () => {
  it('forma el código con el largo de cada registro', () => {
    expect(codeFromName('Harina de Trigo')).toBe('harina-de-trigo');
    const long = 'Salsa de chile guajillo con ajo tostado y comino';
    expect(codeFromName(long)).toBe('salsa-de-chile-guajillo-con-ajo-tostado-y-comino');
    const item = codeFromName(long, inventoryItemCodeLength);
    expect(item.length).toBeLessThanOrEqual(40);
    // Se recorta a 40 y no termina en guion.
    expect(item).toBe('salsa-de-chile-guajillo-con-ajo-tostado');
  });

  it('agrega sufijos libres sin pasar del máximo, igual que Go', () => {
    expect(freeCode('harina-de-trigo', ['HARINA-DE-TRIGO'], 40)).toBe('harina-de-trigo-2');
    expect(freeCode('harina-de-trigo', ['harina-de-trigo', 'harina-de-trigo-2'], 40)).toBe(
      'harina-de-trigo-3',
    );
    expect(freeCode('x'.repeat(40), ['x'.repeat(40)], 40)).toBe(`${'x'.repeat(38)}-2`);
    expect(freeCode(`${'a'.repeat(37)}-bc`, [`${'a'.repeat(37)}-bc`], 40)).toBe(
      `${'a'.repeat(37)}-2`,
    );
    expect(freeCode('C-HAR', ['c-har', 'c-har-2', 'c-har-3'], 40)).toBe('C-HAR-4');
    expect(freeCode('', ['algo'])).toBe('');
    // Los productos conservan su largo de 60.
    expect(suggestProductCode('Machacado Natural', ['machacado-natural'])).toBe(
      'machacado-natural-2',
    );
  });

  it('propone el código de una presentación con el del producto', () => {
    expect(suggestVariantCode('cafe-de-olla', 'Grande', [])).toBe('cafe-de-olla-grande');
    expect(suggestVariantCode('CAFE-01', 'Grande', ['cafe-01-grande'])).toBe('cafe-01-grande-2');
    expect(suggestVariantCode('', 'Media orden', [])).toBe('media-orden');
    expect(suggestVariantCode('cafe-de-olla', '***', [])).toBe('');
  });

  it('solo sigue al nombre mientras el código no se editó', () => {
    expect(followsSuggestion('', 'harina')).toBe(true);
    expect(followsSuggestion('harina', 'harina')).toBe(true);
    expect(followsSuggestion('HAR-01', 'harina')).toBe(false);
  });
});
