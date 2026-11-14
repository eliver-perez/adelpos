import { cashChangeEvent } from '../hooks/useCashEvents';
import type { CashChange } from '../hooks/useCashEvents';

// Reconexión con el servidor (etapa 9G, fase C, §4.2 y decisión D11).
//
// - Mientras la terminal no alcanza al servidor, la sesión se vuelve a consultar sola con espera creciente
//   (reconnectDelays) y la capa «Sin conexión con el servidor» bloquea las operaciones sin desmontar las
//   pantallas.
// - Al volver, la terminal **vuelve a consultar el estado vigente** de todo lo que está a la vista (mesas,
//   cuentas, cocina y caja) en lugar de depender de haber recibido todos los eventos en tiempo real: el canal
//   SSE reanuda desde su cursor, pero la consulta completa es la garantía.

// Esperas entre reintentos sin conexión (milisegundos): crecen hasta 10 s. El canal de eventos de Rust tiene su
// propia espera (0.5 a 15 s); la sesión no espera más de 10 s para que la terminal vuelva pronto al recuperar la red.
export const reconnectDelays = [1000, 2000, 4000, 8000, 10000] as const;

export function reconnectDelay(failures: number) {
  const index = Math.min(Math.max(failures, 1), reconnectDelays.length) - 1;
  return reconnectDelays[index]!;
}

// Evento de la aplicación al recuperar la conexión: la captura reintenta el guardado que quedó incierto.
export const connectionRestoredEvent = 'adel-connection-restored';

// Pide a todas las pantallas montadas que vuelvan a consultar al servidor.
export function announceReconnection() {
  window.dispatchEvent(new CustomEvent('adel-server-change'));
  const cash: CashChange = { kinds: [], aggregates: [] };
  window.dispatchEvent(new CustomEvent(cashChangeEvent, { detail: cash }));
  window.dispatchEvent(new CustomEvent(connectionRestoredEvent));
}
