import { randomUUID } from 'node:crypto';
import { copyFileSync, mkdirSync } from 'node:fs';
import { join } from 'node:path';
import { expect, test } from '@playwright/test';
import type { Locator, Page, TestInfo } from '@playwright/test';
import { installDriver } from './native-driver';
import { openAdministration, openSection } from './navigation';
import { chooseOption, expectSelectValue, openSelect } from './select';

// Etapa 9C, parte D2: mapa de mesas más legible (mosaicos con icono, nombre grande, estado, tiempo y
// capacidad; panel lateral con la acción principal destacada; buscador de mesas y selector de área en
// el encabezado), selector de hora (§2.1) y «Apariencia predeterminada para usuarios nuevos» (§4.4).
// Mismo transporte que las demás especificaciones (Go temporal y base adel_pos_test_*); las áreas y
// las mesas se crean por la API con nombres propios. Vistas a 1280 × 720 y 1920 × 1080 en ambos
// temas sin desbordamiento horizontal, con capturas (copia opcional en ADEL_SHOTS_DIR).

type Driver = Awaited<ReturnType<typeof installDriver>>;

async function create(driver: Driver, route: string, body: Record<string, unknown>) {
  const response = await driver.request('POST', route, body);
  expect(response.status, `${route}: ${JSON.stringify(response.body)}`).toBe(200);
  return response.body as { id: string; revision: number };
}

async function table(
  driver: Driver,
  areaID: string,
  name: string,
  values: Record<string, unknown>,
) {
  return create(driver, 'restaurant/tables', {
    operation_id: randomUUID(),
    area_id: areaID,
    name,
    shape: 'rectangle',
    y: 60,
    width: 120,
    height: 100,
    seats: 4,
    active: true,
    ...values,
  });
}

const sizes = [
  { width: 1280, height: 720 },
  { width: 1920, height: 1080 },
];

// Recorre los dos tamaños y los dos temas: sin desbordamiento horizontal, la comprobación propia de
// la vista y una captura por combinación.
async function views(
  page: Page,
  testInfo: TestInfo,
  name: string,
  check?: (size: { width: number; height: number }) => Promise<void>,
) {
  for (const size of sizes) {
    await page.setViewportSize(size);
    for (const theme of ['light', 'dark']) {
      await page.evaluate((value) => {
        document.documentElement.dataset.theme = value;
      }, theme);
      await page.waitForTimeout(250);
      const overflow = await page.evaluate(
        () => document.documentElement.scrollWidth - window.innerWidth,
      );
      expect(overflow, `${name} se desborda a ${size.width} × ${size.height} (${theme})`).toBe(0);
      await check?.(size);
      const file = `${name}-${theme}-${size.width}x${size.height}.png`;
      const path = testInfo.outputPath(file);
      // La aplicación vuelve a aplicar el tema del usuario cuando se actualiza su sesión: la captura
      // se repite si el tema cambió mientras se tomaba.
      for (let attempt = 0; attempt < 3; attempt += 1) {
        await page.evaluate((value) => {
          document.documentElement.dataset.theme = value;
        }, theme);
        await page.waitForTimeout(250);
        await page.screenshot({ path });
        const kept = await page.evaluate(() => document.documentElement.dataset.theme);
        if (kept === theme) {
          break;
        }
      }
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
  await page.evaluate(() => {
    document.documentElement.dataset.theme = 'light';
  });
  await page.setViewportSize({ width: 1280, height: 720 });
}

// Cada nombre de mesa se ve completo dentro de su mosaico y con letra de al menos 13 px.
async function namesFit(canvas: Locator) {
  const tiles = canvas.locator('.restaurant-table');
  const count = await tiles.count();
  expect(count).toBeGreaterThan(0);
  for (let index = 0; index < count; index += 1) {
    const tile = tiles.nth(index);
    const name = tile.locator('.table-tile-name strong');
    const outer = (await tile.boundingBox())!;
    const inner = (await name.boundingBox())!;
    const label = await tile.getAttribute('aria-label');
    expect(inner.x, `${label}: nombre recortado`).toBeGreaterThanOrEqual(outer.x - 0.5);
    expect(inner.y, `${label}: nombre recortado`).toBeGreaterThanOrEqual(outer.y - 0.5);
    expect(inner.x + inner.width, `${label}: nombre recortado`).toBeLessThanOrEqual(
      outer.x + outer.width + 0.5,
    );
    expect(inner.y + inner.height, `${label}: nombre recortado`).toBeLessThanOrEqual(
      outer.y + outer.height + 0.5,
    );
    const size = await name.evaluate((element) => parseFloat(getComputedStyle(element).fontSize));
    expect(size, `${label}: nombre pequeño`).toBeGreaterThanOrEqual(13);
  }
}

test('mesas, selector de hora y apariencia 9C (parte D2)', async ({ page }, testInfo) => {
  test.setTimeout(240000);
  const errors: string[] = [];
  page.on('pageerror', (error) => errors.push(error.message));
  const driver = await installDriver(page);
  await driver.request('POST', 'auth/login', { username: 'admin', password: 'admin' });
  const keyboard = await driver.request('PUT', 'terminal/preferences', {
    keyboard_mode: 'disabled',
  });
  expect(keyboard.status, JSON.stringify(keyboard.body)).toBeLessThan(300);
  const patio = await create(driver, 'restaurant/areas', {
    operation_id: randomUUID(),
    name: 'Patio D2',
    active: true,
  });
  const garden = await create(driver, 'restaurant/areas', {
    operation_id: randomUUID(),
    name: 'Jardín D2',
    active: true,
  });
  await table(driver, patio.id, 'Mesa 61', {
    shape: 'round',
    x: 40,
    width: 100,
    height: 100,
    seats: 2,
  });
  const occupied = await table(driver, patio.id, 'Mesa 62', { x: 200 });
  const blocked = await table(driver, patio.id, 'Mesa 63', {
    x: 380,
    width: 100,
    height: 72,
    seats: 6,
  });
  await table(driver, patio.id, 'Barra 4', { x: 540, y: 300, width: 160, height: 60 });
  await table(driver, garden.id, 'Mesa 71', { x: 40 });
  // Mesa ocupada: una cuenta abierta (vacía) creada por la API de prueba.
  await create(driver, 'restaurant/accounts', {
    operation_id: randomUUID(),
    mode: 'dine_in',
    table_id: occupied.id,
  });
  const service = await driver.request('POST', `restaurant/tables/${blocked.id}/service`, {
    operation_id: randomUUID(),
    revision: blocked.revision,
    out_of_service: true,
  });
  expect(service.status, JSON.stringify(service.body)).toBe(200);
  await driver.request('POST', 'auth/logout');
  await page.setViewportSize({ width: 1280, height: 720 });
  await page.goto('/');
  await page.getByLabel('Usuario', { exact: true }).fill('admin');
  await page.getByLabel('Contraseña', { exact: true }).fill('admin');
  await page.getByRole('button', { name: 'Entrar', exact: true }).click();

  // 1. Encabezado del mapa: selector de área y buscador de mesas.
  await openSection(page, 'Mesas');
  const areas = page.getByRole('group', { name: 'Áreas del restaurante' });
  await areas.getByRole('button', { name: 'Patio D2', exact: true }).click();
  await expect(areas.getByRole('button', { name: 'Patio D2', exact: true })).toHaveAttribute(
    'aria-pressed',
    'true',
  );
  const search = page.getByRole('searchbox', { name: 'Buscar mesa por nombre, número o área' });
  await expect(search).toBeVisible();
  for (const button of await areas.getByRole('button').all()) {
    expect((await button.boundingBox())!.height).toBeGreaterThanOrEqual(44);
  }
  expect((await search.boundingBox())!.height).toBeGreaterThanOrEqual(44);

  // 2. Mosaicos: icono de mesa, nombre grande, estado en texto, tiempo ocupada y capacidad.
  const canvas = page.locator('.table-canvas[aria-label="Mapa de mesas"]');
  const busy = canvas.getByRole('button', {
    name: /^Mesa 62, Ocupada, 1 cuenta, \d+ min, mesa para 4$/,
  });
  await expect(busy).toBeVisible();
  await expect(busy.locator('.table-tile-icon')).toBeVisible();
  await expect(busy.locator('.table-tile-state')).toHaveText('Ocupada');
  await expect(busy.locator('.table-tile-time')).toHaveText(/^\d+ min$/);
  await expect(busy.locator('.table-tile-seats')).toHaveText('4');
  await expect(
    canvas.getByRole('button', { name: 'Mesa 61, Libre, 0 cuentas, mesa para 2' }),
  ).toBeVisible();
  const outOfService = canvas.getByRole('button', {
    name: 'Mesa 63, Fuera de servicio, 0 cuentas',
  });
  await expect(outOfService.locator('.table-tile-state')).toHaveText('Fuera de servicio');
  // La mesa redonda sigue siendo un círculo.
  const round = (await canvas.getByRole('button', { name: /^Mesa 61, / }).boundingBox())!;
  expect(Math.abs(round.width - round.height)).toBeLessThanOrEqual(1.5);
  await expect(page.getByRole('list', { name: 'Datos de cada mesa' })).toContainText(
    'Capacidad (lugares)',
  );

  // 3. Panel lateral de la mesa ocupada: las mismas acciones, iconos a la izquierda y «Capturar»
  // como acción principal.
  await busy.click();
  const panel = page.getByRole('complementary', { name: 'Panel de Mesa 62' });
  await expect(panel.getByRole('heading', { name: 'Mesa 62' })).toBeVisible();
  const facts = panel.getByRole('list', { name: 'Datos de la mesa' });
  await expect(facts).toContainText('Mesa para 4');
  await expect(facts).toContainText(/Abierta hace \d+ min/);
  const capture = panel.getByRole('button', { name: 'Capturar', exact: true });
  await expect(capture).toHaveClass(/primary-button/);
  await expect(capture.locator('svg')).toHaveCount(1);
  for (const label of ['Precuenta', 'Liberar mesa']) {
    await expect(
      panel.getByRole('button', { name: label, exact: true }).locator('svg'),
    ).toHaveCount(1);
  }
  await expect(page.locator('[data-sonner-toast]')).toHaveCount(0, { timeout: 15000 });
  await views(page, testInfo, 'mapa-panel', async () => {
    await expect(panel).toBeVisible();
    await namesFit(canvas);
  });

  // 4. Buscador: encuentra mesas de otra área; elegir una cambia de área y abre su panel.
  await search.fill('71');
  const found = page.getByRole('group', { name: 'Mesas encontradas' });
  await expect(found.getByRole('button')).toHaveCount(1);
  await views(page, testInfo, 'mapa-buscador', async () => {
    await expect(found).toBeVisible();
  });
  await found.getByRole('button', { name: 'Mesa 71, Jardín D2, Libre', exact: true }).click();
  await expect(page.getByRole('complementary', { name: 'Panel de Mesa 71' })).toBeVisible();
  await expect(areas.getByRole('button', { name: 'Jardín D2', exact: true })).toHaveAttribute(
    'aria-pressed',
    'true',
  );
  await expect(search).toHaveValue('');
  await expect(canvas.getByRole('button', { name: /^Mesa 71, Libre/ })).toBeFocused();

  // 5. Configuración general: apariencia para usuarios nuevos y selector de hora.
  await openAdministration(page, 'Configuración');
  const appearance = page.getByRole('combobox', {
    name: 'Apariencia predeterminada para usuarios nuevos',
  });
  await expect(appearance).toBeVisible();
  await expect(page.getByText(/Cada usuario la cambia en Mis preferencias/)).toBeVisible();
  await expect(page.getByRole('combobox', { name: 'Zona horaria del restaurante' })).toBeVisible();
  await expect(page.getByText('Datos del establecimiento.', { exact: true })).toBeVisible();
  await views(page, testInfo, 'configuracion-general');
  const dayStart = page.getByRole('group', { name: 'Hora de inicio del día' });
  const hour = dayStart.getByRole('combobox', { name: 'Hora', exact: true });
  const minutes = dayStart.getByRole('combobox', { name: 'Minutos', exact: true });
  await expectSelectValue(hour, '05');
  await expectSelectValue(minutes, '00');
  await expect(dayStart.locator('input:not([aria-hidden="true"])')).toHaveCount(0);
  await dayStart.scrollIntoViewIfNeeded();
  const list = await openSelect(hour);
  await expect(list.getByRole('option')).toHaveCount(24);
  await views(page, testInfo, 'selector-hora', async () => {
    await expect(list).toBeVisible();
    const box = (await list.boundingBox())!;
    const viewport = page.viewportSize()!;
    expect(box.y + box.height).toBeLessThanOrEqual(viewport.height + 0.5);
  });
  await page.keyboard.press('Escape');
  await expect(hour).toHaveAttribute('aria-expanded', 'false');
  // Con el teclado: escribir 0 y 4 lleva a las 04; los minutos van de 5 en 5.
  await hour.focus();
  await page.keyboard.press('0');
  await page.keyboard.press('4');
  await page.keyboard.press('Enter');
  await expectSelectValue(hour, '04');
  await chooseOption(minutes, '30');
  await page.getByRole('button', { name: 'Guardar hora de inicio', exact: true }).click();
  await expect
    .poll(async () => (await driver.request('GET', 'settings/business-day')).body)
    .toMatchObject({ operating_day_cutoff: '04:30' });
  // Se restablece la hora de siempre para no alterar otras pruebas.
  await chooseOption(dayStart.getByRole('combobox', { name: 'Hora', exact: true }), '05');
  await chooseOption(dayStart.getByRole('combobox', { name: 'Minutos', exact: true }), '00');
  await page.getByRole('button', { name: 'Guardar hora de inicio', exact: true }).click();
  await expect
    .poll(async () => (await driver.request('GET', 'settings/business-day')).body)
    .toMatchObject({ operating_day_cutoff: '05:00' });

  // 6. Registro de movimientos: la variante con fecha propone 00:00 en «Desde» y 23:59 en «Hasta».
  await openAdministration(page, 'Registro de movimientos');
  const from = page.getByRole('group', { name: 'Desde', exact: true });
  await from.getByRole('textbox', { name: 'Fecha', exact: true }).fill('01112026');
  await expectSelectValue(from.getByRole('combobox', { name: 'Hora', exact: true }), '00');
  await expectSelectValue(from.getByRole('combobox', { name: 'Minutos', exact: true }), '00');
  const until = page.getByRole('group', { name: 'Hasta', exact: true });
  await until.getByRole('textbox', { name: 'Fecha', exact: true }).fill('30122026');
  await expectSelectValue(until.getByRole('combobox', { name: 'Hora', exact: true }), '23');
  await expectSelectValue(until.getByRole('combobox', { name: 'Minutos', exact: true }), '59');
  await page.getByRole('button', { name: 'Aplicar filtros', exact: true }).click();
  await expect(page.locator('[data-sonner-toast]')).toHaveCount(0, { timeout: 15000 });
  await views(page, testInfo, 'registro-filtros-fecha-hora');
  expect(errors).toEqual([]);
});
