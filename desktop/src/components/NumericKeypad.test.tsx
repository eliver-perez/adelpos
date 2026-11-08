// @vitest-environment jsdom

import { useState } from 'react';
import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { AmountField, NumericKeypad } from './NumericKeypad';
import {
  acceptAmountText,
  applyAmountKey,
  isZeroAmountText,
  toWireAmount,
} from '../services/amountInput';
import type { AmountKey } from '../services/amountInput';

afterEach(cleanup);

function typeKeys(keys: AmountKey[], start = '') {
  return keys.reduce((value, key) => applyAmountKey(value, key), start);
}

function Example() {
  const [amount, setAmount] = useState('');
  const [reference, setReference] = useState('');
  return (
    <>
      <AmountField label="Importe" value={amount} onChange={setAmount} active />
      <input
        aria-label="Referencia"
        value={reference}
        onChange={(event) => setReference(event.target.value)}
      />
      <NumericKeypad value={amount} onChange={setAmount} label="Teclado de cobro" captureKeyboard />
      <output data-testid="importe">{amount}</output>
    </>
  );
}

function amountText() {
  return screen.getByTestId('importe').textContent;
}

describe('validación textual de importes (S7)', () => {
  it('ignora el segundo punto, el tercer decimal y el undécimo dígito entero', () => {
    expect(typeKeys(['1', '2', '.', '.', '5'])).toBe('12.5');
    expect(typeKeys(['1', '.', '2', '5', '9'])).toBe('1.25');
    expect(typeKeys(['1', '2', '3', '4', '5', '6', '7', '8', '9', '0', '1'])).toBe('1234567890');
  });

  it('maneja vacío y cero sin ceros a la izquierda', () => {
    expect(typeKeys(['.'])).toBe('0.');
    expect(typeKeys(['0', '0'])).toBe('0');
    expect(typeKeys(['0', '5'])).toBe('5');
    expect(typeKeys(['00'])).toBe('0');
    expect(typeKeys(['5', '00'])).toBe('500');
    expect(typeKeys(['5', '.', '1', '00'])).toBe('5.10');
    expect(typeKeys(['backspace'])).toBe('');
    expect(typeKeys(['0', '.', 'backspace'])).toBe('0');
    expect(typeKeys(['9', '.', '5', 'clear'])).toBe('');
  });

  it('rechaza signos y cadenas mal formadas conservando el importe anterior', () => {
    expect(acceptAmountText('+5', '12')).toBe('12');
    expect(acceptAmountText('-5', '12')).toBe('12');
    expect(acceptAmountText('1e3', '12')).toBe('12');
    expect(acceptAmountText('12.5.1', '12')).toBe('12');
    expect(acceptAmountText('05', '12')).toBe('12');
    expect(acceptAmountText('12.345', '12.34')).toBe('12.34');
    expect(acceptAmountText('12,5', '12')).toBe('12.5');
    expect(acceptAmountText('.5', '')).toBe('0.5');
    expect(acceptAmountText('', '12')).toBe('');
    expect(toWireAmount('12.')).toBe('12');
    expect(toWireAmount('')).toBe('');
    expect(isZeroAmountText('0.00')).toBe(true);
    expect(isZeroAmountText('10')).toBe(false);
  });
});

describe('teclado numérico fijo de cobro (S7)', () => {
  it('escribe sobre el estado del importe con teclas táctiles', () => {
    render(<Example />);
    for (const key of ['Punto decimal', '5', 'Punto decimal', '0', '7', '9']) {
      fireEvent.click(screen.getByRole('button', { name: key }));
    }
    expect(amountText()).toBe('0.50');
    fireEvent.click(screen.getByRole('button', { name: 'Borrar último carácter' }));
    expect(amountText()).toBe('0.5');
    fireEvent.click(screen.getByRole('button', { name: 'Limpiar importe' }));
    expect(amountText()).toBe('');
    fireEvent.click(screen.getByRole('button', { name: 'Doble cero' }));
    fireEvent.click(screen.getByRole('button', { name: '0' }));
    expect(amountText()).toBe('0');
  });

  it('admite el teclado físico sin que + o un doble punto corrompan el importe', () => {
    render(<Example />);
    const input = screen.getByLabelText('Importe') as HTMLInputElement;
    expect(input.type).toBe('text');
    expect(input.inputMode).toBe('decimal');
    expect(input.dataset.virtualKeyboard).toBe('off');
    for (const key of ['1', '2', '+', ',', '.', '5', '-', '0', '3', 'e']) {
      fireEvent.keyDown(input, { key });
    }
    expect(amountText()).toBe('12.50');
    expect(input.value).toBe('12.50');
    fireEvent.keyDown(input, { key: 'Backspace' });
    expect(amountText()).toBe('12.5');
    fireEvent.change(input, { target: { value: '12.5+' } });
    expect(amountText()).toBe('12.5');
    fireEvent.keyDown(input, { key: 'Escape' });
    expect(amountText()).toBe('');
  });

  it('no intercepta la escritura física en otros campos como la referencia', () => {
    render(<Example />);
    const reference = screen.getByLabelText('Referencia');
    fireEvent.keyDown(reference, { key: '7' });
    expect(amountText()).toBe('');
    fireEvent.keyDown(document.body, { key: '7' });
    expect(amountText()).toBe('7');
  });

  it('agrupa teclas de cobro como botones sin depender de un input simulado', () => {
    render(<Example />);
    const keypad = screen.getByRole('group', { name: 'Teclado de cobro' });
    const keys = keypad.querySelectorAll('button');
    expect(keys).toHaveLength(14);
    keys.forEach((key) => expect(key.getAttribute('type')).toBe('button'));
  });
});
