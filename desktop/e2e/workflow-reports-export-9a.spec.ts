import { expect, test } from '@playwright/test';
import { installDriver } from './native-driver';
import { openAdministration } from './navigation';
import { notice } from './notices';
import { chooseOption } from './select';

// Etapa 9A, parte C1: exportación de un reporte a PDF y Excel desde la pantalla Reportes con la
// operación cerrada export_report. El diálogo nativo «Guardar como» se simula en native-driver.ts
// (guardar o cancelar); Go genera el archivo real y Node lo valida como Rust (tipo exacto, firma y
// nombre saneado) sin que los bytes ni los tokens lleguen al navegador. El contenido del PDF y del
// libro se prueba en Go (texto extraído, libro abierto con excelize y descomprimido). El nombre del
// archivo la ordena después de las especificaciones existentes: solo lee, no crea registros.

test('reportes 9A: exportar PDF y Excel con el diálogo nativo simulado (guardar y cancelar)', async ({
  page,
}) => {
  test.setTimeout(180000);
  const errors: string[] = [];
  page.on('pageerror', (error) => errors.push(error.message));
  const driver = await installDriver(page);
  await driver.request('POST', 'auth/login', { username: 'admin', password: 'admin' });
  const keyboard = await driver.request('PUT', 'terminal/preferences', {
    keyboard_mode: 'disabled',
  });
  expect(keyboard.status, JSON.stringify(keyboard.body)).toBeLessThan(300);
  await driver.request('POST', 'auth/logout');
  await page.setViewportSize({ width: 1280, height: 720 });
  await page.goto('/');
  await page.getByLabel('Usuario', { exact: true }).fill('admin');
  await page.getByLabel('Contraseña', { exact: true }).fill('admin');
  await page.getByRole('button', { name: 'Entrar', exact: true }).click();
  await openAdministration(page, 'Reportes');
  await expect(page.getByRole('heading', { name: 'Reportes', exact: true })).toBeVisible();
  await page
    .getByRole('tablist', { name: 'Grupos de reportes' })
    .getByRole('tab', { name: 'Pagos', exact: true })
    .click();
  const picker = page.getByRole('combobox', { name: 'Reporte', exact: true });
  if (await picker.isVisible()) {
    await chooseOption(picker, 'Cobros por método');
  } else {
    await page
      .getByRole('navigation', { name: 'Reportes de Pagos' })
      .getByRole('button', { name: 'Cobros por método', exact: true })
      .click();
  }
  await expect(page.getByRole('heading', { name: 'Cobros por método', exact: true })).toBeVisible();
  const exportGroup = page.getByRole('group', { name: 'Exportar' });
  const namePattern = /^cobros-por-metodo_\d{4}-\d{2}-\d{2}(_\d{4}-\d{2}-\d{2})?\.(pdf|xlsx)$/;

  // 1. PDF guardado: el navegador recibe solo el resultado; Node conserva el archivo real de Go.
  await exportGroup.getByRole('button', { name: 'Exportar PDF', exact: true }).click();
  await expect(notice(page, 'Cobros por método: PDF guardado.')).toBeVisible({ timeout: 30000 });
  expect(driver.exports).toHaveLength(1);
  const pdf = driver.exports[0]!;
  expect(pdf.report).toBe('payments.by_method');
  expect(pdf.contentType).toBe('application/pdf');
  expect(pdf.bytes.subarray(0, 5).toString('latin1')).toBe('%PDF-');
  expect(pdf.fileName).toMatch(namePattern);
  expect(pdf.fileName.endsWith('.pdf')).toBe(true);
  await expect(notice(page, pdf.fileName)).toBeVisible();

  // 2. Excel cancelado en el diálogo: no hay error ni archivo.
  driver.nextSaveDialog('cancel');
  await exportGroup.getByRole('button', { name: 'Exportar Excel', exact: true }).click();
  await expect(notice(page, /Exportación a Excel cancelada/)).toBeVisible({ timeout: 30000 });
  expect(driver.exports).toHaveLength(1);
  await expect(
    exportGroup.getByRole('button', { name: 'Exportar Excel', exact: true }),
  ).toBeEnabled();

  // 3. Excel guardado: libro .xlsx real (firma PK\x03\x04) con su nombre sugerido.
  await exportGroup.getByRole('button', { name: 'Exportar Excel', exact: true }).click();
  await expect(notice(page, 'Cobros por método: Excel guardado.')).toBeVisible({
    timeout: 30000,
  });
  expect(driver.exports).toHaveLength(2);
  const workbook = driver.exports[1]!;
  expect(workbook.contentType).toBe(
    'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
  );
  expect([...workbook.bytes.subarray(0, 4)]).toEqual([0x50, 0x4b, 0x03, 0x04]);
  expect(workbook.fileName).toMatch(namePattern);
  expect(workbook.fileName.endsWith('.xlsx')).toBe(true);

  // 4. La operación cerrada rechaza reportes fuera de la lista y parámetros no admitidos sin pedir
  // nada a Go, igual que Rust.
  const rejected = await page.evaluate(async () => {
    const native = window as unknown as {
      __TAURI_INTERNALS__: { invoke: (command: string, args: unknown) => Promise<unknown> };
    };
    const outcomes: string[] = [];
    for (const args of [
      { report: '../auth/me', format: 'pdf', query: {} },
      { report: 'payments.by_method', format: 'csv', query: {} },
      { report: 'payments.by_method', format: 'pdf', query: { page: '2' } },
      { report: 'payments.by_method', format: 'xlsx', query: { paper: 'a4' } },
    ]) {
      try {
        await native.__TAURI_INTERNALS__.invoke('export_report', args);
        outcomes.push('aceptado');
      } catch {
        outcomes.push('rechazado');
      }
    }
    return outcomes;
  });
  expect(rejected).toEqual(['rechazado', 'rechazado', 'rechazado', 'rechazado']);
  expect(driver.exports).toHaveLength(2);
  expect(errors).toEqual([]);
});
