import type { Diagnostics } from './diagnostics';

// «Estado del sistema» en lenguaje sencillo (etapa 9C, §4.1): si el servidor responde, si hay
// conexión con la base de datos, la versión y qué hacer si algo falla. Sin nombres de tecnologías,
// direcciones, puertos ni identificadores: esos datos quedan en «Información para soporte».
export type CheckState = 'good' | 'problem' | 'pending';

export interface PlainStatus {
  tone: CheckState;
  summary: string;
  advice: string | null;
  server: CheckState;
  database: CheckState;
  versionMatches: CheckState;
}

const restartServer =
  'Reinicia el equipo servidor. Si el problema continúa, comunícate con soporte.';
const callSupport = 'Comunícate con soporte.';

export function plainStatus(diagnostics: Diagnostics | null, configError: boolean): PlainStatus {
  if (configError) {
    return {
      tone: 'problem',
      summary: 'Esta terminal no tiene la conexión con el servidor configurada.',
      advice: callSupport,
      server: 'problem',
      database: 'pending',
      versionMatches: 'pending',
    };
  }
  if (!diagnostics) {
    return {
      tone: 'pending',
      summary: 'Comprobando la conexión…',
      advice: null,
      server: 'pending',
      database: 'pending',
      versionMatches: 'pending',
    };
  }
  const server: CheckState = diagnostics.backend === 'connected' ? 'good' : 'problem';
  let database: CheckState = 'pending';
  if (diagnostics.postgres === 'available' && diagnostics.schema === 'compatible') {
    database = 'good';
  } else if (diagnostics.postgres === 'unavailable' || diagnostics.schema === 'incompatible') {
    database = 'problem';
  }
  const versionMatches: CheckState =
    server !== 'good' ? 'pending' : diagnostics.compatible ? 'good' : 'problem';
  if (server !== 'good') {
    return {
      tone: 'problem',
      summary:
        diagnostics.backend === 'invalid'
          ? 'Respondió un equipo que no es el servidor de AD-EL POS.'
          : 'El servidor no responde.',
      advice:
        diagnostics.backend === 'invalid'
          ? callSupport
          : 'Revisa que el equipo servidor esté encendido y conectado a la red. Si sigue sin responder, reinícialo; si el problema continúa, comunícate con soporte.',
      server,
      database,
      versionMatches,
    };
  }
  if (versionMatches === 'problem') {
    return {
      tone: 'problem',
      summary: 'La versión de esta terminal y la del servidor no coinciden.',
      advice: `${callSupport} Hay que actualizar AD-EL POS en esta terminal o en el servidor.`,
      server,
      database,
      versionMatches,
    };
  }
  if (database === 'problem') {
    return {
      tone: 'problem',
      summary:
        diagnostics.postgres === 'unavailable'
          ? 'El servidor responde, pero no tiene conexión con la base de datos.'
          : 'El servidor responde, pero la base de datos necesita una actualización.',
      advice: diagnostics.postgres === 'unavailable' ? restartServer : callSupport,
      server,
      database,
      versionMatches,
    };
  }
  if (database === 'pending') {
    return {
      tone: 'problem',
      summary: 'El servidor responde, pero no se pudo comprobar la base de datos.',
      advice: restartServer,
      server,
      database,
      versionMatches,
    };
  }
  return {
    tone: 'good',
    summary: 'Todo funciona correctamente: el servidor responde y la base de datos está conectada.',
    advice: null,
    server,
    database,
    versionMatches,
  };
}
