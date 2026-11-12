import { randomUUID } from 'node:crypto';
import { copyFileSync, mkdirSync } from 'node:fs';
import { join } from 'node:path';
import { expect, test } from '@playwright/test';
import type { Locator, Page, TestInfo } from '@playwright/test';
import { installDriver } from './native-driver';
import { captureBar, cashButton, topBar } from './navigation';

// Etapa 9C, fase C: recorrido solo con teclado (atajos, paleta de comandos, Escape) y el menú
// nativo simulado por el puente de pruebas (menu_subscribe y menu_set_state de menu.rs). Mismo
// transporte que las demás especificaciones (Go temporal y base adel_pos_test_*); el área y la mesa
// se crean por la API con nombres propios. Los atajos usan ⌘ en macOS y Ctrl en los demás sistemas,
// como la aplicación.

type Driver = Awaited<ReturnType<typeof installDriver>>;

async function create(driver: Driver, route: string, body: Record<string, unknown>) {
  const response = await driver.request('POST', route, body);
  expect(response.status, `${route}: ${JSON.stringify(response.body)}`).toBe(200);
  return response.body as { id: string };
}

// Lleva el foco al control con la tecla Tab (en WebKit, Opción+Tab recorre también los botones,
// como en Safari).
async function tabTo(page: Page, target: Locator, tabKey: string, limit = 60) {
  for (let step = 0; step < limit; step += 1) {
    const focused = await target.evaluate((element) => element === document.activeElement);
    if (focused) {
      return;
    }
    await page.keyboard.press(tabKey);
  }
  throw new Error('El control no se alcanzó con el tabulador.');
}

// La paleta y los diálogos caben sin desplazamiento horizontal a 1280 × 720 y 1920 × 1080, en ambos
// temas, con capturas (copia opcional en ADEL_SHOTS_DIR).
async function checkFits(page: Page, testInfo: TestInfo, name: string, dialog: Locator) {
  for (const size of [
    { width: 1280, height: 720 },
    { width: 1920, height: 1080 },
  ]) {
    await page.setViewportSize(size);
    for (const theme of ['light', 'dark']) {
      await page.evaluate((value) => {
        document.documentElement.dataset.theme = value;
      }, theme);
      await page.waitForTimeout(200);
      const overflow = await page.evaluate(
        () => document.documentElement.scrollWidth - window.innerWidth,
      );
      expect(overflow, `${name} se desborda a ${size.width} × ${size.height} (${theme})`).toBe(0);
      const box = await dialog.boundingBox();
      expect(box).not.toBeNull();
      expect(box!.x).toBeGreaterThanOrEqual(0);
      expect(box!.y).toBeGreaterThanOrEqual(0);
      expect(box!.x + box!.width).toBeLessThanOrEqual(size.width + 0.5);
      expect(box!.y + box!.height).toBeLessThanOrEqual(size.height + 0.5);
      const file = `${name}-${theme}-${size.width}x${size.height}.png`;
      const path = testInfo.outputPath(file);
      await page.screenshot({ path });
      const directory = process.env.ADEL_SHOTS_DIR;
      if (directory) {
        mkdirSync(directory, { recursive: true });
        copyFileSync(
          path,
          join(directory, `${process.env.ADEL_BROWSER_ENGINE ?? 'browser'}-${file}`),
        );
      }
    }
  }
  await page.setViewportSize({ width: 1280, height: 720 });
}

function palette(page: Page) {
  return page.getByRole('dialog', { name: 'Paleta de comandos' });
}

test('teclado 9C: atajos, paleta, Escape y menú nativo simulado', async ({
  page,
  browserName,
}, testInfo) => {
  test.setTimeout(240000);
  const errors: string[] = [];
  page.on('pageerror', (error) => errors.push(error.message));
  const driver = await installDriver(page);
  await driver.request('POST', 'auth/login', { username: 'admin', password: 'admin' });
  const keyboardMode = await driver.request('PUT', 'terminal/preferences', {
    keyboard_mode: 'disabled',
  });
  expect(keyboardMode.status, JSON.stringify(keyboardMode.body)).toBeLessThan(300);
  const area = await create(driver, 'restaurant/areas', {
    operation_id: randomUUID(),
    name: 'Terraza teclado',
    active: true,
  });
  await create(driver, 'restaurant/tables', {
    operation_id: randomUUID(),
    area_id: area.id,
    name: 'Mesa 5',
    shape: 'rectangle',
    x: 40,
    y: 40,
    width: 100,
    height: 72,
    seats: 4,
    active: true,
  });
  await driver.request('POST', 'auth/logout');
  await page.setViewportSize({ width: 1280, height: 720 });
  await page.goto('/');
  const mac = await page.evaluate(() =>
    /Mac|iPhone|iPad/.test(`${navigator.platform} ${navigator.userAgent}`),
  );
  const mod = mac ? 'Meta' : 'Control';
  const tabKey = browserName === 'webkit' ? 'Alt+Tab' : 'Tab';
  const fullscreenKey = mac ? 'Control+Meta+f' : 'F11';

  // Antes de iniciar sesión el menú solo habilita Ayuda y la pantalla completa.
  await expect
    .poll(() => driver.menuState())
    .toEqual(['help.about', 'help.shortcuts', 'view.fullscreen']);

  // 1. Acceso con el teclado.
  // El campo Usuario ya trae «admin»: se selecciona su texto y se reemplaza.
  await page.getByLabel('Usuario', { exact: true }).focus();
  await page.keyboard.press(`${mod}+a`);
  await page.keyboard.type('admin');
  await page.keyboard.press('Tab');
  await page.keyboard.type('admin');
  await page.keyboard.press('Enter');
  const bar = topBar(page);
  const tables = bar.getByRole('button', { name: 'Mesas', exact: true });
  await expect(tables).toHaveAttribute('aria-current', 'page');
  // El atajo aparece en la descripción del botón y el menú se habilita según los permisos.
  await expect(tables).toHaveAttribute('title', mac ? 'Mesas · ⌘1' : 'Mesas · Ctrl+1');
  await expect(tables).toHaveAttribute('aria-keyshortcuts', mac ? 'Meta+1' : 'Control+1');
  await expect.poll(() => driver.menuState()).toContain('view.kitchen');
  expect(driver.menuState()).toEqual(
    expect.arrayContaining(['view.administration', 'file.account_without_table', 'cash.open']),
  );
  expect(driver.menuState()).not.toContain('file.print_precheck');

  // 2. Secciones con ⌘/Ctrl + 1–5.
  await page.keyboard.press(`${mod}+3`);
  await expect(page.getByRole('heading', { name: 'Cocina y preparación' })).toBeVisible();
  await page.keyboard.press(`${mod}+4`);
  await expect(page.getByRole('heading', { name: 'Caja', exact: true })).toBeVisible();
  await expect(cashButton(page)).toHaveAttribute('aria-current', 'page');
  await page.keyboard.press(`${mod}+5`);
  await expect(page.getByRole('heading', { name: 'Administración', exact: true })).toBeVisible();
  await page.keyboard.press(`${mod}+2`);
  await expect(bar.getByRole('button', { name: 'Para llevar', exact: true })).toHaveAttribute(
    'aria-current',
    'page',
  );
  await page.keyboard.press(`${mod}+1`);
  await expect(tables).toHaveAttribute('aria-current', 'page');

  // 3. Paleta: «productos» lleva al Catálogo; Intro ejecuta la opción activa.
  await page.keyboard.press(`${mod}+k`);
  await expect(palette(page)).toBeVisible();
  await expect(palette(page).getByRole('combobox')).toBeFocused();
  await page.keyboard.type('productos');
  await expect(palette(page).getByRole('option').first()).toContainText('Catálogo');
  await expect(palette(page).getByRole('option').first()).toHaveAttribute('aria-selected', 'true');
  await page.keyboard.press('Enter');
  await expect(palette(page)).toHaveCount(0);
  await expect(page.getByRole('navigation', { name: 'Ruta de navegación' })).toHaveText(
    'Administración›Catálogo›Productos',
  );
  // «Personal» y Escape: vuelve un nivel (a Administración).
  await page.keyboard.press(`${mod}+k`);
  await page.keyboard.type('personal');
  await page.keyboard.press('Enter');
  await expect(page.getByRole('heading', { name: 'Personal', exact: true })).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(page.getByRole('heading', { name: 'Administración', exact: true })).toBeVisible();
  // Escape dentro de la paleta solo la cierra.
  await page.keyboard.press(`${mod}+k`);
  await page.keyboard.type('retiro');
  await expect(palette(page).getByRole('option', { name: /Retiro/ })).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(palette(page)).toHaveCount(0);
  await expect(page.getByRole('heading', { name: 'Administración', exact: true })).toBeVisible();

  // 4. «mesa 5» en la paleta elige la mesa como en el mapa.
  await page.keyboard.press(`${mod}+k`);
  await page.keyboard.type('mesa 5');
  await expect(palette(page).getByRole('option').first()).toContainText('Mesa 5');
  await page.keyboard.press('Enter');
  const panel = page.getByRole('complementary', { name: 'Panel de Mesa 5' });
  await expect(panel).toBeVisible();
  await expect(tables).toHaveAttribute('aria-current', 'page');

  // 5. ⌘/Ctrl + N: «Nueva cuenta en mesa» = «Capturar» del panel (la cuenta se crea con el primer
  // producto). ⌘/Ctrl + F lleva al buscador de productos; mientras se escribe, los atajos no
  // actúan.
  await page.keyboard.press(`${mod}+n`);
  await expect(page.getByRole('heading', { name: 'Mesa 5 · Cuenta nueva' })).toBeVisible();
  await expect(captureBar(page)).toBeVisible();
  await page.keyboard.press(`${mod}+f`);
  const search = page.getByRole('searchbox', { name: 'Buscar producto por nombre o código' });
  await expect(search).toBeFocused();
  await page.keyboard.type('Filete');
  await page.keyboard.press(`${mod}+3`);
  await expect(search).toBeFocused();
  await expect(page.getByRole('heading', { name: 'Mesa 5 · Cuenta nueva' })).toBeVisible();
  const bill = page.getByRole('complementary', { name: 'Cuenta actual' });
  await tabTo(page, page.getByRole('button', { name: /^Filete de prueba/ }), tabKey);
  await page.keyboard.press('Enter');
  await expect(bill.getByRole('article', { name: '1 × Filete de prueba' })).toContainText('Nuevo');
  const send = bill.getByRole('button', { name: 'Enviar a cocina · 1', exact: true });
  await expect(send).toBeEnabled();
  await tabTo(page, send, tabKey);
  await page.keyboard.press('Enter');
  await expect(bill.getByRole('article', { name: '1 × Filete de prueba' })).toContainText(
    'En cocina',
  );
  await expect(page.getByRole('heading', { name: /^Mesa 5 · Cuenta #\d+$/ })).toBeVisible();

  // 6. ⌘/Ctrl + P abre la precuenta (como su botón) sin imprimir: imprimir sigue siendo un botón.
  await expect.poll(() => driver.menuState()).toContain('file.print_precheck');
  await page.keyboard.press(`${mod}+p`);
  const precheck = page.getByRole('region', { name: 'Precuenta' });
  await expect(precheck).toBeVisible();
  await expect(precheck.getByRole('button', { name: 'Imprimir precuenta' })).toBeVisible();
  await page.waitForTimeout(500);
  await expect(page.getByText(/Precuenta enviada/)).toHaveCount(0);

  // 7. Escape: cierra la precuenta, sale de la captura («‹ Mesas») y cierra el panel de la mesa.
  await page.keyboard.press('Escape');
  await expect(precheck).toHaveCount(0);
  await expect(page.getByRole('heading', { name: /^Mesa 5 · Cuenta #\d+$/ })).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(page.locator('.app-topbar')).toBeVisible();
  await expect(panel).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(panel).toHaveCount(0);

  // 8. En el mapa (sin buscador propio), ⌘/Ctrl + F busca mesas.
  await page.keyboard.press(`${mod}+f`);
  const tableSearch = page.getByRole('dialog', { name: 'Buscar mesa' });
  await expect(tableSearch).toBeVisible();
  await page.keyboard.type('mesa 5');
  await page.keyboard.press('Enter');
  await expect(panel).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(panel).toHaveCount(0);

  // 9. Menú nativo simulado: las opciones llegan por el canal y ejecutan la misma acción.
  expect(await driver.clickMenu('view.kitchen')).toBe(true);
  await expect(page.getByRole('heading', { name: 'Cocina y preparación' })).toBeVisible();
  await expect.poll(() => driver.menuState()).not.toContain('file.print_precheck');
  expect(await driver.clickMenu('file.print_precheck')).toBe(false);
  expect(await driver.clickMenu('help.shortcuts')).toBe(true);
  const shortcuts = page.getByRole('dialog', { name: 'Atajos de teclado' });
  await expect(shortcuts).toBeVisible();
  await expect(shortcuts.getByText(mac ? '⌘1' : 'Ctrl+1', { exact: true })).toBeVisible();
  await expect(shortcuts.getByText(mac ? '⌃⌘F' : 'F11', { exact: true })).toBeVisible();
  const theme = await page.evaluate(() => document.documentElement.dataset.theme);
  await checkFits(page, testInfo, 'shortcuts', shortcuts);
  await page.keyboard.press('Escape');
  await expect(shortcuts).toHaveCount(0);
  await page.keyboard.press(`${mod}+k`);
  await page.keyboard.type('caja');
  await checkFits(page, testInfo, 'palette', palette(page));
  await page.evaluate((value) => {
    document.documentElement.dataset.theme = value;
  }, theme);
  await page.keyboard.press('Escape');
  await expect(palette(page)).toHaveCount(0);
  expect(await driver.clickMenu('help.about')).toBe(true);
  const about = page.getByRole('dialog', { name: 'Acerca de AD-EL POS' });
  await expect(about.getByText(/^Versión \d+\.\d+\.\d+$/)).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(about).toHaveCount(0);

  // Caja: las opciones de la hoja siguen el estado del turno, como el menú rápido; abrir la hoja
  // no registra nada.
  const shift = await cashButton(page).getAttribute('data-shift-state');
  if (shift === 'open') {
    expect(await driver.clickMenu('cash.income')).toBe(true);
    const sheet = page.getByRole('dialog', { name: 'Ingreso de efectivo' });
    await expect(sheet).toBeVisible();
    await page.keyboard.press('Escape');
    await expect(sheet).toHaveCount(0);
  } else {
    await expect.poll(() => driver.menuState()).not.toContain('cash.income');
    expect(await driver.clickMenu('cash.income')).toBe(false);
  }

  // Tema claro u oscuro: la misma operación de Mis preferencias (se restaura al terminar).
  expect(await driver.clickMenu('view.theme')).toBe(true);
  await expect
    .poll(() => page.evaluate(() => document.documentElement.dataset.theme))
    .not.toBe(theme);
  expect(await driver.clickMenu('view.theme')).toBe(true);
  await expect.poll(() => page.evaluate(() => document.documentElement.dataset.theme)).toBe(theme);

  // 10. Sin doble ejecución: el atajo de pantalla completa y su gemela del menú alternan una sola
  // vez; pasada la ventana de gemelas, el menú vuelve a alternar.
  await expect(page.getByRole('button', { name: 'Salir de pantalla completa' })).toBeVisible();
  await page.keyboard.press(fullscreenKey);
  expect(await driver.clickMenu('view.fullscreen')).toBe(true);
  await expect(page.getByRole('button', { name: 'Pantalla completa', exact: true })).toBeVisible();
  await page.waitForTimeout(1000);
  await expect(page.getByRole('button', { name: 'Pantalla completa', exact: true })).toBeVisible();
  expect(await driver.clickMenu('view.fullscreen')).toBe(true);
  await expect(page.getByRole('button', { name: 'Salir de pantalla completa' })).toBeVisible();

  // El puente rechaza, como Rust, un estado con opciones fuera de la lista cerrada.
  const rejected = await page.evaluate(async () => {
    const native = window as unknown as {
      __TAURI_INTERNALS__: { invoke: (command: string, args: unknown) => Promise<unknown> };
    };
    try {
      await native.__TAURI_INTERNALS__.invoke('menu_set_state', { enabled: ['api_request'] });
      return 'aceptado';
    } catch (error) {
      return String(error);
    }
  });
  expect(rejected).toBe('Opción de menú no reconocida.');

  // 11. «Cerrar sesión» del menú: vuelve al acceso y el menú deshabilita lo que exige sesión.
  expect(await driver.clickMenu('file.logout')).toBe(true);
  await expect(page.getByRole('heading', { name: 'Inicia sesión' })).toBeVisible();
  await expect
    .poll(() => driver.menuState())
    .toEqual(['help.about', 'help.shortcuts', 'view.fullscreen']);
  expect(errors).toEqual([]);
});
