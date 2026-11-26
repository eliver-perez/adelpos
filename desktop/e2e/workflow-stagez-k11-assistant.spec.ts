import { randomBytes } from 'node:crypto';
import { expect, test } from '@playwright/test';
import type { Page } from '@playwright/test';
import { installDriver } from './native-driver';
import { admin, call, dismissNotices, lightTheme, login, shot } from './license-flow';
import { openAdministration } from './navigation';
import { notice } from './notices';
import { claimPairing } from './pairing';
import { chooseOption } from './select';
import {
  asDisplay,
  categoryWithProduct,
  coverage,
  patchStation,
  requestDisplayCodeWhenAllowed,
  stations,
} from './kitchen-coverage-flow';
import type { Display } from './kitchen-coverage-flow';

// Etapa K1.1, Fase B (§4.1 y §6 de la especificación; K11-D6), con Go y PostgreSQL reales: una tableta pide su código
// como pantalla de cocina y la computadora principal lo aprueba en Sistema › Terminales. Sin estaciones que reciban
// comandas en pantalla, el asistente ofrece crear «Cocina» ahí mismo; después se marcan dos categorías y el resumen
// dice qué recibirá la pantalla. Termina con «Cocina» creada y las dos categorías en su ruta (lo dice Go en la
// cobertura) y la tableta, al conectarse, muestra Cocina.
//
// La base es compartida con las especificaciones anteriores: las estaciones que ya reciben en pantalla se desactivan
// al empezar y se vuelven a activar al terminar, para que la pantalla quede «sin estaciones».

function assistant(page: Page, name: string) {
  return page.getByRole('region', { name: `Qué recibe ${name}` });
}

test('el asistente al aprobar una pantalla de cocina sin estaciones termina con «Cocina» y dos categorías (etapa K1.1)', async ({
  page,
}, testInfo) => {
  test.setTimeout(15 * 60_000);
  await page.setViewportSize({ width: 1280, height: 900 });
  const driver = await installDriver(page);
  await call(driver, 'POST', 'auth/login', admin);
  await lightTheme(driver);
  const suffix = randomBytes(3).toString('hex');
  const existing = await stations(driver);
  expect(
    existing.filter((station) => station.name === 'Cocina'),
    'la base no tiene todavía una estación «Cocina»',
  ).toEqual([]);
  const paused = existing.filter((station) => station.active && station.reception !== 'printer');
  for (const station of paused) {
    await patchStation(driver, station.id, { active: false });
  }
  try {
    const breakfast = await categoryWithProduct(driver, `Desayunos ${suffix}`, `K11A-D-${suffix}`);
    const burgers = await categoryWithProduct(driver, `Hamburguesas ${suffix}`, `K11A-H-${suffix}`);
    const name = `Tableta asistente ${suffix}`;
    const pairing = await requestDisplayCodeWhenAllowed(name);

    // La computadora principal aprueba el código en Terminales: se abre el asistente.
    // La sesión del puente se cierra antes de entrar por la pantalla (como en las especificaciones de la K1 y la 9D);
    // después, las llamadas del puente usan la sesión de la pantalla.
    await call(driver, 'POST', 'auth/logout');
    await login(page, admin.username, admin.password);
    await openAdministration(page, 'Terminales');
    const request = page.locator('.pairing-request').filter({ hasText: name });
    await request
      .getByRole('textbox', { name: new RegExp(`^Código que muestra ${name}`) })
      .fill(pairing.code);
    await request.getByRole('button', { name: 'Aprobar', exact: true }).click();
    await expect(
      notice(page, `Terminal ${name} aprobada. Se conectará en unos segundos.`),
    ).toBeVisible();
    const steps = assistant(page, name);
    await expect(steps).toBeVisible();
    await expect(steps.getByText('1. Estaciones de esta pantalla')).toHaveAttribute(
      'aria-current',
      'step',
    );

    // 1. Sin estaciones que reciban en pantalla: crear «Cocina». Con impresoras de cocina activas en la base se
    // elige «Sin impresora: solo pantalla», como la configuración del propietario.
    await expect(steps).toContainText('No hay estaciones que reciban comandas en pantalla.');
    const create = steps.getByRole('button', { name: 'Crear «Cocina»', exact: true });
    await expect(create).toBeVisible();
    const printer = steps.getByRole('combobox', { name: 'Impresora de la estación' });
    if (await printer.isVisible()) {
      await chooseOption(printer, 'Sin impresora: solo pantalla');
    }
    await dismissNotices(page);
    await shot(page, testInfo, 'k11-asistente-estaciones');
    await create.click();
    await expect(notice(page, 'Estación Cocina creada.')).toBeVisible();
    await expect(steps).toContainText('Cocina · Solo pantalla de cocina');
    await expect(create).toHaveCount(0);

    // 2. Qué categorías llegan: las dos nuevas no tienen ruta.
    await steps.getByRole('button', { name: 'Siguiente', exact: true }).click();
    const breakfastBox = steps.getByRole('checkbox', { name: new RegExp(`^Desayunos ${suffix}`) });
    const burgersBox = steps.getByRole('checkbox', { name: new RegExp(`^Hamburguesas ${suffix}`) });
    await expect(breakfastBox).not.toBeChecked();
    await expect(steps.locator('label').filter({ hasText: `Desayunos ${suffix}` })).toContainText(
      'sin ruta',
    );
    await breakfastBox.check();
    await burgersBox.check();
    await dismissNotices(page);
    await shot(page, testInfo, 'k11-asistente-categorias');
    await steps.getByRole('button', { name: 'Guardar y seguir', exact: true }).click();
    await expect(notice(page, '2 categorías guardadas.')).toBeVisible();

    // 3. Resumen: lo que recibirá la pantalla y lo que no.
    await expect(steps.getByText('3. Resumen')).toHaveAttribute('aria-current', 'step');
    const summary = steps.getByLabel('Resumen de lo que recibe');
    await expect(summary).toContainText(`Desayunos ${suffix}`);
    await expect(summary).toContainText(`Hamburguesas ${suffix}`);
    await expect(summary).toContainText(/^Esta pantalla recibirá: /);
    await dismissNotices(page);
    await shot(page, testInfo, 'k11-asistente-resumen');
    await steps.getByRole('button', { name: 'Terminar', exact: true }).click();
    await expect(steps).toHaveCount(0);

    // Lo que quedó en Go: «Cocina» de solo pantalla con las dos categorías en su ruta.
    const resolved = await coverage(driver);
    const cocina = resolved.stations.find((station) => station.name === 'Cocina');
    expect(cocina).toMatchObject({ active: true, reception: 'kds' });
    expect(cocina!.categories.map((category) => category.id).sort()).toEqual(
      [breakfast.categoryID, burgers.categoryID].sort(),
    );
    for (const id of [breakfast.categoryID, burgers.categoryID]) {
      expect(resolved.categories.find((category) => category.id === id)?.station_ids).toEqual([
        cocina!.id,
      ]);
    }

    // La tableta reclama su credencial y, al conectarse, muestra Cocina (una pantalla nueva muestra todas).
    const claimed = await claimPairing(pairing);
    expect(claimed.status).toBe(200);
    const display = claimed.body as unknown as Display;
    expect((await asDisplay(display, 'POST', 'kitchen-display/connect', {})).status).toBe(200);
    const orders = await asDisplay(display, 'GET', 'kitchen-display/orders');
    expect(orders.status, JSON.stringify(orders.body)).toBe(200);
    expect(
      (orders.body.display.stations as { name: string }[]).map((station) => station.name),
    ).toContain('Cocina');
    // Revocada al terminar: no ocupa el cupo de pantalla de cocina de las siguientes especificaciones.
    await call(driver, 'POST', `devices/${display.device_id}/revoke`, {});
  } finally {
    for (const station of paused) {
      await patchStation(driver, station.id, { active: true });
    }
  }
});
