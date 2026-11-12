import { expect } from '@playwright/test';
import type { Locator } from '@playwright/test';

// Ayudante único de Playwright para el selector de la etapa 9C (components/Select.tsx), que
// sustituye a todos los <select>: el disparador es un botón con role="combobox" y la lista vive en
// un portal (role="listbox", enlazada con aria-controls). Se elige como lo haría una persona:
// abrir, (opcional) escribir en la búsqueda integrada y tocar la opción.

// Etiqueta exacta (texto), expresión regular sobre la etiqueta, { label } o { value } (data-value).
export type OptionMatch = string | RegExp | { label: string | RegExp } | { value: string };

function cssString(text: string) {
  return text.replaceAll('\\', '\\\\').replaceAll('"', '\\"');
}

// Abre la lista (si no lo está) y devuelve su listbox.
export async function openSelect(combobox: Locator): Promise<Locator> {
  await expect(combobox).toBeEnabled();
  if ((await combobox.getAttribute('aria-expanded')) !== 'true') {
    await combobox.click();
  }
  await expect(combobox).toHaveAttribute('aria-expanded', 'true');
  const id = await combobox.getAttribute('aria-controls');
  return combobox.page().locator(`[id="${cssString(id ?? '')}"]`);
}

// Cierra con Esc sobre el disparador (enfocarlo no cierra la lista: es parte del selector).
export async function closeSelect(combobox: Locator) {
  if ((await combobox.getAttribute('aria-expanded')) === 'true') {
    await combobox.press('Escape');
    await expect(combobox).toHaveAttribute('aria-expanded', 'false');
  }
}

function optionLocator(listbox: Locator, option: OptionMatch): Locator {
  if (typeof option === 'string' || option instanceof RegExp) {
    return listbox.getByRole('option', { name: option, exact: typeof option === 'string' });
  }
  if ('value' in option) {
    return listbox.locator(`[role="option"][data-value="${cssString(option.value)}"]`);
  }
  return listbox.getByRole('option', {
    name: option.label,
    exact: typeof option.label === 'string',
  });
}

// Escribe en la búsqueda integrada de la lista abierta (la de insumos y proveedores consulta a Go).
export async function searchInSelect(combobox: Locator, text: string) {
  const listbox = await openSelect(combobox);
  const search = listbox.locator('xpath=..').getByRole('searchbox');
  await search.fill(text);
}

// Elige una opción y espera a que la lista se cierre. Con search escribe antes en la búsqueda.
export async function chooseOption(
  combobox: Locator,
  option: OptionMatch,
  { search }: { search?: string } = {},
) {
  const listbox = await openSelect(combobox);
  if (search !== undefined) {
    await searchInSelect(combobox, search);
  }
  await optionLocator(listbox, option).first().click();
  await expect(combobox).toHaveAttribute('aria-expanded', 'false');
}

// Selección múltiple: alterna cada opción (la lista queda abierta) y cierra con «Listo».
export async function chooseOptions(combobox: Locator, options: OptionMatch[]) {
  const listbox = await openSelect(combobox);
  for (const option of options) {
    await optionLocator(listbox, option).first().click();
  }
  await listbox.locator('xpath=..').getByRole('button', { name: 'Listo', exact: true }).click();
  await expect(combobox).toHaveAttribute('aria-expanded', 'false');
}

// Comprueba cuántas opciones coinciden (por ejemplo, 0 para «no se ofrece») y cierra la lista.
export async function expectOptionCount(combobox: Locator, option: OptionMatch, count: number) {
  const listbox = await openSelect(combobox);
  await expect(optionLocator(listbox, option)).toHaveCount(count);
  await closeSelect(combobox);
}

// Etiquetas visibles de las opciones, en orden (sin las de acción).
export async function optionLabels(combobox: Locator): Promise<string[]> {
  const listbox = await openSelect(combobox);
  const labels = await listbox
    .locator('[role="option"]:not(.select-action) .select-option-label')
    .allTextContents();
  await closeSelect(combobox);
  return labels;
}

// El disparador conserva el valor elegido en su atributo value (en el múltiple, separado por comas).
export async function expectSelectValue(combobox: Locator, value: string | RegExp) {
  await expect(combobox).toHaveAttribute('value', value);
}

// Texto que muestra el disparador: la opción elegida o el texto de «sin elegir».
export async function expectSelectText(combobox: Locator, text: string | RegExp) {
  await expect(combobox.locator('.select-value')).toHaveText(text);
}

// Valor de la primera opción que coincide (sin elegirla).
export async function optionValue(combobox: Locator, option: OptionMatch): Promise<string> {
  const listbox = await openSelect(combobox);
  const value = await optionLocator(listbox, option).first().getAttribute('data-value');
  await closeSelect(combobox);
  return value ?? '';
}
