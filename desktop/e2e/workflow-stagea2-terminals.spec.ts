import { randomUUID } from 'node:crypto';
import { expect, test } from '@playwright/test';
import type { Page } from '@playwright/test';
import { installDriver } from './native-driver';
import { openAdministration, topBar } from './navigation';
import { notice } from './notices';
import { chooseOption, expectSelectText } from './select';
import { claimPairing, requestPairingCode } from './pairing';

const serverURL = process.env.ADEL_BROWSER_SERVER_URL ?? 'http://127.0.0.1:8183';

// Etapa A2, partes C y D (§5, §4.5 y decisiones A2-D3 y A2-D9) con Go y PostgreSQL reales: en Sistema › Terminales ›
// Editar de un teléfono de meseros emparejado (e2e/pairing.ts, como lo haría la aplicación Android) se elige su
// «Impresora de precuentas», «Ver configuración» la informa y Go la guarda (la configuración y la terminal del teléfono
// la devuelven); «Sin impresora» la quita; «Avisar platillos listos», «Sonido» y «Vibración» se cambian y el teléfono
// los recibe en sus avisos (GET ready-alerts con su credencial); y una terminal de escritorio no ofrece nada de esto. La impresora es de aceptación: una cola del equipo
// que no existe (firma de datos de prueba en backend/internal/maintenance/signatures.go); aquí no se imprime nada. Se
// ordena después de las especificaciones de la A1 y antes de workflow-zz-test-data.

const precheckPrinterName = 'Precuentas A2 E2E';

async function login(page: Page) {
  await page.getByLabel('Usuario', { exact: true }).fill('admin');
  await page.getByLabel('Contraseña', { exact: true }).fill('admin');
  await page.getByRole('button', { name: 'Entrar', exact: true }).click();
}

test('Terminales: impresora de precuentas y avisos de platillos listos de un teléfono de meseros (etapa A2, partes C y D)', async ({
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
  const phoneToken = String(claimed.body.device_token);
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
  // El aviso «Terminal actualizada.» del guardado anterior puede seguir en pantalla: se espera a que Go lo tenga.
  await expect
    .poll(async () => {
      const cleared = await driver.request('GET', `devices/${phoneID}/configuration`);
      return cleared.body.precheck_printer;
    })
    .toBeNull();
  await page.getByRole('button', { name: 'Cerrar editor', exact: true }).click();
  await device.getByRole('button', { name: 'Ver configuración', exact: true }).click();
  await expect(panel).toContainText('Sin impresora: este dispositivo no imprime precuentas');
  await panel.getByRole('button', { name: 'Cerrar configuración', exact: true }).click();

  // 4. Avisos de platillos listos: activados por omisión; se apaga el sonido y el teléfono lo recibe en sus avisos.
  await device.getByRole('button', { name: 'Editar', exact: true }).click();
  const alerts = page.getByRole('button', { name: 'Avisar platillos listos', exact: true });
  const sound = page.getByRole('button', { name: 'Sonido', exact: true });
  await expect(alerts).toHaveAttribute('aria-pressed', 'true');
  await expect(sound).toHaveAttribute('aria-pressed', 'true');
  await sound.click();
  await expect(sound).toHaveAttribute('aria-pressed', 'false');
  await page.getByRole('button', { name: 'Guardar cambios', exact: true }).click();
  await expect(notice(page, 'Terminal actualizada.')).toBeVisible();
  const readBatch = async () => {
    const readyAlerts = await fetch(`${serverURL}/api/v1/ready-alerts`, {
      headers: {
        'X-Device-ID': phoneID,
        'X-Device-Token': phoneToken,
        'X-Client-Version': '0.2.0',
        'X-Client-Kind': 'android',
      },
    });
    expect(readyAlerts.status).toBe(200);
    return (await readyAlerts.json()) as {
      active: boolean;
      settings: Record<string, boolean>;
    };
  };
  // El aviso «Terminal actualizada.» del guardado anterior puede seguir en pantalla: se espera a que Go lo tenga.
  await expect
    .poll(async () => (await readBatch()).settings)
    .toEqual({ enabled: true, sound: false, vibration: true });
  // Nadie entró con PIN en este teléfono: el servicio no corre, pero ve sus ajustes.
  expect((await readBatch()).active).toBe(false);
  await page.getByRole('button', { name: 'Cerrar editor', exact: true }).click();
  await device.getByRole('button', { name: 'Ver configuración', exact: true }).click();
  await expect(panel).toContainText('Avisar platillos listosActivado · sin sonido, con vibración');
  await panel.getByRole('button', { name: 'Cerrar configuración', exact: true }).click();

  // 5. Una terminal de escritorio no ofrece la impresora de precuentas ni los avisos.
  const desk = page.locator('tbody tr').filter({ hasText: 'Escritorio' }).first();
  await desk.getByRole('button', { name: 'Editar', exact: true }).click();
  await expect(page.getByRole('heading', { name: /^Editar / })).toBeVisible();
  await expect(page.getByRole('combobox', { name: 'Impresora de precuentas' })).toHaveCount(0);
  await expect(
    page.getByRole('button', { name: 'Avisar platillos listos', exact: true }),
  ).toHaveCount(0);
});
