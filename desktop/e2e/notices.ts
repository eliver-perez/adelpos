import { expect } from '@playwright/test';
import type { Locator, Page } from '@playwright/test';

// Avisos del sistema único de mensajes (Sonner, etapa 7.1). Viven en la pila fija de la página,
// arriba al centro y debajo de la cabecera, fuera del panel que originó la acción.
export function notice(page: Page, text: string | RegExp): Locator {
  return page.locator('[data-sonner-toaster]').getByText(text);
}

// Comprueba la presentación de un aviso visible: arriba al centro, por debajo de la cabecera,
// con un botón de cierre táctil de al menos 44 px que lo retira con un toque.
export async function expectNoticeLayout(page: Page, text: string | RegExp) {
  const message = notice(page, text);
  await expect(message).toBeVisible();
  const toast = page.locator('[data-sonner-toaster] .app-toast').filter({ hasText: text });
  const header = await page.locator('.application-header').boundingBox();
  const box = await toast.boundingBox();
  const viewport = page.viewportSize();
  expect(header).not.toBeNull();
  expect(box).not.toBeNull();
  expect(viewport).not.toBeNull();
  if (!header || !box || !viewport) {
    return;
  }
  // Debajo de la cabecera y centrado en la ventana (tolerancia de unos píxeles).
  expect(box.y).toBeGreaterThanOrEqual(header.y + header.height - 1);
  const center = box.x + box.width / 2;
  expect(Math.abs(center - viewport.width / 2)).toBeLessThan(8);
  const close = toast.getByRole('button', { name: 'Cerrar aviso' });
  const closeBox = await close.boundingBox();
  expect(closeBox?.width ?? 0).toBeGreaterThanOrEqual(44);
  expect(closeBox?.height ?? 0).toBeGreaterThanOrEqual(44);
  await close.click();
  await expect(message).toHaveCount(0);
}

// En pantalla táctil el puntero queda donde fue el último toque. Un aviso de éxito que aparece
// bajo ese punto no despliega la pila ni detiene su cierre automático (unos 4 s).
export async function expectNoticeClosesUnderPointer(page: Page, text: string | RegExp) {
  const message = notice(page, text);
  await expect(message).toBeVisible();
  const box = await message.boundingBox();
  expect(box).not.toBeNull();
  if (!box) {
    return;
  }
  await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
  const frame = page.locator('[data-sonner-toast]').filter({ hasText: text });
  await expect(frame).toHaveAttribute('data-expanded', 'false');
  await expect(message).toHaveCount(0, { timeout: 7000 });
}
