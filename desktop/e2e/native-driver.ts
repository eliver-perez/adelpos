import { readFileSync } from 'node:fs';
import type { Page } from '@playwright/test';

// Servidor Go temporal de la aceptación. Por omisión el puerto 8183; ADEL_BROWSER_SERVER_URL permite
// ejecutar dos aceptaciones aisladas a la vez con servidores temporales en otros puertos locales.
const serverURL = process.env.ADEL_BROWSER_SERVER_URL ?? 'http://127.0.0.1:8183';

// Etapa 9G (§5.1): el puente se identifica como lo hace Rust (with_client_headers de api.rs): la versión de la
// aplicación (la de config/product.json, que sync-metadata copia a Cargo.toml) y el tipo de cliente.
const clientHeaders = {
  'X-Client-Version': (
    JSON.parse(readFileSync(new URL('../src/generated/product.json', import.meta.url), 'utf8')) as {
      version: string;
    }
  ).version,
  'X-Client-Kind': 'desktop',
};

// Terminal temporal que este mismo proceso de Playwright configuró, por token de aprovisionamiento.
// La aceptación corre con un solo worker: cuando varias especificaciones comparten la base temporal
// (por ejemplo restaurant.spec.ts e inventory.spec.ts en la corrida completa), la primera configura
// el establecimiento y el catálogo mínimo, y las siguientes reutilizan esa terminal en lugar de
// rechazar la base. Una base configurada por otro proceso se sigue rechazando.
const preparedTerminals = new Map<string, { deviceID: string; deviceToken: string }>();

// Exportación de reportes (etapa 9A, parte C1): simulación de la operación cerrada export_report de
// src-tauri/src/export.rs. Usa la misma lista cerrada de reportes (leída de export.rs) y la misma
// lista blanca de parámetros que Rust (la de reports_* en api.rs, sin paginación ni búsqueda, más
// paper para el PDF); pide el archivo a Go desde Node con las credenciales que solo conoce Node,
// valida el tipo exacto y la firma como Rust y simula el diálogo «Guardar como». Los bytes se quedan
// en Node (exports): el navegador recibe solo {result, file_name, size}, como con Rust.
const exportFormats: Record<string, { contentType: string; signature: Buffer; extension: string }> =
  {
    pdf: { contentType: 'application/pdf', signature: Buffer.from('%PDF-'), extension: '.pdf' },
    xlsx: {
      contentType: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
      signature: Buffer.from([0x50, 0x4b, 0x03, 0x04]),
      extension: '.xlsx',
    },
  };

export interface SimulatedExport {
  report: string;
  format: string;
  fileName: string;
  contentType: string;
  bytes: Buffer;
}

// Port de sanitize_file_name de export.rs: nombre base ASCII, sin punto inicial, extensión forzada
// y hasta 100 caracteres.
function sanitizeFileName(name: string, extension: string) {
  const base = name.split(/[/\\]/).pop() ?? '';
  let cleaned = base.replace(/[^A-Za-z0-9._ -]/g, '_');
  if (cleaned.toLowerCase().endsWith(extension)) {
    cleaned = cleaned.slice(0, cleaned.length - extension.length);
  }
  let stem = cleaned.replace(/^[. ]+|[. ]+$/g, '');
  if (/^[_-]*$/.test(stem)) stem = 'reporte';
  stem = stem.slice(0, 100 - extension.length).replace(/[. ]+$/, '');
  return `${stem}${extension}`;
}

// Menú nativo (etapa 9C, fase C): simulación de las operaciones cerradas menu_subscribe y
// menu_set_state de src-tauri/src/menu.rs. Lee la misma lista cerrada MENU_ACTIONS de menu.rs (las
// entradas conservan la forma `MenuAction { id: "…", … }`), rechaza como Rust cualquier
// identificador fuera de ella y entrega los eventos por el canal con el mismo mensaje
// `{ action }`. La prueba «elige» una opción con clickMenu(id): como en el menú real, una opción
// deshabilitada no se puede elegir.
function readMenuActions() {
  const source = readFileSync(new URL('../src-tauri/src/menu.rs', import.meta.url), 'utf8');
  const start = source.indexOf('pub const MENU_ACTIONS');
  const block = source.slice(start, source.indexOf('];', start));
  return new Set(
    [...block.matchAll(/MenuAction\s*\{\s*id:\s*"([a-z_.]+)"/g)].map((match) => match[1]!),
  );
}

// Etapa 9F (§3): las pruebas solo corren contra un servidor que se declara en modo de pruebas
// (`test_mode` de la configuración, que exige una base desechable adel_pos_test_*). Así un
// ADEL_BROWSER_SERVER_URL equivocado nunca escribe en una instalación real. Se exporta solo para
// probar el rechazo con un servidor simulado (src/testing/requireTestServer.test.ts).
export async function requireTestServer() {
  let testMode = false;
  try {
    const response = await fetch(`${serverURL}/api/v1/version`);
    const version = (await response.json()) as { test_mode?: unknown };
    testMode = response.ok && version.test_mode === true;
  } catch {
    testMode = false;
  }
  if (!testMode) {
    throw new Error('Las pruebas solo corren contra un servidor en modo de pruebas');
  }
}

export async function installDriver(page: Page) {
  await requireTestServer();
  let fullscreen = true;
  const menuActions = readMenuActions();
  let menuChannel: number | null = null;
  let menuIndex = 0;
  let menuEnabled = new Set<string>(['view.fullscreen', 'help.shortcuts', 'help.about']);
  let menuPrecheckName = 'precheck';
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
  for (const match of rust.matchAll(
    /"([a-z_]+)"\s*=>\s*\{?\s*Some\(\(\s*Method::(\w+),\s*"([^"]+)"\s*\)\)/g,
  )) {
    const [, operation, method, route] = match;
    if (!operation || !method || !route) continue;
    routes.set(operation, { method, path: route });
  }
  // rustfmt escribe algunas entradas con llaves (`"op" => { (Method::GET, format!(…)) }` o
  // `"op" => { Some((Method::POST, "…")) }`): las dos expresiones aceptan ambas formas para que el
  // puente conozca todas las rutas de la lista cerrada.
  for (const match of rust.matchAll(
    /"([a-z_]+)"\s*=>\s*\{?\s*\(\s*Method::(\w+),\s*format!\("([^"]+)"\)/g,
  )) {
    const [, operation, method, route] = match;
    if (!operation || !method || !route) continue;
    routes.set(operation, { method, path: route });
  }
  const exportSource = readFileSync(new URL('../src-tauri/src/export.rs', import.meta.url), 'utf8');
  const exportableStart = exportSource.indexOf('const EXPORTABLE_REPORTS');
  const exportable = new Set(
    [
      ...exportSource
        .slice(exportableStart, exportSource.indexOf('];', exportableStart))
        .matchAll(/"([a-z_.]+)"/g),
    ].map((match) => match[1]),
  );
  // La lista blanca de reports_* dentro de allowed_query_keys (api_request también menciona el
  // prefijo para su tiempo de espera, antes de esa función).
  const reportKeysStart = rust.indexOf(
    'operation.starts_with("reports_")',
    rust.indexOf('fn allowed_query_keys'),
  );
  const reportQueryKeys = new Set(
    [...rust.slice(reportKeysStart, rust.indexOf(']', reportKeysStart)).matchAll(/"([a-z_]+)"/g)]
      .map((match) => match[1])
      .filter((key) => key !== 'reports_'),
  );
  // Cola de respuestas del diálogo «Guardar como» que controla la prueba; sin respuesta, guarda.
  const saveDialogChoices: ('save' | 'cancel')[] = [];
  const exports: SimulatedExport[] = [];
  async function exportReport(args: Record<string, unknown>) {
    const report = String(args.report);
    const format = String(args.format);
    const kind = exportFormats[format];
    if (!kind || !exportable.has(report))
      throw new Error('El reporte no se puede exportar desde el puente nativo.');
    const query = (args.query ?? {}) as Record<string, string>;
    for (const [key, value] of Object.entries(query)) {
      const accepted =
        key === 'paper'
          ? format === 'pdf' && (value === 'letter' || value === 'a4')
          : !['page', 'page_size', 'search', 'format'].includes(key) && reportQueryKeys.has(key);
      if (!accepted || value.length > 120 || /[\u0000-\u001f\u007f]/.test(value))
        throw new Error('Parámetros de exportación no autorizados.');
    }
    const response = await fetch(
      `${serverURL}/api/v1/reports/export/${report}?${new URLSearchParams({ ...query, format })}`,
      {
        headers: {
          ...clientHeaders,
          'X-Device-ID': deviceID,
          'X-Device-Token': deviceToken,
          Authorization: `Bearer ${sessionToken}`,
        },
      },
    );
    if (response.status !== 200) {
      const body = await response.json();
      const code = String(body?.error?.code ?? '');
      if (response.status === 401 && (code.startsWith('SESSION_') || code.startsWith('DEVICE_')))
        sessionToken = '';
      delete body.device_token;
      delete body.session_token;
      return { status: response.status, body };
    }
    const bytes = Buffer.from(await response.arrayBuffer());
    const contentType = (response.headers.get('content-type') ?? '').trim().toLowerCase();
    if (contentType !== kind.contentType)
      throw new Error('El servidor no respondió con el tipo de archivo esperado.');
    if (
      bytes.length > 32 * 1024 * 1024 ||
      !bytes.subarray(0, kind.signature.length).equals(kind.signature)
    )
      throw new Error('El archivo recibido no es válido.');
    const disposition = response.headers.get('content-disposition') ?? '';
    const suggested = /filename="?([^";]*)"?/i.exec(disposition)?.[1] ?? '';
    const fileName = sanitizeFileName(suggested, kind.extension);
    if ((saveDialogChoices.shift() ?? 'save') === 'cancel') {
      return { status: 200, body: { result: 'cancelled' } };
    }
    exports.push({ report, format, fileName, contentType, bytes });
    return { status: 200, body: { result: 'saved', file_name: fileName, size: bytes.length } };
  }
  function nextSaveDialog(choice: 'save' | 'cancel') {
    saveDialogChoices.push(choice);
  }
  async function request(method: string, route: string, body?: unknown) {
    const response = await fetch(`${serverURL}/api/v1/${route}`, {
      method,
      headers: {
        ...clientHeaders,
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
            ...clientHeaders,
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
      if (command === 'menu_subscribe') {
        // Una suscripción nueva sustituye a la anterior, como en Rust.
        menuChannel = Number(args.channel);
        menuIndex = 0;
        return menuChannel;
      }
      if (command === 'menu_set_state') {
        const enabled = args.enabled;
        if (!Array.isArray(enabled) || enabled.length > menuActions.size) {
          return { __bridgeRejection: 'Estado del menú no válido.' };
        }
        if (enabled.some((id) => typeof id !== 'string' || !menuActions.has(id))) {
          return { __bridgeRejection: 'Opción de menú no reconocida.' };
        }
        // Etapa 9F (§4.5): el nombre visible de la precuenta, de la misma lista cerrada que menu.rs.
        const precheckName = args.precheckName;
        if (
          precheckName !== undefined &&
          precheckName !== null &&
          !['precheck', 'sales_ticket', 'bill'].includes(String(precheckName))
        ) {
          return { __bridgeRejection: 'Nombre de la precuenta no reconocido.' };
        }
        menuEnabled = new Set(enabled as string[]);
        if (typeof precheckName === 'string') {
          menuPrecheckName = precheckName;
        }
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
      if (command === 'export_report') {
        // Como en Tauri, un rechazo del puente llega al WebView como texto (no como Error).
        try {
          return await exportReport(args);
        } catch (error) {
          return { __bridgeRejection: error instanceof Error ? error.message : String(error) };
        }
      }
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
        return bridge.__nativeTestBridge(command, body).then((result) => {
          if (result && typeof result === 'object' && '__bridgeRejection' in result) {
            throw (result as { __bridgeRejection: string }).__bridgeRejection;
          }
          return result;
        });
      },
      transformCallback: (callback: (message: unknown) => void) => {
        callbacks.set(++nextCallback, callback);
        return nextCallback;
      },
      unregisterCallback: (id: number) => callbacks.delete(id),
      runCallback: (id: number, message: unknown) => callbacks.get(id)?.(message),
    };
  });
  // Elige una opción del menú nativo simulado. Devuelve false si la opción está deshabilitada (no
  // se envía nada); una opción fuera de la lista cerrada es un error de la prueba.
  async function clickMenu(action: string) {
    if (!menuActions.has(action)) {
      throw new Error(`Opción de menú desconocida: ${action}`);
    }
    if (!menuEnabled.has(action) || menuChannel === null) {
      return false;
    }
    const channel = menuChannel;
    await page.evaluate(
      ({ channel, index, message }) => {
        const native = window as unknown as {
          __TAURI_INTERNALS__: { runCallback: (id: number, message: unknown) => void };
        };
        native.__TAURI_INTERNALS__.runCallback(channel, { index, message });
      },
      { channel, index: menuIndex++, message: { action } },
    );
    return true;
  }
  function menuState() {
    return [...menuEnabled].sort();
  }
  // Clave del nombre de la precuenta que la interfaz informó al menú nativo (etapa 9F).
  function menuPrecheck() {
    return menuPrecheckName;
  }
  return { request, nextSaveDialog, exports, clickMenu, menuState, menuPrecheck, menuActions };
}
