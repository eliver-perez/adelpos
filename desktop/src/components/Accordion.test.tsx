// @vitest-environment jsdom

import { act, cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { Accordion } from './Accordion';
import { Form } from './Forms';

afterEach(cleanup);

describe('Accordion compartido', () => {
  it('reabre un error del servidor si se cerró mientras el formulario guardaba', async () => {
    let reject!: (error: Error) => void;
    const pending = new Promise<void>((_, rejectPromise) => {
      reject = rejectPromise;
    });
    render(
      <Accordion title="Datos" defaultOpen>
        <Form onSave={() => pending}>
          <input aria-label="Dato" />
        </Form>
      </Accordion>,
    );
    fireEvent.click(screen.getByRole('button', { name: 'Guardar cambios' }));
    fireEvent.click(screen.getByRole('button', { name: 'Datos' }));
    await act(async () => reject(new Error('La revisión cambió')));
    expect(screen.getByRole('button', { name: 'Datos' }).getAttribute('aria-expanded')).toBe(
      'true',
    );
    // El error del servidor es un aviso persistente del sistema único de mensajes.
    expect((await screen.findByRole('alert', { name: 'Error' })).textContent).toContain(
      'La revisión cambió',
    );
  });

  it('conserva los campos montados y sus valores al cerrar y abrir', () => {
    render(
      <Accordion title="Programación">
        <label>
          Fecha
          <input defaultValue="original" />
        </label>
      </Accordion>,
    );
    const button = screen.getByRole('button', { name: 'Programación' });
    const field = screen.getByLabelText('Fecha');
    fireEvent.click(button);
    fireEvent.change(field, { target: { value: 'conservado' } });
    fireEvent.click(button);
    expect(button.getAttribute('aria-expanded')).toBe('false');
    expect(field.isConnected).toBe(true);
    expect(field.closest('[inert]')).not.toBeNull();
    fireEvent.click(button);
    expect((field as HTMLInputElement).value).toBe('conservado');
    expect(screen.getByRole('region', { name: 'Programación' }).id).toBe(
      button.getAttribute('aria-controls'),
    );
  });

  it('revela y enfoca una validación nativa dentro de una sección cerrada', () => {
    render(
      <Accordion title="Datos">
        <input aria-label="Obligatorio" required />
      </Accordion>,
    );
    const field = screen.getByLabelText('Obligatorio');
    fireEvent.invalid(field);
    expect(screen.getByRole('button').getAttribute('aria-expanded')).toBe('true');
    expect(document.activeElement).toBe(field);
  });

  it('revela errores del servidor sin recrear el control', () => {
    const { rerender } = render(
      <Accordion title="Datos">
        <input aria-label="Dato" />
      </Accordion>,
    );
    const original = screen.getByLabelText('Dato');
    rerender(
      <Accordion title="Datos" hasError>
        <input aria-label="Dato" />
      </Accordion>,
    );
    expect(screen.getByRole('button').getAttribute('aria-expanded')).toBe('true');
    expect(screen.getByLabelText('Dato')).toBe(original);
  });
});
