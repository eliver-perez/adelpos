import { copyFileSync, mkdirSync } from 'node:fs';
import { join } from 'node:path';
import { expect, test } from '@playwright/test';
import type { Locator, Page, TestInfo } from '@playwright/test';
import { formatMoney } from '../src/services/money';
import { cleanServer, installDriver } from './native-driver';
import { openAdministration, topBar } from './navigation';
import { notice } from './notices';
import { chooseOption } from './select';

// Etapa 9H, fase D (§5 y §7 «Asistente»), con Go y PostgreSQL reales: el asistente de migración del sistema
// anterior en Sistema › Mantenimiento, en el menú Archivo y en la paleta.
//
// - La preparación del paquete es simulada por el puente (sin Docker): su avance, su resultado (con un paquete
//   sintético real que queda elegido para importar) y el error DOCKER_MISSING; cada una queda anotada en Go.
// - La subida, el análisis, la importación, el reemplazo, el informe y su PDF son reales, con el paquete sintético
//   de la Fase B (`ad-el-pos-server legacy-synthetic`).
// - Todas las especificaciones comparten la base de cada motor y las anteriores ya crearon operación, catálogo,
//   mesas y usuarios: ahí la importación se rechaza (eso se prueba). El camino completo usa un segundo servidor
//   temporal con una base limpia (ADEL_BROWSER_CLEAN_SERVER_URL y ADEL_BROWSER_CLEAN_PROVISIONING, que arrancan
//   scripts/browser-tests.py, scripts/windows/dev.mjs y el arnés aislado); sin él, esas pruebas se omiten.

type Driver = Awaited<ReturnType<typeof installDriver>>;

const passphrase = 'Frase de migración 9H';
const clean = cleanServer();

async function login(page: Page, username = 'admin', password = 'admin') {
  await page.goto('/');
  await page.getByLabel('Usuario', { exact: true }).fill(username);
  await page.getByLabel('Contraseña', { exact: true }).fill(password);
  await page.getByRole('button', { name: 'Entrar', exact: true }).click();
  await expect(topBar(page)).toBeVisible();
}

async function logout(page: Page) {
  await page.getByRole('button', { name: /^Menú del usuario/ }).click();
  await page
    .getByRole('group', { name: 'Menú del usuario' })
    .getByRole('button', { name: 'Cerrar sesión', exact: true })
    .click();
  await expect(page.getByRole('heading', { name: 'Inicia sesión' })).toBeVisible();
}

function wizardTitle(page: Page) {
  return page.getByRole('heading', { name: 'Migrar del sistema anterior', level: 1 });
}

async function openWizard(page: Page) {
  await openAdministration(page, 'Mantenimiento');
  await page
    .getByRole('region', { name: 'Migrar del sistema anterior' })
    .getByRole('button', { name: 'Abrir el asistente de migración' })
    .click();
  await expect(wizardTitle(page)).toBeVisible();
  await expect(page.getByRole('navigation', { name: 'Ruta de navegación' })).toHaveText(
    'Administración›Sistema›Mantenimiento›Migrar del sistema anterior',
  );
}

function prepareStep(page: Page) {
  return page.getByRole('region', { name: '1. Preparar el paquete' });
}

function importStep(page: Page) {
  return page.getByRole('region', { name: '2. Importar el paquete' });
}

function currentBatch(page: Page) {
  return page.getByRole('region', { name: 'Ya hay datos importados del sistema anterior' });
}

// Captura de página completa en tema claro y oscuro (copia opcional en ADEL_SHOTS_DIR).
async function shot(page: Page, testInfo: TestInfo, name: string) {
  for (const theme of ['light', 'dark']) {
    await page.evaluate((value) => {
      document.documentElement.dataset.theme = value;
    }, theme);
    const file = `${name}-${theme}.png`;
    const path = testInfo.outputPath(file);
    await page.screenshot({ path, fullPage: true });
    const directory = process.env.ADEL_SHOTS_DIR;
    if (directory) {
      mkdirSync(directory, { recursive: true });
      copyFileSync(
        path,
        join(directory, `${process.env.ADEL_BROWSER_ENGINE ?? 'browser'}-${file}`),
      );
    }
  }
  await page.evaluate(() => {
    document.documentElement.dataset.theme = 'light';
  });
}

async function call(driver: Driver, method: string, route: string, body?: unknown) {
  const response = await driver.request(method, route, body);
  expect(response.status, `${method} ${route}: ${JSON.stringify(response.body)}`).toBe(200);
  return response.body;
}

// Elige el paquete (diálogo nativo simulado) y lo revisa con la frase de paso: subida por partes y análisis.
async function reviewPackage(page: Page, driver: Driver, path: string, phrase: string) {
  const step = importStep(page);
  driver.legacy.nextPackage(path);
  await step.getByRole('button', { name: /^Elegir (el|otro) paquete$/ }).click();
  await expect(step).toContainText(path.split('/').pop()!);
  await step.getByLabel('Frase de paso del paquete', { exact: true }).fill(phrase);
  await step.getByRole('button', { name: 'Revisar el paquete', exact: true }).click();
}

// Espera el informe de una importación: en el paso 2 si se siguió el avance o, si terminó mientras se estaba fuera
// del asistente, con «Ver el informe» del lote vigente.
async function importReport(page: Page): Promise<Locator> {
  await expect(currentBatch(page)).toBeVisible({ timeout: 90000 });
  const inStep = importStep(page).getByRole('heading', { name: 'Informe de la importación' });
  const button = currentBatch(page).getByRole('button', { name: 'Ver el informe', exact: true });
  await expect(inStep.or(button).first()).toBeVisible({ timeout: 90000 });
  if (await inStep.isVisible()) {
    return importStep(page);
  }
  await button.click();
  await expect(
    currentBatch(page).getByRole('heading', { name: 'Informe de la importación' }),
  ).toBeVisible();
  return currentBatch(page);
}

test('preparar el paquete (simulado): Docker sin instalar, avance, salir y volver, resultado y Registro de movimientos', async ({
  page,
}, testInfo) => {
  test.setTimeout(120000);
  const driver = await installDriver(page);
  await login(page);
  await openWizard(page);
  // Al abrir el asistente se borran los restos de una preparación anterior (sin esperar).
  await expect.poll(() => driver.legacy.cleanups()).toBeGreaterThan(0);
  const step = prepareStep(page);
  const prepare = step.getByRole('button', { name: 'Preparar el paquete', exact: true });
  await expect(prepare).toBeDisabled();

  // Los archivos se eligen con los diálogos nativos; un diálogo cancelado no cambia nada.
  await step.getByRole('button', { name: 'Elegir el respaldo', exact: true }).click();
  await expect(step).toContainText('Sin elegir');
  driver.legacy.nextBackup('possys-sintetico.bak');
  await step.getByRole('button', { name: 'Elegir el respaldo', exact: true }).click();
  await expect(step).toContainText('possys-sintetico.bak');
  driver.legacy.nextOutput('paquete-9h');
  await step.getByRole('button', { name: 'Elegir dónde guardarlo', exact: true }).click();
  await expect(step).toContainText('paquete-9h.adelmig');
  await expect(prepare).toBeEnabled();

  // La frase de paso se escribe dos veces y se explica que no se puede recuperar.
  await expect(step).toContainText('si se olvida, no se puede recuperar');
  await step.getByLabel('Frase de paso del paquete', { exact: true }).fill('corta');
  await prepare.click();
  await expect(step.getByRole('alert')).toHaveText(
    'La frase de paso debe tener al menos 8 caracteres.',
  );
  await step.getByLabel('Frase de paso del paquete', { exact: true }).fill(passphrase);
  await step.getByLabel('Escríbela otra vez', { exact: true }).fill(`${passphrase}!`);
  await prepare.click();
  await expect(step.getByRole('alert')).toHaveText('Las dos frases de paso no coinciden.');

  // Sin Docker Desktop: qué instalar, con la dirección de docker.com en texto.
  driver.legacy.nextPreparation('DOCKER_MISSING');
  await step.getByLabel('Escríbela otra vez', { exact: true }).fill(passphrase);
  await prepare.click();
  const failure = step.getByRole('alert');
  await expect(failure).toContainText('Falta instalar Docker Desktop');
  await expect(failure).toContainText('https://www.docker.com/products/docker-desktop/');
  await shot(page, testInfo, 'migracion-sin-docker');

  // Preparación correcta: avance por pasos; salir y volver a entrar conserva el avance o el resultado.
  driver.legacy.nextPreparation('success');
  await step.getByLabel('Frase de paso del paquete', { exact: true }).fill(passphrase);
  await step.getByLabel('Escríbela otra vez', { exact: true }).fill(passphrase);
  await prepare.click();
  await expect(step.getByRole('list', { name: 'Avance de la preparación' })).toBeVisible();
  await page
    .getByRole('navigation', { name: 'Ruta de navegación' })
    .getByRole('button', { name: 'Mantenimiento', exact: true })
    .click();
  await page
    .getByRole('region', { name: 'Migrar del sistema anterior' })
    .getByRole('button', { name: 'Abrir el asistente de migración' })
    .click();
  const ready = prepareStep(page).getByRole('status').filter({ hasText: 'El paquete está listo.' });
  await expect(ready).toBeVisible({ timeout: 30000 });
  await expect(ready).toContainText('Ventas del 01/10/2019 al 12/11/2026');
  await expect(ready).toContainText('Respaldo del 12/11/2026');
  await expect(ready.getByRole('list', { name: 'Conteos del paquete preparado' })).toContainText(
    '240Ventas',
  );
  await expect(ready).not.toContainText('No se pudo anotar');
  // El paquete preparado queda elegido en el paso 2.
  await expect(importStep(page)).toContainText('paquete-9h.adelmig');
  await shot(page, testInfo, 'migracion-preparado');

  // Las dos preparaciones quedaron en el Registro de movimientos, sin rutas de archivos.
  expect(driver.legacy.records.map((record) => record.succeeded)).toEqual([false, true]);
  const audit = await call(driver, 'GET', 'audit?action=legacy.preparation_recorded&page_size=10');
  const entries = audit.items as { success: boolean; after_state: unknown }[];
  expect(entries.length).toBeGreaterThanOrEqual(2);
  expect(entries.slice(0, 2).map((entry) => entry.success)).toEqual([true, false]);
  expect(JSON.stringify(entries)).not.toContain('paquete-9h');
  expect(JSON.stringify(entries)).toContain('DOCKER_MISSING');
});

test('rechazo en una instalación con operación propia: el resumen previo dice por qué y no ofrece importar', async ({
  page,
}) => {
  test.setTimeout(120000);
  const driver = await installDriver(page);
  await login(page);
  await openWizard(page);
  const path = driver.legacy.syntheticPackage(passphrase);
  await reviewPackage(page, driver, path, passphrase);
  const step = importStep(page);
  await expect(step.getByRole('heading', { name: 'Resumen previo' })).toBeVisible({
    timeout: 60000,
  });
  const refusal = step.getByRole('alert').filter({ hasText: 'No se puede importar' });
  await expect(refusal).toContainText('No se puede importar en esta instalación.');
  await expect(refusal).toContainText('AD-EL POS');
  await expect(refusal.getByRole('listitem').first()).toBeVisible();
  await expect(step.getByRole('button', { name: 'Importar', exact: true })).toHaveCount(0);
  await expect(step.getByRole('button', { name: 'Reemplazar lo importado' })).toHaveCount(0);
  // Elegir otro paquete cancela la subida anterior (Go la borra).
  const status = await call(driver, 'GET', 'maintenance/legacy');
  expect(status.uploads).toHaveLength(1);
  driver.legacy.nextPackage(path);
  await step.getByRole('button', { name: 'Elegir otro paquete', exact: true }).click();
  await expect(step.getByRole('heading', { name: 'Resumen previo' })).toHaveCount(0);
  await expect
    .poll(async () => (await call(driver, 'GET', 'maintenance/legacy')).uploads)
    .toEqual([]);
  expect(status.current_batch).toBeNull();
});

test('menú Archivo y paleta abren el asistente', async ({ page, browserName }) => {
  const driver = await installDriver(page);
  await login(page);
  await expect.poll(() => driver.menuState()).toContain('file.legacy_migration');
  expect(await driver.clickMenu('file.legacy_migration')).toBe(true);
  await expect(wizardTitle(page)).toBeVisible();
  await page.getByRole('button', { name: /^AD-EL POS · Ir al inicio/ }).click();
  await expect(wizardTitle(page)).toHaveCount(0);
  // El despachador descarta la gemela de una acción que llega por otra vía en menos de 700 ms
  // (commands/dispatcher.ts): la paleta se abre después de esa ventana.
  await page.waitForTimeout(800);
  const mac = await page.evaluate(() =>
    /Mac|iPhone|iPad/.test(`${navigator.platform} ${navigator.userAgent}`),
  );
  await page.keyboard.press(`${mac ? 'Meta' : 'Control'}+k`);
  const palette = page.getByRole('dialog', { name: 'Paleta de comandos' });
  await expect(palette).toBeVisible();
  await page.keyboard.type('migrar');
  await expect(palette.getByRole('option').first()).toContainText('Migrar del sistema anterior…');
  // La paleta no repite el asistente como pantalla: es la acción de Archivo.
  await expect(palette.getByRole('option', { name: /Migrar del sistema anterior/ })).toHaveCount(1);
  await page.keyboard.press('Enter');
  await expect(wizardTitle(page)).toBeVisible();
  expect(browserName).toBeTruthy();
});

test.describe.serial('instalación limpia (segundo servidor temporal)', () => {
  test.skip(
    !clean,
    'Sin ADEL_BROWSER_CLEAN_SERVER_URL: el camino completo necesita una base limpia.',
  );

  async function cleanDriver(page: Page) {
    return installDriver(page, { server: clean!, catalog: false });
  }

  test('asistente completo: frase incorrecta, resumen previo, confirmar, avance, informe y PDF', async ({
    page,
  }, testInfo) => {
    test.setTimeout(180000);
    const driver = await cleanDriver(page);
    await login(page);
    await openWizard(page);
    await expect(currentBatch(page)).toHaveCount(0);
    const path = driver.legacy.syntheticPackage(passphrase);
    const step = importStep(page);

    // Frase de paso incorrecta: Go la rechaza al analizar; se vuelve a escribir sin subir otra vez.
    await reviewPackage(page, driver, path, 'Otra frase de paso');
    await expect(step.getByRole('alert')).toHaveText('La frase de paso no es correcta.', {
      timeout: 60000,
    });
    const status = await call(driver, 'GET', 'maintenance/legacy');
    expect(status.uploads).toHaveLength(1);
    await step.getByLabel('Frase de paso del paquete', { exact: true }).fill(passphrase);
    await step.getByRole('button', { name: 'Revisar el paquete', exact: true }).click();

    // Resumen previo: conteos, periodo, totales, datos incoherentes y avisos.
    await expect(step.getByRole('heading', { name: 'Resumen previo' })).toBeVisible({
      timeout: 60000,
    });
    const runs = (await call(driver, 'GET', 'maintenance/legacy')).recent_runs as {
      id: string;
      kind: string;
      status: string;
    }[];
    const analysis = await call(driver, 'GET', `maintenance/legacy/runs/${runs[0]!.id}`);
    expect(analysis.status).toBe('succeeded');
    expect(analysis.summary.clean).toBe(true);
    const counts = analysis.summary.counts as Record<string, number>;
    await expect(step.locator('.legacy-sentence').first()).toContainText(
      `${counts.sales!.toLocaleString('es-MX')} ventas del ${analysis.summary.sales_period.label.replace(' a ', ' al ')}`,
    );
    await expect(step.getByRole('list', { name: 'Conteos del paquete' })).toContainText(
      'productos',
    );
    await expect(step.getByRole('heading', { name: 'Datos incoherentes' })).toBeVisible();
    await expect(step).toContainText('no guardaba dólares');
    await expect(step.getByRole('alert')).toHaveCount(0);
    await shot(page, testInfo, 'migracion-resumen-previo');

    // Confirmar: la confirmación resume lo que se importa; cancelar no hace nada.
    await step.getByRole('button', { name: 'Importar', exact: true }).click();
    const dialog = page.getByRole('alertdialog', { name: 'Importar el paquete' });
    await expect(dialog).toContainText('ventas');
    await dialog.getByRole('button', { name: 'Revisar otra vez', exact: true }).click();
    await expect(dialog).toHaveCount(0);
    await step.getByRole('button', { name: 'Importar', exact: true }).click();
    await page
      .getByRole('alertdialog', { name: 'Importar el paquete' })
      .getByRole('button', { name: 'Importar', exact: true })
      .click();

    // Avance y, al terminar, el informe con la validación sin diferencias y su PDF.
    await expect(
      step
        .getByRole('progressbar', { name: 'Avance de la importación' })
        .or(step.getByRole('heading', { name: 'Informe de la importación' }))
        .first(),
    ).toBeVisible({ timeout: 30000 });
    const report = await importReport(page);
    await expect(report.getByText('Todo cuadra.')).toBeVisible();
    const countsGroup = report.getByRole('button', { name: /^Conteos/ });
    await expect(countsGroup).toContainText('Todo cuadra');
    await countsGroup.click();
    await expect(report.getByRole('table').first()).toContainText('Cuadra');
    await shot(page, testInfo, 'migracion-informe');
    await expect(report).toContainText('Antes de importar se hizo un respaldo');
    driver.nextSaveDialog('save');
    await report.getByRole('button', { name: 'Descargar informe (PDF)', exact: true }).click();
    await expect(notice(page, 'Informe guardado.')).toBeVisible();
    const pdf = driver.exports.find((entry) => entry.report === 'legacy.report');
    expect(pdf?.bytes.subarray(0, 5).toString()).toBe('%PDF-');
    const after = await call(driver, 'GET', 'maintenance/legacy');
    expect(after.current_batch.status).toBe('completed');
    expect(after.current_batch.has_differences).toBe(false);
    expect(after.current_batch.imported_by).toBe('admin');
    expect(after.uploads).toEqual([]);
  });

  test('si ya hay un lote, el asistente lo dice y el reemplazo pide confirmación en rojo', async ({
    page,
  }) => {
    test.setTimeout(180000);
    const driver = await cleanDriver(page);
    await login(page);
    const before = (await call(driver, 'GET', 'maintenance/legacy')).current_batch;
    await openWizard(page);
    await expect(currentBatch(page)).toContainText('por admin');
    const path = driver.legacy.syntheticPackage(passphrase);
    await reviewPackage(page, driver, path, passphrase);
    const step = importStep(page);
    await expect(step.getByRole('heading', { name: 'Resumen previo' })).toBeVisible({
      timeout: 60000,
    });
    await expect(
      step.getByRole('status').filter({ hasText: 'Ya hay datos importados' }),
    ).toBeVisible();
    await expect(step.getByRole('button', { name: 'Importar', exact: true })).toHaveCount(0);
    const replace = step.getByRole('button', { name: 'Reemplazar lo importado', exact: true });
    await expect(replace).toHaveClass(/danger-button/);
    await replace.click();
    const dialog = page.getByRole('alertdialog', {
      name: 'Reemplazar lo importado del sistema anterior',
    });
    await expect(dialog).toContainText('no se puede deshacer salvo restaurando ese respaldo');
    await expect(dialog.getByRole('button', { name: 'Reemplazar lo importado' })).toHaveClass(
      /danger-solid-button/,
    );
    await dialog.getByRole('button', { name: 'Conservar lo importado', exact: true }).click();
    await expect(dialog).toHaveCount(0);
    expect((await call(driver, 'GET', 'maintenance/legacy')).current_batch.id).toBe(before.id);
    await replace.click();
    await page
      .getByRole('alertdialog', { name: 'Reemplazar lo importado del sistema anterior' })
      .getByRole('button', { name: 'Reemplazar lo importado', exact: true })
      .click();

    // Salir del asistente durante la importación y volver: se ve el avance o, si ya terminó, el lote nuevo.
    await page
      .getByRole('navigation', { name: 'Ruta de navegación' })
      .getByRole('button', { name: 'Mantenimiento', exact: true })
      .click();
    await page
      .getByRole('region', { name: 'Migrar del sistema anterior' })
      .getByRole('button', { name: 'Abrir el asistente de migración' })
      .click();
    await expect
      .poll(async () => (await call(driver, 'GET', 'maintenance/legacy')).current_batch?.id, {
        timeout: 90000,
      })
      .not.toBe(before.id);
    const report = await importReport(page);
    await expect(report.getByText('Todo cuadra.')).toBeVisible();
    const after = await call(driver, 'GET', 'maintenance/legacy');
    const batches = await call(driver, 'GET', `maintenance/legacy/batches/${before.id}`);
    expect(batches.batch.status).toBe('replaced');
    expect(after.current_batch.status).toBe('completed');
  });

  test('«Asignar contraseña» a un usuario importado y su primer inicio de sesión', async ({
    page,
  }) => {
    test.setTimeout(120000);
    const driver = await cleanDriver(page);
    await login(page);
    const users = (await call(driver, 'GET', 'users')) as {
      id: string;
      username: string;
      display_name: string;
      has_password: boolean;
      active: boolean;
    }[];
    const imported = users.find((user) => user.username === 'cajera')!;
    expect(imported.has_password).toBe(false);
    expect(imported.active).toBe(true);
    await openAdministration(page, 'Usuarios');
    const row = page
      .getByRole('row')
      .filter({ has: page.getByRole('cell', { name: 'cajera', exact: true }) });
    await expect(row).toContainText('Sin acceso');
    await expect(
      page.getByRole('row').filter({ has: page.getByRole('cell', { name: 'admin', exact: true }) }),
    ).not.toContainText('Sin acceso');
    await row.getByRole('button', { name: 'Asignar contraseña', exact: true }).click();
    const form = page.getByRole('region', {
      name: `Asignar contraseña a ${imported.display_name}`,
    });
    await form.getByLabel('Contraseña inicial', { exact: true }).fill('Inicial 9H cajera');
    await form.getByLabel('Escríbela otra vez', { exact: true }).fill('Otra 9H cajera');
    await form.getByRole('button', { name: 'Asignar contraseña', exact: true }).click();
    await expect(notice(page, 'Las dos contraseñas no coinciden.')).toBeVisible();
    await form.getByLabel('Escríbela otra vez', { exact: true }).fill('Inicial 9H cajera');
    await form.getByRole('button', { name: 'Asignar contraseña', exact: true }).click();
    await expect(notice(page, /Contraseña asignada/)).toBeVisible();
    await expect(form).toHaveCount(0);
    await expect(row).not.toContainText('Sin acceso');
    await expect(row.getByRole('button', { name: 'Asignar contraseña' })).toHaveCount(0);
    // Ya tiene contraseña: Go no permite asignarle otra.
    const again = await driver.request('POST', `users/${imported.id}/password`, {
      password: 'Tercera 9H cajera',
    });
    expect(again.status).toBe(409);

    // Primer inicio de sesión: entra con la contraseña inicial, se le recomienda cambiarla y la cambia.
    await logout(page);
    await login(page, 'cajera', 'Inicial 9H cajera');
    const warning = page.getByRole('status').filter({
      hasText: 'Recomendamos cambiar tu contraseña inicial.',
    });
    await expect(warning).toBeVisible();
    await warning.getByRole('button', { name: 'Cambiar contraseña', exact: true }).click();
    await page.getByLabel('Contraseña actual', { exact: true }).fill('Inicial 9H cajera');
    await page.getByLabel('Nueva contraseña', { exact: true }).fill('Propia 9H cajera');
    await page.getByLabel('Confirmación', { exact: true }).fill('Propia 9H cajera');
    await page.getByRole('button', { name: 'Cambiar contraseña', exact: true }).last().click();
    await expect(notice(page, 'Contraseña cambiada.')).toBeVisible();
    await expect(warning).toHaveCount(0);
    await logout(page);
  });

  test('reportes con el filtro «Datos» del historial importado', async ({ page }) => {
    test.setTimeout(120000);
    const driver = await cleanDriver(page);
    await login(page);
    const catalog = await call(driver, 'GET', 'reports/catalog');
    const byMonth = (catalog.groups as { reports: { id: string; filters?: { key: string }[] }[] }[])
      .flatMap((group) => group.reports)
      .find((report) => report.id === 'sales.by_month');
    test.skip(
      !byMonth?.filters?.some((filter) => filter.key === 'data_source'),
      'El filtro «Datos» de los reportes llega con la Fase C, parte 2.',
    );
    const batch = (await call(driver, 'GET', 'maintenance/legacy')).current_batch;
    const year = new Date(batch.sales_last_at).getFullYear();
    const expected = await call(
      driver,
      'GET',
      `reports/run/sales.by_month?period=year&year=${year}&data_source=legacy`,
    );
    await openAdministration(page, 'Reportes');
    await page
      .getByRole('tablist', { name: 'Grupos de reportes' })
      .getByRole('tab', { name: 'Ventas', exact: true })
      .click();
    const picker = page.getByRole('combobox', { name: 'Reporte', exact: true });
    if (await picker.isVisible()) {
      await chooseOption(picker, 'Ventas por mes');
    } else {
      await page
        .getByRole('navigation', { name: 'Reportes de Ventas' })
        .getByRole('button', { name: 'Ventas por mes', exact: true })
        .click();
    }
    await page
      .getByRole('group', { name: 'Elegir periodo' })
      .getByRole('button', { name: 'Año', exact: true })
      .click();
    await page.getByLabel('Año', { exact: true }).fill(String(year));
    await page.getByRole('button', { name: 'Aplicar periodo', exact: true }).click();
    const data = page
      .getByRole('group', { name: 'Filtros adicionales' })
      .getByRole('combobox', { name: 'Datos', exact: true });
    await chooseOption(data, 'Solo sistema anterior');
    const table = page.getByRole('region', { name: 'Tabla: Ventas por mes' });
    await expect(table.locator('tfoot')).toContainText(formatMoney(expected.totals.net));
  });
});
