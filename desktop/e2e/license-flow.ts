import { copyFileSync, mkdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { expect } from '@playwright/test';
import type { Page, TestInfo } from '@playwright/test';
import type { installDriver } from './native-driver';
import { issueTestLicense } from './native-driver';
import { openAdministration, topBar } from './navigation';

// Ayudas de las especificaciones de la etapa L1 (Fase E): entrar, el tema claro de las capturas, Configuración ›
// Licencia y una segunda terminal de administración para cambiar la licencia mientras la página usa otro usuario.

export type Driver = Awaited<ReturnType<typeof installDriver>>;

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
// $SP/shots-l1/escritorio).
export async function shot(page: Page, testInfo: TestInfo, name: string) {
  const path = testInfo.outputPath(`${name}.png`);
  await page.screenshot({ path, fullPage: true });
  const directory = process.env.ADEL_SHOTS_DIR;
  if (directory) {
    mkdirSync(directory, { recursive: true });
    copyFileSync(path, join(directory, `${testInfo.project.name}-${name}.png`));
  }
}

// Otra terminal de escritorio con su propia credencial (alta con una invitación, como en la 9H) y una sesión de
// administrador: con ella la prueba cambia la licencia sin tocar la sesión de la página.
export async function otherTerminal(driver: Driver, serverURL: string, name: string) {
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
