import { randomBytes, randomUUID } from 'node:crypto';
import { expect, test } from '@playwright/test';
import { installDriver } from './native-driver';
import {
  admin,
  call,
  captureTakeaway,
  enrollTerminal,
  lightTheme,
  login,
  operationMessage,
  otherTerminal,
  restoreTestLicense,
  seatUsage,
  sharedServerURL,
  shot,
  simpleProduct,
  terminalPage,
} from './license-flow';
import { notice } from './notices';
import { openAdministration, openSection, topBar } from './navigation';

// Etapa L1, Fase E (§5 y §7 de la especificación), con Go y PostgreSQL reales: los cupos de la licencia en el Go
// temporal compartido. La licencia de prueba la firma license-test (POST license/import desde otra terminal de
// administración) con un solo cupo libre de comandero y de pantalla de cocina: lo que ya ocupan las especificaciones
// anteriores, más uno. Al terminar queda la licencia de las demás especificaciones, también si la prueba falla.
//
// 1. Una tableta (por la API) captura y ocupa el último cupo de comandero; la barra (con su página) intenta capturar y
//    ve el mensaje de §5 con el cupo, cuántos hay en uso y el límite, nunca un error genérico.
// 2. Terminales: el resumen de los cupos en lugar del de sesiones por tipo, qué cupo ocupa cada terminal y «Liberar
//    cupo» con su confirmación, que cierra la sesión de la tableta. Después la barra sí captura.
// 3. Preparación: la tableta ocupa el último cupo de pantalla de cocina; en la barra, Preparación dice que no hay cupo.
//    Al liberarlo en Terminales, «Volver a intentar» abre el tablero. Salir de Preparación libera el cupo en Go.

test('cupos: sin cupo de comandero y de pantalla de cocina, «Liberar cupo» en Terminales y Preparación', async ({
  page,
  browser,
}, testInfo) => {
  test.setTimeout(300000);
  await page.setViewportSize({ width: 1280, height: 900 });
  const driver = await installDriver(page);
  await call(driver, 'POST', 'auth/login', admin);
  await lightTheme(driver);
  const suffix = randomBytes(3).toString('hex');
  const item = await simpleProduct(driver, `Agua cupos ${suffix}`, `AGUA-CUPOS-${suffix}`);
  const control = await otherTerminal(driver, sharedServerURL, `Administración cupos ${suffix}`);
  const tabletName = `Tableta del salón ${suffix}`;
  const tablet = await otherTerminal(driver, sharedServerURL, tabletName);
  const barCredentials = await enrollTerminal(driver, sharedServerURL, `Barra ${suffix}`);
  const identity = driver.license.identity;
  await call(driver, 'POST', 'auth/logout');
  const bar = await terminalPage(browser, barCredentials, { main: false });
  let tabletAccount: { id: string; revision: number } | null = null;

  try {
    const before = await seatUsage(control);
    const takers = before.order_takers.in_use + 1;
    const kitchens = before.kitchen_displays.in_use + 1;
    await control.installLicense(identity, [
      '--type',
      'perpetual',
      '--plan',
      'premium',
      '--cash-registers',
      '20',
      '--order-takers',
      String(takers),
      '--kitchen-displays',
      String(kitchens),
      '--licensee',
      'Restaurante de aceptación',
    ]);
    await login(page, admin.username, admin.password);
    await expect(topBar(page)).toBeVisible();
    await login(bar.page, admin.username, admin.password);
    await expect(topBar(bar.page)).toBeVisible();

    // 1. La tableta ocupa el último cupo de comandero (captura sin turno de caja).
    const opened = (await tablet.expectOK('POST', 'restaurant/accounts', {
      operation_id: randomUUID(),
      mode: 'takeaway',
      name: `Pedido de la tableta ${suffix}`,
    })) as { account: { id: string; revision: number } };
    tabletAccount = opened.account;
    expect((await seatUsage(control)).order_takers.in_use).toBe(takers);
    await openSection(bar.page, 'Para llevar');
    await captureTakeaway(bar.page, item);
    const noTaker = `No hay cupo de comandero libre (${takers} de ${takers} en uso). Pide que se cierre la sesión de otra terminal.`;
    await expect(operationMessage(bar.page, noTaker)).toBeVisible({ timeout: 15000 });
    await shot(bar.page, testInfo, 'cupo-comandero');

    // 2. Terminales: el resumen de los cupos y qué ocupa cada terminal.
    await openAdministration(page, 'Terminales');
    const summary = page.getByRole('region', { name: 'Cupos de la licencia' });
    await expect(summary).toContainText(`Comanderos${takers} de ${takers}`);
    await expect(summary).toContainText(`Pantallas de cocina${kitchens - 1} de ${kitchens}`);
    await expect(summary).toContainText('Cajas');
    await expect(page.getByRole('columnheader', { name: 'Cupo', exact: true })).toBeVisible();
    const tabletRow = page.locator('tbody tr').filter({ hasText: tabletName });
    await expect(tabletRow).toContainText('Comandero');
    await shot(page, testInfo, 'cupos-terminales');
    const release = tabletRow.getByRole('button', { name: 'Liberar cupo', exact: true });
    await release.click();
    let dialog = page.getByRole('alertdialog', { name: `Liberar el cupo de ${tabletName}` });
    await expect(dialog).toContainText(
      'Se cierra la sesión de quien esté usando esta terminal y su cupo de comandero queda libre para otra terminal.',
    );
    await shot(page, testInfo, 'cupos-liberar-confirmacion');
    await dialog.getByRole('button', { name: 'Liberar cupo', exact: true }).click();
    await expect(notice(page, `Cupo de ${tabletName} liberado.`)).toBeVisible();
    await expect(release).toHaveCount(0, { timeout: 15000 });
    await expect(summary).toContainText(`Comanderos${takers - 1} de ${takers}`);
    // La sesión de la tableta se cerró.
    const revoked = await tablet.request('GET', 'auth/me');
    expect(revoked.status).toBe(401);
    // Ahora la barra sí captura.
    await bar.page.getByRole('button', { name: new RegExp(`^${item.product}`) }).click();
    const bill = bar.page.getByRole('complementary', { name: 'Cuenta actual' });
    await expect(bill.getByRole('article', { name: new RegExp(`× ${item.product}`) })).toBeVisible({
      timeout: 15000,
    });
    await expect(bill.locator('.capture-sync-status')).toHaveText('Guardado', { timeout: 15000 });
    await expect.poll(async () => (await seatUsage(control)).order_takers.in_use).toBe(takers);

    // 3. Preparación: la tableta vuelve a entrar y ocupa el último cupo de pantalla de cocina.
    await tablet.expectOK('POST', 'auth/login', admin);
    await tablet.expectOK('POST', 'kitchen/display-mode', { active: true });
    await openSection(bar.page, 'Cocina');
    await bar.page
      .getByRole('group', { name: 'Funciones de cocina' })
      .getByRole('button', { name: 'Preparación', exact: true })
      .click();
    await expect(
      bar.page.getByRole('heading', { name: 'Preparación no está disponible en esta terminal' }),
    ).toBeVisible({ timeout: 15000 });
    await expect(bar.page.getByRole('alert')).toContainText(
      `No hay cupo de pantalla de cocina libre (${kitchens} de ${kitchens} en uso). Pide que otra terminal salga de Preparación.`,
    );
    await shot(bar.page, testInfo, 'cupo-cocina');
    await openAdministration(page, 'Configuración');
    await openAdministration(page, 'Terminales');
    await expect(tabletRow).toContainText('Pantalla de cocina');
    await tabletRow.getByRole('button', { name: 'Liberar cupo', exact: true }).click();
    dialog = page.getByRole('alertdialog', { name: `Liberar el cupo de ${tabletName}` });
    await expect(dialog).toContainText('su cupo de pantalla de cocina queda libre');
    await dialog.getByRole('button', { name: 'Liberar cupo', exact: true }).click();
    await expect(notice(page, `Cupo de ${tabletName} liberado.`)).toBeVisible();
    await bar.page.getByRole('button', { name: 'Volver a intentar', exact: true }).click();
    await expect(bar.page.getByLabel('Estación de preparación')).toBeVisible({ timeout: 15000 });
    await expect
      .poll(async () => (await seatUsage(control)).kitchen_displays.in_use)
      .toBe(kitchens);
    // Salir de Preparación avisa a Go y libera el cupo.
    await openSection(bar.page, 'Mesas');
    await expect
      .poll(async () => (await seatUsage(control)).kitchen_displays.in_use)
      .toBe(kitchens - 1);
  } finally {
    await restoreTestLicense(control, identity);
    // La base es la compartida: sin la cuenta vacía de la tableta ni sus sesiones abiertas.
    if (tabletAccount) {
      await tablet.request('POST', 'auth/login', admin);
      await tablet.request('POST', `restaurant/accounts/${tabletAccount.id}/discard`, {
        operation_id: randomUUID(),
        revision: tabletAccount.revision,
      });
    }
    await tablet.request('POST', 'auth/logout');
    await bar.driver.request('POST', 'auth/logout');
    await bar.context.close();
  }
});
