import product from '../generated/product.json';
import { APIError } from './api';

// Compatibilidad entre esta aplicación y el servidor del restaurante (etapa 9G, §5.1). No es una regla de negocio:
// solo decide qué aviso ve quien usa la terminal cuando una de las dos partes es demasiado vieja.
// - Rust envía en cada petición la versión de la aplicación (X-Client-Version) y Go responde 426
//   CLIENT_UPDATE_REQUIRED si es anterior a la mínima que acepta: «Actualiza esta terminal».
// - Si el servidor informa (GET /api/v1/version) una versión anterior a min_server_version de product.json, la
//   aplicación usa funciones que ese servidor no tiene: «Actualiza el servidor».

// compareVersions compara versiones MAJOR.MINOR.PATCH (un sufijo «-beta…» va antes de la versión final). Devuelve
// null si alguna no es válida.
export function compareVersions(left: string, right: string): number | null {
  const parse = (text: string) => {
    const match = /^(\d{1,6})\.(\d{1,6})\.(\d{1,6})(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$/.exec(
      text,
    );
    if (!match) {
      return null;
    }
    return {
      parts: [Number(match[1]), Number(match[2]), Number(match[3])],
      prerelease: match[4] !== undefined,
    };
  };
  const a = parse(left);
  const b = parse(right);
  if (!a || !b) {
    return null;
  }
  for (const [index, part] of a.parts.entries()) {
    const other = b.parts[index] ?? 0;
    if (part !== other) {
      return part < other ? -1 : 1;
    }
  }
  if (a.prerelease !== b.prerelease) {
    return a.prerelease ? -1 : 1;
  }
  return 0;
}

// serverOutdated dice si la versión que informa el servidor es anterior a la que necesita esta aplicación. Una
// versión ausente o ilegible no se trata como vieja (el diagnóstico del estado del sistema la reporta aparte).
export function serverOutdated(
  serverVersion: unknown,
  minimum: string = product.min_server_version,
): boolean {
  if (typeof serverVersion !== 'string') {
    return false;
  }
  const comparison = compareVersions(serverVersion, minimum);
  return comparison !== null && comparison < 0;
}

// clientUpdateRequired reconoce el rechazo de Go a una aplicación anterior a la mínima que acepta.
export function clientUpdateRequired(error: unknown): boolean {
  return error instanceof APIError && error.code === 'CLIENT_UPDATE_REQUIRED';
}

// Textos de los avisos, en palabras del restaurante.
export const compatibilityNotices = {
  terminal: {
    title: 'Actualiza esta terminal',
    description:
      'El servidor del restaurante necesita una versión más reciente de AD-EL POS en este equipo. Instala la versión más reciente y vuelve a abrir la aplicación.',
  },
  server: {
    title: 'Actualiza el servidor',
    description:
      'La computadora principal del restaurante tiene una versión anterior de AD-EL POS que esta terminal no puede usar. Actualiza AD-EL POS en la computadora principal y vuelve a comprobar.',
  },
} as const;
