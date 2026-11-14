import { describe, expect, it } from 'vitest';
import product from '../generated/product.json';
import { APIError } from './api';
import { clientUpdateRequired, compareVersions, serverOutdated } from './compatibility';

// Etapa 9G (§5.1): la terminal reconoce un servidor más viejo de lo que necesita y el rechazo de Go a una
// aplicación demasiado vieja.
describe('compatibilidad de versiones', () => {
  it('compara versiones del producto', () => {
    expect(compareVersions('0.10.0', '0.9.5')).toBe(1);
    expect(compareVersions('0.9.5', '0.10.0')).toBe(-1);
    expect(compareVersions('1.0.0', '1.0.0')).toBe(0);
    expect(compareVersions('1.0.0-beta.1', '1.0.0')).toBe(-1);
    expect(compareVersions('1.0.0+7', '1.0.0')).toBe(0);
    expect(compareVersions('nueva', '1.0.0')).toBeNull();
    expect(compareVersions('1.0', '1.0.0')).toBeNull();
  });

  it('avisa solo si el servidor es anterior a la versión mínima que necesita la aplicación', () => {
    expect(serverOutdated(product.min_server_version)).toBe(false);
    expect(serverOutdated(product.version)).toBe(false);
    expect(serverOutdated('0.0.1')).toBe(true);
    expect(serverOutdated('99.0.0')).toBe(false);
    expect(serverOutdated(undefined)).toBe(false);
    expect(serverOutdated('ilegible')).toBe(false);
    expect(serverOutdated('0.9.4', '0.10.0')).toBe(true);
  });

  it('reconoce «Actualiza esta terminal» de Go', () => {
    expect(
      clientUpdateRequired(new APIError('CLIENT_UPDATE_REQUIRED', 'Actualiza esta terminal', 426)),
    ).toBe(true);
    expect(clientUpdateRequired(new APIError('SESSION_EXPIRED', 'Sesión vencida', 401))).toBe(
      false,
    );
    expect(clientUpdateRequired(new Error('otro'))).toBe(false);
  });

  it('la versión mínima de servidor no es posterior a la versión de la aplicación', () => {
    expect(compareVersions(product.min_server_version, product.version)).not.toBe(1);
    expect(compareVersions(product.min_client_version, product.version)).not.toBe(1);
  });
});
