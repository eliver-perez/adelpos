import { describe, expect, it } from 'vitest';
import cases from '../../../backend/internal/moneytext/testdata/cases.json';
import {
  formatDeduction,
  formatDenomination,
  formatDollars,
  formatMoney,
  formatSignedMoney,
} from './money';
import type { MoneyCurrency } from './money';

// La tabla de casos es la misma que usan las pruebas de Go (backend/internal/moneytext). Go
// imprime el negativo con el guion ASCII; la pantalla usa el menos tipográfico U+2212.
interface MoneyCase {
  currency: string;
  amount: string;
  printed: string;
}

function onScreen(printed: string) {
  return printed.replace('-', '−');
}

function currencyOf(entry: MoneyCase): MoneyCurrency {
  return entry.currency === 'USD' ? 'USD' : 'MXN';
}

describe('formato único de importes', () => {
  it.each(cases.format as MoneyCase[])('formatMoney($currency, "$amount")', (entry) => {
    expect(formatMoney(entry.amount, currencyOf(entry))).toBe(onScreen(entry.printed));
  });

  it.each(cases.signed as MoneyCase[])('formatSignedMoney($currency, "$amount")', (entry) => {
    expect(formatSignedMoney(entry.amount, currencyOf(entry))).toBe(onScreen(entry.printed));
  });

  it.each(cases.denomination as MoneyCase[])(
    'formatDenomination($currency, "$amount")',
    (entry) => {
      expect(formatDenomination(entry.amount, currencyOf(entry))).toBe(onScreen(entry.printed));
    },
  );

  it('usa pesos por omisión y muestra «—» sin valor', () => {
    expect(formatMoney('1500')).toBe('$1,500.00');
    expect(formatMoney(null)).toBe('—');
    expect(formatMoney(undefined)).toBe('—');
    expect(formatSignedMoney(null)).toBe('—');
    expect(formatDenomination(undefined)).toBe('—');
  });

  it('formatDollars equivale al formato en dólares', () => {
    expect(formatDollars('2500.5')).toBe('US$2,500.50');
    expect(formatDollars('-7')).toBe('−US$7.00');
    expect(formatDollars('')).toBe('—');
  });

  it('formatDeduction antepone el menos a un importe que se resta', () => {
    expect(formatDeduction('50')).toBe('\u2212$50.00');
    expect(formatDeduction('1250.5')).toBe('\u2212$1,250.50');
    expect(formatDeduction('0.00')).toBe('$0.00');
    expect(formatDeduction('-15')).toBe('+$15.00');
    expect(formatDeduction('12', 'USD')).toBe('\u2212US$12.00');
    expect(formatDeduction('')).toBe('—');
  });

  it('no pasa por number: conserva cifras que un number redondearía', () => {
    // 9007199254740993 no es representable como number (se volvería …992).
    expect(formatMoney('9007199254740993.01')).toBe('$9,007,199,254,740,993.01');
    expect(formatMoney('0.30000000000000004')).toBe('$0.30000000000000004');
    expect(formatMoney('123456789012345678901234567890.12')).toBe(
      '$123,456,789,012,345,678,901,234,567,890.12',
    );
  });

  it('la reexportación de payments.ts es la misma función', async () => {
    const payments = await import('./payments');
    expect(payments.formatMoney).toBe(formatMoney);
    expect(payments.formatDollars).toBe(formatDollars);
  });
});
