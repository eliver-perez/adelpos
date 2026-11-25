import { execFileSync } from 'node:child_process';
import { realpathSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { expect, test } from '@playwright/test';
import type { Page } from '@playwright/test';
import { installDriver } from './native-driver';
import {
  admin,
  call,
  enrollTerminal,
  lightTheme,
  login,
  sharedServerURL,
  shot,
  terminalPage,
} from './license-flow';
import { topBar } from './navigation';

// Etapa K1, Fase E (§7 de la especificación y K1-D10): el «Reconectando» falso de la 0.15.0. Con Go y PostgreSQL reales,
// el escritorio sin hacer nada más de un minuto (dos renovaciones normales del canal de eventos, que Go cierra cada 30 s)
// sigue «En línea» sin un solo parpadeo; con el Go temporal de la prueba pausado de 1 a 5 s (kill -STOP y -CONT, como un
// corte breve de la red) tampoco se ve nada; pausado unos 40 s sí aparece «Sin conexión» con su capa y se recupera solo
// al reanudarlo. Lo mismo en una terminal por la red (sin servidor propio).
//
// Solo se pausa el proceso del Go temporal de esta corrida: el único que escucha en el puerto de la prueba, que no es
// 8181 ni 8443 y cuyo programa es bin/ad-el-pos-server de este árbol con «serve». Siempre se reanuda: al terminar cada
// pausa, en afterEach y al salir el proceso de Playwright.

const repository = realpathSync(fileURLToPath(new URL('../..', import.meta.url)));
const pausedServers = new Set<number>();

function resumeServers() {
  for (const pid of pausedServers) {
    try {
      process.kill(pid, 'SIGCONT');
    } catch {
      // El proceso ya terminó.
    }
    pausedServers.delete(pid);
  }
}

process.once('exit', resumeServers);
test.afterEach(resumeServers);

function output(command: string, args: string[]) {
  return execFileSync(command, args, { encoding: 'utf8' }).trim();
}

// El PID del Go temporal de la prueba, verificado (nunca el servicio de la Mac ni otro proceso).
function temporaryServerPID() {
  const port = Number(new URL(sharedServerURL).port);
  if (!Number.isInteger(port) || port <= 0 || port === 8181 || port === 8443) {
    throw new Error(`El puerto ${port} no es el de un Go temporal de pruebas.`);
  }
  const listeners = output('lsof', ['-nP', `-iTCP:${port}`, '-sTCP:LISTEN', '-t']).split('\n');
  if (listeners.length !== 1) {
    throw new Error(`Se esperaba un solo proceso escuchando en ${port}: ${listeners.join(', ')}`);
  }
  const pid = Number(listeners[0]);
  const command = output('ps', ['-o', 'command=', '-p', String(pid)]);
  if (!command.startsWith(`${repository}/bin/ad-el-pos-server serve `)) {
    throw new Error(`El proceso ${pid} no es el Go temporal de este árbol: ${command}`);
  }
  const ports = output('lsof', ['-nP', '-a', '-p', String(pid), '-iTCP', '-sTCP:LISTEN']);
  if (/:(8181|8443)\b/.test(ports)) {
    throw new Error(`El proceso ${pid} escucha en un puerto del servicio real.`);
  }
  return pid;
}

// Pausa el Go temporal `milliseconds` y lo reanuda (también si la espera falla).
async function pauseServer(pid: number, milliseconds: number, during?: () => Promise<void>) {
  pausedServers.add(pid);
  process.kill(pid, 'SIGSTOP');
  const started = Date.now();
  try {
    if (during) {
      await during();
    }
    const left = milliseconds - (Date.now() - started);
    if (left > 0) {
      await new Promise((resolve) => setTimeout(resolve, left));
    }
  } finally {
    process.kill(pid, 'SIGCONT');
    pausedServers.delete(pid);
  }
}

// Registro en la página de todo lo que no sea «En línea»: el indicador de la barra y la capa «Sin conexión con el
// servidor», revisados cada 100 ms (un parpadeo de un instante también cuenta).
async function watchLink(page: Page) {
  await page.evaluate(() => {
    const target = window as unknown as { __adelLinkLog?: string[]; __adelLinkWatch?: number };
    target.__adelLinkLog = [];
    if (target.__adelLinkWatch !== undefined) {
      window.clearInterval(target.__adelLinkWatch);
    }
    target.__adelLinkWatch = window.setInterval(() => {
      const states = Array.from(document.querySelectorAll<HTMLElement>('.server-status'))
        .map((element) => element.dataset.state ?? '')
        .filter((state) => state !== 'online');
      const layer = document.getElementById('connection-lost-title') !== null;
      if (states.length > 0 || layer) {
        target.__adelLinkLog!.push(
          `${new Date().toISOString()} ${states.join(',')}${layer ? ' capa' : ''}`,
        );
      }
    }, 100);
  });
}

async function linkLog(page: Page) {
  return page.evaluate(
    () => (window as unknown as { __adelLinkLog?: string[] }).__adelLinkLog ?? [],
  );
}

// El indicador de la barra superior (fuera de la navegación principal, junto al menú del usuario).
function status(page: Page) {
  return page.locator('.app-topbar .server-status');
}

async function expectOnline(page: Page) {
  await expect(status(page)).toHaveAttribute('data-state', 'online');
}

// Pausas breves (1, 3 y 5 s): ni la barra ni la capa cambian, tampoco en los 12 s siguientes (cuando vencerían las
// peticiones que quedaron esperando).
async function shortOutages(page: Page, pid: number) {
  for (const seconds of [1, 3, 5]) {
    await watchLink(page);
    await pauseServer(pid, seconds * 1000);
    await page.waitForTimeout(12_000);
    expect(await linkLog(page), `pausa de ${seconds} s`).toEqual([]);
    await expectOnline(page);
  }
}

// Una pausa larga: «Sin conexión» con su capa mientras dura; al reanudar, «En línea» y la capa se va sola.
async function longOutage(
  page: Page,
  pid: number,
  testInfo: Parameters<typeof shot>[1],
  name: string,
) {
  const layer = page.getByRole('alertdialog', { name: 'Sin conexión con el servidor' });
  await pauseServer(pid, 42_000, async () => {
    // 3 fallas seguidas y 20 s desde la primera: las peticiones vencen a los 12 s y el canal mudo a los 10 s.
    await expect(status(page)).toHaveAttribute('data-state', 'offline', { timeout: 40_000 });
    await expect(layer).toBeVisible();
    await shot(page, testInfo, name, { fullPage: false });
  });
  await expect(status(page)).toHaveAttribute('data-state', 'online', { timeout: 20_000 });
  await expect(layer).toBeHidden();
}

test('el escritorio sin hacer nada y con cortes breves sigue En línea; un corte largo avisa y se recupera (etapa K1)', async ({
  page,
}, testInfo) => {
  test.setTimeout(300_000);
  await page.setViewportSize({ width: 1280, height: 800 });
  const driver = await installDriver(page);
  await call(driver, 'POST', 'auth/login', admin);
  await lightTheme(driver);
  await call(driver, 'POST', 'auth/logout');
  const pid = temporaryServerPID();

  await login(page, admin.username, admin.password);
  await expect(topBar(page)).toBeVisible();
  await expectOnline(page);

  // Más de un minuto sin tocar nada: Go renueva el canal dos veces y la barra no parpadea.
  await watchLink(page);
  await page.waitForTimeout(65_000);
  expect(await linkLog(page), 'un minuto sin hacer nada').toEqual([]);
  await expectOnline(page);

  await shortOutages(page, pid);
  await longOutage(page, pid, testInfo, 'k1-reconexion-sin-conexion');
  // Después de recuperarse, otra vez nada en una pausa breve.
  await watchLink(page);
  await pauseServer(pid, 3000);
  await page.waitForTimeout(12_000);
  expect(await linkLog(page), 'pausa de 3 s después del corte largo').toEqual([]);
});

test('una terminal por la red: los cortes breves no se ven; uno largo avisa y se recupera (etapa K1)', async ({
  page,
  browser,
}, testInfo) => {
  test.setTimeout(300_000);
  const driver = await installDriver(page);
  await call(driver, 'POST', 'auth/login', admin);
  await lightTheme(driver);
  const credentials = await enrollTerminal(
    driver,
    sharedServerURL,
    `Caja por la red K1 ${Date.now()}`,
  );
  await call(driver, 'POST', 'auth/logout');
  const pid = temporaryServerPID();

  const terminal = await terminalPage(browser, credentials, { main: false });
  try {
    await login(terminal.page, admin.username, admin.password);
    await expect(topBar(terminal.page)).toBeVisible();
    await expectOnline(terminal.page);
    await watchLink(terminal.page);
    await terminal.page.waitForTimeout(35_000);
    expect(await linkLog(terminal.page), 'una renovación del canal').toEqual([]);

    await shortOutages(terminal.page, pid);
    await longOutage(terminal.page, pid, testInfo, 'k1-reconexion-terminal-red');
  } finally {
    await terminal.context.close();
  }
});
