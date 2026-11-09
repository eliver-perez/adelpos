import { expect, test } from '@playwright/test';
import { installDriver } from './native-driver';
import { configureKitchen, checkPreparation } from './kitchen-flow';
import {
  checkDineInPayment,
  checkTakeawayDelivery,
  configureReceiptPrinter,
} from './payments-flow';

test('operación real, acordeones y ergonomía a 1280 × 720', async ({ page }, testInfo) => {
  const errors: string[] = [];
  page.on('pageerror', (error) => errors.push(error.message));
  const driver = await installDriver(page);
  await page.goto('/');
  await page.getByLabel('Usuario', { exact: true }).fill('admin');
  await page.getByLabel('Contraseña', { exact: true }).fill('admin');
  await page.getByRole('button', { name: 'Entrar', exact: true }).click();
  await configureKitchen(page);
  await configureReceiptPrinter(page);
  await page.getByRole('button', { name: 'Restaurante', exact: true }).click();
  await page.getByRole('button', { name: 'Áreas y distribución', exact: true }).click();
  await page.getByRole('button', { name: 'Crear área', exact: true }).click();
  // Etapa 6.1 §2.2: el orden se elige por posición, no con un número.
  await expect(page.getByLabel('Orden del área', { exact: true })).toHaveCount(0);
  await expect(page.getByRole('combobox', { name: 'Posición del área', exact: true })).toHaveValue(
    '',
  );
  await page.getByLabel('Nombre del área', { exact: true }).fill('Salón principal');
  await page.getByRole('button', { name: 'Guardar área', exact: true }).click();
  await page.getByRole('button', { name: 'Crear mesa', exact: true }).click();
  await page.getByLabel('Nombre o número de mesa', { exact: true }).fill('Mesa 1');
  // Etapa 6.1 §2.1: la barra de acciones queda fija al pie del área desplazable, sin subir. Con
  // el inicio del formulario a la vista (WebKit no desplaza al rellenar), el botón principal debe
  // verse aunque el final del formulario quede fuera de la ventana.
  await page.getByLabel('Nombre o número de mesa', { exact: true }).scrollIntoViewIfNeeded();
  await expect(
    page.getByRole('button', { name: 'Guardar y crear otra', exact: true }),
  ).toBeVisible();
  const tableActions = await page
    .getByRole('button', { name: 'Guardar mesa', exact: true })
    .boundingBox();
  expect(tableActions!.y + tableActions!.height).toBeLessThanOrEqual(page.viewportSize()!.height);
  const tableForm = await page
    .getByRole('button', { name: 'Guardar mesa', exact: true })
    .locator('xpath=ancestor::form')
    .boundingBox();
  // El formulario continúa por debajo de la barra: está fija, no en su lugar natural al final.
  expect(tableForm!.y + tableForm!.height).toBeGreaterThan(
    tableActions!.y + tableActions!.height + 20,
  );
  // Etapa 6.1 §1.1: una mesa redonda de 100 × 100 debe dibujarse como círculo en cualquier ventana.
  await page.getByRole('combobox', { name: /^Forma/ }).selectOption('round');
  await page.getByLabel('Alto', { exact: true }).fill('100');
  await page.getByRole('button', { name: 'Guardar mesa', exact: true }).click();
  await expect(page.getByRole('button', { name: /Mesa 1, Libre/ })).toBeVisible();
  for (const size of [
    { width: 1920, height: 1080 },
    { width: 1024, height: 768 },
    { width: 1280, height: 720 },
  ]) {
    await page.setViewportSize(size);
    const box = await page.getByRole('button', { name: /Mesa 1, Libre/ }).boundingBox();
    expect(Math.abs(box!.width - box!.height)).toBeLessThanOrEqual(1.5);
  }
  await page.getByRole('button', { name: /Mesa 1, Libre/ }).focus();
  await page.keyboard.press('ArrowRight');
  await page.getByRole('button', { name: 'Guardar distribución', exact: true }).click();
  await expect
    .poll(async () => (await driver.request('GET', 'restaurant/overview')).body.tables[0].x)
    .toBe(30);
  await page.getByRole('button', { name: 'Mesas', exact: true }).click();
  await page.getByRole('button', { name: /Mesa 1, Libre/ }).click();
  await page.getByRole('button', { name: 'Abrir cuenta en mesa', exact: true }).click();
  await page.getByRole('button', { name: 'Abrir cuenta y capturar', exact: true }).click();
  await page.getByRole('button', { name: /Filete de prueba/ }).click();
  await page.getByLabel('Cantidad del producto').fill('2');
  for (const size of [
    { width: 1920, height: 1080 },
    { width: 2560, height: 1440 },
    { width: 1024, height: 768 },
    { width: 900, height: 1200 },
    { width: 1280, height: 720 },
  ]) {
    await page.setViewportSize(size);
    await expect(page.getByLabel('Cantidad del producto')).toHaveValue('2');
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
    ).toBe(true);
    await page.screenshot({
      path: testInfo.outputPath(`capture-${size.width}x${size.height}.png`),
    });
  }
  await expect(
    page.getByRole('button', { name: 'Guardar en borrador', exact: true }),
  ).toBeEnabled();
  await page.getByRole('button', { name: 'Guardar en borrador', exact: true }).click();
  const confirm = page.getByRole('button', { name: 'Confirmar pedido', exact: true });
  await expect(confirm).toBeEnabled();
  const bounds = await confirm.boundingBox();
  expect(bounds!.y + bounds!.height).toBeLessThanOrEqual(720);
  await confirm.click();
  await expect(
    page.locator('.bill-line:not(.bill-line-draft):not(.bill-line-cancelled)'),
  ).toHaveCount(1);
  await expect(page.getByRole('complementary', { name: 'Cuenta actual' })).toContainText('$300.00');
  await checkPreparation(page);
  await page.getByRole('button', { name: 'Modificar', exact: true }).click();
  await page
    .getByLabel('Observaciones para preparación (sin cargo automático)')
    .fill('Empacar por separado');
  await page.getByLabel('Motivo de modificación').fill('Indicaciones del cliente');
  await page.getByRole('button', { name: 'Confirmar modificación', exact: true }).click();
  await expect(page.getByRole('complementary', { name: 'Cuenta actual' })).toContainText(
    'Empacar por separado',
  );
  await page.getByLabel('Acciones de cuenta').selectOption('history');
  await page.getByRole('button', { name: /Producto modificado/ }).click();
  await expect(page.getByRole('heading', { name: 'Antes', exact: true })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Después', exact: true })).toBeVisible();
  await page.getByRole('button', { name: 'Volver', exact: true }).click();
  await page.getByLabel('Acciones de cuenta').selectOption('split');
  await page.getByLabel(/Filete de prueba · 2 disponibles/).fill('1');
  await page.getByRole('button', { name: 'Dividir cuenta', exact: true }).click();
  await expect(page.getByRole('complementary', { name: 'Cuenta actual' })).toContainText('$150.00');
  await checkDineInPayment(page);
  await page.getByRole('button', { name: 'Para llevar', exact: true }).first().click();
  await page.getByRole('button', { name: 'Nuevo para llevar', exact: true }).click();
  await page.getByLabel('Nombre del cliente (opcional)').fill('Cliente temporal');
  await page.getByLabel('Origen del pedido').fill('Ventanilla');
  const scheduled = page.getByRole('button', { name: /Pedido programado/ });
  await scheduled.focus();
  await page.keyboard.press('Enter');
  await page.getByRole('combobox', { name: 'Programación', exact: true }).selectOption('delivery');
  await page.getByLabel('Fecha y hora · America/Monterrey').fill('2026-12-15T19:30');
  await scheduled.click();
  await scheduled.focus();
  await page.keyboard.press('Space');
  await expect(page.getByLabel('Fecha y hora · America/Monterrey')).toHaveValue('2026-12-15T19:30');
  await page.getByRole('button', { name: 'Abrir cuenta y capturar', exact: true }).click();
  const bill = page.getByRole('complementary', { name: 'Cuenta actual' });
  await expect(bill).toContainText('Cliente temporal');
  await expect(bill).toContainText('Pago: sin registro de cobro');
  await expect(bill).toContainText('Entrega: no entregado');
  await expect(page.getByRole('button', { name: /Marcar.*entregado|Cobrar/ })).toHaveCount(0);
  await checkTakeawayDelivery(page);

  for (const theme of ['light', 'dark']) {
    await page.evaluate((value) => {
      document.documentElement.dataset.theme = value;
    }, theme);
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
    ).toBe(true);
    await page.screenshot({ path: testInfo.outputPath(`takeaway-${theme}-1280x720.png`) });
  }
  await page.emulateMedia({ reducedMotion: 'reduce' });
  await page.getByRole('button', { name: 'Nuevo para llevar', exact: true }).click();
  await driver.request('PUT', 'terminal/preferences', { keyboard_mode: 'always' });
  // La preferencia se refresca por el mismo sondeo de sesión de la aplicación nativa.
  await expect
    .poll(async () => page.locator('.virtual-keyboard').count(), { timeout: 12000 })
    .toBe(0);
  await page.waitForTimeout(5200);
  await page.getByLabel('Nombre del cliente (opcional)').focus();
  await expect(page.getByRole('group', { name: 'Teclado virtual alfanumérico' })).toBeVisible();
  await page.getByRole('button', { name: 'Escribir ñ', exact: true }).click();
  await expect(page.getByLabel('Nombre del cliente (opcional)')).toHaveValue('ñ');
  const keyboard = await page.locator('.virtual-keyboard').boundingBox();
  const focused = await page.getByLabel('Nombre del cliente (opcional)').boundingBox();
  expect(focused!.y + focused!.height).toBeLessThanOrEqual(keyboard!.y);
  await page.screenshot({ path: testInfo.outputPath('keyboard-field-1280x720.png') });
  const save = page.getByRole('button', { name: 'Abrir cuenta y capturar', exact: true });
  await save.scrollIntoViewIfNeeded();
  const saveBounds = await save.boundingBox();
  expect(saveBounds!.y + saveBounds!.height).toBeLessThanOrEqual(keyboard!.y);
  await page.screenshot({ path: testInfo.outputPath('keyboard-1280x720.png') });
  await page.getByRole('button', { name: 'Cerrar teclado', exact: true }).click();
  // Etapa 6.1 §2.6: la vista previa salió del menú principal y vive dentro de Catálogo.
  await expect(
    page.getByRole('button', { name: 'Probar personalización', exact: true }),
  ).toHaveCount(0);
  await page.getByRole('button', { name: 'Catálogo', exact: true }).click();
  await page.getByRole('tab', { name: 'Vista previa', exact: true }).click();
  await page
    .getByRole('button', { name: 'Excepción autorizada de disponibilidad', exact: true })
    .click();
  await expect(page.locator('details, summary')).toHaveCount(0);
  await page
    .getByRole('button', { name: 'Excepción autorizada de disponibilidad', exact: true })
    .focus();
  await page.keyboard.press('Space');
  await expect(
    page.getByRole('button', { name: 'Excepción autorizada de disponibilidad', exact: true }),
  ).toHaveAttribute('aria-expanded', 'false');
  expect(errors).toEqual([]);
});
