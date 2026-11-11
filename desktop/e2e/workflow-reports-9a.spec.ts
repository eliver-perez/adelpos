import { randomUUID } from 'node:crypto';
import { expect, test } from '@playwright/test';
import type { Page, TestInfo } from '@playwright/test';
import { formatMoney } from '../src/services/money';
import { installDriver } from './native-driver';
import { notice } from './notices';

// Etapa 9A, parte C2: módulo Reportes contra Go y PostgreSQL reales. Se crean por la API dos
// cuentas cobradas y cerradas con un producto propio (tarjeta de débito y efectivo); la interfaz
// muestra el resumen ejecutivo con esas ventas, abre el reporte detallado desde una tarjeta con su
// tendencia, filtra las ventas por periodo (semana actual y semana del año ISO con su rango) y
// consulta y exporta (PDF y Excel, con el diálogo simulado de la parte C1) un reporte de pagos.
// Todas las cifras que se comprueban las calcula Go (se comparan con la respuesta de la API, nunca
// con sumas propias). Sin desbordamiento horizontal a 1280 × 720 ni a 1920 × 1080, en tema claro y
// oscuro. El nombre del archivo la ordena después de las especificaciones existentes en la corrida
// completa: sus ventas no alteran los conteos que ellas comprueban.

type Driver = Awaited<ReturnType<typeof installDriver>>;

const productName = 'Agua de jamaica 9A';
const productCode = 'JAMAICA-9A';

async function setTheme(page: Page, theme: string) {
  await page.evaluate((value) => {
    document.documentElement.dataset.theme = value;
  }, theme);
  await expect(page.locator('html')).toHaveAttribute('data-theme', theme);
  await page.waitForTimeout(150);
}

// La página no se desborda a lo ancho en los dos tamaños y los dos temas; con «capture» guarda la
// vista completa en cada combinación.
async function checkLayout(page: Page, testInfo: TestInfo, name: string, capture: boolean) {
  for (const size of [
    { width: 1920, height: 1080 },
    { width: 1280, height: 720 },
  ]) {
    await page.setViewportSize(size);
    for (const theme of ['light', 'dark']) {
      await setTheme(page, theme);
      const overflow = await page.evaluate(
        () => document.documentElement.scrollWidth - window.innerWidth,
      );
      expect(overflow, `${name} se desborda a ${size.width} × ${size.height} (${theme})`).toBe(0);
      if (capture) {
        await expect(page.locator('[data-sonner-toast]')).toHaveCount(0, { timeout: 9000 });
        await setTheme(page, theme);
        // Desde el inicio de la página: los elementos fijos y la lista lateral quedan en su lugar.
        await page.evaluate(() => window.scrollTo(0, 0));
        await page.screenshot({
          path: testInfo.outputPath(
            `9a-reportes-${name}-${theme}-${size.width}x${size.height}.png`,
          ),
          fullPage: true,
        });
      }
    }
  }
  await page.setViewportSize({ width: 1280, height: 720 });
  await setTheme(page, 'light');
}

// Abre un reporte de un grupo: por debajo de 1440 px con el selector «Reporte» y en pantallas anchas
// con la lista lateral (solo uno de los dos es visible).
async function openReport(page: Page, group: string, title: string) {
  await page
    .getByRole('tablist', { name: 'Grupos de reportes' })
    .getByRole('tab', { name: group, exact: true })
    .click();
  const picker = page.getByRole('combobox', { name: 'Reporte', exact: true });
  if (await picker.isVisible()) {
    await picker.selectOption({ label: title });
  } else {
    await page
      .getByRole('navigation', { name: `Reportes de ${group}` })
      .getByRole('button', { name: title, exact: true })
      .click();
  }
  await expect(page.getByRole('heading', { name: title, exact: true })).toBeVisible();
}

async function call(driver: Driver, method: string, route: string, body?: unknown) {
  const response = await driver.request(method, route, body);
  expect(response.status, `${method} ${route}: ${JSON.stringify(response.body)}`).toBe(200);
  return response.body;
}

// Cuenta sin mesa con el producto de la prueba, confirmada y liquidada con un solo pago total (queda
// cerrada: la venta se reconoce en el día comercial del cierre).
async function closeAccount(
  driver: Driver,
  productID: string,
  quantity: number,
  expectedBalance: string,
  payment: Record<string, string>,
) {
  const opened = await call(driver, 'POST', 'restaurant/accounts', {
    operation_id: randomUUID(),
    mode: 'dine_in',
    name: 'Reportes 9A',
  });
  const accountID = opened.account.id as string;
  const route = `restaurant/accounts/${accountID}`;
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
  const finance = await call(driver, 'GET', `payments/accounts/${accountID}`);
  expect(finance.balance).toBe(expectedBalance);
  const paid = await call(driver, 'POST', `payments/accounts/${accountID}/payments`, {
    operation_id: randomUUID(),
    mode: 'total',
    expected_balance: finance.balance,
    ...payment,
  });
  expect(paid.closed, JSON.stringify(paid)).toBe(true);
}

// Precondición por la API: un producto propio, el turno abierto de la caja de la terminal y dos
// cuentas cerradas: dos unidades pagadas con tarjeta de débito ($70.00) y una en efectivo ($35.00),
// para que los cobros por método tengan una distribución.
async function closedSales(driver: Driver) {
  const category = await call(driver, 'POST', 'catalog/categories', {
    name: 'Bebidas 9A',
    active: true,
    color: '#1f9a78',
  });
  const product = await call(driver, 'POST', 'catalog/products', {
    name: productName,
    active: true,
    category_id: category.id,
    code: productCode,
    kind: 'product',
    base_price: '35',
  });
  const current = await call(driver, 'GET', 'payments/shifts/current');
  expect(current.register, JSON.stringify(current)).toBeTruthy();
  let shift = current.shift as { id: string; state: string } | null;
  if (!shift) {
    shift = await call(driver, 'POST', 'payments/shifts', {
      operation_id: randomUUID(),
      opening_float: '500',
    });
  }
  expect(shift!.state).toBe('open');
  const methods = (await call(driver, 'GET', 'payments/methods')) as {
    id: string;
    code: string | null;
  }[];
  const debit = methods.find((method) => method.code === 'debit');
  const cash = methods.find((method) => method.code === 'cash');
  expect(debit, 'método de débito').toBeTruthy();
  expect(cash, 'método de efectivo').toBeTruthy();
  await closeAccount(driver, product.id, 2, '70.00', { method_id: debit!.id });
  await closeAccount(driver, product.id, 1, '35.00', { method_id: cash!.id, received: '35' });
}

test('reportes 9A: resumen ejecutivo con una venta real, reporte desde una tarjeta, periodos y pagos', async ({
  page,
}, testInfo) => {
  test.setTimeout(300000);
  const errors: string[] = [];
  page.on('pageerror', (error) => errors.push(error.message));
  const driver = await installDriver(page);
  await driver.request('POST', 'auth/login', { username: 'admin', password: 'admin' });
  // En la corrida completa restaurant.spec.ts deja el teclado virtual siempre visible.
  const keyboard = await driver.request('PUT', 'terminal/preferences', {
    keyboard_mode: 'disabled',
  });
  expect(keyboard.status, JSON.stringify(keyboard.body)).toBeLessThan(300);
  await closedSales(driver);
  await driver.request('POST', 'auth/logout');
  await page.setViewportSize({ width: 1280, height: 720 });
  await page.goto('/');
  await page.getByLabel('Usuario', { exact: true }).fill('admin');
  await page.getByLabel('Contraseña', { exact: true }).fill('admin');
  await page.getByRole('button', { name: 'Entrar', exact: true }).click();
  const reports = page
    .getByRole('navigation', { name: 'Navegación principal' })
    .getByRole('button', { name: 'Reportes', exact: true });
  await expect(reports).toBeVisible();
  await reports.click();
  await expect(page.getByRole('heading', { name: 'Reportes', exact: true })).toBeVisible();

  // 1. Resumen ejecutivo: los grupos salen del catálogo de Go y la tarjeta «Ventas netas» muestra
  // la cifra que Go calcula para hoy (incluye la cuenta cerrada de esta prueba).
  const catalog = await call(driver, 'GET', 'reports/catalog');
  const titles = [
    'Resumen ejecutivo',
    ...catalog.groups.map((group: { title: string }) => group.title),
  ];
  await expect(
    page.getByRole('tablist', { name: 'Grupos de reportes' }).getByRole('tab'),
  ).toHaveText(titles);
  await expect(page.getByRole('heading', { name: 'Resumen ejecutivo', exact: true })).toBeVisible();
  const dashboard = await call(driver, 'GET', 'reports/dashboard?period=today&compare=previous');
  const salesSection = dashboard.sections.find((section: { id: string }) => section.id === 'sales');
  const net = salesSection.indicators.find(
    (indicator: { key: string }) => indicator.key === 'net_sales',
  );
  expect(Number(net.value)).toBeGreaterThanOrEqual(105);
  const sales = page.getByRole('region', { name: 'Ventas', exact: true });
  const netCard = sales.getByRole('button', { name: /^Ventas netas/ });
  await expect(netCard).toContainText(formatMoney(net.value));
  await expect(netCard).toContainText('frente al periodo anterior');
  await expect(page.getByText(dashboard.period.label).first()).toBeVisible();
  await expect(page.getByText(/Actualizado a las/)).toBeVisible();
  // «Actualizar» vuelve a consultar sin cambiar de pantalla.
  await page.getByRole('button', { name: 'Actualizar', exact: true }).click();
  await expect(netCard).toContainText(formatMoney(net.value));
  await expect(page.locator('input[type="number"]')).toHaveCount(0);
  await checkLayout(page, testInfo, 'resumen', true);

  // 2. La tarjeta abre su reporte detallado con el mismo periodo; el pie de la tabla trae el total
  // de Go.
  await netCard.click();
  await expect(page.getByRole('heading', { name: 'Ventas por día', exact: true })).toBeVisible();
  await expect(
    page.getByRole('tablist', { name: 'Grupos de reportes' }).getByRole('tab', { name: 'Ventas' }),
  ).toHaveAttribute('aria-selected', 'true');
  const byDay = await call(driver, 'GET', 'reports/run/sales.by_day?period=today');
  const table = page.getByRole('region', { name: 'Tabla: Ventas por día' });
  await expect(table.locator('tfoot')).toContainText(formatMoney(byDay.totals.net));
  await expect(page.getByText(byDay.meta.date_basis)).toBeVisible();
  // Con la semana actual (siete días comerciales, incluidos los días sin ventas) el reporte dibuja
  // la tendencia; su tabla alternativa lista los mismos días.
  await page.getByRole('button', { name: 'Semana actual', exact: true }).click();
  const byWeekDay = await call(driver, 'GET', 'reports/run/sales.by_day?period=current_week');
  await expect(table.locator('tfoot')).toContainText(formatMoney(byWeekDay.totals.net));
  const trend = page.getByRole('figure', { name: 'Venta neta por día' });
  await expect(trend).toBeVisible();
  await expect(trend.getByRole('img')).toHaveAttribute(
    'aria-label',
    /Venta neta por día: 7 puntos/,
  );
  await checkLayout(page, testInfo, 'ventas-dia', true);
  await trend.getByRole('button', { name: 'Ver como tabla', exact: true }).click();
  await expect(trend.getByRole('region', { name: 'Datos de Venta neta por día' })).toContainText(
    formatMoney(byWeekDay.rows.find((row: { net: string }) => row.net !== '0.00')?.net ?? '0.00'),
  );
  await trend.getByRole('button', { name: 'Ver gráfico', exact: true }).click();

  // 3. Ventas por producto en la semana actual: el renglón del producto de la prueba con la venta
  // neta de Go y el periodo aplicado que resolvió Go.
  // En pantalla ancha los reportes del grupo se listan a un lado; a 1280 × 720 se eligen con el
  // selector «Reporte» para dejar el ancho a la tabla.
  await page.setViewportSize({ width: 1920, height: 1080 });
  await expect(page.getByRole('navigation', { name: 'Reportes de Ventas' })).toBeVisible();
  await expect(page.getByRole('combobox', { name: 'Reporte', exact: true })).toBeHidden();
  await page.setViewportSize({ width: 1280, height: 720 });
  await expect(page.getByRole('navigation', { name: 'Reportes de Ventas' })).toBeHidden();
  await openReport(page, 'Ventas', 'Ventas por producto');
  await page.getByRole('button', { name: 'Semana actual', exact: true }).click();
  const weekly = await call(driver, 'GET', 'reports/run/sales.by_product?period=current_week');
  const productRow = page.getByRole('row').filter({ hasText: productName });
  await expect(productRow).toBeVisible();
  const goRow = weekly.rows.find((row: { product: string }) => row.product === productName);
  expect(goRow?.net).toBe('105.00');
  await expect(productRow).toContainText('$105.00');
  const filters = page.getByRole('region', { name: 'Filtros del reporte' });
  await expect(filters).toContainText(weekly.meta.period.label);
  await checkLayout(page, testInfo, 'ventas-producto', true);

  // 4. Semana del año (ISO 8601): se completa con la semana comercial de Go, muestra su rango y Go
  // devuelve la misma semana.
  await page.getByRole('button', { name: 'Semana del año', exact: true }).click();
  const isoWeek = await call(
    driver,
    'GET',
    `reports/run/sales.by_product?period=iso_week&week=${weekly.meta.period.iso_year}-W${String(weekly.meta.period.iso_week).padStart(2, '0')}`,
  );
  await expect(filters).toContainText(isoWeek.meta.period.label);
  await expect(filters).toContainText(
    `Semana ${weekly.meta.period.iso_week} de ${weekly.meta.period.iso_year}: del`,
  );
  await expect(productRow).toContainText('$105.00');
  await checkLayout(page, testInfo, 'periodo', true);
  await page.getByRole('button', { name: 'Hoy', exact: true }).click();

  // 5. Pagos: cobros por método del día con el importe de Go.
  await openReport(page, 'Pagos', 'Cobros por método');
  const byMethod = await call(driver, 'GET', 'reports/run/payments.by_method?period=today');
  const paymentsTable = page.getByRole('region', { name: 'Tabla: Cobros por método' });
  await expect(paymentsTable.locator('tfoot')).toContainText(formatMoney(byMethod.totals.net));
  // Con pocos métodos, la distribución del cobrado neto se dibuja como barra segmentada.
  if (byMethod.rows.length >= 2 && byMethod.rows.length <= 5) {
    await expect(page.getByRole('figure', { name: 'Distribución de cobrado neto' })).toBeVisible();
  }
  await checkLayout(page, testInfo, 'pagos', true);

  // 6. Exportación PDF y Excel con la operación export_report y el diálogo «Guardar como» simulado
  // en native-driver.ts (parte C1): el navegador solo recibe el resultado y el aviso lo comunica;
  // cancelar el diálogo no guarda nada.
  const exportGroup = page.getByRole('group', { name: 'Exportar' });
  await exportGroup.getByRole('button', { name: 'Exportar PDF', exact: true }).click();
  await expect(notice(page, 'Cobros por método: PDF guardado.')).toBeVisible({ timeout: 30000 });
  await exportGroup.getByRole('button', { name: 'Exportar Excel', exact: true }).click();
  await expect(notice(page, 'Cobros por método: Excel guardado.')).toBeVisible({
    timeout: 30000,
  });
  expect(driver.exports.map((entry) => [entry.report, entry.format])).toEqual([
    ['payments.by_method', 'pdf'],
    ['payments.by_method', 'xlsx'],
  ]);
  driver.nextSaveDialog('cancel');
  await exportGroup.getByRole('button', { name: 'Exportar PDF', exact: true }).click();
  await expect(
    notice(page, 'Exportación a PDF cancelada: no se guardó ningún archivo.'),
  ).toBeVisible({ timeout: 30000 });
  expect(driver.exports).toHaveLength(2);
  expect(errors).toEqual([]);
});
