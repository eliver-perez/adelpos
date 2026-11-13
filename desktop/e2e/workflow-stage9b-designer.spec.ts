import { randomUUID } from 'node:crypto';
import { expect, test } from '@playwright/test';
import type { Page, TestInfo } from '@playwright/test';
import { installDriver } from './native-driver';
import { openAdministration, openSection } from './navigation';
import { notice } from './notices';

// El nombre «workflow-stage9b-…» la ordena después de las especificaciones existentes: todas comparten
// la base de cada motor y varias esperan folios y configuraciones de una instalación recién creada.
//
// Etapa 9B, fase C: editor visual de documentos contra Go y PostgreSQL reales. Antes de editar se
// cobra un ticket (por la API) que queda en la cola de la impresora de tickets de la aceptación (desde
// la etapa 9F, una cola del equipo que no existe: el destino «agent» ya no sirve para tickets y las
// muestras siguen yendo a él). Después, en
// la interfaz: abrir Documentos, duplicar el ticket predeterminado, agregar un texto fijo, cambiar
// su alineación y tamaño, ver la vista previa a 58 y 80 mm, guardar la versión 2, activarla,
// imprimir una muestra en el destino controlado (nunca CUPS ni TCP reales) y ver su estado,
// consultar el historial y restaurar la predeterminada. El ticket cobrado antes de editar se
// reimprime idéntico y «Imprimir con la plantilla vigente» es una acción distinta en los documentos
// de la cuenta. Sin desbordamiento horizontal a 1280 × 720 ni a 1920 × 1080 en ambos temas.
// ADEL_DESIGNER_SHOTS permite guardar las capturas del editor fuera de los artefactos.

type Driver = Awaited<ReturnType<typeof installDriver>>;

const fixedText = 'GRACIAS 9B';
const templateName = 'Ticket de la casa 9B';
const printerName = 'Destino controlado 9B';
// Etapa 9F (decisión D4): impresora de tickets válida que nunca imprime (cola inexistente); su firma está
// en backend/internal/maintenance/signatures.go.
const receiptPrinterName = 'Tickets controlados 9B';
const customer = 'Cliente diseñador 9B';

async function call(driver: Driver, method: string, route: string, body?: unknown) {
  const response = await driver.request(method, route, body);
  expect(response.status, `${method} ${route}: ${JSON.stringify(response.body)}`).toBe(200);
  return response.body;
}

async function setTheme(page: Page, theme: string) {
  await page.evaluate((value) => {
    document.documentElement.dataset.theme = value;
  }, theme);
  await expect(page.locator('html')).toHaveAttribute('data-theme', theme);
  await page.waitForTimeout(150);
}

// Sin desbordamiento horizontal en los dos tamaños y los dos temas; guarda la captura de cada uno.
async function checkLayout(page: Page, testInfo: TestInfo, name: string) {
  // Los avisos ya comprobados se cierran para que no tapen el editor en las capturas.
  const close = page.locator('[data-sonner-toaster]').getByRole('button', { name: 'Cerrar aviso' });
  // Un aviso de éxito se cierra solo a los 4 s y Sonner reacomoda la pila: si desaparece durante el
  // clic, el clic se descarta y se vuelve a contar (con un tope para que el bucle siempre termine).
  for (let attempt = 0; attempt < 20 && (await close.count()) > 0; attempt++) {
    await close
      .first()
      .click({ timeout: 2000 })
      .catch(() => undefined);
    await page.waitForTimeout(250);
  }
  await expect(close).toHaveCount(0, { timeout: 6000 });
  for (const size of [
    { width: 1280, height: 720 },
    { width: 1920, height: 1080 },
  ]) {
    await page.setViewportSize(size);
    for (const theme of ['light', 'dark']) {
      await setTheme(page, theme);
      await expect(page.locator('.paper-strip').first()).toBeVisible();
      const overflow = await page.evaluate(
        () => document.documentElement.scrollWidth - window.innerWidth,
      );
      expect(overflow, `${name} se desborda a ${size.width} × ${size.height} (${theme})`).toBe(0);
      const file = `9b-editor-${name}-${theme}-${size.width}x${size.height}.png`;
      const directory = process.env.ADEL_DESIGNER_SHOTS;
      // La sesión vuelve a aplicar el tema guardado al refrescarse: si cambió durante la captura,
      // se repite con el tema pedido.
      for (let attempt = 0; attempt < 3; attempt++) {
        await setTheme(page, theme);
        await page.screenshot({
          path: directory ? `${directory}/${file}` : testInfo.outputPath(file),
          fullPage: true,
        });
        const current = await page.evaluate(() => document.documentElement.dataset.theme);
        if (current === theme) {
          break;
        }
      }
    }
  }
  await page.setViewportSize({ width: 1280, height: 720 });
  await setTheme(page, 'light');
}

// Espera a que un trabajo de la cola llegue a un estado que ya no cambia solo: el destino «agent» queda
// en intervención y la cola inexistente, en error tras los tres intentos automáticos (unos 6 s), de modo
// que resolverlo después no compite con el trabajador.
async function settledJob(driver: Driver, jobID: string) {
  let job: { id: string; state: string; revision: number; attempts: number } | null = null;
  await expect
    .poll(
      async () => {
        job = (await call(driver, 'GET', `kitchen/jobs/${jobID}`)).job;
        if (job!.state === 'error' && job!.attempts < 3) {
          return 'reintentándose';
        }
        return job!.state;
      },
      { timeout: 30000 },
    )
    .not.toMatch(/^(pending|processing|reintentándose)$/);
  return job!;
}

// Precondición por la API: el destino controlado de las muestras, la impresora de tickets de la
// aceptación, turno abierto y un pedido para llevar con un abono en efectivo, cuyo ticket queda en la
// cola antes de editar.
async function paidTicket(driver: Driver) {
  await call(driver, 'POST', 'kitchen/printers', {
    operation_id: randomUUID(),
    name: printerName,
    transport: 'agent',
    destination: 'Agente de aceptación no disponible',
    port: 0,
    paper_width: 80,
    copies: 1,
    supports_cut: false,
    cut: false,
    cutter_option: '',
    active: true,
    fallback_id: null,
  });
  const tickets = await call(driver, 'POST', 'kitchen/printers', {
    operation_id: randomUUID(),
    name: receiptPrinterName,
    transport: 'cups',
    destination: 'ADEL_E2E_SIN_IMPRESORA',
    port: 9100,
    paper_width: 80,
    copies: 1,
    supports_cut: false,
    cut: false,
    cutter_option: '',
    active: true,
    fallback_id: null,
  });
  const settings = await call(driver, 'GET', 'payments/settings');
  await call(driver, 'PUT', 'payments/settings', {
    operation_id: randomUUID(),
    revision: settings.revision,
    shared_shifts: settings.shared_shifts,
    print_receipt_default: true,
    receipt_printer_id: tickets.id,
    refund_authorization_threshold: settings.refund_authorization_threshold,
  });
  const current = await call(driver, 'GET', 'payments/shifts/current');
  if (!current.shift) {
    await call(driver, 'POST', 'payments/shifts', {
      operation_id: randomUUID(),
      opening_float: '500',
    });
  }
  const category = await call(driver, 'POST', 'catalog/categories', {
    name: 'Bebidas 9B',
    active: true,
    color: '#1f9a78',
  });
  const product = await call(driver, 'POST', 'catalog/products', {
    name: 'Limonada 9B',
    active: true,
    category_id: category.id,
    code: 'LIMONADA-9B',
    kind: 'product',
    base_price: '60',
  });
  const opened = await call(driver, 'POST', 'restaurant/accounts', {
    operation_id: randomUUID(),
    mode: 'takeaway',
    name: customer,
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
  const methods = (await call(driver, 'GET', 'payments/methods')) as {
    id: string;
    code: string | null;
  }[];
  const cash = methods.find((method) => method.code === 'cash');
  const finance = await call(driver, 'GET', `payments/accounts/${accountID}`);
  await call(driver, 'POST', `payments/accounts/${accountID}/payments`, {
    operation_id: randomUUID(),
    mode: 'partial',
    amount: '20',
    expected_balance: finance.balance,
    method_id: cash!.id,
    received: '20',
    print: true,
  });
  const after = await call(driver, 'GET', `payments/accounts/${accountID}`);
  const receipt = (after.documents as { ticket_id: string; kind: string; job_id: string }[]).find(
    (document) => document.kind === 'receipt',
  );
  expect(receipt, JSON.stringify(after.documents)).toBeTruthy();
  // La impresora de tickets de la aceptación no tiene hardware: el trabajo termina en error y se
  // cancela administrativamente para poder reimprimirlo (como en la cola real).
  const job = await settledJob(driver, receipt!.job_id);
  if (job.state !== 'cancelled' && job.state !== 'submitted') {
    await call(driver, 'POST', `kitchen/jobs/${job.id}/resolve`, {
      operation_id: randomUUID(),
      revision: job.revision,
      decision: 'cancel',
      reason: 'Destino de aceptación sin hardware físico',
      printer_id: null,
    });
  }
  const original = await call(driver, 'GET', `kitchen/tickets/${receipt!.ticket_id}`);
  return { accountID, ticketID: receipt!.ticket_id, original: original.preview as string };
}

test('diseñador 9B: duplicar, editar, vista previa 58/80, guardar, activar, muestra, historial y reimpresión exacta', async ({
  page,
}, testInfo) => {
  test.setTimeout(300000);
  const errors: string[] = [];
  page.on('pageerror', (error) => errors.push(error.message));
  const driver = await installDriver(page);
  await driver.request('POST', 'auth/login', { username: 'admin', password: 'admin' });
  const keyboard = await driver.request('PUT', 'terminal/preferences', {
    keyboard_mode: 'disabled',
  });
  expect(keyboard.status, JSON.stringify(keyboard.body)).toBeLessThan(300);
  const sale = await paidTicket(driver);
  expect(sale.original).not.toContain(fixedText);
  await driver.request('POST', 'auth/logout');

  await page.setViewportSize({ width: 1280, height: 720 });
  await page.goto('/');
  await page.getByLabel('Usuario', { exact: true }).fill('admin');
  await page.getByLabel('Contraseña', { exact: true }).fill('admin');
  await page.getByRole('button', { name: 'Entrar', exact: true }).click();
  await openAdministration(page, 'Documentos');
  await expect(page.getByRole('heading', { name: 'Documentos', exact: true })).toBeVisible();
  const kinds = page.getByRole('navigation', { name: 'Tipos de documento' });
  for (const group of ['Venta', 'Cocina', 'Caja']) {
    await expect(kinds.getByRole('heading', { name: group, exact: true })).toBeVisible();
  }
  const receipts = page.getByRole('region', { name: 'Plantillas de Ticket de venta' });
  await expect(receipts.getByText('Predeterminada · solo lectura')).toBeVisible();

  // 1. Duplicar la predeterminada (solo lectura) y abrir la copia en el editor.
  await receipts.getByRole('button', { name: 'Duplicar Predeterminada', exact: true }).click();
  const nameDialog = page.getByRole('dialog', { name: 'Duplicar plantilla' });
  await nameDialog.getByRole('textbox').fill(templateName);
  await nameDialog.getByRole('button', { name: 'Duplicar', exact: true }).click();
  await expect(notice(page, `Plantilla «${templateName}» creada.`)).toBeVisible();
  const editor = page.getByRole('region', { name: `Editor de ${templateName}` });
  await expect(editor).toBeVisible();
  const paper = editor.locator('.paper-strip');
  await expect(paper).toBeVisible();
  await expect(editor.getByText('Vista previa con datos de muestra')).toBeVisible();
  await expect(editor.getByText('Sin cambios pendientes')).toBeVisible();

  // 2. Agregar un texto fijo después del nombre del establecimiento y configurarlo.
  const structure = editor.getByRole('list', { name: 'Estructura de la plantilla' });
  await structure.getByRole('button', { name: /^1\. Nombre del establecimiento/ }).click();
  await editor
    .getByRole('region', { name: 'Elementos disponibles' })
    .getByRole('button', { name: 'Agregar Texto fijo', exact: true })
    .click();
  await expect(structure.getByRole('button', { name: /^2\. Texto fijo/ })).toHaveAttribute(
    'aria-pressed',
    'true',
  );
  const properties = editor.getByRole('region', { name: 'Propiedades del bloque' });
  await properties.getByRole('textbox', { name: 'Texto', exact: true }).fill(fixedText);
  await properties
    .getByRole('group', { name: 'Alineación', exact: true })
    .getByRole('button', { name: 'Centro', exact: true })
    .click();
  await properties
    .getByRole('group', { name: 'Tamaño', exact: true })
    .getByRole('button', { name: 'Doble ancho', exact: true })
    .click();
  await expect(editor.getByText('Cambios sin guardar')).toBeVisible();
  // La vista previa muestra la línea de Go: doble ancho, centrada en la mitad de columnas.
  const line = paper
    .locator('.paper-line[data-size="double_width"]')
    .filter({ hasText: fixedText });
  await expect(line).toBeVisible({ timeout: 10000 });
  await expect(paper).toHaveAttribute('data-paper', '80');
  await expect(editor.getByText('42 columnas')).toBeVisible();
  const text80 = (await line.textContent()) ?? '';
  expect(text80.trim()).toBe(fixedText);

  // 3. Vista previa a 58 mm: la misma línea centrada en 15 caracteres de doble ancho (21 en 80 mm),
  // con el relleno que calcula Go.
  await editor
    .getByRole('group', { name: 'Ancho del papel', exact: true })
    .getByRole('button', { name: '58 mm', exact: true })
    .click();
  await expect(paper).toHaveAttribute('data-paper', '58');
  await expect(editor.getByText('30 columnas')).toBeVisible();
  const text58 = (await line.textContent()) ?? '';
  expect(text58.trim()).toBe(fixedText);
  const leading = (text: string) => text.length - text.trimStart().length;
  expect(leading(text80)).toBeGreaterThan(leading(text58));
  const box58 = await paper.boundingBox();
  await editor
    .getByRole('group', { name: 'Ancho del papel', exact: true })
    .getByRole('button', { name: '80 mm', exact: true })
    .click();
  await expect(paper).toHaveAttribute('data-paper', '80');
  // Tocar la línea en la vista previa elige su bloque.
  await structure.getByRole('button', { name: /^1\. Nombre del establecimiento/ }).click();
  await paper.getByRole('button', { name: `Elegir Texto fijo · «${fixedText}»` }).click();
  await expect(structure.getByRole('button', { name: /^2\. Texto fijo/ })).toHaveAttribute(
    'aria-pressed',
    'true',
  );
  expect(box58).not.toBeNull();
  await checkLayout(page, testInfo, 'ticket');

  // 4. Guardar: versión 2.
  await editor.getByRole('button', { name: 'Guardar versión', exact: true }).click();
  const saveDialog = page.getByRole('dialog', { name: 'Guardar la versión 2' });
  await saveDialog
    .getByRole('textbox', { name: 'Nota de la versión (opcional)' })
    .fill('Agradecimiento al inicio');
  await saveDialog.getByRole('button', { name: 'Guardar versión', exact: true }).click();
  await expect(notice(page, 'Versión 2 guardada.')).toBeVisible();
  await expect(editor.getByText(/versión 2 guardada/)).toBeVisible();
  await expect(editor.getByText('Sin cambios pendientes')).toBeVisible();

  // 5. Activar con confirmación.
  await editor.getByRole('button', { name: 'Activar', exact: true }).click();
  const activate = page.getByRole('alertdialog', { name: 'Activar plantilla' });
  await expect(activate).toContainText('conservan su formato al reimprimirse');
  await activate.getByRole('button', { name: 'Activar', exact: true }).click();
  await expect(notice(page, `«${templateName}» es ahora la plantilla activa.`)).toBeVisible();
  await expect(editor.getByText(/versión 2 guardada · activa/)).toBeVisible();

  // 6. Muestra en el destino controlado: «enviada a la cola», nunca «impresa».
  await editor.getByRole('button', { name: 'Imprimir muestra', exact: true }).click();
  const sample = page.getByRole('dialog', { name: 'Imprimir una muestra' });
  await expect(sample).toContainText('No crea ventas, pagos, movimientos de caja ni documentos');
  await sample.getByRole('button', { name: new RegExp(printerName) }).click();
  await expect(sample).toContainText('Negrita y tamaños: No admitido');
  await sample.getByRole('button', { name: 'Continuar', exact: true }).click();
  await sample.getByRole('button', { name: 'Imprimir muestra', exact: true }).click();
  await expect(
    notice(page, /Muestra (enviada a la cola|aceptada)|Requiere intervención/),
  ).toBeVisible();
  await expect(
    sample.getByText(/Requiere intervención: revisa el trabajo en Cocina → Cola de impresión/),
  ).toBeVisible({
    timeout: 20000,
  });
  await expect(sample).not.toContainText(/\bimpres[oa]\b/i);
  await sample.getByRole('button', { name: 'Listo', exact: true }).click();

  // 7. Historial: las dos versiones con su nota y la vista previa de la versión 1.
  await editor.getByRole('button', { name: 'Historial', exact: true }).click();
  const history = page.getByRole('dialog', { name: `Historial de «${templateName}»` });
  await expect(history.getByRole('button', { name: /Versión 2 · vigente/ })).toBeVisible();
  await expect(history.getByText('Agradecimiento al inicio')).toBeVisible();
  await history.getByRole('button', { name: /^Versión 1/ }).click();
  await expect(history.getByRole('heading', { name: 'Versión 1', exact: true })).toBeVisible();
  await expect(history.locator('.paper-strip')).toBeVisible();
  await expect(history.locator('.paper-strip')).not.toContainText(fixedText);
  await expect(history.getByRole('button', { name: 'Restaurar esta versión' })).toBeVisible();
  await history.getByRole('button', { name: 'Cerrar', exact: true }).click();
  await editor.getByRole('button', { name: 'Volver a plantillas' }).click();
  await expect(receipts.getByText(templateName, { exact: true })).toBeVisible();

  // 8. Capturas del editor de la comanda y del corte definitivo (predeterminadas, solo lectura).
  for (const [kind, name] of [
    ['Comanda', 'comanda'],
    ['Corte definitivo', 'corte'],
  ] as const) {
    await kinds.getByRole('button', { name: kind, exact: true }).click();
    await page
      .getByRole('region', { name: `Plantillas de ${kind}` })
      .getByRole('button', { name: 'Ver Predeterminada', exact: true })
      .click();
    const view = page.getByRole('region', { name: 'Editor de Predeterminada' });
    await expect(view.locator('.paper-strip')).toBeVisible();
    await expect(view.getByText(/no se edita/)).toBeVisible();
    await checkLayout(page, testInfo, name);
    await view.getByRole('button', { name: 'Volver a plantillas' }).click();
  }
  await kinds.getByRole('button', { name: 'Ticket de venta', exact: true }).click();

  // 9. El ticket cobrado antes de editar se reimprime idéntico (misma versión de plantilla, sin
  // marca de reimpresión), aunque la plantilla activa ya es otra.
  await driver.request('POST', 'auth/login', { username: 'admin', password: 'admin' });
  const reprint = await call(
    driver,
    'POST',
    `payments/accounts/${sale.accountID}/documents/reprint`,
    {
      operation_id: randomUUID(),
      ticket_id: sale.ticketID,
      // Etapa 9F (decisión D3): reimprimir un documento que ya salió pide motivo.
      reason: 'Copia de la aceptación del diseñador',
    },
  );
  expect(reprint.status, JSON.stringify(reprint)).toBeTruthy();
  // La copia exacta también termina sin imprimirse (impresora sin hardware). La reimpresión y
  // la impresión con la plantilla vigente se bloquean entre sí mientras haya una copia sin resolver,
  // así que se cancela administrativamente antes de pedir la otra acción (como en la cola real).
  const copy = await settledJob(driver, reprint.job_id as string);
  if (copy.state !== 'cancelled' && copy.state !== 'submitted') {
    await call(driver, 'POST', `kitchen/jobs/${copy.id}/resolve`, {
      operation_id: randomUUID(),
      revision: copy.revision,
      decision: 'cancel',
      reason: 'Destino de aceptación sin hardware físico',
      printer_id: null,
    });
  }
  const reprinted = await call(driver, 'GET', `kitchen/tickets/${sale.ticketID}`);
  expect(reprinted.preview).toBe(sale.original);
  expect(reprinted.preview).not.toContain(fixedText);
  expect(reprinted.preview).not.toMatch(/REIMPRESI/i);

  // 10. En los documentos de la cuenta, «Imprimir con la plantilla vigente» es otra acción, con
  // su confirmación, y usa la plantilla activa.
  await openSection(page, 'Para llevar');
  await page
    .getByRole('group', { name: 'Pedidos para llevar' })
    .getByRole('button')
    .filter({ hasText: customer })
    .click();
  const bill = page.getByRole('complementary', { name: 'Cuenta actual' });
  await bill.getByRole('button', { name: /^Cobrar/ }).click();
  const documents = page.getByRole('region', { name: 'Documentos de la cuenta' });
  await expect(documents.getByRole('button', { name: 'Reimprimir ticket de pago' })).toBeVisible();
  const jobTickets = async () =>
    ((await call(driver, 'GET', 'kitchen/jobs')) as { ticket_id: string }[]).map(
      (job) => job.ticket_id,
    );
  const before = new Set(await jobTickets());
  await documents.getByRole('button', { name: 'Imprimir con la plantilla vigente' }).click();
  const rerender = page.getByRole('alertdialog', { name: 'Imprimir con la plantilla vigente' });
  await expect(rerender).toContainText('envía una copia idéntica al original');
  await expect(rerender).toContainText('plantilla activa hoy');
  await rerender.getByRole('button', { name: 'Imprimir con la plantilla vigente' }).click();
  await expect(notice(page, /Documento con la plantilla vigente solicitado/)).toBeVisible();
  // El documento con la plantilla vigente es un ticket nuevo (no una copia del original).
  const created = (await jobTickets()).filter((ticket) => !before.has(ticket));
  expect(created).toHaveLength(1);
  const renderedTicket = await call(driver, 'GET', `kitchen/tickets/${created[0]}`);
  expect(renderedTicket.preview).toContain(fixedText);
  // El original sigue igual después de la acción.
  expect((await call(driver, 'GET', `kitchen/tickets/${sale.ticketID}`)).preview).toBe(
    sale.original,
  );

  // 11. Restaurar la predeterminada: activa la del sistema y conserva la copia.
  await openAdministration(page, 'Documentos');
  await receipts.getByRole('button', { name: 'Restaurar predeterminada', exact: true }).click();
  const restore = page.getByRole('alertdialog', { name: 'Restaurar la plantilla predeterminada' });
  await expect(restore).toContainText('se conservan');
  await restore.getByRole('button', { name: 'Restaurar predeterminada', exact: true }).click();
  await expect(notice(page, 'Plantilla predeterminada activa.')).toBeVisible();
  await expect(receipts.getByText(templateName, { exact: true })).toBeVisible();
  await expect(
    receipts.locator('.documents-template-active').getByText('Predeterminada', { exact: true }),
  ).toBeVisible();
  await expect(page.locator('input[type="number"]')).toHaveCount(0);
  expect(errors).toEqual([]);
});
