import type { LinkSnapshot } from './serverLink';

// Estado del servidor en la barra superior (etapa 9C, §1.1). Desde la etapa K1 (Fase E, K1-D10) sale de un solo estado
// de conexión con la tolerancia de la A3 (navigation/serverLink.ts), que alimentan la sesión, cada respuesta de la API y
// el canal de eventos:
// - «Sin conexión» solo con varias fallas seguidas durante unos 20 s (o sin red del sistema en una terminal por la red);
// - «Reconectando» solo si el canal de eventos (con events.read) está caído más de esa tolerancia mientras Go responde:
//   la renovación normal del canal cada 30 s y un corte breve no se notan;
// - «En línea» en cualquier otro caso, y con el primer éxito.
export type ServerState = 'online' | 'reconnecting' | 'offline';

export function serverState(link: LinkSnapshot, realtimeEnabled: boolean): ServerState {
  if (link.offline) {
    return 'offline';
  }
  if (realtimeEnabled && link.channelDown) {
    return 'reconnecting';
  }
  return 'online';
}

export const serverStateLabels: Record<ServerState, string> = {
  online: 'En línea',
  reconnecting: 'Reconectando',
  // Etapa 9D (decisión 9): «Sin conexión», el texto de la paleta del propietario (en rojo).
  offline: 'Sin conexión',
};

// Descripción accesible y del aviso: qué significa cada estado para quien usa la terminal.
export const serverStateDescriptions: Record<ServerState, string> = {
  online: 'El equipo servidor responde y los cambios llegan al momento.',
  reconnecting:
    'El equipo servidor responde, pero el aviso inmediato de cambios no llega desde hace un rato; la terminal sigue intentándolo y se actualiza cada pocos segundos.',
  offline:
    'No hay comunicación con el equipo servidor. Se reintenta automáticamente; espera a que vuelva la conexión para guardar cambios.',
};
