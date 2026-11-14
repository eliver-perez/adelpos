import { expect, test } from '@playwright/test';
import type { Page } from '@playwright/test';
import { installDriver } from './native-driver';
import { openAdministration, topBar } from './navigation';
import { notice } from './notices';
import { claimPairing, pairingState, requestPairingCode } from './pairing';

// Etapa 9G, fase B (§3.2–§3.4) con Go y PostgreSQL reales: emparejamiento con código y administración de
// terminales. La terminal nueva se simula por la API de Go (e2e/pairing.ts, lo mismo que hace Rust); la
// aprobación se hace en Sistema › Terminales escribiendo el código, como en la computadora principal
// (el puente llega a Go por loopback, la escucha local). La segunda prueba es la propia terminal nueva:
// «Buscar el servidor» con la dirección escrita a mano, el código en grande y el acceso normal al
// emparejarse; revocada, se empareja de nuevo. El servidor temporal no tiene escucha de red: el cifrado y
// la fijación del certificado se prueban en Go y en Rust.

const serverURL = process.env.ADEL_BROWSER_SERVER_URL ?? 'http://127.0.0.1:8183';

async function login(page: Page, username = 'admin', password = 'admin') {
  await page.getByLabel('Usuario', { exact: true }).fill(username);
  await page.getByLabel('Contraseña', { exact: true }).fill(password);
  await page.getByRole('button', { name: 'Entrar', exact: true }).click();
}

// Petición de la terminal emparejada (sus credenciales) a Go.
async function asTerminal(
  device: { id: string; token: string; session?: string },
  method: string,
  route: string,
  body?: unknown,
) {
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    'X-Device-ID': device.id,
    'X-Device-Token': device.token,
  };
  if (device.session) {
    headers.Authorization = `Bearer ${device.session}`;
  }
  const response = await fetch(`${serverURL}/api/v1/${route}`, {
    method,
    headers,
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  return { status: response.status, body: (await response.json()) as Record<string, unknown> };
}

function wrongCode(code: string) {
  return code === '000000' ? '111111' : '000000';
}

test('Terminales: aprobación con código, renombrar, configuración, revocación inmediata y sesiones por tipo (etapa 9G, fase B)', async ({
  page,
}) => {
  const driver = await installDriver(page);
  await page.goto('/');
  await login(page);
  await expect(topBar(page)).toBeVisible();
  await openAdministration(page, 'Terminales');
  const panel = page.getByRole('region', { name: 'Terminales nuevas' });
  await expect(panel).toContainText('No hay terminales esperando aprobación.');
  // El servidor temporal no abre la escucha de la red local.
  await expect(panel).toContainText('Esta computadora todavía no recibe terminales de la red.');

  // 1. Una terminal nueva pide su código: aparece con su nombre, sistema y hora.
  const caja = await requestPairingCode('Caja 2');
  const item = panel.getByRole('listitem').filter({ hasText: 'Caja 2' });
  await expect(item).toContainText('Escritorio · Windows · versión 0.10.0', { timeout: 10000 });
  await expect(item).toContainText('pidió su código a las');
  const field = item.getByRole('textbox', { name: /^Código que muestra Caja 2/ });

  // 2. Un código incorrecto no la aprueba y cuenta un intento.
  await field.fill(wrongCode(caja.code));
  await item.getByRole('button', { name: 'Aprobar', exact: true }).click();
  await expect(notice(page, /no coincide con el de la terminal/)).toBeVisible();
  await expect(item).toContainText('Quedan 4 intentos antes de rechazarla.');
  expect(await pairingState(caja)).toBe('pending');

  // 3. El código correcto (escrito como lo muestra la terminal) la aprueba; la credencial se entrega
  //    una sola vez.
  await field.fill(`${caja.code.slice(0, 3)} ${caja.code.slice(3)}`);
  await item.getByRole('button', { name: 'Aprobar', exact: true }).click();
  await expect(
    notice(page, 'Terminal Caja 2 aprobada. Se conectará en unos segundos.'),
  ).toBeVisible();
  await expect(panel).toContainText('No hay terminales esperando aprobación.');
  expect(await pairingState(caja)).toBe('approved');
  const claimed = await claimPairing(caja);
  expect(claimed.status).toBe(200);
  const device = { id: String(claimed.body.device_id), token: String(claimed.body.device_token) };
  expect(device.token).toHaveLength(43);
  const again = await claimPairing(caja);
  expect(again.status).toBe(409);
  expect((again.body.error as { code: string }).code).toBe('PAIRING_ALREADY_CLAIMED');

  // 4. Un usuario inicia sesión en la terminal emparejada: la lista muestra tipo, sistema, usuario
  //    actual y «En línea»; el conteo de sesiones por tipo lo incluye (no limita nada).
  const role = await driver.request('POST', 'roles', {
    name: 'Caja emparejada 9G',
    permissions: ['events.read', 'devices.read'],
  });
  expect(role.status).toBe(200);
  const user = await driver.request('POST', 'users', {
    username: 'caja.dos9g',
    display_name: 'Cajera de la barra',
    role_id: role.body.id,
    active: true,
    password: 'clave-temporal-9g',
  });
  expect(user.status).toBe(200);
  const login9g = await asTerminal(device, 'POST', 'auth/login', {
    username: 'caja.dos9g',
    password: 'clave-temporal-9g',
  });
  expect(login9g.status).toBe(200);
  const paired = { ...device, session: String(login9g.body.session_token) };
  const row = page.locator('tbody tr').filter({ hasText: 'Caja 2' });
  await expect(row).toContainText('Escritorio');
  await expect(row).toContainText('Windows');
  await expect(row).toContainText('Cajera de la barra', { timeout: 10000 });
  await expect(row).toContainText('En línea');
  const summary = page.getByRole('region', { name: 'Sesiones activas por tipo de terminal' });
  await expect(summary).toContainText('Escritorio');
  await expect(summary).toContainText('Comandero Android0');
  await expect(summary).toContainText('Solo se cuentan; todavía no se limita ninguna.');

  // 5. Renombrar y ver su configuración (pantalla de inicio, estaciones y el hueco de las impresoras).
  await row.getByRole('button', { name: 'Editar', exact: true }).click();
  await page.getByRole('textbox', { name: 'Nombre' }).fill('Caja de la barra');
  await page.getByRole('button', { name: 'Guardar cambios', exact: true }).click();
  await expect(notice(page, 'Terminal actualizada.')).toBeVisible();
  await page.getByRole('button', { name: 'Cerrar editor', exact: true }).click();
  const renamed = page.locator('tbody tr').filter({ hasText: 'Caja de la barra' });
  await renamed.getByRole('button', { name: 'Ver configuración', exact: true }).click();
  const configuration = page.getByRole('region', { name: 'Configuración de Caja de la barra' });
  await expect(configuration).toContainText('Windows · versión 0.10.0');
  await expect(configuration).toContainText('Mesas');
  await expect(configuration).toContainText('Todas');
  await expect(configuration).toContainText('Sin impresoras propias');
  await configuration.getByRole('button', { name: 'Cerrar configuración', exact: true }).click();

  // 6. Revocar: la terminal pierde el flujo de eventos en segundos y cada petición se rechaza.
  const controller = new AbortController();
  const stream = await fetch(`${serverURL}/api/v1/events`, {
    headers: {
      'X-Device-ID': paired.id,
      'X-Device-Token': paired.token,
      Authorization: `Bearer ${paired.session}`,
    },
    signal: controller.signal,
  });
  expect(stream.status).toBe(200);
  const reader = stream.body!.getReader();
  const decoder = new TextDecoder();
  let received = '';
  const finished = (async () => {
    for (;;) {
      const part = await reader.read();
      if (part.done) {
        return;
      }
      received += decoder.decode(part.value, { stream: true });
    }
  })();
  await renamed.getByRole('button', { name: 'Revocar', exact: true }).click();
  const dialog = page.getByRole('alertdialog', { name: 'Revocar Caja de la barra' });
  await expect(dialog).toContainText('habrá que emparejarla de nuevo');
  const revokedAt = Date.now();
  await dialog.getByRole('button', { name: 'Confirmar revocación', exact: true }).click();
  await expect(notice(page, 'Terminal Caja de la barra revocada.')).toBeVisible();
  await Promise.race([
    finished,
    new Promise((_, reject) => setTimeout(() => reject(new Error('el flujo no se cortó')), 8000)),
  ]);
  controller.abort();
  expect(received).toContain('DEVICE_REVOKED');
  expect(Date.now() - revokedAt).toBeLessThan(8000);
  const rejected = await asTerminal(paired, 'GET', 'auth/me');
  expect(rejected.status).toBe(401);
  expect((rejected.body.error as { code: string }).code).toBe('DEVICE_REVOKED');
  await expect(renamed).toContainText('Revocada');

  // 7. Rechazar otra solicitud.
  const other = await requestPairingCode('Caja 3');
  const otherItem = panel.getByRole('listitem').filter({ hasText: 'Caja 3' });
  await otherItem.getByRole('button', { name: 'Rechazar', exact: true }).click();
  await page
    .getByRole('alertdialog', { name: 'Rechazar Caja 3' })
    .getByRole('button', { name: 'Rechazar terminal', exact: true })
    .click();
  await expect(notice(page, 'Se rechazó la terminal Caja 3.')).toBeVisible();
  expect(await pairingState(other)).toBe('rejected');

  // 8. Todo queda en el Registro de movimientos.
  await openAdministration(page, 'Registro de movimientos');
  for (const label of [
    'Terminal aprobada',
    'Terminal emparejada',
    'Terminal revocada',
    'Terminal rechazada',
  ]) {
    await expect(page.getByText(label, { exact: true }).first()).toBeVisible();
  }
});

test('Terminal nueva: dirección escrita a mano, código en grande, acceso al emparejarse y emparejar de nuevo al revocarla (etapa 9G, fase B)', async ({
  page,
}) => {
  const driver = await installDriver(page);
  driver.setInstallation({ chosen: true, mode: 'terminal-only' });
  await page.goto('/');
  await expect(page.getByRole('heading', { name: 'Buscar el servidor' })).toBeVisible();
  const address = page.getByRole('textbox', { name: 'Dirección de la computadora principal' });
  const name = page.getByRole('textbox', { name: 'Nombre de esta terminal' });
  await expect(name).toHaveValue('Caja 2');

  // 1. Una dirección fuera de la red local no se acepta.
  await address.fill('8.8.8.8');
  await page.getByRole('button', { name: 'Pedir código', exact: true }).click();
  await expect(page.getByText(/red local del restaurante \(por ejemplo/)).toBeVisible();

  // 2. Con la dirección de la computadora principal aparece el código en grande y el nombre.
  await address.fill('192.168.1.20');
  await name.fill('Caja 4');
  await page.getByRole('button', { name: 'Pedir código', exact: true }).click();
  const code = page.locator('.pairing-code');
  await expect(code).toHaveText(/^\d{3} \d{3}$/);
  await expect(page.locator('.pairing-server')).toContainText('Computadora principal de');
  await expect(page.getByText('Caja 4', { exact: true })).toBeVisible();
  await expect(page.locator('.find-server-status')).toContainText(
    'Esperando la aprobación de la computadora principal',
  );
  await expect(page.getByRole('timer')).toContainText('El código vence en');

  // 3. En la computadora principal se escribe ese código (por la escucha local).
  const digits = (await code.textContent())!.replace(/\D/g, '');
  const admin = await driver.request('POST', 'auth/login', {
    username: 'admin',
    password: 'admin',
  });
  expect(admin.status).toBe(200);
  const pending = await driver.request('GET', 'devices/pairing-requests');
  const entry = (pending.body.requests as { id: string; name: string }[]).find(
    (request) => request.name === 'Caja 4',
  );
  expect(entry).toBeTruthy();
  const approved = await driver.request('POST', `devices/pairing-requests/${entry!.id}/approve`, {
    code: digits,
  });
  expect(approved.status).toBe(200);

  // 4. La terminal recibe su credencial, se reinicia y muestra el acceso normal.
  await expect(page.locator('.find-server-status')).toContainText('Terminal emparejada', {
    timeout: 10000,
  });
  await expect(page.getByRole('heading', { name: 'Inicia sesión' })).toBeVisible({
    timeout: 15000,
  });
  await login(page);
  await expect(topBar(page)).toBeVisible();
  await openAdministration(page, 'Terminales');
  const row = page.locator('tbody tr').filter({ hasText: 'Caja 4' });
  await expect(row).toContainText('macOS');
  await expect(row).toContainText('En línea');

  // 5. Revocada (aquí, desde la propia terminal), pide emparejarse de nuevo y vuelve a «Buscar el
  //    servidor».
  await row.getByRole('button', { name: 'Revocar', exact: true }).click();
  await page
    .getByRole('alertdialog', { name: 'Revocar Caja 4' })
    .getByRole('button', { name: 'Confirmar revocación', exact: true })
    .click();
  await expect(
    page.getByRole('heading', { name: 'Esta terminal necesita emparejarse de nuevo' }),
  ).toBeVisible({ timeout: 15000 });
  await page.getByRole('button', { name: 'Emparejar de nuevo', exact: true }).click();
  await page
    .getByRole('alertdialog', { name: 'Emparejar de nuevo' })
    .getByRole('button', { name: 'Emparejar de nuevo', exact: true })
    .click();
  await expect(page.getByRole('heading', { name: 'Buscar el servidor' })).toBeVisible({
    timeout: 15000,
  });
});
