import { randomUUID } from 'node:crypto';
import { copyFileSync, mkdirSync } from 'node:fs';
import { join } from 'node:path';
import { expect, test } from '@playwright/test';
import type { Locator, Page, TestInfo } from '@playwright/test';
import { installDriver } from './native-driver';
import { openAdministration, openSection } from './navigation';
import { notice } from './notices';
import { chooseOption } from './select';

// Etapa 9D, fase E (§5 y §7): «Cancelar cuenta» con motivo del catálogo en una cuenta con productos
// enviados a cocina, con el permiso propio (administrador) y con la autorización de un supervisor
// (usuario creado en la prueba sin «Cancelar cuentas»); la mesa queda libre; las dos aparecen en
// «Cuentas canceladas» con «Autorizó» y en «Cancelaciones por motivo» (pantalla, PDF y Excel); el
// catálogo de motivos en Cobros y el umbral de cuenta abierta en «Funciones del establecimiento» se
// guardan. Mismo transporte que las demás especificaciones (Go temporal y base adel_pos_test_*).
// Capturas en tema claro y oscuro a 1280 × 720 y 1920 × 1080 (copia opcional en ADEL_SHOTS_DIR).

type Driver = Awaited<ReturnType<typeof installDriver>>;

async function call(driver: Driver, method: string, route: string, body?: unknown) {
  const response = await driver.request(method, route, body);
  expect(response.status, `${method} ${route}: ${JSON.stringify(response.body)}`).toBe(200);
  return response.body;
}

async function table(driver: Driver, areaID: string, name: string, x: number) {
  return (await call(driver, 'POST', 'restaurant/tables', {
    operation_id: randomUUID(),
    area_id: areaID,
    name,
    shape: 'rectangle',
    x,
    y: 60,
    width: 120,
    height: 100,
    seats: 4,
    active: true,
  })) as { id: string; revision: number };
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
  const bill = await call(driver, 'GET', route);
  return { id, folio: bill.account.folio as number, total: bill.account.totals.total as string };
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

// Recorre los dos tamaños y los dos temas: sin desbordamiento horizontal, la comprobación de la
// vista y una captura de la página.
async function views(page: Page, testInfo: TestInfo, name: string, check?: () => Promise<void>) {
  await clearToasts(page);
  for (const size of sizes) {
    await page.setViewportSize(size);
    for (const theme of themes) {
      const file = `${name}-${theme}-${size.width}x${size.height}.png`;
      // La aplicación vuelve a aplicar el tema del usuario cuando se actualiza su sesión: la
      // captura se repite si el tema cambió mientras se tomaba.
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

// Abre la mesa en el mapa del área de la prueba y elige «Cancelar cuenta» en «Más…» (antes de
// elegirla, `inspect` revisa la opción con el menú abierto).
async function openCancel(
  page: Page,
  area: string,
  name: string,
  inspect?: (option: Locator) => Promise<void>,
) {
  await openSection(page, 'Mesas');
  await page
    .getByRole('group', { name: 'Áreas del restaurante' })
    .getByRole('button', { name: area, exact: true })
    .click();
  await page
    .locator('.table-canvas[aria-label="Mapa de mesas"]')
    .getByRole('button', { name: new RegExp(`^${name},`) })
    .click();
  const panel = page.getByRole('complementary', { name: `Panel de ${name}` });
  await panel.getByRole('button', { name: 'Más…', exact: true }).click();
  const option = panel
    .getByRole('group', { name: 'Más acciones de la cuenta' })
    .getByRole('button', { name: 'Cancelar cuenta', exact: true });
  await expect(option).toHaveClass(/danger-button/);
  await inspect?.(option);
  await option.click();
  const region = page.getByRole('region', { name: 'Cancelar cuenta' });
  await expect(region).toBeVisible();
  return { panel, region };
}

function reasonButton(region: Locator, name: string) {
  return region
    .getByRole('group', { name: 'Motivo de la cancelación' })
    .getByRole('button', { name: new RegExp(`^${name}`) });
}

// Abre un reporte de Ventas: por debajo de 1440 px con el selector «Reporte» y en pantallas anchas
// con la lista lateral.
async function openReport(page: Page, title: string) {
  await page
    .getByRole('tablist', { name: 'Grupos de reportes' })
    .getByRole('tab', { name: 'Ventas', exact: true })
    .click();
  const picker = page.getByRole('combobox', { name: 'Reporte', exact: true });
  if (await picker.isVisible()) {
    await chooseOption(picker, title);
  } else {
    await page
      .getByRole('navigation', { name: 'Reportes de Ventas' })
      .getByRole('button', { name: title, exact: true })
      .click();
  }
  await expect(page.getByRole('heading', { name: title, exact: true })).toBeVisible();
  return page.getByRole('region', { name: `Tabla: ${title}` });
}

test('cancelar cuenta con permiso propio y con supervisor, reportes, catálogo y umbral (etapa 9D, fase E)', async ({
  page,
}, testInfo) => {
  test.setTimeout(420000);
  const errors: string[] = [];
  page.on('pageerror', (error) => errors.push(error.message));
  const driver = await installDriver(page);
  await call(driver, 'POST', 'auth/login', { username: 'admin', password: 'admin' });
  await call(driver, 'PUT', 'terminal/preferences', { keyboard_mode: 'disabled' });

  // 1. Datos por la API: dos mesas con productos enviados a cocina, una tercera con un abono
  // parcial (para el bloqueo por pagos) y un usuario sin «Cancelar cuentas».
  const category = await call(driver, 'POST', 'catalog/categories', {
    name: 'Platillos 9D-E',
    active: true,
    color: '#d64545',
  });
  const product = await call(driver, 'POST', 'catalog/products', {
    name: 'Tacos 9D-E',
    active: true,
    category_id: category.id,
    code: 'TACOS-9DE',
    kind: 'product',
    base_price: '115',
  });
  const productID = product.id as string;
  const areaName = 'Terraza 9D-E';
  const area = (await call(driver, 'POST', 'restaurant/areas', {
    operation_id: randomUUID(),
    name: areaName,
    active: true,
  })) as { id: string };
  const own = await table(driver, area.id, 'Mesa E1', 40);
  const supervised = await table(driver, area.id, 'Mesa E2', 220);
  const paid = await table(driver, area.id, 'Mesa E3', 400);
  const ownAccount = await sentAccount(driver, own.id, productID, 3);
  const supervisedAccount = await sentAccount(driver, supervised.id, productID, 2);
  const paidAccount = await sentAccount(driver, paid.id, productID, 1);
  const current = await call(driver, 'GET', 'payments/shifts/current');
  let shift = current.shift as { id: string; state: string } | null;
  if (!shift) {
    shift = await call(driver, 'POST', 'payments/shifts', {
      operation_id: randomUUID(),
      opening_float: '0',
    });
  }
  expect(shift!.state).toBe('open');
  const methods = (await call(driver, 'GET', 'payments/methods')) as {
    id: string;
    code: string | null;
  }[];
  const cash = methods.find((method) => method.code === 'cash');
  const finance = await call(driver, 'GET', `payments/accounts/${paidAccount.id}`);
  await call(driver, 'POST', `payments/accounts/${paidAccount.id}/payments`, {
    operation_id: randomUUID(),
    mode: 'partial',
    amount: '50',
    expected_balance: finance.balance,
    method_id: cash!.id,
    received: '50',
  });
  const role = await call(driver, 'POST', 'roles', {
    name: 'Mesero sin cancelar 9D',
    permissions: [
      'accounts.read',
      'accounts.open',
      'restaurant.tables.read',
      'restaurant.areas.read',
      'orders.history.read',
    ],
  });
  const waiterPassword = 'clave-mesero-9d-e';
  await call(driver, 'POST', 'users', {
    username: 'mesero9de',
    display_name: 'Mesero 9D',
    role_id: role.id,
    active: true,
    password: waiterPassword,
  });
  const reasons = (await call(driver, 'GET', 'restaurant/cancel-reasons')) as {
    id: string;
    name: string;
    requires_description: boolean;
    prepared_is_waste: boolean;
    active: boolean;
  }[];
  expect(reasons.map((reason) => reason.name)).toEqual(
    expect.arrayContaining([
      'Cliente se retiró sin pagar',
      'Cliente canceló el pedido',
      'Error de captura',
      'Otro',
    ]),
  );
  await driver.request('POST', 'auth/logout');

  await page.setViewportSize({ width: 1280, height: 720 });
  await page.goto('/');
  await login(page, 'admin', 'admin');

  // 2. Bloqueo por pagos: el botón sigue a la vista y explica que primero va la devolución.
  {
    const { panel, region } = await openCancel(page, areaName, 'Mesa E3', async (option) => {
      // La opción sigue habilitada y dice qué hacer primero (descripción accesible del botón).
      await expect(option).toBeEnabled();
      await expect(option).toHaveAccessibleDescription('Primero registra la devolución');
      await views(page, testInfo, 'cancelar-cuenta-menu-bloqueado', async () => {
        await expect(option).toBeVisible();
      });
    });
    const blocked = region.getByRole('alert');
    await expect(blocked).toContainText('Primero registra la devolución');
    await expect(blocked).toContainText('(pagado $50.00)');
    await expect(region.getByRole('button', { name: 'Ir a devoluciones' })).toBeVisible();
    await expect(region.getByRole('button', { name: 'Cancelar cuenta', exact: true })).toHaveCount(
      0,
    );
    await views(page, testInfo, 'cancelar-cuenta-bloqueada-pago', async () => {
      await expect(blocked).toBeVisible();
      await expect(panel).toBeVisible();
    });
    await region.getByRole('button', { name: 'Volver sin cancelar' }).click();
    await expect(region).toHaveCount(0);
  }

  // 3. Con el permiso propio: motivo «Cliente se retiró sin pagar», importe de Go, efecto en cocina
  // con la marca de merma y confirmación roja. La mesa queda libre.
  {
    const { region } = await openCancel(page, areaName, 'Mesa E1');
    await expect(
      region.getByRole('heading', { name: `Cancelar cuenta · #${ownAccount.folio}` }),
    ).toBeVisible();
    const summary = region.getByLabel('Lo que se cancela');
    await expect(summary).toContainText('3 unidades');
    await expect(summary).toContainText(`$${ownAccount.total}`);
    await expect(region.getByText('Autorización de supervisor')).toHaveCount(0);
    const submit = region.getByRole('button', { name: 'Cancelar cuenta', exact: true });
    await expect(submit).toHaveClass(/danger-solid-button/);
    await expect(submit).toBeDisabled();
    await reasonButton(region, 'Cliente se retiró sin pagar').click();
    await expect(region.getByText(/lo preparado cuenta como merma/)).toBeVisible();
    await expect(region.getByText(/Mesa E1 queda libre si no tiene otras cuentas/)).toBeVisible();
    await expect(submit).toBeEnabled();
    await views(page, testInfo, 'cancelar-cuenta-sin-supervisor', async () => {
      await expect(submit).toBeVisible();
    });
    await submit.click();
    const dialog = page.getByRole('alertdialog', {
      name: `¿Cancelar la cuenta #${ownAccount.folio}?`,
    });
    await expect(dialog).toHaveClass(/dialog-panel-danger/);
    await expect(dialog).toContainText('Motivo: Cliente se retiró sin pagar');
    await expect(dialog).toContainText(
      `Se cancelan 3 unidades enviadas a cocina por $${ownAccount.total}.`,
    );
    await views(page, testInfo, 'cancelar-cuenta-confirmacion', async () => {
      await expect(dialog).toBeVisible();
    });
    const confirm = dialog.getByRole('button', { name: 'Cancelar cuenta', exact: true });
    await expect(confirm).toHaveClass(/danger-solid-button/);
    await confirm.click();
    await expect(notice(page, `Cuenta #${ownAccount.folio} cancelada.`)).toBeVisible();
    await expect(region).toHaveCount(0);
    await expect(
      page
        .locator('.table-canvas[aria-label="Mapa de mesas"]')
        .getByRole('button', { name: /^Mesa E1, Libre/ }),
    ).toBeVisible();
  }

  // 4. Sin «Cancelar cuentas»: el mesero cancela con la autorización del administrador, motivo
  // «Otro» con descripción obligatoria.
  await clearToasts(page);
  await logout(page);
  await login(page, 'mesero9de', waiterPassword);
  {
    const { region } = await openCancel(page, areaName, 'Mesa E2');
    const fieldset = region.getByRole('group', { name: 'Autorización de supervisor' });
    await expect(fieldset).toContainText('«Cancelar cuentas»');
    await reasonButton(region, 'Otro').click();
    const description = region.getByRole('textbox', { name: /^Descripción del motivo «Otro»/ });
    const submit = region.getByRole('button', { name: 'Cancelar cuenta', exact: true });
    await description.fill('abc');
    await expect(submit).toBeDisabled();
    await description.fill('Se equivocaron de mesa');
    await fieldset.getByLabel('Usuario del supervisor', { exact: true }).fill('admin');
    const password = fieldset.getByLabel('Contraseña del supervisor', { exact: true });
    await password.fill('admin');
    await expect(submit).toBeEnabled();
    await views(page, testInfo, 'cancelar-cuenta-con-supervisor', async () => {
      await expect(fieldset).toBeVisible();
    });
    await submit.click();
    const dialog = page.getByRole('alertdialog', {
      name: `¿Cancelar la cuenta #${supervisedAccount.folio}?`,
    });
    await dialog.getByRole('button', { name: 'Cancelar cuenta', exact: true }).click();
    await expect(notice(page, `Cuenta #${supervisedAccount.folio} cancelada.`)).toBeVisible();
    await expect(
      page
        .locator('.table-canvas[aria-label="Mapa de mesas"]')
        .getByRole('button', { name: /^Mesa E2, Libre/ }),
    ).toBeVisible();
  }
  await clearToasts(page);
  await logout(page);

  // Go registró quién canceló y quién autorizó; ninguna de las dos es venta.
  await call(driver, 'POST', 'auth/login', { username: 'admin', password: 'admin' });
  const ownBill = await call(driver, 'GET', `restaurant/accounts/${ownAccount.id}`);
  expect(ownBill.account.state).toBe('voided');
  expect(ownBill.cancellation).toEqual(
    expect.objectContaining({
      reason_name: 'Cliente se retiró sin pagar',
      prepared_is_waste: true,
      cancelled_by_name: 'Administrador',
      authorized_by_name: null,
      cancelled_units: 3,
      cancelled_amount: ownAccount.total,
    }),
  );
  const supervisedBill = await call(driver, 'GET', `restaurant/accounts/${supervisedAccount.id}`);
  expect(supervisedBill.cancellation).toEqual(
    expect.objectContaining({
      reason_name: 'Otro',
      description: 'Se equivocaron de mesa',
      cancelled_by_name: 'Mesero 9D',
      authorized_by_name: 'Administrador',
      cancelled_units: 2,
    }),
  );
  await driver.request('POST', 'auth/logout');

  // 5. Reportes: «Cuentas canceladas» con «Autorizó» y «Cancelaciones por motivo», en pantalla y
  // exportados a PDF y Excel (los títulos y columnas vienen de Go).
  await login(page, 'admin', 'admin');
  await openAdministration(page, 'Reportes');
  const cancelled = await openReport(page, 'Cuentas canceladas');
  await expect(cancelled.getByRole('columnheader', { name: /^Autorizó/ })).toBeVisible();
  const supervisedRow = cancelled.getByRole('row').filter({ hasText: 'Se equivocaron de mesa' });
  await expect(supervisedRow).toContainText('Mesero 9D');
  await expect(supervisedRow).toContainText('Administrador');
  const ownRow = cancelled.getByRole('row').filter({ hasText: 'Cliente se retiró sin pagar' });
  await expect(ownRow.first()).toContainText(`$${ownAccount.total}`);
  const exportGroup = page.getByRole('group', { name: 'Exportar' });
  await exportGroup.getByRole('button', { name: 'Exportar PDF', exact: true }).click();
  await expect(notice(page, 'Cuentas canceladas: PDF guardado.')).toBeVisible({ timeout: 30000 });
  await exportGroup.getByRole('button', { name: 'Exportar Excel', exact: true }).click();
  await expect(notice(page, 'Cuentas canceladas: Excel guardado.')).toBeVisible({
    timeout: 30000,
  });
  const byReason = await openReport(page, 'Cancelaciones por motivo');
  await expect(
    byReason.getByRole('row').filter({ hasText: 'Cliente se retiró sin pagar' }),
  ).toBeVisible();
  await expect(byReason.getByRole('row').filter({ hasText: 'Otro' })).toBeVisible();
  await exportGroup.getByRole('button', { name: 'Exportar PDF', exact: true }).click();
  await expect(notice(page, 'Cancelaciones por motivo: PDF guardado.')).toBeVisible({
    timeout: 30000,
  });
  await exportGroup.getByRole('button', { name: 'Exportar Excel', exact: true }).click();
  await expect(notice(page, 'Cancelaciones por motivo: Excel guardado.')).toBeVisible({
    timeout: 30000,
  });
  const exported = driver.exports.map((entry) => `${entry.report}:${entry.contentType}`);
  expect(exported).toEqual(
    expect.arrayContaining([
      'sales.voided_accounts:application/pdf',
      'sales.voided_accounts:application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
      'sales.cancellations_by_reason:application/pdf',
      'sales.cancellations_by_reason:application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
    ]),
  );
  for (const file of driver.exports.slice(-4)) {
    expect(file.fileName).toMatch(/^(cuentas-canceladas|cancelaciones-por-motivo)_/);
  }

  // 6. Catálogo de motivos en Cobros, junto a los motivos de descuento: crear uno con su marca de
  // merma y ver la lista con su estilo.
  await clearToasts(page);
  await openAdministration(page, 'Cobros');
  await page.getByRole('button', { name: /^Motivos de cancelación/ }).click();
  const catalog = page.getByRole('group', { name: 'Motivos de cancelación' });
  await expect(catalog.getByRole('button', { name: /^Cliente se retiró sin pagar/ })).toBeVisible();
  await catalog.getByRole('button', { name: '+ Nuevo motivo', exact: true }).click();
  await page.getByLabel(/Nombre del motivo/).fill('Cuenta duplicada 9D');
  await page.getByRole('button', { name: 'Lo enviado a cocina cuenta como merma' }).click();
  await page.getByRole('button', { name: 'Crear motivo', exact: true }).click();
  await expect(notice(page, 'Motivo de cancelación creado.')).toBeVisible();
  await expect(
    catalog.getByRole('button', { name: /^Cuenta duplicada 9D · cuenta como merma/ }),
  ).toBeVisible();
  await catalog.getByRole('button', { name: /^Error de captura/ }).click();
  await views(page, testInfo, 'motivos-cancelacion', async () => {
    await expect(catalog).toBeVisible();
  });
  await page.getByRole('button', { name: 'Cancelar', exact: true }).click();

  // 7. Umbral de cuenta abierta en «Funciones del establecimiento».
  await openAdministration(page, 'Configuración');
  const features = page.getByRole('region', { name: 'Funciones del establecimiento' });
  const hours = features.getByRole('textbox', {
    name: /Marcar una cuenta abierta después de \(horas\)/,
  });
  await expect(hours).toHaveValue('12');
  expect(await hours.getAttribute('type')).toBe('text');
  await hours.fill('6');
  await expect(features.getByText(/más de 6 horas se marcan en ámbar en Mesas/)).toBeVisible();
  await features.getByRole('button', { name: 'Guardar umbral', exact: true }).click();
  await expect(notice(page, 'Umbral de cuenta abierta guardado.')).toBeVisible();
  await views(page, testInfo, 'umbral-cuenta-abierta', async () => {
    // El campo queda al centro, por encima de la barra fija de «Guardar umbral».
    await hours.evaluate((element) => element.scrollIntoView({ block: 'center' }));
    await expect(hours).toBeVisible();
  });

  await call(driver, 'POST', 'auth/login', { username: 'admin', password: 'admin' });
  const saved = (await call(driver, 'GET', 'restaurant/cancel-reasons')) as {
    name: string;
    prepared_is_waste: boolean;
  }[];
  expect(saved.find((reason) => reason.name === 'Cuenta duplicada 9D')?.prepared_is_waste).toBe(
    true,
  );
  const threshold = await call(driver, 'GET', 'settings/long-open-hours');
  expect(threshold.long_open_hours).toBe(6);
  // Se deja el umbral de 12 h para las especificaciones siguientes.
  await call(driver, 'PUT', 'settings/long-open-hours', {
    long_open_hours: 12,
    revision: threshold.revision,
  });
  await driver.request('POST', 'auth/logout');
  expect(errors).toEqual([]);
});
