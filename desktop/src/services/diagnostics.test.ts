import { afterEach, describe, expect, it, vi } from 'vitest';
import { getDiagnostics, interpret } from './diagnostics';
import type { WireSnapshot } from './diagnostics';
import product from '../generated/product.json';

function healthy(): WireSnapshot {
  return {
    live: { status: 200, body: { status: 'alive' } },
    ready: {
      status: 200,
      body: { status: 'ready', checks: { postgres: 'available', schema: 'compatible' } },
    },
    version: {
      status: 200,
      body: {
        product: product.technical_id,
        api_version: product.api_version,
        schema_version: product.schema_version,
        server_version: product.version,
      },
    },
  };
}

afterEach(() => vi.unstubAllGlobals());

describe('real connection states', () => {
  it('recognizes a compatible backend and database', () => {
    const d = interpret(healthy());
    expect(d.backend).toBe('connected');
    expect(d.postgres).toBe('available');
    expect(d.compatible).toBe(true);
  });
  it('does not report a database outage as a backend outage', () => {
    const snapshot = healthy();
    snapshot.ready = {
      status: 503,
      body: { status: 'not_ready', checks: { postgres: 'unavailable', schema: 'unknown' } },
    };
    const d = interpret(snapshot);
    expect(d.backend).toBe('connected');
    expect(d.postgres).toBe('unavailable');
    expect(d.message).toContain('PostgreSQL no está disponible');
  });
  it('reports schema incompatibility', () => {
    const snapshot = healthy();
    snapshot.ready = {
      status: 503,
      body: { status: 'not_ready', checks: { postgres: 'available', schema: 'incompatible' } },
    };
    expect(interpret(snapshot).message).toContain('migraciones');
  });
  it('rejects other products and unsupported API contracts', () => {
    const snapshot = healthy();
    snapshot.version = { status: 200, body: { api_version: 'v99', server_version: '9.0.0' } };
    expect(interpret(snapshot).compatible).toBe(false);
  });
  it('treats non-JSON or unrecognized replies as unknown', () => {
    const snapshot = healthy();
    snapshot.ready = { status: 200, body: '<html>not a backend</html>' };
    expect(interpret(snapshot).postgres).toBe('unknown');
    snapshot.live = { status: 200, body: {} };
    expect(interpret(snapshot).backend).toBe('invalid');
  });
  it('handles connection refusal without stale healthy state', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new TypeError('Connection refused')));
    const d = await getDiagnostics({
      installation_mode: 'terminal-only',
      server_url: 'http://127.0.0.1:1',
    });
    expect(d.backend).toBe('disconnected');
    expect(d.postgres).toBe('unknown');
    expect(d.serverVersion).toBeNull();
  });
});
