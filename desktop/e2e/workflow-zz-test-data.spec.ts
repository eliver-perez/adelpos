import { randomUUID } from 'node:crypto';
import { mkdirSync } from 'node:fs';
import { join } from 'node:path';
import { expect, test } from '@playwright/test';
import type { Page } from '@playwright/test';
import { installDriver } from './native-driver';
import { openAdministration } from './navigation';
import { notice } from './notices';
import { receiptPrinterName, receiptPrinterQueue } from './payments-flow';

// Etapa 9F, fase A (§3 y §8): «Quitar datos de prueba» en Administración › Sistema › Mantenimiento con
// Go y PostgreSQL reales. Por el puente se crean los datos con las firmas exactas de los archivos de
// prueba (la impresora «Destino controlado E2E» de kitchen-flow.ts con sus estaciones «Cocina caliente» y
// «Freidoras» y la ruta de «Platillos», y la impresora de tickets «Tickets controlados E2E» de
// payments-flow.ts: desde la revisión 9F (B3) Go no acepta una impresora de otra terminal como impresora
// de tickets); en la corrida completa ya los
// dejaron las especificaciones anteriores, con ventas y trabajos de impresión, y se reutilizan. La
// pantalla debe mostrar exactamente la lista que calcula Go, confirmar en rojo que primero se respalda,
// avisar con el nombre del respaldo y dejar la instalación sin nada por quitar.
//
// Se llama «zz» para correr al final de la corrida completa (orden alfabético, una base por motor): deja la
// base compartida sin impresora de tickets, sin la ruta de «Platillos» y con las estaciones y destinos de
// prueba desactivados, de lo que dependen las especificaciones anteriores.

type Driver = Awaited<ReturnType<typeof installDriver>>;

interface Entry {
  kind: string;
  id: string;
  name: string;
  reason: string;
}

interface Preview {
  plan_id: string;
  remove: Entry[];
  deactivate: Entry[];
  unlink: Entry[];
  keep: Entry[];
}

const acceptancePrinter = 'Destino controlado E2E';
const acceptanceDestination = 'Agente de aceptación no disponible';

async function call(driver: Driver, method: string, route: string, body?: unknown) {
  const response = await driver.request(method, route, body);
  expect(response.status, `${method} ${route}: ${JSON.stringify(response.body)}`).toBe(200);
  return response.body;
}

async function login(page: Page) {
  await page.getByLabel('Usuario', { exact: true }).fill('admin');
  await page.getByLabel('Contraseña', { exact: true }).fill('admin');
  await page.getByRole('button', { name: 'Entrar', exact: true }).click();
  await expect(page.getByRole('navigation', { name: 'Navegación principal' })).toBeVisible();
}

// Crea (o reutiliza) los datos de prueba de kitchen-flow.ts y payments-flow.ts por el puente.
async function prepareTestData(driver: Driver) {
  const printers = (await call(driver, 'GET', 'kitchen/printers')) as {
    id: string;
    name: string;
  }[];
  let printerID = printers.find((printer) => printer.name === acceptancePrinter)?.id;
  if (!printerID) {
    const created = await call(driver, 'POST', 'kitchen/printers', {
      operation_id: randomUUID(),
      name: acceptancePrinter,
      transport: 'agent',
      destination: acceptanceDestination,
      port: 0,
      paper_width: 80,
      copies: 1,
      supports_cut: false,
      cut: false,
      cutter_option: '',
      active: true,
      fallback_id: null,
    });
    printerID = created.id as string;
  }
  const stations = (await call(driver, 'GET', 'kitchen/stations')) as {
    id: string;
    name: string;
  }[];
  const stationIDs: string[] = [];
  for (const [name, reception] of [
    ['Cocina caliente', 'printer'],
    ['Freidoras', 'both'],
  ] as const) {
    let stationID = stations.find((station) => station.name === name)?.id;
    if (!stationID) {
      const created = await call(driver, 'POST', 'kitchen/stations', {
        operation_id: randomUUID(),
        name,
        description: '',
        active: true,
        sort_order: 0,
        reception,
        printer_id: printerID,
      });
      stationID = created.id as string;
    }
    stationIDs.push(stationID);
  }
  const categories = (await call(driver, 'GET', 'catalog/categories?search=Platillos')) as {
    items: { id: string; name: string }[];
  };
  const category = categories.items.find((item) => item.name === 'Platillos');
  expect(category, 'el catálogo mínimo de la aceptación crea «Platillos»').toBeTruthy();
  const routes = (await call(driver, 'GET', 'kitchen/routes')) as { category_id: string | null }[];
  if (!routes.some((route) => route.category_id === category!.id)) {
    await call(driver, 'POST', 'kitchen/routes', {
      operation_id: randomUUID(),
      category_id: category!.id,
      mode: 'prepare',
      station_ids: stationIDs,
    });
  }
  // payments-flow.ts (configureReceiptPrinter): la impresora de tickets de la aceptación, una cola del
  // equipo que no existe (firma de datos de prueba).
  let receiptID = printers.find((printer) => printer.name === receiptPrinterName)?.id;
  if (!receiptID) {
    const created = await call(driver, 'POST', 'kitchen/printers', {
      operation_id: randomUUID(),
      name: receiptPrinterName,
      transport: 'cups',
      destination: receiptPrinterQueue,
      port: 9100,
      paper_width: 80,
      copies: 1,
      cutter_option: '',
      accepts_escpos: false,
      active: true,
      fallback_id: null,
    });
    receiptID = created.id as string;
  }
  const settings = await call(driver, 'GET', 'payments/settings');
  await call(driver, 'PUT', 'payments/settings', {
    operation_id: randomUUID(),
    revision: settings.revision,
    shared_shifts: settings.shared_shifts,
    print_receipt_default: settings.print_receipt_default,
    receipt_printer_id: receiptID,
    refund_authorization_threshold: settings.refund_authorization_threshold,
  });
}

function names(entries: Entry[]) {
  return entries.map((entry) => entry.name);
}

test('quitar datos de prueba con respaldo previo (etapa 9F, fase A)', async ({ page }) => {
  test.setTimeout(240000);
  const errors: string[] = [];
  page.on('pageerror', (error) => errors.push(error.message));
  const driver = await installDriver(page);
  await call(driver, 'POST', 'auth/login', { username: 'admin', password: 'admin' });
  await call(driver, 'PUT', 'terminal/preferences', { keyboard_mode: 'disabled' });
  await prepareTestData(driver);

  // La lista que calcula Go es la referencia de lo que la pantalla debe mostrar.
  const expected = (await call(driver, 'GET', 'maintenance/test-data')) as Preview;
  const touched = [...names(expected.remove), ...names(expected.deactivate)];
  for (const name of [acceptancePrinter, 'Cocina caliente', 'Freidoras']) {
    expect(touched, `«${name}» debe quitarse o desactivarse`).toContain(name);
  }
  expect(names(expected.unlink)).toContain('Impresora de tickets de Cobros');
  await driver.request('POST', 'auth/logout');

  await page.goto('/');
  await login(page);
  await openAdministration(page, 'Mantenimiento');
  await expect(page.getByRole('heading', { name: 'Mantenimiento', exact: true })).toBeVisible();
  const panel = page.getByRole('region', { name: 'Quitar datos de prueba' });
  // Nada se consulta ni se ofrece quitar antes de revisar.
  await expect(panel.getByRole('button', { name: 'Quitar datos de prueba' })).toHaveCount(0);
  await panel.getByRole('button', { name: 'Revisar datos de prueba', exact: true }).click();
  const groups: [keyof Preview, string][] = [
    ['remove', 'Se eliminará'],
    ['deactivate', 'Se desactivará'],
    ['unlink', 'Se desvinculará'],
    ['keep', 'Se conserva'],
  ];
  for (const [key, title] of groups) {
    const entries = expected[key] as Entry[];
    const list = panel.getByRole('list', { name: title, exact: true });
    if (entries.length === 0) {
      await expect(list).toHaveCount(0);
      continue;
    }
    await expect(list.getByRole('listitem')).toHaveCount(entries.length);
    for (const entry of entries) {
      await expect(list).toContainText(entry.name);
      await expect(list).toContainText(entry.reason);
    }
  }
  // Captura opcional de la lista revisada (ADEL_SHOTS_DIR), como en las especificaciones de la 9D.
  const shots = process.env.ADEL_SHOTS_DIR;
  if (shots) {
    mkdirSync(shots, { recursive: true });
    const engine = process.env.ADEL_BROWSER_ENGINE ?? 'browser';
    await page.screenshot({
      path: join(shots, `${engine}-mantenimiento-lista.png`),
      fullPage: true,
    });
  }
  const quitar = panel.getByRole('button', { name: 'Quitar datos de prueba', exact: true });
  await expect(quitar).toHaveClass(/danger-button/);
  await quitar.click();
  const dialog = page.getByRole('alertdialog', { name: 'Quitar datos de prueba' });
  await expect(dialog).toContainText('Primero se crea un respaldo de la base de datos');
  await expect(dialog).toContainText('no se tocan');
  const confirm = dialog.getByRole('button', { name: 'Quitar datos de prueba', exact: true });
  await expect(confirm).toHaveClass(/danger-solid-button/);
  await confirm.click();

  // Aviso con lo quitado y el nombre del respaldo (sin carpeta); después, nada por quitar.
  await expect(notice(page, 'Datos de prueba quitados.')).toBeVisible({ timeout: 60000 });
  const backup = page.locator('[data-sonner-toaster]').getByText(/Respaldo previo: /);
  await expect(backup).toContainText(
    /Respaldo previo: adel_pos_test_[0-9a-z_]+-\d{8}T\d{6}Z-[0-9a-f]{6}\.dump/,
  );
  await expect(backup).not.toContainText('/');
  await expect(panel.getByText('No hay datos de prueba.', { exact: true })).toBeVisible();
  await expect(
    panel.getByText(/Respaldo creado antes de quitar los datos de prueba/),
  ).toBeVisible();

  // Comprobación con Go: la impresora de prueba ya no es destino de nada.
  const after = (await call(driver, 'GET', 'maintenance/test-data')) as Preview;
  expect(after.remove.length + after.deactivate.length + after.unlink.length).toBe(0);
  const settings = await call(driver, 'GET', 'payments/settings');
  expect(settings.receipt_printer_id).toBeNull();
  const printers = (await call(driver, 'GET', 'kitchen/printers')) as {
    name: string;
    active: boolean;
  }[];
  expect(
    printers.filter((printer) => printer.name === acceptancePrinter && printer.active),
  ).toEqual([]);
  const stations = (await call(driver, 'GET', 'kitchen/stations')) as {
    name: string;
    active: boolean;
  }[];
  expect(
    stations.filter(
      (station) => ['Cocina caliente', 'Freidoras'].includes(station.name) && station.active,
    ),
  ).toEqual([]);
  expect(errors).toEqual([]);
});
