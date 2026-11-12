import { randomUUID } from 'node:crypto';
import { expect, test } from '@playwright/test';
import type { Locator, Page, TestInfo } from '@playwright/test';
import {
  checkInventoryNavigation,
  groupTablist,
  inventoryGroups,
  openInventorySection,
} from './inventory-navigation';
import { installDriver } from './native-driver';
import { openAdministration, topBar } from './navigation';
import { notice } from './notices';
import { chooseOption, expectSelectText } from './select';

// Etapa 9A, fase A (A2): navegación agrupada de Inventarios, alta de un insumo con el código
// sugerido por Go y una categoría creada desde su editor, consulta de la categoría y una recepción
// directa con «Compras de mostrador» con el comercio concreto. Mismo transporte de prueba que las
// demás especificaciones (Go temporal y base adel_pos_test_*). El nombre del archivo la ordena
// después de las especificaciones existentes en la corrida completa: su compra directa no altera
// los conteos que ellas comprueban.

type Driver = Awaited<ReturnType<typeof installDriver>>;

const itemName = 'Queso Oaxaca 9A';
const takenCode = 'queso-oaxaca-9a';
const suggestedCode = 'queso-oaxaca-9a-2';
const categoryName = 'Lácteos 9A';
const storeName = 'Central de abastos E2E, bodega 12';

async function setTheme(page: Page, theme: string) {
  await page.evaluate((value) => {
    document.documentElement.dataset.theme = value;
  }, theme);
  await expect(page.locator('html')).toHaveAttribute('data-theme', theme);
  await page.waitForTimeout(150);
}

// La página no se desborda a lo ancho a 1920 × 1080 ni a 1280 × 720, en tema claro y oscuro; con
// «capture» guarda la vista a 1280 × 720 en ambos temas.
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
      if (capture && size.width === 1280) {
        await expect(page.locator('[data-sonner-toast]')).toHaveCount(0, { timeout: 9000 });
        await setTheme(page, theme);
        await page.screenshot({
          path: testInfo.outputPath(`9a-${name}-${theme}-1280x720.png`),
          fullPage: true,
        });
      }
    }
  }
  await setTheme(page, 'light');
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

// Precondición por la API: un insumo que ya usa el código que se sugeriría para el nuevo, para
// comprobar que Go propone el siguiente libre.
async function preparePreconditions(driver: Driver) {
  const units = await driver.request('GET', 'inventory/units');
  const kilogram = (units.body as { id: string; code: string }[]).find(
    (unit) => unit.code === 'kg',
  );
  expect(kilogram, 'unidad kg').toBeTruthy();
  await created(driver, 'inventory/items', {
    code: takenCode,
    name: 'Queso Oaxaca anterior 9A',
    description: '',
    base_unit_id: kilogram!.id,
    kind: 'purchased',
    active: true,
  });
}

async function chooseItem(line: Locator, code: string, label: string) {
  const select = line.getByRole('combobox', { name: /^Insumo/ });
  await chooseOption(select, label, { search: code });
}

test('inventarios 9A: navegación agrupada, código sugerido, categoría desde el insumo y compra de mostrador', async ({
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
  await preparePreconditions(driver);
  await openAdministration(page, 'Inventarios');
  await expect(page.getByRole('heading', { name: 'Inventarios', exact: true })).toBeVisible();

  // 1. Navegación agrupada: cinco grupos y sus secciones, cada fila en una sola línea, sin
  // desbordar a 1920 × 1080 ni a 1280 × 720, en ambos temas.
  await expect(groupTablist(page).getByRole('tab')).toHaveText(inventoryGroups);
  for (const size of [
    { width: 1920, height: 1080 },
    { width: 1280, height: 720 },
  ]) {
    for (const theme of ['light', 'dark']) {
      await setTheme(page, theme);
      await checkInventoryNavigation(page, size, async (name) => {
        if (size.width === 1280) {
          await setTheme(page, theme);
          await page.getByRole('navigation', { name: 'Inventarios' }).screenshot({
            path: testInfo.outputPath(`9a-${name}-${theme}-1280x720.png`),
          });
        }
      });
    }
  }
  await setTheme(page, 'light');
  // Cada grupo recuerda su última sección.
  await openInventorySection(page, 'Conteos');
  await openInventorySection(page, 'Recepciones');
  await groupTablist(page).getByRole('tab', { name: 'Existencias', exact: true }).click();
  await expect(
    page
      .getByRole('tablist', { name: 'Apartados de Existencias' })
      .getByRole('tab', { name: 'Conteos', exact: true }),
  ).toHaveAttribute('aria-selected', 'true');

  // 2. Insumo nuevo: el código se sugiere desde el nombre y Go propone el libre (el base ya existe).
  await openInventorySection(page, 'Insumos');
  await page.getByRole('button', { name: 'Nuevo insumo', exact: true }).click();
  await page.getByLabel('Nombre del insumo', { exact: true }).fill(itemName);
  const code = page.getByLabel(/^Código interno/);
  await expect(code).toHaveValue(suggestedCode);
  await chooseOption(page.getByRole('combobox', { name: /^Unidad base/ }), 'Kilogramo (kg)');
  await page.getByLabel(/^Existencia mínima/).fill('3');
  await page.getByLabel(/^Descripción/).fill('Para quesadillas');

  // 3. «+ Nueva categoría» desde el selector: se crea sin salir del insumo, queda seleccionada y
  // lo capturado se conserva; el insumo todavía no se crea.
  await chooseOption(page.getByRole('combobox', { name: /^Categoría/ }), '+ Nueva categoría');
  const dialog = page.getByRole('dialog', { name: 'Nueva categoría de insumos' });
  await expect(dialog).toBeVisible();
  await dialog.getByLabel(/^Nombre de la categoría/).fill(categoryName);
  await checkLayout(page, testInfo, 'categoria-rapida', true);
  await dialog.getByRole('button', { name: 'Crear y seleccionar', exact: true }).click();
  await expect(notice(page, `Categoría «${categoryName}» creada y seleccionada.`)).toBeVisible();
  await expect(dialog).toHaveCount(0);
  const category = page.getByRole('combobox', { name: /^Categoría/ });
  await expectSelectText(category, categoryName);
  await expect(page.getByLabel('Nombre del insumo', { exact: true })).toHaveValue(itemName);
  await expect(code).toHaveValue(suggestedCode);
  await expect(page.getByLabel(/^Descripción/)).toHaveValue('Para quesadillas');
  const before = await driver.request('GET', `inventory/items?search=${suggestedCode}`);
  expect(before.body.total).toBe(0);
  await page.getByRole('button', { name: 'Crear insumo', exact: true }).click();
  await expect(notice(page, 'Insumo creado. Ya puedes agregar sus presentaciones.')).toBeVisible();
  const saved = await driver.request('GET', `inventory/items?search=${suggestedCode}`);
  expect(saved.body.total).toBe(1);
  expect(saved.body.items[0].code).toBe(suggestedCode);
  expect(saved.body.items[0].category_name).toBe(categoryName);
  await page.getByRole('button', { name: 'Volver al listado', exact: true }).click();

  // 4. Configuración → Categorías: búsqueda y «Ver insumos» abre Insumos filtrado.
  await openInventorySection(page, 'Categorías');
  await page.getByLabel('Buscar categoría', { exact: true }).fill('9a');
  const categoryRow = page.getByRole('row').filter({ hasText: categoryName });
  await expect(categoryRow).toContainText('1 insumo');
  await checkLayout(page, testInfo, 'categorias', true);
  await categoryRow.getByRole('button', { name: `Ver los insumos de ${categoryName}` }).click();
  await expect(
    page
      .getByRole('tablist', { name: 'Apartados de Catálogo' })
      .getByRole('tab', { name: 'Insumos', exact: true }),
  ).toHaveAttribute('aria-selected', 'true');
  await expect(page.getByRole('row').filter({ hasText: itemName })).toBeVisible();
  await expect(page.getByRole('row').filter({ hasText: 'Queso Oaxaca anterior 9A' })).toHaveCount(
    0,
  );

  // 5. Compras → Recepciones: compra de mostrador preseleccionada, con el comercio concreto.
  await openInventorySection(page, 'Recepciones');
  await page.getByRole('button', { name: 'Compra de mostrador', exact: true }).click();
  const panel = page.getByRole('region', { name: 'Recepción directa' });
  await expect(
    panel.getByRole('button', { name: 'Compras de mostrador', exact: true }),
  ).toHaveAttribute('aria-pressed', 'true');
  await expect(panel.getByRole('combobox', { name: /^Proveedor/ })).toHaveCount(0);
  await panel.getByLabel(/^Comercio/).fill(storeName);
  const line = panel.getByRole('group', { name: 'Renglón 1' });
  await chooseItem(line, suggestedCode, `${itemName} · ${suggestedCode}`);
  await line.getByLabel(/^Cantidad/).fill('2');
  await line.getByLabel(/^Precio unitario sin IVA/).fill('80');
  await line.getByRole('button', { name: 'IVA 0 %', exact: true }).click();
  await expect(page.locator('input[type="number"]')).toHaveCount(0);
  await checkLayout(page, testInfo, 'compra-mostrador', true);
  await panel.getByRole('button', { name: 'Registrar recepción', exact: true }).click();
  await confirm(page, 'Registrar recepción', `«Compras de mostrador» (${storeName})`);
  await expect(
    notice(page, /^Recepción \d+ registrada: 1 renglón en «Almacén general»\.$/),
  ).toBeVisible();
  const detail = page.getByRole('region', { name: /^Recepción \d+ · Compras de mostrador$/ });
  await expect(detail).toContainText('Compra de mostrador');
  await expect(detail).toContainText(storeName);
  await expect(detail.getByRole('region', { name: 'Totales de la recepción' })).toContainText(
    '$160.00',
  );
  await page.getByRole('button', { name: 'Volver a recepciones', exact: true }).click();
  await chooseOption(page.getByRole('combobox', { name: /^Tipo de recepción/ }), {
    value: 'counter',
  });
  const counterRow = page.getByRole('row').filter({ hasText: `Mostrador · ${storeName}` });
  await expect(counterRow).toContainText('$160.00');
  await checkLayout(page, testInfo, 'recepciones-mostrador', true);

  // 6. Proveedores: «Compras de mostrador» se distingue y no lleva RFC ni insumos surtidos.
  await openInventorySection(page, 'Proveedores');
  const counterSupplier = page.getByRole('row').filter({ hasText: 'Compras de mostrador' }).first();
  await expect(counterSupplier).toBeVisible();
  await counterSupplier.getByRole('button', { name: /^Editar el proveedor/ }).click();
  await expect(page.getByLabel(/^RFC/)).toHaveCount(0);
  await expect(page.getByRole('region', { name: 'Insumos que surte' })).toHaveCount(0);
  await expect(page.getByText(/Proveedor especial para compras ocasionales/)).toBeVisible();
  await page.getByRole('button', { name: 'Volver al listado', exact: true }).click();

  // 7. Devoluciones a proveedor: la lista nueva del grupo Compras.
  await openInventorySection(page, 'Devoluciones a proveedor');
  await expect(
    page.getByRole('region', { name: 'Filtros de devoluciones a proveedor' }),
  ).toBeVisible();
  await checkLayout(page, testInfo, 'devoluciones', false);

  // Go confirma lo que mostró la pantalla: la recepción es del proveedor de mostrador, con su
  // comercio, y no registró el último precio.
  const counterReceipts = await driver.request('GET', 'inventory/receipts?supplier_kind=counter');
  expect(counterReceipts.body.total).toBeGreaterThanOrEqual(1);
  const latest = counterReceipts.body.items[0];
  expect(latest.counter_store).toBe(storeName);
  expect(latest.supplier.kind).toBe('counter');
  const supplier = await driver.request('GET', `inventory/suppliers/${latest.supplier.id}`);
  expect(supplier.body.items).toEqual([]);
  expect(errors).toEqual([]);
});
