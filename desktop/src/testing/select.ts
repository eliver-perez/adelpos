import { fireEvent, waitFor, within } from '@testing-library/react';

// Ayudantes de Vitest para el selector único (components/Select.tsx, etapa 9C): abren la lista del
// disparador (role="combobox") y eligen o leen sus opciones por etiqueta, como lo haría una persona.

type OptionName = string | RegExp;

// Abre la lista (si no lo está) y devuelve el listbox del portal.
export function openSelect(combobox: HTMLElement): HTMLElement {
  if (combobox.getAttribute('aria-expanded') !== 'true') {
    fireEvent.click(combobox);
  }
  const id = combobox.getAttribute('aria-controls');
  const listbox = id ? document.getElementById(id) : null;
  if (!listbox) {
    throw new Error('La lista del selector no se abrió.');
  }
  return listbox;
}

export function closeSelect(combobox: HTMLElement) {
  if (combobox.getAttribute('aria-expanded') === 'true') {
    fireEvent.keyDown(combobox, { key: 'Escape' });
  }
}

// Elige una opción por su nombre visible; con search escribe primero en la búsqueda de la lista.
export function chooseOption(combobox: HTMLElement, name: OptionName, search?: string) {
  const listbox = openSelect(combobox);
  if (search !== undefined) {
    typeSearch(combobox, search);
  }
  fireEvent.click(within(listbox).getByRole('option', { name }));
}

// Igual que chooseOption, pero espera a que la opción aparezca (datos que cargan de Go).
export async function chooseOptionAsync(combobox: HTMLElement, name: OptionName, search?: string) {
  const listbox = openSelect(combobox);
  if (search !== undefined) {
    typeSearch(combobox, search);
  }
  const option = await within(listbox).findByRole('option', { name });
  fireEvent.click(option);
}

// Elige por valor (el atributo data-value de cada opción), para las pruebas que ya trabajaban con
// identificadores.
export function chooseValue(combobox: HTMLElement, value: string) {
  const listbox = openSelect(combobox);
  const option = Array.from(listbox.querySelectorAll<HTMLElement>('[role="option"]')).find(
    (entry) => entry.dataset.value === value,
  );
  if (!option) {
    throw new Error(`El selector no ofrece el valor «${value}».`);
  }
  fireEvent.click(option);
}

// La lista se abre una sola vez antes de esperar: dentro de waitFor no se abre ni se cierra nada
// (cada cambio del DOM vuelve a ejecutar la comprobación y abrir y cerrar no terminaría nunca).
export async function chooseValueAsync(combobox: HTMLElement, value: string) {
  const listbox = openSelect(combobox);
  await waitFor(() => {
    const found = Array.from(listbox.querySelectorAll<HTMLElement>('[role="option"]')).some(
      (entry) => entry.dataset.value === value,
    );
    if (!found) {
      throw new Error(`El selector todavía no ofrece el valor «${value}».`);
    }
  });
  chooseValue(combobox, value);
}

export function typeSearch(combobox: HTMLElement, text: string) {
  const listbox = openSelect(combobox);
  const popup = listbox.closest('.select-popup');
  const input = popup?.querySelector<HTMLInputElement>('input[role="searchbox"]');
  if (!input) {
    throw new Error('El selector no tiene búsqueda.');
  }
  fireEvent.change(input, { target: { value: text } });
}

// Nombres visibles de las opciones (sin las de acción si actions es false). Cierra la lista si
// estaba cerrada; por eso no debe usarse dentro de waitFor (usar waitForOption).
export function optionLabels(combobox: HTMLElement, { actions = true } = {}): string[] {
  const wasOpen = combobox.getAttribute('aria-expanded') === 'true';
  const listbox = openSelect(combobox);
  const labels = within(listbox)
    .queryAllByRole('option')
    .filter((option) => actions || !option.classList.contains('select-action'))
    .map((option) => option.querySelector('.select-option-label')?.textContent ?? '');
  if (!wasOpen) {
    closeSelect(combobox);
  }
  return labels;
}

// Espera a que exista una opción con ese nombre (abre la lista y la deja abierta).
export async function findOption(combobox: HTMLElement, name: OptionName) {
  const listbox = openSelect(combobox);
  return within(listbox).findByRole('option', { name });
}

// Espera a que la lista ofrezca esa opción (datos que llegan de Go) y la vuelve a cerrar.
export async function waitForOption(combobox: HTMLElement, name: OptionName) {
  const listbox = openSelect(combobox);
  const option = await within(listbox).findByRole('option', { name });
  closeSelect(combobox);
  return option;
}

export function queryOption(combobox: HTMLElement, name: OptionName) {
  const listbox = openSelect(combobox);
  const option = within(listbox).queryByRole('option', { name });
  closeSelect(combobox);
  return option;
}

// Valor elegido (el disparador lo conserva en su atributo value; en el múltiple, separado por
// comas).
export function selectValue(combobox: HTMLElement): string {
  return combobox.getAttribute('value') ?? '';
}

// Texto que muestra el disparador (la opción elegida o el texto de «sin elegir»).
export function selectText(combobox: HTMLElement): string {
  return combobox.querySelector('.select-value')?.textContent ?? '';
}

export async function waitForSelectValue(combobox: HTMLElement, value: string) {
  await waitFor(() => {
    if (selectValue(combobox) !== value) {
      throw new Error(`El selector vale «${selectValue(combobox)}» y no «${value}».`);
    }
  });
}
