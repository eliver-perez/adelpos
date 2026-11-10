import { expect, test } from '@playwright/test';
import type { Page, TestInfo } from '@playwright/test';
import { installDriver } from './native-driver';
import { notice } from './notices';

// Etapa 8, fase A: recorrido corto de Inventarios con el mismo transporte de prueba que
// restaurant.spec.ts (Go temporal y base adel_pos_test_*). Crea un insumo con una presentación,
// registra su existencia inicial y la comprueba en Existencias y en Movimientos.

// La página no se desborda a lo ancho a 1280 × 720 y 1920 × 1080, en tema claro y oscuro. Con
// «capture» guarda la vista a 1280 × 720 en ambos temas.
async function checkLayout(page: Page, testInfo: TestInfo, name: string, capture: boolean) {
  for (const size of [
    { width: 1920, height: 1080 },
    { width: 1280, height: 720 },
  ]) {
    await page.setViewportSize(size);
    for (const theme of ['light', 'dark']) {
      await page.evaluate((value) => {
        document.documentElement.dataset.theme = value;
      }, theme);
      // Deja terminar el cambio de tamaño y las transiciones de color antes de medir.
      await page.waitForTimeout(300);
      const overflow = await page.evaluate(
        () => document.documentElement.scrollWidth - window.innerWidth,
      );
      expect(overflow, `${name} se desborda a ${size.width} × ${size.height} (${theme})`).toBe(0);
      if (capture && size.width === 1280) {
        // La captura completa de la página (ancho de 1280) se toma sin avisos encima.
        await expect(page.locator('[data-sonner-toast]')).toHaveCount(0, { timeout: 8000 });
        await page.screenshot({
          path: testInfo.outputPath(`inventory-${name}-${theme}-1280x720.png`),
          fullPage: true,
        });
      }
    }
  }
  await page.evaluate(() => {
    document.documentElement.dataset.theme = 'light';
  });
}

async function openTab(page: Page, name: string) {
  await page.getByRole('tab', { name, exact: true }).click();
  await expect(page.getByRole('tab', { name, exact: true })).toHaveAttribute(
    'aria-selected',
    'true',
  );
}

test('inventarios: insumo con presentación, existencia inicial, existencias y movimientos', async ({
  page,
}, testInfo) => {
  const errors: string[] = [];
  page.on('pageerror', (error) => errors.push(error.message));
  const driver = await installDriver(page);
  await page.setViewportSize({ width: 1280, height: 720 });
  await page.goto('/');
  await page.getByLabel('Usuario', { exact: true }).fill('admin');
  await page.getByLabel('Contraseña', { exact: true }).fill('admin');
  await page.getByRole('button', { name: 'Entrar', exact: true }).click();
  await page
    .getByRole('navigation', { name: 'Navegación principal' })
    .getByRole('button', { name: 'Inventarios', exact: true })
    .click();
  await expect(page.getByRole('heading', { name: 'Inventarios', exact: true })).toBeVisible();
  await expect(
    page.getByRole('tablist', { name: 'Secciones de inventario' }).getByRole('tab'),
  ).toHaveText([
    'Resumen',
    'Insumos',
    'Recetas',
    'Almacenes',
    'Existencias',
    'Movimientos',
    'Transferencias',
    'Ajustes y mermas',
    'Conteos',
    'Compras',
    'Recepciones',
    'Proveedores',
    'Configuración',
  ]);

  // Resumen: la función sembrada por la 0010 está desactivada y se administra en Configuración.
  const feature = page.getByRole('region', { name: 'Control de inventario' });
  await expect(feature).toContainText('Desactivado');
  await expect(feature.getByRole('button', { name: 'Administrar funciones' })).toBeVisible();
  await expect(page.getByRole('region', { name: 'Cifras de inventario' })).toContainText(
    'Almacenes activos',
  );
  await checkLayout(page, testInfo, 'resumen-inicial', false);

  // Insumos: alta en kilogramos y presentación capturada en gramos (Go la convierte).
  await openTab(page, 'Insumos');
  await page.getByRole('button', { name: 'Nuevo insumo', exact: true }).click();
  await page.getByLabel('Nombre del insumo', { exact: true }).fill('Harina de prueba');
  await page.getByLabel('Código interno', { exact: true }).fill('HAR-E2E');
  await page
    .getByRole('combobox', { name: /^Unidad base/ })
    .selectOption({ label: 'Kilogramo (kg)' });
  await page.getByLabel(/^Costo de referencia/).fill('18.5');
  await page.getByLabel(/^Existencia mínima/).fill('5');
  await expect(page.locator('input[type="number"]')).toHaveCount(0);
  await page.getByRole('button', { name: 'Crear insumo', exact: true }).click();
  await expect(notice(page, 'Insumo creado. Ya puedes agregar sus presentaciones.')).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Harina de prueba', exact: true })).toBeVisible();
  await page.getByRole('button', { name: 'Nueva presentación', exact: true }).click();
  await page.getByLabel(/^Nombre de la presentación/).fill('Costal 25 kg');
  await page.getByLabel(/^Contenido/).fill('25000');
  await page
    .getByRole('combobox', { name: /^Unidad del contenido/ })
    .selectOption({ label: 'Gramo (g)' });
  await page.getByRole('button', { name: 'Predeterminada para compra', exact: true }).click();
  await page.getByRole('button', { name: 'Agregar presentación', exact: true }).click();
  await expect(notice(page, 'Presentación agregada.')).toBeVisible();
  const presentations = page.getByRole('region', { name: 'Presentaciones de compra' });
  await expect(presentations).toContainText('Costal 25 kg');
  await expect(presentations).toContainText('25 kg');
  await checkLayout(page, testInfo, 'insumo-editor', true);
  await page.getByRole('button', { name: 'Volver al listado', exact: true }).click();
  const itemRow = page.getByRole('row').filter({ hasText: 'Harina de prueba' });
  await expect(itemRow).toContainText('HAR-E2E');
  await expect(itemRow).toContainText('compra: Costal 25 kg');
  await expect(itemRow).toContainText('$18.50 / kg');
  await checkLayout(page, testInfo, 'insumos', true);

  // Existencia inicial: 2 costales a $450 cada uno.
  await openTab(page, 'Existencias');
  await page.getByRole('button', { name: 'Existencia inicial', exact: true }).click();
  const line = page.getByRole('group', { name: 'Renglón 1' });
  await line.getByLabel('Buscar insumo', { exact: true }).fill('HAR-E2E');
  await line
    .getByRole('combobox', { name: 'Insumo', exact: true })
    .selectOption({ label: 'Harina de prueba · HAR-E2E' });
  await line.getByLabel('Cantidad', { exact: true }).fill('2');
  await line
    .getByRole('combobox', { name: 'Unidad o presentación', exact: true })
    .selectOption({ label: 'Costal 25 kg (25\u00a0kg)' });
  await line.getByLabel(/^Costo unitario/).fill('450');
  await page.getByLabel(/^Motivo u observación/).fill('Inventario de apertura E2E');
  await checkLayout(page, testInfo, 'existencia-inicial', true);
  await page.getByRole('button', { name: 'Registrar existencia inicial', exact: true }).click();
  const dialog = page.getByRole('alertdialog');
  await expect(dialog).toContainText('Se registrará 1 renglón en «Almacén general»');
  await dialog.getByRole('button', { name: 'Registrar existencia inicial', exact: true }).click();
  await expect(
    notice(page, 'Existencia inicial registrada: 1 renglón en «Almacén general».'),
  ).toBeVisible();
  const result = page.getByRole('region', { name: 'Existencia inicial registrada' });
  await expect(result).toContainText('2 · Costal 25 kg');
  await expect(result).toContainText('50 kg');
  await expect(result).toContainText('$900.00');
  await result.getByRole('button', { name: 'Volver a existencias', exact: true }).click();
  const stockRow = page.getByRole('row').filter({ hasText: 'Harina de prueba' });
  await expect(stockRow).toContainText('50 kg');
  await expect(stockRow).toContainText('Almacén general: 50 kg');
  await expect(stockRow).toContainText('$18.00 / kg');
  await expect(stockRow).toContainText('$900.00');
  await expect(page.locator('.inventory-total')).toContainText('$900.00');
  await checkLayout(page, testInfo, 'existencias', true);

  // Movimientos: el libro registra la entrada con su origen y responsable, sin identificadores.
  await openTab(page, 'Movimientos');
  const movement = page.getByRole('row').filter({ hasText: 'Harina de prueba' });
  await expect(movement).toContainText('+50 kg');
  await expect(movement).toContainText('Existencia inicial');
  await expect(movement).toContainText('Inventario de apertura E2E');
  await expect(movement).toContainText('Administrador');
  await expect(movement).toContainText('+$900.00');
  await checkLayout(page, testInfo, 'movimientos', true);

  // El resumen refleja el insumo y su valor (el administrador tiene los permisos de valuación).
  await openTab(page, 'Resumen');
  const figures = page.getByRole('region', { name: 'Cifras de inventario' });
  await expect(figures.locator('.inventory-figure').first()).toContainText('1');
  await expect(figures).toContainText('$900.00');
  await checkLayout(page, testInfo, 'resumen', true);

  // El resto de las pestañas tampoco se desborda.
  await openTab(page, 'Almacenes');
  await expect(page.getByRole('region', { name: 'Almacenes', exact: true })).toContainText(
    'Predeterminado',
  );
  await checkLayout(page, testInfo, 'almacenes', true);
  await openTab(page, 'Configuración');
  await expect(page.getByLabel(/^Hora de corte/)).toHaveValue('05:00');
  await checkLayout(page, testInfo, 'configuracion', true);

  // Go confirma lo que mostró la pantalla: 50 kg en el almacén general y un movimiento inicial.
  const stock = await driver.request('GET', 'inventory/stock?search=HAR-E2E');
  expect(stock.body.items[0].quantity).toBe('50');
  const movements = await driver.request('GET', 'inventory/movements?kind=initial');
  expect(movements.body.total).toBe(1);
  expect(errors).toEqual([]);
});
