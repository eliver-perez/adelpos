import { randomUUID } from 'node:crypto';
import { copyFileSync, mkdirSync } from 'node:fs';
import { join } from 'node:path';
import { expect, test } from '@playwright/test';
import type { Locator, Page, TestInfo } from '@playwright/test';
import { installDriver } from './native-driver';
import { appBar, cashButton, openSection, topBar } from './navigation';

// Etapa 9D, fases B y C (§2 y §3): barra superior con las secciones a la izquierda y Caja a la
// derecha, y Mesas con el panel lateral siempre visible, mapa con escala automática y
// desplazamiento, un solo botón principal por panel, tiempos en días y horas, advertencia de cuenta
// abierta por mucho tiempo y sin «Actualizar». Mismo transporte que las demás especificaciones (Go
// temporal y base adel_pos_test_*). Capturas en tema claro y oscuro a 1280 × 720 y 1920 × 1080
// (copia opcional en ADEL_SHOTS_DIR).
//
// Datos reales por la API. Dos presentaciones se simulan en el puente de la prueba, como lo vería la
// terminal: una cuenta abierta hace «3 d 14 h» con la marca long_open (Go calcula ambas con su hora;
// la prueba no puede envejecer una cuenta) y «Sin conexión» (el puente rechaza las peticiones).

type Driver = Awaited<ReturnType<typeof installDriver>>;

async function call(driver: Driver, method: string, route: string, body?: unknown) {
  const response = await driver.request(method, route, body);
  expect(response.status, `${method} ${route}: ${JSON.stringify(response.body)}`).toBe(200);
  return response.body;
}

interface Geometry {
  x: number;
  y: number;
  width: number;
  height: number;
  shape?: 'rectangle' | 'round';
}

async function table(driver: Driver, areaID: string, name: string, geometry: Geometry) {
  return (await call(driver, 'POST', 'restaurant/tables', {
    operation_id: randomUUID(),
    area_id: areaID,
    name,
    shape: geometry.shape ?? 'rectangle',
    x: geometry.x,
    y: geometry.y,
    width: geometry.width,
    height: geometry.height,
    seats: 4,
    active: true,
  })) as { id: string; revision: number };
}

async function account(driver: Driver, tableID: string, productID?: string): Promise<string> {
  const opened = await call(driver, 'POST', 'restaurant/accounts', {
    operation_id: randomUUID(),
    mode: 'dine_in',
    table_id: tableID,
  });
  const id = opened.account.id as string;
  if (!productID) {
    return id;
  }
  const route = `restaurant/accounts/${id}`;
  const drafted = await call(driver, 'PUT', `${route}/draft`, {
    operation_id: randomUUID(),
    revision: opened.account.revision,
    items: [{ id: randomUUID(), product_id: productID, variant_id: null, quantity: 1 }],
  });
  const preview = await call(driver, 'GET', `${route}/preview`);
  await call(driver, 'POST', `${route}/confirm`, {
    operation_id: randomUUID(),
    revision: drafted.account.revision,
    expected_fingerprint: preview.fingerprint,
  });
  return id;
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

// Recorre los dos tamaños y los dos temas: sin desbordamiento horizontal, la comprobación de la
// vista y una captura (de la página o de un elemento).
async function views(
  page: Page,
  testInfo: TestInfo,
  name: string,
  options: {
    check?: (size: { width: number; height: number }) => Promise<void>;
    element?: Locator;
    only?: number;
  } = {},
) {
  for (const size of sizes.filter((entry) => !options.only || entry.width === options.only)) {
    await page.setViewportSize(size);
    for (const theme of themes) {
      const file = `${name}-${theme}-${size.width}x${size.height}.png`;
      // La aplicación vuelve a aplicar el tema del usuario cuando se actualiza su sesión: la captura
      // se repite si el tema cambió mientras se tomaba.
      for (let attempt = 0; attempt < 3; attempt += 1) {
        await setTheme(page, theme);
        await page.waitForTimeout(250);
        const overflow = await page.evaluate(
          () => document.documentElement.scrollWidth - window.innerWidth,
        );
        expect(overflow, `${name} se desborda a ${size.width} × ${size.height} (${theme})`).toBe(0);
        await options.check?.(size);
        const path = testInfo.outputPath(file);
        if (options.element) {
          await options.element.screenshot({ path });
        } else {
          await page.screenshot({ path });
        }
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

async function box(locator: Locator) {
  const value = await locator.boundingBox();
  expect(value).not.toBeNull();
  return value!;
}

// Ningún texto de la barra se corta: cada botón y cada texto con su ancho completo.
async function expectNothingCut(bar: Locator) {
  const cut = await bar.evaluate((root) =>
    Array.from(
      root.querySelectorAll<HTMLElement>('button, button span, button strong, [role="status"]'),
    )
      .filter((element) => element.offsetParent !== null)
      .filter((element) => element.scrollWidth > element.clientWidth + 1)
      .map((element) => `${element.tagName} ${element.className} «${element.textContent}»`),
  );
  expect(cut).toEqual([]);
  const overflow = await bar.evaluate((root) => root.scrollWidth - root.clientWidth);
  expect(overflow).toBeLessThanOrEqual(0);
}

// Barra superior (§2): Mesas, Para llevar, Cocina y Administración seguidas y del mismo ancho a la
// izquierda; Caja (dos renglones), el servidor, el usuario y pantalla completa a la derecha; botones
// de 52–56 px y nada cortado.
async function checkBar(page: Page) {
  const sections = topBar(page).getByRole('button');
  await expect(sections).toHaveText(['Mesas', 'Para llevar', 'Cocina', 'Administración']);
  const widths: number[] = [];
  for (let index = 0; index < 4; index += 1) {
    widths.push((await box(sections.nth(index))).width);
  }
  expect(Math.max(...widths) - Math.min(...widths)).toBeLessThanOrEqual(1);
  const administration = await box(sections.nth(3));
  const cash = cashButton(page);
  const cashBox = await box(cash);
  expect(cashBox.x).toBeGreaterThan(administration.x + administration.width);
  const lines = cash.locator('.cash-shortcut-lines > *');
  await expect(lines).toHaveCount(2);
  await expect(lines.first()).toHaveText('Caja');
  expect((await box(lines.nth(1))).y).toBeGreaterThan((await box(lines.first())).y);
  const server = page.locator('.app-topbar .server-status');
  const user = page.getByRole('button', { name: /^Menú del usuario/ });
  const fullscreen = page.locator('.app-topbar .window-controls-bar button');
  const order = [cashBox, await box(server), await box(user), await box(fullscreen)];
  for (let index = 1; index < order.length; index += 1) {
    expect(order[index]!.x).toBeGreaterThan(order[index - 1]!.x);
  }
  const heights = await appBar(page).evaluate((root) =>
    Array.from(root.querySelectorAll<HTMLElement>('button'))
      .filter((element) => element.offsetParent !== null)
      .map((element) => Math.round(element.getBoundingClientRect().height)),
  );
  for (const height of heights) {
    expect(height).toBeGreaterThanOrEqual(52);
    expect(height).toBeLessThanOrEqual(56);
  }
  await expectNothingCut(appBar(page));
}

test('barra superior y Mesas (etapa 9D, fases B y C)', async ({ page }, testInfo) => {
  test.setTimeout(420000);
  const errors: string[] = [];
  page.on('pageerror', (error) => errors.push(error.message));
  const driver = await installDriver(page);
  await call(driver, 'POST', 'auth/login', { username: 'admin', password: 'admin' });
  await call(driver, 'PUT', 'terminal/preferences', { keyboard_mode: 'disabled' });

  // 1. Datos por la API: un área con seis mesas en una parte del lienzo (la escala automática las
  // agranda) y otra con 35 mesas pequeñas por todo el lienzo (a 1280 × 720 no caben con 64 px).
  const category = await call(driver, 'POST', 'catalog/categories', {
    name: 'Platillos BC',
    active: true,
    color: '#d64545',
  });
  const product = await call(driver, 'POST', 'catalog/products', {
    name: 'Guiso BC',
    active: true,
    category_id: category.id,
    code: 'GUISO-BC',
    kind: 'product',
    base_price: '120',
  });
  const salon = (await call(driver, 'POST', 'restaurant/areas', {
    operation_id: randomUUID(),
    name: 'Salón BC',
    active: true,
  })) as { id: string };
  const geometry: Record<string, Geometry> = {
    'Mesa 101': { x: 300, y: 120, width: 120, height: 100 },
    'Mesa 102': { x: 460, y: 120, width: 120, height: 100 },
    'Mesa 103': { x: 620, y: 120, width: 120, height: 100 },
    'Mesa 104': { x: 300, y: 270, width: 110, height: 110, shape: 'round' },
    'Mesa 105': { x: 460, y: 270, width: 120, height: 100 },
    'Mesa 106': { x: 620, y: 270, width: 120, height: 100 },
  };
  const created: Record<string, { id: string; revision: number }> = {};
  for (const [name, entry] of Object.entries(geometry)) {
    created[name] = await table(driver, salon.id, name, entry);
  }
  await account(driver, created['Mesa 102']!.id, product.id as string);
  const longOpenAccount = await account(driver, created['Mesa 103']!.id);
  await call(driver, 'POST', `restaurant/tables/${created['Mesa 106']!.id}/service`, {
    operation_id: randomUUID(),
    revision: created['Mesa 106']!.revision,
    out_of_service: true,
  });
  const terrace = (await call(driver, 'POST', 'restaurant/areas', {
    operation_id: randomUUID(),
    name: 'Terraza BC',
    active: true,
  })) as { id: string };
  for (let row = 0; row < 5; row += 1) {
    for (let column = 0; column < 7; column += 1) {
      await table(driver, terrace.id, `T${row * 7 + column + 1}`, {
        x: 10 + column * 140,
        y: 10 + row * 110,
        width: 64,
        height: 56,
      });
    }
  }
  // Turno abierto para ver «Caja · Turno abierto».
  const current = await call(driver, 'GET', 'payments/shifts/current');
  if (!current.shift) {
    await call(driver, 'POST', 'payments/shifts', {
      operation_id: randomUUID(),
      opening_float: '0',
    });
  }
  await driver.request('POST', 'auth/logout');

  // Puente de la prueba: la cuenta de la Mesa 103 abierta hace 3 d 14 h con la marca long_open de
  // Go, y la terminal sin conexión a pedido.
  const longSeconds = (3 * 24 + 14) * 3600 + 20 * 60;
  await page.addInitScript(
    ({ tableID, accountID, seconds }) => {
      const bridge = window as unknown as {
        __TAURI_INTERNALS__: {
          invoke: (command: string, args?: Record<string, unknown>) => Promise<unknown>;
        };
        __adelOffline?: boolean;
      };
      const original = bridge.__TAURI_INTERNALS__.invoke;
      const aged = (entry: Record<string, unknown>) => {
        if (entry.id === accountID) {
          entry.elapsed_seconds = seconds;
          entry.long_open = true;
        }
      };
      bridge.__TAURI_INTERNALS__.invoke = async (command, args) => {
        if (command === 'api_request' && bridge.__adelOffline) {
          throw 'No se pudo conectar con el equipo servidor.';
        }
        const result = (await original(command, args)) as {
          body?: Record<string, unknown> & {
            tables?: Record<string, unknown>[];
            items?: Record<string, unknown>[];
            account?: Record<string, unknown>;
          };
        };
        if (command === 'api_request' && result?.body) {
          for (const entry of result.body.tables ?? []) {
            if (entry.id === tableID) {
              entry.occupied_seconds = seconds;
              entry.long_open = true;
            }
          }
          for (const entry of result.body.items ?? []) {
            aged(entry);
          }
          if (result.body.account) {
            aged(result.body.account);
          }
        }
        return result;
      };
    },
    { tableID: created['Mesa 103']!.id, accountID: longOpenAccount, seconds: longSeconds },
  );

  await page.setViewportSize({ width: 1280, height: 720 });
  await page.goto('/');
  await page.getByLabel('Usuario', { exact: true }).fill('admin');
  await page.getByLabel('Contraseña', { exact: true }).fill('admin');
  await page.getByRole('button', { name: 'Entrar', exact: true }).click();
  await expect(cashButton(page)).toHaveAttribute('data-shift-state', 'open', { timeout: 20000 });

  // 2. Barra superior (§2) a 1280 y 1920, en ambos temas: orden, mismo ancho, Caja en dos
  // renglones a la derecha, «En línea» en azul, alturas de 52–56 px y nada cortado.
  const server = page.locator('.app-topbar .server-status');
  await expect(server).toHaveText('En línea');
  await views(page, testInfo, 'barra', {
    element: appBar(page),
    check: async () => {
      await checkBar(page);
      expect(await computed(server.locator('.server-status-text'), 'color')).toBe(
        await token(page, '--accent'),
      );
      expect(
        await computed(page.locator('.app-topbar .cash-shortcut-indicator'), 'background-color'),
      ).toBe(await token(page, '--accent'));
    },
  });
  // El menú rápido de Caja se abre a la derecha sin salirse de la ventana.
  await cashButton(page).click();
  const cashMenu = page.getByRole('group', { name: 'Menú rápido de caja' });
  await expect(cashMenu).toBeVisible();
  const menuBox = await box(cashMenu);
  expect(menuBox.x).toBeGreaterThanOrEqual(0);
  expect(menuBox.x + menuBox.width).toBeLessThanOrEqual(1280);
  await page.keyboard.press('Escape');
  await expect(cashMenu).toHaveCount(0);

  // 3. Mesas sin mesa elegida: panel siempre visible con el resumen del área, acciones rápidas y la
  // ayuda (que ya no está sobre el mapa); sin «Actualizar».
  await openSection(page, 'Mesas');
  await page
    .getByRole('group', { name: 'Áreas del restaurante' })
    .getByRole('button', { name: 'Salón BC', exact: true })
    .click();
  const map = page.getByRole('region', { name: 'Mapa del restaurante' });
  const summary = page.getByRole('complementary', { name: 'Resumen del área' });
  await expect(summary).toBeVisible();
  await expect(summary.getByRole('heading', { name: 'Salón BC' })).toBeVisible();
  await expect(summary).toContainText('Toca una mesa para ver sus cuentas');
  await expect(map).not.toContainText('Toca una mesa');
  await expect(page.getByRole('button', { name: 'Actualizar', exact: true })).toHaveCount(0);
  await expect(page.getByRole('button', { name: 'Reintentar', exact: true })).toHaveCount(0);
  const quick = summary.getByRole('group', { name: 'Acciones rápidas' });
  await expect(quick.getByRole('button')).toHaveText([
    'Cuenta sin mesa',
    'Nuevo pedido para llevar',
  ]);
  const counts = summary.getByRole('group', { name: 'Mesas por estado' });
  await expect(counts.getByRole('button')).toHaveText([
    '3Libres',
    '2Ocupadas',
    '0Esperando pago',
    '0Pago parcial',
  ]);
  await expect(summary).toContainText('1 mesa fuera de servicio');

  // Cada número resalta esas mesas en el mapa; tocarlo de nuevo quita el resaltado.
  const canvas = page.locator('.table-canvas[aria-label="Mapa de mesas"]');
  const occupiedCount = counts.getByRole('button', { name: /ocupadas · resaltar en el mapa/ });
  await occupiedCount.click();
  await expect(occupiedCount).toHaveAttribute('aria-pressed', 'true');
  await expect(canvas.locator('.table-highlighted')).toHaveCount(2);
  await expect(canvas.getByRole('button', { name: /^Mesa 102, Ocupada/ })).toHaveClass(
    /table-highlighted/,
  );
  await expect(canvas.getByRole('button', { name: /^Mesa 101, Libre/ })).not.toHaveClass(
    /table-highlighted/,
  );
  await occupiedCount.click();
  await expect(canvas.locator('.table-highlighted')).toHaveCount(0);

  // 4. Mapa con escala automática (§3.2): todas las mesas del área a la vista, una sola escala, nada
  // menor de 64 px y el mapa a la misma altura que el panel.
  const viewport = page.locator('.table-map-operation .table-map-viewport');
  const checkFit = async (expectOverflow: boolean, names: Record<string, Geometry>) => {
    await expect(canvas).toHaveAttribute('data-overflow', String(expectOverflow));
    const scale = Number(await canvas.getAttribute('data-scale'));
    expect(scale).toBeGreaterThan(0);
    const frame = await box(viewport);
    for (const [name, entry] of Object.entries(names)) {
      const tile = await box(canvas.getByRole('button', { name: new RegExp(`^${name}, `) }));
      expect(Math.abs(tile.width - entry.width * scale)).toBeLessThanOrEqual(1);
      expect(Math.abs(tile.height - entry.height * scale)).toBeLessThanOrEqual(1);
      expect(Math.min(tile.width, tile.height)).toBeGreaterThanOrEqual(63.5);
      if (!expectOverflow) {
        expect(tile.x).toBeGreaterThanOrEqual(frame.x);
        expect(tile.y).toBeGreaterThanOrEqual(frame.y);
        expect(tile.x + tile.width).toBeLessThanOrEqual(frame.x + frame.width + 0.5);
        expect(tile.y + tile.height).toBeLessThanOrEqual(frame.y + frame.height + 0.5);
      }
    }
    return scale;
  };
  const panelSameHeight = async (panel: Locator) => {
    const mapBox = await box(map);
    const panelBox = await box(panel);
    expect(Math.abs(mapBox.y - panelBox.y)).toBeLessThanOrEqual(1);
    expect(Math.abs(mapBox.y + mapBox.height - (panelBox.y + panelBox.height))).toBeLessThanOrEqual(
      1,
    );
    return panelBox.width;
  };
  const visibleSalon = Object.fromEntries(
    Object.entries(geometry).filter(([name]) => name !== 'Mesa 106'),
  );
  const salonScale = await checkFit(false, { ...visibleSalon, 'Mesa 106': geometry['Mesa 106']! });
  // Las seis mesas ocupan una parte del lienzo: la escala las agranda (más de 1 px por unidad).
  expect(salonScale).toBeGreaterThan(1);
  const round = await box(canvas.getByRole('button', { name: /^Mesa 104, / }));
  expect(Math.abs(round.width - round.height)).toBeLessThanOrEqual(1);
  const summaryWidth = await panelSameHeight(summary);
  expect(Math.round(summaryWidth)).toBe(360);

  // 5. Tiempos en días y horas y advertencia ámbar de la cuenta abierta por mucho tiempo (§3.4).
  const aged = canvas.getByRole('button', { name: /^Mesa 103, Ocupada/ });
  await expect(aged).toHaveClass(/table-long-open/);
  await expect(aged).toHaveAccessibleName(/3 d 14 h, abierta hace más de \d+ h/);
  const agedTime = aged.locator('.table-tile-long-open');
  await expect(agedTime).toHaveText('3 d 14 h');
  expect(await computed(agedTime, 'color')).toBe(await token(page, '--warning'));
  await expect(agedTime.locator('svg')).toHaveAttribute('data-icon', 'alert');
  await views(page, testInfo, 'mesas-sin-mesa', {
    check: async () => {
      await expect(summary).toBeVisible();
      await checkFit(false, visibleSalon);
      await panelSameHeight(summary);
    },
  });

  // 6. Con mesa elegida: el panel conserva su ancho; «Capturar pedidos en esta mesa» es el único
  // botón principal (relleno azul) y «Cobrar» queda con contorno (§3.3).
  await aged.click();
  const agedPanel = page.getByRole('complementary', { name: 'Panel de Mesa 103' });
  const agedFact = agedPanel.locator('.table-panel-long-open');
  await expect(agedFact).toContainText('Abierta hace 3 d 14 h');
  expect(await computed(agedFact, 'color')).toBe(await token(page, '--warning'));
  await canvas.getByRole('button', { name: /^Mesa 102, Ocupada/ }).click();
  const panel = page.getByRole('complementary', { name: 'Panel de Mesa 102' });
  const capture = panel.getByRole('button', { name: 'Capturar pedidos en esta mesa', exact: true });
  await expect(capture).toHaveClass(/primary-button/);
  const checkout = panel.getByRole('button', { name: /^Cobrar · \$/ });
  await expect(checkout).not.toHaveClass(/primary-button/);
  await expect(checkout).toHaveClass(/capture-checkout-outline/);
  await expect(panel.locator('.primary-button')).toHaveCount(1);
  expect(Math.round(await panelSameHeight(panel))).toBe(360);
  await views(page, testInfo, 'mesas-con-mesa', {
    check: async (size) => {
      await expect(panel).toBeVisible();
      await panelSameHeight(panel);
      expect(await computed(capture, 'background-color')).toBe(await token(page, '--accent'));
      expect(await computed(checkout, 'border-top-color')).toBe(await token(page, '--accent'));
      const action = await box(capture);
      expect(action.height).toBeGreaterThanOrEqual(52);
      expect(action.y + action.height).toBeLessThanOrEqual(size.height);
      await expectNothingCut(panel.locator('.table-panel-footer'));
    },
  });
  await page.keyboard.press('Escape');
  await expect(summary).toBeVisible();

  // 7. Muchas mesas: a 1280 × 720 no caben con 64 px; el mapa se desplaza con barras y arrastrando.
  const terraceNames: Record<string, Geometry> = {};
  for (let index = 1; index <= 35; index += 1) {
    terraceNames[`T${index}`] = { x: 0, y: 0, width: 64, height: 56 };
  }
  await page
    .getByRole('group', { name: 'Áreas del restaurante' })
    .getByRole('button', { name: 'Terraza BC', exact: true })
    .click();
  await checkFit(true, terraceNames);
  expect(Number(await canvas.getAttribute('data-scale'))).toBeCloseTo(64 / 56, 3);
  expect(await computed(viewport, 'overflow-x')).toBe('auto');
  expect(await computed(viewport, 'overflow-y')).toBe('auto');
  const scroll = () =>
    viewport.evaluate((element) => ({
      left: element.scrollLeft,
      top: element.scrollTop,
      width: element.scrollWidth - element.clientWidth,
      height: element.scrollHeight - element.clientHeight,
    }));
  const before = await scroll();
  expect(before.width).toBeGreaterThan(0);
  expect(before.height).toBeGreaterThan(0);
  expect(before.left).toBe(0);
  // Arrastre que empieza sobre una mesa: desplaza el mapa y no elige la mesa.
  const start = await box(canvas.getByRole('button', { name: /^T17, / }));
  const x = start.x + start.width / 2;
  const y = start.y + start.height / 2;
  await page.mouse.move(x, y);
  await page.mouse.down();
  for (let step = 1; step <= 10; step += 1) {
    await page.mouse.move(x - step * 20, y - step * 10);
  }
  await page.mouse.up();
  const after = await scroll();
  expect(after.left).toBeGreaterThan(100);
  expect(after.top).toBeGreaterThan(40);
  await expect(summary).toBeVisible();
  await expect(page.getByRole('complementary', { name: /^Panel de / })).toHaveCount(0);
  // Un toque normal sí elige la mesa.
  await canvas.getByRole('button', { name: /^T17, / }).click();
  await expect(page.getByRole('complementary', { name: 'Panel de T17' })).toBeVisible();
  await page.keyboard.press('Escape');
  await views(page, testInfo, 'mesas-desplazamiento', {
    only: 1280,
    check: async () => {
      await expect(canvas).toHaveAttribute('data-overflow', 'true');
    },
  });
  // A 1920 × 1080 las 35 mesas caben sin desplazamiento.
  await page.setViewportSize({ width: 1920, height: 1080 });
  await checkFit(false, terraceNames);
  await page.setViewportSize({ width: 1280, height: 720 });

  // 8. Sin conexión: el servidor «Sin conexión» en rojo y «Reintentar» en Mesas (§3.5, decisión 9).
  await page.evaluate(() => {
    (window as unknown as { __adelOffline: boolean }).__adelOffline = true;
  });
  const offline = page.locator('.app-topbar .server-status-offline');
  await expect(offline).toHaveText('Sin conexión', { timeout: 15000 });
  expect(await computed(offline, 'color')).toBe(await token(page, '--danger'));
  const retry = page.getByRole('button', { name: 'Reintentar', exact: true });
  await expect(retry).toBeVisible();
  await expect(page.getByRole('button', { name: 'Actualizar', exact: true })).toHaveCount(0);
  await expectNothingCut(appBar(page));
  await page.locator('[data-sonner-toaster] .app-toast-close').first().click();
  await views(page, testInfo, 'barra-sin-conexion', {
    element: appBar(page),
    check: async () => {
      await expect(offline).toBeVisible();
      await expectNothingCut(appBar(page));
    },
  });
  await page.evaluate(() => {
    (window as unknown as { __adelOffline: boolean }).__adelOffline = false;
  });
  await retry.click();
  await expect(page.locator('.app-topbar .server-status-online')).toBeVisible({ timeout: 15000 });
  await expect(retry).toHaveCount(0);

  // 9. Cuentas y Para llevar: sin «Actualizar»; la tarjeta de la cuenta abierta por mucho tiempo en
  // ámbar y el detalle con «Capturar productos en esta cuenta» como único botón principal.
  await page
    .getByRole('group', { name: 'Vistas operativas' })
    .getByRole('button', { name: 'Cuentas', exact: true })
    .click();
  await expect(page.getByRole('button', { name: 'Actualizar', exact: true })).toHaveCount(0);
  const cards = page.getByRole('group', { name: 'Cuentas disponibles' });
  const agedCard = cards.getByRole('button').filter({ hasText: 'Mesa 103' }).first();
  await expect(agedCard).toHaveClass(/account-long-open/);
  await expect(agedCard.locator('.account-long-open-time')).toContainText('3 d 14 h');
  const card = cards.getByRole('button').filter({ hasText: 'Mesa 102' }).first();
  await card.click();
  const bill = page.getByRole('complementary', { name: 'Cuenta actual' });
  const captureAccount = bill.getByRole('button', {
    name: 'Capturar productos en esta cuenta',
    exact: true,
  });
  await expect(captureAccount).toHaveClass(/primary-button/);
  await expect(bill.getByRole('button', { name: /^Cobrar · \$/ })).toHaveClass(
    /capture-checkout-outline/,
  );
  await expect(bill.getByRole('button', { name: /^Enviar a cocina/ })).toHaveClass(
    /secondary-button/,
  );
  await expect(bill.locator('.primary-button')).toHaveCount(1);
  await views(page, testInfo, 'cuentas-detalle', {
    check: async () => {
      expect(await computed(captureAccount, 'background-color')).toBe(
        await token(page, '--accent'),
      );
    },
  });
  await openSection(page, 'Para llevar');
  await expect(page.getByRole('button', { name: 'Nuevo pedido', exact: true })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Actualizar', exact: true })).toHaveCount(0);

  // 10. Captura: la barra reducida conserva Caja y el usuario, sin «Actualizar»; «Reintentar»
  // solo sin conexión. Dentro de la captura «Enviar a cocina» y «Cobrar» siguen como principales.
  await openSection(page, 'Mesas');
  await page
    .getByRole('group', { name: 'Áreas del restaurante' })
    .getByRole('button', { name: 'Salón BC', exact: true })
    .click();
  await canvas.getByRole('button', { name: /^Mesa 102, Ocupada/ }).click();
  await panel.getByRole('button', { name: 'Capturar pedidos en esta mesa', exact: true }).click();
  const reduced = page.locator('.capture-topbar');
  await expect(reduced.getByRole('button', { name: /^Caja · / })).toBeVisible();
  await expect(reduced.getByRole('button', { name: /^Menú del usuario/ })).toBeVisible();
  await expect(reduced.getByRole('button', { name: 'Actualizar' })).toHaveCount(0);
  await expect(reduced.getByRole('button', { name: 'Reintentar' })).toHaveCount(0);
  await expect(
    page.getByRole('complementary', { name: 'Cuenta actual' }).getByRole('button', {
      name: /^Cobrar · \$/,
    }),
  ).toHaveClass(/primary-button/);
  await page.evaluate(() => {
    (window as unknown as { __adelOffline: boolean }).__adelOffline = true;
  });
  await expect(reduced.getByRole('button', { name: 'Reintentar' })).toBeVisible({
    timeout: 15000,
  });
  const reducedCut = await reduced.evaluate((root) =>
    Array.from(root.querySelectorAll<HTMLElement>('.cash-shortcut button, .user-menu button'))
      .filter((element) => element.scrollWidth > element.clientWidth + 1)
      .map((element) => element.textContent),
  );
  expect(reducedCut).toEqual([]);
  await page.evaluate(() => {
    (window as unknown as { __adelOffline: boolean }).__adelOffline = false;
  });
  await reduced.getByRole('button', { name: 'Reintentar' }).click();
  await expect(reduced.getByRole('button', { name: 'Reintentar' })).toHaveCount(0, {
    timeout: 15000,
  });
  await reduced.locator('.capture-topbar-back').click();
  await expect(panel).toBeVisible();

  // 11. «Áreas y distribución» (diseñador) sigue funcionando: mover una mesa con las flechas y
  // guardar la distribución.
  await page
    .getByRole('group', { name: 'Vistas operativas' })
    .getByRole('button', { name: 'Áreas y distribución', exact: true })
    .click();
  const designer = page.locator('.table-canvas[aria-label="Diseñador de distribución de mesas"]');
  await page
    .locator('.table-map-designer')
    .getByRole('group', { name: 'Áreas del restaurante' })
    .getByRole('button', { name: 'Salón BC', exact: true })
    .click();
  const movable = designer.getByRole('button', { name: /^Mesa 105, / });
  await expect(movable).toBeVisible();
  await movable.focus();
  await page.keyboard.press('ArrowRight');
  await page.getByRole('button', { name: 'Guardar distribución', exact: true }).click();
  await expect
    .poll(async () => {
      const overview = await call(driver, 'GET', 'restaurant/overview');
      return (overview.tables as { id: string; x: number }[]).find(
        (entry) => entry.id === created['Mesa 105']!.id,
      )?.x;
    })
    .toBe(470);

  // 12. «Nuevo pedido para llevar» del resumen: la misma apertura de «Nuevo pedido», en Para llevar.
  await page
    .getByRole('group', { name: 'Vistas operativas' })
    .getByRole('button', { name: 'Mesas', exact: true })
    .click();
  // La Mesa 102 sigue elegida desde el paso 10: Escape vuelve al resumen.
  await expect(panel).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(summary).toBeVisible();
  await summary.getByRole('button', { name: 'Nuevo pedido para llevar', exact: true }).click();
  await expect(page.getByRole('heading', { name: /^Para llevar · Pedido #/ })).toBeVisible();
  await reduced.locator('.capture-topbar-back').click();
  await expect(
    topBar(page).getByRole('button', { name: 'Para llevar', exact: true }),
  ).toHaveAttribute('aria-current', 'page');
  await expect(page.getByRole('button', { name: 'Nuevo pedido', exact: true })).toBeVisible();
  expect(errors).toEqual([]);
});
