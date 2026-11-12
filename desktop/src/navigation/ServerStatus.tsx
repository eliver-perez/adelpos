import { serverStateDescriptions, serverStateLabels } from './serverState';
import type { ServerState } from './serverState';

// Indicador del servidor (§1.1 de la 9C y §2 de la 9D): punto de color y texto («En línea» en azul,
// «Reconectando» en ámbar, «Sin conexión» en rojo). En la barra reducida de la captura, en línea se
// reduce al punto con su descripción accesible; reconectando o sin conexión, el texto queda siempre
// visible como aviso.
export function ServerStatus({ state }: { state: ServerState }) {
  const label = serverStateLabels[state];
  return (
    <div
      className={`server-status server-status-${state}`}
      role="status"
      aria-label={`Servidor: ${label}`}
      title={serverStateDescriptions[state]}
      data-state={state}
    >
      <span className="server-status-dot" aria-hidden="true" />
      <span className="server-status-text" aria-hidden="true">
        {label}
      </span>
    </div>
  );
}
