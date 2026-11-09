import { expect, test } from '@playwright/test';
import type { Page, TestInfo } from '@playwright/test';
import { installDriver } from './native-driver';
import { configureKitchen, checkPreparation } from './kitchen-flow';
import {
  checkDineInPayment,
  checkTakeawayDelivery,
  configureReceiptPrinter,
  enableUSDCash,
} from './payments-flow';

// Revisión visual de la captura: 1280 × 720, 1920 × 1080 y 1024 × 768 en tema claro y oscuro.
async function captureThemes(page: Page, testInfo: TestInfo, name: string) {
  for (const size of [
    { width: 1280, height: 720 },
    { width: 1920, height: 1080 },
    { width: 1024, height: 768 },
  ]) {
    await page.setViewportSize(size);
    for (const theme of ['light', 'dark']) {
      await page.evaluate((value) => {
        document.documentElement.dataset.theme = value;
      }, theme);
      // Deja terminar el cambio de tamaño y las transiciones de color antes de la captura.
      await page.waitForTimeout(300);
      expect(
        await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
      ).toBe(true);
      await page.screenshot({
        path: testInfo.outputPath(`${name}-${theme}-${size.width}x${size.height}.png`),
      });
    }
  }
  await page.evaluate(() => {
    document.documentElement.dataset.theme = 'light';
  });
  await page.setViewportSize({ width: 1280, height: 720 });
}

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
  // Etapa 6.1 §3.3: el pago en dólares se activa antes de abrir el turno.
  await enableUSDCash(page);
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
  // Etapa 6.1 §4: captura táctil. Categoría → producto directo → producto con opciones → enviar
  // a cocina → cobrar. Todas las cifras que se comprueban las calcula Go.
  await expect(page.getByRole('heading', { name: 'Mesa 1 · Cuenta #1' })).toBeVisible();
  await expect(page.getByRole('combobox', { name: 'Cuenta de la mesa' })).toHaveCount(0);
  await expect(page.getByRole('combobox', { name: 'Acciones de cuenta' })).toHaveCount(0);
  const bill = page.getByRole('complementary', { name: 'Cuenta actual' });
  const dishes = page
    .getByRole('navigation', { name: 'Categorías' })
    .getByRole('button', { name: /Platillos/ });
  await dishes.click();
  await expect(dishes).toHaveAttribute('aria-pressed', 'true');
  await expect(dishes).toContainText('2 productos');
  const categoryBox = await dishes.boundingBox();
  expect(categoryBox!.height).toBeGreaterThanOrEqual(64);
  const steak = page.getByRole('button', { name: /^Filete de prueba/ });
  await expect(steak).toContainText('$150.00');
  const steakBox = await steak.boundingBox();
  expect(steakBox!.height).toBeGreaterThanOrEqual(108);
  // Un toque agrega; el segundo suma a la misma partida nueva.
  await steak.click();
  await steak.click();
  const steakLine = bill.getByRole('article', { name: '2 × Filete de prueba' });
  await expect(steakLine).toContainText('Nuevo');
  // Producto con elección obligatoria: la personalización se abre en el panel central.
  const burger = page.getByRole('button', { name: /^Hamburguesa de prueba/ });
  await expect(burger).toContainText('Opciones');
  await burger.click();
  const options = page.getByRole('region', { name: 'Personalizar Hamburguesa de prueba' });
  await expect(page.getByRole('dialog')).toHaveCount(0);
  await expect(
    options.getByRole('button', { name: 'Papas a la francesa', exact: true }),
  ).toHaveAttribute('aria-pressed', 'true');
  const rings = options.getByRole('button', { name: 'Aros de cebolla +$10.00', exact: true });
  const ringsBox = await rings.boundingBox();
  expect(ringsBox!.height).toBeGreaterThanOrEqual(56);
  await rings.click();
  await expect(rings).toHaveAttribute('aria-pressed', 'true');
  const add = options.getByRole('button', { name: /^Agregar a la cuenta/ });
  await expect(add).toHaveText('Agregar a la cuenta · $130.00');
  // Revisión visual de la personalización y sin desplazamiento horizontal en otros tamaños.
  for (const size of [
    { width: 1920, height: 1080 },
    { width: 2560, height: 1440 },
    { width: 1024, height: 768 },
    { width: 900, height: 1200 },
    { width: 1280, height: 720 },
  ]) {
    await page.setViewportSize(size);
    await expect(rings).toHaveAttribute('aria-pressed', 'true');
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
    ).toBe(true);
    const addBox = await add.boundingBox();
    expect(addBox!.y + addBox!.height).toBeLessThanOrEqual(size.height);
  }
  await expect(bill.locator('.capture-sync-status')).toHaveText('Guardado');
  await expect(add).toHaveText('Agregar a la cuenta · $130.00');
  await captureThemes(page, testInfo, 'capture-options');
  await add.click();
  const burgerLine = bill.getByRole('article', { name: '1 × Hamburguesa de prueba' });
  await expect(burgerLine).toContainText('Aros de cebolla');
  await expect(page.getByRole('region', { name: 'Productos' })).toBeVisible();
  // El borrador se guarda con antirrebote y los importes llegan de Go.
  await expect(steakLine).toContainText('$300.00');
  await expect(burgerLine).toContainText('$130.00');
  await expect(bill.locator('.capture-grand-total')).toContainText('$430.00');
  await expect(bill).toContainText('Precios con IVA incluido');
  for (const size of [
    { width: 1920, height: 1080 },
    { width: 2560, height: 1440 },
    { width: 1024, height: 768 },
    { width: 900, height: 1200 },
    { width: 1280, height: 720 },
  ]) {
    await page.setViewportSize(size);
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
    ).toBe(true);
    const sendBox = await bill
      .getByRole('button', { name: 'Enviar a cocina · 3', exact: true })
      .boundingBox();
    expect(sendBox!.y + sendBox!.height).toBeLessThanOrEqual(size.height);
  }
  await expect(bill.locator('.capture-sync-status')).toHaveText('Guardado');
  await captureThemes(page, testInfo, 'capture');
  const send = bill.getByRole('button', { name: 'Enviar a cocina · 3', exact: true });
  await expect(send).toBeEnabled();
  await send.click();
  await expect(bill.locator('.capture-line-confirmed')).toHaveCount(2);
  await expect(steakLine).toContainText('En cocina');
  await expect(
    bill.getByRole('button', { name: 'Enviar a cocina · 0', exact: true }),
  ).toBeDisabled();
  await expect(bill.locator('.capture-grand-total')).toContainText('$430.00');
  await checkPreparation(page);
  await steakLine.getByRole('button', { name: 'Modificar', exact: true }).click();
  const edit = page.getByRole('region', { name: 'Personalizar Filete de prueba' });
  await edit.getByRole('button', { name: 'Escribir otra', exact: true }).click();
  await edit.getByLabel('Nueva indicación para cocina').fill('Empacar por separado');
  await edit.getByRole('button', { name: 'Agregar indicación', exact: true }).click();
  await edit.getByLabel('Motivo de modificación').fill('Indicaciones del cliente');
  await edit.getByRole('button', { name: /^Confirmar modificación/ }).click();
  await expect(steakLine).toContainText('Empacar por separado');
  // Cancelación de una partida enviada con su flujo y permiso existentes.
  await burgerLine.getByRole('button', { name: 'Cancelar unidades', exact: true }).click();
  await page.getByLabel('Motivo de cancelación').fill('El cliente cambió de opinión');
  await page
    .getByRole('button', { name: 'Confirmar cancelación de unidades', exact: true })
    .click();
  await expect(burgerLine).toContainText('Cancelado');
  await expect(bill.locator('.capture-grand-total')).toContainText('$300.00');
  // «Más…» reúne las acciones menos frecuentes; ya no hay lista desplegable de acciones.
  await bill.getByRole('button', { name: 'Más…', exact: true }).click();
  await bill
    .getByRole('group', { name: 'Más acciones de la cuenta' })
    .getByRole('button', { name: 'Historial de cambios', exact: true })
    .click();
  await expect(page.getByRole('button', { name: /Producto cancelado/ })).toBeVisible();
  await page.getByRole('button', { name: /Producto modificado/ }).click();
  await expect(page.getByRole('heading', { name: 'Antes', exact: true })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Después', exact: true })).toBeVisible();
  await page.getByRole('button', { name: 'Volver', exact: true }).click();
  await bill.getByRole('button', { name: 'Dividir', exact: true }).click();
  await page.getByLabel(/Filete de prueba · 2 disponibles/).fill('1');
  await page.getByRole('button', { name: 'Dividir cuenta', exact: true }).click();
  await expect(bill.locator('.capture-grand-total')).toContainText('$150.00');
  // Las cuentas de la mesa quedan como botones en la barra superior: la división muestra la
  // cuenta resultante y la original sigue a un toque.
  const tableAccounts = page.getByRole('group', { name: 'Cuentas de la mesa' });
  await expect(page.getByRole('heading', { name: 'Mesa 1 · Cuenta #2' })).toBeVisible();
  await expect(
    tableAccounts.getByRole('button', { name: 'Cuenta #2 · Cuenta · separada', exact: true }),
  ).toHaveAttribute('aria-pressed', 'true');
  await expect(
    tableAccounts.getByRole('button', { name: '+ Nueva cuenta', exact: true }),
  ).toBeVisible();
  await tableAccounts.getByRole('button', { name: 'Cuenta #1', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Mesa 1 · Cuenta #1' })).toBeVisible();
  await expect(bill.locator('.capture-grand-total')).toContainText('$150.00');
  await tableAccounts
    .getByRole('button', { name: 'Cuenta #2 · Cuenta · separada', exact: true })
    .click();
  await expect(page.getByRole('heading', { name: 'Mesa 1 · Cuenta #2' })).toBeVisible();
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
  // La cuenta actual es ahora el pedido para llevar recién abierto.
  await expect(bill).toContainText('Cliente temporal');
  await expect(bill).toContainText('Pago: sin registro de cobro');
  await expect(bill).toContainText('Entrega: no entregado');
  // Sin productos enviados no se entrega ni se cobra: «Cobrar» conserva su lugar desactivado.
  await expect(page.getByRole('button', { name: /Marcar.*entregado/ })).toHaveCount(0);
  await expect(bill.getByRole('button', { name: 'Cobrar', exact: true })).toBeDisabled();
  await checkTakeawayDelivery(page);

  // La misma pantalla de captura sirve para llevar, con los datos del pedido arriba; la cuenta
  // refleja la entrega y el cobro registrados.
  await expect(page.getByRole('heading', { name: /^Para llevar · Pedido #/ })).toBeVisible();
  await expect(bill).toContainText('Entrega: entregado');
  await expect(page.locator('.capture-topbar')).toContainText('Cliente temporal');
  await expect(page.locator('.capture-topbar')).toContainText('Entrega programada');
  for (const theme of ['light', 'dark']) {
    await page.evaluate((value) => {
      document.documentElement.dataset.theme = value;
    }, theme);
    await page.waitForTimeout(300);
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
    ).toBe(true);
    await page.screenshot({ path: testInfo.outputPath(`takeaway-${theme}-1280x720.png`) });
  }
  await page.evaluate(() => {
    document.documentElement.dataset.theme = 'light';
  });
  await page.emulateMedia({ reducedMotion: 'reduce' });
  await page.locator('.capture-topbar').getByRole('button', { name: 'Para llevar' }).click();
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
