import { randomUUID } from 'node:crypto';
import { copyFileSync, mkdirSync } from 'node:fs';
import { join } from 'node:path';
import { expect, test } from '@playwright/test';
import type { Locator, Page, TestInfo } from '@playwright/test';
import { installDriver } from './native-driver';
import { openSection } from './navigation';
import { notice } from './notices';

// Etapa 9D, fase D (§4): Para llevar y «Cuenta sin mesa» con creación diferida (salir sin productos
// no deja nada; con el primer producto la cuenta nace), «Descartar cuenta vacía» sin motivo en Mesas,
// Cuentas y Para llevar (no es cancelación ni aparece en «Cuentas canceladas»), tarjetas compactas,
// «Enviado · sin confirmar en cocina» una sola vez por producto y el menú por producto con
// «Cancelar unidades» en rojo. Mismo transporte que las demás especificaciones (Go temporal y base
// adel_pos_test_*), datos reales por la API. Capturas en tema claro y oscuro a 1280 × 720 y
// 1920 × 1080 (copia opcional en ADEL_SHOTS_DIR).

type Driver = Awaited<ReturnType<typeof installDriver>>;

async function call(driver: Driver, method: string, route: string, body?: unknown) {
  const response = await driver.request(method, route, body);
  expect(response.status, `${method} ${route}: ${JSON.stringify(response.body)}`).toBe(200);
  return response.body;
}

interface ServerAccount {
  id: string;
  folio: number;
  mode: string;
  state: string;
  table_id: string | null;
  preparation_state: string;
  discardable: boolean;
}

// Cuenta abierta con su primer producto (la apertura de la etapa 7.1) y, si se pide, enviada a
// cocina con la cotización vigente.
async function openWith(
  driver: Driver,
  body: Record<string, unknown>,
  productID: string,
  send: boolean,
): Promise<ServerAccount> {
  const opened = await call(driver, 'POST', 'restaurant/accounts', {
    operation_id: randomUUID(),
    ...body,
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
  const account = opened.account as ServerAccount & { revision: number };
  if (!send) {
    return account;
  }
  const route = `restaurant/accounts/${account.id}`;
  const preview = await call(driver, 'GET', `${route}/preview`);
  await call(driver, 'POST', `${route}/confirm`, {
    operation_id: randomUUID(),
    revision: account.revision,
    expected_fingerprint: preview.fingerprint,
  });
  return account;
}

async function readAccount(driver: Driver, id: string): Promise<ServerAccount> {
  return (await call(driver, 'GET', `restaurant/accounts/${id}`)).account as ServerAccount;
}

async function allAccounts(driver: Driver, mode: string): Promise<ServerAccount[]> {
  const page = await call(
    driver,
    'GET',
    `restaurant/accounts?mode=${mode}&state=all&page_size=100`,
  );
  return page.items as ServerAccount[];
}

const sizes = [
  { width: 1280, height: 720 },
  { width: 1920, height: 1080 },
];
const themes = ['light', 'dark'] as const;

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

// Recorre los dos tamaños y los dos temas: sin desbordamiento horizontal, la comprobación de la
// vista y una captura.
async function views(
  page: Page,
  testInfo: TestInfo,
  name: string,
  check?: (size: { width: number; height: number }) => Promise<void>,
) {
  for (const size of sizes) {
    await page.setViewportSize(size);
    for (const theme of themes) {
      const file = `${name}-${theme}-${size.width}x${size.height}.png`;
      for (let attempt = 0; attempt < 3; attempt += 1) {
        await page.evaluate((value) => {
          document.documentElement.dataset.theme = value;
        }, theme);
        await page.waitForTimeout(250);
        const overflow = await page.evaluate(
          () => document.documentElement.scrollWidth - window.innerWidth,
        );
        expect(overflow, `${name} se desborda a ${size.width} × ${size.height} (${theme})`).toBe(0);
        await check?.(size);
        await page.screenshot({ path: testInfo.outputPath(file) });
        if ((await page.evaluate(() => document.documentElement.dataset.theme)) === theme) {
          break;
        }
      }
      keep(testInfo, file);
    }
  }
  await page.evaluate(() => {
    document.documentElement.dataset.theme = 'light';
  });
  await page.setViewportSize({ width: 1280, height: 720 });
}

async function token(page: Page, name: string) {
  return page.evaluate((variable) => {
    const probe = document.createElement('span');
    probe.style.color = `var(${variable})`;
    document.body.appendChild(probe);
    const color = getComputedStyle(probe).color;
    probe.remove();
    return color;
  }, name);
}

async function computed(locator: Locator, property: string) {
  return locator.evaluate(
    (element, name) => getComputedStyle(element).getPropertyValue(name),
    property,
  );
}

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

// Tarjetas del listado que se ven completas dentro de su columna desplazable.
async function visibleCards(list: Locator) {
  return list.evaluate((root) => {
    let scroller: Element | null = root;
    while (scroller && !/(auto|scroll)/.test(getComputedStyle(scroller).overflowY)) {
      scroller = scroller.parentElement;
    }
    const frame = (scroller ?? document.documentElement).getBoundingClientRect();
    const bottom = Math.min(frame.bottom, window.innerHeight);
    return Array.from(root.querySelectorAll<HTMLElement>('.account-list-card')).filter((card) => {
      const box = card.getBoundingClientRect();
      return box.top >= frame.top - 1 && box.bottom <= bottom + 1;
    }).length;
  });
}

async function voidedFolios(driver: Driver) {
  const report = await call(driver, 'GET', 'reports/run/sales.voided_accounts?period=current_year');
  return ((report.rows ?? []) as Record<string, unknown>[]).map((row) => String(row.folio));
}

test('cuentas y Para llevar: creación diferida, descartar, tarjetas y menú por producto (etapa 9D, fase D)', async ({
  page,
}, testInfo) => {
  test.setTimeout(420000);
  const errors: string[] = [];
  page.on('pageerror', (error) => errors.push(error.message));
  const driver = await installDriver(page);
  await call(driver, 'POST', 'auth/login', { username: 'admin', password: 'admin' });
  await call(driver, 'PUT', 'terminal/preferences', { keyboard_mode: 'disabled' });

  // 1. Datos por la API. Un producto sin ruta de preparación: su liberación queda bloqueada y la
  // cuenta muestra «Enviado · sin confirmar en cocina» (como en la instalación real).
  const category = await call(driver, 'POST', 'catalog/categories', {
    name: 'Platillos D',
    active: true,
    color: '#d64545',
  });
  const product = await call(driver, 'POST', 'catalog/products', {
    name: 'Guiso D',
    active: true,
    category_id: category.id,
    code: 'GUISO-D',
    kind: 'product',
    base_price: '95',
  });
  const productID = product.id as string;
  const area = (await call(driver, 'POST', 'restaurant/areas', {
    operation_id: randomUUID(),
    name: 'Salón D',
    active: true,
  })) as { id: string };
  const tables: Record<string, string> = {};
  for (const [index, name] of ['Mesa D1', 'Mesa D2', 'Mesa D3'].entries()) {
    const created = await call(driver, 'POST', 'restaurant/tables', {
      operation_id: randomUUID(),
      area_id: area.id,
      name,
      shape: 'rectangle',
      x: 300 + index * 160,
      y: 160,
      width: 120,
      height: 100,
      seats: 4,
      active: true,
    });
    tables[name] = created.id as string;
  }
  // Mesa D2: solo un borrador sin enviar (como las cuentas #1 y #3 de la instalación real).
  const draftOnly = await openWith(
    driver,
    { mode: 'dine_in', table_id: tables['Mesa D2'] },
    productID,
    false,
  );
  // Mesa D3: un producto enviado, con la liberación bloqueada.
  const sent = await openWith(
    driver,
    { mode: 'dine_in', table_id: tables['Mesa D3'] },
    productID,
    true,
  );
  // Para llevar: un pedido vacío (como los #5, #8, #9 y #10) y siete enviados para ver cuántas
  // tarjetas caben.
  const emptyTakeaway = (
    await call(driver, 'POST', 'restaurant/accounts', {
      operation_id: randomUUID(),
      mode: 'takeaway',
      origin: 'Para llevar',
    })
  ).account as ServerAccount;
  const takeaways: ServerAccount[] = [];
  for (let index = 1; index <= 7; index += 1) {
    takeaways.push(
      await openWith(
        driver,
        { mode: 'takeaway', origin: 'Para llevar', customer_name: `Cliente D${index}` },
        productID,
        true,
      ),
    );
  }
  await expect
    .poll(async () => (await readAccount(driver, sent.id)).preparation_state, { timeout: 20000 })
    .toBe('pending_release');
  await driver.request('POST', 'auth/logout');

  await page.setViewportSize({ width: 1280, height: 720 });
  await page.goto('/');
  await page.getByLabel('Usuario', { exact: true }).fill('admin');
  await page.getByLabel('Contraseña', { exact: true }).fill('admin');
  await page.getByRole('button', { name: 'Entrar', exact: true }).click();
  await expect(page.locator('.app-topbar')).toBeVisible();
  await call(driver, 'POST', 'auth/login', { username: 'admin', password: 'admin' });

  // 2. «Nuevo pedido» entra a la captura sin crear nada; salir sin productos no deja nada.
  const takeawayBefore = (await allAccounts(driver, 'takeaway')).length;
  await openSection(page, 'Para llevar');
  await page.getByRole('button', { name: 'Nuevo pedido', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Para llevar · Pedido nuevo' })).toBeVisible();
  const bill = page.getByRole('complementary', { name: 'Cuenta actual' });
  await expect(bill.getByRole('heading', { name: 'Pedido nuevo' })).toBeVisible();
  await expect(page.locator('.capture-topbar')).toContainText(
    'El pedido se abre al guardar el primer producto',
  );
  await page.locator('.capture-topbar-back').click();
  await expect(page.getByRole('button', { name: 'Nuevo pedido', exact: true })).toBeVisible();
  await page.waitForTimeout(1200);
  expect((await allAccounts(driver, 'takeaway')).length).toBe(takeawayBefore);

  // 3. Con el primer producto el pedido nace; vacío de nuevo (solo borrador) se descarta desde
  // «Más…» de la captura y se vuelve al listado.
  await page.getByRole('button', { name: 'Nuevo pedido', exact: true }).click();
  await page
    .getByRole('navigation', { name: 'Categorías' })
    .getByRole('button', { name: /Platillos D/ })
    .click();
  await page.getByRole('button', { name: /^Guiso D/ }).click();
  const born = page.getByRole('heading', { name: /^Para llevar · Pedido #\d+$/ });
  await expect(born).toBeVisible({ timeout: 10000 });
  await expect(bill.locator('.capture-sync-status')).toHaveText('Guardado');
  const bornFolio = Number(/#(\d+)/.exec((await born.textContent()) ?? '')?.[1]);
  const bornAccount = (await allAccounts(driver, 'takeaway')).find(
    (entry) => entry.folio === bornFolio,
  )!;
  expect(bornAccount).toBeTruthy();
  expect(bornAccount.table_id).toBeNull();
  expect(bornAccount.discardable).toBe(true);
  await bill.getByRole('button', { name: 'Más…', exact: true }).click();
  const more = bill.getByRole('group', { name: 'Más acciones de la cuenta' });
  await expect(more.getByRole('button', { name: 'Cancelar cuenta', exact: true })).toHaveCount(0);
  await more.getByRole('button', { name: 'Descartar cuenta vacía', exact: true }).click();
  let dialog = page.getByRole('alertdialog', { name: `¿Descartar el pedido #${bornFolio}?` });
  await expect(dialog).toContainText('Se borrará 1 producto sin enviar.');
  await expect(dialog).toContainText('no es una cancelación');
  await dialog.getByRole('button', { name: 'Descartar cuenta vacía', exact: true }).click();
  await expect(notice(page, `Pedido #${bornFolio} descartado.`)).toBeVisible();
  await expect(page.getByRole('button', { name: 'Nuevo pedido', exact: true })).toBeVisible();
  await expect(page.getByRole('heading', { name: /^Para llevar · Pedido #/ })).toHaveCount(0);
  expect((await readAccount(driver, bornAccount.id)).state).toBe('voided');

  // 4. Tarjetas compactas (§4.3): dos renglones, al menos el doble por pantalla (antes cabían unas
  // tres a 1280 × 720) y la selección en azul.
  const orders = page.getByRole('group', { name: 'Pedidos para llevar' });
  const firstSent = orders
    .getByRole('button')
    .filter({ hasText: `Pedido #${takeaways[0]!.folio} · Cliente D1` });
  await firstSent.click();
  await expect(firstSent).toHaveAttribute('aria-pressed', 'true');
  await expect(firstSent.locator(':scope > span')).toHaveCount(2);
  await expect(firstSent.locator('.account-card-line')).toContainText('$95.00');
  await expect(firstSent.locator('.account-card-tags')).toContainText('Sin confirmar en cocina');
  await expect(firstSent.locator('.account-card-tags')).toContainText('1 unidad');
  // La tarjeta de la 9C medía unos 157 px (cabían dos y media debajo de los filtros a 1280 × 720);
  // la compacta mide menos de 64 px y caben al menos cinco en ese mismo espacio.
  const cardBox = await firstSent.boundingBox();
  expect(cardBox!.height).toBeGreaterThanOrEqual(44);
  expect(cardBox!.height).toBeLessThanOrEqual(64);
  expect(await visibleCards(orders)).toBeGreaterThanOrEqual(5);
  await page.setViewportSize({ width: 1920, height: 1080 });
  // A 1920 × 1080 caben los ocho pedidos de la prueba (la 9C mostraba unos cinco).
  expect(await visibleCards(orders)).toBeGreaterThanOrEqual(8);
  await page.setViewportSize({ width: 1280, height: 720 });
  await clearToasts(page);
  await views(page, testInfo, 'fase-d-para-llevar', async () => {
    expect(await computed(firstSent, 'background-color')).toBe(await token(page, '--accent-soft'));
    expect(await computed(firstSent, 'border-top-color')).toBe(await token(page, '--accent'));
  });

  // 5. «Enviado · sin confirmar en cocina» una sola vez por producto (no en el encabezado) y menú
  // por producto: tocar el renglón abre «Modificar» y «Cancelar unidades» en rojo.
  await expect(bill.locator('.capture-bill-header')).not.toContainText('cocina');
  const line = bill.getByRole('article', { name: '1 × Guiso D' });
  await expect(line.locator('.capture-tag')).toHaveText('Enviado · sin confirmar en cocina');
  await expect(page.getByText(/revisar cocina/)).toHaveCount(0);
  await expect(line.getByRole('button', { name: 'Cancelar unidades', exact: true })).toHaveCount(0);
  await line.locator('.capture-line-row').click();
  const menu = line.getByRole('group', { name: 'Acciones de 1 × Guiso D' });
  await expect(menu).toBeVisible();
  const cancelUnits = menu.getByRole('button', { name: 'Cancelar unidades', exact: true });
  await expect(menu.getByRole('button', { name: 'Modificar', exact: true })).toBeVisible();
  expect(await computed(cancelUnits, 'color')).toBe(await token(page, '--danger'));
  expect((await cancelUnits.boundingBox())!.height).toBeGreaterThanOrEqual(44);
  await page.keyboard.press('Escape');
  await expect(menu).toHaveCount(0);
  await line.getByRole('button', { name: 'Acciones de 1 × Guiso D', exact: true }).click();
  await expect(menu).toBeVisible();
  await views(page, testInfo, 'fase-d-detalle-menu-producto', async () => {
    await expect(menu).toBeVisible();
    await menu.scrollIntoViewIfNeeded();
    // Cada producto ocupa su renglón y su menú, sin botones repetidos por producto.
    expect((await menu.boundingBox())!.height).toBeLessThanOrEqual(80);
    // «Cancelar unidades» cabe en una línea, sin cortarse.
    expect(
      await cancelUnits.evaluate((element) => element.scrollWidth <= element.clientWidth + 1),
    ).toBe(true);
    expect((await cancelUnits.boundingBox())!.height).toBeLessThanOrEqual(48);
  });
  await cancelUnits.click();
  await expect(
    page.getByRole('heading', { name: 'Cancelar unidades · Guiso D', exact: true }),
  ).toBeVisible();
  await page.getByRole('button', { name: 'Volver sin cancelar', exact: true }).click();
  await expect(line).toBeVisible();

  // 6. Pedido vacío: «Descartar cuenta vacía» a la vista en el detalle, sin motivo; no aparece en
  // «Cuentas canceladas».
  await orders
    .getByRole('button')
    .filter({ hasText: `Pedido #${emptyTakeaway.folio}` })
    .click();
  await expect(bill.getByRole('heading', { name: `Cuenta #${emptyTakeaway.folio}` })).toBeVisible();
  const visibleDiscard = bill.getByRole('button', { name: 'Descartar cuenta vacía', exact: true });
  expect(await computed(visibleDiscard, 'color')).toBe(await token(page, '--danger'));
  await visibleDiscard.click();
  dialog = page.getByRole('alertdialog', { name: `¿Descartar el pedido #${emptyTakeaway.folio}?` });
  await expect(dialog).toHaveClass(/dialog-panel-danger/);
  await dialog.getByRole('button', { name: 'Descartar cuenta vacía', exact: true }).click();
  await expect(notice(page, `Pedido #${emptyTakeaway.folio} descartado.`)).toBeVisible();
  await expect(
    orders.getByRole('button').filter({ hasText: `Pedido #${emptyTakeaway.folio}` }),
  ).toHaveCount(0);
  expect((await readAccount(driver, emptyTakeaway.id)).state).toBe('voided');

  // 7. «Cuenta sin mesa» del resumen de Mesas: sin productos no deja nada; con el primero nace en
  // el salón y sin mesa (se descarta al final).
  await openSection(page, 'Mesas');
  await page
    .getByRole('group', { name: 'Áreas del restaurante' })
    .getByRole('button', { name: 'Salón D', exact: true })
    .click();
  const summary = page.getByRole('complementary', { name: 'Resumen del área' });
  const dineBefore = await allAccounts(driver, 'dine_in');
  await summary.getByRole('button', { name: 'Cuenta sin mesa', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Cuenta nueva sin mesa' })).toBeVisible();
  await page.locator('.capture-topbar-back').click();
  await expect(page.getByRole('group', { name: 'Cuentas disponibles' })).toBeVisible();
  await page.waitForTimeout(1200);
  expect((await allAccounts(driver, 'dine_in')).length).toBe(dineBefore.length);
  await page.getByRole('button', { name: 'Cuenta sin mesa', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Cuenta nueva sin mesa' })).toBeVisible();
  await page
    .getByRole('navigation', { name: 'Categorías' })
    .getByRole('button', { name: /Platillos D/ })
    .click();
  await page.getByRole('button', { name: /^Guiso D/ }).click();
  await expect(bill.getByRole('heading', { name: /^Cuenta #\d+$/ })).toBeVisible({
    timeout: 10000,
  });
  await expect(bill.locator('.capture-sync-status')).toHaveText('Guardado');
  const known = new Set(dineBefore.map((entry) => entry.id));
  const withoutTable = (await allAccounts(driver, 'dine_in')).filter(
    (entry) => !known.has(entry.id),
  );
  expect(withoutTable).toHaveLength(1);
  expect(withoutTable[0]).toEqual(
    expect.objectContaining({ state: 'open', table_id: null, discardable: true }),
  );
  await page.locator('.capture-topbar-back').click();

  // 8. Mesas › Cuentas: tarjetas compactas con la mesa; el detalle con el texto de cocina.
  const cards = page.getByRole('group', { name: 'Cuentas disponibles' });
  const sentCard = cards.getByRole('button').filter({ hasText: 'Mesa D3' }).first();
  await sentCard.click();
  await expect(sentCard).toHaveAttribute('aria-pressed', 'true');
  await expect(sentCard.locator(':scope > span')).toHaveCount(2);
  await expect(bill.getByRole('heading', { name: `Cuenta #${sent.folio}` })).toBeVisible();
  await expect(bill.locator('.capture-bill-header')).toContainText('Pago: sin registro de cobro');
  await expect(bill.locator('.capture-bill-header')).not.toContainText('cocina');
  await clearToasts(page);
  await views(page, testInfo, 'fase-d-mesas-cuentas');
  // Descartar desde el detalle de Cuentas la cuenta sin mesa recién creada.
  for (const entry of withoutTable) {
    await cards
      .getByRole('button')
      .filter({ hasText: `Cuenta #${entry.folio}` })
      .first()
      .click();
    await bill.getByRole('button', { name: 'Descartar cuenta vacía', exact: true }).click();
    dialog = page.getByRole('alertdialog', { name: `¿Descartar la cuenta #${entry.folio}?` });
    await dialog.getByRole('button', { name: 'Descartar cuenta vacía', exact: true }).click();
    await expect(notice(page, `Cuenta #${entry.folio} descartada.`)).toBeVisible();
  }

  // 9. Mesas: la Mesa D2 (solo borrador) se descarta desde el panel, sin motivo, y queda libre; la
  // Mesa D3 (producto enviado) conserva «Cancelar cuenta».
  // Las pestañas «Mesas / Cuentas / Áreas y distribución» miden al menos 44 px.
  const viewTabs = page.getByRole('group', { name: 'Vistas operativas' }).getByRole('button');
  for (let index = 0; index < (await viewTabs.count()); index += 1) {
    expect((await viewTabs.nth(index).boundingBox())!.height).toBeGreaterThanOrEqual(44);
  }
  await page
    .getByRole('group', { name: 'Vistas operativas' })
    .getByRole('button', { name: 'Mesas', exact: true })
    .click();
  const map = page.getByRole('region', { name: 'Mapa del restaurante' });
  await map.getByRole('button', { name: /^Mesa D3, / }).click();
  let panel = page.getByRole('complementary', { name: 'Panel de Mesa D3' });
  await expect(panel.getByRole('heading', { name: `Cuenta #${sent.folio}` })).toBeVisible();
  await expect(panel.getByRole('button', { name: 'Descartar cuenta vacía' })).toHaveCount(0);
  await panel.getByRole('button', { name: 'Más…', exact: true }).click();
  await expect(
    panel
      .getByRole('group', { name: 'Más acciones de la cuenta' })
      .getByRole('button', { name: 'Cancelar cuenta', exact: true }),
  ).toBeVisible();
  await page.keyboard.press('Escape');
  await map.getByRole('button', { name: /^Mesa D2, / }).click();
  panel = page.getByRole('complementary', { name: 'Panel de Mesa D2' });
  const panelDiscard = panel.getByRole('button', { name: 'Descartar cuenta vacía', exact: true });
  await expect(panelDiscard).toBeVisible();
  await expect(panel.getByRole('button', { name: 'Liberar mesa' })).toHaveCount(0);
  const discardBox = await panelDiscard.boundingBox();
  expect(discardBox!.height).toBeGreaterThanOrEqual(44);
  // Su texto cabe en una línea.
  expect(
    await panelDiscard.evaluate((element) => element.scrollWidth <= element.clientWidth + 1),
  ).toBe(true);
  await panelDiscard.click();
  dialog = page.getByRole('alertdialog', { name: `¿Descartar la cuenta #${draftOnly.folio}?` });
  await expect(dialog).toContainText('Mesa D2 quedará libre.');
  await expect(dialog).toContainText('Se borrará 1 producto sin enviar.');
  await clearToasts(page);
  await views(page, testInfo, 'fase-d-descartar-confirmacion', async () => {
    await expect(dialog).toBeVisible();
  });
  await dialog.getByRole('button', { name: 'Descartar cuenta vacía', exact: true }).click();
  await expect(notice(page, `Cuenta #${draftOnly.folio} descartada.`)).toBeVisible();
  await expect(panel.locator('.table-panel-state')).toHaveText('Libre');
  expect((await readAccount(driver, draftOnly.id)).state).toBe('voided');

  // 10. Descartar no es cancelar: ninguna cuenta descartada aparece en «Cuentas canceladas».
  const reported = await voidedFolios(driver);
  for (const folio of [bornFolio, emptyTakeaway.folio, draftOnly.folio]) {
    expect(reported).not.toContain(String(folio));
  }
  expect(errors).toEqual([]);
});
