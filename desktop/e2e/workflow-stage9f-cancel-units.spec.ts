import { randomUUID } from 'node:crypto';
import { copyFileSync, mkdirSync } from 'node:fs';
import { join } from 'node:path';
import { expect, test } from '@playwright/test';
import type { Page, TestInfo } from '@playwright/test';
import { installDriver } from './native-driver';
import { openAdministration, openSection } from './navigation';
import { notice } from './notices';
import { chooseOption } from './select';

// Etapa 9F, fase E (§7.2): un cajero sin «Cancelar partidas o cantidades confirmadas» cancela una
// unidad ya enviada a cocina con la autorización del administrador. Go le indica desde el principio
// que hace falta un supervisor (`cancel_units_requires_authorization`), el panel lo pide, el evento
// guarda quién canceló y quién autorizó, y «Modificaciones de cantidades» y «Productos cancelados»
// muestran «Autorizó» en pantalla y en las exportaciones PDF y Excel. Además comprueba que la ruta
// antigua de anulación responde 410 sin tocar la cuenta. Mismo transporte que las demás
// especificaciones (Go temporal y base adel_pos_test_*). Capturas en tema claro y oscuro a 1280 × 720
// y 1920 × 1080 (copia opcional en ADEL_SHOTS_DIR).

type Driver = Awaited<ReturnType<typeof installDriver>>;

async function call(driver: Driver, method: string, route: string, body?: unknown) {
  const response = await driver.request(method, route, body);
  expect(response.status, `${method} ${route}: ${JSON.stringify(response.body)}`).toBe(200);
  return response.body;
}

// Cuenta abierta en la mesa con el producto confirmado (enviado a cocina).
async function sentAccount(driver: Driver, tableID: string, productID: string, quantity: number) {
  const opened = await call(driver, 'POST', 'restaurant/accounts', {
    operation_id: randomUUID(),
    mode: 'dine_in',
    table_id: tableID,
  });
  const id = opened.account.id as string;
  const route = `restaurant/accounts/${id}`;
  const drafted = await call(driver, 'PUT', `${route}/draft`, {
    operation_id: randomUUID(),
    revision: opened.account.revision,
    items: [{ id: randomUUID(), product_id: productID, variant_id: null, quantity }],
  });
  const preview = await call(driver, 'GET', `${route}/preview`);
  await call(driver, 'POST', `${route}/confirm`, {
    operation_id: randomUUID(),
    revision: drafted.account.revision,
    expected_fingerprint: preview.fingerprint,
  });
  const bill = await call(driver, 'GET', route);
  return { id, folio: bill.account.folio as number, revision: bill.account.revision as number };
}

const sizes = [
  { width: 1280, height: 720 },
  { width: 1920, height: 1080 },
];
const themes = ['light', 'dark'] as const;

async function setTheme(page: Page, theme: string) {
  await page.evaluate((value) => {
    document.documentElement.dataset.theme = value;
  }, theme);
}

function keep(testInfo: TestInfo, file: string) {
  const directory = process.env.ADEL_SHOTS_DIR;
  if (directory) {
    mkdirSync(directory, { recursive: true });
    copyFileSync(
      testInfo.outputPath(file),
      join(directory, `${process.env.ADEL_BROWSER_ENGINE ?? 'browser'}-${file}`),
    );
  }
}

// Cierra los avisos que sigan a la vista para que no tapen las capturas.
async function clearToasts(page: Page) {
  const close = page.locator('[data-sonner-toaster] .app-toast-close');
  for (let attempt = 0; attempt < 10 && (await close.count()) > 0; attempt += 1) {
    await close
      .first()
      .click({ timeout: 2000 })
      .catch(() => undefined);
    await page.waitForTimeout(150);
  }
  await expect(page.locator('[data-sonner-toast]')).toHaveCount(0, { timeout: 15000 });
}

// Recorre los dos tamaños y los dos temas: sin desbordamiento horizontal, la comprobación de la
// vista y una captura de la página.
async function views(page: Page, testInfo: TestInfo, name: string, check?: () => Promise<void>) {
  await clearToasts(page);
  for (const size of sizes) {
    await page.setViewportSize(size);
    for (const theme of themes) {
      const file = `${name}-${theme}-${size.width}x${size.height}.png`;
      // La aplicación vuelve a aplicar el tema del usuario cuando se actualiza su sesión: la
      // captura se repite si el tema cambió mientras se tomaba.
      for (let attempt = 0; attempt < 3; attempt += 1) {
        await setTheme(page, theme);
        await page.waitForTimeout(200);
        const overflow = await page.evaluate(
          () => document.documentElement.scrollWidth - window.innerWidth,
        );
        expect(overflow, `${name} se desborda a ${size.width} × ${size.height} (${theme})`).toBe(0);
        await check?.();
        await page.screenshot({ path: testInfo.outputPath(file) });
        const kept = await page.evaluate(() => document.documentElement.dataset.theme);
        if (kept === theme) {
          break;
        }
      }
      keep(testInfo, file);
    }
  }
  await setTheme(page, 'light');
  await page.setViewportSize({ width: 1280, height: 720 });
}

async function login(page: Page, username: string, password: string) {
  await page.getByLabel('Usuario', { exact: true }).fill(username);
  await page.getByLabel('Contraseña', { exact: true }).fill(password);
  await page.getByRole('button', { name: 'Entrar', exact: true }).click();
}

async function logout(page: Page) {
  await page.getByRole('button', { name: /^Menú del usuario/ }).click();
  await page
    .getByRole('group', { name: 'Menú del usuario' })
    .getByRole('button', { name: 'Cerrar sesión', exact: true })
    .click();
  await expect(page.getByRole('heading', { name: 'Inicia sesión' })).toBeVisible();
}

// Abre un reporte de un grupo: por debajo de 1440 px con el selector «Reporte» y en pantallas
// anchas con la lista lateral.
async function openReport(page: Page, group: string, title: string) {
  await page
    .getByRole('tablist', { name: 'Grupos de reportes' })
    .getByRole('tab', { name: group, exact: true })
    .click();
  const picker = page.getByRole('combobox', { name: 'Reporte', exact: true });
  if (await picker.isVisible()) {
    await chooseOption(picker, title);
  } else {
    await page
      .getByRole('navigation', { name: `Reportes de ${group}` })
      .getByRole('button', { name: title, exact: true })
      .click();
  }
  await expect(page.getByRole('heading', { name: title, exact: true })).toBeVisible();
  return page.getByRole('region', { name: `Tabla: ${title}` });
}

test('cajero sin permiso cancela unidades con la autorización del administrador y el reporte muestra «Autorizó» (etapa 9F, fase E)', async ({
  page,
}, testInfo) => {
  test.setTimeout(300000);
  const errors: string[] = [];
  page.on('pageerror', (error) => errors.push(error.message));
  const driver = await installDriver(page);
  await call(driver, 'POST', 'auth/login', { username: 'admin', password: 'admin' });
  await call(driver, 'PUT', 'terminal/preferences', { keyboard_mode: 'disabled' });

  // 1. Datos por la API: una mesa con tres productos enviados a cocina y un cajero sin «Cancelar
  // partidas o cantidades confirmadas» (sí abre cuentas y captura).
  const category = await call(driver, 'POST', 'catalog/categories', {
    name: 'Platillos 9F-E',
    active: true,
    color: '#d64545',
  });
  const product = await call(driver, 'POST', 'catalog/products', {
    name: 'Tacos 9F-E',
    active: true,
    category_id: category.id,
    code: 'TACOS-9FE',
    kind: 'product',
    base_price: '95',
  });
  const areaName = 'Terraza 9F-E';
  const area = (await call(driver, 'POST', 'restaurant/areas', {
    operation_id: randomUUID(),
    name: areaName,
    active: true,
  })) as { id: string };
  const mesa = (await call(driver, 'POST', 'restaurant/tables', {
    operation_id: randomUUID(),
    area_id: area.id,
    name: 'Mesa F1',
    shape: 'rectangle',
    x: 40,
    y: 60,
    width: 120,
    height: 100,
    seats: 4,
    active: true,
  })) as { id: string };
  const account = await sentAccount(driver, mesa.id, product.id as string, 3);
  const role = await call(driver, 'POST', 'roles', {
    name: 'Cajero sin cancelar unidades 9F',
    permissions: [
      'accounts.read',
      'accounts.open',
      'restaurant.tables.read',
      'restaurant.areas.read',
      'orders.capture',
      'orders.confirm',
      'orders.history.read',
      'catalog.read',
      'catalog.prices.read',
    ],
  });
  const cashierPassword = 'clave-cajero-9f-e';
  await call(driver, 'POST', 'users', {
    username: 'cajero9fe',
    display_name: 'Cajero 9F',
    role_id: role.id,
    active: true,
    password: cashierPassword,
  });

  // 2. La ruta antigua de anulación ya no existe: 410 y la cuenta sigue igual.
  const retired = await driver.request('POST', `restaurant/accounts/${account.id}/void`, {
    operation_id: randomUUID(),
    revision: account.revision,
    reason: 'Cliente se fue sin consumir',
  });
  expect(retired.status).toBe(410);
  expect(retired.body.error.code).toBe('ROUTE_RETIRED');
  const untouched = await call(driver, 'GET', `restaurant/accounts/${account.id}`);
  expect(untouched.account.state).toBe('open');
  expect(untouched.account.revision).toBe(account.revision);
  await driver.request('POST', 'auth/logout');

  // 3. El cajero abre la captura de la mesa y elige «Cancelar unidades» en el menú del producto.
  await page.setViewportSize({ width: 1280, height: 720 });
  await page.goto('/');
  await login(page, 'cajero9fe', cashierPassword);
  await openSection(page, 'Mesas');
  await page
    .getByRole('group', { name: 'Áreas del restaurante' })
    .getByRole('button', { name: areaName, exact: true })
    .click();
  await page
    .locator('.table-canvas[aria-label="Mapa de mesas"]')
    .getByRole('button', { name: /^Mesa F1,/ })
    .click();
  const tablePanel = page.getByRole('complementary', { name: 'Panel de Mesa F1' });
  await tablePanel
    .getByRole('button', { name: 'Capturar pedidos en esta mesa', exact: true })
    .click();
  const bill = page.getByRole('complementary', { name: 'Cuenta actual' });
  const line = bill.getByRole('article', { name: '3 × Tacos 9F-E' });
  await expect(line).toBeVisible();
  await line.getByRole('button', { name: 'Acciones de 3 × Tacos 9F-E', exact: true }).click();
  const menu = line.getByRole('group', { name: 'Acciones de 3 × Tacos 9F-E' });
  // Sin «Modificar» (no tiene ese permiso), pero sí «Cancelar unidades», en rojo.
  await expect(menu.getByRole('button', { name: 'Modificar', exact: true })).toHaveCount(0);
  const cancelUnits = menu.getByRole('button', { name: 'Cancelar unidades', exact: true });
  await expect(cancelUnits).toHaveClass(/danger-button/);
  await cancelUnits.click();
  await expect(
    page.getByRole('heading', { name: 'Cancelar unidades · Tacos 9F-E', exact: true }),
  ).toBeVisible();

  // 4. Go pidió supervisor desde el principio: el envío espera su usuario y contraseña.
  const fieldset = page.getByRole('group', { name: 'Autorización de supervisor' });
  await expect(fieldset).toBeVisible();
  await expect(fieldset).toContainText('«Cancelar partidas o cantidades confirmadas»');
  const submit = page.getByRole('button', {
    name: 'Confirmar cancelación de unidades',
    exact: true,
  });
  await expect(submit).toHaveClass(/danger-solid-button/);
  await page.getByRole('textbox', { name: 'Unidades por cancelar' }).fill('1');
  await page.getByLabel('Motivo de cancelación').fill('Se enfrió en la mesa 9F');
  await expect(submit).toBeDisabled();
  await fieldset.getByLabel('Usuario del supervisor', { exact: true }).fill('admin');
  const password = fieldset.getByLabel('Contraseña del supervisor', { exact: true });
  await password.fill('admin');
  await expect(submit).toBeEnabled();
  await views(page, testInfo, 'cancelar-unidades-con-supervisor', async () => {
    await expect(fieldset).toBeVisible();
  });
  await submit.click();
  const dialog = page.getByRole('alertdialog', { name: '¿Cancelar 1 unidad de Tacos 9F-E?' });
  await expect(dialog).toHaveClass(/dialog-panel-danger/);
  await expect(dialog).toContainText('Autoriza: admin');
  await dialog.getByRole('button', { name: 'Cancelar unidades', exact: true }).click();
  await expect(notice(page, 'Cancelación registrada: 1 unidad de Tacos 9F-E')).toBeVisible();
  await expect(bill.getByRole('article', { name: '2 × Tacos 9F-E' })).toBeVisible();
  await clearToasts(page);
  await logout(page);

  // 5. Go registró quién canceló y quién autorizó en el historial de la cuenta.
  await call(driver, 'POST', 'auth/login', { username: 'admin', password: 'admin' });
  const history = await call(driver, 'GET', `restaurant/accounts/${account.id}/history`);
  const cancellation = (
    history.items as {
      kind: string;
      author_name: string;
      payload: { after: Record<string, unknown> };
    }[]
  ).find((event) => event.kind === 'item.cancelled');
  expect(cancellation?.author_name).toBe('Cajero 9F');
  expect(cancellation?.payload.after).toEqual(
    expect.objectContaining({
      quantity: 1,
      reason: 'Se enfrió en la mesa 9F',
      authorized_by_username: 'admin',
      authorized_by_name: 'Administrador',
    }),
  );
  expect(JSON.stringify(history)).not.toContain('"password"');
  await driver.request('POST', 'auth/logout');

  // 6. Reportes: «Autorizó» en «Modificaciones de cantidades» y «Productos cancelados», en pantalla
  // y exportados a PDF y Excel (títulos, columnas y valores vienen de Go).
  await login(page, 'admin', 'admin');
  await openAdministration(page, 'Reportes');
  const changes = await openReport(page, 'Productos', 'Modificaciones de cantidades');
  await expect(changes.getByRole('columnheader', { name: /^Autorizó/ })).toBeVisible();
  const changeRow = changes.getByRole('row').filter({ hasText: 'Se enfrió en la mesa 9F' });
  await expect(changeRow).toContainText('Cajero 9F');
  await expect(changeRow).toContainText('Administrador');
  await views(page, testInfo, 'modificaciones-de-cantidades-autorizo', async () => {
    await expect(changeRow).toBeVisible();
  });
  const exportGroup = page.getByRole('group', { name: 'Exportar' });
  await exportGroup.getByRole('button', { name: 'Exportar PDF', exact: true }).click();
  await expect(notice(page, 'Modificaciones de cantidades: PDF guardado.')).toBeVisible({
    timeout: 30000,
  });
  await exportGroup.getByRole('button', { name: 'Exportar Excel', exact: true }).click();
  await expect(notice(page, 'Modificaciones de cantidades: Excel guardado.')).toBeVisible({
    timeout: 30000,
  });
  const cancelled = await openReport(page, 'Productos', 'Productos cancelados');
  await expect(cancelled.getByRole('columnheader', { name: /^Autorizó/ })).toBeVisible();
  const cancelledRow = cancelled.getByRole('row').filter({ hasText: 'Se enfrió en la mesa 9F' });
  await expect(cancelledRow).toContainText('Cajero 9F');
  await expect(cancelledRow).toContainText('Administrador');
  const exported = driver.exports.map((entry) => `${entry.report}:${entry.contentType}`);
  expect(exported).toEqual(
    expect.arrayContaining([
      'products.quantity_changes:application/pdf',
      'products.quantity_changes:application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
    ]),
  );
  await clearToasts(page);
  await logout(page);
  expect(errors).toEqual([]);
});
