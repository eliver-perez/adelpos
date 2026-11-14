import { randomUUID } from 'node:crypto';
import { expect, test } from '@playwright/test';
import { installDriver } from './native-driver';
import { openAdministration, openSection } from './navigation';
import { notice } from './notices';
import { receiptPrinterName, receiptPrinterQueue } from './payments-flow';
import { chooseOption } from './select';

// Etapa 9F, fase B (§4.1–§4.5 y §8) contra Go y PostgreSQL reales. Sin impresora de tickets, la
// precuenta avisa en el momento con «Configurar impresora» (que lleva a Cobros › Políticas de cobro) y
// «Reintentar»; el nombre visible elegido en Administración › Documentos («Ticket de venta», revisión 9F
// M3) cambia botones, avisos, el menú nativo y el encabezado impreso; una impresión que no salió se explica y se reintenta con un toque, sin motivo;
// reimprimir pide motivo; la «Cola de impresión» tiene altura limitada y su detalle aparece a la vista; y
// el corte de papel se elige por impresora y viaja en la prueba de ancho. La impresora de tickets es la
// de la aceptación (payments-flow.ts): una cola del equipo que no existe, de modo que nada sale en papel.
//
// El nombre «workflow-stage9f-printing» la ordena después de las especificaciones anteriores y antes de
// «workflow-zz-test-data», que quita los datos de prueba al final.

type Driver = Awaited<ReturnType<typeof installDriver>>;

interface Job {
  id: string;
  state: string;
  attempts: number;
  revision: number;
  ticket_kind?: string;
  printer_snapshot: { cut_mode?: string; cut_feed_lines?: number; cut?: boolean };
}

async function call(driver: Driver, method: string, route: string, body?: unknown) {
  const response = await driver.request(method, route, body);
  expect(response.status, `${method} ${route}: ${JSON.stringify(response.body)}`).toBe(200);
  return response.body;
}

async function job(driver: Driver, id: string): Promise<Job> {
  return (await call(driver, 'GET', `kitchen/jobs/${id}`)).job as Job;
}

// Espera a que el trabajo termine sus intentos automáticos en error (la cola no existe): a partir de
// ahí ya no cambia solo y reintentarlo o resolverlo no compite con el trabajador.
async function finalError(driver: Driver, id: string, attempts: number) {
  await expect
    .poll(
      async () => {
        const current = await job(driver, id);
        return current.state === 'error' && current.attempts >= attempts;
      },
      { timeout: 40000 },
    )
    .toBe(true);
  return job(driver, id);
}

async function setReceipt(driver: Driver, printerID: string | null, precheckName: string) {
  const settings = await call(driver, 'GET', 'payments/settings');
  await call(driver, 'PUT', 'payments/settings', {
    operation_id: randomUUID(),
    revision: settings.revision,
    shared_shifts: settings.shared_shifts,
    print_receipt_default: settings.print_receipt_default,
    receipt_printer_id: printerID,
    refund_authorization_threshold: settings.refund_authorization_threshold,
  });
  // Revisión 9F (M3): el nombre de la precuenta se guarda con la identidad del establecimiento.
  const identity = await call(driver, 'GET', 'documents/identity');
  await call(driver, 'PUT', 'documents/identity', {
    operation_id: randomUUID(),
    revision: identity.revision,
    additional_info: identity.additional_info,
    header_text: identity.header_text,
    footer_text: identity.footer_text,
    presentation: identity.presentation,
    precheck_name: precheckName,
  });
}

// La impresora de tickets de la aceptación: la de payments-flow.ts si ya existe (corrida completa) o
// una igual (misma firma de datos de prueba) si la especificación corre sola.
async function receiptPrinter(driver: Driver) {
  const printers = (await call(driver, 'GET', 'kitchen/printers')) as {
    id: string;
    name: string;
  }[];
  const found = printers.find((printer) => printer.name === receiptPrinterName);
  if (found) {
    return found.id;
  }
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
  return created.id as string;
}

// Mesa con una cuenta y un producto confirmado, y un turno abierto para poder abrir el cobro.
async function prepareAccount(driver: Driver) {
  const category = await call(driver, 'POST', 'catalog/categories', {
    name: 'Bebidas 9F',
    active: true,
    color: '#1f9a78',
  });
  const product = await call(driver, 'POST', 'catalog/products', {
    name: 'Agua de jamaica 9F',
    active: true,
    category_id: category.id,
    code: 'JAMAICA-9F',
    kind: 'product',
    base_price: '30',
  });
  const area = await call(driver, 'POST', 'restaurant/areas', {
    operation_id: randomUUID(),
    name: 'Terraza 9F',
    active: true,
  });
  const table = await call(driver, 'POST', 'restaurant/tables', {
    operation_id: randomUUID(),
    area_id: area.id,
    name: 'Mesa 9F',
    shape: 'rectangle',
    x: 40,
    y: 60,
    width: 110,
    height: 100,
    seats: 4,
    active: true,
  });
  const opened = await call(driver, 'POST', 'restaurant/accounts', {
    operation_id: randomUUID(),
    mode: 'dine_in',
    table_id: table.id,
  });
  const accountID = opened.account.id as string;
  const route = `restaurant/accounts/${accountID}`;
  const drafted = await call(driver, 'PUT', `${route}/draft`, {
    operation_id: randomUUID(),
    revision: opened.account.revision,
    items: [{ id: randomUUID(), product_id: product.id, variant_id: null, quantity: 1 }],
  });
  const preview = await call(driver, 'GET', `${route}/preview`);
  await call(driver, 'POST', `${route}/confirm`, {
    operation_id: randomUUID(),
    revision: drafted.account.revision,
    expected_fingerprint: preview.fingerprint,
  });
  const current = await call(driver, 'GET', 'payments/shifts/current');
  if (!current.shift) {
    await call(driver, 'POST', 'payments/shifts', {
      operation_id: randomUUID(),
      opening_float: '0',
    });
  }
  return accountID;
}

test('impresión 9F: aviso sin impresora, nombre de la precuenta, reintento, motivo, cola y corte', async ({
  page,
}) => {
  test.setTimeout(360000);
  const errors: string[] = [];
  page.on('pageerror', (error) => errors.push(error.message));
  const driver = await installDriver(page);
  await call(driver, 'POST', 'auth/login', { username: 'admin', password: 'admin' });
  await call(driver, 'PUT', 'terminal/preferences', { keyboard_mode: 'disabled' });
  const printerID = await receiptPrinter(driver);
  const accountID = await prepareAccount(driver);
  // Sin impresora de tickets y con el nombre de siempre.
  await setReceipt(driver, null, 'precheck');
  await driver.request('POST', 'auth/logout');

  await page.setViewportSize({ width: 1280, height: 720 });
  await page.goto('/');
  await page.getByLabel('Usuario', { exact: true }).fill('admin');
  await page.getByLabel('Contraseña', { exact: true }).fill('admin');
  await page.getByRole('button', { name: 'Entrar', exact: true }).click();

  const openTable = async () => {
    await openSection(page, 'Mesas');
    await page
      .getByRole('group', { name: 'Áreas del restaurante' })
      .getByRole('button', { name: 'Terraza 9F', exact: true })
      .click();
    await page
      .locator('.table-canvas[aria-label="Mapa de mesas"]')
      .getByRole('button', { name: /^Mesa 9F,/ })
      .click();
    return page.getByRole('complementary', { name: 'Panel de Mesa 9F' });
  };

  // 1. §4.1: sin impresora, el aviso llega en el momento con «Configurar impresora» y «Reintentar».
  let panel = await openTable();
  await panel.getByRole('button', { name: 'Precuenta', exact: true }).click();
  const precheck = page.getByRole('region', { name: 'Precuenta' });
  await precheck.getByRole('button', { name: 'Imprimir precuenta', exact: true }).click();
  // Solo el aviso vigente: el que se retira conserva su texto mientras dura su animación de salida.
  const missing = page
    .locator('[data-sonner-toast]:not([data-removed="true"]) .app-toast')
    .filter({ hasText: 'No hay impresora configurada para precuentas.' });
  await expect(missing).toBeVisible();
  await expect(missing).not.toContainText('Cola de impresión');
  await expect(missing.getByRole('button', { name: 'Reintentar', exact: true })).toBeVisible();
  const jobsBefore = ((await call(driver, 'GET', 'kitchen/jobs')) as Job[]).length;
  await missing.getByRole('button', { name: 'Reintentar', exact: true }).click();
  // El reintento vuelve a fallar: un aviso nuevo con las mismas acciones.
  await expect(missing).toHaveCount(1);
  await expect(missing).toBeVisible();
  // Ningún trabajo fallido se encoló.
  expect(((await call(driver, 'GET', 'kitchen/jobs')) as Job[]).length).toBe(jobsBefore);
  await missing.getByRole('button', { name: 'Configurar impresora', exact: true }).click();
  await expect(page.getByRole('navigation', { name: 'Ruta de navegación' })).toHaveText(
    'Administración›Cobros',
  );
  const printerField = page.getByRole('combobox', { name: /Impresora de tickets/ });
  await expect(printerField).toBeInViewport();
  await chooseOption(printerField, receiptPrinterName);
  await page.getByRole('button', { name: 'Guardar políticas', exact: true }).click();
  await expect(notice(page, 'Políticas de cobro guardadas.')).toBeVisible();
  // §4.5: el nombre visible, de la lista cerrada, en Administración › Documentos (revisión 9F, M3).
  await expect(page.getByText(/Se elige en Administración › Documentos/)).toBeVisible();
  await openAdministration(page, 'Documentos');
  await page.getByRole('tab', { name: 'Identidad del establecimiento', exact: true }).click();
  await chooseOption(
    page.getByRole('combobox', { name: 'Nombre de la precuenta' }),
    'Ticket de venta',
  );
  await page.getByRole('button', { name: 'Guardar cambios', exact: true }).click();
  await expect(notice(page, 'Identidad del establecimiento guardada.')).toBeVisible();

  // 2. §4.5: botones, menú nativo, panel y encabezado impreso con «Ticket de venta». Mesas conserva
  // la precuenta que se dejó abierta: al volver ya se llama «Ticket de venta» (llega con la sesión).
  await openSection(page, 'Mesas');
  const reopened = page.getByRole('region', { name: 'Ticket de venta' });
  await expect(reopened).toBeVisible({ timeout: 15000 });
  await reopened.getByRole('button', { name: 'Volver', exact: true }).click();
  panel = page.getByRole('complementary', { name: 'Panel de Mesa 9F' });
  const button = panel.getByRole('button', { name: 'Ticket de venta', exact: true });
  await expect(button).toBeVisible();
  await expect.poll(() => driver.menuPrecheck()).toBe('sales_ticket');
  await button.click();
  const ticket = page.getByRole('region', { name: 'Ticket de venta' });
  await expect(ticket.getByLabel('Vista previa del ticket de venta')).toContainText(
    'TICKET DE VENTA',
  );
  await expect(ticket.getByLabel('Vista previa del ticket de venta')).not.toContainText(
    'PRECUENTA',
  );
  await expect(ticket.getByLabel('Vista previa del ticket de venta')).toContainText('CFDI');
  await ticket.getByRole('button', { name: 'Imprimir ticket de venta', exact: true }).click();
  await expect(notice(page, /Ticket de venta enviado a la cola de impresión/)).toBeVisible();
  // El puente de la prueba comparte la sesión que abrió la pantalla.
  const finance = await call(driver, 'GET', `payments/accounts/${accountID}`);
  const document = (
    finance.documents as { ticket_id: string; kind: string; job_id: string }[]
  ).find((entry) => entry.kind === 'precheck')!;
  expect(document).toBeTruthy();
  await finalError(driver, document.job_id, 3);
  await ticket.getByRole('button', { name: 'Volver', exact: true }).click();

  // 3. §4.2: la impresión que no salió se explica sin términos técnicos y se reintenta sin motivo. La cola
  // de la aceptación no existe en el equipo: desde la revisión 9F (B4) se explica como impresora no
  // encontrada (no como «no responde»), que se resuelve configurándola.
  await panel.getByRole('button', { name: /^Cobrar/ }).click();
  const documents = page.getByRole('region', { name: 'Documentos de la cuenta' });
  await expect(documents).toContainText('No se encontró la impresora de caja en el equipo.');
  await expect(documents).not.toContainText('no responde');
  await expect(documents).not.toContainText('Cola de impresión');
  await documents.getByRole('button', { name: 'Reintentar', exact: true }).click();
  await expect(notice(page, /Reintento enviado a la impresora/)).toBeVisible();
  const retried = await finalError(driver, document.job_id, 4);
  // El reintento usó el mismo trabajo; se cancela administrativamente para reimprimirlo después.
  await call(driver, 'POST', `kitchen/jobs/${retried.id}/resolve`, {
    operation_id: randomUUID(),
    revision: retried.revision,
    decision: 'cancel',
    reason: 'Impresora de la aceptación sin hardware',
    printer_id: null,
  });

  // 4. Reimprimir lo que ya salió (o se canceló) pide motivo.
  await expect(documents.getByText('Cancelado')).toBeVisible({ timeout: 15000 });
  await documents.getByRole('button', { name: 'Reimprimir ticket de venta', exact: true }).click();
  const reason = documents.getByRole('form', { name: 'Reimpresión con motivo' });
  const reprint = reason.getByRole('button', { name: 'Reimprimir', exact: true });
  await expect(reprint).toBeDisabled();
  await reason.getByLabel('Motivo de la reimpresión').fill('El cliente pidió otra copia');
  await reprint.click();
  await expect(notice(page, /Copia solicitada/)).toBeVisible();

  // 5. §4.3: la cola con muchos trabajos tiene altura limitada y el detalle aparece a la vista.
  for (let index = 0; index < 12; index += 1) {
    await call(driver, 'POST', 'kitchen/test-print', {
      operation_id: randomUUID(),
      printer_id: printerID,
    });
  }
  await openSection(page, 'Cocina');
  await page.getByRole('button', { name: 'Cola de impresión', exact: true }).click();
  const list = page.getByRole('list', { name: 'Trabajos de impresión' });
  await expect(list.getByRole('listitem')).not.toHaveCount(0);
  await expect
    .poll(() => list.evaluate((element) => element.scrollHeight > element.clientHeight))
    .toBe(true);
  await expect(list).not.toContainText(/submitted|intervention|ADEL_E2E/);
  const last = list.getByRole('listitem').last();
  await last.scrollIntoViewIfNeeded();
  await last.getByRole('button', { name: 'Consultar trabajo e intentos', exact: true }).click();
  const detail = page.getByRole('region', { name: 'Detalle del trabajo de impresión' });
  await expect(detail).toBeInViewport();
  await expect(detail.getByRole('button', { name: 'Información para soporte' })).toBeVisible();
  await detail.getByRole('button', { name: 'Cerrar revisión', exact: true }).click();

  // 6. §4.4: corte por impresora (tipo y renglones de avance) y la prueba de ancho también corta.
  await page.getByRole('button', { name: 'Impresoras', exact: true }).click();
  await page.locator('.kitchen-list-button').filter({ hasText: receiptPrinterName }).click();
  await page.getByRole('button', { name: /^Corte de papel y respaldo/ }).click();
  await expect(page.getByLabel('Cortar el papel al terminar')).toBeChecked();
  await expect(page.getByLabel('El modelo y su controlador admiten corte')).toHaveCount(0);
  await page.getByRole('button', { name: 'Parcial', exact: true }).click();
  await chooseOption(
    page.getByRole('combobox', { name: 'Renglones de avance antes del corte' }),
    '6',
  );
  await page.getByRole('button', { name: 'Guardar impresora', exact: true }).click();
  await expect(notice(page, 'Impresora guardada.')).toBeVisible();
  const saved = (
    (await call(driver, 'GET', 'kitchen/printers')) as {
      id: string;
      cut: boolean;
      cut_mode: string;
      cut_feed_lines: number;
    }[]
  ).find((printer) => printer.id === printerID)!;
  expect(saved).toMatchObject({ cut: true, cut_mode: 'partial', cut_feed_lines: 6 });
  await page.getByRole('button', { name: /^Imprimir prueba de ancho/ }).click();
  await expect(notice(page, /Prueba de ancho de \d+ columnas enviada a la cola/)).toBeVisible();
  const jobs = (await call(driver, 'GET', 'kitchen/jobs')) as Job[];
  const width = jobs.find((entry) => entry.ticket_kind === 'test')!;
  expect((await job(driver, width.id)).printer_snapshot).toMatchObject({
    cut: true,
    cut_mode: 'partial',
    cut_feed_lines: 6,
  });

  // Se deja el nombre de siempre para las especificaciones siguientes.
  await setReceipt(driver, printerID, 'precheck');
  expect(errors).toEqual([]);
});
