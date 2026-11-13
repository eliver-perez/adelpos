import { randomUUID } from 'node:crypto';
import { copyFileSync, mkdirSync } from 'node:fs';
import { join } from 'node:path';
import { expect, test } from '@playwright/test';
import type { Locator, Page, TestInfo } from '@playwright/test';
import { installDriver } from './native-driver';
import { appBar, cashButton, openSection, topBar } from './navigation';

// Etapa 9F, fase D (§6): barra superior compacta y leyenda del mapa de mesas. La barra se recorre a
// 1200 px (la ventana de unos 1500 px de Windows al 125 %, decisión D9), 1280, 1440, 1500 y 1920 px
// en ambos temas: sin desbordes ni desplazamiento horizontal, todos los botones visibles y tocables
// (≥ 44 px), el isotipo «AE» en la compacta, el estado del turno y «En línea» legibles y el nombre
// del usuario (largo a propósito) acortado con puntos suspensivos si no cabe; también «Sin
// conexión» a 1200 px. El logotipo no conserva el foco tras un clic (no se le enciende el contorno
// con la tecla siguiente) y lo muestra al llegar con el teclado. La leyenda del mapa se acomoda en
// dos renglones sin barra horizontal. Mismo transporte que las demás especificaciones (Go temporal y
// base adel_pos_test_*); el área, las mesas y el usuario se crean por la API con nombres propios.
// Capturas en ADEL_SHOTS_DIR si se indica.

type Driver = Awaited<ReturnType<typeof installDriver>>;

async function call(driver: Driver, method: string, route: string, body?: unknown) {
  const response = await driver.request(method, route, body);
  expect(response.status, `${method} ${route}: ${JSON.stringify(response.body)}`).toBe(200);
  return response.body;
}

const widths = [1200, 1280, 1440, 1500, 1920];
const themes = ['light', 'dark'] as const;
// Debajo de 1280 px la barra es la compacta.
const compactBelow = 1280;
const userName = 'María Guadalupe Hernández Villarreal';

function keep(testInfo: TestInfo, file: string) {
  const directory = process.env.ADEL_SHOTS_DIR;
  if (directory) {
    mkdirSync(directory, { recursive: true });
    copyFileSync(
      testInfo.outputPath(file),
      join(directory, `${process.env.ADEL_BROWSER_ENGINE ?? 'browser'}-${file}`),
    );
  }
}

// Aplica el tema y espera a que se pinte. La aplicación vuelve a aplicar el tema del usuario cuando
// se actualiza su sesión: quien llama comprueba después que el tema se conservó.
async function setTheme(page: Page, theme: string) {
  await page.evaluate((value) => {
    document.documentElement.dataset.theme = value;
  }, theme);
  await page.waitForTimeout(200);
}

async function themeKept(page: Page, theme: string) {
  return (await page.evaluate(() => document.documentElement.dataset.theme)) === theme;
}

// Cierra los avisos a la vista y espera a que terminen de salir (los de error no se cierran solos).
async function clearToasts(page: Page) {
  const close = page.locator('[data-sonner-toaster] .app-toast-close');
  for (let attempt = 0; attempt < 10 && (await close.count()) > 0; attempt += 1) {
    await close
      .first()
      .click({ timeout: 2000 })
      .catch(() => undefined);
    await page.waitForTimeout(150);
  }
  await expect(page.locator('[data-sonner-toast]')).toHaveCount(0, { timeout: 15000 });
}

// Sin desplazamiento horizontal en la página ni dentro de la barra.
async function expectNoHorizontalScroll(page: Page, label: string) {
  const overflow = await page.evaluate(() => ({
    page: document.documentElement.scrollWidth - document.documentElement.clientWidth,
    bar: (() => {
      const bar = document.querySelector<HTMLElement>('.app-topbar');
      return bar ? bar.scrollWidth - bar.clientWidth : 0;
    })(),
  }));
  expect(overflow.page, `${label}: la página se desplaza de lado`).toBeLessThanOrEqual(0);
  expect(overflow.bar, `${label}: la barra se desborda`).toBeLessThanOrEqual(0);
}

// Todos los botones de la barra a la vista, dentro de la ventana, de al menos 44 × 44 px y sin nada
// encima (el punto central de cada uno es el propio botón). Ningún texto cortado salvo el nombre y
// el rol del usuario, que se acortan con puntos suspensivos.
async function expectBarUsable(page: Page, label: string) {
  const report = await appBar(page).evaluate((root) => {
    const buttons = Array.from(root.querySelectorAll<HTMLElement>('button')).filter(
      (element) => element.offsetParent !== null,
    );
    const problems: string[] = [];
    for (const button of buttons) {
      const rect = button.getBoundingClientRect();
      const name = button.getAttribute('aria-label') ?? button.textContent ?? '';
      if (rect.width < 44 || rect.height < 44) {
        problems.push(`«${name}» mide ${Math.round(rect.width)} × ${Math.round(rect.height)}`);
      }
      if (rect.left < 0 || rect.right > window.innerWidth + 0.5) {
        problems.push(`«${name}» sale de la ventana (${Math.round(rect.left)}–${rect.right})`);
      }
      const hit = document.elementFromPoint(rect.left + rect.width / 2, rect.top + rect.height / 2);
      if (!hit || !button.contains(hit)) {
        problems.push(`«${name}» está tapado`);
      }
    }
    const cut = Array.from(
      root.querySelectorAll<HTMLElement>(
        'button span, button strong, button small, [role="status"]',
      ),
    )
      .filter((element) => element.offsetParent !== null)
      .filter((element) => !element.closest('.user-menu-identity'))
      .filter((element) => element.scrollWidth > element.clientWidth + 1)
      .map((element) => `${element.className} «${element.textContent}»`);
    // El nombre y el rol del usuario: si no caben, con puntos suspensivos.
    const identity = Array.from(
      root.querySelectorAll<HTMLElement>('.user-menu-identity strong, .user-menu-identity small'),
    )
      .filter((element) => element.offsetParent !== null)
      .filter((element) => element.scrollWidth > element.clientWidth + 1)
      .filter((element) => getComputedStyle(element).textOverflow !== 'ellipsis')
      .map((element) => element.textContent);
    return { count: buttons.length, problems, cut, identity };
  });
  expect(report.count, `${label}: botones de la barra`).toBeGreaterThanOrEqual(8);
  expect(report.problems, label).toEqual([]);
  expect(report.cut, `${label}: textos cortados`).toEqual([]);
  expect(report.identity, `${label}: el usuario se corta sin puntos suspensivos`).toEqual([]);
}

async function visibleText(locator: Locator) {
  return locator.evaluate((element) => {
    const visible = Array.from(element.querySelectorAll<HTMLElement>('*'))
      .filter((child) => child.childElementCount === 0 && child.offsetParent !== null)
      .map((child) => child.textContent?.trim() ?? '')
      .filter((text) => text !== '');
    return visible.join(' ');
  });
}

// Contenido de la barra en cada ancho: isotipo o logotipo completo, estado del turno (corto en la
// compacta), «En línea» con su texto y el nombre del usuario visible.
async function checkBarContent(page: Page, width: number) {
  const compact = width < compactBelow;
  const logo = page.locator('.topbar-logo');
  const isotype = logo.locator('img.brand-logo-compact');
  if (compact) {
    await expect(isotype).toBeVisible();
    await expect(isotype).toHaveAttribute('alt', 'AD-EL POS');
    expect(await isotype.evaluate((image: HTMLImageElement) => image.naturalWidth)).toBe(128);
    const size = await isotype.boundingBox();
    expect(Math.round(size!.width)).toBe(40);
    expect(Math.round(size!.height)).toBe(40);
    await expect(logo.locator('img.brand-logo-light')).toBeHidden();
    await expect(logo.locator('img.brand-logo-dark')).toBeHidden();
  } else {
    await expect(isotype).toBeHidden();
    const full =
      (await page.evaluate(() => document.documentElement.dataset.theme)) === 'dark'
        ? logo.locator('img.brand-logo-dark')
        : logo.locator('img.brand-logo-light');
    await expect(full).toBeVisible();
  }
  const cash = cashButton(page);
  await expect(cash).toHaveAttribute('title', /^Caja · Turno abierto/);
  expect(await visibleText(cash)).toBe(compact ? 'Caja Abierto' : 'Caja Turno abierto');
  const server = page.locator('.app-topbar .server-status');
  await expect(server.locator('.server-status-text')).toBeVisible();
  await expect(server.locator('.server-status-text')).toHaveText('En línea');
  await expect(server).toHaveAttribute('title', /^En línea: /);
  const name = page.locator('.app-topbar .user-menu-identity strong');
  await expect(name).toBeVisible();
  await expect(name).toHaveText(userName);
  // Aunque se acorte, se lee una parte del nombre.
  expect((await name.boundingBox())!.width).toBeGreaterThan(60);
}

// Leyenda del mapa: sin desplazamiento horizontal, cada estado con su muestra, su icono y su texto
// completos dentro del marco. Devuelve el número de renglones.
async function checkLegend(page: Page, label: string) {
  const legend = page.locator('.table-map-operation .map-legend');
  await expect(legend).toBeVisible();
  const report = await legend.evaluate((root) => {
    const frame = root.getBoundingClientRect();
    const items = Array.from(root.querySelectorAll<HTMLElement>('.map-legend-state'));
    const problems: string[] = [];
    for (const item of items) {
      const rect = item.getBoundingClientRect();
      if (rect.left < frame.left - 0.5 || rect.right > frame.right + 0.5) {
        problems.push(`«${item.textContent}» sale de la leyenda`);
      }
      if (item.scrollWidth > item.clientWidth + 1) {
        problems.push(`«${item.textContent}» cortado`);
      }
      if (!item.querySelector('svg[data-icon]')) {
        problems.push(`«${item.textContent}» sin icono`);
      }
    }
    const rows = new Set(items.map((item) => Math.round(item.getBoundingClientRect().top)));
    return {
      overflow: root.scrollWidth - root.clientWidth,
      overflowX: getComputedStyle(root).overflowX,
      texts: items.map((item) => item.textContent?.trim()),
      rows: rows.size,
      problems,
    };
  });
  expect(report.overflow, `${label}: la leyenda se desplaza de lado`).toBeLessThanOrEqual(0);
  expect(report.overflowX, label).toBe('visible');
  expect(report.problems, label).toEqual([]);
  expect(report.texts).toEqual([
    'Libre',
    'Ocupada',
    'Esperando pago',
    'Pago parcial',
    'Fuera de servicio',
    'Tiempo ocupada',
    'Capacidad (lugares)',
  ]);
  // La leyenda queda dentro del marco del mapa, debajo de las mesas.
  const frame = await page.locator('.table-map-operation .table-map-frame').boundingBox();
  const box = await legend.boundingBox();
  expect(box!.y + box!.height).toBeLessThanOrEqual(frame!.y + frame!.height + 1);
  return report.rows;
}

test('barra compacta y leyenda del mapa (etapa 9F, fase D)', async ({
  page,
  browserName,
}, testInfo) => {
  test.setTimeout(300000);
  const errors: string[] = [];
  page.on('pageerror', (error) => errors.push(error.message));
  const driver = await installDriver(page);
  await call(driver, 'POST', 'auth/login', { username: 'admin', password: 'admin' });
  await call(driver, 'PUT', 'terminal/preferences', { keyboard_mode: 'disabled' });

  // 1. Datos por la API: un área con mesas en varios estados posibles, el turno abierto (Caja ·
  // Turno abierto) y una usuaria con el rol de administración y un nombre largo.
  const area = (await call(driver, 'POST', 'restaurant/areas', {
    operation_id: randomUUID(),
    name: 'Salón 9F',
    active: true,
  })) as { id: string };
  for (let index = 0; index < 6; index += 1) {
    await call(driver, 'POST', 'restaurant/tables', {
      operation_id: randomUUID(),
      area_id: area.id,
      name: `Mesa 9F-${index + 1}`,
      shape: 'rectangle',
      x: 80 + (index % 3) * 220,
      y: 80 + Math.floor(index / 3) * 200,
      width: 140,
      height: 110,
      seats: 4,
      active: true,
    });
  }
  const current = await call(driver, 'GET', 'payments/shifts/current');
  if (!current.shift) {
    await call(driver, 'POST', 'payments/shifts', {
      operation_id: randomUUID(),
      opening_float: '0',
    });
  }
  const roles = (await call(driver, 'GET', 'roles')) as { id: string; code: string }[];
  const administrator = roles.find((role) => role.code === 'administrator');
  expect(administrator).toBeDefined();
  const password = 'clave-barra-9f-d';
  await call(driver, 'POST', 'users', {
    username: 'barra9fd',
    display_name: userName,
    role_id: administrator!.id,
    active: true,
    password,
  });
  await driver.request('POST', 'auth/logout');

  // Puente de la prueba: la terminal sin conexión a pedido (el puente rechaza las peticiones).
  await page.addInitScript(() => {
    const bridge = window as unknown as {
      __TAURI_INTERNALS__: {
        invoke: (command: string, args?: Record<string, unknown>) => Promise<unknown>;
      };
      __adelOffline?: boolean;
    };
    const original = bridge.__TAURI_INTERNALS__.invoke;
    bridge.__TAURI_INTERNALS__.invoke = async (command, args) => {
      if (command === 'api_request' && bridge.__adelOffline) {
        throw 'No se pudo conectar con el equipo servidor.';
      }
      return original(command, args);
    };
  });

  await page.setViewportSize({ width: 1280, height: 720 });
  await page.goto('/');
  // Atajos con ⌘ en macOS (también el WebKit de Playwright en Windows, que se anuncia como Mac) y
  // Ctrl en Windows; en WebKit, Tab solo recorre los botones con Opción (como en Safari).
  const mac = await page.evaluate(() =>
    /Mac|iPhone|iPad/.test(`${navigator.platform} ${navigator.userAgent}`),
  );
  const mod = mac ? 'Meta' : 'Control';
  const backTab = browserName === 'webkit' ? 'Shift+Alt+Tab' : 'Shift+Tab';
  await page.getByLabel('Usuario', { exact: true }).fill('barra9fd');
  await page.getByLabel('Contraseña', { exact: true }).fill(password);
  await page.getByRole('button', { name: 'Entrar', exact: true }).click();
  await expect(cashButton(page)).toHaveAttribute('data-shift-state', 'open', { timeout: 20000 });
  await expect(page.locator('.app-topbar .server-status-online')).toBeVisible({ timeout: 15000 });

  // 2. Barra superior en los cinco anchos y ambos temas.
  for (const width of widths) {
    await page.setViewportSize({ width, height: 800 });
    for (const theme of themes) {
      const file = `barra-${theme}-${width}.png`;
      for (let attempt = 0; attempt < 3; attempt += 1) {
        await setTheme(page, theme);
        const label = `barra a ${width} px (${theme})`;
        await expectNoHorizontalScroll(page, label);
        await expectBarUsable(page, label);
        await checkBarContent(page, width);
        await appBar(page).screenshot({ path: testInfo.outputPath(file) });
        if (await themeKept(page, theme)) {
          break;
        }
      }
      keep(testInfo, file);
    }
  }
  await setTheme(page, 'light');

  // 3. Foco del logotipo a 1200 px. Tras un clic el logotipo no conserva el foco: la tecla siguiente
  // (Escape o un atajo) no le enciende el contorno, que antes aparecía en Chromium (WebView2).
  await page.setViewportSize({ width: 1200, height: 800 });
  const logo = page.getByRole('button', { name: /^AD-EL POS · Ir al inicio/ });
  const logoFocus = () =>
    logo.evaluate((element) => ({
      focused: document.activeElement === element,
      visible: element.matches(':focus-visible'),
      outline: getComputedStyle(element).outlineStyle,
    }));
  const noRing = { focused: false, visible: false, outline: 'none' };
  await topBar(page).getByRole('button', { name: 'Administración', exact: true }).click();
  await expect(
    topBar(page).getByRole('button', { name: 'Administración', exact: true }),
  ).toHaveAttribute('aria-current', 'page');
  await logo.click();
  await expect(
    topBar(page).getByRole('button', { name: 'Administración', exact: true }),
  ).not.toHaveAttribute('aria-current', 'page');
  expect(await logoFocus()).toEqual(noRing);
  await page.keyboard.press('Escape');
  await page.keyboard.press('ArrowDown');
  expect(await logoFocus()).toEqual(noRing);
  await page.keyboard.press(`${mod}+2`);
  await expect(
    topBar(page).getByRole('button', { name: 'Para llevar', exact: true }),
  ).toHaveAttribute('aria-current', 'page');
  expect(await logoFocus()).toEqual(noRing);
  // La paleta devuelve el foco al control anterior al cerrarse: tampoco al logotipo.
  await logo.click();
  await page.keyboard.press(`${mod}+k`);
  await expect(page.getByRole('dialog', { name: /Paleta de comandos/ })).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(page.getByRole('dialog', { name: /Paleta de comandos/ })).toHaveCount(0);
  expect(await logoFocus()).toEqual(noRing);
  // Con el teclado sí: Mayúsculas+Tab desde Mesas llega al logotipo con su contorno, e Intro vuelve
  // al inicio conservando el foco y el contorno.
  await openSection(page, 'Para llevar');
  await topBar(page).getByRole('button', { name: 'Mesas', exact: true }).focus();
  await page.keyboard.press(backTab);
  const ring = { focused: true, visible: true, outline: 'solid' };
  expect(await logoFocus()).toEqual(ring);
  await page.keyboard.press('Enter');
  await expect(
    topBar(page).getByRole('button', { name: 'Para llevar', exact: true }),
  ).not.toHaveAttribute('aria-current', 'page');
  expect(await logoFocus()).toEqual(ring);

  // 4. «Sin conexión» a 1200 px: el aviso rojo con su texto, sin desbordes; solo el nombre del
  // usuario cede espacio.
  await openSection(page, 'Mesas');
  await page.evaluate(() => {
    (window as unknown as { __adelOffline: boolean }).__adelOffline = true;
  });
  const offline = page.locator('.app-topbar .server-status-offline');
  await expect(offline).toHaveText('Sin conexión', { timeout: 15000 });
  await expect(offline).toHaveAttribute('title', /^Sin conexión: /);
  // El aviso de error no se cierra solo y queda sobre la barra: se cierra para comprobarla.
  await clearToasts(page);
  for (const theme of themes) {
    await setTheme(page, theme);
    await expectNoHorizontalScroll(page, `sin conexión (${theme})`);
    await expectBarUsable(page, `sin conexión (${theme})`);
    await appBar(page).screenshot({ path: testInfo.outputPath(`barra-sin-conexion-${theme}.png`) });
    keep(testInfo, `barra-sin-conexion-${theme}.png`);
  }
  await setTheme(page, 'light');
  await page.evaluate(() => {
    (window as unknown as { __adelOffline: boolean }).__adelOffline = false;
  });
  await page.getByRole('button', { name: 'Reintentar', exact: true }).click();
  await expect(page.locator('.app-topbar .server-status-online')).toBeVisible({ timeout: 15000 });

  // 5. Leyenda del mapa (§6.2): sin barra horizontal en ningún ancho; a 1200 px se acomoda en dos
  // renglones y a 1920 px cabe en uno.
  await page
    .getByRole('group', { name: 'Áreas del restaurante' })
    .getByRole('button', { name: 'Salón 9F', exact: true })
    .click();
  await expect(
    page.locator('.table-canvas[aria-label="Mapa de mesas"]').getByRole('button', {
      name: /^Mesa 9F-1, /,
    }),
  ).toBeVisible();
  const rows: Record<number, number> = {};
  for (const width of widths) {
    await page.setViewportSize({ width, height: 800 });
    for (const theme of themes) {
      await setTheme(page, theme);
      const label = `leyenda a ${width} px (${theme})`;
      await expectNoHorizontalScroll(page, label);
      rows[width] = await checkLegend(page, label);
      const file = `leyenda-${theme}-${width}.png`;
      await page.locator('.table-map-operation .table-map-frame').screenshot({
        path: testInfo.outputPath(file),
      });
      keep(testInfo, file);
    }
  }
  await setTheme(page, 'light');
  expect(rows[1200]).toBeGreaterThanOrEqual(2);
  expect(rows[1920]).toBe(1);
  await page.setViewportSize({ width: 1280, height: 720 });
  expect(errors).toEqual([]);
});
