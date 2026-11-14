import { invoke, isTauri } from '@tauri-apps/api/core';
import product from '../generated/product.json';
import defaultConnection from '../generated/connection.json';

export interface ConnectionConfig {
  // Etapa 9G: también «Solo servidor»; una terminal sin servidor todavía tiene la dirección vacía.
  installation_mode: 'server-terminal' | 'server-only' | 'terminal-only';
  server_url: string;
}
export interface HTTPResult {
  status: number;
  body: unknown;
}
export interface WireSnapshot {
  live: HTTPResult | null;
  ready: HTTPResult | null;
  version: HTTPResult | null;
}
export interface Diagnostics {
  backend: 'connected' | 'disconnected' | 'invalid';
  postgres: 'available' | 'unavailable' | 'unknown';
  schema: 'compatible' | 'incompatible' | 'unknown';
  serverVersion: string | null;
  compatible: boolean;
  message: string;
  checkedAt: Date;
}

function record(value: unknown): Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : {};
}

export function interpret(snapshot: WireSnapshot): Diagnostics {
  const live = record(snapshot.live?.body);
  const version = record(snapshot.version?.body);
  const readiness = record(snapshot.ready?.body);
  const checks = record(readiness.checks);
  const alive = snapshot.live?.status === 200 && live.status === 'alive';
  const postgres =
    checks.postgres === 'available' || checks.postgres === 'unavailable'
      ? checks.postgres
      : 'unknown';
  const schema =
    checks.schema === 'compatible' || checks.schema === 'incompatible' ? checks.schema : 'unknown';
  const compatible =
    snapshot.version?.status === 200 &&
    version.product === product.technical_id &&
    version.api_version === product.api_version &&
    version.schema_version === product.schema_version;
  const readyValid =
    (snapshot.ready?.status === 200 && readiness.status === 'ready') ||
    (snapshot.ready?.status === 503 && readiness.status === 'not_ready');
  let message = 'Conexión verificada. El servidor y la base de datos están disponibles.';
  if (!alive)
    message = snapshot.live
      ? 'La dirección respondió, pero no corresponde a un servidor compatible.'
      : 'No se pudo conectar con el servidor. Comprueba que esté encendido y que la dirección y la red sean correctas.';
  else if (!compatible)
    message =
      'El servidor respondió con una versión o un contrato incompatible. Revisa las versiones de ambos componentes.';
  else if (!readyValid || postgres === 'unknown')
    message =
      'El servidor está activo, pero no se pudo verificar la disponibilidad de la base de datos.';
  else if (postgres === 'unavailable')
    message =
      'El servidor está activo, pero PostgreSQL no está disponible. Revisa el servicio de base de datos.';
  else if (schema !== 'compatible')
    message =
      'PostgreSQL está disponible, pero el esquema requiere revisión. Ejecuta o revisa las migraciones desde el servidor.';
  return {
    backend: alive ? 'connected' : snapshot.live ? 'invalid' : 'disconnected',
    postgres: alive && readyValid ? postgres : 'unknown',
    schema: alive && readyValid ? schema : 'unknown',
    serverVersion: typeof version.server_version === 'string' ? version.server_version : null,
    compatible: Boolean(compatible),
    message,
    checkedAt: new Date(),
  };
}

export async function connectionConfig(): Promise<ConnectionConfig> {
  if (isTauri()) {
    return invoke<ConnectionConfig>('connection_config');
  }
  return defaultConnection as ConnectionConfig;
}

async function browserGET(base: string, route: string): Promise<HTTPResult | null> {
  try {
    const response = await fetch(`${base}/api/v1/${route}`, {
      signal: AbortSignal.timeout(3000),
      cache: 'no-store',
      redirect: 'error',
    });
    return { status: response.status, body: (await response.json()) as unknown };
  } catch {
    return null;
  }
}

export async function getDiagnostics(config: ConnectionConfig): Promise<Diagnostics> {
  try {
    if (isTauri()) {
      return interpret(await invoke<WireSnapshot>('server_snapshot'));
    }
    const [live, ready, version] = await Promise.all(
      ['health/live', 'health/ready', 'version'].map((route) =>
        browserGET(config.server_url, route),
      ),
    );
    return interpret({ live: live ?? null, ready: ready ?? null, version: version ?? null });
  } catch {
    return interpret({ live: null, ready: null, version: null });
  }
}
