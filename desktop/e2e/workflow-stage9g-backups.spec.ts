import { expect, test } from '@playwright/test';
import type { Page } from '@playwright/test';
import {
  existsSync,
  mkdirSync,
  readdirSync,
  renameSync,
  rmSync,
  statSync,
  writeFileSync,
} from 'node:fs';
import { dirname, join } from 'node:path';
import { installDriver } from './native-driver';
import { notice } from './notices';
import { openAdministration, topBar } from './navigation';

// Etapa 9G, fase F (§7) con Go y PostgreSQL reales: Sistema › Mantenimiento › «Respaldos» (respaldo automático,
// cuántos conservar, carpeta elegida con el diálogo nativo simulado y «Respaldar ahora» con el respaldo real de
// pg_dump), la carpeta elegida que deja de estar disponible (el respaldo queda en la predeterminada y se avisa) y el
// respaldo que falla (aviso persistente en la pantalla de inicio hasta un respaldo correcto posterior).
//
// La carpeta predeterminada es la backup_directory del servidor temporal («backups» junto a su configuración, en el
// directorio temporal de la prueba, igual que el token de aprovisionamiento). Para que un respaldo falle de verdad,
// la prueba aparta esa carpeta y deja un archivo con su nombre; al terminar todo vuelve como estaba (la carpeta
// predeterminada, los respaldos automáticos activos y 30 conservados) para las especificaciones siguientes.

const serverDirectory = dirname(process.env.ADEL_BROWSER_PROVISIONING!);
const defaultFolder = join(serverDirectory, 'backups');
const asideFolder = join(serverDirectory, 'backups-apartada');
const chosenFolder = join(serverDirectory, 'usb-respaldos', 'Respaldos');

function backupsSection(page: Page) {
  return page.getByRole('region', { name: 'Respaldos' });
}

async function openBackups(page: Page) {
  await openAdministration(page, 'Mantenimiento');
  await expect(backupsSection(page).getByRole('button', { name: 'Respaldar ahora' })).toBeEnabled();
}

// «Respaldar ahora» con el respaldo real: espera a que aparezca en la lista (un renglón más) y devuelve ese renglón.
// Un resultado fallido además se avisa como error (que permanece hasta cerrarlo): se comprueba y se cierra para que
// no tape los avisos siguientes.
async function backupNow(page: Page, failure?: string) {
  const section = backupsSection(page);
  const items = section.getByRole('list', { name: 'Últimos respaldos' }).getByRole('listitem');
  // El botón aparece cuando la sección ya cargó su lista: se cuenta después.
  await expect(section.getByRole('button', { name: 'Respaldar ahora' })).toBeEnabled();
  const before = await items.count();
  await section.getByRole('button', { name: 'Respaldar ahora', exact: true }).click();
  await expect(items).toHaveCount(before + 1, { timeout: 60000 });
  await expect(section.getByRole('button', { name: 'Respaldar ahora' })).toBeEnabled();
  if (failure) {
    await closeNotice(page, failure);
  }
  return items.first();
}

// Comprueba un aviso de error y lo cierra.
async function closeNotice(page: Page, text: string | RegExp) {
  await expect(notice(page, text)).toBeVisible();
  await page
    .locator('[data-sonner-toast]')
    .filter({ hasText: text })
    .getByRole('button', { name: 'Cerrar aviso' })
    .click();
  await expect(notice(page, text)).toHaveCount(0);
}

// El aviso de la pantalla de inicio: el logotipo vuelve al inicio de la terminal (sea cual sea).
async function homeAlert(page: Page) {
  await page.getByRole('button', { name: /^AD-EL POS · Ir al inicio/ }).click();
  return page.locator('.admin-main .backup-warning');
}

// Quita el archivo que ocupa el nombre de la carpeta predeterminada (paso 5) y devuelve la carpeta apartada.
function restoreDefaultFolder() {
  if (existsSync(defaultFolder) && statSync(defaultFolder).isFile()) {
    rmSync(defaultFolder);
  }
  if (existsSync(asideFolder) && !existsSync(defaultFolder)) {
    renameSync(asideFolder, defaultFolder);
  }
}

test.afterAll(() => {
  restoreDefaultFolder();
  rmSync(join(serverDirectory, 'usb-respaldos'), { recursive: true, force: true });
});

test('respaldos: configuración, carpeta, «Respaldar ahora» y aviso de fallo en la pantalla de inicio (etapa 9G, fase F)', async ({
  page,
}) => {
  test.setTimeout(240000);
  const driver = await installDriver(page);
  await page.goto('/');
  await page.getByLabel('Usuario', { exact: true }).fill('admin');
  await page.getByLabel('Contraseña', { exact: true }).fill('admin');
  await page.getByRole('button', { name: 'Entrar', exact: true }).click();
  await expect(topBar(page)).toBeVisible();

  // 1. La sección muestra la programación de una instalación nueva. Se desactiva el respaldo automático durante la
  //    prueba (así el trabajador diario no se cruza con ella) y se cambian los respaldos que se conservan; el cambio
  //    queda en el Registro de movimientos.
  await openBackups(page);
  const section = backupsSection(page);
  const automatic = section.getByRole('button', { name: 'Respaldo automático diario' });
  await expect(automatic).toHaveAttribute('aria-pressed', 'true');
  await expect(section.getByRole('button', { name: /^Hora de inicio del día \(/ })).toHaveAttribute(
    'aria-pressed',
    'true',
  );
  await expect(section.getByLabel(/Respaldos automáticos que se conservan/)).toHaveValue('30');
  await expect(section).toContainText('Se guardan en la carpeta predeterminada');
  await section.getByRole('button', { name: 'Otra hora', exact: true }).click();
  await expect(
    section.getByRole('group', { name: 'Hora del respaldo', exact: true }),
  ).toBeVisible();
  await section.getByRole('button', { name: /^Hora de inicio del día \(/ }).click();
  await automatic.click();
  await section.getByLabel(/Respaldos automáticos que se conservan/).fill('7');
  await section.getByRole('button', { name: 'Guardar respaldos automáticos', exact: true }).click();
  await expect(notice(page, 'Respaldos automáticos guardados.')).toBeVisible();
  await expect(section).toContainText('El respaldo automático diario está desactivado.');
  const audit = await driver.request('GET', 'audit?action=maintenance.backup_settings_changed');
  expect(JSON.stringify(audit.body)).toContain('maintenance.backup_settings_changed');

  // 2. La carpeta se elige con el diálogo nativo: cancelarlo no cambia nada; elegir una la guarda (Go la valida).
  driver.nextBackupFolder('cancel');
  await section.getByRole('button', { name: 'Elegir carpeta', exact: true }).click();
  await expect(section).toContainText('Se guardan en la carpeta predeterminada');
  mkdirSync(chosenFolder, { recursive: true });
  driver.nextBackupFolder(chosenFolder);
  await section.getByRole('button', { name: 'Elegir carpeta', exact: true }).click();
  await expect(notice(page, 'Carpeta de los respaldos guardada.')).toBeVisible();
  await expect(section).toContainText('Se guardan en la carpeta «Respaldos».');
  // Una carpeta que no existe no se acepta.
  driver.nextBackupFolder(join(serverDirectory, 'no-existe'));
  await section.getByRole('button', { name: 'Elegir carpeta', exact: true }).click();
  await closeNotice(page, /No se encontró esa carpeta/);
  await expect(section).toContainText('Se guardan en la carpeta «Respaldos».');

  // 3. «Respaldar ahora» con el respaldo real: queda en la carpeta elegida, revisado y en la lista.
  let latest = await backupNow(page);
  await expect(latest).toContainText('Manual');
  await expect(latest).toContainText('Correcto y revisado');
  await expect(latest).toContainText('En la carpeta «Respaldos»');
  expect(readdirSync(chosenFolder).filter((name) => name.endsWith('-manual.dump'))).toHaveLength(1);
  await expect(section.getByRole('alert')).toHaveCount(0);

  // 4. La carpeta elegida desaparece (se desconectó la USB): el respaldo queda en la predeterminada y se avisa en
  //    Mantenimiento y en la pantalla de inicio.
  rmSync(join(serverDirectory, 'usb-respaldos'), { recursive: true, force: true });
  latest = await backupNow(page);
  await expect(latest).toContainText('Correcto y revisado');
  await expect(latest).toContainText('En la carpeta predeterminada');
  await expect(latest).toContainText('La carpeta elegida no se encontró');
  await expect(section.getByRole('alert')).toContainText(
    'La carpeta de los respaldos no estaba disponible',
  );
  await expect(section).toContainText('Ahora no se encuentra');
  let alert = await homeAlert(page);
  await expect(alert).toContainText('La carpeta de los respaldos no estaba disponible');

  // 5. Ni la carpeta elegida ni la predeterminada sirven: el respaldo falla y el aviso de la pantalla de inicio lo
  //    dice hasta que haya un respaldo correcto.
  await alert.getByRole('button', { name: 'Ver respaldos', exact: true }).click();
  await expect(section).toBeVisible();
  renameSync(defaultFolder, asideFolder);
  writeFileSync(defaultFolder, 'no es una carpeta');
  latest = await backupNow(page, 'No se pudo guardar el archivo en ninguna carpeta.');
  await expect(latest).toContainText('Falló');
  await expect(section.getByRole('alert')).toContainText(
    'No se pudo crear el respaldo de la base de datos',
  );
  alert = await homeAlert(page);
  await expect(alert).toContainText('No se pudo crear el respaldo de la base de datos');
  await expect(alert).toContainText('No se pudo guardar el archivo en ninguna carpeta.');
  // Fuera de la pantalla de inicio el aviso no se repite.
  await openAdministration(page, 'Configuración');
  await expect(page.locator('.admin-main .backup-warning')).toHaveCount(0);

  // 6. Vuelve la carpeta predeterminada: un respaldo correcto posterior quita el aviso.
  restoreDefaultFolder();
  await openBackups(page);
  await section
    .getByRole('button', { name: 'Usar la carpeta predeterminada', exact: true })
    .click();
  await expect(
    notice(page, 'Los respaldos se guardarán en la carpeta predeterminada.'),
  ).toBeVisible();
  latest = await backupNow(page);
  await expect(latest).toContainText('Correcto y revisado');
  await expect(section.getByRole('alert')).toHaveCount(0);
  alert = await homeAlert(page);
  // El aviso se consulta al llegar al inicio: se da tiempo a la consulta antes de comprobar que no aparece.
  await page.waitForTimeout(1500);
  await expect(alert).toHaveCount(0);

  // 7. Se deja la configuración como estaba: respaldo automático activo y 30 conservados.
  await openBackups(page);
  await section.getByRole('button', { name: 'Respaldo automático diario' }).click();
  await section.getByLabel(/Respaldos automáticos que se conservan/).fill('30');
  await section.getByRole('button', { name: 'Guardar respaldos automáticos', exact: true }).click();
  await expect(section.getByRole('button', { name: 'Respaldo automático diario' })).toHaveAttribute(
    'aria-pressed',
    'true',
  );
  await expect(section.getByLabel(/Respaldos automáticos que se conservan/)).toHaveValue('30');
});
