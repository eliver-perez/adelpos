import { expect } from '@playwright/test';
import type { Locator, Page } from '@playwright/test';

// Avisos del sistema único de mensajes (Sonner, etapa 7.1). Viven en la pila fija de la página,
// arriba al centro y debajo de la cabecera, fuera del panel que originó la acción.
export function notice(page: Page, text: string | RegExp): Locator {
  return page.locator('[data-sonner-toaster]').getByText(text);
}

// Indica si el aviso dejó de moverse: dos mediciones separadas por 150 ms coinciden.
async function settled(page: Page, toast: Locator) {
  const first = await toast.boundingBox();
  await page.waitForTimeout(150);
  const second = await toast.boundingBox();
  if (!first || !second) {
    return false;
  }
  return (
    first.x === second.x &&
    first.y === second.y &&
    first.width === second.width &&
    first.height === second.height
  );
}

// Comprueba la presentación de un aviso visible: arriba al centro, por debajo de la barra superior
// (o, durante la captura, de su barra reducida; etapa 9C), con un botón de cierre táctil de al
// menos 44 px que lo retira con un toque.
export async function expectNoticeLayout(page: Page, text: string | RegExp) {
  const message = notice(page, text);
  await expect(message).toBeVisible();
  const toast = page.locator('[data-sonner-toaster] .app-toast').filter({ hasText: text });
  const header = await page
    .locator('.app-topbar, .capture-topbar')
    .filter({ visible: true })
    .first()
    .boundingBox();
  // La entrada del aviso es animada (se desliza y escala al pasar al frente de la pila): se mide
  // cuando terminó. Si el aviso aparece justo al cerrarse un panel, la medición inmediata lo
  // encontraría a medio camino.
  await expect(page.locator('[data-sonner-toast]').filter({ hasText: text })).toHaveAttribute(
    'data-mounted',
    'true',
  );
  await expect.poll(async () => settled(page, toast)).toBe(true);
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

// Registro de la vida de los avisos durante todo un recorrido (etapa 7.1, fase D). La página
// observa la pila cada 100 ms: anota cuándo apareció cada aviso, cuándo Sonner empezó a retirarlo
// y, mientras otro más reciente lo tapa, si su contenido quedó oculto (solo debe asomar el borde de
// su tarjeta). Se instala antes de abrir la página.
interface NoticeLife {
  tone: string;
  text: string;
  first: number;
  removed: number | null;
  last: number;
}

interface NoticeWatch {
  lives: NoticeLife[];
  stackedSamples: number;
  exposed: string[];
}

export async function watchNotices(page: Page) {
  await page.addInitScript(() => {
    const lives = new WeakMap<Element, NoticeLife>();
    const watch: NoticeWatch = { lives: [], stackedSamples: 0, exposed: [] };
    (window as unknown as { __adelNotices: NoticeWatch }).__adelNotices = watch;
    window.setInterval(() => {
      const now = performance.now();
      for (const frame of Array.from(document.querySelectorAll('[data-sonner-toast]'))) {
        const toast = frame.querySelector('.app-toast');
        if (!toast) {
          continue;
        }
        let life = lives.get(frame);
        if (!life) {
          life = {
            tone: toast.getAttribute('data-tone') ?? '',
            text: frame.querySelector('.app-toast-message')?.textContent ?? '',
            first: now,
            removed: null,
            last: now,
          };
          lives.set(frame, life);
          watch.lives.push(life);
        }
        life.last = now;
        const removed = frame.getAttribute('data-removed') === 'true';
        if (removed && life.removed === null) {
          life.removed = now;
        }
        const covered =
          !removed &&
          frame.getAttribute('data-mounted') === 'true' &&
          frame.getAttribute('data-front') === 'false' &&
          frame.getAttribute('data-expanded') === 'false';
        if (covered) {
          watch.stackedSamples += 1;
          const content = frame.querySelector('.app-toast-content');
          const visible = content && getComputedStyle(content).visibility !== 'hidden';
          if (visible && !watch.exposed.includes(life.text)) {
            watch.exposed.push(life.text);
          }
        }
      }
    }, 100);
  });
}

// Cada aviso de éxito empieza a retirarse a sus 4 s aunque otros más recientes lo tapen (margen
// para la animación de entrada y el muestreo; un éxito idéntico repetido lo sustituiría y
// reiniciaría su tiempo, cosa que este recorrido no hace), y un aviso tapado nunca deja ver su
// texto ni sus botones debajo del más reciente.
export async function expectNoticesClosedOnTime(page: Page) {
  const watch = await page.evaluate(
    () => (window as unknown as { __adelNotices: NoticeWatch }).__adelNotices,
  );
  const successes = watch.lives.filter((life) => life.tone === 'success');
  expect(successes.length).toBeGreaterThan(10);
  expect(watch.stackedSamples).toBeGreaterThan(0);
  expect(watch.exposed).toEqual([]);
  for (const life of successes) {
    const shown = (life.removed ?? life.last) - life.first;
    expect(shown, `El aviso «${life.text}» siguió visible ${Math.round(shown)} ms`).toBeLessThan(
      5000,
    );
  }
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
