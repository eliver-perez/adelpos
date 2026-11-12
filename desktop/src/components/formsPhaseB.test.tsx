// @vitest-environment jsdom

// Etapa 6.1 · fase B: componentes compartidos de formularios (posición, color, horarios y alta
// continua).

import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { useState } from 'react';
import { Form } from './Forms';
import { PositionField } from './PositionField';
import { ColorField, colorName } from './catalog/ColorField';
import { ScheduleEditor, emptySchedule } from './catalog/EditorFields';
import { currentAfter, orderSiblings, positionPayload } from '../services/positions';
import { scheduleSummary } from '../services/scheduleSummary';
import type { PaletteColor, Schedule } from '../services/catalog';
import { chooseOption, optionLabels } from '../testing/select';

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const siblings = [
  { id: 'desayunos', name: 'Desayunos' },
  { id: 'bebidas', name: 'Bebidas' },
  { id: 'postres', name: 'Postres' },
];

const palette: PaletteColor[] = [
  { hex: '#d64545', name: 'Rojo' },
  { hex: '#3f9a4a', name: 'Verde' },
  { hex: '#4370d6', name: 'Azul' },
];

describe('selector de posición', () => {
  it('ordena como Go y conserva el orden del servidor en los empates', () => {
    const ordered = orderSiblings([
      { id: 'c', sort_order: 20 },
      { id: 'a', sort_order: 0 },
      { id: 'b', sort_order: 0 },
      { id: 'd', sort_order: 10 },
    ]);
    expect(ordered.map((entry) => entry.id)).toEqual(['a', 'b', 'd', 'c']);
  });

  it('muestra «Al inicio» y «Después de…» sin ofrecer el propio registro', () => {
    render(
      <PositionField
        label="Posición"
        siblings={siblings}
        recordID="bebidas"
        value={undefined}
        onChange={() => {}}
      />,
    );
    const select = screen.getByLabelText('Posición') as HTMLButtonElement;
    expect(optionLabels(select)).toEqual([
      'Al inicio',
      'Después de «Desayunos»',
      'Después de «Postres»',
    ]);
    // Al editar aparece seleccionada la posición actual: después del hermano anterior.
    expect(select.value).toBe('desayunos');
    expect(screen.queryByRole('textbox')).toBeNull();
  });

  it('el primero aparece «Al inicio» y uno nuevo después del último', () => {
    expect(currentAfter(siblings, 'desayunos')).toBe('');
    expect(currentAfter(siblings, '')).toBe('postres');
    expect(currentAfter([], '')).toBe('');
    // Un registro que llega de otra lista (otra categoría) también va al final.
    expect(currentAfter(siblings, 'externo')).toBe('postres');
  });

  it('envía la posición elegida y la omite si el registro conserva su lugar', () => {
    expect(positionPayload(siblings, 'bebidas', undefined)).toBeUndefined();
    expect(positionPayload(siblings, 'bebidas', '')).toEqual({ after: '' });
    expect(positionPayload(siblings, '', undefined)).toEqual({ after: 'postres' });
    expect(positionPayload(siblings, 'externo', undefined)).toEqual({ after: 'postres' });
  });

  it('cambia la posición elegida al seleccionar otra opción', () => {
    let chosen: string | undefined;
    function Editor() {
      const [after, setAfter] = useState<string | undefined>(undefined);
      chosen = after;
      return (
        <PositionField
          label="Posición"
          siblings={siblings}
          recordID="postres"
          value={after}
          onChange={setAfter}
        />
      );
    }
    render(<Editor />);
    const select = screen.getByLabelText('Posición') as HTMLButtonElement;
    expect(select.value).toBe('bebidas');
    chooseOption(select, 'Al inicio');
    expect(chosen).toBe('');
    expect(select.value).toBe('');
  });
});

describe('selector de color', () => {
  function Editor({ initial }: { initial: string }) {
    const [color, setColor] = useState(initial);
    return (
      <>
        <ColorField value={color} palette={palette} onChange={setColor} />
        <output>{color || 'vacío'}</output>
      </>
    );
  }

  it('no admite escritura y elige un color de la paleta', () => {
    render(<Editor initial="" />);
    expect(screen.queryByRole('textbox')).toBeNull();
    const trigger = screen.getByRole('button', { name: 'Color de la categoría: Sin color' });
    expect(trigger.getAttribute('aria-expanded')).toBe('false');
    expect(screen.queryByRole('button', { name: 'Quitar color' })).toBeNull();
    fireEvent.click(trigger);
    const grid = screen.getByRole('group', { name: 'Paleta de colores' });
    const options = within(grid).getAllByRole('button');
    expect(options.map((option) => option.textContent)).toEqual(['Rojo', 'Verde', 'Azul']);
    expect(options.every((option) => option.getAttribute('aria-pressed') === 'false')).toBe(true);
    fireEvent.click(within(grid).getByRole('button', { name: 'Verde' }));
    expect(screen.getByText('#3f9a4a')).toBeTruthy();
    expect(screen.queryByRole('group', { name: 'Paleta de colores' })).toBeNull();
    fireEvent.click(screen.getByRole('button', { name: 'Color de la categoría: Verde' }));
    expect(screen.getByRole('button', { name: 'Verde' }).getAttribute('aria-pressed')).toBe('true');
    expect(screen.getByRole('button', { name: 'Rojo' }).getAttribute('aria-pressed')).toBe('false');
  });

  it('quita el color y vuelve a «Sin color»', () => {
    render(<Editor initial="#d64545" />);
    expect(screen.getByRole('button', { name: 'Color de la categoría: Rojo' })).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: 'Quitar color' }));
    expect(screen.getByText('vacío')).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Color de la categoría: Sin color' })).toBeTruthy();
  });

  it('muestra un color guardado fuera de la paleta como «Color personalizado»', () => {
    render(<Editor initial="#14665c" />);
    const trigger = screen.getByRole('button', {
      name: 'Color de la categoría: Color personalizado',
    });
    fireEvent.click(trigger);
    const grid = screen.getByRole('group', { name: 'Paleta de colores' });
    expect(
      within(grid)
        .getAllByRole('button')
        .some((option) => option.getAttribute('aria-pressed') === 'true'),
    ).toBe(false);
    expect(colorName('#D64545', palette)).toBe('Rojo');
    expect(colorName('', palette)).toBe('Sin color');
  });

  it('Escape cierra la paleta sin cambiar el color', () => {
    render(<Editor initial="#d64545" />);
    fireEvent.click(screen.getByRole('button', { name: 'Color de la categoría: Rojo' }));
    fireEvent.keyDown(screen.getByRole('button', { name: 'Azul' }), { key: 'Escape' });
    expect(screen.queryByRole('group', { name: 'Paleta de colores' })).toBeNull();
    expect(screen.getByText('#d64545')).toBeTruthy();
  });
});

describe('restricciones de horario', () => {
  it('resume el estado del horario', () => {
    expect(scheduleSummary(emptySchedule())).toBe('Disponible siempre');
    expect(
      scheduleSummary({
        days: [1, 2, 3, 4, 5],
        windows: [{ start: '07:00', end: '12:00' }],
      }),
    ).toBe('Lun a vie · 7:00–12:00');
    expect(scheduleSummary({ starts_on: '2026-11-15', days: [], windows: [] })).toBe(
      'Desde 15/11/2026',
    );
    expect(
      scheduleSummary({
        starts_on: '2026-11-15',
        ends_on: '2026-11-30',
        days: [0, 6],
        windows: [],
      }),
    ).toBe('Del 15/11/2026 al 30/11/2026 · Sáb, dom');
    expect(
      scheduleSummary({
        days: [],
        windows: [
          { start: '20:00', end: '24:00' },
          { start: '00:00', end: '02:00' },
        ],
      }),
    ).toBe('Todos los días · 0:00–2:00, 20:00–24:00');
    expect(scheduleSummary({ days: [1, 3, 5], windows: [] })).toBe('Lun, mié, vie');
  });

  it('usa botones táctiles para los días y explica la hora final con ejemplos', () => {
    let latest: Schedule = emptySchedule();
    function Editor() {
      const [schedule, setSchedule] = useState<Schedule>(emptySchedule());
      latest = schedule;
      return <ScheduleEditor value={schedule} onChange={setSchedule} />;
    }
    render(<Editor />);
    const trigger = screen.getByRole('button', { name: /Restricciones de horario/ });
    expect(trigger.textContent).toContain('Disponible siempre');
    expect(screen.queryByText('Fechas, días y horarios')).toBeNull();
    fireEvent.click(trigger);
    const days = screen.getByRole('group', { name: 'Días de la semana' });
    expect(within(days).queryByRole('checkbox')).toBeNull();
    const buttons = within(days).getAllByRole('button');
    expect(buttons.map((button) => button.textContent)).toEqual([
      'Lun',
      'Mar',
      'Mié',
      'Jue',
      'Vie',
      'Sáb',
      'Dom',
    ]);
    fireEvent.click(within(days).getByRole('button', { name: 'Lun' }));
    fireEvent.click(within(days).getByRole('button', { name: 'Dom' }));
    expect(within(days).getByRole('button', { name: 'Lun' }).getAttribute('aria-pressed')).toBe(
      'true',
    );
    expect(latest.days).toEqual([0, 1]);
    expect(trigger.textContent).toContain('Lun, dom');
    expect(screen.getByText(/deja de estar disponible justo a las 15:00/)).toBeTruthy();
    expect(screen.getByText(/20:00 a 24:00 y 00:00 a 02:00/)).toBeTruthy();
    // Los campos de fecha de la fase A se conservan.
    expect((screen.getByLabelText('Desde la fecha') as HTMLInputElement).placeholder).toBe(
      'dd/mm/aaaa',
    );
  });
});

describe('formulario con alta continua', () => {
  it('ofrece «Guardar y crear otra» solo si se indica y distingue ambos envíos', async () => {
    const save = vi.fn(async () => {});
    const saveAndNew = vi.fn(async () => {});
    const { rerender } = render(
      <Form onSave={save} label="Guardar mesa">
        <input aria-label="Nombre" defaultValue="Mesa 1" />
      </Form>,
    );
    expect(screen.queryByRole('button', { name: 'Guardar y crear otra' })).toBeNull();
    rerender(
      <Form onSave={save} onSaveAndNew={saveAndNew} label="Guardar mesa">
        <input aria-label="Nombre" defaultValue="Mesa 1" />
      </Form>,
    );
    const actions = screen.getByRole('button', { name: 'Guardar mesa' }).parentElement!;
    // La barra de acciones es la que la hoja de estilos fija al pie del área desplazable.
    expect(actions.className).toBe('form-actions');
    fireEvent.click(screen.getByRole('button', { name: 'Guardar y crear otra' }));
    await waitFor(() => expect(saveAndNew).toHaveBeenCalledTimes(1));
    expect(save).not.toHaveBeenCalled();
    await screen.findByText('Registro guardado. Captura el siguiente.');
    await waitFor(() => expect(document.activeElement).toBe(screen.getByLabelText('Nombre')));
    fireEvent.click(screen.getByRole('button', { name: 'Guardar mesa' }));
    await waitFor(() => expect(save).toHaveBeenCalledTimes(1));
    expect(saveAndNew).toHaveBeenCalledTimes(1);
  });
});
