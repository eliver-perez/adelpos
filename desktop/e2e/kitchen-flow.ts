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

// Lote enviado desde la captura táctil: 2 filetes y 1 hamburguesa con aros de cebolla, cada uno
// en las dos estaciones de la ruta de Platillos.
export async function checkPreparation(page: Page) {
  await page.getByRole('button', { name: 'Cocina y preparación', exact: true }).click();
  await page.getByRole('button', { name: 'Preparación', exact: true }).click();
  const steaks = page.locator('.kitchen-preparation-card').filter({ hasText: 'Filete de prueba' });
  await expect(steaks).toHaveCount(2);
  await expect(steaks.first()).toContainText('2 × Filete de prueba');
  await steaks.first().getByRole('button', { name: 'En preparación', exact: true }).click();
  await steaks.first().getByRole('button', { name: 'Marcar producto listo', exact: true }).click();
  await expect(steaks).toHaveCount(1);
  await steaks.getByRole('button', { name: 'Marcar producto listo', exact: true }).click();
  await expect(steaks).toHaveCount(0);
  const burgers = page
    .locator('.kitchen-preparation-card')
    .filter({ hasText: 'Hamburguesa de prueba' });
  await expect(burgers).toHaveCount(2);
  for (const remaining of [1, 0]) {
    await burgers
      .first()
      .getByRole('button', { name: 'Marcar producto listo', exact: true })
      .click();
    await expect(burgers).toHaveCount(remaining);
  }
  await page.getByRole('button', { name: 'Comandas', exact: true }).click();
  await expect(page.getByText('Recepción KDS pendiente', { exact: true })).toBeVisible();
  await page.locator('.kitchen-list-button').first().click();
  await expect(page.locator('.kitchen-document')).toContainText('COCINA CALIENTE');
  await expect(page.locator('.kitchen-document')).toContainText('FREIDORAS');
  // La elección de la personalización llega a la comanda.
  await expect(page.locator('.kitchen-document')).toContainText('Aros de cebolla');
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
  await expect(page.getByRole('complementary', { name: 'Cuenta actual' })).toContainText(
    'Preparación: Listo',
  );
}
