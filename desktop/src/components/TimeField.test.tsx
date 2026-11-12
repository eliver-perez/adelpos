// @vitest-environment jsdom

import { useState } from 'react';
import { act, cleanup, fireEvent, render, screen, within } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import {
  DateTimeField,
  TimeField,
  formatLocalDateTime,
  hourOptions,
  minuteOptions,
  splitTime,
} from './TimeField';
import type { MinuteStep } from './TimeField';
import { chooseOption, openSelect, optionLabels, selectText, selectValue } from '../testing/select';

afterEach(cleanup);

// Etapa 9C (§2.1): selector de hora con dos selectores de la fase B, 24 h, pasos de 5 o 1, sin
// escritura libre; «24:00» solo en la hora final de las restricciones y variante con fecha.

function Time({
  initial = '',
  step,
  endOfDay = false,
}: {
  initial?: string;
  step?: MinuteStep;
  endOfDay?: boolean;
}) {
  const [value, setValue] = useState(initial);
  return (
    <>
      <TimeField
        label="Hora inicial"
        hint="En la zona horaria del restaurante."
        value={value}
        step={step}
        endOfDay={endOfDay}
        onChange={setValue}
      />
      <output aria-label="Valor">{value}</output>
    </>
  );
}

function DateTime({ initial = '', defaultTime = '' }: { initial?: string; defaultTime?: string }) {
  const [value, setValue] = useState(initial);
  return (
    <form aria-label="Programación">
      <DateTimeField
        label="Fecha y hora"
        value={value}
        defaultTime={defaultTime}
        onChange={setValue}
      />
      <output aria-label="Valor">{value}</output>
      <button type="button" onClick={() => setValue('2027-01-02T08:15')}>
        Cambiar desde fuera
      </button>
    </form>
  );
}

function group(name: string) {
  return screen.getByRole('group', { name });
}

function hour(name = 'Hora inicial') {
  return within(group(name)).getByRole('combobox', { name: 'Hora' });
}

function minutes(name = 'Hora inicial') {
  return within(group(name)).getByRole('combobox', { name: 'Minutos' });
}

function output() {
  return screen.getByLabelText('Valor').textContent;
}

describe('selector de hora (etapa 9C, §2.1)', () => {
  it('hora y minutos son dos selectores de 24 h, de 5 en 5, sin campo de escritura', () => {
    render(<Time />);
    const field = group('Hora inicial');
    expect(field.getAttribute('aria-describedby')).toBeTruthy();
    expect(field.querySelector('input:not([aria-hidden="true"])')).toBeNull();
    expect(field.querySelector('input[type="time"], input[type="number"]')).toBeNull();
    expect(selectText(hour())).toBe('--');
    expect(optionLabels(hour())).toEqual(
      Array.from({ length: 24 }, (_, index) => String(index).padStart(2, '0')),
    );
    expect(optionLabels(minutes())).toEqual([
      '00',
      '05',
      '10',
      '15',
      '20',
      '25',
      '30',
      '35',
      '40',
      '45',
      '50',
      '55',
    ]);
    // Elegir la hora sin minutos la deja en punto; después se ajustan los minutos.
    chooseOption(hour(), '14');
    expect(output()).toBe('14:00');
    chooseOption(minutes(), '35');
    expect(output()).toBe('14:35');
    expect(selectValue(hour())).toBe('14');
    expect(selectValue(minutes())).toBe('35');
  });

  it('de 1 en 1 donde hace falta precisión', () => {
    render(<Time step={1} />);
    const labels = optionLabels(minutes());
    expect(labels).toHaveLength(60);
    expect(labels[7]).toBe('07');
  });

  it('conserva una hora guardada que no cae en el paso', () => {
    render(<Time initial="05:07" />);
    expect(selectText(hour())).toBe('05');
    expect(selectText(minutes())).toBe('07');
    expect(optionLabels(minutes())).toContain('07');
    expect(optionLabels(minutes())).toHaveLength(13);
    // Cambiar la hora no toca los minutos guardados.
    chooseOption(hour(), '06');
    expect(output()).toBe('06:07');
    chooseOption(minutes(), '10');
    expect(output()).toBe('06:10');
    expect(optionLabels(minutes())).not.toContain('07');
  });

  it('«24:00» solo existe en la hora final y deja los minutos en 00', () => {
    render(<Time initial="22:30" endOfDay />);
    const hours = optionLabels(hour());
    expect(hours).toHaveLength(25);
    expect(hours.at(-1)).toBe('24');
    const list = openSelect(hour());
    expect(within(list).getByText('Medianoche, fin del día')).toBeTruthy();
    chooseOption(hour(), /^24/);
    expect(output()).toBe('24:00');
    expect((minutes() as HTMLButtonElement).disabled).toBe(true);
    chooseOption(hour(), '23');
    expect(output()).toBe('23:00');
    expect((minutes() as HTMLButtonElement).disabled).toBe(false);
    cleanup();
    render(<Time initial="09:00" />);
    expect(optionLabels(hour())).not.toContain('24');
  });

  it('se usa con el teclado: escribir 1 y 4 lleva a las 14 e Intro la elige', () => {
    render(<Time initial="09:00" />);
    const trigger = hour();
    trigger.focus();
    fireEvent.keyDown(trigger, { key: '1' });
    fireEvent.keyDown(trigger, { key: '4' });
    const active = trigger.getAttribute('aria-activedescendant');
    expect(document.getElementById(active ?? '')?.textContent).toContain('14');
    fireEvent.keyDown(trigger, { key: 'Enter' });
    expect(output()).toBe('14:00');
    // Flecha abajo abre los minutos y avanza de 5 en 5.
    const minute = minutes();
    minute.focus();
    fireEvent.keyDown(minute, { key: 'ArrowDown' });
    fireEvent.keyDown(minute, { key: 'ArrowDown' });
    fireEvent.keyDown(minute, { key: 'Enter' });
    expect(output()).toBe('14:05');
  });

  it('las opciones y el formato son funciones puras', () => {
    expect(splitTime('07:45')).toEqual({ hour: '07', minute: '45' });
    expect(splitTime('7:45')).toEqual({ hour: '', minute: '' });
    expect(hourOptions('', true).map((option) => option.value)).toContain('24');
    expect(minuteOptions(5, '', '24').map((option) => option.value)).toEqual(['00']);
    expect(
      minuteOptions(5, '58')
        .map((option) => option.value)
        .at(-1),
    ).toBe('58');
    expect(formatLocalDateTime('2026-12-15T19:30')).toBe('15/12/2026 19:30');
  });
});

describe('selector de fecha y hora (etapa 9C, §2.1)', () => {
  it('arma «AAAA-MM-DDTHH:MM» solo con la fecha y la hora completas', () => {
    render(<DateTime />);
    const field = group('Fecha y hora');
    const date = within(field).getByRole('textbox', { name: 'Fecha' }) as HTMLInputElement;
    fireEvent.change(date, { target: { value: '15122026' } });
    expect(date.value).toBe('15/12/2026');
    // Sin hora aún no hay valor completo.
    expect(output()).toBe('');
    chooseOption(hour('Fecha y hora'), '19');
    chooseOption(minutes('Fecha y hora'), '30');
    expect(output()).toBe('2026-12-15T19:30');
    // Una fecha incompleta deja el valor vacío y el campo inválido con su explicación.
    fireEvent.change(date, { target: { value: '15/12/20' } });
    expect(output()).toBe('');
    expect(date.validationMessage).toBe('Completa la fecha como dd/mm/aaaa.');
    expect(field.textContent).toContain('Completa la fecha como dd/mm/aaaa.');
    fireEvent.change(date, { target: { value: '16/12/2026' } });
    expect(output()).toBe('2026-12-16T19:30');
    expect(date.validationMessage).toBe('');
  });

  it('muestra un valor recibido, lo reemplaza desde fuera y «Quitar» lo vacía', () => {
    render(<DateTime initial="2026-12-15T19:32" />);
    const field = group('Fecha y hora');
    const date = within(field).getByRole('textbox', { name: 'Fecha' }) as HTMLInputElement;
    expect(date.value).toBe('15/12/2026');
    expect(selectText(hour('Fecha y hora'))).toBe('19');
    // 32 no cae en el paso de 5 y se conserva.
    expect(selectText(minutes('Fecha y hora'))).toBe('32');
    act(() => {
      fireEvent.click(screen.getByRole('button', { name: 'Cambiar desde fuera' }));
    });
    expect(date.value).toBe('02/01/2027');
    expect(selectText(hour('Fecha y hora'))).toBe('08');
    expect(selectText(minutes('Fecha y hora'))).toBe('15');
    fireEvent.click(within(field).getByRole('button', { name: 'Quitar' }));
    expect(output()).toBe('');
    expect(date.value).toBe('');
    expect(selectText(hour('Fecha y hora'))).toBe('--');
  });

  it('con hora propuesta, una fecha sin hora toma esa hora; una hora sin fecha la pide', () => {
    render(<DateTime defaultTime="23:59" />);
    const field = group('Fecha y hora');
    const date = within(field).getByRole('textbox', { name: 'Fecha' }) as HTMLInputElement;
    chooseOption(hour('Fecha y hora'), '10');
    expect(output()).toBe('');
    expect(date.validationMessage).toBe('Escribe también la fecha.');
    fireEvent.click(within(field).getByRole('button', { name: 'Quitar' }));
    fireEvent.change(date, { target: { value: '01/03/2027' } });
    expect(output()).toBe('2027-03-01T23:59');
    expect(selectText(minutes('Fecha y hora'))).toBe('59');
  });
});

describe('sin campos nativos de hora ni de número (etapa 9C)', () => {
  it('ningún archivo de la interfaz declara type="time", datetime-local ni type="number"', () => {
    const sources = import.meta.glob<string>(
      ['../**/*.tsx', '!../**/*.test.tsx', '!../testing/**'],
      { query: '?raw', import: 'default', eager: true },
    );
    expect(Object.keys(sources).length).toBeGreaterThan(100);
    for (const [name, source] of Object.entries(sources)) {
      // Los comentarios pueden nombrar el control prohibido para explicar por qué no se usa.
      const code = source
        .split('\n')
        .filter((line) => !/^\s*(\/\/|\*|\/\*|\{\/\*)/.test(line))
        .join('\n');
      expect(code, name).not.toMatch(/type=["'](time|datetime-local|number)["']/);
    }
  });
});
