import { expect } from '@playwright/test';
import type { Page } from '@playwright/test';

export async function configureKitchen(page: Page) {
  await page.getByRole('button', { name: 'Salir de pantalla completa', exact: true }).click();
  await page.getByRole('button', { name: 'Pantalla completa', exact: true }).click();
  await page.getByRole('button', { name: 'Cocina y preparación', exact: true }).click();
  await page.getByRole('button', { name: 'Impresoras', exact: true }).click();
  await page.getByRole('button', { name: 'Registrar impresora', exact: true }).click();
  await page.getByLabel('Nombre descriptivo de impresora').fill('Destino controlado E2E');
  await page
    .getByRole('combobox', { name: 'Transporte de impresión', exact: true })
    .selectOption('agent');
  await page.getByLabel('Referencia del destino remoto').fill('Agente de aceptación no disponible');
  await page.getByRole('button', { name: 'Guardar impresora', exact: true }).click();
  await expect(
    page.getByRole('button', {
      name: 'Enviar una impresión de prueba a Destino controlado E2E',
      exact: true,
    }),
  ).toBeVisible();
  await page.getByRole('button', { name: 'Estaciones', exact: true }).click();
  for (const [name, reception] of [
    ['Cocina caliente', 'printer'],
    ['Freidoras', 'both'],
  ]) {
    await page.getByRole('button', { name: 'Nueva estación', exact: true }).click();
    await page.getByLabel('Nombre de la estación', { exact: true }).fill(name!);
    await page
      .getByRole('combobox', { name: 'Recepción de comandas', exact: true })
      .selectOption(reception!);
    await page
      .getByRole('combobox', { name: 'Impresora compartida o exclusiva', exact: true })
      .selectOption({ label: 'Destino controlado E2E' });
    await page.getByRole('button', { name: 'Guardar estación', exact: true }).click();
    await expect(page.locator('.kitchen-list-button').filter({ hasText: name })).toBeVisible();
  }
  await page.getByRole('button', { name: 'Comandas y tiempos', exact: true }).click();
  await page
    .getByRole('combobox', { name: 'Agrupación del establecimiento', exact: true })
    .selectOption('unified');
  await page.getByRole('button', { name: 'Guardar configuración de cocina', exact: true }).click();
  await page.getByRole('button', { name: 'Enrutamiento', exact: true }).click();
  await page.getByRole('button', { name: 'Nueva ruta', exact: true }).click();
  await page
    .getByRole('combobox', { name: 'Categoría de la ruta', exact: true })
    .selectOption({ label: 'Platillos' });
  await page.getByRole('checkbox', { name: 'Cocina caliente', exact: true }).check();
  await page.getByRole('checkbox', { name: 'Freidoras', exact: true }).check();
  await page.getByRole('button', { name: 'Guardar ruta', exact: true }).click();
  await expect(
    page.locator('.kitchen-list-button').filter({ hasText: 'Categoría · Platillos' }),
  ).toBeVisible();
  await page.getByRole('button', { name: 'Restaurante', exact: true }).click();
}

export async function checkPreparation(page: Page) {
  await page.getByRole('button', { name: 'Cocina y preparación', exact: true }).click();
  await page.getByRole('button', { name: 'Preparación', exact: true }).click();
  await expect(
    page.locator('.kitchen-preparation-card').filter({ hasText: 'Filete de prueba' }),
  ).toHaveCount(2);
  await page.getByRole('button', { name: 'En preparación', exact: true }).first().click();
  await page.getByRole('button', { name: 'Marcar producto listo', exact: true }).first().click();
  await expect(
    page.locator('.kitchen-preparation-card').filter({ hasText: 'Filete de prueba' }),
  ).toHaveCount(1);
  await page.getByRole('button', { name: 'Marcar producto listo', exact: true }).click();
  await expect(
    page.locator('.kitchen-preparation-card').filter({ hasText: 'Filete de prueba' }),
  ).toHaveCount(0);
  await page.getByRole('button', { name: 'Comandas', exact: true }).click();
  await expect(page.getByText('Recepción KDS pendiente', { exact: true })).toBeVisible();
  await page.locator('.kitchen-list-button').first().click();
  await expect(page.locator('.kitchen-document')).toContainText('COCINA CALIENTE');
  await expect(page.locator('.kitchen-document')).toContainText('FREIDORAS');
  await page.getByRole('button', { name: 'Cola de impresión', exact: true }).click();
  await expect(page.locator('.kitchen-job')).toHaveCount(1);
  await expect(page.locator('.kitchen-job')).toContainText('Requiere intervención');
  await page.getByRole('button', { name: 'Consultar trabajo e intentos', exact: true }).click();
  await page
    .getByLabel('Motivo de la decisión de impresión')
    .fill('Destino de aceptación sin hardware físico');
  await page.getByRole('button', { name: 'Registrar decisión', exact: true }).click();
  await expect(page.locator('.kitchen-job')).toContainText('Cancelado administrativamente');
  await page.getByRole('button', { name: 'Restaurante', exact: true }).click();
  await expect(page.locator('.account-preparation-status')).toContainText('Listo');
}
