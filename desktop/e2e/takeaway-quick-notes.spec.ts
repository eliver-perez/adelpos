import { expect, test } from '@playwright/test';
import type { Locator, Page, TestInfo } from '@playwright/test';
import { installDriver } from './native-driver';

// Etapa 8, fase B (Bq): en la captura, una indicación rápida con efecto aplica la misma selección
// del personalizador y las guarniciones de varias porciones se sustituyen en dos toques. Mismo
// transporte de prueba que restaurant.spec.ts (Go temporal y base adel_pos_test_*). El catálogo
// del recorrido se crea por la API con la sesión del administrador, en una categoría propia. El
// nombre del archivo lo ordena después de restaurant.spec.ts, que espera la cuenta #1 y los dos
// productos de «Platillos» en una base recién preparada.

type Driver = Awaited<ReturnType<typeof installDriver>>;

async function create(driver: Driver, route: string, body: unknown) {
  const response = await driver.request('POST', route, body);
  expect(response.status, `${route}: ${JSON.stringify(response.body)}`).toBe(200);
  return response.body as { id: string; options?: { id: string; complement_id: string }[] };
}

// Filete empanizado de $140 con arroz ($30) y ensalada ($25) incluidos y hasta dos porciones
// (ejemplo del propietario), cebolla incluida y queso opcional, e indicaciones de su categoría.
async function prepareCatalog(driver: Driver) {
  // Categoría propia: el recorrido no altera los conteos de «Platillos» de restaurant.spec.ts.
  const category = await create(driver, 'catalog/categories', {
    name: 'Platos fuertes',
    active: true,
    color: '#1f9a78',
  });
  const onion = await create(driver, 'catalog/ingredients', {
    name: 'Cebolla',
    active: true,
    unit: 'porción',
  });
  const cheese = await create(driver, 'catalog/ingredients', {
    name: 'Queso',
    active: true,
    unit: 'porción',
  });
  const complements: { id: string }[] = [];
  for (const name of ['Arroz', 'Ensalada', 'Papas', 'Pasta']) {
    complements.push(
      await create(driver, 'catalog/complements', {
        name,
        active: true,
        product_id: null,
        ingredient_id: null,
      }),
    );
  }
  const values = ['30', '25', '35', '20'];
  const sides = await create(driver, 'catalog/groups', {
    name: 'Guarniciones',
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
  await create(driver, 'catalog/products', {
    name: 'Filete empanizado',
    active: true,
    category_id: category.id,
    code: 'FILETE-EMP',
    kind: 'product',
    base_price: '140',
    ingredients: [
      {
        ingredient_id: onion.id,
        included_quantity: '1',
        removable: true,
        addable: true,
        max_quantity: '3',
        extra_price: '5',
        recipe_quantity: '0',
      },
      {
        ingredient_id: cheese.id,
        included_quantity: '0',
        removable: false,
        addable: true,
        max_quantity: '2',
        extra_price: '12.50',
        recipe_quantity: '0',
      },
    ],
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
  for (const [text, effect, ingredient] of [
    ['Sin cebolla', 'omit', onion.id],
    ['Queso extra', 'extra', cheese.id],
    ['Bien dorado', 'none', null],
  ] as const) {
    await create(driver, 'catalog/quick-notes', {
      category_id: category.id,
      text,
      active: true,
      effect,
      ingredient_id: ingredient,
      revision: 0,
    });
  }
}

// Sin desbordamiento horizontal a 1280 × 720 y 1920 × 1080 en claro y oscuro; a 1280 × 720 guarda
// la vista de la sección indicada en ambos temas.
async function checkPanel(page: Page, testInfo: TestInfo, section: Locator, name: string) {
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
      const panel = page.locator('.capture-customization-body');
      const panelOverflow = await panel.evaluate(
        (element) => element.scrollWidth - element.clientWidth,
      );
      expect(panelOverflow, `El panel se desborda (${theme})`).toBe(0);
      if (size.width === 1280) {
        // La vista se guarda sin avisos encima y con la sección al inicio del panel.
        await expect(page.locator('[data-sonner-toast]')).toHaveCount(0, { timeout: 8000 });
        await section.evaluate((element) => element.scrollIntoView({ block: 'start' }));
        await page.screenshot({ path: testInfo.outputPath(`${name}-${theme}-1280x720.png`) });
      }
    }
  }
  await page.evaluate(() => {
    document.documentElement.dataset.theme = 'light';
  });
}

test('captura: indicación con efecto y sustitución de guarniciones en dos toques', async ({
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

  await page.getByRole('button', { name: 'Restaurante', exact: true }).click();
  await page.getByRole('button', { name: 'Para llevar', exact: true }).first().click();
  await page.getByRole('button', { name: 'Nuevo pedido', exact: true }).click();
  await expect(page.getByRole('heading', { name: /^Para llevar · Pedido #/ })).toBeVisible();
  await page
    .getByRole('navigation', { name: 'Categorías' })
    .getByRole('button', { name: /Platos fuertes/ })
    .click();
  await page.getByRole('button', { name: 'Personalizar Filete empanizado', exact: true }).click();
  const panel = page.getByRole('region', { name: 'Personalizar Filete empanizado' });
  const add = panel.getByRole('button', { name: /^Agregar a la cuenta/ });
  await expect(add).toHaveText('Agregar a la cuenta · $140.00');

  // Guarniciones: «2 de 2», precio en la tarjeta y «Sin cambio» con el cálculo de Go.
  const sides = panel.getByRole('region', { name: 'Guarniciones' });
  await expect(sides.locator('.capture-block-count')).toHaveText('2 de 2');
  await expect(sides.locator('.capture-block-charge')).toHaveText('Sin cambio');
  const card = (name: string) => sides.getByRole('group', { name: new RegExp(`^${name}: `) });
  await expect(card('Arroz')).toContainText('$30.00');
  await expect(card('Arroz')).toHaveAttribute('aria-label', 'Arroz: 1 porción');
  // Pasar del máximo avisa en el grupo.
  await card('Papas').getByRole('button', { name: 'Agregar una porción de Papas' }).click();
  await expect(sides.getByRole('status')).toContainText('admite hasta 2 porciones');
  // Sustituir en dos toques: «−» en arroz y toque en papas ($35 > $30: +$5.00 de Go).
  const minus = card('Arroz').getByRole('button', { name: 'Quitar una porción de Arroz' });
  const minusBox = await minus.boundingBox();
  expect(minusBox!.width).toBeGreaterThanOrEqual(44);
  expect(minusBox!.height).toBeGreaterThanOrEqual(44);
  await minus.click();
  await card('Papas').getByRole('button', { name: 'Agregar una porción de Papas' }).click();
  await expect(card('Papas')).toHaveAttribute('aria-label', 'Papas: 1 porción');
  await expect(card('Arroz')).toHaveAttribute('aria-label', 'Arroz: sin elegir');
  await expect(sides.locator('.capture-block-count')).toHaveText('2 de 2');
  await expect(sides.locator('.capture-block-charge')).toHaveText('+$5.00');
  await expect(add).toHaveText('Agregar a la cuenta · $145.00');
  await checkPanel(page, testInfo, sides, 'capture-complements');

  // Indicación con efecto: «Queso extra» fija la misma porción que el botón «Extra queso» y cobra
  // lo mismo ($12.50); «Sin cebolla» retira la cebolla del personalizador.
  const notes = panel.getByRole('region', { name: 'Indicaciones para cocina' });
  const ingredients = panel.getByRole('region', { name: 'Ingredientes' });
  await notes.getByRole('button', { name: 'Queso extra', exact: true }).click();
  await expect(ingredients.getByRole('button', { name: /^Extra queso/ })).toHaveAttribute(
    'aria-pressed',
    'true',
  );
  await expect(add).toHaveText('Agregar a la cuenta · $157.50');
  await notes.getByRole('button', { name: 'Sin cebolla', exact: true }).click();
  await expect(
    ingredients.getByRole('button', { name: 'Sin cebolla', exact: true }),
  ).toHaveAttribute('aria-pressed', 'true');
  await expect(
    notes.getByRole('button', { name: 'Quitar indicación Sin cebolla', exact: true }),
  ).toHaveAttribute('aria-pressed', 'true');
  await checkPanel(page, testInfo, ingredients, 'capture-quick-notes');
  await expect(add).toHaveText('Agregar a la cuenta · $157.50');
  await add.click();
  const bill = page.getByRole('complementary', { name: 'Cuenta actual' });
  const line = bill.getByRole('article', { name: '1 × Filete empanizado' });
  await expect(line).toContainText('$157.50');
  // El resumen muestra cada indicación una sola vez.
  await expect(line.locator('.capture-line-detail')).toContainText('Queso extra, Sin cebolla');
  await expect(line.locator('.capture-line-detail')).not.toContainText('Extra Queso');
  await expect(line.locator('.capture-line-detail')).not.toContainText('Sin Cebolla');
  await expect(bill.locator('.capture-sync-status')).toHaveText('Guardado');
  // El borrador guarda los identificadores de las indicaciones y la selección estructurada.
  const heading = await page
    .getByRole('heading', { name: /^Para llevar · Pedido #/ })
    .textContent();
  const folio = Number(/#(\d+)/.exec(heading ?? '')?.[1]);
  const accounts = (await driver.request('GET', 'restaurant/accounts')).body as {
    items: { id: string; folio: number }[];
  };
  const account = accounts.items.find((entry) => entry.folio === folio);
  expect(account).toBeTruthy();
  const saved = (await driver.request('GET', `restaurant/accounts/${account!.id}`)).body as {
    draft: {
      customization: {
        ingredients: { ingredient_id: string; quantity: string }[];
        quick_notes?: { text: string }[];
      };
    }[];
  };
  const customization = saved.draft[0]!.customization;
  expect(customization.quick_notes?.map((entry) => entry.text)).toEqual([
    'Queso extra',
    'Sin cebolla',
  ]);
  expect(customization.ingredients.map((entry) => entry.quantity).sort()).toEqual(['0', '1']);
  expect(errors).toEqual([]);
});
