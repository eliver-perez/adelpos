import { randomBytes, randomUUID } from 'node:crypto';
import { expect, test } from '@playwright/test';
import type { Page, TestInfo } from '@playwright/test';
import { installDriver } from './native-driver';
import { admin, call, lightTheme, login, shot } from './license-flow';
import { openAdministration, openSection } from './navigation';
import { PairingRequestError, claimPairing, requestPairingCode } from './pairing';
import { chooseOptions } from './select';

// Etapa K1, Fase E (§6 de la especificación; K1-D6, K1-D9, K1-D15, K1-D17 y K1-D30), con Go y PostgreSQL reales: una
// tableta se empareja como pantalla de cocina (por la API, como lo hará la aplicación de cocina) y se conecta. En
// Sistema › Terminales se ve como «Pantalla de cocina», con su versión instalada, su conexión y el cupo que ocupa; se
// cambian sus estaciones, letra, tema y sonido, y los cambios le llegan a la tableta al momento (su evento de
// configuración). Los colores por tiempo se cambian en Cocina › Comandas y tiempos. «Liberar cupo» dice que no es
// definitivo (la tableta lo pide otra vez con «Usar esta pantalla») y «Revocar» la desconecta.

const serverURL = process.env.ADEL_BROWSER_SERVER_URL ?? 'http://127.0.0.1:8183';

interface Display {
  device_id: string;
  device_token: string;
}

// Una petición de la pantalla de cocina con su credencial (sin sesión), como la aplicación de cocina.
// La versión de la aplicación de cocina que dice la tableta en cada petición (se actualiza encima a media prueba).
let installedVersion = '0.1.0';

async function asDisplay(display: Display, method: string, route: string, body?: unknown) {
  const response = await fetch(`${serverURL}/api/v1/${route}`, {
    method,
    headers: {
      'Content-Type': 'application/json',
      'X-Device-ID': display.device_id,
      'X-Device-Token': display.device_token,
      'X-Client-Kind': 'kitchen',
      'X-Client-Version': installedVersion,
    },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  return { status: response.status, body: (await response.json()) as Record<string, any> };
}

async function displayOrders(display: Display) {
  const reply = await asDisplay(display, 'GET', 'kitchen-display/orders');
  expect(reply.status, JSON.stringify(reply.body)).toBe(200);
  return reply.body;
}

// Los eventos que recibe la pantalla desde el cursor.
async function displayEvents(display: Display, after: number) {
  const reply = await asDisplay(display, 'GET', `events/snapshot?after=${after}`);
  expect(reply.status).toBe(200);
  return (reply.body.events as { kind: string; aggregate_id: string | null }[]).map(
    (event) => event.kind,
  );
}

// Go acepta 5 solicitudes de emparejamiento por origen cada 10 minutos (regla de la 9G); en la corrida completa esta
// llega después de las de la 9G, la A1 y la A2: se espera a que vuelva a aceptarla, como en la A2.
async function requestDisplayCodeWhenAllowed(name: string) {
  const deadline = Date.now() + 11 * 60_000;
  for (;;) {
    try {
      return await requestPairingCode(name, 'Android 14', 'kitchen');
    } catch (error) {
      const limited = error instanceof PairingRequestError && error.code === 'PAIRING_RATE_LIMITED';
      if (!limited || Date.now() > deadline) {
        throw error;
      }
      await new Promise((resolve) => setTimeout(resolve, 20_000));
    }
  }
}

// Captura en una ventana alta: la barra de «Guardar» queda fija al pie del área con desplazamiento y, en una captura de
// página completa, se dibuja a media imagen encima de los campos.
async function tallShot(page: Page, testInfo: TestInfo, name: string) {
  await page.setViewportSize({ width: 1280, height: 2200 });
  await shot(page, testInfo, name, { fullPage: false });
  await page.setViewportSize({ width: 1280, height: 900 });
}

function deviceRow(page: Page, name: string) {
  return page.locator('tr').filter({ hasText: name });
}

test('una pantalla de cocina en Terminales: ajustes, versión instalada, colores, cupo y revocar (etapa K1)', async ({
  page,
}, testInfo) => {
  test.setTimeout(15 * 60_000);
  installedVersion = '0.1.0';
  await page.setViewportSize({ width: 1280, height: 900 });
  const driver = await installDriver(page);
  await call(driver, 'POST', 'auth/login', admin);
  await lightTheme(driver);
  const suffix = randomBytes(3).toString('hex');
  // Dos estaciones que reciben comandas en pantalla.
  for (const name of [`Plancha ${suffix}`, `Fríos ${suffix}`]) {
    await call(driver, 'POST', 'kitchen/stations', {
      operation_id: randomUUID(),
      name,
      active: true,
      reception: 'kds',
    });
  }
  // La tableta pide su código como pantalla de cocina; la computadora principal lo aprueba.
  const name = `Tableta de cocina ${suffix}`;
  const pairing = await requestDisplayCodeWhenAllowed(name);
  await call(driver, 'POST', `devices/pairing-requests/${pairing.requestID}/approve`, {
    code: pairing.code,
  });
  const claimed = await claimPairing(pairing);
  expect(claimed.status).toBe(200);
  const display = claimed.body as unknown as Display;
  const connected = await asDisplay(display, 'POST', 'kitchen-display/connect', {});
  expect(connected.status, JSON.stringify(connected.body)).toBe(200);
  await call(driver, 'POST', 'auth/logout');

  await login(page, admin.username, admin.password);
  await openAdministration(page, 'Terminales');
  const row = deviceRow(page, name);
  await expect(row).toContainText('Pantalla de cocina');
  await expect(row).toContainText('Android 14 · versión instalada 0.1.0');
  // La versión instalada (K1-D9): la tableta actualizada encima lo dice en su siguiente petición.
  installedVersion = '0.1.1';
  await asDisplay(display, 'GET', 'kitchen-display/orders');
  await expect(row).toContainText('versión instalada 0.1.1', { timeout: 15000 });
  // También la computadora principal dice su versión instalada.
  await expect(page.locator('tbody')).toContainText(
    /Esta computadora · versión instalada \d+\.\d+\.\d+/,
  );

  await row.getByRole('button', { name: 'Ver configuración', exact: true }).click();
  const panel = page.getByRole('region', { name: `Configuración de ${name}` });
  await expect(panel).toContainText('Ocupa un cupo de pantalla de cocina');
  await expect(panel).toContainText('Todas las que reciben comandas en pantalla');
  await expect(panel).toContainText('En línea');
  await expect(panel).toContainText('Oscuro');
  await shot(page, testInfo, 'k1-terminales-configuracion');
  await panel.getByRole('button', { name: 'Cerrar configuración', exact: true }).click();

  // Editar: solo una estación, letra grande, tema claro, sin sonido. Le llega a la tableta al momento.
  const before = Number((await displayOrders(display)).cursor);
  await row.getByRole('button', { name: 'Editar', exact: true }).click();
  const editor = page.locator('section').filter({ hasText: `Editar ${name}` });
  await editor
    .getByRole('checkbox', {
      name: 'Mostrar todas las estaciones que reciben comandas en pantalla',
    })
    .uncheck();
  await chooseOptions(
    editor.getByRole('combobox', { name: 'Estaciones que muestra esta pantalla' }),
    [`Plancha ${suffix}`],
  );
  await editor
    .getByRole('group', { name: 'Tamaño de letra de la pantalla' })
    .getByRole('button', { name: 'Grande' })
    .click();
  await editor
    .getByRole('group', { name: 'Tema de la pantalla' })
    .getByRole('button', { name: 'Claro' })
    .click();
  await editor
    .getByRole('group', { name: 'Sonido de la pantalla' })
    .getByRole('button', { name: 'Desactivado' })
    .click();
  await expect(editor.getByRole('combobox', { name: 'Volumen' })).toBeDisabled();
  await tallShot(page, testInfo, 'k1-terminales-editar');
  await editor.getByRole('button', { name: 'Guardar cambios', exact: true }).click();
  await expect(page.getByText('Pantalla de cocina actualizada.')).toBeVisible();
  const changed = await displayOrders(display);
  expect(changed.display).toMatchObject({
    text_size: 'large',
    theme: 'light',
    sound_enabled: false,
    all_stations: false,
  });
  expect((changed.display.stations as { name: string }[]).map((station) => station.name)).toEqual([
    `Plancha ${suffix}`,
  ]);
  expect(await displayEvents(display, before)).toContain('kitchen_display.configuration.changed');
  await page.getByRole('button', { name: 'Cerrar editor', exact: true }).click();

  // Los colores por tiempo de todas las pantallas, en Cocina › Comandas y tiempos.
  const limits = Number((await displayOrders(display)).cursor);
  await openSection(page, 'Cocina');
  await page.getByRole('button', { name: 'Comandas y tiempos', exact: true }).click();
  await page.getByLabel('Color de atención desde (minutos de espera)').fill('12');
  await page.getByLabel('Color de retraso desde (minutos de espera)').fill('25');
  await page.getByRole('button', { name: 'Guardar configuración de cocina', exact: true }).click();
  await expect(page.getByText('Configuración de cocina guardada.')).toBeVisible();
  const colors = await displayOrders(display);
  expect(colors.display).toMatchObject({ warning_minutes: 12, late_minutes: 25 });
  expect(await displayEvents(display, limits)).toContain('kitchen_display.configuration.changed');
  await tallShot(page, testInfo, 'k1-cocina-colores');
  await page.getByLabel('Color de atención desde (minutos de espera)').fill('10');
  await page.getByLabel('Color de retraso desde (minutos de espera)').fill('20');
  await page.getByRole('button', { name: 'Guardar configuración de cocina', exact: true }).click();
  await expect(page.getByText('Configuración de cocina guardada.').first()).toBeVisible();

  // «Liberar cupo»: no es definitivo (K1-D30).
  await openAdministration(page, 'Terminales');
  await deviceRow(page, name).getByRole('button', { name: 'Liberar cupo', exact: true }).click();
  const release = page.getByRole('alertdialog');
  await expect(release).toContainText('«Usar esta pantalla»');
  await expect(release).toContainText('revócala');
  await shot(page, testInfo, 'k1-terminales-liberar-cupo', { fullPage: false });
  await release.getByRole('button', { name: 'Liberar cupo', exact: true }).click();
  await expect(page.getByText(`Cupo de ${name} liberado.`)).toBeVisible();
  expect((await asDisplay(display, 'GET', 'kitchen-display/orders')).body.error.code).toBe(
    'KITCHEN_DISPLAY_SEAT_RELEASED',
  );
  await deviceRow(page, name)
    .getByRole('button', { name: 'Ver configuración', exact: true })
    .click();
  await expect(page.getByRole('region', { name: `Configuración de ${name}` })).toContainText(
    'En la tableta se puede pedir otra vez con «Usar esta pantalla».',
    { timeout: 15000 },
  );
  // «Usar esta pantalla» en la tableta lo vuelve a ocupar.
  expect(
    (await asDisplay(display, 'POST', 'kitchen-display/connect', { take_seat: true })).status,
  ).toBe(200);

  // «Revocar» la desconecta al momento.
  await deviceRow(page, name).getByRole('button', { name: 'Revocar', exact: true }).click();
  const revoke = page.getByRole('alertdialog');
  await expect(revoke).toContainText('deja de mostrar pedidos al momento');
  await revoke.getByRole('button', { name: 'Confirmar revocación', exact: true }).click();
  await expect(page.getByText(`Terminal ${name} revocada.`)).toBeVisible();
  const revoked = await asDisplay(display, 'GET', 'kitchen-display/orders');
  expect(revoked.status).toBe(401);
  expect(revoked.body.error.code).toBe('DEVICE_REVOKED');
});
