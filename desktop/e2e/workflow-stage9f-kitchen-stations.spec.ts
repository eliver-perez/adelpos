import { randomUUID } from 'node:crypto';
import { copyFileSync, mkdirSync } from 'node:fs';
import { join } from 'node:path';
import { expect, test } from '@playwright/test';
import type { Locator, Page, TestInfo } from '@playwright/test';
import { installDriver } from './native-driver';
import { appBar, openAdministration, openSection, topBar } from './navigation';
import { notice } from './notices';
import { chooseOption, chooseOptions, optionLabels } from './select';

// Etapa 9F, fase C (§5 y §8): terminal de cocina con Go y PostgreSQL reales. El administrador configura
// por la interfaz la terminal (pantalla inicial «Cocina» y «Estaciones que muestra esta terminal»: dos
// de tres estaciones) y el usuario de cocina («Estaciones que puede atender»: una). El usuario de cocina,
// con solo permisos de cocina, entra directo a Cocina › Preparación con la barra superior reducida a
// Cocina, ve solo la intersección (la estación que la terminal muestra y él atiende), las tarjetas con
// color y texto según el tiempo contra el objetivo, botones de al menos 64 px, sin «Actualizar» y con
// «Reintentar» solo sin conexión. Con otra estación en el usuario no hay estaciones en común y la
// pantalla lo explica. Al terminar se restaura la terminal (Mesas y todas las estaciones), porque las
// especificaciones siguientes comparten la base.
//
// Go calcula los tiempos con su hora y la prueba no puede envejecer una comanda: el puente de la prueba
// presenta dos pedidos como de hace 12 y 20 minutos (ámbar y rojo); «Sin conexión» se simula rechazando
// las peticiones en el puente (como en las especificaciones de la 9D). Capturas en tema claro y oscuro
// a 1280 × 720 y 1920 × 1080 (copia opcional en ADEL_SHOTS_DIR).

type Driver = Awaited<ReturnType<typeof installDriver>>;

const cookUser = 'cocina_9f';
const cookPassword = 'Clave de cocina 9F';
const terminalName = 'WebView temporal';
const stations = ['Plancha 9F-C', 'Freidora 9F-C', 'Barra 9F-C'] as const;
const product = 'Hamburguesa 9F-C';

async function call(driver: Driver, method: string, route: string, body?: unknown) {
  const response = await driver.request(method, route, body);
  expect(response.status, `${method} ${route}: ${JSON.stringify(response.body)}`).toBe(200);
  return response.body;
}

// Pedido para llevar con el producto confirmado (enviado a cocina a sus dos estaciones).
async function sentOrder(driver: Driver, customer: string, productID: string) {
  const opened = await call(driver, 'POST', 'restaurant/accounts', {
    operation_id: randomUUID(),
    mode: 'takeaway',
    name: customer,
  });
  const route = `restaurant/accounts/${opened.account.id as string}`;
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
  const bill = await call(driver, 'GET', route);
  return bill.account.folio as number;
}

// Datos por la API (antes de abrir la aplicación): tres estaciones de pantalla de cocina, un producto
// con ruta a la plancha y a la freidora, tres pedidos enviados y el usuario de cocina.
async function prepare(driver: Driver) {
  const existing = (await call(driver, 'GET', 'kitchen/stations')) as { name: string }[];
  for (const name of stations) {
    if (!existing.some((station) => station.name === name)) {
      await call(driver, 'POST', 'kitchen/stations', {
        operation_id: randomUUID(),
        name,
        description: '',
        active: true,
        sort_order: 0,
        reception: 'kds',
        printer_id: null,
      });
    }
  }
  const all = (await call(driver, 'GET', 'kitchen/stations')) as { id: string; name: string }[];
  const id = (name: string) => all.find((station) => station.name === name)!.id;
  const category = await call(driver, 'POST', 'catalog/categories', {
    name: 'Cocina 9F-C',
    active: true,
    color: '#d64545',
  });
  const created = await call(driver, 'POST', 'catalog/products', {
    name: product,
    active: true,
    category_id: category.id,
    code: 'HAMB-9FC',
    kind: 'product',
    base_price: '120',
  });
  await call(driver, 'POST', 'kitchen/routes', {
    operation_id: randomUUID(),
    product_id: created.id,
    mode: 'prepare',
    station_ids: [id('Plancha 9F-C'), id('Freidora 9F-C')],
  });
  const folios = {
    late: await sentOrder(driver, 'Cliente atrasado 9F', created.id as string),
    warning: await sentOrder(driver, 'Cliente por vencer 9F', created.id as string),
    normal: await sentOrder(driver, 'Cliente a tiempo 9F', created.id as string),
  };
  const role = await call(driver, 'POST', 'roles', {
    name: 'Cocina 9F-C',
    permissions: [
      'kitchen.states.read',
      'kitchen.states.manage',
      'kitchen.tickets.read',
      'kitchen.kds.ack',
      'events.read',
    ],
  });
  await call(driver, 'POST', 'users', {
    username: cookUser,
    display_name: 'Cocinera 9F',
    role_id: role.id,
    active: true,
    password: cookPassword,
  });
  return folios;
}

// Restaura la terminal compartida: Mesas como inicio y todas las estaciones, sin aviso sonoro.
async function restoreTerminal(driver: Driver) {
  await call(driver, 'POST', 'auth/login', { username: 'admin', password: 'admin' });
  const devices = (await call(driver, 'GET', 'devices')) as {
    id: string;
    name: string;
    register_id: string | null;
    keyboard_mode: string;
  }[];
  const device = devices.find((entry) => entry.name === terminalName)!;
  await call(driver, 'PATCH', `devices/${device.id}`, {
    name: device.name,
    register_id: device.register_id,
    keyboard_mode: device.keyboard_mode,
    home_screen: 'tables',
  });
  const access = (await call(driver, 'GET', 'kitchen/access')) as {
    device_id: string;
    all_stations: boolean;
    station_ids: string[];
  }[];
  const current = access.find((entry) => entry.device_id === device.id)!;
  await call(driver, 'PUT', 'kitchen/access', {
    operation_id: randomUUID(),
    device_id: device.id,
    all_stations: true,
    station_ids: [],
    sound_alert: false,
    expected_all_stations: current.all_stations,
    expected_station_ids: current.station_ids,
  });
  await call(driver, 'POST', 'auth/logout');
}

async function login(page: Page, username: string, password: string) {
  await page.getByLabel('Usuario', { exact: true }).fill(username);
  await page.getByLabel('Contraseña', { exact: true }).fill(password);
  await page.getByRole('button', { name: 'Entrar', exact: true }).click();
  await expect(appBar(page)).toBeVisible();
}

async function logout(page: Page) {
  await page.getByRole('button', { name: /^Menú del usuario/ }).click();
  await page
    .getByRole('group', { name: 'Menú del usuario' })
    .getByRole('button', { name: 'Cerrar sesión', exact: true })
    .click();
  await expect(page.getByRole('heading', { name: 'Inicia sesión' })).toBeVisible();
}

async function setTheme(page: Page, theme: string) {
  await page.evaluate((value) => {
    document.documentElement.dataset.theme = value;
  }, theme);
}

// Valor de un token de color en el tema vigente, como lo calcula el navegador.
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

async function background(locator: Locator) {
  return locator.evaluate((element) => getComputedStyle(element).backgroundColor);
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
}

// Recorre dos tamaños y los dos temas: sin desbordamiento horizontal, la comprobación y una captura.
async function views(page: Page, testInfo: TestInfo, name: string, check: () => Promise<void>) {
  await clearToasts(page);
  for (const size of [
    { width: 1280, height: 720 },
    { width: 1920, height: 1080 },
  ]) {
    await page.setViewportSize(size);
    for (const theme of ['light', 'dark']) {
      await setTheme(page, theme);
      await page.waitForTimeout(200);
      const overflow = await page.evaluate(
        () => document.documentElement.scrollWidth - window.innerWidth,
      );
      expect(overflow, `${name} se desborda a ${size.width} × ${size.height} (${theme})`).toBe(0);
      await check();
      const file = `${name}-${theme}-${size.width}x${size.height}.png`;
      await page.screenshot({ path: testInfo.outputPath(file) });
      keep(testInfo, file);
    }
  }
  await setTheme(page, 'light');
  await page.setViewportSize({ width: 1280, height: 720 });
}

test('terminal de cocina por estaciones: intersección, colores y sin «Actualizar» (etapa 9F, fase C)', async ({
  page,
}, testInfo) => {
  test.setTimeout(300000);
  const errors: string[] = [];
  page.on('pageerror', (error) => errors.push(error.message));
  const driver = await installDriver(page);
  await call(driver, 'POST', 'auth/login', { username: 'admin', password: 'admin' });
  await call(driver, 'PUT', 'terminal/preferences', { keyboard_mode: 'disabled' });
  const folios = await prepare(driver);
  await call(driver, 'POST', 'auth/logout');

  // Puente de la prueba: los pedidos «atrasado» y «por vencer» se presentan como de hace 20 y 12
  // minutos (objetivo 15 y advertencia 15 de una estación nueva: ámbar desde 11 min 15 s), y la
  // terminal sin conexión a pedido.
  await page.addInitScript(
    ({ late, warning }) => {
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
          body?: { tasks?: Record<string, unknown>[] };
        };
        if (command === 'api_request' && args?.operation === 'kitchen_board') {
          for (const task of result?.body?.tasks ?? []) {
            if (task.folio === late) task.elapsed_seconds = 20 * 60;
            if (task.folio === warning) task.elapsed_seconds = 12 * 60;
          }
        }
        return result;
      };
    },
    { late: folios.late, warning: folios.warning },
  );

  try {
    await page.setViewportSize({ width: 1280, height: 720 });
    await page.goto('/');
    await login(page, 'admin', 'admin');

    // 1. Sistema › Terminales: la terminal abre en Cocina.
    await openAdministration(page, 'Terminales');
    const row = page.locator('tbody tr').filter({ hasText: terminalName });
    await row.getByRole('button', { name: 'Editar', exact: true }).click();
    await expect(
      page.getByText(/Las estaciones que muestra y el aviso sonoro se eligen/),
    ).toBeVisible();
    await page
      .getByRole('group', { name: 'Pantalla inicial de la terminal' })
      .getByRole('button', { name: 'Cocina', exact: true })
      .click();
    await page.getByRole('button', { name: 'Guardar cambios', exact: true }).click();
    await expect(notice(page, 'Terminal actualizada.')).toBeVisible();

    // 2. Cocina › Estaciones › Terminales de cocina: la terminal muestra la plancha y la freidora.
    await openSection(page, 'Cocina');
    await page.getByRole('button', { name: 'Estaciones', exact: true }).click();
    await page.getByRole('button', { name: /^Terminales de cocina/ }).click();
    await chooseOption(page.getByRole('combobox', { name: 'Terminal', exact: true }), terminalName);
    await expect(page.getByText('Esta terminal abre en Cocina › Preparación.')).toBeVisible();
    const showAll = page.getByLabel('Mostrar todas las estaciones', { exact: true });
    await expect(showAll).toBeChecked();
    await showAll.uncheck();
    await chooseOptions(
      page.getByRole('combobox', { name: 'Estaciones que muestra esta terminal' }),
      ['Plancha 9F-C', 'Freidora 9F-C'],
    );
    await page.getByRole('button', { name: 'Guardar terminal de cocina', exact: true }).click();
    await expect(notice(page, `Terminal ${terminalName} guardada.`)).toBeVisible();

    // 3. Personal › Usuarios: la cocinera atiende solo la freidora.
    await openAdministration(page, 'Usuarios');
    await page
      .locator('tbody tr')
      .filter({ hasText: cookUser })
      .getByRole('button', { name: 'Administrar', exact: true })
      .click();
    await page.getByRole('button', { name: 'Estaciones de cocina', exact: true }).click();
    const attendsAll = page.getByLabel('Puede atender todas las estaciones', { exact: true });
    await expect(attendsAll).toBeChecked();
    await attendsAll.uncheck();
    await chooseOptions(page.getByRole('combobox', { name: 'Estaciones que puede atender' }), [
      'Freidora 9F-C',
    ]);
    await page.getByRole('button', { name: 'Guardar estaciones', exact: true }).click();
    await expect(notice(page, 'Estaciones del usuario guardadas.')).toBeVisible();
    await logout(page);

    // 4. La cocinera entra directo a Cocina › Preparación; la barra solo tiene Cocina.
    await login(page, cookUser, cookPassword);
    await expect(page.getByRole('heading', { name: 'Cocina y preparación' })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Preparación', exact: true })).toHaveAttribute(
      'aria-pressed',
      'true',
    );
    await expect(topBar(page).getByRole('button')).toHaveCount(1);
    await expect(topBar(page).getByRole('button', { name: 'Cocina', exact: true })).toBeVisible();
    await expect(appBar(page).getByRole('button', { name: /^Caja/ })).toHaveCount(0);
    await expect(
      page.getByRole('group', { name: 'Funciones de cocina' }).getByRole('button'),
    ).toHaveText(['Preparación', 'Comandas']);

    // 5. Intersección: solo la freidora, con los tres pedidos.
    const cards = page.locator('.kitchen-task-card');
    await expect(cards).toHaveCount(3);
    await expect(cards.filter({ hasText: 'Plancha 9F-C' })).toHaveCount(0);
    expect(
      await optionLabels(page.getByRole('combobox', { name: 'Estación de preparación' })),
    ).toEqual(['Todas mis estaciones', 'Freidora 9F-C']);
    const late = cards.filter({ hasText: `#${folios.late} ·` });
    const warning = cards.filter({ hasText: `#${folios.warning} ·` });
    const normal = cards.filter({ hasText: `#${folios.normal} ·` });
    // Más antiguos primero (el orden de Go).
    await expect(cards.first()).toContainText(`#${folios.late} ·`);

    // 6. Colores por tiempo con texto, botones grandes y sin «Actualizar».
    await expect(page.getByRole('button', { name: /Actualizar/ })).toHaveCount(0);
    await expect(page.getByRole('button', { name: 'Reintentar', exact: true })).toHaveCount(0);
    await views(page, testInfo, 'cocina-preparacion', async () => {
      await expect(late).toHaveAttribute('data-urgency', 'late');
      await expect(late).toContainText('Atrasada');
      await expect(warning).toHaveAttribute('data-urgency', 'warning');
      await expect(warning).toContainText('Por vencer');
      await expect(normal).toHaveAttribute('data-urgency', 'normal');
      expect(await background(late)).toBe(await token(page, '--danger-soft'));
      expect(await background(warning)).toBe(await token(page, '--warning-soft'));
      expect(await background(normal)).toBe(await token(page, '--panel'));
      for (const button of await cards.getByRole('button').all()) {
        const box = await button.boundingBox();
        expect(box!.height).toBeGreaterThanOrEqual(64);
      }
    });

    // 7. «Marcar producto listo» en la freidora.
    await normal.getByRole('button', { name: 'Marcar producto listo', exact: true }).click();
    await expect(notice(page, `${product} · listo en Freidora 9F-C`)).toBeVisible();
    await expect(cards).toHaveCount(2);

    // 8. Sin conexión: «Reintentar» aparece y desaparece al volver.
    await page.evaluate(() => {
      (window as unknown as { __adelOffline: boolean }).__adelOffline = true;
    });
    await expect(appBar(page).locator('.server-status-offline')).toHaveText('Sin conexión', {
      timeout: 15000,
    });
    const retry = page.getByRole('button', { name: 'Reintentar', exact: true });
    await expect(retry).toBeVisible();
    await page.evaluate(() => {
      (window as unknown as { __adelOffline: boolean }).__adelOffline = false;
    });
    await retry.click();
    await expect(appBar(page).locator('.server-status-online')).toBeVisible({ timeout: 15000 });
    await expect(retry).toHaveCount(0);
    await logout(page);

    // 9. Sin estaciones en común: la cocinera pasa a atender la barra, que la terminal no muestra.
    await login(page, 'admin', 'admin');
    await openAdministration(page, 'Usuarios');
    await page
      .locator('tbody tr')
      .filter({ hasText: cookUser })
      .getByRole('button', { name: 'Administrar', exact: true })
      .click();
    await page.getByRole('button', { name: 'Estaciones de cocina', exact: true }).click();
    await chooseOptions(page.getByRole('combobox', { name: 'Estaciones que puede atender' }), [
      'Freidora 9F-C',
      'Barra 9F-C',
    ]);
    await page.getByRole('button', { name: 'Guardar estaciones', exact: true }).click();
    await expect(notice(page, 'Estaciones del usuario guardadas.')).toBeVisible();
    await logout(page);
    await login(page, cookUser, cookPassword);
    const scope = page.getByRole('status', { name: 'Sin estaciones en común' });
    await expect(scope).toContainText(
      'Esta terminal muestra Freidora 9F-C y Plancha 9F-C, y tu usuario atiende Barra 9F-C: no hay ' +
        'estaciones en común. Pide que ajusten la terminal o tu usuario.',
    );
    await expect(cards).toHaveCount(0);
    await expect(page.getByText('No hay unidades de preparación en este filtro.')).toHaveCount(0);
    await views(page, testInfo, 'cocina-sin-estaciones-en-comun', async () => {
      await expect(scope).toBeVisible();
    });
    await logout(page);
    expect(errors).toEqual([]);
  } finally {
    await restoreTerminal(driver);
  }
});
