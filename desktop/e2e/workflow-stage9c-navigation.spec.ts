import { randomUUID } from 'node:crypto';
import { copyFileSync, mkdirSync } from 'node:fs';
import { join } from 'node:path';
import { expect, test } from '@playwright/test';
import type { Locator, Page, TestInfo } from '@playwright/test';
import { installDriver } from './native-driver';
import {
  captureBar,
  cashButton,
  openAdministration,
  openCash,
  openSection,
  topBar,
} from './navigation';
import { notice } from './notices';

// Etapa 9C, fase A: navegación completa por la barra superior, Administración por mosaicos con su
// ruta de navegación, inicio por terminal (Sistema → Terminales y el logotipo), «Estado del
// sistema» en lenguaje sencillo desde Configuración, «Registro de movimientos», la captura con la
// barra reducida y vistas a 1280 × 720 y 1920 × 1080 en ambos temas sin desbordamientos. Mismo
// transporte que las demás especificaciones (Go temporal y base adel_pos_test_*); el área y la
// mesa se crean por la API con nombres propios. El nombre la ordena después de las existentes.

type Driver = Awaited<ReturnType<typeof installDriver>>;

async function create(driver: Driver, route: string, body: Record<string, unknown>) {
  const response = await driver.request('POST', route, body);
  expect(response.status, `${route}: ${JSON.stringify(response.body)}`).toBe(200);
  return response.body as { id: string };
}

// Copia opcional de las capturas a un directorio de revisión (ADEL_SHOTS_DIR).
function keepShot(path: string, name: string) {
  const directory = process.env.ADEL_SHOTS_DIR;
  if (!directory) {
    return;
  }
  mkdirSync(directory, { recursive: true });
  copyFileSync(path, join(directory, `${process.env.ADEL_BROWSER_ENGINE ?? 'browser'}-${name}`));
}

// Sin desplazamiento horizontal en 1280 × 720 y 1920 × 1080, claro y oscuro, con capturas. La barra
// superior (o la reducida de la captura) queda dentro de la ventana y sus botones miden 52–56 px.
async function checkViews(page: Page, testInfo: TestInfo, name: string, bar: Locator) {
  for (const size of [
    { width: 1280, height: 720 },
    { width: 1920, height: 1080 },
  ]) {
    await page.setViewportSize(size);
    for (const theme of ['light', 'dark']) {
      await page.evaluate((value) => {
        document.documentElement.dataset.theme = value;
      }, theme);
      await page.waitForTimeout(300);
      const overflow = await page.evaluate(
        () => document.documentElement.scrollWidth - window.innerWidth,
      );
      expect(overflow, `${name} se desborda a ${size.width} × ${size.height} (${theme})`).toBe(0);
      const box = await bar.boundingBox();
      expect(box).not.toBeNull();
      expect(box!.x).toBeGreaterThanOrEqual(0);
      expect(box!.x + box!.width).toBeLessThanOrEqual(size.width + 0.5);
      const file = `${name}-${theme}-${size.width}x${size.height}.png`;
      const path = testInfo.outputPath(file);
      await page.screenshot({ path });
      keepShot(path, file);
    }
  }
  await page.evaluate(() => {
    document.documentElement.dataset.theme = 'light';
  });
  await page.setViewportSize({ width: 1280, height: 720 });
}

// Cada botón de la barra superior mide entre 52 y 56 px de alto (§0-B).
async function expectTouchHeights(bar: Locator) {
  const buttons = bar.locator('.topbar-button');
  const count = await buttons.count();
  expect(count).toBeGreaterThan(3);
  for (let index = 0; index < count; index += 1) {
    const box = await buttons.nth(index).boundingBox();
    expect(box!.height).toBeGreaterThanOrEqual(52);
    expect(box!.height).toBeLessThanOrEqual(56);
  }
}

function breadcrumb(page: Page) {
  return page.getByRole('navigation', { name: 'Ruta de navegación' });
}

test('navegación 9C: barra superior, mosaicos, ruta, inicio por terminal y captura reducida', async ({
  page,
}, testInfo) => {
  test.setTimeout(240000);
  const errors: string[] = [];
  page.on('pageerror', (error) => errors.push(error.message));
  const driver = await installDriver(page);
  await driver.request('POST', 'auth/login', { username: 'admin', password: 'admin' });
  const keyboard = await driver.request('PUT', 'terminal/preferences', {
    keyboard_mode: 'disabled',
  });
  expect(keyboard.status, JSON.stringify(keyboard.body)).toBeLessThan(300);
  const area = await create(driver, 'restaurant/areas', {
    operation_id: randomUUID(),
    name: 'Terraza 9C',
    active: true,
  });
  await create(driver, 'restaurant/tables', {
    operation_id: randomUUID(),
    area_id: area.id,
    name: 'Mesa 9C',
    shape: 'rectangle',
    x: 40,
    y: 40,
    width: 100,
    height: 72,
    seats: 2,
    active: true,
  });
  const terminal = ((await driver.request('GET', 'auth/me')).body as { terminal: { name: string } })
    .terminal.name;
  await driver.request('POST', 'auth/logout');
  await page.setViewportSize({ width: 1280, height: 720 });
  await page.goto('/');
  await page.getByLabel('Usuario', { exact: true }).fill('admin');
  await page.getByLabel('Contraseña', { exact: true }).fill('admin');
  await page.getByRole('button', { name: 'Entrar', exact: true }).click();

  // 1. La aplicación abre en el inicio de la terminal (Mesas por omisión), nunca en «Estado del
  // sistema». Sin menú lateral ni navegación angosta.
  const bar = topBar(page);
  await expect(bar.getByRole('button', { name: 'Mesas', exact: true })).toHaveAttribute(
    'aria-current',
    'page',
  );
  await expect(page.getByRole('heading', { name: 'Estado del sistema' })).toHaveCount(0);
  await expect(page.getByRole('combobox', { name: 'Pantalla', exact: true })).toHaveCount(0);
  await expect(page.locator('.admin-sidebar, .sidebar, .mobile-navigation')).toHaveCount(0);
  // Etapa 9D (§2): las cuatro secciones a la izquierda y Caja a la derecha.
  await expect(bar.getByRole('button')).toHaveText([
    'Mesas',
    'Para llevar',
    'Cocina',
    'Administración',
  ]);
  await expect(cashButton(page)).toBeVisible();
  await expect(page.getByRole('status', { name: 'Servidor: En línea' })).toBeVisible();
  await expectTouchHeights(page.locator('.app-topbar'));
  await checkViews(page, testInfo, 'bar-mesas', page.locator('.app-topbar'));

  // 2. Navegación completa por la barra: cada sección con su resaltado.
  await openSection(page, 'Para llevar');
  await expect(bar.getByRole('button', { name: 'Para llevar', exact: true })).toHaveAttribute(
    'aria-current',
    'page',
  );
  await expect(page.getByRole('button', { name: 'Nuevo pedido', exact: true })).toBeVisible();
  // Para llevar no repite las pestañas internas; Cuentas y Áreas siguen dentro de Mesas.
  await expect(page.getByRole('group', { name: 'Vistas operativas' })).toHaveCount(0);
  await openSection(page, 'Cocina');
  await expect(page.getByRole('heading', { name: 'Cocina y preparación' })).toBeVisible();
  await expect(bar.getByRole('button', { name: 'Cocina', exact: true })).toHaveAttribute(
    'aria-current',
    'page',
  );
  // Caja conserva el menú rápido de la 7.1 y su indicador del turno.
  const cash = cashButton(page);
  await expect(cash).toHaveAttribute('data-shift-state', /open|closing|none/);
  await openCash(page);
  await expect(page.getByRole('heading', { name: 'Caja', exact: true })).toBeVisible();
  await expect(cash).toHaveAttribute('aria-current', 'page');
  await openSection(page, 'Mesas');
  const views = page.getByRole('group', { name: 'Vistas operativas' });
  await expect(views.getByRole('button')).toHaveText(['Mesas', 'Cuentas', 'Áreas y distribución']);

  // 3. Pantalla completa: un botón de icono que alterna reducir y maximizar.
  await page.getByRole('button', { name: 'Salir de pantalla completa', exact: true }).click();
  await page.getByRole('button', { name: 'Pantalla completa', exact: true }).click();
  await expect(
    page.getByRole('button', { name: 'Salir de pantalla completa', exact: true }),
  ).toBeVisible();

  // 4. Administración por mosaicos con Personal y Sistema como segunda pantalla de mosaicos.
  await bar.getByRole('button', { name: 'Administración', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Administración', exact: true })).toBeVisible();
  const tiles = page.locator('.admin-tiles');
  await expect(tiles.getByRole('button')).toHaveText([
    /^Catálogo/,
    /^Inventarios/,
    /^Reportes/,
    /^Documentos/,
    /^Cobros/,
    /^Personal/,
    /^Sistema/,
  ]);
  await expect(bar.getByRole('button', { name: 'Administración', exact: true })).toHaveAttribute(
    'aria-current',
    'page',
  );
  await checkViews(page, testInfo, 'administration', page.locator('.app-topbar'));
  await tiles.getByRole('button', { name: 'Personal', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Personal', exact: true })).toBeVisible();
  await expect(tiles.getByRole('button')).toHaveText([/^Usuarios/, /^Roles/, /^Sesiones/]);
  await expect(breadcrumb(page)).toHaveText('Administración›Personal');
  await tiles.getByRole('button', { name: 'Usuarios', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Usuarios', exact: true })).toBeVisible();
  await expect(breadcrumb(page)).toHaveText('Administración›Personal›Usuarios');
  // Cada parte de la ruta es tocable.
  await breadcrumb(page).getByRole('button', { name: 'Personal', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Personal', exact: true })).toBeVisible();
  await breadcrumb(page).getByRole('button', { name: 'Administración', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Administración', exact: true })).toBeVisible();

  // 5. Ruta con la pestaña del módulo como último tramo: «‹ Administración › Catálogo › …».
  await tiles.getByRole('button', { name: 'Catálogo', exact: true }).click();
  await expect(breadcrumb(page)).toHaveText('Administración›Catálogo›Productos');
  await page.getByRole('tab', { name: 'Combos', exact: true }).click();
  await expect(breadcrumb(page)).toHaveText('Administración›Catálogo›Combos');
  await checkViews(page, testInfo, 'catalog-route', breadcrumb(page));
  await openAdministration(page, 'Inventarios');
  await expect(page.getByRole('heading', { name: 'Inventarios', exact: true })).toBeVisible();
  await expect(breadcrumb(page)).toHaveText('Administración›Inventarios›Resumen');

  // 6. Sistema: «Registro de movimientos» (antes Auditoría) y «Ver estado del sistema».
  await openAdministration(page, 'Sistema');
  await expect(tiles.getByRole('button')).toHaveText([
    /^Configuración/,
    /^Terminales/,
    /^Cajas físicas/,
    /^Registro de movimientos/,
  ]);
  await tiles.getByRole('button', { name: 'Registro de movimientos', exact: true }).click();
  await expect(
    page.getByRole('heading', { name: 'Registro de movimientos', exact: true }),
  ).toBeVisible();
  await expect(page.getByText('Auditoría', { exact: true })).toHaveCount(0);
  await openAdministration(page, 'Configuración');
  await page.getByRole('button', { name: 'Ver estado del sistema', exact: true }).click();
  await expect(
    page.getByRole('heading', { name: 'Estado del sistema', exact: true }),
  ).toBeVisible();
  await expect(breadcrumb(page)).toHaveText(
    'Administración›Sistema›Configuración›Estado del sistema',
  );
  await expect(page.locator('.system-status-summary')).toContainText('Todo funciona correctamente');
  const visible = await page.locator('main').innerText();
  for (const technical of ['PostgreSQL', 'Backend Go', '127.0.0.1', 'no detiene Go']) {
    expect(visible).not.toContain(technical);
  }
  const support = page.getByRole('button', { name: /Información para soporte/ });
  await expect(support).toHaveAttribute('aria-expanded', 'false');
  await support.click();
  await expect(page.locator('.support-information')).toContainText('127.0.0.1');
  await checkViews(page, testInfo, 'system-status', breadcrumb(page));

  // 7. Inicio por terminal: Sistema → Terminales con el permiso de administrarlas, y el logotipo
  // vuelve a la pantalla elegida. Se restaura Mesas al terminar.
  await openAdministration(page, 'Terminales');
  const row = page.locator('tbody tr').filter({ hasText: terminal });
  await row.getByRole('button', { name: 'Editar', exact: true }).click();
  const home = page.getByRole('group', { name: 'Pantalla inicial de la terminal' });
  await expect(home.getByRole('button', { name: 'Mesas', exact: true })).toHaveAttribute(
    'aria-pressed',
    'true',
  );
  await home.getByRole('button', { name: 'Para llevar', exact: true }).click();
  await page.getByRole('button', { name: 'Guardar cambios', exact: true }).click();
  await expect(notice(page, 'Terminal actualizada.')).toBeVisible();
  await expect(row).toContainText('Para llevar');
  await page.getByRole('button', { name: /^AD-EL POS · Ir al inicio \(Para llevar\)/ }).click();
  await expect(bar.getByRole('button', { name: 'Para llevar', exact: true })).toHaveAttribute(
    'aria-current',
    'page',
  );
  await openAdministration(page, 'Terminales');
  await row.getByRole('button', { name: 'Editar', exact: true }).click();
  await home.getByRole('button', { name: 'Mesas', exact: true }).click();
  await page.getByRole('button', { name: 'Guardar cambios', exact: true }).click();
  await expect(row).toContainText('Mesas');
  await expect(
    page.getByRole('button', { name: /^AD-EL POS · Ir al inicio \(Mesas\)/ }),
  ).toBeVisible();

  // 8. Menú del usuario: Mis preferencias (sin «Pantalla inicial») y Cambiar contraseña.
  await page.getByRole('button', { name: /^Menú del usuario/ }).click();
  const userMenu = page.getByRole('group', { name: 'Menú del usuario' });
  await expect(userMenu.getByRole('button')).toHaveText([
    'Mis preferencias',
    'Cambiar contraseña',
    'Cerrar sesión',
  ]);
  await userMenu.getByRole('button', { name: 'Mis preferencias', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Mis preferencias', exact: true })).toBeVisible();
  await expect(page.getByText('Pantalla inicial', { exact: true })).toHaveCount(0);

  // 9. Captura con la barra reducida: «‹ Mesas», mesa y cuenta, Caja y el usuario; Enviar a cocina
  // y Cobrar no se mueven.
  await page.getByRole('button', { name: /^AD-EL POS · Ir al inicio/ }).click();
  await page
    .getByRole('group', { name: 'Áreas del restaurante' })
    .getByRole('button', { name: 'Terraza 9C', exact: true })
    .click();
  await page.getByRole('button', { name: /^Mesa 9C, Libre/ }).click();
  await page
    .getByRole('complementary', { name: 'Panel de Mesa 9C' })
    .getByRole('button', { name: 'Capturar', exact: true })
    .click();
  await expect(page.getByRole('heading', { name: 'Mesa 9C · Cuenta nueva' })).toBeVisible();
  await expect(page.locator('.app-topbar')).toHaveCount(0);
  const reduced = captureBar(page);
  await expect(reduced.getByRole('button', { name: 'Mesas', exact: true })).toBeVisible();
  await expect(reduced.getByRole('button', { name: /^Caja · / })).toBeVisible();
  await expect(reduced.getByRole('button', { name: /^Menú del usuario/ })).toBeVisible();
  await expect(reduced.getByRole('button', { name: 'Administración' })).toHaveCount(0);
  const bill = page.getByRole('complementary', { name: 'Cuenta actual' });
  await expect(bill.getByRole('button', { name: /^Enviar a cocina/ })).toBeVisible();
  await expect(bill.getByRole('button', { name: /^Cobrar/ })).toBeVisible();
  await checkViews(page, testInfo, 'capture-reduced', reduced);
  await reduced.getByRole('button', { name: 'Mesas', exact: true }).click();
  await expect(page.locator('.app-topbar')).toBeVisible();
  await expect(page.getByRole('button', { name: /^Mesa 9C, Libre/ })).toBeVisible();
  expect(errors).toEqual([]);
});
