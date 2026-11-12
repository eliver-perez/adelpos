import { randomUUID } from 'node:crypto';
import { expect, test } from '@playwright/test';
import type { Locator, Page, TestInfo } from '@playwright/test';
import { installDriver } from './native-driver';
import { openAdministration, topBar } from './navigation';
import { notice } from './notices';
import { groupTablist, inventoryGroups, openInventorySection } from './inventory-navigation';
import { chooseOption, expectSelectValue } from './select';

// Etapa 8, fase C (interfaz de compras): recorrido de proveedores, órdenes de compra, recepciones
// y vínculo con gastos de caja con el mismo transporte de prueba que las demás especificaciones
// (Go temporal y base adel_pos_test_*). Crea un proveedor propio con el insumo que surte, una
// orden de 20 cajas que se emite y se recibe en 12 + 8 cajas (pendiente y estados calculados por
// Go), comprueba la existencia y los movimientos, registra una recepción directa y la vincula con
// un gasto de caja registrado en un turno abierto. Los insumos y el gasto se preparan por la API
// del puente (no son parte de lo que se prueba). El nombre del archivo la ordena después de las
// especificaciones existentes en la corrida completa: no altera lo que ellas comprueban.

type Driver = Awaited<ReturnType<typeof installDriver>>;

const supplierName = 'Distribuidora E2E C';
const sodaName = 'Refresco E2E C';
const sodaCode = 'REF-E2E-C';
const cupName = 'Vaso E2E C';
const cupCode = 'VAS-E2E-C';
const expenseConcept = 'Vasos para llevar E2E';

// La página no se desborda a lo ancho a 1920 × 1080 ni a 1280 × 720, en tema claro y oscuro. Con
// «capture» guarda la vista a 1280 × 720 en ambos temas.
async function checkLayout(page: Page, testInfo: TestInfo, name: string, capture: boolean) {
  for (const size of [
    { width: 1920, height: 1080 },
    { width: 1280, height: 720 },
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
      if (capture && size.width === 1280) {
        await expect(page.locator('[data-sonner-toast]')).toHaveCount(0, { timeout: 9000 });
        // La aplicación vuelve a aplicar el tema del usuario al refrescar la sesión: se fija de
        // nuevo justo antes de la captura.
        await page.evaluate((value) => {
          document.documentElement.dataset.theme = value;
        }, theme);
        await expect(page.locator('html')).toHaveAttribute('data-theme', theme);
        await page.waitForTimeout(150);
        await page.screenshot({
          path: testInfo.outputPath(`c2-${name}-${theme}-1280x720.png`),
          fullPage: true,
        });
      }
    }
  }
  await page.evaluate(() => {
    document.documentElement.dataset.theme = 'light';
  });
}

// Abre una sección de Inventarios por su grupo y su nombre (navegación agrupada de la etapa 9A).
async function openTab(page: Page, name: string) {
  await openInventorySection(page, name);
}

async function confirm(page: Page, button: string, text?: string | RegExp) {
  const dialog = page.getByRole('alertdialog');
  await expect(dialog).toBeVisible();
  if (text) {
    await expect(dialog).toContainText(text);
  }
  await dialog.getByRole('button', { name: button, exact: true }).click();
  await expect(dialog).toHaveCount(0);
}

async function created(driver: Driver, route: string, body: Record<string, unknown>) {
  const response = await driver.request('POST', route, { operation_id: randomUUID(), ...body });
  expect(response.status, `${route}: ${JSON.stringify(response.body)}`).toBe(200);
  return response.body as Record<string, unknown> & { id: string };
}

// Precondiciones por la API con la sesión del administrador: dos insumos en piezas (el refresco
// con su presentación «Caja con 24») y un gasto de $116.00 en un turno abierto de la caja de la
// terminal (se abre con fondo de $1,000.00 si no hay turno).
async function preparePreconditions(driver: Driver) {
  const units = await driver.request('GET', 'inventory/units');
  const piece = (units.body as { id: string; code: string }[]).find((unit) => unit.code === 'pza');
  expect(piece, 'unidad pza').toBeTruthy();
  const soda = await created(driver, 'inventory/items', {
    code: sodaCode,
    name: sodaName,
    description: '',
    base_unit_id: piece!.id,
    kind: 'purchased',
    active: true,
  });
  await created(driver, `inventory/items/${soda.id}/presentations`, {
    name: 'Caja con 24',
    quantity: '24',
    unit_id: null,
    barcode: null,
    active: true,
    purchase_default: true,
  });
  const cup = await created(driver, 'inventory/items', {
    code: cupCode,
    name: cupName,
    description: '',
    base_unit_id: piece!.id,
    kind: 'purchased',
    active: true,
  });
  const current = await driver.request('GET', 'payments/shifts/current');
  expect(current.body.register, JSON.stringify(current.body)).toBeTruthy();
  let shift = current.body.shift as { id: string; state: string } | null;
  if (!shift) {
    shift = (await created(driver, 'payments/shifts', { opening_float: '1000' })) as unknown as {
      id: string;
      state: string;
    };
  }
  expect(shift.state).toBe('open');
  const catalog = await driver.request('GET', 'payments/cash/catalog');
  const category = (
    catalog.body.expense_categories as { id: string; name: string; active: boolean }[]
  ).find((entry) => entry.name === 'Insumos' && entry.active);
  expect(category, 'categoría de gasto «Insumos»').toBeTruthy();
  const expense = await created(driver, `payments/shifts/${shift.id}/expenses`, {
    category_id: category!.id,
    amount: '116',
    concept: expenseConcept,
    reference: 'Nota 77',
  });
  return { sodaID: soda.id, cupID: cup.id, expenseID: expense.id };
}

async function chooseItem(line: Locator, code: string, label: string) {
  const select = line.getByRole('combobox', { name: /^Insumo/ });
  await chooseOption(select, label, { search: code });
}

async function chooseSupplier(scope: Page | Locator) {
  const select = scope.getByRole('combobox', { name: /^Proveedor/ });
  await chooseOption(select, supplierName, { search: 'E2E C' });
}

test('compras: proveedor, orden de 20 cajas recibida en 12 + 8, recepción directa y vínculo con un gasto', async ({
  page,
}, testInfo) => {
  test.setTimeout(240000);
  const errors: string[] = [];
  page.on('pageerror', (error) => errors.push(error.message));
  const driver = await installDriver(page);
  // En la corrida completa restaurant.spec.ts deja el teclado virtual siempre visible en la
  // terminal; esta especificación lo desactiva antes de entrar.
  await driver.request('POST', 'auth/login', { username: 'admin', password: 'admin' });
  const keyboard = await driver.request('PUT', 'terminal/preferences', {
    keyboard_mode: 'disabled',
  });
  expect(keyboard.status, JSON.stringify(keyboard.body)).toBeLessThan(300);
  await driver.request('POST', 'auth/logout');
  await page.setViewportSize({ width: 1280, height: 720 });
  await page.goto('/');
  await page.getByLabel('Usuario', { exact: true }).fill('admin');
  await page.getByLabel('Contraseña', { exact: true }).fill('admin');
  await page.getByRole('button', { name: 'Entrar', exact: true }).click();
  // Etapa 9C: Inventarios, Reportes y Documentos son mosaicos de Administración.
  await expect(
    topBar(page).getByRole('button', { name: 'Administración', exact: true }),
  ).toBeVisible();
  const prepared = await preparePreconditions(driver);
  await openAdministration(page, 'Inventarios');
  await expect(page.getByRole('heading', { name: 'Inventarios', exact: true })).toBeVisible();
  // Etapa 9A: cinco grupos en la primera fila (las secciones van en la segunda).
  await expect(groupTablist(page).getByRole('tab')).toHaveText(inventoryGroups);
  await expect(page.locator('input[type="number"]')).toHaveCount(0);

  // 1. Proveedor con RFC y el insumo que surte (presentación habitual, código y preferido).
  await openTab(page, 'Proveedores');
  await page.getByRole('button', { name: 'Nuevo proveedor', exact: true }).click();
  await page.getByLabel(/^Nombre comercial/).fill(supplierName);
  await page.getByLabel(/^Razón social/).fill('Distribuidora E2E C, S.A. de C.V.');
  await page.getByLabel(/^RFC/).fill('dne010203ab1');
  await page.getByLabel(/^Contacto/).fill('Laura Pérez');
  await page.getByLabel('Teléfono', { exact: true }).fill('868 555 0101');
  await expect(page.locator('input[type="number"]')).toHaveCount(0);
  await page.getByRole('button', { name: 'Crear proveedor', exact: true }).click();
  await expect(
    notice(page, 'Proveedor creado. Ya puedes agregar los insumos que surte.'),
  ).toBeVisible();
  await expect(page.getByRole('heading', { name: supplierName, exact: true })).toBeVisible();
  // Go guarda el RFC en mayúsculas.
  await expect(page.getByLabel(/^RFC/)).toHaveValue('DNE010203AB1');
  const supplied = page.getByRole('region', { name: 'Insumos que surte' });
  await supplied.getByRole('button', { name: 'Agregar insumo surtido', exact: true }).click();
  const itemForm = page.getByRole('form', { name: 'Nuevo insumo surtido' });
  const itemSelect = itemForm.getByRole('combobox', { name: /^Insumo surtido/ });
  await chooseOption(itemSelect, `${sodaName} · ${sodaCode}`, { search: sodaCode });
  await chooseOption(
    itemForm.getByRole('combobox', { name: /^Presentación habitual/ }),
    'Caja con 24 (24 pza)',
  );
  await itemForm.getByLabel(/^Código del proveedor/).fill('R-24');
  await itemForm
    .getByRole('button', { name: 'Proveedor preferido de este insumo', exact: true })
    .click();
  await itemForm.getByRole('button', { name: 'Agregar insumo surtido', exact: true }).click();
  await expect(notice(page, `Insumo agregado a «${supplierName}».`)).toBeVisible();
  await expect(supplied).toContainText(sodaName);
  await expect(supplied).toContainText('Presentación habitual: Caja con 24');
  await expect(supplied).toContainText('Código del proveedor: R-24');
  await expect(supplied.getByText('Preferido', { exact: true })).toBeVisible();
  await checkLayout(page, testInfo, 'proveedor-editor', true);
  await page.getByRole('button', { name: 'Volver al listado', exact: true }).click();
  const supplierRow = page.getByRole('row').filter({ hasText: supplierName });
  await expect(supplierRow).toContainText('DNE010203AB1');
  await expect(supplierRow).toContainText('1 insumo');
  await expect(supplierRow).toContainText('Sin recepciones');
  await checkLayout(page, testInfo, 'proveedores', true);

  // 2. Orden de 20 cajas a $250 con $50 de descuento e IVA 16 %: Go calcula los totales.
  await openTab(page, 'Órdenes de compra');
  await page.getByRole('button', { name: 'Nueva orden de compra', exact: true }).click();
  await chooseSupplier(page);
  const orderLine = page.getByRole('group', { name: 'Renglón 1' });
  await chooseItem(orderLine, sodaCode, `${sodaName} · ${sodaCode}`);
  // Se propone la presentación habitual con el proveedor.
  await expectSelectValue(
    orderLine.getByRole('combobox', { name: /^Unidad o presentación/ }),
    /^presentation:/,
  );
  await orderLine.getByLabel(/^Cantidad/).fill('20');
  await orderLine.getByLabel(/^Precio unitario sin IVA/).fill('250');
  await orderLine.getByLabel(/^Descuento comercial/).fill('50');
  await expect(orderLine.getByRole('button', { name: 'IVA 16 %', exact: true })).toHaveAttribute(
    'aria-pressed',
    'true',
  );
  await expect(page.locator('input[type="number"]')).toHaveCount(0);
  await page.getByRole('button', { name: 'Crear borrador', exact: true }).click();
  await expect(notice(page, /^Orden \d+ creada como borrador\.$/)).toBeVisible();
  const totals = page.getByRole('region', { name: 'Totales de la orden' });
  await expect(totals).toContainText('$4,950.00');
  await expect(totals).toContainText('−$50.00');
  await expect(totals).toContainText('$792.00');
  await expect(totals).toContainText('$5,742.00');
  await expect(orderLine.getByLabel('Importes del renglón')).toContainText('$5,000.00');
  await checkLayout(page, testInfo, 'orden-editor', true);
  await page.getByRole('button', { name: 'Emitir orden', exact: true }).click();
  await confirm(page, 'Emitir orden', '$5,742.00');
  await expect(
    notice(page, new RegExp(`^Orden \\d+ emitida a «${supplierName}»\\.$`)),
  ).toBeVisible();
  const orderPanel = page.getByRole('region', {
    name: new RegExp(`^Orden \\d+ · ${supplierName}$`),
  });
  await expect(orderPanel.locator('.tag', { hasText: 'Emitida' }).first()).toBeVisible();
  const orderRow = orderPanel.getByRole('row').filter({ hasText: sodaName });
  await expect(orderRow).toContainText('Caja con 24');

  // 3. Primera recepción: 12 de 20 cajas.
  await orderPanel.getByRole('button', { name: 'Registrar recepción', exact: true }).click();
  const receiveLine = page.getByRole('group', { name: `Renglón 1: ${sodaName}` });
  const received = receiveLine.getByLabel(/^Cantidad recibida/);
  await expect(received).toHaveValue('20');
  await received.fill('12');
  await page.getByLabel(/^Referencia del proveedor/).fill('F-E2E-1');
  await checkLayout(page, testInfo, 'recepcion-orden', true);
  await page.getByRole('button', { name: 'Registrar recepción', exact: true }).click();
  await confirm(page, 'Registrar recepción', '1 renglón');
  await expect(
    notice(page, /^Recepción \d+ registrada: 1 renglón en «Almacén general»\.$/),
  ).toBeVisible();
  let detail = page.getByRole('region', { name: new RegExp(`^Recepción \\d+ · ${supplierName}$`) });
  let quantities = detail.getByRole('table', { name: 'Cantidades recibidas' });
  let receiptRow = quantities.getByRole('row').filter({ hasText: sodaName });
  // Solicitado 20, recibido antes 0, recibido 12 (288 piezas), pendiente después 8.
  await expect(receiptRow.getByRole('cell').nth(2)).toHaveText('20');
  await expect(receiptRow.getByRole('cell').nth(3)).toHaveText('0');
  await expect(receiptRow.getByRole('cell').nth(4)).toContainText('12');
  await expect(receiptRow.getByRole('cell').nth(4)).toContainText('288 pza');
  await expect(receiptRow.getByRole('cell').nth(5)).toHaveText('8');
  await expect(detail).toContainText('IVA incluido en el costo');
  const amounts = detail.getByRole('table', { name: 'Importes de la recepción' });
  await expect(amounts).toContainText('$3,445.20');
  await expect(amounts).toContainText('−$30.00');
  await expect(amounts).toContainText('$11.9625 / pza');
  await checkLayout(page, testInfo, 'recepcion-detalle', true);
  await detail.getByRole('button', { name: /^Volver a la orden \d+$/ }).click();
  await expect(
    orderPanel.locator('.tag', { hasText: 'Parcialmente recibida' }).first(),
  ).toBeVisible();
  await expect(orderRow.getByRole('cell').nth(3)).toHaveText('12');
  await expect(orderRow.getByRole('cell').nth(4)).toContainText('8');

  // 4. Segunda recepción: las 8 cajas pendientes (propuestas por Go) completan la orden.
  await orderPanel.getByRole('button', { name: 'Registrar recepción', exact: true }).click();
  await expect(
    page.getByRole('group', { name: `Renglón 1: ${sodaName}` }).getByLabel(/^Cantidad recibida/),
  ).toHaveValue('8');
  await page.getByRole('button', { name: 'Registrar recepción', exact: true }).click();
  await confirm(page, 'Registrar recepción');
  await expect(
    notice(page, /^Recepción \d+ registrada: 1 renglón en «Almacén general»\.$/),
  ).toBeVisible();
  detail = page.getByRole('region', { name: new RegExp(`^Recepción \\d+ · ${supplierName}$`) });
  quantities = detail.getByRole('table', { name: 'Cantidades recibidas' });
  receiptRow = quantities.getByRole('row').filter({ hasText: sodaName });
  await expect(receiptRow.getByRole('cell').nth(3)).toHaveText('12');
  await expect(receiptRow.getByRole('cell').nth(5)).toHaveText('0');
  await expect(detail.getByRole('table', { name: 'Importes de la recepción' })).toContainText(
    '$2,296.80',
  );
  await detail.getByRole('button', { name: /^Volver a la orden \d+$/ }).click();
  await expect(orderPanel.locator('.tag', { hasText: 'Recibida' }).first()).toBeVisible();
  await expect(orderRow.getByRole('cell').nth(3)).toHaveText('20');
  await expect(orderRow.getByRole('cell').nth(4)).toContainText('0');
  await expect(
    orderPanel.getByRole('table', { name: 'Recepciones de la orden' }).getByRole('row'),
  ).toHaveCount(3);
  await orderPanel.getByRole('button', { name: 'Volver al listado', exact: true }).click();
  const listRow = page.getByRole('row').filter({ hasText: supplierName });
  await expect(listRow).toContainText('Recibida');
  await expect(listRow).toContainText('$5,742.00');
  await expect(listRow).toContainText('2 recepciones');
  await checkLayout(page, testInfo, 'compras', true);

  // 5. Existencia y movimientos: 20 cajas = 480 piezas en dos entradas por compra.
  await openTab(page, 'Existencias');
  const stockRow = page.getByRole('row').filter({ hasText: sodaName });
  await expect(stockRow).toContainText('480 pza');
  await openTab(page, 'Movimientos');
  const movementRows = page.getByRole('row').filter({ hasText: sodaName });
  await expect(movementRows).toHaveCount(2);
  await expect(movementRows.filter({ hasText: '+288 pza' })).toContainText('Entrada por compra');
  await expect(movementRows.filter({ hasText: '+192 pza' })).toContainText(
    'Recepción de mercancía',
  );
  const stock = await driver.request('GET', `inventory/stock?search=${sodaCode}`);
  expect(stock.body.items[0].quantity).toBe('480');
  const movements = await driver.request(
    'GET',
    `inventory/movements?kind=receipt&item_id=${prepared.sodaID}`,
  );
  expect(movements.body.total).toBe(2);

  // 6. Recepción directa (sin orden): 50 vasos a $2.00 con IVA 16 % = $116.00.
  await openTab(page, 'Recepciones');
  await page.getByRole('button', { name: 'Recepción directa', exact: true }).click();
  const directPanel = page.getByRole('region', { name: 'Recepción directa' });
  await chooseSupplier(directPanel);
  const directLine = directPanel.getByRole('group', { name: 'Renglón 1' });
  await chooseItem(directLine, cupCode, `${cupName} · ${cupCode}`);
  await directLine.getByLabel(/^Cantidad/).fill('50');
  await directLine.getByLabel(/^Precio unitario sin IVA/).fill('2');
  await directPanel.getByLabel(/^Referencia del proveedor/).fill('Nota 77');
  await expect(page.locator('input[type="number"]')).toHaveCount(0);
  await checkLayout(page, testInfo, 'recepcion-directa', true);
  await directPanel.getByRole('button', { name: 'Registrar recepción', exact: true }).click();
  await confirm(page, 'Registrar recepción', `recepción directa de «${supplierName}»`);
  await expect(
    notice(page, /^Recepción \d+ registrada: 1 renglón en «Almacén general»\.$/),
  ).toBeVisible();
  detail = page.getByRole('region', { name: new RegExp(`^Recepción \\d+ · ${supplierName}$`) });
  await expect(detail).toContainText('Recepción directa');
  await expect(detail.getByRole('region', { name: 'Totales de la recepción' })).toContainText(
    '$116.00',
  );
  const direct = await driver.request('GET', 'inventory/receipts?direct=true');
  expect(direct.body.total).toBe(1);
  expect(direct.body.items[0].total).toBe('116.00');
  const directID = direct.body.items[0].id as string;

  // 7. Vínculo total con el gasto de $116.00 registrado en el turno abierto.
  const links = detail.getByRole('region', { name: 'Vínculo con gastos de caja' });
  await expect(links).toContainText('La recepción no está vinculada con ningún gasto.');
  await links.getByRole('button', { name: 'Vincular con un gasto', exact: true }).click();
  const candidates = links.getByRole('region', { name: 'Gastos de caja para vincular' });
  const expenseRow = candidates.getByRole('row').filter({ hasText: expenseConcept });
  await expect(expenseRow).toContainText('$116.00');
  await expect(expenseRow).toContainText('Se puede vincular');
  await expenseRow.getByRole('button', { name: /^Elegir el gasto/ }).click();
  const linkForm = links.getByRole('form', { name: 'Vincular el gasto elegido' });
  await linkForm
    .getByRole('button', { name: 'Por vincular de la recepción: $116.00', exact: true })
    .click();
  await expect(linkForm.getByLabel(/^Importe a vincular/)).toHaveValue('116.00');
  await checkLayout(page, testInfo, 'vinculo-gasto', true);
  await linkForm.getByRole('button', { name: 'Vincular gasto', exact: true }).click();
  await confirm(page, 'Vincular gasto', 'no modifica el gasto ni el turno');
  await expect(notice(page, /^Gasto vinculado con la recepción \d+\.$/)).toBeVisible();
  const linkRow = links.getByRole('row').filter({ hasText: expenseConcept });
  await expect(linkRow).toContainText('Vigente');
  await expect(linkRow).toContainText('$116.00');
  await expect(links.locator('.inventory-facts')).toContainText('Por vincular$0.00');
  await checkLayout(page, testInfo, 'vinculo-registrado', true);
  const candidatesAfter = await driver.request(
    'GET',
    `inventory/cash-expenses?receipt_id=${directID}`,
  );
  const linkedExpense = (
    candidatesAfter.body.items as { id: string; linked_amount: string; linkable: boolean }[]
  ).find((entry) => entry.id === prepared.expenseID);
  expect(linkedExpense?.linked_amount).toBe('116.00');
  expect(linkedExpense?.linkable).toBe(false);

  // 8. Listado de recepciones: las tres, con el vínculo de la directa.
  await detail.getByRole('button', { name: 'Volver a recepciones', exact: true }).click();
  await expect(page.getByRole('row').filter({ hasText: supplierName })).toHaveCount(3);
  await expect(page.getByRole('row').filter({ hasText: 'Directa' })).toContainText('$116.00');
  await checkLayout(page, testInfo, 'recepciones', true);
  expect(errors).toEqual([]);
});
