import { randomUUID } from 'node:crypto';
import { expect, test } from '@playwright/test';
import type { Page, TestInfo } from '@playwright/test';
import { installDriver } from './native-driver';
import { openAdministration, openSection } from './navigation';
import { expectNoticesClosedOnTime, notice, watchNotices } from './notices';
import { checkCashShift } from './cash-flow';
import { configureKitchen, checkPreparation } from './kitchen-flow';
import {
  checkDineInPayment,
  checkTakeawayDelivery,
  sendTakeawayToKitchen,
  configureReceiptPrinter,
  enableUSDCash,
} from './payments-flow';
import { chooseOption, expectSelectValue } from './select';

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

// Etapa 7.1 (fase D): el pie de la cuenta de la captura cabe completo a 1280 × 720 y 1920 × 1080
// en claro y oscuro. Ni el pie ni sus filas (totales, acciones secundarias y principales) se
// desbordan a lo ancho, y los importes y todos sus botones, «Más…» incluido, quedan dentro de la
// cuenta y de la ventana (la cuenta oculta lo que sobresale, así que un desborde los recortaba).
async function expectBillFooterFits(page: Page) {
  for (const size of [
    { width: 1280, height: 720 },
    { width: 1920, height: 1080 },
  ]) {
    await page.setViewportSize(size);
    for (const theme of ['light', 'dark']) {
      await page.evaluate((value) => {
        document.documentElement.dataset.theme = value;
      }, theme);
      await page.waitForTimeout(300);
      const layout = await page.evaluate(() => {
        const bill = document.querySelector('.capture-bill')!.getBoundingClientRect();
        const footer = document.querySelector<HTMLElement>('.capture-bill-footer')!;
        const boxes = [
          footer,
          ...Array.from(footer.children),
          ...Array.from(footer.querySelectorAll('button, dd')),
        ] as HTMLElement[];
        return {
          viewport: window.innerWidth,
          billLeft: bill.left,
          billRight: bill.right,
          boxes: boxes.map((element) => {
            const box = element.getBoundingClientRect();
            return {
              // Botones e importes por su texto; el pie y sus filas por su clase.
              label: element.matches('button, dd')
                ? (element.textContent?.trim() ?? '')
                : `.${element.className.split(' ').join('.')}`,
              left: box.left,
              right: box.right,
              scrollWidth: element.scrollWidth,
              clientWidth: element.clientWidth,
            };
          }),
        };
      });
      expect(layout.boxes.length).toBeGreaterThan(4);
      for (const box of layout.boxes) {
        expect(box.scrollWidth, `${box.label} se desborda`).toBeLessThanOrEqual(box.clientWidth);
        expect(box.left, `${box.label} sale de la cuenta`).toBeGreaterThanOrEqual(
          layout.billLeft - 0.5,
        );
        expect(box.right, `${box.label} sale de la cuenta`).toBeLessThanOrEqual(
          layout.billRight + 0.5,
        );
        expect(box.right, `${box.label} sale de la ventana`).toBeLessThanOrEqual(layout.viewport);
      }
      const footer = page.locator('.capture-bill-footer');
      await expect(footer.getByRole('button', { name: 'Más…', exact: true })).toBeInViewport({
        ratio: 1,
      });
      await expect(footer.locator('.capture-grand-total dd')).toBeInViewport({ ratio: 1 });
    }
  }
  await page.evaluate(() => {
    document.documentElement.dataset.theme = 'light';
  });
  await page.setViewportSize({ width: 1280, height: 720 });
}

// Etapa 6.2: revisión visual del mapa con el panel lateral de la mesa a 1280 × 720 y 1920 × 1080
// en tema claro y oscuro. El mapa y el panel quedan sin desplazamiento horizontal y las acciones
// principales del panel dentro de la ventana.
async function panelThemes(page: Page, testInfo: TestInfo, name: string, action: string) {
  for (const size of [
    { width: 1280, height: 720 },
    { width: 1920, height: 1080 },
  ]) {
    await page.setViewportSize(size);
    for (const theme of ['light', 'dark']) {
      await page.evaluate((value) => {
        document.documentElement.dataset.theme = value;
      }, theme);
      await page.waitForTimeout(300);
      expect(
        await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
      ).toBe(true);
      const button = page
        .getByRole('complementary', { name: /^Panel de / })
        .getByRole('button', { name: action, exact: true });
      const box = await button.boundingBox();
      expect(box!.height).toBeGreaterThanOrEqual(52);
      expect(box!.y + box!.height).toBeLessThanOrEqual(size.height);
      await expect(page.getByRole('region', { name: 'Mapa del restaurante' })).toBeVisible();
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

// Cuentas abiertas de una mesa según el servidor (no según la interfaz).
async function openTableAccounts(
  driver: Awaited<ReturnType<typeof installDriver>>,
  tableID: string,
) {
  const result = await driver.request(
    'GET',
    `restaurant/accounts?mode=dine_in&state=open&table_id=${tableID}&page_size=50`,
  );
  return (result.body.items as { id: string; name: string }[]) ?? [];
}

test('operación real, acordeones y ergonomía a 1280 × 720', async ({ page }, testInfo) => {
  const errors: string[] = [];
  page.on('pageerror', (error) => errors.push(error.message));
  const driver = await installDriver(page);
  await watchNotices(page);
  await page.goto('/');
  await page.getByLabel('Usuario', { exact: true }).fill('admin');
  await page.getByLabel('Contraseña', { exact: true }).fill('admin');
  await page.getByRole('button', { name: 'Entrar', exact: true }).click();
  await configureKitchen(page);
  await configureReceiptPrinter(page);
  // Etapa 6.1 §3.3: el pago en dólares se activa antes de abrir el turno.
  await enableUSDCash(page);
  // Etapa 9C: Mesas es una sección de la barra superior; «Áreas y distribución» sigue como pestaña.
  await openSection(page, 'Mesas');
  await page
    .getByRole('group', { name: 'Vistas operativas' })
    .getByRole('button', { name: 'Áreas y distribución', exact: true })
    .click();
  await page.getByRole('button', { name: 'Crear área', exact: true }).click();
  // Etapa 6.1 §2.2: el orden se elige por posición, no con un número.
  await expect(page.getByLabel('Orden del área', { exact: true })).toHaveCount(0);
  await expectSelectValue(
    page.getByRole('combobox', { name: 'Posición del área', exact: true }),
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
  await chooseOption(page.getByRole('combobox', { name: /^Forma/ }), { value: 'round' });
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
  await page
    .getByRole('group', { name: 'Vistas operativas' })
    .getByRole('button', { name: 'Mesas', exact: true })
    .click();
  const tableID = (await driver.request('GET', 'restaurant/overview')).body.tables[0].id as string;
  // Etapa 6.2 §1.1–1.2: tocar la mesa abre el panel lateral (sin ventana emergente).
  await expect(page.getByRole('button', { name: 'Abrir cuenta en mesa', exact: true })).toHaveCount(
    0,
  );
  // Etapa 7.1 §2.4: la leyenda del mapa muestra los cinco estados.
  const legend = page.getByRole('list', { name: 'Estados de las mesas' });
  for (const state of ['Libre', 'Ocupada', 'Esperando pago', 'Pago parcial', 'Fuera de servicio']) {
    await expect(legend).toContainText(state);
  }
  await page.getByRole('button', { name: /Mesa 1, Libre/ }).click();
  const tablePanel = page.getByRole('complementary', { name: 'Panel de Mesa 1' });
  // Etapa 7.1 §2.3: la mesa libre tiene la misma estructura que una ocupada, con totales en cero,
  // «Capturar» en su lugar y las acciones que no aplican desactivadas.
  await expect(tablePanel.locator('.table-panel-state')).toHaveText('Libre');
  await expect(tablePanel).toContainText('Mesa para 4');
  await expect(tablePanel.locator('.table-panel-totals')).toContainText('$0.00');
  for (const label of ['Cobrar', 'Precuenta', 'Dividir']) {
    await expect(tablePanel.getByRole('button', { name: label, exact: true })).toBeDisabled();
  }
  await expect(page.getByRole('dialog')).toHaveCount(0);
  const captureButton = tablePanel.getByRole('button', { name: 'Capturar', exact: true });
  await panelThemes(page, testInfo, 'table-panel-free', 'Capturar');
  // Etapa 7.1 §2.1: mesa libre → capturar → salir sin productos → la mesa sigue libre.
  await captureButton.click();
  await expect(page.getByRole('heading', { name: 'Mesa 1 · Cuenta nueva' })).toBeVisible();
  await page.locator('.capture-topbar').getByRole('button', { name: /Mesas/ }).click();
  await expect(tablePanel.locator('.table-panel-state')).toHaveText('Libre');
  await expect(page.getByRole('button', { name: /Mesa 1, Libre/ })).toBeVisible();
  expect(await openTableAccounts(driver, tableID)).toHaveLength(0);
  // Doble toque: dos clics en el mismo instante, antes de que React vuelva a dibujar.
  await captureButton.evaluate((element) => {
    (element as HTMLButtonElement).click();
    (element as HTMLButtonElement).click();
  });
  // Etapa 6.1 §4: captura táctil. Categoría → producto directo → producto con opciones → enviar
  // a cocina → cobrar. Todas las cifras que se comprueban las calcula Go.
  await expect(page.getByRole('heading', { name: 'Mesa 1 · Cuenta nueva' })).toBeVisible();
  expect(await openTableAccounts(driver, tableID)).toHaveLength(0);
  // Sin formulario: Go asignará la referencia neutra (el nombre de la mesa).
  await expect(page.getByLabel('Nombre o referencia de cuenta')).toHaveCount(0);
  await expect(page.locator('.capture-topbar')).toContainText('Mesa para 4');
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
  // Etapa 7.1: el primer guardado crea la cuenta con sus productos, una sola vez.
  await expect(page.getByRole('heading', { name: 'Mesa 1 · Cuenta #1' })).toBeVisible();
  await expect.poll(async () => (await openTableAccounts(driver, tableID)).length).toBe(1);
  expect((await openTableAccounts(driver, tableID))[0]!.name).toBe('Mesa 1');
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
  await expectBillFooterFits(page);
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
  // Etapa 7.1: la cancelación pide confirmación en el diálogo estándar con tono de peligro.
  const cancelDialog = page.getByRole('alertdialog', {
    name: /^¿Cancelar .*Hamburguesa de prueba\?$/,
  });
  await expect(cancelDialog).toHaveClass(/dialog-panel-danger/);
  await cancelDialog.getByRole('button', { name: 'Cancelar unidades', exact: true }).click();
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
    tableAccounts.getByRole('button', {
      name: 'Cuenta #2 · Separada de la cuenta #1',
      exact: true,
    }),
  ).toHaveAttribute('aria-pressed', 'true');
  await expect(
    tableAccounts.getByRole('button', { name: 'Nueva cuenta', exact: true }),
  ).toBeVisible();
  await tableAccounts.getByRole('button', { name: 'Cuenta #1', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Mesa 1 · Cuenta #1' })).toBeVisible();
  await expect(bill.locator('.capture-grand-total')).toContainText('$150.00');
  await tableAccounts
    .getByRole('button', { name: 'Cuenta #2 · Separada de la cuenta #1', exact: true })
    .click();
  await expect(page.getByRole('heading', { name: 'Mesa 1 · Cuenta #2' })).toBeVisible();
  // Datos de la cuenta durante la captura: nombre opcional y observaciones con la edición
  // existente; sin selector de comensales (pendiente de migración).
  await page.locator('.capture-topbar').getByRole('button', { name: 'Datos de la cuenta' }).click();
  const details = page.getByRole('region', { name: 'Datos de la cuenta #2' });
  await expect(details.getByLabel('Nombre de la cuenta (opcional)')).toHaveValue(
    'Separada de la cuenta #1',
  );
  await expect(details.getByText(/comensales/i)).toHaveCount(0);
  await details.getByLabel('Nombre de la cuenta (opcional)').fill('');
  await details.getByLabel('Observaciones').fill('Junto a la ventana');
  await details.getByRole('button', { name: 'Guardar datos de la cuenta', exact: true }).click();
  await expect(page.getByRole('region', { name: 'Productos' })).toBeVisible();
  await expect(bill).toContainText('Observaciones: Junto a la ventana');
  await expect(tableAccounts.getByRole('button', { name: 'Cuenta #2', exact: true })).toBeVisible();
  // Mesa ocupada → panel lateral: cuentas como botones, productos, totales de Go y acciones.
  await page.locator('.capture-topbar').getByRole('button', { name: /Mesas/ }).click();
  await expect(tablePanel).toBeVisible();
  await expect(tablePanel.locator('.table-panel-state')).toHaveText('Ocupada');
  await expect(tablePanel).toContainText(/Mesa para 4 · Abierta hace/);
  const panelAccounts = tablePanel.getByRole('group', { name: 'Cuentas abiertas de la mesa' });
  await expect(
    panelAccounts.getByRole('button', { name: 'Cuenta #2', exact: true }),
  ).toHaveAttribute('aria-pressed', 'true');
  await panelAccounts.getByRole('button', { name: 'Cuenta #1', exact: true }).click();
  await expect(tablePanel.getByRole('heading', { name: 'Cuenta #1', exact: true })).toBeVisible();
  await expect(
    tablePanel
      .getByRole('list', { name: 'Productos de la cuenta' })
      .getByText('1 × Filete de prueba'),
  ).toBeVisible();
  await expect(tablePanel).toContainText('En cocina');
  await expect(tablePanel.locator('.table-panel-balance')).toContainText('$150.00');
  for (const label of ['Capturar', 'Cobrar · $150.00', 'Precuenta', 'Dividir', 'Más…']) {
    await expect(tablePanel.getByRole('button', { name: label, exact: true })).toBeVisible();
  }
  await panelThemes(page, testInfo, 'table-panel', 'Capturar');
  // «Nueva cuenta» (icono +) entra a capturar en la mesa ocupada; sin productos no se crea.
  await tablePanel.getByRole('button', { name: 'Nueva cuenta', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Mesa 1 · Cuenta nueva' })).toBeVisible();
  await page.locator('.capture-topbar').getByRole('button', { name: /Mesas/ }).click();
  await expect(tablePanel).toBeVisible();
  expect(await openTableAccounts(driver, tableID)).toHaveLength(2);
  // Etapa 7.1 §2.2: una cuenta vacía (de las que dejaba la versión anterior; aquí se crea con la
  // API de prueba) se libera sin motivo con «Liberar mesa» y queda auditada en Go.
  await driver.request('POST', 'restaurant/accounts', {
    operation_id: randomUUID(),
    mode: 'dine_in',
    table_id: tableID,
  });
  const emptyAccount = panelAccounts.getByRole('button', { name: 'Cuenta #3', exact: true });
  await expect(emptyAccount).toBeVisible({ timeout: 10000 });
  await emptyAccount.click();
  await expect(tablePanel.getByRole('heading', { name: 'Cuenta #3', exact: true })).toBeVisible();
  // Una cuenta con productos no ofrece «Liberar mesa»; la vacía sí.
  await tablePanel.getByRole('button', { name: 'Liberar mesa', exact: true }).click();
  const releaseDialog = page.getByRole('alertdialog', { name: '¿Liberar Mesa 1?' });
  await expect(releaseDialog).toContainText('Cuenta #3');
  await releaseDialog.getByRole('button', { name: 'Liberar mesa', exact: true }).click();
  await expect(notice(page, 'Cuenta #3 liberada.')).toBeVisible();
  await expect.poll(async () => (await openTableAccounts(driver, tableID)).length).toBe(2);
  await expect(page.getByRole('region', { name: 'Mapa del restaurante' })).toBeVisible();
  await panelAccounts.getByRole('button', { name: 'Cuenta #1', exact: true }).click();
  await expect(tablePanel.getByRole('heading', { name: 'Cuenta #1', exact: true })).toBeVisible();
  await expect(tablePanel.getByRole('button', { name: 'Liberar mesa', exact: true })).toHaveCount(
    0,
  );
  // Capturar y cobrar desde el panel lateral.
  await panelAccounts.getByRole('button', { name: 'Cuenta #2', exact: true }).click();
  await expect(tablePanel.getByRole('heading', { name: 'Cuenta #2', exact: true })).toBeVisible();
  await tablePanel.getByRole('button', { name: 'Capturar', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Mesa 1 · Cuenta #2' })).toBeVisible();
  // Etapa 7 (fase B): turno por denominaciones, ingreso, retiro, corte parcial, arqueo ciego que
  // bloquea el cobro y cierre con faltante. Luego el cobro abre un turno nuevo en la misma caja.
  await checkCashShift(page, testInfo);
  await expect(page.getByRole('heading', { name: 'Mesa 1 · Cuenta #2' })).toBeVisible();
  await checkDineInPayment(page);
  // Etapa 7.1 §2.4: precuenta impresa → la mesa queda «Esperando pago» (Go calcula el estado y
  // el evento de la precuenta llega al mapa).
  await page
    .getByRole('group', { name: 'Vistas operativas' })
    .getByRole('button', { name: 'Mesas', exact: true })
    .click();
  await page.getByRole('button', { name: /^Mesa 1, Ocupada/ }).click();
  await expect(tablePanel.getByRole('heading', { name: 'Cuenta #1', exact: true })).toBeVisible();
  await tablePanel.getByRole('button', { name: 'Precuenta', exact: true }).click();
  const precheckPanel = page.getByRole('region', { name: 'Precuenta' });
  await precheckPanel.getByRole('button', { name: 'Imprimir precuenta', exact: true }).click();
  await expect(notice(page, /Precuenta.*(cola|Cola) de impresión/)).toBeVisible();
  await precheckPanel.getByRole('button', { name: 'Volver', exact: true }).click();
  const awaitingTable = page.getByRole('button', { name: /^Mesa 1, Esperando pago/ });
  await expect(awaitingTable).toBeVisible({ timeout: 10000 });
  await expect(awaitingTable).toContainText('Precuenta');
  await expect(tablePanel.locator('.table-panel-state')).toHaveText('Esperando pago');
  await panelThemes(page, testInfo, 'table-awaiting-payment', 'Capturar');
  await openSection(page, 'Para llevar');
  // Etapa 6.2 §2: «Nuevo pedido» entra directo a la captura, sin formulario previo.
  await page.getByRole('button', { name: 'Nuevo pedido', exact: true }).click();
  await expect(page.getByRole('heading', { name: /^Para llevar · Pedido #/ })).toBeVisible();
  await expect(page.getByRole('region', { name: 'Productos' })).toBeVisible();
  await expect(page.locator('.capture-topbar')).toContainText('Sin nombre del cliente');
  // «Datos del pedido»: origen y programación; el nombre del cliente se deja vacío.
  await page.locator('.capture-topbar').getByRole('button', { name: 'Datos del pedido' }).click();
  const order = page.getByRole('region', { name: /^Datos del pedido #/ });
  const origins = order.getByRole('group', { name: 'Origen del pedido' });
  await expect(origins.getByRole('button', { name: 'Para llevar', exact: true })).toHaveAttribute(
    'aria-pressed',
    'true',
  );
  await origins.getByRole('button', { name: 'Ventanilla', exact: true }).click();
  const scheduled = page.getByRole('button', { name: /Pedido programado/ });
  await scheduled.focus();
  await page.keyboard.press('Enter');
  await chooseOption(page.getByRole('combobox', { name: 'Programación', exact: true }), {
    value: 'delivery',
  });
  await page.getByLabel('Fecha y hora · America/Monterrey').fill('2026-12-15T19:30');
  await scheduled.click();
  await scheduled.focus();
  await page.keyboard.press('Space');
  await expect(page.getByLabel('Fecha y hora · America/Monterrey')).toHaveValue('2026-12-15T19:30');
  await order.getByRole('button', { name: 'Guardar datos del pedido', exact: true }).click();
  await expect(page.getByRole('region', { name: 'Productos' })).toBeVisible();
  await expect(page.locator('.capture-topbar')).toContainText('Ventanilla');
  await expect(page.locator('.capture-topbar')).toContainText('Entrega programada');
  // La cuenta actual es ahora el pedido para llevar recién abierto.
  await expect(bill).toContainText('Pago: sin registro de cobro');
  await expect(bill).toContainText('Entrega: no entregado');
  // Sin productos enviados no se entrega ni se cobra: «Cobrar» conserva su lugar desactivado.
  await expect(page.getByRole('button', { name: /Marcar.*entregado/ })).toHaveCount(0);
  await expect(bill.getByRole('button', { name: 'Cobrar', exact: true })).toBeDisabled();
  // Se envía a cocina sin nombre: no se bloquea y la captura avisa con discreción.
  await sendTakeawayToKitchen(page);
  const unnamed = page.locator('.capture-topbar').getByRole('status');
  await expect(unnamed).toHaveText(/^Pedido sin nombre · se identifica como Pedido #\d+$/);
  await page.locator('.capture-topbar').getByRole('button', { name: 'Datos del pedido' }).click();
  await order.getByLabel('Nombre del cliente (opcional)').fill('Cliente temporal');
  await order.getByRole('button', { name: 'Guardar datos del pedido', exact: true }).click();
  await expect(bill).toContainText('Cliente temporal');
  await expect(unnamed).toHaveCount(0);
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
  await page.getByRole('button', { name: 'Nuevo pedido', exact: true }).click();
  await page.locator('.capture-topbar').getByRole('button', { name: 'Datos del pedido' }).click();
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
  const save = page.getByRole('button', { name: 'Guardar datos del pedido', exact: true });
  await save.scrollIntoViewIfNeeded();
  const saveBounds = await save.boundingBox();
  expect(saveBounds!.y + saveBounds!.height).toBeLessThanOrEqual(keyboard!.y);
  await page.screenshot({ path: testInfo.outputPath('keyboard-1280x720.png') });
  await page.getByRole('button', { name: 'Cerrar teclado', exact: true }).click();
  // Etapa 6.1 §2.6: la vista previa salió del menú principal y vive dentro de Catálogo.
  await expect(
    page.getByRole('button', { name: 'Probar personalización', exact: true }),
  ).toHaveCount(0);
  // Etapa 9C: desde la captura se vuelve con «‹ Para llevar» y se entra a Administración → Catálogo.
  await openAdministration(page, 'Catálogo');
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
  // Etapa 7.1 (fase D): en todo el recorrido cada éxito se retiró a su tiempo y ningún aviso tapado
  // dejó ver su contenido debajo del más reciente.
  await expectNoticesClosedOnTime(page);
  expect(errors).toEqual([]);
});
