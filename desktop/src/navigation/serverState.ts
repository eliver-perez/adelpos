import { realtimeLabels } from '../hooks/useRealtime';

// Estado del servidor en la barra superior (etapa 9C, §1.1): sustituye a «Sesión verificada · En
// tiempo real». Se deriva de las dos fuentes reales de la aplicación:
// - useSession: la consulta de la sesión cada 5 s; si falla, la terminal no alcanza al servidor
//   (fuera de línea).
// - useRealtime: el canal de eventos (solo con events.read). Conectado es «en línea»; recuperando,
//   interrumpido o no disponible es «reconectando» (la terminal sigue al día por sondeo).
// Sin events.read no hay canal: con la sesión verificada el estado es «en línea».
export type ServerState = 'online' | 'reconnecting' | 'offline';

export function serverState(
  connectionError: string,
  realtimeEnabled: boolean,
  realtimeStatus: string,
): ServerState {
  if (connectionError) {
    return 'offline';
  }
  if (!realtimeEnabled) {
    return 'online';
  }
  // Antes del primer mensaje del canal el estado inicial es la actualización periódica: la sesión
  // está verificada y el canal aún se abre.
  if (realtimeStatus === realtimeLabels.connected || realtimeStatus === realtimeLabels.periodic) {
    return 'online';
  }
  return 'reconnecting';
}

export const serverStateLabels: Record<ServerState, string> = {
  online: 'En línea',
  reconnecting: 'Reconectando',
  offline: 'Fuera de línea',
};

// Descripción accesible y del aviso: qué significa cada estado para quien usa la terminal.
export const serverStateDescriptions: Record<ServerState, string> = {
  online: 'El servidor responde y los cambios se reciben al momento.',
  reconnecting:
    'El servidor responde, pero el aviso inmediato de cambios se está recuperando; la terminal se actualiza cada pocos segundos.',
  offline:
    'No hay comunicación con el servidor. Se reintenta automáticamente; los cambios deben confirmarse por el servidor.',
};
