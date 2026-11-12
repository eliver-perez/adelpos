import { randomUUID } from 'node:crypto';
import { expect, test } from '@playwright/test';
import type { Page, TestInfo } from '@playwright/test';
import { installDriver } from './native-driver';
import { openAdministration, openSection, topBar } from './navigation';
import { notice } from './notices';
import { sendTakeawayToKitchen } from './payments-flow';
import { groupTablist, inventoryGroups, openInventorySection } from './inventory-navigation';
import { chooseOption, optionLabels } from './select';

// Etapa 8, fase B: recorrido del consumo de inventario con el mismo transporte de prueba que las
// demás especificaciones (Go temporal y base adel_pos_test_*). Crea un insumo con existencia
// inicial, configura la receta de «Filete de prueba» (catálogo mínimo de la prueba) y una porción
// habitual, activa «Control de inventario» en la pantalla de funciones existente, envía el filete
// a cocina en un pedido para llevar y comprueba en Movimientos el consumo que escribe el
// trabajador de Go; después cancela la partida, la clasifica «No preparado» y comprueba la
// reversión. Se ejecuta después de inventory.spec.ts y restaurant.spec.ts en la corrida completa
// (orden alfabético): no altera lo que esas especificaciones comprueban.

type Driver = Awaited<ReturnType<typeof installDriver>>;

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
        await expect(page.locator('[data-sonner-toast]')).toHaveCount(0, { timeout: 8000 });
        // La aplicación vuelve a aplicar el tema del usuario al refrescar la sesión: se fija de
        // nuevo justo antes de la captura.
        await page.evaluate((value) => {
          document.documentElement.dataset.theme = value;
        }, theme);
        await expect(page.locator('html')).toHaveAttribute('data-theme', theme);
        await page.waitForTimeout(150);
        await page.screenshot({
          path: testInfo.outputPath(`b2-${name}-${theme}-1280x720.png`),
          fullPage: true,
        });
      }
    }
  }
  await page.evaluate(() => {
    document.documentElement.dataset.theme = 'light';
  });
}

async function openInventory(page: Page) {
  await openAdministration(page, 'Inventarios');
  await expect(page.getByRole('heading', { name: 'Inventarios', exact: true })).toBeVisible();
}

// Abre una sección de Inventarios por su grupo y su nombre (navegación agrupada de la etapa 9A).
async function openTab(page: Page, name: string) {
  await openInventorySection(page, name);
}

// Precondiciones por la API (no son parte de lo que se prueba): el filete necesita una ruta de
// cocina para liberarse sin advertencias. En la corrida completa ya la configuró
// restaurant.spec.ts; si esta especificación corre sola, se registra «Sin preparación» para el
// producto. También se agrega un ingrediente del catálogo para el apartado de porciones. Devuelve
// el estado de receta con que debe aparecer el filete: «Sin receta» con una ruta que prepara o
// «Sin preparación» con la ruta «Sin preparación».
async function preparePreconditions(driver: Driver) {
  const targets = await driver.request(
    'GET',
    `inventory/recipe-targets?kind=product&search=${encodeURIComponent('Filete de prueba')}`,
  );
  const steak = targets.body.items?.[0];
  expect(steak?.name).toBe('Filete de prueba');
  if (steak.kitchen_route_mode === null) {
    const route = await driver.request('POST', 'kitchen/routes', {
      operation_id: randomUUID(),
      product_id: steak.product_id,
      category_id: null,
      mode: 'none',
      station_ids: [],
    });
    expect(route.status, JSON.stringify(route.body)).toBeLessThan(300);
  }
  const portions = await driver.request(
    'GET',
    `inventory/ingredient-portions?search=${encodeURIComponent('Carne extra E2E')}`,
  );
  if (portions.body.total === 0) {
    const ingredient = await driver.request('POST', 'catalog/ingredients', {
      id: '',
      revision: 0,
      name: 'Carne extra E2E',
      description: '',
      active: true,
      unit: 'porción',
    });
    expect(ingredient.status, JSON.stringify(ingredient.body)).toBeLessThan(300);
  }
  return steak.kitchen_route_mode === 'prepare' ? 'Sin receta' : 'Sin preparación';
}

test('inventario: receta, control activado, consumo al enviar a cocina y cancelación clasificada', async ({
  page,
}, testInfo) => {
  test.setTimeout(150000);
  const errors: string[] = [];
  page.on('pageerror', (error) => errors.push(error.message));
  const driver = await installDriver(page);
  // En la corrida completa restaurant.spec.ts deja el teclado virtual siempre visible en la
  // terminal; esta especificación lo desactiva antes de entrar para no depender de ese estado.
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
  await expect(
    topBar(page).getByRole('button', { name: 'Administración', exact: true }),
  ).toBeVisible();
  const pendingStatus = await preparePreconditions(driver);
  await openInventory(page);
  // Etapa 9A: cinco grupos en la primera fila (las secciones van en la segunda).
  await expect(groupTablist(page).getByRole('tab')).toHaveText(inventoryGroups);

  // Insumo en kilogramos con costo de referencia y existencia inicial de 5 kg a $180.
  await openTab(page, 'Insumos');
  await page.getByRole('button', { name: 'Nuevo insumo', exact: true }).click();
  await page.getByLabel('Nombre del insumo', { exact: true }).fill('Carne de prueba E2E');
  await page.getByLabel(/^Código interno/).fill('CAR-E2E');
  await chooseOption(page.getByRole('combobox', { name: /^Unidad base/ }), 'Kilogramo (kg)');
  await page.getByLabel(/^Costo de referencia/).fill('180');
  await page.getByRole('button', { name: 'Crear insumo', exact: true }).click();
  await expect(notice(page, 'Insumo creado. Ya puedes agregar sus presentaciones.')).toBeVisible();
  await page.getByRole('button', { name: 'Volver al listado', exact: true }).click();
  await openTab(page, 'Existencias');
  await page.getByRole('button', { name: 'Existencia inicial', exact: true }).click();
  const balance = page.getByRole('group', { name: 'Renglón 1' });
  await chooseOption(
    balance.getByRole('combobox', { name: 'Insumo', exact: true }),
    'Carne de prueba E2E · CAR-E2E',
    { search: 'CAR-E2E' },
  );
  await balance.getByLabel('Cantidad', { exact: true }).fill('5');
  await balance.getByLabel(/^Costo unitario/).fill('180');
  await page.getByRole('button', { name: 'Registrar existencia inicial', exact: true }).click();
  await page
    .getByRole('alertdialog')
    .getByRole('button', { name: 'Registrar existencia inicial', exact: true })
    .click();
  await expect(
    notice(page, 'Existencia inicial registrada: 1 renglón en «Almacén general».'),
  ).toBeVisible();

  // Recetas: el filete aparece con el filtro de lo pendiente y se le configura la versión 1 con
  // 150 g de carne capturados en gramos (el insumo está en kilogramos).
  await openTab(page, 'Recetas');
  await chooseOption(
    page.getByRole('combobox', { name: 'Estado de la receta', exact: true }),
    pendingStatus,
  );
  await page.getByLabel('Buscar producto o complemento', { exact: true }).fill('Filete');
  const steakRow = page.getByRole('row').filter({ hasText: 'Filete de prueba' });
  await expect(steakRow).toContainText(pendingStatus);
  await checkLayout(page, testInfo, 'recetas-lista', true);
  await steakRow
    .getByRole('button', { name: 'Configurar la receta de Filete de prueba', exact: true })
    .click();
  await expect(page.getByRole('heading', { name: 'Filete de prueba', exact: true })).toBeVisible();
  await expect(page.getByText(/Al guardarla se crea la versión 1/)).toBeVisible();
  await page.getByRole('button', { name: 'Agregar insumo', exact: true }).click();
  const line = page.getByRole('group', { name: 'Renglón 1' });
  await chooseOption(
    line.getByRole('combobox', { name: 'Insumo', exact: true }),
    'Carne de prueba E2E · CAR-E2E',
    { search: 'CAR-E2E' },
  );
  const unit = line.getByRole('combobox', { name: /^Unidad/ });
  // Solo las unidades de masa (la dimensión de la unidad base que entrega Go).
  expect(await optionLabels(unit)).toEqual(['Gramo (g)', 'Kilogramo (kg)']);
  await chooseOption(unit, 'Gramo (g)');
  await line.getByLabel(/^Cantidad/).fill('150');
  await expect(page.locator('input[type="number"]')).toHaveCount(0);
  await page.getByRole('button', { name: 'Crear receta (versión 1)', exact: true }).click();
  await expect(notice(page, 'Receta creada como versión 1.')).toBeVisible();
  await expect(
    page.getByRole('button', { name: 'Guardar como versión 2', exact: true }),
  ).toBeVisible();
  await expect(line).toContainText('En la unidad base: 0.15');
  await expect(
    page.getByRole('region', { name: 'Costo teórico de la versión vigente' }),
  ).toContainText('$27.00');
  await checkLayout(page, testInfo, 'recetas-editor', true);
  await page.getByRole('button', { name: 'Volver al listado', exact: true }).click();
  await chooseOption(
    page.getByRole('combobox', { name: 'Estado de la receta', exact: true }),
    'Con receta',
  );
  await expect(page.getByRole('row').filter({ hasText: 'Filete de prueba' })).toContainText(
    'Con receta',
  );
  await checkLayout(page, testInfo, 'recetas-con-receta', false);

  // Porción habitual de un ingrediente del catálogo: 50 g de carne por porción.
  await page.getByRole('button', { name: 'Porciones habituales', exact: true }).click();
  await page.getByLabel('Buscar ingrediente', { exact: true }).fill('Carne extra');
  await page
    .getByRole('button', { name: 'Editar la porción de Carne extra E2E', exact: true })
    .click();
  const portion = page.getByRole('region', { name: 'Porción de Carne extra E2E' });
  await chooseOption(
    portion.getByRole('combobox', { name: 'Insumo por porción', exact: true }),
    'Carne de prueba E2E · CAR-E2E',
    { search: 'CAR-E2E' },
  );
  await chooseOption(portion.getByRole('combobox', { name: /^Unidad de la porción/ }), 'Gramo (g)');
  await portion.getByLabel(/^Cantidad por porción/).fill('50');
  await portion.getByRole('button', { name: 'Guardar porción', exact: true }).click();
  await expect(notice(page, 'Porción habitual guardada.')).toBeVisible();
  await expect(page.getByRole('row').filter({ hasText: 'Carne extra E2E' })).toContainText(
    '50 g de Carne de prueba E2E',
  );
  await checkLayout(page, testInfo, 'porciones', true);

  // «Control de inventario» se activa en la pantalla de funciones existente.
  await openTab(page, 'Resumen');
  await page.getByRole('button', { name: 'Administrar funciones', exact: true }).click();
  const features = page.getByRole('region', { name: 'Funciones del establecimiento' });
  const toggle = features.getByRole('button', { name: 'Control de inventario', exact: true });
  await expect(toggle).toHaveAttribute('aria-pressed', 'false');
  await toggle.click();
  await expect(notice(page, 'Control de inventario: activada.')).toBeVisible();
  await expect(toggle).toHaveAttribute('aria-pressed', 'true');

  // Pedido para llevar con el filete, enviado a cocina, y su cancelación ya liberado.
  await openSection(page, 'Para llevar');
  await page.getByRole('button', { name: 'Nuevo pedido', exact: true }).click();
  await expect(page.getByRole('heading', { name: /^Para llevar · Pedido #/ })).toBeVisible();
  await sendTakeawayToKitchen(page);

  // El trabajador de Go escribe el consumo en segundo plano: se espera a que lo registre.
  await expect
    .poll(
      async () => {
        const result = await driver.request(
          'GET',
          'inventory/movements?kind=consumption&source_kind=consumption',
        );
        return result.body.items?.filter(
          (movement: { item_code: string }) => movement.item_code === 'CAR-E2E',
        ).length;
      },
      { timeout: 20000 },
    )
    .toBe(1);
  const bill = page.getByRole('complementary', { name: 'Cuenta actual' });
  const steakLine = bill.getByRole('article', { name: '1 × Filete de prueba' });
  await steakLine.getByRole('button', { name: 'Cancelar unidades', exact: true }).click();
  await page.getByLabel('Motivo de cancelación').fill('El cliente ya no lo quiso');
  // Los eventos de cocina e inventario de la liberación recién enviada pueden volver a dibujar la
  // cuenta justo al pulsar: si el diálogo de confirmación no aparece, se vuelve a pulsar.
  const cancelDialog = page.getByRole('alertdialog', { name: /^¿Cancelar .*Filete de prueba\?$/ });
  await expect(async () => {
    if (!(await cancelDialog.isVisible())) {
      await page
        .getByRole('button', { name: 'Confirmar cancelación de unidades', exact: true })
        .click({ timeout: 2000 });
    }
    await expect(cancelDialog).toBeVisible({ timeout: 2000 });
  }).toPass({ timeout: 20000 });
  await cancelDialog.getByRole('button', { name: 'Cancelar unidades', exact: true }).click();
  await expect(steakLine).toContainText('Cancelado');

  // Movimientos: el consumo del filete con su origen y el proceso de Go como responsable.
  await openInventory(page);
  await openTab(page, 'Movimientos');
  const consumption = page
    .getByRole('row')
    .filter({ hasText: 'Carne de prueba E2E' })
    .filter({ hasText: 'Consumo por preparación' });
  await expect(consumption).toContainText('−0.15 kg');
  await expect(consumption).toContainText('Liberación a cocina');
  await expect(consumption).toContainText('Proceso de consumo de inventario');
  await expect(consumption).toContainText('−$27.00');
  await consumption
    .getByRole('button', {
      name: 'Ver consumo de la partida del movimiento de Carne de prueba E2E',
      exact: true,
    })
    .click();
  const trace = page.getByRole('region', { name: 'Consumo de la partida' });
  await expect(trace).toContainText('Consumo de la partida · Filete de prueba');
  await expect(trace).toContainText('Consumo registrado');
  await expect(trace).toContainText('Para llevar');
  await expect(
    trace.getByRole('row').filter({ hasText: 'Carne de prueba E2E' }).first(),
  ).toContainText('0.15 kg');
  await checkLayout(page, testInfo, 'traza-consumo', true);
  await trace.getByRole('button', { name: 'Volver a movimientos', exact: true }).click();

  // La cancelación de una partida ya consumida queda por clasificar hasta el corte.
  await expect
    .poll(
      async () =>
        (await driver.request('GET', 'inventory/cancellations?state=pending')).body.pending,
      { timeout: 20000 },
    )
    .toBeGreaterThan(0);
  await openTab(page, 'Ajustes y mermas');
  const card = page.getByRole('article', { name: 'Cancelación de Filete de prueba' }).first();
  await expect(card).toContainText(/Pedido #\d+/);
  await expect(card).toContainText('Límite para clasificar');
  await expect(card).toContainText('0.15 kg');
  await checkLayout(page, testInfo, 'cancelaciones', true);
  await card.getByRole('button', { name: 'No preparado', exact: true }).click();
  await expect(notice(page, 'Filete de prueba: No preparado.')).toBeVisible();
  const history = page.getByRole('region', { name: 'Historial de clasificaciones' });
  await expect(
    history.getByRole('row').filter({ hasText: 'Filete de prueba' }).first(),
  ).toContainText('Clasificada por el encargado');

  // «No preparado» revierte el consumo: la existencia vuelve a 5 kg.
  await openTab(page, 'Movimientos');
  await expect(
    page.getByRole('row').filter({ hasText: 'Carne de prueba E2E' }).filter({
      hasText: 'Reversión de consumo',
    }),
  ).toContainText('+0.15 kg');
  const stock = await driver.request('GET', 'inventory/stock?search=CAR-E2E');
  expect(stock.body.items[0].quantity).toBe('5');
  expect(errors).toEqual([]);
});
