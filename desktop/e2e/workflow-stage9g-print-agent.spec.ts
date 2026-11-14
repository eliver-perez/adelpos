import { copyFileSync, mkdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { expect, test } from '@playwright/test';
import type { Page, TestInfo } from '@playwright/test';
import { installDriver } from './native-driver';
import { openAdministration, openSection } from './navigation';
import { notice } from './notices';
import { chooseOption, optionLabels } from './select';

// Etapa 9G, fase E (§6, decisiones D10 y D13) contra Go y PostgreSQL reales, con el agente de impresión de la
// segunda terminal simulado en el puente (native-driver.ts, como print_agent.rs: solo la credencial de la terminal
// y un lp simulado que nunca imprime):
//   1. La terminal «Caja 9G-E» informa las impresoras de su computadora; Sistema › Terminales las muestra con cuál
//      se puede usar.
//   2. En Cocina › Impresoras se registra «Impresora USB 9G-E» eligiendo la terminal y una de sus impresoras (la de
//      oficina no se ofrece); la lista dice en qué terminal está.
//   3. Con la aplicación de esa terminal cerrada, la impresión de prueba espera y la cola dice «La Caja 9G-E está
//      desconectada»; al abrirse (el agente simulado trabaja), la reclama, la entrega a su cola en modo raw con
//      argumentos separados y la cola dice «Enviado a la impresora».
//   4. En Cobros › Políticas de cobro la impresora de tickets de una caja se elige viendo en qué terminal está; se
//      deja otra vez la general para no cambiar las especificaciones siguientes.
// La cola de la impresora es «ADEL_E2E_SIN_IMPRESORA», que no existe en ninguna computadora: ni un agente real la
// imprimiría (firma de datos de prueba en maintenance/signatures.go). Capturas en tema claro y oscuro (copia
// opcional en ADEL_SHOTS_DIR).

type Driver = Awaited<ReturnType<typeof installDriver>>;

const serverURL = process.env.ADEL_BROWSER_SERVER_URL ?? 'http://127.0.0.1:8183';
const clientVersion = (
  JSON.parse(readFileSync(new URL('../src/generated/product.json', import.meta.url), 'utf8')) as {
    version: string;
  }
).version;
const terminalName = 'Caja 9G-E';
const printerName = 'Impresora USB 9G-E';
const queue = 'ADEL_E2E_SIN_IMPRESORA';

async function call(driver: Driver, method: string, route: string, body?: unknown) {
  const response = await driver.request(method, route, body);
  expect(response.status, `${method} ${route}: ${JSON.stringify(response.body)}`).toBe(200);
  return response.body;
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

async function shots(page: Page, testInfo: TestInfo, name: string) {
  for (const theme of ['light', 'dark']) {
    await page.evaluate((value) => {
      document.documentElement.dataset.theme = value;
    }, theme);
    await page.waitForTimeout(200);
    const overflow = await page.evaluate(
      () => document.documentElement.scrollWidth - window.innerWidth,
    );
    expect(overflow, `${name} se desborda (${theme})`).toBe(0);
    const file = `${name}-${theme}.png`;
    await page.screenshot({ path: testInfo.outputPath(file) });
    keep(testInfo, file);
  }
  await page.evaluate(() => {
    document.documentElement.dataset.theme = 'light';
  });
}

async function login(page: Page) {
  await page.getByLabel('Usuario', { exact: true }).fill('admin');
  await page.getByLabel('Contraseña', { exact: true }).fill('admin');
  await page.getByRole('button', { name: 'Entrar', exact: true }).click();
  await expect(page.getByRole('navigation', { name: 'Navegación principal' })).toBeVisible();
}

test('Impresión con varias terminales: impresoras de cada terminal, desconectada y de vuelta, e impresora de tickets por caja (etapa 9G, fase E)', async ({
  page,
  browser,
}, testInfo) => {
  test.setTimeout(180000);
  const errors: string[] = [];
  page.on('pageerror', (error) => errors.push(error.message));
  const driver = await installDriver(page);
  await page.setViewportSize({ width: 1280, height: 720 });

  // La segunda terminal se autoriza por la API; su aplicación queda «cerrada» (el agente no trabaja todavía).
  await call(driver, 'POST', 'auth/login', { username: 'admin', password: 'admin' });
  const invitation = await call(driver, 'POST', 'devices/invitations');
  const enrolled = await fetch(`${serverURL}/api/v1/devices/enroll`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'X-Client-Version': clientVersion,
      'X-Client-Kind': 'desktop',
    },
    body: JSON.stringify({ code: invitation.code, name: terminalName }),
  });
  expect(enrolled.status).toBe(200);
  const credentials = (await enrolled.json()) as { device_id: string; device_token: string };
  await call(driver, 'POST', 'auth/logout');
  const contextB = await browser.newContext({ viewport: { width: 1280, height: 720 } });
  const pageB = await contextB.newPage();
  const terminal = await installDriver(pageB, {
    terminal: { deviceID: credentials.device_id, deviceToken: credentials.device_token },
  });

  // 1. La terminal informa las impresoras de su computadora (solo lecturas en la real).
  const reported = await terminal.printAgent.report([
    { queue, model: 'POS-80 Series', connection: 'usb', printer_type: 36876 },
    {
      queue: 'Oficina_9GE',
      model: 'Generic PostScript Printer',
      connection: 'ipp',
      printer_type: null,
    },
  ]);
  expect(reported.printers.map((printer) => [printer.queue, printer.usable])).toEqual([
    [queue, true],
    ['Oficina_9GE', false],
  ]);
  await page.goto('/');
  await login(page);
  await openAdministration(page, 'Terminales');
  const row = page.locator('tr').filter({ hasText: terminalName });
  await row.getByRole('button', { name: 'Ver configuración', exact: true }).click();
  const configuration = page.getByRole('region', { name: `Configuración de ${terminalName}` });
  const local = configuration.getByRole('region', {
    name: `Impresoras de la computadora de ${terminalName}`,
  });
  await expect(local).toContainText(`${queue} · POS-80 Series · Se puede usar`);
  await expect(local).toContainText('Oficina_9GE · Generic PostScript Printer · No se puede usar');
  await expect(local).toContainText('Desconectada: sus trabajos esperan');
  await expect(configuration).toContainText('Sin impresoras propias');
  await local.scrollIntoViewIfNeeded();
  await shots(page, testInfo, 'terminal-impresoras');

  // 2. Cocina › Impresoras: la impresora USB de la terminal se elige de lo que informó.
  await openSection(page, 'Cocina');
  await page.getByRole('button', { name: 'Impresoras', exact: true }).click();
  await page.getByRole('button', { name: 'Registrar impresora', exact: true }).click();
  await page.getByLabel('Nombre descriptivo de impresora').fill(printerName);
  await chooseOption(page.getByRole('combobox', { name: 'Transporte de impresión', exact: true }), {
    value: 'agent',
  });
  await chooseOption(
    page.getByRole('combobox', { name: 'Terminal donde está conectada', exact: true }),
    { value: credentials.device_id },
  );
  const queueField = page.getByRole('combobox', { name: 'Impresora de esa terminal', exact: true });
  expect(await optionLabels(queueField)).toEqual([
    `${queue} · POS-80 Series`,
    'Oficina_9GE · Generic PostScript Printer',
  ]);
  await chooseOption(queueField, `${queue} · POS-80 Series`);
  await page.getByRole('button', { name: 'Guardar impresora', exact: true }).click();
  await expect(notice(page, 'Impresora guardada.')).toBeVisible();
  await expect(page.locator('.kitchen-list-button').filter({ hasText: printerName })).toContainText(
    `Impresora USB de ${terminalName}`,
  );

  // 3. Con la terminal cerrada, la prueba espera y la cola lo dice; al abrirse, la imprime.
  await page
    .getByRole('button', { name: `Enviar una impresión de prueba a ${printerName}`, exact: true })
    .click();
  await expect(
    notice(page, `Impresión de prueba enviada a la cola de ${printerName}.`),
  ).toBeVisible();
  await page.getByRole('button', { name: 'Cola de impresión', exact: true }).click();
  const list = page.getByRole('list', { name: 'Trabajos de impresión' });
  const card = list.getByRole('listitem').filter({ hasText: printerName }).first();
  await expect(card).toContainText(`La ${terminalName} está desconectada.`);
  await expect(card).toContainText(`${printerName} · en ${terminalName}`);
  await expect(card).toContainText('Pendiente');
  await expect(card.getByRole('button', { name: 'Reintentar' })).toHaveCount(0);
  await card.scrollIntoViewIfNeeded();
  await shots(page, testInfo, 'cola-terminal-desconectada');
  const processed = await terminal.printAgent.run();
  expect(processed).toHaveLength(1);
  expect(processed[0]).toMatchObject({ drawer: false, state: 'submitted' });
  expect(processed[0]!.size).toBeGreaterThan(0);
  // Los argumentos separados de lp en modo raw, como en print_agent.rs (nunca un intérprete de órdenes).
  expect(terminal.printAgent.calls[0]).toEqual([
    '-d',
    queue,
    '-o',
    'raw',
    '-n',
    '1',
    '-t',
    `ADEL-${processed[0]!.id}`,
    `adel-agent-${processed[0]!.id}.bin`,
  ]);
  await expect(card).toContainText('Enviado a la impresora', { timeout: 15000 });
  await expect(card).not.toContainText('desconectada');

  // 4. Cobros › Políticas de cobro: la impresora de tickets de una caja se elige viendo dónde está.
  await openAdministration(page, 'Cobros');
  await page.getByRole('button', { name: /Políticas de cobro/ }).click();
  const ownPrinter = page.getByRole('combobox', { name: /^Impresora propia de / }).first();
  await chooseOption(ownPrinter, `${printerName} · en ${terminalName}`);
  await page.getByRole('button', { name: 'Guardar políticas', exact: true }).click();
  await expect(notice(page, 'Políticas de cobro guardadas.')).toBeVisible();
  const settings = await call(driver, 'GET', 'payments/settings');
  const chosen = (
    settings.register_printers as { printer_name: string | null; terminal_name: string | null }[]
  ).filter((register) => register.printer_name === printerName);
  expect(chosen).toEqual([expect.objectContaining({ terminal_name: terminalName })]);
  await expect(notice(page, 'Políticas de cobro guardadas.')).toHaveCount(0, { timeout: 10000 });
  await chooseOption(ownPrinter, 'Usar la impresora de tickets general');
  await page.getByRole('button', { name: 'Guardar políticas', exact: true }).click();
  await expect(notice(page, 'Políticas de cobro guardadas.')).toBeVisible();
  const restored = await call(driver, 'GET', 'payments/settings');
  expect(
    (restored.register_printers as { printer_id: string | null }[]).every(
      (register) => register.printer_id === null,
    ),
  ).toBe(true);
  // La impresora de la terminal ya no tiene trabajos pendientes: nada quedó esperando.
  const jobs = (await call(driver, 'GET', 'kitchen/jobs')) as {
    printer_name: string;
    state: string;
  }[];
  expect(
    jobs.filter((job) => job.printer_name === printerName && job.state !== 'submitted'),
  ).toEqual([]);
  await contextB.close();
  expect(errors).toEqual([]);
});
