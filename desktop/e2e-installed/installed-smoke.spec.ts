import { spawn } from 'node:child_process';
import { existsSync, mkdirSync, statSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { chromium, expect, test } from '@playwright/test';
import type { Browser, Page } from '@playwright/test';
import { openAdministration, openCash, openSection } from '../e2e/navigation';
import { notice } from '../e2e/notices';
import { configureKitchen } from '../e2e/kitchen-flow';
import {
  checkDineInPayment,
  checkTakeawayDelivery,
  configureReceiptPrinter,
  enableUSDCash,
  sendTakeawayToKitchen,
} from '../e2e/payments-flow';

// Prueba de humo de AD-EL POS INSTALADO en Windows (etapa 9E, §5.2). Recorre la operación de un día con la
// aplicación real: asistente inicial, catálogo, cocina, área y mesas, turno, cobro de mesa (efectivo con
// cambio, dólares, precuenta, descuento y tarjeta), pedido para llevar con pesos y dólares, cancelación con
// motivo, corte parcial, cierre de turno y el reporte de ventas en PDF y Excel con el diálogo nativo.
// Reutiliza los recorridos de e2e/ (solo interfaz) y las cifras las calcula Go. Los pasos van en serie y
// sobre la misma base: si uno falla, se corrige y se vuelve a correr desde ese paso con --grep.
// Etapa 9F (decisión D1): scripts/windows/installed-smoke.ps1 abre la aplicación con una carpeta de datos
// temporal, otro perfil de credenciales y ADEL_TEST_MODE=1, y la prueba exige que el servidor lo declare
// (test_mode en /api/v1/version) antes de escribir nada: nunca corre sobre los datos reales.

const here = path.dirname(fileURLToPath(import.meta.url));
const artifacts = path.resolve(here, '..', '..', '.local', 'installed-artifacts');
const cdp = process.env.ADEL_INSTALLED_CDP ?? 'http://127.0.0.1:9222';

let browser: Browser;
let page: Page;

test.describe.configure({ mode: 'serial' });

test.beforeAll(async () => {
  mkdirSync(artifacts, { recursive: true });
  browser = await chromium.connectOverCDP(cdp);
  const pages = browser.contexts().flatMap((context) => context.pages());
  page = pages.find((candidate) => candidate.url().includes('tauri.localhost')) ?? pages[0]!;
  // La ventana real no tiene viewport emulado: los recorridos de e2e/ comparan contra el tamaño visible.
  const size = await page.evaluate(() => ({
    width: window.innerWidth,
    height: window.innerHeight,
  }));
  page.viewportSize = () => size;
  page.setViewportSize = async () => {};
  // La primera vez el servidor local crea la base: se espera a que responda y se exige el modo de pruebas.
  await expect
    .poll(async () => (await serverSnapshot()).version?.body?.test_mode ?? null, {
      timeout: 180000,
      intervals: [1000],
    })
    .not.toBeNull();
  if ((await serverSnapshot()).version?.body?.test_mode !== true) {
    throw new Error('Las pruebas solo corren contra un servidor en modo de pruebas');
  }
});

// Estado del servidor por el puente real (la misma operación que usa la barra superior).
async function serverSnapshot() {
  return page.evaluate(async () => {
    const internals = (
      window as unknown as {
        __TAURI_INTERNALS__: { invoke: (command: string, args: unknown) => Promise<unknown> };
      }
    ).__TAURI_INTERNALS__;
    return (await internals.invoke('server_snapshot', {})) as {
      version: { status: number; body: { test_mode?: boolean } } | null;
    };
  });
}

async function shot(name: string) {
  await page.screenshot({ path: path.join(artifacts, `${name}.png`) });
}

// Petición por el puente real de Tauri (la misma operación cerrada que usan las pantallas).
async function api(operation: string, body?: unknown) {
  return page.evaluate(
    async ([name, payload]) => {
      const internals = (
        window as unknown as {
          __TAURI_INTERNALS__: {
            invoke: (command: string, args: unknown) => Promise<{ status: number; body: any }>;
          };
        }
      ).__TAURI_INTERNALS__;
      return internals.invoke('api_request', { operation: name, body: payload });
    },
    [operation, body ?? null] as const,
  );
}

async function typeOnKeypad(keypad: ReturnType<Page['getByRole']>, digits: string) {
  for (const digit of digits) {
    await keypad
      .getByRole('button', { name: digit === '.' ? 'Punto decimal' : digit, exact: true })
      .click();
  }
}

test('asistente inicial y acceso con admin', async () => {
  // La primera vez el servidor local crea la base: la pantalla se actualiza sola cada 5 s.
  await expect(page.getByRole('heading', { name: 'Prepara tu establecimiento' })).toBeVisible({
    timeout: 180000,
  });
  await shot('01-asistente');
  await page.getByLabel('Nombre comercial', { exact: true }).fill('Fonda Doña Ñeli');
  await page.getByRole('button', { name: 'Configurar establecimiento', exact: true }).click();
  await expect(page.getByLabel('Usuario', { exact: true })).toBeVisible({ timeout: 60000 });
  await page.getByLabel('Usuario', { exact: true }).fill('admin');
  await page.getByLabel('Contraseña', { exact: true }).fill('admin');
  await page.getByRole('button', { name: 'Entrar', exact: true }).click();
  await expect(page.getByRole('navigation', { name: 'Navegación principal' })).toBeVisible();
  await shot('02-inicio');
});

test('catálogo, cocina, impresora de tickets y pago en dólares', async () => {
  // Catálogo mínimo de la prueba por el puente real: los mismos datos que el recorrido de e2e/.
  // La sesión es la que abrió la interfaz: otro inicio de sesión por el puente la reemplazaría mientras la
  // interfaz consulta con la anterior.
  const category = (
    await api('catalog_create_categories', { name: 'Platillos', active: true, color: '#d64545' })
  ).body;
  expect(category.id).toBeTruthy();
  const steak = await api('catalog_create_products', {
    name: 'Filete de prueba',
    active: true,
    category_id: category.id,
    code: 'FILETE',
    kind: 'product',
    base_price: '150',
  });
  expect(steak.status).toBe(200);
  await page.reload();
  await expect(page.getByRole('navigation', { name: 'Navegación principal' })).toBeVisible();
  await configureKitchen(page);
  await configureReceiptPrinter(page);
  await enableUSDCash(page);
  await shot('03-configuracion');
});

test('área y mesas', async () => {
  await openSection(page, 'Mesas');
  await page
    .getByRole('group', { name: 'Vistas operativas' })
    .getByRole('button', { name: 'Áreas y distribución', exact: true })
    .click();
  await page.getByRole('button', { name: 'Crear área', exact: true }).click();
  await page.getByLabel('Nombre del área', { exact: true }).fill('Salón principal');
  await page.getByRole('button', { name: 'Guardar área', exact: true }).click();
  for (const [name, x] of [
    ['Mesa 1', '40'],
    ['Mesa 2', '320'],
  ] as const) {
    await page.getByRole('button', { name: 'Crear mesa', exact: true }).click();
    await page.getByLabel('Nombre o número de mesa', { exact: true }).fill(name);
    // Cada mesa en su lugar: con la posición predeterminada quedarían encimadas.
    await page.getByLabel('Posición horizontal', { exact: true }).fill(x);
    await page.getByRole('button', { name: 'Guardar mesa', exact: true }).click();
    await expect(page.getByRole('button', { name: new RegExp(`^${name}, Libre`) })).toBeVisible();
  }
  await page
    .getByRole('group', { name: 'Vistas operativas' })
    .getByRole('button', { name: 'Mesas', exact: true })
    .click();
  await shot('04-mesas');
});

async function captureSteakAtTable(table: string) {
  await openSection(page, 'Mesas');
  await page.getByRole('button', { name: new RegExp(`^${table}, Libre`) }).click();
  await page
    .getByRole('complementary', { name: `Panel de ${table}` })
    .getByRole('button', { name: 'Capturar pedidos en esta mesa', exact: true })
    .click();
  await expect(page.getByRole('heading', { name: `${table} · Cuenta nueva` })).toBeVisible();
  const bill = page.getByRole('complementary', { name: 'Cuenta actual' });
  await page.getByRole('button', { name: /^Filete de prueba/ }).click();
  await expect(
    page.getByRole('heading', { name: new RegExp(`^${table} · Cuenta #\\d+`) }),
  ).toBeVisible();
  const send = bill.getByRole('button', { name: 'Enviar a cocina · 1', exact: true });
  await expect(send).toBeEnabled();
  await send.click();
  await expect(bill.getByRole('article', { name: '1 × Filete de prueba' })).toContainText(
    'En cocina',
  );
}

test('mesa: captura, envío a cocina, turno y cobro en efectivo, dólares y tarjeta', async () => {
  await captureSteakAtTable('Mesa 1');
  await shot('05-captura');
  // Abre el turno (fondo $500 y dólar a $18.50), abono en efectivo con cambio, precuenta, descuento del 10 %
  // y tarjeta que liquida (recorrido de e2e/payments-flow.ts).
  await checkDineInPayment(page);
  await shot('06-cobro-mesa');
  await page.getByRole('button', { name: 'Volver a la cuenta', exact: true }).first().click();
});

test('pedido para llevar con pesos y dólares', async () => {
  await openSection(page, 'Para llevar');
  await page.getByRole('button', { name: 'Nuevo pedido', exact: true }).click();
  await sendTakeawayToKitchen(page);
  await checkTakeawayDelivery(page);
  await shot('07-para-llevar');
});

test('cancelar una cuenta con motivo', async () => {
  await captureSteakAtTable('Mesa 2');
  await openSection(page, 'Mesas');
  await page.getByRole('button', { name: /^Mesa 2,/ }).click();
  const panel = page.getByRole('complementary', { name: 'Panel de Mesa 2' });
  await panel.getByRole('button', { name: 'Más…', exact: true }).click();
  await panel
    .getByRole('group', { name: 'Más acciones de la cuenta' })
    .getByRole('button', { name: 'Cancelar cuenta', exact: true })
    .click();
  const region = page.getByRole('region', { name: 'Cancelar cuenta' });
  await region
    .getByRole('group', { name: 'Motivo de la cancelación' })
    .getByRole('button', { name: /^Cliente se retiró sin pagar/ })
    .click();
  await region.getByRole('button', { name: 'Cancelar cuenta', exact: true }).click();
  const dialog = page.getByRole('alertdialog');
  await expect(dialog).toContainText('Motivo: Cliente se retiró sin pagar');
  await dialog.getByRole('button', { name: 'Cancelar cuenta', exact: true }).click();
  await expect(notice(page, /Cuenta #\d+ cancelada\./)).toBeVisible();
  await expect(page.getByRole('button', { name: /^Mesa 2, Libre/ })).toBeVisible();
  await shot('08-cancelada');
});

test('corte parcial y cierre de turno', async () => {
  await openCash(page);
  const actions = page.getByRole('group', { name: 'Acciones del turno' });
  await actions.getByRole('button', { name: 'Corte parcial', exact: true }).click();
  const cut = page.getByRole('region', { name: 'Corte parcial', exact: true });
  await cut.getByRole('button', { name: 'Generar corte parcial', exact: true }).click();
  await expect(notice(page, 'Corte parcial #1 generado.')).toBeVisible();
  await actions.getByRole('button', { name: 'Arqueo y cierre', exact: true }).click();
  await page
    .getByRole('region', { name: 'Arqueo de cierre', exact: true })
    .getByRole('button', { name: 'Iniciar arqueo de cierre', exact: true })
    .click();
  await page
    .getByRole('alertdialog')
    .getByRole('button', { name: 'Iniciar arqueo y pausar la caja', exact: true })
    .click();
  const closing = page.getByRole('region', { name: 'Arqueo de cierre en curso', exact: true });
  await expect(closing).toBeVisible();
  // Conteo ciego: 1 × $500 y 1 × $200 en pesos.
  const keypad = closing.getByRole('group', { name: 'Teclado numérico del arqueo' });
  await closing.getByLabel('Billetes de $500', { exact: true }).click();
  await typeOnKeypad(keypad, '1');
  await closing.getByLabel('Billetes de $200', { exact: true }).click();
  await typeOnKeypad(keypad, '1');
  await expect(closing.getByLabel('Total declarado', { exact: true })).toContainText('$700.00');
  await expect(closing.getByText('Borrador guardado.', { exact: true })).toBeVisible();
  await closing.getByRole('button', { name: 'Confirmar cierre del turno', exact: true }).click();
  await page
    .getByRole('alertdialog')
    .getByRole('button', { name: 'Cerrar el turno', exact: true })
    .click();
  const result = page.getByRole('region', { name: 'Resultado del cierre', exact: true });
  await expect(result).toContainText('quedó cerrado');
  await shot('09-cierre');
  await page.getByRole('button', { name: 'Volver a la caja', exact: true }).click();
});

// Exporta el reporte abierto con el diálogo nativo «Guardar reporte», que contesta save-dialog.ps1.
async function exportReport(kind: 'PDF' | 'Excel', file: string) {
  const answer = spawn(
    'powershell.exe',
    [
      '-NoProfile',
      '-ExecutionPolicy',
      'Bypass',
      '-File',
      path.join(here, 'save-dialog.ps1'),
      '-Path',
      file,
    ],
    { stdio: 'inherit', windowsHide: true },
  );
  const done = new Promise<number | null>((resolve) => answer.on('exit', resolve));
  await page.getByRole('button', { name: `Exportar ${kind}`, exact: true }).click();
  expect(await done).toBe(0);
  await expect
    .poll(() => existsSync(file) && statSync(file).size > 0, { timeout: 60000 })
    .toBe(true);
}

test('reporte de ventas en PDF y Excel', async () => {
  await openAdministration(page, 'Reportes');
  await page
    .getByRole('tablist', { name: 'Grupos de reportes' })
    .getByRole('tab', { name: 'Ventas', exact: true })
    .click();
  const picker = page.getByRole('combobox', { name: 'Reporte', exact: true });
  if (await picker.isVisible()) {
    await picker.click();
    await page.getByRole('option', { name: /^Ventas por día/ }).click();
  } else {
    await page
      .getByRole('navigation', { name: 'Reportes de Ventas' })
      .getByRole('button', { name: /^Ventas por día/ })
      .click();
  }
  await expect(page.getByRole('heading', { name: /^Ventas por día/ })).toBeVisible();
  await shot('10-reporte');
  const folder = path.join(artifacts, 'exportaciones');
  mkdirSync(folder, { recursive: true });
  // Nombres únicos: un archivo existente haría que Windows preguntara si se reemplaza.
  const stamp = new Date().toISOString().replace(/[-:.]/g, '').slice(0, 15);
  await exportReport('PDF', path.join(folder, `ventas-por-dia-${stamp}.pdf`));
  await exportReport('Excel', path.join(folder, `ventas-por-dia-${stamp}.xlsx`));
  await openAdministration(page, 'Configuración');
});
