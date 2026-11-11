import { randomUUID } from 'node:crypto';
import { expect, test } from '@playwright/test';
import type { Locator, Page, TestInfo } from '@playwright/test';
import { installDriver } from './native-driver';

// Etapa 9A, parte A1. (1) §3.1: un producto con «El precio puede bajar al cambiar complementos»
// baja de precio al sustituir un complemento por uno más barato, con la diferencia con signo que
// calcula Go. (2) §33: «Poner fuera de servicio» cabe en una línea en el panel de la mesa a
// 1280 × 720 y 1920 × 1080. Mismo transporte de prueba que restaurant.spec.ts (Go temporal y base
// adel_pos_test_*); el catálogo, el área y la mesa se crean por la API con la sesión del
// administrador, con nombres propios. El nombre del archivo lo ordena después de restaurant.spec.ts,
// que espera la cuenta #1 en una base recién preparada.

type Driver = Awaited<ReturnType<typeof installDriver>>;

async function create(driver: Driver, route: string, body: Record<string, unknown>) {
  const response = await driver.request('POST', route, body);
  expect(response.status, `${route}: ${JSON.stringify(response.body)}`).toBe(200);
  return response.body as {
    id: string;
    revision: number;
    price_can_decrease?: boolean;
    options?: { id: string; complement_id: string }[];
  };
}

// Filete de $150 con arroz rojo ($20) y ensalada mixta ($15) incluidos, hasta dos porciones de guarnición y la
// opción activada (ejemplo del propietario: incluidos $35, elegidos $25 → $140).
async function prepareCatalog(driver: Driver) {
  const category = await create(driver, 'catalog/categories', {
    name: 'Precio flexible',
    active: true,
    color: '#1f9a78',
  });
  const complements: { id: string }[] = [];
  for (const name of ['Arroz rojo', 'Ensalada mixta', 'Puré de papa', 'Frijoles de olla']) {
    complements.push(
      await create(driver, 'catalog/complements', {
        name,
        active: true,
        product_id: null,
        ingredient_id: null,
      }),
    );
  }
  const values = ['20', '15', '25', '10'];
  const sides = await create(driver, 'catalog/groups', {
    name: 'Guarniciones del filete',
    active: true,
    pricing_mode: 'value',
    min_quantity: '0',
    max_quantity: '2',
    options: complements.map((complement, index) => ({
      complement_id: complement.id,
      portion_id: null,
      reference_value: values[index],
      extra_price: '0',
      max_quantity: null,
      active: true,
      tiers: [],
    })),
  });
  const optionOf = (index: number) =>
    sides.options!.find((entry) => entry.complement_id === complements[index]!.id)!.id;
  const product = await create(driver, 'catalog/products', {
    name: 'Filete flexible',
    active: true,
    category_id: category.id,
    code: 'FILETE-FLEX-9A',
    kind: 'product',
    base_price: '150',
    price_can_decrease: true,
    groups: [
      {
        group_id: sides.id,
        min_quantity: null,
        max_quantity: null,
        included: [
          { option_id: optionOf(0), quantity: '1' },
          { option_id: optionOf(1), quantity: '1' },
        ],
      },
    ],
  });
  expect(product.price_can_decrease).toBe(true);
}

// Número de líneas que ocupa el texto de un botón (un rectángulo por línea del texto).
async function textLines(button: Locator) {
  return button.locator('.icon-label-text').evaluate((element) => {
    const range = document.createRange();
    range.selectNodeContents(element);
    const tops = new Set(Array.from(range.getClientRects()).map((rect) => Math.round(rect.top)));
    return tops.size;
  });
}

async function checkServiceButton(page: Page, testInfo: TestInfo, panel: Locator) {
  const button = panel.getByRole('button', { name: 'Poner fuera de servicio', exact: true });
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
      expect(
        await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
      ).toBe(true);
      expect(await textLines(button), `líneas a ${size.width} × ${size.height}`).toBe(1);
      const box = await button.boundingBox();
      expect(box!.height).toBeGreaterThanOrEqual(44);
      expect(box!.y + box!.height).toBeLessThanOrEqual(size.height);
      // Las demás acciones secundarias siguen en su fila, sin desbordarse.
      for (const label of ['Precuenta', 'Dividir']) {
        const other = panel.getByRole('button', { name: label, exact: true });
        expect(await other.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
          true,
        );
      }
      await page.screenshot({
        path: testInfo.outputPath(`table-service-${theme}-${size.width}x${size.height}.png`),
      });
    }
  }
  await page.evaluate(() => {
    document.documentElement.dataset.theme = 'light';
  });
  await page.setViewportSize({ width: 1280, height: 720 });
}

test('precio que baja al sustituir complementos y «Poner fuera de servicio» en una línea', async ({
  page,
}, testInfo) => {
  const errors: string[] = [];
  page.on('pageerror', (error) => errors.push(error.message));
  const driver = await installDriver(page);
  await page.setViewportSize({ width: 1280, height: 720 });
  await page.goto('/');
  await page.getByLabel('Usuario', { exact: true }).fill('admin');
  await page.getByLabel('Contraseña', { exact: true }).fill('admin');
  await page.getByRole('button', { name: 'Entrar', exact: true }).click();
  await expect(page.getByRole('navigation', { name: 'Navegación principal' })).toBeVisible();
  await prepareCatalog(driver);
  const area = await create(driver, 'restaurant/areas', {
    operation_id: randomUUID(),
    name: 'Terraza 9A',
    active: true,
  });
  await create(driver, 'restaurant/tables', {
    operation_id: randomUUID(),
    area_id: area.id,
    name: 'Mesa 9A',
    shape: 'rectangle',
    x: 40,
    y: 40,
    width: 100,
    height: 72,
    seats: 4,
    active: true,
  });

  // §33: «Poner fuera de servicio» en una sola línea en el panel de la mesa libre.
  await page.getByRole('button', { name: 'Restaurante', exact: true }).click();
  await page.getByRole('button', { name: 'Mesas', exact: true }).click();
  await page
    .getByRole('group', { name: 'Áreas del restaurante' })
    .getByRole('button', { name: 'Terraza 9A', exact: true })
    .click();
  await page.getByRole('button', { name: /^Mesa 9A, Libre/ }).click();
  const tablePanel = page.getByRole('complementary', { name: 'Panel de Mesa 9A' });
  await checkServiceButton(page, testInfo, tablePanel);
  // La regla no cambia: la mesa libre queda fuera de servicio y se vuelve a habilitar.
  await tablePanel.getByRole('button', { name: 'Poner fuera de servicio', exact: true }).click();
  await expect(tablePanel.locator('.table-panel-state')).toHaveText('Fuera de servicio');
  await tablePanel.getByRole('button', { name: 'Habilitar', exact: true }).click();
  await expect(tablePanel.locator('.table-panel-state')).toHaveText('Libre');

  // §3.1: sustituir el arroz ($20) por frijoles ($10) baja el precio de $150 a $140. Los nombres
  // del catálogo son propios: los complementos y grupos tienen nombre único.
  await page.getByRole('button', { name: 'Para llevar', exact: true }).first().click();
  await page.getByRole('button', { name: 'Nuevo pedido', exact: true }).click();
  await expect(page.getByRole('heading', { name: /^Para llevar · Pedido #/ })).toBeVisible();
  await page
    .getByRole('navigation', { name: 'Categorías' })
    .getByRole('button', { name: /Precio flexible/ })
    .click();
  await page.getByRole('button', { name: 'Personalizar Filete flexible', exact: true }).click();
  const panel = page.getByRole('region', { name: 'Personalizar Filete flexible' });
  const add = panel.getByRole('button', { name: /^Agregar a la cuenta/ });
  await expect(add).toHaveText('Agregar a la cuenta · $150.00');
  const sides = panel.getByRole('region', { name: 'Guarniciones del filete' });
  await expect(sides.locator('.capture-block-charge')).toHaveText('Sin cambio');
  const card = (name: string) => sides.getByRole('group', { name: new RegExp(`^${name}: `) });
  await card('Arroz rojo')
    .getByRole('button', { name: 'Quitar una porción de Arroz rojo' })
    .click();
  // Quitar sin reemplazo también baja: −$20.00 → $130.00.
  await expect(sides.locator('.capture-block-charge')).toHaveText('−$20.00');
  await expect(add).toHaveText('Agregar a la cuenta · $130.00');
  await card('Frijoles de olla')
    .getByRole('button', { name: 'Agregar una porción de Frijoles de olla' })
    .click();
  await expect(card('Frijoles de olla')).toHaveAttribute(
    'aria-label',
    'Frijoles de olla: 1 porción',
  );
  await expect(sides.locator('.capture-block-charge')).toHaveText('−$10.00');
  await expect(add).toHaveText('Agregar a la cuenta · $140.00');
  // El desglose calculado por Go ya no habla de «Diferencia positiva».
  await expect(panel.getByText('Diferencia positiva')).toHaveCount(0);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(
    true,
  );
  await page.screenshot({ path: testInfo.outputPath('capture-price-decrease-1280x720.png') });
  await add.click();
  const bill = page.getByRole('complementary', { name: 'Cuenta actual' });
  const line = bill.getByRole('article', { name: '1 × Filete flexible' });
  await expect(line).toContainText('$140.00');
  await expect(bill.locator('.capture-sync-status')).toHaveText('Guardado');

  expect(errors).toEqual([]);
});
