import { expect, test } from '@playwright/test';
import type { Locator, Page } from '@playwright/test';
import { installDriver } from './native-driver';
import { openAdministration, topBar } from './navigation';
import { notice } from './notices';

// Etapa A3, §3 (decisiones A3-D2 a A3-D6) con Go y PostgreSQL reales: una instalación nueva trae el rol Mesero con sus
// permisos de fábrica; su pantalla de permisos abre en la vista sencilla (capacidades, también parciales), y la
// detallada agrupa por área con «x de y», «Marcar todo el grupo» y «Quitar todo el grupo», el buscador, las dependencias
// automáticas y el aviso de lo que deja de funcionar, todo leído del catálogo de Go; el rol se guarda y «Restablecer
// permisos predeterminados» muestra qué se agrega y qué se quita, lo aplica y queda en el Registro de movimientos. El
// Administrador no tiene el botón. Se ordena después de las especificaciones de la A2 y antes de workflow-zz-test-data, y
// deja el Mesero como llegó (con sus permisos de fábrica).

async function login(page: Page) {
  await page.getByLabel('Usuario', { exact: true }).fill('admin');
  await page.getByLabel('Contraseña', { exact: true }).fill('admin');
  await page.getByRole('button', { name: 'Entrar', exact: true }).click();
}

// Encabezado de la sección de un grupo, dentro de la pantalla de permisos (la barra superior también tiene «Para
// llevar»).
function groupHeader(page: Page, name: string): Locator {
  return page.locator('.permission-editor').getByRole('button', { name: new RegExp(`^${name}`) });
}

// Las secciones de la vista detallada están plegadas: se abre la del grupo antes de tocar sus permisos.
async function openGroup(page: Page, name: string): Promise<Locator> {
  const header = groupHeader(page, name);
  if ((await header.getAttribute('aria-expanded')) !== 'true') {
    await header.click();
  }
  await expect(header).toHaveAttribute('aria-expanded', 'true');
  return header.locator('xpath=ancestor::section[1]');
}

test('Roles: permisos agrupados, vista sencilla, dependencias y «Restablecer permisos predeterminados» (etapa A3)', async ({
  page,
}) => {
  const driver = await installDriver(page);
  await page.goto('/');
  await login(page);
  await expect(topBar(page)).toBeVisible();
  const roles = (await driver.request('GET', 'roles')).body as unknown as {
    id: string;
    code: string | null;
    name: string;
    permissions: string[];
  }[];
  const waiter = roles.find((role) => role.code === 'waiter')!;
  const factory = (await driver.request('GET', `roles/${waiter.id}/defaults`)).body as unknown as {
    factory: string[];
    add: string[];
    remove: string[];
  };
  // 1. Instalación nueva: el Mesero llega con sus permisos de fábrica.
  expect(factory.add).toEqual([]);
  expect(factory.remove).toEqual([]);
  expect(factory.factory).toContain('accounts.open');
  expect(factory.factory).not.toContain('orders.cancel');

  await openAdministration(page, 'Roles');
  // El Administrador no tiene el botón (siempre tiene todo).
  const adminRow = page.locator('tbody tr').filter({ hasText: 'Administrador' });
  await adminRow.getByRole('button', { name: 'Editar permisos', exact: true }).click();
  await expect(page.getByRole('tab', { name: 'Vista sencilla' })).toHaveAttribute(
    'aria-selected',
    'true',
  );
  await expect(
    page.getByRole('button', { name: 'Restablecer permisos predeterminados' }),
  ).toHaveCount(0);
  await page.getByRole('button', { name: 'Cerrar editor', exact: true }).click();

  // 2. Vista sencilla del Mesero: «Atender mesas» y «Para llevar» completas; «Cobrar», sin marcar; «Manejar caja», no.
  const waiterRow = page.locator('tbody tr').filter({ hasText: /^Mesero/ });
  await waiterRow.getByRole('button', { name: 'Editar permisos', exact: true }).click();
  const tables = page.getByRole('checkbox', { name: 'Atender mesas', exact: true });
  await expect(tables).toBeChecked();
  await expect(page.getByRole('checkbox', { name: 'Para llevar', exact: true })).toBeChecked();
  // «Cobrar» comparte con «Atender mesas» imprimir la precuenta: el Mesero la tiene en parte.
  const collect = page.getByRole('checkbox', { name: 'Cobrar', exact: true });
  await expect(collect).toHaveAttribute('aria-checked', 'mixed');
  await expect(page.getByText(/^Parcial: 1 de 9$/)).toBeVisible();

  // 3. Vista detallada: grupos con «x de y», el buscador y las dependencias.
  await page.getByRole('tab', { name: 'Vista detallada' }).click();
  const takeaway = groupHeader(page, 'Para llevar');
  await expect(takeaway).toContainText('3 de 4');
  const orders = await openGroup(page, 'Captura y pedidos');
  await expect(orders.getByRole('button', { name: /^Captura y pedidos/ })).toContainText('2 de 5');
  // Marcar «Cancelar partidas o cantidades confirmadas» marca solo lo que necesita (modificar lo enviado).
  await orders
    .getByRole('checkbox', { name: 'Cancelar partidas o cantidades confirmadas', exact: true })
    .check();
  await expect(page.locator('.permission-change')).toContainText(
    'También se marcó lo que necesita',
  );
  await expect(
    orders.getByRole('checkbox', {
      name: 'Modificar partidas confirmadas sin aprobación adicional',
      exact: true,
    }),
  ).toBeChecked();
  await expect(orders.getByRole('button', { name: /^Captura y pedidos/ })).toContainText('4 de 5');
  // Quitar «Consultar cuentas e importes» avisa todo lo que dejará de funcionar sin quitarlo.
  const accounts = await openGroup(page, 'Mesas y cuentas');
  await accounts
    .getByRole('checkbox', { name: 'Consultar cuentas e importes', exact: true })
    .uncheck();
  await expect(page.locator('.permission-change')).toContainText('dejarán de funcionar');
  await expect(page.locator('.permission-change')).toContainText('«Abrir cuentas»');
  await expect(
    accounts.getByRole('checkbox', { name: 'Abrir cuentas', exact: true }),
  ).toBeChecked();
  await accounts
    .getByRole('checkbox', { name: 'Consultar cuentas e importes', exact: true })
    .check();
  // «Quitar todo el grupo» y «Marcar todo el grupo» en Para llevar.
  const takeawayGroup = await openGroup(page, 'Para llevar');
  await takeawayGroup.getByRole('button', { name: 'Quitar todo el grupo', exact: true }).click();
  await expect(takeaway).toContainText('0 de 4');
  await takeawayGroup.getByRole('button', { name: 'Marcar todo el grupo', exact: true }).click();
  await expect(takeaway).toContainText('4 de 4');
  // El buscador filtra por la descripción: solo queda el grupo con coincidencias, abierto.
  await page.getByRole('searchbox', { name: /^Buscar permiso/ }).fill('cajón de dinero');
  await expect(
    page.getByRole('checkbox', { name: 'Abrir el cajón de dinero manualmente', exact: true }),
  ).toBeVisible();
  await expect(groupHeader(page, 'Mesas y cuentas')).toHaveCount(0);
  await page.getByRole('searchbox', { name: /^Buscar permiso/ }).fill('');

  // 4. Guardar: Go recibe el conjunto nuevo (modificar y cancelar lo enviado, y el umbral de Para llevar).
  await page.getByRole('button', { name: 'Guardar rol', exact: true }).click();
  await expect(notice(page, 'Rol guardado.')).toBeVisible();
  await expect
    .poll(async () => {
      const preview = (await driver.request('GET', `roles/${waiter.id}/defaults`))
        .body as unknown as {
        add: string[];
        remove: string[];
      };
      return [...preview.remove].sort();
    })
    .toEqual(['orders.cancel', 'orders.edit', 'restaurant.settings.manage']);

  // 5. «Restablecer permisos predeterminados»: vista previa, confirmación y Registro de movimientos.
  await page.getByRole('button', { name: 'Restablecer permisos predeterminados' }).click();
  const dialog = page.getByRole('alertdialog');
  await expect(dialog).toContainText('Se quitarán (3):');
  await expect(dialog).toContainText('Cancelar partidas o cantidades confirmadas');
  await expect(dialog).not.toContainText('Se agregarán');
  await expect(dialog).toContainText('No cambian el límite de descuento');
  await dialog.getByRole('button', { name: 'Restablecer permisos', exact: true }).click();
  await expect(notice(page, 'Permisos predeterminados de Mesero restablecidos.')).toBeVisible();
  await expect
    .poll(async () => {
      const preview = (await driver.request('GET', `roles/${waiter.id}/defaults`))
        .body as unknown as {
        add: string[];
        remove: string[];
      };
      return preview.add.length + preview.remove.length;
    })
    .toBe(0);
  await page.getByRole('tab', { name: 'Vista sencilla' }).click();
  await expect(page.getByRole('checkbox', { name: 'Atender mesas', exact: true })).toBeChecked();
  const audit = (await driver.request('GET', 'audit?action=role.defaults_restored'))
    .body as unknown as {
    items: { entity_id: string; success: boolean }[];
  };
  expect(audit.items.some((item) => item.entity_id === waiter.id && item.success)).toBe(true);
  await page.getByRole('button', { name: 'Cerrar editor', exact: true }).click();
  await openAdministration(page, 'Registro de movimientos');
  await expect(
    page.getByText('Permisos predeterminados del rol restablecidos').first(),
  ).toBeVisible();
});
