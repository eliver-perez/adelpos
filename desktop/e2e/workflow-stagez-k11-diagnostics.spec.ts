import { randomBytes, randomUUID } from 'node:crypto';
import { expect, test } from '@playwright/test';
import type { Page, TestInfo } from '@playwright/test';
import { installDriver } from './native-driver';
import { admin, call, dismissNotices, lightTheme, login, shot } from './license-flow';
import type { Driver } from './license-flow';
import { openAdministration, openSection } from './navigation';
import { notice } from './notices';
import { claimPairing } from './pairing';
import { chooseOption } from './select';
import {
  asDisplay,
  categoryWithProduct,
  createPrinter,
  createStation,
  patchStation,
  requestDisplayCodeWhenAllowed,
  setPrinterActive,
  setUnrouted,
} from './kitchen-coverage-flow';
import type { Display } from './kitchen-coverage-flow';

// Etapa K1.1, Fase B (§3, §4.2, §4.3, §4.4 y §6 de la especificación; K11-D5, K11-D7 y K11-D8), con Go y PostgreSQL
// reales:
//   1. «Ver configuración» de una pantalla de cocina en Terminales: qué le llega por estación y cada aviso de Go
//      («La estación … no tiene categorías ni productos asignados.», la impresora desactivada, «Ninguna de sus
//      estaciones recibe comandas en pantalla.» y «Esta pantalla no tiene estaciones.»), cada uno con su botón.
//   2. Cocina › Enrutamiento: la lista «Sin ruta» con lo que dice el ajuste, «Productos sin ruta» («Enviar a una
//      estación» y de vuelta a «No necesitan preparación») y los avisos de rutas a estaciones desactivadas.
//   3. Cocina › Estaciones: «Computadoras en modo Cocina» y la recepción en pantalla en lenguaje sencillo.
//   4. Preparación: «Por enviar a cocina» con un pedido programado y «Enviar a cocina ahora». Desde la etapa M1 (§5.3,
//      M1-D17) quitarle la hora a otro pedido lo envía a cocina en ese momento: ya no espera en el panel.
// Los datos llevan un sufijo propio para convivir con la base compartida de las especificaciones anteriores.

// Captura en una ventana alta: la barra de «Guardar» queda fija al pie del área con desplazamiento y, en una captura de
// página completa, se dibuja a media imagen encima de los campos (como en la K1).
async function tallShot(page: Page, testInfo: TestInfo, name: string) {
  await page.setViewportSize({ width: 1280, height: 1600 });
  await shot(page, testInfo, name, { fullPage: false });
  await page.setViewportSize({ width: 1280, height: 900 });
}

function deviceRow(page: Page, name: string) {
  return page.locator('tr').filter({ hasText: name });
}

// Pedido para llevar enviado a cocina (con hora, si se pide), como en la especificación de la 9D.
async function sendTakeaway(
  driver: Driver,
  productID: string,
  customer: string,
  scheduledLocal?: string,
) {
  const opened = await call(driver, 'POST', 'restaurant/accounts', {
    operation_id: randomUUID(),
    mode: 'takeaway',
    origin: 'Para llevar',
    customer_name: customer,
    ...(scheduledLocal ? { scheduled_local: scheduledLocal, schedule_kind: 'preparation' } : {}),
    items: [
      {
        id: randomUUID(),
        product_id: productID,
        variant_id: null,
        quantity: 1,
        customization: { ingredients: [], groups: [], components: [], notes: '' },
      },
    ],
  });
  const account = opened.account as { id: string; folio: number; revision: number };
  const route = `restaurant/accounts/${account.id}`;
  const preview = await call(driver, 'GET', `${route}/preview`);
  await call(driver, 'POST', `${route}/confirm`, {
    operation_id: randomUUID(),
    revision: account.revision,
    expected_fingerprint: preview.fingerprint,
  });
  return account;
}

// Quitarle la hora a un pedido programado. Hasta la K1.1 lo dejaba esperando a que alguien lo enviara (K11-D9); desde la
// etapa M1 (§5.3, M1-D17) Go lo envía a cocina en ese momento.
async function unschedule(driver: Driver, id: string) {
  const current = (await call(driver, 'GET', `restaurant/accounts/${id}`)).account as Record<
    string,
    unknown
  >;
  const body: Record<string, unknown> = { operation_id: randomUUID() };
  for (const key of [
    'revision',
    'name',
    'mode',
    'table_id',
    'price_list_id',
    'customer_name',
    'phone',
    'origin',
    'notes',
    'assigned_user_id',
  ]) {
    body[key] = current[key] ?? null;
  }
  await call(driver, 'PATCH', `restaurant/accounts/${id}`, {
    ...body,
    scheduled_local: '',
    schedule_kind: '',
  });
}

function tomorrowAt(hour: number) {
  const date = new Date(Date.now() + 24 * 60 * 60 * 1000);
  const pad = (value: number) => String(value).padStart(2, '0');
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(hour)}:00`;
}

test('diagnóstico de una pantalla de cocina, «Sin ruta» y «Productos sin ruta», Estaciones y «Por enviar a cocina» (etapa K1.1)', async ({
  page,
}, testInfo) => {
  test.setTimeout(15 * 60_000);
  await page.setViewportSize({ width: 1280, height: 900 });
  const driver = await installDriver(page);
  await call(driver, 'POST', 'auth/login', admin);
  await lightTheme(driver);
  const suffix = randomBytes(3).toString('hex');
  const kitchen = `Cocina ${suffix}`;
  const bar = `Barra ${suffix}`;
  const printer = await createPrinter(driver, `Impresora K11 ${suffix}`);
  const cocina = await createStation(driver, kitchen, 'kds');
  const barra = await createStation(driver, bar, 'kds');
  const drinks = await categoryWithProduct(driver, `Refrescos ${suffix}`, `K11D-R-${suffix}`);
  const extras = await categoryWithProduct(driver, `Extras ${suffix}`, `K11D-E-${suffix}`);
  const desserts = await categoryWithProduct(driver, `Postres ${suffix}`, `K11D-P-${suffix}`);
  // Extras va a Barra (que más adelante se desactiva).
  await call(driver, 'POST', 'kitchen/routes', {
    operation_id: randomUUID(),
    product_id: null,
    category_id: extras.categoryID,
    mode: 'prepare',
    station_ids: [barra.id],
  });
  // La tableta se empareja y muestra solo la estación nueva.
  const name = `Tableta diagnóstico ${suffix}`;
  const pairing = await requestDisplayCodeWhenAllowed(name);
  await call(driver, 'POST', `devices/pairing-requests/${pairing.requestID}/approve`, {
    code: pairing.code,
  });
  const claimed = await claimPairing(pairing);
  expect(claimed.status).toBe(200);
  const display = claimed.body as unknown as Display;
  expect((await asDisplay(display, 'POST', 'kitchen-display/connect', {})).status).toBe(200);
  await call(driver, 'PUT', 'kitchen/access', {
    operation_id: randomUUID(),
    device_id: display.device_id,
    all_stations: false,
    station_ids: [cocina.id],
    expected_all_stations: true,
    expected_station_ids: [],
  });
  let revoked = false;
  try {
    // La sesión del puente se cierra antes de entrar por la pantalla (como en las especificaciones de la K1 y la 9D);
    // después, las llamadas del puente usan la sesión de la pantalla.
    await call(driver, 'POST', 'auth/logout');
    await login(page, admin.username, admin.password);

    // 1. «Ver configuración»: la estación no tiene categorías; el botón abre el asistente en ese paso.
    await openAdministration(page, 'Terminales');
    await deviceRow(page, name)
      .getByRole('button', { name: 'Ver configuración', exact: true })
      .click();
    let diagnostics = page.getByRole('region', { name: 'Qué recibe esta pantalla' });
    const warnings = diagnostics.getByRole('list', { name: 'Avisos de la pantalla' });
    await expect(warnings).toContainText(
      `La estación ${kitchen} no tiene categorías ni productos asignados.`,
    );
    await expect(diagnostics.getByRole('article', { name: `Estación ${kitchen}` })).toContainText(
      'No le llega ningún producto.',
    );
    await dismissNotices(page);
    await shot(page, testInfo, 'k11-diagnostico-sin-categorias');
    await warnings.getByRole('button', { name: 'Elegir sus categorías', exact: true }).click();
    const steps = page.getByRole('region', { name: `Qué recibe ${name}` });
    await expect(steps.getByText('2. Qué categorías llegan')).toHaveAttribute(
      'aria-current',
      'step',
    );
    await steps.getByRole('checkbox', { name: new RegExp(`^Refrescos ${suffix}`) }).check();
    await steps.getByRole('button', { name: 'Guardar y seguir', exact: true }).click();
    await expect(steps.getByLabel('Resumen de lo que recibe')).toContainText(
      `Esta pantalla recibirá: Refrescos ${suffix}.`,
    );
    await steps.getByRole('button', { name: 'Terminar', exact: true }).click();
    await deviceRow(page, name)
      .getByRole('button', { name: 'Ver configuración', exact: true })
      .click();
    diagnostics = page.getByRole('region', { name: 'Qué recibe esta pantalla' });
    await expect(diagnostics.getByRole('article', { name: `Estación ${kitchen}` })).toContainText(
      `Categorías: Refrescos ${suffix} (1 producto)`,
    );
    await expect(diagnostics.getByRole('list', { name: 'Avisos de la pantalla' })).toHaveCount(0);

    // La impresora de la estación desactivada: el aviso lleva a Cocina › Impresoras.
    await patchStation(driver, cocina.id, { reception: 'both', printer_id: printer.id });
    await setPrinterActive(driver, printer.id, false);
    await expect(diagnostics.getByRole('list', { name: 'Avisos de la pantalla' })).toContainText(
      `La impresora Impresora K11 ${suffix} de ${kitchen} está desactivada`,
      { timeout: 15000 },
    );
    await diagnostics
      .getByRole('button', { name: 'Ir a Cocina › Impresoras', exact: true })
      .click();
    await expect(page.getByRole('button', { name: 'Impresoras', exact: true })).toHaveAttribute(
      'aria-pressed',
      'true',
    );
    await setPrinterActive(driver, printer.id, true);

    // Ninguna de sus estaciones recibe en pantalla: «Configurar qué recibe» ofrece recibir también en pantalla.
    await patchStation(driver, cocina.id, { reception: 'printer' });
    await openAdministration(page, 'Terminales');
    await deviceRow(page, name)
      .getByRole('button', { name: 'Ver configuración', exact: true })
      .click();
    diagnostics = page.getByRole('region', { name: 'Qué recibe esta pantalla' });
    const screenWarnings = diagnostics.getByRole('list', { name: 'Avisos de la pantalla' });
    await expect(screenWarnings).toContainText(
      'Ninguna de sus estaciones recibe comandas en pantalla.',
    );
    await screenWarnings
      .getByRole('button', { name: 'Configurar qué recibe', exact: true })
      .click();
    await expect(steps.getByText('1. Estaciones de esta pantalla')).toHaveAttribute(
      'aria-current',
      'step',
    );
    const printing = steps.locator('li').filter({ hasText: kitchen });
    await printing
      .getByRole('button', { name: 'Recibir también en pantalla', exact: true })
      .click();
    await expect(
      notice(page, `${kitchen} ahora imprime y se ve en la pantalla de cocina.`),
    ).toBeVisible();
    await steps.getByRole('button', { name: 'Saltar el asistente', exact: true }).click();

    // Sin estaciones: con la estación desactivada, «Esta pantalla no tiene estaciones.».
    await patchStation(driver, cocina.id, { active: false });
    await deviceRow(page, name)
      .getByRole('button', { name: 'Ver configuración', exact: true })
      .click();
    diagnostics = page.getByRole('region', { name: 'Qué recibe esta pantalla' });
    await expect(diagnostics.getByRole('list', { name: 'Avisos de la pantalla' })).toContainText(
      'Esta pantalla no tiene estaciones.',
    );
    await expect(
      diagnostics
        .getByRole('list', { name: 'Avisos de la pantalla' })
        .getByRole('button', { name: 'Configurar qué recibe', exact: true }),
    ).toBeVisible();
    // «Estaciones que muestra» dice que la elegida está desactivada.
    await expect(page.getByRole('region', { name: `Configuración de ${name}` })).toContainText(
      `${kitchen} (desactivada)`,
    );
    await dismissNotices(page);
    await shot(page, testInfo, 'k11-diagnostico-sin-estaciones');
    await patchStation(driver, cocina.id, { active: true });

    // 2. Cocina › Enrutamiento: «Sin ruta» con lo que dice el ajuste.
    await openSection(page, 'Cocina');
    await page.getByRole('button', { name: 'Enrutamiento', exact: true }).click();
    const unrouted = page.getByRole('region', { name: 'Productos sin ruta' });
    const list = unrouted.getByRole('region', { name: 'Lo que no va a ninguna estación' });
    await expect(list).toContainText(`Postres ${suffix}`);
    await expect(list).toContainText('(no necesitan preparación).');
    await expect(list).not.toContainText(`Refrescos ${suffix}`);
    await unrouted.getByRole('button', { name: 'Enviar a una estación', exact: true }).click();
    await chooseOption(
      unrouted.getByRole('combobox', { name: 'Estación para los productos sin ruta' }),
      bar,
    );
    await unrouted.getByRole('button', { name: 'Guardar productos sin ruta', exact: true }).click();
    await expect(notice(page, 'Productos sin ruta guardado.')).toBeVisible();
    await expect(list).toContainText(`(van a ${bar}).`);
    // Barra desactivada: lo que iba a ella no necesita preparación por ahora y Enrutamiento lo avisa.
    await patchStation(driver, barra.id, { active: false });
    const routing = unrouted.getByRole('list', { name: 'Avisos de enrutamiento' });
    await expect(routing).toContainText(`Extras ${suffix} va a ${bar}, que está desactivada`, {
      timeout: 15000,
    });
    await expect(routing).toContainText(
      `Los productos sin ruta iban a ${bar}, que está desactivada`,
    );
    await expect(list).toContainText(`Extras ${suffix} (su estación está desactivada)`);
    await expect(list).toContainText('(no necesitan preparación).');
    await dismissNotices(page);
    await shot(page, testInfo, 'k11-enrutamiento-sin-ruta');
    await routing
      .locator('li')
      .filter({ hasText: `Extras ${suffix} va a` })
      .getByRole('button', { name: 'Cambiar la ruta', exact: true })
      .click();
    await expect(page.getByRole('heading', { name: `Ruta de Extras ${suffix}` })).toBeVisible();
    await unrouted.getByRole('button', { name: 'No necesitan preparación', exact: true }).click();
    await unrouted.getByRole('button', { name: 'Guardar productos sin ruta', exact: true }).click();
    await expect(notice(page, 'Productos sin ruta guardado.').first()).toBeVisible();
    await expect(routing.getByText(/Los productos sin ruta iban a/)).toHaveCount(0, {
      timeout: 15000,
    });

    // 3. Cocina › Estaciones: «Computadoras en modo Cocina» y la recepción en pantalla.
    await page.getByRole('button', { name: 'Estaciones', exact: true }).click();
    await page.getByRole('button', { name: /^Computadoras en modo Cocina/ }).click();
    await expect(
      page.getByText('Las pantallas de cocina (tabletas) se configuran en Sistema › Terminales.'),
    ).toBeVisible();
    await page.locator('.kitchen-list-button').filter({ hasText: kitchen }).click();
    await expect(
      page.getByText(
        'Las pantallas de cocina muestran estos pedidos al momento. En Comandas se ve si la pantalla ya los recibió.',
      ),
    ).toBeVisible();
    await dismissNotices(page);
    await tallShot(page, testInfo, 'k11-estaciones');

    // La tableta deja su cupo de pantalla de cocina antes de abrir Preparación en esta computadora.
    await call(driver, 'POST', `devices/${display.device_id}/revoke`, {});
    revoked = true;

    // 4. «Por enviar a cocina»: uno programado para mañana; al otro se le quita la hora y se envía al momento (M1-D17).
    const scheduled = await sendTakeaway(
      driver,
      desserts.productID,
      `Programado ${suffix}`,
      tomorrowAt(13),
    );
    const waiting = await sendTakeaway(
      driver,
      drinks.productID,
      `Sin hora ${suffix}`,
      tomorrowAt(14),
    );
    await unschedule(driver, waiting.id);
    await page.getByRole('button', { name: 'Preparación', exact: true }).click();
    const panel = page.getByRole('button', { name: /^Por enviar a cocina/ });
    await expect(panel).toBeVisible({ timeout: 15000 });
    const scheduledCard = page.getByRole('article', { name: `Pedido #${scheduled.folio}` });
    await expect(scheduledCard).toContainText(
      /Programado: (se envía solo a su hora|espera a que lo envíes)/,
    );
    await expect(
      scheduledCard.getByRole('button', { name: 'Cambiar la hora', exact: true }),
    ).toBeVisible();
    await expect(scheduledCard).toContainText(
      'Cambia cuándo llega a cocina. No cambia el pedido ni su precio.',
    );
    // El pedido sin hora ya está en cocina: no aparece en el panel.
    const waitingCard = page.getByRole('article', { name: `Pedido #${waiting.folio}` });
    await expect(waitingCard).toHaveCount(0);
    const unscheduled = (await call(driver, 'GET', `restaurant/accounts/${waiting.id}`))
      .account as {
      preparation_state: string;
    };
    expect(['pending_release', 'scheduled']).not.toContain(unscheduled.preparation_state);
    await dismissNotices(page);
    await shot(page, testInfo, 'k11-panel');
    await scheduledCard.getByRole('button', { name: 'Enviar a cocina ahora', exact: true }).click();
    await expect(notice(page, `Pedido #${scheduled.folio} enviado a cocina.`)).toBeVisible();
    await expect(scheduledCard).toHaveCount(0);
    for (const old of [
      'Bloqueado por configuración',
      'Liberar a preparación',
      'Ajustar programación',
    ]) {
      await expect(page.getByText(old)).toHaveCount(0);
    }
  } finally {
    await setPrinterActive(driver, printer.id, true);
    await setUnrouted(driver, null);
    if (!revoked) {
      await call(driver, 'POST', `devices/${display.device_id}/revoke`, {});
    }
  }
});
