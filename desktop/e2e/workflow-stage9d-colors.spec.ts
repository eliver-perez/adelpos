import { randomUUID } from 'node:crypto';
import { copyFileSync, mkdirSync } from 'node:fs';
import { join } from 'node:path';
import { expect, test } from '@playwright/test';
import type { Locator, Page, TestInfo } from '@playwright/test';
import { installDriver } from './native-driver';
import { cashButton, openSection } from './navigation';
import { notice } from './notices';

// Etapa 9D, fase A (§1 y §7): color de marca azul, paleta de estados de mesa con icono, avisos por
// tipo, importes en el color del texto, acciones destructivas en rojo, indicadores de la barra y
// cursor «no disponible» en los controles deshabilitados. Mismo transporte que las demás
// especificaciones (Go temporal y base adel_pos_test_*). Capturas en tema claro y oscuro a
// 1280 × 720 y 1920 × 1080 (copia opcional en ADEL_SHOTS_DIR).
//
// Datos reales por la API: mesas libre, ocupada, con pago parcial (pedido confirmado y abono en
// efectivo con el turno abierto) y fuera de servicio, y un pedido para llevar. «Esperando pago»
// exige una precuenta impresa, que necesita una impresora de tickets; para la captura, el puente de
// la prueba presenta esa mesa (ocupada, con su pedido confirmado) como «Esperando pago». «Sin
// conexión» se simula como lo vería la terminal: el puente nativo rechaza las peticiones a Go.

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
    width: 110,
    height: 100,
    seats: 4,
    active: true,
  })) as { id: string; revision: number };
}

// Cuenta abierta en la mesa; con producto, el pedido queda confirmado (enviado a cocina).
async function account(
  driver: Driver,
  values: Record<string, unknown>,
  productID?: string,
): Promise<string> {
  const opened = await call(driver, 'POST', 'restaurant/accounts', {
    operation_id: randomUUID(),
    ...values,
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
// vista y una captura (de la página, de la región indicada o de un elemento).
async function views(
  page: Page,
  testInfo: TestInfo,
  name: string,
  options: {
    check?: (theme: string) => Promise<void>;
    element?: Locator;
    clipHeight?: number;
    pause?: number;
  } = {},
) {
  for (const size of sizes) {
    await page.setViewportSize(size);
    for (const theme of themes) {
      // La aplicación vuelve a aplicar el tema del usuario cuando se actualiza su sesión: la captura
      // se repite si el tema cambió mientras se tomaba.
      const file = `${name}-${theme}-${size.width}x${size.height}.png`;
      for (let attempt = 0; attempt < 3; attempt += 1) {
        await setTheme(page, theme);
        await page.waitForTimeout(options.pause ?? 200);
        const overflow = await page.evaluate(
          () => document.documentElement.scrollWidth - window.innerWidth,
        );
        expect(overflow, `${name} se desborda a ${size.width} × ${size.height} (${theme})`).toBe(0);
        await options.check?.(theme);
        const path = testInfo.outputPath(file);
        if (options.element) {
          await options.element.screenshot({ path });
        } else if (options.clipHeight) {
          await page.screenshot({
            path,
            clip: { x: 0, y: 0, width: size.width, height: options.clipHeight },
          });
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

// Color calculado de un token del tema vigente («rgb(r, g, b)»), para compararlo con el color
// calculado de un elemento.
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

// Cierra los avisos que sigan a la vista para que no tapen las capturas de las pantallas.
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

const toastTokens = {
  success: '--accent',
  info: '--info',
  warning: '--warning',
  error: '--danger',
} as const;

// Aviso visible del tipo indicado: su icono, el color del icono y del borde en ambos temas, y sus
// capturas (la barra y el aviso). trigger vuelve a mostrar el aviso si se cerró solo.
async function toastViews(
  page: Page,
  testInfo: TestInfo,
  tone: keyof typeof toastTokens,
  icon: string,
  trigger?: () => Promise<void>,
) {
  const toast = page.locator(`[data-sonner-toaster] .app-toast-${tone}`).first();
  await expect(toast).toBeVisible();
  await expect(toast.locator('svg.app-toast-icon')).toHaveAttribute('data-icon', icon);
  await views(page, testInfo, `aviso-${tone}`, {
    clipHeight: 260,
    pause: 150,
    check: async () => {
      if (!(await toast.isVisible()) && trigger) {
        await trigger();
      }
      await expect(toast).toBeVisible();
      const expected = await token(page, toastTokens[tone]);
      expect(await computed(toast.locator('svg.app-toast-icon'), 'color')).toBe(expected);
      expect(await computed(toast, 'border-left-color')).toBe(expected);
    },
  });
}

test('color de marca, estados, avisos, destructivos y cursor (etapa 9D, fase A)', async ({
  page,
}, testInfo) => {
  test.setTimeout(360000);
  const errors: string[] = [];
  page.on('pageerror', (error) => errors.push(error.message));
  const driver = await installDriver(page);
  await call(driver, 'POST', 'auth/login', { username: 'admin', password: 'admin' });
  // En la corrida completa restaurant.spec.ts deja el teclado virtual siempre visible.
  await call(driver, 'PUT', 'terminal/preferences', { keyboard_mode: 'disabled' });

  // 1. Datos por la API.
  const category = await call(driver, 'POST', 'catalog/categories', {
    name: 'Platillos 9D',
    active: true,
    color: '#d64545',
  });
  const product = await call(driver, 'POST', 'catalog/products', {
    name: 'Enchiladas 9D',
    active: true,
    category_id: category.id,
    code: 'ENCHILADAS-9D',
    kind: 'product',
    base_price: '150',
  });
  const productID = product.id as string;
  const area = (await call(driver, 'POST', 'restaurant/areas', {
    operation_id: randomUUID(),
    name: 'Salón 9D',
    active: true,
  })) as { id: string };
  await table(driver, area.id, 'Mesa 91', 20);
  const occupied = await table(driver, area.id, 'Mesa 92', 150);
  const awaiting = await table(driver, area.id, 'Mesa 93', 280);
  const partial = await table(driver, area.id, 'Mesa 94', 410);
  const blocked = await table(driver, area.id, 'Mesa 95', 540);
  const releasable = [
    await table(driver, area.id, 'Mesa 96', 670),
    await table(driver, area.id, 'Mesa 97', 800),
  ];
  await account(driver, { mode: 'dine_in', table_id: occupied.id });
  await account(driver, { mode: 'dine_in', table_id: awaiting.id }, productID);
  const partialAccount = await account(
    driver,
    { mode: 'dine_in', table_id: partial.id },
    productID,
  );
  for (const entry of releasable) {
    await account(driver, { mode: 'dine_in', table_id: entry.id });
  }
  await account(driver, { mode: 'takeaway', name: 'Cliente 9D' }, productID);
  await call(driver, 'POST', `restaurant/tables/${blocked.id}/service`, {
    operation_id: randomUUID(),
    revision: blocked.revision,
    out_of_service: true,
  });
  // Turno abierto (o el que ya estuviera) y abono parcial en efectivo a la cuenta de la Mesa 94.
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
  const finance = await call(driver, 'GET', `payments/accounts/${partialAccount}`);
  await call(driver, 'POST', `payments/accounts/${partialAccount}/payments`, {
    operation_id: randomUUID(),
    mode: 'partial',
    amount: '50',
    expected_balance: finance.balance,
    method_id: cash!.id,
    received: '50',
  });
  await driver.request('POST', 'auth/logout');

  // Puente de la prueba: «Esperando pago» para la Mesa 93 y la terminal sin conexión a pedido.
  await page.addInitScript((awaitingID) => {
    const bridge = window as unknown as {
      __TAURI_INTERNALS__: {
        invoke: (command: string, args?: Record<string, unknown>) => Promise<unknown>;
      };
      __adelOffline?: boolean;
    };
    const original = bridge.__TAURI_INTERNALS__.invoke;
    bridge.__TAURI_INTERNALS__.invoke = async (command, args) => {
      if (command === 'api_request' && bridge.__adelOffline) {
        throw 'No se pudo conectar con el equipo servidor.';
      }
      const result = (await original(command, args)) as {
        body?: { tables?: { id: string; state: string }[] };
      };
      if (command === 'api_request' && args?.operation === 'restaurant_overview') {
        for (const entry of result.body?.tables ?? []) {
          if (entry.id === awaitingID) {
            entry.state = 'awaiting_payment';
          }
        }
      }
      return result;
    };
  }, awaiting.id);

  await page.setViewportSize({ width: 1280, height: 720 });
  await page.goto('/');
  await page.getByLabel('Usuario', { exact: true }).fill('admin');
  await page.getByLabel('Contraseña', { exact: true }).fill('admin');
  await page.getByRole('button', { name: 'Entrar', exact: true }).click();
  const mac = await page.evaluate(() =>
    /Mac|iPhone|iPad/.test(`${navigator.platform} ${navigator.userAgent}`),
  );

  // 2. Mapa: cada estado con su color (tokens --table-*) y su icono, también «Libre».
  await openSection(page, 'Mesas');
  const areas = page.getByRole('group', { name: 'Áreas del restaurante' });
  await areas.getByRole('button', { name: 'Salón 9D', exact: true }).click();
  const canvas = page.locator('.table-canvas[aria-label="Mapa de mesas"]');
  const tiles = {
    free: canvas.getByRole('button', { name: /^Mesa 91, Libre/ }),
    occupied: canvas.getByRole('button', { name: /^Mesa 92, Ocupada/ }),
    awaiting_payment: canvas.getByRole('button', { name: /^Mesa 93, Esperando pago/ }),
    partial_payment: canvas.getByRole('button', { name: /^Mesa 94, Pago parcial/ }),
    out_of_service: canvas.getByRole('button', { name: /^Mesa 95, Fuera de servicio/ }),
  };
  const icons = {
    free: 'check',
    occupied: 'clock',
    awaiting_payment: 'receipt',
    partial_payment: 'cash',
    out_of_service: 'lock',
  };
  const tileTokens = {
    free: 'free',
    occupied: 'occupied',
    awaiting_payment: 'awaiting',
    partial_payment: 'partial',
    out_of_service: 'out',
  };
  for (const [state, tile] of Object.entries(tiles) as [keyof typeof tiles, Locator][]) {
    await expect(tile).toBeVisible();
    await expect(tile.locator('.table-tile-state svg')).toHaveAttribute('data-icon', icons[state]);
    expect(await computed(tile, 'color'), state).toBe(
      await token(page, `--table-${tileTokens[state]}-text`),
    );
  }
  expect(await computed(tiles.occupied, 'background-color')).toBe(
    await token(page, '--table-occupied-bg'),
  );
  expect(await computed(tiles.partial_payment, 'border-top-color')).toBe(
    await token(page, '--grape'),
  );
  const legend = page.getByRole('list', { name: 'Estados de las mesas' });
  await expect(legend.locator('svg')).toHaveCount(5);
  await expect(legend.locator('svg[data-icon="check"]')).toHaveCount(1);

  // 3. Mesa libre: «Cobrar» deshabilitado con el cursor «no disponible» (§1.6); el de progreso
  // solo mientras se espera de verdad (aria-busy).
  await tiles.free.click();
  const freePanel = page.getByRole('complementary', { name: 'Panel de Mesa 91' });
  await expect(freePanel.locator('.table-panel-state svg')).toHaveAttribute('data-icon', 'check');
  const checkout = freePanel.getByRole('button', { name: /^Cobrar/ });
  await expect(checkout).toBeDisabled();
  expect(await computed(checkout, 'cursor')).toBe('not-allowed');
  await freePanel.evaluate((element) => element.setAttribute('aria-busy', 'true'));
  expect(await computed(checkout, 'cursor')).toBe('progress');
  await freePanel.evaluate((element) => element.removeAttribute('aria-busy'));
  expect(await computed(checkout, 'cursor')).toBe('not-allowed');
  // Un botón habilitado conserva la mano.
  expect(
    await computed(freePanel.getByRole('button', { name: 'Capturar', exact: true }), 'cursor'),
  ).toBe('pointer');

  // 4. Aviso de información (turquesa): la precuenta con el atajo sin una cuenta con productos.
  const precheckHint = 'Elige una cuenta con productos enviados a cocina.';
  const showInfo = async () => {
    // El atajo no actúa mientras se escribe en un campo: se quita el foco antes de pulsarlo.
    await page.evaluate(() => (document.activeElement as HTMLElement | null)?.blur());
    await page.keyboard.press(`${mac ? 'Meta' : 'Control'}+p`);
    await expect(notice(page, precheckHint)).toBeVisible();
  };
  await showInfo();
  await toastViews(page, testInfo, 'info', 'info', showInfo);

  // 5. Ocupada seleccionada: mismo relleno azul claro, borde azul principal más grueso.
  await tiles.occupied.click();
  await expect(tiles.occupied).toHaveClass(/selected-table/);
  expect(await computed(tiles.occupied, 'outline-width')).toBe('4px');
  expect(await computed(tiles.occupied, 'outline-color')).toBe(await token(page, '--accent'));
  expect(await computed(tiles.occupied, 'border-top-color')).toBe(await token(page, '--accent'));

  // 6. Pago parcial seleccionado: total y saldo en el color del texto y en negritas (§1.4).
  await tiles.partial_payment.click();
  await expect(tiles.partial_payment).toHaveClass(/selected-table/);
  expect(await computed(tiles.partial_payment, 'outline-width')).toBe('3px');
  const partialPanel = page.getByRole('complementary', { name: 'Panel de Mesa 94' });
  const balance = partialPanel.locator('.table-panel-balance dd');
  await expect(balance).toHaveText('$100.00');
  await expect(partialPanel.locator('.table-panel-state svg')).toHaveAttribute('data-icon', 'cash');
  await clearToasts(page);
  await views(page, testInfo, 'mesas', {
    check: async () => {
      expect(await computed(balance, 'color')).toBe(await token(page, '--text'));
      expect(Number(await computed(balance, 'font-weight'))).toBeGreaterThanOrEqual(700);
      await expect(legend).toBeVisible();
    },
  });

  // 7. Barra: «Turno abierto» y «En línea» en azul.
  const bar = page.locator('.app-topbar');
  const cashState = cashButton(page);
  await expect(cashState).toHaveAttribute('data-shift-state', 'open', { timeout: 20000 });
  await clearToasts(page);
  const server = page.locator('.app-topbar .server-status');
  const activeSection = page.locator('.app-topbar .topbar-section-active').first();
  await views(page, testInfo, 'barra-turno-abierto', {
    element: bar,
    check: async () => {
      // Sección activa: fondo suave con su borde y texto en el azul principal (§1.1).
      expect(await computed(activeSection, 'background-color')).toBe(
        await token(page, '--accent-soft'),
      );
      expect(await computed(activeSection, 'border-top-color')).toBe(
        await token(page, '--accent-border'),
      );
      expect(await computed(activeSection, 'color')).toBe(await token(page, '--accent'));
      expect(
        await computed(page.locator('.app-topbar .cash-shortcut-indicator'), 'background-color'),
      ).toBe(await token(page, '--accent'));
      expect(await computed(server.locator('.server-status-dot'), 'background-color')).toBe(
        await token(page, '--accent'),
      );
    },
  });

  // 8. «Liberar mesa» en rojo con su confirmación roja; el aviso de éxito es azul con palomita.
  const releaseNames = ['Mesa 96', 'Mesa 97'];
  const release = async () => {
    const name = releaseNames.shift();
    expect(name, 'mesas para liberar').toBeTruthy();
    await canvas.getByRole('button', { name: new RegExp(`^${name}, Ocupada`) }).click();
    const panel = page.getByRole('complementary', { name: `Panel de ${name}` });
    const button = panel.getByRole('button', { name: 'Liberar mesa', exact: true });
    await expect(button).toHaveClass(/danger-button/);
    expect(await computed(button, 'color')).toBe(await token(page, '--danger'));
    await button.click();
    const dialog = page.getByRole('alertdialog', { name: `¿Liberar ${name}?` });
    await expect(dialog).toHaveClass(/dialog-panel-danger/);
    const confirm = dialog.getByRole('button', { name: 'Liberar mesa', exact: true });
    await expect(confirm).toHaveClass(/danger-solid-button/);
    return { dialog, confirm, name: name! };
  };
  const first = await release();
  await clearToasts(page);
  await views(page, testInfo, 'confirmar-destructivo', {
    check: async () => {
      await expect(first.dialog).toBeVisible();
      expect(await computed(first.confirm, 'background-color')).toBe(await token(page, '--danger'));
    },
  });
  await first.confirm.click();
  await expect(notice(page, `${first.name} quedó libre.`)).toBeVisible();
  await toastViews(page, testInfo, 'success', 'success', async () => {
    const next = await release();
    await next.confirm.click();
    await expect(notice(page, `${next.name} quedó libre.`)).toBeVisible();
  });

  // 9. Mesas › Cuentas y Para llevar: tarjeta seleccionada en azul suave y total en el color del
  // texto.
  await page
    .getByRole('group', { name: 'Vistas operativas' })
    .getByRole('button', { name: 'Cuentas', exact: true })
    .click();
  const cards = page.getByRole('group', { name: 'Cuentas disponibles' });
  const card = cards.getByRole('button').filter({ hasText: 'Mesa 94' }).first();
  await card.click();
  await expect(card).toHaveAttribute('aria-pressed', 'true');
  await clearToasts(page);
  await views(page, testInfo, 'mesas-cuentas', {
    check: async () => {
      expect(await computed(card, 'background-color')).toBe(await token(page, '--accent-soft'));
      expect(await computed(card, 'border-top-color')).toBe(await token(page, '--accent'));
    },
  });
  await openSection(page, 'Para llevar');
  const orders = page.getByRole('group', { name: 'Pedidos para llevar' });
  const order = orders.getByRole('button').filter({ hasText: 'Cliente 9D' }).first();
  await order.click();
  await expect(order).toHaveAttribute('aria-pressed', 'true');
  const total = page.locator('.capture-totals .capture-grand-total dd').first();
  await expect(total).toBeVisible();
  await clearToasts(page);
  await views(page, testInfo, 'para-llevar', {
    check: async () => {
      expect(await computed(total, 'color')).toBe(await token(page, '--text'));
    },
  });

  // 10. Caja en arqueo: ámbar. El puente de la prueba comparte la sesión de la página: las
  // peticiones directas a Go se hacen como el mismo administrador.
  const closing = await call(driver, 'POST', `payments/shifts/${shift!.id}/closing`, {
    operation_id: randomUUID(),
  });
  await expect(cashState).toHaveAttribute('data-shift-state', 'closing', { timeout: 20000 });
  await clearToasts(page);
  await views(page, testInfo, 'barra-arqueo', {
    element: bar,
    check: async () => {
      expect(
        await computed(page.locator('.app-topbar .cash-shortcut-indicator'), 'background-color'),
      ).toBe(await token(page, '--warning-mark'));
    },
  });

  // 11. Sin conexión: el servidor en rojo y el aviso de advertencia (ámbar con triángulo).
  await page.evaluate(() => {
    (window as unknown as { __adelOffline: boolean }).__adelOffline = true;
  });
  const offline = page.locator('.app-topbar .server-status-offline');
  await expect(offline).toBeVisible({ timeout: 15000 });
  await toastViews(page, testInfo, 'warning', 'alert');
  await views(page, testInfo, 'barra-sin-conexion', {
    element: bar,
    check: async () => {
      expect(await computed(offline, 'color')).toBe(await token(page, '--danger'));
    },
  });
  await page.evaluate(() => {
    (window as unknown as { __adelOffline: boolean }).__adelOffline = false;
  });
  await expect(page.locator('.app-topbar .server-status-online')).toBeVisible({ timeout: 15000 });
  await expect(page.locator('[data-sonner-toaster] .app-toast-warning')).toHaveCount(0, {
    timeout: 10000,
  });

  // 12. Aviso de error (rojo con equis): la confirmación de la contraseña no coincide (no llega a
  // Go).
  await page.getByRole('button', { name: /^Menú del usuario/ }).click();
  await page
    .getByRole('group', { name: 'Menú del usuario' })
    .getByRole('button', { name: 'Cambiar contraseña' })
    .click();
  await page.getByLabel('Contraseña actual').fill('admin');
  await page.getByLabel('Nueva contraseña').fill('nueva-clave-9d');
  await page.getByLabel('Confirmación').fill('otra-clave-9d');
  await page
    .locator('form')
    .getByRole('button', { name: 'Cambiar contraseña', exact: true })
    .click();
  await expect(notice(page, 'La confirmación no coincide.')).toBeVisible();
  await toastViews(page, testInfo, 'error', 'error');
  await page
    .locator('[data-sonner-toaster] .app-toast-error')
    .getByRole('button', { name: 'Cerrar aviso' })
    .click();

  // 13. Turno cerrado: gris. El arqueo vacío se confirma para no dejar el turno abierto a las
  // especificaciones siguientes.
  await call(driver, 'POST', `payments/shifts/${shift!.id}/closing/confirm`, {
    operation_id: randomUUID(),
    count_revision: closing.count.revision,
    print: false,
  });
  await expect(cashState).toHaveAttribute('data-shift-state', 'none', { timeout: 20000 });
  await clearToasts(page);
  await views(page, testInfo, 'barra-sin-turno', {
    element: bar,
    check: async () => {
      expect(await computed(page.locator('.app-topbar .cash-shortcut-state'), 'color')).toBe(
        await token(page, '--muted'),
      );
    },
  });
  expect(errors).toEqual([]);
});
