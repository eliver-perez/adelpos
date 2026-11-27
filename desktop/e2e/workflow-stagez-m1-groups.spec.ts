import { randomBytes, randomUUID } from 'node:crypto';
import { expect, test } from '@playwright/test';
import type { Page } from '@playwright/test';
import { admin, call, dismissNotices, lightTheme, login, shot } from './license-flow';
import type { Driver } from './license-flow';
import { installDriver } from './native-driver';
import { openSection } from './navigation';
import { notice } from './notices';
import { chooseOptions } from './select';

// Etapa M1, Fase B (§3 de la especificación; M1-D4 y contrato §10), con Go y PostgreSQL reales:
//   1. Áreas y distribución: crear una mesa divisible («Mesa 3» con 3A, 3B y 3C) y los rechazos de Go tal cual (un
//      nombre que ya usa una mesa del área y una sección que ya es de otro grupo).
//   2. El mapa: el marco tenue del grupo con su nombre; cada sección se sigue tocando.
//   3. El atajo «Unir Mesa 3» con una sección ocupada: el texto de Go y no se puede unir.
//   4. Con las secciones libres, el atajo une las tres en la pantalla y la unión nace con el primer producto, con el
//      nombre del grupo.
//   5. Un grupo con mesas unidas no se quita (el texto de Go); ya libre, se quita y sus mesas siguen como mesas normales.
// Los datos llevan un sufijo propio para convivir con la base compartida. Capturas en tema claro.

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

// Descarta una cuenta con solo productos sin enviar (deja la mesa libre; con la unión sin cuentas, la deshace).
async function discard(driver: Driver, id: string) {
  const current = (await call(driver, 'GET', `restaurant/accounts/${id}`)).account as {
    revision: number;
  };
  await call(driver, 'POST', `restaurant/accounts/${id}/discard`, {
    operation_id: randomUUID(),
    revision: current.revision,
  });
}

async function overviewOf(driver: Driver, areaID: string) {
  const overview = (await call(driver, 'GET', 'restaurant/overview')) as {
    unions?: { id: string; name: string; area_id: string; group_id: string | null }[];
    groups?: { id: string; name: string; area_id: string; table_ids: string[] }[];
  };
  return {
    unions: (overview.unions ?? []).filter((union) => union.area_id === areaID),
    groups: (overview.groups ?? []).filter((group) => group.area_id === areaID),
  };
}

function mapOf(page: Page) {
  return page.locator('.table-canvas[aria-label="Mapa de mesas"]');
}

async function showArea(page: Page, area: string) {
  await openSection(page, 'Mesas');
  await page
    .getByRole('group', { name: 'Vistas operativas' })
    .getByRole('button', { name: 'Mesas', exact: true })
    .click();
  await page
    .getByRole('group', { name: 'Áreas del restaurante' })
    .getByRole('button', { name: area, exact: true })
    .click();
}

async function openGroups(page: Page, area: string) {
  await openSection(page, 'Mesas');
  await page
    .getByRole('group', { name: 'Vistas operativas' })
    .getByRole('button', { name: 'Áreas y distribución', exact: true })
    .click();
  await page
    .getByRole('group', { name: 'Áreas del restaurante' })
    .getByRole('button', { name: area, exact: true })
    .click();
  const section = page.getByRole('button', { name: /^Mesas divisibles/ });
  if ((await section.getAttribute('aria-expanded')) !== 'true') {
    await section.click();
  }
}

async function createGroup(page: Page, name: string, sections: string[]) {
  await page.getByRole('button', { name: 'Crear grupo', exact: true }).click();
  await page.getByLabel('Nombre del grupo', { exact: true }).fill(name);
  await chooseOptions(page.getByRole('combobox', { name: 'Secciones, en orden' }), sections);
  await page.getByRole('button', { name: 'Guardar grupo', exact: true }).click();
}

test('mesas divisibles: grupo en Áreas y distribución, marco en el mapa y atajo «Unir Mesa 3» (etapa M1, §3)', async ({
  page,
}, testInfo) => {
  test.setTimeout(300000);
  const errors: string[] = [];
  page.on('pageerror', (error) => errors.push(error.message));
  const driver = await installDriver(page);
  // Etapa M1 (M1-D21): la prueba entra por la API antes que la página y después usa la sesión de la página.
  await call(driver, 'POST', 'auth/login', admin);
  await lightTheme(driver);
  await call(driver, 'PUT', 'terminal/preferences', { keyboard_mode: 'disabled' });
  const suffix = randomBytes(3).toString('hex');
  const areaName = `Patio G ${suffix}`;
  const categoryName = `Postres G ${suffix}`;
  const productName = `Flan G ${suffix}`;
  const category = (await call(driver, 'POST', 'catalog/categories', {
    name: categoryName,
    active: true,
    color: '#d64545',
  })) as { id: string };
  const product = (await call(driver, 'POST', 'catalog/products', {
    name: productName,
    active: true,
    category_id: category.id,
    code: `FLAN-G-${suffix}`,
    kind: 'product',
    base_price: '45',
  })) as { id: string };
  const area = (await call(driver, 'POST', 'restaurant/areas', {
    operation_id: randomUUID(),
    name: areaName,
    active: true,
  })) as { id: string };
  const ids: Record<string, string> = {};
  // La mesa en U: tres secciones juntas; aparte, la Mesa 4 y otras dos mesas sueltas.
  const layout: [string, number, number][] = [
    ['3A', 80, 80],
    ['3B', 200, 80],
    ['3C', 320, 80],
    ['Mesa 4', 560, 80],
    ['5A', 80, 320],
    ['5B', 200, 320],
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
    })) as { id: string };
    ids[name] = created.id;
  }
  await call(driver, 'POST', 'auth/logout');

  await page.setViewportSize({ width: 1280, height: 900 });
  await login(page);
  await expect(page.locator('.app-topbar')).toBeVisible();

  // 1. Crear «Mesa 3» con 3A, 3B y 3C, en ese orden; sus secciones no cambian de nombre.
  await openGroups(page, areaName);
  await expect(page.getByText('Esta área no tiene mesas divisibles.')).toBeVisible();
  await createGroup(page, 'Mesa 3', ['3A', '3B', '3C']);
  await expect(notice(page, 'Grupo guardado.')).toBeVisible();
  const list = page.getByRole('list', { name: 'Mesas divisibles del área' });
  await expect(list).toContainText('Mesa 3 · 3A, 3B, 3C · 12 lugares');
  await expect
    .poll(async () => (await overviewOf(driver, area.id)).groups.map((group) => group.name))
    .toEqual(['Mesa 3']);
  expect((await overviewOf(driver, area.id)).groups[0]!.table_ids).toEqual([
    ids['3A'],
    ids['3B'],
    ids['3C'],
  ]);
  // El diseñador también dibuja el grupo junto.
  await expect(
    page.locator(
      '.table-canvas[aria-label="Diseñador de distribución de mesas"] .table-group-frame',
    ),
  ).toContainText('Mesa 3');
  await dismissNotices(page);
  await list.scrollIntoViewIfNeeded();
  await shot(page, testInfo, 'm1-grupo-areas-y-distribucion', { fullPage: false });

  // Los rechazos de Go, tal cual: el nombre de una mesa del área y una sección que ya es de otro grupo.
  await createGroup(page, 'Mesa 4', ['5A', '5B']);
  await expect(
    notice(page, 'Ya hay una mesa o un grupo con ese nombre en esta área.'),
  ).toBeVisible();
  await dismissNotices(page);
  await page.getByLabel('Nombre del grupo', { exact: true }).fill('Mesa 9');
  await chooseOptions(page.getByRole('combobox', { name: 'Secciones, en orden' }), ['3A']);
  await page.getByRole('button', { name: 'Guardar grupo', exact: true }).click();
  await expect(notice(page, /ya es parte del grupo Mesa 3/)).toBeVisible();
  await dismissNotices(page);
  expect((await overviewOf(driver, area.id)).groups.map((group) => group.name)).toEqual(['Mesa 3']);

  // 2. En el mapa, el marco tenue del grupo con su nombre; cada sección se sigue tocando.
  await showArea(page, areaName);
  const frame = mapOf(page).locator('.table-group-frame');
  await expect(frame).toHaveCount(1);
  await expect(frame.getByRole('button', { name: 'Unir Mesa 3', exact: true })).toBeVisible();
  await mapOf(page).getByRole('button', { name: /^3B, / }).click();
  await expect(page.getByRole('complementary', { name: 'Panel de 3B' })).toBeVisible();

  // 3. Con 3B ocupada, el atajo no une: el texto de Go lo explica.
  const busy = await openAccount(driver, ids['3B']!, product.id);
  await expect(mapOf(page).getByRole('button', { name: /^3B, Ocupada/ })).toBeVisible({
    timeout: 15000,
  });
  await frame.getByRole('button', { name: 'Unir Mesa 3', exact: true }).click();
  let dialog = page.getByRole('dialog', { name: 'Unir Mesa 3' });
  await expect(dialog).toContainText(
    'La sección 3B está ocupada. Únela con las secciones libres para crecer.',
  );
  await expect(dialog.getByRole('button', { name: 'Unir', exact: true })).toBeDisabled();
  await shot(page, testInfo, 'm1-atajo-grupo-seccion-ocupada', { fullPage: false });
  await dialog.getByRole('button', { name: 'Cancelar', exact: true }).click();
  await discard(driver, busy.id);
  await expect(mapOf(page).getByRole('button', { name: /^3B, Libre/ })).toBeVisible({
    timeout: 15000,
  });
  await page.getByRole('complementary', { name: /^Panel de / }).press('Escape');
  await dismissNotices(page);
  await shot(page, testInfo, 'm1-mapa-grupo', { fullPage: false });

  // 4. Con todo libre, «Unir Mesa 3» desde el panel de 3A: la unión vive en la pantalla y nace con el primer producto,
  // con el nombre del grupo.
  await mapOf(page).getByRole('button', { name: /^3A, / }).click();
  let panel = page.getByRole('complementary', { name: 'Panel de 3A' });
  await panel.getByRole('button', { name: 'Unir Mesa 3', exact: true }).click();
  dialog = page.getByRole('dialog', { name: 'Unir Mesa 3' });
  await expect(dialog.locator('.join-tables-name')).toHaveText('Mesa 3 · 12 lugares');
  await dialog.getByRole('button', { name: 'Unir', exact: true }).click();
  panel = page.getByRole('complementary', { name: 'Panel de Mesa 3' });
  await expect(panel).toContainText('La unión se guarda con el primer producto');
  await panel.getByRole('button', { name: 'Capturar pedidos en esta mesa', exact: true }).click();
  await page
    .getByRole('navigation', { name: 'Categorías' })
    .getByRole('button', { name: new RegExp(categoryName) })
    .click();
  await page.getByRole('button', { name: new RegExp(`^${productName}`) }).click();
  await expect(page.getByRole('heading', { name: /^Mesa 3 · Cuenta #\d+$/ })).toBeVisible({
    timeout: 15000,
  });
  await expect
    .poll(async () => (await overviewOf(driver, area.id)).unions.map((union) => union.name))
    .toEqual(['Mesa 3']);
  const [joined] = (await overviewOf(driver, area.id)).unions;
  await page.locator('.capture-topbar-back').click();
  await expect(mapOf(page).locator(`[data-union-id="${joined!.id}"]`)).toContainText(
    'Mesa 3 · 12 lugares',
  );
  // Con una sección unida, el marco ya no ofrece el atajo.
  await expect(frame.getByRole('button', { name: 'Unir Mesa 3', exact: true })).toHaveCount(0);
  await dismissNotices(page);
  await shot(page, testInfo, 'm1-mapa-grupo-unido', { fullPage: false });

  // 5. Un grupo con mesas unidas no se quita: el texto de Go.
  await openGroups(page, areaName);
  await list.getByRole('button', { name: 'Quitar Mesa 3', exact: true }).click();
  let confirmRemove = page.getByRole('alertdialog', { name: '¿Quitar Mesa 3?' });
  await confirmRemove.getByRole('button', { name: 'Quitar grupo', exact: true }).click();
  await expect(
    notice(page, 'El grupo tiene mesas unidas. Cámbialo cuando se liberen.'),
  ).toBeVisible();
  await dismissNotices(page);
  // Ya libre (la cuenta se descarta y la unión se deshace sola), el grupo se quita.
  const account = (
    (await call(driver, 'GET', `restaurant/accounts?table_id=${ids['3A']}&state=open`)) as {
      items: { id: string }[];
    }
  ).items[0]!;
  await discard(driver, account.id);
  await expect.poll(async () => (await overviewOf(driver, area.id)).unions).toEqual([]);
  await list.getByRole('button', { name: 'Quitar Mesa 3', exact: true }).click();
  confirmRemove = page.getByRole('alertdialog', { name: '¿Quitar Mesa 3?' });
  await confirmRemove.getByRole('button', { name: 'Quitar grupo', exact: true }).click();
  await expect(notice(page, 'Mesa 3 quitado. Sus mesas siguen como mesas normales.')).toBeVisible();
  await expect(page.getByText('Esta área no tiene mesas divisibles.')).toBeVisible();
  await expect.poll(async () => (await overviewOf(driver, area.id)).groups).toEqual([]);
  // Las secciones siguen como mesas normales, con sus nombres.
  await showArea(page, areaName);
  await expect(mapOf(page).locator('.table-group-frame')).toHaveCount(0);
  for (const name of ['3A', '3B', '3C']) {
    await expect(
      mapOf(page).getByRole('button', { name: new RegExp(`^${name}, Libre`) }),
    ).toBeVisible();
  }
  expect(errors).toEqual([]);
});
