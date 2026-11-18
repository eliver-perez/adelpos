import { randomUUID } from 'node:crypto';
import { expect, test } from '@playwright/test';
import type { Page } from '@playwright/test';
import { installDriver } from './native-driver';
import { openAdministration, topBar } from './navigation';
import { notice } from './notices';
import { chooseOption, expectSelectText } from './select';
import { claimPairing, requestPairingCode } from './pairing';

// Etapa A2, parte C (§5 y decisión A2-D3) con Go y PostgreSQL reales: en Sistema › Terminales › Editar de un teléfono
// de meseros emparejado (e2e/pairing.ts, como lo haría la aplicación Android) se elige su «Impresora de precuentas»,
// «Ver configuración» la informa y Go la guarda (la configuración y la terminal del teléfono la devuelven); «Sin
// impresora» la quita, y una terminal de escritorio no la ofrece. La impresora es de aceptación: una cola del equipo
// que no existe (firma de datos de prueba en backend/internal/maintenance/signatures.go); aquí no se imprime nada. Se
// ordena después de las especificaciones de la A1 y antes de workflow-zz-test-data.

const precheckPrinterName = 'Precuentas A2 E2E';

async function login(page: Page) {
  await page.getByLabel('Usuario', { exact: true }).fill('admin');
  await page.getByLabel('Contraseña', { exact: true }).fill('admin');
  await page.getByRole('button', { name: 'Entrar', exact: true }).click();
}

test('Terminales: impresora de precuentas de un teléfono de meseros (etapa A2, parte C)', async ({
  page,
}) => {
  const driver = await installDriver(page);
  await page.goto('/');
  await login(page);
  await expect(topBar(page)).toBeVisible();

  // Un teléfono de meseros emparejado y la impresora de precuentas de aceptación.
  const pairing = await requestPairingCode('Tableta de precuentas A2', 'Android 15', 'android');
  const approved = await driver.request(
    'POST',
    `devices/pairing-requests/${pairing.requestID}/approve`,
    { code: pairing.code },
  );
  expect(approved.status).toBe(200);
  const claimed = await claimPairing(pairing);
  expect(claimed.status).toBe(200);
  const phoneID = String(claimed.body.device_id);
  const printers = (await driver.request('GET', 'kitchen/printers')).body as unknown as {
    id: string;
    name: string;
  }[];
  let printerID = printers.find((printer) => printer.name === precheckPrinterName)?.id;
  if (!printerID) {
    const created = await driver.request('POST', 'kitchen/printers', {
      operation_id: randomUUID(),
      name: 'Precuentas A2 E2E',
      transport: 'cups',
      destination: 'ADEL_E2E_SIN_IMPRESORA',
      port: 9100,
      paper_width: 80,
      copies: 1,
      cutter_option: '',
      accepts_escpos: false,
      active: true,
      fallback_id: null,
    });
    expect(created.status, JSON.stringify(created.body)).toBe(200);
    printerID = String(created.body.id);
  }

  // 1. Elegirla en Terminales › Editar (sin impresora por omisión).
  await openAdministration(page, 'Terminales');
  const device = page.locator('tbody tr').filter({ hasText: 'Tableta de precuentas A2' });
  await expect(device).toContainText('Teléfono o tableta');
  await device.getByRole('button', { name: 'Editar', exact: true }).click();
  const select = page.getByRole('combobox', { name: 'Impresora de precuentas' });
  await expectSelectText(select, 'Sin impresora');
  await chooseOption(select, precheckPrinterName);
  await page.getByRole('button', { name: 'Guardar cambios', exact: true }).click();
  await expect(notice(page, 'Terminal actualizada.')).toBeVisible();
  const configuration = await driver.request('GET', `devices/${phoneID}/configuration`);
  expect(configuration.body.precheck_printer).toMatchObject({
    id: printerID,
    name: precheckPrinterName,
    problem: '',
  });
  await page.getByRole('button', { name: 'Cerrar editor', exact: true }).click();

  // 2. «Ver configuración» la informa.
  await device.getByRole('button', { name: 'Ver configuración', exact: true }).click();
  const panel = page.getByRole('region', { name: 'Configuración de Tableta de precuentas A2' });
  await expect(panel).toContainText(`Impresora de precuentas${precheckPrinterName}`);
  await panel.getByRole('button', { name: 'Cerrar configuración', exact: true }).click();

  // 3. «Sin impresora» la quita: el teléfono deja de imprimir precuentas.
  await device.getByRole('button', { name: 'Editar', exact: true }).click();
  await expectSelectText(select, precheckPrinterName);
  await chooseOption(select, 'Sin impresora');
  await page.getByRole('button', { name: 'Guardar cambios', exact: true }).click();
  await expect(notice(page, 'Terminal actualizada.')).toBeVisible();
  const cleared = await driver.request('GET', `devices/${phoneID}/configuration`);
  expect(cleared.body.precheck_printer).toBeNull();
  await page.getByRole('button', { name: 'Cerrar editor', exact: true }).click();
  await device.getByRole('button', { name: 'Ver configuración', exact: true }).click();
  await expect(panel).toContainText('Sin impresora: este dispositivo no imprime precuentas');
  await panel.getByRole('button', { name: 'Cerrar configuración', exact: true }).click();

  // 4. Una terminal de escritorio no la ofrece.
  const desk = page.locator('tbody tr').filter({ hasText: 'Escritorio' }).first();
  await desk.getByRole('button', { name: 'Editar', exact: true }).click();
  await expect(page.getByRole('heading', { name: /^Editar / })).toBeVisible();
  await expect(page.getByRole('combobox', { name: 'Impresora de precuentas' })).toHaveCount(0);
});
