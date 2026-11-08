// @vitest-environment jsdom

import { useState } from 'react';
import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { VirtualKeyboard } from './VirtualKeyboard';

afterEach(cleanup);

function Example({ mode = 'always' }: { mode?: 'always' | 'disabled' | 'auto' }) {
  const [value, setValue] = useState('Mesa');
  return (
    <>
      <input aria-label="Nombre" value={value} onChange={(event) => setValue(event.target.value)} />
      <input aria-label="Cantidad" type="number" step="1" />
      <input aria-label="Clave" type="password" />
      <output>{value}</output>
      <VirtualKeyboard mode={mode} />
    </>
  );
}

describe('teclado virtual de terminal', () => {
  it('edita una selección y actualiza React sin impedir la escritura física', () => {
    render(<Example />);
    const input = screen.getByLabelText('Nombre') as HTMLInputElement;
    fireEvent.focusIn(input);
    input.setSelectionRange(0, 4);
    fireEvent.click(screen.getByRole('button', { name: 'Escribir ñ' }));
    expect(screen.getByRole('status').textContent).toBe('ñ');
    fireEvent.change(input, { target: { value: 'Físico' } });
    expect(screen.getByRole('status').textContent).toBe('Físico');
    fireEvent.click(screen.getByRole('button', { name: 'Listo' }));
    expect(document.documentElement.style.getPropertyValue('--virtual-keyboard-height')).toBe(
      '0px',
    );
  });

  it('utiliza teclado numérico para cantidades y excluye contraseñas', () => {
    render(<Example />);
    fireEvent.focusIn(screen.getByLabelText('Cantidad'));
    expect(screen.getByRole('group', { name: 'Teclado virtual numérico' })).toBeTruthy();
    expect(screen.queryByRole('button', { name: 'Espacio' })).toBeNull();
    fireEvent.focusIn(screen.getByLabelText('Clave'));
    expect(screen.queryByRole('group')).toBeNull();
  });

  it('excluye de forma explícita los campos marcados y conserva los de referencia', () => {
    render(
      <>
        <input aria-label="Importe" inputMode="decimal" data-virtual-keyboard="off" />
        <div data-virtual-keyboard="off">
          <input aria-label="Efectivo recibido" inputMode="decimal" />
        </div>
        <input aria-label="Referencia" />
        <VirtualKeyboard mode="always" />
      </>,
    );
    fireEvent.focusIn(screen.getByLabelText('Importe'));
    expect(screen.queryByRole('group')).toBeNull();
    fireEvent.focusIn(screen.getByLabelText('Efectivo recibido'));
    expect(screen.queryByRole('group')).toBeNull();
    fireEvent.focusIn(screen.getByLabelText('Referencia'));
    expect(screen.getByRole('group', { name: 'Teclado virtual alfanumérico' })).toBeTruthy();
    fireEvent.focusIn(screen.getByLabelText('Importe'));
    expect(screen.queryByRole('group')).toBeNull();
  });

  it('respeta la preferencia desactivada', () => {
    render(<Example mode="disabled" />);
    fireEvent.focusIn(screen.getByLabelText('Nombre'));
    expect(screen.queryByRole('group')).toBeNull();
  });
});
