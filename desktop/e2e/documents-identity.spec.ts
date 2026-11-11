import { deflateSync } from 'node:zlib';
import { expect, test } from '@playwright/test';
import type { Page, TestInfo } from '@playwright/test';
import { installDriver } from './native-driver';
import { notice } from './notices';

// Etapa 9B, fase B2: pestaña «Identidad del establecimiento» de la pantalla Documentos. Sube un
// PNG de prueba (generado aquí, sin archivos externos), comprueba la vista previa del original y de
// cómo saldrá en papel de 58 y 80 mm (Go convierte y guarda las conversiones), cambia la
// presentación con controles táctiles, la guarda, retira el logotipo con confirmación y comprueba
// que no hay desbordamiento horizontal a 1280 × 720 y 1920 × 1080 en ambos temas. No imprime: el
// resultado en papel es una prueba física pendiente.

// crc32 de PNG (polinomio 0xEDB88320).
function crc32(data: Buffer) {
  let crc = 0xffffffff;
  for (const byte of data) {
    crc ^= byte;
    for (let bit = 0; bit < 8; bit++) {
      crc = crc & 1 ? (crc >>> 1) ^ 0xedb88320 : crc >>> 1;
    }
  }
  return (crc ^ 0xffffffff) >>> 0;
}

function chunk(type: string, data: Buffer) {
  const length = Buffer.alloc(4);
  length.writeUInt32BE(data.length);
  const body = Buffer.concat([Buffer.from(type, 'latin1'), data]);
  const checksum = Buffer.alloc(4);
  checksum.writeUInt32BE(crc32(body));
  return Buffer.concat([length, body, checksum]);
}

// Logotipo de prueba de 240 × 96: un marco negro con una franja central sobre fondo blanco.
function testLogo() {
  const width = 240;
  const height = 96;
  const rows: Buffer[] = [];
  for (let y = 0; y < height; y++) {
    const row = Buffer.alloc(1 + width * 3, 0xff);
    row[0] = 0;
    for (let x = 0; x < width; x++) {
      const frame = x < 8 || x >= width - 8 || y < 8 || y >= height - 8;
      const stripe = y >= 40 && y < 56 && x >= 30 && x < width - 30;
      if (frame || stripe) {
        row.fill(0x10, 1 + x * 3, 1 + x * 3 + 3);
      }
    }
    rows.push(row);
  }
  const header = Buffer.alloc(13);
  header.writeUInt32BE(width, 0);
  header.writeUInt32BE(height, 4);
  header[8] = 8;
  header[9] = 2;
  return Buffer.concat([
    Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]),
    chunk('IHDR', header),
    chunk('IDAT', deflateSync(Buffer.concat(rows))),
    chunk('IEND', Buffer.alloc(0)),
  ]);
}

async function setTheme(page: Page, theme: string) {
  await page.evaluate((value) => {
    document.documentElement.dataset.theme = value;
  }, theme);
  await expect(page.locator('html')).toHaveAttribute('data-theme', theme);
}

async function expectNoOverflow(page: Page, testInfo: TestInfo, label: string) {
  for (const size of [
    { width: 1280, height: 720 },
    { width: 1920, height: 1080 },
  ]) {
    await page.setViewportSize(size);
    for (const theme of ['light', 'dark']) {
      await setTheme(page, theme);
      const overflow = await page.evaluate(
        () => document.documentElement.scrollWidth - window.innerWidth,
      );
      expect(overflow, `${label} se desborda a ${size.width} × ${size.height} (${theme})`).toBe(0);
      await page.screenshot({
        path: testInfo.outputPath(`identity-${label}-${theme}-${size.width}x${size.height}.png`),
        fullPage: true,
      });
    }
  }
  await setTheme(page, 'light');
  await page.setViewportSize({ width: 1280, height: 720 });
}

test('identidad 9B: subir logotipo, vista previa 58/80, presentación, retiro y sin desbordamiento', async ({
  page,
}, testInfo) => {
  test.setTimeout(180000);
  const errors: string[] = [];
  page.on('pageerror', (error) => errors.push(error.message));
  const driver = await installDriver(page);
  await driver.request('POST', 'auth/login', { username: 'admin', password: 'admin' });
  const keyboard = await driver.request('PUT', 'terminal/preferences', {
    keyboard_mode: 'disabled',
  });
  expect(keyboard.status, JSON.stringify(keyboard.body)).toBeLessThan(300);
  await driver.request('POST', 'auth/logout');
  await page.setViewportSize({ width: 1280, height: 720 });
  await page.goto('/');
  await page.getByLabel('Usuario', { exact: true }).fill('admin');
  await page.getByLabel('Contraseña', { exact: true }).fill('admin');
  await page.getByRole('button', { name: 'Entrar', exact: true }).click();
  await page
    .getByRole('navigation', { name: 'Navegación principal' })
    .getByRole('button', { name: 'Documentos', exact: true })
    .click();
  await page.getByRole('tab', { name: 'Identidad del establecimiento', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Datos del establecimiento' })).toBeVisible();
  await expect(
    page.getByText(/La marca AD-EL POS de la aplicación no cambia/).first(),
  ).toBeVisible();
  await expect(page.getByText(/Sin logotipo: el bloque «Logotipo» no imprime nada/)).toBeVisible();
  await expectNoOverflow(page, testInfo, 'sin-logotipo');

  // 1. Subir el PNG: Go valida, guarda el original y convierte a 58 y 80 mm.
  await page.getByLabel('Subir logotipo', { exact: true }).setInputFiles({
    name: 'logotipo-prueba.png',
    mimeType: 'image/png',
    buffer: testLogo(),
  });
  await expect(notice(page, /Logotipo cargado/)).toBeVisible({ timeout: 30000 });
  await expect(page.getByAltText('Logotipo original del restaurante')).toBeVisible();
  const paper58 = page.getByAltText('Logotipo convertido para papel de 58 mm');
  const paper80 = page.getByAltText('Logotipo convertido para papel de 80 mm');
  await expect(paper58).toBeVisible();
  await expect(paper80).toBeVisible();
  // 60 % del ancho imprimible: 230 × 92 puntos en 384 y 346 × 138 en 576 (con el margen blanco).
  await expect(page.getByText('384 × 92 puntos', { exact: true })).toBeVisible();
  await expect(page.getByText('576 × 138 puntos', { exact: true })).toBeVisible();
  const box58 = await paper58.boundingBox();
  const box80 = await paper80.boundingBox();
  expect(box58 && box80 && box80.width > box58.width).toBe(true);
  await expectNoOverflow(page, testInfo, 'con-logotipo');

  // 2. Cambiar la presentación: vista previa sin guardar y después guardada.
  await page
    .getByRole('group', { name: 'Alineación del logotipo' })
    .getByRole('button', { name: 'Derecha' })
    .click();
  await page.getByRole('button', { name: '100 %', exact: true }).click();
  await page
    .getByRole('group', { name: 'Método de conversión' })
    .getByRole('button', { name: 'Difuminado' })
    .click();
  await expect(page.getByText(/Vista previa sin guardar/)).toBeVisible({ timeout: 15000 });
  await expect(page.getByText('384 × 154 puntos', { exact: true })).toBeVisible();
  await page.getByRole('button', { name: 'Guardar cambios', exact: true }).click();
  await expect(notice(page, 'Identidad del establecimiento guardada.')).toBeVisible({
    timeout: 30000,
  });
  await expect(page.getByText('Presentación guardada.')).toBeVisible();
  await expect(page.getByText('Sin cambios pendientes.')).toBeVisible();
  const saved = await driver.request('GET', 'documents/identity');
  expect(saved.status).toBe(200);
  const presentation = (saved.body as { presentation: Record<string, unknown> }).presentation;
  expect(presentation).toMatchObject({ width_percent: 100, align: 'right', method: 'dither' });

  // 3. Retirar con confirmación: los documentos ya emitidos conservan su logotipo.
  await page.getByRole('button', { name: 'Retirar logotipo', exact: true }).click();
  const dialog = page.getByRole('alertdialog');
  await expect(dialog.getByText(/conservan su logotipo al reimprimirse/)).toBeVisible();
  await dialog.getByRole('button', { name: 'Retirar logotipo', exact: true }).click();
  await expect(notice(page, /Logotipo retirado/)).toBeVisible({ timeout: 30000 });
  await expect(page.getByAltText('Logotipo original del restaurante')).toHaveCount(0);
  await expect(page.getByText(/Sin logotipo: el bloque «Logotipo» no imprime nada/)).toBeVisible();
  await expectNoOverflow(page, testInfo, 'retirado');
  expect(errors).toEqual([]);
});
