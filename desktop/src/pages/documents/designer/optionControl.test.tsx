// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { OptionSpec } from '../../../services/documents';
import { block, catalog, kitchenKind } from './designerTestData';
import { OptionControl } from './OptionControl';

// Revisión de la etapa 9B (hallazgo medio 3): «Importes» de la lista de cocina solo ofrece lo que
// declara el catálogo de Go («Según la configuración de Cocina» u «Ocultar», nunca «Sí») y muestra
// la explicación de Go de cómo imprimir los importes.

const help =
  'Para imprimir los importes, activa «Incluir importes comerciales en comandas» en Cocina → ' +
  'Comandas y tiempos; con «Ocultar», esta plantilla no los imprime aunque Cocina los muestre.';

const prices: OptionSpec = {
  key: 'show_prices',
  label: 'Importes',
  type: 'enum',
  default: 'inherit',
  values: [
    { value: 'inherit', label: 'Según la configuración de Cocina' },
    { value: 'no', label: 'Ocultar' },
  ],
  help,
};

afterEach(cleanup);

describe('opciones de la lista de cocina', () => {
  it('«Importes» ofrece solo los valores del catálogo y explica cómo imprimirlos', () => {
    const onChange = vi.fn();
    render(
      <OptionControl
        spec={prices}
        block={block('lista', 'kitchen_items', { show_prices: 'inherit' })}
        kind={kitchenKind}
        catalog={catalog}
        disabled={false}
        onChange={onChange}
      />,
    );
    const group = screen.getByRole('group', { name: 'Importes' });
    const choices = Array.from(group.querySelectorAll('button')).map(
      (button) => button.textContent,
    );
    expect(choices).toEqual(['Según la configuración de Cocina', 'Ocultar']);
    expect(screen.queryByRole('button', { name: 'Sí' })).toBeNull();
    expect(screen.getByText(help)).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: 'Ocultar' }));
    expect(onChange).toHaveBeenCalledWith('show_prices', 'no');
  });

  it('una opción sin explicación no agrega texto', () => {
    const { container } = render(
      <OptionControl
        spec={{ ...prices, help: undefined }}
        block={block('lista', 'kitchen_items', {})}
        kind={kitchenKind}
        catalog={catalog}
        disabled={false}
        onChange={() => undefined}
      />,
    );
    expect(container.querySelector('p')).toBeNull();
  });
});
