import { serverStateDescriptions, serverStateLabels } from './serverState';
import type { ServerState } from './serverState';

// Indicador compacto del servidor (§1.1). En línea se reduce a un punto de color con su descripción
// accesible (el texto aparece en monitores amplios); reconectando o fuera de línea, el texto queda
// siempre visible como aviso.
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
