import { expect, test } from '@playwright/test';
import type { Page } from '@playwright/test';
import { installDriver } from './native-driver';
import { openAdministration, topBar } from './navigation';
import { notice } from './notices';
import { chooseOption } from './select';
import { claimPairing, requestPairingCode } from './pairing';

// Etapa A1, fase B (§3.1 y §3.2) con Go y PostgreSQL reales: el PIN de los meseros en el escritorio. En Personal ›
// Usuarios se asigna el PIN con su nombre corto y su color (un PIN trivial se rechaza con el mensaje de Go); el
// bloqueo por 5 intentos fallidos se provoca por la API con un teléfono de meseros emparejado (e2e/pairing.ts, como lo
// haría la aplicación Android) y se quita con «Desbloquear»; en Sistema › Terminales se elige el bloqueo por
// inactividad del teléfono; y el administrador define su propio PIN en «Mi PIN». Se ordena después de las
// especificaciones de la 9H y antes de workflow-zz-test-data.

const serverURL = process.env.ADEL_BROWSER_SERVER_URL ?? 'http://127.0.0.1:8183';

async function login(page: Page) {
  await page.getByLabel('Usuario', { exact: true }).fill('admin');
  await page.getByLabel('Contraseña', { exact: true }).fill('admin');
  await page.getByRole('button', { name: 'Entrar', exact: true }).click();
}

// Petición del teléfono de meseros a Go: su credencial y su versión, sin sesión (la entrada con PIN).
async function asWaiterPhone(
  phone: { id: string; token: string },
  method: string,
  route: string,
  body?: unknown,
) {
  const response = await fetch(`${serverURL}/api/v1/${route}`, {
    method,
    headers: {
      'Content-Type': 'application/json',
      'X-Device-ID': phone.id,
      'X-Device-Token': phone.token,
      'X-Client-Version': '0.1.0',
      'X-Client-Kind': 'android',
    },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  return { status: response.status, body: (await response.json()) as Record<string, unknown> };
}

function errorCode(reply: { body: Record<string, unknown> }) {
  return (reply.body.error as { code?: string } | undefined)?.code;
}

test('PIN de los meseros: asignar, rechazar uno trivial, bloquear y desbloquear, «Mi PIN» y bloqueo por inactividad (etapa A1, fase B)', async ({
  page,
}) => {
  const driver = await installDriver(page);
  await page.goto('/');
  await login(page);
  await expect(topBar(page)).toBeVisible();

  // Una mesera con el rol Mesero (que ya trae waiter_devices.use) y un teléfono de meseros emparejado.
  const roles = await driver.request('GET', 'roles');
  const waiterRole = (roles.body as unknown as { id: string; code: string | null }[]).find(
    (role) => role.code === 'waiter',
  );
  expect(waiterRole).toBeTruthy();
  const created = await driver.request('POST', 'users', {
    username: 'andrea.a1',
    display_name: 'Andrea Meseros A1',
    role_id: waiterRole!.id,
    active: true,
    password: 'clave-temporal-a1',
  });
  expect(created.status).toBe(200);
  const waiterID = String(created.body.id);
  const pairing = await requestPairingCode('Tableta de meseros A1', 'Android 15', 'android');
  const approved = await driver.request(
    'POST',
    `devices/pairing-requests/${pairing.requestID}/approve`,
    {
      code: pairing.code,
    },
  );
  expect(approved.status).toBe(200);
  const claimed = await claimPairing(pairing);
  expect(claimed.status).toBe(200);
  const phone = { id: String(claimed.body.device_id), token: String(claimed.body.device_token) };

  // 1. Asignar el PIN: uno trivial se rechaza con el mensaje de Go; el bueno se guarda con nombre corto y color.
  await openAdministration(page, 'Usuarios');
  // En la fila propia no se ofrece el PIN: el del administrador va en «Mi PIN» (revisión R3, B6).
  const own = page.locator('tbody tr').filter({ has: page.getByText('admin', { exact: true }) });
  await expect(own.getByRole('button', { name: 'Administrar', exact: true })).toBeVisible();
  await expect(own.getByRole('button', { name: /^(Asignar|Cambiar) PIN$/ })).toHaveCount(0);
  const row = page.locator('tbody tr').filter({ hasText: 'andrea.a1' });
  await expect(row).toContainText('Sin PIN');
  await row.getByRole('button', { name: 'Asignar PIN', exact: true }).click();
  const panel = page.getByRole('region', { name: 'Asignar PIN a Andrea Meseros A1' });
  await panel.getByLabel('PIN', { exact: true }).fill('1234');
  await panel.getByLabel('Escríbelo otra vez', { exact: true }).fill('1234');
  await panel.getByRole('button', { name: 'Asignar PIN', exact: true }).click();
  await expect(notice(page, /muy fácil de adivinar/)).toBeVisible();
  await panel.getByLabel('PIN', { exact: true }).fill('4829');
  await panel.getByLabel('Escríbelo otra vez', { exact: true }).fill('4829');
  await panel.getByLabel(/^Nombre corto/).fill('Andrea');
  await panel.getByRole('button', { name: /^Color del distintivo/ }).click();
  await panel
    .getByRole('group', { name: 'Paleta de colores' })
    .getByRole('button', { name: 'Celeste' })
    .click();
  await panel.getByRole('button', { name: 'Asignar PIN', exact: true }).click();
  await expect(notice(page, 'PIN guardado.')).toBeVisible();
  await expect(row).toContainText('Con PIN · Andrea');
  const listed = await asWaiterPhone(phone, 'GET', 'auth/pin-users');
  expect(listed.status).toBe(200);
  const waiter = (listed.body.users as Record<string, unknown>[]).find(
    (user) => user.id === waiterID,
  );
  expect(waiter).toMatchObject({
    name: 'Andrea',
    initial: 'A',
    color: '#3591d4',
    pin_version: 1,
    locked_until: null,
    requires_unlock: false,
  });

  // 2. Cinco fallos desde el teléfono bloquean el PIN; Usuarios lo muestra y «Desbloquear» lo quita.
  for (let attempt = 1; attempt <= 4; attempt++) {
    const failed = await asWaiterPhone(phone, 'POST', 'auth/pin-login', {
      user_id: waiterID,
      pin: '4828',
    });
    expect(failed.status).toBe(401);
    expect(errorCode(failed)).toBe('PIN_INVALID');
  }
  const locked = await asWaiterPhone(phone, 'POST', 'auth/pin-login', {
    user_id: waiterID,
    pin: '4828',
  });
  expect(locked.status).toBe(429);
  expect(errorCode(locked)).toBe('PIN_LOCKED');
  // El primer bloqueo es por tiempo (A1-D16: el largo llega con el tercero seguido en 24 horas).
  const lockDetails = (locked.body.error as { details?: Record<string, unknown> }).details;
  expect(lockDetails?.requires_unlock).toBe(false);
  expect(typeof lockDetails?.locked_until).toBe('string');
  await page.reload();
  await expect(topBar(page)).toBeVisible();
  await openAdministration(page, 'Usuarios');
  await expect(row).toContainText(/PIN bloqueado hasta las \d{2}:\d{2}/);
  await row.getByRole('button', { name: 'Desbloquear', exact: true }).click();
  await expect(notice(page, 'PIN de Andrea Meseros A1 desbloqueado.')).toBeVisible();
  await expect(row).toContainText('Con PIN · Andrea');
  const entered = await asWaiterPhone(phone, 'POST', 'auth/pin-login', {
    user_id: waiterID,
    pin: '4829',
  });
  expect(entered.status).toBe(200);
  expect(String(entered.body.offline_credential)).toHaveLength(43);

  // 3. Bloqueo por inactividad del teléfono en Sistema › Terminales (2 minutos por omisión).
  await openAdministration(page, 'Terminales');
  const device = page.locator('tbody tr').filter({ hasText: 'Tableta de meseros A1' });
  await expect(device).toContainText('Teléfono o tableta');
  await device.getByRole('button', { name: 'Editar', exact: true }).click();
  const idle = page.getByRole('combobox', { name: 'Bloqueo por inactividad' });
  await expect(idle).toContainText('2 minutos (recomendado)');
  await chooseOption(idle, '5 minutos');
  await page.getByRole('button', { name: 'Guardar cambios', exact: true }).click();
  await expect(notice(page, 'Terminal actualizada.')).toBeVisible();
  await page.getByRole('button', { name: 'Cerrar editor', exact: true }).click();
  await device.getByRole('button', { name: 'Ver configuración', exact: true }).click();
  const configuration = page.getByRole('region', {
    name: 'Configuración de Tableta de meseros A1',
  });
  await expect(configuration).toContainText('5 minutos sin uso');
  const refreshed = await asWaiterPhone(phone, 'GET', 'auth/pin-users');
  expect((refreshed.body.device as { idle_lock_seconds: number }).idle_lock_seconds).toBe(300);

  // 4. «Mi PIN»: el administrador define el suyo con su contraseña actual.
  await page.getByRole('button', { name: /^Menú del usuario/ }).click();
  await page
    .getByRole('group', { name: 'Menú del usuario' })
    .getByRole('button', { name: 'Mi PIN', exact: true })
    .click();
  await expect(page.getByRole('heading', { name: 'Mi PIN', exact: true })).toBeVisible();
  await page.getByLabel('Contraseña actual', { exact: true }).fill('admin');
  await page.getByLabel('PIN', { exact: true }).fill('7305');
  await page.getByLabel('Escríbelo otra vez', { exact: true }).fill('7305');
  await page.getByRole('button', { name: 'Guardar mi PIN', exact: true }).click();
  await expect(notice(page, 'PIN guardado.')).toBeVisible();
  await expect(page.getByRole('button', { name: 'Cambiar mi PIN', exact: true })).toBeVisible();
});
