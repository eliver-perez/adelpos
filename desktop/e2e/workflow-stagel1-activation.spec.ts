import { readFileSync } from 'node:fs';
import { expect, test } from '@playwright/test';
import {
  installDriver,
  issueTestLicense,
  licenseVectors,
  simulatedLicenseServer,
  unlicensedServer,
} from './native-driver';
import { call, dismissNotices, lightTheme, openLicense, shot } from './license-flow';
import { notice } from './notices';
import { topBar } from './navigation';

// Etapa L1, Fase E (§7 de la especificación), con Go y PostgreSQL reales: la activación del primer arranque y
// Configuración › Licencia, contra un Go temporal **sin activar** (ADEL_BROWSER_UNLICENSED_SERVER_URL) y el servidor de
// licencias simulado de la Fase B (license-test server, en loopback). Nunca se llama a un servidor de licencias real:
// los códigos son los inventados del simulador (su primer grupo decide el comportamiento) y los .lic los firma
// license-test con las llaves de prueba de vectors.json.
//
// 1. Primer arranque: el formato del código al escribirlo, un código que no existe, «Activar sin internet» (guardar la
//    solicitud .licreq con el diálogo simulado e importar el .lic; uno de otra computadora se rechaza por su motivo).
// 2. Configuración › Licencia: estado, plan, cliente, vencimiento, entorno y cupos; una licencia revocada; activar en
//    línea; «Buscar actualización»; «Liberar esta licencia» con su confirmación; una transferencia pendiente
//    («Comunícate con soporte»); «Transferir a esta computadora» con su confirmación; y la solicitud de renovación.

const server = unlicensedServer();
const simulator = simulatedLicenseServer();

function decodedPayload(content: string) {
  const request = JSON.parse(content) as { schema_version: string; payload_b64u: string };
  expect(request.schema_version).toBe('1.0');
  return JSON.parse(Buffer.from(request.payload_b64u, 'base64url').toString('utf8')) as Record<
    string,
    unknown
  >;
}

test.describe.serial('activación (Go temporal sin activar)', () => {
  test.skip(
    !server || !simulator,
    'Sin ADEL_BROWSER_UNLICENSED_SERVER_URL ni el servidor de licencias simulado: la activación necesita una base sin activar.',
  );

  test('primer arranque: formato, código inexistente y activación sin internet', async ({
    page,
  }, testInfo) => {
    test.setTimeout(180000);
    const driver = await installDriver(page, { server: server!, license: false, catalog: false });
    await call(driver, 'POST', 'auth/login', { username: 'admin', password: 'admin' });
    await lightTheme(driver);
    const status = (await call(driver, 'GET', 'license/status')) as { state: string };
    expect(status.state).toBe('not_activated');
    await page.goto('/');
    await expect(page.getByRole('heading', { name: 'Activa AD-EL POS' })).toBeVisible();
    await expect(
      page.getByText(/Este sistema no tiene una licencia activada/).first(),
    ).toBeVisible();
    await shot(page, testInfo, 'activacion-primer-arranque');

    // El formato se valida al escribir y el código se ordena solo.
    const field = page.getByLabel('Código de la licencia');
    const activate = page.getByRole('button', { name: 'Activar', exact: true });
    await field.fill('zzz2');
    await expect(field).toHaveValue('ZZZ-2');
    await expect(page.getByText('Faltan 11 caracteres.')).toBeVisible();
    await expect(activate).toBeDisabled();
    await field.fill('ZZZ-2222-2222-22I2');
    await expect(page.getByText(/El código no usa I/)).toBeVisible();
    await field.fill('zzz222222222222');
    await expect(field).toHaveValue('ZZZ-2222-2222-2222');
    await expect(page.getByText('El código tiene el formato correcto.')).toBeVisible();
    await shot(page, testInfo, 'activacion-codigo-valido');
    // Un código que el servidor de licencias no conoce: el texto sale del código de error.
    await activate.click();
    await expect(
      notice(page, 'No existe una licencia con ese código. Revisa que esté bien escrito.'),
    ).toBeVisible({ timeout: 30000 });

    // Sin internet: la solicitud lleva el código escrito y la firma de esta instalación.
    await page.getByRole('button', { name: /Activar sin internet/ }).click();
    driver.license.nextSave('save');
    await page.getByRole('button', { name: /Guardar la solicitud/ }).click();
    await expect(
      notice(page, /^Solicitud guardada: solicitud-activar-.*\.licreq\.$/),
    ).toBeVisible();
    expect(driver.license.requests).toHaveLength(1);
    const payload = decodedPayload(driver.license.requests[0]!.content);
    expect(payload.action).toBe('activate');
    expect(payload.license_key).toBe('ZZZ-2222-2222-2222');
    expect(typeof payload.server_id).toBe('string');
    await shot(page, testInfo, 'activacion-sin-internet');

    // Un archivo que no es de licencia y uno de otra computadora no se instalan.
    driver.license.nextLicenseFile(driver.license.writeFile('notas.txt', 'no es una licencia'));
    await page.getByRole('button', { name: /Importar archivo de licencia/ }).click();
    await expect(
      notice(page, /Elige el archivo de licencia \(un archivo terminado en \.lic\)/),
    ).toBeVisible();
    // La de otra computadora: firmada para una instalación de vectors.json (sin la carpeta identity de esta).
    const vectors = JSON.parse(readFileSync(licenseVectors, 'utf8')) as {
      installations: { server_id: string; public_key: string }[];
    };
    const other = vectors.installations[0]!;
    driver.license.nextLicenseFile(
      driver.license.writeFile(
        'de-otra-computadora.lic',
        issueTestLicense('', ['--server-id', other.server_id, '--public-key', other.public_key]),
      ),
    );
    await page.getByRole('button', { name: /Importar archivo de licencia/ }).click();
    await expect(
      notice(
        page,
        'El archivo de licencia es de otra computadora y no se instaló. Tu licencia actual no cambió.',
      ),
    ).toBeVisible();

    // El .lic de esta instalación: queda activada y la pantalla del primer arranque se va.
    const license = driver.license.issue([
      '--type',
      'subscription',
      '--expires-at',
      '30d',
      '--plan',
      'elemental',
      '--licensee',
      'Restaurante sin internet',
    ]);
    driver.license.nextLicenseFile(driver.license.writeFile('licencia-prueba.lic', license));
    await page.getByRole('button', { name: /Importar archivo de licencia/ }).click();
    await expect(notice(page, 'Archivo de licencia importado.')).toBeVisible();
    await expect(page.getByRole('heading', { name: 'Activa AD-EL POS' })).toHaveCount(0, {
      timeout: 15000,
    });
    await expect(topBar(page)).toBeVisible();
    expect(((await call(driver, 'GET', 'license/status')) as { state: string }).state).toBe(
      'active',
    );
  });

  test('Configuración › Licencia: estado, activar en línea, liberar, transferir y renovar sin internet', async ({
    page,
  }, testInfo) => {
    test.setTimeout(240000);
    const driver = await installDriver(page, { server: server!, license: false, catalog: false });
    await call(driver, 'POST', 'auth/login', { username: 'admin', password: 'admin' });
    await page.goto('/');
    await expect(topBar(page)).toBeVisible();
    await openLicense(page);
    const details = page.getByRole('region', { name: 'Estado de la licencia' });
    await expect(details).toContainText('Vigente');
    await expect(details).toContainText('Restaurante sin internet');
    await expect(details).toContainText('Suscripción mensual');
    await expect(details).toContainText('Servidor de licencias de pruebas');
    const seats = details.getByRole('table', { name: 'Cupos de la licencia' });
    await expect(seats.getByRole('row', { name: /Comanderos/ })).toContainText('3');
    // Ningún identificador a la vista (la licencia, la activación ni el servidor).
    const status = (await call(driver, 'GET', 'license/status')) as {
      server_id: string;
      license: { license_id: string; activation_id: string };
    };
    for (const identifier of [
      status.server_id,
      status.license.license_id,
      status.license.activation_id,
    ]) {
      await expect(page.locator('main')).not.toContainText(identifier);
    }
    await shot(page, testInfo, 'licencia-configuracion');

    // Una licencia revocada (otra licencia para esta instalación la reemplaza): aviso fijo y la activación.
    await driver.license.install([
      '--status',
      'revoked',
      '--reason',
      'transferred',
      '--licensee',
      'Restaurante sin internet',
    ]);
    const fixed = page.getByRole('alert', { name: 'Aviso de la licencia' });
    await expect(fixed).toContainText('La licencia se transfirió a otra computadora', {
      timeout: 15000,
    });
    await expect(details).toContainText('Revocada');
    await shot(page, testInfo, 'licencia-revocada');

    // Activar en línea (Perpetua del Plan elemental del simulador).
    const activation = page.getByRole('region', { name: 'Activar la licencia' });
    await activation.getByLabel('Código de la licencia').fill('PRP-2222-2222-2222');
    await activation.getByRole('button', { name: 'Activar', exact: true }).click();
    await expect(notice(page, 'Licencia activada.')).toBeVisible({ timeout: 60000 });
    await expect(details).toContainText('Vigente');
    await expect(details).toContainText('Perpetua');
    await expect(details).toContainText('Plan elemental');
    await expect(fixed).toHaveCount(0, { timeout: 15000 });

    // «Buscar actualización».
    await page.getByRole('button', { name: /Buscar actualización/ }).click();
    await expect(
      notice(page, 'Licencia consultada: tiene la información más reciente.'),
    ).toBeVisible({ timeout: 60000 });

    // «Liberar esta licencia», con su confirmación en rojo; cancelar no hace nada.
    const release = page.getByRole('button', { name: 'Liberar esta licencia', exact: true });
    await expect(release).toHaveClass(/danger-button/);
    await release.click();
    let dialog = page.getByRole('alertdialog', { name: 'Liberar esta licencia' });
    await expect(dialog).toContainText('queda libre para activarla en otra computadora');
    await shot(page, testInfo, 'licencia-liberar-confirmacion', { fullPage: false });
    await dialog.getByRole('button', { name: 'Conservar la licencia', exact: true }).click();
    await expect(details).toContainText('Vigente');
    await release.click();
    dialog = page.getByRole('alertdialog', { name: 'Liberar esta licencia' });
    await dialog.getByRole('button', { name: 'Liberar esta licencia', exact: true }).click();
    await expect(notice(page, 'La licencia se liberó de esta computadora.')).toBeVisible({
      timeout: 60000,
    });
    await expect(details).toContainText('Revocada');
    await expect(fixed).toContainText('La licencia se liberó en esta computadora', {
      timeout: 15000,
    });

    // Activa en otra computadora con una transferencia pendiente: «Comunícate con soporte», sin transferir.
    await activation.getByLabel('Código de la licencia').fill('ACN-2222-2222-2222');
    await activation.getByRole('button', { name: 'Activar', exact: true }).click();
    await expect(
      notice(page, /tiene una transferencia pendiente\. Comunícate con soporte\./),
    ).toBeVisible({ timeout: 60000 });
    await expect(page.getByRole('region', { name: 'Transferir la licencia' })).toHaveCount(0);

    // Activa en otra computadora con la transferencia disponible: el botón aparece y pide confirmación. (Sin el aviso de
    // error anterior, que no se va solo, en la captura.)
    await dismissNotices(page);
    await activation.getByLabel('Código de la licencia').fill('ACT-2222-2222-2222');
    await activation.getByRole('button', { name: 'Activar', exact: true }).click();
    const offer = page.getByRole('region', { name: 'Transferir la licencia' });
    await expect(offer).toBeVisible({ timeout: 60000 });
    await shot(page, testInfo, 'licencia-transferir');
    await offer.getByRole('button', { name: /Transferir a esta computadora/ }).click();
    dialog = page.getByRole('alertdialog', { name: 'Transferir la licencia a esta computadora' });
    await expect(dialog).toContainText('esa computadora deja de poder capturar pedidos');
    await dialog
      .getByRole('button', { name: 'Transferir a esta computadora', exact: true })
      .click();
    await expect(notice(page, 'Licencia transferida a esta computadora.')).toBeVisible({
      timeout: 60000,
    });
    await expect(details).toContainText('Vigente');

    // Solicitud sin internet de renovación (con la activación vigente).
    const offline = page.getByRole('group', { name: 'Solicitud sin internet' });
    await expect(offline.getByRole('button', { name: 'Activación', exact: true })).toBeDisabled();
    await offline.getByRole('button', { name: 'Renovación', exact: true }).click();
    driver.license.nextSave('save');
    await offline.getByRole('button', { name: /Guardar la solicitud/ }).click();
    await expect(
      notice(page, /^Solicitud guardada: solicitud-renovar-.*\.licreq\.$/),
    ).toBeVisible();
    const renew = driver.license.requests.at(-1)!;
    expect(renew.action).toBe('renew');
    expect(decodedPayload(renew.content).action).toBe('renew');
    await shot(page, testInfo, 'licencia-administrar');
  });
});
