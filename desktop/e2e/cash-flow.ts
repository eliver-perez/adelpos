import { expect } from '@playwright/test';
import type { Locator, Page, TestInfo } from '@playwright/test';
import { openAdministration, openCash, openSection } from './navigation';
import { expectNoticeClosesUnderPointer, expectNoticeLayout, notice } from './notices';

// Recorrido de caja de la etapa 7 (fases B y C) contra Go y PostgreSQL reales: apertura por
// denominaciones, ingreso y retiro autoautorizado, corte parcial con su vista previa, tope de caja
// activado y desactivado, arqueo ciego que pausa el cobro, cierre con faltante y vista previa del
// corte definitivo. Etapa 7.1 (fase C): aviso sin turno en la captura, ingreso desde el atajo
// «Caja» con su aviso de confirmación y paneles que se cierran tras registrar. Nada se imprime: el
// destino de la aceptación no tiene hardware. Todas las cifras que se verifican las calcula Go; la
// interfaz solo las muestra. El administrador tiene todos los permisos, por lo que su propio
// permiso autoriza el retiro y ve el esperado y las diferencias después del cierre.

async function typeOnKeypad(keypad: Locator, digits: string) {
  for (const digit of digits) {
    const name = digit === '.' ? 'Punto decimal' : digit;
    await keypad.getByRole('button', { name, exact: true }).click();
  }
}

// Sin desplazamiento horizontal en tema claro y oscuro, con capturas para la revisión visual.
async function cashThemes(page: Page, testInfo: TestInfo, name: string) {
  for (const size of [
    { width: 1280, height: 720 },
    { width: 1920, height: 1080 },
  ]) {
    await page.setViewportSize(size);
    for (const theme of ['light', 'dark']) {
      await page.evaluate((value) => {
        document.documentElement.dataset.theme = value;
      }, theme);
      await page.waitForTimeout(300);
      expect(
        await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
      ).toBe(true);
      await page.screenshot({
        path: testInfo.outputPath(`${name}-${theme}-${size.width}x${size.height}.png`),
      });
    }
  }
  await page.evaluate(() => {
    document.documentElement.dataset.theme = 'light';
  });
  await page.setViewportSize({ width: 1280, height: 720 });
}

function cashActions(page: Page) {
  return page.getByRole('group', { name: 'Acciones del turno' });
}

function toastWith(page: Page, text: string | RegExp) {
  return page.locator('[data-sonner-toaster] .app-toast').filter({ hasText: text });
}

// 0. Etapa 7.1: sin turno, la captura sigue disponible con un aviso discreto y el atajo «Caja» de
// la barra lo indica.
async function noShiftInCapture(page: Page) {
  await expect(page.getByRole('heading', { name: 'Mesa 1 · Cuenta #2' })).toBeVisible();
  const notice = page.getByRole('status', { name: 'Turno de caja', exact: true });
  await expect(notice).toContainText('No hay turno abierto: puedes capturar, pero no cobrar.');
  await expect(notice.getByRole('button', { name: 'Abrir turno', exact: true })).toBeVisible();
  const shortcut = page
    .locator('.capture-topbar')
    .getByRole('button', { name: 'Caja · Sin turno', exact: true });
  await expect(shortcut).toBeVisible();
  const box = await shortcut.boundingBox();
  expect(box!.height).toBeGreaterThanOrEqual(44);
}

// 1. Apertura por denominaciones: 2 × $200 + 1 × $100; Go calcula el fondo de $500.00.
async function openByDenominations(page: Page, testInfo: TestInfo) {
  await openCash(page);
  const opening = page.getByRole('region', { name: 'Turno de caja', exact: true });
  await expect(opening).toContainText('No hay un turno abierto en Caja 1');
  await opening.getByRole('button', { name: 'Por denominaciones', exact: true }).click();
  const keypad = opening.getByRole('group', { name: 'Teclado numérico del conteo' });
  const twoHundred = opening.getByLabel('Billetes de $200', { exact: true });
  await twoHundred.click();
  await typeOnKeypad(keypad, '2');
  await opening.getByLabel('Billetes de $100', { exact: true }).click();
  await typeOnKeypad(keypad, '1');
  await expect(twoHundred).toHaveValue('2');
  const box = await twoHundred.boundingBox();
  expect(box!.height).toBeGreaterThanOrEqual(44);
  await expect(opening.getByLabel('Importe de Billetes de $200', { exact: true })).toHaveText(
    '$400.00',
  );
  await expect(opening.getByLabel('Fondo contado', { exact: true })).toContainText('$500.00');
  await cashThemes(page, testInfo, 'cash-opening');
  await opening
    .getByRole('button', { name: 'Abrir turno con fondo de $500.00', exact: true })
    .click();
  await expect(notice(page, 'Turno de Caja 1 abierto con fondo de $500.00.')).toBeVisible();
  const status = page.getByLabel('Turno de Caja 1', { exact: true });
  await expect(status).toContainText('Abierto');
  await expect(status).toContainText('$500.00');
}

// 2. Ingreso de $100 desde el atajo «Caja» de la captura (etapa 7.1) y retiro de $200 desde Caja,
// autoautorizado por el permiso del administrador.
async function incomeFromShortcut(page: Page, testInfo: TestInfo) {
  await openSection(page, 'Mesas');
  await expect(page.getByRole('heading', { name: 'Mesa 1 · Cuenta #2' })).toBeVisible();
  await expect(page.getByRole('status', { name: 'Turno de caja', exact: true })).toHaveCount(0);
  const shortcut = page
    .locator('.capture-topbar')
    .getByRole('button', { name: 'Caja · Turno abierto', exact: true });
  await shortcut.click();
  const menu = page.getByRole('group', { name: 'Menú rápido de caja' });
  for (const option of ['Ingreso', 'Retiro', 'Gasto', 'Corte parcial', 'Ir a Caja']) {
    const button = menu.getByRole('button', { name: option, exact: true });
    await expect(button).toBeEnabled();
    const box = await button.boundingBox();
    expect(box!.height).toBeGreaterThanOrEqual(44);
  }
  await menu.getByRole('button', { name: 'Ingreso', exact: true }).click();
  // La hoja superpuesta contiene el mismo formulario de Caja.
  const sheet = page.getByRole('dialog', { name: 'Ingreso de efectivo', exact: true });
  const income = sheet.getByRole('region', { name: 'Ingreso de efectivo', exact: true });
  await income.getByRole('button', { name: 'Aportación adicional al fondo', exact: true }).click();
  await typeOnKeypad(income.getByRole('group', { name: 'Teclado numérico del importe' }), '100');
  await cashThemes(page, testInfo, 'cash-shortcut-sheet');
  await income.getByRole('button', { name: 'Registrar ingreso $100.00', exact: true }).click();
  // La hoja se cierra y el aviso confirma tipo, importe y concepto, con «Imprimir comprobante»
  // (no se imprimió al registrar). No se pulsa: el destino de la aceptación no tiene hardware.
  await expect(sheet).toHaveCount(0);
  const title = 'Ingreso registrado: $100.00 · Aportación adicional al fondo';
  await expect(
    toastWith(page, title).getByRole('button', { name: 'Imprimir comprobante', exact: true }),
  ).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Mesa 1 · Cuenta #2' })).toBeVisible();
  // El aviso aparece arriba al centro, debajo de la cabecera, y se cierra con un toque.
  await expectNoticeLayout(page, title);
  // «Ir a Caja» abre la pantalla Caja; el ingreso ya cuenta en los registros del turno.
  await shortcut.click();
  await menu.getByRole('button', { name: 'Ir a Caja', exact: true }).click();
  await expect(page.getByText(/1 operación de efectivo/)).toBeVisible();
}

async function withdrawalFromCash(page: Page, testInfo: TestInfo) {
  // Etapa 7.1: tarjetas con icono grande y acento por tipo, en ambos temas.
  await cashActions(page).scrollIntoViewIfNeeded();
  await cashThemes(page, testInfo, 'cash-action-cards');
  await cashActions(page).getByRole('button', { name: 'Retiro de efectivo', exact: true }).click();
  await expect(
    cashActions(page).getByRole('button', { name: 'Retiro de efectivo', exact: true }),
  ).toHaveAttribute('aria-pressed', 'true');
  const withdrawal = page.getByRole('region', { name: 'Retiro de efectivo', exact: true });
  await withdrawal.getByRole('button', { name: 'Resguardo', exact: true }).click();
  await typeOnKeypad(
    withdrawal.getByRole('group', { name: 'Teclado numérico del importe' }),
    '200',
  );
  // §0-B: con su propio permiso no se pide una segunda contraseña.
  await expect(withdrawal.getByText(/Autorización propia/)).toBeVisible();
  await expect(withdrawal.getByLabel('Contraseña del supervisor')).toHaveCount(0);
  await withdrawal.getByRole('button', { name: 'Registrar retiro $200.00', exact: true }).click();
  // Etapa 7.1: el panel se cierra y vuelve a Operación; el aviso lleva la autorización.
  await expect(withdrawal).toHaveCount(0);
  await expect(cashActions(page)).toBeVisible();
  await expect(toastWith(page, 'Retiro registrado: $200.00 · Resguardo')).toContainText(
    'Autorizado con el permiso propio de Administrador',
  );
  await expectNoticeClosesUnderPointer(page, 'Retiro registrado: $200.00 · Resguardo');
}

// 3. Corte parcial: informe que no mueve dinero; con permiso, Go entrega el esperado ($400.00).
async function partialCut(page: Page) {
  await cashActions(page).getByRole('button', { name: 'Corte parcial', exact: true }).click();
  const cut = page.getByRole('region', { name: 'Corte parcial', exact: true });
  await expect(cut).toContainText('no mueve dinero');
  await expect(cut.getByRole('button', { name: 'Retirar efectivo', exact: true })).toBeVisible();
  await cut.getByRole('button', { name: 'Generar corte parcial', exact: true }).click();
  // Etapa 7.1: el panel se cierra con un aviso sin cifras; el corte se consulta al reabrirlo.
  await expect(notice(page, 'Corte parcial #1 generado.')).toBeVisible();
  await expect(cut).toHaveCount(0);
  await cashActions(page).getByRole('button', { name: 'Corte parcial', exact: true }).click();
  await cut.getByRole('button', { name: /^Corte parcial #1 · / }).click();
  const report = cut.getByRole('article', { name: 'Corte parcial #1', exact: true });
  await expect(report).toBeVisible();
  await expect(report.getByLabel('Efectivo esperado en caja', { exact: true })).toContainText(
    '$400.00',
  );
  await expect(report).toContainText('$200.00');
}

// 3b. Vista previa del corte (fase C): Go la renderiza con la plantilla de impresión y, para el
// administrador, con el efectivo esperado. Solo consulta: no imprime ni registra documentos.
async function previewPartialCut(page: Page, testInfo: TestInfo) {
  const cut = page.getByRole('region', { name: 'Corte parcial', exact: true });
  await cut.getByRole('button', { name: 'Vista previa', exact: true }).click();
  const preview = cut.getByLabel('Vista previa de CORTE PARCIAL #1', { exact: true });
  await expect(preview).toContainText('CORTE PARCIAL #1');
  await expect(preview).toContainText('Caja: Caja 1');
  await expect(preview).toContainText('VENTAS Y PAGOS POR MÉTODO');
  await expect(preview).toContainText('EFECTIVO ESPERADO');
  await expect(preview).toContainText('$400.00');
  await expect(preview).toContainText('No es un comprobante fiscal');
  await expect(cut.getByRole('button', { name: 'Imprimir el corte', exact: true })).toHaveAttribute(
    'aria-pressed',
    'false',
  );
  await cashThemes(page, testInfo, 'cash-cut-preview');
  await cut.getByRole('button', { name: 'Ocultar vista previa', exact: true }).click();
}

// 3c. Tope de caja (fase C): se activa la función, se fija un tope de $300.00 y el administrador
// (que consulta el esperado de $400.00) ve el aviso que calcula Go. Al desactivar la función el
// aviso desaparece: desactivada no tiene efecto.
async function cashLimitWarning(page: Page, testInfo: TestInfo) {
  const features = page.getByRole('region', { name: 'Funciones del establecimiento' });
  const toggle = features.getByRole('button', { name: 'Tope de efectivo en caja', exact: true });
  await openAdministration(page, 'Configuración');
  await expect(toggle).toHaveAttribute('aria-pressed', 'false');
  await toggle.click();
  await expect(notice(page, 'Tope de efectivo en caja: activada.')).toBeVisible();

  await openCash(page);
  const sections = page.getByRole('tablist', { name: 'Secciones de caja' });
  await sections.getByRole('tab', { name: 'Configuración', exact: true }).click();
  await page.getByRole('button', { name: /^Tope de caja/ }).click();
  const limit = page.getByLabel(/^Tope en pesos/);
  await limit.fill('300');
  await page.getByRole('button', { name: 'Guardar tope de caja', exact: true }).click();
  await expect(limit).toHaveValue('300.00');
  await sections.getByRole('tab', { name: 'Operación', exact: true }).click();
  const warning = page.getByRole('status', { name: 'Tope de caja superado', exact: true });
  await expect(warning).toContainText('considera un retiro');
  await expect(warning).toContainText('no impide cobrar');
  await cashThemes(page, testInfo, 'cash-limit');

  await openAdministration(page, 'Configuración');
  await toggle.click();
  await expect(notice(page, 'Tope de efectivo en caja: desactivada.')).toBeVisible();
  await openCash(page);
  await expect(page.getByLabel('Turno de Caja 1', { exact: true })).toContainText('Abierto');
  await expect(warning).toHaveCount(0);
}

// 4. Arqueo: se confirma la pausa y el cobro de la cuenta queda bloqueado con el mensaje de Go.
async function startClosingAndCheckCheckout(page: Page) {
  await cashActions(page).getByRole('button', { name: 'Arqueo y cierre', exact: true }).click();
  await page
    .getByRole('region', { name: 'Arqueo de cierre', exact: true })
    .getByRole('button', { name: 'Iniciar arqueo de cierre', exact: true })
    .click();
  const dialog = page.getByRole('alertdialog');
  await expect(dialog).toContainText('quedará pausada');
  await dialog
    .getByRole('button', { name: 'Iniciar arqueo y pausar la caja', exact: true })
    .click();
  const banner = page.getByRole('status', { name: 'Cierre en curso', exact: true });
  await expect(banner).toContainText('Arqueo iniciado por Administrador');
  await expect(banner).toContainText('WebView temporal');
  await expect(
    page.getByRole('region', { name: 'Arqueo de cierre en curso', exact: true }),
  ).toBeVisible();

  await openSection(page, 'Mesas');
  // El atajo «Caja» de la captura refleja la pausa; no es el aviso sin turno.
  await expect(
    page.locator('.capture-topbar').getByRole('button', { name: 'Caja · En arqueo', exact: true }),
  ).toBeVisible();
  await expect(page.getByRole('status', { name: 'Turno de caja', exact: true })).toHaveCount(0);
  const bill = page.getByRole('complementary', { name: 'Cuenta actual' });
  await bill.getByRole('button', { name: 'Cobrar · $150.00', exact: true }).click();
  const shift = page.getByRole('region', { name: 'Turno de caja', exact: true });
  await expect(shift).toContainText('La caja Caja 1 está en arqueo de cierre');
  await expect(shift.getByRole('button', { name: /^Abrir turno/ })).toHaveCount(0);
  await expect(
    page
      .getByRole('group', { name: 'Métodos de pago disponibles' })
      .getByRole('button', { name: 'Efectivo', exact: true }),
  ).toBeDisabled();
  await page.getByRole('button', { name: 'Volver a la cuenta', exact: true }).first().click();
}

// 5. Conteo ciego (1 × $200 + 1 × $100 = $300.00 declarados), confirmación y resultado con el
// faltante de $100.00 que calcula Go frente a los $400.00 esperados.
async function countAndClose(page: Page, testInfo: TestInfo) {
  await openCash(page);
  const closing = page.getByRole('region', { name: 'Arqueo de cierre en curso', exact: true });
  await expect(closing).toBeVisible();
  const keypad = closing.getByRole('group', { name: 'Teclado numérico del arqueo' });
  await closing.getByLabel('Billetes de $200', { exact: true }).click();
  await typeOnKeypad(keypad, '1');
  await closing.getByLabel('Billetes de $100', { exact: true }).click();
  await typeOnKeypad(keypad, '1');
  await expect(closing.getByLabel('Total declarado', { exact: true })).toContainText('$300.00');
  // El borrador se guarda solo en el servidor (sobrevive a la navegación y al cierre de la app).
  await expect(closing.getByText(/Cambios sin guardar/)).toHaveCount(0);
  await expect(closing.getByText(/Borrador guardado en el servidor/)).toBeVisible();
  // Captura ciega: ninguna cifra del esperado ni de diferencias en la pantalla del arqueo.
  const cashPage = page.locator('.cash-page');
  await expect(cashPage.getByText(/Efectivo esperado/)).toHaveCount(0);
  await expect(cashPage.getByText(/Diferencia/)).toHaveCount(0);
  await expect(cashPage.getByText('$400.00')).toHaveCount(0);
  await cashThemes(page, testInfo, 'cash-closing');
  const confirm = closing.getByRole('button', { name: 'Confirmar cierre del turno', exact: true });
  const bounds = await confirm.boundingBox();
  expect(bounds!.height).toBeGreaterThanOrEqual(44);
  await confirm.click();
  const dialog = page.getByRole('alertdialog');
  await expect(dialog).toContainText('$300.00');
  await dialog.getByRole('button', { name: 'Cerrar el turno', exact: true }).click();

  const result = page.getByRole('region', { name: 'Resultado del cierre', exact: true });
  await expect(result).toContainText('quedó cerrado');
  await expect(result.getByLabel('Efectivo declarado', { exact: true })).toContainText('$300.00');
  await expect(result.getByLabel('Efectivo esperado en caja', { exact: true })).toContainText(
    '$400.00',
  );
  const differences = result.getByLabel('Diferencias del arqueo', { exact: true });
  await expect(differences).toContainText('Diferencia en pesos · Faltante');
  await expect(differences).toContainText('−$100.00');
  // Vista previa del corte definitivo (fase C) con lo declarado, el esperado y la diferencia.
  await result.getByRole('button', { name: /^Corte definitivo/ }).click();
  await result.getByRole('button', { name: 'Vista previa', exact: true }).click();
  const preview = result.getByLabel('Vista previa de CORTE DEFINITIVO', { exact: true });
  await expect(preview).toContainText('EFECTIVO DECLARADO');
  await expect(preview).toContainText('$300.00');
  await expect(preview).toContainText('DIFERENCIAS (CONTADO - ESPERADO)');
  await expect(preview).toContainText('-$100.00');
  await cashThemes(page, testInfo, 'cash-closure');
  await page.getByRole('button', { name: 'Volver a la caja', exact: true }).click();
  await expect(page.getByText('No hay un turno abierto en Caja 1.', { exact: true })).toBeVisible();
}

// 6. Movimientos, historial y reportes del turno cerrado, con las cifras de Go.
async function reviewClosedShift(page: Page) {
  const sections = page.getByRole('tablist', { name: 'Secciones de caja' });
  await sections.getByRole('tab', { name: 'Movimientos', exact: true }).click();
  // El nombre accesible de un <select> dentro de su etiqueta incluye la opción elegida.
  const chooser = page.getByRole('combobox', { name: /^Turno/ });
  await expect(chooser).toHaveValue('');
  const closedOption = chooser.locator('option', { hasText: 'Cerrado' }).first();
  await chooser.selectOption({ value: (await closedOption.getAttribute('value'))! });
  const table = page.locator('.cash-page table').first();
  await expect(table).toContainText('Aportación adicional al fondo');
  await expect(table).toContainText('Resguardo');
  await expect(table).toContainText('Salida');

  await sections.getByRole('tab', { name: 'Historial', exact: true }).click();
  const closedRow = page.locator('.cash-page tbody tr', { hasText: 'Faltante' }).first();
  await expect(closedRow).toContainText('$300.00');
  await expect(closedRow).toContainText('−$100.00 · Faltante');
  await closedRow.getByRole('button', { name: /^Detalle del turno de Caja 1/ }).click();
  await expect(
    page.getByRole('region', { name: 'Resultado del cierre', exact: true }),
  ).toContainText('quedó cerrado');
  await page.getByRole('button', { name: 'Volver al historial', exact: true }).click();

  await sections.getByRole('tab', { name: 'Reportes', exact: true }).click();
  await expect(page.getByText('La caja no tiene un turno activo.', { exact: true })).toBeVisible();
  await sections.getByRole('tab', { name: 'Operación', exact: true }).click();
}

// Recorrido completo; termina en Mesas con la cuenta lista para el recorrido de cobro, que
// abre un turno nuevo en la misma caja.
export async function checkCashShift(page: Page, testInfo: TestInfo) {
  await noShiftInCapture(page);
  await openByDenominations(page, testInfo);
  await incomeFromShortcut(page, testInfo);
  await withdrawalFromCash(page, testInfo);
  await partialCut(page);
  await previewPartialCut(page, testInfo);
  await cashLimitWarning(page, testInfo);
  await startClosingAndCheckCheckout(page);
  await countAndClose(page, testInfo);
  await reviewClosedShift(page);
  await openSection(page, 'Mesas');
}
