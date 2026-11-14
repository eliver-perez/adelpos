import { expect, test } from '@playwright/test';
import type { Page } from '@playwright/test';
import { installDriver } from './native-driver';
import { openAdministration, topBar } from './navigation';

// Etapa 9G, fase A (§2.1–§2.3) con Go y PostgreSQL reales: elección de la modalidad en el primer
// arranque, «Servicio del servidor» y «Modalidad de esta computadora» en Mantenimiento, la pantalla de
// estado de «Solo servidor» (sin interfaz de venta, con acceso a Administración) y «Buscar el servidor» de
// una terminal sin servidor. El puente simula la modalidad que guarda Rust (lista cerrada) y el reinicio
// de la aplicación (recarga la página); el servidor temporal corre con «serve», así que no es un servicio
// de la computadora y no ofrece «Reiniciar». No cambia nada en la base compartida.

async function login(page: Page) {
  await page.getByLabel('Usuario', { exact: true }).fill('admin');
  await page.getByLabel('Contraseña', { exact: true }).fill('admin');
  await page.getByRole('button', { name: 'Entrar', exact: true }).click();
}

// Cambia la modalidad desde Mantenimiento; la aplicación se recarga con la nueva.
async function changeMode(page: Page, label: string, dialogTitle: string, expected: RegExp) {
  const section = page.getByRole('region', { name: 'Modalidad de esta computadora' });
  await section.getByRole('radio', { name: new RegExp(label) }).click();
  await section.getByRole('button', { name: 'Cambiar la modalidad', exact: true }).click();
  const dialog = page.getByRole('alertdialog', { name: dialogTitle });
  await expect(dialog).toContainText(expected);
  await expect(dialog).toContainText('La aplicación se reinicia para aplicar el cambio.');
  await dialog.getByRole('button', { name: `Cambiar a «${label}»`, exact: true }).click();
  await expect(page.getByText(`Modalidad cambiada a «${label}».`)).toBeVisible();
}

test('modalidad de la computadora: primer arranque, Mantenimiento, «Solo servidor» y «Buscar el servidor» (etapa 9G, fase A)', async ({
  page,
}) => {
  const driver = await installDriver(page);

  // 1. Primer arranque: sin modalidad elegida se pregunta, con la del instalador sugerida.
  driver.setInstallation({ chosen: false, mode: 'server-terminal' });
  await page.goto('/');
  await expect(
    page.getByRole('heading', { name: '¿Cómo se usará esta computadora?' }),
  ).toBeVisible();
  const options = page.getByRole('radiogroup', { name: 'Modalidad de esta computadora' });
  await expect(options.getByRole('radio')).toHaveCount(3);
  await expect(options.getByRole('radio', { name: /Servidor y terminal/ })).toHaveAttribute(
    'aria-checked',
    'true',
  );
  await page.getByRole('button', { name: 'Continuar', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Inicia sesión' })).toBeVisible();
  await login(page);
  await expect(topBar(page)).toBeVisible();

  // 2. Mantenimiento: el servidor temporal no es un servicio de la computadora (sin «Reiniciar») y la
  //    modalidad vigente aparece marcada.
  await openAdministration(page, 'Mantenimiento');
  const service = page.getByRole('region', { name: 'Servicio del servidor' });
  await expect(service).toContainText('no está instalado como servicio de la computadora');
  await expect(service).toContainText('Activo desde hace');
  await expect(service.getByRole('button', { name: 'Reiniciar' })).toHaveCount(0);
  const modality = page.getByRole('region', { name: 'Modalidad de esta computadora' });
  await expect(modality.getByRole('radio', { name: /Servidor y terminal/ })).toContainText(
    'Actual',
  );
  await expect(modality.getByRole('button', { name: 'Cambiar la modalidad' })).toBeDisabled();

  // 3. «Solo servidor»: sin interfaz de venta; estado del servidor y Administración.
  await changeMode(page, 'Solo servidor', 'Cambiar la modalidad', /ya no se venderá/);
  await expect(page.getByRole('heading', { name: 'Servidor del restaurante' })).toBeVisible({
    timeout: 15000,
  });
  await expect(topBar(page)).toHaveCount(0);
  const serverNavigation = page.getByRole('navigation', { name: 'Secciones del servidor' });
  await expect(serverNavigation.getByRole('button', { name: 'Estado del servidor' })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Mesas', exact: true })).toHaveCount(0);
  await expect(
    page.getByText(/terminales? conectadas? de|Todavía no hay otras terminales/),
  ).toBeVisible();
  await expect(page.getByText('Sin registro')).toBeVisible();

  // Antes de iniciar sesión, «Solo servidor» muestra el estado y el acceso a Administración.
  await page.getByRole('button', { name: /^Menú del usuario/ }).click();
  await page
    .getByRole('group', { name: 'Menú del usuario' })
    .getByRole('button', { name: 'Cerrar sesión', exact: true })
    .click();
  await expect(page.getByRole('heading', { name: 'Servidor del restaurante' })).toBeVisible();
  await expect(page.locator('.server-only-state')).toContainText('El servidor está activo.');
  await page.getByRole('button', { name: 'Entrar a Administración', exact: true }).click();
  await login(page);
  await expect(page.getByRole('heading', { name: 'Servidor del restaurante' })).toBeVisible();
  await page.getByRole('button', { name: 'Abrir Administración', exact: true }).click();
  for (const tile of ['Sistema', 'Mantenimiento']) {
    await page.locator('.admin-tiles').getByRole('button', { name: tile, exact: true }).click();
  }
  await changeMode(page, 'Servidor y terminal', 'Cambiar la modalidad', /también como caja/);
  await expect(topBar(page)).toBeVisible({ timeout: 15000 });
  await expect(topBar(page).getByRole('button', { name: 'Mesas', exact: true })).toBeVisible();

  // 4. «Solo terminal»: deja de ser servidor sin borrar la base y pasa a «Buscar el servidor».
  await openAdministration(page, 'Mantenimiento');
  await changeMode(
    page,
    'Solo terminal',
    'Dejar de ser el servidor',
    /La base de datos se conserva en esta computadora y no se borra nada/,
  );
  await expect(page.getByRole('heading', { name: 'Buscar el servidor' })).toBeVisible({
    timeout: 15000,
  });
  await expect(page.locator('.find-server-status')).toContainText(
    'Esta terminal todavía no está conectada al servidor del restaurante.',
  );
  await expect(page.getByRole('note')).toContainText('sus datos se conservan');
  await page
    .getByRole('button', { name: 'Usar esta computadora como servidor', exact: true })
    .click();
  await page
    .getByRole('alertdialog', { name: 'Usar esta computadora como servidor' })
    .getByRole('button', { name: 'Usar como servidor', exact: true })
    .click();
  await expect(topBar(page)).toBeVisible({ timeout: 15000 });
});
