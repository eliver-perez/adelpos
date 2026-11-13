import { createServer } from 'node:http';
import type { Server } from 'node:http';
import type { AddressInfo } from 'node:net';
import { afterAll, beforeAll, describe, expect, it, vi } from 'vitest';

// Etapa 9F (§8, revisión independiente B5): el puente de Playwright (e2e/native-driver.ts) se niega a
// correr contra un servidor que no se declara en modo de pruebas. Aquí se comprueba ese rechazo con un
// servidor simulado en un puerto libre de 127.0.0.1 que responde GET /api/v1/version como se le pida;
// el puente se importa con ADEL_BROWSER_SERVER_URL apuntando a él, igual que en una aceptación aislada.

type Reply = { status: number; body: string };

let reply: Reply = { status: 200, body: '{}' };
let requested: string[] = [];
let server: Server;
let requireTestServer: () => Promise<void>;

beforeAll(async () => {
  server = createServer((request, response) => {
    requested.push(`${request.method} ${request.url}`);
    response.writeHead(reply.status, { 'Content-Type': 'application/json' });
    response.end(reply.body);
  });
  await new Promise<void>((resolve) => server.listen(0, '127.0.0.1', resolve));
  const { port } = server.address() as AddressInfo;
  vi.stubEnv('ADEL_BROWSER_SERVER_URL', `http://127.0.0.1:${port}`);
  vi.resetModules();
  ({ requireTestServer } = await import('../../e2e/native-driver'));
});

afterAll(async () => {
  vi.unstubAllEnvs();
  await new Promise<void>((resolve) => server.close(() => resolve()));
});

function answer(status: number, body: unknown) {
  reply = { status, body: typeof body === 'string' ? body : JSON.stringify(body) };
  requested = [];
}

const refusal = 'Las pruebas solo corren contra un servidor en modo de pruebas';

describe('requireTestServer (puente de las pruebas de navegador)', () => {
  it('rechaza un servidor que no informa el modo de pruebas', async () => {
    answer(200, { name: 'AD-EL POS', version: '0.9.4', api: 'v1' });
    await expect(requireTestServer()).rejects.toThrow(refusal);
    expect(requested).toEqual(['GET /api/v1/version']);
  });

  it('rechaza test_mode: false y cualquier valor que no sea exactamente true', async () => {
    for (const value of [false, 'true', 1, null]) {
      answer(200, { version: '0.9.4', test_mode: value });
      await expect(requireTestServer()).rejects.toThrow(refusal);
    }
  });

  it('rechaza una respuesta con error o que no es JSON', async () => {
    answer(500, { test_mode: true });
    await expect(requireTestServer()).rejects.toThrow(refusal);
    answer(200, '<html>no es la API</html>');
    await expect(requireTestServer()).rejects.toThrow(refusal);
  });

  it('acepta solo un servidor en modo de pruebas', async () => {
    answer(200, { version: '0.9.4', test_mode: true });
    await expect(requireTestServer()).resolves.toBeUndefined();
    expect(requested).toEqual(['GET /api/v1/version']);
  });
});
