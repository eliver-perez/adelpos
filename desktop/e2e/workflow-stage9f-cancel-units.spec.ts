import { randomUUID } from 'node:crypto';
import { copyFileSync, mkdirSync } from 'node:fs';
import { join } from 'node:path';
import { expect, test } from '@playwright/test';
import type { Page, TestInfo } from '@playwright/test';
import { installDriver } from './native-driver';
import { openSection } from './navigation';

// Etapa 9F, fase E (§7.1): la ruta antigua de anulación responde 410 sin tocar la cuenta. Hasta la 9H
// esta especificación también hacía que un cajero sin «Cancelar partidas o cantidades confirmadas»
// cancelara una unidad con la autorización del administrador; la etapa 9H retiró esa autorización
// (decisión del propietario): ahora comprueba que, sin el permiso propio, «Cancelar unidades» no se
// ofrece, «Modificar» no deja bajar la cantidad enviada y Go rechaza la cancelación sin camino de
// supervisor. La cancelación con el permiso propio y los reportes con «Autorizó» están en
// workflow-stage9h-adjustments.spec.ts. Mismo transporte que las demás especificaciones (Go temporal y
// base adel_pos_test_*). Capturas en tema claro y oscuro a 1280 × 720 y 1920 × 1080 (copia opcional en
// ADEL_SHOTS_DIR).

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

test('la ruta antigua de anulación responde 410 y sin «Cancelar partidas» no se ofrece «Cancelar unidades» (etapa 9F, fase E; ajustada en la 9H)', async ({
  page,
}, testInfo) => {
  test.setTimeout(300000);
  const errors: string[] = [];
  page.on('pageerror', (error) => errors.push(error.message));
  const driver = await installDriver(page);
  await call(driver, 'POST', 'auth/login', { username: 'admin', password: 'admin' });
  await call(driver, 'PUT', 'terminal/preferences', { keyboard_mode: 'disabled' });

  // 1. Datos por la API: una mesa con tres productos enviados a cocina y un cajero sin «Cancelar
  // partidas o cantidades confirmadas» (sí abre cuentas, captura y modifica).
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
      'orders.edit',
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

  // 3. El cajero abre la captura de la mesa: el menú del producto ofrece «Modificar», pero no
  // «Cancelar unidades».
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
  const modify = menu.getByRole('button', { name: 'Modificar', exact: true });
  await expect(modify).toBeVisible();
  await expect(menu.getByRole('button', { name: 'Cancelar unidades', exact: true })).toHaveCount(0);
  await views(page, testInfo, 'sin-cancelar-unidades', async () => {
    await expect(modify).toBeVisible();
  });

  // 4. «Modificar» no deja bajar la cantidad enviada (sin formulario de supervisor) y sí subirla.
  await modify.click();
  const panel = page.getByRole('region', { name: 'Personalizar Tacos 9F-E' });
  await expect(panel).toBeVisible();
  const remove = panel.getByRole('button', {
    name: 'Quitar una unidad de Tacos 9F-E',
    exact: true,
  });
  await expect(remove).toBeDisabled();
  await expect(page.getByRole('group', { name: 'Autorización de supervisor' })).toHaveCount(0);
  await panel
    .getByRole('button', { name: 'Agregar una unidad de Tacos 9F-E', exact: true })
    .click();
  await expect(remove).toBeEnabled();
  await panel.getByRole('button', { name: 'Volver', exact: true }).click();
  const discard = page.getByRole('alertdialog', { name: 'Descartar la selección' });
  await discard.getByRole('button', { name: 'Descartar selección', exact: true }).click();
  await expect(panel).toHaveCount(0);

  // 5. Go también lo rechaza sin el permiso propio, aunque se envíe la autorización de un supervisor
  // (ya no existe ese camino), y la cuenta no cambia.
  const current = await call(driver, 'GET', `restaurant/accounts/${account.id}`);
  const refused = await driver.request('POST', `restaurant/accounts/${account.id}/cancel`, {
    operation_id: randomUUID(),
    revision: current.account.revision,
    version_id: (current.lines as { version_id: string }[])[0]!.version_id,
    quantity: 1,
    reason: 'Se enfrió en la mesa 9F',
    authorization: { username: 'admin', password: 'admin' },
  });
  expect(refused.status).toBe(403);
  expect(refused.body.error.code).toBe('PERMISSION_DENIED');
  const after = await call(driver, 'GET', `restaurant/accounts/${account.id}`);
  expect(after.account.revision).toBe(current.account.revision);
  expect(after.account.confirmed_quantity).toBe(3);
  expect(JSON.stringify(after)).not.toContain('cancel_units_requires_authorization');
  await clearToasts(page);
  await logout(page);
  expect(errors).toEqual([]);
});
