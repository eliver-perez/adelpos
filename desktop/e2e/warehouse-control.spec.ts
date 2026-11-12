import { randomUUID } from 'node:crypto';
import { expect, test } from '@playwright/test';
import type { Locator, Page, TestInfo } from '@playwright/test';
import { installDriver } from './native-driver';
import { openAdministration, topBar } from './navigation';
import { notice } from './notices';
import {
  allInventorySections,
  checkInventoryNavigation,
  expectInventorySection,
  groupTablist,
  inventoryGroups,
  openInventorySection,
} from './inventory-navigation';
import { chooseOption, expectOptionCount } from './select';

// Etapa 8, fase D (interfaz de control de inventario): recorrido de transferencias, mermas,
// conteos físicos y alertas con el mismo transporte de prueba que las demás especificaciones (Go
// temporal y base adel_pos_test_*). Prepara por la API del puente un insumo propio con 20 kg en el
// almacén general (mínimo de 100 kg para que Go emita la alerta «Bajo mínimo») y un segundo
// almacén; después, desde la pantalla, transfiere 5 kg a ese almacén, registra una merma de 1 kg,
// cuenta 4.5 kg en el segundo almacén (diferencia de −0.5 kg calculada por Go), lo aprueba y lo
// aplica, y comprueba el movimiento y la alerta en el resumen. Recorre todas las pestañas sin
// desbordamiento horizontal. El nombre del archivo la ordena después de las especificaciones
// existentes en la corrida completa: no altera lo que ellas comprueban.

type Driver = Awaited<ReturnType<typeof installDriver>>;

const itemName = 'Harina E2E D';
const itemCode = 'HAR-E2E-D';
const itemLabel = `${itemName} · ${itemCode}`;
const barName = 'Barra E2E D';

async function setTheme(page: Page, theme: string) {
  await page.evaluate((value) => {
    document.documentElement.dataset.theme = value;
  }, theme);
}

// Fija el tema y espera a que siga aplicado después de una pausa (un refresco de la sesión puede
// volver a aplicar el tema del usuario entre el cambio y la captura).
async function keepTheme(page: Page, theme: string) {
  await expect
    .poll(async () => {
      await setTheme(page, theme);
      await page.waitForTimeout(150);
      return page.evaluate(() => document.documentElement.dataset.theme);
    })
    .toBe(theme);
}

// La página no se desborda a lo ancho a 1920 × 1080 ni a 1280 × 720, en tema claro y oscuro. Con
// «capture» guarda la vista a 1280 × 720 en ambos temas.
async function checkLayout(page: Page, testInfo: TestInfo, name: string, capture: boolean) {
  for (const size of [
    { width: 1920, height: 1080 },
    { width: 1280, height: 720 },
  ]) {
    await page.setViewportSize(size);
    for (const theme of ['light', 'dark']) {
      await setTheme(page, theme);
      await page.waitForTimeout(300);
      const overflow = await page.evaluate(
        () => document.documentElement.scrollWidth - window.innerWidth,
      );
      expect(overflow, `${name} se desborda a ${size.width} × ${size.height} (${theme})`).toBe(0);
      if (capture && size.width === 1280) {
        await expect(page.locator('[data-sonner-toast]')).toHaveCount(0, { timeout: 9000 });
        // La aplicación vuelve a aplicar el tema del usuario al refrescar la sesión: se fija de
        // nuevo hasta que se conserva justo antes de la captura.
        await keepTheme(page, theme);
        await page.screenshot({
          path: testInfo.outputPath(`d2-${name}-${theme}-1280x720.png`),
          fullPage: true,
        });
      }
    }
  }
  await setTheme(page, 'light');
}

// La navegación agrupada (etapa 9A) cabe a 1280 × 720 y a 1920 × 1080 sin desplazarse a lo ancho:
// cada fila (grupos y secciones de cada grupo) en una sola línea de columnas iguales, con pestañas
// de al menos 44 px, en ambos temas; a 1280 × 720 se captura cada grupo.
async function checkTabBar(page: Page, testInfo: TestInfo) {
  for (const size of [
    { width: 1920, height: 1080 },
    { width: 1280, height: 720 },
  ]) {
    for (const theme of ['light', 'dark']) {
      await setTheme(page, theme);
      await checkInventoryNavigation(page, size, async (name) => {
        if (size.width !== 1280) {
          return;
        }
        await keepTheme(page, theme);
        await page.getByRole('navigation', { name: 'Inventarios' }).screenshot({
          path: testInfo.outputPath(`9a-${name}-${theme}-1280x720.png`),
        });
      });
    }
  }
  await setTheme(page, 'light');
}

// Abre una sección de Inventarios por su grupo y su nombre (navegación agrupada de la etapa 9A).
async function openTab(page: Page, name: string) {
  await openInventorySection(page, name);
}

async function confirm(page: Page, button: string, text?: string | RegExp) {
  const dialog = page.getByRole('alertdialog');
  await expect(dialog).toBeVisible();
  if (text) {
    await expect(dialog).toContainText(text);
  }
  await dialog.getByRole('button', { name: button, exact: true }).click();
  await expect(dialog).toHaveCount(0);
}

async function created(driver: Driver, route: string, body: Record<string, unknown>) {
  const response = await driver.request('POST', route, { operation_id: randomUUID(), ...body });
  expect(response.status, `${route}: ${JSON.stringify(response.body)}`).toBe(200);
  return response.body as Record<string, unknown> & { id: string };
}

// Precondiciones por la API con la sesión del administrador: el insumo en kilogramos con mínimo de
// 100 kg, su existencia inicial de 20 kg a $18 en el almacén general y el segundo almacén.
async function preparePreconditions(driver: Driver) {
  const units = await driver.request('GET', 'inventory/units');
  const kilogram = (units.body as { id: string; code: string }[]).find(
    (unit) => unit.code === 'kg',
  );
  expect(kilogram, 'unidad kg').toBeTruthy();
  const flour = await created(driver, 'inventory/items', {
    code: itemCode,
    name: itemName,
    description: '',
    base_unit_id: kilogram!.id,
    kind: 'purchased',
    active: true,
    min_stock: '100',
  });
  const warehouses = await driver.request('GET', 'inventory/warehouses');
  const general = (warehouses.body as { id: string; is_default: boolean }[]).find(
    (warehouse) => warehouse.is_default,
  );
  expect(general, 'almacén predeterminado').toBeTruthy();
  await created(driver, 'inventory/initial-balances', {
    warehouse_id: general!.id,
    reason: 'Existencia de la prueba D2',
    lines: [{ item_id: flour.id, quantity: '20', unit_cost: '18' }],
  });
  await created(driver, 'inventory/warehouses', {
    name: barName,
    description: 'Almacén de la prueba D2',
    active: true,
  });
}

async function chooseItem(scope: Locator, label = 'Insumo') {
  const select = scope.getByRole('combobox', { name: new RegExp(`^${label}`) });
  await chooseOption(select, itemLabel, { search: itemCode });
}

test('control de inventario: transferencia, merma, conteo aplicado y alerta en el resumen', async ({
  page,
}, testInfo) => {
  test.setTimeout(240000);
  const errors: string[] = [];
  page.on('pageerror', (error) => errors.push(error.message));
  const driver = await installDriver(page);
  // En la corrida completa restaurant.spec.ts deja el teclado virtual siempre visible en la
  // terminal; esta especificación lo desactiva antes de entrar.
  await driver.request('POST', 'auth/login', { username: 'admin', password: 'admin' });
  const keyboard = await driver.request('PUT', 'terminal/preferences', {
    keyboard_mode: 'disabled',
  });
  expect(keyboard.status, JSON.stringify(keyboard.body)).toBeLessThan(300);
  await driver.request('POST', 'auth/logout');
  await page.setViewportSize({ width: 1280, height: 720 });
  await page.goto('/');
  await page.getByLabel('Usuario', { exact: true }).fill('admin');
  await page.getByLabel('Contraseña', { exact: true }).fill('admin');
  await page.getByRole('button', { name: 'Entrar', exact: true }).click();
  // Etapa 9C: Inventarios, Reportes y Documentos son mosaicos de Administración.
  await expect(
    topBar(page).getByRole('button', { name: 'Administración', exact: true }),
  ).toBeVisible();
  await preparePreconditions(driver);
  await openAdministration(page, 'Inventarios');
  await expect(page.getByRole('heading', { name: 'Inventarios', exact: true })).toBeVisible();
  // Los cinco grupos con sus secciones en el orden de la especificación (§3.6 de la etapa 9A).
  await expect(groupTablist(page).getByRole('tab')).toHaveText(inventoryGroups);
  await checkTabBar(page, testInfo);

  // 1. Transferencia de 5 kg del almacén general a la barra: Go registra salida y entrada.
  await openTab(page, 'Transferencias');
  await page.getByRole('button', { name: 'Nueva transferencia', exact: true }).click();
  const destination = page.getByRole('combobox', { name: /^Almacén de destino/ });
  await expectOptionCount(destination, barName, 1);
  // El destino no ofrece el almacén de origen.
  await expectOptionCount(destination, 'Almacén general', 0);
  await chooseOption(destination, barName);
  const transferLine = page.getByRole('group', { name: 'Renglón 1' });
  await chooseItem(transferLine);
  await transferLine.getByLabel(/^Cantidad/).fill('5');
  await expect(page.locator('input[type="number"]')).toHaveCount(0);
  await checkLayout(page, testInfo, 'transferencia-captura', true);
  await page
    .locator('form')
    .getByRole('button', { name: 'Registrar transferencia', exact: true })
    .click();
  await confirm(page, 'Registrar transferencia', `De «Almacén general» a «${barName}»`);
  await expect(notice(page, /^Transferencia \d+ registrada/)).toBeVisible();
  const transferPanel = page.getByRole('region', { name: /^Transferencia \d+$/ });
  await expect(transferPanel).toContainText(`Almacén general → ${barName}`);
  const transferRow = transferPanel.getByRole('row').filter({ hasText: itemName });
  await expect(transferRow).toContainText('15 kg');
  await expect(transferRow).toContainText('5 kg');
  await expect(transferRow).toContainText('$90.00');
  await checkLayout(page, testInfo, 'transferencia', true);
  await transferPanel.getByRole('button', { name: 'Volver al listado', exact: true }).click();
  await expect(
    page.getByRole('region', { name: 'Transferencias registradas' }).getByRole('row', {
      name: new RegExp(`Almacén general → ${barName}`),
    }),
  ).toHaveCount(1);

  // 2. Merma de 1 kg por rotura en el almacén general: solo salida, se aplica directo.
  await openTab(page, 'Ajustes y mermas');
  await expect(
    page.getByRole('heading', { name: 'Cancelaciones por clasificar', exact: true }),
  ).toBeVisible();
  await page.getByRole('button', { name: 'Registrar merma', exact: true }).click();
  await page
    .getByRole('group', { name: 'Motivo de la merma' })
    .getByRole('button', { name: 'Rotura', exact: true })
    .click();
  const wasteLine = page.getByRole('group', { name: 'Renglón 1' });
  await chooseItem(wasteLine);
  await wasteLine.getByLabel(/^Cantidad/).fill('1');
  await expect(wasteLine).toContainText('La merma solo registra salidas');
  await expect(page.locator('input[type="number"]')).toHaveCount(0);
  await checkLayout(page, testInfo, 'merma-captura', true);
  await page.locator('form').getByRole('button', { name: 'Registrar merma', exact: true }).click();
  await confirm(page, 'Registrar merma', 'La merma se aplica de inmediato');
  await expect(notice(page, /^Merma \d+ registrada y aplicada\.$/)).toBeVisible();
  const wastePanel = page.getByRole('region', { name: /^Merma \d+$/ });
  await expect(wastePanel).toContainText('Aplicado');
  const wasteRow = wastePanel.getByRole('row').filter({ hasText: itemName });
  await expect(wasteRow).toContainText('−1 kg');
  await expect(wasteRow).toContainText('Merma');
  await expect(wasteRow).toContainText('14 kg');
  await wastePanel.getByRole('button', { name: 'Volver al listado', exact: true }).click();
  const wasteListRow = page
    .getByRole('region', { name: 'Ajustes y mermas registrados' })
    .getByRole('row')
    .filter({ hasText: 'Rotura' });
  await expect(wasteListRow).toContainText('Aplicado');
  await checkLayout(page, testInfo, 'ajustes-y-mermas', true);

  // 3. Conteo de la barra: 4.5 kg contados contra 5 kg teóricos a la hora del renglón.
  await openTab(page, 'Conteos');
  await page.getByRole('button', { name: 'Nuevo conteo', exact: true }).click();
  await chooseOption(page.getByRole('combobox', { name: /^Almacén que se cuenta/ }), barName);
  await page.getByRole('button', { name: 'Selección de insumos', exact: true }).click();
  await chooseItem(page.getByRole('region', { name: 'Nuevo conteo' }));
  await page.getByRole('button', { name: 'Agregar a la selección', exact: true }).click();
  await expect(page.getByRole('list', { name: 'Insumos elegidos' })).toContainText(itemName);
  await page.getByRole('button', { name: 'Crear conteo', exact: true }).click();
  await expect(notice(page, /^Conteo \d+ abierto en «Barra E2E D» con 1 insumo\.$/)).toBeVisible();
  const countPanel = page.getByRole('region', { name: /^Conteo \d+ · Barra E2E D$/ });
  await expect(countPanel).toContainText('La venta sigue durante el conteo');
  const countLine = countPanel.getByRole('listitem', { name: `Renglón de ${itemName}` });
  await countLine.getByRole('textbox', { name: `Cantidad contada de ${itemName}` }).fill('4.5');
  await expect(countLine).toContainText('Sin guardar');
  await expect(page.locator('input[type="number"]')).toHaveCount(0);
  await countPanel.getByRole('button', { name: 'Guardar avance', exact: true }).click();
  await expect(notice(page, 'Avance guardado: 1 renglón.')).toBeVisible();
  await expect(countLine).toContainText('Contado');
  // Quien aprueba ve el teórico y la diferencia que calcula Go.
  await expect(countLine).toContainText('Teórico a esa hora');
  await expect(countLine).toContainText('5 kg');
  await expect(countLine).toContainText('−0.5 kg');
  await expect(countLine).toContainText('Faltante');
  await checkLayout(page, testInfo, 'conteo-captura', true);
  await countPanel.getByRole('button', { name: 'Enviar a revisión', exact: true }).click();
  await confirm(page, 'Enviar a revisión', 'Se enviará con 1 de 1 insumo contado');
  await expect(notice(page, /^Conteo \d+ enviado a revisión\.$/)).toBeVisible();
  await countPanel.getByRole('button', { name: 'Aprobar', exact: true }).click();
  await confirm(page, 'Aprobar conteo');
  await expect(notice(page, /^Conteo \d+ aprobado\.$/)).toBeVisible();
  await expect(countPanel).toContainText('Aprobó');
  await checkLayout(page, testInfo, 'conteo-diferencias', true);
  await countPanel.getByRole('button', { name: 'Aplicar ajustes', exact: true }).click();
  await confirm(page, 'Aplicar ajustes', 'se registrará 1 movimiento de ajuste');
  await expect(notice(page, /^Conteo \d+ aplicado: se registraron sus ajustes\.$/)).toBeVisible();
  await expect(countLine).toContainText('Faltante de conteo');
  await expect(countLine).toContainText('Teórico aplicado');
  await expect(countPanel).toContainText('Aplicado');

  // El movimiento del conteo queda en el libro con su origen.
  await openTab(page, 'Movimientos');
  const countMovement = page
    .getByRole('row')
    .filter({ hasText: itemName })
    .filter({ hasText: 'Faltante de conteo' });
  await expect(countMovement).toContainText('−0.5 kg');
  await expect(countMovement).toContainText(barName);
  await expect(countMovement).toContainText('Conteo físico');

  // 4. Resumen: la alerta «Bajo mínimo» del insumo (14 + 4.5 kg contra 100 kg) es visible.
  await openTab(page, 'Resumen');
  const alerts = page.getByRole('region', { name: 'Alertas de inventario' });
  const belowMinimum = alerts.getByRole('list', { name: 'Bajo mínimo' });
  await expect(belowMinimum).toContainText(itemName);
  await expect(
    belowMinimum.getByRole('button', { name: `Ver existencias: ${itemName}` }),
  ).toBeVisible();
  await checkLayout(page, testInfo, 'resumen-alertas', true);
  // La alerta lleva a Existencias con el insumo buscado.
  await belowMinimum.getByRole('button', { name: `Ver existencias: ${itemName}` }).click();
  await expectInventorySection(page, 'Existencias');
  await expect(page.getByLabel('Buscar insumo', { exact: true })).toHaveValue(itemCode);
  await expect(page.getByRole('row').filter({ hasText: itemName })).toContainText('18.5 kg');

  // 5. Ninguna sección se desborda a lo ancho a 1280 × 720 ni a 1920 × 1080.
  for (const tab of allInventorySections) {
    await openTab(page, tab);
    await page.waitForTimeout(200);
    await checkLayout(page, testInfo, `pestana-${tab}`, false);
  }

  // Go confirma lo que mostró la pantalla.
  const stock = await driver.request('GET', `inventory/stock?search=${itemCode}`);
  expect(stock.body.items[0].quantity).toBe('18.5');
  const counts = await driver.request('GET', 'inventory/counts?state=applied');
  expect(counts.body.total).toBeGreaterThanOrEqual(1);
  expect(errors).toEqual([]);
});
