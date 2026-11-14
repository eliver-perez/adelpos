import { randomUUID } from 'node:crypto';
import { copyFileSync, mkdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { expect, test } from '@playwright/test';
import type { Locator, Page, TestInfo } from '@playwright/test';
import { installDriver } from './native-driver';
import { openSection } from './navigation';

// Etapa 9G, fase C (§4.1–§4.3, decisiones D2 y D11):
//   1. «Buscar el servidor» lista la computadora principal que encontró la búsqueda (simulada en el puente con el
//      identificador real del servidor temporal); se elige y sigue el emparejamiento con código. Si la red no
//      permite buscar se explica y la dirección escrita a mano sigue disponible.
//   2. Dos terminales a la vez en dos contextos del navegador contra el mismo Go temporal (una con la terminal que
//      configuró la base y otra con su propia credencial): la mesa que ocupa una aparece ocupada en la otra con la
//      cuenta y sus productos; si las dos cambian la misma cuenta, ninguna sobrescribe a la otra en silencio (aviso
//      claro, vista actualizada y «Conservar mis productos nuevos»); sin conexión, la capa «Sin conexión con el
//      servidor» bloquea las operaciones sin perder lo capturado y, al volver, lo guarda y la otra terminal lo ve.
// Capturas en tema claro y oscuro (copia opcional en ADEL_SHOTS_DIR).

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

async function shots(page: Page, testInfo: TestInfo, name: string) {
  for (const theme of ['light', 'dark']) {
    await page.evaluate((value) => {
      document.documentElement.dataset.theme = value;
    }, theme);
    await page.waitForTimeout(200);
    const overflow = await page.evaluate(
      () => document.documentElement.scrollWidth - window.innerWidth,
    );
    expect(overflow, `${name} se desborda (${theme})`).toBe(0);
    const file = `${name}-${theme}.png`;
    await page.screenshot({ path: testInfo.outputPath(file) });
    keep(testInfo, file);
  }
  await page.evaluate(() => {
    document.documentElement.dataset.theme = 'light';
  });
}

async function login(page: Page, username: string, password: string) {
  await page.getByLabel('Usuario', { exact: true }).fill(username);
  await page.getByLabel('Contraseña', { exact: true }).fill(password);
  await page.getByRole('button', { name: 'Entrar', exact: true }).click();
}

// Puente de la prueba: guardados del borrador retenidos a pedido (para que otra terminal guarde antes) y la
// terminal sin conexión a pedido (el puente rechaza las peticiones), como en las especificaciones de la 9D.
async function controllableBridge(page: Page) {
  await page.addInitScript(() => {
    const bridge = window as unknown as {
      __TAURI_INTERNALS__: {
        invoke: (command: string, args?: Record<string, unknown>) => Promise<unknown>;
      };
      __adelOffline?: boolean;
      __adelHoldDraft?: boolean;
      __adelHeld?: (() => void)[];
    };
    bridge.__adelHeld = [];
    const original = bridge.__TAURI_INTERNALS__.invoke;
    bridge.__TAURI_INTERNALS__.invoke = async (command, args) => {
      if (command === 'api_request' && bridge.__adelOffline) {
        throw 'No se pudo conectar con el servidor. Comprueba su estado y la confianza TLS.';
      }
      if (
        command === 'api_request' &&
        args?.operation === 'restaurant_save_draft' &&
        bridge.__adelHoldDraft
      ) {
        await new Promise<void>((resolve) => bridge.__adelHeld!.push(resolve));
      }
      return original(command, args);
    };
  });
}

async function setOffline(page: Page, offline: boolean) {
  await page.evaluate((value) => {
    (window as unknown as { __adelOffline: boolean }).__adelOffline = value;
  }, offline);
}

async function holdDrafts(page: Page, hold: boolean) {
  await page.evaluate((value) => {
    const bridge = window as unknown as { __adelHoldDraft: boolean; __adelHeld: (() => void)[] };
    bridge.__adelHoldDraft = value;
    if (!value) {
      for (const release of bridge.__adelHeld.splice(0)) {
        release();
      }
    }
  }, hold);
}

async function openTableCapture(page: Page, area: string, table: string) {
  await openSection(page, 'Mesas');
  await page
    .getByRole('group', { name: 'Áreas del restaurante' })
    .getByRole('button', { name: area, exact: true })
    .click();
  await page
    .locator('.table-canvas[aria-label="Mapa de mesas"]')
    .getByRole('button', { name: new RegExp(`^${table},`) })
    .click();
  await page
    .getByRole('complementary', { name: `Panel de ${table}` })
    .getByRole('button', { name: 'Capturar pedidos en esta mesa', exact: true })
    .click();
  return page.getByRole('complementary', { name: 'Cuenta actual' });
}

function productButton(page: Page, name: string): Locator {
  return page.getByRole('button', { name: new RegExp(`^${name}`) });
}

async function chooseCategory(page: Page, category: string) {
  const button = page
    .getByRole('navigation', { name: 'Categorías' })
    .getByRole('button', { name: new RegExp(category) });
  await button.click();
  await expect(button).toHaveAttribute('aria-pressed', 'true');
}

test('Buscar el servidor: la computadora principal encontrada en la red se elige de la lista y sigue el código (etapa 9G, fase C)', async ({
  page,
}, testInfo) => {
  const driver = await installDriver(page);
  driver.setInstallation({ chosen: true, mode: 'terminal-only' });
  driver.setDiscovery('blocked');
  await page.setViewportSize({ width: 1280, height: 720 });
  await page.goto('/');
  await expect(page.getByRole('heading', { name: 'Buscar el servidor' })).toBeVisible();
  // 1. La red no permite buscar: se explica y la dirección a mano sigue disponible.
  await expect(page.getByText(/permiso de usar la red local/)).toBeVisible();
  await expect(
    page.getByRole('textbox', { name: 'Dirección de la computadora principal' }),
  ).toBeVisible();
  // 2. «Buscar otra vez» encuentra la computadora principal (nombre del establecimiento y de la computadora).
  driver.setDiscovery('server');
  await page.getByRole('button', { name: 'Buscar otra vez', exact: true }).click();
  const list = page.getByRole('list', { name: 'Computadoras encontradas' });
  const found = list.getByRole('button', {
    name: 'Pedir código a Aceptación temporal — Computadora de pruebas',
    exact: true,
  });
  await expect(found).toBeVisible();
  // Sin direcciones ni identificadores en la pantalla.
  await expect(page.locator('.find-server')).not.toContainText(/\d+\.\d+\.\d+\.\d+|8443/);
  await page.getByRole('textbox', { name: 'Nombre de esta terminal' }).fill('Barra norte 9G-C');
  await shots(page, testInfo, 'buscar-servidor-encontrado');
  await found.click();
  const code = page.locator('.pairing-code');
  await expect(code).toHaveText(/^\d{3} \d{3}$/);
  // 3. Se aprueba en la computadora principal con el código y la terminal queda emparejada.
  const digits = (await code.textContent())!.replace(/\D/g, '');
  await call(driver, 'POST', 'auth/login', { username: 'admin', password: 'admin' });
  const pending = await call(driver, 'GET', 'devices/pairing-requests');
  const entry = (pending.requests as { id: string; name: string }[]).find(
    (request) => request.name === 'Barra norte 9G-C',
  );
  expect(entry).toBeTruthy();
  await call(driver, 'POST', `devices/pairing-requests/${entry!.id}/approve`, { code: digits });
  await expect(page.locator('.find-server-status')).toContainText('Terminal emparejada', {
    timeout: 10000,
  });
  await expect(page.getByRole('heading', { name: 'Inicia sesión' })).toBeVisible({
    timeout: 15000,
  });
});

test('Dos terminales a la vez: mesa y cuenta compartidas, conflicto sin sobrescribir y «Sin conexión» sin perder lo capturado (etapa 9G, fase C)', async ({
  browser,
}, testInfo) => {
  test.setTimeout(240000);
  const contextA = await browser.newContext({ viewport: { width: 1280, height: 720 } });
  const pageA = await contextA.newPage();
  const driverA = await installDriver(pageA);
  await controllableBridge(pageA);

  // 1. Datos por la API: catálogo, una mesa y una mesera con su propia terminal (enrolada con una invitación).
  await call(driverA, 'POST', 'auth/login', { username: 'admin', password: 'admin' });
  await call(driverA, 'PUT', 'terminal/preferences', { keyboard_mode: 'disabled' });
  const category = await call(driverA, 'POST', 'catalog/categories', {
    name: 'Sopas 9G-C',
    active: true,
    color: '#d64545',
  });
  for (const [name, code, price] of [
    ['Caldo 9G-C', 'CALDO-9GC', '80'],
    ['Agua 9G-C', 'AGUA-9GC', '20'],
  ]) {
    await call(driverA, 'POST', 'catalog/products', {
      name,
      active: true,
      category_id: category.id,
      code,
      kind: 'product',
      base_price: price,
    });
  }
  const area = 'Terraza 9G-C';
  const created = (await call(driverA, 'POST', 'restaurant/areas', {
    operation_id: randomUUID(),
    name: area,
    active: true,
  })) as { id: string };
  await call(driverA, 'POST', 'restaurant/tables', {
    operation_id: randomUUID(),
    area_id: created.id,
    name: 'T9GC-1',
    shape: 'rectangle',
    x: 40,
    y: 60,
    width: 120,
    height: 100,
    seats: 4,
    active: true,
  });
  const role = await call(driverA, 'POST', 'roles', {
    name: 'Mesera 9G-C',
    permissions: [
      'accounts.read',
      'accounts.open',
      'restaurant.tables.read',
      'restaurant.areas.read',
      'orders.capture',
      'orders.confirm',
      'catalog.read',
      'catalog.prices.read',
      'events.read',
    ],
  });
  const waiterPassword = 'clave-mesera-9g-c';
  await call(driverA, 'POST', 'users', {
    username: 'mesera9gc',
    display_name: 'Mesera 9G-C',
    role_id: role.id,
    active: true,
    password: waiterPassword,
  });
  const invitation = await call(driverA, 'POST', 'devices/invitations');
  const enrolled = await fetch(`${serverURL}/api/v1/devices/enroll`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'X-Client-Version': clientVersion,
      'X-Client-Kind': 'desktop',
    },
    body: JSON.stringify({ code: invitation.code, name: 'Barra sur 9G-C' }),
  });
  expect(enrolled.status).toBe(200);
  const credentials = (await enrolled.json()) as { device_id: string; device_token: string };
  await call(driverA, 'PATCH', `devices/${credentials.device_id}`, {
    name: 'Barra sur 9G-C',
    register_id: null,
    keyboard_mode: 'disabled',
  });
  await call(driverA, 'POST', 'auth/logout');
  const contextB = await browser.newContext({ viewport: { width: 1280, height: 720 } });
  const pageB = await contextB.newPage();
  await installDriver(pageB, {
    terminal: { deviceID: credentials.device_id, deviceToken: credentials.device_token },
  });
  await controllableBridge(pageB);

  // 2. Cada terminal inicia sesión con su usuario (una sesión por usuario).
  await pageA.goto('/');
  await login(pageA, 'admin', 'admin');
  await pageB.goto('/');
  await login(pageB, 'mesera9gc', waiterPassword);

  // 3. La terminal A ocupa la mesa con un producto; la B la ve ocupada y con ese producto en su cuenta.
  const billA = await openTableCapture(pageA, area, 'T9GC-1');
  await chooseCategory(pageA, 'Sopas 9G-C');
  await productButton(pageA, 'Caldo 9G-C').click();
  await expect(billA.getByRole('article', { name: '1 × Caldo 9G-C' })).toBeVisible();
  await expect(pageA.getByRole('heading', { name: /^T9GC-1 · Cuenta #\d+$/ })).toBeVisible({
    timeout: 15000,
  });
  await openSection(pageB, 'Mesas');
  await pageB
    .getByRole('group', { name: 'Áreas del restaurante' })
    .getByRole('button', { name: area, exact: true })
    .click();
  await expect(
    pageB
      .locator('.table-canvas[aria-label="Mapa de mesas"]')
      .getByRole('button', { name: /^T9GC-1,/ }),
  ).toHaveAccessibleName(/Ocupada/, { timeout: 15000 });
  const billB = await openTableCapture(pageB, area, 'T9GC-1');
  await expect(billB.getByRole('article', { name: '1 × Caldo 9G-C' })).toBeVisible({
    timeout: 15000,
  });

  // 4. Las dos cambian la misma cuenta: el guardado de A queda retenido mientras B guarda primero. A recibe el
  //    aviso claro de que otra terminal cambió la cuenta, conserva sus productos y no sobrescribe nada.
  async function revision() {
    const accounts = await driverA.request('GET', 'restaurant/accounts?page_size=50');
    const items = (accounts.body.items ?? []) as { table_name: string | null; revision: number }[];
    return items.find((item) => item.table_name === 'T9GC-1')?.revision ?? 0;
  }
  const before = await revision();
  expect(before).toBeGreaterThan(0);
  await holdDrafts(pageA, true);
  await chooseCategory(pageA, 'Sopas 9G-C');
  await productButton(pageA, 'Agua 9G-C').click();
  await expect(billA.getByRole('article', { name: '1 × Agua 9G-C' })).toBeVisible();
  await chooseCategory(pageB, 'Sopas 9G-C');
  await productButton(pageB, 'Caldo 9G-C').click();
  await expect(billB.getByRole('article', { name: '2 × Caldo 9G-C' })).toBeVisible();
  // B ya guardó (la revisión de la cuenta subió) antes de soltar el guardado de A.
  await expect.poll(revision, { timeout: 15000 }).toBeGreaterThan(before);
  await holdDrafts(pageA, false);
  const conflict = billA.getByText(/Otra terminal cambió esta cuenta mientras agregabas productos/);
  await expect(conflict).toBeVisible({ timeout: 15000 });
  await shots(pageA, testInfo, 'dos-terminales-conflicto');
  await billA.getByRole('button', { name: 'Conservar mis productos nuevos', exact: true }).click();
  await expect(conflict).toHaveCount(0, { timeout: 15000 });
  // La vista de A es la actualizada (con el producto de B) más el suyo; B también lo ve.
  await expect(billA.getByRole('article', { name: '2 × Caldo 9G-C' })).toBeVisible({
    timeout: 15000,
  });
  await expect(billA.getByRole('article', { name: '1 × Agua 9G-C' })).toBeVisible();
  await expect(billB.getByRole('article', { name: '1 × Agua 9G-C' })).toBeVisible({
    timeout: 15000,
  });

  // 5. Sin conexión en B: el producto que se toca antes de que se note la pérdida se conserva; la capa bloquea las
  //    operaciones (el contenido queda inerte) y, al volver, se guarda y A lo ve.
  await setOffline(pageB, true);
  await productButton(pageB, 'Agua 9G-C').click();
  await expect(billB.getByRole('article', { name: '2 × Agua 9G-C' })).toBeVisible();
  const layer = pageB.getByRole('alertdialog', { name: 'Sin conexión con el servidor' });
  await expect(layer).toBeVisible({ timeout: 15000 });
  await expect(layer).toContainText('se conserva en esta pantalla');
  await expect(layer).toContainText(/Se vuelve a intentar sola en \d+ s\.|Intentando conectar/);
  await expect(pageB.locator('main.admin-main')).toHaveAttribute('inert', '');
  await expect(billB.getByRole('article', { name: '2 × Agua 9G-C' })).toBeVisible();
  // Un toque sobre la pantalla de abajo no llega: la capa lo cubre.
  const covered = await productButton(pageB, 'Caldo 9G-C').evaluate((element) => {
    const box = element.getBoundingClientRect();
    const hit = document.elementFromPoint(box.left + box.width / 2, box.top + box.height / 2);
    return !!hit?.closest('.connection-lost-layer');
  });
  expect(covered).toBe(true);
  await shots(pageB, testInfo, 'sin-conexion-capa');
  await setOffline(pageB, false);
  await layer.getByRole('button', { name: 'Reintentar', exact: true }).click();
  await expect(layer).toHaveCount(0, { timeout: 15000 });
  await expect(pageB.locator('main.admin-main')).not.toHaveAttribute('inert', '');
  await expect(billB.getByRole('article', { name: '2 × Agua 9G-C' })).toBeVisible();
  await expect(billA.getByRole('article', { name: '2 × Agua 9G-C' })).toBeVisible({
    timeout: 20000,
  });
  await contextB.close();
  await contextA.close();
});
