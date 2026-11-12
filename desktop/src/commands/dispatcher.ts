// Despachador único de acciones (etapa 9C, fase C): el menú nativo, los atajos y la paleta ejecutan
// las acciones por aquí.
//
// Protección contra la doble ejecución: en la ventana nativa, un atajo que también es acelerador
// del menú puede llegar dos veces (la pulsación al WebView y el evento del menú desde Rust). Una
// misma acción que llega por otra vía dentro de `twinWindowMs` se considera su gemela y se descarta
// una sola vez; una pulsación que no se ejecutó (por ejemplo, mientras se escribe en un campo)
// también consume a su gemela del menú. Mientras una acción asíncrona sigue en curso (una
// navegación que espera «Salir sin guardar», guardar el tema) no se vuelve a ejecutar.

export type CommandSource = 'menu' | 'keyboard' | 'palette';

export const twinWindowMs = 700;

interface LastEvent {
  source: CommandSource;
  at: number;
  paired: boolean;
}

export interface Dispatcher {
  // ¿Es la gemela de la última llegada de la acción por otra vía? Si lo es, la consume.
  isTwin: (id: string, source: CommandSource) => boolean;
  // Registra una llegada que no se ejecuta (deshabilitada, mientras se escribe, con un diálogo
  // abierto), para descartar su gemela.
  record: (id: string, source: CommandSource) => void;
  // Gemela → nada; si no, la registra sin ejecutarla.
  suppress: (id: string, source: CommandSource) => void;
  // Gemela o en curso → nada; si no, la ejecuta. Devuelve si se ejecutó.
  dispatch: (id: string, source: CommandSource, run: () => unknown) => boolean;
  running: (id: string) => boolean;
}

export function createDispatcher(now: () => number = () => performance.now()): Dispatcher {
  const last = new Map<string, LastEvent>();
  const inFlight = new Set<string>();

  function isTwin(id: string, source: CommandSource) {
    const previous = last.get(id);
    if (!previous || previous.paired || previous.source === source) {
      return false;
    }
    if (now() - previous.at >= twinWindowMs) {
      return false;
    }
    previous.paired = true;
    return true;
  }

  function record(id: string, source: CommandSource) {
    last.set(id, { source, at: now(), paired: false });
  }

  return {
    isTwin,
    record,
    suppress(id, source) {
      if (!isTwin(id, source)) {
        record(id, source);
      }
    },
    dispatch(id, source, run) {
      if (isTwin(id, source) || inFlight.has(id)) {
        return false;
      }
      record(id, source);
      let result: unknown;
      try {
        result = run();
      } catch (error) {
        console.error(error);
        return true;
      }
      if (result instanceof Promise) {
        inFlight.add(id);
        result.catch((error: unknown) => console.error(error)).finally(() => inFlight.delete(id));
      }
      return true;
    },
    running(id) {
      return inFlight.has(id);
    },
  };
}
