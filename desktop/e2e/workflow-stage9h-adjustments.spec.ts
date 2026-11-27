import { randomUUID } from 'node:crypto';
import { copyFileSync, mkdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { expect, test } from '@playwright/test';
import type { Page, TestInfo } from '@playwright/test';
import { installDriver } from './native-driver';
import { openAdministration, openSection } from './navigation';
import { notice } from './notices';
import { chooseOption } from './select';

// Etapa 9H, fase A (§2.2, §2.3 y §2.4), contra el Go temporal y una base adel_pos_test_*:
//   1. «Cancelar unidades» sin autorización de supervisor: una cajera con el permiso propio cancela una
//      unidad enviada sin que se pida a nadie más; el historial y el Registro de movimientos dicen quién
//      canceló y «Modificaciones de cantidades» y «Productos cancelados» conservan «Autorizó», vacía.
//   2. Diseñador de documentos: la opción «Mostrar precio unitario» de la lista de productos está
//      desactivada por omisión y, activada, la vista previa de Go muestra «2 × $250.00» en el mismo
//      renglón a 80 mm y en un segundo renglón debajo del nombre a 58 mm.
//   3. El mismo usuario con sesión en dos terminales a la vez: iniciar sesión en la segunda no cierra la
//      primera, iniciar de nuevo en la misma terminal sí cierra la anterior de esa terminal, y
//      «Sesiones activas por tipo de terminal» cuenta sesiones, no usuarios.
// Capturas en tema claro y oscuro a 1280 × 720 y 1920 × 1080 (copia opcional en ADEL_SHOTS_DIR).

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

// Recorre los dos tamaños y los dos temas: sin desbordamiento horizontal, la comprobación de la vista
// y una captura de la página.
async function views(page: Page, testInfo: TestInfo, name: string, check?: () => Promise<void>) {
  await clearToasts(page);
  for (const size of sizes) {
    await page.setViewportSize(size);
    for (const theme of themes) {
      const file = `${name}-${theme}-${size.width}x${size.height}.png`;
      // La aplicación vuelve a aplicar el tema del usuario cuando se actualiza su sesión: la captura
      // se repite si el tema cambió mientras se tomaba.
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

// Abre un reporte de un grupo: por debajo de 1440 px con el selector «Reporte» y en pantallas anchas
// con la lista lateral.
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
  return id;
}

test('«Cancelar unidades» con el permiso propio, sin autorización de supervisor, y «Autorizó» vacía en los reportes (etapa 9H, fase A)', async ({
  page,
}, testInfo) => {
  test.setTimeout(300000);
  const errors: string[] = [];
  page.on('pageerror', (error) => errors.push(error.message));
  const driver = await installDriver(page);
  await call(driver, 'POST', 'auth/login', { username: 'admin', password: 'admin' });
  await call(driver, 'PUT', 'terminal/preferences', { keyboard_mode: 'disabled' });

  // 1. Datos por la API: una mesa con tres productos enviados a cocina y una cajera con «Cancelar
  // partidas o cantidades confirmadas».
  const category = await call(driver, 'POST', 'catalog/categories', {
    name: 'Antojitos 9H-A',
    active: true,
    color: '#d64545',
  });
  const product = await call(driver, 'POST', 'catalog/products', {
    name: 'Sopes 9H-A',
    active: true,
    category_id: category.id,
    code: 'SOPES-9HA',
    kind: 'product',
    base_price: '150',
  });
  const areaName = 'Patio 9H-A';
  const area = (await call(driver, 'POST', 'restaurant/areas', {
    operation_id: randomUUID(),
    name: areaName,
    active: true,
  })) as { id: string };
  const table = (await call(driver, 'POST', 'restaurant/tables', {
    operation_id: randomUUID(),
    area_id: area.id,
    name: 'Mesa H1',
    shape: 'rectangle',
    x: 40,
    y: 60,
    width: 120,
    height: 100,
    seats: 4,
    active: true,
  })) as { id: string };
  const accountID = await sentAccount(driver, table.id, product.id as string, 3);
  const role = await call(driver, 'POST', 'roles', {
    name: 'Cajera con cancelar unidades 9H-A',
    permissions: [
      'accounts.read',
      'accounts.open',
      'restaurant.tables.read',
      'restaurant.areas.read',
      'orders.capture',
      'orders.confirm',
      'orders.edit',
      'orders.cancel',
      'orders.history.read',
      'catalog.read',
      'catalog.prices.read',
    ],
  });
  const cashierPassword = 'clave-cajera-9h-a';
  await call(driver, 'POST', 'users', {
    username: 'cajera9ha',
    display_name: 'Cajera 9H-A',
    role_id: role.id,
    active: true,
    password: cashierPassword,
  });
  await driver.request('POST', 'auth/logout');

  // 2. La cajera elige «Cancelar unidades» en el menú del producto: el panel no pide supervisor.
  await page.setViewportSize({ width: 1280, height: 720 });
  await page.goto('/');
  await login(page, 'cajera9ha', cashierPassword);
  await openSection(page, 'Mesas');
  await page
    .getByRole('group', { name: 'Áreas del restaurante' })
    .getByRole('button', { name: areaName, exact: true })
    .click();
  await page
    .locator('.table-canvas[aria-label="Mapa de mesas"]')
    .getByRole('button', { name: /^Mesa H1,/ })
    .click();
  await page
    .getByRole('complementary', { name: 'Panel de Mesa H1' })
    .getByRole('button', { name: 'Capturar pedidos en esta mesa', exact: true })
    .click();
  const bill = page.getByRole('complementary', { name: 'Cuenta actual' });
  const line = bill.getByRole('article', { name: '3 × Sopes 9H-A' });
  await expect(line).toBeVisible();
  await line.getByRole('button', { name: 'Acciones de 3 × Sopes 9H-A', exact: true }).click();
  const cancelUnits = line
    .getByRole('group', { name: 'Acciones de 3 × Sopes 9H-A' })
    .getByRole('button', { name: 'Cancelar unidades', exact: true });
  await expect(cancelUnits).toHaveClass(/danger-button/);
  await cancelUnits.click();
  await expect(
    page.getByRole('heading', { name: 'Cancelar unidades · Sopes 9H-A', exact: true }),
  ).toBeVisible();
  await expect(page.getByRole('group', { name: 'Autorización de supervisor' })).toHaveCount(0);
  await expect(page.getByLabel('Contraseña del supervisor')).toHaveCount(0);
  await page.getByRole('textbox', { name: 'Unidades por cancelar' }).fill('1');
  await page.getByLabel('Motivo de cancelación').fill('Se cayó al servir 9H');
  const submit = page.getByRole('button', {
    name: 'Confirmar cancelación de unidades',
    exact: true,
  });
  await expect(submit).toBeEnabled();
  await views(page, testInfo, '9h-cancelar-unidades-sin-supervisor', async () => {
    await expect(submit).toBeVisible();
  });
  await submit.click();
  const dialog = page.getByRole('alertdialog', { name: '¿Cancelar 1 unidad de Sopes 9H-A?' });
  await expect(dialog).toHaveClass(/dialog-panel-danger/);
  await expect(dialog).not.toContainText('Autoriza');
  await dialog.getByRole('button', { name: 'Cancelar unidades', exact: true }).click();
  await expect(notice(page, 'Cancelación registrada: 1 unidad de Sopes 9H-A')).toBeVisible();
  await expect(bill.getByRole('article', { name: '2 × Sopes 9H-A' })).toBeVisible();
  await clearToasts(page);
  await logout(page);

  // 3. Go registró quién canceló, sin «Autorizó», en el historial y en el Registro de movimientos.
  await call(driver, 'POST', 'auth/login', { username: 'admin', password: 'admin' });
  const history = await call(driver, 'GET', `restaurant/accounts/${accountID}/history`);
  const cancellation = (
    history.items as {
      kind: string;
      author_name: string;
      payload: { after: Record<string, unknown> };
    }[]
  ).find((event) => event.kind === 'item.cancelled');
  expect(cancellation?.author_name).toBe('Cajera 9H-A');
  expect(cancellation?.payload.after).toEqual(
    expect.objectContaining({ quantity: 1, reason: 'Se cayó al servir 9H' }),
  );
  expect(cancellation?.payload.after).not.toHaveProperty('authorized_by');
  const audit = await call(
    driver,
    'GET',
    'audit?action=restaurant.item.cancelled&actor=cajera9ha&page_size=10',
  );
  expect(
    (audit.items as { entity_id: string }[]).some((item) => item.entity_id === accountID),
  ).toBe(true);
  await driver.request('POST', 'auth/logout');

  // 4. Reportes: «Autorizó» sigue en «Modificaciones de cantidades» y «Productos cancelados», vacía.
  await login(page, 'admin', 'admin');
  await openAdministration(page, 'Reportes');
  const changes = await openReport(page, 'Productos', 'Modificaciones de cantidades');
  await expect(changes.getByRole('columnheader', { name: /^Autorizó/ })).toBeVisible();
  const changeRow = changes.getByRole('row').filter({ hasText: 'Se cayó al servir 9H' });
  await expect(changeRow).toContainText('Cajera 9H-A');
  await expect(changeRow).not.toContainText('Administrador');
  await views(page, testInfo, '9h-modificaciones-de-cantidades', async () => {
    await expect(changeRow).toBeVisible();
  });
  const cancelled = await openReport(page, 'Productos', 'Productos cancelados');
  await expect(cancelled.getByRole('columnheader', { name: /^Autorizó/ })).toBeVisible();
  const cancelledRow = cancelled.getByRole('row').filter({ hasText: 'Se cayó al servir 9H' });
  await expect(cancelledRow).toContainText('Cajera 9H-A');
  await expect(cancelledRow).not.toContainText('Administrador');
  await clearToasts(page);
  await logout(page);
  expect(errors).toEqual([]);
});

test('«Mostrar precio unitario» del diseñador: desactivada por omisión y en la vista previa a 80 y 58 mm (etapa 9H, fase A)', async ({
  page,
}, testInfo) => {
  test.setTimeout(240000);
  const errors: string[] = [];
  page.on('pageerror', (error) => errors.push(error.message));
  const driver = await installDriver(page);
  await call(driver, 'POST', 'auth/login', { username: 'admin', password: 'admin' });
  await call(driver, 'PUT', 'terminal/preferences', { keyboard_mode: 'disabled' });
  await driver.request('POST', 'auth/logout');
  await page.setViewportSize({ width: 1280, height: 720 });
  await page.goto('/');
  await login(page, 'admin', 'admin');
  await openAdministration(page, 'Documentos');

  // 1. Se duplica la predeterminada del ticket (que no cambia) y se abre la copia en el editor.
  const templateName = 'Ticket con precio unitario 9H';
  const receipts = page.getByRole('region', { name: 'Plantillas de Ticket de pago' });
  await receipts.getByRole('button', { name: 'Duplicar Predeterminada', exact: true }).click();
  const nameDialog = page.getByRole('dialog', { name: 'Duplicar plantilla' });
  await nameDialog.getByRole('textbox').fill(templateName);
  await nameDialog.getByRole('button', { name: 'Duplicar', exact: true }).click();
  await expect(notice(page, `Plantilla «${templateName}» creada.`)).toBeVisible();
  const editor = page.getByRole('region', { name: `Editor de ${templateName}` });
  const paper = editor.locator('.paper-strip');
  await expect(paper).toBeVisible();
  await expect(paper).toHaveAttribute('data-paper', '80');
  const lines = paper.locator('.paper-line');
  await expect(lines.filter({ hasText: 'Platillo de muestra' })).toHaveText(
    /^2 x Platillo de muestra +\$500\.00$/,
  );
  await expect(paper).not.toContainText('×');

  // 2. «Mostrar precio unitario» está desactivada; activada, Go lo pone en el mismo renglón a 42
  // columnas («2 × $250.00 Platillo de muestra … $500.00»).
  const structure = editor.getByRole('list', { name: 'Estructura de la plantilla' });
  await structure.getByRole('button', { name: /^\d+\. Lista de productos/ }).click();
  const properties = editor.getByRole('region', { name: 'Propiedades del bloque' });
  const toggle = properties.getByRole('button', { name: 'Mostrar precio unitario', exact: true });
  await expect(toggle).toHaveAttribute('aria-pressed', 'false');
  await toggle.click();
  await expect(toggle).toHaveAttribute('aria-pressed', 'true');
  await expect(
    structure.getByRole('button', { name: /^\d+\. Lista de productos · Con precio unitario/ }),
  ).toBeVisible();
  await expect(lines.filter({ hasText: 'Platillo de muestra' })).toHaveText(
    /^2 × \$250\.00 Platillo de muestra +\$500\.00$/,
    { timeout: 10000 },
  );
  await expect(lines.filter({ hasText: 'Bebida de muestra' })).toHaveText(
    /^1 × \$45\.00 Bebida de muestra +\$45\.00$/,
  );
  await views(page, testInfo, '9h-precio-unitario-80mm', async () => {
    await expect(paper).toBeVisible();
  });

  // 3. A 58 mm (30 columnas) no cabe: los renglones quedan como siempre (el nombre largo se corta como
  // antes) y el precio unitario va en un renglón propio debajo del nombre.
  await editor
    .getByRole('group', { name: 'Ancho del papel', exact: true })
    .getByRole('button', { name: '58 mm', exact: true })
    .click();
  await expect(paper).toHaveAttribute('data-paper', '58');
  await expect(editor.getByText('30 columnas')).toBeVisible();
  const texts = async () => (await lines.allTextContents()).map((text) => text.replace(/ /g, ' '));
  // El renglón que termina con el importe y el siguiente, para cada producto.
  const withNext = async (amount: string) => {
    const all = await texts();
    const index = all.findIndex((text) => text.endsWith(amount));
    return index < 0 ? '' : `${all[index]}|${all[index + 1] ?? ''}`;
  };
  await expect.poll(() => withNext('$500.00')).toMatch(/^a +\$500\.00\| {3}2 × \$250\.00$/);
  await expect
    .poll(() => withNext('$45.00'))
    .toMatch(/^1 x Bebida de muestra +\$45\.00\| {3}1 × \$45\.00$/);
  await views(page, testInfo, '9h-precio-unitario-58mm', async () => {
    await expect(paper).toBeVisible();
  });
  expect(errors).toEqual([]);
});

test('el mismo usuario con sesión en dos terminales a la vez; iniciar de nuevo en la misma cierra la anterior (etapa 9H, fase A)', async ({
  browser,
}, testInfo) => {
  test.setTimeout(240000);
  const contextA = await browser.newContext({ viewport: { width: 1280, height: 720 } });
  const pageA = await contextA.newPage();
  const driverA = await installDriver(pageA);
  await call(driverA, 'POST', 'auth/login', { username: 'admin', password: 'admin' });
  await call(driverA, 'PUT', 'terminal/preferences', { keyboard_mode: 'disabled' });

  // 1. Una segunda terminal con su propia credencial (enrolada con una invitación).
  const terminalName = 'Comedor 9H-A';
  const invitation = await call(driverA, 'POST', 'devices/invitations');
  const enrolled = await fetch(`${serverURL}/api/v1/devices/enroll`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'X-Client-Version': clientVersion,
      'X-Client-Kind': 'desktop',
    },
    body: JSON.stringify({ code: invitation.code, name: terminalName }),
  });
  expect(enrolled.status).toBe(200);
  const credentials = (await enrolled.json()) as { device_id: string; device_token: string };
  await call(driverA, 'PATCH', `devices/${credentials.device_id}`, {
    name: terminalName,
    register_id: null,
    keyboard_mode: 'disabled',
  });
  const activeSessions = async () =>
    ((await call(driverA, 'GET', 'devices/sessions-summary')) as { total: number }).total;
  await call(driverA, 'POST', 'auth/logout');

  // 2. El usuario inicia sesión en la terminal A desde su ventana. Iniciar sesión otra vez en la misma
  // terminal cierra la sesión anterior de esa terminal: el conteo no cambia y la ventana sigue con la nueva.
  await pageA.goto('/');
  await login(pageA, 'admin', 'admin');
  await expect(pageA.getByRole('button', { name: /^Menú del usuario/ })).toBeVisible();
  const before = await activeSessions();
  // Volver a entrar a propósito con la misma terminal: el puente espera a que terminen las peticiones de la página en
  // camino (etapa M1, §5.8), para que ninguna reciba SESSION_REPLACED con la sesión anterior.
  await call(driverA, 'POST', 'auth/login', { username: 'admin', password: 'admin' });
  expect(await activeSessions()).toBe(before);

  // 3. El mismo usuario inicia sesión en la terminal B: la A sigue con su sesión y el conteo sube en uno
  // (cuenta sesiones, no usuarios).
  const contextB = await browser.newContext({ viewport: { width: 1280, height: 720 } });
  const pageB = await contextB.newPage();
  await installDriver(pageB, {
    terminal: { deviceID: credentials.device_id, deviceToken: credentials.device_token },
  });
  await pageB.goto('/');
  await login(pageB, 'admin', 'admin');
  await expect(pageB.getByRole('button', { name: /^Menú del usuario/ })).toBeVisible();
  await expect.poll(activeSessions).toBe(before + 1);
  await call(driverA, 'GET', 'auth/me');
  await openAdministration(pageA, 'Terminales');
  const row = pageA.locator('tbody tr').filter({ hasText: terminalName });
  await expect(row).toContainText('Administrador', { timeout: 10000 });
  await expect(row).toContainText('En línea');
  // Etapa L1 (Fase E, §5): el resumen de los cupos de la licencia reemplaza al de sesiones por tipo de terminal.
  const summary = pageA.getByRole('region', { name: 'Cupos de la licencia' });
  await expect(summary).toContainText('Comanderos');
  await views(pageA, testInfo, '9h-mismo-usuario-dos-terminales', async () => {
    await expect(row).toBeVisible();
  });
  // La terminal B sigue trabajando con su sesión.
  await openSection(pageB, 'Mesas');
  await expect(pageB.getByRole('heading', { name: 'Inicia sesión' })).toHaveCount(0);

  // 4. Otra ventana con la credencial de la terminal B (como si la aplicación de esa terminal se
  // hubiera abierto de nuevo) inicia sesión con el mismo usuario: la sesión anterior de B se cierra,
  // la de A no, y el conteo no cambia.
  const contextC = await browser.newContext({ viewport: { width: 1280, height: 720 } });
  const pageC = await contextC.newPage();
  const driverC = await installDriver(pageC, {
    terminal: { deviceID: credentials.device_id, deviceToken: credentials.device_token },
  });
  await call(driverC, 'POST', 'auth/login', { username: 'admin', password: 'admin' });
  await expect(pageB.getByRole('heading', { name: 'Inicia sesión' })).toBeVisible({
    timeout: 20000,
  });
  await expect(
    pageB.getByText(
      'Tu sesión se cerró porque el usuario inició sesión de nuevo en esta terminal.',
    ),
  ).toBeVisible();
  await call(driverA, 'GET', 'auth/me');
  await expect(pageA.getByRole('heading', { name: 'Inicia sesión' })).toHaveCount(0);
  expect(await activeSessions()).toBe(before + 1);
  const sessions = (await call(driverA, 'GET', 'sessions')) as {
    username: string;
    device_id: string;
    revoked_at: string | null;
    revoke_reason: string | null;
  }[];
  const onB = sessions.filter(
    (session) => session.device_id === credentials.device_id && session.username === 'admin',
  );
  expect(onB.filter((session) => session.revoked_at === null)).toHaveLength(1);
  expect(onB.filter((session) => session.revoke_reason === 'replaced')).toHaveLength(1);
  await call(driverC, 'POST', 'auth/logout');
  await contextC.close();
  await contextB.close();
  await contextA.close();
});
