import { randomBytes, randomUUID } from 'node:crypto';
import { expect, test } from '@playwright/test';
import type { Locator, Page } from '@playwright/test';
import { createStation } from './kitchen-coverage-flow';
import { admin, call, dismissNotices, lightTheme, login, shot } from './license-flow';
import type { Driver } from './license-flow';
import { installDriver } from './native-driver';
import { openSection } from './navigation';
import { chooseOption } from './select';

// Etapa M1, Fase B (§5.1, §5.2 y §5.4 de la especificación; M1-D15, M1-D18 y M1-D34), con Go y PostgreSQL reales:
//   1. La etiqueta de cocina de cada producto sigue a Go en el panel de la mesa y en la captura: «En cocina» al
//      enviarse, «Listo» después de marcarlo listo en Preparación del escritorio (el caso de la cuenta #26 de la Mesa 1
//      del propietario, que se quedaba en «En cocina»).
//   2. Un refresco sin ruta («No necesitan preparación») no lleva ninguna etiqueta de cocina, en ningún lado.
//   3. Preparación en lenguaje sencillo: la tarjeta liberada dice «Por preparar» y el filtro vacío «No hay pedidos con
//      este filtro.».
// Los datos llevan un sufijo propio para convivir con la base compartida. Capturas en tema claro.

async function openWithItems(driver: Driver, tableID: string, productIDs: string[]) {
  const opened = await call(driver, 'POST', 'restaurant/accounts', {
    operation_id: randomUUID(),
    mode: 'dine_in',
    table_id: tableID,
    items: productIDs.map((productID) => ({
      id: randomUUID(),
      product_id: productID,
      variant_id: null,
      quantity: 1,
      customization: { ingredients: [], groups: [], components: [], notes: '' },
    })),
  });
  const account = opened.account as { id: string; folio: number; revision: number };
  const route = `restaurant/accounts/${account.id}`;
  const preview = await call(driver, 'GET', `${route}/preview`);
  await call(driver, 'POST', `${route}/confirm`, {
    operation_id: randomUUID(),
    revision: account.revision,
    expected_fingerprint: preview.fingerprint,
  });
  return account;
}

function tagOf(container: Locator, name: string) {
  return container.locator('li, article').filter({ hasText: name }).locator('.capture-tag');
}

async function openTable(page: Page, area: string, table: string) {
  await openSection(page, 'Mesas');
  await page
    .getByRole('group', { name: 'Áreas del restaurante' })
    .getByRole('button', { name: area, exact: true })
    .click();
  await page
    .locator('.table-canvas[aria-label="Mapa de mesas"]')
    .getByRole('button', { name: new RegExp(`^${table}, `) })
    .click();
  return page.getByRole('complementary', { name: `Panel de ${table}` });
}

test('etiquetas de cocina por producto y Preparación en lenguaje sencillo (etapa M1, §5.1, §5.2 y §5.4)', async ({
  page,
}, testInfo) => {
  test.setTimeout(240000);
  const errors: string[] = [];
  page.on('pageerror', (error) => errors.push(error.message));
  const driver = await installDriver(page);
  // Etapa M1 (M1-D21): la prueba entra por la API antes que la página y después usa la sesión de la página.
  await call(driver, 'POST', 'auth/login', admin);
  await lightTheme(driver);
  await call(driver, 'PUT', 'terminal/preferences', { keyboard_mode: 'disabled' });
  const suffix = randomBytes(3).toString('hex');
  const areaName = `Salón L ${suffix}`;
  const tableName = `Mesa L${suffix}`;
  const station = await createStation(driver, `Plancha L ${suffix}`, 'kds');
  const category = (await call(driver, 'POST', 'catalog/categories', {
    name: `Comida L ${suffix}`,
    active: true,
    color: '#d64545',
  })) as { id: string };
  const dishName = `Hamburguesa L ${suffix}`;
  const drinkName = `Coca Cola L ${suffix}`;
  const dish = (await call(driver, 'POST', 'catalog/products', {
    name: dishName,
    active: true,
    category_id: category.id,
    code: `HAMB-L-${suffix}`,
    kind: 'product',
    base_price: '120',
  })) as { id: string };
  // El refresco no tiene ruta: con «Productos sin ruta» en «No necesitan preparación» (lo de siempre) no genera tarea.
  const drink = (await call(driver, 'POST', 'catalog/products', {
    name: drinkName,
    active: true,
    category_id: category.id,
    code: `COCA-L-${suffix}`,
    kind: 'product',
    base_price: '30',
  })) as { id: string };
  await call(driver, 'POST', 'kitchen/routes', {
    operation_id: randomUUID(),
    product_id: dish.id,
    mode: 'prepare',
    station_ids: [station.id],
  });
  const area = (await call(driver, 'POST', 'restaurant/areas', {
    operation_id: randomUUID(),
    name: areaName,
    active: true,
  })) as { id: string };
  const table = (await call(driver, 'POST', 'restaurant/tables', {
    operation_id: randomUUID(),
    area_id: area.id,
    name: tableName,
    shape: 'rectangle',
    x: 200,
    y: 160,
    width: 120,
    height: 100,
    seats: 4,
    active: true,
  })) as { id: string };
  const account = await openWithItems(driver, table.id, [dish.id, drink.id]);
  // Go ya separa lo que necesita preparación (§5.2): el refresco va sin tarea y la cuenta sí la necesita.
  await expect
    .poll(
      async () => {
        const bill = (await call(driver, 'GET', `restaurant/accounts/${account.id}`)) as {
          lines: { quote_line: { name: string }; preparation_required?: boolean }[];
        };
        return bill.lines
          .map((line) => `${line.quote_line.name}:${line.preparation_required}`)
          .sort()
          .join('|');
      },
      { timeout: 20000 },
    )
    .toBe([`${drinkName}:false`, `${dishName}:true`].sort().join('|'));
  await call(driver, 'POST', 'auth/logout');

  await page.setViewportSize({ width: 1280, height: 900 });
  await login(page);
  await expect(page.locator('.app-topbar')).toBeVisible();

  // 1. Recién enviado: la hamburguesa «En cocina» y el refresco sin etiqueta, en el panel de la mesa.
  let panel = await openTable(page, areaName, tableName);
  const lines = panel.getByRole('list', { name: 'Productos de la cuenta' });
  await expect(tagOf(lines, dishName)).toHaveText('En cocina');
  await expect(tagOf(lines, drinkName)).toHaveCount(0);

  // 2. Preparación: la tarjeta dice «Por preparar»; el filtro sin pedidos, «No hay pedidos con este filtro.».
  await openSection(page, 'Cocina');
  await page.getByRole('button', { name: 'Preparación', exact: true }).click();
  const card = page.locator('.kitchen-preparation-card').filter({ hasText: dishName });
  await expect(card).toHaveCount(1, { timeout: 15000 });
  await expect(card.locator('.kitchen-state-label')).toHaveText('Por preparar');
  await expect(page.getByText('Liberado a preparación')).toHaveCount(0);
  await expect(
    page.locator('.kitchen-preparation-card').filter({ hasText: drinkName }),
  ).toHaveCount(0);
  await card.scrollIntoViewIfNeeded();
  await dismissNotices(page);
  await shot(page, testInfo, 'm1-preparacion-por-preparar', { fullPage: false });
  await chooseOption(page.getByRole('combobox', { name: 'Avance de preparación' }), 'Listos');
  const ready = page.locator('.kitchen-preparation-card').filter({ hasText: dishName });
  await expect(ready).toHaveCount(0);
  await chooseOption(
    page.getByRole('combobox', { name: 'Estación de preparación' }),
    `Plancha L ${suffix}`,
  );
  await expect(page.getByText('No hay pedidos con este filtro.')).toBeVisible();
  await expect(page.getByText(/unidades de preparación/)).toHaveCount(0);
  await dismissNotices(page);
  await shot(page, testInfo, 'm1-preparacion-filtro-vacio', { fullPage: false });
  await chooseOption(
    page.getByRole('combobox', { name: 'Avance de preparación' }),
    'Pendientes y en preparación',
  );
  // 3. «Marcar producto listo» en Preparación del escritorio (§5.1: también falla con esa marca).
  await card.getByRole('button', { name: 'Marcar producto listo', exact: true }).click();
  await expect(card).toHaveCount(0);

  // 4. La etiqueta pasa a «Listo» en el panel de la mesa y en la captura; el refresco sigue sin etiqueta.
  panel = await openTable(page, areaName, tableName);
  await expect(tagOf(lines, dishName)).toHaveText('Listo', { timeout: 15000 });
  await expect(tagOf(lines, drinkName)).toHaveCount(0);
  await dismissNotices(page);
  await shot(page, testInfo, 'm1-panel-listo-sin-etiqueta', { fullPage: false });
  await panel.getByRole('button', { name: 'Capturar pedidos en esta mesa', exact: true }).click();
  const bill = page.getByRole('complementary', { name: 'Cuenta actual' });
  await expect(
    page.getByRole('heading', { name: `${tableName} · Cuenta #${account.folio}` }),
  ).toBeVisible();
  await expect(
    bill.getByRole('article', { name: `1 × ${dishName}` }).locator('.capture-tag'),
  ).toHaveText('Listo');
  await expect(
    bill.getByRole('article', { name: `1 × ${drinkName}` }).locator('.capture-tag'),
  ).toHaveCount(0);
  // Ya no se ve «En cocina» con el estado real en gris al lado.
  await expect(bill.getByText('En cocina')).toHaveCount(0);
  await dismissNotices(page);
  await shot(page, testInfo, 'm1-captura-listo-sin-etiqueta', { fullPage: false });
  await openSection(page, 'Mesas');
  expect(errors).toEqual([]);
});
