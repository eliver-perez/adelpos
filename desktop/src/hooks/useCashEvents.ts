import { useEffect, useRef } from 'react';

// Evento de la aplicación que publica useRealtime cuando llegan eventos del ámbito «cash» por SSE.
// Solo lleva tipos e identificadores (turno o caja), nunca importes: quien lo recibe vuelve a
// consultar el estado autoritativo de Go.
export const cashChangeEvent = 'adel-cash-change';

export interface CashChange {
  kinds: string[];
  aggregates: string[];
}

// Ejecuta la acción cada vez que llega un cambio de caja. El sondeo de cada pantalla sigue activo
// como recuperación si el canal en tiempo real no está disponible.
export function useCashEvents(onChange: (change: CashChange) => void, enabled = true) {
  const callback = useRef(onChange);
  useEffect(() => {
    callback.current = onChange;
  });
  useEffect(() => {
    if (!enabled) {
      return;
    }
    const listener = (event: Event) => {
      const detail = (event as CustomEvent<CashChange>).detail ?? { kinds: [], aggregates: [] };
      callback.current(detail);
    };
    window.addEventListener(cashChangeEvent, listener);
    return () => window.removeEventListener(cashChangeEvent, listener);
  }, [enabled]);
}
