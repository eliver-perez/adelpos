import { randomUUID } from 'node:crypto';
import { copyFileSync, mkdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { expect, test } from '@playwright/test';
import type { Page, TestInfo } from '@playwright/test';
import { installDriver } from './native-driver';
import { openSection } from './navigation';
import { notice } from './notices';

// Etapa 9G, fase D (§5.4, decisión D8): un cajero con «Marcar productos como agotados» marca desde la captura un
// producto como agotado con un toque; el producto queda marcado y no se puede agregar, Go rechaza capturarlo
// (PRODUCT_SOLD_OUT) y el cambio llega en tiempo real a otra terminal y desde otra terminal (evento del catálogo),
// antes que el sondeo de 10 s del listado. «Disponible otra vez» lo devuelve con otro toque, y el Registro de
// movimientos guarda quién lo hizo. Mismo transporte que las demás especificaciones (Go temporal y base
// adel_pos_test_*); la «otra terminal» es una segunda terminal enrolada que habla con Go desde Node. Capturas en tema
// claro y oscuro a 1280 × 720 y 1920 × 1080 (copia opcional en ADEL_SHOTS_DIR).

type Driver = Awaited<ReturnType<typeof installDriver>>;

const serverURL = process.env.ADEL_BROWSER_SERVER_URL ?? 'http://127.0.0.1:8183';
const clientVersion = (
  JSON.parse(readFileSync(new URL('../src/generated/product.json', import.meta.url), 'utf8')) as {
    version: string;
  }
).version;

async function call(driver: Driver, method: string, route: string, body?: unknown) {
  const response = await driver.request(method, route, body);
  expect(response.status, `${method} ${route}: ${JSON.stringify(response.body)}`).toBe(200);
  return response.body;
}

// Segunda terminal: se enrola con una invitación y habla con Go directamente, como lo haría otra caja.
class OtherTerminal {
  private deviceID = '';
  private deviceToken = '';
  private sessionToken = '';

  async enroll(code: string) {
    const enrolled = await this.send('POST', 'devices/enroll', {
      code,
      name: 'Barra de agotados 9G',
    });
    expect(enrolled.status).toBe(200);
    this.deviceID = enrolled.body.device_id;
    this.deviceToken = enrolled.body.device_token;
  }

  async login(username: string, password: string) {
    const login = await this.send('POST', 'auth/login', { username, password });
    expect(login.status).toBe(200);
    this.sessionToken = login.body.session_token;
  }

  async send(method: string, route: string, body?: unknown) {
    const response = await fetch(`${serverURL}/api/v1/${route}`, {
      method,
      headers: {
        'Content-Type': 'application/json',
        'X-Client-Version': clientVersion,
        'X-Client-Kind': 'desktop',
        'X-Device-ID': this.deviceID,
        'X-Device-Token': this.deviceToken,
        Authorization: `Bearer ${this.sessionToken}`,
      },
      body: body === undefined ? undefined : JSON.stringify(body),
    });
    return { status: response.status, body: await response.json() };
  }
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

// Recorre los dos tamaños y los dos temas: sin desbordamiento horizontal, la comprobación de la vista y una
// captura de la página.
async function views(page: Page, testInfo: TestInfo, name: string, check?: () => Promise<void>) {
  await clearToasts(page);
  for (const size of sizes) {
    await page.setViewportSize(size);
    for (const theme of themes) {
      const file = `${name}-${theme}-${size.width}x${size.height}.png`;
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

test('«Agotado» desde la captura con un toque, en tiempo real entre terminales (etapa 9G, fase D)', async ({
  page,
}, testInfo) => {
  test.setTimeout(240000);
  const errors: string[] = [];
  page.on('pageerror', (error) => errors.push(error.message));
  const driver = await installDriver(page);
  await call(driver, 'POST', 'auth/login', { username: 'admin', password: 'admin' });
  await call(driver, 'PUT', 'terminal/preferences', { keyboard_mode: 'disabled' });

  // 1. Datos por la API: dos bebidas, una mesa, un cajero con el permiso de «Agotado» y una segunda terminal.
  const category = await call(driver, 'POST', 'catalog/categories', {
    name: 'Bebidas 9G',
    active: true,
    color: '#d64545',
  });
  const shake = await call(driver, 'POST', 'catalog/products', {
    name: 'Malteada 9G',
    active: true,
    category_id: category.id,
    code: 'MALTEADA-9G',
    kind: 'product',
    base_price: '65',
  });
  const lemonade = await call(driver, 'POST', 'catalog/products', {
    name: 'Limonada 9G',
    active: true,
    category_id: category.id,
    code: 'LIMONADA-9G',
    kind: 'product',
    base_price: '35',
  });
  const areaName = 'Barra 9G';
  const area = (await call(driver, 'POST', 'restaurant/areas', {
    operation_id: randomUUID(),
    name: areaName,
    active: true,
  })) as { id: string };
  await call(driver, 'POST', 'restaurant/tables', {
    operation_id: randomUUID(),
    area_id: area.id,
    name: 'Mesa G1',
    shape: 'rectangle',
    x: 40,
    y: 60,
    width: 120,
    height: 100,
    seats: 4,
    active: true,
  });
  const role = await call(driver, 'POST', 'roles', {
    name: 'Cajero con agotados 9G',
    permissions: [
      'accounts.read',
      'accounts.open',
      'restaurant.tables.read',
      'restaurant.areas.read',
      'orders.capture',
      'orders.confirm',
      'catalog.read',
      'catalog.prices.read',
      'catalog.sold_out.manage',
      'events.read',
    ],
  });
  const cashierPassword = 'clave-cajero-9g';
  await call(driver, 'POST', 'users', {
    username: 'cajero9g',
    display_name: 'Cajero 9G',
    role_id: role.id,
    active: true,
    password: cashierPassword,
  });
  const invitation = await call(driver, 'POST', 'devices/invitations');
  await driver.request('POST', 'auth/logout');
  const other = new OtherTerminal();
  await other.enroll(invitation.code as string);
  await other.login('admin', 'admin');

  // 2. El cajero abre la captura de la mesa en la categoría de bebidas.
  await page.setViewportSize({ width: 1280, height: 720 });
  await page.goto('/');
  await login(page, 'cajero9g', cashierPassword);
  await openSection(page, 'Mesas');
  await page
    .getByRole('group', { name: 'Áreas del restaurante' })
    .getByRole('button', { name: areaName, exact: true })
    .click();
  await page
    .locator('.table-canvas[aria-label="Mapa de mesas"]')
    .getByRole('button', { name: /^Mesa G1,/ })
    .click();
  await page
    .getByRole('complementary', { name: 'Panel de Mesa G1' })
    .getByRole('button', { name: 'Capturar pedidos en esta mesa', exact: true })
    .click();
  const drinks = page
    .getByRole('navigation', { name: 'Categorías' })
    .getByRole('button', { name: /Bebidas 9G/ });
  await drinks.click();
  await expect(drinks).toHaveAttribute('aria-pressed', 'true');
  const shakeTile = page.getByRole('button', { name: /^Malteada 9G/ });
  await expect(shakeTile).toBeEnabled();

  // 3. «Marcar agotados» y un toque: la malteada queda agotada hasta el inicio del día.
  const mode = page.getByRole('button', { name: 'Marcar agotados', exact: true });
  await mode.click();
  await expect(mode).toHaveAttribute('aria-pressed', 'true');
  await expect(page.getByText('Toca un producto para marcarlo como agotado')).toBeVisible();
  await expect(page.getByRole('button', { name: 'Solo por hoy', exact: true })).toHaveAttribute(
    'aria-pressed',
    'true',
  );
  await page.getByRole('button', { name: 'Marcar como agotado: Malteada 9G', exact: true }).click();
  await expect(notice(page, '«Malteada 9G» quedó agotado por hoy.')).toBeVisible();
  const soldOutShake = page.getByRole('button', {
    name: 'Disponible otra vez: Malteada 9G',
    exact: true,
  });
  await expect(soldOutShake).toContainText('Agotado');
  await views(page, testInfo, 'captura-marcar-agotados', async () => {
    await expect(soldOutShake).toBeVisible();
  });

  // 4. Fuera del modo, la malteada se ve agotada y no se puede tocar; la limonada sí.
  await mode.click();
  await expect(page.getByText('Toca un producto para marcarlo como agotado')).toHaveCount(0);
  await expect(shakeTile).toBeDisabled();
  await expect(shakeTile).toContainText('Agotado');
  await expect(page.getByRole('button', { name: /^Limonada 9G/ })).toBeEnabled();
  await views(page, testInfo, 'captura-producto-agotado', async () => {
    await expect(shakeTile).toBeDisabled();
  });

  // 5. Go rechaza capturar el producto agotado en cualquier terminal.
  const rejected = await other.send('POST', 'catalog/preview', {
    lines: [{ product_id: shake.id, variant_id: null, quantity: 1, customization: {} }],
  });
  expect(rejected.status).toBe(409);
  expect(rejected.body.error.code).toBe('PRODUCT_SOLD_OUT');
  expect(rejected.body.error.message).toContain('Malteada 9G está agotado');

  // 5 bis. La limonada se agrega a la cuenta (su primer producto la abre).
  await page.getByRole('button', { name: /^Limonada 9G/ }).click();
  const bill = page.getByRole('complementary', { name: 'Cuenta actual' });
  await expect(bill.getByRole('article', { name: '1 × Limonada 9G' })).toBeVisible();
  await expect(page.getByRole('heading', { name: /^Mesa G1 · Cuenta #/ })).toBeVisible();

  // 6. Otra terminal marca la limonada agotada hasta nuevo aviso: la captura lo muestra por el evento del catálogo,
  // antes del sondeo de 10 s del listado.
  const marked = await other.send('POST', `catalog/products/${lemonade.id}/sold-out`, {
    operation_id: randomUUID(),
    sold_out: true,
    indefinite: true,
  });
  expect(marked.status).toBe(200);
  expect(marked.body).toMatchObject({ sold_out: true, indefinite: true, sold_out_until: null });
  const lemonadeTile = page.getByRole('button', { name: /^Limonada 9G/ });
  await expect(lemonadeTile).toContainText('Agotado hasta nuevo aviso', { timeout: 6000 });
  await expect(lemonadeTile).toBeDisabled();

  // 6 bis (revisión de la fase D, hallazgo 3): la limonada que ya estaba en el borrador se conserva, pero agregarle
  // otra unidad ya no se guarda: Go la rechaza y la cuenta lo dice. Al quitar la unidad de más, el borrador se guarda.
  await bill
    .getByRole('button', { name: 'Agregar una unidad de Limonada 9G', exact: true })
    .click();
  await expect(bill.getByText('Limonada 9G está agotado')).toBeVisible();
  await bill.getByRole('button', { name: 'Quitar una unidad de Limonada 9G', exact: true }).click();
  await expect(bill.getByText('Limonada 9G está agotado')).toHaveCount(0);
  await expect(bill.getByRole('article', { name: '1 × Limonada 9G' })).toBeVisible();

  // 7. «Disponible otra vez» con un toque: también llega a la otra terminal.
  await mode.click();
  await page.getByRole('button', { name: 'Disponible otra vez: Limonada 9G', exact: true }).click();
  await expect(notice(page, '«Limonada 9G» está disponible otra vez.')).toBeVisible();
  await page.getByRole('button', { name: 'Disponible otra vez: Malteada 9G', exact: true }).click();
  await expect(notice(page, '«Malteada 9G» está disponible otra vez.')).toBeVisible();
  await mode.click();
  await expect(shakeTile).toBeEnabled();
  await expect(lemonadeTile).toBeEnabled();
  const products = await other.send('GET', `catalog/products?category_id=${category.id}`);
  expect(products.status).toBe(200);
  for (const item of products.body.items as { sold_out: boolean }[]) {
    expect(item.sold_out).toBe(false);
  }

  // 8. El Registro de movimientos guarda quién marcó y quién devolvió cada producto.
  const audit = await other.send('GET', 'audit?action=catalog.product.sold_out&page_size=10');
  expect(audit.status).toBe(200);
  const actors = (audit.body.items as { actor_username: string; action: string }[]).map(
    (entry) => entry.actor_username,
  );
  expect(actors).toEqual(expect.arrayContaining(['cajero9g', 'admin']));
  const again = await other.send(
    'GET',
    'audit?action=catalog.product.available_again&page_size=10',
  );
  expect(
    (again.body.items as { actor_username: string }[]).filter(
      (entry) => entry.actor_username === 'cajero9g',
    ),
  ).toHaveLength(2);
  await other.send('POST', 'auth/logout');
  expect(errors).toEqual([]);
});
