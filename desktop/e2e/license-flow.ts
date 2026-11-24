import { copyFileSync, mkdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { expect } from '@playwright/test';
import type { Browser, Locator, Page, TestInfo } from '@playwright/test';
import { installDriver, issueTestLicense } from './native-driver';
import { openAdministration, topBar } from './navigation';

// Ayudas de las especificaciones de la etapa L1 (Fase E): entrar, el tema claro de las capturas, Configuración ›
// Licencia, otras terminales (por la API o con su propia página) y dejar el Go temporal compartido con la licencia de
// las demás especificaciones al terminar.

export type Driver = Awaited<ReturnType<typeof installDriver>>;

// El Go temporal compartido (el mismo valor por omisión que native-driver.ts).
export const sharedServerURL = process.env.ADEL_BROWSER_SERVER_URL ?? 'http://127.0.0.1:8183';
export const admin = { username: 'admin', password: 'admin' };

const clientHeaders = {
  'X-Client-Version': (
    JSON.parse(readFileSync(new URL('../src/generated/product.json', import.meta.url), 'utf8')) as {
      version: string;
    }
  ).version,
  'X-Client-Kind': 'desktop',
};

export async function call(driver: Driver, method: string, route: string, body?: unknown) {
  const response = await driver.request(method, route, body);
  expect(response.status, `${method} ${route}: ${JSON.stringify(response.body)}`).toBe(200);
  return response.body;
}

// Las capturas de la etapa van en tema claro: la preferencia del usuario se guarda en Go (la aplica App.tsx).
export async function lightTheme(driver: Driver) {
  const preferences = (await call(driver, 'GET', 'preferences')) as { home_screen?: string };
  await call(driver, 'PUT', 'preferences', {
    theme: 'light',
    home_screen: preferences.home_screen ?? 'tables',
  });
}

export async function login(page: Page, username = 'admin', password = 'admin') {
  await page.goto('/');
  await page.getByLabel('Usuario', { exact: true }).fill(username);
  await page.getByLabel('Contraseña', { exact: true }).fill(password);
  await page.getByRole('button', { name: 'Entrar', exact: true }).click();
}

export async function logout(page: Page) {
  await page.getByRole('button', { name: /^Menú del usuario/ }).click();
  await page
    .getByRole('group', { name: 'Menú del usuario' })
    .getByRole('button', { name: 'Cerrar sesión', exact: true })
    .click();
  await expect(page.getByRole('heading', { name: 'Inicia sesión' })).toBeVisible();
}

export async function openLicense(page: Page) {
  await openAdministration(page, 'Configuración');
  await page.getByRole('button', { name: 'Licencia', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Licencia', level: 1 })).toBeVisible();
  await expect(page.getByRole('navigation', { name: 'Ruta de navegación' })).toHaveText(
    'Administración›Sistema›Configuración›Licencia',
  );
}

export async function expectHome(page: Page) {
  await expect(topBar(page)).toBeVisible();
}

// Captura en la carpeta de la prueba y, si se indica, también en ADEL_SHOTS_DIR (las de la etapa:
// $SP/shots-l1/escritorio). Con un diálogo abierto, solo la ventana (`fullPage: false`): su fondo oscurecido es fijo.
export async function shot(
  page: Page,
  testInfo: TestInfo,
  name: string,
  options: { fullPage?: boolean } = {},
) {
  // Desde arriba: con la página desplazada, la captura de página completa dibuja la barra fija a media imagen.
  await page.evaluate(() => window.scrollTo(0, 0));
  // Los avisos entran con una animación corta.
  await page.waitForTimeout(400);
  const path = testInfo.outputPath(`${name}.png`);
  await page.screenshot({ path, fullPage: options.fullPage ?? true });
  const directory = process.env.ADEL_SHOTS_DIR;
  if (directory) {
    mkdirSync(directory, { recursive: true });
    copyFileSync(path, join(directory, `${testInfo.project.name}-${name}.png`));
  }
}

// Alta de otra terminal de escritorio con su propia credencial (con una invitación, como en la 9H), sin caja.
export async function enrollTerminal(driver: Driver, serverURL: string, name: string) {
  const invitation = (await call(driver, 'POST', 'devices/invitations')) as { code: string };
  const enrolled = await fetch(`${serverURL}/api/v1/devices/enroll`, {
    method: 'POST',
    headers: { ...clientHeaders, 'Content-Type': 'application/json' },
    body: JSON.stringify({ code: invitation.code, name }),
  });
  expect(enrolled.status).toBe(200);
  const credentials = (await enrolled.json()) as { device_id: string; device_token: string };
  await call(driver, 'PATCH', `devices/${credentials.device_id}`, {
    name,
    register_id: null,
    keyboard_mode: 'disabled',
  });
  return credentials;
}

// La página de otra terminal de escritorio con su credencial. Con `main: false` es una terminal emparejada con la
// computadora principal (sin servidor propio), como la de una caja en otra computadora.
export async function terminalPage(
  browser: Browser,
  credentials: { device_id: string; device_token: string },
  options: { main?: boolean } = {},
) {
  const context = await browser.newContext({ viewport: { width: 1280, height: 800 } });
  const page = await context.newPage();
  const driver = await installDriver(page, {
    terminal: { deviceID: credentials.device_id, deviceToken: credentials.device_token },
  });
  if (options.main === false) {
    driver.setInstallation({ chosen: true, mode: 'terminal-only', paired: true });
  }
  return { context, page, driver };
}

// Otra terminal de escritorio con su propia credencial y una sesión de administrador por la API: con ella la prueba
// cambia la licencia u ocupa cupos sin tocar la sesión de la página.
export async function otherTerminal(driver: Driver, serverURL: string, name: string) {
  const credentials = await enrollTerminal(driver, serverURL, name);
  let session = '';
  async function request(method: string, route: string, body?: unknown) {
    const response = await fetch(`${serverURL}/api/v1/${route}`, {
      method,
      headers: {
        ...clientHeaders,
        'Content-Type': 'application/json',
        'X-Device-ID': credentials.device_id,
        'X-Device-Token': credentials.device_token,
        ...(session ? { Authorization: `Bearer ${session}` } : {}),
      },
      body: body === undefined || method === 'GET' ? undefined : JSON.stringify(body),
    });
    const result = (await response.json()) as Record<string, unknown>;
    if (response.ok && typeof result.session_token === 'string') session = result.session_token;
    return { status: response.status, body: result };
  }
  async function expectOK(method: string, route: string, body?: unknown) {
    const response = await request(method, route, body);
    expect(response.status, `${method} ${route}: ${JSON.stringify(response.body)}`).toBe(200);
    return response.body;
  }
  await expectOK('POST', 'auth/login', { username: 'admin', password: 'admin' });
  return {
    id: credentials.device_id,
    token: credentials.device_token,
    request,
    expectOK,
    // Instala una licencia de prueba firmada para el Go temporal (POST license/import).
    installLicense: (identity: string, args?: string[]) =>
      expectOK('POST', 'license/import', { license: issueTestLicense(identity, args) }),
  };
}

// Cupos de la licencia en uso y su límite (devices/sessions-summary, Fase D).
export type Seat = 'cash_registers' | 'order_takers' | 'kitchen_displays';
export async function seatUsage(control: ControlTerminal) {
  const summary = (await control.expectOK('GET', 'devices/sessions-summary')) as {
    seats: Record<Seat, { in_use: number; limit: number }>;
  };
  return summary.seats;
}

export type ControlTerminal = Awaited<ReturnType<typeof otherTerminal>>;

// Deja el Go temporal compartido con la licencia de las demás especificaciones (perpetua, 20 de cada cupo), desde la
// terminal de administración de la prueba. Va en el finally de cada especificación de la etapa que cambia la licencia,
// también si la prueba falla.
export async function restoreTestLicense(control: ControlTerminal, identity: string) {
  await control.expectOK('POST', 'auth/login', admin);
  await control.installLicense(identity);
}

// Una categoría con un producto sencillo (sin personalización) para capturar desde la página.
export async function simpleProduct(driver: Driver, name: string, code: string) {
  const category = (await call(driver, 'POST', 'catalog/categories', {
    name: `Bebidas ${name}`,
    active: true,
    color: '#d64545',
  })) as { id: string; name: string };
  await call(driver, 'POST', 'catalog/products', {
    name,
    active: true,
    category_id: category.id,
    code,
    kind: 'product',
    base_price: '20',
  });
  return { category: category.name, product: name };
}

// Para llevar › Nuevo pedido › el producto: el pedido nace al agregar el primer producto (POST restaurant/accounts),
// que es cuando Go pide el cupo y revisa la hora.
export async function captureTakeaway(page: Page, item: { category: string; product: string }) {
  await page.getByRole('button', { name: 'Nuevo pedido', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Para llevar · Pedido nuevo' })).toBeVisible();
  await page
    .getByRole('navigation', { name: 'Categorías' })
    .getByRole('button', { name: new RegExp(item.category) })
    .click();
  await page.getByRole('button', { name: new RegExp(`^${item.product}`) }).click();
}

// El mensaje de una operación rechazada (en el panel de la operación o en la pila de avisos), aparte de los avisos de la
// licencia y de la hora, que pueden decir lo mismo.
export function operationMessage(page: Page, text: string | RegExp): Locator {
  return page.locator('.notice, [data-sonner-toaster]').filter({ hasText: text }).first();
}

// Cierra los avisos de la pila (los de error no se van solos), del de enfrente hacia atrás, para que una captura
// posterior no muestre el aviso de un paso anterior.
export async function dismissNotices(page: Page) {
  const toasts = page.locator('[data-sonner-toast]');
  for (let attempt = 0; attempt < 10 && (await toasts.count()) > 0; attempt += 1) {
    const front = page.locator('[data-sonner-toast][data-front="true"]');
    const target = (await front.count()) > 0 ? front.first() : toasts.first();
    await target.getByRole('button', { name: 'Cerrar aviso' }).click({ force: true });
    await page.waitForTimeout(500);
  }
  await expect(toasts).toHaveCount(0, { timeout: 10000 });
}
