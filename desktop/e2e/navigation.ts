import { expect } from '@playwright/test';
import type { Locator, Page } from '@playwright/test';

// Navegación de la etapa 9C para los recorridos: barra superior fija (Mesas, Para llevar, Cocina y
// Administración a la izquierda; desde la etapa 9D, Caja a la derecha junto al servidor y el
// usuario), mosaicos de Administración con Personal y Sistema como segunda pantalla de mosaicos, y
// la barra reducida de la captura. Todos los recorridos llegan a cada pantalla por aquí para no
// depender de un menú lateral.

export type BarSection = 'Mesas' | 'Para llevar' | 'Cocina';

export type AdministrationScreen =
  | 'Catálogo'
  | 'Inventarios'
  | 'Reportes'
  | 'Documentos'
  | 'Cobros'
  | 'Personal'
  | 'Sistema'
  | 'Usuarios'
  | 'Roles'
  | 'Sesiones'
  | 'Configuración'
  | 'Terminales'
  | 'Cajas físicas'
  | 'Registro de movimientos';

// Mosaicos que se tocan desde Administración para llegar a cada pantalla.
const administrationPaths: Record<AdministrationScreen, string[]> = {
  Catálogo: ['Catálogo'],
  Inventarios: ['Inventarios'],
  Reportes: ['Reportes'],
  Documentos: ['Documentos'],
  Cobros: ['Cobros'],
  Personal: ['Personal'],
  Sistema: ['Sistema'],
  Usuarios: ['Personal', 'Usuarios'],
  Roles: ['Personal', 'Roles'],
  Sesiones: ['Personal', 'Sesiones'],
  Configuración: ['Sistema', 'Configuración'],
  Terminales: ['Sistema', 'Terminales'],
  'Cajas físicas': ['Sistema', 'Cajas físicas'],
  'Registro de movimientos': ['Sistema', 'Registro de movimientos'],
};

export function topBar(page: Page): Locator {
  return page.getByRole('navigation', { name: 'Navegación principal' });
}

export function captureBar(page: Page): Locator {
  return page.locator('.capture-topbar');
}

// Barra superior completa: secciones a la izquierda y Caja, servidor, usuario y pantalla completa a
// la derecha (etapa 9D, §2).
export function appBar(page: Page): Locator {
  return page.locator('.app-topbar');
}

// Botón Caja de la barra superior (su nombre accesible incluye el estado del turno). Desde la etapa
// 9D está a la derecha, fuera del grupo de secciones.
export function cashButton(page: Page): Locator {
  return appBar(page).getByRole('button', { name: /^Caja/ });
}

// Sale de la captura con «‹ Mesas» (o «‹ Para llevar»/«‹ Cuentas») si la barra reducida está a la
// vista: la barra superior completa solo existe fuera de la captura (§1.2).
export async function leaveCapture(page: Page) {
  const back = captureBar(page).locator('.capture-topbar-back');
  if (await back.isVisible()) {
    await back.click();
  }
  await expect(topBar(page)).toBeVisible();
}

export async function openSection(page: Page, section: BarSection) {
  await leaveCapture(page);
  await topBar(page).getByRole('button', { name: section, exact: true }).click();
}

// «Caja» abre el menú rápido de la 7.1; «Ir a Caja» abre la pantalla. Funciona también desde la
// barra reducida de la captura (sin salir de ella), como en la operación real.
export async function openCash(page: Page) {
  const bar = (await captureBar(page).isVisible()) ? captureBar(page) : appBar(page);
  await bar.getByRole('button', { name: /^Caja · / }).click();
  await page
    .getByRole('group', { name: 'Menú rápido de caja' })
    .getByRole('button', { name: 'Ir a Caja', exact: true })
    .click();
}

export async function openAdministration(page: Page, target: AdministrationScreen) {
  await leaveCapture(page);
  await topBar(page).getByRole('button', { name: 'Administración', exact: true }).click();
  for (const tile of administrationPaths[target]) {
    await page.locator('.admin-tiles').getByRole('button', { name: tile, exact: true }).click();
  }
}

// Vuelve a la captura de la cuenta seleccionada en el panel lateral de la mesa.
export async function returnToTableCapture(page: Page, table: string, heading: string | RegExp) {
  await openSection(page, 'Mesas');
  const panel = page.getByRole('complementary', { name: `Panel de ${table}` });
  await panel.getByRole('button', { name: 'Capturar pedidos en esta mesa', exact: true }).click();
  await expect(page.getByRole('heading', { name: heading })).toBeVisible();
}
