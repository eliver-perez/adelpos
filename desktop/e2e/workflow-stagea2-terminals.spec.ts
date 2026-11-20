import { randomUUID } from 'node:crypto';
import { expect, test } from '@playwright/test';
import type { Page } from '@playwright/test';
import { installDriver } from './native-driver';
import { openAdministration, topBar } from './navigation';
import { notice } from './notices';
import { chooseOption, expectSelectText } from './select';
import { claimPairing, PairingRequestError, requestPairingCode } from './pairing';

const serverURL = process.env.ADEL_BROWSER_SERVER_URL ?? 'http://127.0.0.1:8183';

// Etapa A2, partes C y D (§5, §4.5 y decisiones A2-D3 y A2-D9) con Go y PostgreSQL reales: en Sistema › Terminales ›
// Editar de un teléfono de meseros emparejado (e2e/pairing.ts, como lo haría la aplicación Android) se elige su
// «Impresora de precuentas», «Ver configuración» la informa y Go la guarda (la configuración y la terminal del teléfono
// la devuelven); «Sin impresora» la quita; «Avisar platillos listos», «Sonido» y «Vibración» se cambian y el teléfono
// los recibe en sus avisos (GET ready-alerts con su credencial); y una terminal de escritorio no ofrece nada de esto.
// Desde la etapa A3 (§5, A3-D7) también lo que cada tipo ve: el teléfono elige su pantalla inicial solo entre Mesas y
// Para llevar, sin caja física ni teclado (Go rechaza «Cocina»), su caja es un guion en la tabla, el identificador ya no
// está en la tabla y «Ver configuración» lo dice como «Identificador»; la terminal de escritorio conserva las cuatro
// pantallas. Se hace aquí, con el mismo teléfono, para no pedir otro código de emparejamiento (límite de 5 por origen
// cada 10 minutos). La impresora es de aceptación: una cola del equipo
// que no existe (firma de datos de prueba en backend/internal/maintenance/signatures.go); aquí no se imprime nada. Se
// ordena después de las especificaciones de la A1 y antes de workflow-zz-test-data.

const precheckPrinterName = 'Precuentas A2 E2E';

// En la corrida completa esta es la sexta solicitud de emparejamiento desde 127.0.0.1 en pocos minutos (9G: Caja 2, 3 y
// 4 y Barra norte; A1: la tableta del PIN) y Go acepta 5 por origen cada 10 minutos (regla de seguridad de la 9G, que no
// se relaja en modo de pruebas): se espera a que vuelva a aceptarla, como lo haría una terminal real.
async function requestPairingCodeWhenAllowed(name: string) {
  const deadline = Date.now() + 11 * 60_000;
  for (;;) {
    try {
      return await requestPairingCode(name, 'Android 15', 'android');
    } catch (error) {
      const limited = error instanceof PairingRequestError && error.code === 'PAIRING_RATE_LIMITED';
      if (!limited || Date.now() > deadline) {
        throw error;
      }
      await new Promise((resolve) => setTimeout(resolve, 20_000));
    }
  }
}

async function login(page: Page) {
  await page.getByLabel('Usuario', { exact: true }).fill('admin');
  await page.getByLabel('Contraseña', { exact: true }).fill('admin');
  await page.getByRole('button', { name: 'Entrar', exact: true }).click();
}

test('Terminales: impresora de precuentas y avisos de platillos listos de un teléfono de meseros (etapa A2, partes C y D)', async ({
  page,
}) => {
  // Hasta 11 minutos de espera por el límite de solicitudes, más la prueba.
  test.setTimeout(15 * 60_000);
  // installDriver prepara la base (asistente inicial): sin eso Go no acepta emparejamientos (409 SETUP_REQUIRED).
  const driver = await installDriver(page);
  // Un teléfono de meseros emparejado y la impresora de precuentas de aceptación. El código se pide antes de entrar,
  // porque puede haber que esperar al límite de solicitudes (y una solicitud vence a los 5 minutos).
  const pairing = await requestPairingCodeWhenAllowed('Tableta de precuentas A2');

  await page.goto('/');
  await login(page);
  await expect(topBar(page)).toBeVisible();

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
  const homes = page.getByRole('group', { name: 'Pantalla inicial de la terminal' });
  await expect(homes.getByRole('button')).toHaveText(['Mesas', 'Para llevar', 'Cocina', 'Caja']);
  await expect(page.getByRole('combobox', { name: 'Caja física' })).toBeVisible();
  await page.getByRole('button', { name: 'Cerrar editor', exact: true }).click();

  // 6. Etapa A3 (§5): en la tabla la caja del teléfono es un guion y ninguna fila muestra el identificador.
  await expect(device.locator('td').nth(2)).toHaveText('—');
  await expect(page.locator('tbody')).not.toContainText('Código');
  await expect(page.locator('tbody')).not.toContainText(phoneID);

  // 7. El teléfono elige su pantalla inicial solo entre Mesas y Para llevar, sin caja física ni teclado, y Go la guarda.
  await device.getByRole('button', { name: 'Editar', exact: true }).click();
  await expect(homes.getByRole('button')).toHaveText(['Mesas', 'Para llevar']);
  await expect(page.getByRole('combobox', { name: 'Caja física' })).toHaveCount(0);
  await expect(page.getByRole('combobox', { name: 'Teclado virtual' })).toHaveCount(0);
  await homes.getByRole('button', { name: 'Para llevar', exact: true }).click();
  await page.getByRole('button', { name: 'Guardar cambios', exact: true }).click();
  // El aviso «Terminal actualizada.» del guardado anterior puede seguir en pantalla: se espera a que Go lo tenga.
  await expect
    .poll(async () => {
      const saved = await driver.request('GET', `devices/${phoneID}/configuration`);
      return (saved.body.device as { home_screen?: string }).home_screen;
    })
    .toBe('takeaway');
  // Go rechaza «Cocina» en un teléfono aunque la pida otro cliente, con un mensaje claro.
  const rejected = await driver.request('PATCH', `devices/${phoneID}`, {
    name: 'Tableta de precuentas A2',
    register_id: null,
    keyboard_mode: 'auto',
    home_screen: 'kitchen',
  });
  expect(rejected.status).toBe(400);
  expect(JSON.stringify(rejected.body)).toContain(
    'En un teléfono o tableta de meseros la pantalla inicial es Mesas o Para llevar.',
  );
  await page.getByRole('button', { name: 'Cerrar editor', exact: true }).click();
  await expect(device.locator('td').nth(3)).toHaveText('Para llevar', { timeout: 10000 });

  // 8. «Ver configuración» dice el identificador y no lo que no le aplica.
  await device.getByRole('button', { name: 'Ver configuración', exact: true }).click();
  await expect(panel).toContainText(`Identificador${phoneID}`);
  await expect(panel).toContainText('Pantalla inicialPara llevar');
  await expect(panel).not.toContainText('Caja física');
  await expect(panel).not.toContainText('Estaciones que muestra en Cocina');
  await panel.getByRole('button', { name: 'Cerrar configuración', exact: true }).click();
});
