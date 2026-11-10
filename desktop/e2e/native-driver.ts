import { readFileSync } from 'node:fs';
import type { Page } from '@playwright/test';

// Servidor Go temporal de la aceptación. Por omisión el puerto 8183; ADEL_BROWSER_SERVER_URL permite
// ejecutar dos aceptaciones aisladas a la vez con servidores temporales en otros puertos locales.
const serverURL = process.env.ADEL_BROWSER_SERVER_URL ?? 'http://127.0.0.1:8183';

// Terminal temporal que este mismo proceso de Playwright configuró, por token de aprovisionamiento.
// La aceptación corre con un solo worker: cuando varias especificaciones comparten la base temporal
// (por ejemplo restaurant.spec.ts e inventory.spec.ts en la corrida completa), la primera configura
// el establecimiento y el catálogo mínimo, y las siguientes reutilizan esa terminal en lugar de
// rechazar la base. Una base configurada por otro proceso se sigue rechazando.
const preparedTerminals = new Map<string, { deviceID: string; deviceToken: string }>();

export async function installDriver(page: Page) {
  let fullscreen = true;
  let streamGeneration = 0;
  const streams = new Map<number, AbortController>();
  page.on('close', () => {
    for (const stream of streams.values()) stream.abort();
  });
  // Sustitución exclusiva de IPC para la aceptación del WebView. Los secretos se quedan
  // en Node; el navegador recibe el mismo contrato saneado que devuelve Rust.
  let deviceID = '';
  let deviceToken = '';
  let sessionToken = '';
  const provisioning = readFileSync(process.env.ADEL_BROWSER_PROVISIONING!, 'utf8').trim();
  const rust = readFileSync(new URL('../src-tauri/src/api.rs', import.meta.url), 'utf8');
  const routes = new Map<string, { method: string; path: string }>();
  for (const match of rust.matchAll(/"([a-z_]+)"\s*=>\s*Some\(\(Method::(\w+),\s*"([^"]+)"\)\)/g)) {
    const [, operation, method, route] = match;
    if (!operation || !method || !route) continue;
    routes.set(operation, { method, path: route });
  }
  // rustfmt escribe algunas entradas con llaves (`"op" => { (Method::GET, format!(…)) }`): se aceptan
  // ambas formas para que el puente conozca todas las rutas de la lista cerrada.
  for (const match of rust.matchAll(
    /"([a-z_]+)"\s*=>\s*\{?\s*\(\s*Method::(\w+),\s*format!\("([^"]+)"\)/g,
  )) {
    const [, operation, method, route] = match;
    if (!operation || !method || !route) continue;
    routes.set(operation, { method, path: route });
  }
  async function request(method: string, route: string, body?: unknown) {
    const response = await fetch(`${serverURL}/api/v1/${route}`, {
      method,
      headers: {
        'Content-Type': 'application/json',
        'X-Provisioning-Token': provisioning,
        'X-Device-ID': deviceID,
        'X-Device-Token': deviceToken,
        Authorization: `Bearer ${sessionToken}`,
      },
      body: body === undefined || method === 'GET' ? undefined : JSON.stringify(body),
    });
    const result = await response.json();
    if (response.ok) {
      if (result.device_token) {
        deviceID = result.device_id;
        deviceToken = result.device_token;
        sessionToken = '';
      }
      if (result.session_token) sessionToken = result.session_token;
      if (route === 'auth/logout') sessionToken = '';
    }
    delete result.device_token;
    delete result.session_token;
    return { status: response.status, body: result };
  }
  const setup = await request('GET', 'setup/status');
  const prepared = setup.body.configured ? preparedTerminals.get(provisioning) : undefined;
  if (!setup.body.configured) {
    await request('POST', 'setup', {
      name: 'Aceptación temporal',
      timezone: 'America/Monterrey',
      currency: 'MXN',
      theme: 'light',
      terminal_name: 'WebView temporal',
    });
    preparedTerminals.set(provisioning, { deviceID, deviceToken });
  } else if (prepared) {
    deviceID = prepared.deviceID;
    deviceToken = prepared.deviceToken;
  } else {
    throw new Error('La aceptación debe comenzar con una base sin configurar');
  }
  // Catálogo mínimo, solo al configurar la base: una especificación posterior lo encuentra hecho.
  async function prepareCatalog() {
    await request('POST', 'auth/login', { username: 'admin', password: 'admin' });
    // Catálogo mínimo de la base temporal: una categoría con color de la paleta, un producto que se
    // agrega con un toque y otro con una elección obligatoria (guarnición, con papas incluidas y
    // aros de cebolla con cargo extra) para el recorrido de la captura táctil.
    const category = (
      await request('POST', 'catalog/categories', {
        name: 'Platillos',
        active: true,
        color: '#d64545',
      })
    ).body;
    const product = (
      await request('POST', 'catalog/products', {
        name: 'Filete de prueba',
        active: true,
        category_id: category.id,
        code: 'FILETE',
        kind: 'product',
        base_price: '150',
      })
    ).body;
    const complements: { id: string }[] = [];
    for (const name of ['Papas a la francesa', 'Aros de cebolla']) {
      complements.push(
        (
          await request('POST', 'catalog/complements', {
            name,
            active: true,
            product_id: null,
            ingredient_id: null,
          })
        ).body,
      );
    }
    const garnish = (
      await request('POST', 'catalog/groups', {
        name: 'Guarnición',
        active: true,
        pricing_mode: 'value',
        min_quantity: '1',
        max_quantity: '1',
        options: complements.map((complement, index) => ({
          complement_id: complement.id,
          portion_id: null,
          reference_value: '0',
          extra_price: index === 0 ? '0' : '10',
          max_quantity: null,
          active: true,
          tiers: [],
        })),
      })
    ).body;
    const fries = garnish.options?.find(
      (option: { complement_id: string }) => option.complement_id === complements[0]?.id,
    );
    const burger = (
      await request('POST', 'catalog/products', {
        name: 'Hamburguesa de prueba',
        active: true,
        category_id: category.id,
        code: 'HAMBURGUESA',
        kind: 'product',
        base_price: '120',
        groups: [
          {
            group_id: garnish.id,
            min_quantity: null,
            max_quantity: null,
            included: [{ option_id: fries?.id, quantity: '1' }],
          },
        ],
      })
    ).body;
    if (!product.id || !fries || !burger.id)
      throw new Error('No se pudo preparar el catálogo temporal');
    await request('POST', 'auth/logout');
  }
  if (!prepared) {
    await prepareCatalog();
  }

  async function streamEvents(
    identifier: number,
    callbackID: number,
    after: number,
    controller: AbortController,
  ) {
    let cursor = after;
    let index = 0;
    const deliver = async (message: unknown) => {
      if (controller.signal.aborted || page.isClosed()) return;
      await page.evaluate(
        ({ callbackID, index, message }) => {
          const native = window as unknown as {
            __TAURI_INTERNALS__: { runCallback: (id: number, message: unknown) => void };
          };
          native.__TAURI_INTERNALS__.runCallback(callbackID, { index, message });
        },
        { callbackID, index: index++, message },
      );
    };
    try {
      while (!controller.signal.aborted && !page.isClosed()) {
        const response = await fetch(`${serverURL}/api/v1/events?after=${cursor}`, {
          headers: {
            'X-Device-ID': deviceID,
            'X-Device-Token': deviceToken,
            Authorization: `Bearer ${sessionToken}`,
          },
          signal: controller.signal,
        });
        if (!response.ok) {
          await deliver({ connection: 'unauthorized', status: response.status });
          break;
        }
        await deliver({ connection: 'connected' });
        const reader = response.body?.getReader();
        if (!reader) break;
        const decoder = new TextDecoder();
        let buffer = '';
        while (!controller.signal.aborted) {
          const part = await reader.read();
          if (part.done) break;
          buffer += decoder.decode(part.value, { stream: true });
          let end = buffer.indexOf('\n\n');
          while (end >= 0) {
            const frame = buffer.slice(0, end);
            buffer = buffer.slice(end + 2);
            const data = frame.split('\n').find((line) => line.startsWith('data: '));
            if (data) {
              const message = JSON.parse(data.slice(6)) as { cursor?: number };
              if (message.cursor !== undefined) cursor = message.cursor;
              await deliver(message);
            }
            end = buffer.indexOf('\n\n');
          }
        }
      }
    } catch {
      // Cerrar una vista o terminar la prueba cancela la conexión, igual que Rust.
    } finally {
      streams.delete(identifier);
    }
  }
  await page.exposeFunction(
    '__nativeTestBridge',
    async (command: string, args: Record<string, unknown> = {}) => {
      if (command === 'window_fullscreen_state') return fullscreen;
      if (command === 'window_set_fullscreen') {
        fullscreen = !!args.fullscreen;
        return;
      }
      if (command === 'events_start') {
        const identifier = ++streamGeneration;
        const controller = new AbortController();
        streams.set(identifier, controller);
        void streamEvents(identifier, Number(args.channel), Number(args.after ?? 0), controller);
        return identifier;
      }
      if (command === 'events_stop') {
        streams.get(Number(args.subscriptionId))?.abort();
        return;
      }
      if (command === 'authentication_state')
        return {
          enrolled: !!deviceID,
          has_session: !!sessionToken,
          local_setup_available: false,
          local_enrollment_available: false,
        };
      if (command === 'connection_config')
        return { installation_mode: 'server-terminal', server_url: serverURL };
      if (command === 'server_snapshot')
        return {
          live: await request('GET', 'health/live'),
          ready: await request('GET', 'health/ready'),
          version: await request('GET', 'version'),
        };
      if (command !== 'api_request') throw new Error('Comando de prueba no autorizado');
      const operation = String(args.operation);
      const entry = routes.get(operation);
      if (!entry) throw new Error(`Ruta nativa no reconocida: ${operation}`);
      let route = entry.path.replace('{identifier}', String(args.resourceId));
      if (args.query) route += `?${new URLSearchParams(args.query as Record<string, string>)}`;
      return request(entry.method, route, args.body ?? undefined);
    },
  );
  await page.addInitScript(() => {
    const bridge = window as unknown as {
      isTauri: boolean;
      __TAURI_INTERNALS__: unknown;
      __nativeTestBridge: (command: string, args?: unknown) => Promise<unknown>;
    };
    bridge.isTauri = true;
    let nextCallback = 0;
    const callbacks = new Map<number, (message: unknown) => void>();
    bridge.__TAURI_INTERNALS__ = {
      invoke: (command: string, args?: Record<string, unknown>) => {
        const body = { ...args };
        if (body.channel && typeof body.channel === 'object' && 'id' in body.channel)
          body.channel = body.channel.id;
        return bridge.__nativeTestBridge(command, body);
      },
      transformCallback: (callback: (message: unknown) => void) => {
        callbacks.set(++nextCallback, callback);
        return nextCallback;
      },
      unregisterCallback: (id: number) => callbacks.delete(id),
      runCallback: (id: number, message: unknown) => callbacks.get(id)?.(message),
    };
  });
  return { request };
}
