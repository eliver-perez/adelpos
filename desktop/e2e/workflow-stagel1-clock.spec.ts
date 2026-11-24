import { randomBytes } from 'node:crypto';
import { expect, test } from '@playwright/test';
import {
  installDriver,
  licenseServerControl,
  serverClockAvailable,
  setServerClockOffset,
  simulatedLicenseServer,
} from './native-driver';
import {
  admin,
  call,
  captureTakeaway,
  dismissNotices,
  enrollTerminal,
  lightTheme,
  login,
  operationMessage,
  otherTerminal,
  restoreTestLicense,
  sharedServerURL,
  shot,
  simpleProduct,
  terminalPage,
} from './license-flow';
import { notice } from './notices';
import { openSection, topBar } from './navigation';

// Etapa L1, Fase E (§6.2 y §7 de la especificación), con Go y PostgreSQL reales: el bloqueo por la fecha y hora de la
// computadora principal. La hora se mueve con el reloj simulado del Go temporal (ADEL_TEST_CLOCK_FILE, Fase A) y la de
// «internet» con el control del servidor de licencias simulado (/_prueba/hora, Fase B): nunca se cambia la hora de la
// Mac ni se llama a un servidor de licencias real. Al terminar, los dos relojes vuelven a la hora real y queda la
// licencia de las demás especificaciones, también si la prueba falla.
//
// La licencia se activa en línea con el simulador (un código PRP inventado, distinto en cada corrida): «Comprobar la
// hora con internet» le pide la hora con la consulta de esa activación. Se activa con la hora del simulador 5 horas
// atrás, para que la licencia vigente quede emitida antes de la hora atrasada del paso 5 (el piso firmado de la §6.3).
//
// 1. Con el reloj 3 horas atrás, la computadora principal abre sola «Sistema detenido por la fecha y hora de la
//    computadora» con sus cinco partes: qué pasó (las dos fechas que da Go), por qué se detiene, qué sí se puede hacer,
//    cómo resolverlo (Mac, Windows, «Comprobar la hora con internet» y el desbloqueo de soporte) y «Tu licencia está
//    bien…». Reportes sigue disponible, con el aviso fijo para volver.
// 2. «Comprobar la hora con internet» con la hora de internet correcta: no coincide y dice a qué hora corregirla.
// 3. El desbloqueo de soporte: el código de solicitud, una respuesta pegada que no es válida y un archivo que no es la
//    respuesta. (Una respuesta válida necesita la llave privada de soporte, que nunca está en las pruebas: la aceptan
//    las pruebas de Go de la Fase C.)
// 4. En otra terminal: el aviso fijo con la misma causa y, al capturar, «No se puede registrar: …», nunca un error
//    genérico.
// 5. Corregir el reloj quita el bloqueo solo, en las dos terminales. Con el reloj otra vez atrás y la misma hora en
//    internet (el reloj ya es correcto: la última actividad estaba en el futuro), «Comprobar la hora con internet» lo
//    quita.
// 6. Con la licencia vencida a la vez: los dos problemas por separado, sin «Tu licencia está bien…».

const clockTerminalMessage =
  'No se puede registrar: la computadora principal tiene la fecha y hora incorrectas. Avisa al encargado.';

test('bloqueo por la fecha y hora: pantalla completa, comprobar con internet, desbloqueo de soporte y terminales', async ({
  page,
  browser,
}, testInfo) => {
  test.skip(
    !serverClockAvailable() || !simulatedLicenseServer(),
    'Sin el reloj simulado del Go temporal ni el servidor de licencias simulado.',
  );
  test.setTimeout(300000);
  await page.setViewportSize({ width: 1280, height: 900 });
  const driver = await installDriver(page);
  await call(driver, 'POST', 'auth/login', admin);
  await lightTheme(driver);
  const suffix = randomBytes(3).toString('hex');
  const item = await simpleProduct(driver, `Agua hora ${suffix}`, `AGUA-HORA-${suffix}`);
  const control = await otherTerminal(driver, sharedServerURL, `Administración hora ${suffix}`);
  const identity = driver.license.identity;
  const credentials = await enrollTerminal(driver, sharedServerURL, `Caja de la barra ${suffix}`);
  await call(driver, 'POST', 'auth/logout');

  const terminal = await terminalPage(browser, credentials, { main: false });
  const licenseOk = page.locator('.clock-block').getByRole('note');
  const screenTitle = page.getByRole('heading', {
    name: 'Sistema detenido por la fecha y hora de la computadora',
    level: 1,
  });
  try {
    // La activación en línea, con la hora del simulador 5 horas atrás.
    const alphabet = '23456789ABCDEFGHJKMNPQRSTUVWXYZ';
    const group = () =>
      Array.from(randomBytes(4), (byte) => alphabet[byte % alphabet.length]).join('');
    expect((await licenseServerControl('/_prueba/hora', { desfase: '-5h' })).status).toBe(200);
    await control.expectOK('POST', 'license/activate', {
      license_key: `PRP-${group()}-${group()}-${group()}`,
    });
    expect((await licenseServerControl('/_prueba/hora', { desfase: '0s' })).status).toBe(200);
    await login(page, admin.username, admin.password);
    await expect(topBar(page)).toBeVisible();
    await login(terminal.page, admin.username, admin.password);
    await expect(topBar(terminal.page)).toBeVisible();

    // 1. El reloj de la computadora principal 3 horas atrás: la pantalla se abre sola.
    setServerClockOffset('-3h');
    await expect(screenTitle).toBeVisible({ timeout: 30000 });
    // Las dos fechas las da Go (auth/me), completas y en la zona horaria del restaurante.
    const me = (await control.expectOK('GET', 'auth/me')) as {
      license: { clock: { blocked: boolean; last_activity_time_text: string } };
    };
    const clock = me.license.clock;
    expect(clock.blocked).toBe(true);
    const what = page.getByRole('group', { name: 'Qué pasó' });
    await expect(what).toContainText(
      /La fecha y hora de esta computadora es \S+ \d{1,2} \S+ \d{4}, \d{2}:\d{2}\./,
    );
    await expect(what).toContainText(
      `La última actividad registrada en el sistema fue el ${clock.last_activity_time_text}.`,
    );
    await expect(what).toContainText(
      'La hora de la computadora no puede ser anterior a lo que ya se registró.',
    );
    await expect(page.getByRole('group', { name: 'Por qué se detiene' })).toContainText(
      'Para proteger tus cortes y reportes: si se registran ventas o cobros con una fecha atrasada, quedarían fuera de orden y los cortes no cuadrarían.',
    );
    const allowed = page.getByRole('group', { name: 'Qué sí puedes hacer' });
    await expect(allowed).toContainText('Consultar reportes e historial y hacer respaldos.');
    await expect(allowed.getByRole('button', { name: /Ir a Reportes/ })).toBeVisible();
    await expect(allowed.getByRole('button', { name: /Ir a Respaldos/ })).toBeVisible();
    const how = page.getByRole('group', { name: 'Cómo resolverlo' });
    await expect(how).toContainText('Corrige la fecha y hora de la computadora.');
    await expect(how).toContainText(
      'En Mac: menú Apple › Configuración del Sistema › General › Fecha y hora.',
    );
    await expect(how).toContainText('En Windows: Configuración › Hora e idioma › Fecha y hora.');
    await expect(how).toContainText(
      'Si la hora de la computadora es correcta y el problema sigue, comunícate con soporte.',
    );
    await expect(licenseOk).toHaveText(
      'Tu licencia está bien. Este bloqueo es solo por la fecha y hora.',
    );
    // La pantalla ya lo explica todo: no repite su aviso fijo.
    await expect(page.getByRole('alert', { name: 'Aviso de la fecha y hora' })).toHaveCount(0);
    await shot(page, testInfo, 'hora-pantalla-bloqueo');

    // 2. Internet tiene la hora real: no coincide y dice a qué hora corregir el reloj.
    const check = how.getByRole('button', { name: /Comprobar la hora con internet/ });
    await check.click();
    await expect(how.getByRole('alert')).toContainText(
      /La fecha y hora de esta computadora no coinciden con la de internet\. Corrígela a \S+ \d{1,2} \S+ \d{4}, \d{2}:\d{2} y vuelve a comprobar\./,
      { timeout: 60000 },
    );

    // 3. El desbloqueo de soporte: el código de solicitud, una respuesta pegada que no vale y un archivo equivocado.
    await how.getByRole('button', { name: /Desbloqueo de soporte/ }).click();
    const support = page.getByRole('region', { name: 'Desbloqueo de soporte' });
    const code = support.getByLabel('Código de solicitud');
    await expect(code).toBeVisible();
    expect(((await code.textContent()) ?? '').replace(/[^0-9A-Z]/g, '')).toHaveLength(24);
    await expect(support).toContainText(/Vale hasta el .+ y solo para esta computadora\./);
    await support.getByLabel('Respuesta de soporte').fill('respuesta copiada a medias');
    await support.getByRole('button', { name: /Desbloquear/ }).click();
    await expect(
      notice(page, 'La respuesta de soporte no es válida. Revisa que se haya copiado completa.'),
    ).toBeVisible();
    driver.license.nextUnlockFile(driver.license.writeFile('respuesta.txt', 'no es la respuesta'));
    await support.getByRole('button', { name: /Importar la respuesta/ }).click();
    await expect(
      notice(page, 'Elige la respuesta de soporte (un archivo terminado en .desbloqueo).'),
    ).toBeVisible();
    driver.license.nextUnlockFile(
      driver.license.writeFile('respuesta.desbloqueo', 'respuesta dañada de soporte'),
    );
    await support.getByRole('button', { name: /Importar la respuesta/ }).click();
    await expect(
      notice(page, 'La respuesta de soporte no es válida. Revisa que se haya copiado completa.'),
    ).toHaveCount(1, { timeout: 15000 });
    await expect(screenTitle).toBeVisible();
    await shot(page, testInfo, 'hora-desbloqueo-soporte');
    await dismissNotices(page);

    // Reportes sigue disponible, con el aviso fijo para volver a la pantalla.
    await allowed.getByRole('button', { name: /Ir a Reportes/ }).click();
    const mainClockNotice = page.getByRole('alert', { name: 'Aviso de la fecha y hora' });
    await expect(mainClockNotice).toContainText(
      'Sistema detenido por la fecha y hora de la computadora',
    );
    await mainClockNotice.getByRole('button', { name: /Ver cómo resolverlo/ }).click();
    await expect(screenTitle).toBeVisible();

    // 4. Otra terminal: el aviso fijo con la misma causa, sin la pantalla de la computadora principal, y el mensaje al
    // capturar.
    const terminalNotice = terminal.page.getByRole('alert', { name: 'Aviso de la fecha y hora' });
    await expect(terminalNotice).toContainText(clockTerminalMessage, { timeout: 30000 });
    await expect(terminalNotice.getByRole('button', { name: /Ver cómo resolverlo/ })).toHaveCount(
      0,
    );
    await openSection(terminal.page, 'Para llevar');
    await captureTakeaway(terminal.page, item);
    await expect(operationMessage(terminal.page, clockTerminalMessage)).toBeVisible({
      timeout: 15000,
    });
    await shot(terminal.page, testInfo, 'hora-terminal');

    // 5a. Corregir el reloj: el bloqueo se quita solo, en las dos terminales.
    setServerClockOffset('');
    await expect(
      page.getByRole('heading', { name: 'La fecha y hora de la computadora es correcta' }),
    ).toBeVisible({ timeout: 30000 });
    await expect(terminalNotice).toHaveCount(0, { timeout: 30000 });
    await page.getByRole('button', { name: 'Volver al inicio', exact: true }).click();
    await expect(topBar(page)).toBeVisible();

    // 5b. Otra vez atrás, pero ahora internet dice la misma hora: el reloj es correcto y la última actividad estaba en
    // el futuro, así que «Comprobar la hora con internet» la reemplaza y desbloquea.
    setServerClockOffset('-3h');
    await expect(screenTitle).toBeVisible({ timeout: 30000 });
    const internet = await licenseServerControl('/_prueba/hora', { desfase: '-3h' });
    expect(internet.status).toBe(200);
    await page
      .getByRole('group', { name: 'Cómo resolverlo' })
      .getByRole('button', { name: /Comprobar la hora con internet/ })
      .click();
    await expect(
      notice(page, 'La hora coincide con la de internet: el sistema se desbloqueó.'),
    ).toBeVisible({ timeout: 60000 });
    await expect(
      page.getByRole('heading', { name: 'La fecha y hora de la computadora es correcta' }),
    ).toBeVisible({ timeout: 30000 });
    await expect(page.getByRole('alert', { name: 'Aviso de la fecha y hora' })).toHaveCount(0);
    await shot(page, testInfo, 'hora-desbloqueada');
    setServerClockOffset('');
    await licenseServerControl('/_prueba/hora', { desfase: '0s' });

    // 6. También vencida: los dos problemas por separado. (El reloj, 6 horas atrás: la última actividad quedó 3 horas
    // atrás en el paso 5.)
    await dismissNotices(page);
    await page.getByRole('button', { name: 'Volver al inicio', exact: true }).click();
    await expect(topBar(page)).toBeVisible();
    setServerClockOffset('-6h');
    await expect(screenTitle).toBeVisible({ timeout: 30000 });
    await control.installLicense(identity, [
      '--type',
      'subscription',
      '--expires-at',
      '-6d',
      '--licensee',
      'Restaurante de aceptación',
    ]);
    const licenseProblem = page.getByRole('group', { name: /Además, la licencia/ });
    await expect(licenseProblem).toContainText('Además, la licencia: Vencida', { timeout: 15000 });
    await expect(licenseProblem).toContainText('La licencia del sistema venció.');
    await expect(licenseProblem).toContainText(
      'Es otro problema, aparte de la fecha y hora: corregir el reloj no lo resuelve.',
    );
    await expect(licenseOk).toHaveCount(0);
    await expect(page.getByRole('alert', { name: 'Aviso de la licencia' })).toContainText(
      'La licencia del sistema venció',
    );
    await shot(page, testInfo, 'hora-y-licencia');
  } finally {
    setServerClockOffset('');
    await licenseServerControl('/_prueba/hora', { desfase: '0s' });
    await restoreTestLicense(control, identity);
    // Go vuelve a leer el reloj simulado cada segundo: la especificación siguiente empieza sin el bloqueo.
    await expect
      .poll(
        async () =>
          (
            (await control.expectOK('GET', 'auth/me')) as {
              license: { clock: { blocked: boolean } };
            }
          ).license.clock.blocked,
        { timeout: 15000 },
      )
      .toBe(false);
    await terminal.context.close();
  }
});
