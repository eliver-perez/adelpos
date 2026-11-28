import { randomBytes, randomUUID } from 'node:crypto';
import { expect, test } from '@playwright/test';
import type { Locator, Page } from '@playwright/test';
import { admin, call, dismissNotices, lightTheme, login, shot } from './license-flow';
import type { Driver } from './license-flow';
import { installDriver } from './native-driver';
import { openSection } from './navigation';
import { notice } from './notices';

// Etapa M1, Fase B (§1 y §2.3 de la especificación; contrato §10), con Go y PostgreSQL reales:
//   1. «Unir con…» desde una mesa libre: la vista previa de Go (nombre y capacidad), las mesas ocupadas y fuera de
//      servicio deshabilitadas con su motivo, y la unión que vive en la pantalla (contorno discontinuo): salir sin
//      capturar no guarda nada y la unión nace con el primer producto (M1-D9).
//   2. El contorno sólido con el nombre y la capacidad de Go, tocar cualquier mesa unida abre la unión y en la lista de
//      mesas encontradas la unión es una sola entrada.
//   3. «Agregar mesa» y «Quitar mesa».
//   4. «Unir con…» desde una mesa ocupada: la unión nace al momento con su cuenta.
//   5. Los rechazos con el texto de Go: la unión en pantalla que al guardar encuentra una mesa ocupada (no guarda
//      nada, conserva lo capturado y ofrece enviar solo a la mesa de inicio) y unir dos mesas ocupadas.
// Los datos llevan un sufijo propio para convivir con la base compartida. Capturas en tema claro.

interface ServerUnion {
  id: string;
  name: string;
  area_id: string;
  table_ids: string[];
  seats: number;
  account_count: number;
}

async function openAccount(driver: Driver, tableID: string, productID: string) {
  const opened = await call(driver, 'POST', 'restaurant/accounts', {
    operation_id: randomUUID(),
    mode: 'dine_in',
    table_id: tableID,
    items: [
      {
        id: randomUUID(),
        product_id: productID,
        variant_id: null,
        quantity: 1,
        customization: { ingredients: [], groups: [], components: [], notes: '' },
      },
    ],
  });
  return opened.account as { id: string; folio: number; revision: number };
}

async function unionsOf(driver: Driver, areaID: string) {
  const overview = (await call(driver, 'GET', 'restaurant/overview')) as { unions?: ServerUnion[] };
  return (overview.unions ?? []).filter((union) => union.area_id === areaID);
}

function mapOf(page: Page) {
  return page.locator('.table-canvas[aria-label="Mapa de mesas"]');
}

function tableButton(page: Page, name: string) {
  return mapOf(page).getByRole('button', { name: new RegExp(`^${name}, `) });
}

async function showArea(page: Page, area: string) {
  await openSection(page, 'Mesas');
  await page
    .getByRole('group', { name: 'Áreas del restaurante' })
    .getByRole('button', { name: area, exact: true })
    .click();
}

function joinDialog(page: Page, title: string) {
  return page.getByRole('dialog', { name: title });
}

function option(dialog: Locator, table: string) {
  return dialog.locator('.join-table-option').filter({ hasText: new RegExp(`^${table}`) });
}

async function addProduct(page: Page, category: string, product: string) {
  await page
    .getByRole('navigation', { name: 'Categorías' })
    .getByRole('button', { name: new RegExp(category) })
    .click();
  await page.getByRole('button', { name: new RegExp(`^${product}`) }).click();
}

test('unir mesas en el escritorio: diferida, al momento, contorno, agregar, quitar y rechazos (etapa M1, §1)', async ({
  page,
}, testInfo) => {
  test.setTimeout(360000);
  const errors: string[] = [];
  page.on('pageerror', (error) => errors.push(error.message));
  const driver = await installDriver(page);
  // Etapa M1 (M1-D21): la prueba entra por la API antes que la página y después usa la sesión de la página.
  await call(driver, 'POST', 'auth/login', admin);
  await lightTheme(driver);
  await call(driver, 'PUT', 'terminal/preferences', { keyboard_mode: 'disabled' });
  const suffix = randomBytes(3).toString('hex');
  const areaName = `Terraza U ${suffix}`;
  const categoryName = `Bebidas U ${suffix}`;
  const productName = `Limonada U ${suffix}`;
  const category = (await call(driver, 'POST', 'catalog/categories', {
    name: categoryName,
    active: true,
    color: '#d64545',
  })) as { id: string };
  const product = (await call(driver, 'POST', 'catalog/products', {
    name: productName,
    active: true,
    category_id: category.id,
    code: `LIM-U-${suffix}`,
    kind: 'product',
    base_price: '40',
  })) as { id: string };
  const area = (await call(driver, 'POST', 'restaurant/areas', {
    operation_id: randomUUID(),
    name: areaName,
    active: true,
  })) as { id: string };
  const ids: Record<string, string> = {};
  const layout: [string, number, number][] = [
    ['Mesa 51', 60, 60],
    ['Mesa 52', 200, 60],
    ['Mesa 53', 340, 60],
    ['Mesa 54', 480, 60],
    ['Mesa 55', 60, 280],
    ['Mesa 56', 200, 280],
    ['Mesa 57', 340, 280],
    ['Mesa 58', 480, 280],
  ];
  for (const [name, x, y] of layout) {
    const created = (await call(driver, 'POST', 'restaurant/tables', {
      operation_id: randomUUID(),
      area_id: area.id,
      name,
      shape: 'rectangle',
      x,
      y,
      width: 110,
      height: 90,
      seats: 4,
      active: true,
    })) as { id: string; revision: number };
    ids[name] = created.id;
    if (name === 'Mesa 57') {
      await call(driver, 'POST', `restaurant/tables/${created.id}/service`, {
        operation_id: randomUUID(),
        revision: created.revision,
        out_of_service: true,
      });
    }
  }
  // Mesa 55 ocupada: desde ella la unión nace al momento y sus cuentas pasan a la unión.
  const occupied = await openAccount(driver, ids['Mesa 55']!, product.id);
  await call(driver, 'POST', 'auth/logout');

  await page.setViewportSize({ width: 1280, height: 900 });
  await login(page);
  await expect(page.locator('.app-topbar')).toBeVisible();
  await showArea(page, areaName);

  // 1. «Unir con…» desde la Mesa 51 (libre). Las mesas que no se pueden unir se ven deshabilitadas con su motivo.
  await tableButton(page, 'Mesa 51').click();
  let panel = page.getByRole('complementary', { name: 'Panel de Mesa 51' });
  await panel.getByRole('button', { name: 'Unir con…', exact: true }).click();
  let dialog = joinDialog(page, 'Unir Mesa 51 con…');
  await expect(option(dialog, 'Mesa 55')).toBeDisabled();
  await expect(option(dialog, 'Mesa 55')).toContainText('Ocupada');
  await expect(option(dialog, 'Mesa 57')).toBeDisabled();
  await expect(option(dialog, 'Mesa 57')).toContainText('Fuera de servicio o desactivada');
  await expect(dialog.getByRole('button', { name: 'Unir', exact: true })).toBeDisabled();
  await option(dialog, 'Mesa 52').click();
  await expect(option(dialog, 'Mesa 52')).toHaveAttribute('aria-pressed', 'true');
  // El nombre y la capacidad los da Go (M1-D2): «Mesa» una sola vez y la suma de los lugares.
  await expect(dialog.locator('.join-tables-name')).toHaveText('Mesa 51 + 52 · 8 lugares');
  await dismissNotices(page);
  await shot(page, testInfo, 'm1-unir-con-vista-previa', { fullPage: false });
  await dialog.getByRole('button', { name: 'Unir', exact: true }).click();
  await expect(dialog).toHaveCount(0);
  // La unión vive en la pantalla: su panel y su contorno discontinuo, sin nada en Go.
  panel = page.getByRole('complementary', { name: 'Panel de Mesa 51 + 52' });
  await expect(panel).toContainText('Mesa para 8');
  await expect(panel).toContainText('La unión se guarda con el primer producto');
  await expect(mapOf(page).locator('.table-union-screen')).toContainText(
    'Mesa 51 + 52 · 8 lugares · sin guardar',
  );
  expect(await unionsOf(driver, area.id)).toEqual([]);

  // Salir de la captura sin productos no guarda nada.
  await panel.getByRole('button', { name: 'Capturar pedidos en esta mesa', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Mesa 51 + 52 · Cuenta nueva' })).toBeVisible();
  await expect(page.locator('.capture-topbar')).toContainText(
    'La unión y la cuenta se guardan con el primer producto',
  );
  await page.locator('.capture-topbar-back').click();
  await expect(panel).toBeVisible();
  await page.waitForTimeout(1000);
  expect(await unionsOf(driver, area.id)).toEqual([]);

  // Con el primer producto nacen la unión y la cuenta.
  await panel.getByRole('button', { name: 'Capturar pedidos en esta mesa', exact: true }).click();
  await addProduct(page, categoryName, productName);
  const joinedHeading = page.getByRole('heading', { name: /^Mesa 51 \+ 52 · Cuenta #\d+$/ });
  await expect(joinedHeading).toBeVisible({ timeout: 15000 });
  const joinedFolio = Number(/#(\d+)/.exec((await joinedHeading.textContent()) ?? '')?.[1]);
  await expect
    .poll(async () => (await unionsOf(driver, area.id)).map((union) => union.name))
    .toEqual(['Mesa 51 + 52']);
  const [joined] = await unionsOf(driver, area.id);
  expect(joined!.table_ids).toEqual([ids['Mesa 51'], ids['Mesa 52']]);
  expect(joined!.seats).toBe(8);

  // 2. El contorno sólido con el nombre y la capacidad; tocar la otra mesa abre la unión.
  await page.locator('.capture-topbar-back').click();
  const outline = mapOf(page).locator(`[data-union-id="${joined!.id}"]`);
  await expect(outline).toContainText('Mesa 51 + 52 · 8 lugares');
  await expect(mapOf(page).locator('.table-union-screen')).toHaveCount(0);
  await page.getByRole('complementary', { name: /^Panel de / }).press('Escape');
  await tableButton(page, 'Mesa 52').click();
  panel = page.getByRole('complementary', { name: 'Panel de Mesa 51 + 52' });
  await expect(panel).toContainText(/Cuenta #\d+/);
  await expect(tableButton(page, 'Mesa 51')).toHaveAccessibleName(
    /^Mesa 51, unida en Mesa 51 \+ 52, /,
  );
  await dismissNotices(page);
  await shot(page, testInfo, 'm1-mapa-union-y-panel', { fullPage: false });
  // En la lista de mesas encontradas, la unión es una sola entrada.
  await page.getByRole('searchbox', { name: 'Buscar mesa por nombre, número o área' }).fill(suffix);
  const found = page.getByRole('group', { name: 'Mesas encontradas' });
  await expect(found.getByRole('button', { name: /^Mesa 51 \+ 52, / })).toHaveCount(1);
  await expect(found.getByRole('button', { name: /^Mesa 51, / })).toHaveCount(0);
  await expect(found.getByRole('button', { name: /^Mesa 52, / })).toHaveCount(0);
  await page.getByRole('searchbox', { name: 'Buscar mesa por nombre, número o área' }).fill('');
  // En Cuentas, la cuenta de la unión dice el nombre de la unión (el escritorio arma ese lugar, M1-D14).
  const views = page.getByRole('group', { name: 'Vistas operativas' });
  await views.getByRole('button', { name: 'Cuentas', exact: true }).click();
  await page.getByLabel('Buscar cuenta o folio', { exact: true }).fill(String(joinedFolio));
  const card = page
    .locator('.account-list-card')
    .filter({ hasText: new RegExp(`^Cuenta #${joinedFolio}(?!\\d)`) });
  await expect(card.locator('.account-card-place')).toHaveText('Mesa 51 + 52');
  await views.getByRole('button', { name: 'Mesas', exact: true }).click();
  await tableButton(page, 'Mesa 52').click();
  await expect(panel).toContainText(`Cuenta #${joinedFolio}`);

  // 3. «Agregar mesa» (vista previa de Go) y «Quitar mesa» (ninguna cuenta sale de la unión).
  await panel.getByRole('button', { name: 'Agregar mesa', exact: true }).click();
  dialog = joinDialog(page, 'Agregar una mesa a Mesa 51 + 52');
  await option(dialog, 'Mesa 53').click();
  await expect(dialog.locator('.join-tables-name')).toHaveText('Mesa 51 + 52 + 53 · 12 lugares');
  await dialog.getByRole('button', { name: 'Agregar', exact: true }).click();
  await expect(notice(page, 'Se agregó Mesa 53: ahora Mesa 51 + 52 + 53.')).toBeVisible();
  panel = page.getByRole('complementary', { name: 'Panel de Mesa 51 + 52 + 53' });
  await expect(panel).toContainText('Mesa para 12');
  await panel.getByRole('button', { name: 'Quitar mesa', exact: true }).click();
  dialog = joinDialog(page, 'Quitar una mesa de Mesa 51 + 52 + 53');
  await expect(dialog).toContainText('ninguna cuenta sale de la unión');
  await option(dialog, 'Mesa 52').click();
  await dialog.getByRole('button', { name: 'Quitar', exact: true }).click();
  await expect(notice(page, 'Se quitó Mesa 52: ahora Mesa 51 + 53.')).toBeVisible();
  await expect
    .poll(async () => (await unionsOf(driver, area.id)).map((union) => union.name))
    .toEqual(['Mesa 51 + 53']);
  await expect(tableButton(page, 'Mesa 52')).toHaveAccessibleName(/^Mesa 52, Libre/);

  // 4. Desde la Mesa 55 (ocupada) la unión nace al momento con su cuenta.
  await tableButton(page, 'Mesa 55').click();
  panel = page.getByRole('complementary', { name: 'Panel de Mesa 55' });
  await panel.getByRole('button', { name: 'Unir con…', exact: true }).click();
  dialog = joinDialog(page, 'Unir Mesa 55 con…');
  await option(dialog, 'Mesa 56').click();
  await expect(dialog.locator('.join-tables-name')).toHaveText('Mesa 55 + 56 · 8 lugares');
  await dialog.getByRole('button', { name: 'Unir', exact: true }).click();
  await expect(notice(page, 'Mesas unidas: Mesa 55 + 56.')).toBeVisible();
  const grown = (await unionsOf(driver, area.id)).find((union) => union.name === 'Mesa 55 + 56');
  expect(grown?.account_count).toBe(1);
  panel = page.getByRole('complementary', { name: 'Panel de Mesa 55 + 56' });
  await expect(panel).toContainText(`Cuenta #${occupied.folio}`);

  // 5a. Una unión en pantalla que al guardar encuentra una mesa ocupada: Go no guarda nada y lo dice; lo capturado se
  // conserva y se puede enviar solo a la mesa de inicio.
  await tableButton(page, 'Mesa 54').click();
  panel = page.getByRole('complementary', { name: 'Panel de Mesa 54' });
  await panel.getByRole('button', { name: 'Unir con…', exact: true }).click();
  dialog = joinDialog(page, 'Unir Mesa 54 con…');
  await option(dialog, 'Mesa 52').click();
  await expect(dialog.locator('.join-tables-name')).toHaveText('Mesa 52 + 54 · 8 lugares');
  await dialog.getByRole('button', { name: 'Unir', exact: true }).click();
  panel = page.getByRole('complementary', { name: 'Panel de Mesa 52 + 54' });
  await panel.getByRole('button', { name: 'Capturar pedidos en esta mesa', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Mesa 52 + 54 · Cuenta nueva' })).toBeVisible();
  // Otra terminal abre una cuenta en la Mesa 52 mientras tanto.
  await openAccount(driver, ids['Mesa 52']!, product.id);
  await addProduct(page, categoryName, productName);
  const rejected = page.getByRole('alertdialog', { name: 'No se pudieron unir las mesas' });
  await expect(rejected).toContainText(
    'Solo se pueden unir mesas libres: la Mesa 52 ya está ocupada.',
    { timeout: 15000 },
  );
  await expect(rejected).toContainText('No se guardó nada.');
  expect((await unionsOf(driver, area.id)).map((union) => union.name).sort()).toEqual([
    'Mesa 51 + 53',
    'Mesa 55 + 56',
  ]);
  await dismissNotices(page);
  await shot(page, testInfo, 'm1-union-rechazada', { fullPage: false });
  await rejected.getByRole('button', { name: 'Enviar solo a la Mesa 54', exact: true }).click();
  await expect(page.getByRole('heading', { name: /^Mesa 54 · Cuenta #\d+$/ })).toBeVisible({
    timeout: 15000,
  });
  await expect(
    page.getByRole('complementary', { name: 'Cuenta actual' }).getByRole('article', {
      name: `1 × ${productName}`,
    }),
  ).toBeVisible();

  // 5b. Unir dos mesas ocupadas: el rechazo de Go en el diálogo, tal cual.
  await page.locator('.capture-topbar-back').click();
  panel = page.getByRole('complementary', { name: 'Panel de Mesa 54' });
  await panel.getByRole('button', { name: 'Unir con…', exact: true }).click();
  dialog = joinDialog(page, 'Unir Mesa 54 con…');
  await option(dialog, 'Mesa 58').click();
  await expect(dialog.locator('.join-tables-name')).toHaveText('Mesa 54 + 58 · 8 lugares');
  await openAccount(driver, ids['Mesa 58']!, product.id);
  await dialog.getByRole('button', { name: 'Unir', exact: true }).click();
  await expect(dialog.getByRole('alert')).toHaveText(
    'La Mesa 54 y la Mesa 58 tienen cuentas abiertas. Para juntar sus cuentas usa «Combinar cuentas».',
  );
  await dialog.getByRole('button', { name: 'Cancelar', exact: true }).click();
  await expect(dialog).toHaveCount(0);
  expect((await unionsOf(driver, area.id)).length).toBe(2);
  expect(errors).toEqual([]);
});
