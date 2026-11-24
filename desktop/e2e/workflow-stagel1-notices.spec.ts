import { randomBytes } from 'node:crypto';
import { expect, test } from '@playwright/test';
import { installDriver } from './native-driver';
import {
  admin,
  call,
  lightTheme,
  login,
  logout,
  otherTerminal,
  restoreTestLicense,
  sharedServerURL,
  shot,
} from './license-flow';
import { topBar } from './navigation';

// Etapa L1, Fase E (§7 de la especificación), con Go y PostgreSQL reales: los avisos de la licencia en el Go temporal
// compartido. El estado lo pone una licencia de prueba firmada por license-test (POST license/import, con las llaves de
// vectors.json) desde otra terminal de administración, sin tocar la sesión de la página; nunca se llama a un servidor
// de licencias real. Al terminar queda la licencia de las demás
// especificaciones (restoreTestLicense), también si la prueba falla.
//
// 1. 7 días antes del vencimiento de una suscripción: un aviso diario para quien tiene «Administrar la licencia», que
//    se cierra con «Entendido» y no vuelve ese día (tampoco al volver a entrar), y un indicador discreto para los demás.
// 2. En tolerancia: un aviso diario para todos, con los días que quedan antes de que se bloquee la captura; cerrarlo es
//    por usuario.
// 3. Vencida: un aviso fijo (sin «Entendido») que explica qué se puede hacer y qué no.

test('avisos de la licencia: 7 días antes del vencimiento, tolerancia y vencida', async ({
  page,
}, testInfo) => {
  test.setTimeout(180000);
  await page.setViewportSize({ width: 1280, height: 800 });
  const driver = await installDriver(page);
  await call(driver, 'POST', 'auth/login', admin);
  await lightTheme(driver);
  // Un mesero sin «Administrar la licencia» (nombres únicos: la base es la compartida).
  const suffix = randomBytes(3).toString('hex');
  const role = (await call(driver, 'POST', 'roles', {
    name: `Mesero avisos ${suffix}`,
    permissions: [
      'accounts.read',
      'accounts.open',
      'restaurant.tables.read',
      'restaurant.areas.read',
      'orders.capture',
      'catalog.read',
      'catalog.prices.read',
      'events.read',
    ],
  })) as { id: string };
  const waiter = { username: `meseroavisos${suffix}`, password: 'clave-mesero-avisos' };
  await call(driver, 'POST', 'users', {
    username: waiter.username,
    display_name: 'Mesero de los avisos',
    role_id: role.id,
    active: true,
    password: waiter.password,
  });
  const control = await otherTerminal(driver, sharedServerURL, `Administración avisos ${suffix}`);
  const install = (args: string[]) =>
    control.installLicense(driver.license.identity, [
      ...args,
      '--licensee',
      'Restaurante de aceptación',
    ]);
  await call(driver, 'POST', 'auth/logout');
  const licenseNotice = page.getByRole('status', { name: 'Aviso de la licencia' });
  const fixedNotice = page.getByRole('alert', { name: 'Aviso de la licencia' });
  const understood = licenseNotice.getByRole('button', { name: 'Entendido', exact: true });

  try {
    // 1. La suscripción vence en 5 días.
    await install(['--type', 'subscription', '--expires-at', '5d']);
    await login(page, admin.username, admin.password);
    await expect(topBar(page)).toBeVisible();
    await expect(licenseNotice).toContainText(/La licencia vence en [45] días/);
    await expect(licenseNotice).toContainText(
      'Renuévala para que el sistema siga funcionando sin interrupciones.',
    );
    await expect(licenseNotice.getByRole('button', { name: /Ver la licencia/ })).toBeVisible();
    await shot(page, testInfo, 'aviso-vencimiento');
    await understood.click();
    await expect(licenseNotice).toHaveCount(0);
    // Una vez al día: al volver a entrar ese mismo día no aparece.
    await logout(page);
    await login(page, admin.username, admin.password);
    await expect(topBar(page)).toBeVisible();
    await expect(page.getByRole('button', { name: /^Menú del usuario/ })).toBeVisible();
    await page.waitForTimeout(1000);
    await expect(licenseNotice).toHaveCount(0);
    // Quien no administra la licencia ve solo el indicador discreto.
    await logout(page);
    await login(page, waiter.username, waiter.password);
    await expect(topBar(page)).toBeVisible();
    const indicator = page.locator('.license-indicator');
    await expect(indicator).toHaveText(/^La licencia vence en [45] días\.$/);
    await expect(licenseNotice).toHaveCount(0);
    await shot(page, testInfo, 'aviso-vencimiento-indicador');
    await logout(page);

    // 2. Venció hace 2 días: tolerancia (5 días en una suscripción), con los días que quedan, para todos.
    await install(['--type', 'subscription', '--expires-at', '-2d']);
    await login(page, waiter.username, waiter.password);
    await expect(topBar(page)).toBeVisible();
    await expect(licenseNotice).toContainText(
      'La licencia venció y está en sus días de tolerancia',
    );
    await expect(licenseNotice).toContainText(
      /Quedan? \d+ días? antes de que se bloquee la captura\. Avisa al encargado para que la renueve\./,
    );
    await understood.click();
    await expect(licenseNotice).toHaveCount(0);
    await logout(page);
    // Cerrarlo es por usuario: el administrador también lo ve, con la indicación de renovarla.
    await login(page, admin.username, admin.password);
    await expect(topBar(page)).toBeVisible();
    await expect(licenseNotice).toContainText(
      /Quedan? \d+ días? antes de que se bloquee la captura\. Renuévala para seguir capturando pedidos\./,
    );
    await shot(page, testInfo, 'aviso-tolerancia');

    // 3. Vencida: aviso fijo, sin «Entendido», con lo que sí y lo que no se puede hacer.
    await install(['--type', 'subscription', '--expires-at', '-6d']);
    await expect(fixedNotice).toContainText('La licencia del sistema venció', { timeout: 15000 });
    await expect(fixedNotice).toContainText(
      'Se puede seguir cobrando lo que está abierto, hacer cortes y consultar reportes. No se pueden capturar pedidos nuevos ni abrir un turno sin cuentas pendientes.',
    );
    await expect(fixedNotice.getByRole('button', { name: 'Entendido' })).toHaveCount(0);
    await expect(licenseNotice).toHaveCount(0);
    await shot(page, testInfo, 'aviso-vencida');
  } finally {
    await restoreTestLicense(control, driver.license.identity);
  }
  await expect(fixedNotice).toHaveCount(0, { timeout: 15000 });
});
